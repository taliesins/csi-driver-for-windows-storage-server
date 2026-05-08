package iscsi

import (
	"context"
	"errors"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	corelisters "k8s.io/client-go/listers/core/v1"
	storagelisters "k8s.io/client-go/listers/storage/v1"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
)

func TestCreateVolumeParametersStripsCSIKeys(t *testing.T) {
	params := createVolumeParameters(map[string]string{
		"protocol":     "iscsi",
		"targetPortal": "10.0.0.1",
		"csi.storage.k8s.io/provisioner-secret-name":      "chap",
		"csi.storage.k8s.io/provisioner-secret-namespace": "default",
	})

	assert.Equal(t, map[string]string{
		"protocol":     "iscsi",
		"targetPortal": "10.0.0.1",
	}, params)
}

func TestSecretRefForOperationExpandsTemplates(t *testing.T) {
	ref := secretRefForOperation(map[string]string{
		"csi.storage.k8s.io/controller-publish-secret-name":      "${pvc.name}-chap",
		"csi.storage.k8s.io/controller-publish-secret-namespace": "${pvc.namespace}",
	}, "controller-publish", "db", "data", "pvc-123")

	require.NotNil(t, ref)
	assert.Equal(t, "data-chap", ref.Name)
	assert.Equal(t, "db", ref.Namespace)
}

func TestSecretDataForOperationDoesNotExposeValuesInReference(t *testing.T) {
	params := map[string]string{
		"csi.storage.k8s.io/provisioner-secret-name":      "iscsi-chap",
		"csi.storage.k8s.io/provisioner-secret-namespace": "db",
	}
	client := fake.NewSimpleClientset(&v1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "iscsi-chap", Namespace: "db"},
		Data: map[string][]byte{
			"node.session.auth.username": []byte("dbnode01"),
			"node.session.auth.password": []byte("S3cret!P@ssw0rd"),
		},
	})

	ref := secretRefForOperation(params, "provisioner", "db", "data", "pvc-123")
	require.NotNil(t, ref)
	assert.Equal(t, "iscsi-chap", ref.Name)
	assert.Equal(t, "db", ref.Namespace)
	assert.NotContains(t, ref.Name, "dbnode01")
	assert.NotContains(t, ref.Name, "S3cret!P@ssw0rd")
	assert.NotContains(t, ref.Namespace, "dbnode01")
	assert.NotContains(t, ref.Namespace, "S3cret!P@ssw0rd")

	data, err := secretDataForOperation(context.Background(), client, params, "provisioner", "db", "data", "pvc-123")
	require.NoError(t, err)

	assert.Equal(t, "dbnode01", data["node.session.auth.username"])
	assert.Equal(t, "S3cret!P@ssw0rd", data["node.session.auth.password"])
}

func TestPersistentVolumeForClaimSetsCSIFieldsAndDeleteFinalizer(t *testing.T) {
	deletePolicy := v1.PersistentVolumeReclaimDelete
	sc := &storagev1.StorageClass{
		ObjectMeta:    metav1.ObjectMeta{Name: "iscsi"},
		ReclaimPolicy: &deletePolicy,
		Parameters: map[string]string{
			"csi.storage.k8s.io/node-stage-secret-name":      "iscsi-chap",
			"csi.storage.k8s.io/node-stage-secret-namespace": "${pvc.namespace}",
			"csi.storage.k8s.io/fstype":                      "xfs",
		},
		MountOptions: []string{"discard"},
	}
	pvc := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "data", Namespace: "db", UID: types.UID("claim-uid")},
		Spec: v1.PersistentVolumeClaimSpec{
			AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
			Resources: v1.VolumeResourceRequirements{
				Requests: v1.ResourceList{
					v1.ResourceStorage: resource.MustParse("1Gi"),
				},
			},
		},
	}
	volume := &csi.Volume{
		VolumeId:      "volume-handle",
		CapacityBytes: 1024,
		VolumeContext: map[string]string{
			"targetPortal": "10.0.0.1:3260",
			"iqn":          "iqn.2024-01.com.example:data",
			"lun":          "0",
		},
	}

	pv := persistentVolumeForClaim(driverName, pvc, sc, "pvc-claim-uid", volume)

	assert.Contains(t, pv.Finalizers, pvProtectionFinalizer)
	assert.Equal(t, "volume-handle", pv.Spec.CSI.VolumeHandle)
	assert.Equal(t, "xfs", pv.Spec.CSI.FSType)
	assert.Equal(t, "10.0.0.1:3260", pv.Spec.CSI.VolumeAttributes["targetPortal"])
	require.NotNil(t, pv.Spec.CSI.NodeStageSecretRef)
	assert.Equal(t, "iscsi-chap", pv.Spec.CSI.NodeStageSecretRef.Name)
	assert.Equal(t, "db", pv.Spec.CSI.NodeStageSecretRef.Namespace)
	assert.Equal(t, []string{"discard"}, pv.Spec.MountOptions)
}

