# SMB VHDX RWX PVC and Pods

Creates the SMB credentials secret, one RWX PVC, and two pods that write to the
same SMB VHDX-backed volume.

## Apply

```bash
kubectl create namespace apps --dry-run=client -o yaml | kubectl apply -f -
kubectl -n apps apply -f secret-smb.yaml
kubectl -n apps apply -f pvc.yaml
kubectl -n apps apply -f pod-a.yaml
kubectl -n apps apply -f pod-b.yaml
kubectl -n apps get pvc shared-smb-vhdx-data
```
