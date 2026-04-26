# SMB StorageClass

Creates `StorageClass/smb-for-windows-rwx` for dynamically provisioned,
directory-backed SMB shares on the Windows storage server.

Update the ACL values and `secret-smb.yaml` to match your Windows domain or
local account before applying.

## Apply

```bash
kubectl apply -f storageclass.yaml
```
