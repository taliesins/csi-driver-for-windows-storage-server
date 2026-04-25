//go:build integration

package iscsi

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	integrationInitialDiskSizeBytes int64 = 64 * 1024 * 1024
	integrationResizedDiskSizeBytes int64 = 128 * 1024 * 1024
)

func TestWinRMBackendIntegration_RunPowerShell(t *testing.T) {
	backend := newIntegrationWinRMBackend(t)
	backend.PSModuleImport = "$null = $true"

	ctx := newIntegrationWinRMContext(t, backend)
	var out struct {
		OK           bool   `json:"ok"`
		ComputerName string `json:"computerName"`
	}

	err := backend.runPS(ctx, `@{ ok=$true; computerName=$env:COMPUTERNAME }`, &out)
	require.NoError(t, err)
	assert.True(t, out.OK)
	assert.NotEmpty(t, out.ComputerName)
}

func TestWinRMBackendIntegration_GetDirectoryFreeCapacity(t *testing.T) {
	parentDir := strings.TrimSpace(os.Getenv("WINRM_TEST_PARENT_DIR"))
	if parentDir == "" {
		t.Skip("set WINRM_TEST_PARENT_DIR to run iSCSI backend integration tests")
	}

	backend := newIntegrationWinRMBackend(t)
	ctx := newIntegrationWinRMContext(t, backend)

	free, err := backend.GetDirectoryFreeCapacity(ctx, parentDir)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, free, int64(0))
}

