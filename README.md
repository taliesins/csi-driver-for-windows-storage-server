# CSI Driver for Windows File Storage Server

### Overview

This repository provides CSI drivers for dynamic provisioning of Windows File Storage Server volumes over **iSCSI**, **NFS**, and **SMB** protocols. The drivers support dynamic provisioning, snapshots, restore, online expansion, and static PV import.

### Supported Drivers

| Driver | CSI Plugin Name | Storage Class | Access Mode | Backing Store |
|--------|-----------------|---------------|-------------|---------------|
| iSCSI | `iscsi.csi.windows.microsoft.com` | `iscsi-for-windows-rwo` | RWO | VHDX + iSCSI target |
| NFS (directory) | `nfs.csi.windows.microsoft.com` | `nfs-for-windows-rwx` | RWX | Directories |
| NFS VHDX | `nfs-vhdx.csi.windows.microsoft.com` | `nfs-vhdx-for-windows-rwx` | RWX | One VHDX per volume |
| SMB (directory) | `smb.csi.windows.microsoft.com` | `smb-for-windows-rwx` | RWX | Directories |
| SMB VHDX | `smb-vhdx.csi.windows.microsoft.com` | `smb-vhdx-for-windows-rwx` | RWX | One VHDX per volume |

### Feature Matrix

| Feature | iSCSI | NFS (dir) | NFS VHDX | SMB (dir) | SMB VHDX |
| --- | --- | --- | --- | --- | --- |
| Dynamic provisioning | yes | yes | yes | yes | yes |
| Filesystem PVC | yes | yes | yes | yes | yes |
| RWX PVC | no | yes | yes | yes | yes |
| Arbitrary existing path import | no | yes | yes | yes | yes |
| Snapshots | yes | no | yes | no | yes |
| Restore from snapshot | yes | no | yes | no | yes |
| Online expansion | yes | yes | yes | yes | yes |
| Raw block volume | yes | no | no | no | no |
| CHAP or iSCSI initiator auth | yes | no | no | no | no |
| SMB mount credentials | no | no | no | yes | yes |
| Static PV import | yes | yes | yes | yes | yes |

#### Feature Descriptions

| Feature | Description |
|---------|-------------|
| Dynamic provisioning | Automatically create volumes on demand when a PVC is submitted, without manual pre-provisioning. |
| Filesystem PVC | Expose the provisioned volume as a mounted filesystem inside a pod (the default CSI volume mode). |
| RWX PVC | Allow the volume to be mounted as read-write by multiple pods simultaneously (only directory-backed NFS/SMB supports this). |
| Arbitrary existing path import | Import an existing directory or share on the Windows storage server as a PV, rather than creating a new one. |
| Snapshots | Create point-in-time snapshots of a volume. Only iSCSI and VHDX-backed drivers support this (they have access to the underlying VHDX). |
| Restore from snapshot | Recreate a volume from a previously taken snapshot. |
| Online expansion | Increase the size of an existing volume while it is in use, without unmounting. |
| Raw block volume | Expose the volume as a raw block device (`volumeMode: Block`) instead of a mounted filesystem. Only iSCSI supports this. |
| CHAP or iSCSI initiator auth | Use CHAP (Challenge-Handshake Authentication Protocol) or iSCSI initiator credentials to authenticate the connection to the iSCSI target. |
| SMB mount credentials | Automatically inject SMB username/password for node-stage mounting, so pods do not need to manage credentials themselves. |
| Static PV import | Manually create a PV that references an already-existing volume on the storage server. |

### Protocol Walkthroughs

- **iSCSI** — Provisions RWO iSCSI volumes backed by VHDX files and iSCSI targets.
- **NFS (directory)** — Provisions RWX NFS shares backed by normal directories.
- **NFS VHDX** — Provisions RWX NFS shares backed by one VHDX per volume.
- **SMB (directory)** — Provisions RWX SMB shares backed by normal directories.
- **SMB VHDX** — Provisions RWX SMB shares backed by one VHDX per volume.

Each protocol folder under [`examples/`](./examples/) starts with `01-storageclass/` and builds on the PVC names from the previous examples.

---

### Prerequisites

All drivers require a **Windows Server with Storage Server** setup.

#### Essentials (File & Storage, iSCSI Target, iSCSI Initiator)

```powershell
# Install core roles/features
Install-WindowsFeature -Name `
  FS-FileServer, `         # File Server
  Storage-Services, `      # Storage management service
  FS-iSCSITarget-Server, ` # iSCSI Target Server
  MSiSCSI `                # Microsoft iSCSI Initiator service
  -IncludeManagementTools -Verbose
```

#### Optional: Multipath IO (recommended for iSCSI with multiple target paths)

```powershell
Install-WindowsFeature -Name Multipath-IO -IncludeManagementTools -Verbose
```

#### Verify

```powershell
Get-WindowsFeature FS-FileServer,Storage-Services,FS-iSCSITarget-Server,MSiSCSI,Multipath-IO |
  Format-Table DisplayName, Name, InstallState
