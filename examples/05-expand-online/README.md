# Online Volume Expansion

Increase `PVC/apps/db-data` from 200Gi to 300Gi.
The controller grows the VHDX; the node rescans iSCSI and grows the FS online.

## Patch
```bash
kubectl -n apps patch pvc db-data \
  --type merge \
  -p '{"spec":{"resources":{"requests":{"storage":"300Gi"}}}}'
```

Watch events / conditions until Bound and capacity reflects the new size:
```bash
kubectl -n apps get pvc db-data -w
```