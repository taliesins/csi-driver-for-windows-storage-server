package iscsi

import (
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestMain(m *testing.M) {
	m.Run()
}

func TestProtocolConstants(t *testing.T) {
	tests := []struct {
		name    string
		proto   Protocol
		wantStr string
	}{
		{"iscsi protocol constant", ProtocolISCSI, "iscsi"},
		{"nfs protocol constant", ProtocolNFS, "nfs"},
		{"smb protocol constant", ProtocolSMB, "smb"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.proto) != tt.wantStr {
				t.Errorf("Protocol %s string value = %q, want %q", tt.name, string(tt.proto), tt.wantStr)
			}
		})
	}
}

func TestBackendSelectorSelect(t *testing.T) {
	iscsiB := &IscsiBackend{}
	nfsB := &NfsBackend{}
	smbB := &SmbBackend{}

	tests := []struct {
		name     string
		selector *BackendSelector
		protocol Protocol
		wantNil  bool
		wantErr  bool
		errCode  codes.Code
	}{
		{
			name:     "select iscsi backend",
			selector: NewBackendSelector(iscsiB, nfsB, smbB),
			protocol: ProtocolISCSI,
			wantNil:  false,
			wantErr:  false,
		},
		{
			name:     "select nfs backend",
			selector: NewBackendSelector(iscsiB, nfsB, smbB),
			protocol: ProtocolNFS,
			wantNil:  false,
			wantErr:  false,
		},
		{
			name:     "select smb backend",
			selector: NewBackendSelector(iscsiB, nfsB, smbB),
			protocol: ProtocolSMB,
			wantNil:  false,
			wantErr:  false,
		},
		{
			name:     "nil iscsi backend",
			selector: NewBackendSelector(nil, nfsB, smbB),
			protocol: ProtocolISCSI,
			wantNil:  true,
			wantErr:  true,
			errCode:  codes.NotFound,
		},
		{
			name:     "nil nfs backend",
			selector: NewBackendSelector(iscsiB, nil, smbB),
			protocol: ProtocolNFS,
			wantNil:  true,
			wantErr:  true,
			errCode:  codes.NotFound,
		},
		{
			name:     "nil smb backend",
			selector: NewBackendSelector(iscsiB, nfsB, nil),
			protocol: ProtocolSMB,
			wantNil:  true,
			wantErr:  true,
			errCode:  codes.NotFound,
		},
		{
			name:     "unknown protocol",
			selector: NewBackendSelector(iscsiB, nfsB, smbB),
			protocol: Protocol("unknown"),
			wantNil:  true,
			wantErr:  true,
			errCode:  codes.InvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend, err := tt.selector.Select(tt.protocol)
			if tt.wantErr {
				if err == nil {
					t.Errorf("Select() expected error, got nil")
					return
				}
				if tt.errCode != 0 {
					s, ok := status.FromError(err)
					if !ok {
						t.Errorf("Select() error = %v, expected gRPC status with code %v", err, tt.errCode)
						return
					}
					if s.Code() != tt.errCode {
						t.Errorf("Select() error code = %v, want %v", s.Code(), tt.errCode)
					}
				}
				return
			}
			if err != nil {
				t.Errorf("Select() unexpected error = %v", err)
				return
			}
			if tt.wantNil && backend != nil {
				t.Errorf("Select() expected nil backend, got non-nil")
			}
			if !tt.wantNil && backend == nil {
				t.Errorf("Select() expected non-nil backend, got nil")
			}
		})
	}
}

