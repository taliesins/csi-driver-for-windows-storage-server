package iscsi

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	corelisters "k8s.io/client-go/listers/core/v1"
	storagelisters "k8s.io/client-go/listers/storage/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	registrationv1 "k8s.io/kubelet/pkg/apis/pluginregistration/v1"

	"google.golang.org/grpc"
)

const (
	defaultControllerLivenessPort = 29752
	defaultNodeLivenessPort       = 29753
	defaultKubernetesResyncPeriod = 5 * time.Second

	pvProtectionFinalizer = "windows-storage.csi.windows.microsoft.com/pv-protection"
	vaProtectionFinalizer = "windows-storage.csi.windows.microsoft.com/volumeattachment-protection"

	selectedNodeAnnotation = "volume.kubernetes.io/selected-node"
	csiParamPrefix         = "csi.storage.k8s.io/"

	internalProvisionerPVKeyPrefix  = "pv/"
	internalProvisionerPVCKeyPrefix = "pvc/"
)

type InternalKubernetesConfig struct {
	Enabled                 bool
	ControllerPublish       bool
	LivenessPort            int
	KubeletRegistrationPath string
	ResyncPeriod            time.Duration
}

var newKubernetesClientForRun = newInClusterKubernetesClient

func (d *driver) debugf(format string, args ...interface{}) {
	if d != nil && d.debug {
		klog.Infof("DEBUG: "+format, args...)
	}
}

func (d *driver) startInternalKubernetes(ctx context.Context) {
	cfg := d.internalKubernetes.withDefaults(d.mode)
	if !cfg.Enabled {
		return
	}
	if cfg.LivenessPort > 0 {
		startInternalHealthServer(ctx, cfg.LivenessPort)
	}

	switch d.mode {
	case DriverModeController:
		client, err := newKubernetesClientForRun()
		if err != nil {
			klog.Fatalf("create Kubernetes client for built-in CSI controllers: %v", err)
		}
		d.startInternalControllerReconcilers(ctx, client, cfg)
	case DriverModeNode:
		if err := d.startInternalNodeRegistrar(ctx, cfg.KubeletRegistrationPath); err != nil {
			klog.Fatalf("start built-in CSI node registrar: %v", err)
		}
	}
}

func (cfg InternalKubernetesConfig) withDefaults(mode DriverMode) InternalKubernetesConfig {
	if cfg.ResyncPeriod <= 0 {
		cfg.ResyncPeriod = defaultKubernetesResyncPeriod
	}
	if cfg.LivenessPort == 0 {
		if mode == DriverModeController {
			cfg.LivenessPort = defaultControllerLivenessPort
		} else {
			cfg.LivenessPort = defaultNodeLivenessPort
		}
	}
	return cfg
}

func newInClusterKubernetesClient() (kubernetes.Interface, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(config)
}

func startInternalHealthServer(ctx context.Context, port int) {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	go func() {
		klog.Infof("starting built-in liveness endpoint: port=%d", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			klog.Fatalf("built-in liveness endpoint failed: %v", err)
		}
	}()
}

func (d *driver) startInternalControllerReconcilers(ctx context.Context, client kubernetes.Interface, cfg InternalKubernetesConfig) {
	factory := informers.NewSharedInformerFactory(client, cfg.ResyncPeriod)
	pvInformer := factory.Core().V1().PersistentVolumes()
	pvcInformer := factory.Core().V1().PersistentVolumeClaims()
	vaInformer := factory.Storage().V1().VolumeAttachments()

	provisionerQueue := workqueue.NewTypedRateLimitingQueueWithConfig(workqueue.DefaultTypedControllerRateLimiter[string](), workqueue.TypedRateLimitingQueueConfig[string]{Name: "windows-storage-internal-provisioner"})
	attacherQueue := workqueue.NewTypedRateLimitingQueueWithConfig(workqueue.DefaultTypedControllerRateLimiter[string](), workqueue.TypedRateLimitingQueueConfig[string]{Name: "windows-storage-internal-attacher"})

	if _, err := pvInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { d.enqueueProvisionerPV(provisionerQueue, obj) },
		UpdateFunc: func(_, obj interface{}) { d.enqueueProvisionerPV(provisionerQueue, obj) },
		DeleteFunc: func(obj interface{}) { d.enqueueProvisionerPV(provisionerQueue, obj) },
	}); err != nil {
		klog.Fatalf("register built-in provisioner PV informer handler: %v", err)
	}
	if _, err := pvcInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { d.enqueueProvisionerPVC(provisionerQueue, obj) },
		UpdateFunc: func(_, obj interface{}) { d.enqueueProvisionerPVC(provisionerQueue, obj) },
		DeleteFunc: func(obj interface{}) { d.enqueueProvisionerPVC(provisionerQueue, obj) },
	}); err != nil {
		klog.Fatalf("register built-in provisioner PVC informer handler: %v", err)
	}
	if _, err := vaInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			d.enqueueProvisionerPVForVolumeAttachment(provisionerQueue, obj)
			if cfg.ControllerPublish {
				d.enqueueAttacherVolumeAttachment(attacherQueue, obj)
			}
		},
		UpdateFunc: func(_, obj interface{}) {
			d.enqueueProvisionerPVForVolumeAttachment(provisionerQueue, obj)
			if cfg.ControllerPublish {
				d.enqueueAttacherVolumeAttachment(attacherQueue, obj)
			}
		},
		DeleteFunc: func(obj interface{}) {
			d.enqueueProvisionerPVForVolumeAttachment(provisionerQueue, obj)
			if cfg.ControllerPublish {
				d.enqueueAttacherVolumeAttachment(attacherQueue, obj)
			}
		},
	}); err != nil {
		klog.Fatalf("register built-in VolumeAttachment informer handler: %v", err)
	}

	klog.Infof("starting built-in CSI provisioner informers: driver=%q resync=%s", d.name, cfg.ResyncPeriod)
	factory.Start(ctx.Done())
	if !cache.WaitForCacheSync(ctx.Done(), pvInformer.Informer().HasSynced, pvcInformer.Informer().HasSynced, vaInformer.Informer().HasSynced) {
		klog.Warningf("built-in CSI controller informer cache sync stopped: driver=%q", d.name)
		provisionerQueue.ShutDown()
		attacherQueue.ShutDown()
		return
	}

	go func() {
		<-ctx.Done()
		provisionerQueue.ShutDown()
		attacherQueue.ShutDown()
	}()
	go d.runInternalProvisioner(ctx, client, pvInformer.Lister(), pvcInformer.Lister(), vaInformer.Lister(), provisionerQueue)
	if cfg.ControllerPublish {
		klog.Infof("starting built-in CSI attacher informers: driver=%q resync=%s", d.name, cfg.ResyncPeriod)
		go d.runInternalAttacher(ctx, client, pvInformer.Lister(), vaInformer.Lister(), attacherQueue)
	}
}

