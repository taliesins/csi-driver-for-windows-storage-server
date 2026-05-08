package iscsi

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	iscsilib "github.com/taliesins/csi-driver-for-windows-storage-server/pkg/iscsilib"
	mount "k8s.io/mount-utils"
)

// ---------------------------------------------------------------------------
// mockMount implements mount.Interface for testing
// ---------------------------------------------------------------------------

type mockMount struct {
	mountPaths       []string
	unmountPaths     []string
	formatAndMounts  []formatMountRecord
	isMountPointErrs map[string]error
}

type formatMountRecord struct {
	source  string
	target  string
	fsType  string
	options []string
}

func (m *mockMount) List() ([]mount.MountPoint, error) {
	var result []mount.MountPoint
	for _, p := range m.mountPaths {
		result = append(result, mount.MountPoint{Path: p})
	}
	return result, nil
}

func (m *mockMount) IsMountPoint(file string) (bool, error) {
	if m.isMountPointErrs != nil {
		if err, ok := m.isMountPointErrs[file]; ok {
			return false, err
		}
	}
	return containsString(m.mountPaths, file), nil
}

func (m *mockMount) IsLikelyNotMountPoint(file string) (bool, error) {
	if m.isMountPointErrs != nil {
		if err, ok := m.isMountPointErrs[file]; ok {
			return false, err
		}
	}
	return !containsString(m.mountPaths, file), nil
}

func (m *mockMount) CanSafelySkipMountPointCheck() bool {
	return false
}

func (m *mockMount) GetMountRefs(path string) ([]string, error) {
	return nil, nil
}

func (m *mockMount) Mount(source, target, fstype string, options []string) error {
	m.mountPaths = append(m.mountPaths, target)
	m.formatAndMounts = append(m.formatAndMounts, formatMountRecord{source: source, target: target, fsType: fstype, options: options})
	return nil
}

func (m *mockMount) MountSensitive(source, target, fstype string, options, sensitiveOptions []string) error {
	return nil
}

func (m *mockMount) MountSensitiveWithoutSystemd(source, target, fstype string, options, sensitiveOptions []string) error {
	return nil
}

func (m *mockMount) MountSensitiveWithoutSystemdWithMountFlags(source, target, fstype string, options, sensitiveOptions, mountFlags []string) error {
	return nil
}

