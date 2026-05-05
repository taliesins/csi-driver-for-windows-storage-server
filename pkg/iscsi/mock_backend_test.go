package iscsi

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// ---------------------------------------------------------------------------
// MockBackend (generated-style mock for testing)
// ---------------------------------------------------------------------------

// MockBackend is a mock implementation of the Backend interface for testing.
type MockBackend struct {
	mock.Mock
}

func mockInt64(v any) int64 {
	switch n := v.(type) {
	case int:
		return int64(n)
	case int64:
		return n
	default:
		return 0
	}
}

// EnsureTarget mocks EnsureTarget.
func (m *MockBackend) EnsureTarget(ctx context.Context, targetName, targetIQN string) (string, error) {
	args := m.Called(ctx, targetName, targetIQN)
	return args.String(0), args.Error(1)
}

// CreateVirtualDisk mocks CreateVirtualDisk.
func (m *MockBackend) CreateVirtualDisk(ctx context.Context, name, parentDir string, sizeBytes int64) (string, int64, error) {
	args := m.Called(ctx, name, parentDir, sizeBytes)
	return args.String(0), mockInt64(args.Get(1)), args.Error(2)
}

// MapDiskToTarget mocks MapDiskToTarget.
func (m *MockBackend) MapDiskToTarget(ctx context.Context, targetName, vhdxPath string) (int32, error) {
	args := m.Called(ctx, targetName, vhdxPath)
	return args.Get(0).(int32), args.Error(1)
}

// UnmapDiskFromTarget mocks UnmapDiskFromTarget.
func (m *MockBackend) UnmapDiskFromTarget(ctx context.Context, targetName, vhdxPath string) error {
	args := m.Called(ctx, targetName, vhdxPath)
	return args.Error(0)
}

// DeleteVirtualDisk mocks DeleteVirtualDisk.
func (m *MockBackend) DeleteVirtualDisk(ctx context.Context, vhdxPath string) error {
	args := m.Called(ctx, vhdxPath)
	return args.Error(0)
}

// GetVolumeByName mocks GetVolumeByName.
func (m *MockBackend) GetVolumeByName(ctx context.Context, name, parentDir string) (bool, string, int64, string, string, int32, error) {
	args := m.Called(ctx, name, parentDir)
	return args.Bool(0), args.String(1), mockInt64(args.Get(2)), args.String(3), args.String(4), args.Get(5).(int32), args.Error(6)
}

// AllowInitiator mocks AllowInitiator.
func (m *MockBackend) AllowInitiator(ctx context.Context, targetName, initiatorIQN string) error {
	args := m.Called(ctx, targetName, initiatorIQN)
	return args.Error(0)
}

// DenyInitiator mocks DenyInitiator.
func (m *MockBackend) DenyInitiator(ctx context.Context, targetName, initiatorIQN string) error {
	args := m.Called(ctx, targetName, initiatorIQN)
	return args.Error(0)
}

// GetDirectoryFreeCapacity mocks GetDirectoryFreeCapacity.
func (m *MockBackend) GetDirectoryFreeCapacity(ctx context.Context, parentDir string) (int64, error) {
	args := m.Called(ctx, parentDir)
	return args.Get(0).(int64), args.Error(1)
}

// CreateSnapshot mocks CreateSnapshot.
func (m *MockBackend) CreateSnapshot(ctx context.Context, vhdxPath, description string) (SnapshotInfo, error) {
	args := m.Called(ctx, vhdxPath, description)
	return args.Get(0).(SnapshotInfo), args.Error(1)
}

// DeleteSnapshot mocks DeleteSnapshot.
func (m *MockBackend) DeleteSnapshot(ctx context.Context, snapshotID string) error {
	args := m.Called(ctx, snapshotID)
	return args.Error(0)
}

// ListSnapshots mocks ListSnapshots.
func (m *MockBackend) ListSnapshots(ctx context.Context, vhdxPath string) ([]SnapshotInfo, error) {
	args := m.Called(ctx, vhdxPath)
	return args.Get(0).([]SnapshotInfo), args.Error(1)
}

// ExportSnapshotAsVirtualDisk mocks ExportSnapshotAsVirtualDisk.
func (m *MockBackend) ExportSnapshotAsVirtualDisk(ctx context.Context, snapshotID string) (string, error) {
	args := m.Called(ctx, snapshotID)
	return args.String(0), args.Error(1)
}