func TestVolumeCapabilityForPVCMapsAccessModeAndMount(t *testing.T) {
	fsMode := v1.PersistentVolumeFilesystem
	capability := volumeCapability(&fsMode, []v1.PersistentVolumeAccessMode{v1.ReadWriteMany}, "ext4", []string{"noatime"})

	assert.Equal(t, csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER, capability.GetAccessMode().GetMode())
	assert.Equal(t, "ext4", capability.GetMount().GetFsType())
	assert.Equal(t, []string{"noatime"}, capability.GetMount().GetMountFlags())
}

func TestReconcilePersistentVolumeClaimRollsBackBackendVolumeWhenPVCreateFails(t *testing.T) {
	_, d, backend := newTestConsolidatedControllerServer(t)
	pvc := internalTestPVC("data", "db", "claim-uid", "iscsi")
	sc := internalTestStorageClass(d.name, "iscsi")
	client := fake.NewSimpleClientset(sc, pvc)
	client.PrependReactor("create", "persistentvolumes", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("api unavailable")
	})

	var deletedVHDXPaths []string
	backend.createVirtualDiskFn = func(ctx context.Context, name, parentDir string, sizeBytes int64) (string, int64, error) {
		return "D:\\vhdx\\" + name + ".vhdx", sizeBytes, nil
	}
	backend.ensureTargetFn = func(ctx context.Context, targetName, targetIQN string) (string, error) {
		return firstNonEmpty(targetIQN, targetName), nil
	}
	backend.mapDiskToTargetFn = func(ctx context.Context, targetName, vhdxPath string) (int32, error) {
		return 0, nil
	}
	backend.deleteVirtualDiskFn = func(ctx context.Context, vhdxPath string) error {
		deletedVHDXPaths = append(deletedVHDXPaths, vhdxPath)
		return nil
	}

	err := d.reconcilePersistentVolumeClaim(context.Background(), client, pvc)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "create PersistentVolume")
	assert.Equal(t, []string{"D:\\vhdx\\pvc-claim-uid.vhdx"}, deletedVHDXPaths)
}

func TestReconcilePersistentVolumeDeletionDefersWhileVolumeAttachmentExists(t *testing.T) {
	_, d, backend := newTestConsolidatedControllerServer(t)
	pv := internalTestReleasedPV(t, d.name)
	pvName := pv.Name
	va := &storagev1.VolumeAttachment{
		ObjectMeta: metav1.ObjectMeta{Name: "va-" + pv.Name},
		Spec: storagev1.VolumeAttachmentSpec{
			Attacher: d.name,
			NodeName: "iqn.2004-10.com.ubuntu:01:test",
			Source: storagev1.VolumeAttachmentSource{
				PersistentVolumeName: &pvName,
			},
		},
	}
	client := fake.NewSimpleClientset(pv, va)
	backend.deleteVirtualDiskFn = func(ctx context.Context, vhdxPath string) error {
		t.Fatalf("DeleteVolume should be deferred while a VolumeAttachment exists")
		return nil
	}

	err := d.reconcilePersistentVolumeDeletion(context.Background(), client, internalTestVolumeAttachmentLister(t, va), pv)

	require.NoError(t, err)
	_, err = client.CoreV1().PersistentVolumes().Get(context.Background(), pv.Name, metav1.GetOptions{})
	require.NoError(t, err)
}

