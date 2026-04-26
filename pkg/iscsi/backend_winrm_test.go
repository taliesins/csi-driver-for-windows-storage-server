package iscsi

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/masterzen/winrm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// WinRMBackend tests
// ---------------------------------------------------------------------------

func TestNewWinRMBackend_Defaults(t *testing.T) {
	b := NewWinRMBackend("10.0.0.1", 5985, false, true, "admin", "pass", 0)

	assert.Equal(t, "10.0.0.1", b.Endpoint.Host)
	assert.Equal(t, 5985, b.Endpoint.Port)
	assert.False(t, b.Endpoint.HTTPS)
	assert.True(t, b.Endpoint.Insecure)
	assert.Equal(t, "admin", b.User)
	assert.Equal(t, "pass", b.Pass)
	assert.Equal(t, "basic", b.Auth)
	assert.Equal(t, 60*time.Second, b.Timeout)
	assert.Equal(t, "Import-Module IscsiTarget", b.PSModuleImport)
}

func TestNewWinRMBackend_FullConfig(t *testing.T) {
	b := NewWinRMBackend("192.168.1.100", 5986, true, false, "svc", "secret", 120*time.Second)

	require.NotNil(t, b.Endpoint)
	assert.Equal(t, "192.168.1.100", b.Endpoint.Host)
	assert.Equal(t, 5986, b.Endpoint.Port)
	assert.True(t, b.Endpoint.HTTPS)
	assert.False(t, b.Endpoint.Insecure)
	assert.Equal(t, 120*time.Second, b.Timeout)
}

func TestWinRMBackend_ClientParametersAuth(t *testing.T) {
	tests := []struct {
		name    string
		auth    string
		wantErr bool
	}{
		{name: "default", auth: "", wantErr: false},
		{name: "basic", auth: "basic", wantErr: false},
		{name: "ntlm", auth: "ntlm", wantErr: false},
		{name: "negotiate alias", auth: "negotiate", wantErr: false},
		{name: "invalid", auth: "kerberos", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewWinRMBackend("10.0.0.1", 5985, false, true, "admin", "pass", 0)
			b.Auth = tt.auth

			params, err := b.clientParameters()
			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, params)
		})
	}
}

type fakeWinRMClient struct {
	stdout  string
	stderr  string
	code    int
	err     error
	command string
	stdin   string
	started chan struct{}
	release chan struct{}
}

func (f *fakeWinRMClient) RunPSWithContextWithString(ctx context.Context, command string, stdin string) (string, string, int, error) {
	f.command = command
	f.stdin = stdin
	if f.started != nil {
		close(f.started)
	}
	if f.release != nil {
		<-f.release
	}
	return f.stdout, f.stderr, f.code, f.err
}

