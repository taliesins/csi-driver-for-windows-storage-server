# Static PV Import (pre-provisioned)

Bind a PVC to a pre-existing volume. Set `spec.csi.volumeHandle` to the exact
VolumeId (base64 JSON) produced by your controller for that volume.

## Apply
```bash
kubectl apply -f pv.yaml
kubectl -n apps apply -f pvc.yaml
kubectl get pv preprovisioned-vhdx
kubectl -n apps get pvc preprovisioned-claim
```
### One-time: namespace + storageclass

If you haven’t yet:
```bash
kubectl apply -f examples/01-storageclass/storageclass.yaml
kubectl create ns apps
```
