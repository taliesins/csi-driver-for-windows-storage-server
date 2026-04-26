# SMB VHDX Online Volume Expansion

Increase `PVC/apps/shared-smb-vhdx-data` from 100Gi to 150Gi. The controller
expands the backing VHDX and the mounted filesystem on the Windows storage
server. Node expansion is not required for SMB.

## Patch

```bash
kubectl -n apps patch pvc shared-smb-vhdx-data \
  --type merge \
  -p '{"spec":{"resources":{"requests":{"storage":"150Gi"}}}}'
```

Watch events and capacity:

```bash
kubectl -n apps get pvc shared-smb-vhdx-data -w
```
