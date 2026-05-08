package iscsi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	iscsilib "github.com/taliesins/csi-driver-for-windows-storage-server/pkg/iscsilib"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	klog "k8s.io/klog/v2"
	mount "k8s.io/mount-utils"
	utilexec "k8s.io/utils/exec"
)

// Mockable function references for testing
var (
	iscsilibConnect      = func(c *iscsilib.Connector) (string, error) { return c.Connect() }
	iscsilibDisconnect   = func(c *iscsilib.Connector) error { return c.Disconnect() }
	disconnectVolume     = func(c *iscsilib.Connector) error { return c.DisconnectVolume() }
	getConnectorFromFile = iscsilib.GetConnectorFromFile
	formatAndMount       = func(m *mount.SafeFormatAndMount, source, target, fsType string, options []string) error {
		return m.FormatAndMount(source, target, fsType, options)
	}
	formatAndMountSensitive = func(m *mount.SafeFormatAndMount, source, target, fsType string, options, sensitiveOptions []string) error {
		return m.FormatAndMountSensitive(source, target, fsType, options, sensitiveOptions)
	}
	verifyStagedFilesystem = waitForStagedFilesystemReady
	iscsilibExpandVolume   = func(m mount.Interface, resizer iscsilib.Resizer, volumePath string) error {
		return iscsilib.ExpandVolume(m, resizer, volumePath)
	}
	fsUsageFunc                = fsUsage
	nodePublishDeviceWait      = 60 * time.Second
	stagedFilesystemReadyWait  = 10 * time.Second
	stagedFilesystemProbeDelay = 500 * time.Millisecond
)

type nodeServer struct {
	Driver  *driver
	mounter *mount.SafeFormatAndMount
	exec    utilexec.Interface
	resizer *mount.ResizeFs // keep as-is per your current code
	csi.UnimplementedNodeServer
}

func (ns *nodeServer) nodeID() string {
	if ns == nil || ns.Driver == nil {
		return ""
	}
	return ns.Driver.nodeID
}

// ---------- init / small helpers ----------

func (ns *nodeServer) init() error {
	if ns.exec == nil {
		ns.exec = utilexec.New()
	}
	if ns.mounter == nil {
		ns.mounter = &mount.SafeFormatAndMount{
			Interface: mount.New(""),
			Exec:      ns.exec,
		}
	}
	if ns.resizer == nil {
		// unchanged per your current project
		ns.resizer = mount.NewResizeFs(ns.mounter.Exec)
	}
	return nil
}

func getStr(m map[string]string, k string) (string, bool) {
	v, ok := m[k]
	return strings.TrimSpace(v), ok && strings.TrimSpace(v) != ""
}

func fsTypeFromCapability(vc *csi.VolumeCapability) string {
	if vc == nil {
		return ""
	}
	if mount := vc.GetMount(); mount != nil {
		return strings.TrimSpace(mount.GetFsType())
	}
	return ""
}

func mountOptionsFromCapability(vc *csi.VolumeCapability) []string {
	var opts []string
	if vc == nil {
		return opts
	}
	mount := vc.GetMount()
	if mount == nil {
		return opts
	}
	for _, opt := range mount.GetMountFlags() {
		opt = strings.TrimSpace(opt)
		if opt != "" {
			opts = append(opts, opt)
		}
	}
	return opts
}

func appendMountOptions(opts []string, raw string) []string {
	for _, opt := range strings.Split(raw, ",") {
		opt = strings.TrimSpace(opt)
		if opt != "" {
			opts = append(opts, opt)
		}
	}
	return opts
}

func splitSensitiveMountOptions(opts []string) ([]string, []string) {
	var normal, sensitive []string
	for _, opt := range opts {
		trimmed := strings.TrimSpace(opt)
		if trimmed == "" {
			continue
		}
		if isSensitiveMountOption(trimmed) {
			sensitive = append(sensitive, trimmed)
			continue
		}
		normal = append(normal, trimmed)
	}
	return normal, sensitive
}

func isSensitiveMountOption(opt string) bool {
	key, _, ok := strings.Cut(strings.ToLower(strings.TrimSpace(opt)), "=")
	if !ok {
		return false
	}
	switch strings.TrimSpace(key) {
	case "password", "pass", "credentials", "credential":
		return true
	default:
		return false
	}
}

func mountPrepared(m *mount.SafeFormatAndMount, source, target, fsType string, opts, sensitiveOpts []string) error {
	if len(sensitiveOpts) > 0 {
		return m.MountSensitive(source, target, fsType, opts, sensitiveOpts)
	}
	return m.Mount(source, target, fsType, opts)
}

func formatAndMountPrepared(m *mount.SafeFormatAndMount, source, target, fsType string, opts, sensitiveOpts []string) error {
	if len(sensitiveOpts) > 0 {
		return formatAndMountSensitive(m, source, target, fsType, opts, sensitiveOpts)
	}
	return formatAndMount(m, source, target, fsType, opts)
}

func mountOptionsReadOnly(opts []string) bool {
	for _, opt := range opts {
		switch strings.ToLower(strings.TrimSpace(opt)) {
		case "ro", "readonly":
			return true
		}
	}
	return false
}

func waitForStagedFilesystemReady(path string, opts []string, timeout time.Duration) error {
	if mountOptionsReadOnly(opts) {
		return nil
	}
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		if err := probeWritablePath(path); err != nil {
			lastErr = err
		} else {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for writable filesystem at %s: %w", path, lastErr)
		}
		time.Sleep(stagedFilesystemProbeDelay)
	}
}

