# ISCSI CSI driver for Windows File Storage Server iSCSI

### Overview

This is a repository for iscsi for windows CSI driver, csi plugin name: `iscsi.csi.windows.microsoft.com`. It can dynamically create/resize/delete, snapshot/restore from snapshot, attach/mount, detach/unmount iSCSI volumes.

This driver requires a Windows Server with Storage Server setup to support iSCSI.

Essentials (File & Storage, iSCSI Target, iSCI Initiator)
```
# Install core roles/features
Install-WindowsFeature -Name `
  FS-FileServer, `         # File Server
  Storage-Services, `      # Storage management service
  FS-iSCSITarget-Server, ` # iSCSI Target Server
  MSiSCSI `                # Microsoft iSCSI Initiator service
  -IncludeManagementTools -Verbose
```

Optional: Mulipath IO (recommended if you’ll expose multiple target paths)
```
Install-WindowsFeature -Name Multipath-IO -IncludeManagementTools -Verbose
```

Verify
```
Get-WindowsFeature FS-FileServer,Storage-Services,FS-iSCSITarget-Server,MSiSCSI,Multipath-IO |
  Format-Table DisplayName, Name, InstallState
```

Windows Server bootstrap script:

Run the PowerShell setup script from an elevated PowerShell session on the Windows Server that will host the iSCSI target:

```powershell
$AllowedClient = "Any"   # Example: "203.0.113.10/32". Use "Any" only in an isolated lab.
$WinRMUser = "csi-winrm-test"
$StoragePath = "C:\data\taliesins\csi-driver-iscsi-for-windows2\vhdx"
$CertDnsName = $env:COMPUTERNAME          # Or the DNS name you will use as WINRM_HOST.

.\deploy\install-windows-machine.ps1 `
  -AllowedClient $AllowedClient `
  -WinRMUser $WinRMUser `
  -StoragePath $StoragePath `
  -CertDnsName $CertDnsName
```

Use `-AllowedClient Any` and `-IscsiTargetRemoteAddress Any` only in an isolated lab. The script installs the iSCSI target features, creates the VHDX storage directory, configures a local WinRM admin user, enables WinRM HTTPS with Basic authentication on port `5986`, keeps unencrypted WinRM disabled, and opens iSCSI target port `3260`.



### Project status: Alpha

### Container Images & Kubernetes Compatibility:

|driver version  | supported k8s version | status |
|----------------|-----------------------|--------|
|master branch   | 1.19+                 | alpha   |




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
git clone https://github.com/taliesins/csi-driver-iscsi-for-windows.git
cd csi-driver-iscsi-for-windows
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
kubectl -n kube-system get pod -o wide -l app=csi-iscsi-for-windows-node
```

#### 5. Uninstall the driver:
```sh
./deploy/uninstall-driver.sh master local
```

---

### Install driver on a Kubernetes cluster

- install by [kubectl](./docs/install-iscsi-csi-driver.md)

### Troubleshooting

- [CSI driver troubleshooting guide](./docs/csi-debug.md)

### Kubernetes Development

Please refer to [development guide](./docs/csi-dev.md)

## Community, discussion, contribution, and support

You can reach the maintainers of this project at:

- [Github](https://github.com/taliesins/csi-driver-iscsi-for-windows/issues)
