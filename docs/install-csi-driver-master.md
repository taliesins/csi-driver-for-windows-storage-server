# Install ISCSI CSI driver master version on a kubernetes cluster

## Install with kubectl

- remote install

```console
curl -skSL https://raw.githubusercontent.com/taliesins/csi-driver-iscsi-for-windows/master/deploy/install-driver.sh | bash -s master --
```

- local install

```console
git clone https://github.com/taliesins/csi-driver-iscsi-for-windows.git
cd csi-driver-iscsi-for-windows
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
curl -skSL https://raw.githubusercontent.com/taliesins/csi-driver-iscsi-for-windows/master/deploy/uninstall-driver.sh | bash -s master --
```
