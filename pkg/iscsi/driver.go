// driver.go
/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
...
*/

package iscsi

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	klog "k8s.io/klog/v2"
)

var (
	newBackendFromEnvForRun        = newWinRMBackendFromEnv
	newNonBlockingGRPCServerForRun = NewNonBlockingGRPCServer
)

const driverRunDirEnv = "CSI_DRIVER_RUN_DIR"

// Backend is the minimal interface the ControllerServer needs.
// Implemented by WinRMBackend in backend_winrm.go.
type Backend interface {
	EnsureTarget(ctx context.Context, targetIQN string) error
	CreateVirtualDisk(ctx context.Context, name, parentDir string, sizeBytes int64) (string, int64, error)
	MapDiskToTarget(ctx context.Context, targetIQN, vhdxPath string) (int32, error)
	UnmapDiskFromTarget(ctx context.Context, targetIQN, vhdxPath string) error
	DeleteVirtualDisk(ctx context.Context, vhdxPath string) error
	GetVolumeByName(ctx context.Context, name, parentDir string) (bool, string, int64, string, int32, error)
	AllowInitiator(ctx context.Context, targetIQN, initiatorIQN string) error
	DenyInitiator(ctx context.Context, targetIQN, initiatorIQN string) error
	GetDirectoryFreeCapacity(ctx context.Context, parentDir string) (int64, error)
	// 03-snapshots
	CreateSnapshot(ctx context.Context, vhdxPath, description string) (SnapshotInfo, error)
	DeleteSnapshot(ctx context.Context, snapshotID string) error
	ListSnapshots(ctx context.Context, vhdxPath string) ([]SnapshotInfo, error)
	ExportSnapshotAsVirtualDisk(ctx context.Context, snapshotID string) (string, error)
	// expansion + query
	ResizeVirtualDisk(ctx context.Context, vhdxPath string, newSizeBytes int64) (int64, error)
	GetVolumeInfo(ctx context.Context, vhdxPath string) (VolumeInfo, error)
	GetTargetInitiators(ctx context.Context, targetIQN string) ([]string, error)

	CreateNfsShare(ctx context.Context, name, parentDir string, sizeBytes int64, clients []string, opts ...NfsShareOptions) (VolumeInfo, error)
	GetNfsShare(ctx context.Context, name, parentDir string) (bool, VolumeInfo, error)
	DeleteNfsShare(ctx context.Context, name, path string) error
	CreateSmbShare(ctx context.Context, name, parentDir string, sizeBytes int64, fullAccess, changeAccess, readAccess []string, opts ...SmbShareOptions) (VolumeInfo, error)
	GetSmbShare(ctx context.Context, name, parentDir string) (bool, VolumeInfo, error)
	DeleteSmbShare(ctx context.Context, name, path string) error
	ResizeFileShare(ctx context.Context, path string, newSizeBytes int64) (int64, error)
	RestoreSnapshotAsFileShare(ctx context.Context, snapshotID, destinationPath string) error
	MountFileShareVirtualDisk(ctx context.Context, vhdxPath, mountPath string) error
	UnmountFileShareVirtualDisk(ctx context.Context, vhdxPath, mountPath string) error
}

const (
	fileShareBackendDirectory = "directory"
	fileShareBackendVHDX      = "vhdx"
)

type NfsShareOptions struct {
	ClientType            string
	Permission            string
	AllowRootAccess       *bool
	Authentication        []string
	AnonymousUID          *int
	AnonymousGID          *int
	LanguageEncoding      string
	EnableAnonymousAccess *bool
	EnableUnmappedAccess  *bool
}

type SmbShareOptions struct {
	NoAccess              []string
	Description           string
	EncryptData           *bool
	CompressData          *bool
	ContinuouslyAvailable *bool
	CachingMode           string
	FolderEnumerationMode string
	ConcurrentUserLimit   uint32
}

type driver struct {
	name             string
	protocol         Protocol
	fileShareBackend string
	nodeID           string
	version          string
	endpoint         string
	cap              []*csi.VolumeCapability_AccessMode
	cscap            []*csi.ControllerServiceCapability

	backend Backend // <-- wired for controllerserver.go to use
}

