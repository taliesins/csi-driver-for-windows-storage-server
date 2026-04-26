# Restore SMB VHDX PVC from Snapshot

Creates `PVC/apps/shared-smb-vhdx-restore` from
`VolumeSnapshot/apps/shared-smb-vhdx-snap-1`.

## Apply

```bash
kubectl -n apps apply -f pvc-restore.yaml
kubectl -n apps get pvc shared-smb-vhdx-restore -o wide
```
