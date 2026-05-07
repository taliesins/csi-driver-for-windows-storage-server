# CSI Driver for Windows File Storage Server

### Overview

This repository provides a consolidated CSI driver for dynamic provisioning of Windows File Storage Server volumes over **iSCSI**, **NFS**, and **SMB** protocols. The driver supports dynamic provisioning, snapshots, restore, online expansion, and static PV import.

### Supported Modes

All modes use the CSI plugin name `windows-storage.csi.windows.microsoft.com`. StorageClasses select the backend with the `protocol` parameter and, for NFS/SMB, the optional `shareBackend` parameter.

| Mode | Storage Class | Parameters | Access Mode | Backing Store |
|--------|-----------------|---------------|-------------|---------------|
| iSCSI | `iscsi-for-windows-rwo` | `protocol: iscsi` | RWO | VHDX + iSCSI target |
| NFS (directory) | `nfs-for-windows-rwx` | `protocol: nfs`, `shareBackend: directory` | RWX | Directories |
| NFS VHDX | `nfs-vhdx-for-windows-rwx` | `protocol: nfs`, `shareBackend: vhdx` | RWX | One VHDX per volume |
| SMB (directory) | `smb-for-windows-rwx` | `protocol: smb`, `shareBackend: directory` | RWX | Directories |
| SMB VHDX | `smb-vhdx-for-windows-rwx` | `protocol: smb`, `shareBackend: vhdx` | RWX | One VHDX per volume |

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
| NFS Kerberos authentication | no | yes | yes | no | no |
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
| NFS Kerberos authentication | Create Windows NFS shares with `sys`, `krb5`, `krb5i`, or `krb5p` authentication and mount them from Linux nodes with the matching `sec=` option. |
| SMB mount credentials | Automatically inject SMB username/password for node-stage mounting, so pods do not need to manage credentials themselves. |
| Static PV import | Manually create a PV that references an already-existing volume on the storage server. |

### Protocol Walkthroughs

- **iSCSI** — Provisions RWO iSCSI volumes backed by VHDX files and iSCSI targets. `iqnPrefix` is optional; when omitted, the driver uses the Windows target name and reads back the Windows-generated `TargetIqn`.
- **NFS (directory)** — Provisions RWX NFS shares backed by normal directories. Kerberos is optional with `krb5`, `krb5i`, or `krb5p`.
- **NFS VHDX** — Provisions RWX NFS shares backed by one VHDX per volume. Kerberos is optional with `krb5`, `krb5i`, or `krb5p`.
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

#### Kubernetes Node iSCSI Setup

For iSCSI volumes, every Linux node that can run pods using the driver must have
open-iscsi installed, `iscsid` available, and a stable initiator name in
`/etc/iscsi/initiatorname.iscsi`.

```sh
sudo cat /etc/iscsi/initiatorname.iscsi
# InitiatorName=iqn.1993-08.org.debian:01:...
```

The Helm chart reads this file from the host and uses that initiator IQN as the
CSI node ID. When a pod moves to another node, Kubernetes attach calls cause the
controller to remove the old node initiator from the Windows target and add the
new node initiator before the node logs in.

#### Optional: iSCSI CHAP

iSCSI CHAP uses one Secret shared across the controller and node paths. The
controller uses it to configure Windows target CHAP and reverse CHAP; the node
uses it to configure Linux open-iscsi discovery and login.

```yaml
parameters:
  csi.storage.k8s.io/provisioner-secret-name: "iscsi-chap"
  csi.storage.k8s.io/provisioner-secret-namespace: "${pvc.namespace}"
  csi.storage.k8s.io/controller-publish-secret-name: "iscsi-chap"
  csi.storage.k8s.io/controller-publish-secret-namespace: "${pvc.namespace}"
  csi.storage.k8s.io/node-stage-secret-name: "iscsi-chap"
  csi.storage.k8s.io/node-stage-secret-namespace: "${pvc.namespace}"
```