func (d *driver) enqueueProvisionerPV(queue workqueue.TypedRateLimitingInterface[string], obj interface{}) {
	pv, ok := persistentVolumeFromEventObject(obj)
	if !ok || pv.Name == "" {
		return
	}
	queue.Add(internalProvisionerPVKeyPrefix + pv.Name)
}

func (d *driver) enqueueProvisionerPVC(queue workqueue.TypedRateLimitingInterface[string], obj interface{}) {
	pvc, ok := persistentVolumeClaimFromEventObject(obj)
	if !ok {
		return
	}
	key, err := cache.MetaNamespaceKeyFunc(pvc)
	if err != nil {
		klog.Warningf("built-in provisioner: build PVC queue key failed: namespace=%q pvc=%q error=%v", pvc.Namespace, pvc.Name, err)
		return
	}
	queue.Add(internalProvisionerPVCKeyPrefix + key)
}

func (d *driver) enqueueProvisionerPVForVolumeAttachment(queue workqueue.TypedRateLimitingInterface[string], obj interface{}) {
	va, ok := volumeAttachmentFromEventObject(obj)
	if !ok || va.Spec.Attacher != d.name || va.Spec.Source.PersistentVolumeName == nil {
		return
	}
	pvName := strings.TrimSpace(*va.Spec.Source.PersistentVolumeName)
	if pvName == "" {
		return
	}
	queue.Add(internalProvisionerPVKeyPrefix + pvName)
}

func (d *driver) enqueueAttacherVolumeAttachment(queue workqueue.TypedRateLimitingInterface[string], obj interface{}) {
	va, ok := volumeAttachmentFromEventObject(obj)
	if !ok || va.Spec.Attacher != d.name || va.Name == "" {
		return
	}
	queue.Add(va.Name)
}

func persistentVolumeFromEventObject(obj interface{}) (*v1.PersistentVolume, bool) {
	if tombstone, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		obj = tombstone.Obj
	}
	pv, ok := obj.(*v1.PersistentVolume)
	return pv, ok
}

func persistentVolumeClaimFromEventObject(obj interface{}) (*v1.PersistentVolumeClaim, bool) {
	if tombstone, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		obj = tombstone.Obj
	}
	pvc, ok := obj.(*v1.PersistentVolumeClaim)
	return pvc, ok
}

func volumeAttachmentFromEventObject(obj interface{}) (*storagev1.VolumeAttachment, bool) {
	if tombstone, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		obj = tombstone.Obj
	}
	va, ok := obj.(*storagev1.VolumeAttachment)
	return va, ok
}

func (d *driver) runInternalProvisioner(ctx context.Context, client kubernetes.Interface, pvLister corelisters.PersistentVolumeLister, pvcLister corelisters.PersistentVolumeClaimLister, vaLister storagelisters.VolumeAttachmentLister, queue workqueue.TypedRateLimitingInterface[string]) {
	for {
		key, shutdown := queue.Get()
		if shutdown {
			klog.Infof("stopping built-in CSI provisioner")
			return
		}
		err := d.reconcileInternalProvisioner(ctx, client, pvLister, pvcLister, vaLister, key)
		if err != nil && ctx.Err() == nil {
			klog.Warningf("built-in provisioner: reconcile failed: key=%q error=%v", key, err)
			queue.AddRateLimited(key)
		} else {
			queue.Forget(key)
		}
		queue.Done(key)
	}
}

