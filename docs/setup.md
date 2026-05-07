# Project setup

This guide covers the local tools and cluster prerequisites used to build, test, document, and install the Windows Storage CSI driver.

## Local development tools

Use the devcontainer when possible; it includes Go, Docker, Helm, helm-docs, Make, and pre-commit. On a local workstation, install:

- Go with toolchain auto-download enabled, or the Go version from `go.mod`
- Docker with Buildx
- Helm 3
- helm-docs, if you want to run `helm-docs` directly instead of `make chart-docs`
- kubectl
- GNU Make
- pre-commit, if you want local hook parity with CI

Run the standard validation loop:

```sh
make test
make chart-lint
make chart-docs
```

`make chart-docs` runs `helm-docs` and regenerates `chart/csi-driver-for-windows-storage-server/README.md` from `Chart.yaml`, `values.yaml`, and `README.md.gotmpl`.

## Windows storage server

Run the Windows setup script from an elevated PowerShell session on the Windows Server that will host iSCSI, NFS, or SMB storage:

```powershell
$AllowedClient = "Any"
$WinRMHost = "win-storage.lab.local"
$WinRMUser = "csi-winrm-test"
$WinRMPassword = Read-Host "Password for $WinRMUser" -AsSecureString
$StoragePath = "C:\data\csi-driver-for-windows-storage-server\vhdx"

.\deploy\install-windows-machine.ps1 `
  -AllowedClient $AllowedClient `
  -WinRMUser $WinRMUser `
  -WinRMPassword $WinRMPassword `
  -StoragePath $StoragePath `
  -CertDnsName $WinRMHost
```

The Helm controller validates WinRM on startup. Make sure the Windows host is reachable from Kubernetes and that the chart values match the listener:

```yaml
winrm:
  host: win-storage.lab.local
  port: 5986
  tls: true
  insecure: true
  auth: basic
  user: csi-winrm-test
  password: "<password>"
```

Use `auth: ntlm` when testing shows the Windows listener requires Negotiate/NTLM for the account you are using.

## Kubernetes nodes

By default, the Helm chart installs iSCSI node support. Each Linux node must have open-iscsi installed and an initiator name at `/etc/iscsi/initiatorname.iscsi`.

For NFS/SMB-only clusters, set:

```yaml
drivers:
  windows-storage:
    needsIscsi: false
```

That removes the iSCSI host mounts and makes the node plugin use the Kubernetes node name as its CSI node ID.

## Install the chart

Install the released OCI chart:

```sh
helm upgrade --install --create-namespace csi-driver-for-windows-storage-server oci://ghcr.io/taliesins/helm/csi-driver-for-windows-storage-server -n kube-system \
  --set winrm.host=win-storage.lab.local \
  --set winrm.user=csi-winrm-test \
  --set-string winrm.password='<password>'
```

Install the local chart while developing:

```sh
helm upgrade --install --create-namespace csi-driver-for-windows-storage-server ./chart/csi-driver-for-windows-storage-server -n kube-system -f csi-driver-overrides.yaml
```

Verify the installation:

```sh
helm status csi-driver-for-windows-storage-server -n kube-system
kubectl get pods -n kube-system -l app.kubernetes.io/instance=csi-driver-for-windows-storage-server
```

For all Helm values, see the generated [chart documentation](../chart/csi-driver-for-windows-storage-server/README.md).
