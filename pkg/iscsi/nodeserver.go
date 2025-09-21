package iscsi

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	iscsilib "github.com/taliesins/csi-driver-iscsi-for-windows/pkg/iscsilib"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	mount "k8s.io/mount-utils"
	utilexec "k8s.io/utils/exec"
)

type nodeServer struct {
	Driver  *driver
	mounter *mount.SafeFormatAndMount
	exec    utilexec.Interface
	resizer *mount.ResizeFs // keep as-is per your current code
	csi.UnimplementedNodeServer
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

func isBlockDevice(p string) (bool, error) {
	st, err := os.Stat(p)
	if err != nil {
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
	return filepath.Join(staging, "connector.json")
}

// ---------- CHAP secrets parsing (matches iscsiadm keys) ----------
func parseChapSecrets(secrets map[string]string) (disc iscsilib.Secrets, sess iscsilib.Secrets, doChapDisc bool, authType string) {
	// Discovery CHAP
	if u, ok := secrets["discovery.sendtargets.auth.username"]; ok && u != "" {
		disc.SecretsType = "chap"
		disc.UserName = u
		disc.Password = strings.TrimSpace(secrets["discovery.sendtargets.auth.password"])
		disc.UserNameIn = strings.TrimSpace(secrets["discovery.sendtargets.auth.username_in"])
		disc.PasswordIn = strings.TrimSpace(secrets["discovery.sendtargets.auth.password_in"])
		doChapDisc = true
	}

	// Session CHAP
	if u, ok := secrets["node.session.auth.username"]; ok && u != "" {
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
	if pc == nil {
		err = status.Error(codes.InvalidArgument, "publishContext is required")
		return
	}
	var ok bool
	if portal, ok = getStr(pc, "targetPortal"); !ok {
		err = status.Error(codes.InvalidArgument, "publishContext[targetPortal] missing")
		return
	}
	if iqn, ok = getStr(pc, "iqn"); !ok {
		err = status.Error(codes.InvalidArgument, "publishContext[iqn] missing")
		return
	}
	lunStr, ok := getStr(pc, "lun")
	if !ok {
		err = status.Error(codes.InvalidArgument, "publishContext[lun] missing")
		return
	}
	li, conv := strconv.Atoi(lunStr)
	if conv != nil {
		err = status.Errorf(codes.InvalidArgument, "invalid lun: %q", lunStr)
		return
	}
	lun = li

	vc := req.GetVolumeContext()
	if vc != nil {
		if v, ok := getStr(vc, "fsType"); ok {
			fsType = v
		}
		if v, ok := getStr(vc, "mountOptions"); ok {
			for _, mo := range strings.Split(v, ",") {
				mo = strings.TrimSpace(mo)
				if mo != "" {
					mountOpts = append(mountOpts, mo)
				}
			}
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

	discSec, sessSec, chapDisc, authType = parseChapSecrets(req.GetSecrets())
	return
}

func (ns *nodeServer) parsePublish(req *csi.NodePublishVolumeRequest) (portal, iqn string, lun int, fsType string, mountOpts []string, ro bool, err error) {
	if err = ns.init(); err != nil {
		return
	}
	pc := req.GetPublishContext()
	if pc == nil {
		err = status.Error(codes.InvalidArgument, "publishContext is required")
		return
	}
	var ok bool
	if portal, ok = getStr(pc, "targetPortal"); !ok {
		err = status.Error(codes.InvalidArgument, "publishContext[targetPortal] missing")
		return
	}
	if iqn, ok = getStr(pc, "iqn"); !ok {
		err = status.Error(codes.InvalidArgument, "publishContext[iqn] missing")
		return
	}
	lunStr, ok := getStr(pc, "lun")
	if !ok {
		err = status.Error(codes.InvalidArgument, "publishContext[lun] missing")
		return
	}
	li, conv := strconv.Atoi(lunStr)
	if conv != nil {
		err = status.Errorf(codes.InvalidArgument, "invalid lun: %q", lunStr)
		return
	}
	lun = li

	vc := req.GetVolumeContext()
	if vc != nil {
		if v, ok := getStr(vc, "fsType"); ok {
			fsType = v
		}
		if v, ok := getStr(vc, "mountOptions"); ok {
			for _, mo := range strings.Split(v, ",") {
				mo = strings.TrimSpace(mo)
				if mo != "" {
					mountOpts = append(mountOpts, mo)
				}
			}
		}
	}
	if fsType == "" {
		fsType = "ext4"
	}
	ro = req.GetReadonly()
	return
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

	portal, iqn, lun, fsType, mountOpts, iface, chapDisc, discSec, sessSec, authType, err := ns.parseStage(req)
	if err != nil {
		return nil, err
	}

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
	device, err := conn.Connect()
	if err != nil || device == "" {
		return nil, status.Errorf(codes.Internal, "iSCSI connect failed: %v", err)
	}

	// Persist the connector (used by publish/unpublish/unstage)
	_ = conn.Persist(stageConnectorFile(req.GetStagingTargetPath()))

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
	if notMnt {
		if err := ns.mounter.FormatAndMount(device, req.GetStagingTargetPath(), fsType, mountOpts); err != nil {
			return nil, status.Errorf(codes.Internal, "format+mount staging failed: %v", err)
		}
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

	// Unmount staging if present
	if err := mount.CleanupMountPoint(req.GetStagingTargetPath(), ns.mounter, false); err != nil && !os.IsNotExist(err) {
		return nil, status.Errorf(codes.Internal, "unstage cleanup failed: %v", err)
	}

	// Load connector and disconnect volume (multipath-aware)
	if conn, err := iscsilib.GetConnectorFromFile(stageConnectorFile(req.GetStagingTargetPath())); err == nil {
		_ = conn.DisconnectVolume()
		_ = os.Remove(stageConnectorFile(req.GetStagingTargetPath()))
	}

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
		conn, err := iscsilib.GetConnectorFromFile(stageConnectorFile(staging))
		if err != nil || conn == nil || conn.MountTargetDevice == nil {
			// Fallback: compute the by-path and bind mount
			device := filepath.Join("/dev/disk/by-path",
				fmt.Sprintf("ip-%s-iscsi-%s-lun-%d", portal, iqn, lun))
			if err := ns.waitForPath(device, 60*time.Second); err != nil {
				return nil, status.Errorf(codes.Internal, "cannot resolve device for block publish: %v", err)
			}
			if err := ensureFile(req.GetTargetPath()); err != nil {
				return nil, status.Errorf(codes.Internal, "create block target: %v", err)
			}
			opts := []string{"bind"}
			if ro {
				opts = append(opts, "ro")
			}
			if err := ns.mounter.Mount(device, req.GetTargetPath(), "", opts); err != nil {
				return nil, status.Errorf(codes.Internal, "bind-mount block device: %v", err)
			}
			return &csi.NodePublishVolumeResponse{}, nil
		}

		device := conn.MountTargetDevice.GetPath()
		if err := ensureFile(req.GetTargetPath()); err != nil {
			return nil, status.Errorf(codes.Internal, "create block target: %v", err)
		}
		opts := []string{"bind"}
		if ro {
			opts = append(opts, "ro")
		}
		if err := ns.mounter.Mount(device, req.GetTargetPath(), "", opts); err != nil {
			return nil, status.Errorf(codes.Internal, "bind-mount block device: %v", err)
		}
		return &csi.NodePublishVolumeResponse{}, nil
	}

	// Filesystem volumes: ensure staging is mounted, then bind mount to target
	if staging == "" {
		return nil, status.Error(codes.InvalidArgument, "stagingTargetPath required for mount volumes")
	}
	// Tolerate reboot recovery: if staging isn't mounted, recover using connector info.
	notMnt, merr := ns.mounter.IsLikelyNotMountPoint(staging)
	if merr == nil && notMnt {
		if conn, err := iscsilib.GetConnectorFromFile(stageConnectorFile(staging)); err == nil && conn != nil {
			device := conn.MountTargetDevice.GetPath()
			if device == "" {
				device = filepath.Join("/dev/disk/by-path",
					fmt.Sprintf("ip-%s-iscsi-%s-lun-%d", portal, iqn, lun))
				if err := ns.waitForPath(device, 60*time.Second); err != nil {
					return nil, status.Errorf(codes.Internal, "recover staging path: %v", err)
				}
			}
			if err := ns.mounter.FormatAndMount(device, staging, fsType, mountOpts); err != nil {
				return nil, status.Errorf(codes.Internal, "format+mount staging fallback failed: %v", err)
			}
		}
	}
	if err := os.MkdirAll(req.GetTargetPath(), 0o755); err != nil {
		return nil, status.Errorf(codes.Internal, "mkdir target: %v", err)
	}
	opts := []string{"bind"}
	if ro {
		opts = append(opts, "ro")
	}
	if err := ns.mounter.Mount(staging, req.GetTargetPath(), "", opts); err != nil {
		return nil, status.Errorf(codes.Internal, "bind-mount publish failed: %v", err)
	}
	return &csi.NodePublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "volumeID missing")
	}
	if req.GetTargetPath() == "" {
		return nil, status.Error(codes.InvalidArgument, "targetPath missing")
	}
	if err := mount.CleanupMountPoint(req.GetTargetPath(), ns.mounter, false); err != nil && !os.IsNotExist(err) {
		return nil, status.Errorf(codes.Internal, "cleanup target mount: %v", err)
	}
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

	available, capacity, used, inodes, inodesFree, inodesUsed, err := fsUsage(req.GetVolumePath())
	if err != nil {
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

	// Moved into iscsilib: rescan iSCSI + resize multipath (if any) + grow filesystem
	if err := iscsilib.ExpandVolume(ns.mounter.Interface, ns.resizer, req.GetVolumePath()); err != nil {
		return nil, status.Errorf(codes.Internal, "expand failed: %v", err)
	}

	return &csi.NodeExpandVolumeResponse{}, nil
}