// ResizeVirtualDisk mocks ResizeVirtualDisk.
func (m *MockBackend) ResizeVirtualDisk(ctx context.Context, vhdxPath string, newSizeBytes int64) (int64, error) {
	args := m.Called(ctx, vhdxPath, newSizeBytes)
	return mockInt64(args.Get(0)), args.Error(1)
}

// GetVolumeInfo mocks GetVolumeInfo.
func (m *MockBackend) GetVolumeInfo(ctx context.Context, vhdxPath string) (VolumeInfo, error) {
	args := m.Called(ctx, vhdxPath)
	return args.Get(0).(VolumeInfo), args.Error(1)
}

// GetTargetInitiators mocks GetTargetInitiators.
func (m *MockBackend) GetTargetInitiators(ctx context.Context, targetName string) ([]string, error) {
	args := m.Called(ctx, targetName)
	return args.Get(0).([]string), args.Error(1)
}

func (m *MockBackend) CreateNfsShare(ctx context.Context, name, parentDir string, sizeBytes int64, clients []string, opts ...NfsShareOptions) (VolumeInfo, error) {
	callArgs := []any{ctx, name, parentDir, sizeBytes, clients}
	for _, opt := range opts {
		callArgs = append(callArgs, opt)
	}
	args := m.Called(callArgs...)
	return args.Get(0).(VolumeInfo), args.Error(1)
}

func (m *MockBackend) GetNfsShare(ctx context.Context, name, parentDir string) (bool, VolumeInfo, error) {
	args := m.Called(ctx, name, parentDir)
	return args.Bool(0), args.Get(1).(VolumeInfo), args.Error(2)
}

func (m *MockBackend) DeleteNfsShare(ctx context.Context, name, path string) error {
	args := m.Called(ctx, name, path)
	return args.Error(0)
}

func (m *MockBackend) CreateSmbShare(ctx context.Context, name, parentDir string, sizeBytes int64, fullAccess, changeAccess, readAccess []string, opts ...SmbShareOptions) (VolumeInfo, error) {
	callArgs := []any{ctx, name, parentDir, sizeBytes, fullAccess, changeAccess, readAccess}
	for _, opt := range opts {
		callArgs = append(callArgs, opt)
	}
	args := m.Called(callArgs...)
	return args.Get(0).(VolumeInfo), args.Error(1)
}

func (m *MockBackend) GetSmbShare(ctx context.Context, name, parentDir string) (bool, VolumeInfo, error) {
	args := m.Called(ctx, name, parentDir)
	return args.Bool(0), args.Get(1).(VolumeInfo), args.Error(2)
}

func (m *MockBackend) DeleteSmbShare(ctx context.Context, name, path string) error {
	args := m.Called(ctx, name, path)
	return args.Error(0)
}

func (m *MockBackend) ResizeFileShare(ctx context.Context, path string, newSizeBytes int64) (int64, error) {
	args := m.Called(ctx, path, newSizeBytes)
	return mockInt64(args.Get(0)), args.Error(1)
}

func (m *MockBackend) RestoreSnapshotAsFileShare(ctx context.Context, snapshotID, destinationPath string) error {
	args := m.Called(ctx, snapshotID, destinationPath)
	return args.Error(0)
}

func (m *MockBackend) MountFileShareVirtualDisk(ctx context.Context, vhdxPath, mountPath string) error {
	args := m.Called(ctx, vhdxPath, mountPath)
	return args.Error(0)
}

func (m *MockBackend) UnmountFileShareVirtualDisk(ctx context.Context, vhdxPath, mountPath string) error {
	args := m.Called(ctx, vhdxPath, mountPath)
	return args.Error(0)
}

// ---------------------------------------------------------------------------
// MockBackend usage examples (integration-style tests)
// ---------------------------------------------------------------------------

func TestMockBackend_EnsureTarget(t *testing.T) {
	m := new(MockBackend)
	m.On("EnsureTarget", mock.Anything, "target-test", "iqn.2024-01.com.example:test").
		Return("iqn.2024-01.com.example:test", nil)

	targetIQN, err := m.EnsureTarget(context.Background(), "target-test", "iqn.2024-01.com.example:test")
	assert.NoError(t, err)
	assert.Equal(t, "iqn.2024-01.com.example:test", targetIQN)

	m.AssertExpectations(t)
}

