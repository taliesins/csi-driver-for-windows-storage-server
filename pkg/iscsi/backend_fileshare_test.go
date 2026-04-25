package iscsi

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWinRMBackend_CreateNfsShare(t *testing.T) {
	backend := newUnitWinRMBackend()
	calls := 0
	backend.psRunner = func(ctx context.Context, script string, out any) error {
		calls++
		if calls == 1 {
			assert.Contains(t, script, "Import-Module NFS")
			assert.Contains(t, script, "New-NfsShare")
			assert.Contains(t, script, "Grant-NfsSharePermission")
			assert.Contains(t, script, "csi-nfs-test")
			assert.Contains(t, script, "D:\\shares")
			copyTestOutput(out, struct {
				Name          string `json:"name"`
				NfsServer     string `json:"nfsServer"`
				NfsExportPath string `json:"nfsExportPath"`
				VHDXPath      string `json:"vhdxPath"`
				SizeBytes     int64  `json:"sizeBytes"`
				CapacityBytes int64  `json:"capacityBytes"`
			}{
				Name:          "csi-nfs-test",
				NfsServer:     "server01",
				NfsExportPath: "/csi-nfs-test",
				VHDXPath:      "D:\\shares\\csi-nfs-test",
				SizeBytes:     0,
				CapacityBytes: 1024,
			})
		} else {
			assert.Contains(t, script, ".csi-volume.json")
			copyTestOutput(out, struct {
				CapacityBytes int64 `json:"capacityBytes"`
			}{CapacityBytes: 1024})
		}
		return nil
	}

	info, err := backend.CreateNfsShare(context.Background(), "csi-nfs-test", "D:\\shares", 1024, []string{"10.0.0.10"})
	require.NoError(t, err)
	assert.Equal(t, ProtocolNFS, info.Protocol)
	assert.Equal(t, "server01", info.NfsServer)
	assert.Equal(t, "/csi-nfs-test", info.NfsExportPath)
}

