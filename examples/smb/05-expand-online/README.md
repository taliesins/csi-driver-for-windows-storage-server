# SMB Online Volume Expansion

Increase `PVC/apps/shared-smb-data` from 1Gi to 2Gi. The controller expands
the backing file-share storage. Node expansion is not required for SMB.

## Patch

```bash
kubectl -n apps patch pvc shared-smb-data \
  --type merge \
  -p '{"spec":{"resources":{"requests":{"storage":"2Gi"}}}}'
```

Watch events and capacity:

```bash
kubectl -n apps get pvc shared-smb-data -w
```
