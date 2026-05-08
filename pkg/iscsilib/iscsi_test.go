package iscsilib

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fakeExecCommand(t *testing.T, stdout, stderr string, exitCode int, check func(command string, args []string)) func(string, ...string) *exec.Cmd {
	t.Helper()

	return func(command string, args ...string) *exec.Cmd {
		if check != nil {
			check(command, args)
		}
		helperArgs := []string{"-test.run=TestHelperProcess", "--", command}
		helperArgs = append(helperArgs, args...)
		cmd := exec.Command(os.Args[0], helperArgs...)
		cmd.Env = append(os.Environ(),
			"GO_WANT_HELPER_PROCESS=1",
			"GO_HELPER_STDOUT="+base64.StdEncoding.EncodeToString([]byte(stdout)),
			"GO_HELPER_STDERR="+base64.StdEncoding.EncodeToString([]byte(stderr)),
			fmt.Sprintf("GO_HELPER_EXIT_CODE=%d", exitCode),
		)
		return cmd
	}
}

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	if sleepMS, _ := strconv.Atoi(os.Getenv("GO_HELPER_SLEEP_MS")); sleepMS > 0 {
		time.Sleep(time.Duration(sleepMS) * time.Millisecond)
	}
	stdout, _ := base64.StdEncoding.DecodeString(os.Getenv("GO_HELPER_STDOUT"))
	stderr, _ := base64.StdEncoding.DecodeString(os.Getenv("GO_HELPER_STDERR"))
	_, _ = fmt.Fprint(os.Stdout, string(stdout))
	_, _ = fmt.Fprint(os.Stderr, string(stderr))
	code, _ := strconv.Atoi(os.Getenv("GO_HELPER_EXIT_CODE"))
	os.Exit(code)
}

func TestParseSessions(t *testing.T) {
	sessions := parseSessions(`
tcp: [1] 10.0.0.1:3260,1 iqn.2024-01.com.example:vol1
iser: [42] 10.0.0.2:3260,2 iqn.2024-01.com.example:vol2
malformed
`)

	require.Len(t, sessions, 2)
	assert.Equal(t, iscsiSession{
		Protocol: "tcp",
		ID:       1,
		Portal:   "10.0.0.1:3260",
		IQN:      "iqn.2024-01.com.example:vol1",
		Name:     "vol1",
	}, sessions[0])
	assert.Equal(t, iscsiSession{
		Protocol: "iser",
		ID:       42,
		Portal:   "10.0.0.2:3260",
		IQN:      "iqn.2024-01.com.example:vol2",
		Name:     "vol2",
	}, sessions[1])
}

func TestExtractTransportName(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{name: "missing", output: "iface.net_ifacename = default\n", want: ""},
		{name: "empty defaults tcp", output: "iface.transport_name = \n", want: "tcp"},
		{name: "explicit", output: "iface.transport_name = iser\n", want: "iser"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, extractTransportName(tt.output))
		})
	}
}

func TestPathExists(t *testing.T) {
	t.Run("tcp uses stat", func(t *testing.T) {
		originalStat := osStat
		osStat = func(name string) (os.FileInfo, error) {
			assert.Equal(t, "/dev/sdb", name)
			return nil, nil
		}
		t.Cleanup(func() { osStat = originalStat })

		devicePath := "/dev/sdb"
		require.NoError(t, pathExists(&devicePath, "tcp"))
		assert.Equal(t, "/dev/sdb", devicePath)
	})

	t.Run("tcp returns non-not-exist stat errors", func(t *testing.T) {
		originalStat := osStat
		osStat = func(name string) (os.FileInfo, error) {
			return nil, errors.New("permission denied")
		}
		t.Cleanup(func() { osStat = originalStat })

		devicePath := "/dev/sdb"
		err := pathExists(&devicePath, "tcp")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "permission denied")
	})

	t.Run("non tcp uses first glob match", func(t *testing.T) {
		originalGlob := filepathGlob
		filepathGlob = func(pattern string) ([]string, error) {
			assert.Equal(t, "/dev/disk/by-path/pci-*", pattern)
			return []string{"/dev/disk/by-path/pci-1", "/dev/disk/by-path/pci-2"}, nil
		}
		t.Cleanup(func() { filepathGlob = originalGlob })

		devicePath := "/dev/disk/by-path/pci-*"
		require.NoError(t, pathExists(&devicePath, "iser"))
		assert.Equal(t, "/dev/disk/by-path/pci-1", devicePath)
	})

	t.Run("non tcp missing glob", func(t *testing.T) {
		originalGlob := filepathGlob
		filepathGlob = func(pattern string) ([]string, error) {
			return nil, nil
		}
		t.Cleanup(func() { filepathGlob = originalGlob })

		devicePath := "/dev/disk/by-path/pci-*"
		assert.ErrorIs(t, pathExists(&devicePath, "iser"), os.ErrNotExist)
	})
}