func TestWinRMBackend_CreateNfsShareWithOptions(t *testing.T) {
	backend := newUnitWinRMBackend()
	allowRoot := false
	enableAnonymous := true
	enableUnmapped := true
	uid := 65534
	gid := 65534
	calls := 0
	backend.psRunner = func(ctx context.Context, script string, out any) error {
		calls++
		if calls == 1 {
			assert.Contains(t, script, "$params.Authentication = @('sys','krb5')")
			assert.Contains(t, script, "$params.AnonymousUid = 65534")
			assert.Contains(t, script, "$params.AnonymousGid = 65534")
			assert.Contains(t, script, "$params.LanguageEncoding = 'EUC-JP'")
			assert.Contains(t, script, "$params.EnableAnonymousAccess = $true")
			assert.Contains(t, script, "$params.EnableUnmappedAccess = $true")
			assert.Contains(t, script, "-ClientType 'clientgroup'")
			assert.Contains(t, script, "-Permission 'readonly'")
			assert.Contains(t, script, "-AllowRootAccess $false")
			copyTestOutput(out, struct {
				Name          string `json:"name"`
				NfsServer     string `json:"nfsServer"`
				NfsExportPath string `json:"nfsExportPath"`
				VHDXPath      string `json:"vhdxPath"`
				SizeBytes     int64  `json:"sizeBytes"`
				CapacityBytes int64  `json:"capacityBytes"`
			}{
				Name:          "csi-nfs-options",
				NfsServer:     "server01",
				NfsExportPath: "/csi-nfs-options",
				VHDXPath:      "D:\\shares\\csi-nfs-options",
				SizeBytes:     0,
				CapacityBytes: 4096,
			})
		} else {
			assert.Contains(t, script, ".csi-volume.json")
			copyTestOutput(out, struct {
				CapacityBytes int64 `json:"capacityBytes"`
			}{CapacityBytes: 4096})
		}
		return nil
	}

	info, err := backend.CreateNfsShare(context.Background(), "csi-nfs-options", "D:\\shares", 4096, []string{"nfs-clients"}, NfsShareOptions{
		ClientType:            "clientgroup",
		Permission:            "readonly",
		AllowRootAccess:       &allowRoot,
		Authentication:        []string{"sys", "krb5"},
		AnonymousUID:          &uid,
		AnonymousGID:          &gid,
		LanguageEncoding:      "EUC-JP",
		EnableAnonymousAccess: &enableAnonymous,
		EnableUnmappedAccess:  &enableUnmapped,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(4096), info.CapacityBytes)
}

func TestWinRMBackend_NfsShareError(t *testing.T) {
	backend := newUnitWinRMBackend()
	backend.psRunner = func(ctx context.Context, script string, out any) error {
		return errors.New("nfs module unavailable")
	}

	_, err := backend.CreateNfsShare(context.Background(), "bad", "D:\\shares", 1024, nil)
	require.Error(t, err)
}

func TestWinRMBackend_DeleteNfsShare(t *testing.T) {
	backend := newUnitWinRMBackend()
	backend.psRunner = func(ctx context.Context, script string, out any) error {
		assert.Contains(t, script, "Remove-NfsShare")
		assert.Contains(t, script, "-Confirm:$false")
		assert.NotContains(t, script, "Remove-NfsShare -Name $name -Force")
		assert.Contains(t, script, "Remove-Item")
		return nil
	}

	require.NoError(t, backend.DeleteNfsShare(context.Background(), "csi-nfs-test", "D:\\shares\\csi-nfs-test"))
}

func TestWinRMBackend_CreateSmbShare(t *testing.T) {
	backend := newUnitWinRMBackend()
	calls := 0
	backend.psRunner = func(ctx context.Context, script string, out any) error {
		calls++
		if calls == 1 {
			assert.Contains(t, script, "Import-Module SmbShare")
			assert.Contains(t, script, "New-SmbShare")
			assert.Contains(t, script, "csi-smb-test")
			assert.Contains(t, script, "DOMAIN\\storage")
			copyTestOutput(out, struct {
				Name          string `json:"name"`
				SmbServer     string `json:"smbServer"`
				SmbShareName  string `json:"smbShareName"`
				VHDXPath      string `json:"vhdxPath"`
				SizeBytes     int64  `json:"sizeBytes"`
				CapacityBytes int64  `json:"capacityBytes"`
			}{
				Name:          "csi-smb-test",
				SmbServer:     "server01",
				SmbShareName:  "csi-smb-test",
				VHDXPath:      "D:\\shares\\csi-smb-test",
				SizeBytes:     0,
				CapacityBytes: 2048,
			})
		} else {
			assert.Contains(t, script, ".csi-volume.json")
			copyTestOutput(out, struct {
				CapacityBytes int64 `json:"capacityBytes"`
			}{CapacityBytes: 2048})
		}
		return nil
	}

	info, err := backend.CreateSmbShare(context.Background(), "csi-smb-test", "D:\\shares", 2048, []string{"DOMAIN\\storage"}, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, ProtocolSMB, info.Protocol)
	assert.Equal(t, "server01", info.SmbServer)
	assert.Equal(t, "csi-smb-test", info.SmbShareName)
}

func TestWinRMBackend_CreateSmbShareWithOptions(t *testing.T) {
	backend := newUnitWinRMBackend()
	encrypt := true
	compress := true
	continuouslyAvailable := false
	calls := 0
	backend.psRunner = func(ctx context.Context, script string, out any) error {
		calls++
		if calls == 1 {
			assert.Contains(t, script, "$params.NoAccess = @('DOMAIN\\blocked')")
			assert.Contains(t, script, "$params.Description = 'CSI managed share'")
			assert.Contains(t, script, "$params.EncryptData = $true")
			assert.Contains(t, script, "$params.CompressData = $true")
			assert.Contains(t, script, "$params.ContinuouslyAvailable = $false")
			assert.Contains(t, script, "$params.CachingMode = 'None'")
			assert.Contains(t, script, "$params.FolderEnumerationMode = 'AccessBased'")
			assert.Contains(t, script, "$params.ConcurrentUserLimit = 128")
			copyTestOutput(out, struct {
				Name          string `json:"name"`
				SmbServer     string `json:"smbServer"`
				SmbShareName  string `json:"smbShareName"`
				VHDXPath      string `json:"vhdxPath"`
				SizeBytes     int64  `json:"sizeBytes"`
				CapacityBytes int64  `json:"capacityBytes"`
			}{
				Name:          "csi-smb-options",
				SmbServer:     "server01",
				SmbShareName:  "csi-smb-options",
				VHDXPath:      "D:\\shares\\csi-smb-options",
				SizeBytes:     0,
				CapacityBytes: 8192,
			})
		} else {
			assert.Contains(t, script, ".csi-volume.json")
			copyTestOutput(out, struct {
				CapacityBytes int64 `json:"capacityBytes"`
			}{CapacityBytes: 8192})
		}
		return nil
	}

	info, err := backend.CreateSmbShare(context.Background(), "csi-smb-options", "D:\\shares", 8192, []string{"DOMAIN\\storage"}, nil, nil, SmbShareOptions{
		NoAccess:              []string{"DOMAIN\\blocked"},
		Description:           "CSI managed share",
		EncryptData:           &encrypt,
		CompressData:          &compress,
		ContinuouslyAvailable: &continuouslyAvailable,
		CachingMode:           "None",
		FolderEnumerationMode: "AccessBased",
		ConcurrentUserLimit:   128,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(8192), info.CapacityBytes)
}

func TestWinRMBackend_DeleteSmbShare(t *testing.T) {
	backend := newUnitWinRMBackend()
	backend.psRunner = func(ctx context.Context, script string, out any) error {
		assert.Contains(t, script, "Remove-SmbShare")
		assert.Contains(t, script, "Remove-Item")
		return nil
	}

	require.NoError(t, backend.DeleteSmbShare(context.Background(), "csi-smb-test", "D:\\shares\\csi-smb-test"))
}

func TestWinRMBackend_ResizeFileShare(t *testing.T) {
	backend := newUnitWinRMBackend()
	backend.psRunner = func(ctx context.Context, script string, out any) error {
		assert.Contains(t, script, ".csi-volume.json")
		assert.Contains(t, script, "D:\\shares\\csi-resize")
		assert.Contains(t, script, "4096")
		copyTestOutput(out, struct {
			CapacityBytes int64 `json:"capacityBytes"`
		}{CapacityBytes: 4096})
		return nil
	}

	actual, err := backend.ResizeFileShare(context.Background(), "D:\\shares\\csi-resize", 4096)
	require.NoError(t, err)
	assert.Equal(t, int64(4096), actual)
}

func TestWinRMBackend_RestoreSnapshotAsFileShare(t *testing.T) {
	backend := newUnitWinRMBackend()
	backend.psRunner = func(ctx context.Context, script string, out any) error {
		assert.Contains(t, script, "Copy-CsiDirectoryMirror")
		assert.Contains(t, script, "D:\\shares\\.csi-snapshots\\snap-001")
		assert.Contains(t, script, "D:\\shares\\restore-target")
		return nil
	}

	require.NoError(t, backend.RestoreSnapshotAsFileShare(context.Background(), "D:\\shares\\.csi-snapshots\\snap-001", "D:\\shares\\restore-target"))
}
