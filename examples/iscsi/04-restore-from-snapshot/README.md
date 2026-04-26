# Restore iSCSI PVC from Snapshot

Creates `PVC/apps/db-data-restore` from `VolumeSnapshot/apps/db-data-snap-1`.

## Apply

```bash
kubectl -n apps apply -f pvc-restore.yaml
kubectl -n apps get pvc db-data-restore -o wide
```
