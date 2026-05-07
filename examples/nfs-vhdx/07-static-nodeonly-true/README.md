# NFS VHDX Static PV with nodeOnly=true

Use this example when the Windows NFS export already exists, the VHDX is already
mounted/exported on the Windows storage server, and you only want the Linux CSI
node side installed.

```bash
kubectl create ns apps
kubectl apply -f pv.yaml
kubectl -n apps apply -f pvc.yaml
kubectl -n apps apply -f pod.yaml
```

Replace the server name and export path before applying.
