# Examples

These examples are split by protocol so each walkthrough uses the right driver
name, access mode, credentials, and feature set.

| Feature | iSCSI | NFS directory | NFS VHDX | SMB directory | SMB VHDX |
| --- | --- | --- | --- | --- | --- |
| Dynamic provisioning | yes | yes | yes | yes | yes |
| Filesystem PVC | yes | yes | yes | yes | yes |
| RWX PVC | no | yes | yes | yes | yes |
| Arbitrary existing path import | no | yes | yes | yes | yes |
| Snapshots | yes | no | yes | no | yes |
| Restore from snapshot | yes | no | yes | no | yes |
| Online expansion | yes | yes | yes | yes | yes |
| Raw block volume | yes | no | no | no | no |
| CHAP or iSCSI initiator auth | yes | no | no | no | no |
| SMB mount credentials | no | no | no | yes | yes |
| Static PV import | yes | yes | yes | yes | yes |

## Protocol Walkthroughs

- `iscsi/` provisions RWO iSCSI volumes backed by VHDX files and iSCSI targets.
- `nfs/` provisions RWX NFS shares backed by normal directories.
- `nfs-vhdx/` provisions RWX NFS shares backed by one VHDX per volume.
- `smb/` provisions RWX SMB shares backed by normal directories.
- `smb-vhdx/` provisions RWX SMB shares backed by one VHDX per volume.

Each protocol folder starts with `01-storageclass/` and then builds on the PVC
names from the previous examples.
