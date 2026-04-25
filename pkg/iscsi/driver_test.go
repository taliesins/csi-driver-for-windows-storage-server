package iscsi

import (
	"os"
	"testing"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Driver tests
// ---------------------------------------------------------------------------

func TestNewDriver(t *testing.T) {
	d := NewDriver("node-001", "unix:///var/run/csi/csi.sock")

	assert.Equal(t, "iscsi.csi.windows.microsoft.com", d.name)
	assert.Equal(t, ProtocolISCSI, d.protocol)
	assert.Equal(t, "0.1.0", d.version)
	assert.Equal(t, "node-001", d.nodeID)
	assert.Equal(t, "unix:///var/run/csi/csi.sock", d.endpoint)
	require.NotNil(t, d.cap)
	assert.Len(t, d.cap, 3)
	require.NotNil(t, d.cscap)
	assert.Len(t, d.cscap, 7)
}

func TestNewProtocolDriver(t *testing.T) {
	tests := []struct {
		name       string
		protocol   Protocol
		wantDriver string
		wantCaps   int
	}{
		{name: "iscsi", protocol: ProtocolISCSI, wantDriver: "iscsi.csi.windows.microsoft.com", wantCaps: 3},
		{name: "nfs", protocol: ProtocolNFS, wantDriver: "nfs.csi.windows.microsoft.com", wantCaps: 6},
		{name: "smb", protocol: ProtocolSMB, wantDriver: "smb.csi.windows.microsoft.com", wantCaps: 6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewProtocolDriver(tt.protocol, "node-001", "unix:///var/run/csi/csi.sock")
			assert.Equal(t, tt.wantDriver, d.name)
			assert.Equal(t, tt.protocol, d.protocol)
			assert.Len(t, d.cap, tt.wantCaps)
		})
	}
}

func TestNewNamedDriver(t *testing.T) {
	d := NewNamedDriver("nfs.csi.windows.microsoft.com", "node-001", "unix:///var/run/csi/csi.sock")
	assert.Equal(t, "nfs.csi.windows.microsoft.com", d.name)
	assert.Equal(t, ProtocolNFS, d.protocol)
}

func TestNewNodeServer(t *testing.T) {
	d := NewDriver("node-001", "unix:///var/run/csi/csi.sock")
	ns := NewNodeServer(d)

	require.NotNil(t, ns)
	assert.Equal(t, d, ns.Driver)
}

func TestNewControllerServer(t *testing.T) {
	d := NewDriver("node-001", "unix:///var/run/csi/csi.sock")
	cs := NewControllerServer(d)

	require.NotNil(t, cs)
	assert.Equal(t, d, cs.Driver)
}

func TestAddVolumeCapabilityAccessModes(t *testing.T) {
	d := NewDriver("node-001", "unix:///var/run/csi/csi.sock")

	caps := d.AddVolumeCapabilityAccessModes([]csi.VolumeCapability_AccessMode_Mode{
		csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
		csi.VolumeCapability_AccessMode_SINGLE_NODE_MULTI_WRITER,
	})

	require.NotNil(t, caps)
	assert.Len(t, caps, 2)
	assert.Len(t, d.cap, 2)
}

func TestAddControllerServiceCapabilities(t *testing.T) {
	d := NewDriver("node-001", "unix:///var/run/csi/csi.sock")

	d.AddControllerServiceCapabilities([]csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME,
	})

	assert.Len(t, d.cscap, 2)
}

// ---------------------------------------------------------------------------
// volID encode/decode tests
// ---------------------------------------------------------------------------

func TestEncodeDecodeVolID(t *testing.T) {
	original := volID{
		VolumeName:   "k8s-csi-test-volume-001",
		TargetPortal: "10.0.0.1:3260",
		TargetIQN:    "iqn.2024-01.com.example:test-volume",
		LUN:          0,
		VHDXPath:     "D:\\vhdx\\test-volume.vhdx",
		SizeBytes:    1073741824,
	}

	encoded := encodeVolID(original)
	assert.NotEmpty(t, encoded)

	decoded, err := decodeVolID(encoded)
	require.NoError(t, err)
	assert.Equal(t, original, decoded)
}

func TestDecodeVolID_InvalidBase64(t *testing.T) {
	_, err := decodeVolID("not-valid-base64!!!")
	assert.Error(t, err)
}

func TestDecodeVolID_EmptyString(t *testing.T) {
	v, err := decodeVolID("")
	assert.Error(t, err)
	assert.Equal(t, volID{}, v)
}

func TestDecodeVolID_MalformedJSON(t *testing.T) {
	_, err := decodeVolID("eyJrZXkiOiAidmFsdWUifQ==") // valid base64 but not JSON
	assert.Error(t, err)
}

func TestDecodeVolID_InvalidJSON(t *testing.T) {
	_, err := decodeVolID("aW52YWxpZCBqc29u") // "invalid json" in base64
	assert.Error(t, err)
}

