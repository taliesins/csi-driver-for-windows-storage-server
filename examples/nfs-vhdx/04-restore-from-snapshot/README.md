# Restore NFS VHDX PVC from Snapshot

Creates `PVC/apps/shared-nfs-vhdx-restore` from
`VolumeSnapshot/apps/shared-nfs-vhdx-snap-1`.

## Apply

```bash
kubectl -n apps apply -f pvc-restore.yaml
kubectl -n apps get pvc shared-nfs-vhdx-restore -o wide
```
