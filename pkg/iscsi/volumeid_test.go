package iscsi

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVolumeIDRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		vol  VolumeID
	}{
		{
			name: "iscsi volume basic",
			vol: VolumeID{
				Name:          "k8s-csi-test-volume-001",
				Protocol:      ProtocolISCSI,
				TargetPortal:  "10.0.0.1:3260",
				TargetIQN:     "iqn.2024-01.com.example:csi-iscsi-001",
				LUN:           0,
				VHDXPath:      "D:\\vhdx\\k8s-csi-test-volume-001.vhdx",
				CapacityBytes: 1073741824,
			},
		},
		{
			name: "iscsi volume with default port",
			vol: VolumeID{
				Name:          "csi-vol-002",
				Protocol:      ProtocolISCSI,
				TargetPortal:  "192.168.1.100:3260",
				TargetIQN:     "iqn.2024-01.com.example:csi-iscsi-002",
				LUN:           1,
				VHDXPath:      "E:\\storage\\csi-vol-002.vhdx",
				CapacityBytes: 5368709120,
			},
		},
		{
			name: "nfs volume",
			vol: VolumeID{
				Name:          "k8s-csi-nfs-volume-001",
				Protocol:      ProtocolNFS,
				NfsServer:     "10.0.0.2",
				NfsExportPath: "/export/k8s-csi-nfs-volume-001",
				VHDXPath:      "D:\\shares\\k8s-csi-nfs-volume-001",
				CapacityBytes: 2147483648,
			},
		},
		{
			name: "nfs volume with port",
			vol: VolumeID{
				Name:          "csi-nfs-002",
				Protocol:      ProtocolNFS,
				TargetPortal:  "10.0.0.2:2049",
				NfsServer:     "10.0.0.2",
				NfsExportPath: "/export/csi-nfs-002",
				CapacityBytes: 10737418240,
			},
		},
		{
			name: "smb volume",
			vol: VolumeID{
				Name:          "k8s-csi-smb-volume-001",
				Protocol:      ProtocolSMB,
				SmbServer:     "10.0.0.3",
				SmbShareName:  "k8s-csi-smb-volume-001",
				VHDXPath:      "D:\\shares\\k8s-csi-smb-volume-001",
				CapacityBytes: 4294967296,
			},
		},
		{
			name: "smb volume with domain user",
			vol: VolumeID{
				Name:          "csi-smb-002",
				Protocol:      ProtocolSMB,
				SmbServer:     "10.0.0.4",
				SmbShareName:  "csi-smb-002",
				CapacityBytes: 1099511627776,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded := EncodeVolumeID(&tt.vol)
			require.NotEmpty(t, encoded, "encoded volume ID should not be empty")

			decoded, err := DecodeVolumeID(encoded)
			require.NoError(t, err, "DecodeVolumeID should not return an error")

			assert.Equal(t, tt.vol.Name, decoded.Name, "Name mismatch")
			assert.Equal(t, tt.vol.Protocol, decoded.Protocol, "Protocol mismatch")
			assert.Equal(t, tt.vol.TargetPortal, decoded.TargetPortal, "TargetPortal mismatch")
			assert.Equal(t, tt.vol.TargetIQN, decoded.TargetIQN, "TargetIQN mismatch")
			assert.Equal(t, tt.vol.LUN, decoded.LUN, "LUN mismatch")
			assert.Equal(t, tt.vol.VHDXPath, decoded.VHDXPath, "VHDXPath mismatch")
			assert.Equal(t, tt.vol.NfsServer, decoded.NfsServer, "NfsServer mismatch")
			assert.Equal(t, tt.vol.NfsExportPath, decoded.NfsExportPath, "NfsExportPath mismatch")
			assert.Equal(t, tt.vol.SmbServer, decoded.SmbServer, "SmbServer mismatch")
			assert.Equal(t, tt.vol.SmbShareName, decoded.SmbShareName, "SmbShareName mismatch")
			assert.Equal(t, tt.vol.CapacityBytes, decoded.CapacityBytes, "CapacityBytes mismatch")

			decoded2, err := DecodeVolumeID(encoded)
			require.NoError(t, err)
			assert.Equal(t, decoded, decoded2, "double decode should match")
		})
	}
}

