package iscsilib

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	mount "k8s.io/mount-utils"
)

func fakeExecCommandContext(t *testing.T, stdout, stderr string, exitCode, sleepMS int) func(context.Context, string, ...string) *exec.Cmd {
	t.Helper()

	return func(ctx context.Context, command string, args ...string) *exec.Cmd {
		helperArgs := []string{"-test.run=TestHelperProcess", "--", command}
		helperArgs = append(helperArgs, args...)
		cmd := exec.CommandContext(ctx, os.Args[0], helperArgs...)
		cmd.Env = append(os.Environ(),
			"GO_WANT_HELPER_PROCESS=1",
			"GO_HELPER_STDOUT="+base64String(stdout),
			"GO_HELPER_STDERR="+base64String(stderr),
			fmt.Sprintf("GO_HELPER_EXIT_CODE=%d", exitCode),
			fmt.Sprintf("GO_HELPER_SLEEP_MS=%d", sleepMS),
		)
		return cmd
	}
}

func base64String(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}

func TestExecWithTimeout(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		originalExecCommandContext := execCommandContext
		execCommandContext = fakeExecCommandContext(t, "ok", "", 0, 0)
		t.Cleanup(func() { execCommandContext = originalExecCommandContext })

		out, err := ExecWithTimeout("fake", []string{"arg"}, time.Second)
		require.NoError(t, err)
		assert.Equal(t, []byte("ok"), out)
	})

	t.Run("timeout", func(t *testing.T) {
		originalExecCommandContext := execCommandContext
		execCommandContext = fakeExecCommandContext(t, "", "", 0, 200)
		t.Cleanup(func() { execCommandContext = originalExecCommandContext })

		out, err := ExecWithTimeout("fake", nil, 10*time.Millisecond)
		require.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Nil(t, out)
	})
}

func TestFlushMultipathDevice(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		calls := captureExecWithTimeout(t, nil, nil)

		err := FlushMultipathDevice(&Device{Name: "mpatha", Type: "mpath"})
		require.NoError(t, err)
		require.Len(t, *calls, 1)
		assert.Equal(t, "multipath", (*calls)[0].command)
		assert.Equal(t, []string{"-f", filepath.Join("/dev/mapper", "mpatha")}, (*calls)[0].args)
		assert.Equal(t, 5*time.Second, (*calls)[0].timeout)
	})

	t.Run("map in use", func(t *testing.T) {
		originalStat := osStat
		osStat = func(name string) (os.FileInfo, error) {
			return nil, nil
		}
		captureExecWithTimeout(t, nil, []error{errors.New("map in use")})
		t.Cleanup(func() { osStat = originalStat })

		err := FlushMultipathDevice(&Device{Name: "mpatha", Type: "mpath"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "probably still in use")
	})

	t.Run("missing after flush is success", func(t *testing.T) {
		originalStat := osStat
		osStat = func(name string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		}
		captureExecWithTimeout(t, nil, []error{errors.New("not found")})
		t.Cleanup(func() { osStat = originalStat })

		require.NoError(t, FlushMultipathDevice(&Device{Name: "mpatha", Type: "mpath"}))
	})
}

func TestResizeMultipathDevice(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		originalExecCommand := execCommand
		execCommand = fakeExecCommand(t, "", "", 0, func(command string, args []string) {
			assert.Equal(t, "multipathd", command)
			assert.Equal(t, []string{"resize", "map", "mpatha"}, args)
		})
		t.Cleanup(func() { execCommand = originalExecCommand })

		require.NoError(t, ResizeMultipathDevice(&Device{Name: "mpatha", Type: "mpath"}))
	})

	t.Run("failure includes output", func(t *testing.T) {
		originalExecCommand := execCommand
		execCommand = fakeExecCommand(t, "resize failed", "", 1, nil)
		t.Cleanup(func() { execCommand = originalExecCommand })

		err := ResizeMultipathDevice(&Device{Name: "mpatha", Type: "mpath"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "resize failed")
	})
}

func TestMultipathConsistency(t *testing.T) {
	t.Run("consistent devices", func(t *testing.T) {
		originalExecWithTimeout := execWithTimeout
		execWithTimeout = func(command string, args []string, timeout time.Duration) ([]byte, error) {
			return []byte("wwid-1\n"), nil
		}
		t.Cleanup(func() { execWithTimeout = originalExecWithTimeout })

		conn := &Connector{
			MountTargetDevice: &Device{Name: "wwid-1", Type: "mpath", Size: "10G"},
			Devices: []Device{
				{Name: "sda", Hctl: "2:0:0:3", Type: "disk", Size: "10G"},
				{Name: "sdb", Hctl: "3:0:0:3", Type: "disk", Size: "10G"},
			},
		}

		assert.True(t, conn.IsMultipathEnabled())
		require.NoError(t, conn.IsMultipathConsistent())
	})

	t.Run("size mismatch", func(t *testing.T) {
		originalExecWithTimeout := execWithTimeout
		execWithTimeout = func(command string, args []string, timeout time.Duration) ([]byte, error) {
			return []byte("wwid-1\n"), nil
		}
		t.Cleanup(func() { execWithTimeout = originalExecWithTimeout })

		conn := &Connector{
			MountTargetDevice: &Device{Name: "wwid-1", Type: "mpath", Size: "10G"},
			Devices:           []Device{{Name: "sda", Hctl: "2:0:0:3", Type: "disk", Size: "11G"}},
		}

		err := conn.IsMultipathConsistent()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "size differ")
	})

	t.Run("wwid mismatch", func(t *testing.T) {
		originalExecWithTimeout := execWithTimeout
		execWithTimeout = func(command string, args []string, timeout time.Duration) ([]byte, error) {
			return []byte("other-wwid\n"), nil
		}
		t.Cleanup(func() { execWithTimeout = originalExecWithTimeout })

		conn := &Connector{
			MountTargetDevice: &Device{Name: "wwid-1", Type: "mpath", Size: "10G"},
			Devices:           []Device{{Name: "sda", Hctl: "2:0:0:3", Type: "disk", Size: "10G"}},
		}

		err := conn.IsMultipathConsistent()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "WWIDs differ")
	})
}