func (m *mockMount) Unmount(target string) error {
	m.unmountPaths = append(m.unmountPaths, target)
	for i, path := range m.mountPaths {
		if path == target {
			m.mountPaths = append(m.mountPaths[:i], m.mountPaths[i+1:]...)
			break
		}
	}
	return nil
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Test helper - create a nodeServer with mock mount
// ---------------------------------------------------------------------------

func newTestNodeServer(t *testing.T) (*nodeServer, *driver, *mockMount) {
	t.Helper()
	mm := &mockMount{
		isMountPointErrs: make(map[string]error),
	}
	d := NewDriver("node-001", "unix:///var/run/csi/csi.sock")
	ns := &nodeServer{
		Driver:  d,
		mounter: &mount.SafeFormatAndMount{Interface: mm},
	}
	return ns, d, mm
}

// ---------------------------------------------------------------------------
// NodeStageVolume tests
// ---------------------------------------------------------------------------

func TestNodeStageVolume_VolumeIDRequired(t *testing.T) {
	ns, _, _ := newTestNodeServer(t)
	_, err := ns.NodeStageVolume(context.Background(), &csi.NodeStageVolumeRequest{
		VolumeId:          "",
		StagingTargetPath: "/staging",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "volumeID missing")
}

func TestNodeStageVolume_StagingTargetPathRequired(t *testing.T) {
	ns, _, _ := newTestNodeServer(t)
	_, err := ns.NodeStageVolume(context.Background(), &csi.NodeStageVolumeRequest{
		VolumeId: "test-vol",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "stagingTargetPath missing")
}

func TestNodeStageVolume_VolumeCapabilityRequired(t *testing.T) {
	ns, _, _ := newTestNodeServer(t)
	_, err := ns.NodeStageVolume(context.Background(), &csi.NodeStageVolumeRequest{
		VolumeId:          "test-vol",
		StagingTargetPath: "/staging",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "volumeCapability missing")
}

func TestStageConnectorFileIsOutsideStagingMount(t *testing.T) {
	stagingTargetPath := filepath.Join(t.TempDir(), "globalmount")

	assert.Equal(t, filepath.Dir(stagingTargetPath), filepath.Dir(stageConnectorFile(stagingTargetPath)))
	assert.NotEqual(t, legacyStageConnectorFile(stagingTargetPath), stageConnectorFile(stagingTargetPath))
}

func TestNodeStageVolume_IscsiConnectionRequired(t *testing.T) {
	ns, _, _ := newTestNodeServer(t)
	_, err := ns.NodeStageVolume(context.Background(), &csi.NodeStageVolumeRequest{
		VolumeId:          "test-vol",
		StagingTargetPath: "/staging",
		VolumeCapability:  &csi.VolumeCapability{},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "targetPortal is required")
}

func TestNodeStageVolume_InvalidIscsiInputs(t *testing.T) {
	tests := []struct {
		name           string
		publishContext map[string]string
		secrets        map[string]string
		wantErr        string
	}{
		{
			name: "target portal is URL",
			publishContext: map[string]string{
				"targetPortal": "https://storage.example.test/wsman",
				"iqn":          "iqn.2024-01.com.example:test-volume",
				"lun":          "0",
			},
			wantErr: "targetPortal must be a host",
		},
		{
			name: "negative lun",
			publishContext: map[string]string{
				"targetPortal": "10.0.0.1:3260",
				"iqn":          "iqn.2024-01.com.example:test-volume",
				"lun":          "-1",
			},
			wantErr: "lun must be non-negative",
		},
		{
			name: "short chap secret",
			publishContext: map[string]string{
				"targetPortal": "10.0.0.1:3260",
				"iqn":          "iqn.2024-01.com.example:test-volume",
				"lun":          "0",
			},
			secrets: map[string]string{
				"node.session.auth.username": "session-user",
				"node.session.auth.password": "short",
			},
			wantErr: "between 12 and 16",
		},
		{
			name: "discovery reverse requires discovery chap",
			publishContext: map[string]string{
				"targetPortal": "10.0.0.1:3260",
				"iqn":          "iqn.2024-01.com.example:test-volume",
				"lun":          "0",
			},
			secrets: map[string]string{
				"discovery.sendtargets.auth.username_in": "disc-user-in",
				"discovery.sendtargets.auth.password_in": "disc-pass-in",
			},
			wantErr: "discovery reverse CHAP requires",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ns, _, _ := newTestNodeServer(t)
			_, err := ns.NodeStageVolume(context.Background(), &csi.NodeStageVolumeRequest{
				VolumeId:          "test-vol",
				StagingTargetPath: "/staging",
				VolumeCapability: &csi.VolumeCapability{
					AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
					AccessType: &csi.VolumeCapability_Block{Block: &csi.VolumeCapability_BlockVolume{}},
				},
				PublishContext: tt.publishContext,
				Secrets:        tt.secrets,
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestNodeStageVolume_BlockVolume(t *testing.T) {
	ns, _, mockMount := newTestNodeServer(t)
	stagingTargetPath := t.TempDir()

	var gotConnector iscsilib.Connector
	originalConnect := iscsilibConnect
	iscsilibConnect = func(c *iscsilib.Connector) (string, error) {
		c.MountTargetDevice = &iscsilib.Device{Name: "sdb", Type: "disk"}
		gotConnector = *c
		return c.MountTargetDevice.GetPath(), nil
	}
	t.Cleanup(func() { iscsilibConnect = originalConnect })

	_, err := ns.NodeStageVolume(context.Background(), &csi.NodeStageVolumeRequest{
		VolumeId:          "test-vol",
		StagingTargetPath: stagingTargetPath,
		VolumeCapability: &csi.VolumeCapability{
			AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
			AccessType: &csi.VolumeCapability_Block{Block: &csi.VolumeCapability_BlockVolume{}},
		},
		PublishContext: map[string]string{
			"targetPortal": "10.0.0.1:3260",
			"iqn":          "iqn.2024-01.com.example:test-volume",
			"lun":          "2",
		},
		VolumeContext: map[string]string{
			"iface":        "iface0",
			"mountOptions": "ignored-for-block",
		},
		Secrets: map[string]string{
			"discovery.sendtargets.auth.username":    "disc-user",
			"discovery.sendtargets.auth.password":    "DiscPass1234",
			"discovery.sendtargets.auth.username_in": "disc-user-in",
			"discovery.sendtargets.auth.password_in": "disc-pass-in",
			"node.session.auth.username":             "session-user",
			"node.session.auth.password":             "session-pass",
			"node.session.auth.username_in":          "session-user-in",
			"node.session.auth.password_in":          "session-pass-in",
		},
	})

	require.NoError(t, err)
	assert.Equal(t, "test-vol", gotConnector.VolumeName)
	assert.Equal(t, "iqn.2024-01.com.example:test-volume", gotConnector.TargetIqn)
	assert.Equal(t, []string{"10.0.0.1:3260"}, gotConnector.TargetPortals)
	assert.Equal(t, int32(2), gotConnector.Lun)
	assert.Equal(t, "iface0", gotConnector.Interface)
	assert.True(t, gotConnector.DoDiscovery)
	assert.True(t, gotConnector.DoCHAPDiscovery)
	assert.Equal(t, "chap", gotConnector.AuthType)
	assert.Equal(t, iscsilib.Secrets{
		SecretsType: "chap",
		UserName:    "disc-user",
		Password:    "DiscPass1234",
		UserNameIn:  "disc-user-in",
		PasswordIn:  "disc-pass-in",
	}, gotConnector.DiscoverySecrets)
	assert.Equal(t, iscsilib.Secrets{
		SecretsType: "chap",
		UserName:    "session-user",
		Password:    "session-pass",
		UserNameIn:  "session-user-in",
		PasswordIn:  "session-pass-in",
	}, gotConnector.SessionSecrets)
	assert.FileExists(t, stageConnectorFile(stagingTargetPath))
	rawConnector, err := os.ReadFile(stageConnectorFile(stagingTargetPath))
	require.NoError(t, err)
	var persistedConnector iscsilib.Connector
	require.NoError(t, json.Unmarshal(rawConnector, &persistedConnector))
	require.NotNil(t, persistedConnector.MountTargetDevice)
	assert.Equal(t, "sdb", persistedConnector.MountTargetDevice.Name)
	assert.Empty(t, mockMount.formatAndMounts)
}

func TestNodeStageVolume_StaticISCSIUsesVolumeContextWithoutPublishContext(t *testing.T) {
	ns, _, _ := newTestNodeServer(t)
	stagingTargetPath := t.TempDir()

	var gotConnector iscsilib.Connector
	originalConnect := iscsilibConnect
	iscsilibConnect = func(c *iscsilib.Connector) (string, error) {
		c.MountTargetDevice = &iscsilib.Device{Name: "sdb", Type: "disk"}
		gotConnector = *c
		return c.MountTargetDevice.GetPath(), nil
	}
	t.Cleanup(func() { iscsilibConnect = originalConnect })

	resp, err := ns.NodeStageVolume(context.Background(), &csi.NodeStageVolumeRequest{
		VolumeId:          "static-iscsi-volume",
		StagingTargetPath: stagingTargetPath,
		VolumeCapability: &csi.VolumeCapability{
			AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
			AccessType: &csi.VolumeCapability_Block{Block: &csi.VolumeCapability_BlockVolume{}},
		},
		VolumeContext: map[string]string{
			"targetPortal": "10.0.0.1:3260",
			"targetIQN":    "iqn.2024-01.com.example:static-volume",
			"lun":          "3",
			"iface":        "storage0",
		},
	})

	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, "static-iscsi-volume", gotConnector.VolumeName)
	assert.Equal(t, "iqn.2024-01.com.example:static-volume", gotConnector.TargetIqn)
	assert.Equal(t, []string{"10.0.0.1:3260"}, gotConnector.TargetPortals)
	assert.Equal(t, int32(3), gotConnector.Lun)
	assert.Equal(t, "storage0", gotConnector.Interface)
}

func TestNodeStageVolume_StaticISCSIUsesVolumeIDWithoutPublishContext(t *testing.T) {
	ns, _, _ := newTestNodeServer(t)
	stagingTargetPath := t.TempDir()

	var gotConnector iscsilib.Connector
	originalConnect := iscsilibConnect
	iscsilibConnect = func(c *iscsilib.Connector) (string, error) {
		c.MountTargetDevice = &iscsilib.Device{Name: "sdb", Type: "disk"}
		gotConnector = *c
		return c.MountTargetDevice.GetPath(), nil
	}
	t.Cleanup(func() { iscsilibConnect = originalConnect })

	resp, err := ns.NodeStageVolume(context.Background(), &csi.NodeStageVolumeRequest{
		VolumeId:          "iscsi://10.0.0.1:3260/iqn.2024-01.com.example:static-volume/lun/4?name=static-iscsi-volume",
		StagingTargetPath: stagingTargetPath,
		VolumeCapability: &csi.VolumeCapability{
			AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
			AccessType: &csi.VolumeCapability_Block{Block: &csi.VolumeCapability_BlockVolume{}},
		},
	})

	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, "iqn.2024-01.com.example:static-volume", gotConnector.TargetIqn)
	assert.Equal(t, []string{"10.0.0.1:3260"}, gotConnector.TargetPortals)
	assert.Equal(t, int32(4), gotConnector.Lun)
}

func TestNodeStageVolume_FilesystemFsType(t *testing.T) {
	tests := []struct {
		name             string
		capabilityFsType string
		volumeContext    map[string]string
		wantFsType       string
		wantOptions      []string
	}{
		{
			name:             "uses CSI mount fs type",
			capabilityFsType: "xfs",
			volumeContext:    map[string]string{"mountOptions": "discard,noatime"},
			wantFsType:       "xfs",
			wantOptions:      []string{"discard", "noatime"},
		},
		{
			name:             "trims CSI mount fs type",
			capabilityFsType: " xfs ",
			wantFsType:       "xfs",
		},
		{
			name:          "falls back to volume context fs type",
			volumeContext: map[string]string{"fsType": "xfs"},
			wantFsType:    "xfs",
		},
		{
			name:             "prefers CSI mount fs type over volume context",
			capabilityFsType: "xfs",
			volumeContext:    map[string]string{"fsType": "ext4"},
			wantFsType:       "xfs",
		},
		{
			name:       "defaults to ext4",
			wantFsType: "ext4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ns, _, _ := newTestNodeServer(t)
			stagingTargetPath := t.TempDir()

			originalConnect := iscsilibConnect
			iscsilibConnect = func(c *iscsilib.Connector) (string, error) {
				c.MountTargetDevice = &iscsilib.Device{Name: "sdb", Type: "disk"}
				return "/dev/sdb", nil
			}
			t.Cleanup(func() { iscsilibConnect = originalConnect })

			var gotSource, gotTarget, gotFsType string
			var gotOptions []string
			originalFormatAndMount := formatAndMount
			formatAndMount = func(m *mount.SafeFormatAndMount, source, target, fsType string, options []string) error {
				gotSource = source
				gotTarget = target
				gotFsType = fsType
				gotOptions = append([]string(nil), options...)
				return nil
			}
			t.Cleanup(func() { formatAndMount = originalFormatAndMount })

			resp, err := ns.NodeStageVolume(context.Background(), &csi.NodeStageVolumeRequest{
				VolumeId:          "test-vol",
				StagingTargetPath: stagingTargetPath,
				VolumeCapability: &csi.VolumeCapability{
					AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
					AccessType: &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{
						FsType:     tt.capabilityFsType,
						MountFlags: []string{"rw"},
					}},
				},
				PublishContext: map[string]string{
					"targetPortal": "10.0.0.1:3260",
					"iqn":          "iqn.2024-01.com.example:test-volume",
					"lun":          "0",
				},
				VolumeContext: tt.volumeContext,
			})

			require.NoError(t, err)
			assert.NotNil(t, resp)
			assert.Equal(t, "/dev/sdb", gotSource)
			assert.Equal(t, stagingTargetPath, gotTarget)
			assert.Equal(t, tt.wantFsType, gotFsType)
			assert.Contains(t, gotOptions, "rw")
			for _, opt := range tt.wantOptions {
				assert.Contains(t, gotOptions, opt)
			}
		})
	}
}

func TestNodeStageVolume_NFS(t *testing.T) {
	ns, _, mockMount := newTestNodeServer(t)
	stagingTargetPath := t.TempDir()

	resp, err := ns.NodeStageVolume(context.Background(), &csi.NodeStageVolumeRequest{
		VolumeId:          "nfs://10.0.0.2/export/test",
		StagingTargetPath: stagingTargetPath,
		VolumeCapability: &csi.VolumeCapability{
			AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
			AccessType: &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{
				MountFlags: []string{"sync"},
			}},
		},
		PublishContext: map[string]string{
			"protocol":      "nfs",
			"nfsServer":     "10.0.0.2",
			"nfsExportPath": "/export/test",
		},
		VolumeContext: map[string]string{
			"mountOptions":      "nfsvers=4.1,hard",
			"nfsVersion":        "4.2",
			"nfsAuthentication": "sys,krb5p",
		},
	})

	require.NoError(t, err)
	assert.NotNil(t, resp)
	require.Len(t, mockMount.formatAndMounts, 1)
	assert.Equal(t, "10.0.0.2:/export/test", mockMount.formatAndMounts[0].source)
	assert.Equal(t, "nfs", mockMount.formatAndMounts[0].fsType)
	assert.Contains(t, mockMount.formatAndMounts[0].options, "sync")
	assert.Contains(t, mockMount.formatAndMounts[0].options, "nfsvers=4.1")
	assert.Contains(t, mockMount.formatAndMounts[0].options, "vers=4.2")
	assert.Contains(t, mockMount.formatAndMounts[0].options, "sec=krb5p")
}

func TestNodeStageVolume_NFSWithoutPublishContextUsesVolumeContext(t *testing.T) {
	ns, _, mockMount := newTestNodeServer(t)
	stagingTargetPath := t.TempDir()

	resp, err := ns.NodeStageVolume(context.Background(), &csi.NodeStageVolumeRequest{
		VolumeId:          "nfs://10.0.0.2/export/test",
		StagingTargetPath: stagingTargetPath,
		VolumeCapability: &csi.VolumeCapability{
			AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
			AccessType: &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{}},
		},
		VolumeContext: map[string]string{
			"protocol":               "nfs",
			"nfsServer":              "10.0.0.4",
			"nfsExportPath":          "/export/context",
			"nfsMountAuthentication": "krb5i",
		},
	})

	require.NoError(t, err)
	assert.NotNil(t, resp)
	require.Len(t, mockMount.formatAndMounts, 1)
	assert.Equal(t, "10.0.0.4:/export/context", mockMount.formatAndMounts[0].source)
	assert.Equal(t, "nfs", mockMount.formatAndMounts[0].fsType)
	assert.Contains(t, mockMount.formatAndMounts[0].options, "sec=krb5i")
}

func TestNodeStageVolume_SMB(t *testing.T) {
	ns, _, mockMount := newTestNodeServer(t)
	stagingTargetPath := t.TempDir()

	resp, err := ns.NodeStageVolume(context.Background(), &csi.NodeStageVolumeRequest{
		VolumeId:          "smb://10.0.0.3/share",
		StagingTargetPath: stagingTargetPath,
		VolumeCapability: &csi.VolumeCapability{
			AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
			AccessType: &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{}},
		},
		PublishContext: map[string]string{
			"protocol":     "smb",
			"smbServer":    "10.0.0.3",
			"smbShareName": "share",
		},
		VolumeContext: map[string]string{
			"smbVersion": "3.1.1",
			"smbSeal":    "true",
		},
		Secrets: map[string]string{
			"smbUsername": "storage-user",
			"smbPassword": "storage-pass",
			"smbDomain":   "EXAMPLE",
		},
	})

	require.NoError(t, err)
	assert.NotNil(t, resp)
	require.Len(t, mockMount.formatAndMounts, 1)
	assert.Equal(t, "//10.0.0.3/share", mockMount.formatAndMounts[0].source)
	assert.Equal(t, "cifs", mockMount.formatAndMounts[0].fsType)
	assert.Contains(t, mockMount.formatAndMounts[0].options, "vers=3.1.1")
	assert.Contains(t, mockMount.formatAndMounts[0].options, "username=storage-user")
	assert.Contains(t, mockMount.formatAndMounts[0].options, "password=storage-pass")
	assert.Contains(t, mockMount.formatAndMounts[0].options, "domain=EXAMPLE")
	assert.Contains(t, mockMount.formatAndMounts[0].options, "seal")
}

func TestNodeStageVolume_SMBWithoutPublishContextUsesVolumeID(t *testing.T) {
	ns, _, mockMount := newTestNodeServer(t)
	stagingTargetPath := t.TempDir()

	resp, err := ns.NodeStageVolume(context.Background(), &csi.NodeStageVolumeRequest{
		VolumeId:          "smb://10.0.0.3/share",
		StagingTargetPath: stagingTargetPath,
		VolumeCapability: &csi.VolumeCapability{
			AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
			AccessType: &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{}},
		},
	})

	require.NoError(t, err)
	assert.NotNil(t, resp)
	require.Len(t, mockMount.formatAndMounts, 1)
	assert.Equal(t, "//10.0.0.3/share", mockMount.formatAndMounts[0].source)
	assert.Equal(t, "cifs", mockMount.formatAndMounts[0].fsType)
}

// ---------------------------------------------------------------------------
// NodeUnstageVolume tests
// ---------------------------------------------------------------------------

func TestNodeUnstageVolume_VolumeIDRequired(t *testing.T) {
	ns, _, _ := newTestNodeServer(t)
	_, err := ns.NodeUnstageVolume(context.Background(), &csi.NodeUnstageVolumeRequest{
		VolumeId:          "",
		StagingTargetPath: "/staging",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "volumeID missing")
}

func TestNodeUnstageVolume_StagingTargetPathRequired(t *testing.T) {
	ns, _, _ := newTestNodeServer(t)
	_, err := ns.NodeUnstageVolume(context.Background(), &csi.NodeUnstageVolumeRequest{
		VolumeId: "test-vol",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "stagingTargetPath missing")
}

func TestNodeUnstageVolume_Success(t *testing.T) {
	ns, _, _ := newTestNodeServer(t)
	stagingTargetPath := t.TempDir()

	resp, err := ns.NodeUnstageVolume(context.Background(), &csi.NodeUnstageVolumeRequest{
		VolumeId:          "test-vol",
		StagingTargetPath: stagingTargetPath,
	})

	require.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestNodeUnstageVolume_DisconnectsISCSISession(t *testing.T) {
	ns, _, mockMount := newTestNodeServer(t)
	stagingTargetPath := t.TempDir()
	mockMount.mountPaths = []string{stagingTargetPath}
	conn := &iscsilib.Connector{
		TargetIqn:     "iqn.2024-01.com.example:test-volume",
		TargetPortals: []string{"10.0.0.1:3260"},
	}

	originalGetConnector := getConnectorFromFile
	originalDisconnectVolume := disconnectVolume
	originalDisconnect := iscsilibDisconnect
	calls := []string{}
	getConnectorFromFile = func(filePath string) (*iscsilib.Connector, error) {
		assert.Equal(t, stageConnectorFile(stagingTargetPath), filePath)
		return conn, nil
	}
	disconnectVolume = func(got *iscsilib.Connector) error {
		assert.Same(t, conn, got)
		assert.Equal(t, []string{stagingTargetPath}, mockMount.unmountPaths)
		calls = append(calls, "device")
		return nil
	}
	iscsilibDisconnect = func(got *iscsilib.Connector) error {
		assert.Same(t, conn, got)
		calls = append(calls, "session")
		return nil
	}
	t.Cleanup(func() {
		getConnectorFromFile = originalGetConnector
		disconnectVolume = originalDisconnectVolume
		iscsilibDisconnect = originalDisconnect
	})

	resp, err := ns.NodeUnstageVolume(context.Background(), &csi.NodeUnstageVolumeRequest{
		VolumeId:          "test-vol",
		StagingTargetPath: stagingTargetPath,
	})

	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, []string{"device", "session"}, calls)
}

func TestNodeUnstageVolume_ReadsLegacyConnectorAfterUnmount(t *testing.T) {
	ns, _, mockMount := newTestNodeServer(t)
	stagingTargetPath := t.TempDir()
	mockMount.mountPaths = []string{stagingTargetPath}
	require.NoError(t, os.WriteFile(legacyStageConnectorFile(stagingTargetPath), []byte(`{
  "target_iqn": "iqn.2024-01.com.example:test-volume",
  "target_portal": ["10.0.0.1:3260"],
  "mount_target_device": {"name": "sdb", "type": "disk"},
  "devices": [{"name": "sdb", "type": "disk"}]
}`), 0o600))

	originalGetConnector := getConnectorFromFile
	originalDisconnectVolume := disconnectVolume
	originalDisconnect := iscsilibDisconnect
	var disconnected bool
	getConnectorFromFile = func(filePath string) (*iscsilib.Connector, error) {
		if filePath == legacyStageConnectorFile(stagingTargetPath) && len(mockMount.unmountPaths) > 0 {
			return &iscsilib.Connector{
				TargetIqn:     "iqn.2024-01.com.example:test-volume",
				TargetPortals: []string{"10.0.0.1:3260"},
			}, nil
		}
		return nil, os.ErrNotExist
	}
	disconnectVolume = func(got *iscsilib.Connector) error {
		assert.Equal(t, "iqn.2024-01.com.example:test-volume", got.TargetIqn)
		disconnected = true
		return nil
	}
	iscsilibDisconnect = func(got *iscsilib.Connector) error {
		assert.True(t, disconnected)
		return nil
	}
	t.Cleanup(func() {
		getConnectorFromFile = originalGetConnector
		disconnectVolume = originalDisconnectVolume
		iscsilibDisconnect = originalDisconnect
	})

	resp, err := ns.NodeUnstageVolume(context.Background(), &csi.NodeUnstageVolumeRequest{
		VolumeId:          "test-vol",
		StagingTargetPath: stagingTargetPath,
	})

	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.True(t, disconnected)
	assert.NoFileExists(t, legacyStageConnectorFile(stagingTargetPath))
}

func TestNodeUnstageVolume_ReturnsErrorWhenSessionDisconnectFails(t *testing.T) {
	ns, _, _ := newTestNodeServer(t)
	stagingTargetPath := t.TempDir()

	originalGetConnector := getConnectorFromFile
	originalDisconnectVolume := disconnectVolume
	originalDisconnect := iscsilibDisconnect
	getConnectorFromFile = func(filePath string) (*iscsilib.Connector, error) {
		return &iscsilib.Connector{}, nil
	}
	disconnectVolume = func(got *iscsilib.Connector) error {
		return nil
	}
	iscsilibDisconnect = func(got *iscsilib.Connector) error {
		return errors.New("logout failed")
	}
	t.Cleanup(func() {
		getConnectorFromFile = originalGetConnector
		disconnectVolume = originalDisconnectVolume
		iscsilibDisconnect = originalDisconnect
	})

	_, err := ns.NodeUnstageVolume(context.Background(), &csi.NodeUnstageVolumeRequest{
		VolumeId:          "test-vol",
		StagingTargetPath: stagingTargetPath,
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "iSCSI logout failed")
}

// ---------------------------------------------------------------------------
// NodePublishVolume tests
// ---------------------------------------------------------------------------

func TestNodePublishVolume_VolumeIDRequired(t *testing.T) {
	ns, _, _ := newTestNodeServer(t)
	_, err := ns.NodePublishVolume(context.Background(), &csi.NodePublishVolumeRequest{
		VolumeId: "",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "volumeID missing")
}

func TestNodePublishVolume_TargetPathRequired(t *testing.T) {
	ns, _, _ := newTestNodeServer(t)
	_, err := ns.NodePublishVolume(context.Background(), &csi.NodePublishVolumeRequest{
		VolumeId: "test-vol",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "targetPath missing")
}

func TestNodePublishVolume_VolumeCapabilityRequired(t *testing.T) {
	ns, _, _ := newTestNodeServer(t)
	_, err := ns.NodePublishVolume(context.Background(), &csi.NodePublishVolumeRequest{
		VolumeId:   "test-vol",
		TargetPath: "/target",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "volumeCapability missing")
}

func TestNodePublishVolume_BlockVolume(t *testing.T) {
	ns, _, _ := newTestNodeServer(t)
	originalWait := nodePublishDeviceWait
	nodePublishDeviceWait = 10 * time.Millisecond
	defer func() { nodePublishDeviceWait = originalWait }()

	_, err := ns.NodePublishVolume(context.Background(), &csi.NodePublishVolumeRequest{
		VolumeId:          "test-vol",
		TargetPath:        "/target",
		StagingTargetPath: "/staging",
		VolumeCapability: &csi.VolumeCapability{
			AccessType: &csi.VolumeCapability_Block{Block: &csi.VolumeCapability_BlockVolume{}},
		},
		PublishContext: map[string]string{
			"targetPortal": "10.0.0.1:3260",
			"iqn":          "iqn.2024-01.com.example:test-volume",
			"lun":          "0",
		},
	})

	// This will fail because the staging connector file doesn't exist,
	// and the by-path device doesn't exist. But we can verify error handling.
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot resolve device for block publish")
}

func TestNodePublishVolume_BlockVolumeWithPersistedConnector(t *testing.T) {
	ns, _, mockMount := newTestNodeServer(t)
	stagingTargetPath := t.TempDir()
	targetPath := filepath.Join(t.TempDir(), "block-target")
	conn := &iscsilib.Connector{
		MountTargetDevice: &iscsilib.Device{Name: "sdb", Type: "disk"},
	}
	originalGetConnector := getConnectorFromFile
	getConnectorFromFile = func(filePath string) (*iscsilib.Connector, error) {
		assert.Equal(t, stageConnectorFile(stagingTargetPath), filePath)
		return conn, nil
	}
	t.Cleanup(func() { getConnectorFromFile = originalGetConnector })

	resp, err := ns.NodePublishVolume(context.Background(), &csi.NodePublishVolumeRequest{
		VolumeId:          "test-vol",
		TargetPath:        targetPath,
		StagingTargetPath: stagingTargetPath,
		Readonly:          true,
		VolumeCapability: &csi.VolumeCapability{
			AccessType: &csi.VolumeCapability_Block{Block: &csi.VolumeCapability_BlockVolume{}},
		},
		PublishContext: map[string]string{
			"targetPortal": "10.0.0.1:3260",
			"iqn":          "iqn.2024-01.com.example:test-volume",
			"lun":          "0",
		},
	})

	require.NoError(t, err)
	assert.NotNil(t, resp)
	require.Len(t, mockMount.formatAndMounts, 1)
	assert.Equal(t, conn.MountTargetDevice.GetPath(), mockMount.formatAndMounts[0].source)
	assert.Equal(t, targetPath, mockMount.formatAndMounts[0].target)
	assert.Contains(t, mockMount.formatAndMounts[0].options, "bind")
	assert.Contains(t, mockMount.formatAndMounts[0].options, "ro")
	assert.FileExists(t, targetPath)
}

func TestNodePublishVolume_ParsePublishErrors(t *testing.T) {
	tests := []struct {
		name           string
		publishContext map[string]string
		wantErr        string
	}{
		{name: "missing publish context", wantErr: "targetPortal is required"},
		{name: "missing target portal", publishContext: map[string]string{"iqn": "iqn.2024-01.com.example:test", "lun": "0"}, wantErr: "targetPortal is required"},
		{name: "missing iqn", publishContext: map[string]string{"targetPortal": "10.0.0.1:3260", "lun": "0"}, wantErr: "iqn is required"},
		{name: "missing lun", publishContext: map[string]string{"targetPortal": "10.0.0.1:3260", "iqn": "iqn.2024-01.com.example:test"}, wantErr: "lun is required"},
		{name: "invalid lun", publishContext: map[string]string{"targetPortal": "10.0.0.1:3260", "iqn": "iqn.2024-01.com.example:test", "lun": "bad"}, wantErr: "invalid lun"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ns, _, _ := newTestNodeServer(t)
			_, err := ns.NodePublishVolume(context.Background(), &csi.NodePublishVolumeRequest{
				VolumeId:          "test-vol",
				TargetPath:        filepath.Join(t.TempDir(), "target"),
				StagingTargetPath: t.TempDir(),
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{}},
				},
				PublishContext: tt.publishContext,
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestNodePublishVolume_StaticISCSIUsesVolumeContextWithoutPublishContext(t *testing.T) {
	ns, _, mockMount := newTestNodeServer(t)
	stagingTargetPath := t.TempDir()
	targetPath := t.TempDir()

	resp, err := ns.NodePublishVolume(context.Background(), &csi.NodePublishVolumeRequest{
		VolumeId:          "static-iscsi-volume",
		TargetPath:        targetPath,
		StagingTargetPath: stagingTargetPath,
		VolumeCapability: &csi.VolumeCapability{
			AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
			AccessType: &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{FsType: "ext4"}},
		},
		VolumeContext: map[string]string{
			"targetPortal": "10.0.0.1:3260",
			"iqn":          "iqn.2024-01.com.example:static-volume",
			"lun":          "3",
		},
	})

	require.NoError(t, err)
	assert.NotNil(t, resp)
	require.Len(t, mockMount.formatAndMounts, 1)
	assert.Equal(t, stagingTargetPath, mockMount.formatAndMounts[0].source)
	assert.Equal(t, targetPath, mockMount.formatAndMounts[0].target)
	assert.Contains(t, mockMount.formatAndMounts[0].options, "bind")
}

func TestNodePublishVolume_FileShareValidation(t *testing.T) {
	t.Run("block mode rejected", func(t *testing.T) {
		ns, _, _ := newTestNodeServer(t)
		_, err := ns.NodePublishVolume(context.Background(), &csi.NodePublishVolumeRequest{
			VolumeId:   "nfs://10.0.0.2/export/test",
			TargetPath: filepath.Join(t.TempDir(), "target"),
			VolumeCapability: &csi.VolumeCapability{
				AccessType: &csi.VolumeCapability_Block{Block: &csi.VolumeCapability_BlockVolume{}},
			},
			PublishContext: map[string]string{"protocol": "nfs"},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "do not support block mode")
	})

	t.Run("staging path required", func(t *testing.T) {
		ns, _, _ := newTestNodeServer(t)
		_, err := ns.NodePublishVolume(context.Background(), &csi.NodePublishVolumeRequest{
			VolumeId:   "smb://10.0.0.3/share",
			TargetPath: filepath.Join(t.TempDir(), "target"),
			VolumeCapability: &csi.VolumeCapability{
				AccessType: &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{}},
			},
			PublishContext: map[string]string{"protocol": "smb"},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "stagingTargetPath required")
	})
}

func TestNodePublishVolume_FileShareWithoutPublishContextUsesVolumeID(t *testing.T) {
	ns, _, mockMount := newTestNodeServer(t)
	stagingTargetPath := t.TempDir()
	targetPath := t.TempDir()

	resp, err := ns.NodePublishVolume(context.Background(), &csi.NodePublishVolumeRequest{
		VolumeId:          "nfs://10.0.0.2/export/test",
		TargetPath:        targetPath,
		StagingTargetPath: stagingTargetPath,
		VolumeCapability: &csi.VolumeCapability{
			AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER},
			AccessType: &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{}},
		},
	})

	require.NoError(t, err)
	assert.NotNil(t, resp)
	require.Len(t, mockMount.formatAndMounts, 1)
	assert.Equal(t, stagingTargetPath, mockMount.formatAndMounts[0].source)
	assert.Equal(t, targetPath, mockMount.formatAndMounts[0].target)
	assert.Contains(t, mockMount.formatAndMounts[0].options, "bind")
}

func TestNodePublishVolume_FilesystemVolume(t *testing.T) {
	ns, _, mockMount := newTestNodeServer(t)
	stagingTargetPath := t.TempDir()
	targetPath := t.TempDir()

	_, err := ns.NodePublishVolume(context.Background(), &csi.NodePublishVolumeRequest{
		VolumeId:          "test-vol",
		TargetPath:        targetPath,
		StagingTargetPath: stagingTargetPath,
		VolumeCapability: &csi.VolumeCapability{
			AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
			AccessType: &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{FsType: "ext4"}},
		},
		PublishContext: map[string]string{
			"targetPortal": "10.0.0.1:3260",
			"iqn":          "iqn.2024-01.com.example:test-volume",
			"lun":          "0",
		},
	})

	require.NoError(t, err)
	require.Len(t, mockMount.formatAndMounts, 1)
	assert.Equal(t, stagingTargetPath, mockMount.formatAndMounts[0].source)
	assert.Equal(t, targetPath, mockMount.formatAndMounts[0].target)
	assert.Equal(t, "", mockMount.formatAndMounts[0].fsType)
	assert.Contains(t, mockMount.formatAndMounts[0].options, "bind")
	assert.NotContains(t, mockMount.formatAndMounts[0].options, "ro")
}

func TestNodePublishVolume_ReadOnly(t *testing.T) {
	ns, _, mockMount := newTestNodeServer(t)
	stagingTargetPath := t.TempDir()
	targetPath := t.TempDir()

	_, err := ns.NodePublishVolume(context.Background(), &csi.NodePublishVolumeRequest{
		VolumeId:          "test-vol",
		TargetPath:        targetPath,
		StagingTargetPath: stagingTargetPath,
		Readonly:          true,
		VolumeCapability: &csi.VolumeCapability{
			AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
			AccessType: &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{FsType: "ext4"}},
		},
		PublishContext: map[string]string{
			"targetPortal": "10.0.0.1:3260",
			"iqn":          "iqn.2024-01.com.example:test-volume",
			"lun":          "0",
		},
	})

	require.NoError(t, err)
	require.Len(t, mockMount.formatAndMounts, 1)
	assert.Equal(t, stagingTargetPath, mockMount.formatAndMounts[0].source)
	assert.Equal(t, targetPath, mockMount.formatAndMounts[0].target)
	assert.Contains(t, mockMount.formatAndMounts[0].options, "bind")
	assert.Contains(t, mockMount.formatAndMounts[0].options, "ro")
}

func TestNodePublishVolume_RecoverStagingUsesVolumeCapabilityFsType(t *testing.T) {
	ns, _, mockMount := newTestNodeServer(t)
	stagingTargetPath := t.TempDir()
	targetPath := t.TempDir()
	conn := &iscsilib.Connector{
		MountTargetDevice: &iscsilib.Device{Name: "sdb", Type: "disk"},
	}

	originalGetConnector := getConnectorFromFile
	getConnectorFromFile = func(filePath string) (*iscsilib.Connector, error) {
		assert.Equal(t, stageConnectorFile(stagingTargetPath), filePath)
		return conn, nil
	}
	t.Cleanup(func() { getConnectorFromFile = originalGetConnector })

	var gotSource, gotTarget, gotFsType string
	originalFormatAndMount := formatAndMount
	formatAndMount = func(m *mount.SafeFormatAndMount, source, target, fsType string, options []string) error {
		gotSource = source
		gotTarget = target
		gotFsType = fsType
		return nil
	}
	t.Cleanup(func() { formatAndMount = originalFormatAndMount })

	resp, err := ns.NodePublishVolume(context.Background(), &csi.NodePublishVolumeRequest{
		VolumeId:          "test-vol",
		TargetPath:        targetPath,
		StagingTargetPath: stagingTargetPath,
		VolumeCapability: &csi.VolumeCapability{
			AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
			AccessType: &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{FsType: "xfs"}},
		},
		PublishContext: map[string]string{
			"targetPortal": "10.0.0.1:3260",
			"iqn":          "iqn.2024-01.com.example:test-volume",
			"lun":          "0",
		},
	})

	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, conn.MountTargetDevice.GetPath(), gotSource)
	assert.Equal(t, stagingTargetPath, gotTarget)
	assert.Equal(t, "xfs", gotFsType)
	require.Len(t, mockMount.formatAndMounts, 1)
	assert.Equal(t, stagingTargetPath, mockMount.formatAndMounts[0].source)
	assert.Equal(t, targetPath, mockMount.formatAndMounts[0].target)
	assert.Contains(t, mockMount.formatAndMounts[0].options, "bind")
}

func TestNodePublishVolume_FileShareBind(t *testing.T) {
	ns, _, mockMount := newTestNodeServer(t)
	stagingTargetPath := t.TempDir()
	targetPath := t.TempDir()

	resp, err := ns.NodePublishVolume(context.Background(), &csi.NodePublishVolumeRequest{
		VolumeId:          "nfs://10.0.0.2/export/test",
		TargetPath:        targetPath,
		StagingTargetPath: stagingTargetPath,
		Readonly:          true,
		VolumeCapability: &csi.VolumeCapability{
			AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
			AccessType: &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{}},
		},
		PublishContext: map[string]string{
			"protocol":      "nfs",
			"nfsServer":     "10.0.0.2",
			"nfsExportPath": "/export/test",
		},
	})

	require.NoError(t, err)
	assert.NotNil(t, resp)
	require.Len(t, mockMount.formatAndMounts, 1)
	assert.Equal(t, stagingTargetPath, mockMount.formatAndMounts[0].source)
	assert.Equal(t, targetPath, mockMount.formatAndMounts[0].target)
	assert.Contains(t, mockMount.formatAndMounts[0].options, "bind")
	assert.Contains(t, mockMount.formatAndMounts[0].options, "ro")
}

// ---------------------------------------------------------------------------
// NodeUnpublishVolume tests
// ---------------------------------------------------------------------------

func TestNodeUnpublishVolume_VolumeIDRequired(t *testing.T) {
	ns, _, _ := newTestNodeServer(t)
	_, err := ns.NodeUnpublishVolume(context.Background(), &csi.NodeUnpublishVolumeRequest{
		VolumeId: "",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "volumeID missing")
}

func TestNodeUnpublishVolume_TargetPathRequired(t *testing.T) {
	ns, _, _ := newTestNodeServer(t)
	_, err := ns.NodeUnpublishVolume(context.Background(), &csi.NodeUnpublishVolumeRequest{
		VolumeId: "test-vol",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "targetPath missing")
}

func TestNodeUnpublishVolume_Success(t *testing.T) {
	ns, _, _ := newTestNodeServer(t)
	targetPath := t.TempDir()

	resp, err := ns.NodeUnpublishVolume(context.Background(), &csi.NodeUnpublishVolumeRequest{
		VolumeId:   "test-vol",
		TargetPath: targetPath,
	})

	require.NoError(t, err)
	assert.NotNil(t, resp)
}

// ---------------------------------------------------------------------------
// NodeGetInfo tests
// ---------------------------------------------------------------------------

func TestNodeGetInfo(t *testing.T) {
	ns, d, _ := newTestNodeServer(t)

	resp, err := ns.NodeGetInfo(context.Background(), &csi.NodeGetInfoRequest{})

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, d.nodeID, resp.NodeId)
}

// ---------------------------------------------------------------------------
// NodeGetCapabilities tests
// ---------------------------------------------------------------------------

func TestNodeGetCapabilities(t *testing.T) {
	ns, _, _ := newTestNodeServer(t)

	resp, err := ns.NodeGetCapabilities(context.Background(), &csi.NodeGetCapabilitiesRequest{})

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, resp.Capabilities)
	assert.Len(t, resp.Capabilities, 2)
}

// ---------------------------------------------------------------------------
// NodeGetVolumeStats tests
// ---------------------------------------------------------------------------

func TestNodeGetVolumeStats_VolumeIDRequired(t *testing.T) {
	ns, _, _ := newTestNodeServer(t)
	_, err := ns.NodeGetVolumeStats(context.Background(), &csi.NodeGetVolumeStatsRequest{
		VolumeId:   "",
		VolumePath: "/mnt/test",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "volumeID missing")
}

func TestNodeGetVolumeStats_VolumePathRequired(t *testing.T) {
	ns, _, _ := newTestNodeServer(t)
	_, err := ns.NodeGetVolumeStats(context.Background(), &csi.NodeGetVolumeStatsRequest{
		VolumeId:   "test-vol",
		VolumePath: "",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "volumePath missing")
}

func TestNodeGetVolumeStats_StatsReturned(t *testing.T) {
	ns, _, _ := newTestNodeServer(t)

	originalFsUsageFunc := fsUsageFunc
	fsUsageFunc = func(path string) (int64, int64, int64, int64, int64, int64, error) {
		assert.Equal(t, "/mnt/test", path)
		return 100, 300, 200, 30, 10, 20, nil
	}
	t.Cleanup(func() {
		fsUsageFunc = originalFsUsageFunc
	})

	resp, err := ns.NodeGetVolumeStats(context.Background(), &csi.NodeGetVolumeStatsRequest{
		VolumeId:   "test-vol",
		VolumePath: "/mnt/test",
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, resp.Usage)
	assert.Len(t, resp.Usage, 2)
	// First usage should be BYTES, second should be INODES
	assert.Equal(t, csi.VolumeUsage_BYTES, resp.Usage[0].Unit)
	assert.Equal(t, int64(300), resp.Usage[0].Total)
	assert.Equal(t, int64(100), resp.Usage[0].Available)
	assert.Equal(t, int64(200), resp.Usage[0].Used)
	assert.Equal(t, csi.VolumeUsage_INODES, resp.Usage[1].Unit)
	assert.Equal(t, int64(30), resp.Usage[1].Total)
	assert.Equal(t, int64(10), resp.Usage[1].Available)
	assert.Equal(t, int64(20), resp.Usage[1].Used)
}

func TestNodeGetVolumeStats_MissingPathReturnsEmptyUsage(t *testing.T) {
	ns, _, _ := newTestNodeServer(t)

	originalFsUsageFunc := fsUsageFunc
	fsUsageFunc = func(path string) (int64, int64, int64, int64, int64, int64, error) {
		return 0, 0, 0, 0, 0, 0, os.ErrNotExist
	}
	t.Cleanup(func() {
		fsUsageFunc = originalFsUsageFunc
	})

	resp, err := ns.NodeGetVolumeStats(context.Background(), &csi.NodeGetVolumeStatsRequest{
		VolumeId:   "test-vol",
		VolumePath: filepath.Join(t.TempDir(), "missing"),
	})

	require.NoError(t, err)
	require.Len(t, resp.Usage, 2)
	assert.Equal(t, csi.VolumeUsage_BYTES, resp.Usage[0].Unit)
	assert.Equal(t, csi.VolumeUsage_INODES, resp.Usage[1].Unit)
	assert.Zero(t, resp.Usage[0].Total)
	assert.Zero(t, resp.Usage[1].Total)
}

func TestNodeGetVolumeStats_FsUsageError(t *testing.T) {
	ns, _, _ := newTestNodeServer(t)

	originalFsUsageFunc := fsUsageFunc
	fsUsageFunc = func(path string) (int64, int64, int64, int64, int64, int64, error) {
		return 0, 0, 0, 0, 0, 0, errors.New("statfs failed")
	}
	t.Cleanup(func() {
		fsUsageFunc = originalFsUsageFunc
	})

	resp, err := ns.NodeGetVolumeStats(context.Background(), &csi.NodeGetVolumeStatsRequest{
		VolumeId:   "test-vol",
		VolumePath: t.TempDir(),
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "statfs failed")
	assert.Nil(t, resp)
}

// ---------------------------------------------------------------------------
// NodeExpandVolume tests
// ---------------------------------------------------------------------------

func TestNodeExpandVolume_VolumeIDRequired(t *testing.T) {
	ns, _, _ := newTestNodeServer(t)
	_, err := ns.NodeExpandVolume(context.Background(), &csi.NodeExpandVolumeRequest{
		VolumeId:   "",
		VolumePath: "/mnt/test",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "volumeID missing")
}

func TestNodeExpandVolume_VolumePathRequired(t *testing.T) {
	ns, _, _ := newTestNodeServer(t)
	_, err := ns.NodeExpandVolume(context.Background(), &csi.NodeExpandVolumeRequest{
		VolumeId:   "test-vol",
		VolumePath: "",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "volumePath missing")
}

func TestNodeExpandVolume_Success(t *testing.T) {
	ns, _, _ := newTestNodeServer(t)

	// Mock ExpandVolume to succeed
	originalExpand := iscsilibExpandVolume
	iscsilibExpandVolume = func(m mount.Interface, resizer iscsilib.Resizer, volumePath string) error {
		assert.Equal(t, "/mnt/test", volumePath)
		return nil
	}
	defer func() { iscsilibExpandVolume = originalExpand }()

	resp, err := ns.NodeExpandVolume(context.Background(), &csi.NodeExpandVolumeRequest{
		VolumeId:   "test-vol",
		VolumePath: "/mnt/test",
	})

	require.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestNodeExpandVolume_FileShareNoOp(t *testing.T) {
	ns, _, _ := newTestNodeServer(t)
	originalExpand := iscsilibExpandVolume
	iscsilibExpandVolume = func(m mount.Interface, resizer iscsilib.Resizer, volumePath string) error {
		t.Fatalf("file-share NodeExpandVolume should not call iSCSI expansion")
		return nil
	}
	defer func() { iscsilibExpandVolume = originalExpand }()

	resp, err := ns.NodeExpandVolume(context.Background(), &csi.NodeExpandVolumeRequest{
		VolumeId:   "nfs://10.0.0.2/export/test?vhdxPath=D%3A%5Cshares%5Ctest",
		VolumePath: "/mnt/test",
	})

	require.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestNodeExpandVolume_Failure(t *testing.T) {
	ns, _, _ := newTestNodeServer(t)

	originalExpand := iscsilibExpandVolume
	iscsilibExpandVolume = func(m mount.Interface, resizer iscsilib.Resizer, volumePath string) error {
		return errors.New("expand failed")
	}
	defer func() { iscsilibExpandVolume = originalExpand }()

	resp, err := ns.NodeExpandVolume(context.Background(), &csi.NodeExpandVolumeRequest{
		VolumeId:   "test-vol",
		VolumePath: "/mnt/test",
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expand failed")
	assert.Nil(t, resp)
}