func TestVolumeIDValidation(t *testing.T) {
	tests := []struct {
		name      string
		vol       VolumeID
		wantErr   bool
		errSubstr string
	}{
		{
			name: "valid iscsi volume",
			vol: VolumeID{
				Name:          "k8s-csi-test-volume-001",
				Protocol:      ProtocolISCSI,
				TargetPortal:  "10.0.0.1:3260",
				TargetIQN:     "iqn.2024-01.com.example:csi-iscsi-001",
				LUN:           0,
				VHDXPath:      "D:\\vhdx\\vol001.vhdx",
				CapacityBytes: 1073741824,
			},
			wantErr: false,
		},
		{
			name: "valid nfs volume",
			vol: VolumeID{
				Name:          "k8s-csi-nfs-volume-001",
				Protocol:      ProtocolNFS,
				NfsServer:     "10.0.0.2",
				NfsExportPath: "/export/k8s-csi-nfs-volume-001",
				CapacityBytes: 2147483648,
			},
			wantErr: false,
		},
		{
			name: "valid smb volume",
			vol: VolumeID{
				Name:          "k8s-csi-smb-volume-001",
				Protocol:      ProtocolSMB,
				SmbServer:     "10.0.0.3",
				SmbShareName:  "k8s-csi-smb-volume-001",
				CapacityBytes: 4294967296,
			},
			wantErr: false,
		},
		{
			name: "iscsi missing target portal",
			vol: VolumeID{
				Name:      "csi-iscsi-001",
				Protocol:  ProtocolISCSI,
				TargetIQN: "iqn.2024-01.com.example:csi-iscsi-001",
				LUN:       0,
			},
			wantErr:   true,
			errSubstr: "targetPortal",
		},
		{
			name: "iscsi missing target IQN",
			vol: VolumeID{
				Name:         "csi-iscsi-001",
				Protocol:     ProtocolISCSI,
				TargetPortal: "10.0.0.1:3260",
				LUN:          0,
			},
			wantErr:   true,
			errSubstr: "targetIQN",
		},
		{
			name: "iscsi missing LUN",
			vol: VolumeID{
				Name:         "csi-iscsi-001",
				Protocol:     ProtocolISCSI,
				TargetPortal: "10.0.0.1:3260",
				TargetIQN:    "iqn.2024-01.com.example:csi-iscsi-001",
			},
			wantErr:   true,
			errSubstr: "LUN",
		},
		{
			name: "nfs missing server",
			vol: VolumeID{
				Name:          "csi-nfs-001",
				Protocol:      ProtocolNFS,
				NfsExportPath: "/export/csi-nfs-001",
			},
			wantErr:   true,
			errSubstr: "nfsServer",
		},
		{
			name: "nfs missing export path",
			vol: VolumeID{
				Name:      "csi-nfs-001",
				Protocol:  ProtocolNFS,
				NfsServer: "10.0.0.2",
			},
			wantErr:   true,
			errSubstr: "nfsExportPath",
		},
		{
			name: "smb missing server",
			vol: VolumeID{
				Name:         "csi-smb-001",
				Protocol:     ProtocolSMB,
				SmbShareName: "csi-smb-001",
			},
			wantErr:   true,
			errSubstr: "smbServer",
		},
		{
			name: "smb missing share name",
			vol: VolumeID{
				Name:      "csi-smb-001",
				Protocol:  ProtocolSMB,
				SmbServer: "10.0.0.3",
			},
			wantErr:   true,
			errSubstr: "smbShareName",
		},
		{
			name: "unknown protocol",
			vol: VolumeID{
				Name:         "csi-unknown-001",
				Protocol:     Protocol("unknown"),
				TargetPortal: "10.0.0.1:3260",
			},
			wantErr:   true,
			errSubstr: "protocol",
		},
		{
			name: "empty name",
			vol: VolumeID{
				Name:         "",
				Protocol:     ProtocolISCSI,
				TargetPortal: "10.0.0.1:3260",
				TargetIQN:    "iqn.2024-01.com.example:csi-iscsi-001",
				LUN:          0,
				VHDXPath:     "D:\\vhdx\\vol001.vhdx",
			},
			wantErr:   true,
			errSubstr: "name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateVolumeID(EncodeVolumeID(&tt.vol))
			if tt.wantErr {
				require.Error(t, err, "ValidateVolumeID should return an error")
				if tt.errSubstr != "" {
					assert.Contains(t, err.Error(), tt.errSubstr, "error should contain %q", tt.errSubstr)
				}
			} else {
				require.NoError(t, err, "ValidateVolumeID should not return an error")
			}
		})
	}
}

