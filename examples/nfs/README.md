# NFS Examples

These examples use `windows-storage.csi.windows.microsoft.com` and
`StorageClass/nfs-for-windows-rwx`. This is the directory-backed NFS mode: each
dynamic volume is a normal folder under `shareParentPath`, and static PVs can
point at an existing NFS export path.

## Order

1. `01-storageclass`
2. `02-rwx-pvc-pods`
3. `05-expand-online`
4. `06-static-pv-import`
5. `07-static-nodeonly-true`

Directory-backed NFS supports RWX filesystem volumes, expansion, and static
imports. Snapshots and restore are provided by the VHDX-backed NFS driver in
`../nfs-vhdx`.