func TestReconcilePersistentVolumeDeletionDeletesBackendWhenNoVolumeAttachmentExists(t *testing.T) {
	_, d, backend := newTestConsolidatedControllerServer(t)
	pv := internalTestReleasedPV(t, d.name)
	client := fake.NewSimpleClientset(pv)
	var deletedVHDXPaths []string
	backend.deleteVirtualDiskFn = func(ctx context.Context, vhdxPath string) error {
		deletedVHDXPaths = append(deletedVHDXPaths, vhdxPath)
		return nil
	}

	err := d.reconcilePersistentVolumeDeletion(context.Background(), client, internalTestVolumeAttachmentLister(t), pv)

	require.NoError(t, err)
	assert.Equal(t, []string{"D:\\vhdx\\test-volume.vhdx"}, deletedVHDXPaths)
	_, err = client.CoreV1().PersistentVolumes().Get(context.Background(), pv.Name, metav1.GetOptions{})
	assert.Error(t, err)
}

func TestReconcileVolumeAttachmentPublishesAndSetsAttachedStatus(t *testing.T) {
	_, d, backend := newTestConsolidatedControllerServer(t)
	pv := internalTestBoundPV(t, d.name)
	pvName := pv.Name
	va := &storagev1.VolumeAttachment{
		ObjectMeta: metav1.ObjectMeta{Name: "va-" + pv.Name},
		Spec: storagev1.VolumeAttachmentSpec{
			Attacher: d.name,
			NodeName: "iqn.2004-10.com.ubuntu:01:test",
			Source: storagev1.VolumeAttachmentSource{
				PersistentVolumeName: &pvName,
			},
		},
	}
	client := fake.NewSimpleClientset(pv, va)
	var allowedInitiators []string
	backend.allowInitiatorFn = func(ctx context.Context, targetName, initiatorIQN string) error {
		allowedInitiators = append(allowedInitiators, initiatorIQN)
		return nil
	}

	err := d.reconcileVolumeAttachment(context.Background(), client, internalTestPersistentVolumeLister(t, pv), va)

	require.NoError(t, err)
	assert.Equal(t, []string{"iqn.2004-10.com.ubuntu:01:test"}, allowedInitiators)
	updated, err := client.StorageV1().VolumeAttachments().Get(context.Background(), va.Name, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Contains(t, updated.Finalizers, vaProtectionFinalizer)
	assert.True(t, updated.Status.Attached)
	assert.Equal(t, "10.0.0.1:3260", updated.Status.AttachmentMetadata["targetPortal"])
}

func TestReconcileVolumeAttachmentUnpublishesAndRemovesFinalizer(t *testing.T) {
	_, d, backend := newTestConsolidatedControllerServer(t)
	pv := internalTestBoundPV(t, d.name)
	pvName := pv.Name
	now := metav1.Now()
	va := &storagev1.VolumeAttachment{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "va-" + pv.Name,
			Finalizers:        []string{vaProtectionFinalizer},
			DeletionTimestamp: &now,
		},
		Spec: storagev1.VolumeAttachmentSpec{
			Attacher: d.name,
			NodeName: "iqn.2004-10.com.ubuntu:01:test",
			Source: storagev1.VolumeAttachmentSource{
				PersistentVolumeName: &pvName,
			},
		},
		Status: storagev1.VolumeAttachmentStatus{Attached: true},
	}
	client := fake.NewSimpleClientset(pv, va)
	var deniedInitiators []string
	backend.denyInitiatorFn = func(ctx context.Context, targetName, initiatorIQN string) error {
		deniedInitiators = append(deniedInitiators, initiatorIQN)
		return nil
	}

	err := d.reconcileVolumeAttachment(context.Background(), client, internalTestPersistentVolumeLister(t, pv), va)

	require.NoError(t, err)
	assert.Equal(t, []string{"iqn.2004-10.com.ubuntu:01:test"}, deniedInitiators)
	updated, err := client.StorageV1().VolumeAttachments().Get(context.Background(), va.Name, metav1.GetOptions{})
	require.NoError(t, err)
	assert.NotContains(t, updated.Finalizers, vaProtectionFinalizer)
}

func TestReconcilePersistentVolumeClaimRejectsVolumeSnapshotDataSource(t *testing.T) {
	_, d, backend := newTestConsolidatedControllerServer(t)
	pvc := internalTestPVC("restore", "db", "restore-uid", "iscsi")
	apiGroup := "snapshot.storage.k8s.io"
	pvc.Spec.DataSource = &v1.TypedLocalObjectReference{
		APIGroup: &apiGroup,
		Kind:     "VolumeSnapshot",
		Name:     "snap-1",
	}
	sc := internalTestStorageClass(d.name, "iscsi")
	client := fake.NewSimpleClientset(sc, pvc)
	backend.createVirtualDiskFn = func(ctx context.Context, name, parentDir string, sizeBytes int64) (string, int64, error) {
		t.Fatalf("CreateVolume should not run for unsupported snapshot dataSource")
		return "", 0, nil
	}

	err := d.reconcilePersistentVolumeClaim(context.Background(), client, pvc)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "VolumeSnapshot dataSource")
}