func TestMockBackend_CreateVirtualDisk(t *testing.T) {
	m := new(MockBackend)
	m.On("CreateVirtualDisk", mock.Anything, "test-vol", "D:\\vhdx", int64(1073741824)).
		Return("D:\\vhdx\\test-vol.vhdx", 1073741824, nil)

	path, size, err := m.CreateVirtualDisk(context.Background(), "test-vol", "D:\\vhdx", 1073741824)
	assert.NoError(t, err)
	assert.Equal(t, "D:\\vhdx\\test-vol.vhdx", path)
	assert.Equal(t, int64(1073741824), size)

	m.AssertExpectations(t)
}

func TestMockBackend_MapDiskToTarget(t *testing.T) {
	m := new(MockBackend)
	m.On("MapDiskToTarget", mock.Anything, "iqn.2024-01.com.example:test", "D:\\vhdx\\test-vol.vhdx").
		Return(int32(0), nil)

	lun, err := m.MapDiskToTarget(context.Background(), "iqn.2024-01.com.example:test", "D:\\vhdx\\test-vol.vhdx")
	assert.NoError(t, err)
	assert.Equal(t, int32(0), lun)

	m.AssertExpectations(t)
}

func TestMockBackend_DeleteVirtualDisk(t *testing.T) {
	m := new(MockBackend)
	m.On("DeleteVirtualDisk", mock.Anything, "D:\\vhdx\\test-vol.vhdx").Return(nil)

	err := m.DeleteVirtualDisk(context.Background(), "D:\\vhdx\\test-vol.vhdx")
	assert.NoError(t, err)

	m.AssertExpectations(t)
}

func TestMockBackend_GetVolumeByName(t *testing.T) {
	m := new(MockBackend)
	m.On("GetVolumeByName", mock.Anything, "test-vol", "D:\\vhdx").
		Return(true, "D:\\vhdx\\test-vol.vhdx", 1073741824, "target-test", "iqn.2024-01.com.example:test", int32(0), nil)

	exists, path, size, targetName, targetIQN, lun, err := m.GetVolumeByName(context.Background(), "test-vol", "D:\\vhdx")
	assert.NoError(t, err)
	assert.True(t, exists)
	assert.Equal(t, "D:\\vhdx\\test-vol.vhdx", path)
	assert.Equal(t, int64(1073741824), size)
	assert.Equal(t, "target-test", targetName)
	assert.Equal(t, "iqn.2024-01.com.example:test", targetIQN)
	assert.Equal(t, int32(0), lun)

	m.AssertExpectations(t)
}

func TestMockBackend_CreateSnapshot(t *testing.T) {
	m := new(MockBackend)
	expected := SnapshotInfo{
		SnapshotID:   "snap-001",
		OriginalPath: "D:\\vhdx\\test-vol.vhdx",
		Description:  "test snapshot",
		SizeBytes:    1073741824,
	}
	m.On("CreateSnapshot", mock.Anything, "D:\\vhdx\\test-vol.vhdx", "test snapshot").
		Return(expected, nil)

	snap, err := m.CreateSnapshot(context.Background(), "D:\\vhdx\\test-vol.vhdx", "test snapshot")
	assert.NoError(t, err)
	assert.Equal(t, expected, snap)

	m.AssertExpectations(t)
}

func TestMockBackend_DeleteSnapshot(t *testing.T) {
	m := new(MockBackend)
	m.On("DeleteSnapshot", mock.Anything, "snap-001").Return(nil)

	err := m.DeleteSnapshot(context.Background(), "snap-001")
	assert.NoError(t, err)

	m.AssertExpectations(t)
}

func TestMockBackend_ListSnapshots(t *testing.T) {
	m := new(MockBackend)
	snaps := []SnapshotInfo{
		{SnapshotID: "snap-001", OriginalPath: "D:\\vhdx\\test-vol.vhdx", SizeBytes: 1073741824},
	}
	m.On("ListSnapshots", mock.Anything, "D:\\vhdx\\test-vol.vhdx").
		Return(snaps, nil)

	result, err := m.ListSnapshots(context.Background(), "D:\\vhdx\\test-vol.vhdx")
	assert.NoError(t, err)
	assert.Equal(t, snaps, result)

	m.AssertExpectations(t)
}

func TestMockBackend_ExportSnapshotAsVirtualDisk(t *testing.T) {
	m := new(MockBackend)
	m.On("ExportSnapshotAsVirtualDisk", mock.Anything, "snap-001").
		Return("D:\\vhdx\\exported-snap.vhdx", nil)

	path, err := m.ExportSnapshotAsVirtualDisk(context.Background(), "snap-001")
	assert.NoError(t, err)
	assert.Equal(t, "D:\\vhdx\\exported-snap.vhdx", path)

	m.AssertExpectations(t)
}

