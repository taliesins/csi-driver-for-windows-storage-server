// controllerserver.go
/*
Copyright ...

Licensed under the Apache License, Version 2.0 ...
*/

package iscsi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	klog "k8s.io/klog/v2"

	"google.golang.org/protobuf/types/known/timestamppb"
)

// ControllerServer implements the CSI Controller service.
type ControllerServer struct {
	Driver *driver
	csi.UnimplementedControllerServer
}

/*
Assumptions / contracts:

- cs.Driver.backend provides the following methods (see your WinRM backend):
  EnsureTarget(ctx, targetIQN) error
  CreateVirtualDisk(ctx, name, parentDir string, sizeBytes int64) (vhdxPath string, actualSize int64, err error)
  MapDiskToTarget(ctx, targetIQN, vhdxPath string) (lun int32, err error)
  UnmapDiskFromTarget(ctx, targetIQN, vhdxPath string) error
  DeleteVirtualDisk(ctx, vhdxPath string) error
  GetVolumeByName(ctx, name, parentDir string) (exists bool, vhdxPath string, sizeBytes int64, targetIQN string, lun int32, err error)
  AllowInitiator(ctx, targetIQN, initiatorIQN string) error
  DenyInitiator(ctx, targetIQN, initiatorIQN string) error
  GetDirectoryFreeCapacity(ctx, parentDir string) (freeBytes int64, err error)
  // 03-snapshots
  CreateSnapshot(ctx, vhdxPath, description string) (SnapshotInfo, error)
  DeleteSnapshot(ctx, snapshotID string) error
  ListSnapshots(ctx context.Context, vhdxPath string) ([]SnapshotInfo, error)
  ExportSnapshotAsVirtualDisk(ctx context.Context, snapshotID string) (exportedVHDXPath string, err error)
  // expansion + query
  ResizeVirtualDisk(ctx context.Context, vhdxPath string, newSizeBytes int64) (actualSizeBytes int64, err error)
  GetVolumeInfo(ctx context.Context, vhdxPath string) (VolumeInfo, error)
  GetTargetInitiators(ctx context.Context, targetIQN string) ([]string, error)
*/

// ---------- helper types ----------

type volID struct {
	VolumeName   string `json:"name"`
	TargetPortal string `json:"targetPortal"` // host:port
	TargetIQN    string `json:"targetIQN"`
	LUN          int32  `json:"lun"`
	VHDXPath     string `json:"vhdxPath"`
	SizeBytes    int64  `json:"sizeBytes"`
}

type snapID struct {
	SnapshotID   string `json:"snapshotId"`   // provider GUID/string
	OriginalPath string `json:"originalPath"` // VHDX path
}

// backend Snapshot/Volume info shapes (must match your backend)
type SnapshotInfo struct {
	SnapshotID   string
	OriginalPath string
	Description  string
	CreatedAt    time.Time
	SizeBytes    int64
}
type VolumeInfo struct {
	VHDXPath  string
	SizeBytes int64
	Targets   []string
	LUN       *int32
}

func encodeVolID(v volID) string {
	b, _ := json.Marshal(v)
	return base64.RawURLEncoding.EncodeToString(b)
}
func decodeVolID(id string) (volID, error) {
	var out volID
	b, err := base64.RawURLEncoding.DecodeString(id)
	if err != nil {
		return out, err
	}
	return out, json.Unmarshal(b, &out)
}
func encodeSnapID(s snapID) string {
	b, _ := json.Marshal(s)
	return base64.RawURLEncoding.EncodeToString(b)
}
func decodeSnapID(id string) (snapID, error) {
	var out snapID
	b, err := base64.RawURLEncoding.DecodeString(id)
	if err != nil {
		return out, err
	}
	return out, json.Unmarshal(b, &out)
}

func getStringParam(params map[string]string, key string) (string, bool) {
	v, ok := params[key]
	v = strings.TrimSpace(v)
	return v, ok && v != ""
}
func requiredBytesFromRange(cr *csi.CapacityRange, minGiB int64) (int64, error) {
	min := minGiB << 30
	if cr == nil {
		return min, nil
	}
	req := cr.GetRequiredBytes()
	lim := cr.GetLimitBytes()
	switch {
	case req > 0 && lim > 0:
		if req > lim {
			return 0, status.Error(codes.InvalidArgument, "requiredBytes > limitBytes")
		}
		if req < min {
			return min, nil
		}
		return req, nil
	case req > 0:
		if req < min {
			return min, nil
		}
		return req, nil
	case lim > 0:
		if lim < min {
			return min, nil
		}
		return lim, nil
	default:
		return min, nil
	}
}