func TestReconcilePersistentVolumeClaimRejectsPVCCloneWithoutCreatingBackendVolume(t *testing.T) {
	_, d, backend := newTestConsolidatedControllerServer(t)
	sourcePVC := internalTestPVC("source", "db", "source-uid", "iscsi")
	sourcePVC.Spec.VolumeName = "source-pv"
	sourcePV := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: "source-pv"},
		Spec: v1.PersistentVolumeSpec{
			PersistentVolumeSource: v1.PersistentVolumeSource{
				CSI: &v1.CSIPersistentVolumeSource{
					Driver:       d.name,
					VolumeHandle: newTestVolumeIDWithTargetName(t),
				},
			},
		},
	}
	pvc := internalTestPVC("clone", "db", "clone-uid", "iscsi")
	pvc.Spec.DataSource = &v1.TypedLocalObjectReference{
		Kind: "PersistentVolumeClaim",
		Name: sourcePVC.Name,
	}
	sc := internalTestStorageClass(d.name, "iscsi")
	client := fake.NewSimpleClientset(sc, sourcePVC, sourcePV, pvc)
	backend.createVirtualDiskFn = func(ctx context.Context, name, parentDir string, sizeBytes int64) (string, int64, error) {
		t.Fatalf("CreateVirtualDisk should not run for unsupported volume clone dataSource")
		return "", 0, nil
	}

	err := d.reconcilePersistentVolumeClaim(context.Background(), client, pvc)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "only snapshot volume content sources are supported")
}

func internalTestPersistentVolumeLister(t *testing.T, pvs ...*v1.PersistentVolume) corelisters.PersistentVolumeLister {
	t.Helper()
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	for _, pv := range pvs {
		require.NoError(t, indexer.Add(pv))
	}
	return corelisters.NewPersistentVolumeLister(indexer)
}

func internalTestVolumeAttachmentLister(t *testing.T, vas ...*storagev1.VolumeAttachment) storagelisters.VolumeAttachmentLister {
	t.Helper()
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
	for _, va := range vas {
		require.NoError(t, indexer.Add(va))
	}
	return storagelisters.NewVolumeAttachmentLister(indexer)
}

func internalTestStorageClass(driverName, name string) *storagev1.StorageClass {
	return &storagev1.StorageClass{
		ObjectMeta:  metav1.ObjectMeta{Name: name},
		Provisioner: driverName,
		Parameters: map[string]string{
			"targetPortal":   "10.0.0.1:3260",
			"vhdxParentPath": "D:\\vhdx",
			"iqnPrefix":      "iqn.2024-01.com.example",
		},
	}
}

func internalTestPVC(name, namespace, uid, storageClassName string) *v1.PersistentVolumeClaim {
	return &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, UID: types.UID(uid)},
		Spec: v1.PersistentVolumeClaimSpec{
			StorageClassName: &storageClassName,
			AccessModes:      []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
			Resources: v1.VolumeResourceRequirements{
				Requests: v1.ResourceList{
					v1.ResourceStorage: resource.MustParse("1Gi"),
				},
			},
		},
	}
}

func internalTestReleasedPV(t *testing.T, driverName string) *v1.PersistentVolume {
	t.Helper()
	pv := internalTestBoundPV(t, driverName)
	pv.Finalizers = []string{pvProtectionFinalizer}
	pv.Status.Phase = v1.VolumeReleased
	return pv
}

func internalTestBoundPV(t *testing.T, driverName string) *v1.PersistentVolume {
	t.Helper()
	return &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pvc-claim-uid",
		},
		Spec: v1.PersistentVolumeSpec{
			PersistentVolumeReclaimPolicy: v1.PersistentVolumeReclaimDelete,
			AccessModes:                   []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
			PersistentVolumeSource: v1.PersistentVolumeSource{
				CSI: &v1.CSIPersistentVolumeSource{
					Driver:       driverName,
					VolumeHandle: newTestVolumeIDWithTargetName(t),
				},
			},
		},
		Status: v1.PersistentVolumeStatus{Phase: v1.VolumeBound},
	}
}
