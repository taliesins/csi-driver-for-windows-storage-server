# ISCSI CSI driver for Kubernetes

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



### Project status: Alpha

### Container Images & Kubernetes Compatibility:

|driver version  | supported k8s version | status |
|----------------|-----------------------|--------|
|master branch   | 1.19+                 | alpha   |

### Install driver on a Kubernetes cluster

- install by [kubectl](./docs/install-iscsi-csi-driver.md)

### Troubleshooting

- [CSI driver troubleshooting guide](./docs/csi-debug.md)

### Kubernetes Development

Please refer to [development guide](./docs/csi-dev.md)

## Community, discussion, contribution, and support

You can reach the maintainers of this project at:

- [Github](https://github.com/taliesins/csi-driver-iscsi-for-windows/issues)