// ---------- Controller RPCs ----------

func (cs *ControllerServer) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "volume name is required")
	}
	if len(req.GetVolumeCapabilities()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "volume capabilities are required")
	}
	// Accept only SINGLE_NODE_* modes
	for _, vc := range req.GetVolumeCapabilities() {
		switch vc.GetAccessMode().GetMode() {
		case csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
			csi.VolumeCapability_AccessMode_SINGLE_NODE_SINGLE_WRITER,
			csi.VolumeCapability_AccessMode_SINGLE_NODE_MULTI_WRITER:
			// ok
		default:
			return nil, status.Error(codes.InvalidArgument, "only SINGLE_NODE_* access modes supported")
		}
	}

	params := req.GetParameters()
	targetPortal, ok := getStringParam(params, "targetPortal")
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "parameter targetPortal is required")
	}
	portalPortStr, _ := getStringParam(params, "portalPort")
	portalPort := 3260
	if portalPortStr != "" {
		p, err := strconv.Atoi(portalPortStr)
		if err != nil || p <= 0 {
			return nil, status.Errorf(codes.InvalidArgument, "invalid portalPort: %q", portalPortStr)
		}
		portalPort = p
	}
	parentDir, ok := getStringParam(params, "vhdxParentPath")
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "parameter vhdxParentPath is required")
	}
	iqnPrefix, ok := getStringParam(params, "iqnPrefix")
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "parameter iqnPrefix is required")
	}

	size, err := requiredBytesFromRange(req.GetCapacityRange(), 1)
	if err != nil {
		return nil, err
	}
	volName := req.GetName()
	targetIQN := fmt.Sprintf("%s:%s", iqnPrefix, volName)

	// Create from snapshot?
	if src := req.GetVolumeContentSource(); src != nil && src.GetSnapshot() != nil {
		sid, err := decodeSnapID(src.GetSnapshot().GetSnapshotId())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid snapshotId: %v", err)
		}
		exportedPath, err := cs.Driver.backend.ExportSnapshotAsVirtualDisk(ctx, sid.SnapshotID)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "ExportSnapshotAsVirtualDisk: %v", err)
		}
		if err := cs.Driver.backend.EnsureTarget(ctx, targetIQN); err != nil {
			return nil, status.Errorf(codes.Internal, "EnsureTarget: %v", err)
		}
		lun, err := cs.Driver.backend.MapDiskToTarget(ctx, targetIQN, exportedPath)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "MapDiskToTarget(exported): %v", err)
		}
		vi, _ := cs.Driver.backend.GetVolumeInfo(ctx, exportedPath)
		vid := encodeVolID(volID{
			VolumeName:   volName,
			TargetPortal: fmt.Sprintf("%s:%d", targetPortal, portalPort),
			TargetIQN:    targetIQN,
			LUN:          lun,
			VHDXPath:     exportedPath,
			SizeBytes:    vi.SizeBytes,
		})
		return &csi.CreateVolumeResponse{
			Volume: &csi.Volume{
				VolumeId:      vid,
				CapacityBytes: vi.SizeBytes,
				VolumeContext: map[string]string{
					"targetPortal": fmt.Sprintf("%s:%d", targetPortal, portalPort),
					"iqn":          targetIQN,
					"lun":          strconv.Itoa(int(lun)),
					"source":       "snapshot",
				},
				ContentSource: req.GetVolumeContentSource(),
			},
		}, nil
	}

	// Idempotency: already exists?
	exists, vhdxPath, existingSize, existingTarget, existingLUN, err := cs.Driver.backend.GetVolumeByName(ctx, volName, parentDir)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "GetVolumeByName: %v", err)
	}
	if exists {
		if size > 0 && existingSize > 0 && size > existingSize {
			return nil, status.Errorf(codes.AlreadyExists, "volume %q exists smaller (%dB) than requested (%dB)", volName, existingSize, size)
		}
		if existingTarget == "" {
			existingTarget = targetIQN
		}
		if err := cs.Driver.backend.EnsureTarget(ctx, existingTarget); err != nil {
			return nil, status.Errorf(codes.Internal, "EnsureTarget(existing): %v", err)
		}
		if existingLUN < 0 {
			lun, err := cs.Driver.backend.MapDiskToTarget(ctx, existingTarget, vhdxPath)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "MapDiskToTarget(existing): %v", err)
			}
			existingLUN = lun
		}
		vid := encodeVolID(volID{
			VolumeName:   volName,
			TargetPortal: fmt.Sprintf("%s:%d", targetPortal, portalPort),
			TargetIQN:    existingTarget,
			LUN:          existingLUN,
			VHDXPath:     vhdxPath,
			SizeBytes:    existingSize,
		})
		return &csi.CreateVolumeResponse{
			Volume: &csi.Volume{
				VolumeId:      vid,
				CapacityBytes: existingSize,
				VolumeContext: map[string]string{
					"targetPortal": fmt.Sprintf("%s:%d", targetPortal, portalPort),
					"iqn":          existingTarget,
					"lun":          strconv.Itoa(int(existingLUN)),
				},
			},
		}, nil
	}

	// Create new VHDX and map it
	vhdxPath, actual, err := cs.Driver.backend.CreateVirtualDisk(ctx, volName, parentDir, size)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "CreateVirtualDisk: %v", err)
	}
	if err := cs.Driver.backend.EnsureTarget(ctx, targetIQN); err != nil {
		return nil, status.Errorf(codes.Internal, "EnsureTarget: %v", err)
	}
	lun, err := cs.Driver.backend.MapDiskToTarget(ctx, targetIQN, vhdxPath)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "MapDiskToTarget: %v", err)
	}

	vid := encodeVolID(volID{
		VolumeName:   volName,
		TargetPortal: fmt.Sprintf("%s:%d", targetPortal, portalPort),
		TargetIQN:    targetIQN,
		LUN:          lun,
		VHDXPath:     vhdxPath,
		SizeBytes:    actual,
	})
	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      vid,
			CapacityBytes: actual,
			VolumeContext: map[string]string{
				"targetPortal": fmt.Sprintf("%s:%d", targetPortal, portalPort),
				"iqn":          targetIQN,
				"lun":          strconv.Itoa(int(lun)),
			},
		},
	}, nil
}

