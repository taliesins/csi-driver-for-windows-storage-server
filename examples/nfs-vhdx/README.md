# NFS VHDX Examples

These examples use `nfs-vhdx.csi.windows.microsoft.com` and
`StorageClass/nfs-vhdx-for-windows-rwx`. This mode creates one VHDX per volume,
mounts it on the Windows storage server, and exports that mounted path through
NFS.

## Order

1. `01-storageclass`
2. `02-rwx-pvc-pods`
3. `03-snapshots`
4. `04-restore-from-snapshot`
5. `05-expand-online`
6. `06-static-pv-import`
7. `07-static-nodeonly-true`

NFS VHDX supports RWX filesystem volumes, snapshots, restore, expansion, and
static imports. Use `../nfs` when you want a normal directory-backed share
without snapshots.
