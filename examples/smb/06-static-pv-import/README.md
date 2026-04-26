# SMB Static PV Import

Bind a PVC to an existing SMB share managed outside dynamic provisioning. Set
`spec.csi.volumeHandle` to a valid SMB volume ID for the server, share name, and
backing path.

The sample uses the URI volume ID form supported by the driver. Replace the
values before applying.

## Apply

```bash
kubectl apply -f pv.yaml
kubectl -n apps apply -f pvc.yaml
kubectl get pv preprovisioned-smb
kubectl -n apps get pvc preprovisioned-smb-claim
```