The Windows target side is configured from `node.session.auth.username` and
`node.session.auth.password`. Mutual CHAP uses
`node.session.auth.username_in` and `node.session.auth.password_in` for reverse
CHAP. Discovery CHAP keys configure Linux open-iscsi discovery; Windows Server
iSCSI target CHAP is configured per target.

### Windows Server Bootstrap

Run the PowerShell setup script from an elevated PowerShell session on the Windows Server:

```powershell
$AllowedClient = "Any"   # Example: "203.0.113.10/32". Use "Any" only in an isolated lab.
$WinRMHost = "win-storage.lab.local"      # DNS name or IP address the Kubernetes cluster can reach.
$WinRMUser = "csi-winrm-test"
$WinRMPassword = Read-Host "Password for $WinRMUser" -AsSecureString
$StoragePath = "C:\data\taliesins\csi-driver-for-windows-storage-server\vhdx"
$CertDnsName = $WinRMHost                 # Use the same name you will set as winrm.host.

.\deploy\install-windows-machine.ps1 `
  -AllowedClient $AllowedClient `
  -WinRMUser $WinRMUser `
  -WinRMPassword $WinRMPassword `
  -StoragePath $StoragePath `
  -CertDnsName $CertDnsName
```

Use `-AllowedClient Any` and `-IscsiTargetRemoteAddress Any` only in an isolated lab. The script installs the iSCSI target features, creates the VHDX storage directory, records it as the Windows machine environment variable `CSI_VHDX_PARENT_PATH`, configures a local WinRM admin user, enables WinRM HTTPS with Basic authentication on port `5986`, keeps unencrypted WinRM disabled, and opens iSCSI target port `3260`.

Use the same WinRM details when installing the Helm chart. The chart passes them only to the controller pod; node pods mount iSCSI, NFS, or SMB volumes locally and do not need the Windows admin WinRM credentials.

| Helm value | Source |
|------------|--------|
| `winrm.host` | `$WinRMHost`, the DNS name or IP address reachable from the Kubernetes cluster |
| `winrm.user` | `$WinRMUser` |
| `winrm.password` | The password entered for `$WinRMPassword` |
| `winrm.port` | `5986` |
| `winrm.tls` | `true` |
| `winrm.insecure` | `true` when using the bootstrap script's self-signed certificate |

#### Optional: NFS Kerberos

Kerberos for NFS does not use a password in the StorageClass. The Windows server
and Linux nodes authenticate through your Kerberos/Active Directory setup. Use
`krb5` for authentication, `krb5i` for authentication plus integrity, or
`krb5p` for authentication plus privacy.

Before enabling it:

- Join the Windows storage server to the domain that provides Kerberos.
- Configure the required NFS service principal/SPN for the storage server, for example `nfs/win-storage.lab.local`.
- Configure every Linux node that can run NFS pods with Kerberos and NFS client support. The install script expects the host files to be available at `/etc/krb5.conf` and `/etc/krb5.keytab`; the node pods read them through the existing `/host` mount.

Run the Windows bootstrap with Kerberos enabled:

```powershell
.\deploy\install-windows-machine.ps1 `
  -AllowedClient $AllowedClient `
  -WinRMUser $WinRMUser `
  -WinRMPassword $WinRMPassword `
  -StoragePath $StoragePath `
  -CertDnsName $CertDnsName `
  -EnableNfsKerberos `
  -NfsKerberosFlavor krb5p
```

Validate the Windows NFS share path and authentication setting with a disposable
test share:

```powershell
.\deploy\validate-windows-machine.ps1 `
  -StoragePath $StoragePath `
  -SharePath "C:\data\taliesins\csi-driver-for-windows-storage-server\shares" `
  -NfsKerberos `
  -NfsKerberosFlavor krb5p
```

When installing the manifest-based drivers, enable the NFS Kerberos node
environment:

```sh
./deploy/install-driver.sh master local --nfs-kerberos --nfs-kerberos-flavor krb5p
```