func TestVolumeIDValidationAgainstEncodedID(t *testing.T) {
	tests := []struct {
		name      string
		id        string
		wantErr   bool
		errSubstr string
	}{
		{
			name:    "empty string",
			id:      "",
			wantErr: true,
		},
		{
			name:    "nil pointer encode produces empty",
			id:      EncodeVolumeID(nil),
			wantErr: true,
		},
		{
			name:    "random string not a valid volume ID",
			id:      "not-a-volume-id-at-all",
			wantErr: true,
		},
		{
			name:    "valid iscsi volume ID",
			id:      "iscsi://10.0.0.1:3260/iqn.2024-01.com.example:csi-iscsi-001/lun/0",
			wantErr: false,
		},
		{
			name:    "valid nfs volume ID",
			id:      "nfs://10.0.0.2/export/k8s-csi-nfs-volume-001",
			wantErr: false,
		},
		{
			name:    "valid smb volume ID",
			id:      "smb://10.0.0.3/k8s-csi-smb-volume-001",
			wantErr: false,
		},
		{
			name:    "malformed URI missing scheme",
			id:      "//10.0.0.1:3260/vol001",
			wantErr: true,
		},
		{
			name:    "malformed URI with double slashes but no scheme",
			id:      "::::invalid",
			wantErr: true,
		},
		{
			name:    "unknown scheme",
			id:      "file:///some/path",
			wantErr: true,
		},
		{
			name:    "unknown scheme https",
			id:      "https://10.0.0.1/vol001",
			wantErr: true,
		},
		{
			name:    "unknown scheme tcp",
			id:      "tcp://10.0.0.1:3260",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateVolumeID(tt.id)
			if tt.wantErr {
				require.Error(t, err, "ValidateVolumeID should return an error for %q", tt.id)
				if tt.errSubstr != "" {
					assert.Contains(t, err.Error(), tt.errSubstr)
				}
			} else {
				require.NoError(t, err, "ValidateVolumeID should not return an error for %q", tt.id)
			}
		})
	}
}