func (d *driver) reconcileInternalProvisioner(ctx context.Context, client kubernetes.Interface, pvLister corelisters.PersistentVolumeLister, pvcLister corelisters.PersistentVolumeClaimLister, vaLister storagelisters.VolumeAttachmentLister, key string) error {
	switch {
	case strings.HasPrefix(key, internalProvisionerPVKeyPrefix):
		pvName := strings.TrimSpace(strings.TrimPrefix(key, internalProvisionerPVKeyPrefix))
		if pvName == "" {
			return nil
		}
		pv, err := pvLister.Get(pvName)
		if apierrors.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("get cached PersistentVolume %q: %w", pvName, err)
		}
		return d.reconcilePersistentVolumeDeletion(ctx, client, vaLister, pv)
	case strings.HasPrefix(key, internalProvisionerPVCKeyPrefix):
		nsName := strings.TrimPrefix(key, internalProvisionerPVCKeyPrefix)
		namespace, name, err := cache.SplitMetaNamespaceKey(nsName)
		if err != nil {
			return fmt.Errorf("split PersistentVolumeClaim key %q: %w", nsName, err)
		}
		pvc, err := pvcLister.PersistentVolumeClaims(namespace).Get(name)
		if apierrors.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("get cached PersistentVolumeClaim %q: %w", nsName, err)
		}
		return d.reconcilePersistentVolumeClaim(ctx, client, pvc)
	default:
		return fmt.Errorf("unknown provisioner queue key %q", key)
	}
}

