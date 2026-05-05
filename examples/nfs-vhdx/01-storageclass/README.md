# NFS VHDX StorageClass

Creates `StorageClass/nfs-vhdx-for-windows-rwx` for dynamically provisioned NFS
shares backed by per-volume VHDX files on the Windows storage server.

Kerberos is optional. Set `nfsAuthentication` to one or more Windows NFS
authentication flavors (`sys`, `krb5`, `krb5i`, `krb5p`). The node adds the
matching Linux mount `sec=` option; if multiple Kerberos flavors are listed, it
uses the strongest one unless `nfsMountAuthentication` is set.

## Apply

```bash
kubectl apply -f storageclass.yaml
```