This sets the NFS and NFS VHDX node daemonsets to use the host Kerberos files:

```text
KRB5_CONFIG=/host/etc/krb5.conf
KRB5_KTNAME=FILE:/host/etc/krb5.keytab
KRB5_CLIENT_KTNAME=FILE:/host/etc/krb5.keytab
```

Use matching StorageClass parameters for NFS or NFS VHDX:

```yaml
parameters:
  nfsAuthentication: "krb5p"
  nfsMountAuthentication: "krb5p"
```

`nfsAuthentication` controls the Windows NFS share authentication setting.
`nfsMountAuthentication` controls the Linux mount option and becomes
`sec=krb5p` in this example. If you set multiple authentication values such as
`"sys,krb5p"`, set `nfsMountAuthentication` explicitly to the Kerberos flavor
the nodes should mount with. As a shorthand, `nfsKerberos: "true"` is equivalent
to `nfsAuthentication: "krb5"` and `nfsMountAuthentication: "krb5"`.

Uninstall accepts the same flags, although no separate Kerberos cleanup is
required because the daemonsets are deleted:

```sh
./deploy/uninstall-driver.sh master local --nfs-kerberos --nfs-kerberos-flavor krb5p
```

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

#### Build the driver

```sh
make build
```

#### Run tests

```sh
make test
```

#### Lint and format

```sh
make lint
```

#### Build Docker image

```sh
make image
```

#### Release with goreleaser

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

The manifest installer creates the controller WinRM Secret from environment
variables when `Secret/kube-system/csi-driver-winrm` does not already exist:

```sh
export WINRM_HOST=win-storage.lab.local
export WINRM_USER=csi-winrm-test
export WINRM_PASSWORD='<password>'
# Optional: override the PowerShell module import used by the WinRM backend.
# export WINRM_PS_IMPORT='Import-Module IscsiTarget'

./deploy/install-driver.sh master local
```

For static, pre-provisioned volumes where Windows storage is already configured,
install only the Linux node side:

```sh
./deploy/install-driver.sh master local --node-only
```

Node-only mode skips the controller, WinRM Secret, and Windows-side setup. It
also disables CSI attach for iSCSI, so static iSCSI PVs must include
`targetPortal`, `iqn` or `targetIQN`, and `lun` in `volumeAttributes`, or encode
those values in `volumeHandle` as an `iscsi://.../lun/<n>` URI. The Windows
iSCSI target must already allow every Linux node initiator that may run the pod.

#### 4. Check the driver pods:

```sh
kubectl -n kube-system get pods -o wide \
  -l app
```

#### 5. Uninstall the driver:

```sh
./deploy/uninstall-driver.sh master local
```

---

### Install driver on a Kubernetes cluster

- Install by [kubectl](./docs/install-csi-driver-master.md)
- Install by [Helm](./docs/install-csi-driver-master.md)

### Install via Helm (OCI / GHCR)

The chart is published as an OCI artifact to GHCR at `oci://ghcr.io/taliesins/helm/csi-driver-for-windows-storage-server`.

#### 1. Install from GHCR

WinRM connection details are required for the controller. Set `winrm.host` to the Windows storage server DNS name or IP address reachable from the Kubernetes cluster, and set `winrm.user` / `winrm.password` to the account configured by the Windows bootstrap script.

By default, the chart deploys controller, node, and CSIDriver objects for all
five drivers: iSCSI, NFS, NFS VHDX, SMB, and SMB VHDX. Disable individual
drivers with `drivers.<name>.enabled=false` when you only need a subset.
For static, pre-provisioned volumes, set `nodeOnly=true` to skip all controller
and WinRM setup and install only the Linux node side.

```sh
helm upgrade --install --create-namespace csi-driver-for-windows-storage-server oci://ghcr.io/taliesins/helm/csi-driver-for-windows-storage-server -n=kube-system \
  --set winrm.host=win-storage.lab.local \
  --set winrm.user=csi-winrm-test \
  --set-string winrm.password='<password>'
```