func TestEncodeDecode_SpecialChars(t *testing.T) {
	original := volID{
		VolumeName:   "vol-with-special_chars.123",
		TargetPortal: "192.168.1.100:3260",
		TargetIQN:    "iqn.2024-01.com.example:special-vol!@#",
		LUN:          255,
		VHDXPath:     "C:\\Program Files\\k8s\\csi\\vol.vhdx",
		SizeBytes:    999999999999,
	}

	encoded := encodeVolID(original)
	decoded, err := decodeVolID(encoded)
	require.NoError(t, err)
	assert.Equal(t, original, decoded)
}

// ---------------------------------------------------------------------------
// snapID encode/decode tests
// ---------------------------------------------------------------------------

func TestEncodeDecodeSnapID(t *testing.T) {
	original := snapID{
		SnapshotID:   "snap-001",
		OriginalPath: "D:\\vhdx\\test-volume.vhdx",
	}

	encoded := encodeSnapID(original)
	assert.NotEmpty(t, encoded)

	decoded, err := decodeSnapID(encoded)
	require.NoError(t, err)
	assert.Equal(t, original, decoded)
}

func TestDecodeSnapID_InvalidBase64(t *testing.T) {
	_, err := decodeSnapID("not-valid-base64!!!")
	assert.Error(t, err)
}

func TestDecodeSnapID_EmptyString(t *testing.T) {
	s, err := decodeSnapID("")
	assert.Error(t, err)
	assert.Equal(t, snapID{}, s)
}

func TestEncodeDecodeSnapID_SpecialChars(t *testing.T) {
	original := snapID{
		SnapshotID:   "snap-with-special_chars.123",
		OriginalPath: "C:\\Program Files\\k8s\\csi\\snap.vhdx",
	}

	encoded := encodeSnapID(original)
	decoded, err := decodeSnapID(encoded)
	require.NoError(t, err)
	assert.Equal(t, original, decoded)
}

// ---------------------------------------------------------------------------
// getStr helper test
// ---------------------------------------------------------------------------

func TestGetStr(t *testing.T) {
	tests := []struct {
		name    string
		m       map[string]string
		key     string
		wantVal string
		wantOK  bool
	}{
		{
			name:    "key exists",
			m:       map[string]string{"key": "value"},
			key:     "key",
			wantVal: "value",
			wantOK:  true,
		},
		{
			name:    "key does not exist",
			m:       map[string]string{"key": "value"},
			key:     "other",
			wantVal: "",
			wantOK:  false,
		},
		{
			name:    "empty map",
			m:       map[string]string{},
			key:     "key",
			wantVal: "",
			wantOK:  false,
		},
		{
			name:    "nil map",
			m:       nil,
			key:     "key",
			wantVal: "",
			wantOK:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, ok := getStr(tt.m, tt.key)
			assert.Equal(t, tt.wantVal, val)
			assert.Equal(t, tt.wantOK, ok)
		})
	}
}

// ---------------------------------------------------------------------------
// ensureFile tests
// ---------------------------------------------------------------------------

func TestEnsureFile(t *testing.T) {
	// Test with a temp directory
	tmpDir := t.TempDir()
	targetPath := tmpDir + "/test-file"

	err := ensureFile(targetPath)
	assert.NoError(t, err)

	info, err := os.Stat(targetPath)
	assert.NoError(t, err)
	assert.False(t, info.IsDir())
	assert.Equal(t, int64(0), info.Size())
}

func TestEnsureFile_AlreadyExists(t *testing.T) {
	tmpDir := t.TempDir()
	targetPath := tmpDir + "/test-file"

	// Create the file first
	f, err := os.Create(targetPath)
	assert.NoError(t, err)
	f.Close()

	// Calling again should succeed
	err = ensureFile(targetPath)
	assert.NoError(t, err)
}

// ---------------------------------------------------------------------------
// waitForPath tests
// ---------------------------------------------------------------------------

func TestNodeServer_waitForPath(t *testing.T) {
	ns, _, _ := newTestNodeServer(t)

	// Test with a path that exists (temp dir)
	tmpDir := t.TempDir()

	err := ns.waitForPath(tmpDir, 2*time.Second)
	assert.NoError(t, err)
}

func TestNodeServer_waitForPath_NonExistent(t *testing.T) {
	ns, _, _ := newTestNodeServer(t)

	// Test with a non-existent path and very short timeout
	err := ns.waitForPath("/nonexistent/path/that/does/not/exist", 100*time.Millisecond)
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// isBlockDevice tests
// ---------------------------------------------------------------------------

func TestIsBlockDevice(t *testing.T) {
	// Test with a temp file (not a block device)
	tmpDir := t.TempDir()
	tmpFile := tmpDir + "/test-file"
	err := os.WriteFile(tmpFile, []byte("test"), 0o644)
	assert.NoError(t, err)

	isBlock, err := isBlockDevice(tmpFile)
	assert.NoError(t, err)
	assert.False(t, isBlock)
}

func TestIsBlockDevice_NonExistent(t *testing.T) {
	isBlock, err := isBlockDevice("/nonexistent/device")
	assert.NoError(t, err)
	assert.False(t, isBlock)
}
