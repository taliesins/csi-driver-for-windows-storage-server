# iSCSI Filesystem PVC and Pod

Creates a 1Gi RWO PVC and runs Postgres mounting it.

## Apply

```bash
kubectl create ns apps
kubectl -n apps apply -f pvc.yaml
kubectl -n apps apply -f pod.yaml
kubectl -n apps get pvc,pod
```

Apply `secret-chap.yaml` first only if the StorageClass enables CHAP secret
parameters. Use the same Secret for provisioning, controller publish, and node
stage so the controller can configure Windows target CHAP and the node can log
in with matching Linux open-iscsi credentials.