func probeWritablePath(path string) error {
	probePath := filepath.Join(path, fmt.Sprintf(".csi-ready-%d", time.Now().UnixNano()))
	f, err := os.OpenFile(probePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, writeErr := f.Write([]byte("ok\n"))
	syncErr := f.Sync()
	closeErr := f.Close()
	removeErr := os.Remove(probePath)
	return errors.Join(writeErr, syncErr, closeErr, removeErr)
}

func iscsiConnectionFromContexts(volumeID string, contexts ...map[string]string) (portal, iqn string, lun int, err error) {
	if volumeIDContext := volumeContextFromVolumeID(volumeID, ProtocolISCSI); volumeIDContext != nil {
		contexts = append(contexts, volumeIDContext)
	}
	portal = firstContextValueForKeys([]string{"targetPortal", "portal"}, contexts...)
	if portal == "" {
		err = status.Error(codes.InvalidArgument, "targetPortal is required in publishContext, volumeContext, or volumeId")
		return
	}
	if err = validateTargetPortal(portal); err != nil {
		return
	}
	iqn = firstContextValueForKeys([]string{"iqn", "targetIQN", "targetIqn"}, contexts...)
	if iqn == "" {
		err = status.Error(codes.InvalidArgument, "iqn is required in publishContext, volumeContext, or volumeId")
		return
	}
	lunStr := firstContextValueForKeys([]string{"lun", "LUN"}, contexts...)
	if lunStr == "" {
		err = status.Error(codes.InvalidArgument, "lun is required in publishContext, volumeContext, or volumeId")
		return
	}
	li, conv := strconv.Atoi(lunStr)
	if conv != nil {
		err = status.Errorf(codes.InvalidArgument, "invalid lun: %q", lunStr)
		return
	}
	if li < 0 {
		err = status.Errorf(codes.InvalidArgument, "lun must be non-negative, got %q", lunStr)
		return
	}
	lun = li
	return
}

func isBlockDevice(p string) (bool, error) {
	st, err := os.Stat(p)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return (st.Mode() & os.ModeDevice) != 0, nil
}

func ensureFile(target string) error {
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	_, err := os.Stat(target)
	if os.IsNotExist(err) {
		f, e := os.OpenFile(target, os.O_CREATE, 0o600)
		if e != nil {
			return e
		}
		return f.Close()
	}
	return err
}

func (ns *nodeServer) waitForPath(path string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if _, err := os.Stat(path); err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for %s", path)
		}
		time.Sleep(300 * time.Millisecond)
	}
}

func stageConnectorFile(staging string) string {
	staging = filepath.Clean(staging)
	return filepath.Join(filepath.Dir(staging), "."+filepath.Base(staging)+".connector.json")
}

func legacyStageConnectorFile(staging string) string {
	return filepath.Join(staging, "connector.json")
}

func getConnectorFromStaging(staging string, includeLegacy bool) (*iscsilib.Connector, string, error) {
	paths := []string{stageConnectorFile(staging)}
	if includeLegacy {
		paths = append(paths, legacyStageConnectorFile(staging))
	}
	var lastErr error
	for _, path := range paths {
		conn, err := getConnectorFromFile(path)
		if err == nil {
			return conn, path, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, "", err
		}
		lastErr = err
	}
	return nil, "", lastErr
}

func getCleanupConnectorFromStaging(staging string, includeLegacy bool) (*iscsilib.Connector, string, error) {
	paths := []string{stageConnectorFile(staging)}
	if includeLegacy {
		paths = append(paths, legacyStageConnectorFile(staging))
	}
	var lastErr error
	for _, path := range paths {
		conn, err := getConnectorFromFile(path)
		if err == nil {
			return conn, path, nil
		}
		if errors.Is(err, os.ErrNotExist) {
			lastErr = err
			continue
		}
		if !isMissingMountTargetDevice(err) {
			return nil, "", err
		}
		conn, staleErr := readPersistedConnector(path)
		if staleErr != nil {
			if errors.Is(staleErr, os.ErrNotExist) {
				lastErr = staleErr
				continue
			}
			return nil, "", staleErr
		}
		return conn, path, nil
	}
	return nil, "", lastErr
}

func readPersistedConnector(path string) (*iscsilib.Connector, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	conn := iscsilib.Connector{}
	if err := json.Unmarshal(raw, &conn); err != nil {
		return nil, err
	}
	if conn.MountTargetDevice == nil {
		return nil, fmt.Errorf("mountTargetDevice in the connector is nil")
	}
	return &conn, nil
}

func isMissingMountTargetDevice(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "mounttargetdevice") && strings.Contains(msg, "not found")
}