func TestWaitForPathToExist(t *testing.T) {
	t.Run("retries until stat succeeds", func(t *testing.T) {
		originalStat := osStat
		originalSleep := sleep
		attempts := 0
		osStat = func(name string) (os.FileInfo, error) {
			attempts++
			if attempts < 3 {
				return nil, os.ErrNotExist
			}
			return nil, nil
		}
		sleep = func(time.Duration) {}
		t.Cleanup(func() {
			osStat = originalStat
			sleep = originalSleep
		})

		devicePath := "/dev/sdb"
		require.NoError(t, waitForPathToExist(&devicePath, 3, 1, "tcp"))
		assert.Equal(t, 3, attempts)
	})

	t.Run("nil path is invalid", func(t *testing.T) {
		err := waitForPathToExist(nil, 1, 1, "tcp")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unspecified devicePath")
	})
}

func TestGetMultipathDevice(t *testing.T) {
	t.Run("shared multipath child", func(t *testing.T) {
		devices := []Device{
			{Name: "sda", Children: []Device{{Name: "mpatha", Type: "mpath"}}},
			{Name: "sdb", Children: []Device{{Name: "mpatha", Type: "mpath"}}},
		}

		got, err := getMultipathDevice(devices)
		require.NoError(t, err)
		assert.Equal(t, "mpatha", got.Name)
	})

	t.Run("missing child", func(t *testing.T) {
		_, err := getMultipathDevice([]Device{{Name: "sda"}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "multipathd")
	})

	t.Run("different multipath children", func(t *testing.T) {
		devices := []Device{
			{Name: "sda", Children: []Device{{Name: "mpatha", Type: "mpath"}}},
			{Name: "sdb", Children: []Device{{Name: "mpathb", Type: "mpath"}}},
		}

		_, err := getMultipathDevice(devices)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "common multipath device")
	})

	t.Run("child must be multipath", func(t *testing.T) {
		_, err := getMultipathDevice([]Device{{Name: "sda", Children: []Device{{Name: "sdb", Type: "disk"}}}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "mpath type")
	})
}

func TestConnectorGetMountTargetDevice(t *testing.T) {
	t.Run("single device", func(t *testing.T) {
		conn := Connector{Devices: []Device{{Name: "sda", Type: "disk"}}}

		got, err := conn.getMountTargetDevice()
		require.NoError(t, err)
		assert.Equal(t, "sda", got.Name)
	})

	t.Run("no devices", func(t *testing.T) {
		conn := Connector{}

		_, err := conn.getMountTargetDevice()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does not contain any device")
	})

	t.Run("multiple devices use multipath target", func(t *testing.T) {
		conn := Connector{Devices: []Device{
			{Name: "sda", Children: []Device{{Name: "mpatha", Type: "mpath"}}},
			{Name: "sdb", Children: []Device{{Name: "mpatha", Type: "mpath"}}},
		}}

		got, err := conn.getMountTargetDevice()
		require.NoError(t, err)
		assert.Equal(t, "mpatha", got.Name)
	})
}

func TestGetSCSIDevicesParsesLsblkTree(t *testing.T) {
	originalExecCommand := execCommand
	execCommand = fakeExecCommand(t, "sda sda  2:0:0:1 disk iscsi 10G\nsda1 sda1 sda 2:0:0:1 part  10G\n", "", 0, func(command string, args []string) {
		assert.Equal(t, "lsblk", command)
		assert.Contains(t, args, "/dev/sda")
	})
	t.Cleanup(func() { execCommand = originalExecCommand })

	devices, err := GetSCSIDevices([]string{"/dev/sda"}, true)
	require.NoError(t, err)
	require.Len(t, devices, 1)
	assert.Equal(t, "sda", devices[0].Name)
	assert.Equal(t, "iscsi", devices[0].Transport)
	require.Len(t, devices[0].Children, 1)
	assert.Equal(t, "sda1", devices[0].Children[0].Name)
}

func TestGetSCSIDevicesNonStrictAllowsPartialLsblkResults(t *testing.T) {
	originalExecCommand := execCommand
	execCommand = fakeExecCommand(t, "sda sda  2:0:0:1 disk iscsi 10G\n", "some paths were missing", 64, nil)
	t.Cleanup(func() { execCommand = originalExecCommand })

	devices, err := GetSCSIDevices([]string{"/dev/sda", "/dev/missing"}, false)
	require.NoError(t, err)
	require.Len(t, devices, 1)
	assert.Equal(t, "sda", devices[0].Name)
}

func TestGetISCSIDevicesFiltersTransport(t *testing.T) {
	originalExecCommand := execCommand
	execCommand = fakeExecCommand(t, "sda sda  2:0:0:1 disk iscsi 10G\nsdb sdb  3:0:0:1 disk sata 10G\n", "", 0, nil)
	t.Cleanup(func() { execCommand = originalExecCommand })

	devices, err := GetISCSIDevices(nil, true)
	require.NoError(t, err)
	require.Len(t, devices, 1)
	assert.Equal(t, "sda", devices[0].Name)
	assert.Equal(t, "iscsi", devices[0].Transport)
}

func TestConnectorPersistAndGetConnectorFromFile(t *testing.T) {
	originalExecCommand := execCommand
	execCommand = fakeExecCommand(t, "sdb sdb  2:0:0:1 disk iscsi 10G\n", "", 0, nil)
	t.Cleanup(func() { execCommand = originalExecCommand })

	filePath := filepath.Join(t.TempDir(), "connector.json")
	conn := &Connector{
		VolumeName:        "vol-001",
		TargetIqn:         "iqn.2024-01.com.example:vol-001",
		TargetPortals:     []string{"10.0.0.1:3260"},
		Lun:               2,
		MountTargetDevice: &Device{Name: "sdb", Type: "disk"},
		Devices:           []Device{{Name: "sdb", Type: "disk"}},
	}

	require.NoError(t, conn.Persist(filePath))
	got, err := GetConnectorFromFile(filePath)
	require.NoError(t, err)
	assert.Equal(t, "vol-001", got.VolumeName)
	require.NotNil(t, got.MountTargetDevice)
	assert.Equal(t, "sdb", got.MountTargetDevice.Name)
	require.Len(t, got.Devices, 1)
	assert.Equal(t, "sdb", got.Devices[0].Name)
}

func TestDisconnectLogsOutFullPortalAndDeletesDBEntry(t *testing.T) {
	calls := captureExecWithTimeout(t, nil, nil)

	err := Disconnect("iqn.2024-01.com.example:vol-001", []string{"10.0.0.1:3260"})

	require.NoError(t, err)
	require.Len(t, *calls, 2)
	assert.Equal(t, []string{"-m", "node", "-T", "iqn.2024-01.com.example:vol-001", "-p", "10.0.0.1:3260", "-u"}, (*calls)[0].args)
	assert.Equal(t, []string{"-m", "node", "-T", "iqn.2024-01.com.example:vol-001", "-o", "delete"}, (*calls)[1].args)
}

func TestGetConnectorFromFileRequiresMountTargetDevice(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "connector.json")
	require.NoError(t, os.WriteFile(filePath, []byte(`{"volume_name":"vol-001"}`), 0o600))

	_, err := GetConnectorFromFile(filePath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mountTargetDevice")
}

func TestDeviceHelpers(t *testing.T) {
	assert.Equal(t, filepath.Join("/dev", "sdb"), (&Device{Name: "sdb", Type: "disk"}).GetPath())
	assert.Equal(t, filepath.Join("/dev/mapper", "mpatha"), (&Device{Name: "mpatha", Type: "mpath"}).GetPath())

	hctl, err := (&Device{Name: "sdb", Hctl: "3:0:1:4"}).HCTL()
	require.NoError(t, err)
	assert.Equal(t, &HCTL{HBA: 3, Channel: 0, Target: 1, LUN: 4}, hctl)

	_, err = (&Device{Name: "sdb", Hctl: "not:hctl"}).HCTL()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid HCTL")
}

func TestDeviceWWID(t *testing.T) {
	originalExecWithTimeout := execWithTimeout
	execWithTimeout = func(command string, args []string, timeout time.Duration) ([]byte, error) {
		assert.Equal(t, "scsi_id", command)
		assert.Equal(t, []string{"-g", "-u", filepath.Join("/dev", "sdb")}, args)
		assert.Equal(t, time.Second, timeout)
		return []byte("wwid-123\n"), nil
	}
	t.Cleanup(func() { execWithTimeout = originalExecWithTimeout })

	got, err := (&Device{Name: "sdb", Type: "disk"}).WWID()
	require.NoError(t, err)
	assert.Equal(t, "wwid-123", got)
}

func TestDeviceWWIDError(t *testing.T) {
	originalExecWithTimeout := execWithTimeout
	execWithTimeout = func(command string, args []string, timeout time.Duration) ([]byte, error) {
		return nil, errors.New("scsi_id failed")
	}
	t.Cleanup(func() { execWithTimeout = originalExecWithTimeout })

	_, err := (&Device{Name: "sdb", Type: "disk"}).WWID()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "scsi_id failed")
}
