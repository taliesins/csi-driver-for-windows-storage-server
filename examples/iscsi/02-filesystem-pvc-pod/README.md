# iSCSI Filesystem PVC and Pod

Creates a 200Gi RWO PVC and runs Postgres mounting it.

## Apply

```bash
kubectl create ns apps
kubectl -n apps apply -f pvc.yaml
kubectl -n apps apply -f pod.yaml
kubectl -n apps get pvc,pod
```

Apply `secret-chap.yaml` first only if the StorageClass enables
`csi.storage.k8s.io/node-stage-secret-name` for CHAP.
