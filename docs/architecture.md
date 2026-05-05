# Architecture

## System Overview

```mermaid
graph TB
    subgraph KubernetesCluster["Kubernetes Cluster"]
        subgraph ControlPlane["Control Plane"]
            APIServer["API Server"]
            CSIDriver["CSI Driver Object<br/>(iscsi.csi.windows.microsoft.com)"]
        end

        subgraph ControllerPod["Controller Pod<br/>(deployment-controller.yaml)"]
            Provisioner["csi-provisioner"]
            Attacher["csi-attacher"]
            Controller["ControllerServer<br/>(controllerserver.go)"]
            WinRMBackend["WinRMBackend<br/>(backend_winrm.go)"]
        end

        subgraph NodePod["Node Pod<br/>(daemonset-node.yaml)"]
            Registrar["node-driver-registrar"]
            NodeServer["NodeServer<br/>(nodeserver.go)"]
            iSCSILib["iscsilib<br/>(iscsi.go / iscsiadm.go)"]
            Mounter["mount-utils<br/>(FormatAndMount / ResizeFs)"]
        end

        subgraph WindowsHost["Windows iSCSI Storage Server<br/>(external host)"]
            iSCSITarget["iSCSI Target Server<br/>(Windows Feature)"]
            VHDXStore["VHDX Storage<br/>(vhdxParentPath)"]
        end
    end

    APIServer --> CSIDriver
    APIServer --> Provisioner
    APIServer --> Attacher
    APIServer --> Registrar
    Provisioner ==>|CSI gRPC| Controller
    Attacher ==>|CSI gRPC| Controller
    APIServer ==>|CSI gRPC via kubelet| NodeServer

    Controller -->|WinRM (HTTP-ES)| WinRMBackend
    WinRMBackend -->|PowerShell / IscsiTarget| iSCSITarget

    WinRMBackend -.->|CreateVirtualDisk| VHDXStore
    WinRMBackend -.->|DeleteVirtualDisk| VHDXStore

    NodeServer --> iSCSILib
    iSCSILib -->|iscsiadm CLI| iSCSITarget
    iSCSILib -->|blockdev / lsblk| NodeOS["Node OS<br/>(Linux)"]
    Mounter --> NodeOS

    Controller -. Volume lifecycle .-> WinRMBackend
    NodeServer -. Volume attach/mount .-> iSCSILib

    style KubernetesCluster fill:#e1f5fe
    style ControlPlane fill:#fff3e0
    style ControllerPod fill:#e8f5e9
    style NodePod fill:#f3e5f5
    style WindowsHost fill:#ffebee
    style iSCSITarget fill:#fff9c4
```

## Component Responsibilities

```mermaid
graph LR
    subgraph cmd["cmd/csiplugin"]
        Main["main.go<br/>(entry point)"]
    end

    subgraph pkg_iscsi["pkg/iscsi"]
        Driver["driver.go<br/>(Driver + Backend wiring)"]
        Identity["identityserver.go<br/>(GetPluginInfo, Probe)"]
        Controller["controllerserver.go<br/>(Create/Delete/Publish/Expand/Snapshot)"]
        Node["nodeserver.go<br/>(Stage/Publish/Expand/Stats)"]
        Server["server.go<br/>(NonBlockingGRPCServer)"]
        Backend["backend_winrm.go<br/>(WinRM → PowerShell)"]
    end

    subgraph pkg_iscsilib["pkg/iscsilib"]
        Connector["iscsi.go<br/>(Connector: connect/disconnect/multipath)"]
        CLI["iscsiadm.go<br/>(iscsiadm wrapper)"]
        Expand["expand.go<br/>(rescan + resize)"]
        MP["multipath.go<br/>(flush/resize multipath)"]
    end

    Main --> Driver
    Driver --> Identity
    Driver --> Controller
    Driver --> Node
    Driver --> Server
    Driver --> Backend

    Controller --> Backend
    Node --> Connector
    Connector --> CLI
    Connector --> Expand
    Expand --> MP
```

## Controller Volume Creation Flow

