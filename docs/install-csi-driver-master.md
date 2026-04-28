# Install ISCSI CSI driver master version on a kubernetes cluster

## Install with Helm

The project includes a Helm chart at [`chart/csi-driver-for-windows-storage-server/`](../chart/csi-driver-for-windows-storage-server/).

```console
helm install csi-driver-for-windows-storage-server ./chart/csi-driver-for-windows-storage-server \
  --namespace kube-system \
  --create-namespace
```

Customize image tag or pull policy:

```console
helm install csi-driver-for-windows-storage-server ./chart/csi-driver-for-windows-storage-server \
  --namespace kube-system \
  --set image.tag=v0.2.0 \
  --set image.pullPolicy=Always
```

Verify the installation:

```console
helm status csi-driver-for-windows-storage-server -n kube-system
kubectl get pods -n kube-system -l app.kubernetes.io/instance=csi-driver-for-windows-storage-server
```

Upgrade or uninstall:

```console
# Upgrade
helm upgrade csi-driver-for-windows-storage-server ./chart/csi-driver-for-windows-storage-server \
  --namespace kube-system

# Uninstall
helm uninstall csi-driver-for-windows-storage-server -n kube-system
```

For available values, see [`values.yaml`](../chart/csi-driver-for-windows-storage-server/values.yaml).

## Install with kubectl

- remote install

```console
curl -skSL https://raw.githubusercontent.com/taliesins/csi-driver-for-windows-storage-server/master/deploy/install-driver.sh | bash -s master --
```

- local install

```console
git clone https://github.com/taliesins/csi-driver-for-windows-storage-server.git
cd csi-driver-for-windows-storage-server
./deploy/install-driver.sh master local
```

- check pods status:

```console
kubectl -n kube-system get pod -o wide -l app=csi-iscsi-for-windows-node
```

example output:

```console
NAME                                       READY   STATUS    RESTARTS   AGE     IP             NODE
csi-iscsi-for-windows-node-cvgbs                        3/3     Running   0          35s     10.240.0.35    k8s-agentpool-22533604-1
csi-iscsi-for-windows-node-dr4s4                        3/3     Running   0          35s     10.240.0.4     k8s-agentpool-22533604-0
```

- clean up ISCSI CSI driver

```console
curl -skSL https://raw.githubusercontent.com/taliesins/csi-driver-for-windows-storage-server/master/deploy/uninstall-driver.sh | bash -s master --
```