const driverName = "iscsi.csi.windows.microsoft.com"
const nfsDriverName = "nfs.csi.windows.microsoft.com"
const smbDriverName = "smb.csi.windows.microsoft.com"
const nfsVHDXDriverName = "nfs-vhdx.csi.windows.microsoft.com"
const smbVHDXDriverName = "smb-vhdx.csi.windows.microsoft.com"

var version = "0.1.0"

func NewDriver(nodeID, endpoint string) *driver {
	return NewProtocolDriver(ProtocolISCSI, nodeID, endpoint)
}

func NewProtocolDriver(protocol Protocol, nodeID, endpoint string) *driver {
	name, err := driverNameForProtocol(protocol)
	if err != nil {
		klog.Warningf("unknown driver protocol %q; defaulting to %s", protocol, ProtocolISCSI)
		protocol = ProtocolISCSI
		name = driverName
	}
	return newNamedProtocolDriver(name, protocol, nodeID, endpoint)
}

func NewNamedDriver(name, nodeID, endpoint string) *driver {
	protocol, backend, err := driverConfigForName(name)
	if err != nil {
		klog.Warningf("unknown CSI driver name %q; defaulting protocol to %s", name, ProtocolISCSI)
		protocol = ProtocolISCSI
		backend = ""
	}
	return newNamedProtocolDriverWithShareBackend(name, protocol, backend, nodeID, endpoint)
}

func newNamedProtocolDriver(name string, protocol Protocol, nodeID, endpoint string) *driver {
	backend := ""
	if protocol == ProtocolNFS || protocol == ProtocolSMB {
		backend = fileShareBackendDirectory
	}
	return newNamedProtocolDriverWithShareBackend(name, protocol, backend, nodeID, endpoint)
}

func newNamedProtocolDriverWithShareBackend(name string, protocol Protocol, fileShareBackend, nodeID, endpoint string) *driver {
	klog.V(1).Infof("driver: %s protocol: %s version: %s nodeID: %s endpoint: %s", name, protocol, version, nodeID, endpoint)

	d := &driver{
		name:             driverName,
		protocol:         protocol,
		fileShareBackend: fileShareBackend,
		version:          version,
		nodeID:           nodeID,
		endpoint:         endpoint,
	}
	d.name = name

	accessModes := []csi.VolumeCapability_AccessMode_Mode{
		csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
		csi.VolumeCapability_AccessMode_SINGLE_NODE_SINGLE_WRITER,
		csi.VolumeCapability_AccessMode_SINGLE_NODE_MULTI_WRITER,
	}
	if protocol == ProtocolNFS || protocol == ProtocolSMB {
		accessModes = append(accessModes,
			csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY,
			csi.VolumeCapability_AccessMode_MULTI_NODE_SINGLE_WRITER,
			csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER)
	}
	d.AddVolumeCapabilityAccessModes(accessModes)

	// Advertise Controller RPCs we actually implement (see controllerserver.go). :contentReference[oaicite:1]{index=1}
	controllerCaps := []csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME,
		csi.ControllerServiceCapability_RPC_GET_CAPACITY,
		csi.ControllerServiceCapability_RPC_EXPAND_VOLUME,
		// If your CSI lib includes this enum (it should, since ControllerGetVolume is implemented):
		csi.ControllerServiceCapability_RPC_GET_VOLUME,
	}
	if protocol == ProtocolISCSI || fileShareBackend == fileShareBackendVHDX {
		controllerCaps = append(controllerCaps, csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT)
	}
	if protocol == ProtocolISCSI || fileShareBackend == fileShareBackendVHDX {
		controllerCaps = append(controllerCaps, csi.ControllerServiceCapability_RPC_LIST_SNAPSHOTS)
	}
	d.AddControllerServiceCapabilities(controllerCaps)

	return d
}

func driverNameForProtocol(protocol Protocol) (string, error) {
	switch protocol {
	case ProtocolISCSI:
		return driverName, nil
	case ProtocolNFS:
		return nfsDriverName, nil
	case ProtocolSMB:
		return smbDriverName, nil
	default:
		return "", fmt.Errorf("unknown protocol: %s", protocol)
	}
}

func protocolForDriverName(name string) (Protocol, error) {
	protocol, _, err := driverConfigForName(name)
	return protocol, err
}