func (d *driver) reconcilePersistentVolumeClaim(ctx context.Context, client kubernetes.Interface, pvc *v1.PersistentVolumeClaim) error {
	if pvc.DeletionTimestamp != nil || pvc.Spec.VolumeName != "" {
		d.debugf("built-in provisioner: skipping PVC: namespace=%q pvc=%q phase=%s volumeName=%q deleting=%t", pvc.Namespace, pvc.Name, pvc.Status.Phase, pvc.Spec.VolumeName, pvc.DeletionTimestamp != nil)
		return nil
	}
	scName := storageClassNameForPVC(pvc)
	if scName == "" {
		d.debugf("built-in provisioner: skipping PVC without storageClassName: namespace=%q pvc=%q", pvc.Namespace, pvc.Name)
		return nil
	}
	sc, err := client.StorageV1().StorageClasses().Get(ctx, scName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get StorageClass %q: %w", scName, err)
	}
	if sc.Provisioner != d.name {
		d.debugf("built-in provisioner: skipping PVC for another provisioner: namespace=%q pvc=%q storageClass=%q provisioner=%q", pvc.Namespace, pvc.Name, sc.Name, sc.Provisioner)
		return nil
	}
	if sc.VolumeBindingMode != nil && *sc.VolumeBindingMode == storagev1.VolumeBindingWaitForFirstConsumer && strings.TrimSpace(pvc.Annotations[selectedNodeAnnotation]) == "" {
		d.debugf("built-in provisioner: waiting for selected node: namespace=%q pvc=%q storageClass=%q", pvc.Namespace, pvc.Name, sc.Name)
		return nil
	}

	pvName := persistentVolumeNameForClaim(pvc)
	if _, err := client.CoreV1().PersistentVolumes().Get(ctx, pvName, metav1.GetOptions{}); err == nil {
		d.debugf("built-in provisioner: PV already exists for PVC: namespace=%q pvc=%q pv=%q", pvc.Namespace, pvc.Name, pvName)
		return nil
	} else if !apierrors.IsNotFound(err) {
		return fmt.Errorf("get PersistentVolume %q: %w", pvName, err)
	}

	params := createVolumeParameters(sc.Parameters)
	secrets, err := secretDataForOperation(ctx, client, sc.Parameters, "provisioner", pvc.Namespace, pvc.Name, pvName)
	if err != nil {
		return err
	}
	capacity := requestedStorageBytes(pvc)
	volumeCaps := volumeCapabilitiesForPVC(pvc, sc)
	contentSource, err := volumeContentSourceForPVC(ctx, client, d.name, pvc)
	if err != nil {
		return err
	}
	klog.Infof("built-in provisioner: creating volume: namespace=%q pvc=%q pv=%q storageClass=%q requestedBytes=%d reclaimPolicy=%s selectedNode=%q", pvc.Namespace, pvc.Name, pvName, sc.Name, capacity, reclaimPolicyForStorageClass(sc), pvc.Annotations[selectedNodeAnnotation])
	d.debugf("built-in provisioner: CreateVolume inputs: pv=%q parameterKeys=%q secretKeys=%q capabilities=%d", pvName, sortedMapKeys(params), sortedMapKeys(secrets), len(volumeCaps))

	resp, err := NewControllerServer(d).CreateVolume(ctx, &csi.CreateVolumeRequest{
		Name:                pvName,
		CapacityRange:       &csi.CapacityRange{RequiredBytes: capacity},
		VolumeCapabilities:  volumeCaps,
		Parameters:          params,
		Secrets:             secrets,
		VolumeContentSource: contentSource,
	})
	if err != nil {
		return fmt.Errorf("CreateVolume: %w", err)
	}
	pv := persistentVolumeForClaim(d.name, pvc, sc, pvName, resp.GetVolume())
	_, err = client.CoreV1().PersistentVolumes().Create(ctx, pv, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	if err != nil {
		volumeID := resp.GetVolume().GetVolumeId()
		if volumeID != "" {
			klog.Warningf("built-in provisioner: PV create failed after backend volume creation; rolling back backend volume: namespace=%q pvc=%q pv=%q volumeHandle=%q error=%v", pvc.Namespace, pvc.Name, pvName, volumeID, err)
			if _, rollbackErr := NewControllerServer(d).DeleteVolume(ctx, &csi.DeleteVolumeRequest{VolumeId: volumeID}); rollbackErr != nil {
				return fmt.Errorf("create PersistentVolume %q: %w (rollback DeleteVolume %q failed: %v)", pvName, err, volumeID, rollbackErr)
			}
			klog.Infof("built-in provisioner: rolled back backend volume after PV create failure: namespace=%q pvc=%q pv=%q volumeHandle=%q", pvc.Namespace, pvc.Name, pvName, volumeID)
		}
		return fmt.Errorf("create PersistentVolume %q: %w", pvName, err)
	}
	klog.Infof("built-in provisioner: volume created and PV published: namespace=%q pvc=%q pv=%q volumeHandle=%q capacityBytes=%d", pvc.Namespace, pvc.Name, pvName, resp.GetVolume().GetVolumeId(), resp.GetVolume().GetCapacityBytes())
	return nil
}

func (d *driver) reconcilePersistentVolumeDeletion(ctx context.Context, client kubernetes.Interface, vaLister storagelisters.VolumeAttachmentLister, pv *v1.PersistentVolume) error {
	if pv.Spec.CSI == nil || pv.Spec.CSI.Driver != d.name {
		d.debugf("built-in provisioner: skipping PV for another driver: pv=%q", pv.Name)
		return nil
	}
	reclaimPolicy := pv.Spec.PersistentVolumeReclaimPolicy
	deleting := pv.DeletionTimestamp != nil
	released := pv.Status.Phase == v1.VolumeReleased
	if !deleting && !released {
		return nil
	}
	if reclaimPolicy != v1.PersistentVolumeReclaimDelete {
		d.debugf("built-in provisioner: PV is not eligible for backend deletion: pv=%q phase=%s reclaimPolicy=%s deleting=%t", pv.Name, pv.Status.Phase, reclaimPolicy, deleting)
		if deleting && hasString(pv.Finalizers, pvProtectionFinalizer) {
			return updatePVFinalizers(ctx, client, pv, removeString(pv.Finalizers, pvProtectionFinalizer))
		}
		return nil
	}

	volumeID := ""
	if pv.Spec.CSI != nil {
		volumeID = pv.Spec.CSI.VolumeHandle
	}
	if volumeID == "" {
		return fmt.Errorf("PV %q is missing CSI volume handle", pv.Name)
	}
	hasAttachment, err := d.volumeAttachmentExistsForPV(vaLister, pv.Name)
	if err != nil {
		return err
	}
	if hasAttachment {
		klog.Infof("built-in provisioner: deferring backend volume deletion until VolumeAttachment is removed: pv=%q phase=%s deleting=%t volumeHandle=%q", pv.Name, pv.Status.Phase, deleting, volumeID)
		return nil
	}
	klog.Infof("built-in provisioner: deleting backend volume: pv=%q phase=%s deleting=%t volumeHandle=%q", pv.Name, pv.Status.Phase, deleting, volumeID)
	if _, err := NewControllerServer(d).DeleteVolume(ctx, &csi.DeleteVolumeRequest{VolumeId: volumeID}); err != nil {
		return fmt.Errorf("DeleteVolume: %w", err)
	}
	if hasString(pv.Finalizers, pvProtectionFinalizer) {
		if err := updatePVFinalizers(ctx, client, pv, removeString(pv.Finalizers, pvProtectionFinalizer)); err != nil {
			return err
		}
	}
	if !deleting {
		if err := client.CoreV1().PersistentVolumes().Delete(ctx, pv.Name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("delete PersistentVolume %q: %w", pv.Name, err)
		}
	}
	klog.Infof("built-in provisioner: backend volume deleted: pv=%q volumeHandle=%q", pv.Name, volumeID)
	return nil
}

func (d *driver) runInternalAttacher(ctx context.Context, client kubernetes.Interface, pvLister corelisters.PersistentVolumeLister, vaLister storagelisters.VolumeAttachmentLister, queue workqueue.TypedRateLimitingInterface[string]) {
	for {
		key, shutdown := queue.Get()
		if shutdown {
			klog.Infof("stopping built-in CSI attacher")
			return
		}
		err := d.reconcileInternalAttacher(ctx, client, pvLister, vaLister, key)
		if err != nil && ctx.Err() == nil {
			klog.Warningf("built-in attacher: reconcile failed: volumeAttachment=%q error=%v", key, err)
			queue.AddRateLimited(key)
		} else {
			queue.Forget(key)
		}
		queue.Done(key)
	}
}

func (d *driver) reconcileInternalAttacher(ctx context.Context, client kubernetes.Interface, pvLister corelisters.PersistentVolumeLister, vaLister storagelisters.VolumeAttachmentLister, key string) error {
	vaName := strings.TrimSpace(key)
	if vaName == "" {
		return nil
	}
	va, err := vaLister.Get(vaName)
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("get cached VolumeAttachment %q: %w", vaName, err)
	}
	if va.Spec.Attacher != d.name {
		d.debugf("built-in attacher: skipping VolumeAttachment for another attacher: volumeAttachment=%q attacher=%q", va.Name, va.Spec.Attacher)
		return nil
	}
	return d.reconcileVolumeAttachment(ctx, client, pvLister, va)
}

