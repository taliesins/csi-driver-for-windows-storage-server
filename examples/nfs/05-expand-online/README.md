# NFS Online Volume Expansion

Increase `PVC/apps/shared-nfs-data` from 100Gi to 150Gi. The controller expands
the backing file-share storage. Node expansion is not required for NFS.

## Patch

```bash
kubectl -n apps patch pvc shared-nfs-data \
  --type merge \
  -p '{"spec":{"resources":{"requests":{"storage":"150Gi"}}}}'
```

Watch events and capacity:

```bash
kubectl -n apps get pvc shared-nfs-data -w
```