type fakeMountInterface struct {
	mounts  []mount.MountPoint
	listErr error
}

func (f *fakeMountInterface) Mount(source string, target string, fstype string, options []string) error {
	return nil
}
func (f *fakeMountInterface) MountSensitive(source string, target string, fstype string, options []string, sensitiveOptions []string) error {
	return nil
}
func (f *fakeMountInterface) MountSensitiveWithoutSystemd(source string, target string, fstype string, options []string, sensitiveOptions []string) error {
	return nil
}
func (f *fakeMountInterface) MountSensitiveWithoutSystemdWithMountFlags(source string, target string, fstype string, options []string, sensitiveOptions []string, mountFlags []string) error {
	return nil
}
func (f *fakeMountInterface) Unmount(target string) error { return nil }
func (f *fakeMountInterface) List() ([]mount.MountPoint, error) {
	return f.mounts, f.listErr
}
func (f *fakeMountInterface) IsLikelyNotMountPoint(file string) (bool, error) { return false, nil }
func (f *fakeMountInterface) CanSafelySkipMountPointCheck() bool              { return false }
func (f *fakeMountInterface) IsMountPoint(file string) (bool, error)          { return true, nil }
func (f *fakeMountInterface) GetMountRefs(pathname string) ([]string, error)  { return nil, nil }

type fakeResizer struct {
	devicePath string
	mountPoint string
	err        error
}

func (f *fakeResizer) Resize(devicePath, mountPoint string) (bool, error) {
	f.devicePath = devicePath
	f.mountPoint = mountPoint
	return f.err == nil, f.err
}

func TestExpandVolume(t *testing.T) {
	originalGetDeviceNameFromMount := getDeviceNameFromMount
	t.Cleanup(func() { getDeviceNameFromMount = originalGetDeviceNameFromMount })

	t.Run("resizes filesystem", func(t *testing.T) {
		volumePath := t.TempDir()
		calls := captureExecWithTimeout(t, nil, []error{errors.New("rescan is non fatal")})
		resizer := &fakeResizer{}
		mounter := &fakeMountInterface{mounts: []mount.MountPoint{{Device: "/dev/sdb", Path: volumePath}}}
		getDeviceNameFromMount = func(mounter mount.Interface, mountPath string) (string, int, error) {
			assert.Equal(t, volumePath, mountPath)
			return "/dev/sdb", 1, nil
		}

		err := ExpandVolume(mounter, resizer, volumePath)
		require.NoError(t, err)
		require.Len(t, *calls, 1)
		assert.Equal(t, []string{"-m", "session", "--rescan"}, (*calls)[0].args)
		assert.Equal(t, "/dev/sdb", resizer.devicePath)
		assert.Equal(t, volumePath, resizer.mountPoint)
	})

	t.Run("mount lookup error", func(t *testing.T) {
		volumePath := t.TempDir()
		captureExecWithTimeout(t, nil, nil)
		getDeviceNameFromMount = func(mounter mount.Interface, mountPath string) (string, int, error) {
			return "", 0, errors.New("list failed")
		}

		err := ExpandVolume(&fakeMountInterface{listErr: errors.New("list failed")}, &fakeResizer{}, volumePath)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot resolve device")
	})

	t.Run("filesystem resize error", func(t *testing.T) {
		volumePath := t.TempDir()
		captureExecWithTimeout(t, nil, nil)
		resizer := &fakeResizer{err: errors.New("resize failed")}
		mounter := &fakeMountInterface{mounts: []mount.MountPoint{{Device: "/dev/sdb", Path: volumePath}}}
		getDeviceNameFromMount = func(mounter mount.Interface, mountPath string) (string, int, error) {
			return "/dev/sdb", 1, nil
		}

		err := ExpandVolume(mounter, resizer, volumePath)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "filesystem resize failed")
	})

	t.Run("multipath resize failure is non fatal", func(t *testing.T) {
		volumePath := t.TempDir()
		captureExecWithTimeout(t, nil, nil)
		originalExecCommand := execCommand
		execCommand = fakeExecCommand(t, "resize failed", "", 1, nil)
		t.Cleanup(func() { execCommand = originalExecCommand })

		resizer := &fakeResizer{}
		mounter := &fakeMountInterface{mounts: []mount.MountPoint{{Device: "/dev/mapper/mpatha", Path: volumePath}}}
		getDeviceNameFromMount = func(mounter mount.Interface, mountPath string) (string, int, error) {
			return "/dev/mapper/mpatha", 1, nil
		}
		err := ExpandVolume(mounter, resizer, volumePath)
		require.NoError(t, err)
		assert.Equal(t, "/dev/mapper/mpatha", resizer.devicePath)
	})
}