func removeStagingConnectorFiles(staging string) error {
	for _, path := range []string{stageConnectorFile(staging), legacyStageConnectorFile(staging)} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// ---------- CHAP secrets parsing (matches iscsiadm keys) ----------
func parseChapSecrets(secrets map[string]string) (disc iscsilib.Secrets, sess iscsilib.Secrets, doChapDisc bool, authType string, err error) {
	if err = validateChapSecrets(secrets); err != nil {
		return
	}
	// Discovery CHAP
	if u := strings.TrimSpace(secrets["discovery.sendtargets.auth.username"]); u != "" {
		disc.SecretsType = "chap"
		disc.UserName = u
		disc.Password = strings.TrimSpace(secrets["discovery.sendtargets.auth.password"])
		disc.UserNameIn = strings.TrimSpace(secrets["discovery.sendtargets.auth.username_in"])
		disc.PasswordIn = strings.TrimSpace(secrets["discovery.sendtargets.auth.password_in"])
		doChapDisc = true
	}

	// Session CHAP
	if u := strings.TrimSpace(secrets["node.session.auth.username"]); u != "" {
		sess.SecretsType = "chap"
		sess.UserName = u
		sess.Password = strings.TrimSpace(secrets["node.session.auth.password"])
		sess.UserNameIn = strings.TrimSpace(secrets["node.session.auth.username_in"])
		sess.PasswordIn = strings.TrimSpace(secrets["node.session.auth.password_in"])
		authType = "chap"
	}
	return
}

// ---------- request parsing ----------

func (ns *nodeServer) parseStage(req *csi.NodeStageVolumeRequest) (portal, iqn string, lun int, fsType string, mountOpts []string, iface string, chapDisc bool, discSec, sessSec iscsilib.Secrets, authType string, err error) {
	if err = ns.init(); err != nil {
		return
	}
	pc := req.GetPublishContext()
	vc := req.GetVolumeContext()
	portal, iqn, lun, err = iscsiConnectionFromContexts(req.GetVolumeId(), pc, vc)
	if err != nil {
		return
	}

	fsType = fsTypeFromCapability(req.GetVolumeCapability())
	mountOpts = append(mountOpts, mountOptionsFromCapability(req.GetVolumeCapability())...)
	if vc != nil {
		if v, ok := getStr(vc, "fsType"); ok && fsType == "" {
			fsType = v
		}
		if v, ok := getStr(vc, "mountOptions"); ok {
			mountOpts = appendMountOptions(mountOpts, v)
		}
		if v, ok := getStr(vc, "iface"); ok {
			iface = v
		}
	}
	if iface == "" {
		iface = "default"
	}
	if fsType == "" {
		fsType = "ext4"
	}

	discSec, sessSec, chapDisc, authType, err = parseChapSecrets(req.GetSecrets())
	return
}

func (ns *nodeServer) parsePublish(req *csi.NodePublishVolumeRequest) (portal, iqn string, lun int, fsType string, mountOpts []string, ro bool, err error) {
	if err = ns.init(); err != nil {
		return
	}
	pc := req.GetPublishContext()
	vc := req.GetVolumeContext()
	portal, iqn, lun, err = iscsiConnectionFromContexts(req.GetVolumeId(), pc, vc)
	if err != nil {
		return
	}

	fsType = fsTypeFromCapability(req.GetVolumeCapability())
	mountOpts = append(mountOpts, mountOptionsFromCapability(req.GetVolumeCapability())...)
	if vc != nil {
		if v, ok := getStr(vc, "fsType"); ok && fsType == "" {
			fsType = v
		}
		if v, ok := getStr(vc, "mountOptions"); ok {
			mountOpts = appendMountOptions(mountOpts, v)
		}
	}
	if fsType == "" {
		fsType = "ext4"
	}
	ro = req.GetReadonly()
	return
}

func publishProtocol(req interface {
	GetPublishContext() map[string]string
	GetVolumeContext() map[string]string
	GetVolumeId() string
}) Protocol {
	if proto := protocolFromContext(req.GetPublishContext(), req.GetVolumeContext()); proto != "" {
		return proto
	}
	if proto, err := ParseProtocolFromVolumeID(req.GetVolumeId()); err == nil {
		switch proto {
		case ProtocolNFS, ProtocolSMB:
			return proto
		}
	}
	return ProtocolISCSI
}

func protocolFromContext(contexts ...map[string]string) Protocol {
	for _, ctx := range contexts {
		proto := strings.ToLower(strings.TrimSpace(ctx["protocol"]))
		switch Protocol(proto) {
		case ProtocolNFS:
			return ProtocolNFS
		case ProtocolSMB:
			return ProtocolSMB
		}
	}
	return ""
}

func mountOptionsFromContext(vc map[string]string) []string {
	var opts []string
	if v, ok := getStr(vc, "mountOptions"); ok {
		opts = appendMountOptions(opts, v)
	}
	return opts
}

func firstContextValue(key string, contexts ...map[string]string) string {
	for _, ctx := range contexts {
		if value, ok := getStr(ctx, key); ok {
			return value
		}
	}
	return ""
}

func firstContextValueForKeys(keys []string, contexts ...map[string]string) string {
	for _, ctx := range contexts {
		for _, key := range keys {
			if value, ok := getStr(ctx, key); ok {
				return value
			}
		}
	}
	return ""
}

func volumeContextFromVolumeID(volumeID string, proto Protocol) map[string]string {
	decoded, err := DecodeVolumeID(volumeID)
	if err != nil || decoded.Protocol != proto {
		return nil
	}
	return volumeContextForVolumeID(decoded)
}

func nfsMountAuthenticationFromContext(contexts ...map[string]string) (string, error) {
	if value := firstContextValue("nfsMountAuthentication", contexts...); value != "" {
		return normalizeNfsAuthenticationFlavor(value)
	}
	raw := firstContextValue("nfsAuthentication", contexts...)
	if raw == "" {
		raw = firstContextValue("nfsauthentication", contexts...)
	}
	auth, err := normalizeNfsAuthentication(raw)
	if err != nil {
		return "", err
	}
	return preferredNfsMountAuthentication(auth), nil
}

func firstSecretValue(secrets map[string]string, keys ...string) (string, bool) {
	for _, key := range keys {
		if value, ok := getStr(secrets, key); ok {
			return value, true
		}
	}
	return "", false
}

func appendOptionIfMissing(opts []string, prefix, value string) []string {
	if value == "" {
		return opts
	}
	if prefix != "" {
		for _, opt := range opts {
			if strings.HasPrefix(strings.ToLower(opt), strings.ToLower(prefix)) {
				return opts
			}
		}
	}
	return append(opts, value)
}

func (ns *nodeServer) stageFileShareVolume(req *csi.NodeStageVolumeRequest, proto Protocol) (*csi.NodeStageVolumeResponse, error) {
	if err := ns.init(); err != nil {
		return nil, err
	}
	if req.GetVolumeCapability().GetBlock() != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%s volumes do not support block mode", proto)
	}
	pc := req.GetPublishContext()
	vc := req.GetVolumeContext()
	volumeIDContext := volumeContextFromVolumeID(req.GetVolumeId(), proto)
	var source, fsType string
	switch proto {
	case ProtocolNFS:
		server := firstContextValueForKeys([]string{"nfsServer", "server"}, pc, vc, volumeIDContext)
		exportPath := firstContextValueForKeys([]string{"nfsExportPath", "exportPath"}, pc, vc, volumeIDContext)
		if server == "" || exportPath == "" {
			return nil, status.Error(codes.InvalidArgument, "nfsServer/server and nfsExportPath/exportPath are required in publishContext, volumeContext, or volumeId")
		}
		source = server + ":" + exportPath
		fsType = "nfs"
	case ProtocolSMB:
		server := firstContextValueForKeys([]string{"smbServer", "server"}, pc, vc, volumeIDContext)
		share := firstContextValueForKeys([]string{"smbShareName", "share"}, pc, vc, volumeIDContext)
		if server == "" || share == "" {
			return nil, status.Error(codes.InvalidArgument, "smbServer/server and smbShareName/share are required in publishContext, volumeContext, or volumeId")
		}
		source = fmt.Sprintf("//%s/%s", server, share)
		fsType = "cifs"
	}
	opts := append(mountOptionsFromCapability(req.GetVolumeCapability()), mountOptionsFromContext(vc)...)
	var sensitiveOpts []string
	switch proto {
	case ProtocolNFS:
		if version, ok := getStr(vc, "nfsVersion"); ok {
			opts = appendOptionIfMissing(opts, "vers=", "vers="+version)
		}
		authentication, err := nfsMountAuthenticationFromContext(vc, pc, volumeIDContext)
		if err != nil {
			return nil, err
		}
		if authentication != "" {
			opts = appendOptionIfMissing(opts, "sec=", "sec="+authentication)
		}
	case ProtocolSMB:
		if version, ok := getStr(vc, "smbVersion"); ok {
			opts = appendOptionIfMissing(opts, "vers=", "vers="+version)
		}
		if username, ok := firstSecretValue(req.GetSecrets(), "smbUsername", "username"); ok {
			opts = appendOptionIfMissing(opts, "username=", "username="+username)
		}
		if password, ok := firstSecretValue(req.GetSecrets(), "smbPassword", "password"); ok {
			sensitiveOpts = appendOptionIfMissing(sensitiveOpts, "password=", "password="+password)
		}
		if domain, ok := firstSecretValue(req.GetSecrets(), "smbDomain", "domain"); ok {
			opts = appendOptionIfMissing(opts, "domain=", "domain="+domain)
		}
		if seal, ok := getStr(vc, "smbSeal"); ok {
			if enabled, err := strconv.ParseBool(seal); err == nil && enabled {
				opts = appendOptionIfMissing(opts, "seal", "seal")
			}
		}
	}
	opts, extractedSensitiveOpts := splitSensitiveMountOptions(opts)
	sensitiveOpts = append(extractedSensitiveOpts, sensitiveOpts...)
	if err := os.MkdirAll(req.GetStagingTargetPath(), 0o755); err != nil {
		return nil, status.Errorf(codes.Internal, "mkdir staging: %v", err)
	}
	notMnt, merr := ns.mounter.IsLikelyNotMountPoint(req.GetStagingTargetPath())
	if merr != nil && !os.IsNotExist(merr) {
		return nil, status.Errorf(codes.Internal, "check staging mount: %v", merr)
	}
	if notMnt {
		klog.Infof("NodeStageVolume: mounting file-share volume: node=%q protocol=%s source=%q stagingTargetPath=%q fsType=%q options=%q", ns.nodeID(), proto, source, req.GetStagingTargetPath(), fsType, sanitizeMountOptionsForLog(opts))
		if err := mountPrepared(ns.mounter, source, req.GetStagingTargetPath(), fsType, opts, sensitiveOpts); err != nil {
			return nil, status.Errorf(codes.Internal, "mount %s volume: %v", proto, err)
		}
		klog.Infof("NodeStageVolume: file-share volume mounted: node=%q protocol=%s source=%q stagingTargetPath=%q", ns.nodeID(), proto, source, req.GetStagingTargetPath())
	} else {
		klog.Infof("NodeStageVolume: file-share staging path already mounted: node=%q protocol=%s source=%q stagingTargetPath=%q", ns.nodeID(), proto, source, req.GetStagingTargetPath())
	}
	return &csi.NodeStageVolumeResponse{}, nil
}

