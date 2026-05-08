package iscsi

import (
	"context"
	"fmt"
	"strings"

	klog "k8s.io/klog/v2"
)

type loggingBackend struct {
	inner Backend
}

func wrapBackendForDebug(inner Backend) Backend {
	if inner == nil {
		return nil
	}
	return &loggingBackend{inner: inner}
}

func debugActionStart(action string, fields ...any) {
	klog.Infof("controller debug action start: %s%s", action, safeLogKV(fields...))
}

func debugActionDone(action string, err error, fields ...any) {
	if err != nil {
		klog.Warningf("controller debug action failed: %s%s error=%v", action, safeLogKV(fields...), err)
		return
	}
	klog.Infof("controller debug action completed: %s%s", action, safeLogKV(fields...))
}

func safeLogKV(fields ...any) string {
	if len(fields) == 0 {
		return ""
	}
	var b strings.Builder
	for i := 0; i < len(fields); i += 2 {
		key := fmt.Sprint(fields[i])
		value := "<missing>"
		if i+1 < len(fields) {
			value = safeLogValue(key, fields[i+1])
		}
		b.WriteByte(' ')
		b.WriteString(key)
		b.WriteByte('=')
		b.WriteString(value)
	}
	return b.String()
}

func safeLogValue(key string, value any) string {
	if isSensitiveLogKey(key) {
		return "<redacted>"
	}
	switch v := value.(type) {
	case string:
		return fmt.Sprintf("%q", v)
	case []string:
		return fmt.Sprintf("%q", v)
	default:
		return fmt.Sprint(v)
	}
}

func isSensitiveLogKey(key string) bool {
	key = strings.ToLower(key)
	for _, marker := range []string{"password", "secret", "token"} {
		if strings.Contains(key, marker) {
			return true
		}
	}
	return false
}

func sanitizeMountOptionsForLog(opts []string) []string {
	return sanitizeMountOptionsForLogWithUsername(opts, false)
}

func sanitizeMountOptionsForDebugLog(opts []string) []string {
	return sanitizeMountOptionsForLogWithUsername(opts, true)
}

func sanitizeMountOptionsForLogWithUsername(opts []string, includeUsername bool) []string {
	if len(opts) == 0 {
		return nil
	}
	out := make([]string, 0, len(opts))
	for _, opt := range opts {
		trimmed := strings.TrimSpace(opt)
		lower := strings.ToLower(trimmed)
		switch {
		case strings.HasPrefix(lower, "password="),
			strings.HasPrefix(lower, "pass="),
			strings.HasPrefix(lower, "credentials="),
			strings.HasPrefix(lower, "credential="):
			key, _, _ := strings.Cut(trimmed, "=")
			out = append(out, key+"=<redacted>")
		case strings.HasPrefix(lower, "username="),
			strings.HasPrefix(lower, "user="):
			key, _, _ := strings.Cut(trimmed, "=")
			if includeUsername {
				out = append(out, trimmed)
			} else {
				out = append(out, key+"=<redacted>")
			}
		default:
			out = append(out, trimmed)
		}
	}
	return out
}

func (b *loggingBackend) EnsureTarget(ctx context.Context, targetName, targetIQN string) (string, error) {
	debugActionStart("EnsureTarget", "targetName", targetName, "targetIQN", targetIQN)
	actualTargetIQN, err := b.inner.EnsureTarget(ctx, targetName, targetIQN)
	debugActionDone("EnsureTarget", err, "targetName", targetName, "actualTargetIQN", actualTargetIQN)
	return actualTargetIQN, err
}

func (b *loggingBackend) ConfigureTargetChap(ctx context.Context, targetName string, opts TargetChapOptions) error {
	debugActionStart("ConfigureTargetChap", "targetName", targetName, "chapEnabled", opts.Enabled(), "chapUser", opts.ChapUser, "reverseChapUser", opts.ReverseChapUser)
	err := b.inner.ConfigureTargetChap(ctx, targetName, opts)
	debugActionDone("ConfigureTargetChap", err, "targetName", targetName)
	return err
}

