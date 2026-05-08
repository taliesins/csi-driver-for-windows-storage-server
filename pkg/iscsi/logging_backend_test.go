package iscsi

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSafeLogKVRedactsSensitiveValues(t *testing.T) {
	got := safeLogKV("targetName", "vol-a", "chapSecret", "super-secret", "password", "pw")

	assert.Contains(t, got, `targetName="vol-a"`)
	assert.Contains(t, got, "chapSecret=<redacted>")
	assert.Contains(t, got, "password=<redacted>")
	assert.NotContains(t, got, "super-secret")
	assert.NotContains(t, got, "pw")
}

func TestSanitizeMountOptionsForLogRedactsCredentials(t *testing.T) {
	got := sanitizeMountOptionsForLog([]string{
		"rw",
		"username=dbuser",
		"password=S3cret!",
		"credentials=/etc/cifs-creds",
	})

	assert.Equal(t, []string{
		"rw",
		"username=<redacted>",
		"password=<redacted>",
		"credentials=<redacted>",
	}, got)
}

func TestSanitizeMountOptionsForDebugLogKeepsUsername(t *testing.T) {
	got := sanitizeMountOptionsForDebugLog([]string{
		"rw",
		"username=dbuser",
		"password=S3cret!",
	})

	assert.Equal(t, []string{
		"rw",
		"username=dbuser",
		"password=<redacted>",
	}, got)
}