// ---------- NodeStage / NodeUnstage ----------

func (ns *nodeServer) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "volumeID missing")
	}
	if req.GetStagingTargetPath() == "" {
		return nil, status.Error(codes.InvalidArgument, "stagingTargetPath missing")
	}
	if req.GetVolumeCapability() == nil {
		return nil, status.Error(codes.InvalidArgument, "volumeCapability missing")
	}
	if proto := publishProtocol(req); proto == ProtocolNFS || proto == ProtocolSMB {
		return ns.stageFileShareVolume(req, proto)
	}

	portal, iqn, lun, fsType, mountOpts, iface, chapDisc, discSec, sessSec, authType, err := ns.parseStage(req)
	if err != nil {
		return nil, err
	}
	klog.Infof("NodeStageVolume: connecting iSCSI volume: node=%q targetPortal=%q targetIQN=%q lun=%d stagingTargetPath=%q fsType=%q options=%q", ns.nodeID(), portal, iqn, lun, req.GetStagingTargetPath(), fsType, sanitizeMountOptionsForLog(mountOpts))

	// Build connector and connect via iscsilib (handles discovery, CHAP, login, path wait)
	conn := &iscsilib.Connector{
		VolumeName:       req.GetVolumeId(),
		TargetIqn:        iqn,
		TargetPortals:    []string{portal},
		Lun:              int32(lun),
		Interface:        iface,
		DoDiscovery:      true,
		DoCHAPDiscovery:  chapDisc,
		DiscoverySecrets: discSec,
		SessionSecrets:   sessSec,
		AuthType:         authType,
		RetryCount:       60,
		CheckInterval:    2,
	}
	device, err := iscsilibConnect(conn)
	if err != nil || device == "" {
		return nil, status.Errorf(codes.Internal, "iSCSI connect failed: %v", err)
	}
	klog.Infof("NodeStageVolume: iSCSI volume connected: node=%q targetPortal=%q targetIQN=%q lun=%d device=%q", ns.nodeID(), portal, iqn, lun, device)

	// Persist the connector beside the staging mount so filesystem mounts do not hide it.
	connectorFile := stageConnectorFile(req.GetStagingTargetPath())
	if err := os.MkdirAll(filepath.Dir(connectorFile), 0o755); err != nil {
		return nil, status.Errorf(codes.Internal, "mkdir iSCSI connector directory: %v", err)
	}
	if err := conn.Persist(connectorFile); err != nil {
		cleanupErr := errors.Join(disconnectVolume(conn), iscsilibDisconnect(conn))
		if cleanupErr != nil {
			return nil, status.Errorf(codes.Internal, "persist iSCSI connector: %v (rollback disconnect failed: %v)", err, cleanupErr)
		}
		return nil, status.Errorf(codes.Internal, "persist iSCSI connector: %v", err)
	}

	// Block: staging path just needs to exist
	if req.GetVolumeCapability().GetBlock() != nil {
		if err := os.MkdirAll(req.GetStagingTargetPath(), 0o755); err != nil {
			return nil, status.Errorf(codes.Internal, "mkdir staging: %v", err)
		}
		return &csi.NodeStageVolumeResponse{}, nil
	}

	// FS: format+mount at staging if not already mounted
	notMnt, merr := ns.mounter.IsLikelyNotMountPoint(req.GetStagingTargetPath())
	if merr != nil && !os.IsNotExist(merr) {
		return nil, status.Errorf(codes.Internal, "check staging mount: %v", merr)
	}
	if err := os.MkdirAll(req.GetStagingTargetPath(), 0o755); err != nil {
		return nil, status.Errorf(codes.Internal, "mkdir staging: %v", err)
	}
	normalMountOpts, sensitiveMountOpts := splitSensitiveMountOptions(mountOpts)
	if notMnt {
		klog.Infof("NodeStageVolume: formatting/mounting iSCSI volume: node=%q targetPortal=%q targetIQN=%q lun=%d device=%q stagingTargetPath=%q fsType=%q options=%q", ns.nodeID(), portal, iqn, lun, device, req.GetStagingTargetPath(), fsType, sanitizeMountOptionsForLog(normalMountOpts))
		if err := formatAndMountPrepared(ns.mounter, device, req.GetStagingTargetPath(), fsType, normalMountOpts, sensitiveMountOpts); err != nil {
			return nil, status.Errorf(codes.Internal, "format+mount staging failed: %v", err)
		}
		klog.Infof("NodeStageVolume: iSCSI volume staged: node=%q targetPortal=%q targetIQN=%q lun=%d device=%q stagingTargetPath=%q", ns.nodeID(), portal, iqn, lun, device, req.GetStagingTargetPath())
	} else {
		klog.Infof("NodeStageVolume: iSCSI staging path already mounted: node=%q targetPortal=%q targetIQN=%q lun=%d stagingTargetPath=%q", ns.nodeID(), portal, iqn, lun, req.GetStagingTargetPath())
	}
	if err := verifyStagedFilesystem(req.GetStagingTargetPath(), normalMountOpts, stagedFilesystemReadyWait); err != nil {
		return nil, status.Errorf(codes.Internal, "staging filesystem not ready: %v", err)
	}
	return &csi.NodeStageVolumeResponse{}, nil
}

