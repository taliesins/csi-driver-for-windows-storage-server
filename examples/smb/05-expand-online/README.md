# SMB Online Volume Expansion

Increase `PVC/apps/shared-smb-data` from 100Gi to 150Gi. The controller expands
the backing file-share storage. Node expansion is not required for SMB.

## Patch

```bash
kubectl -n apps patch pvc shared-smb-data \
  --type merge \
  -p '{"spec":{"resources":{"requests":{"storage":"150Gi"}}}}'
```

Watch events and capacity:

```bash
kubectl -n apps get pvc shared-smb-data -w
```
