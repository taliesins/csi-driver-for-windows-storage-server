# Volume Snapshots

Takes a snapshot of `PVC/apps/db-data`.

## Apply
```bash
kubectl apply -f volumesnapshotclass.yaml
kubectl -n apps apply -f snapshot.yaml
kubectl -n apps get volumesnapshot db-data-snap-1 -o wide
```