func (cs *ControllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "volume_id is required")
	}
	id, err := decodeVolID(req.GetVolumeId())
	if err != nil {
		// idempotent delete
		klog.Warningf("DeleteVolume: decode error: %v", err)
		return &csi.DeleteVolumeResponse{}, nil
	}
	// best-effort unmap + delete
	if id.TargetIQN != "" && id.VHDXPath != "" {
		if err := cs.Driver.backend.UnmapDiskFromTarget(ctx, id.TargetIQN, id.VHDXPath); err != nil {
			klog.Warningf("UnmapDiskFromTarget: %v", err)
		}
	}
	if id.VHDXPath != "" {
		if err := cs.Driver.backend.DeleteVirtualDisk(ctx, id.VHDXPath); err != nil {
			klog.Warningf("DeleteVirtualDisk: %v", err)
		}
	}
	return &csi.DeleteVolumeResponse{}, nil
}

func (cs *ControllerServer) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	if req.GetVolumeId() == "" || req.GetNodeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "volume_id and node_id are required")
	}
	id, err := decodeVolID(req.GetVolumeId())
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "invalid volume_id: %v", err)
	}
	// enforce SINGLE_NODE_* modes
	switch req.GetVolumeCapability().GetAccessMode().GetMode() {
	case csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
		csi.VolumeCapability_AccessMode_SINGLE_NODE_SINGLE_WRITER,
		csi.VolumeCapability_AccessMode_SINGLE_NODE_MULTI_WRITER:
	default:
		return nil, status.Error(codes.FailedPrecondition, "only SINGLE_NODE_* access modes supported")
	}
	initiatorIQN := req.GetNodeId()
	if !strings.HasPrefix(initiatorIQN, "iqn.") {
		return nil, status.Errorf(codes.InvalidArgument, "node_id must be an initiator IQN, got %q", initiatorIQN)
	}
	if err := cs.Driver.backend.AllowInitiator(ctx, id.TargetIQN, initiatorIQN); err != nil {
		return nil, status.Errorf(codes.Internal, "AllowInitiator: %v", err)
	}
	return &csi.ControllerPublishVolumeResponse{
		PublishContext: map[string]string{
			"targetPortal": id.TargetPortal,
			"iqn":          id.TargetIQN,
			"lun":          strconv.Itoa(int(id.LUN)),
		},
	}, nil
}