func (b *loggingBackend) CreateVirtualDisk(ctx context.Context, name, parentDir string, sizeBytes int64) (string, int64, error) {
	debugActionStart("CreateVirtualDisk", "name", name, "parentDir", parentDir, "sizeBytes", sizeBytes)
	vhdxPath, actualSizeBytes, err := b.inner.CreateVirtualDisk(ctx, name, parentDir, sizeBytes)
	debugActionDone("CreateVirtualDisk", err, "name", name, "vhdxPath", vhdxPath, "actualSizeBytes", actualSizeBytes)
	return vhdxPath, actualSizeBytes, err
}

func (b *loggingBackend) MapDiskToTarget(ctx context.Context, targetName, vhdxPath string) (int32, error) {
	debugActionStart("MapDiskToTarget", "targetName", targetName, "vhdxPath", vhdxPath)
	lun, err := b.inner.MapDiskToTarget(ctx, targetName, vhdxPath)
	debugActionDone("MapDiskToTarget", err, "targetName", targetName, "vhdxPath", vhdxPath, "lun", lun)
	return lun, err
}

func (b *loggingBackend) UnmapDiskFromTarget(ctx context.Context, targetName, vhdxPath string) error {
	debugActionStart("UnmapDiskFromTarget", "targetName", targetName, "vhdxPath", vhdxPath)
	err := b.inner.UnmapDiskFromTarget(ctx, targetName, vhdxPath)
	debugActionDone("UnmapDiskFromTarget", err, "targetName", targetName, "vhdxPath", vhdxPath)
	return err
}

func (b *loggingBackend) DeleteVirtualDisk(ctx context.Context, vhdxPath string) error {
	debugActionStart("DeleteVirtualDisk", "vhdxPath", vhdxPath)
	err := b.inner.DeleteVirtualDisk(ctx, vhdxPath)
	debugActionDone("DeleteVirtualDisk", err, "vhdxPath", vhdxPath)
	return err
}

func (b *loggingBackend) LookupTargetNameByIQN(ctx context.Context, targetIQN string) (string, error) {
	debugActionStart("LookupTargetNameByIQN", "targetIQN", targetIQN)
	targetName, err := b.inner.LookupTargetNameByIQN(ctx, targetIQN)
	debugActionDone("LookupTargetNameByIQN", err, "targetIQN", targetIQN, "targetName", targetName)
	return targetName, err
}

func (b *loggingBackend) DeleteTarget(ctx context.Context, targetName string) error {
	debugActionStart("DeleteTarget", "targetName", targetName)
	err := b.inner.DeleteTarget(ctx, targetName)
	debugActionDone("DeleteTarget", err, "targetName", targetName)
	return err
}

func (b *loggingBackend) GetVolumeByName(ctx context.Context, name, parentDir string) (bool, string, int64, string, string, int32, error) {
	debugActionStart("GetVolumeByName", "name", name, "parentDir", parentDir)
	exists, vhdxPath, sizeBytes, targetName, targetIQN, lun, err := b.inner.GetVolumeByName(ctx, name, parentDir)
	debugActionDone("GetVolumeByName", err, "name", name, "exists", exists, "vhdxPath", vhdxPath, "sizeBytes", sizeBytes, "targetName", targetName, "targetIQN", targetIQN, "lun", lun)
	return exists, vhdxPath, sizeBytes, targetName, targetIQN, lun, err
}

func (b *loggingBackend) AllowInitiator(ctx context.Context, targetName, initiatorIQN string) error {
	debugActionStart("AllowInitiator", "targetName", targetName, "initiatorIQN", initiatorIQN)
	err := b.inner.AllowInitiator(ctx, targetName, initiatorIQN)
	debugActionDone("AllowInitiator", err, "targetName", targetName, "initiatorIQN", initiatorIQN)
	return err
}

func (b *loggingBackend) DenyInitiator(ctx context.Context, targetName, initiatorIQN string) error {
	debugActionStart("DenyInitiator", "targetName", targetName, "initiatorIQN", initiatorIQN)
	err := b.inner.DenyInitiator(ctx, targetName, initiatorIQN)
	debugActionDone("DenyInitiator", err, "targetName", targetName, "initiatorIQN", initiatorIQN)
	return err
}

func (b *loggingBackend) GetDirectoryFreeCapacity(ctx context.Context, parentDir string) (int64, error) {
	debugActionStart("GetDirectoryFreeCapacity", "parentDir", parentDir)
	freeBytes, err := b.inner.GetDirectoryFreeCapacity(ctx, parentDir)
	debugActionDone("GetDirectoryFreeCapacity", err, "parentDir", parentDir, "freeBytes", freeBytes)
	return freeBytes, err
}

