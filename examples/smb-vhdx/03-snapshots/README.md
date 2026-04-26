# SMB VHDX Volume Snapshots

Takes a snapshot of `PVC/apps/shared-smb-vhdx-data`.

## Apply

```bash
kubectl apply -f volumesnapshotclass.yaml
kubectl -n apps apply -f snapshot.yaml
kubectl -n apps get volumesnapshot shared-smb-vhdx-snap-1 -o wide
```
