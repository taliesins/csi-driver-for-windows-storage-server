## Windows Storage CSI driver troubleshooting tips

### volume attach/mount failed

In this case, one can verify the Windows Storage CSI driver pod is up and running and also
all the containers in the same POD are healthy.

```console
kubectl get pods
```

Once verified all containers in the POD are healthy, one can also check
problematic application pod `describe` output.

```console
kubectl describe <App POD>
```

You can also get detailed logging of the mount/attach failure from the Windows Storage
node plugin POD container as shown below.

- locate csi driver pod

```api
kubectl get pods
```

from above output make use of the pod name and check the logs of the driver
container as shown below

- get csi driver logs

```
kubectl logs -f csi-windows-storage-node-klh5c -c windows-storage
I1217 14:40:55.928307       7 driver.go:48] Driver: windows-storage.csi.windows.microsoft.com version: 1.0.0
I1217 14:40:55.928339       7 driver.go:89] Enabling volume access mode: SINGLE_NODE_WRITER
I1217 14:40:55.928347       7 driver.go:100] Enabling controller service capability: UNKNOWN
I1217 14:40:55.929521       7 server.go:107] Listening for connections on address: &net.UnixAddr{Name:"//csi/csi.sock", Net:"unix"}
I1217 14:40:55.956864       7 utils.go:63] GRPC call: /csi.v1.Identity/GetPluginInfo
I1217 14:40:55.956877       7 utils.go:64] GRPC request: {}
I1217 14:40:55.957869       7 identityserver.go:32] Using default GetPluginInfo
I1217 14:40:55.957874       7 utils.go:69] GRPC response: {"name":"windows-storage.csi.windows.microsoft.com","vendor_version":"1.0.0"}
I1217 14:40:56.767355       7 utils.go:63] GRPC call: /csi.v1.Identity/GetPluginInfo
I1217 14:40:56.767375       7 utils.go:64] GRPC request: {}
I1217 14:40:56.767437       7 identityserver.go:32] Using default GetPluginInfo
I1217 14:40:56.767445       7 utils.go:69] GRPC response: {"name":"windows-storage.csi.windows.microsoft.com","vendor_version":"1.0.0"}
```

#### Update driver version quickly by editing driver deployment directly

Windows Storage node plugin has been deployed as a `daemonset` object in your cluster, if
a quick update of the plugin image is required, you can do that by editing
the `daemonset` deployment object for the new image as shown below.

- update daemonset deployment

```console
kubectl get ds
NAME             DESIRED   CURRENT   READY   UP-TO-DATE   AVAILABLE   NODE SELECTOR            AGE
csi-windows-storage-node   1         1         1       1            1           kubernetes.io/os=linux   51m

kubectl edit daemonset csi-windows-storage-node
```

change below config, e.g.

```console
        image: ghcr.io/taliesins/csi-driver-for-windows-storage-server:0.2.0
        imagePullPolicy: IfNotPresent

```

#### Get more details about the Windows Storage CSI driver object

One can list the CSI driver object as shown below.

```
kubectl get csidriver
NAME               ATTACHREQUIRED   PODINFOONMOUNT   STORAGECAPACITY   TOKENREQUESTS   REQUIRESREPUBLISH   MODES        AGE
windows-storage.csi.windows.microsoft.com   false            false            false             <unset>         false               Persistent   22m
```