func (ns *nodeServer) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "volumeID missing")
	}
	if req.GetStagingTargetPath() == "" {
		return nil, status.Error(codes.InvalidArgument, "stagingTargetPath missing")
	}

	if err := ns.init(); err != nil {
		return nil, status.Errorf(codes.Internal, "node server init failed: %v", err)
	}

	staging := req.GetStagingTargetPath()
	conn, _, err := getCleanupConnectorFromStaging(staging, false)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, status.Errorf(codes.Internal, "load iSCSI connector: %v", err)
		}
		conn = nil
	}

	notMnt, mountErr := ns.mounter.IsLikelyNotMountPoint(staging)
	if mountErr != nil && !os.IsNotExist(mountErr) {
		return nil, status.Errorf(codes.Internal, "check staging mount: %v", mountErr)
	}
	if mountErr == nil && !notMnt {
		targetIQN, targetPortals := "", []string(nil)
		if conn != nil {
			targetIQN = conn.TargetIqn
			targetPortals = conn.TargetPortals
		}
		klog.Infof("NodeUnstageVolume: unmounting staging path: node=%q volumeID=%q targetIQN=%q targetPortals=%q stagingTargetPath=%q", ns.nodeID(), req.GetVolumeId(), targetIQN, targetPortals, staging)
		if err := ns.mounter.Unmount(staging); err != nil {
			return nil, status.Errorf(codes.Internal, "unstage unmount failed: %v", err)
		}
		klog.Infof("NodeUnstageVolume: staging path unmounted: node=%q volumeID=%q targetIQN=%q targetPortals=%q stagingTargetPath=%q", ns.nodeID(), req.GetVolumeId(), targetIQN, targetPortals, staging)
	}

	if conn == nil {
		conn, _, err = getCleanupConnectorFromStaging(staging, true)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return nil, status.Errorf(codes.Internal, "load iSCSI connector: %v", err)
			}
			conn = nil
		}
	}

	if conn != nil {
		klog.Infof("NodeUnstageVolume: disconnecting iSCSI volume: node=%q volumeID=%q targetIQN=%q targetPortals=%q", ns.nodeID(), req.GetVolumeId(), conn.TargetIqn, conn.TargetPortals)
		if err := disconnectVolume(conn); err != nil {
			return nil, status.Errorf(codes.Internal, "iSCSI disconnect volume failed: %v", err)
		}
		if err := iscsilibDisconnect(conn); err != nil {
			return nil, status.Errorf(codes.Internal, "iSCSI logout failed: %v", err)
		}
		if err := removeStagingConnectorFiles(staging); err != nil {
			return nil, status.Errorf(codes.Internal, "remove iSCSI connector file failed: %v", err)
		}
		klog.Infof("NodeUnstageVolume: iSCSI volume disconnected: node=%q volumeID=%q targetIQN=%q targetPortals=%q", ns.nodeID(), req.GetVolumeId(), conn.TargetIqn, conn.TargetPortals)
	}

	klog.Infof("NodeUnstageVolume: cleaning staging path: node=%q volumeID=%q stagingTargetPath=%q", ns.nodeID(), req.GetVolumeId(), staging)
	if err := mount.CleanupMountPoint(staging, ns.mounter, false); err != nil && !os.IsNotExist(err) {
		return nil, status.Errorf(codes.Internal, "unstage cleanup failed: %v", err)
	}
	klog.Infof("NodeUnstageVolume: staging cleanup completed: node=%q volumeID=%q stagingTargetPath=%q", ns.nodeID(), req.GetVolumeId(), staging)

	return &csi.NodeUnstageVolumeResponse{}, nil
}

