# Filesystem PVC + Pod

Creates a 200Gi RWO PVC and runs Postgres mounting it.

## Apply
```bash
kubectl create ns apps
kubectl -n apps apply -f secret-chap.yaml
kubectl -n apps apply -f pvc.yaml
kubectl -n apps apply -f pod.yaml
kubectl -n apps get pvc,pod
```
