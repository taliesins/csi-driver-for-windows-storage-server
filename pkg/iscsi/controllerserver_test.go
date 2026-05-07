package iscsi

import (
	"context"
	"errors"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// mockBackend implements Backend for testing
// ---------------------------------------------------------------------------

type mockBackend struct {
	ensureTargetFn              func(ctx context.Context, targetName, targetIQN string) (string, error)
	configureTargetChapFn       func(ctx context.Context, targetName string, opts TargetChapOptions) error
	createVirtualDiskFn         func(ctx context.Context, name, parentDir string, sizeBytes int64) (string, int64, error)
	mapDiskToTargetFn           func(ctx context.Context, targetName, vhdxPath string) (int32, error)
	unmapDiskFromTargetFn       func(ctx context.Context, targetName, vhdxPath string) error
	deleteVirtualDiskFn         func(ctx context.Context, vhdxPath string) error
	getVolumeByNameFn           func(ctx context.Context, name, parentDir string) (bool, string, int64, string, string, int32, error)
	allowInitiatorFn            func(ctx context.Context, targetName, initiatorIQN string) error
	denyInitiatorFn             func(ctx context.Context, targetName, initiatorIQN string) error
	getDirectoryFreeCapFn       func(ctx context.Context, parentDir string) (int64, error)
	createSnapshotFn            func(ctx context.Context, vhdxPath, desc string) (SnapshotInfo, error)
	deleteSnapshotFn            func(ctx context.Context, snapshotID string) error
	listSnapshotsFn             func(ctx context.Context, vhdxPath string) ([]SnapshotInfo, error)
	exportSnapFn                func(ctx context.Context, snapshotID string) (string, error)
	resizeVirtualDiskFn         func(ctx context.Context, vhdxPath string, newSizeBytes int64) (int64, error)
	getVolumeInfoFn             func(ctx context.Context, vhdxPath string) (VolumeInfo, error)
	getTargetInitiatorsFn       func(ctx context.Context, targetName string) ([]string, error)
	createNfsShareFn            func(ctx context.Context, name, parentDir string, sizeBytes int64, clients []string) (VolumeInfo, error)
	createNfsShareWithOptionsFn func(ctx context.Context, name, parentDir string, sizeBytes int64, clients []string, opts ...NfsShareOptions) (VolumeInfo, error)
	getNfsShareFn               func(ctx context.Context, name, parentDir string) (bool, VolumeInfo, error)
	deleteNfsShareFn            func(ctx context.Context, name, path string) error
	createSmbShareFn            func(ctx context.Context, name, parentDir string, sizeBytes int64, fullAccess, changeAccess, readAccess []string) (VolumeInfo, error)
	createSmbShareWithOptionsFn func(ctx context.Context, name, parentDir string, sizeBytes int64, fullAccess, changeAccess, readAccess []string, opts ...SmbShareOptions) (VolumeInfo, error)
	getSmbShareFn               func(ctx context.Context, name, parentDir string) (bool, VolumeInfo, error)
	deleteSmbShareFn            func(ctx context.Context, name, path string) error
	resizeFileShareFn           func(ctx context.Context, path string, newSizeBytes int64) (int64, error)
	restoreFileShareSnapshotFn  func(ctx context.Context, snapshotID, destinationPath string) error
	mountFileShareVhdxFn        func(ctx context.Context, vhdxPath, mountPath string) error
	unmountFileShareVhdxFn      func(ctx context.Context, vhdxPath, mountPath string) error
}

func (m *mockBackend) EnsureTarget(ctx context.Context, targetName, targetIQN string) (string, error) {
	if m.ensureTargetFn != nil {
		return m.ensureTargetFn(ctx, targetName, targetIQN)
	}
	return firstNonEmpty(targetIQN, targetName), nil
}
func (m *mockBackend) ConfigureTargetChap(ctx context.Context, targetName string, opts TargetChapOptions) error {
	if m.configureTargetChapFn != nil {
		return m.configureTargetChapFn(ctx, targetName, opts)
	}
	return nil
}
func (m *mockBackend) CreateVirtualDisk(ctx context.Context, name, parentDir string, sizeBytes int64) (string, int64, error) {
	if m.createVirtualDiskFn != nil {
		return m.createVirtualDiskFn(ctx, name, parentDir, sizeBytes)
	}
	return "", 0, errors.New("not implemented")
}
func (m *mockBackend) MapDiskToTarget(ctx context.Context, targetName, vhdxPath string) (int32, error) {
	if m.mapDiskToTargetFn != nil {
		return m.mapDiskToTargetFn(ctx, targetName, vhdxPath)
	}
	return 0, errors.New("not implemented")
}
func (m *mockBackend) UnmapDiskFromTarget(ctx context.Context, targetName, vhdxPath string) error {
	if m.unmapDiskFromTargetFn != nil {
		return m.unmapDiskFromTargetFn(ctx, targetName, vhdxPath)
	}
	return nil
}
func (m *mockBackend) DeleteVirtualDisk(ctx context.Context, vhdxPath string) error {
	if m.deleteVirtualDiskFn != nil {
		return m.deleteVirtualDiskFn(ctx, vhdxPath)
	}
	return nil
}
func (m *mockBackend) GetVolumeByName(ctx context.Context, name, parentDir string) (bool, string, int64, string, string, int32, error) {
	if m.getVolumeByNameFn != nil {
		return m.getVolumeByNameFn(ctx, name, parentDir)
	}
	return false, "", 0, "", "", -1, nil
}
func (m *mockBackend) AllowInitiator(ctx context.Context, targetName, initiatorIQN string) error {
	if m.allowInitiatorFn != nil {
		return m.allowInitiatorFn(ctx, targetName, initiatorIQN)
	}
	return nil
}
func (m *mockBackend) DenyInitiator(ctx context.Context, targetName, initiatorIQN string) error {
	if m.denyInitiatorFn != nil {
		return m.denyInitiatorFn(ctx, targetName, initiatorIQN)
	}
	return nil
}
func (m *mockBackend) GetDirectoryFreeCapacity(ctx context.Context, parentDir string) (int64, error) {
	if m.getDirectoryFreeCapFn != nil {
		return m.getDirectoryFreeCapFn(ctx, parentDir)
	}
	return 0, nil
}
func (m *mockBackend) CreateSnapshot(ctx context.Context, vhdxPath, desc string) (SnapshotInfo, error) {
	if m.createSnapshotFn != nil {
		return m.createSnapshotFn(ctx, vhdxPath, desc)
	}
	return SnapshotInfo{}, errors.New("not implemented")
}
func (m *mockBackend) DeleteSnapshot(ctx context.Context, snapshotID string) error {
	if m.deleteSnapshotFn != nil {
		return m.deleteSnapshotFn(ctx, snapshotID)
	}
	return nil
}
func (m *mockBackend) ListSnapshots(ctx context.Context, vhdxPath string) ([]SnapshotInfo, error) {
	if m.listSnapshotsFn != nil {
		return m.listSnapshotsFn(ctx, vhdxPath)
	}
	return nil, nil
}
func (m *mockBackend) ExportSnapshotAsVirtualDisk(ctx context.Context, snapshotID string) (string, error) {
	if m.exportSnapFn != nil {
		return m.exportSnapFn(ctx, snapshotID)
	}
	return "", errors.New("not implemented")
}
func (m *mockBackend) ResizeVirtualDisk(ctx context.Context, vhdxPath string, newSizeBytes int64) (int64, error) {
	if m.resizeVirtualDiskFn != nil {
		return m.resizeVirtualDiskFn(ctx, vhdxPath, newSizeBytes)
	}
	return 0, errors.New("not implemented")
}
func (m *mockBackend) GetVolumeInfo(ctx context.Context, vhdxPath string) (VolumeInfo, error) {
	if m.getVolumeInfoFn != nil {
		return m.getVolumeInfoFn(ctx, vhdxPath)
	}
	return VolumeInfo{}, nil
}
func (m *mockBackend) GetTargetInitiators(ctx context.Context, targetName string) ([]string, error) {
	if m.getTargetInitiatorsFn != nil {
		return m.getTargetInitiatorsFn(ctx, targetName)
	}
	return nil, nil
}
func (m *mockBackend) CreateNfsShare(ctx context.Context, name, parentDir string, sizeBytes int64, clients []string, opts ...NfsShareOptions) (VolumeInfo, error) {
	if m.createNfsShareWithOptionsFn != nil {
		return m.createNfsShareWithOptionsFn(ctx, name, parentDir, sizeBytes, clients, opts...)
	}
	if m.createNfsShareFn != nil {
		return m.createNfsShareFn(ctx, name, parentDir, sizeBytes, clients)
	}
	return VolumeInfo{}, errors.New("not implemented")
}
func (m *mockBackend) GetNfsShare(ctx context.Context, name, parentDir string) (bool, VolumeInfo, error) {
	if m.getNfsShareFn != nil {
		return m.getNfsShareFn(ctx, name, parentDir)
	}
	return false, VolumeInfo{}, nil
}
func (m *mockBackend) DeleteNfsShare(ctx context.Context, name, path string) error {
	if m.deleteNfsShareFn != nil {
		return m.deleteNfsShareFn(ctx, name, path)
	}
	return nil
}
func (m *mockBackend) CreateSmbShare(ctx context.Context, name, parentDir string, sizeBytes int64, fullAccess, changeAccess, readAccess []string, opts ...SmbShareOptions) (VolumeInfo, error) {
	if m.createSmbShareWithOptionsFn != nil {
		return m.createSmbShareWithOptionsFn(ctx, name, parentDir, sizeBytes, fullAccess, changeAccess, readAccess, opts...)
	}
	if m.createSmbShareFn != nil {
		return m.createSmbShareFn(ctx, name, parentDir, sizeBytes, fullAccess, changeAccess, readAccess)
	}
	return VolumeInfo{}, errors.New("not implemented")
}
func (m *mockBackend) GetSmbShare(ctx context.Context, name, parentDir string) (bool, VolumeInfo, error) {
	if m.getSmbShareFn != nil {
		return m.getSmbShareFn(ctx, name, parentDir)
	}
	return false, VolumeInfo{}, nil
}
func (m *mockBackend) DeleteSmbShare(ctx context.Context, name, path string) error {
	if m.deleteSmbShareFn != nil {
		return m.deleteSmbShareFn(ctx, name, path)
	}
	return nil
}
func (m *mockBackend) ResizeFileShare(ctx context.Context, path string, newSizeBytes int64) (int64, error) {
	if m.resizeFileShareFn != nil {
		return m.resizeFileShareFn(ctx, path, newSizeBytes)
	}
	return newSizeBytes, nil
}
func (m *mockBackend) RestoreSnapshotAsFileShare(ctx context.Context, snapshotID, destinationPath string) error {
	if m.restoreFileShareSnapshotFn != nil {
		return m.restoreFileShareSnapshotFn(ctx, snapshotID, destinationPath)
	}
	return nil
}
func (m *mockBackend) MountFileShareVirtualDisk(ctx context.Context, vhdxPath, mountPath string) error {
	if m.mountFileShareVhdxFn != nil {
		return m.mountFileShareVhdxFn(ctx, vhdxPath, mountPath)
	}
	return nil
}
func (m *mockBackend) UnmountFileShareVirtualDisk(ctx context.Context, vhdxPath, mountPath string) error {
	if m.unmountFileShareVhdxFn != nil {
		return m.unmountFileShareVhdxFn(ctx, vhdxPath, mountPath)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Test helper - create a ControllerServer with mock backend
// ---------------------------------------------------------------------------

func newTestControllerServer(t *testing.T) (*ControllerServer, *driver, *mockBackend) {
	t.Helper()
	return newTestControllerServerForProtocol(t, ProtocolISCSI)
}

func newTestControllerServerForProtocol(t *testing.T, protocol Protocol) (*ControllerServer, *driver, *mockBackend) {
	t.Helper()
	backend := ""
	if protocol == ProtocolNFS || protocol == ProtocolSMB {
		backend = fileShareBackendDirectory
	}
	return newTestControllerServerForProtocolAndBackend(t, protocol, backend)
}

func newTestControllerServerForProtocolAndBackend(t *testing.T, protocol Protocol, fileShareBackend string) (*ControllerServer, *driver, *mockBackend) {
	t.Helper()
	mockBackend := &mockBackend{}
	name, err := driverNameForProtocol(protocol)
	require.NoError(t, err)
	d := newNamedProtocolDriverWithShareBackend(name, protocol, fileShareBackend, "node-001", "unix:///var/run/csi/csi.sock")
	d.backend = mockBackend
	cs := NewControllerServer(d)
	return cs, d, mockBackend
}

func newTestConsolidatedControllerServer(t *testing.T) (*ControllerServer, *driver, *mockBackend) {
	t.Helper()
	mockBackend := &mockBackend{}
	d := newNamedProtocolDriverWithShareBackend(driverName, "", "", "node-001", "unix:///var/run/csi/csi.sock")
	d.backend = mockBackend
	cs := NewControllerServer(d)
	return cs, d, mockBackend
}

func newTestVolumeID(t *testing.T) string {
	t.Helper()
	vid := volID{
		VolumeName:   "test-volume",
		TargetPortal: "10.0.0.1:3260",
		TargetIQN:    "iqn.2024-01.com.example:test-volume",
		LUN:          0,
		VHDXPath:     "D:\\vhdx\\test-volume.vhdx",
		SizeBytes:    1073741824,
	}
	return encodeVolID(vid)
}

func newTestVolumeIDWithTargetName(t *testing.T) string {
	t.Helper()
	vid := volID{
		VolumeName:   "test-volume",
		TargetPortal: "10.0.0.1:3260",
		TargetName:   "test-volume",
		TargetIQN:    "iqn.1991-05.com.microsoft:win-storage-test-volume",
		LUN:          0,
		VHDXPath:     "D:\\vhdx\\test-volume.vhdx",
		SizeBytes:    1073741824,
	}
	return encodeVolID(vid)
}

func newTestSnapshotID(t *testing.T) string {
	t.Helper()
	sid := snapID{
		SnapshotID:   "snap-001",
		OriginalPath: "D:\\vhdx\\test-volume.vhdx",
	}
	return encodeSnapID(sid)
}

func TestRequiredBytesFromRange(t *testing.T) {
	const oneGiB = int64(1 << 30)
	tests := []struct {
		name    string
		r       *csi.CapacityRange
		want    int64
		wantErr string
	}{
		{name: "nil range uses minimum", want: oneGiB},
		{name: "empty range uses minimum", r: &csi.CapacityRange{}, want: oneGiB},
		{name: "required below minimum", r: &csi.CapacityRange{RequiredBytes: 1}, want: oneGiB},
		{name: "required above minimum", r: &csi.CapacityRange{RequiredBytes: oneGiB * 2}, want: oneGiB * 2},
		{name: "limit below minimum", r: &csi.CapacityRange{LimitBytes: 1}, want: oneGiB},
		{name: "required and limit valid", r: &csi.CapacityRange{RequiredBytes: oneGiB * 2, LimitBytes: oneGiB * 3}, want: oneGiB * 2},
		{name: "required greater than limit", r: &csi.CapacityRange{RequiredBytes: oneGiB * 3, LimitBytes: oneGiB * 2}, wantErr: "requiredBytes > limitBytes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := requiredBytesFromRange(tt.r, 1)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestTargetPortalAddress(t *testing.T) {
	tests := []struct {
		name   string
		host   string
		port   int
		expect string
	}{
		{name: "host without port", host: "win-storage.lab.local", port: 3260, expect: "win-storage.lab.local:3260"},
		{name: "host with port", host: "win-storage.lab.local:3260", port: 3260, expect: "win-storage.lab.local:3260"},
		{name: "custom port", host: "win-storage.lab.local", port: 3200, expect: "win-storage.lab.local:3200"},
		{name: "ipv6", host: "2001:db8::1", port: 3260, expect: "[2001:db8::1]:3260"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expect, targetPortalAddress(tt.host, tt.port))
		})
	}
}

func TestFileShareParamValidation(t *testing.T) {
	t.Run("invalid backend", func(t *testing.T) {
		cs, _, _ := newTestControllerServerForProtocol(t, ProtocolNFS)
		_, err := cs.fileShareBackendFromParams(map[string]string{"shareBackend": "bad"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "shareBackend")
	})

	t.Run("driver rejects different backend", func(t *testing.T) {
		cs, _, _ := newTestControllerServerForProtocolAndBackend(t, ProtocolNFS, fileShareBackendVHDX)
		_, err := cs.fileShareBackendFromParams(map[string]string{"shareBackend": fileShareBackendDirectory})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "only supports")
	})

	t.Run("invalid nfs bool", func(t *testing.T) {
		_, err := nfsOptionsFromParams(map[string]string{"nfsAllowRootAccess": "not-bool"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be a boolean")
	})

	t.Run("invalid nfs integer", func(t *testing.T) {
		_, err := nfsOptionsFromParams(map[string]string{"nfsAnonymousUid": "bad"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be an integer")
	})

	t.Run("invalid nfs permission", func(t *testing.T) {
		_, err := nfsOptionsFromParams(map[string]string{"nfsPermission": "admin"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "nfsPermission")
	})

	t.Run("invalid nfs authentication", func(t *testing.T) {
		_, err := nfsOptionsFromParams(map[string]string{"nfsAuthentication": "password"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "nfsAuthentication")
	})

	t.Run("valid nfs aliases", func(t *testing.T) {
		opts, err := nfsOptionsFromParams(map[string]string{
			"nfsPermission":            "read-write",
			"nfsEnableAnonymousAccess": "yes",
			"nfsEnableUnmappedAccess":  "no",
			"nfsAnonymousUid":          "65534",
			"nfsAnonymousGid":          "65533",
			"nfsAuthentication":        "sys, krb5",
			"nfsLanguageEncoding":      "EUC-JP",
			"nfsClientType":            "clientgroup",
		})
		require.NoError(t, err)
		assert.Equal(t, "readwrite", opts.Permission)
		assert.Equal(t, "clientgroup", opts.ClientType)
		require.NotNil(t, opts.EnableAnonymousAccess)
		assert.True(t, *opts.EnableAnonymousAccess)
		require.NotNil(t, opts.EnableUnmappedAccess)
		assert.False(t, *opts.EnableUnmappedAccess)
		require.NotNil(t, opts.AnonymousUID)
		assert.Equal(t, 65534, *opts.AnonymousUID)
		require.NotNil(t, opts.AnonymousGID)
		assert.Equal(t, 65533, *opts.AnonymousGID)
		assert.Equal(t, []string{"sys", "krb5"}, opts.Authentication)
		assert.Equal(t, "krb5", opts.MountAuthentication)
		assert.Equal(t, "EUC-JP", opts.LanguageEncoding)
	})

	t.Run("valid nfs kerberos shortcut", func(t *testing.T) {
		opts, err := nfsOptionsFromParams(map[string]string{"nfsKerberos": "krb5p"})
		require.NoError(t, err)
		assert.Equal(t, []string{"krb5p"}, opts.Authentication)
		assert.Equal(t, "krb5p", opts.MountAuthentication)
	})

	t.Run("invalid smb bool", func(t *testing.T) {
		_, err := smbOptionsFromParams(map[string]string{"smbEncryptData": "not-bool"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be a boolean")
	})

	t.Run("invalid smb uint", func(t *testing.T) {
		_, err := smbOptionsFromParams(map[string]string{"smbConcurrentUserLimit": "-1"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "non-negative integer")
	})
}

// ---------------------------------------------------------------------------
// CreateVolume tests
// ---------------------------------------------------------------------------

func TestCreateVolume_VolumeNameRequired(t *testing.T) {
	cs, _, _ := newTestControllerServer(t)
	_, err := cs.CreateVolume(context.Background(), &csi.CreateVolumeRequest{
		Name: "",
		VolumeCapabilities: []*csi.VolumeCapability{
			{AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER}},
		},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "volume name is required")
}

func TestCreateVolume_CapabilitiesRequired(t *testing.T) {
	cs, _, _ := newTestControllerServer(t)
	_, err := cs.CreateVolume(context.Background(), &csi.CreateVolumeRequest{
		Name: "test-volume",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "volume capabilities are required")
}

func TestCreateVolume_TargetPortalRequired(t *testing.T) {
	cs, _, _ := newTestControllerServer(t)
	_, err := cs.CreateVolume(context.Background(), &csi.CreateVolumeRequest{
		Name: "test-volume",
		VolumeCapabilities: []*csi.VolumeCapability{
			{AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER}},
		},
		Parameters: map[string]string{
			"vhdxParentPath": "D:\\vhdx",
			"iqnPrefix":      "iqn.2024-01.com.example",
		},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "targetPortal is required")
}

func TestTargetChapOptionsFromSecrets(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		opts, err := targetChapOptionsFromSecrets(nil)
		require.NoError(t, err)
		assert.False(t, opts.Enabled())
	})

	t.Run("session chap", func(t *testing.T) {
		opts, err := targetChapOptionsFromSecrets(map[string]string{
			"node.session.auth.username": " dbnode01 ",
			"node.session.auth.password": " S3cret! ",
		})
		require.NoError(t, err)
		assert.Equal(t, TargetChapOptions{ChapUser: "dbnode01", ChapSecret: "S3cret!"}, opts)
	})

	t.Run("mutual chap", func(t *testing.T) {
		opts, err := targetChapOptionsFromSecrets(map[string]string{
			"node.session.auth.username":    "dbnode01",
			"node.session.auth.password":    "S3cret!",
			"node.session.auth.username_in": "targetid",
			"node.session.auth.password_in": "TargetS3cret!",
		})
		require.NoError(t, err)
		assert.Equal(t, TargetChapOptions{
			ChapUser:          "dbnode01",
			ChapSecret:        "S3cret!",
			ReverseChapUser:   "targetid",
			ReverseChapSecret: "TargetS3cret!",
		}, opts)
	})

	t.Run("discovery chap is linux only", func(t *testing.T) {
		opts, err := targetChapOptionsFromSecrets(map[string]string{
			"discovery.sendtargets.auth.username": "discuser",
			"discovery.sendtargets.auth.password": "DiscS3cret!",
		})
		require.NoError(t, err)
		assert.False(t, opts.Enabled())
	})

	t.Run("missing password", func(t *testing.T) {
		_, err := targetChapOptionsFromSecrets(map[string]string{
			"node.session.auth.username": "dbnode01",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "node.session.auth.password")
	})

	t.Run("reverse requires session chap", func(t *testing.T) {
		_, err := targetChapOptionsFromSecrets(map[string]string{
			"node.session.auth.username_in": "targetid",
			"node.session.auth.password_in": "TargetS3cret!",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "reverse CHAP requires")
	})
}

func TestCreateVolume_VhdxParentPathOptionalUsesBackendDefault(t *testing.T) {
	cs, _, mockBackend := newTestControllerServer(t)

	mockBackend.getVolumeByNameFn = func(ctx context.Context, name, parentDir string) (bool, string, int64, string, string, int32, error) {
		assert.Empty(t, parentDir)
		return false, "", 0, "", "", -1, nil
	}
	mockBackend.createVirtualDiskFn = func(ctx context.Context, name, parentDir string, sizeBytes int64) (string, int64, error) {
		assert.Empty(t, parentDir)
		return "E:\\iSCSIVirtualDisks\\" + name + ".vhdx", sizeBytes, nil
	}
	mockBackend.ensureTargetFn = func(ctx context.Context, targetName, targetIQN string) (string, error) {
		return firstNonEmpty(targetIQN, targetName), nil
	}
	mockBackend.mapDiskToTargetFn = func(ctx context.Context, targetName, vhdxPath string) (int32, error) {
		assert.Equal(t, "E:\\iSCSIVirtualDisks\\test-volume.vhdx", vhdxPath)
		return 0, nil
	}

	resp, err := cs.CreateVolume(context.Background(), &csi.CreateVolumeRequest{
		Name: "test-volume",
		VolumeCapabilities: []*csi.VolumeCapability{
			{AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER}},
		},
		Parameters: map[string]string{
			"targetPortal": "10.0.0.1:3260",
			"iqnPrefix":    "iqn.2024-01.com.example",
			"iface":        "storage0",
		},
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "iqn.2024-01.com.example:test-volume", resp.Volume.VolumeContext["iqn"])
	assert.Equal(t, "storage0", resp.Volume.VolumeContext["iface"])
}

func TestCreateVolume_IqnPrefixOptionalUsesWindowsGeneratedIQN(t *testing.T) {
	cs, _, mockBackend := newTestControllerServer(t)

	mockBackend.getVolumeByNameFn = func(ctx context.Context, name, parentDir string) (bool, string, int64, string, string, int32, error) {
		return false, "", 0, "", "", -1, nil
	}
	mockBackend.createVirtualDiskFn = func(ctx context.Context, name, parentDir string, sizeBytes int64) (string, int64, error) {
		return "D:\\vhdx\\" + name + ".vhdx", sizeBytes, nil
	}
	mockBackend.ensureTargetFn = func(ctx context.Context, targetName, targetIQN string) (string, error) {
		assert.Equal(t, "test-volume", targetName)
		assert.Empty(t, targetIQN)
		return "iqn.1991-05.com.microsoft:win-storage-test-volume", nil
	}
	mockBackend.mapDiskToTargetFn = func(ctx context.Context, targetName, vhdxPath string) (int32, error) {
		assert.Equal(t, "test-volume", targetName)
		return 0, nil
	}

	resp, err := cs.CreateVolume(context.Background(), &csi.CreateVolumeRequest{
		Name: "test-volume",
		VolumeCapabilities: []*csi.VolumeCapability{
			{AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER}},
		},
		Parameters: map[string]string{
			"targetPortal":   "10.0.0.1:3260",
			"vhdxParentPath": "D:\\vhdx",
		},
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "iqn.1991-05.com.microsoft:win-storage-test-volume", resp.Volume.VolumeContext["iqn"])
	decoded, err := decodeVolID(resp.Volume.VolumeId)
	require.NoError(t, err)
	assert.Equal(t, "test-volume", decoded.TargetName)
	assert.Equal(t, "iqn.1991-05.com.microsoft:win-storage-test-volume", decoded.TargetIQN)
}

func TestCreateVolume_ConfiguresWindowsTargetChap(t *testing.T) {
	cs, _, mockBackend := newTestControllerServer(t)
	var gotTargetName string
	var gotChap TargetChapOptions
	var configureTargetChapCalls int

	mockBackend.createVirtualDiskFn = func(ctx context.Context, name, parentDir string, sizeBytes int64) (string, int64, error) {
		return "D:\\vhdx\\" + name + ".vhdx", sizeBytes, nil
	}
	mockBackend.ensureTargetFn = func(ctx context.Context, targetName, targetIQN string) (string, error) {
		return "iqn.1991-05.com.microsoft:win-storage-test-volume", nil
	}
	mockBackend.configureTargetChapFn = func(ctx context.Context, targetName string, opts TargetChapOptions) error {
		configureTargetChapCalls++
		gotTargetName = targetName
		gotChap = opts
		return nil
	}
	mockBackend.mapDiskToTargetFn = func(ctx context.Context, targetName, vhdxPath string) (int32, error) {
		return 0, nil
	}

	resp, err := cs.CreateVolume(context.Background(), &csi.CreateVolumeRequest{
		Name: "test-volume",
		VolumeCapabilities: []*csi.VolumeCapability{
			{AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER}},
		},
		Parameters: map[string]string{
			"targetPortal": "10.0.0.1",
		},
		Secrets: map[string]string{
			"node.session.auth.username":    "dbnode01",
			"node.session.auth.password":    "S3cret!",
			"node.session.auth.username_in": "targetid",
			"node.session.auth.password_in": "TargetS3cret!",
		},
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, 1, configureTargetChapCalls)
	assert.Equal(t, "test-volume", gotTargetName)
	assert.Equal(t, TargetChapOptions{
		ChapUser:          "dbnode01",
		ChapSecret:        "S3cret!",
		ReverseChapUser:   "targetid",
		ReverseChapSecret: "TargetS3cret!",
	}, gotChap)
}

func TestCreateVolume_InvalidAccessMode(t *testing.T) {
	cs, _, _ := newTestControllerServer(t)
	_, err := cs.CreateVolume(context.Background(), &csi.CreateVolumeRequest{
		Name: "test-volume",
		VolumeCapabilities: []*csi.VolumeCapability{
			{AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_SINGLE_WRITER}},
		},
		Parameters: map[string]string{
			"targetPortal":   "10.0.0.1:3260",
			"vhdxParentPath": "D:\\vhdx",
			"iqnPrefix":      "iqn.2024-01.com.example",
		},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "access mode is not supported")
}

func TestCreateVolume_ConsolidatedDriverRejectsMultiNodeISCSI(t *testing.T) {
	cs, _, _ := newTestConsolidatedControllerServer(t)

	_, err := cs.CreateVolume(context.Background(), &csi.CreateVolumeRequest{
		Name: "test-volume",
		VolumeCapabilities: []*csi.VolumeCapability{
			{AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER}},
		},
		Parameters: map[string]string{
			"protocol":     "iscsi",
			"targetPortal": "10.0.0.1:3260",
		},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "access mode is not supported")
}

func TestCreateVolume_ConsolidatedDriverAcceptsMultiNodeNFS(t *testing.T) {
	cs, _, mockBackend := newTestConsolidatedControllerServer(t)
	mockBackend.getNfsShareFn = func(ctx context.Context, name, parentDir string) (bool, VolumeInfo, error) {
		return true, VolumeInfo{
			VolumeName:    name,
			Protocol:      ProtocolNFS,
			NfsServer:     "win-storage.lab.local",
			NfsExportPath: "/test-volume",
			VHDXPath:      "D:\\shares\\test-volume",
			CapacityBytes: 1073741824,
		}, nil
	}

	resp, err := cs.CreateVolume(context.Background(), &csi.CreateVolumeRequest{
		Name: "test-volume",
		VolumeCapabilities: []*csi.VolumeCapability{
			{AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER}},
		},
		Parameters: map[string]string{
			"protocol":        "nfs",
			"shareBackend":    fileShareBackendDirectory,
			"shareParentPath": "D:\\shares",
		},
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "nfs", resp.Volume.VolumeContext["protocol"])
}

func TestCreateVolume_ProtocolMismatch(t *testing.T) {
	cs, _, _ := newTestControllerServerForProtocol(t, ProtocolNFS)

	_, err := cs.CreateVolume(context.Background(), &csi.CreateVolumeRequest{
		Name: "wrong-protocol",
		VolumeCapabilities: []*csi.VolumeCapability{
			{AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER}},
		},
		Parameters: map[string]string{
			"protocol":        "smb",
			"shareParentPath": "D:\\shares",
		},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not match CSI driver")
}

func TestCreateVolume_NewVolume_Success(t *testing.T) {
	cs, _, mockBackend := newTestControllerServer(t)

	mockBackend.getVolumeByNameFn = func(ctx context.Context, name, parentDir string) (bool, string, int64, string, string, int32, error) {
		return false, "", 0, "", "", -1, nil
	}
	mockBackend.createVirtualDiskFn = func(ctx context.Context, name, parentDir string, sizeBytes int64) (string, int64, error) {
		return "D:\\vhdx\\" + name + ".vhdx", sizeBytes, nil
	}
	mockBackend.ensureTargetFn = func(ctx context.Context, targetName, targetIQN string) (string, error) {
		assert.Equal(t, "iqn.2024-01.com.example:test-volume", targetName)
		assert.Equal(t, "iqn.2024-01.com.example:test-volume", targetIQN)
		return targetIQN, nil
	}
	mockBackend.mapDiskToTargetFn = func(ctx context.Context, targetName, vhdxPath string) (int32, error) {
		assert.Equal(t, "iqn.2024-01.com.example:test-volume", targetName)
		assert.Equal(t, "D:\\vhdx\\test-volume.vhdx", vhdxPath)
		return 0, nil
	}

	resp, err := cs.CreateVolume(context.Background(), &csi.CreateVolumeRequest{
		Name: "test-volume",
		VolumeCapabilities: []*csi.VolumeCapability{
			{AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER}},
		},
		CapacityRange: &csi.CapacityRange{
			RequiredBytes: 1073741824,
		},
		Parameters: map[string]string{
			"targetPortal":   "10.0.0.1:3260",
			"vhdxParentPath": "D:\\vhdx",
			"iqnPrefix":      "iqn.2024-01.com.example",
		},
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, resp.Volume)
	assert.NotEmpty(t, resp.Volume.VolumeId)
	assert.Equal(t, int64(1073741824), resp.Volume.CapacityBytes)
	assert.Equal(t, "iqn.2024-01.com.example:test-volume", resp.Volume.VolumeContext["iqn"])
	assert.Equal(t, "0", resp.Volume.VolumeContext["lun"])
	assert.Equal(t, "10.0.0.1:3260", resp.Volume.VolumeContext["targetPortal"])
}

func TestCreateVolume_Idempotent_ExistingVolume(t *testing.T) {
	cs, _, mockBackend := newTestControllerServer(t)

	mockBackend.getVolumeByNameFn = func(ctx context.Context, name, parentDir string) (bool, string, int64, string, string, int32, error) {
		return true, "D:\\vhdx\\test-volume.vhdx", 1073741824, "iqn.2024-01.com.example:test-volume", "iqn.2024-01.com.example:test-volume", 0, nil
	}
	mockBackend.ensureTargetFn = func(ctx context.Context, targetName, targetIQN string) (string, error) {
		return firstNonEmpty(targetIQN, targetName), nil
	}
	mockBackend.configureTargetChapFn = func(ctx context.Context, targetName string, opts TargetChapOptions) error {
		t.Fatalf("ConfigureTargetChap should not run for an idempotent existing volume")
		return nil
	}

	resp, err := cs.CreateVolume(context.Background(), &csi.CreateVolumeRequest{
		Name: "test-volume",
		VolumeCapabilities: []*csi.VolumeCapability{
			{AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER}},
		},
		Parameters: map[string]string{
			"targetPortal":   "10.0.0.1:3260",
			"vhdxParentPath": "D:\\vhdx",
			"iqnPrefix":      "iqn.2024-01.com.example",
		},
		Secrets: map[string]string{
			"node.session.auth.username": "dbnode01",
			"node.session.auth.password": "S3cret!",
		},
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.NotEmpty(t, resp.Volume.VolumeId)
	assert.Equal(t, int64(1073741824), resp.Volume.CapacityBytes)
}

func TestCreateVolume_SizeConflict(t *testing.T) {
	cs, _, mockBackend := newTestControllerServer(t)

	mockBackend.getVolumeByNameFn = func(ctx context.Context, name, parentDir string) (bool, string, int64, string, string, int32, error) {
		return true, "D:\\vhdx\\test-volume.vhdx", 536870912, "", "", -1, nil
	}

	resp, err := cs.CreateVolume(context.Background(), &csi.CreateVolumeRequest{
		Name: "test-volume",
		VolumeCapabilities: []*csi.VolumeCapability{
			{AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER}},
		},
		CapacityRange: &csi.CapacityRange{
			RequiredBytes: 1073741824,
		},
		Parameters: map[string]string{
			"targetPortal":   "10.0.0.1:3260",
			"vhdxParentPath": "D:\\vhdx",
			"iqnPrefix":      "iqn.2024-01.com.example",
		},
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "AlreadyExists")
	assert.Contains(t, err.Error(), "exists smaller")
	assert.Nil(t, resp)
}

func TestCreateVolume_FromSnapshot(t *testing.T) {
	cs, _, mockBackend := newTestControllerServer(t)

	snapID := "snap-001"
	mockBackend.exportSnapFn = func(ctx context.Context, snapshotID string) (string, error) {
		assert.Equal(t, snapID, snapshotID)
		return "D:\\vhdx\\exported-snap.vhdx", nil
	}
	mockBackend.ensureTargetFn = func(ctx context.Context, targetName, targetIQN string) (string, error) {
		return firstNonEmpty(targetIQN, targetName), nil
	}
	mockBackend.mapDiskToTargetFn = func(ctx context.Context, targetName, vhdxPath string) (int32, error) {
		return 0, nil
	}
	mockBackend.getVolumeInfoFn = func(ctx context.Context, vhdxPath string) (VolumeInfo, error) {
		return VolumeInfo{VHDXPath: vhdxPath, SizeBytes: 1073741824}, nil
	}

	resp, err := cs.CreateVolume(context.Background(), &csi.CreateVolumeRequest{
		Name: "test-volume-from-snap",
		VolumeCapabilities: []*csi.VolumeCapability{
			{AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER}},
		},
		VolumeContentSource: &csi.VolumeContentSource{
			Type: &csi.VolumeContentSource_Snapshot{
				Snapshot: &csi.VolumeContentSource_SnapshotSource{
					SnapshotId: newTestSnapshotID(t),
				},
			},
		},
		Parameters: map[string]string{
			"targetPortal":   "10.0.0.1:3260",
			"vhdxParentPath": "D:\\vhdx",
			"iqnPrefix":      "iqn.2024-01.com.example",
		},
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, resp.Volume)
	assert.Equal(t, "snapshot", resp.Volume.VolumeContext["source"])
	require.NotNil(t, resp.Volume.ContentSource)
	require.NotNil(t, resp.Volume.ContentSource.GetSnapshot())
}

func TestCreateVolume_NFS(t *testing.T) {
	cs, _, mockBackend := newTestControllerServerForProtocol(t, ProtocolNFS)

	mockBackend.getNfsShareFn = func(ctx context.Context, name, parentDir string) (bool, VolumeInfo, error) {
		assert.Equal(t, "nfs-vol", name)
		assert.Equal(t, "D:\\shares", parentDir)
		return false, VolumeInfo{}, nil
	}
	mockBackend.createNfsShareWithOptionsFn = func(ctx context.Context, name, parentDir string, sizeBytes int64, clients []string, opts ...NfsShareOptions) (VolumeInfo, error) {
		assert.Equal(t, []string{"10.0.0.10"}, clients)
		require.Len(t, opts, 1)
		assert.Equal(t, []string{"krb5p"}, opts[0].Authentication)
		assert.Equal(t, "krb5p", opts[0].MountAuthentication)
		return VolumeInfo{
			VolumeName:    name,
			Protocol:      ProtocolNFS,
			NfsServer:     "nfs-server",
			NfsExportPath: "/" + name,
			VHDXPath:      parentDir + "\\" + name,
			CapacityBytes: sizeBytes,
		}, nil
	}

	resp, err := cs.CreateVolume(context.Background(), &csi.CreateVolumeRequest{
		Name: "nfs-vol",
		VolumeCapabilities: []*csi.VolumeCapability{
			{AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER}},
		},
		Parameters: map[string]string{
			"protocol":        "nfs",
			"shareParentPath": "D:\\shares",
			"nfsServer":       "10.0.0.2",
			"nfsClientName":   "10.0.0.10",
			"nfsKerberos":     "krb5p",
		},
	})

	require.NoError(t, err)
	require.NotNil(t, resp.Volume)
	assert.Equal(t, "nfs", resp.Volume.VolumeContext["protocol"])
	assert.Equal(t, "10.0.0.2", resp.Volume.VolumeContext["nfsServer"])
	assert.Equal(t, "/nfs-vol", resp.Volume.VolumeContext["nfsExportPath"])
	assert.Equal(t, "krb5p", resp.Volume.VolumeContext["nfsAuthentication"])
	assert.Equal(t, "krb5p", resp.Volume.VolumeContext["nfsMountAuthentication"])
	decoded, err := DecodeVolumeID(resp.Volume.VolumeId)
	require.NoError(t, err)
	assert.Equal(t, ProtocolNFS, decoded.Protocol)
	assert.Equal(t, "krb5p", decoded.NfsAuthentication)
	assert.Equal(t, "krb5p", decoded.NfsMountAuthentication)
}

func TestCreateVolume_SMB(t *testing.T) {
	cs, _, mockBackend := newTestControllerServerForProtocol(t, ProtocolSMB)

	mockBackend.getSmbShareFn = func(ctx context.Context, name, parentDir string) (bool, VolumeInfo, error) {
		return false, VolumeInfo{}, nil
	}
	mockBackend.createSmbShareFn = func(ctx context.Context, name, parentDir string, sizeBytes int64, fullAccess, changeAccess, readAccess []string) (VolumeInfo, error) {
		assert.Equal(t, []string{"DOMAIN\\storage"}, fullAccess)
		return VolumeInfo{
			VolumeName:    name,
			Protocol:      ProtocolSMB,
			SmbServer:     "smb-server",
			SmbShareName:  name,
			VHDXPath:      parentDir + "\\" + name,
			CapacityBytes: sizeBytes,
		}, nil
	}

	resp, err := cs.CreateVolume(context.Background(), &csi.CreateVolumeRequest{
		Name: "smb-vol",
		VolumeCapabilities: []*csi.VolumeCapability{
			{AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER}},
		},
		Parameters: map[string]string{
			"protocol":        "smb",
			"shareParentPath": "D:\\shares",
			"smbServer":       "10.0.0.3",
			"smbFullAccess":   "DOMAIN\\storage",
		},
	})

	require.NoError(t, err)
	require.NotNil(t, resp.Volume)
	assert.Equal(t, "smb", resp.Volume.VolumeContext["protocol"])
	assert.Equal(t, "10.0.0.3", resp.Volume.VolumeContext["smbServer"])
	assert.Equal(t, "smb-vol", resp.Volume.VolumeContext["smbShareName"])
	decoded, err := DecodeVolumeID(resp.Volume.VolumeId)
	require.NoError(t, err)
	assert.Equal(t, ProtocolSMB, decoded.Protocol)
}

func TestCreateVolume_NFSOptionsAndSnapshotRestore(t *testing.T) {
	cs, _, mockBackend := newTestControllerServerForProtocolAndBackend(t, ProtocolNFS, fileShareBackendVHDX)
	var mounted bool
	mockBackend.getNfsShareFn = func(ctx context.Context, name, parentDir string) (bool, VolumeInfo, error) {
		return false, VolumeInfo{}, nil
	}
	mockBackend.getVolumeByNameFn = func(ctx context.Context, name, parentDir string) (bool, string, int64, string, string, int32, error) {
		assert.Equal(t, "D:\\nfs-vhdx", parentDir)
		return false, "", 0, "", "", -1, nil
	}
	mockBackend.exportSnapFn = func(ctx context.Context, snapshotID string) (string, error) {
		assert.Equal(t, "snap-001", snapshotID)
		return "D:\\nfs-vhdx\\nfs-from-snap.vhdx", nil
	}
	mockBackend.getVolumeInfoFn = func(ctx context.Context, vhdxPath string) (VolumeInfo, error) {
		return VolumeInfo{VHDXPath: vhdxPath, SizeBytes: 1073741824}, nil
	}
	mockBackend.mountFileShareVhdxFn = func(ctx context.Context, vhdxPath, mountPath string) error {
		assert.Equal(t, "D:\\nfs-vhdx\\nfs-from-snap.vhdx", vhdxPath)
		assert.Equal(t, "D:\\shares\\nfs-from-snap", mountPath)
		mounted = true
		return nil
	}
	mockBackend.createNfsShareWithOptionsFn = func(ctx context.Context, name, parentDir string, sizeBytes int64, clients []string, opts ...NfsShareOptions) (VolumeInfo, error) {
		require.Len(t, opts, 1)
		assert.Equal(t, []string{"10.0.0.10", "10.0.0.11"}, clients)
		assert.Equal(t, "clientgroup", opts[0].ClientType)
		assert.Equal(t, "readonly", opts[0].Permission)
		require.NotNil(t, opts[0].AllowRootAccess)
		assert.False(t, *opts[0].AllowRootAccess)
		assert.Equal(t, []string{"sys", "krb5"}, opts[0].Authentication)
		return VolumeInfo{
			VolumeName:    name,
			Protocol:      ProtocolNFS,
			NfsServer:     "nfs-server",
			NfsExportPath: "/" + name,
			VHDXPath:      parentDir + "\\" + name,
			SharePath:     parentDir + "\\" + name,
			CapacityBytes: sizeBytes,
		}, nil
	}

	resp, err := cs.CreateVolume(context.Background(), &csi.CreateVolumeRequest{
		Name: "nfs-from-snap",
		VolumeCapabilities: []*csi.VolumeCapability{
			{AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER}},
		},
		Parameters: map[string]string{
			"protocol":                "nfs",
			"shareBackend":            "vhdx",
			"shareParentPath":         "D:\\shares",
			"vhdxParentPath":          "D:\\nfs-vhdx",
			"nfsServer":               "10.0.0.2",
			"nfsClientName":           "10.0.0.10,10.0.0.11",
			"nfsClientType":           "clientgroup",
			"nfsPermission":           "ro",
			"nfsAllowRootAccess":      "false",
			"nfsAuthentication":       "sys,krb5",
			"nfsEnableUnmappedAccess": "true",
		},
		VolumeContentSource: &csi.VolumeContentSource{
			Type: &csi.VolumeContentSource_Snapshot{
				Snapshot: &csi.VolumeContentSource_SnapshotSource{SnapshotId: newTestSnapshotID(t)},
			},
		},
	})

	require.NoError(t, err)
	require.NotNil(t, resp.Volume)
	assert.True(t, mounted)
	assert.NotNil(t, resp.Volume.ContentSource)
	decoded, err := DecodeVolumeID(resp.Volume.VolumeId)
	require.NoError(t, err)
	assert.Equal(t, fileShareBackendVHDX, decoded.ShareBackend)
	assert.Equal(t, "D:\\nfs-vhdx\\nfs-from-snap.vhdx", decoded.VHDXPath)
	assert.Equal(t, "D:\\shares\\nfs-from-snap", decoded.SharePath)
}

func TestCreateVolume_SMBOptionsAndSnapshotRestore(t *testing.T) {
	cs, _, mockBackend := newTestControllerServerForProtocolAndBackend(t, ProtocolSMB, fileShareBackendVHDX)
	var mounted bool
	mockBackend.getSmbShareFn = func(ctx context.Context, name, parentDir string) (bool, VolumeInfo, error) {
		return false, VolumeInfo{}, nil
	}
	mockBackend.getVolumeByNameFn = func(ctx context.Context, name, parentDir string) (bool, string, int64, string, string, int32, error) {
		assert.Equal(t, "D:\\smb-vhdx", parentDir)
		return false, "", 0, "", "", -1, nil
	}
	mockBackend.exportSnapFn = func(ctx context.Context, snapshotID string) (string, error) {
		assert.Equal(t, "snap-001", snapshotID)
		return "D:\\smb-vhdx\\smb-from-snap.vhdx", nil
	}
	mockBackend.getVolumeInfoFn = func(ctx context.Context, vhdxPath string) (VolumeInfo, error) {
		return VolumeInfo{VHDXPath: vhdxPath, SizeBytes: 1073741824}, nil
	}
	mockBackend.mountFileShareVhdxFn = func(ctx context.Context, vhdxPath, mountPath string) error {
		assert.Equal(t, "D:\\smb-vhdx\\smb-from-snap.vhdx", vhdxPath)
		assert.Equal(t, "D:\\shares\\smb-from-snap", mountPath)
		mounted = true
		return nil
	}
	mockBackend.createSmbShareWithOptionsFn = func(ctx context.Context, name, parentDir string, sizeBytes int64, fullAccess, changeAccess, readAccess []string, opts ...SmbShareOptions) (VolumeInfo, error) {
		require.Len(t, opts, 1)
		assert.Equal(t, []string{"DOMAIN\\storage"}, fullAccess)
		assert.Equal(t, []string{"DOMAIN\\blocked"}, opts[0].NoAccess)
		assert.Equal(t, "CSI managed share", opts[0].Description)
		require.NotNil(t, opts[0].EncryptData)
		assert.True(t, *opts[0].EncryptData)
		assert.Equal(t, "None", opts[0].CachingMode)
		assert.Equal(t, "AccessBased", opts[0].FolderEnumerationMode)
		assert.Equal(t, uint32(128), opts[0].ConcurrentUserLimit)
		return VolumeInfo{
			VolumeName:    name,
			Protocol:      ProtocolSMB,
			SmbServer:     "smb-server",
			SmbShareName:  name,
			VHDXPath:      parentDir + "\\" + name,
			SharePath:     parentDir + "\\" + name,
			CapacityBytes: sizeBytes,
		}, nil
	}

	resp, err := cs.CreateVolume(context.Background(), &csi.CreateVolumeRequest{
		Name: "smb-from-snap",
		VolumeCapabilities: []*csi.VolumeCapability{
			{AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER}},
		},
		Parameters: map[string]string{
			"protocol":                 "smb",
			"shareBackend":             "vhdx",
			"shareParentPath":          "D:\\shares",
			"vhdxParentPath":           "D:\\smb-vhdx",
			"smbServer":                "10.0.0.3",
			"smbFullAccess":            "DOMAIN\\storage",
			"smbNoAccess":              "DOMAIN\\blocked",
			"smbDescription":           "CSI managed share",
			"smbEncryptData":           "true",
			"smbCachingMode":           "None",
			"smbFolderEnumerationMode": "AccessBased",
			"smbConcurrentUserLimit":   "128",
		},
		VolumeContentSource: &csi.VolumeContentSource{
			Type: &csi.VolumeContentSource_Snapshot{
				Snapshot: &csi.VolumeContentSource_SnapshotSource{SnapshotId: newTestSnapshotID(t)},
			},
		},
	})

	require.NoError(t, err)
	require.NotNil(t, resp.Volume)
	assert.True(t, mounted)
	assert.NotNil(t, resp.Volume.ContentSource)
	decoded, err := DecodeVolumeID(resp.Volume.VolumeId)
	require.NoError(t, err)
	assert.Equal(t, fileShareBackendVHDX, decoded.ShareBackend)
	assert.Equal(t, "D:\\smb-vhdx\\smb-from-snap.vhdx", decoded.VHDXPath)
	assert.Equal(t, "D:\\shares\\smb-from-snap", decoded.SharePath)
}

func TestCreateVolume_VHDXBackedNFS_IdempotentExistingShare(t *testing.T) {
	cs, _, mockBackend := newTestControllerServerForProtocolAndBackend(t, ProtocolNFS, fileShareBackendVHDX)
	var mounted bool
	mockBackend.getNfsShareFn = func(ctx context.Context, name, parentDir string) (bool, VolumeInfo, error) {
		assert.Equal(t, "nfs-existing", name)
		assert.Equal(t, "D:\\shares", parentDir)
		return true, VolumeInfo{
			VolumeName:    name,
			Protocol:      ProtocolNFS,
			NfsServer:     "backend-server",
			NfsExportPath: "/" + name,
			VHDXPath:      "D:\\shares\\nfs-existing",
			CapacityBytes: 1073741824,
		}, nil
	}
	mockBackend.getVolumeByNameFn = func(ctx context.Context, name, parentDir string) (bool, string, int64, string, string, int32, error) {
		assert.Equal(t, "nfs-existing", name)
		assert.Equal(t, "D:\\nfs-vhdx", parentDir)
		return true, "D:\\nfs-vhdx\\nfs-existing.vhdx", 2147483648, "", "", -1, nil
	}
	mockBackend.createVirtualDiskFn = func(ctx context.Context, name, parentDir string, sizeBytes int64) (string, int64, error) {
		t.Fatalf("idempotent VHDX-backed NFS create should not create another virtual disk")
		return "", 0, nil
	}
	mockBackend.createNfsShareWithOptionsFn = func(ctx context.Context, name, parentDir string, sizeBytes int64, clients []string, opts ...NfsShareOptions) (VolumeInfo, error) {
		t.Fatalf("idempotent VHDX-backed NFS create should not recreate an existing share")
		return VolumeInfo{}, nil
	}
	mockBackend.mountFileShareVhdxFn = func(ctx context.Context, vhdxPath, mountPath string) error {
		assert.Equal(t, "D:\\nfs-vhdx\\nfs-existing.vhdx", vhdxPath)
		assert.Equal(t, "D:\\shares\\nfs-existing", mountPath)
		mounted = true
		return nil
	}

	resp, err := cs.CreateVolume(context.Background(), &csi.CreateVolumeRequest{
		Name: "nfs-existing",
		VolumeCapabilities: []*csi.VolumeCapability{
			{AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER}},
		},
		CapacityRange: &csi.CapacityRange{RequiredBytes: 1073741824},
		Parameters: map[string]string{
			"protocol":        "nfs",
			"shareBackend":    "vhdx",
			"shareParentPath": "D:\\shares",
			"vhdxParentPath":  "D:\\nfs-vhdx",
			"nfsServer":       "10.0.0.2",
		},
	})

	require.NoError(t, err)
	require.NotNil(t, resp.Volume)
	assert.True(t, mounted)
	assert.Equal(t, int64(2147483648), resp.Volume.CapacityBytes)
	assert.Equal(t, "10.0.0.2", resp.Volume.VolumeContext["nfsServer"])
	decoded, err := DecodeVolumeID(resp.Volume.VolumeId)
	require.NoError(t, err)
	assert.Equal(t, fileShareBackendVHDX, decoded.ShareBackend)
	assert.Equal(t, "D:\\nfs-vhdx\\nfs-existing.vhdx", decoded.VHDXPath)
	assert.Equal(t, "D:\\shares\\nfs-existing", decoded.SharePath)
	assert.Equal(t, int64(2147483648), decoded.CapacityBytes)
}

func TestCreateVolume_VHDXBackedSMB_ExistingShareMissingVHDX(t *testing.T) {
	cs, _, mockBackend := newTestControllerServerForProtocolAndBackend(t, ProtocolSMB, fileShareBackendVHDX)
	mockBackend.getSmbShareFn = func(ctx context.Context, name, parentDir string) (bool, VolumeInfo, error) {
		assert.Equal(t, "smb-existing", name)
		assert.Equal(t, "D:\\shares", parentDir)
		return true, VolumeInfo{
			VolumeName:    name,
			Protocol:      ProtocolSMB,
			SmbServer:     "smb-server",
			SmbShareName:  name,
			VHDXPath:      "D:\\shares\\smb-existing",
			CapacityBytes: 1073741824,
		}, nil
	}
	mockBackend.getVolumeByNameFn = func(ctx context.Context, name, parentDir string) (bool, string, int64, string, string, int32, error) {
		assert.Equal(t, "smb-existing", name)
		assert.Equal(t, "D:\\smb-vhdx", parentDir)
		return false, "", 0, "", "", -1, nil
	}
	mockBackend.mountFileShareVhdxFn = func(ctx context.Context, vhdxPath, mountPath string) error {
		t.Fatalf("existing SMB share without a VHDX should fail before mounting; vhdxPath=%s mountPath=%s", vhdxPath, mountPath)
		return nil
	}

	resp, err := cs.CreateVolume(context.Background(), &csi.CreateVolumeRequest{
		Name: "smb-existing",
		VolumeCapabilities: []*csi.VolumeCapability{
			{AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER}},
		},
		Parameters: map[string]string{
			"protocol":        "smb",
			"shareBackend":    "vhdx",
			"shareParentPath": "D:\\shares",
			"vhdxParentPath":  "D:\\smb-vhdx",
			"smbServer":       "10.0.0.3",
		},
	})

	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "FailedPrecondition")
	assert.Contains(t, err.Error(), "exists but VHDX")
}

// ---------------------------------------------------------------------------
// DeleteVolume tests
// ---------------------------------------------------------------------------

func TestDeleteVolume_VolumeIDRequired(t *testing.T) {
	cs, _, _ := newTestControllerServer(t)
	_, err := cs.DeleteVolume(context.Background(), &csi.DeleteVolumeRequest{
		VolumeId: "",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "volume_id is required")
}

func TestDeleteVolume_Idempotent_InvalidID(t *testing.T) {
	cs, _, _ := newTestControllerServer(t)
	resp, err := cs.DeleteVolume(context.Background(), &csi.DeleteVolumeRequest{
		VolumeId: "invalid-base64!!!",
	})
	assert.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestDeleteVolume_Success(t *testing.T) {
	cs, _, mockBackend := newTestControllerServer(t)

	mockBackend.deleteVirtualDiskFn = func(ctx context.Context, vhdxPath string) error {
		assert.Equal(t, "D:\\vhdx\\test-volume.vhdx", vhdxPath)
		return nil
	}
	mockBackend.unmapDiskFromTargetFn = func(ctx context.Context, targetIQN, vhdxPath string) error {
		assert.Equal(t, "iqn.2024-01.com.example:test-volume", targetIQN)
		assert.Equal(t, "D:\\vhdx\\test-volume.vhdx", vhdxPath)
		return nil
	}

	resp, err := cs.DeleteVolume(context.Background(), &csi.DeleteVolumeRequest{
		VolumeId: newTestVolumeID(t),
	})

	require.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestDeleteVolume_UsesTargetNameWhenPresent(t *testing.T) {
	cs, _, mockBackend := newTestControllerServer(t)

	mockBackend.unmapDiskFromTargetFn = func(ctx context.Context, targetName, vhdxPath string) error {
		assert.Equal(t, "test-volume", targetName)
		assert.Equal(t, "D:\\vhdx\\test-volume.vhdx", vhdxPath)
		return nil
	}

	resp, err := cs.DeleteVolume(context.Background(), &csi.DeleteVolumeRequest{
		VolumeId: newTestVolumeIDWithTargetName(t),
	})

	require.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestDeleteVolume_NFS(t *testing.T) {
	cs, _, mockBackend := newTestControllerServer(t)
	vid := EncodeVolumeID(&VolumeID{Name: "nfs-vol", Protocol: ProtocolNFS, NfsServer: "10.0.0.2", NfsExportPath: "/nfs-vol"})
	mockBackend.deleteNfsShareFn = func(ctx context.Context, name, path string) error {
		assert.Equal(t, "nfs-vol", name)
		return nil
	}

	resp, err := cs.DeleteVolume(context.Background(), &csi.DeleteVolumeRequest{VolumeId: vid})
	require.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestDeleteVolume_VHDXBackedNFSUnmountsAndDeletes(t *testing.T) {
	cs, _, mockBackend := newTestControllerServerForProtocolAndBackend(t, ProtocolNFS, fileShareBackendVHDX)
	vid := EncodeVolumeID(&VolumeID{
		Name:          "nfs-vhdx",
		Protocol:      ProtocolNFS,
		ShareBackend:  fileShareBackendVHDX,
		SharePath:     "D:\\shares\\nfs-vhdx",
		NfsServer:     "10.0.0.2",
		NfsExportPath: "/nfs-vhdx",
		VHDXPath:      "D:\\nfs-vhdx\\nfs-vhdx.vhdx",
	})
	var deletedShare, unmounted, deletedDisk bool
	mockBackend.deleteNfsShareFn = func(ctx context.Context, name, path string) error {
		assert.Equal(t, "nfs-vhdx", name)
		assert.Equal(t, "D:\\shares\\nfs-vhdx", path)
		deletedShare = true
		return nil
	}
	mockBackend.unmountFileShareVhdxFn = func(ctx context.Context, vhdxPath, mountPath string) error {
		assert.Equal(t, "D:\\nfs-vhdx\\nfs-vhdx.vhdx", vhdxPath)
		assert.Equal(t, "D:\\shares\\nfs-vhdx", mountPath)
		unmounted = true
		return nil
	}
	mockBackend.deleteVirtualDiskFn = func(ctx context.Context, vhdxPath string) error {
		assert.Equal(t, "D:\\nfs-vhdx\\nfs-vhdx.vhdx", vhdxPath)
		deletedDisk = true
		return nil
	}

	resp, err := cs.DeleteVolume(context.Background(), &csi.DeleteVolumeRequest{VolumeId: vid})
	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.True(t, deletedShare)
	assert.True(t, unmounted)
	assert.True(t, deletedDisk)
}

func TestDeleteVolume_VHDXBackedSMBUnmountsAndDeletes(t *testing.T) {
	cs, _, mockBackend := newTestControllerServerForProtocolAndBackend(t, ProtocolSMB, fileShareBackendVHDX)
	vid := EncodeVolumeID(&VolumeID{
		Name:         "smb-vhdx",
		Protocol:     ProtocolSMB,
		ShareBackend: fileShareBackendVHDX,
		SharePath:    "D:\\shares\\smb-vhdx",
		SmbServer:    "10.0.0.3",
		SmbShareName: "smb-vhdx",
		VHDXPath:     "D:\\smb-vhdx\\smb-vhdx.vhdx",
	})
	var deletedShare, unmounted, deletedDisk bool
	mockBackend.deleteSmbShareFn = func(ctx context.Context, name, path string) error {
		assert.Equal(t, "smb-vhdx", name)
		assert.Equal(t, "D:\\shares\\smb-vhdx", path)
		deletedShare = true
		return nil
	}
	mockBackend.unmountFileShareVhdxFn = func(ctx context.Context, vhdxPath, mountPath string) error {
		assert.Equal(t, "D:\\smb-vhdx\\smb-vhdx.vhdx", vhdxPath)
		assert.Equal(t, "D:\\shares\\smb-vhdx", mountPath)
		unmounted = true
		return nil
	}
	mockBackend.deleteVirtualDiskFn = func(ctx context.Context, vhdxPath string) error {
		assert.Equal(t, "D:\\smb-vhdx\\smb-vhdx.vhdx", vhdxPath)
		deletedDisk = true
		return nil
	}

	resp, err := cs.DeleteVolume(context.Background(), &csi.DeleteVolumeRequest{VolumeId: vid})
	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.True(t, deletedShare)
	assert.True(t, unmounted)
	assert.True(t, deletedDisk)
}

func TestControllerPublishVolume_SMB(t *testing.T) {
	cs, _, _ := newTestControllerServerForProtocol(t, ProtocolSMB)
	vid := EncodeVolumeID(&VolumeID{Name: "smb-vol", Protocol: ProtocolSMB, SmbServer: "10.0.0.3", SmbShareName: "smb-vol"})

	resp, err := cs.ControllerPublishVolume(context.Background(), &csi.ControllerPublishVolumeRequest{
		VolumeId: vid,
		NodeId:   "windows-node-1",
		VolumeCapability: &csi.VolumeCapability{
			AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
		},
	})

	require.NoError(t, err)
	assert.Equal(t, "smb", resp.PublishContext["protocol"])
	assert.Equal(t, "10.0.0.3", resp.PublishContext["smbServer"])
	assert.Equal(t, "smb-vol", resp.PublishContext["smbShareName"])
}

// ---------------------------------------------------------------------------
// ControllerPublishVolume tests
// ---------------------------------------------------------------------------

func TestControllerPublishVolume_RequiredFields(t *testing.T) {
	cs, _, _ := newTestControllerServer(t)
	_, err := cs.ControllerPublishVolume(context.Background(), &csi.ControllerPublishVolumeRequest{
		VolumeId: "test",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "node_id are required")

	_, err = cs.ControllerPublishVolume(context.Background(), &csi.ControllerPublishVolumeRequest{
		NodeId: "node-001",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "volume_id are required")
}

func TestControllerPublishVolume_InvalidVolumeID(t *testing.T) {
	cs, _, _ := newTestControllerServer(t)
	_, err := cs.ControllerPublishVolume(context.Background(), &csi.ControllerPublishVolumeRequest{
		VolumeId: "invalid-base64!!!",
		NodeId:   "iqn.2024-01.com.example:node-001",
		VolumeCapability: &csi.VolumeCapability{
			AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
		},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "NotFound")
}

func TestControllerPublishVolume_InvalidInitiatorIQN(t *testing.T) {
	cs, _, _ := newTestControllerServer(t)
	_, err := cs.ControllerPublishVolume(context.Background(), &csi.ControllerPublishVolumeRequest{
		VolumeId: newTestVolumeID(t),
		NodeId:   "invalid-node-id",
		VolumeCapability: &csi.VolumeCapability{
			AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
		},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "initiator IQN")
}

func TestControllerPublishVolume_Success(t *testing.T) {
	cs, _, mockBackend := newTestControllerServer(t)

	mockBackend.allowInitiatorFn = func(ctx context.Context, targetIQN, initiatorIQN string) error {
		assert.Equal(t, "iqn.2024-01.com.example:test-volume", targetIQN)
		assert.Equal(t, "iqn.2024-01.com.example:node-001", initiatorIQN)
		return nil
	}

	resp, err := cs.ControllerPublishVolume(context.Background(), &csi.ControllerPublishVolumeRequest{
		VolumeId: newTestVolumeID(t),
		NodeId:   "iqn.2024-01.com.example:node-001",
		VolumeCapability: &csi.VolumeCapability{
			AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
		},
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, resp.PublishContext)
	assert.Equal(t, "10.0.0.1:3260", resp.PublishContext["targetPortal"])
	assert.Equal(t, "iqn.2024-01.com.example:test-volume", resp.PublishContext["iqn"])
	assert.Equal(t, "0", resp.PublishContext["lun"])
}

func TestControllerPublishVolume_ConfiguresWindowsTargetChap(t *testing.T) {
	cs, _, mockBackend := newTestControllerServer(t)
	var gotTargetName string
	var gotChap TargetChapOptions
	var configureTargetChapCalls int

	mockBackend.configureTargetChapFn = func(ctx context.Context, targetName string, opts TargetChapOptions) error {
		configureTargetChapCalls++
		gotTargetName = targetName
		gotChap = opts
		return nil
	}
	mockBackend.allowInitiatorFn = func(ctx context.Context, targetName, initiatorIQN string) error {
		assert.Equal(t, "iqn.2024-01.com.example:test-volume", targetName)
		return nil
	}

	resp, err := cs.ControllerPublishVolume(context.Background(), &csi.ControllerPublishVolumeRequest{
		VolumeId: newTestVolumeID(t),
		NodeId:   "iqn.2024-01.com.example:node-001",
		VolumeCapability: &csi.VolumeCapability{
			AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
		},
		Secrets: map[string]string{
			"node.session.auth.username": "dbnode01",
			"node.session.auth.password": "S3cret!",
		},
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, 1, configureTargetChapCalls)
	assert.Equal(t, "iqn.2024-01.com.example:test-volume", gotTargetName)
	assert.Equal(t, TargetChapOptions{ChapUser: "dbnode01", ChapSecret: "S3cret!"}, gotChap)
}

func TestControllerPublishVolume_UsesTargetNameWhenPresent(t *testing.T) {
	cs, _, mockBackend := newTestControllerServer(t)

	mockBackend.allowInitiatorFn = func(ctx context.Context, targetName, initiatorIQN string) error {
		assert.Equal(t, "test-volume", targetName)
		assert.Equal(t, "iqn.2024-01.com.example:node-001", initiatorIQN)
		return nil
	}

	resp, err := cs.ControllerPublishVolume(context.Background(), &csi.ControllerPublishVolumeRequest{
		VolumeId: newTestVolumeIDWithTargetName(t),
		NodeId:   "iqn.2024-01.com.example:node-001",
		VolumeCapability: &csi.VolumeCapability{
			AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
		},
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "iqn.1991-05.com.microsoft:win-storage-test-volume", resp.PublishContext["iqn"])
}

// ---------------------------------------------------------------------------
// ControllerUnpublishVolume tests
// ---------------------------------------------------------------------------

func TestControllerUnpublishVolume_RequiredFields(t *testing.T) {
	cs, _, _ := newTestControllerServer(t)
	_, err := cs.ControllerUnpublishVolume(context.Background(), &csi.ControllerUnpublishVolumeRequest{
		VolumeId: "test",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "node_id are required")
}

func TestControllerUnpublishVolume_Success(t *testing.T) {
	cs, _, mockBackend := newTestControllerServer(t)

	mockBackend.denyInitiatorFn = func(ctx context.Context, targetIQN, initiatorIQN string) error {
		assert.Equal(t, "iqn.2024-01.com.example:test-volume", targetIQN)
		assert.Equal(t, "iqn.2024-01.com.example:node-001", initiatorIQN)
		return nil
	}

	resp, err := cs.ControllerUnpublishVolume(context.Background(), &csi.ControllerUnpublishVolumeRequest{
		VolumeId: newTestVolumeID(t),
		NodeId:   "iqn.2024-01.com.example:node-001",
	})

	require.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestControllerUnpublishVolume_UsesTargetNameWhenPresent(t *testing.T) {
	cs, _, mockBackend := newTestControllerServer(t)

	mockBackend.denyInitiatorFn = func(ctx context.Context, targetName, initiatorIQN string) error {
		assert.Equal(t, "test-volume", targetName)
		assert.Equal(t, "iqn.2024-01.com.example:node-001", initiatorIQN)
		return nil
	}

	resp, err := cs.ControllerUnpublishVolume(context.Background(), &csi.ControllerUnpublishVolumeRequest{
		VolumeId: newTestVolumeIDWithTargetName(t),
		NodeId:   "iqn.2024-01.com.example:node-001",
	})

	require.NoError(t, err)
	assert.NotNil(t, resp)
}

// ---------------------------------------------------------------------------
// ValidateVolumeCapabilities tests
// ---------------------------------------------------------------------------

func TestValidateVolumeCapabilities_RequiredFields(t *testing.T) {
	cs, _, _ := newTestControllerServer(t)
	_, err := cs.ValidateVolumeCapabilities(context.Background(), &csi.ValidateVolumeCapabilitiesRequest{
		VolumeId: "",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "volume_id and volume_capabilities are required")

	_, err = cs.ValidateVolumeCapabilities(context.Background(), &csi.ValidateVolumeCapabilitiesRequest{
		VolumeId: "test",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "volume_id and volume_capabilities are required")
}

func TestValidateVolumeCapabilities_InvalidVolumeID(t *testing.T) {
	cs, _, _ := newTestControllerServer(t)
	resp, err := cs.ValidateVolumeCapabilities(context.Background(), &csi.ValidateVolumeCapabilitiesRequest{
		VolumeId: "invalid-base64!!!",
		VolumeCapabilities: []*csi.VolumeCapability{
			{AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER}},
		},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "NotFound")
	assert.Nil(t, resp)
}

func TestValidateVolumeCapabilities_Valid(t *testing.T) {
	cs, _, _ := newTestControllerServer(t)

	expectedCaps := []*csi.VolumeCapability{
		{AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER}},
	}
	resp, err := cs.ValidateVolumeCapabilities(context.Background(), &csi.ValidateVolumeCapabilitiesRequest{
		VolumeId:           newTestVolumeID(t),
		VolumeCapabilities: expectedCaps,
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, resp.Confirmed)
	assert.Equal(t, expectedCaps, resp.Confirmed.VolumeCapabilities)
}

func TestValidateVolumeCapabilities_InvalidAccessMode(t *testing.T) {
	cs, _, _ := newTestControllerServer(t)

	resp, err := cs.ValidateVolumeCapabilities(context.Background(), &csi.ValidateVolumeCapabilitiesRequest{
		VolumeId: newTestVolumeID(t),
		VolumeCapabilities: []*csi.VolumeCapability{
			{AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_SINGLE_WRITER}},
		},
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Nil(t, resp.Confirmed)
	assert.Contains(t, resp.Message, "access mode is not supported")
}

// ---------------------------------------------------------------------------
// ListVolumes tests
// ---------------------------------------------------------------------------

func TestListVolumes_Empty(t *testing.T) {
	cs, _, _ := newTestControllerServer(t)

	resp, err := cs.ListVolumes(context.Background(), &csi.ListVolumesRequest{})

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Empty(t, resp.Entries)
}

// ---------------------------------------------------------------------------
// GetCapacity tests
// ---------------------------------------------------------------------------

func TestGetCapacity_ISCSIUsesBackendDefaultWhenPathMissing(t *testing.T) {
	cs, _, mockBackend := newTestControllerServer(t)

	mockBackend.getDirectoryFreeCapFn = func(ctx context.Context, parentDir string) (int64, error) {
		assert.Empty(t, parentDir)
		return 42, nil
	}

	resp, err := cs.GetCapacity(context.Background(), &csi.GetCapacityRequest{})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, int64(42), resp.AvailableCapacity)
}

func TestGetCapacity_Success(t *testing.T) {
	cs, _, mockBackend := newTestControllerServer(t)

	mockBackend.getDirectoryFreeCapFn = func(ctx context.Context, parentDir string) (int64, error) {
		assert.Equal(t, "D:\\vhdx", parentDir)
		return 107374182400, nil
	}

	resp, err := cs.GetCapacity(context.Background(), &csi.GetCapacityRequest{
		Parameters: map[string]string{
			"vhdxParentPath": "D:\\vhdx",
		},
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, int64(107374182400), resp.AvailableCapacity)
}

func TestGetCapacity_FileShareDriverUsesShareParentPath(t *testing.T) {
	cs, _, mockBackend := newTestControllerServerForProtocol(t, ProtocolNFS)

	mockBackend.getDirectoryFreeCapFn = func(ctx context.Context, parentDir string) (int64, error) {
		assert.Equal(t, "D:\\shares", parentDir)
		return 42, nil
	}

	resp, err := cs.GetCapacity(context.Background(), &csi.GetCapacityRequest{
		Parameters: map[string]string{
			"shareParentPath": "D:\\shares",
		},
	})

	require.NoError(t, err)
	assert.Equal(t, int64(42), resp.AvailableCapacity)
}

// ---------------------------------------------------------------------------
// ControllerGetCapabilities tests
// ---------------------------------------------------------------------------

func TestControllerGetCapabilities(t *testing.T) {
	cs, d, _ := newTestControllerServer(t)

	resp, err := cs.ControllerGetCapabilities(context.Background(), &csi.ControllerGetCapabilitiesRequest{})

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, resp.Capabilities)
	assert.Len(t, resp.Capabilities, len(d.cscap))
}

// ---------------------------------------------------------------------------
// CreateSnapshot tests
// ---------------------------------------------------------------------------

func TestCreateSnapshot_SourceVolumeIDRequired(t *testing.T) {
	cs, _, _ := newTestControllerServer(t)
	_, err := cs.CreateSnapshot(context.Background(), &csi.CreateSnapshotRequest{
		Name: "test-snap",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "source_volume_id required")
}

func TestCreateSnapshot_InvalidSourceVolumeID(t *testing.T) {
	cs, _, _ := newTestControllerServer(t)
	_, err := cs.CreateSnapshot(context.Background(), &csi.CreateSnapshotRequest{
		Name:           "test-snap",
		SourceVolumeId: "invalid-base64!!!",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "NotFound")
}

func TestCreateSnapshot_Success(t *testing.T) {
	cs, _, mockBackend := newTestControllerServer(t)

	expectedSnap := SnapshotInfo{
		SnapshotID:   "snap-001",
		OriginalPath: "D:\\vhdx\\test-volume.vhdx",
		Description:  "test-snap",
		SizeBytes:    1073741824,
	}
	mockBackend.createSnapshotFn = func(ctx context.Context, vhdxPath, desc string) (SnapshotInfo, error) {
		assert.Equal(t, expectedSnap.OriginalPath, vhdxPath)
		assert.Equal(t, expectedSnap.Description, desc)
		return expectedSnap, nil
	}

	resp, err := cs.CreateSnapshot(context.Background(), &csi.CreateSnapshotRequest{
		Name:           "test-snap",
		SourceVolumeId: newTestVolumeID(t),
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, resp.Snapshot)
	assert.Equal(t, newTestSnapshotID(t), resp.Snapshot.SnapshotId)
	assert.Equal(t, newTestVolumeID(t), resp.Snapshot.SourceVolumeId)
	assert.True(t, resp.Snapshot.ReadyToUse)
}

func TestCreateSnapshot_DirectoryBackedNFSRejected(t *testing.T) {
	cs, _, mockBackend := newTestControllerServerForProtocol(t, ProtocolNFS)
	vid := EncodeVolumeID(&VolumeID{
		Name:          "nfs-vol",
		Protocol:      ProtocolNFS,
		ShareBackend:  fileShareBackendDirectory,
		NfsServer:     "10.0.0.2",
		NfsExportPath: "/nfs-vol",
		VHDXPath:      "D:\\shares\\nfs-vol",
	})
	mockBackend.createSnapshotFn = func(ctx context.Context, path, desc string) (SnapshotInfo, error) {
		t.Fatalf("directory-backed NFS CreateSnapshot should not call the backend; path: %s", path)
		return SnapshotInfo{}, nil
	}

	resp, err := cs.CreateSnapshot(context.Background(), &csi.CreateSnapshotRequest{
		Name:           "nfs-snap",
		SourceVolumeId: vid,
	})

	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "directory-backed")
}

func TestCreateSnapshot_VHDXBackedNFS(t *testing.T) {
	cs, _, mockBackend := newTestControllerServerForProtocolAndBackend(t, ProtocolNFS, fileShareBackendVHDX)
	vid := EncodeVolumeID(&VolumeID{
		Name:          "nfs-vol",
		Protocol:      ProtocolNFS,
		ShareBackend:  fileShareBackendVHDX,
		SharePath:     "D:\\shares\\nfs-vol",
		NfsServer:     "10.0.0.2",
		NfsExportPath: "/nfs-vol",
		VHDXPath:      "D:\\nfs-vhdx\\nfs-vol.vhdx",
	})
	mockBackend.createSnapshotFn = func(ctx context.Context, path, desc string) (SnapshotInfo, error) {
		assert.Equal(t, "D:\\nfs-vhdx\\nfs-vol.vhdx", path)
		assert.Equal(t, "nfs-snap", desc)
		return SnapshotInfo{
			SnapshotID:   "snap-001",
			OriginalPath: path,
			Description:  desc,
		}, nil
	}

	resp, err := cs.CreateSnapshot(context.Background(), &csi.CreateSnapshotRequest{
		Name:           "nfs-snap",
		SourceVolumeId: vid,
	})

	require.NoError(t, err)
	require.NotNil(t, resp.Snapshot)
	assert.Equal(t, vid, resp.Snapshot.SourceVolumeId)
	assert.True(t, resp.Snapshot.ReadyToUse)
	sid, err := decodeSnapID(resp.Snapshot.SnapshotId)
	require.NoError(t, err)
	assert.Equal(t, "snap-001", sid.SnapshotID)
	assert.Equal(t, "D:\\nfs-vhdx\\nfs-vol.vhdx", sid.OriginalPath)
}

// ---------------------------------------------------------------------------
// DeleteSnapshot tests
// ---------------------------------------------------------------------------

func TestDeleteSnapshot_SnapshotIDRequired(t *testing.T) {
	cs, _, _ := newTestControllerServer(t)
	_, err := cs.DeleteSnapshot(context.Background(), &csi.DeleteSnapshotRequest{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "snapshot_id required")
}

func TestDeleteSnapshot_InvalidSnapshotID(t *testing.T) {
	cs, _, _ := newTestControllerServer(t)
	resp, err := cs.DeleteSnapshot(context.Background(), &csi.DeleteSnapshotRequest{
		SnapshotId: "invalid-base64!!!",
	})
	assert.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestDeleteSnapshot_Success(t *testing.T) {
	cs, _, mockBackend := newTestControllerServer(t)

	mockBackend.deleteSnapshotFn = func(ctx context.Context, snapshotID string) error {
		assert.Equal(t, "snap-001", snapshotID)
		return nil
	}

	resp, err := cs.DeleteSnapshot(context.Background(), &csi.DeleteSnapshotRequest{
		SnapshotId: newTestSnapshotID(t),
	})

	require.NoError(t, err)
	assert.NotNil(t, resp)
}

// ---------------------------------------------------------------------------
// ListSnapshots tests
// ---------------------------------------------------------------------------

func TestListSnapshots_BySnapshotID(t *testing.T) {
	cs, _, mockBackend := newTestControllerServer(t)

	snaps := []SnapshotInfo{
		{SnapshotID: "snap-001", OriginalPath: "D:\\vhdx\\test-volume.vhdx", SizeBytes: 1073741824},
	}
	mockBackend.listSnapshotsFn = func(ctx context.Context, vhdxPath string) ([]SnapshotInfo, error) {
		assert.Equal(t, "D:\\vhdx\\test-volume.vhdx", vhdxPath)
		return snaps, nil
	}

	resp, err := cs.ListSnapshots(context.Background(), &csi.ListSnapshotsRequest{
		SnapshotId: newTestSnapshotID(t),
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Len(t, resp.Entries, 1)
}

func TestListSnapshots_BySourceVolumeID(t *testing.T) {
	cs, _, mockBackend := newTestControllerServer(t)

	snaps := []SnapshotInfo{
		{SnapshotID: "snap-001", OriginalPath: "D:\\vhdx\\test-volume.vhdx", SizeBytes: 1073741824},
	}
	mockBackend.listSnapshotsFn = func(ctx context.Context, vhdxPath string) ([]SnapshotInfo, error) {
		assert.Equal(t, "D:\\vhdx\\test-volume.vhdx", vhdxPath)
		return snaps, nil
	}

	resp, err := cs.ListSnapshots(context.Background(), &csi.ListSnapshotsRequest{
		SourceVolumeId: newTestVolumeID(t),
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Len(t, resp.Entries, 1)
}

func TestListSnapshots_NFSBySourceVolumeID(t *testing.T) {
	cs, _, mockBackend := newTestControllerServerForProtocol(t, ProtocolNFS)
	vid := EncodeVolumeID(&VolumeID{
		Name:          "nfs-vol",
		Protocol:      ProtocolNFS,
		NfsServer:     "10.0.0.2",
		NfsExportPath: "/nfs-vol",
		VHDXPath:      "D:\\shares\\nfs-vol",
	})
	mockBackend.listSnapshotsFn = func(ctx context.Context, path string) ([]SnapshotInfo, error) {
		t.Fatalf("NFS ListSnapshots should not call the backend; path: %s", path)
		return nil, nil
	}

	resp, err := cs.ListSnapshots(context.Background(), &csi.ListSnapshotsRequest{
		SourceVolumeId: vid,
	})

	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "Unimplemented")
}

func TestListSnapshots_VHDXBackedNFSBySourceVolumeID(t *testing.T) {
	cs, _, mockBackend := newTestControllerServerForProtocolAndBackend(t, ProtocolNFS, fileShareBackendVHDX)
	vid := EncodeVolumeID(&VolumeID{
		Name:          "nfs-vol",
		Protocol:      ProtocolNFS,
		ShareBackend:  fileShareBackendVHDX,
		SharePath:     "D:\\shares\\nfs-vol",
		NfsServer:     "10.0.0.2",
		NfsExportPath: "/nfs-vol",
		VHDXPath:      "D:\\nfs-vhdx\\nfs-vol.vhdx",
	})
	mockBackend.listSnapshotsFn = func(ctx context.Context, path string) ([]SnapshotInfo, error) {
		assert.Equal(t, "D:\\nfs-vhdx\\nfs-vol.vhdx", path)
		return []SnapshotInfo{
			{SnapshotID: "snap-001", OriginalPath: path, SizeBytes: 1073741824},
		}, nil
	}

	resp, err := cs.ListSnapshots(context.Background(), &csi.ListSnapshotsRequest{
		SourceVolumeId: vid,
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Len(t, resp.Entries, 1)
}

func TestListSnapshots_GlobalEnumeration(t *testing.T) {
	cs, _, _ := newTestControllerServer(t)

	resp, err := cs.ListSnapshots(context.Background(), &csi.ListSnapshotsRequest{})

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Empty(t, resp.Entries)
}

// ---------------------------------------------------------------------------
// ControllerExpandVolume tests
// ---------------------------------------------------------------------------

func TestControllerExpandVolume_RequiredFields(t *testing.T) {
	cs, _, _ := newTestControllerServer(t)
	_, err := cs.ControllerExpandVolume(context.Background(), &csi.ControllerExpandVolumeRequest{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "volume_id and capacity_range are required")

	_, err = cs.ControllerExpandVolume(context.Background(), &csi.ControllerExpandVolumeRequest{
		VolumeId: "test",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "volume_id and capacity_range are required")

	_, err = cs.ControllerExpandVolume(context.Background(), &csi.ControllerExpandVolumeRequest{
		VolumeId:      newTestVolumeID(t),
		CapacityRange: &csi.CapacityRange{},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "required_bytes must be > 0")
}

func TestControllerExpandVolume_InvalidVolumeID(t *testing.T) {
	cs, _, _ := newTestControllerServer(t)
	_, err := cs.ControllerExpandVolume(context.Background(), &csi.ControllerExpandVolumeRequest{
		VolumeId: "invalid-base64!!!",
		CapacityRange: &csi.CapacityRange{
			RequiredBytes: 2147483648,
		},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "NotFound")
}

func TestControllerExpandVolume_Success(t *testing.T) {
	cs, _, mockBackend := newTestControllerServer(t)

	mockBackend.resizeVirtualDiskFn = func(ctx context.Context, vhdxPath string, newSizeBytes int64) (int64, error) {
		assert.Equal(t, "D:\\vhdx\\test-volume.vhdx", vhdxPath)
		assert.Equal(t, int64(2147483648), newSizeBytes)
		return 2147483648, nil
	}

	resp, err := cs.ControllerExpandVolume(context.Background(), &csi.ControllerExpandVolumeRequest{
		VolumeId: newTestVolumeID(t),
		CapacityRange: &csi.CapacityRange{
			RequiredBytes: 2147483648,
		},
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, int64(2147483648), resp.CapacityBytes)
	assert.True(t, resp.NodeExpansionRequired)
}

func TestControllerExpandVolume_NFS(t *testing.T) {
	cs, _, mockBackend := newTestControllerServerForProtocol(t, ProtocolNFS)
	vid := EncodeVolumeID(&VolumeID{
		Name:          "nfs-vol",
		Protocol:      ProtocolNFS,
		NfsServer:     "10.0.0.2",
		NfsExportPath: "/nfs-vol",
		VHDXPath:      "D:\\shares\\nfs-vol",
		CapacityBytes: 1073741824,
	})
	mockBackend.resizeFileShareFn = func(ctx context.Context, path string, newSizeBytes int64) (int64, error) {
		assert.Equal(t, "D:\\shares\\nfs-vol", path)
		assert.Equal(t, int64(2147483648), newSizeBytes)
		return newSizeBytes, nil
	}

	resp, err := cs.ControllerExpandVolume(context.Background(), &csi.ControllerExpandVolumeRequest{
		VolumeId:      vid,
		CapacityRange: &csi.CapacityRange{RequiredBytes: 2147483648},
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, int64(2147483648), resp.CapacityBytes)
	assert.False(t, resp.NodeExpansionRequired)
}

func TestControllerExpandVolume_VHDXBackedNFSRemountsShare(t *testing.T) {
	cs, _, mockBackend := newTestControllerServerForProtocolAndBackend(t, ProtocolNFS, fileShareBackendVHDX)
	vid := EncodeVolumeID(&VolumeID{
		Name:          "nfs-vhdx",
		Protocol:      ProtocolNFS,
		ShareBackend:  fileShareBackendVHDX,
		SharePath:     "D:\\shares\\nfs-vhdx",
		NfsServer:     "10.0.0.2",
		NfsExportPath: "/nfs-vhdx",
		VHDXPath:      "D:\\nfs-vhdx\\nfs-vhdx.vhdx",
		CapacityBytes: 1073741824,
	})
	var remounted bool
	mockBackend.resizeVirtualDiskFn = func(ctx context.Context, vhdxPath string, newSizeBytes int64) (int64, error) {
		assert.Equal(t, "D:\\nfs-vhdx\\nfs-vhdx.vhdx", vhdxPath)
		assert.Equal(t, int64(2147483648), newSizeBytes)
		return newSizeBytes, nil
	}
	mockBackend.mountFileShareVhdxFn = func(ctx context.Context, vhdxPath, mountPath string) error {
		assert.Equal(t, "D:\\nfs-vhdx\\nfs-vhdx.vhdx", vhdxPath)
		assert.Equal(t, "D:\\shares\\nfs-vhdx", mountPath)
		remounted = true
		return nil
	}
	mockBackend.resizeFileShareFn = func(ctx context.Context, path string, newSizeBytes int64) (int64, error) {
		t.Fatalf("VHDX-backed file-share expansion should resize the virtual disk, not the directory quota")
		return 0, nil
	}

	resp, err := cs.ControllerExpandVolume(context.Background(), &csi.ControllerExpandVolumeRequest{
		VolumeId:      vid,
		CapacityRange: &csi.CapacityRange{RequiredBytes: 2147483648},
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, int64(2147483648), resp.CapacityBytes)
	assert.False(t, resp.NodeExpansionRequired)
	assert.True(t, remounted)
}

// ---------------------------------------------------------------------------
// ControllerGetVolume tests
// ---------------------------------------------------------------------------

func TestControllerGetVolume_VolumeIDRequired(t *testing.T) {
	cs, _, _ := newTestControllerServer(t)
	_, err := cs.ControllerGetVolume(context.Background(), &csi.ControllerGetVolumeRequest{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "volume_id required")
}

func TestControllerGetVolume_InvalidVolumeID(t *testing.T) {
	cs, _, _ := newTestControllerServer(t)
	_, err := cs.ControllerGetVolume(context.Background(), &csi.ControllerGetVolumeRequest{
		VolumeId: "invalid-base64!!!",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "NotFound")
}

func TestControllerGetVolume_Success(t *testing.T) {
	cs, _, mockBackend := newTestControllerServer(t)

	lun := int32(0)
	mockBackend.getVolumeInfoFn = func(ctx context.Context, vhdxPath string) (VolumeInfo, error) {
		assert.Equal(t, "D:\\vhdx\\test-volume.vhdx", vhdxPath)
		return VolumeInfo{VHDXPath: vhdxPath, SizeBytes: 1073741824, Targets: []string{"iqn.2024-01.com.example:test-volume"}, LUN: &lun}, nil
	}
	mockBackend.getTargetInitiatorsFn = func(ctx context.Context, targetIQN string) ([]string, error) {
		return []string{"iqn.2024-01.com.example:node-001"}, nil
	}

	resp, err := cs.ControllerGetVolume(context.Background(), &csi.ControllerGetVolumeRequest{
		VolumeId: newTestVolumeID(t),
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, resp.Volume)
	require.NotNil(t, resp.Status)
	require.NotNil(t, resp.Status.VolumeCondition)
	assert.False(t, resp.Status.VolumeCondition.Abnormal)
	assert.Equal(t, "OK", resp.Status.VolumeCondition.Message)
	assert.Equal(t, []string{"iqn.2024-01.com.example:node-001"}, resp.Status.PublishedNodeIds)
}

func TestControllerGetVolume_VolumeNotFound(t *testing.T) {
	cs, _, mockBackend := newTestControllerServer(t)

	mockBackend.getVolumeInfoFn = func(ctx context.Context, vhdxPath string) (VolumeInfo, error) {
		return VolumeInfo{VHDXPath: ""}, nil
	}

	resp, err := cs.ControllerGetVolume(context.Background(), &csi.ControllerGetVolumeRequest{
		VolumeId: newTestVolumeID(t),
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
	assert.Nil(t, resp)
}
