# iSCSI Static PV with nodeOnly=true

Use this example when Windows iSCSI storage is already configured and you only
want the Linux CSI node side installed. The controller and WinRM backend are
skipped, so the driver will not create VHDX files, create targets, map LUNs,
configure CHAP, or update Windows target initiator access.

```bash
kubectl create namespace apps --dry-run=client -o yaml | kubectl apply -f -
kubectl apply -f pv.yaml
kubectl -n apps apply -f pvc.yaml
kubectl -n apps apply -f pod.yaml
```

For pod moves between nodes, preconfigure the Windows target to allow every
Linux node initiator IQN that may run the pod.
