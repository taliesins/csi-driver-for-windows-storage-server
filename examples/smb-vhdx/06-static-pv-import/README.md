# SMB VHDX Static PV Import

Bind a PVC to an existing SMB share backed by an existing VHDX. Set
`spec.csi.volumeHandle` to a valid SMB volume ID for the server, share name,
share mount path, and VHDX path.

The sample uses the URI volume ID form supported by the driver. Replace the
values before applying.

## Apply

```bash
kubectl apply -f pv.yaml
kubectl -n apps apply -f pvc.yaml
kubectl get pv preprovisioned-smb-vhdx
kubectl -n apps get pvc preprovisioned-smb-vhdx-claim
```