func driverConfigForName(name string) (Protocol, string, error) {
	switch strings.TrimSpace(name) {
	case driverName:
		return ProtocolISCSI, "", nil
	case nfsDriverName:
		return ProtocolNFS, fileShareBackendDirectory, nil
	case smbDriverName:
		return ProtocolSMB, fileShareBackendDirectory, nil
	case nfsVHDXDriverName:
		return ProtocolNFS, fileShareBackendVHDX, nil
	case smbVHDXDriverName:
		return ProtocolSMB, fileShareBackendVHDX, nil
	default:
		return "", "", fmt.Errorf("unknown CSI driver name: %s", name)
	}
}

func NewNodeServer(d *driver) *nodeServer {
	return &nodeServer{Driver: d}
}

// Provide a constructor for ControllerServer bound to this driver.
func NewControllerServer(d *driver) *ControllerServer {
	return &ControllerServer{Driver: d}
}

func (d *driver) Run() {
	d.ensureRunDirectory()

	// Build backend from environment and attach it.
	b, err := newBackendFromEnvForRun()
	if err != nil {
		klog.Fatalf("failed to init WinRM backend: %v", err)
	}
	d.backend = b

	s := newNonBlockingGRPCServerForRun()
	s.Start(d.endpoint,
		NewDefaultIdentityServer(d),
		NewControllerServer(d),
		NewNodeServer(d))
	s.Wait()
}

func (d *driver) ensureRunDirectory() {
	baseDir := strings.TrimSpace(os.Getenv(driverRunDirEnv))
	if baseDir == "" {
		baseDir = "/var/run"
	}

	if err := os.MkdirAll(filepath.Join(baseDir, d.name), 0o755); err != nil {
		klog.Warningf("failed to create driver run directory: %v", err)
	}
}

func (d *driver) AddVolumeCapabilityAccessModes(vc []csi.VolumeCapability_AccessMode_Mode) []*csi.VolumeCapability_AccessMode {
	var vca []*csi.VolumeCapability_AccessMode
	for _, c := range vc {
		klog.Infof("enabling volume access mode: %v", c.String())
		vca = append(vca, &csi.VolumeCapability_AccessMode{Mode: c})
	}
	d.cap = vca
	return vca
}

func (d *driver) AddControllerServiceCapabilities(cl []csi.ControllerServiceCapability_RPC_Type) {
	var csc []*csi.ControllerServiceCapability
	for _, c := range cl {
		klog.Infof("enabling controller service capability: %v", c.String())
		csc = append(csc, NewControllerServiceCapability(c))
	}
	d.cscap = csc
}

// -------------------- WinRM backend config --------------------

func getenvDefault(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func parseBoolDefault(s string, def bool) bool {
	if s == "" {
		return def
	}
	switch strings.ToLower(s) {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return def
	}
}

func newWinRMBackendFromEnv() (Backend, error) {
	host := getenvDefault("WINRM_HOST", "")
	if host == "" {
		return nil, fmt.Errorf("WINRM_HOST is required")
	}
	portStr := getenvDefault("WINRM_PORT", "")
	var port int
	if portStr == "" {
		// default port depends on TLS
		if parseBoolDefault(os.Getenv("WINRM_TLS"), false) {
			port = 5986
		} else {
			port = 5985
		}
	} else {
		p, err := strconv.Atoi(portStr)
		if err != nil || p <= 0 {
			return nil, fmt.Errorf("invalid WINRM_PORT: %q", portStr)
		}
		port = p
	}

	useTLS := parseBoolDefault(os.Getenv("WINRM_TLS"), false)
	insecure := parseBoolDefault(os.Getenv("WINRM_INSECURE"), true) // allow self-signed by default
	user := getenvDefault("WINRM_USER", "")
	pass := os.Getenv("WINRM_PASSWORD")
	if user == "" || pass == "" {
		return nil, fmt.Errorf("WINRM_USER and WINRM_PASSWORD are required")
	}

	timeout := 60 * time.Second
	if t := strings.TrimSpace(os.Getenv("WINRM_TIMEOUT")); t != "" {
		if dur, err := time.ParseDuration(t); err == nil {
			timeout = dur
		}
	}
	b := NewWinRMBackend(host, port, useTLS, insecure, user, pass, timeout)
	b.Auth = getenvDefault("WINRM_AUTH", "basic")

	if imp := strings.TrimSpace(os.Getenv("WINRM_PS_IMPORT")); imp != "" {
		b.PSModuleImport = imp
	}
	return b, nil
}