func (d *driver) reconcileVolumeAttachment(ctx context.Context, client kubernetes.Interface, pvLister corelisters.PersistentVolumeLister, va *storagev1.VolumeAttachment) error {
	pvName := ""
	if va.Spec.Source.PersistentVolumeName != nil {
		pvName = strings.TrimSpace(*va.Spec.Source.PersistentVolumeName)
	}
	if pvName == "" {
		d.debugf("built-in attacher: skipping VolumeAttachment without PV source: volumeAttachment=%q", va.Name)
		return nil
	}
	pv, err := pvLister.Get(pvName)
	if err != nil {
		return fmt.Errorf("get cached PersistentVolume %q: %w", pvName, err)
	}
	if pv.Spec.CSI == nil || pv.Spec.CSI.Driver != d.name {
		d.debugf("built-in attacher: skipping VolumeAttachment for another driver: volumeAttachment=%q pv=%q", va.Name, pvName)
		return nil
	}

	if va.DeletionTimestamp != nil {
		if !hasString(va.Finalizers, vaProtectionFinalizer) {
			return nil
		}
		klog.Infof("built-in attacher: unpublishing volume: volumeAttachment=%q pv=%q node=%q volumeHandle=%q", va.Name, pvName, va.Spec.NodeName, pv.Spec.CSI.VolumeHandle)
		if _, err := NewControllerServer(d).ControllerUnpublishVolume(ctx, &csi.ControllerUnpublishVolumeRequest{
			VolumeId: pv.Spec.CSI.VolumeHandle,
			NodeId:   va.Spec.NodeName,
		}); err != nil {
			return fmt.Errorf("ControllerUnpublishVolume: %w", err)
		}
		if err := updateVAFinalizers(ctx, client, va, removeString(va.Finalizers, vaProtectionFinalizer)); err != nil {
			return err
		}
		klog.Infof("built-in attacher: volume unpublished: volumeAttachment=%q pv=%q node=%q", va.Name, pvName, va.Spec.NodeName)
		return nil
	}

	if va.Status.Attached {
		d.debugf("built-in attacher: VolumeAttachment already attached: volumeAttachment=%q pv=%q node=%q", va.Name, pvName, va.Spec.NodeName)
		return nil
	}
	if !hasString(va.Finalizers, vaProtectionFinalizer) {
		if err := updateVAFinalizers(ctx, client, va, append(append([]string{}, va.Finalizers...), vaProtectionFinalizer)); err != nil {
			return err
		}
		latest, err := client.StorageV1().VolumeAttachments().Get(ctx, va.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		va = latest
	}

	secrets, err := secretDataFromRef(ctx, client, pv.Spec.CSI.ControllerPublishSecretRef)
	if err != nil {
		return err
	}
	volumeCap := volumeCapabilityForPV(pv)
	klog.Infof("built-in attacher: publishing volume: volumeAttachment=%q pv=%q node=%q volumeHandle=%q", va.Name, pvName, va.Spec.NodeName, pv.Spec.CSI.VolumeHandle)
	d.debugf("built-in attacher: ControllerPublishVolume inputs: volumeAttachment=%q secretKeys=%q accessMode=%s", va.Name, sortedMapKeys(secrets), volumeCap.GetAccessMode().GetMode().String())
	resp, err := NewControllerServer(d).ControllerPublishVolume(ctx, &csi.ControllerPublishVolumeRequest{
		VolumeId:         pv.Spec.CSI.VolumeHandle,
		NodeId:           va.Spec.NodeName,
		VolumeCapability: volumeCap,
		Readonly:         false,
		Secrets:          secrets,
		VolumeContext:    pv.Spec.CSI.VolumeAttributes,
	})
	if err != nil {
		return fmt.Errorf("ControllerPublishVolume: %w", err)
	}
	vaCopy := va.DeepCopy()
	vaCopy.Status.Attached = true
	vaCopy.Status.AttachmentMetadata = resp.GetPublishContext()
	if _, err := client.StorageV1().VolumeAttachments().UpdateStatus(ctx, vaCopy, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update VolumeAttachment status %q: %w", va.Name, err)
	}
	klog.Infof("built-in attacher: volume published: volumeAttachment=%q pv=%q node=%q metadataKeys=%q", va.Name, pvName, va.Spec.NodeName, sortedMapKeys(resp.GetPublishContext()))
	return nil
}

type internalRegistrationServer struct {
	registrationv1.UnimplementedRegistrationServer
	driverName string
	endpoint   string
}

func (s *internalRegistrationServer) GetInfo(context.Context, *registrationv1.InfoRequest) (*registrationv1.PluginInfo, error) {
	klog.Infof("built-in node registrar: kubelet requested plugin info: driver=%q endpoint=%q", s.driverName, s.endpoint)
	return &registrationv1.PluginInfo{
		Type:              registrationv1.CSIPlugin,
		Name:              s.driverName,
		Endpoint:          s.endpoint,
		SupportedVersions: []string{"1.0.0"},
	}, nil
}

func (s *internalRegistrationServer) NotifyRegistrationStatus(_ context.Context, status *registrationv1.RegistrationStatus) (*registrationv1.RegistrationStatusResponse, error) {
	if status.GetPluginRegistered() {
		klog.Infof("built-in node registrar: kubelet registered CSI plugin: driver=%q", s.driverName)
	} else {
		klog.Warningf("built-in node registrar: kubelet failed to register CSI plugin: driver=%q error=%q", s.driverName, status.GetError())
	}
	return &registrationv1.RegistrationStatusResponse{}, nil
}

func (d *driver) startInternalNodeRegistrar(ctx context.Context, kubeletRegistrationPath string) error {
	kubeletRegistrationPath = strings.TrimSpace(kubeletRegistrationPath)
	if kubeletRegistrationPath == "" {
		return fmt.Errorf("kubelet registration path is required")
	}
	socketPath := filepath.Join("/registration", d.name+"-reg.sock")
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove stale registration socket %q: %w", socketPath, err)
	}
	if err := os.MkdirAll(filepath.Dir(socketPath), 0o755); err != nil {
		return fmt.Errorf("create registration socket directory: %w", err)
	}
	lc := net.ListenConfig{}
	listener, err := lc.Listen(ctx, "unix", socketPath)
	if err != nil {
		return fmt.Errorf("listen on registration socket %q: %w", socketPath, err)
	}
	server := grpc.NewServer()
	registrationv1.RegisterRegistrationServer(server, &internalRegistrationServer{
		driverName: d.name,
		endpoint:   kubeletRegistrationPath,
	})
	go func() {
		<-ctx.Done()
		server.GracefulStop()
		_ = listener.Close()
		_ = os.Remove(socketPath)
	}()
	go func() {
		klog.Infof("starting built-in node registrar: socket=%q driver=%q endpoint=%q", socketPath, d.name, kubeletRegistrationPath)
		if err := server.Serve(listener); err != nil && ctx.Err() == nil {
			klog.Fatalf("built-in node registrar failed: %v", err)
		}
	}()
	return nil
}