// ---------- NodePublish / NodeUnpublish ----------

func (ns *nodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "volumeID missing")
	}
	if req.GetTargetPath() == "" {
		return nil, status.Error(codes.InvalidArgument, "targetPath missing")
	}
	if req.GetVolumeCapability() == nil {
		return nil, status.Error(codes.InvalidArgument, "volumeCapability missing")
	}
	if proto := publishProtocol(req); proto == ProtocolNFS || proto == ProtocolSMB {
		if req.GetVolumeCapability().GetBlock() != nil {
			return nil, status.Errorf(codes.InvalidArgument, "%s volumes do not support block mode", proto)
		}
		staging := req.GetStagingTargetPath()
		if staging == "" {
			return nil, status.Error(codes.InvalidArgument, "stagingTargetPath required for mount volumes")
		}
		if err := os.MkdirAll(req.GetTargetPath(), 0o755); err != nil {
			return nil, status.Errorf(codes.Internal, "mkdir target: %v", err)
		}
		opts := []string{"bind"}
		if req.GetReadonly() {
			opts = append(opts, "ro")
		}
		opts, sensitiveOpts := splitSensitiveMountOptions(opts)
		klog.Infof("NodePublishVolume: bind-mounting file-share volume: node=%q protocol=%s source=%q targetPath=%q readonly=%t options=%q", ns.nodeID(), proto, staging, req.GetTargetPath(), req.GetReadonly(), sanitizeMountOptionsForLog(opts))
		if err := mountPrepared(ns.mounter, staging, req.GetTargetPath(), "", opts, sensitiveOpts); err != nil {
			return nil, status.Errorf(codes.Internal, "bind-mount publish failed: %v", err)
		}
		klog.Infof("NodePublishVolume: file-share volume published: node=%q protocol=%s source=%q targetPath=%q", ns.nodeID(), proto, staging, req.GetTargetPath())
		return &csi.NodePublishVolumeResponse{}, nil
	}

	portal, iqn, lun, fsType, mountOpts, ro, err := ns.parsePublish(req)
	if err != nil {
		return nil, err
	}

	staging := req.GetStagingTargetPath()

	// For block: mount the device file directly to the target file
	if req.GetVolumeCapability().GetBlock() != nil {
		if staging == "" {
			return nil, status.Error(codes.InvalidArgument, "stagingTargetPath is required for block to load connector")
		}
		conn, _, err := getConnectorFromStaging(staging, true)
		if err != nil || conn == nil || conn.MountTargetDevice == nil {
			// Fallback: compute the by-path and bind mount
			device := filepath.Join("/dev/disk/by-path",
				fmt.Sprintf("ip-%s-iscsi-%s-lun-%d", portal, iqn, lun))
			if err := ns.waitForPath(device, nodePublishDeviceWait); err != nil {
				return nil, status.Errorf(codes.Internal, "cannot resolve device for block publish: %v", err)
			}
			if err := ensureFile(req.GetTargetPath()); err != nil {
				return nil, status.Errorf(codes.Internal, "create block target: %v", err)
			}
			opts := []string{"bind"}
			if ro {
				opts = append(opts, "ro")
			}
			opts, sensitiveOpts := splitSensitiveMountOptions(opts)
			klog.Infof("NodePublishVolume: bind-mounting iSCSI block volume: node=%q targetPortal=%q targetIQN=%q lun=%d device=%q targetPath=%q readonly=%t options=%q", ns.nodeID(), portal, iqn, lun, device, req.GetTargetPath(), ro, sanitizeMountOptionsForLog(opts))
			if err := mountPrepared(ns.mounter, device, req.GetTargetPath(), "", opts, sensitiveOpts); err != nil {
				return nil, status.Errorf(codes.Internal, "bind-mount block device: %v", err)
			}
			klog.Infof("NodePublishVolume: iSCSI block volume published: node=%q targetPortal=%q targetIQN=%q lun=%d device=%q targetPath=%q", ns.nodeID(), portal, iqn, lun, device, req.GetTargetPath())
			return &csi.NodePublishVolumeResponse{}, nil
		}

		device := ""
		if conn.MountTargetDevice != nil {
			device = conn.MountTargetDevice.GetPath()
		}
		if err := ensureFile(req.GetTargetPath()); err != nil {
			return nil, status.Errorf(codes.Internal, "create block target: %v", err)
		}
		opts := []string{"bind"}
		if ro {
			opts = append(opts, "ro")
		}
		opts, sensitiveOpts := splitSensitiveMountOptions(opts)
		klog.Infof("NodePublishVolume: bind-mounting iSCSI block volume: node=%q targetPortal=%q targetIQN=%q lun=%d device=%q targetPath=%q readonly=%t options=%q", ns.nodeID(), portal, iqn, lun, device, req.GetTargetPath(), ro, sanitizeMountOptionsForLog(opts))
		if err := mountPrepared(ns.mounter, device, req.GetTargetPath(), "", opts, sensitiveOpts); err != nil {
			return nil, status.Errorf(codes.Internal, "bind-mount block device: %v", err)
		}
		klog.Infof("NodePublishVolume: iSCSI block volume published: node=%q targetPortal=%q targetIQN=%q lun=%d device=%q targetPath=%q", ns.nodeID(), portal, iqn, lun, device, req.GetTargetPath())
		return &csi.NodePublishVolumeResponse{}, nil
	}

	// Filesystem volumes: ensure staging is mounted, then bind mount to target
	if staging == "" {
		return nil, status.Error(codes.InvalidArgument, "stagingTargetPath required for mount volumes")
	}
	// Tolerate reboot recovery: if staging isn't mounted, recover using connector info.
	notMnt, merr := ns.mounter.IsLikelyNotMountPoint(staging)
	if merr == nil && notMnt {
		conn, _, err := getConnectorFromStaging(staging, true)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "staging recovery failed: %v", err)
		}
		if conn == nil {
			return nil, status.Error(codes.Internal, "staging recovery failed: connector not found")
		}
		device := conn.MountTargetDevice.GetPath()
		if device == "" {
			device = filepath.Join("/dev/disk/by-path",
				fmt.Sprintf("ip-%s-iscsi-%s-lun-%d", portal, iqn, lun))
			if err := ns.waitForPath(device, 60*time.Second); err != nil {
				return nil, status.Errorf(codes.Internal, "recover staging path: %v", err)
			}
		}
		normalMountOpts, sensitiveMountOpts := splitSensitiveMountOptions(mountOpts)
		if err := formatAndMountPrepared(ns.mounter, device, staging, fsType, normalMountOpts, sensitiveMountOpts); err != nil {
			return nil, status.Errorf(codes.Internal, "format+mount staging fallback failed: %v", err)
		}
		klog.Infof("NodePublishVolume: recovered iSCSI staging mount: node=%q targetPortal=%q targetIQN=%q lun=%d device=%q stagingTargetPath=%q", ns.nodeID(), portal, iqn, lun, device, staging)
	}
	if err := os.MkdirAll(req.GetTargetPath(), 0o755); err != nil {
		return nil, status.Errorf(codes.Internal, "mkdir target: %v", err)
	}
	opts := []string{"bind"}
	if ro {
		opts = append(opts, "ro")
	}
	opts, sensitiveOpts := splitSensitiveMountOptions(opts)
	klog.Infof("NodePublishVolume: bind-mounting iSCSI filesystem volume: node=%q targetPortal=%q targetIQN=%q lun=%d source=%q targetPath=%q readonly=%t options=%q", ns.nodeID(), portal, iqn, lun, staging, req.GetTargetPath(), ro, sanitizeMountOptionsForLog(opts))
	if err := mountPrepared(ns.mounter, staging, req.GetTargetPath(), "", opts, sensitiveOpts); err != nil {
		return nil, status.Errorf(codes.Internal, "bind-mount publish failed: %v", err)
	}
	klog.Infof("NodePublishVolume: iSCSI filesystem volume published: node=%q targetPortal=%q targetIQN=%q lun=%d source=%q targetPath=%q", ns.nodeID(), portal, iqn, lun, staging, req.GetTargetPath())
	return &csi.NodePublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "volumeID missing")
	}
	if req.GetTargetPath() == "" {
		return nil, status.Error(codes.InvalidArgument, "targetPath missing")
	}
	klog.Infof("NodeUnpublishVolume: unmounting published volume: node=%q volumeID=%q targetPath=%q", ns.nodeID(), req.GetVolumeId(), req.GetTargetPath())
	if err := mount.CleanupMountPoint(req.GetTargetPath(), ns.mounter, false); err != nil && !os.IsNotExist(err) {
		return nil, status.Errorf(codes.Internal, "cleanup target mount: %v", err)
	}
	klog.Infof("NodeUnpublishVolume: published volume unmounted: node=%q volumeID=%q targetPath=%q", ns.nodeID(), req.GetVolumeId(), req.GetTargetPath())
	return &csi.NodeUnpublishVolumeResponse{}, nil
}

