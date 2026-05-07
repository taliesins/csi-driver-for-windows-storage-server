# SMB VHDX Examples

These examples use `windows-storage.csi.windows.microsoft.com` and
`StorageClass/smb-vhdx-for-windows-rwx`. This mode creates one VHDX per volume,
mounts it on the Windows storage server, and shares that mounted path through
SMB.

## Order

1. `01-storageclass`
2. `02-rwx-pvc-pods`
3. `03-snapshots`
4. `04-restore-from-snapshot`
5. `05-expand-online`
6. `06-static-pv-import`
7. `07-static-nodeonly-true`

SMB VHDX supports RWX filesystem volumes, node-stage mount credentials,
snapshots, restore, expansion, and static imports. Use `../smb` when you want a
normal directory-backed share without snapshots.
