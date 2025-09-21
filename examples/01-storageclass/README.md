# StorageClass (Windows iSCSI)

Creates `StorageClass/iscsi-for-windows-rwo` that provisions RWO volumes via your Windows
Storage Server controller (WinRM) and publishes over iSCSI.

## Apply
```bash
kubectl apply -f storageclass.yaml