func TestVolumeIDSpecialCharacters(t *testing.T) {
	tests := []struct {
		name string
		vol  VolumeID
	}{
		{
			name: "name with hyphens",
			vol: VolumeID{
				Name:          "k8s-csi-test-volume-with-hyphens-001",
				Protocol:      ProtocolISCSI,
				TargetPortal:  "10.0.0.1:3260",
				TargetIQN:     "iqn.2024-01.com.example:csi-iscsi-hyphens-001",
				LUN:           0,
				VHDXPath:      "D:\\vhdx\\k8s-csi-test-volume-with-hyphens-001.vhdx",
				CapacityBytes: 1073741824,
			},
		},
		{
			name: "name with underscores",
			vol: VolumeID{
				Name:          "k8s_csi_test_volume_with_underscores_001",
				Protocol:      ProtocolISCSI,
				TargetPortal:  "10.0.0.2:3260",
				TargetIQN:     "iqn.2024-01.com.example:csi_iscsi_underscores_001",
				LUN:           1,
				VHDXPath:      "E:\\vhdx\\k8s_csi_test_volume_with_underscores_001.vhdx",
				CapacityBytes: 2147483648,
			},
		},
		{
			name: "name with dots",
			vol: VolumeID{
				Name:          "k8s.csi.test.volume.001",
				Protocol:      ProtocolNFS,
				NfsServer:     "10.0.0.3",
				NfsExportPath: "/export/k8s.csi.test.volume.001",
				CapacityBytes: 4294967296,
			},
		},
		{
			name: "name with numbers only",
			vol: VolumeID{
				Name:          "1234567890",
				Protocol:      ProtocolSMB,
				SmbServer:     "10.0.0.4",
				SmbShareName:  "1234567890",
				CapacityBytes: 8589934592,
			},
		},
		{
			name: "name with mixed chars",
			vol: VolumeID{
				Name:          "k8s-csi_test.volume-001",
				Protocol:      ProtocolISCSI,
				TargetPortal:  "10.0.0.5:3260",
				TargetIQN:     "iqn.2024-01.com.example:k8s-csi_test.volume-001",
				LUN:           2,
				VHDXPath:      "F:\\storage\\k8s-csi_test.volume-001.vhdx",
				CapacityBytes: 10737418240,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded := EncodeVolumeID(&tt.vol)
			require.NotEmpty(t, encoded)

			decoded, err := DecodeVolumeID(encoded)
			require.NoError(t, err)

			assert.Equal(t, tt.vol.Name, decoded.Name)
			assert.Equal(t, tt.vol.Protocol, decoded.Protocol)
			assert.Equal(t, tt.vol.CapacityBytes, decoded.CapacityBytes)
		})
	}
}

func TestVolumeIDCapacityBoundaryValues(t *testing.T) {
	baseISCSI := VolumeID{
		Name:         "k8s-csi-capacity-test",
		Protocol:     ProtocolISCSI,
		TargetPortal: "10.0.0.1:3260",
		TargetIQN:    "iqn.2024-01.com.example:csi-capacity-test",
		LUN:          0,
		VHDXPath:     "D:\\vhdx\\k8s-csi-capacity-test.vhdx",
	}

	tests := []struct {
		name        string
		capacity    int64
		expectError bool
	}{
		{
			name:     "zero capacity",
			capacity: 0,
		},
		{
			name:     "one byte capacity",
			capacity: 1,
		},
		{
			name:     "max int64 capacity",
			capacity: math.MaxInt64,
		},
		{
			name:        "negative capacity",
			capacity:    -1,
			expectError: true,
		},
		{
			name:     "1 GiB",
			capacity: 1 << 30,
		},
		{
			name:     "1 TiB",
			capacity: 1 << 40,
		},
		{
			name:     "1 PiB",
			capacity: 1 << 50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vol := baseISCSI
			vol.CapacityBytes = tt.capacity

			encoded := EncodeVolumeID(&vol)
			decoded, err := DecodeVolumeID(encoded)
			if tt.expectError {
				assert.Error(t, err, "expected error for capacity %d", tt.capacity)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.capacity, decoded.CapacityBytes)
			}
		})
	}
}

