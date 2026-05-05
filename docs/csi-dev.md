# ISCSI CSI driver development guide

## How to build this project

- Clone repo

```console
$ mkdir -p $GOPATH/src/sigs.k8s.io/
$ git clone https://github.com/taliesins/csi-driver-for-windows-storage-server $GOPATH/src/github.com/taliesins/csi-driver-for-windows-storage-server
```

- Build CSI driver

```console
$ cd $GOPATH/src/github.com/taliesins/csi-driver-for-windows-storage-server
$ make
```

## Unit and integration tests

Run the default unit tests on any developer machine or CI runner. These tests do
not require a Windows host, WinRM credentials, or an iSCSI target server.

```console
$ go test ./...
```

WinRM integration tests are opt-in because they require a reachable Windows
server with WinRM enabled. They are guarded by the `integration` build tag and
skip when required environment variables are not set.
The virtual disk and snapshot lifecycle tests also require a WinRM session with
administrator access to the Windows iSCSI Target service.

To prepare a Windows Server host for iSCSI target storage and WinRM HTTPS, run
the setup script from an elevated PowerShell session on that Windows host:

```powershell
.\deploy\install-windows-machine.ps1 `
  -AllowedClient <dev-or-ci-cidr> `
  -IscsiTargetRemoteAddress <cluster-node-cidr> `
  -StoragePath E:\iSCSIVirtualDisks `
  -CertDnsName <windows-host>
```

The Go WinRM client uses Basic authentication over HTTPS for the local test
account, so the setup script enables WinRM Basic auth while keeping unencrypted
WinRM disabled.

```console
$ export WINRM_HOST=<windows-host>
$ export WINRM_USER=<username>
$ export WINRM_PASSWORD=<password>
$ export WINRM_PORT=5986
$ export WINRM_TLS=true
$ export WINRM_INSECURE=true
$ export WINRM_AUTH=ntlm
$ go test -tags=integration ./pkg/iscsi -run TestWinRMBackendIntegration -v
```

Set `WINRM_TEST_PARENT_DIR` to a scratch directory on the Windows host. The
integration suite uses that directory for free-capacity checks and temporary
iSCSI virtual disks that it creates, resizes, snapshots, exports, and deletes.

## How to test CSI driver in local environment

Install `csc` tool according to https://github.com/rexray/gocsi/tree/master/csc

```console
$ mkdir -p $GOPATH/src/github.com
$ cd $GOPATH/src/github.com
$ git clone https://github.com/rexray/gocsi.git
$ cd rexray/gocsi/csc
$ make build
```

#### Start CSI driver locally

```console
$ cd $GOPATH/src/github.com/taliesins/csi-driver-for-windows-storage-server
$ ./_output/csiplugin --endpoint tcp://127.0.0.1:10000 --nodeid CSINode --mode=node -v=5 &
```

Use `--mode=node` for node RPCs such as stage, publish, unpublish, and node info. Use `--mode=controller` on a separate endpoint when testing controller RPCs; controller mode requires `WINRM_HOST`, `WINRM_USER`, and `WINRM_PASSWORD`.

- Get plugin info

```console
$ csc identity plugin-info --endpoint "$endpoint"
"iscsi.csi.windows.microsoft.com"    "v2.0.0"
```

- Publish an iscsi volume

```console
$ export ISCSI_TARGET="iSCSI Target Server IP (Ex: 10.10.10.10)"
$ export IQN="Target IQN"
$ csc node publish --endpoint tcp://127.0.0.1:10000 --target-path /mnt/iscsi --attrib targetPortal=$ISCSI_TARGET --attrib iqn=$IQN --attrib lun=<lun-id> iscsitestvol
iscsitestvol
```

- Unpublish an iscsi volume

```console
$ csc node unpublish --endpoint tcp://127.0.0.1:10000 --target-path /mnt/iscsi iscsitestvol
iscsitestvol
```

- Validate volume capabilities

```console
$ ./_output/csiplugin --endpoint tcp://127.0.0.1:10001 --mode=controller -v=5 &
$ csc controller validate-volume-capabilities --endpoint tcp://127.0.0.1:10001 --cap "$cap" "$volumeid"
```

- Get NodeID

```console
$ csc node get-info --endpoint "$endpoint"
CSINode
```

## How to test CSI driver in a Kubernetes cluster

- Set environment variable

```console
export REGISTRY=<dockerhub-alias>
export IMAGE_VERSION=latest
```

- Build container image and push image to dockerhub

```console
# build docker image
make container
```