func TestWinRMBackendIntegration_VirtualDiskLifecycle(t *testing.T) {
	backend := newIntegrationWinRMBackend(t)
	requireIntegrationIscsiTargetAccess(t, backend)
	parentDir := newIntegrationWinRMWorkspace(t, backend)
	ctx := newIntegrationWinRMLifecycleContext(t, backend)

	resourceName := integrationResourceName("disk")
	targetIQN := fmt.Sprintf("iqn.2024-01.com.example:%s", resourceName)
	initiatorOne := fmt.Sprintf("iqn.2024-01.com.example:%s-node-1", resourceName)
	initiatorTwo := fmt.Sprintf("iqn.2024-01.com.example:%s-node-2", resourceName)

	t.Cleanup(func() {
		cleanupIntegrationTarget(t, backend, targetIQN)
	})

	require.NoError(t, backend.EnsureTarget(ctx, targetIQN))

	vhdxPath, sizeBytes, err := backend.CreateVirtualDisk(ctx, resourceName, parentDir, integrationInitialDiskSizeBytes)
	require.NoError(t, err)
	require.NotEmpty(t, vhdxPath)
	assert.GreaterOrEqual(t, sizeBytes, integrationInitialDiskSizeBytes)

	t.Cleanup(func() {
		cleanupIntegrationVirtualDisk(t, backend, vhdxPath)
	})

	exists, gotPath, gotSize, gotTarget, gotLUN, err := backend.GetVolumeByName(ctx, resourceName, parentDir)
	require.NoError(t, err)
	assert.True(t, exists)
	assert.Equal(t, vhdxPath, gotPath)
	assert.GreaterOrEqual(t, gotSize, integrationInitialDiskSizeBytes)
	assert.Empty(t, gotTarget)
	assert.Equal(t, int32(-1), gotLUN)

	resizedBytes, err := backend.ResizeVirtualDisk(ctx, vhdxPath, integrationResizedDiskSizeBytes)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, resizedBytes, integrationResizedDiskSizeBytes)

	lun, err := backend.MapDiskToTarget(ctx, targetIQN, vhdxPath)
	require.NoError(t, err)
	assert.Equal(t, int32(0), lun)

	exists, gotPath, gotSize, gotTarget, gotLUN, err = backend.GetVolumeByName(ctx, resourceName, parentDir)
	require.NoError(t, err)
	assert.True(t, exists)
	assert.Equal(t, vhdxPath, gotPath)
	assert.GreaterOrEqual(t, gotSize, integrationResizedDiskSizeBytes)
	assert.Equal(t, targetIQN, gotTarget)
	assert.Equal(t, int32(0), gotLUN)

	info, err := backend.GetVolumeInfo(ctx, vhdxPath)
	require.NoError(t, err)
	assert.Equal(t, vhdxPath, info.VHDXPath)
	assert.GreaterOrEqual(t, info.SizeBytes, integrationResizedDiskSizeBytes)
	assert.Contains(t, info.Targets, targetIQN)
	lunPtr, ok := info.LUN.(*int32)
	require.True(t, ok)
	require.NotNil(t, lunPtr)
	assert.Equal(t, int32(0), *lunPtr)

	require.NoError(t, backend.AllowInitiator(ctx, targetIQN, initiatorOne))
	require.NoError(t, backend.AllowInitiator(ctx, targetIQN, initiatorTwo))

	initiators, err := backend.GetTargetInitiators(ctx, targetIQN)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{initiatorOne, initiatorTwo}, initiators)

	require.NoError(t, backend.DenyInitiator(ctx, targetIQN, initiatorOne))

	initiators, err = backend.GetTargetInitiators(ctx, targetIQN)
	require.NoError(t, err)
	assert.NotContains(t, initiators, initiatorOne)
	assert.Contains(t, initiators, initiatorTwo)

	require.NoError(t, backend.UnmapDiskFromTarget(ctx, targetIQN, vhdxPath))

	info, err = backend.GetVolumeInfo(ctx, vhdxPath)
	require.NoError(t, err)
	assert.NotContains(t, info.Targets, targetIQN)
	assert.Nil(t, info.LUN)

	require.NoError(t, backend.DeleteVirtualDisk(ctx, vhdxPath))

	exists, _, _, _, _, err = backend.GetVolumeByName(ctx, resourceName, parentDir)
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestWinRMBackendIntegration_SnapshotLifecycle(t *testing.T) {
	backend := newIntegrationWinRMBackend(t)
	requireIntegrationIscsiTargetAccess(t, backend)
	parentDir := newIntegrationWinRMWorkspace(t, backend)
	ctx := newIntegrationWinRMLifecycleContext(t, backend)

	resourceName := integrationResourceName("snapdisk")
	description := fmt.Sprintf("%s checkpoint", resourceName)

	vhdxPath, sizeBytes, err := backend.CreateVirtualDisk(ctx, resourceName, parentDir, integrationInitialDiskSizeBytes)
	require.NoError(t, err)
	require.NotEmpty(t, vhdxPath)
	assert.GreaterOrEqual(t, sizeBytes, integrationInitialDiskSizeBytes)

	t.Cleanup(func() {
		cleanupIntegrationVirtualDisk(t, backend, vhdxPath)
	})

	snapshot, err := backend.CreateSnapshot(ctx, vhdxPath, description)
	require.NoError(t, err)
	require.NotEmpty(t, snapshot.SnapshotID)
	assert.Equal(t, vhdxPath, snapshot.OriginalPath)
	assert.Equal(t, description, snapshot.Description)

	t.Cleanup(func() {
		cleanupIntegrationSnapshot(t, backend, snapshot.SnapshotID)
	})

	snapshots, err := backend.ListSnapshots(ctx, vhdxPath)
	require.NoError(t, err)
	requireIntegrationSnapshot(t, snapshots, snapshot.SnapshotID)

	exportedPath, err := backend.ExportSnapshotAsVirtualDisk(ctx, snapshot.SnapshotID)
	require.NoError(t, err)
	require.NotEmpty(t, exportedPath)

	t.Cleanup(func() {
		cleanupIntegrationVirtualDisk(t, backend, exportedPath)
	})

	exportedInfo, err := backend.GetVolumeInfo(ctx, exportedPath)
	require.NoError(t, err)
	assert.Equal(t, exportedPath, exportedInfo.VHDXPath)

	require.NoError(t, backend.DeleteSnapshot(ctx, snapshot.SnapshotID))

	snapshots, err = backend.ListSnapshots(ctx, vhdxPath)
	require.NoError(t, err)
	assertIntegrationSnapshotNotContains(t, snapshots, snapshot.SnapshotID)
}

func newIntegrationWinRMBackend(t *testing.T) *WinRMBackend {
	t.Helper()

	host := strings.TrimSpace(os.Getenv("WINRM_HOST"))
	user := firstNonEmptyEnv("WINRM_USER", "WINRM_USERNAME")
	pass := strings.TrimSpace(os.Getenv("WINRM_PASSWORD"))
	if host == "" || user == "" || pass == "" {
		t.Skip("set WINRM_HOST, WINRM_USER or WINRM_USERNAME, and WINRM_PASSWORD to run WinRM integration tests")
	}

	https := integrationBoolEnv(t, false, "WINRM_TLS", "WINRM_HTTPS")
	insecure := integrationBoolEnv(t, true, "WINRM_INSECURE")
	port := integrationPortEnv(t, https)
	timeout := integrationDurationEnv(t, 60*time.Second, "WINRM_TIMEOUT")

	backend := NewWinRMBackend(host, port, https, insecure, user, pass, timeout)
	backend.Auth = strings.TrimSpace(os.Getenv("WINRM_AUTH"))
	return backend
}