func persistentVolumeNameForClaim(pvc *v1.PersistentVolumeClaim) string {
	return "pvc-" + string(pvc.UID)
}

func storageClassNameForPVC(pvc *v1.PersistentVolumeClaim) string {
	if pvc.Spec.StorageClassName == nil {
		return ""
	}
	return strings.TrimSpace(*pvc.Spec.StorageClassName)
}

func requestedStorageBytes(pvc *v1.PersistentVolumeClaim) int64 {
	if pvc == nil {
		return 0
	}
	if q, ok := pvc.Spec.Resources.Requests[v1.ResourceStorage]; ok {
		return q.Value()
	}
	return 0
}

func createVolumeParameters(params map[string]string) map[string]string {
	out := map[string]string{}
	for key, value := range params {
		if strings.HasPrefix(key, csiParamPrefix) {
			continue
		}
		out[key] = value
	}
	return out
}

func volumeCapabilitiesForPVC(pvc *v1.PersistentVolumeClaim, sc *storagev1.StorageClass) []*csi.VolumeCapability {
	return []*csi.VolumeCapability{volumeCapability(pvc.Spec.VolumeMode, pvc.Spec.AccessModes, fsTypeForStorageClass(sc), sc.MountOptions)}
}

func volumeContentSourceForPVC(ctx context.Context, client kubernetes.Interface, driverName string, pvc *v1.PersistentVolumeClaim) (*csi.VolumeContentSource, error) {
	ref := pvcDataSourceRef(pvc)
	if ref == nil || strings.TrimSpace(ref.Name) == "" {
		return nil, nil
	}
	group := ""
	if ref.APIGroup != nil {
		group = strings.TrimSpace(*ref.APIGroup)
	}
	kind := strings.TrimSpace(ref.Kind)
	namespace := pvc.Namespace
	if ref.Namespace != nil && strings.TrimSpace(*ref.Namespace) != "" {
		namespace = strings.TrimSpace(*ref.Namespace)
	}
	if namespace != pvc.Namespace {
		return nil, fmt.Errorf("PVC %s/%s dataSourceRef %s/%s is cross-namespace; built-in provisioner only supports same-namespace data sources", pvc.Namespace, pvc.Name, kind, ref.Name)
	}
	if group == "" && kind == "PersistentVolumeClaim" {
		sourcePVC, err := client.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, ref.Name, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("get source PVC %s/%s: %w", namespace, ref.Name, err)
		}
		if strings.TrimSpace(sourcePVC.Spec.VolumeName) == "" {
			return nil, fmt.Errorf("source PVC %s/%s is not bound", namespace, ref.Name)
		}
		sourcePV, err := client.CoreV1().PersistentVolumes().Get(ctx, sourcePVC.Spec.VolumeName, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("get source PersistentVolume %q: %w", sourcePVC.Spec.VolumeName, err)
		}
		if sourcePV.Spec.CSI == nil || sourcePV.Spec.CSI.Driver != driverName || strings.TrimSpace(sourcePV.Spec.CSI.VolumeHandle) == "" {
			return nil, fmt.Errorf("source PersistentVolume %q is not a CSI volume for driver %q", sourcePV.Name, driverName)
		}
		return &csi.VolumeContentSource{
			Type: &csi.VolumeContentSource_Volume{
				Volume: &csi.VolumeContentSource_VolumeSource{VolumeId: sourcePV.Spec.CSI.VolumeHandle},
			},
		}, nil
	}
	if group == "snapshot.storage.k8s.io" && kind == "VolumeSnapshot" {
		return nil, fmt.Errorf("VolumeSnapshot dataSource %s/%s is not supported by the built-in provisioner yet; create snapshots through the CSI snapshot API or provision a new empty PVC", namespace, ref.Name)
	}
	return nil, fmt.Errorf("PVC %s/%s has unsupported dataSource kind=%q apiGroup=%q", pvc.Namespace, pvc.Name, kind, group)
}