func (cs *ControllerServer) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	if req.GetVolumeId() == "" || req.GetNodeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "volume_id and node_id are required")
	}
	id, err := decodeVolID(req.GetVolumeId())
	if err != nil {
		return &csi.ControllerUnpublishVolumeResponse{}, nil
	}
	if err := cs.Driver.backend.DenyInitiator(ctx, id.TargetIQN, req.GetNodeId()); err != nil {
		klog.Warningf("DenyInitiator: %v", err)
	}
	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

func (cs *ControllerServer) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	if req.GetVolumeId() == "" || len(req.GetVolumeCapabilities()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "volume_id and volume_capabilities are required")
	}
	if _, err := decodeVolID(req.GetVolumeId()); err != nil {
		return nil, status.Errorf(codes.NotFound, "invalid volume_id: %v", err)
	}
	for _, vc := range req.GetVolumeCapabilities() {
		switch vc.GetAccessMode().GetMode() {
		case csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
			csi.VolumeCapability_AccessMode_SINGLE_NODE_SINGLE_WRITER,
			csi.VolumeCapability_AccessMode_SINGLE_NODE_MULTI_WRITER:
			// ok
		default:
			return &csi.ValidateVolumeCapabilitiesResponse{
				Message: "only SINGLE_NODE_* access modes supported",
			}, nil
		}
	}
	return &csi.ValidateVolumeCapabilitiesResponse{
		Confirmed: &csi.ValidateVolumeCapabilitiesResponse_Confirmed{
			VolumeCapabilities: req.GetVolumeCapabilities(),
		},
	}, nil
}

func (cs *ControllerServer) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	// Optional: implement if you can enumerate on backend. For now, empty.
	return &csi.ListVolumesResponse{Entries: []*csi.ListVolumesResponse_Entry{}}, nil
}

func (cs *ControllerServer) GetCapacity(ctx context.Context, req *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error) {
	params := req.GetParameters()
	parentDir, ok := getStringParam(params, "vhdxParentPath")
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "parameter vhdxParentPath is required")
	}
	free, err := cs.Driver.backend.GetDirectoryFreeCapacity(ctx, parentDir)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "GetDirectoryFreeCapacity: %v", err)
	}
	return &csi.GetCapacityResponse{AvailableCapacity: free}, nil
}

func (cs *ControllerServer) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	klog.V(5).Infof("ControllerGetCapabilities")
	return &csi.ControllerGetCapabilitiesResponse{
		Capabilities: cs.Driver.cscap,
	}, nil
}

// ---------- 03-snapshots ----------

func (cs *ControllerServer) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	if req.GetSourceVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "source_volume_id required")
	}
	vid, err := decodeVolID(req.GetSourceVolumeId())
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "invalid source_volume_id: %v", err)
	}
	desc := strings.TrimSpace(req.GetName())
	snap, err := cs.Driver.backend.CreateSnapshot(ctx, vid.VHDXPath, desc)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "CreateSnapshot: %v", err)
	}
	id := encodeSnapID(snapID{SnapshotID: snap.SnapshotID, OriginalPath: snap.OriginalPath})
	return &csi.CreateSnapshotResponse{
		Snapshot: &csi.Snapshot{
			SnapshotId:     id,
			SourceVolumeId: req.GetSourceVolumeId(),
			CreationTime:   timestamppb.New(snap.CreatedAt),
			SizeBytes:      snap.SizeBytes,
			ReadyToUse:     true,
		},
	}, nil
}

func (cs *ControllerServer) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	if req.GetSnapshotId() == "" {
		return nil, status.Error(codes.InvalidArgument, "snapshot_id required")
	}
	sid, err := decodeSnapID(req.GetSnapshotId())
	if err != nil {
		return &csi.DeleteSnapshotResponse{}, nil
	}
	if err := cs.Driver.backend.DeleteSnapshot(ctx, sid.SnapshotID); err != nil {
		klog.Warningf("DeleteSnapshot: %v", err)
	}
	return &csi.DeleteSnapshotResponse{}, nil
}

