# iSCSI Static PV Import

Bind a PVC to an existing iSCSI-backed VHDX. Set `spec.csi.volumeHandle` to the
exact volume ID for that VHDX, including the target portal, target IQN, LUN, and
backing path.

The sample uses the URI volume ID form supported by the driver. Replace the
values before applying.

## Apply

```bash
kubectl apply -f pv.yaml
kubectl -n apps apply -f pvc.yaml
kubectl get pv preprovisioned-iscsi
kubectl -n apps get pvc preprovisioned-iscsi-claim
```
