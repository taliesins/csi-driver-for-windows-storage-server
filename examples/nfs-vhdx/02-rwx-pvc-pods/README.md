# NFS VHDX RWX PVC and Pods

Creates one RWX PVC and two pods that write to the same NFS VHDX-backed volume.

## Apply

```bash
kubectl create namespace apps --dry-run=client -o yaml | kubectl apply -f -
kubectl -n apps apply -f pvc.yaml
kubectl -n apps apply -f pod-a.yaml
kubectl -n apps apply -f pod-b.yaml
kubectl -n apps get pvc shared-nfs-vhdx-data
```