func (b *loggingBackend) CreateSnapshot(ctx context.Context, vhdxPath, description string) (SnapshotInfo, error) {
	debugActionStart("CreateSnapshot", "vhdxPath", vhdxPath, "description", description)
	snapshot, err := b.inner.CreateSnapshot(ctx, vhdxPath, description)
	debugActionDone("CreateSnapshot", err, "vhdxPath", vhdxPath, "snapshotID", snapshot.SnapshotID, "sizeBytes", snapshot.SizeBytes)
	return snapshot, err
}

func (b *loggingBackend) DeleteSnapshot(ctx context.Context, snapshotID string) error {
	debugActionStart("DeleteSnapshot", "snapshotID", snapshotID)
	err := b.inner.DeleteSnapshot(ctx, snapshotID)
	debugActionDone("DeleteSnapshot", err, "snapshotID", snapshotID)
	return err
}

func (b *loggingBackend) ListSnapshots(ctx context.Context, vhdxPath string) ([]SnapshotInfo, error) {
	debugActionStart("ListSnapshots", "vhdxPath", vhdxPath)
	snapshots, err := b.inner.ListSnapshots(ctx, vhdxPath)
	debugActionDone("ListSnapshots", err, "vhdxPath", vhdxPath, "count", len(snapshots))
	return snapshots, err
}

func (b *loggingBackend) ExportSnapshotAsVirtualDisk(ctx context.Context, snapshotID string) (string, error) {
	debugActionStart("ExportSnapshotAsVirtualDisk", "snapshotID", snapshotID)
	vhdxPath, err := b.inner.ExportSnapshotAsVirtualDisk(ctx, snapshotID)
	debugActionDone("ExportSnapshotAsVirtualDisk", err, "snapshotID", snapshotID, "vhdxPath", vhdxPath)
	return vhdxPath, err
}

func (b *loggingBackend) ResizeVirtualDisk(ctx context.Context, vhdxPath string, newSizeBytes int64) (int64, error) {
	debugActionStart("ResizeVirtualDisk", "vhdxPath", vhdxPath, "newSizeBytes", newSizeBytes)
	actualSizeBytes, err := b.inner.ResizeVirtualDisk(ctx, vhdxPath, newSizeBytes)
	debugActionDone("ResizeVirtualDisk", err, "vhdxPath", vhdxPath, "actualSizeBytes", actualSizeBytes)
	return actualSizeBytes, err
}

func (b *loggingBackend) GetVolumeInfo(ctx context.Context, vhdxPath string) (VolumeInfo, error) {
	debugActionStart("GetVolumeInfo", "vhdxPath", vhdxPath)
	info, err := b.inner.GetVolumeInfo(ctx, vhdxPath)
	debugActionDone("GetVolumeInfo", err, "vhdxPath", vhdxPath, "sizeBytes", info.SizeBytes, "targetName", info.TargetName, "targetIQN", info.TargetIQN)
	return info, err
}

func (b *loggingBackend) GetTargetInitiators(ctx context.Context, targetName string) ([]string, error) {
	debugActionStart("GetTargetInitiators", "targetName", targetName)
	initiators, err := b.inner.GetTargetInitiators(ctx, targetName)
	debugActionDone("GetTargetInitiators", err, "targetName", targetName, "initiators", initiators)
	return initiators, err
}

func (b *loggingBackend) CreateNfsShare(ctx context.Context, name, parentDir string, sizeBytes int64, clients []string, opts ...NfsShareOptions) (VolumeInfo, error) {
	debugActionStart("CreateNfsShare", "name", name, "parentDir", parentDir, "sizeBytes", sizeBytes, "clients", clients)
	info, err := b.inner.CreateNfsShare(ctx, name, parentDir, sizeBytes, clients, opts...)
	debugActionDone("CreateNfsShare", err, "name", name, "server", info.NfsServer, "exportPath", info.NfsExportPath, "sharePath", info.SharePath, "capacityBytes", info.CapacityBytes)
	return info, err
}

