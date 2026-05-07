# SMB RWX PVC and Pods

Creates a 1Gi ReadWriteMany PVC and mounts it from two pods.

## Apply

```bash
kubectl create ns apps
kubectl -n apps apply -f secret-smb.yaml
kubectl -n apps apply -f pvc.yaml
kubectl -n apps apply -f pod-a.yaml
kubectl -n apps apply -f pod-b.yaml
kubectl -n apps get pvc,pod -l app=smb-rwx-demo
```