```

### Windows Server Bootstrap

Run the PowerShell setup script from an elevated PowerShell session on the Windows Server:

```powershell
$AllowedClient = "Any"   # Example: "203.0.113.10/32". Use "Any" only in an isolated lab.
$WinRMUser = "csi-winrm-test"
$StoragePath = "C:\data\taliesins\csi-driver-for-windows-storage-server\vhdx"
$CertDnsName = $env:COMPUTERNAME          # Or the DNS name you will use as WINRM_HOST.

.\deploy\install-windows-machine.ps1 `
  -AllowedClient $AllowedClient `
  -WinRMUser $WinRMUser `
  -StoragePath $StoragePath `
  -CertDnsName $CertDnsName
```

Use `-AllowedClient Any` and `-IscsiTargetRemoteAddress Any` only in an isolated lab. The script installs the iSCSI target features, creates the VHDX storage directory, configures a local WinRM admin user, enables WinRM HTTPS with Basic authentication on port `5986`, keeps unencrypted WinRM disabled, and opens iSCSI target port `3260`.

---

### Project status: Alpha

### Container Images & Kubernetes Compatibility

| Driver version | Supported k8s version | Status |
|----------------|-----------------------|--------|
| master branch  | 1.19+                 | alpha  |

---

### Development Environment

This repository includes a devcontainer with all required tools (Go, Docker, Make, pre-commit, etc.) pre-installed. If you open this project in GitHub Codespaces or a compatible devcontainer environment, you do not need to install any prerequisites manually.

If you are not using the devcontainer, ensure you have Go, Docker, and Make installed.

### Running Unit Tests

The default unit test suite is designed to run on Linux, Windows, macOS, CI runners, and the devcontainer. It does not require a Windows Server, WinRM credentials, an iSCSI target, Docker, or a Kubernetes cluster.

Prerequisites for a local machine:

- Go with toolchain auto-download enabled, or Go `1.26.2` installed directly
- Make, if you want to use the Makefile shortcut

Run the unit tests:

```sh
go test ./...
# or
make test
```

If your local Go command is older than the version in `go.mod`, enable toolchain auto-download or run with an explicit toolchain:

```sh
go env -w GOTOOLCHAIN=auto
# or for one command
GOTOOLCHAIN=go1.26.2 go test ./...
```

### Running Integration Tests

WinRM tests that need a real Windows Server are integration tests and are not required for the default unit test suite. To run them, configure a Windows host and use the `integration` build tag. Set `WINRM_TEST_PARENT_DIR` to a scratch directory where the tests can create and delete temporary iSCSI virtual disks. The virtual disk and snapshot lifecycle tests require administrator access to the Windows iSCSI Target service:

```sh
go test -tags=integration ./pkg/iscsi -run TestWinRMBackendIntegration -v
```

### Makefile Commands

The project provides a Makefile for common developer tasks.

| Command            | Description                                 |
|--------------------|---------------------------------------------|
| make build         | Build the driver binary                     |
| make test          | Run all Go tests                            |
| make lint          | Run pre-commit hooks (lint, format, etc.)   |
| make pre-commit    | Install pre-commit git hooks                |
| make image         | Build the Docker image                      |
| make release       | Run goreleaser to build and release         |
| make clean         | Clean up build artifacts                    |

#### Example: Build the driver

```sh
make build
```

#### Example: Run tests

```sh
make test
```

#### Example: Lint and format

```sh
make lint
```

#### Example: Build Docker image

```sh
make image
```

#### Example: Release with goreleaser

```sh
make release
```

### Install in a Local Kubernetes Cluster

You can install the driver in your local Kubernetes cluster (e.g., kind, minikube) using the provided scripts:

#### 1. Clone the repository (if not already done):

```sh
git clone https://github.com/taliesins/csi-driver-for-windows-storage-server.git
cd csi-driver-for-windows-storage-server
```

#### 2. (Optional) Build the driver image for local changes:

```sh
make image
# or to build and load into kind:
# kind load docker-image <your-image-name>
```

#### 3. Install the driver manifests:

```sh
./deploy/install-driver.sh master local
```

#### 4. Check the driver pods:

```sh
kubectl -n kube-system get pod -o wide -l app=csi-for-windows-server-node
```

#### 5. Uninstall the driver:

```sh
./deploy/uninstall-driver.sh master local
```

---

### Install driver on a Kubernetes cluster

- Install by [kubectl](./docs/install-iscsi-csi-driver.md)

### Troubleshooting

- [CSI driver troubleshooting guide](./docs/csi-debug.md)

### Kubernetes Development

Please refer to [development guide](./docs/csi-dev.md)

## Community, discussion, contribution, and support

You can reach the maintainers of this project at:

- [GitHub](https://github.com/taliesins/csi-driver-for-windows-storage-server/issues)