func TestWinRMBackend_RunPS(t *testing.T) {
	t.Run("decodes JSON and wraps script", func(t *testing.T) {
		backend := NewWinRMBackend("10.0.0.1", 5985, false, true, "admin", "pass", 0)
		backend.PSModuleImport = ""
		fakeClient := &fakeWinRMClient{stdout: `{"ok":true}`}
		originalNewClient := newWinRMClientWithParameters
		newWinRMClientWithParameters = func(endpoint *winrm.Endpoint, user, password string, params *winrm.Parameters) (winRMClient, error) {
			assert.Equal(t, backend.Endpoint, endpoint)
			assert.Equal(t, "admin", user)
			assert.Equal(t, "pass", password)
			require.NotNil(t, params)
			return fakeClient, nil
		}
		t.Cleanup(func() { newWinRMClientWithParameters = originalNewClient })

		var out struct {
			OK bool `json:"ok"`
		}
		err := backend.runPS(context.Background(), "@{ ok = $true }", &out)
		require.NoError(t, err)
		assert.True(t, out.OK)
		assert.Equal(t, "Invoke-Expression ([Console]::In.ReadToEnd())", fakeClient.command)
		assert.Contains(t, fakeClient.stdin, "Import-Module IscsiTarget")
		assert.Contains(t, fakeClient.stdin, "@{ ok = $true }")
		assert.Contains(t, fakeClient.stdin, "ConvertTo-Json")
		assert.Equal(t, "Import-Module IscsiTarget", backend.PSModuleImport)
	})

	t.Run("nil output allows empty stdout", func(t *testing.T) {
		backend := NewWinRMBackend("10.0.0.1", 5985, false, true, "admin", "pass", 0)
		backend.PSModuleImport = "Import-Module Custom"
		fakeClient := &fakeWinRMClient{}
		originalNewClient := newWinRMClientWithParameters
		newWinRMClientWithParameters = func(endpoint *winrm.Endpoint, user, password string, params *winrm.Parameters) (winRMClient, error) {
			return fakeClient, nil
		}
		t.Cleanup(func() { newWinRMClientWithParameters = originalNewClient })

		require.NoError(t, backend.runPS(context.Background(), "Do-Something", nil))
		assert.Contains(t, fakeClient.stdin, "Import-Module Custom")
	})

	t.Run("client construction error", func(t *testing.T) {
		backend := NewWinRMBackend("10.0.0.1", 5985, false, true, "admin", "pass", 0)
		originalNewClient := newWinRMClientWithParameters
		newWinRMClientWithParameters = func(endpoint *winrm.Endpoint, user, password string, params *winrm.Parameters) (winRMClient, error) {
			return nil, errors.New("dial failed")
		}
		t.Cleanup(func() { newWinRMClientWithParameters = originalNewClient })

		err := backend.runPS(context.Background(), "Do-Something", nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "winrm.NewClient")
		assert.Contains(t, err.Error(), "dial failed")
	})

	t.Run("run error includes stderr", func(t *testing.T) {
		backend := NewWinRMBackend("10.0.0.1", 5985, false, true, "admin", "pass", 0)
		fakeClient := &fakeWinRMClient{stderr: "remote stderr", err: errors.New("transport failed")}
		originalNewClient := newWinRMClientWithParameters
		newWinRMClientWithParameters = func(endpoint *winrm.Endpoint, user, password string, params *winrm.Parameters) (winRMClient, error) {
			return fakeClient, nil
		}
		t.Cleanup(func() { newWinRMClientWithParameters = originalNewClient })

		err := backend.runPS(context.Background(), "Do-Something", nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "transport failed")
		assert.Contains(t, err.Error(), "remote stderr")
	})

	t.Run("nonzero exit includes stderr", func(t *testing.T) {
		backend := NewWinRMBackend("10.0.0.1", 5985, false, true, "admin", "pass", 0)
		fakeClient := &fakeWinRMClient{stderr: "script failed", code: 1}
		originalNewClient := newWinRMClientWithParameters
		newWinRMClientWithParameters = func(endpoint *winrm.Endpoint, user, password string, params *winrm.Parameters) (winRMClient, error) {
			return fakeClient, nil
		}
		t.Cleanup(func() { newWinRMClientWithParameters = originalNewClient })

		err := backend.runPS(context.Background(), "Do-Something", nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exit code 1")
		assert.Contains(t, err.Error(), "script failed")
	})

	t.Run("invalid JSON", func(t *testing.T) {
		backend := NewWinRMBackend("10.0.0.1", 5985, false, true, "admin", "pass", 0)
		fakeClient := &fakeWinRMClient{stdout: "not json"}
		originalNewClient := newWinRMClientWithParameters
		newWinRMClientWithParameters = func(endpoint *winrm.Endpoint, user, password string, params *winrm.Parameters) (winRMClient, error) {
			return fakeClient, nil
		}
		t.Cleanup(func() { newWinRMClientWithParameters = originalNewClient })

		var out map[string]any
		err := backend.runPS(context.Background(), "Do-Something", &out)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "json unmarshal failed")
		assert.Contains(t, err.Error(), "not json")
	})

	t.Run("empty stdout when output expected", func(t *testing.T) {
		backend := NewWinRMBackend("10.0.0.1", 5985, false, true, "admin", "pass", 0)
		fakeClient := &fakeWinRMClient{stderr: "nothing returned"}
		originalNewClient := newWinRMClientWithParameters
		newWinRMClientWithParameters = func(endpoint *winrm.Endpoint, user, password string, params *winrm.Parameters) (winRMClient, error) {
			return fakeClient, nil
		}
		t.Cleanup(func() { newWinRMClientWithParameters = originalNewClient })

		var out map[string]any
		err := backend.runPS(context.Background(), "Do-Something", &out)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "expected JSON output")
		assert.Contains(t, err.Error(), "nothing returned")
	})

	t.Run("context canceled while command is running", func(t *testing.T) {
		backend := NewWinRMBackend("10.0.0.1", 5985, false, true, "admin", "pass", 0)
		started := make(chan struct{})
		release := make(chan struct{})
		fakeClient := &fakeWinRMClient{started: started, release: release}
		originalNewClient := newWinRMClientWithParameters
		newWinRMClientWithParameters = func(endpoint *winrm.Endpoint, user, password string, params *winrm.Parameters) (winRMClient, error) {
			return fakeClient, nil
		}
		t.Cleanup(func() { newWinRMClientWithParameters = originalNewClient })

		ctx, cancel := context.WithCancel(context.Background())
		errCh := make(chan error, 1)
		go func() {
			errCh <- backend.runPS(ctx, "Do-Something", nil)
		}()
		<-started
		cancel()
		err := <-errCh
		close(release)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "context canceled")
	})
}