func TestBackendSelectorGetProtocol(t *testing.T) {
	iscsiB := &IscsiBackend{}
	nfsB := &NfsBackend{}
	smbB := &SmbBackend{}

	tests := []struct {
		name      string
		selector  *BackendSelector
		volID     string
		wantProto Protocol
		wantErr   bool
	}{
		{
			name:      "detect iscsi protocol from volume ID",
			selector:  NewBackendSelector(iscsiB, nfsB, smbB),
			volID:     "iscsi://10.0.0.1:3260/iqn.2024-01.com.test:vol001/lun/0",
			wantProto: ProtocolISCSI,
			wantErr:   false,
		},
		{
			name:      "detect nfs protocol from volume ID",
			selector:  NewBackendSelector(iscsiB, nfsB, smbB),
			volID:     "nfs://10.0.0.2:/export/vol002",
			wantProto: ProtocolNFS,
			wantErr:   false,
		},
		{
			name:      "detect smb protocol from volume ID",
			selector:  NewBackendSelector(iscsiB, nfsB, smbB),
			volID:     "smb://10.0.0.3/share/vol003",
			wantProto: ProtocolSMB,
			wantErr:   false,
		},
		{
			name:      "unknown scheme in volume ID",
			selector:  NewBackendSelector(iscsiB, nfsB, smbB),
			volID:     "file://some/path",
			wantProto: "",
			wantErr:   true,
		},
		{
			name:      "empty volume ID",
			selector:  NewBackendSelector(iscsiB, nfsB, smbB),
			volID:     "",
			wantProto: "",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proto, err := tt.selector.GetProtocol(tt.volID)
			if tt.wantErr {
				if err == nil {
					t.Errorf("GetProtocol() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("GetProtocol() unexpected error = %v", err)
				return
			}
			if proto != tt.wantProto {
				t.Errorf("GetProtocol() = %v, want %v", proto, tt.wantProto)
			}
		})
	}
}

func TestVolumeInfoJSON(t *testing.T) {
	createdAt := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	vi := VolumeInfo{
		VolumeName:    "test-volume-001",
		Protocol:      ProtocolISCSI,
		TargetPortal:  "10.0.0.1:3260",
		TargetIQN:     "iqn.2024-01.com.test:vol001",
		LUN:           0,
		VHDXPath:      "D:\\vhdx\\vol001.vhdx",
		CapacityBytes: 1073741824,
		CreatedAt:     createdAt,
	}

	// Marshal
	data, err := vi.MarshalJSON()
	if err != nil {
		t.Fatalf("VolumeInfo.MarshalJSON() error = %v", err)
	}
	if len(data) == 0 {
		t.Fatal("VolumeInfo.MarshalJSON() returned empty data")
	}

	// Unmarshal
	var decoded VolumeInfo
	err = decoded.UnmarshalJSON(data)
	if err != nil {
		t.Fatalf("VolumeInfo.UnmarshalJSON() error = %v", err)
	}

	// Verify fields
	if decoded.VolumeName != vi.VolumeName {
		t.Errorf("VolumeName = %q, want %q", decoded.VolumeName, vi.VolumeName)
	}
	if decoded.Protocol != vi.Protocol {
		t.Errorf("Protocol = %v, want %v", decoded.Protocol, vi.Protocol)
	}
	if decoded.TargetPortal != vi.TargetPortal {
		t.Errorf("TargetPortal = %q, want %q", decoded.TargetPortal, vi.TargetPortal)
	}
	if decoded.TargetIQN != vi.TargetIQN {
		t.Errorf("TargetIQN = %q, want %q", decoded.TargetIQN, vi.TargetIQN)
	}
	if decoded.LUN != vi.LUN {
		t.Errorf("LUN = %d, want %d", decoded.LUN, vi.LUN)
	}
	if decoded.VHDXPath != vi.VHDXPath {
		t.Errorf("VHDXPath = %q, want %q", decoded.VHDXPath, vi.VHDXPath)
	}
	if decoded.CapacityBytes != vi.CapacityBytes {
		t.Errorf("CapacityBytes = %d, want %d", decoded.CapacityBytes, vi.CapacityBytes)
	}
}

func TestVolumeInfoInvalidJSON(t *testing.T) {
	var vi VolumeInfo
	err := vi.UnmarshalJSON([]byte("not valid json"))
	if err == nil {
		t.Error("UnmarshalJSON() expected error for invalid JSON, got nil")
	}
}

func TestSnapshotInfoValidation(t *testing.T) {
	tests := []struct {
		name    string
		snap    SnapshotInfo
		wantErr bool
	}{
		{
			name: "valid snapshot info",
			snap: SnapshotInfo{
				SnapshotID:   "snap-001",
				SourceVolume: "vol-001",
				Description:  "test snapshot",
				CreatedAt:    time.Now().UTC(),
				SizeBytes:    1073741824,
				ReadyToUse:   true,
			},
			wantErr: false,
		},
		{
			name: "empty snapshot ID",
			snap: SnapshotInfo{
				SnapshotID:   "",
				SourceVolume: "vol-001",
				CreatedAt:    time.Now().UTC(),
			},
			wantErr: true,
		},
		{
			name: "empty source volume",
			snap: SnapshotInfo{
				SnapshotID:   "snap-001",
				SourceVolume: "",
				CreatedAt:    time.Now().UTC(),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.snap.Validate()
			if tt.wantErr && err == nil {
				t.Error("Validate() expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Validate() unexpected error = %v", err)
			}
		})
	}
}

func TestIscsiBackendGetProtocol(t *testing.T) {
	backend := &IscsiBackend{}
	proto := backend.GetProtocol()
	if proto != ProtocolISCSI {
		t.Errorf("GetProtocol() = %v, want %v", proto, ProtocolISCSI)
	}
}

func TestNfsBackendGetProtocol(t *testing.T) {
	backend := &NfsBackend{}
	proto := backend.GetProtocol()
	if proto != ProtocolNFS {
		t.Errorf("GetProtocol() = %v, want %v", proto, ProtocolNFS)
	}
}

func TestSmbBackendGetProtocol(t *testing.T) {
	backend := &SmbBackend{}
	proto := backend.GetProtocol()
	if proto != ProtocolSMB {
		t.Errorf("GetProtocol() = %v, want %v", proto, ProtocolSMB)
	}
}

func TestNfsPermissionsConstants(t *testing.T) {
	tests := []struct {
		name string
		perm NfsPermissions
		want string
	}{
		{"read only", NfsPermReadOnly, "ro"},
		{"read write", NfsPermReadWrite, "rw"},
		{"root access", NfsPermRootAccess, "no_root_squash"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.perm) != tt.want {
				t.Errorf("NfsPermissions %s = %q, want %q", tt.name, string(tt.perm), tt.want)
			}
		})
	}
}

