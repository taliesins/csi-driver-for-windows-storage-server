# Implementation Plan: Multi-Protocol CSI Driver Expansion

## Overview
Expand `csi-driver-for-windows-storage-server` from iSCSI-only to support **iSCSI + NFS + SMB** protocols on Windows Server 2025 Storage Server, with comprehensive test coverage.

## Architecture Decisions

### 1. Unified Driver (Not Split)
- Keep single codebase, single controller Deployment
- Protocol-specific backends: `IscsiBackend`, `NfsBackend`, `SmbBackend`
- Protocol selected via `protocol` StorageClass parameter
- Driver name: `storage.csi.windows.microsoft.com`

### 2. Backend Interface Split
- `CommonBackend` interface: shared CRUD ops (CreateVolume, DeleteVolume, snapshots, etc.)
- `IscsiBackend`: existing WinRMBackend methods + iSCSI-specific ops
- `NfsBackend`: NFS share management via `StorageFiles` PS module
- `SmbBackend`: SMB share management via `SmbShare` PS module
- `BackendSelector` returns protocol-specific backend

### 3. Volume ID Encoding
- Switch from base64 JSON to URI scheme: `iscsi://`, `nfs://`, `smb://`
- URI scheme = protocol discriminator
- JSON payload = fallback with protocol field

### 4. Node Server
- Linux node DaemonSet: iSCSI + NFS (hostNetwork for iSCSI)
- Windows node DaemonSet (new template): SMB (and optionally NFS)
- Protocol dispatch in `NodeStageVolume` / `NodePublishVolume`

### 5. Testing Strategy
- Unit tests: mock Backend, test all Controller/Node RPC paths
- CSI sanity: `csi-test/v5` framework for spec compliance
- Integration: Helm deploy + PVC lifecycle per protocol

---

## Phase 1: Backend Interface Split + NfsBackend

### Files to Create/Modify

#### `pkg/iscsi/backend.go` (NEW - Common interface + types)
```go
// Protocol type
type Protocol string
const (
    ProtocolISCSI Protocol = "iscsi"
    ProtocolNFS   Protocol = "nfs"
    ProtocolSMB   Protocol = "smb"
)

// CommonBackend - shared across all protocols
type CommonBackend interface {
    CreateVolume(ctx context.Context, params map[string]string) (*VolumeInfo, error)
    DeleteVolume(ctx context.Context, volID string) error
    CreateSnapshot(ctx context.Context, volID, description string) (*SnapshotInfo, error)
    DeleteSnapshot(ctx context.Context, snapshotID string) error
    ListSnapshots(ctx context.Context, volID string, maxEntries int32) (*ListSnapshotsResponse, error)
    GetCapacity(ctx context.Context, params map[string]string) (int64, error)
    ListVolumes(ctx context.Context, maxEntries int32, startToken string) (*ListVolumesResponse, error)
    ExpandVolume(ctx context.Context, volID string, capacityBytes int64) (int64, error)
}

// VolumeInfo - unified volume representation
type VolumeInfo struct {
    Name        string
    Protocol    Protocol
    CapacityBytes int64
    // iSCSI-specific
    TargetPortal string
    TargetIQN    string
    LUN          int32
    VHDXPath     string
    // NFS-specific
    NfsServer    string
    NfsExportPath string
    // SMB-specific
    SmbServer    string
    SmbShareName string
}

// SnapshotInfo - unified snapshot representation
type SnapshotInfo struct {
    SnapshotID     string
    SourceVolumeID string
    CreationTime   int64
    IsReady        bool
}
```

#### `pkg/iscsi/backend_selector.go` (NEW)
```go
type BackendSelector struct {
    iscsiBackend *IscsiBackend
    nfsBackend   *NfsBackend
    smbBackend   *SmbBackend
}

func NewBackendSelector(iscsi *IscsiBackend, nfs *NfsBackend, smb *SmbBackend) *BackendSelector
func (s *BackendSelector) Select(protocol Protocol) (Backend, error)
func (s *BackendSelector) GetProtocol(volID string) (Protocol, error)
```

#### `pkg/iscsi/backend_nfs.go` (NEW)
- `NfsBackend` struct with WinRM connection
- `CreateNfsShare(shareName, path, permissions)` → `New-NfsShare`
- `DeleteNfsShare(shareName)` → `Remove-NfsShare`
- `GetNfsShareAcl(shareName)` → `Get-NfsSharePermission`
- `SetNfsShareAcl(shareName, acl)` → `Grant-NfsSharePermission`
- `EnsureNfsServerComponent()` → check `Get-NfsServerComponent`
- `ExportNfsShare(shareName, exportPath, clients)` → configure NFS export
- `GetNfsExport(shareName)` → return server+export info