func pvcDataSourceRef(pvc *v1.PersistentVolumeClaim) *v1.TypedObjectReference {
	if pvc == nil {
		return nil
	}
	if pvc.Spec.DataSourceRef != nil {
		return pvc.Spec.DataSourceRef
	}
	if pvc.Spec.DataSource == nil {
		return nil
	}
	return &v1.TypedObjectReference{
		APIGroup: pvc.Spec.DataSource.APIGroup,
		Kind:     pvc.Spec.DataSource.Kind,
		Name:     pvc.Spec.DataSource.Name,
	}
}

func volumeCapabilityForPV(pv *v1.PersistentVolume) *csi.VolumeCapability {
	fsType := ""
	if pv.Spec.CSI != nil {
		fsType = pv.Spec.CSI.FSType
	}
	return volumeCapability(pv.Spec.VolumeMode, pv.Spec.AccessModes, fsType, pv.Spec.MountOptions)
}

func volumeCapability(mode *v1.PersistentVolumeMode, accessModes []v1.PersistentVolumeAccessMode, fsType string, mountOptions []string) *csi.VolumeCapability {
	accessMode := csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER
	if len(accessModes) > 0 {
		switch accessModes[0] {
		case v1.ReadOnlyMany:
			accessMode = csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY
		case v1.ReadWriteMany:
			accessMode = csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER
		case v1.ReadWriteOncePod:
			accessMode = csi.VolumeCapability_AccessMode_SINGLE_NODE_SINGLE_WRITER
		}
	}
	capability := &csi.VolumeCapability{
		AccessMode: &csi.VolumeCapability_AccessMode{Mode: accessMode},
	}
	if mode != nil && *mode == v1.PersistentVolumeBlock {
		capability.AccessType = &csi.VolumeCapability_Block{Block: &csi.VolumeCapability_BlockVolume{}}
		return capability
	}
	capability.AccessType = &csi.VolumeCapability_Mount{
		Mount: &csi.VolumeCapability_MountVolume{
			FsType:     fsType,
			MountFlags: append([]string{}, mountOptions...),
		},
	}
	return capability
}

func fsTypeForStorageClass(sc *storagev1.StorageClass) string {
	if sc == nil {
		return ""
	}
	for _, key := range []string{csiParamPrefix + "fstype", "fsType", "fstype"} {
		if value := strings.TrimSpace(sc.Parameters[key]); value != "" {
			return value
		}
	}
	return ""
}

func persistentVolumeForClaim(driverName string, pvc *v1.PersistentVolumeClaim, sc *storagev1.StorageClass, pvName string, volume *csi.Volume) *v1.PersistentVolume {
	reclaimPolicy := reclaimPolicyForStorageClass(sc)
	volumeMode := v1.PersistentVolumeFilesystem
	if pvc.Spec.VolumeMode != nil {
		volumeMode = *pvc.Spec.VolumeMode
	}
	finalizers := []string{}
	if reclaimPolicy == v1.PersistentVolumeReclaimDelete {
		finalizers = append(finalizers, pvProtectionFinalizer)
	}
	attrs := map[string]string{}
	for key, value := range volume.GetVolumeContext() {
		attrs[key] = value
	}
	pv := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name:       pvName,
			Finalizers: finalizers,
		},
		Spec: v1.PersistentVolumeSpec{
			Capacity: v1.ResourceList{
				v1.ResourceStorage: *resource.NewQuantity(volume.GetCapacityBytes(), resource.BinarySI),
			},
			AccessModes:                   append([]v1.PersistentVolumeAccessMode{}, pvc.Spec.AccessModes...),
			PersistentVolumeReclaimPolicy: reclaimPolicy,
			StorageClassName:              sc.Name,
			MountOptions:                  append([]string{}, sc.MountOptions...),
			VolumeMode:                    &volumeMode,
			ClaimRef: &v1.ObjectReference{
				APIVersion: "v1",
				Kind:       "PersistentVolumeClaim",
				Namespace:  pvc.Namespace,
				Name:       pvc.Name,
				UID:        pvc.UID,
			},
			PersistentVolumeSource: v1.PersistentVolumeSource{
				CSI: &v1.CSIPersistentVolumeSource{
					Driver:                     driverName,
					VolumeHandle:               volume.GetVolumeId(),
					FSType:                     fsTypeForStorageClass(sc),
					VolumeAttributes:           attrs,
					ControllerPublishSecretRef: secretRefForOperation(sc.Parameters, "controller-publish", pvc.Namespace, pvc.Name, pvName),
					NodeStageSecretRef:         secretRefForOperation(sc.Parameters, "node-stage", pvc.Namespace, pvc.Name, pvName),
					NodePublishSecretRef:       secretRefForOperation(sc.Parameters, "node-publish", pvc.Namespace, pvc.Name, pvName),
					ControllerExpandSecretRef:  secretRefForOperation(sc.Parameters, "controller-expand", pvc.Namespace, pvc.Name, pvName),
				},
			},
		},
	}
	return pv
}

