# iSCSI StorageClass

Creates `StorageClass/iscsi-for-windows-rwo` for dynamically provisioned iSCSI
volumes backed by VHDX files on the Windows storage server.

`iqnPrefix` is optional. When it is omitted, the driver creates a Windows iSCSI
target using the PVC volume name as the Windows `TargetName`, reads back the
Windows-generated `TargetIqn`, and passes that IQN to the node for login. Set
`iqnPrefix` only when you want to force target IQNs to use your own prefix.

`portalPort` is optional and defaults to the standard iSCSI port `3260`.

`vhdxParentPath` is optional for iSCSI. When it is omitted, the WinRM backend
resolves the path on the Windows server from `CSI_VHDX_PARENT_PATH`; if that is
not set, it falls back to `%SystemDrive%\iSCSIVirtualDisks`.

`csi.storage.k8s.io/node-stage-secret-name` and
`csi.storage.k8s.io/node-stage-secret-namespace` are optional for iSCSI. Set
them only when the Windows iSCSI target requires CHAP.

## Apply

```bash
kubectl apply -f storageclass.yaml
```
