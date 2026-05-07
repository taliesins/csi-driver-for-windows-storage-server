# Install Windows Storage CSI driver master version on a kubernetes cluster

## Install with Helm (OCI / GHCR)

The chart is published as an OCI artifact to GHCR at `oci://ghcr.io/taliesins/helm/csi-driver-for-windows-storage-server`.

### Install from GHCR

WinRM connection details are required for the controller pod. Node pods do not receive these credentials.

```console
helm install csi-driver-for-windows-storage-server oci://ghcr.io/taliesins/helm/csi-driver-for-windows-storage-server \
  --namespace kube-system \
  --create-namespace \
  --set winrm.host=win-storage.lab.local \
  --set winrm.user=csi-winrm-test \
  --set-string winrm.password='<password>'
```

### Specify a version

```console
helm install csi-driver-for-windows-storage-server oci://ghcr.io/taliesins/helm/csi-driver-for-windows-storage-server \
  --namespace kube-system \
  --create-namespace \
  --version 0.1.0 \
  --set winrm.host=win-storage.lab.local \
  --set winrm.user=csi-winrm-test \
  --set-string winrm.password='<password>'
```

### Customize values

Override any value from [`values.yaml`](../chart/csi-driver-for-windows-storage-server/values.yaml):

```console
helm install csi-driver-for-windows-storage-server oci://ghcr.io/taliesins/helm/csi-driver-for-windows-storage-server \
  --namespace kube-system \
  --create-namespace \
  --set winrm.host=win-storage.lab.local \
  --set winrm.user=csi-winrm-test \
  --set-string winrm.password='<password>' \
  --set image.tag=v0.2.0 \
  --set image.pullPolicy=Always
```

Controller leader election is enabled automatically when `controller.replicas` is greater than `1`:

```console
helm upgrade csi-driver-for-windows-storage-server oci://ghcr.io/taliesins/helm/csi-driver-for-windows-storage-server \
  --namespace kube-system \
  --reuse-values \
  --set controller.replicas=2
```

With one controller replica, the chart does not add leader-election arguments or Lease RBAC.

### Verify the installation

```console
helm status csi-driver-for-windows-storage-server -n kube-system
kubectl get pods -n kube-system -l app.kubernetes.io/instance=csi-driver-for-windows-storage-server
```

### Upgrade or uninstall

```console
# Upgrade
helm upgrade csi-driver-for-windows-storage-server oci://ghcr.io/taliesins/helm/csi-driver-for-windows-storage-server \
  --namespace kube-system \
  --reuse-values

# Uninstall
helm uninstall csi-driver-for-windows-storage-server -n kube-system
```

## Install with Helm (local chart)

For development or air-gapped environments, you can install from the local chart directory:

```console
helm install csi-driver-for-windows-storage-server ./chart/csi-driver-for-windows-storage-server \
  --namespace kube-system \
  --create-namespace \
  --set winrm.host=win-storage.lab.local \
  --set winrm.user=csi-winrm-test \
  --set-string winrm.password='<password>'
```

Customize image tag or pull policy:

```console
helm install csi-driver-for-windows-storage-server ./chart/csi-driver-for-windows-storage-server \
  --namespace kube-system \
  --set winrm.host=win-storage.lab.local \
  --set winrm.user=csi-winrm-test \
  --set-string winrm.password='<password>' \
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
  --namespace kube-system \
  --reuse-values

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
kubectl -n kube-system get pod -o wide -l app=csi-windows-storage-node
```

example output:

```console
NAME                                       READY   STATUS    RESTARTS   AGE     IP             NODE
csi-windows-storage-node-cvgbs                        3/3     Running   0          35s     10.240.0.35    k8s-agentpool-22533604-1
csi-windows-storage-node-dr4s4                        3/3     Running   0          35s     10.240.0.4     k8s-agentpool-22533604-0
```

- Clean up CSI driver

```console
curl -skSL https://raw.githubusercontent.com/taliesins/csi-driver-for-windows-storage-server/master/deploy/uninstall-driver.sh | bash -s master --
```