func reclaimPolicyForStorageClass(sc *storagev1.StorageClass) v1.PersistentVolumeReclaimPolicy {
	if sc != nil && sc.ReclaimPolicy != nil {
		return *sc.ReclaimPolicy
	}
	return v1.PersistentVolumeReclaimDelete
}

func secretDataForOperation(ctx context.Context, client kubernetes.Interface, params map[string]string, operation, pvcNamespace, pvcName, pvName string) (map[string]string, error) {
	return secretDataFromRef(ctx, client, secretRefForOperation(params, operation, pvcNamespace, pvcName, pvName))
}

func secretRefForOperation(params map[string]string, operation, pvcNamespace, pvcName, pvName string) *v1.SecretReference {
	nameKeys := []string{csiParamPrefix + operation + "-secret-name", csiParamPrefix + "secret-name"}
	namespaceKeys := []string{csiParamPrefix + operation + "-secret-namespace", csiParamPrefix + "secret-namespace"}
	name := firstParam(params, nameKeys...)
	namespace := firstParam(params, namespaceKeys...)
	if name == "" {
		return nil
	}
	if namespace == "" {
		namespace = pvcNamespace
	}
	return &v1.SecretReference{
		Name:      expandSecretTemplate(name, pvcNamespace, pvcName, pvName),
		Namespace: expandSecretTemplate(namespace, pvcNamespace, pvcName, pvName),
	}
}

func firstParam(params map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(params[key]); value != "" {
			return value
		}
	}
	return ""
}

func expandSecretTemplate(value, pvcNamespace, pvcName, pvName string) string {
	replacer := strings.NewReplacer(
		"${pvc.namespace}", pvcNamespace,
		"${pvc.name}", pvcName,
		"${pv.name}", pvName,
	)
	return replacer.Replace(value)
}

func secretDataFromRef(ctx context.Context, client kubernetes.Interface, ref *v1.SecretReference) (map[string]string, error) {
	if ref == nil || strings.TrimSpace(ref.Name) == "" {
		return nil, nil
	}
	secret, err := client.CoreV1().Secrets(ref.Namespace).Get(ctx, ref.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get Secret %s/%s: %w", ref.Namespace, ref.Name, err)
	}
	data := make(map[string]string, len(secret.Data))
	for key, value := range secret.Data {
		data[key] = string(value)
	}
	return data, nil
}

func updatePVFinalizers(ctx context.Context, client kubernetes.Interface, pv *v1.PersistentVolume, finalizers []string) error {
	updated := pv.DeepCopy()
	updated.Finalizers = uniqueStrings(finalizers)
	if _, err := client.CoreV1().PersistentVolumes().Update(ctx, updated, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update PersistentVolume finalizers %q: %w", pv.Name, err)
	}
	return nil
}

func updateVAFinalizers(ctx context.Context, client kubernetes.Interface, va *storagev1.VolumeAttachment, finalizers []string) error {
	updated := va.DeepCopy()
	updated.Finalizers = uniqueStrings(finalizers)
	if _, err := client.StorageV1().VolumeAttachments().Update(ctx, updated, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update VolumeAttachment finalizers %q: %w", va.Name, err)
	}
	return nil
}

func (d *driver) volumeAttachmentExistsForPV(vaLister storagelisters.VolumeAttachmentLister, pvName string) (bool, error) {
	if vaLister == nil {
		return false, fmt.Errorf("VolumeAttachment lister is required")
	}
	list, err := vaLister.List(labels.Everything())
	if err != nil {
		return false, fmt.Errorf("list cached VolumeAttachments for PersistentVolume %q: %w", pvName, err)
	}
	for _, va := range list {
		if va.Spec.Attacher != d.name || va.Spec.Source.PersistentVolumeName == nil {
			continue
		}
		if strings.TrimSpace(*va.Spec.Source.PersistentVolumeName) == pvName {
			return true, nil
		}
	}
	return false, nil
}

func hasString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func removeString(values []string, needle string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value != needle {
			out = append(out, value)
		}
	}
	return out
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