#### `pkg/iscsi/backend_smb.go` (NEW)
- `SmbBackend` struct with WinRM connection
- `CreateSmbShare(shareName, path, permissions)` → `New-SmbShare`
- `DeleteSmbShare(shareName)` → `Remove-SmbShare`
- `GetSmbShareAcl(shareName)` → `Get-SmbShareAccessControl`
- `SetSmbShareAcl(shareName, acl)` → `Set-SmbShareAccessControl`
- `CreateSmbGlobalMapping(server, share, user, pass)` → `New-SmbGlobalMapping`
- `RemoveSmbGlobalMapping(server, share)` → `Remove-SmbGlobalMapping`
- `GetSmbConnection(server, share)` → `Get-SmbConnection`

### Files to Modify

#### `pkg/iscsi/backend_winrm.go`
- Rename `WinRMBackend` → `IscsiBackend` (or keep as-is, it's iSCSI-specific)
- Extract common `runPS()` helper
- Implement `CommonBackend` interface methods

#### `pkg/iscsi/iscsi.go`
- Update `Connector` struct if needed
- Keep all existing iSCSI device management logic

---

## Phase 2: Volume ID URI Encoding + ControllerServer Protocol Dispatch

### Files to Create/Modify

#### `pkg/iscsi/volumeid.go` (NEW)
```go
const (
    SchemeISCSI = "iscsi"
    SchemeNFS   = "nfs"
    SchemeSMB   = "smb"
)

type VolumeID struct {
    Name       string
    Protocol   Protocol
    // iSCSI
    TargetPortal string
    TargetIQN    string
    LUN          int32
    VHDXPath     string
    // NFS
    NfsServer    string
    NfsExportPath string
    // SMB
    SmbServer    string
    SmbShareName string
    // Common
    CapacityBytes int64
}

func DecodeVolumeID(id string) (*VolumeID, error)
func EncodeVolumeID(v *VolumeID) string
func ValidateVolumeID(id string) error
```

#### `pkg/iscsi/controllerserver.go`
- Update `CreateVolume` to dispatch by protocol
- Update `decodeVolID` → `volumeid.DecodeVolumeID`
- Update `CreateSnapshot` to handle all protocols
- Update `DeleteSnapshot` to handle all protocols
- Implement `ListVolumes` properly
- Implement `ControllerGetVolume` properly
- Add `CreateVolumeContentSource` for cloning
- Add `ValidateVolumeCapabilities` per protocol

#### `pkg/iscsi/nodeserver.go`
- Add protocol detection in `NodeStageVolume`
- Add `mountNfs(devicePath, stagingPath, options)` for Linux
- Add `mountSmb(devicePath, stagingPath, options)` for Windows
- Update `NodeExpandVolume` to handle NFS/SMB (no-op)

---

## Phase 3-6: Comprehensive Unit Tests

### Test Structure
```
pkg/iscsi/
  backend_test.go              # CommonBackend interface tests
  backend_winrm_test.go        # IscsiBackend tests
  backend_nfs_test.go          # NfsBackend tests
  backend_smb_test.go          # SmbBackend tests
  backend_selector_test.go     # BackendSelector tests
  controllerserver_test.go     # ControllerServer RPC tests
  nodeserver_test.go           # NodeServer RPC tests
  volumeid_test.go             # Encode/decode round-trip tests
  driver_test.go               # Driver creation tests

internal/
  mock/
    mock_backend.go            # Generated mock
    mock_winrm.go              # WinRM mock
```

### Test Coverage Matrix

| Component | Test Categories | Coverage Target |
|-----------|----------------|-----------------|
| Backend (iSCSI) | Create/Delete volume, snapshots, expand, error paths | 80%+ |
| Backend (NFS) | Create/Delete share, ACL, export, server component | 80%+ |
| Backend (SMB) | Create/Delete share, ACL, global mapping | 80%+ |
| BackendSelector | Protocol selection, error handling | 90%+ |
| ControllerServer | All RPCs (happy path + error paths) | 80%+ |
| NodeServer | All RPCs (all protocols) | 80%+ |
| VolumeID | Encode/decode all protocols, validation | 95%+ |
| Driver | Creation, validation, capabilities | 90%+ |

### Test Categories Per Component

#### Backend Tests
- Happy path: create → get → delete
- Idempotency: double create, double delete
- Error paths: connection failure, PS error, permission denied
- Edge cases: empty name, special chars, very long names
- Snapshot lifecycle: create → list → delete
- Expand: resize to larger, same size, smaller (error)

#### ControllerServer Tests
- CreateVolume: all protocols, all access modes, all capacity modes
- CreateVolume: from snapshot, from volume (clone)
- CreateVolume: validation failures (missing params, invalid capacity)
- CreateVolume: idempotency (same name twice)
- DeleteVolume: existing, non-existing, with snapshots
- ControllerPublishVolume: all protocols, all access modes
- ControllerUnpublishVolume: existing, non-existing
- NodeStageVolume: all protocols, all volume modes
- NodePublishVolume: all protocols, all fs types
- ControllerExpandVolume: happy path, validation, node expansion required
- Create/Delete/ListSnapshots: all protocols
- ValidateVolumeCapabilities: valid caps, invalid caps, mixed
- GetCapacity: with/without storage pool
- ListVolumes: with/without max entries, pagination
- ControllerGetVolume: existing, non-existing
- GetPluginCapabilities: verification

#### NodeServer Tests
- NodeGetInfo: correct node ID, topology
- NodeGetCapabilities: verify all advertised caps
- NodeStageVolume: all protocols, block + filesystem modes
- NodeStageVolume: validation failures
- NodePublishVolume: all protocols, all access modes
- NodePublishVolume: validation failures
- NodeUnstageVolume: existing, non-existing
- NodeUnpublishVolume: existing, non-existing
- NodeGetVolumeStats: valid path, invalid path
- NodeExpandVolume: all protocols
- NodeExpandVolume: validation failures

#### VolumeID Tests
- Encode → decode round-trip per protocol
- URI scheme detection
- Validation: missing fields, invalid schemes
- Special characters in names
- Capacity boundary values (0, 1, max int64)

---

## Phase 7: CSI Sanity Tests

### Setup
- Use `github.com/kubernetes-csi/csi-test/v5/pkg/sanity`
- Create `tests/sanity/sanity_test.go`
- Run against mock backend (for CI) and real backend (for manual)

### Sanity Test Configuration
```go
config := &sanity.Config{
    Address:         addr,
    TargetPath:      "/tmp/csi/testmount",
    StagingPath:     "/tmp/csi/staging",
    TestVolumeID:    "test-volume",
    TestVolumeSize:  int64(1024 * 1024 * 1024 * 10), // 10GB
    SecretReqFile:   "", // optional CHAP secrets
    SecretRepFile:   "", // optional replication secrets
}
```

### Sanity Test Categories
- IdentityService: GetPluginInfo, Probe, GetPluginCapabilities
- ControllerService: Create/Delete/ControllerPublish/ControllerUnpublish/Expand/List volumes
- ControllerService: Create/Delete/List snapshots
- ControllerService: ValidateVolumeCapabilities, GetCapacity, ControllerGetVolume
- NodeService: Stage/Publish/Unstage/Unpublish/GetVolumeStats/Expand
- NodeService: NodeGetInfo, NodeGetCapabilities

---

## Phase 8-11: Protocol Expansion

### Phase 8: NodeServer NFS Support (Linux)
- `mountNfs(server, exportPath, stagingPath, options)`
- NFS mount options: `nfsvers=4.1`, `hard`, `timeo=600`, `retrans=3`
- Update `GetCapacity` to query NFS server

### Phase 9: SmbBackend + Windows Node DaemonSet
- `backend_smb.go` implementation
- `daemonset-node-windows.yaml` Helm template
- SMB mount via `net use` or `New-SmbGlobalMapping`

### Phase 10: RWX Access Modes + Helm Updates
- NFS: `MULTI_NODE_READER_ONLY`
- SMB: `MULTI_NODE_ALL`
- Update Helm chart: `protocols` config, separate node DaemonSets

### Phase 11: Volume Cloning
- `CreateVolume` from `ContentSource: Volume`
- iSCSI: `Copy-VHD` on Windows Storage Server
- NFS/SMB: `Copy-Item` on Windows Storage Server

---

## Implementation Order

| Phase | Scope | Effort | Dependencies |
|-------|-------|--------|-------------|
| 1 | Backend interface split + NfsBackend | Medium | None |
| 2 | Volume ID URI + Controller dispatch | Medium | Phase 1 |
| 3 | ControllerServer unit tests | Medium | Phase 1 |
| 4 | NodeServer unit tests | Medium | Phase 1 |
| 5 | Backend unit tests | Medium | Phase 1 |
| 6 | VolumeID tests | Short | Phase 2 |
| 7 | CSI sanity framework | Medium | Phases 1-6 |
| 8 | NodeServer NFS support | Short | Phase 7 |
| 9 | SmbBackend + Windows DaemonSet | Large | Phase 7 |
| 10 | RWX + Helm updates | Medium | Phases 8-9 |
| 11 | Volume cloning | Medium | Phase 10 |