func TestLoggingBackendDelegatesBackendCalls(t *testing.T) {
	var calls []string
	inner := &mockBackend{
		ensureTargetFn: func(ctx context.Context, targetName, targetIQN string) (string, error) {
			calls = append(calls, "EnsureTarget:"+targetName+":"+targetIQN)
			return targetIQN, nil
		},
		createVirtualDiskFn: func(ctx context.Context, name, parentDir string, sizeBytes int64) (string, int64, error) {
			calls = append(calls, "CreateVirtualDisk:"+name)
			return parentDir + "\\" + name + ".vhdx", sizeBytes, nil
		},
		mapDiskToTargetFn: func(ctx context.Context, targetName, vhdxPath string) (int32, error) {
			calls = append(calls, "MapDiskToTarget:"+targetName)
			return 1, nil
		},
		deleteVirtualDiskFn: func(ctx context.Context, vhdxPath string) error {
			calls = append(calls, "DeleteVirtualDisk:"+vhdxPath)
			return nil
		},
		lookupTargetNameByIQNFn: func(ctx context.Context, targetIQN string) (string, error) {
			calls = append(calls, "LookupTargetNameByIQN:"+targetIQN)
			return "target-a", nil
		},
		createSnapshotFn: func(ctx context.Context, vhdxPath, desc string) (SnapshotInfo, error) {
			calls = append(calls, "CreateSnapshot:"+desc)
			return SnapshotInfo{SnapshotID: "snap-a", OriginalPath: vhdxPath, SizeBytes: 1024}, nil
		},
		exportSnapFn: func(ctx context.Context, snapshotID string) (string, error) {
			calls = append(calls, "ExportSnapshotAsVirtualDisk:"+snapshotID)
			return "D:\\exports\\snap-a.vhdx", nil
		},
		resizeVirtualDiskFn: func(ctx context.Context, vhdxPath string, newSizeBytes int64) (int64, error) {
			calls = append(calls, "ResizeVirtualDisk:"+vhdxPath)
			return newSizeBytes, nil
		},
		createSmbShareWithOptionsFn: func(ctx context.Context, name, parentDir string, sizeBytes int64, fullAccess, changeAccess, readAccess []string, opts ...SmbShareOptions) (VolumeInfo, error) {
			calls = append(calls, "CreateSmbShare:"+name)
			return VolumeInfo{SmbServer: "10.0.0.3", SmbShareName: name, SharePath: parentDir + "\\" + name, CapacityBytes: sizeBytes}, nil
		},
		createNfsShareWithOptionsFn: func(ctx context.Context, name, parentDir string, sizeBytes int64, clients []string, opts ...NfsShareOptions) (VolumeInfo, error) {
			calls = append(calls, "CreateNfsShare:"+name)
			return VolumeInfo{NfsServer: "10.0.0.2", NfsExportPath: "/" + name, SharePath: parentDir + "\\" + name, CapacityBytes: sizeBytes}, nil
		},
	}
	backend := wrapBackendForDebug(inner)

	iqn, err := backend.EnsureTarget(context.Background(), "target-a", "iqn.2024-01.com.example:target-a")
	require.NoError(t, err)
	assert.Equal(t, "iqn.2024-01.com.example:target-a", iqn)

	require.NoError(t, backend.ConfigureTargetChap(context.Background(), "target-a", TargetChapOptions{ChapUser: "user"}))
	vhdxPath, actualSize, err := backend.CreateVirtualDisk(context.Background(), "target-a", "D:\\vhdx", 1024)
	require.NoError(t, err)
	assert.Equal(t, "D:\\vhdx\\target-a.vhdx", vhdxPath)
	assert.Equal(t, int64(1024), actualSize)
	lun, err := backend.MapDiskToTarget(context.Background(), "target-a", vhdxPath)
	require.NoError(t, err)
	assert.Equal(t, int32(1), lun)
	require.NoError(t, backend.UnmapDiskFromTarget(context.Background(), "target-a", vhdxPath))
	require.NoError(t, backend.DeleteVirtualDisk(context.Background(), "D:\\vhdx\\target-a.vhdx"))
	targetName, err := backend.LookupTargetNameByIQN(context.Background(), "iqn.2024-01.com.example:target-a")
	require.NoError(t, err)
	assert.Equal(t, "target-a", targetName)
	require.NoError(t, backend.DeleteTarget(context.Background(), "target-a"))
	_, _, _, _, _, _, err = backend.GetVolumeByName(context.Background(), "target-a", "D:\\vhdx")
	require.NoError(t, err)
	require.NoError(t, backend.AllowInitiator(context.Background(), "target-a", "iqn.2004-10.com.ubuntu:01:test"))
	require.NoError(t, backend.DenyInitiator(context.Background(), "target-a", "iqn.2004-10.com.ubuntu:01:test"))
	_, err = backend.GetDirectoryFreeCapacity(context.Background(), "D:\\vhdx")
	require.NoError(t, err)

	snapshot, err := backend.CreateSnapshot(context.Background(), vhdxPath, "snap-a")
	require.NoError(t, err)
	assert.Equal(t, "snap-a", snapshot.SnapshotID)
	require.NoError(t, backend.DeleteSnapshot(context.Background(), "snap-a"))
	_, err = backend.ListSnapshots(context.Background(), vhdxPath)
	require.NoError(t, err)
	_, err = backend.ExportSnapshotAsVirtualDisk(context.Background(), "snap-a")
	require.NoError(t, err)
	_, err = backend.ResizeVirtualDisk(context.Background(), vhdxPath, 2048)
	require.NoError(t, err)
	_, err = backend.GetVolumeInfo(context.Background(), vhdxPath)
	require.NoError(t, err)
	_, err = backend.GetTargetInitiators(context.Background(), "target-a")
	require.NoError(t, err)

	nfsInfo, err := backend.CreateNfsShare(context.Background(), "share-a", "D:\\shares", 1024, []string{"10.0.0.10"})
	require.NoError(t, err)
	assert.Equal(t, "/share-a", nfsInfo.NfsExportPath)
	_, _, err = backend.GetNfsShare(context.Background(), "share-a", "D:\\shares")
	require.NoError(t, err)
	require.NoError(t, backend.DeleteNfsShare(context.Background(), "share-a", "D:\\shares\\share-a"))

	info, err := backend.CreateSmbShare(context.Background(), "share-a", "D:\\shares", 1024, []string{"Everyone"}, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, "share-a", info.SmbShareName)
	_, _, err = backend.GetSmbShare(context.Background(), "share-a", "D:\\shares")
	require.NoError(t, err)
	require.NoError(t, backend.DeleteSmbShare(context.Background(), "share-a", "D:\\shares\\share-a"))
	_, err = backend.ResizeFileShare(context.Background(), "D:\\shares\\share-a", 2048)
	require.NoError(t, err)
	require.NoError(t, backend.RestoreSnapshotAsFileShare(context.Background(), "snap-a", "D:\\shares\\share-a"))
	require.NoError(t, backend.MountFileShareVirtualDisk(context.Background(), vhdxPath, "D:\\shares\\share-a"))
	require.NoError(t, backend.UnmountFileShareVirtualDisk(context.Background(), vhdxPath, "D:\\shares\\share-a"))

	assert.Contains(t, calls, "EnsureTarget:target-a:iqn.2024-01.com.example:target-a")
	assert.Contains(t, calls, "CreateVirtualDisk:target-a")
	assert.Contains(t, calls, "MapDiskToTarget:target-a")
	assert.Contains(t, calls, "DeleteVirtualDisk:D:\\vhdx\\target-a.vhdx")
	assert.Contains(t, calls, "LookupTargetNameByIQN:iqn.2024-01.com.example:target-a")
	assert.Contains(t, calls, "CreateSnapshot:snap-a")
	assert.Contains(t, calls, "ExportSnapshotAsVirtualDisk:snap-a")
	assert.Contains(t, calls, "ResizeVirtualDisk:D:\\vhdx\\target-a.vhdx")
	assert.Contains(t, calls, "CreateNfsShare:share-a")
	assert.Contains(t, calls, "CreateSmbShare:share-a")
}
