//go:build integration

package iscsi

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWinRMBackendIntegration_NfsShareLifecycle(t *testing.T) {
	parentDir := strings.TrimSpace(firstNonEmptyEnv("WINRM_TEST_SHARE_DIR", "WINRM_TEST_PARENT_DIR"))
	if parentDir == "" {
		t.Skip("set WINRM_TEST_SHARE_DIR or WINRM_TEST_PARENT_DIR to run NFS share integration tests")
	}

	backend := newIntegrationWinRMBackend(t)
	ctx := newIntegrationWinRMLifecycleContext(t, backend)
	name := integrationResourceName("nfs")

	info, err := backend.CreateNfsShare(ctx, name, parentDir, integrationInitialDiskSizeBytes, []string{"*"})
	if err != nil && strings.Contains(err.Error(), "not recognized") {
		t.Skipf("NFS PowerShell module is unavailable: %v", err)
	}
	require.NoError(t, err)
	assert.Equal(t, ProtocolNFS, info.Protocol)
	assert.Equal(t, "/"+name, info.NfsExportPath)

	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), backend.Timeout+15*time.Second)
		defer cancel()
		if err := backend.DeleteNfsShare(cleanupCtx, name, info.VHDXPath); err != nil {
			t.Logf("cleanup NFS share %q: %v", name, err)
		}
	})

	exists, got, err := backend.GetNfsShare(ctx, name, parentDir)
	require.NoError(t, err)
	assert.True(t, exists)
	assert.Equal(t, name, got.VolumeName)

	exerciseFileShareDataServices(t, backend, ctx, info.VHDXPath, integrationInitialDiskSizeBytes*2)
}

func TestWinRMBackendIntegration_SmbShareLifecycle(t *testing.T) {
	parentDir := strings.TrimSpace(firstNonEmptyEnv("WINRM_TEST_SHARE_DIR", "WINRM_TEST_PARENT_DIR"))
	if parentDir == "" {
		t.Skip("set WINRM_TEST_SHARE_DIR or WINRM_TEST_PARENT_DIR to run SMB share integration tests")
	}

	backend := newIntegrationWinRMBackend(t)
	ctx := newIntegrationWinRMLifecycleContext(t, backend)
	name := integrationResourceName("smb")
	fullAccess := splitCSVParam(os.Getenv("WINRM_TEST_SMB_FULL_ACCESS"))

	info, err := backend.CreateSmbShare(ctx, name, parentDir, integrationInitialDiskSizeBytes, fullAccess, nil, nil)
	if err != nil && strings.Contains(err.Error(), "not recognized") {
		t.Skipf("SMB PowerShell module is unavailable: %v", err)
	}
	require.NoError(t, err)
	assert.Equal(t, ProtocolSMB, info.Protocol)
	assert.Equal(t, name, info.SmbShareName)

	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), backend.Timeout+15*time.Second)
		defer cancel()
		if err := backend.DeleteSmbShare(cleanupCtx, name, info.VHDXPath); err != nil {
			t.Logf("cleanup SMB share %q: %v", name, err)
		}
	})

	exists, got, err := backend.GetSmbShare(ctx, name, parentDir)
	require.NoError(t, err)
	assert.True(t, exists)
	assert.Equal(t, name, got.VolumeName)

	exerciseFileShareDataServices(t, backend, ctx, info.VHDXPath, integrationInitialDiskSizeBytes*2)
}

func exerciseFileShareDataServices(t *testing.T, backend *WinRMBackend, ctx context.Context, path string, resizedBytes int64) {
	t.Helper()

	actual, err := backend.ResizeFileShare(ctx, path, resizedBytes)
	require.NoError(t, err)
	assert.Equal(t, resizedBytes, actual)

	var out map[string]any
	require.NoError(t, backend.runPS(ctx, fmt.Sprintf(`
New-Item -ItemType Directory -Path '%s' -Force | Out-Null
Set-Content -LiteralPath (Join-Path -Path '%s' -ChildPath 'payload.txt') -Value 'payload'
@{ ok=$true }
`, escapePS(path), escapePS(path)), &out))

	snap, err := backend.CreateSnapshot(ctx, path, integrationResourceName("fileshare-snap"))
	require.NoError(t, err)
	assert.Equal(t, path, snap.OriginalPath)
	require.NotEmpty(t, snap.SnapshotID)

	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), backend.Timeout+15*time.Second)
		defer cancel()
		if err := backend.DeleteSnapshot(cleanupCtx, snap.SnapshotID); err != nil {
			t.Logf("cleanup file-share snapshot %q: %v", snap.SnapshotID, err)
		}
	})

	snaps, err := backend.ListSnapshots(ctx, path)
	require.NoError(t, err)
	assert.NotEmpty(t, snaps)

	restorePath := path + "-restore"
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), backend.Timeout+15*time.Second)
		defer cancel()
		var cleanupOut map[string]any
		if err := backend.runPS(cleanupCtx, fmt.Sprintf(`
if (Test-Path -LiteralPath '%s') { Remove-Item -LiteralPath '%s' -Recurse -Force -ErrorAction SilentlyContinue }
@{ ok=$true }
`, escapePS(restorePath), escapePS(restorePath)), &cleanupOut); err != nil {
			t.Logf("cleanup restored file share path %q: %v", restorePath, err)
		}
	})

	require.NoError(t, backend.RestoreSnapshotAsFileShare(ctx, snap.SnapshotID, restorePath))
	require.NoError(t, backend.runPS(ctx, fmt.Sprintf(`
$exists = Test-Path -LiteralPath (Join-Path -Path '%s' -ChildPath 'payload.txt')
@{ exists=$exists }
`, escapePS(restorePath)), &out))
	assert.Equal(t, true, out["exists"])
}
