# NFS VHDX Online Volume Expansion

Increase `PVC/apps/shared-nfs-vhdx-data` from 100Gi to 150Gi. The controller
expands the backing VHDX and the mounted filesystem on the Windows storage
server. Node expansion is not required for NFS.

## Patch

```bash
kubectl -n apps patch pvc shared-nfs-vhdx-data \
  --type merge \
  -p '{"spec":{"resources":{"requests":{"storage":"150Gi"}}}}'
```

Watch events and capacity:

```bash
kubectl -n apps get pvc shared-nfs-vhdx-data -w
```
