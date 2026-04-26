package iscsilib

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type writeRecord struct {
	name    string
	flag    int
	perm    os.FileMode
	content string
}

func captureDeviceWrites(t *testing.T) *[]writeRecord {
	t.Helper()

	records := []writeRecord{}
	dir := t.TempDir()
	originalOpenFile := osOpenFile
	osOpenFile = func(name string, flag int, perm os.FileMode) (*os.File, error) {
		filePath := filepath.Join(dir, strings.NewReplacer("\\", "_", "/", "_", ":", "_").Replace(name))
		f, err := os.OpenFile(filePath, flag|os.O_CREATE, perm)
		if err != nil {
			return nil, err
		}
		records = append(records, writeRecord{name: name, flag: flag, perm: perm, content: filePath})
		return f, nil
	}
	t.Cleanup(func() { osOpenFile = originalOpenFile })
	return &records
}

func readDeviceWriteContent(t *testing.T, record writeRecord) string {
	t.Helper()

	data, err := os.ReadFile(record.content)
	require.NoError(t, err)
	return string(data)
}

func TestDeviceWriteOperations(t *testing.T) {
	records := captureDeviceWrites(t)
	device := &Device{Name: "sdb", Hctl: "3:0:1:4", Type: "disk"}

	require.NoError(t, device.WriteDeviceFile("custom", "hello"))
	require.NoError(t, device.Shutdown())
	require.NoError(t, device.Delete())
	require.NoError(t, device.Rescan())

	require.Len(t, *records, 4)
	assert.Equal(t, filepath.Join("/sys/class/scsi_device", "3:0:1:4", "device", "custom"), (*records)[0].name)
	assert.Equal(t, os.O_TRUNC|os.O_WRONLY, (*records)[0].flag)
	assert.Equal(t, os.FileMode(0o200), (*records)[0].perm)
	assert.Equal(t, "hello", readDeviceWriteContent(t, (*records)[0]))
	assert.Equal(t, "offline\n", readDeviceWriteContent(t, (*records)[1]))
	assert.Equal(t, "1", readDeviceWriteContent(t, (*records)[2]))
	assert.Equal(t, "1", readDeviceWriteContent(t, (*records)[3]))
}

func TestWriteDeviceFileOpenError(t *testing.T) {
	originalOpenFile := osOpenFile
	osOpenFile = func(name string, flag int, perm os.FileMode) (*os.File, error) {
		return nil, errors.New("open failed")
	}
	t.Cleanup(func() { osOpenFile = originalOpenFile })

	err := (&Device{Name: "sdb", Hctl: "3:0:1:4"}).Shutdown()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "open failed")
}

func TestRemoveSCSIDevices(t *testing.T) {
	t.Run("flushes and removes device", func(t *testing.T) {
		originalStat := osStat
		originalExecCommand := execCommand
		osStat = func(name string) (os.FileInfo, error) {
			assert.Equal(t, filepath.Join("/dev", "sdb"), name)
			return nil, nil
		}
		execCommand = fakeExecCommand(t, "", "", 0, func(command string, args []string) {
			assert.Equal(t, "blockdev", command)
			assert.Equal(t, []string{"--flushbufs", filepath.Join("/dev", "sdb")}, args)
		})
		records := captureDeviceWrites(t)
		t.Cleanup(func() {
			osStat = originalStat
			execCommand = originalExecCommand
		})

		err := RemoveSCSIDevices(Device{Name: "sdb", Hctl: "3:0:1:4", Type: "disk"})
		require.NoError(t, err)
		require.Len(t, *records, 2)
		assert.Equal(t, "offline\n", readDeviceWriteContent(t, (*records)[0]))
		assert.Equal(t, "1", readDeviceWriteContent(t, (*records)[1]))
	})

	t.Run("stat error is returned", func(t *testing.T) {
		originalStat := osStat
		osStat = func(name string) (os.FileInfo, error) {
			return nil, errors.New("stat failed")
		}
		t.Cleanup(func() { osStat = originalStat })

		err := RemoveSCSIDevices(Device{Name: "sdb", Hctl: "3:0:1:4", Type: "disk"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "stat failed")
	})

	t.Run("blockdev failure returns output", func(t *testing.T) {
		originalStat := osStat
		originalExecCommand := execCommand
		osStat = func(name string) (os.FileInfo, error) { return nil, nil }
		execCommand = fakeExecCommand(t, "flush failed", "", 1, nil)
		t.Cleanup(func() {
			osStat = originalStat
			execCommand = originalExecCommand
		})

		err := RemoveSCSIDevices(Device{Name: "sdb", Hctl: "3:0:1:4", Type: "disk"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "flush failed")
	})

	t.Run("shutdown not-exist is ignored", func(t *testing.T) {
		originalStat := osStat
		originalExecCommand := execCommand
		originalOpenFile := osOpenFile
		osStat = func(name string) (os.FileInfo, error) { return nil, nil }
		execCommand = fakeExecCommand(t, "", "", 0, nil)
		osOpenFile = func(name string, flag int, perm os.FileMode) (*os.File, error) {
			return nil, os.ErrNotExist
		}
		t.Cleanup(func() {
			osStat = originalStat
			execCommand = originalExecCommand
			osOpenFile = originalOpenFile
		})

		require.NoError(t, RemoveSCSIDevices(Device{Name: "sdb", Hctl: "3:0:1:4", Type: "disk"}))
	})
}

func TestPersistConnectorWrapper(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "connector.json")
	conn := &Connector{
		VolumeName:        "vol-001",
		MountTargetDevice: &Device{Name: "sdb", Type: "disk"},
	}

	require.NoError(t, PersistConnector(conn, filePath))
	assert.FileExists(t, filePath)
}