Node-only Helm install:

```sh
helm upgrade --install --create-namespace csi-driver-for-windows-storage-server oci://ghcr.io/taliesins/helm/csi-driver-for-windows-storage-server -n=kube-system \
  --set nodeOnly=true
```

In node-only mode, dynamic provisioning is disabled because no controller is
running. Static iSCSI PVs must point at an existing Windows target and LUN, and
the Windows target must already permit the Linux node initiators or use CHAP.

#### 2. Specify a version

```sh
helm upgrade --install --create-namespace csi-driver-for-windows-storage-server oci://ghcr.io/taliesins/helm/csi-driver-for-windows-storage-server --version=1.0.0 -n=kube-system \
  --set winrm.host=win-storage.lab.local \
  --set winrm.user=csi-winrm-test \
  --set-string winrm.password='<password>'
```

#### 3. Customize values (optional)

Override any value from [`values.yaml`](./chart/csi-driver-for-windows-storage-server/values.yaml) by putting your changes in a values file, for example `csi-driver-overrides.yaml`:

```yaml
image:
  tag: 1.0.0
  pullPolicy: Always

winrm:
  host: win-storage.lab.local
  port: 5986
  tls: true
  insecure: true
  existingSecret: csi-driver-winrm
```

To run more than one controller replica, set `controller.replicas` above `1`. The chart enables Kubernetes Lease leader election only when multiple controller replicas are requested, so a single-replica controller does not need lease permissions:

```yaml
controller:
  replicas: 2
```

Only the elected controller serves controller RPCs and renews the lease. Node pods do not use leader election.

For iSCSI node identity, the chart defaults to reading
`/etc/iscsi/initiatorname.iscsi` from each Kubernetes node. Override
`node.initiatorNamePath` only if your node image stores the open-iscsi initiator
name somewhere else.

When using `winrm.existingSecret`, create the WinRM credentials secret before installing:

```sh
kubectl -n kube-system create secret generic csi-driver-winrm \
  --from-literal=WINRM_USER=csi-winrm-test \
  --from-literal=WINRM_PASSWORD='<password>'
```

Alternatively, let the chart create the credentials secret:

```yaml
winrm:
  host: win-storage.lab.local
  port: 5986
  tls: true
  insecure: true
  user: csi-winrm-test
  password: "<password>"
```

```sh
helm upgrade --install --create-namespace csi-driver-for-windows-storage-server oci://ghcr.io/taliesins/helm/csi-driver-for-windows-storage-server --version=1.0.0 -n=kube-system -f csi-driver-overrides.yaml
```

#### 4. Verify the installation

```sh
helm status csi-driver-for-windows-storage-server -n=kube-system

kubectl get pods -n kube-system -l app.kubernetes.io/instance=csi-driver-for-windows-storage-server
```

#### 5. Upgrade / uninstall

```sh
# Upgrade
helm upgrade csi-driver-for-windows-storage-server oci://ghcr.io/taliesins/helm/csi-driver-for-windows-storage-server -n=kube-system --reuse-values

# Uninstall
helm uninstall csi-driver-for-windows-storage-server -n=kube-system
```

### Install via Helm (local chart)

For development or air-gapped environments, you can install from the local chart directory:

```sh
helm upgrade --install --create-namespace csi-driver-for-windows-storage-server ./chart/csi-driver-for-windows-storage-server -n=kube-system
```

### Build and publish

CI builds and validates pull requests, publishes `dev-<sha>` and `dev` container/chart builds from `main`, and publishes release containers/charts from `v*` tags.

```sh
make image IMAGE_TAG=dev
make image-push IMAGE_TAG=dev
make chart-package CHART_VERSION=1.0.0 APP_VERSION=dev
make chart-push CHART_VERSION=1.0.0 APP_VERSION=dev
```