```mermaid
sequenceDiagram
    participant K as Kubernetes
    participant A as csi-attacher
    participant C as ControllerServer
    participant W as WinRMBackend
    participant WS as Windows iSCSI Server

    K->>C: CreateVolume(req)<br/>(via csi-provisioner)
    C->>C: validate params<br/>(targetPortal)
    C->>W: resolve vhdxParentPath<br/>(optional)
    C->>C: choose targetName<br/>(iqnPrefix optional)

    alt volume from snapshot
        C->>W: ExportSnapshotAsVirtualDisk(snapshotID)
        W->>WS: PowerShell: Export-IscsiVirtualDiskSnapshot
        WS-->>W: exported VHDX path
    else new volume
        C->>C: check GetVolumeByName(idempotency)
        alt volume exists
            C->>W: EnsureTarget + MapDiskToTarget
        else volume new
            C->>W: CreateVirtualDisk(name, parentDir, size)
            W->>WS: PowerShell: New-IscsiVirtualDisk
            WS-->>W: VHDX path + size
        end
    end

    C->>W: EnsureTarget(targetName, requestedTargetIQN)
    W->>WS: PowerShell: New/Set-IscsiServerTarget
    W-->>C: actual TargetIqn

    C->>W: MapDiskToTarget(targetName, vhdxPath)
    W->>WS: PowerShell: Add-IscsiVirtualDiskTargetMapping
    WS-->>W: LUN (0)

    C-->>K: CreateVolumeResponse<br/>(targetPortal, iqn, lun, volumeID)

    K->>A: VolumeAttachment(nodeID = initiator IQN)
    A->>C: ControllerPublishVolume(volumeID, nodeID)
    C->>W: AllowInitiator(targetName, nodeID)
    W->>WS: PowerShell: Set-IscsiServerTarget -InitiatorIds
    C-->>A: PublishContext<br/>(targetPortal, iqn, lun)
```

## Node Attach / Mount Flow

```mermaid
sequenceDiagram
    participant K as Kubernetes
    participant N as NodeServer
    participant I as iscsilib.Connector
    participant WS as Windows iSCSI Server
    participant OS as Node OS (Linux)

    K->>N: NodeStageVolume(stagingPath, volID)
    N->>N: parse publishContext<br/>(targetPortal, iqn, lun, CHAP secrets)

    N->>I: Connect(targetIQN, portals, lun, CHAP)
    I->>I: discoverTarget (sendtargets)
    I->>WS: iscsiadm -m discoverydb ...
    WS-->>I: target discovered

    I->>WS: iscsiadm -m node -l (login)
    WS-->>I: session established

    I->>OS: wait for /dev/disk/by-path/ip-*
    OS-->>I: device path

    I-->>N: device path (/dev/sdX)

    alt filesystem mode
        N->>OS: FormatAndMount(device, stagingPath, ext4)
    else block mode
        N->>OS: bind-mount device to target file
    end

    N-->>K: NodeStageVolumeResponse

    K->>N: NodePublishVolume(targetPath, stagingPath)
    N->>OS: bind-mount stagingPath → targetPath
    N-->>K: NodePublishVolumeResponse
```

## gRPC Endpoint Summary

| Server | Interface | RPCs Implemented |
|---|---|---|
| **IdentityServer** | CSI probe endpoint | `GetPluginInfo`, `Probe`, `GetPluginCapabilities` |
| **ControllerServer** | CSI controller endpoint | `Create/DeleteVolume`, `ControllerPublish/UnpublishVolume`, `Create/Delete/ListSnapshots`, `ControllerExpandVolume`, `ValidateVolumeCapabilities`, `GetCapacity`, `ControllerGetVolume` |
| **NodeServer** | CSI node endpoint | `NodeStage/UnstageVolume`, `NodePublish/UnpublishVolume`, `NodeGetInfo/GetCapabilities`, `NodeGetVolumeStats`, `NodeExpandVolume` |

## Key Data Flows

### Volume ID Encoding
```
volumeID = base64.RawURLEncode(
  {
    "name":     <volumeName>,
    "targetPortal": <host:port>,
    "targetName": <Windows TargetName>,
    "targetIQN":  <Windows TargetIqn>,
    "lun":        0,
    "vhdxPath":   <Windows server path>,
    "sizeBytes":  <capacity>
  }
)
```

### Snapshot ID Encoding
```
snapshotID = base64.RawURLEncode(
  {
    "snapshotId":  <GUID>,
    "originalPath": <VHDX path>
  }
)
```

### WinRM Backend (Controller → Windows)
The controller communicates with the Windows iSCSI Storage Server via **WinRM** (Windows Remote Management):

| Env Var | Purpose | Default |
|---|---|---|
| `WINRM_HOST` | Windows server hostname | *(required)* |
| `WINRM_PORT` | WinRM port | `5986` (TLS) / `5985` (non-TLS) |
| `WINRM_TLS` | Use HTTPS | `false` |
| `WINRM_INSECURE` | Accept self-signed certs | `true` |
| `WINRM_USER` / `WINRM_PASSWORD` | Auth credentials | *(required)* |
| `WINRM_AUTH` | Auth mode: `basic` or `ntlm` | `basic` |
| `WINRM_TIMEOUT` | PowerShell command timeout | `60s` |

Each backend method wraps a PowerShell script using the `IscsiTarget` module and returns JSON.

### Node iSCSI Initiation (Node → Windows)
The node pod runs on **Linux** and connects to the Windows iSCSI target using:
- `iscsiadm` CLI (sendtargets discovery, node login/logout)
- `lsblk` / `blockdev` for device enumeration
- `multipath` tools for multipath awareness
- `resize2fs` / `xfs_growfs` via `mount.ResizeFs` for filesystem expansion