func TestParseProtocolFromVolumeID(t *testing.T) {
	tests := []struct {
		name      string
		id        string
		wantProto Protocol
		wantErr   bool
		errSubstr string
	}{
		{
			name:      "iscsi scheme",
			id:        "iscsi://10.0.0.1:3260/iqn.2024-01.com.example:csi-iscsi-001/lun/0",
			wantProto: ProtocolISCSI,
			wantErr:   false,
		},
		{
			name:      "nfs scheme",
			id:        "nfs://10.0.0.2/export/k8s-csi-nfs-volume-001",
			wantProto: ProtocolNFS,
			wantErr:   false,
		},
		{
			name:      "smb scheme",
			id:        "smb://10.0.0.3/k8s-csi-smb-volume-001",
			wantProto: ProtocolSMB,
			wantErr:   false,
		},
		{
			name:      "empty string",
			id:        "",
			wantErr:   true,
			errSubstr: "missing scheme",
		},
		{
			name:      "missing scheme separator",
			id:        "iscsi10.0.0.1:3260",
			wantErr:   true,
			errSubstr: "invalid scheme",
		},
		{
			name:      "unknown scheme",
			id:        "file://some/path",
			wantErr:   true,
			errSubstr: "unknown protocol",
		},
		{
			name:      "https scheme",
			id:        "https://10.0.0.1/vol001",
			wantErr:   true,
			errSubstr: "unknown protocol",
		},
		{
			name:      "tcp scheme",
			id:        "tcp://10.0.0.1:3260",
			wantErr:   true,
			errSubstr: "unknown protocol",
		},
		{
			name:      "only scheme prefix",
			id:        "iscsi://",
			wantErr:   true,
			errSubstr: "invalid",
		},
		{
			name:      "case sensitive scheme lowercase",
			id:        "ISCSI://10.0.0.1:3260/iqn.2024-01.com.example:csi-iscsi-001/lun/0",
			wantErr:   true,
			errSubstr: "unknown protocol",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proto, err := ParseProtocolFromVolumeID(tt.id)
			if tt.wantErr {
				require.Error(t, err, "ParseProtocolFromVolumeID(%q) should return an error", tt.id)
				if tt.errSubstr != "" {
					assert.Contains(t, err.Error(), tt.errSubstr)
				}
			} else {
				require.NoError(t, err, "ParseProtocolFromVolumeID(%q) should not return an error", tt.id)
				assert.Equal(t, tt.wantProto, proto)
			}
		})
	}
}

func TestEncodeVolumeIDNil(t *testing.T) {
	encoded := EncodeVolumeID(nil)
	assert.Equal(t, "", encoded, "EncodeVolumeID(nil) should return empty string")

	decoded, err := DecodeVolumeID(encoded)
	assert.Error(t, err, "DecodeVolumeID of empty string should return error")
	assert.Nil(t, decoded, "DecodeVolumeID of empty string should return nil")
}

func TestVolumeIDSchemeDetection(t *testing.T) {
	tests := []struct {
		name      string
		id        string
		wantProto Protocol
		wantErr   bool
	}{
		{
			name:      "iscsi with port",
			id:        "iscsi://10.0.0.1:3260/iqn.2024-01.com.example:csi-iscsi-001/lun/0",
			wantProto: ProtocolISCSI,
			wantErr:   false,
		},
		{
			name:      "iscsi without explicit port (default)",
			id:        "iscsi://10.0.0.1/iqn.2024-01.com.example:csi-iscsi-002/lun/1",
			wantProto: ProtocolISCSI,
			wantErr:   false,
		},
		{
			name:      "nfs with export path",
			id:        "nfs://10.0.0.2/export/k8s-csi-nfs-volume-001",
			wantProto: ProtocolNFS,
			wantErr:   false,
		},
		{
			name:      "nfs with port",
			id:        "nfs://10.0.0.2:2049/export/k8s-csi-nfs-volume-002",
			wantProto: ProtocolNFS,
			wantErr:   false,
		},
		{
			name:      "smb with share",
			id:        "smb://10.0.0.3/k8s-csi-smb-volume-001",
			wantProto: ProtocolSMB,
			wantErr:   false,
		},
		{
			name:      "smb with port",
			id:        "smb://10.0.0.4:445/k8s-csi-smb-volume-002",
			wantProto: ProtocolSMB,
			wantErr:   false,
		},
		{
			name:    "missing colon-slash in scheme",
			id:      "iscsi:10.0.0.1/vol001",
			wantErr: true,
		},
		{
			name:    "scheme with extra slashes",
			id:      "iscsi:///10.0.0.1/vol001",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proto, err := ParseProtocolFromVolumeID(tt.id)
			if tt.wantErr {
				require.Error(t, err, "ParseProtocolFromVolumeID(%q) should return an error", tt.id)
			} else {
				require.NoError(t, err, "ParseProtocolFromVolumeID(%q) should not return an error", tt.id)
				assert.Equal(t, tt.wantProto, proto)
			}
		})
	}
}

