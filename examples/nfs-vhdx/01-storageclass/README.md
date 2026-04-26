# NFS VHDX StorageClass

Creates `StorageClass/nfs-vhdx-for-windows-rwx` for dynamically provisioned NFS
shares backed by per-volume VHDX files on the Windows storage server.

## Apply

```bash
kubectl apply -f storageclass.yaml
```
