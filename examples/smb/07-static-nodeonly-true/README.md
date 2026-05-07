# SMB Static PV with nodeOnly=true

Use this example when the Windows SMB share already exists and you only want the
Linux CSI node side installed.

```bash
kubectl create ns apps
kubectl -n apps apply -f secret-smb.yaml
kubectl apply -f pv.yaml
kubectl -n apps apply -f pvc.yaml
kubectl -n apps apply -f pod.yaml
```

Replace the server name, share name, and credentials before applying.