func newUnitWinRMBackend() *WinRMBackend {
	return &WinRMBackend{
		Endpoint: &Endpoint{Host: "10.0.0.1", Port: 5985},
		User:     "admin",
		Pass:     "pass",
	}
}

// ---------------------------------------------------------------------------
// EnsureTarget tests
// ---------------------------------------------------------------------------

func TestWinRMBackend_EnsureTarget(t *testing.T) {
	tests := []struct {
		name      string
		targetIQN string
		psOut     any
		psErr     error
		wantErr   bool
	}{
		{
			name:      "happy path - new target",
			targetIQN: "iqn.2024-01.com.example:new-target",
			psOut:     map[string]any{"ok": true},
			psErr:     nil,
			wantErr:   false,
		},
		{
			name:      "happy path - target already exists",
			targetIQN: "iqn.2024-01.com.example:existing-target",
			psOut:     map[string]any{"ok": true},
			psErr:     nil,
			wantErr:   false,
		},
		{
			name:      "error from PowerShell",
			targetIQN: "iqn.2024-01.com.example:error-target",
			psOut:     nil,
			psErr:     errors.New("storage error"),
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := newUnitWinRMBackend()
			backend.psRunner = func(ctx context.Context, script string, out any) error {
				// Verify the script contains the target IQN
				assert.Contains(t, script, tt.targetIQN)
				if out != nil {
					copyTestOutput(out, tt.psOut)
				}
				return tt.psErr
			}

			err := backend.EnsureTarget(context.Background(), tt.targetIQN)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// CreateVirtualDisk tests
// ---------------------------------------------------------------------------

func TestWinRMBackend_CreateVirtualDisk(t *testing.T) {
	tests := []struct {
		name      string
		volName   string
		parentDir string
		sizeBytes int64
		psOut     struct {
			Path      string
			SizeBytes int64
		}
		psErr    error
		wantErr  bool
		wantPath string
		wantSize int64
	}{
		{
			name:      "happy path",
			volName:   "k8s-csi-test-volume-001",
			parentDir: "D:\\vhdx",
			sizeBytes: 1073741824,
			psOut: struct {
				Path      string
				SizeBytes int64
			}{Path: "D:\\vhdx\\k8s-csi-test-volume-001.vhdx", SizeBytes: 1073741824},
			psErr:    nil,
			wantErr:  false,
			wantPath: "D:\\vhdx\\k8s-csi-test-volume-001.vhdx",
			wantSize: 1073741824,
		},
		{
			name:      "error from PowerShell",
			volName:   "csi-vol-002",
			parentDir: "E:\\storage",
			sizeBytes: 5368709120,
			psOut: struct {
				Path      string
				SizeBytes int64
			}{},
			psErr:   errors.New("disk creation failed"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := newUnitWinRMBackend()
			backend.psRunner = func(ctx context.Context, script string, out any) error {
				assert.Contains(t, script, tt.volName)
				assert.Contains(t, script, tt.parentDir)
				assert.Contains(t, script, fmt.Sprintf("%d", tt.sizeBytes))
				if out != nil {
					copyTestOutput(out, tt.psOut)
				}
				return tt.psErr
			}

			path, size, err := backend.CreateVirtualDisk(context.Background(), tt.volName, tt.parentDir, tt.sizeBytes)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantPath, path)
				assert.Equal(t, tt.wantSize, size)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// MapDiskToTarget tests
// ---------------------------------------------------------------------------

func TestWinRMBackend_MapDiskToTarget(t *testing.T) {
	tests := []struct {
		name      string
		targetIQN string
		vhdxPath  string
		psOut     struct{ LUN int32 }
		psErr     error
		wantErr   bool
		wantLUN   int32
	}{
		{
			name:      "happy path - new mapping",
			targetIQN: "iqn.2024-01.com.example:vol001",
			vhdxPath:  "D:\\vhdx\\vol001.vhdx",
			psOut:     struct{ LUN int32 }{LUN: 0},
			psErr:     nil,
			wantErr:   false,
			wantLUN:   0,
		},
		{
			name:      "happy path - already mapped",
			targetIQN: "iqn.2024-01.com.example:vol002",
			vhdxPath:  "D:\\vhdx\\vol002.vhdx",
			psOut:     struct{ LUN int32 }{LUN: 0},
			psErr:     nil,
			wantErr:   false,
			wantLUN:   0,
		},
		{
			name:      "error from PowerShell",
			targetIQN: "iqn.2024-01.com.example:vol003",
			vhdxPath:  "D:\\vhdx\\vol003.vhdx",
			psOut:     struct{ LUN int32 }{},
			psErr:     errors.New("mapping failed"),
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := newUnitWinRMBackend()
			backend.psRunner = func(ctx context.Context, script string, out any) error {
				assert.Contains(t, script, tt.targetIQN)
				assert.Contains(t, script, tt.vhdxPath)
				if out != nil {
					copyTestOutput(out, tt.psOut)
				}
				return tt.psErr
			}

			lun, err := backend.MapDiskToTarget(context.Background(), tt.targetIQN, tt.vhdxPath)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantLUN, lun)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// UnmapDiskFromTarget tests
// ---------------------------------------------------------------------------

func TestWinRMBackend_UnmapDiskFromTarget(t *testing.T) {
	tests := []struct {
		name      string
		targetIQN string
		vhdxPath  string
		psErr     error
		wantErr   bool
	}{
		{
			name:      "happy path",
			targetIQN: "iqn.2024-01.com.example:vol001",
			vhdxPath:  "D:\\vhdx\\vol001.vhdx",
			psErr:     nil,
			wantErr:   false,
		},
		{
			name:      "error from PowerShell",
			targetIQN: "iqn.2024-01.com.example:vol002",
			vhdxPath:  "D:\\vhdx\\vol002.vhdx",
			psErr:     errors.New("unmap failed"),
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := newUnitWinRMBackend()
			backend.psRunner = func(ctx context.Context, script string, out any) error {
				assert.Contains(t, script, tt.targetIQN)
				assert.Contains(t, script, tt.vhdxPath)
				return tt.psErr
			}

			err := backend.UnmapDiskFromTarget(context.Background(), tt.targetIQN, tt.vhdxPath)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// DeleteVirtualDisk tests
// ---------------------------------------------------------------------------

func TestWinRMBackend_DeleteVirtualDisk(t *testing.T) {
	tests := []struct {
		name     string
		vhdxPath string
		psErr    error
		wantErr  bool
	}{
		{
			name:     "happy path",
			vhdxPath: "D:\\vhdx\\vol001.vhdx",
			psErr:    nil,
			wantErr:  false,
		},
		{
			name:     "error from PowerShell",
			vhdxPath: "D:\\vhdx\\vol002.vhdx",
			psErr:    errors.New("delete failed"),
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := newUnitWinRMBackend()
			backend.psRunner = func(ctx context.Context, script string, out any) error {
				assert.Contains(t, script, tt.vhdxPath)
				return tt.psErr
			}

			err := backend.DeleteVirtualDisk(context.Background(), tt.vhdxPath)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// GetVolumeByName tests
// ---------------------------------------------------------------------------

func TestWinRMBackend_GetVolumeByName(t *testing.T) {
	tests := []struct {
		name      string
		volName   string
		parentDir string
		psOut     struct {
			Exists    bool
			Path      string
			SizeBytes int64
			TargetIQN string
			LUN       int32
		}
		psErr         error
		wantExists    bool
		wantPath      string
		wantSize      int64
		wantTargetIQN string
		wantLUN       int32
		wantErr       bool
	}{
		{
			name:      "volume exists",
			volName:   "k8s-csi-test-volume-001",
			parentDir: "D:\\vhdx",
			psOut: struct {
				Exists    bool
				Path      string
				SizeBytes int64
				TargetIQN string
				LUN       int32
			}{Exists: true, Path: "D:\\vhdx\\k8s-csi-test-volume-001.vhdx", SizeBytes: 1073741824, TargetIQN: "iqn.2024-01.com.example:vol001", LUN: 0},
			psErr:         nil,
			wantExists:    true,
			wantPath:      "D:\\vhdx\\k8s-csi-test-volume-001.vhdx",
			wantSize:      1073741824,
			wantTargetIQN: "iqn.2024-01.com.example:vol001",
			wantLUN:       0,
			wantErr:       false,
		},
		{
			name:      "volume does not exist",
			volName:   "csi-vol-002",
			parentDir: "E:\\storage",
			psOut: struct {
				Exists    bool
				Path      string
				SizeBytes int64
				TargetIQN string
				LUN       int32
			}{Exists: false},
			psErr:         nil,
			wantExists:    false,
			wantPath:      "",
			wantSize:      0,
			wantTargetIQN: "",
			wantLUN:       -1,
			wantErr:       false,
		},
		{
			name:      "error from PowerShell",
			volName:   "csi-vol-003",
			parentDir: "D:\\vhdx",
			psOut: struct {
				Exists    bool
				Path      string
				SizeBytes int64
				TargetIQN string
				LUN       int32
			}{},
			psErr:         errors.New("query failed"),
			wantExists:    false,
			wantPath:      "",
			wantSize:      0,
			wantTargetIQN: "",
			wantLUN:       -1,
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := newUnitWinRMBackend()
			backend.psRunner = func(ctx context.Context, script string, out any) error {
				assert.Contains(t, script, tt.volName)
				assert.Contains(t, script, tt.parentDir)
				if out != nil {
					copyTestOutput(out, tt.psOut)
				}
				return tt.psErr
			}

			exists, path, size, targetIQN, lun, err := backend.GetVolumeByName(context.Background(), tt.volName, tt.parentDir)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantExists, exists)
				assert.Equal(t, tt.wantPath, path)
				assert.Equal(t, tt.wantSize, size)
				assert.Equal(t, tt.wantTargetIQN, targetIQN)
				assert.Equal(t, tt.wantLUN, lun)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// AllowInitiator tests
// ---------------------------------------------------------------------------

func TestWinRMBackend_AllowInitiator(t *testing.T) {
	tests := []struct {
		name         string
		targetIQN    string
		initiatorIQN string
		psErr        error
		wantErr      bool
	}{
		{
			name:         "happy path - add initiator",
			targetIQN:    "iqn.2024-01.com.example:vol001",
			initiatorIQN: "iqn.2024-01.com.example:node001",
			psErr:        nil,
			wantErr:      false,
		},
		{
			name:         "happy path - initiator already allowed",
			targetIQN:    "iqn.2024-01.com.example:vol002",
			initiatorIQN: "iqn.2024-01.com.example:node002",
			psErr:        nil,
			wantErr:      false,
		},
		{
			name:         "error from PowerShell",
			targetIQN:    "iqn.2024-01.com.example:vol003",
			initiatorIQN: "iqn.2024-01.com.example:node003",
			psErr:        errors.New("allow failed"),
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := newUnitWinRMBackend()
			backend.psRunner = func(ctx context.Context, script string, out any) error {
				assert.Contains(t, script, tt.targetIQN)
				assert.Contains(t, script, tt.initiatorIQN)
				return tt.psErr
			}

			err := backend.AllowInitiator(context.Background(), tt.targetIQN, tt.initiatorIQN)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// DenyInitiator tests
// ---------------------------------------------------------------------------

func TestWinRMBackend_DenyInitiator(t *testing.T) {
	tests := []struct {
		name         string
		targetIQN    string
		initiatorIQN string
		psErr        error
		wantErr      bool
	}{
		{
			name:         "happy path - remove initiator",
			targetIQN:    "iqn.2024-01.com.example:vol001",
			initiatorIQN: "iqn.2024-01.com.example:node001",
			psErr:        nil,
			wantErr:      false,
		},
		{
			name:         "error from PowerShell",
			targetIQN:    "iqn.2024-01.com.example:vol002",
			initiatorIQN: "iqn.2024-01.com.example:node002",
			psErr:        errors.New("deny failed"),
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := newUnitWinRMBackend()
			backend.psRunner = func(ctx context.Context, script string, out any) error {
				assert.Contains(t, script, tt.targetIQN)
				assert.Contains(t, script, tt.initiatorIQN)
				return tt.psErr
			}

			err := backend.DenyInitiator(context.Background(), tt.targetIQN, tt.initiatorIQN)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// GetTargetInitiators tests
// ---------------------------------------------------------------------------

func TestWinRMBackend_GetTargetInitiators(t *testing.T) {
	tests := []struct {
		name      string
		targetIQN string
		psOut     struct{ IQNs []string }
		psErr     error
		wantIQNs  []string
		wantErr   bool
	}{
		{
			name:      "happy path - with initiators",
			targetIQN: "iqn.2024-01.com.example:vol001",
			psOut:     struct{ IQNs []string }{IQNs: []string{"iqn.2024-01.com.example:node001", "iqn.2024-01.com.example:node002"}},
			psErr:     nil,
			wantIQNs:  []string{"iqn.2024-01.com.example:node001", "iqn.2024-01.com.example:node002"},
			wantErr:   false,
		},
		{
			name:      "happy path - no initiators",
			targetIQN: "iqn.2024-01.com.example:vol002",
			psOut:     struct{ IQNs []string }{IQNs: []string{}},
			psErr:     nil,
			wantIQNs:  []string{},
			wantErr:   false,
		},
		{
			name:      "error from PowerShell",
			targetIQN: "iqn.2024-01.com.example:vol003",
			psOut:     struct{ IQNs []string }{},
			psErr:     errors.New("query failed"),
			wantIQNs:  nil,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := newUnitWinRMBackend()
			backend.psRunner = func(ctx context.Context, script string, out any) error {
				assert.Contains(t, script, tt.targetIQN)
				if out != nil {
					copyTestOutput(out, tt.psOut)
				}
				return tt.psErr
			}

			iqns, err := backend.GetTargetInitiators(context.Background(), tt.targetIQN)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantIQNs, iqns)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// GetDirectoryFreeCapacity tests
// ---------------------------------------------------------------------------

func TestWinRMBackend_GetDirectoryFreeCapacity(t *testing.T) {
	tests := []struct {
		name      string
		parentDir string
		psOut     struct{ Free int64 }
		psErr     error
		wantFree  int64
		wantErr   bool
	}{
		{
			name:      "happy path",
			parentDir: "D:\\vhdx",
			psOut:     struct{ Free int64 }{Free: 107374182400},
			psErr:     nil,
			wantFree:  107374182400,
			wantErr:   false,
		},
		{
			name:      "zero free space",
			parentDir: "E:\\storage",
			psOut:     struct{ Free int64 }{Free: 0},
			psErr:     nil,
			wantFree:  0,
			wantErr:   false,
		},
		{
			name:      "error from PowerShell",
			parentDir: "F:\\storage",
			psOut:     struct{ Free int64 }{},
			psErr:     errors.New("query failed"),
			wantFree:  0,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := newUnitWinRMBackend()
			backend.psRunner = func(ctx context.Context, script string, out any) error {
				assert.Contains(t, script, tt.parentDir)
				if out != nil {
					copyTestOutput(out, tt.psOut)
				}
				return tt.psErr
			}

			free, err := backend.GetDirectoryFreeCapacity(context.Background(), tt.parentDir)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantFree, free)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// CreateSnapshot tests
// ---------------------------------------------------------------------------

func TestWinRMBackend_CreateSnapshot(t *testing.T) {
	tests := []struct {
		name     string
		vhdxPath string
		desc     string
		psOut    struct {
			SnapshotID   string
			OriginalPath string
			Description  string
			SizeBytes    int64
		}
		psErr      error
		wantErr    bool
		wantSnapID string
	}{
		{
			name:     "happy path",
			vhdxPath: "D:\\vhdx\\vol001.vhdx",
			desc:     "pre-backup snapshot",
			psOut: struct {
				SnapshotID   string
				OriginalPath string
				Description  string
				SizeBytes    int64
			}{SnapshotID: "snap-001", OriginalPath: "D:\\vhdx\\vol001.vhdx", Description: "pre-backup snapshot", SizeBytes: 1073741824},
			psErr:      nil,
			wantErr:    false,
			wantSnapID: "snap-001",
		},
		{
			name:     "error from PowerShell",
			vhdxPath: "D:\\vhdx\\vol002.vhdx",
			desc:     "another snapshot",
			psOut: struct {
				SnapshotID   string
				OriginalPath string
				Description  string
				SizeBytes    int64
			}{},
			psErr:   errors.New("checkpoint failed"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := newUnitWinRMBackend()
			backend.psRunner = func(ctx context.Context, script string, out any) error {
				assert.Contains(t, script, tt.vhdxPath)
				assert.Contains(t, script, tt.desc)
				assert.Contains(t, script, "Checkpoint-IscsiVirtualDisk")
				if out != nil {
					copyTestOutput(out, tt.psOut)
				}
				return tt.psErr
			}

			snap, err := backend.CreateSnapshot(context.Background(), tt.vhdxPath, tt.desc)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantSnapID, snap.SnapshotID)
				assert.Equal(t, tt.vhdxPath, snap.OriginalPath)
				assert.Equal(t, tt.desc, snap.Description)
			}
		})
	}
}

func TestWinRMBackend_CreateSnapshot_FileShareDirectoryRejected(t *testing.T) {
	backend := newUnitWinRMBackend()
	backend.psRunner = func(ctx context.Context, script string, out any) error {
		t.Fatalf("directory-backed file-share CreateSnapshot should not run PowerShell; script: %s", script)
		return nil
	}

	snap, err := backend.CreateSnapshot(context.Background(), "D:\\shares\\csi-nfs-test", "fileshare snap")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "directory-backed file-share snapshots are not supported")
	assert.Empty(t, snap.SnapshotID)
}

// ---------------------------------------------------------------------------
// DeleteSnapshot tests
// ---------------------------------------------------------------------------

func TestWinRMBackend_DeleteSnapshot(t *testing.T) {
	tests := []struct {
		name       string
		snapshotID string
		psErr      error
		wantErr    bool
	}{
		{
			name:       "happy path",
			snapshotID: "snap-001",
			psErr:      nil,
			wantErr:    false,
		},
		{
			name:       "error from PowerShell",
			snapshotID: "snap-002",
			psErr:      errors.New("delete failed"),
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := newUnitWinRMBackend()
			backend.psRunner = func(ctx context.Context, script string, out any) error {
				assert.Contains(t, script, tt.snapshotID)
				return tt.psErr
			}

			err := backend.DeleteSnapshot(context.Background(), tt.snapshotID)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestWinRMBackend_DeleteSnapshot_FileShareVSSHandle(t *testing.T) {
	snapshotID, err := encodeFileShareSnapshotHandle(fileShareSnapshotHandle{
		SnapshotType: "vss",
		ShadowID:     "{22222222-2222-2222-2222-222222222222}",
		OriginalPath: "D:\\shares\\csi-nfs-test",
	})
	require.NoError(t, err)

	backend := newUnitWinRMBackend()
	backend.psRunner = func(ctx context.Context, script string, out any) error {
		assert.Contains(t, script, "Remove-CsiShadowCopy")
		assert.Contains(t, script, "{22222222-2222-2222-2222-222222222222}")
		assert.NotContains(t, script, "Get-IscsiVirtualDiskSnapshot")
		return nil
	}

	require.NoError(t, backend.DeleteSnapshot(context.Background(), snapshotID))
}

// ---------------------------------------------------------------------------
// ListSnapshots tests
// ---------------------------------------------------------------------------

func TestWinRMBackend_ListSnapshots(t *testing.T) {
	tests := []struct {
		name      string
		vhdxPath  string
		psOut     winRMSnapshotListOutput
		psErr     error
		wantCount int
		wantErr   bool
	}{
		{
			name:     "happy path - multiple snapshots",
			vhdxPath: "D:\\vhdx\\vol001.vhdx",
			psOut: winRMSnapshotListOutput{
				Snapshots: []winRMSnapshotOutput{
					{SnapshotID: "snap-001", OriginalPath: "D:\\vhdx\\vol001.vhdx", Description: "snap 1", SizeBytes: 1073741824},
					{SnapshotID: "snap-002", OriginalPath: "D:\\vhdx\\vol001.vhdx", Description: "snap 2", SizeBytes: 2147483648},
				},
			},
			psErr:     nil,
			wantCount: 2,
			wantErr:   false,
		},
		{
			name:      "happy path - no snapshots",
			vhdxPath:  "D:\\vhdx\\vol002.vhdx",
			psOut:     winRMSnapshotListOutput{Snapshots: []winRMSnapshotOutput{}},
			psErr:     nil,
			wantCount: 0,
			wantErr:   false,
		},
		{
			name:      "error from PowerShell",
			vhdxPath:  "D:\\vhdx\\vol003.vhdx",
			psErr:     errors.New("list failed"),
			wantCount: 0,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := newUnitWinRMBackend()
			backend.psRunner = func(ctx context.Context, script string, out any) error {
				assert.Contains(t, script, tt.vhdxPath)
				if out != nil {
					copyTestOutput(out, tt.psOut)
				}
				return tt.psErr
			}

			snaps, err := backend.ListSnapshots(context.Background(), tt.vhdxPath)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantCount, len(snaps))
			}
		})
	}
}

func TestWinRMBackend_ListSnapshots_FileShareReturnsEmpty(t *testing.T) {
	backend := newUnitWinRMBackend()
	backend.psRunner = func(ctx context.Context, script string, out any) error {
		t.Fatalf("file-share ListSnapshots should not call PowerShell; script: %s", script)
		return nil
	}

	snaps, err := backend.ListSnapshots(context.Background(), "D:\\shares\\csi-nfs-test")
	require.NoError(t, err)
	assert.Empty(t, snaps)
}

// ---------------------------------------------------------------------------
// ExportSnapshotAsVirtualDisk tests
// ---------------------------------------------------------------------------

func TestWinRMBackend_ExportSnapshotAsVirtualDisk(t *testing.T) {
	tests := []struct {
		name       string
		snapshotID string
		psOut      struct{ Path string }
		psErr      error
		wantPath   string
		wantErr    bool
	}{
		{
			name:       "happy path",
			snapshotID: "snap-001",
			psOut:      struct{ Path string }{Path: "D:\\vhdx\\exported-snap-001.vhdx"},
			psErr:      nil,
			wantPath:   "D:\\vhdx\\exported-snap-001.vhdx",
			wantErr:    false,
		},
		{
			name:       "error from PowerShell",
			snapshotID: "snap-002",
			psOut:      struct{ Path string }{},
			psErr:      errors.New("export failed"),
			wantPath:   "",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := newUnitWinRMBackend()
			backend.psRunner = func(ctx context.Context, script string, out any) error {
				assert.Contains(t, script, tt.snapshotID)
				if out != nil {
					copyTestOutput(out, tt.psOut)
				}
				return tt.psErr
			}

			path, err := backend.ExportSnapshotAsVirtualDisk(context.Background(), tt.snapshotID)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantPath, path)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ResizeVirtualDisk tests
// ---------------------------------------------------------------------------

func TestWinRMBackend_ResizeVirtualDisk(t *testing.T) {
	tests := []struct {
		name         string
		vhdxPath     string
		newSizeBytes int64
		psOut        struct{ SizeBytes int64 }
		psErr        error
		wantSize     int64
		wantErr      bool
	}{
		{
			name:         "expand disk",
			vhdxPath:     "D:\\vhdx\\vol001.vhdx",
			newSizeBytes: 2147483648,
			psOut:        struct{ SizeBytes int64 }{SizeBytes: 2147483648},
			psErr:        nil,
			wantSize:     2147483648,
			wantErr:      false,
		},
		{
			name:         "error from PowerShell",
			vhdxPath:     "D:\\vhdx\\vol002.vhdx",
			newSizeBytes: 5368709120,
			psOut:        struct{ SizeBytes int64 }{},
			psErr:        errors.New("resize failed"),
			wantSize:     0,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := newUnitWinRMBackend()
			backend.psRunner = func(ctx context.Context, script string, out any) error {
				assert.Contains(t, script, tt.vhdxPath)
				assert.Contains(t, script, fmt.Sprintf("%d", tt.newSizeBytes))
				if out != nil {
					copyTestOutput(out, tt.psOut)
				}
				return tt.psErr
			}

			size, err := backend.ResizeVirtualDisk(context.Background(), tt.vhdxPath, tt.newSizeBytes)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantSize, size)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// GetVolumeInfo tests
// ---------------------------------------------------------------------------

func TestWinRMBackend_GetVolumeInfo(t *testing.T) {
	tests := []struct {
		name     string
		vhdxPath string
		psOut    struct {
			Path      string
			SizeBytes int64
			Targets   []string
			LUN       *int32
		}
		psErr    error
		wantInfo VolumeInfo
		wantErr  bool
	}{
		{
			name:     "happy path - with targets",
			vhdxPath: "D:\\vhdx\\vol001.vhdx",
			psOut: struct {
				Path      string
				SizeBytes int64
				Targets   []string
				LUN       *int32
			}{
				Path: "D:\\vhdx\\vol001.vhdx", SizeBytes: 1073741824, Targets: []string{"iqn.2024-01.com.example:vol001"},
				LUN: func() *int32 { i := int32(0); return &i }(),
			},
			psErr:   nil,
			wantErr: false,
		},
		{
			name:     "happy path - no targets",
			vhdxPath: "D:\\vhdx\\vol002.vhdx",
			psOut: struct {
				Path      string
				SizeBytes int64
				Targets   []string
				LUN       *int32
			}{
				Path: "D:\\vhdx\\vol002.vhdx", SizeBytes: 0, Targets: []string{}, LUN: nil,
			},
			psErr:   nil,
			wantErr: false,
		},
		{
			name:     "error from PowerShell",
			vhdxPath: "D:\\vhdx\\vol003.vhdx",
			psOut: struct {
				Path      string
				SizeBytes int64
				Targets   []string
				LUN       *int32
			}{},
			psErr:   errors.New("query failed"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := newUnitWinRMBackend()
			backend.psRunner = func(ctx context.Context, script string, out any) error {
				assert.Contains(t, script, tt.vhdxPath)
				if out != nil {
					copyTestOutput(out, tt.psOut)
				}
				return tt.psErr
			}

			info, err := backend.GetVolumeInfo(context.Background(), tt.vhdxPath)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.psOut.Path, info.VHDXPath)
				assert.Equal(t, tt.psOut.SizeBytes, info.SizeBytes)
				assert.Equal(t, tt.psOut.Targets, info.Targets)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// escapePS tests
// ---------------------------------------------------------------------------

func TestEscapePS(t *testing.T) {
	tests := []struct {
		input  string
		output string
	}{
		{input: "normal-string", output: "normal-string"},
		{input: "string-with-'quotes", output: "string-with-''quotes"},
		{input: "all-'single-'quotes", output: "all-''single-''quotes"},
		{input: "", output: ""},
		{input: "no-quotes-at-all", output: "no-quotes-at-all"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("input=%q", tt.input), func(t *testing.T) {
			got := escapePS(tt.input)
			assert.Equal(t, tt.output, got)
		})
	}
}
