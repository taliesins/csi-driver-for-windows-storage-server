# iSCSI Online Volume Expansion

Increase `PVC/apps/db-data` from 1Gi to 2Gi. The controller grows the VHDX;
the node rescans iSCSI and grows the filesystem online.

## Patch

```bash
kubectl -n apps patch pvc db-data \
  --type merge \
  -p '{"spec":{"resources":{"requests":{"storage":"2Gi"}}}}'
```

Watch events and capacity:

```bash
kubectl -n apps get pvc db-data -w
```