func (cs *ControllerServer) ListSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	var snaps []SnapshotInfo
	switch {
	case req.GetSnapshotId() != "":
		sid, err := decodeSnapID(req.GetSnapshotId())
		if err != nil {
			return nil, status.Errorf(codes.NotFound, "invalid snapshot_id: %v", err)
		}
		all, err := cs.Driver.backend.ListSnapshots(ctx, sid.OriginalPath)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "ListSnapshots: %v", err)
		}
		for _, s := range all {
			if strings.EqualFold(s.SnapshotID, sid.SnapshotID) {
				snaps = []SnapshotInfo{s}
				break
			}
		}
	case req.GetSourceVolumeId() != "":
		vid, err := decodeVolID(req.GetSourceVolumeId())
		if err != nil {
			return nil, status.Errorf(codes.NotFound, "invalid source_volume_id: %v", err)
		}
		var e error
		snaps, e = cs.Driver.backend.ListSnapshots(ctx, vid.VHDXPath)
		if e != nil {
			return nil, status.Errorf(codes.Internal, "ListSnapshots: %v", e)
		}
	default:
		// Not implemented: global enumeration (return empty page)
		return &csi.ListSnapshotsResponse{}, nil
	}

	entries := make([]*csi.ListSnapshotsResponse_Entry, 0, len(snaps))
	for _, s := range snaps {
		id := encodeSnapID(snapID{SnapshotID: s.SnapshotID, OriginalPath: s.OriginalPath})
		entries = append(entries, &csi.ListSnapshotsResponse_Entry{
			Snapshot: &csi.Snapshot{
				SnapshotId:   id,
				CreationTime: timestamppb.New(s.CreatedAt),
				SizeBytes:    s.SizeBytes,
				ReadyToUse:   true,
			},
		})
	}
	return &csi.ListSnapshotsResponse{Entries: entries}, nil
}

// ---------- expansion ----------

func (cs *ControllerServer) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	if req.GetVolumeId() == "" || req.GetCapacityRange() == nil {
		return nil, status.Error(codes.InvalidArgument, "volume_id and capacity_range are required")
	}
	id, err := decodeVolID(req.GetVolumeId())
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "invalid volume_id: %v", err)
	}
	want := req.GetCapacityRange().GetRequiredBytes()
	if want <= 0 {
		return nil, status.Error(codes.InvalidArgument, "required_bytes must be > 0")
	}
	actual, err := cs.Driver.backend.ResizeVirtualDisk(ctx, id.VHDXPath, want)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "ResizeVirtualDisk: %v", err)
	}
	// Node must rescan + grow filesystem
	return &csi.ControllerExpandVolumeResponse{
		CapacityBytes:         actual,
		NodeExpansionRequired: true,
	}, nil
}

// ---------- get volume ----------

func (cs *ControllerServer) ControllerGetVolume(ctx context.Context, req *csi.ControllerGetVolumeRequest) (*csi.ControllerGetVolumeResponse, error) {
	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "volume_id required")
	}
	id, err := decodeVolID(req.GetVolumeId())
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "invalid volume_id: %v", err)
	}
	vi, err := cs.Driver.backend.GetVolumeInfo(ctx, id.VHDXPath)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "GetVolumeInfo: %v", err)
	}
	if vi.VHDXPath == "" {
		return nil, status.Errorf(codes.NotFound, "volume not found")
	}
	published, _ := cs.Driver.backend.GetTargetInitiators(ctx, id.TargetIQN)

	lunStr := ""
	if vi.LUN != nil {
		lunStr = strconv.Itoa(int(*vi.LUN))
	}
	return &csi.ControllerGetVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      req.GetVolumeId(),
			CapacityBytes: vi.SizeBytes,
			VolumeContext: map[string]string{
				"vhdxPath": id.VHDXPath,
				"iqn":      id.TargetIQN,
				"lun":      lunStr,
			},
		},
		Status: &csi.ControllerGetVolumeResponse_VolumeStatus{
			PublishedNodeIds: published,
			VolumeCondition: &csi.VolumeCondition{
				Abnormal: false,
				Message:  "OK",
			},
		},
	}, nil
}
