# iSCSI Static PV Import

Bind a PVC to an existing iSCSI-backed VHDX. Set `spec.csi.volumeHandle` to the
exact volume ID for that VHDX, including the target portal, target IQN, LUN, and
backing path. If the Windows target name is different from the target IQN, add
the `targetName` query parameter so controller publish/unpublish can update the
right Windows target object.

The default sample uses the URI volume ID form supported by the driver. The
alternate `pv-volumeattributes.yaml` sample keeps the `volumeHandle` simple and
puts `targetPortal`, `targetIQN`, `lun`, and `iface` in `volumeAttributes`.
Replace the values before applying.

For pod moves between Kubernetes nodes, the Windows target must allow the node
initiator IQN for the node where the pod lands. With the controller installed,
the driver updates that target access during attach. If you use these static PVs
without the controller, preconfigure the target with every Linux node initiator
IQN that may run the pod.

## Apply

```bash
kubectl apply -f pv.yaml
kubectl -n apps apply -f pvc.yaml
kubectl get pv preprovisioned-iscsi
kubectl -n apps get pvc preprovisioned-iscsi-claim
```

## Alternate volumeAttributes form

```bash
kubectl apply -f pv-volumeattributes.yaml
kubectl -n apps apply -f pvc-volumeattributes.yaml
kubectl get pv preprovisioned-iscsi-attributes
kubectl -n apps get pvc preprovisioned-iscsi-attributes-claim
```