// ---------- Capabilities / Info ----------

func (ns *nodeServer) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	return &csi.NodeGetInfoResponse{
		NodeId: ns.Driver.nodeID,
	}, nil
}

func (ns *nodeServer) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	return &csi.NodeGetCapabilitiesResponse{
		Capabilities: []*csi.NodeServiceCapability{
			{Type: &csi.NodeServiceCapability_Rpc{
				Rpc: &csi.NodeServiceCapability_RPC{Type: csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME},
			}},
			{Type: &csi.NodeServiceCapability_Rpc{
				Rpc: &csi.NodeServiceCapability_RPC{Type: csi.NodeServiceCapability_RPC_EXPAND_VOLUME},
			}},
		},
	}, nil
}

// ---------- Stats / Expand ----------

func (ns *nodeServer) NodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "volumeID missing")
	}
	if req.GetVolumePath() == "" {
		return nil, status.Error(codes.InvalidArgument, "volumePath missing")
	}

	// Raw block?
	if ok, _ := isBlockDevice(req.GetVolumePath()); ok {
		cmd := ns.exec.Command("blockdev", "--getsize64", req.GetVolumePath())
		out, err := cmd.CombinedOutput()
		if err != nil {
			return nil, status.Errorf(codes.Internal, "blockdev failed: %v, out=%s", err, string(out))
		}
		sizeStr := strings.TrimSpace(string(out))
		size, err := strconv.ParseInt(sizeStr, 10, 64)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "parse block size: %v", err)
		}
		return &csi.NodeGetVolumeStatsResponse{
			Usage: []*csi.VolumeUsage{{
				Unit:      csi.VolumeUsage_BYTES,
				Total:     size,
				Available: size,
				Used:      0,
			}},
		}, nil
	}

	available, capacity, used, inodes, inodesFree, inodesUsed, err := fsUsageFunc(req.GetVolumePath())
	if err != nil {
		if os.IsNotExist(err) {
			return &csi.NodeGetVolumeStatsResponse{
				Usage: []*csi.VolumeUsage{
					{Unit: csi.VolumeUsage_BYTES},
					{Unit: csi.VolumeUsage_INODES},
				},
			}, nil
		}
		return nil, status.Errorf(codes.Internal, "statfs failed: %v", err)
	}
	return &csi.NodeGetVolumeStatsResponse{
		Usage: []*csi.VolumeUsage{
			{Unit: csi.VolumeUsage_BYTES, Available: available, Total: capacity, Used: used},
			{Unit: csi.VolumeUsage_INODES, Available: inodesFree, Total: inodes, Used: inodesUsed},
		},
	}, nil
}