func TestSmbPermissionsConstants(t *testing.T) {
	tests := []struct {
		name string
		perm SmbPermissions
		want string
	}{
		{"full control", SmbPermFullControl, "FullControl"},
		{"read write", SmbPermReadWrite, "ReadWrite"},
		{"read only", SmbPermReadOnly, "ReadOnly"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.perm) != tt.want {
				t.Errorf("SmbPermissions %s = %q, want %q", tt.name, string(tt.perm), tt.want)
			}
		})
	}
}

func TestNfsShareInfoValidation(t *testing.T) {
	tests := []struct {
		name    string
		info    NfsShareInfo
		wantErr bool
	}{
		{
			name: "valid NFS share info",
			info: NfsShareInfo{
				Name:        "test-share",
				Path:        "/export/test",
				Permissions: NfsPermReadWrite,
				Clients:     []string{"10.0.0.1", "10.0.0.2"},
			},
			wantErr: false,
		},
		{
			name: "empty share name",
			info: NfsShareInfo{
				Name:        "",
				Path:        "/export/test",
				Permissions: NfsPermReadWrite,
			},
			wantErr: true,
		},
		{
			name: "empty export path",
			info: NfsShareInfo{
				Name:        "test-share",
				Path:        "",
				Permissions: NfsPermReadWrite,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.info.Validate()
			if tt.wantErr && err == nil {
				t.Error("Validate() expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Validate() unexpected error = %v", err)
			}
		})
	}
}

func TestSmbShareInfoValidation(t *testing.T) {
	tests := []struct {
		name    string
		info    SmbShareInfo
		wantErr bool
	}{
		{
			name: "valid SMB share info",
			info: SmbShareInfo{
				Name:   "test-share",
				Path:   "D:\\shares\\test",
				Server: "10.0.0.3",
				ACL: []SmbAclEntry{
					{User: "DOMAIN\\admin", Permissions: SmbPermFullControl},
				},
			},
			wantErr: false,
		},
		{
			name: "empty share name",
			info: SmbShareInfo{
				Name:   "",
				Path:   "D:\\shares\\test",
				Server: "10.0.0.3",
			},
			wantErr: true,
		},
		{
			name: "empty server",
			info: SmbShareInfo{
				Name:   "test-share",
				Path:   "D:\\shares\\test",
				Server: "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.info.Validate()
			if tt.wantErr && err == nil {
				t.Error("Validate() expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Validate() unexpected error = %v", err)
			}
		})
	}
}

func TestNfsExportInfoValidation(t *testing.T) {
	tests := []struct {
		name    string
		info    NfsExportInfo
		wantErr bool
	}{
		{
			name: "valid NFS export info",
			info: NfsExportInfo{
				Server:     "10.0.0.2",
				ExportPath: "/export/test",
				Clients:    []string{"10.0.0.1"},
			},
			wantErr: false,
		},
		{
			name: "empty server",
			info: NfsExportInfo{
				Server:     "",
				ExportPath: "/export/test",
			},
			wantErr: true,
		},
		{
			name: "empty export path",
			info: NfsExportInfo{
				Server:     "10.0.0.2",
				ExportPath: "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.info.Validate()
			if tt.wantErr && err == nil {
				t.Error("Validate() expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Validate() unexpected error = %v", err)
			}
		})
	}
}

func TestSmbMappingInfoValidation(t *testing.T) {
	tests := []struct {
		name    string
		info    SmbMappingInfo
		wantErr bool
	}{
		{
			name: "valid SMB mapping",
			info: SmbMappingInfo{
				Server: "10.0.0.3",
				Share:  "test-share",
				User:   "DOMAIN\\admin",
			},
			wantErr: false,
		},
		{
			name: "empty server",
			info: SmbMappingInfo{
				Server: "",
				Share:  "test-share",
			},
			wantErr: true,
		},
		{
			name: "empty share",
			info: SmbMappingInfo{
				Server: "10.0.0.3",
				Share:  "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.info.Validate()
			if tt.wantErr && err == nil {
				t.Error("Validate() expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Validate() unexpected error = %v", err)
			}
		})
	}
}
