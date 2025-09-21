# Raw Block Volume

Requests a 50Gi RWO raw block and exposes it at `/dev/walraw` inside the pod.

## Apply
```bash
kubectl -n apps apply -f pvc-block.yaml
kubectl -n apps apply -f pod-block.yaml
```

Inside the pod:
```bash
kubectl -n apps exec -it wal-writer -- bash
ls -l /dev/walraw
blockdev --getsize64 /dev/walraw

```