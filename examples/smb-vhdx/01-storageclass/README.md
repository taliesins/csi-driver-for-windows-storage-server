# SMB VHDX StorageClass

Creates `StorageClass/smb-vhdx-for-windows-rwx` for dynamically provisioned SMB
shares backed by per-volume VHDX files on the Windows storage server.

Update the ACL values and `secret-smb.yaml` to match your Windows domain or
local account before applying.

## Apply

```bash
kubectl apply -f storageclass.yaml
```
