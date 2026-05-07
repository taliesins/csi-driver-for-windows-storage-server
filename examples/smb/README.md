# SMB Examples

These examples use `windows-storage.csi.windows.microsoft.com` and
`StorageClass/smb-for-windows-rwx`. This is the directory-backed SMB mode: each
dynamic volume is a normal folder under `shareParentPath`, and static PVs can
point at an existing SMB share path.

## Order

1. `01-storageclass`
2. `02-rwx-pvc-pods`
3. `05-expand-online`
4. `06-static-pv-import`
5. `07-static-nodeonly-true`

Directory-backed SMB supports RWX filesystem volumes, node-stage mount
credentials, expansion, and static imports. Snapshots and restore are provided
by the VHDX-backed SMB driver in `../smb-vhdx`.