func newIntegrationWinRMContext(t *testing.T, backend *WinRMBackend) context.Context {
	return newIntegrationWinRMContextWithTimeout(t, backend, 0)
}

func newIntegrationWinRMLifecycleContext(t *testing.T, backend *WinRMBackend) context.Context {
	return newIntegrationWinRMContextWithTimeout(t, backend, 5*time.Minute)
}

func newIntegrationWinRMContextWithTimeout(t *testing.T, backend *WinRMBackend, minimum time.Duration) context.Context {
	t.Helper()

	timeout := backend.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	timeout += 15 * time.Second
	if timeout < minimum {
		timeout = minimum
	}
	timeout = integrationDurationEnv(t, timeout, "WINRM_TEST_TIMEOUT")
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	t.Cleanup(cancel)
	return ctx
}

func newIntegrationWinRMWorkspace(t *testing.T, backend *WinRMBackend) string {
	t.Helper()

	parentDir := strings.TrimSpace(os.Getenv("WINRM_TEST_PARENT_DIR"))
	if parentDir == "" {
		t.Skip("set WINRM_TEST_PARENT_DIR to run iSCSI backend integration tests")
	}

	ctx := newIntegrationWinRMContext(t, backend)
	var out struct {
		Path string `json:"path"`
	}
	err := backend.runPS(ctx, fmt.Sprintf(`
$dir = Join-Path -Path '%s' -ChildPath '%s'
New-Item -ItemType Directory -Path $dir -Force | Out-Null
@{ path=$dir }
`, escapePS(parentDir), escapePS(integrationResourceName("workspace"))), &out)
	require.NoError(t, err)
	require.NotEmpty(t, out.Path)

	t.Cleanup(func() {
		cleanupIntegrationDirectory(t, backend, out.Path)
	})
	return out.Path
}

func requireIntegrationIscsiTargetAccess(t *testing.T, backend *WinRMBackend) {
	t.Helper()

	ctx := newIntegrationWinRMContext(t, backend)
	var out struct {
		OK      bool   `json:"ok"`
		IsAdmin bool   `json:"isAdmin"`
		Message string `json:"message"`
		FQID    string `json:"fqid"`
	}
	err := backend.runPS(ctx, `
$principal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
$isAdmin = $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
try {
  Get-IscsiServerTarget -ComputerName $IscsiTargetComputerName -ErrorAction Stop | Select-Object -First 1 | Out-Null
  @{ ok=$true; isAdmin=$isAdmin; message=''; fqid='' }
} catch {
  @{ ok=$false; isAdmin=$isAdmin; message=$_.Exception.Message; fqid=$_.FullyQualifiedErrorId }
}
`, &out)
	require.NoError(t, err)
	if !out.OK {
		t.Skipf("remote WinRM session cannot access iSCSI Target cmdlets; isAdmin=%t fqid=%s message=%s", out.IsAdmin, out.FQID, out.Message)
	}
}

func integrationResourceName(prefix string) string {
	return fmt.Sprintf("csi-it-%s-%d", prefix, time.Now().UnixNano())
}

func requireIntegrationSnapshot(t *testing.T, snapshots []SnapshotInfo, snapshotID string) {
	t.Helper()

	for _, snapshot := range snapshots {
		if snapshot.SnapshotID == snapshotID {
			return
		}
	}
	require.Failf(t, "snapshot not found", "snapshot %q was not found in %#v", snapshotID, snapshots)
}

func assertIntegrationSnapshotNotContains(t *testing.T, snapshots []SnapshotInfo, snapshotID string) {
	t.Helper()

	for _, snapshot := range snapshots {
		assert.NotEqual(t, snapshotID, snapshot.SnapshotID)
	}
}

