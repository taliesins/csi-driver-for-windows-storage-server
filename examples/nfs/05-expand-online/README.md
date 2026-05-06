# NFS Online Volume Expansion

Increase `PVC/apps/shared-nfs-data` from 1Gi to 2Gi. The controller expands
the backing file-share storage. Node expansion is not required for NFS.

## Patch

```bash
kubectl -n apps patch pvc shared-nfs-data \
  --type merge \
  -p '{"spec":{"resources":{"requests":{"storage":"2Gi"}}}}'
```

Watch events and capacity:

```bash
kubectl -n apps get pvc shared-nfs-data -w
```
