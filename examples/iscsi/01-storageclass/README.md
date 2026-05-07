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

CHAP is optional for iSCSI. To have the driver configure both sides, set the
same Secret for `csi.storage.k8s.io/provisioner-secret-name`,
`csi.storage.k8s.io/controller-publish-secret-name`, and
`csi.storage.k8s.io/node-stage-secret-name`. For each of those name keys, also
set the matching namespace key:
`csi.storage.k8s.io/provisioner-secret-namespace`,
`csi.storage.k8s.io/controller-publish-secret-namespace`, and
`csi.storage.k8s.io/node-stage-secret-namespace`. Both the name and namespace
must be configured for the provisioner, controller-publish, and node-stage
secrets so the driver can resolve secrets for Windows CHAP/reverse CHAP and
Linux open-iscsi login.

Windows target CHAP uses `node.session.auth.username`,
`node.session.auth.password`, and optionally `node.session.auth.username_in` /
`node.session.auth.password_in` for reverse CHAP. Discovery CHAP keys configure
Linux open-iscsi discovery only; Windows Server iSCSI target CHAP is configured
per target.

## Apply

```bash
kubectl apply -f storageclass.yaml
```