func cleanupIntegrationVirtualDisk(t *testing.T, backend *WinRMBackend, vhdxPath string) {
	t.Helper()

	if isIntegrationSnapshotExportPath(vhdxPath) {
		return
	}

	ctx, cancel := newIntegrationWinRMCleanupContext(backend)
	defer cancel()

	var out map[string]any
	err := backend.runPS(ctx, fmt.Sprintf(`
$vd = Get-IscsiVirtualDisk -ComputerName $IscsiTargetComputerName -Path '%s' -ErrorAction SilentlyContinue
if ($vd) {
  foreach ($target in @(Get-MappedIscsiTargetNames -Path '%s')) {
    Remove-IscsiVirtualDiskTargetMapping -ComputerName $IscsiTargetComputerName -TargetName $target -Path '%s' -ErrorAction SilentlyContinue
  }
  foreach ($snapshot in @(Get-IscsiVirtualDiskSnapshot -ComputerName $IscsiTargetComputerName -OriginalPath '%s' -ErrorAction SilentlyContinue)) {
    Remove-IscsiVirtualDiskSnapshot -ComputerName $IscsiTargetComputerName -SnapshotId $snapshot.SnapshotId -ErrorAction SilentlyContinue
  }
  Remove-IscsiVirtualDisk -ComputerName $IscsiTargetComputerName -Path '%s' -ErrorAction SilentlyContinue
}
if (Test-Path -LiteralPath '%s') {
  Remove-Item -LiteralPath '%s' -Force -ErrorAction SilentlyContinue
}
@{ ok=$true }
`, escapePS(vhdxPath), escapePS(vhdxPath), escapePS(vhdxPath), escapePS(vhdxPath), escapePS(vhdxPath), escapePS(vhdxPath), escapePS(vhdxPath)), &out)
	if err != nil {
		t.Logf("cleanup virtual disk %q: %v", vhdxPath, err)
	}
}

func isIntegrationSnapshotExportPath(path string) bool {
	path = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(path), "/", `\`))
	return strings.Contains(path, `globalroot\device\harddiskvolumeshadowcopy`)
}

func cleanupIntegrationSnapshot(t *testing.T, backend *WinRMBackend, snapshotID string) {
	t.Helper()

	ctx, cancel := newIntegrationWinRMCleanupContext(backend)
	defer cancel()

	if err := backend.DeleteSnapshot(ctx, snapshotID); err != nil {
		t.Logf("cleanup snapshot %q: %v", snapshotID, err)
	}
}

func cleanupIntegrationTarget(t *testing.T, backend *WinRMBackend, targetIQN string) {
	t.Helper()

	ctx, cancel := newIntegrationWinRMCleanupContext(backend)
	defer cancel()

	var out map[string]any
	err := backend.runPS(ctx, fmt.Sprintf(`
if (Get-IscsiServerTarget -ComputerName $IscsiTargetComputerName -TargetName '%s' -ErrorAction SilentlyContinue) {
  Remove-IscsiServerTarget -ComputerName $IscsiTargetComputerName -TargetName '%s' -ErrorAction SilentlyContinue | Out-Null
}
@{ ok=$true }
`, escapePS(targetIQN), escapePS(targetIQN)), &out)
	if err != nil {
		t.Logf("cleanup target %q: %v", targetIQN, err)
	}
}

func cleanupIntegrationDirectory(t *testing.T, backend *WinRMBackend, path string) {
	t.Helper()

	ctx, cancel := newIntegrationWinRMCleanupContext(backend)
	defer cancel()

	var out map[string]any
	err := backend.runPS(ctx, fmt.Sprintf(`
if (Test-Path -LiteralPath '%s') {
  Remove-Item -LiteralPath '%s' -Recurse -Force -ErrorAction SilentlyContinue
}
@{ ok=$true }
`, escapePS(path), escapePS(path)), &out)
	if err != nil {
		t.Logf("cleanup directory %q: %v", path, err)
	}
}

func newIntegrationWinRMCleanupContext(backend *WinRMBackend) (context.Context, context.CancelFunc) {
	timeout := backend.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return context.WithTimeout(context.Background(), timeout+15*time.Second)
}

func firstNonEmptyEnv(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

func integrationBoolEnv(t *testing.T, fallback bool, keys ...string) bool {
	t.Helper()

	for _, key := range keys {
		value := strings.TrimSpace(os.Getenv(key))
		if value == "" {
			continue
		}
		parsed, err := strconv.ParseBool(value)
		require.NoErrorf(t, err, "parse %s=%q as bool", key, value)
		return parsed
	}
	return fallback
}

func integrationPortEnv(t *testing.T, https bool) int {
	t.Helper()

	value := strings.TrimSpace(os.Getenv("WINRM_PORT"))
	if value == "" {
		if https {
			return 5986
		}
		return 5985
	}

	port, err := strconv.Atoi(value)
	require.NoErrorf(t, err, "parse WINRM_PORT=%q as int", value)
	require.Greater(t, port, 0)
	return port
}

func integrationDurationEnv(t *testing.T, fallback time.Duration, key string) time.Duration {
	t.Helper()

	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	duration, err := time.ParseDuration(value)
	require.NoErrorf(t, err, "parse %s=%q as duration", key, value)
	require.Greater(t, duration, time.Duration(0))
	return duration
}