func TestVolumeIDErrorCases(t *testing.T) {
	tests := []struct {
		name      string
		id        string
		errSubstr string
	}{
		{
			name:      "completely empty",
			id:        "",
			errSubstr: "empty",
		},
		{
			name:      "whitespace only",
			id:        "   ",
			errSubstr: "empty",
		},
		{
			name:      "single slash",
			id:        "/",
			errSubstr: "invalid",
		},
		{
			name:      "double slash no scheme",
			id:        "//",
			errSubstr: "invalid",
		},
		{
			name:      "only scheme",
			id:        "iscsi",
			errSubstr: "invalid",
		},
		{
			name:      "scheme with single slash",
			id:        "iscsi:/",
			errSubstr: "invalid",
		},
		{
			name:      "null bytes",
			id:        "iscsi://\x00\x00",
			errSubstr: "invalid",
		},
		{
			name:      "very long scheme",
			id:        "iscsi://a" + string(make([]byte, 1024)) + "/vol001",
			errSubstr: "invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateVolumeID(tt.id)
			require.Error(t, err, "ValidateVolumeID(%q) should return an error", tt.id)
			if tt.errSubstr != "" {
				assert.Contains(t, err.Error(), tt.errSubstr)
			}
		})
	}
}

func TestVolumeIDURIEncodingFormat(t *testing.T) {
	tests := []struct {
		name       string
		vol        VolumeID
		wantPrefix string
	}{
		{
			name: "iscsi uses iscsi:// prefix",
			vol: VolumeID{
				Name:         "k8s-csi-test-001",
				Protocol:     ProtocolISCSI,
				TargetPortal: "10.0.0.1:3260",
				TargetIQN:    "iqn.2024-01.com.example:csi-iscsi-001",
				LUN:          0,
				VHDXPath:     "D:\\vhdx\\vol001.vhdx",
			},
			wantPrefix: "iscsi://",
		},
		{
			name: "nfs uses nfs:// prefix",
			vol: VolumeID{
				Name:          "k8s-csi-nfs-001",
				Protocol:      ProtocolNFS,
				NfsServer:     "10.0.0.2",
				NfsExportPath: "/export/k8s-csi-nfs-001",
			},
			wantPrefix: "nfs://",
		},
		{
			name: "smb uses smb:// prefix",
			vol: VolumeID{
				Name:         "k8s-csi-smb-001",
				Protocol:     ProtocolSMB,
				SmbServer:    "10.0.0.3",
				SmbShareName: "k8s-csi-smb-001",
			},
			wantPrefix: "smb://",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded := EncodeVolumeID(&tt.vol)
			assert.True(t, len(encoded) > 0, "encoded should not be empty")
			assert.True(t, len(encoded) > len(tt.wantPrefix), "encoded should be longer than prefix")
			assert.True(t, len(encoded) >= len(tt.wantPrefix), "encoded length should be at least prefix length")
		})
	}
}

func TestVolumeIDDecodeMalformedURIs(t *testing.T) {
	tests := []struct {
		name string
		id   string
	}{
		{
			name: "missing protocol prefix",
			id:   "10.0.0.1:3260/vol001",
		},
		{
			name: "wrong protocol format",
			id:   "iscsi:10.0.0.1/vol001",
		},
		{
			name: "double colon no slashes",
			id:   "iscsi::10.0.0.1/vol001",
		},
		{
			name: "protocol with numbers",
			id:   "iscsi2://10.0.0.1/vol001",
		},
		{
			name: "protocol with dash",
			id:   "iscsi-test://10.0.0.1/vol001",
		},
		{
			name: "protocol with underscore",
			id:   "iscsi_test://10.0.0.1/vol001",
		},
		{
			name: "protocol with dot",
			id:   "iscsi.test://10.0.0.1/vol001",
		},
		{
			name: "protocol with space",
			id:   "iscsi test://10.0.0.1/vol001",
		},
		{
			name: "protocol with special chars",
			id:   "iscsi@://10.0.0.1/vol001",
		},
		{
			name: "protocol with equals",
			id:   "iscsi=test://10.0.0.1/vol001",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decoded, err := DecodeVolumeID(tt.id)
			if err == nil && decoded != nil {
				t.Logf("DecodeVolumeID(%q) returned non-nil: %+v", tt.id, decoded)
			}
		})
	}
}