For a release, run the `Create Release Tag` workflow. Semantic-release reads the conventional squash-merge commits since the previous release, calculates the next SemVer version, updates `CHANGELOG.md`, creates the `vX.Y.Z` tag, and opens the GitHub release. The tag push publishes `ghcr.io/taliesins/csi-driver-for-windows-storage-server:X.Y.Z`, `:latest`, and the matching OCI Helm chart.

The release workflow expects a `GHCR_PASSWORD` repository secret containing a token that can push commits and tags. This is needed because tags created with the default `GITHUB_TOKEN` do not reliably trigger the follow-on publish workflow.

### Build and release lifecycle

#### 1. Branch created

Creating a branch does not publish anything by itself. Use the local targets while developing:

```sh
make test
make image IMAGE_TAG=dev
make chart-lint
make chart-package CHART_VERSION=0.0.0-dev APP_VERSION=dev
```

Branch commits can be local working commits. The pull request title is the conventional commit that will become the squash-merge commit, for example:

```text
feat: Add snapshot support
fix: Correct iSCSI target cleanup
docs: Document chart installation
```

Semantic-release uses the squash-merge commit type later to decide the release version:

- `fix:` creates a patch release.
- `feat:` creates a minor release.
- `BREAKING CHANGE:` in the commit body, or `!` after the type such as `feat!:`, creates a major release.
- Other types such as `docs:`, `test:`, `refactor:`, and `chore:` appear in history but do not create a release by themselves.

#### 2. Pull request created

Opening or updating a pull request runs conventional PR title validation and CI validation only. The workflow:

- Checks that the PR title follows the conventional commit format.
- Runs the Go test suite.
- Builds the Docker image without pushing it.
- Scans the image with Trivy.
- Lints the Helm chart.

No container images or Helm charts are published for pull requests.

#### 3. Merged into main

Merging to `main` runs the same validation and then publishes development artifacts to GHCR:

Use squash merge and keep the squash commit subject the same as the validated PR title. Individual branch commit messages are not linted.

- Container image: `ghcr.io/taliesins/csi-driver-for-windows-storage-server:dev-<sha>`
- Mutable container image: `ghcr.io/taliesins/csi-driver-for-windows-storage-server:dev`
- Helm chart: `oci://ghcr.io/taliesins/helm/csi-driver-for-windows-storage-server` with version `0.0.0-dev.<sha>`
- Mutable Helm chart: same OCI chart with version `0.0.0-dev`

These builds are intended for development and integration testing.

#### 4. Release tag created

Run the `Create Release Tag` workflow. The workflow runs semantic-release on `main`:

- It analyzes conventional squash-merge commits since the last release tag.
- It calculates the next version automatically.
- It updates `CHANGELOG.md` and `package.json`.
- It commits the release metadata with `chore(release): X.Y.Z`.
- It creates and pushes the `vX.Y.Z` tag.
- It creates the GitHub release notes.

The tag push triggers the publish job.

The release publish creates:

- Container image: `ghcr.io/taliesins/csi-driver-for-windows-storage-server:1.0.0`
- Mutable release image: `ghcr.io/taliesins/csi-driver-for-windows-storage-server:latest`
- Helm chart: `oci://ghcr.io/taliesins/helm/csi-driver-for-windows-storage-server` with version `1.0.0`

Install a released chart with:

```sh
helm upgrade --install --create-namespace csi-driver-for-windows-storage-server oci://ghcr.io/taliesins/helm/csi-driver-for-windows-storage-server --version=1.0.1 -n=kube-system \
  --set winrm.host=win-storage.lab.local \
  --set winrm.user=csi-winrm-test \
  --set-string winrm.password='<password>'
```

### Troubleshooting

- [CSI driver troubleshooting guide](./docs/csi-debug.md)

### Kubernetes Development

Please refer to [development guide](./docs/csi-dev.md)

## Community, discussion, contribution, and support

You can reach the maintainers of this project at:

- [GitHub](https://github.com/taliesins/csi-driver-for-windows-storage-server/issues)