func (b *loggingBackend) GetNfsShare(ctx context.Context, name, parentDir string) (bool, VolumeInfo, error) {
	debugActionStart("GetNfsShare", "name", name, "parentDir", parentDir)
	exists, info, err := b.inner.GetNfsShare(ctx, name, parentDir)
	debugActionDone("GetNfsShare", err, "name", name, "exists", exists, "server", info.NfsServer, "exportPath", info.NfsExportPath, "sharePath", info.SharePath)
	return exists, info, err
}

func (b *loggingBackend) DeleteNfsShare(ctx context.Context, name, path string) error {
	debugActionStart("DeleteNfsShare", "name", name, "path", path)
	err := b.inner.DeleteNfsShare(ctx, name, path)
	debugActionDone("DeleteNfsShare", err, "name", name, "path", path)
	return err
}

func (b *loggingBackend) CreateSmbShare(ctx context.Context, name, parentDir string, sizeBytes int64, fullAccess, changeAccess, readAccess []string, opts ...SmbShareOptions) (VolumeInfo, error) {
	debugActionStart("CreateSmbShare", "name", name, "parentDir", parentDir, "sizeBytes", sizeBytes, "fullAccess", fullAccess, "changeAccess", changeAccess, "readAccess", readAccess)
	info, err := b.inner.CreateSmbShare(ctx, name, parentDir, sizeBytes, fullAccess, changeAccess, readAccess, opts...)
	debugActionDone("CreateSmbShare", err, "name", name, "server", info.SmbServer, "share", info.SmbShareName, "sharePath", info.SharePath, "capacityBytes", info.CapacityBytes)
	return info, err
}

func (b *loggingBackend) GetSmbShare(ctx context.Context, name, parentDir string) (bool, VolumeInfo, error) {
	debugActionStart("GetSmbShare", "name", name, "parentDir", parentDir)
	exists, info, err := b.inner.GetSmbShare(ctx, name, parentDir)
	debugActionDone("GetSmbShare", err, "name", name, "exists", exists, "server", info.SmbServer, "share", info.SmbShareName, "sharePath", info.SharePath)
	return exists, info, err
}

func (b *loggingBackend) DeleteSmbShare(ctx context.Context, name, path string) error {
	debugActionStart("DeleteSmbShare", "name", name, "path", path)
	err := b.inner.DeleteSmbShare(ctx, name, path)
	debugActionDone("DeleteSmbShare", err, "name", name, "path", path)
	return err
}

func (b *loggingBackend) ResizeFileShare(ctx context.Context, path string, newSizeBytes int64) (int64, error) {
	debugActionStart("ResizeFileShare", "path", path, "newSizeBytes", newSizeBytes)
	actualSizeBytes, err := b.inner.ResizeFileShare(ctx, path, newSizeBytes)
	debugActionDone("ResizeFileShare", err, "path", path, "actualSizeBytes", actualSizeBytes)
	return actualSizeBytes, err
}

func (b *loggingBackend) RestoreSnapshotAsFileShare(ctx context.Context, snapshotID, destinationPath string) error {
	debugActionStart("RestoreSnapshotAsFileShare", "snapshotID", snapshotID, "destinationPath", destinationPath)
	err := b.inner.RestoreSnapshotAsFileShare(ctx, snapshotID, destinationPath)
	debugActionDone("RestoreSnapshotAsFileShare", err, "snapshotID", snapshotID, "destinationPath", destinationPath)
	return err
}

func (b *loggingBackend) MountFileShareVirtualDisk(ctx context.Context, vhdxPath, mountPath string) error {
	debugActionStart("MountFileShareVirtualDisk", "vhdxPath", vhdxPath, "mountPath", mountPath)
	err := b.inner.MountFileShareVirtualDisk(ctx, vhdxPath, mountPath)
	debugActionDone("MountFileShareVirtualDisk", err, "vhdxPath", vhdxPath, "mountPath", mountPath)
	return err
}

func (b *loggingBackend) UnmountFileShareVirtualDisk(ctx context.Context, vhdxPath, mountPath string) error {
	debugActionStart("UnmountFileShareVirtualDisk", "vhdxPath", vhdxPath, "mountPath", mountPath)
	err := b.inner.UnmountFileShareVirtualDisk(ctx, vhdxPath, mountPath)
	debugActionDone("UnmountFileShareVirtualDisk", err, "vhdxPath", vhdxPath, "mountPath", mountPath)
	return err
}
