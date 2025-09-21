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
	"strconv"
	"strings"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	klog "k8s.io/klog/v2"
)

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
}

type driver struct {
	name     string
	nodeID   string
	version  string
	endpoint string
	cap      []*csi.VolumeCapability_AccessMode
	cscap    []*csi.ControllerServiceCapability

	backend Backend // <-- wired for controllerserver.go to use
}

const driverName = "iscsi.csi.windows.microsoft.com"

var version = "0.1.0"

func NewDriver(nodeID, endpoint string) *driver {
	klog.V(1).Infof("driver: %s version: %s nodeID: %s endpoint: %s", driverName, version, nodeID, endpoint)

	d := &driver{
		name:     driverName,
		version:  version,
		nodeID:   nodeID,
		endpoint: endpoint,
	}

	if err := os.MkdirAll(fmt.Sprintf("/var/run/%s", driverName), 0o755); err != nil {
		panic(err)
	}

	// Access modes we support for volumes
	d.AddVolumeCapabilityAccessModes([]csi.VolumeCapability_AccessMode_Mode{
		csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
		csi.VolumeCapability_AccessMode_SINGLE_NODE_SINGLE_WRITER,
		csi.VolumeCapability_AccessMode_SINGLE_NODE_MULTI_WRITER,
	})

	// Advertise Controller RPCs we actually implement (see controllerserver.go). :contentReference[oaicite:1]{index=1}
	d.AddControllerServiceCapabilities([]csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME,
		csi.ControllerServiceCapability_RPC_GET_CAPACITY,
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT,
		csi.ControllerServiceCapability_RPC_LIST_SNAPSHOTS,
		csi.ControllerServiceCapability_RPC_EXPAND_VOLUME,
		// If your CSI lib includes this enum (it should, since ControllerGetVolume is implemented):
		csi.ControllerServiceCapability_RPC_GET_VOLUME,
	})

	return d
}

func NewNodeServer(d *driver) *nodeServer {
	return &nodeServer{Driver: d}
}

// Provide a constructor for ControllerServer bound to this driver.
func NewControllerServer(d *driver) *ControllerServer {
	return &ControllerServer{Driver: d}
}

func (d *driver) Run() {
	// Build backend from environment and attach it.
	b, err := newWinRMBackendFromEnv()
	if err != nil {
		klog.Fatalf("failed to init WinRM backend: %v", err)
	}
	d.backend = b

	s := NewNonBlockingGRPCServer()
	s.Start(d.endpoint,
		NewDefaultIdentityServer(d),
		NewControllerServer(d),
		NewNodeServer(d))
	s.Wait()
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

	if imp := strings.TrimSpace(os.Getenv("WINRM_PS_IMPORT")); imp != "" {
		b.PSModuleImport = imp
	}
	return b, nil
}
