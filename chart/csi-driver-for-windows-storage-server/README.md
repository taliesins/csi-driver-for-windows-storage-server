# csi-driver-for-windows-storage-server

A Helm chart for the Windows Storage CSI driver

![Version: 0.1.1](https://img.shields.io/badge/Version-0.1.1-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 0.2.0](https://img.shields.io/badge/AppVersion-0.2.0-informational?style=flat-square)

## Installing the Chart

Install the published OCI chart from GHCR:

```sh
helm upgrade --install --create-namespace csi-driver-for-windows-storage-server oci://ghcr.io/taliesins/helm/csi-driver-for-windows-storage-server -n kube-system \
  --set winrm.host=win-storage.lab.local \
  --set winrm.user=csi-winrm-test \
  --set-string winrm.password='<password>'
```

Install the local chart while developing:

```sh
helm upgrade --install --create-namespace csi-driver-for-windows-storage-server ./chart/csi-driver-for-windows-storage-server -n kube-system \
  --set winrm.host=win-storage.lab.local \
  --set winrm.user=csi-winrm-test \
  --set-string winrm.password='<password>'
```

The driver runs its Kubernetes provisioner, attacher, node registrar, and liveness endpoint inside the driver containers; no external helper containers are rendered. Set `drivers.windows-storage.needsIscsi=false` for NFS/SMB-only installs that do not have open-iscsi configured on every Linux node. Set `nodeOnly=true` only for static, pre-provisioned volumes where no controller-side WinRM access is required.

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| nodeOnly | bool | `false` | Install only node-side resources. Use this for static, pre-provisioned volumes when no controller or WinRM Secret is needed. |
| debug | bool | `false` | Enable detailed driver action logging without logging secret values. Disabled by default; enable when troubleshooting. |
| logLevel | int | `2` | Driver and built-in Kubernetes controller verbosity. |
| controller.replicas | int | `1` | Number of controller replicas. Values greater than 1 enable leader election. |
| controller.livenessPort | int | `29752` | HTTP port used by the controller liveness probe. |
| controller.leaderElection.leaseDuration | string | `"15s"` | Kubernetes Lease duration used when controller.replicas is greater than 1. |
| controller.leaderElection.renewDeadline | string | `"10s"` | Kubernetes Lease renew deadline used when controller.replicas is greater than 1. |
| controller.leaderElection.retryPeriod | string | `"2s"` | Kubernetes Lease retry period used when controller.replicas is greater than 1. |
| controller.serviceAccount.create | bool | `true` | Create the controller ServiceAccount. |
| controller.serviceAccount.automount | bool | `true` | Automount the controller ServiceAccount token. |
| controller.serviceAccount.annotations | object | `{}` | Extra annotations to add to the controller ServiceAccount. |
| controller.serviceAccount.name | string | `""` | Existing controller ServiceAccount name. When empty, the chart creates or uses the generated name. |
| node.initiatorNamePath | string | `"/etc/iscsi/initiatorname.iscsi"` | Host path on each Kubernetes node containing the open-iscsi initiator name. |
| node.serviceAccount.create | bool | `true` | Create the node ServiceAccount. |
| node.serviceAccount.automount | bool | `true` | Automount the node ServiceAccount token. |
| node.serviceAccount.annotations | object | `{}` | Extra annotations to add to the node ServiceAccount. |
| node.serviceAccount.name | string | `""` | Existing node ServiceAccount name. When empty, the chart creates or uses the generated name. |
| drivers.windows-storage.enabled | bool | `true` | Render the consolidated Windows Storage CSI driver resources. |
| drivers.windows-storage.name | string | `"windows-storage.csi.windows.microsoft.com"` | CSI driver name advertised to Kubernetes. |
| drivers.windows-storage.attachRequired | bool | `true` | Whether Kubernetes should use ControllerPublishVolume before node staging/publishing. |
| drivers.windows-storage.needsIscsi | bool | `true` | Enable iSCSI host integration. When true, the node plugin reads the open-iscsi initiator from node.initiatorNamePath; set false for NFS/SMB-only installs. |
| drivers.windows-storage.livenessPort | int | `29753` | HTTP port used by the node liveness probe. |
| image.repository | string | `"ghcr.io/taliesins/csi-driver-for-windows-storage-server"` | Driver image repository. |
| image.pullPolicy | string | `"IfNotPresent"` | Driver image pull policy. |
| image.tag | string | `""` | Driver image tag. Defaults to chart appVersion when empty. |
| nameOverride | string | `""` | Override the chart name suffix. |
| fullnameOverride | string | `""` | Override the fully qualified resource name. |
| imagePullSecrets | list | `[]` | Optional image pull secrets for the driver. |
| winrm.host | string | `""` | Windows storage server hostname or IP address reachable from the controller. |
| winrm.port | int | `5986` | WinRM port. |
| winrm.tls | bool | `true` | Use HTTPS for WinRM. |
| winrm.insecure | bool | `true` | Skip WinRM TLS certificate validation. Useful with self-signed Windows listener certificates. |
| winrm.auth | string | `"basic"` | WinRM authentication mechanism. Supported values are basic and ntlm. |
| winrm.timeout | string | `"60s"` | WinRM request timeout. |
| winrm.psImport | string | `""` | Optional PowerShell module import path or statement run before backend commands. |
| winrm.existingSecret | string | `""` | Existing Secret containing WINRM_USER and WINRM_PASSWORD keys. |
| winrm.user | string | `""` | WinRM username used when winrm.existingSecret is empty. |
| winrm.password | string | `""` | WinRM password used when winrm.existingSecret is empty. |