func (ns *nodeServer) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "volumeID missing")
	}
	if req.GetVolumePath() == "" {
		return nil, status.Error(codes.InvalidArgument, "volumePath missing")
	}
	if err := ns.init(); err != nil {
		return nil, status.Errorf(codes.Internal, "init: %v", err)
	}
	if proto, err := ParseProtocolFromVolumeID(req.GetVolumeId()); err == nil && (proto == ProtocolNFS || proto == ProtocolSMB) {
		return &csi.NodeExpandVolumeResponse{}, nil
	}

	// Moved into iscsilib: rescan iSCSI + resize multipath (if any) + grow filesystem
	klog.Infof("NodeExpandVolume: expanding node filesystem: node=%q volumeID=%q volumePath=%q", ns.nodeID(), req.GetVolumeId(), req.GetVolumePath())
	if err := iscsilibExpandVolume(ns.mounter.Interface, ns.resizer, req.GetVolumePath()); err != nil {
		return nil, status.Errorf(codes.Internal, "expand failed: %v", err)
	}
	klog.Infof("NodeExpandVolume: node filesystem expanded: node=%q volumeID=%q volumePath=%q", ns.nodeID(), req.GetVolumeId(), req.GetVolumePath())

	return &csi.NodeExpandVolumeResponse{}, nil
}