func TestMockBackend_ResizeVirtualDisk(t *testing.T) {
	m := new(MockBackend)
	m.On("ResizeVirtualDisk", mock.Anything, "D:\\vhdx\\test-vol.vhdx", int64(2147483648)).
		Return(2147483648, nil)

	size, err := m.ResizeVirtualDisk(context.Background(), "D:\\vhdx\\test-vol.vhdx", 2147483648)
	assert.NoError(t, err)
	assert.Equal(t, int64(2147483648), size)

	m.AssertExpectations(t)
}

func TestMockBackend_GetVolumeInfo(t *testing.T) {
	m := new(MockBackend)
	info := VolumeInfo{VHDXPath: "D:\\vhdx\\test-vol.vhdx", SizeBytes: 1073741824, Targets: []string{"iqn.2024-01.com.example:test"}}
	m.On("GetVolumeInfo", mock.Anything, "D:\\vhdx\\test-vol.vhdx").
		Return(info, nil)

	result, err := m.GetVolumeInfo(context.Background(), "D:\\vhdx\\test-vol.vhdx")
	assert.NoError(t, err)
	assert.Equal(t, info, result)

	m.AssertExpectations(t)
}

func TestMockBackend_GetTargetInitiators(t *testing.T) {
	m := new(MockBackend)
	iqns := []string{"iqn.2024-01.com.example:node-001", "iqn.2024-01.com.example:node-002"}
	m.On("GetTargetInitiators", mock.Anything, "iqn.2024-01.com.example:test").
		Return(iqns, nil)

	result, err := m.GetTargetInitiators(context.Background(), "iqn.2024-01.com.example:test")
	assert.NoError(t, err)
	assert.Equal(t, iqns, result)

	m.AssertExpectations(t)
}

func TestMockBackend_AllowInitiator(t *testing.T) {
	m := new(MockBackend)
	m.On("AllowInitiator", mock.Anything, "iqn.2024-01.com.example:test", "iqn.2024-01.com.example:node-001").
		Return(nil)

	err := m.AllowInitiator(context.Background(), "iqn.2024-01.com.example:test", "iqn.2024-01.com.example:node-001")
	assert.NoError(t, err)

	m.AssertExpectations(t)
}

func TestMockBackend_DenyInitiator(t *testing.T) {
	m := new(MockBackend)
	m.On("DenyInitiator", mock.Anything, "iqn.2024-01.com.example:test", "iqn.2024-01.com.example:node-001").
		Return(nil)

	err := m.DenyInitiator(context.Background(), "iqn.2024-01.com.example:test", "iqn.2024-01.com.example:node-001")
	assert.NoError(t, err)

	m.AssertExpectations(t)
}

func TestMockBackend_GetDirectoryFreeCapacity(t *testing.T) {
	m := new(MockBackend)
	m.On("GetDirectoryFreeCapacity", mock.Anything, "D:\\vhdx").
		Return(int64(107374182400), nil)

	free, err := m.GetDirectoryFreeCapacity(context.Background(), "D:\\vhdx")
	assert.NoError(t, err)
	assert.Equal(t, int64(107374182400), free)

	m.AssertExpectations(t)
}

func TestMockBackend_ErrorReturn(t *testing.T) {
	m := new(MockBackend)
	expectedErr := fmt.Errorf("backend error")
	m.On("EnsureTarget", mock.Anything, "target-test", "iqn.2024-01.com.example:test").
		Return("", expectedErr)

	_, err := m.EnsureTarget(context.Background(), "target-test", "iqn.2024-01.com.example:test")
	assert.Equal(t, expectedErr, err)

	m.AssertExpectations(t)
}

func TestMockBackend_GetVolumeByName_NotFound(t *testing.T) {
	m := new(MockBackend)
	m.On("GetVolumeByName", mock.Anything, "nonexistent-vol", "D:\\vhdx").
		Return(false, "", 0, "", "", int32(-1), nil)

	exists, path, size, targetName, targetIQN, lun, err := m.GetVolumeByName(context.Background(), "nonexistent-vol", "D:\\vhdx")
	assert.NoError(t, err)
	assert.False(t, exists)
	assert.Equal(t, "", path)
	assert.Equal(t, int64(0), size)
	assert.Equal(t, "", targetName)
	assert.Equal(t, "", targetIQN)
	assert.Equal(t, int32(-1), lun)

	m.AssertExpectations(t)
}
