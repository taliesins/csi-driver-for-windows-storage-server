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
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	klog "k8s.io/klog/v2"

	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	chapSecretMinLength = 12
	chapSecretMaxLength = 16
	maxTCPPort          = 65535
)

var iqnPrefixPattern = regexp.MustCompile(`^iqn\.[0-9]{4}-[0-9]{2}\.[A-Za-z0-9][A-Za-z0-9.-]*$`)

// ControllerServer implements the CSI Controller service.
type ControllerServer struct {
	Driver *driver
	csi.UnimplementedControllerServer
}

/*
Assumptions / contracts:

- cs.Driver.backend provides the following methods (see your WinRM backend):
  EnsureTarget(ctx, targetName, targetIQN) (actualTargetIQN string, err error)
  ConfigureTargetChap(ctx, targetName string, opts TargetChapOptions) error
  CreateVirtualDisk(ctx, name, parentDir string, sizeBytes int64) (vhdxPath string, actualSize int64, err error)
  MapDiskToTarget(ctx, targetName, vhdxPath string) (lun int32, err error)
  UnmapDiskFromTarget(ctx, targetName, vhdxPath string) error
  DeleteVirtualDisk(ctx, vhdxPath string) error
  GetVolumeByName(ctx, name, parentDir string) (exists bool, vhdxPath string, sizeBytes int64, targetName string, targetIQN string, lun int32, err error)
  AllowInitiator(ctx, targetName, initiatorIQN string) error
  DenyInitiator(ctx, targetName, initiatorIQN string) error
  GetDirectoryFreeCapacity(ctx, parentDir string) (freeBytes int64, err error)
  // 03-snapshots
  CreateSnapshot(ctx, vhdxPath, description string) (SnapshotInfo, error)
  DeleteSnapshot(ctx, snapshotID string) error
  ListSnapshots(ctx context.Context, vhdxPath string) ([]SnapshotInfo, error)
  ExportSnapshotAsVirtualDisk(ctx context.Context, snapshotID string) (exportedVHDXPath string, err error)
  // expansion + query
  ResizeVirtualDisk(ctx context.Context, vhdxPath string, newSizeBytes int64) (actualSizeBytes int64, err error)
  GetVolumeInfo(ctx context.Context, vhdxPath string) (VolumeInfo, error)
  GetTargetInitiators(ctx context.Context, targetName string) ([]string, error)
*/

// ---------- helper types ----------

type volID struct {
	VolumeName   string `json:"name"`
	TargetPortal string `json:"targetPortal"` // host:port
	TargetName   string `json:"targetName,omitempty"`
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
	SourceVolume string
	Description  string
	CreatedAt    time.Time
	SizeBytes    int64
	ReadyToUse   bool
}
type VolumeInfo struct {
	VolumeName    string
	Protocol      Protocol
	TargetPortal  string
	TargetName    string
	TargetIQN     string
	LUN           any
	VHDXPath      string
	SharePath     string
	NfsServer     string
	NfsExportPath string
	SmbServer     string
	SmbShareName  string
	CapacityBytes int64
	CreatedAt     time.Time
	SizeBytes     int64
	Targets       []string
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

func boolPtr(v bool) *bool {
	return &v
}

func parseBoolParam(params map[string]string, key string) (*bool, error) {
	raw, ok := getStringParam(params, key)
	if !ok {
		return nil, nil
	}
	switch strings.ToLower(raw) {
	case "1", "true", "yes", "y", "on":
		return boolPtr(true), nil
	case "0", "false", "no", "n", "off":
		return boolPtr(false), nil
	default:
		return nil, status.Errorf(codes.InvalidArgument, "parameter %s must be a boolean", key)
	}
}

func parseIntParam(params map[string]string, key string) (*int, error) {
	raw, ok := getStringParam(params, key)
	if !ok {
		return nil, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "parameter %s must be an integer", key)
	}
	return &v, nil
}

func parseUint32Param(params map[string]string, key string) (uint32, error) {
	raw, ok := getStringParam(params, key)
	if !ok {
		return 0, nil
	}
	v, err := strconv.ParseUint(raw, 10, 32)
	if err != nil {
		return 0, status.Errorf(codes.InvalidArgument, "parameter %s must be a non-negative integer", key)
	}
	return uint32(v), nil
}

func validateTCPPort(key string, port int) error {
	if port < 1 || port > maxTCPPort {
		return status.Errorf(codes.InvalidArgument, "parameter %s must be between 1 and 65535", key)
	}
	return nil
}

func parseTCPPortParam(params map[string]string, key string, fallback int) (int, error) {
	raw, ok := getStringParam(params, key)
	if !ok {
		return fallback, validateTCPPort(key, fallback)
	}
	port, err := strconv.Atoi(raw)
	if err != nil {
		return 0, status.Errorf(codes.InvalidArgument, "parameter %s must be an integer between 1 and 65535", key)
	}
	if err := validateTCPPort(key, port); err != nil {
		return 0, err
	}
	return port, nil
}

func validateTargetPortal(value string) error {
	portal := strings.TrimSpace(value)
	if portal == "" {
		return status.Error(codes.InvalidArgument, "parameter targetPortal is required")
	}
	if strings.Contains(portal, "://") || strings.ContainsAny(portal, `/\`) {
		return status.Error(codes.InvalidArgument, "parameter targetPortal must be a host, IP address, or host:port, not a URL or path")
	}
	if strings.ContainsFunc(portal, unicode.IsSpace) {
		return status.Error(codes.InvalidArgument, "parameter targetPortal must not contain whitespace")
	}
	if host, portStr, err := net.SplitHostPort(portal); err == nil {
		if strings.TrimSpace(host) == "" {
			return status.Error(codes.InvalidArgument, "parameter targetPortal host must not be empty")
		}
		port, err := strconv.Atoi(portStr)
		if err != nil {
			return status.Error(codes.InvalidArgument, "parameter targetPortal port must be an integer between 1 and 65535")
		}
		return validateTCPPort("targetPortal port", port)
	}
	if strings.Contains(portal, ":") && net.ParseIP(portal) == nil {
		return status.Error(codes.InvalidArgument, "parameter targetPortal with a port must be formatted as host:port or [ipv6]:port")
	}
	return nil
}

func validateWindowsPathParam(key, value string, required bool) error {
	path := strings.TrimSpace(value)
	if path == "" {
		if required {
			return status.Errorf(codes.InvalidArgument, "parameter %s is required", key)
		}
		return nil
	}
	if !isWindowsAbsolutePath(path) {
		return status.Errorf(codes.InvalidArgument, "parameter %s must be an absolute Windows path or UNC path", key)
	}
	normalized := strings.ReplaceAll(path, "/", `\`)
	normalized = strings.TrimPrefix(normalized, `\\?\`)
	if strings.Contains(normalized, `\..\`) || strings.HasSuffix(normalized, `\..`) || strings.HasPrefix(normalized, `..\`) {
		return status.Errorf(codes.InvalidArgument, "parameter %s must not contain '..' path segments", key)
	}
	if hasInvalidWindowsPathChar(normalized) {
		return status.Errorf(codes.InvalidArgument, "parameter %s contains invalid Windows path characters", key)
	}
	return nil
}

func isWindowsAbsolutePath(path string) bool {
	path = strings.TrimPrefix(path, `\\?\`)
	if len(path) >= 3 && isASCIIAlpha(path[0]) && path[1] == ':' && (path[2] == '\\' || path[2] == '/') {
		return true
	}
	if strings.HasPrefix(path, `\\`) || strings.HasPrefix(path, `//`) {
		trimmed := strings.TrimLeft(path, `\/`)
		parts := strings.FieldsFunc(trimmed, func(r rune) bool { return r == '\\' || r == '/' })
		return len(parts) >= 2 && strings.TrimSpace(parts[0]) != "" && strings.TrimSpace(parts[1]) != ""
	}
	return false
}

func isASCIIAlpha(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
}

func hasInvalidWindowsPathChar(path string) bool {
	for i, r := range path {
		if unicode.IsControl(r) {
			return true
		}
		switch r {
		case '<', '>', '"', '|', '?', '*':
			return true
		case ':':
			if i != 1 {
				return true
			}
		}
	}
	return false
}

func validateIQNPrefix(prefix string) error {
	value := strings.TrimSpace(prefix)
	if value == "" {
		return nil
	}
	if strings.ContainsFunc(value, unicode.IsSpace) || strings.Contains(value, ":") || !iqnPrefixPattern.MatchString(value) {
		return status.Error(codes.InvalidArgument, "parameter iqnPrefix must look like iqn.yyyy-mm.reverse.domain and must not contain whitespace or ':'")
	}
	return nil
}

func validateIQNValue(key, value string) error {
	iqn := strings.TrimSpace(value)
	if iqn == "" {
		return status.Errorf(codes.InvalidArgument, "%s must not be empty", key)
	}
	prefix, suffix, ok := strings.Cut(iqn, ":")
	if !ok || strings.TrimSpace(suffix) == "" {
		return status.Errorf(codes.InvalidArgument, "%s must look like iqn.yyyy-mm.reverse.domain:name", key)
	}
	if err := validateIQNPrefix(prefix); err != nil {
		return status.Errorf(codes.InvalidArgument, "%s must look like iqn.yyyy-mm.reverse.domain:name", key)
	}
	if strings.ContainsFunc(suffix, unicode.IsSpace) || strings.ContainsAny(suffix, `/\`) || hasInvalidWindowsPathChar(strings.ReplaceAll(suffix, ":", "")) {
		return status.Errorf(codes.InvalidArgument, "%s suffix contains unsupported characters", key)
	}
	return nil
}

func validateISCSITargetName(name string) error {
	value := strings.TrimSpace(name)
	if value == "" {
		return status.Error(codes.InvalidArgument, "iSCSI target name must not be empty")
	}
	if len(value) > 223 {
		return status.Error(codes.InvalidArgument, "iSCSI target name must be no more than 223 characters")
	}
	if strings.HasPrefix(strings.ToLower(value), "iqn.") {
		return validateIQNValue("iSCSI target IQN", value)
	}
	if strings.ContainsFunc(value, unicode.IsSpace) || hasInvalidWindowsPathChar(value) || strings.ContainsAny(value, `/\`) {
		return status.Errorf(codes.InvalidArgument, "iSCSI target name %q contains unsupported characters", name)
	}
	return nil
}

func normalizeNfsPermission(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "rw", "readwrite", "read-write":
		return "readwrite", nil
	case "ro", "readonly", "read-only":
		return "readonly", nil
	default:
		return "", status.Errorf(codes.InvalidArgument, "parameter nfsPermission must be readonly/ro or readwrite/rw")
	}
}

func normalizeNfsAuthentication(value string) ([]string, error) {
	rawValues := splitCSVParam(value)
	if len(rawValues) == 0 {
		return nil, nil
	}
	auth := make([]string, 0, len(rawValues))
	seen := map[string]bool{}
	for _, raw := range rawValues {
		normalized, err := normalizeNfsAuthenticationFlavor(raw)
		if err != nil {
			return nil, err
		}
		if !seen[normalized] {
			auth = append(auth, normalized)
			seen[normalized] = true
		}
	}
	return auth, nil
}

func normalizeNfsAuthenticationFlavor(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "sys":
		return "sys", nil
	case "krb5", "kerberos":
		return "krb5", nil
	case "krb5i", "kerberos-integrity":
		return "krb5i", nil
	case "krb5p", "kerberos-privacy":
		return "krb5p", nil
	default:
		return "", status.Errorf(codes.InvalidArgument, "parameter nfsAuthentication must contain only sys, krb5, krb5i, or krb5p")
	}
}

func nfsKerberosAuthentication(value string) ([]string, string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, "", nil
	}
	if parsed, err := strconv.ParseBool(value); err == nil {
		if !parsed {
			return nil, "", nil
		}
		return []string{"krb5"}, "krb5", nil
	}
	flavor, err := normalizeNfsAuthenticationFlavor(value)
	if err != nil {
		return nil, "", status.Errorf(codes.InvalidArgument, "parameter nfsKerberos must be a boolean or one of krb5, krb5i, krb5p")
	}
	if flavor == "sys" {
		return nil, "", status.Errorf(codes.InvalidArgument, "parameter nfsKerberos must be a boolean or one of krb5, krb5i, krb5p")
	}
	return []string{flavor}, flavor, nil
}

func preferredNfsMountAuthentication(auth []string) string {
	rank := map[string]int{"sys": 1, "krb5": 2, "krb5i": 3, "krb5p": 4}
	best := ""
	for _, value := range auth {
		if rank[value] > rank[best] {
			best = value
		}
	}
	return best
}

func nfsAuthenticationParam(params map[string]string) string {
	if v, ok := getStringParam(params, "nfsAuthentication"); ok {
		return v
	}
	if v, ok := getStringParam(params, "nfsauthentication"); ok {
		return v
	}
	return ""
}

func normalizeNfsClientType(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "host":
		return "host", nil
	case "clientgroup", "client-group":
		return "clientgroup", nil
	case "netgroup", "net-group":
		return "netgroup", nil
	default:
		return "", status.Error(codes.InvalidArgument, "parameter nfsClientType must be host, clientgroup, or netgroup")
	}
}

func normalizeSmbCachingMode(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return "", nil
	case "none":
		return "None", nil
	case "manual":
		return "Manual", nil
	case "documents":
		return "Documents", nil
	case "programs":
		return "Programs", nil
	case "branchcache", "branch-cache":
		return "BranchCache", nil
	default:
		return "", status.Error(codes.InvalidArgument, "parameter smbCachingMode must be None, Manual, Documents, Programs, or BranchCache")
	}
}

func normalizeSmbFolderEnumerationMode(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return "", nil
	case "accessbased", "access-based":
		return "AccessBased", nil
	case "unrestricted":
		return "Unrestricted", nil
	default:
		return "", status.Error(codes.InvalidArgument, "parameter smbFolderEnumerationMode must be AccessBased or Unrestricted")
	}
}

func (cs *ControllerServer) fileShareBackendFromParams(params map[string]string) (string, error) {
	defaultBackend := fileShareBackendDirectory
	if cs.Driver != nil && cs.Driver.fileShareBackend != "" {
		defaultBackend = cs.Driver.fileShareBackend
	}
	backend := defaultBackend
	if raw, ok := getStringParam(params, "shareBackend"); ok {
		backend = strings.ToLower(raw)
	}
	switch backend {
	case fileShareBackendDirectory, fileShareBackendVHDX:
	default:
		return "", status.Errorf(codes.InvalidArgument, "parameter shareBackend must be %q or %q", fileShareBackendDirectory, fileShareBackendVHDX)
	}
	if cs.Driver != nil && cs.Driver.fileShareBackend != "" && backend != cs.Driver.fileShareBackend {
		return "", status.Errorf(codes.InvalidArgument, "driver %s only supports shareBackend=%s", cs.Driver.name, cs.Driver.fileShareBackend)
	}
	return backend, nil
}

func fileShareVolumeID(name string, protocol Protocol, backend string, info VolumeInfo, capacityBytes int64) *VolumeID {
	vid := &VolumeID{
		Name:          name,
		Protocol:      protocol,
		ShareBackend:  backend,
		CapacityBytes: capacityBytes,
	}
	switch protocol {
	case ProtocolNFS:
		vid.NfsServer = info.NfsServer
		vid.NfsExportPath = info.NfsExportPath
	case ProtocolSMB:
		vid.SmbServer = info.SmbServer
		vid.SmbShareName = info.SmbShareName
	}
	if backend == fileShareBackendVHDX {
		vid.VHDXPath = info.VHDXPath
		vid.SharePath = info.SharePath
	} else {
		vid.VHDXPath = info.VHDXPath
		vid.SharePath = info.VHDXPath
	}
	return vid
}

func applyNfsOptionsToVolumeID(vid *VolumeID, opts NfsShareOptions) *VolumeID {
	if vid == nil || vid.Protocol != ProtocolNFS {
		return vid
	}
	if len(opts.Authentication) > 0 {
		vid.NfsAuthentication = strings.Join(opts.Authentication, ",")
	}
	if opts.MountAuthentication != "" {
		vid.NfsMountAuthentication = opts.MountAuthentication
	}
	return vid
}

func joinWindowsPath(parent, child string) string {
	parent = strings.TrimRight(strings.TrimSpace(parent), `\/`)
	child = strings.TrimLeft(strings.TrimSpace(child), `\/`)
	if parent == "" {
		return child
	}
	if child == "" {
		return parent
	}
	return parent + `\` + child
}

func nfsOptionsFromParams(params map[string]string) (NfsShareOptions, error) {
	allowRootAccess, err := parseBoolParam(params, "nfsAllowRootAccess")
	if err != nil {
		return NfsShareOptions{}, err
	}
	enableAnonymousAccess, err := parseBoolParam(params, "nfsEnableAnonymousAccess")
	if err != nil {
		return NfsShareOptions{}, err
	}
	enableUnmappedAccess, err := parseBoolParam(params, "nfsEnableUnmappedAccess")
	if err != nil {
		return NfsShareOptions{}, err
	}
	anonymousUID, err := parseIntParam(params, "nfsAnonymousUid")
	if err != nil {
		return NfsShareOptions{}, err
	}
	anonymousGID, err := parseIntParam(params, "nfsAnonymousGid")
	if err != nil {
		return NfsShareOptions{}, err
	}
	permission, err := normalizeNfsPermission(params["nfsPermission"])
	if err != nil {
		return NfsShareOptions{}, err
	}
	authentication, err := normalizeNfsAuthentication(nfsAuthenticationParam(params))
	if err != nil {
		return NfsShareOptions{}, err
	}
	mountAuthentication := ""
	if v, ok := getStringParam(params, "nfsMountAuthentication"); ok {
		mountAuthentication, err = normalizeNfsAuthenticationFlavor(v)
		if err != nil {
			return NfsShareOptions{}, status.Errorf(codes.InvalidArgument, "parameter nfsMountAuthentication must be one of sys, krb5, krb5i, or krb5p")
		}
	}
	if len(authentication) == 0 {
		authentication, mountAuthentication, err = nfsKerberosAuthentication(params["nfsKerberos"])
		if err != nil {
			return NfsShareOptions{}, err
		}
	}
	if mountAuthentication == "" {
		mountAuthentication = preferredNfsMountAuthentication(authentication)
	}
	clientType, err := normalizeNfsClientType(params["nfsClientType"])
	if err != nil {
		return NfsShareOptions{}, err
	}
	return NfsShareOptions{
		ClientType:            clientType,
		Permission:            permission,
		AllowRootAccess:       allowRootAccess,
		Authentication:        authentication,
		MountAuthentication:   mountAuthentication,
		AnonymousUID:          anonymousUID,
		AnonymousGID:          anonymousGID,
		LanguageEncoding:      strings.TrimSpace(params["nfsLanguageEncoding"]),
		EnableAnonymousAccess: enableAnonymousAccess,
		EnableUnmappedAccess:  enableUnmappedAccess,
	}, nil
}

func smbOptionsFromParams(params map[string]string) (SmbShareOptions, error) {
	encryptData, err := parseBoolParam(params, "smbEncryptData")
	if err != nil {
		return SmbShareOptions{}, err
	}
	compressData, err := parseBoolParam(params, "smbCompressData")
	if err != nil {
		return SmbShareOptions{}, err
	}
	continuouslyAvailable, err := parseBoolParam(params, "smbContinuouslyAvailable")
	if err != nil {
		return SmbShareOptions{}, err
	}
	concurrentUserLimit, err := parseUint32Param(params, "smbConcurrentUserLimit")
	if err != nil {
		return SmbShareOptions{}, err
	}
	cachingMode, err := normalizeSmbCachingMode(params["smbCachingMode"])
	if err != nil {
		return SmbShareOptions{}, err
	}
	folderEnumerationMode, err := normalizeSmbFolderEnumerationMode(params["smbFolderEnumerationMode"])
	if err != nil {
		return SmbShareOptions{}, err
	}
	return SmbShareOptions{
		NoAccess:              splitCSVParam(params["smbNoAccess"]),
		Description:           strings.TrimSpace(params["smbDescription"]),
		EncryptData:           encryptData,
		CompressData:          compressData,
		ContinuouslyAvailable: continuouslyAvailable,
		CachingMode:           cachingMode,
		FolderEnumerationMode: folderEnumerationMode,
		ConcurrentUserLimit:   concurrentUserLimit,
	}, nil
}

func (cs *ControllerServer) protocolFromParams(params map[string]string) (Protocol, error) {
	raw := strings.ToLower(strings.TrimSpace(params["protocol"]))
	if raw == "" {
		if cs.Driver != nil && cs.Driver.protocol != "" {
			return cs.Driver.protocol, nil
		}
		return ProtocolISCSI, nil
	}
	switch Protocol(raw) {
	case ProtocolISCSI, ProtocolNFS, ProtocolSMB:
		protocol := Protocol(raw)
		if cs.Driver != nil && cs.Driver.protocol != "" && protocol != cs.Driver.protocol {
			return "", status.Errorf(codes.InvalidArgument, "protocol %s does not match CSI driver %s", protocol, cs.Driver.name)
		}
		return protocol, nil
	default:
		return "", status.Errorf(codes.InvalidArgument, "unsupported protocol: %s", raw)
	}
}

func decodeAnyVolumeID(id string) (*VolumeID, volID, error) {
	if v, err := DecodeVolumeID(id); err == nil {
		return v, volID{
			VolumeName:   v.Name,
			TargetPortal: v.TargetPortal,
			TargetName:   firstNonEmpty(v.TargetName, v.TargetIQN),
			TargetIQN:    v.TargetIQN,
			LUN:          int32(v.LUN),
			VHDXPath:     v.VHDXPath,
			SizeBytes:    v.CapacityBytes,
		}, nil
	}
	legacy, err := decodeVolID(id)
	if err != nil {
		return nil, volID{}, err
	}
	if legacy.TargetName == "" {
		legacy.TargetName = legacy.TargetIQN
	}
	return &VolumeID{
		Name:          legacy.VolumeName,
		Protocol:      ProtocolISCSI,
		TargetPortal:  legacy.TargetPortal,
		TargetName:    legacy.TargetName,
		TargetIQN:     legacy.TargetIQN,
		LUN:           int(legacy.LUN),
		VHDXPath:      legacy.VHDXPath,
		CapacityBytes: legacy.SizeBytes,
	}, legacy, nil
}

func iscsiTargetForVolume(params map[string]string, volName string) (targetName, targetIQN string) {
	if iqnPrefix, ok := getStringParam(params, "iqnPrefix"); ok {
		targetIQN = fmt.Sprintf("%s:%s", iqnPrefix, volName)
		return targetIQN, targetIQN
	}
	return volName, ""
}

func targetNameFromVolID(id volID) string {
	return firstNonEmpty(id.TargetName, id.TargetIQN)
}

func targetPortalAddress(targetPortal string, portalPort int) string {
	targetPortal = strings.TrimSpace(targetPortal)
	if targetPortal == "" {
		return ""
	}
	if _, _, err := net.SplitHostPort(targetPortal); err == nil {
		return targetPortal
	}
	return net.JoinHostPort(targetPortal, strconv.Itoa(portalPort))
}

func withIscsiVolumeContextParams(ctx map[string]string, params map[string]string) map[string]string {
	if iface, ok := getStringParam(params, "iface"); ok {
		ctx["iface"] = iface
	}
	return ctx
}

func validateSecretPrintable(key, value string) error {
	for _, r := range value {
		if unicode.IsControl(r) {
			return status.Errorf(codes.InvalidArgument, "%s must not contain control characters", key)
		}
	}
	return nil
}

func validateChapSecretValue(key, value string) error {
	if err := validateSecretPrintable(key, value); err != nil {
		return err
	}
	if length := len(value); length < chapSecretMinLength || length > chapSecretMaxLength {
		return status.Errorf(codes.InvalidArgument, "%s must be between 12 and 16 characters", key)
	}
	return nil
}

func validateChapUsernameValue(key, value string) error {
	if err := validateSecretPrintable(key, value); err != nil {
		return err
	}
	if len(value) > 223 {
		return status.Errorf(codes.InvalidArgument, "%s must be no more than 223 characters", key)
	}
	if strings.ContainsFunc(value, unicode.IsSpace) {
		return status.Errorf(codes.InvalidArgument, "%s must not contain whitespace", key)
	}
	return nil
}

func validateChapCredentialPair(secrets map[string]string, userKey, passwordKey string) (user, password string, configured bool, err error) {
	user = strings.TrimSpace(secrets[userKey])
	password = strings.TrimSpace(secrets[passwordKey])
	if user == "" && password == "" {
		return "", "", false, nil
	}
	if user == "" || password == "" {
		return "", "", false, status.Errorf(codes.InvalidArgument, "%s and %s must both be set", userKey, passwordKey)
	}
	if err := validateChapUsernameValue(userKey, user); err != nil {
		return "", "", false, err
	}
	if err := validateChapSecretValue(passwordKey, password); err != nil {
		return "", "", false, err
	}
	return user, password, true, nil
}

func validateChapSecrets(secrets map[string]string) error {
	_, _, discoveryConfigured, err := validateChapCredentialPair(secrets, "discovery.sendtargets.auth.username", "discovery.sendtargets.auth.password")
	if err != nil {
		return err
	}
	_, _, discoveryReverseConfigured, err := validateChapCredentialPair(secrets, "discovery.sendtargets.auth.username_in", "discovery.sendtargets.auth.password_in")
	if err != nil {
		return err
	}
	if discoveryReverseConfigured && !discoveryConfigured {
		return status.Error(codes.InvalidArgument, "discovery reverse CHAP requires discovery.sendtargets.auth.username and discovery.sendtargets.auth.password")
	}

	_, _, sessionConfigured, err := validateChapCredentialPair(secrets, "node.session.auth.username", "node.session.auth.password")
	if err != nil {
		return err
	}
	_, _, reverseConfigured, err := validateChapCredentialPair(secrets, "node.session.auth.username_in", "node.session.auth.password_in")
	if err != nil {
		return err
	}
	if reverseConfigured && !sessionConfigured {
		return status.Error(codes.InvalidArgument, "reverse CHAP requires node.session.auth.username and node.session.auth.password")
	}
	return nil
}

func targetChapOptionsFromSecrets(secrets map[string]string) (TargetChapOptions, error) {
	if err := validateChapSecrets(secrets); err != nil {
		return TargetChapOptions{}, err
	}
	chapUser := strings.TrimSpace(secrets["node.session.auth.username"])
	chapSecret := strings.TrimSpace(secrets["node.session.auth.password"])
	reverseChapUser := strings.TrimSpace(secrets["node.session.auth.username_in"])
	reverseChapSecret := strings.TrimSpace(secrets["node.session.auth.password_in"])

	opts := TargetChapOptions{}
	if chapUser != "" || chapSecret != "" {
		if chapUser == "" || chapSecret == "" {
			return TargetChapOptions{}, status.Error(codes.InvalidArgument, "node.session.auth.username and node.session.auth.password must both be set for Windows target CHAP")
		}
		opts.ChapUser = chapUser
		opts.ChapSecret = chapSecret
	}
	if reverseChapUser != "" || reverseChapSecret != "" {
		if reverseChapUser == "" || reverseChapSecret == "" {
			return TargetChapOptions{}, status.Error(codes.InvalidArgument, "node.session.auth.username_in and node.session.auth.password_in must both be set for Windows target reverse CHAP")
		}
		if opts.ChapUser == "" {
			return TargetChapOptions{}, status.Error(codes.InvalidArgument, "reverse CHAP requires node.session.auth.username and node.session.auth.password")
		}
		opts.ReverseChapUser = reverseChapUser
		opts.ReverseChapSecret = reverseChapSecret
	}
	return opts, nil
}

func (cs *ControllerServer) configureTargetChapFromSecrets(ctx context.Context, targetName string, secrets map[string]string) error {
	opts, err := targetChapOptionsFromSecrets(secrets)
	if err != nil {
		return err
	}
	if !opts.Enabled() {
		return nil
	}
	if err := cs.Driver.backend.ConfigureTargetChap(ctx, targetName, opts); err != nil {
		return status.Errorf(codes.Internal, "ConfigureTargetChap: %v", err)
	}
	return nil
}

func volumeContextForVolumeID(v *VolumeID) map[string]string {
	switch v.Protocol {
	case ProtocolNFS:
		ctx := map[string]string{
			"protocol":      string(ProtocolNFS),
			"server":        v.NfsServer,
			"nfsServer":     v.NfsServer,
			"exportPath":    v.NfsExportPath,
			"nfsExportPath": v.NfsExportPath,
		}
		if v.NfsAuthentication != "" {
			ctx["nfsAuthentication"] = v.NfsAuthentication
		}
		if v.NfsMountAuthentication != "" {
			ctx["nfsMountAuthentication"] = v.NfsMountAuthentication
		}
		return ctx
	case ProtocolSMB:
		return map[string]string{
			"protocol":     string(ProtocolSMB),
			"server":       v.SmbServer,
			"smbServer":    v.SmbServer,
			"share":        v.SmbShareName,
			"smbShareName": v.SmbShareName,
		}
	default:
		ctx := map[string]string{
			"targetPortal": v.TargetPortal,
			"iqn":          v.TargetIQN,
			"lun":          strconv.Itoa(v.LUN),
		}
		if strings.TrimSpace(v.TargetName) != "" {
			ctx["targetName"] = v.TargetName
		}
		return ctx
	}
}

func (cs *ControllerServer) supportsAccessModeForProtocol(mode csi.VolumeCapability_AccessMode_Mode, protocol Protocol) bool {
	switch mode {
	case csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
		csi.VolumeCapability_AccessMode_SINGLE_NODE_SINGLE_WRITER,
		csi.VolumeCapability_AccessMode_SINGLE_NODE_MULTI_WRITER:
		return true
	case csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY,
		csi.VolumeCapability_AccessMode_MULTI_NODE_SINGLE_WRITER,
		csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER:
		if protocol != "" {
			return protocol == ProtocolNFS || protocol == ProtocolSMB
		}
		return cs.Driver != nil && (cs.Driver.protocol == "" || cs.Driver.protocol == ProtocolNFS || cs.Driver.protocol == ProtocolSMB)
	default:
		return false
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
	params := req.GetParameters()
	protocol, err := cs.protocolFromParams(params)
	if err != nil {
		return nil, err
	}
	for _, vc := range req.GetVolumeCapabilities() {
		if !cs.supportsAccessModeForProtocol(vc.GetAccessMode().GetMode(), protocol) {
			return nil, status.Error(codes.InvalidArgument, "access mode is not supported by this driver")
		}
	}

	if protocol == ProtocolNFS {
		return cs.createNfsVolume(ctx, req)
	}
	if protocol == ProtocolSMB {
		return cs.createSmbVolume(ctx, req)
	}

	targetPortal, ok := getStringParam(params, "targetPortal")
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "parameter targetPortal is required")
	}
	if err := validateTargetPortal(targetPortal); err != nil {
		return nil, err
	}
	portalPort, err := parseTCPPortParam(params, "portalPort", 3260)
	if err != nil {
		return nil, err
	}
	parentDir, _ := getStringParam(params, "vhdxParentPath")
	if err := validateWindowsPathParam("vhdxParentPath", parentDir, false); err != nil {
		return nil, err
	}
	if iqnPrefix, ok := getStringParam(params, "iqnPrefix"); ok {
		if err := validateIQNPrefix(iqnPrefix); err != nil {
			return nil, err
		}
	}

	size, err := requiredBytesFromRange(req.GetCapacityRange(), 1)
	if err != nil {
		return nil, err
	}
	volName := req.GetName()
	targetName, requestedTargetIQN := iscsiTargetForVolume(params, volName)
	if err := validateISCSITargetName(targetName); err != nil {
		return nil, err
	}
	targetPortalWithPort := targetPortalAddress(targetPortal, portalPort)

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
		targetIQN, err := cs.Driver.backend.EnsureTarget(ctx, targetName, requestedTargetIQN)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "EnsureTarget: %v", err)
		}
		if targetIQN == "" {
			return nil, status.Errorf(codes.Internal, "EnsureTarget returned an empty target IQN for target %q", targetName)
		}
		if err := cs.configureTargetChapFromSecrets(ctx, targetName, req.GetSecrets()); err != nil {
			return nil, err
		}
		lun, err := cs.Driver.backend.MapDiskToTarget(ctx, targetName, exportedPath)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "MapDiskToTarget(exported): %v", err)
		}
		vi, _ := cs.Driver.backend.GetVolumeInfo(ctx, exportedPath)
		vid := encodeVolID(volID{
			VolumeName:   volName,
			TargetPortal: targetPortalWithPort,
			TargetName:   targetName,
			TargetIQN:    targetIQN,
			LUN:          lun,
			VHDXPath:     exportedPath,
			SizeBytes:    vi.SizeBytes,
		})
		return &csi.CreateVolumeResponse{
			Volume: &csi.Volume{
				VolumeId:      vid,
				CapacityBytes: vi.SizeBytes,
				VolumeContext: withIscsiVolumeContextParams(map[string]string{
					"targetPortal": targetPortalWithPort,
					"iqn":          targetIQN,
					"lun":          strconv.Itoa(int(lun)),
					"source":       "snapshot",
				}, params),
				ContentSource: req.GetVolumeContentSource(),
			},
		}, nil
	}

	// Idempotency: already exists?
	exists, vhdxPath, existingSize, existingTargetName, existingTargetIQN, existingLUN, err := cs.Driver.backend.GetVolumeByName(ctx, volName, parentDir)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "GetVolumeByName: %v", err)
	}
	if exists {
		if size > 0 && existingSize > 0 && size > existingSize {
			return nil, status.Errorf(codes.AlreadyExists, "volume %q exists smaller (%dB) than requested (%dB)", volName, existingSize, size)
		}
		if existingTargetName == "" {
			existingTargetName = targetName
		}
		targetIQN, err := cs.Driver.backend.EnsureTarget(ctx, existingTargetName, firstNonEmpty(existingTargetIQN, requestedTargetIQN))
		if err != nil {
			return nil, status.Errorf(codes.Internal, "EnsureTarget(existing): %v", err)
		}
		if targetIQN == "" {
			return nil, status.Errorf(codes.Internal, "EnsureTarget returned an empty target IQN for target %q", existingTargetName)
		}
		if existingLUN < 0 {
			lun, err := cs.Driver.backend.MapDiskToTarget(ctx, existingTargetName, vhdxPath)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "MapDiskToTarget(existing): %v", err)
			}
			existingLUN = lun
		}
		vid := encodeVolID(volID{
			VolumeName:   volName,
			TargetPortal: targetPortalWithPort,
			TargetName:   existingTargetName,
			TargetIQN:    targetIQN,
			LUN:          existingLUN,
			VHDXPath:     vhdxPath,
			SizeBytes:    existingSize,
		})
		return &csi.CreateVolumeResponse{
			Volume: &csi.Volume{
				VolumeId:      vid,
				CapacityBytes: existingSize,
				VolumeContext: withIscsiVolumeContextParams(map[string]string{
					"targetPortal": targetPortalWithPort,
					"iqn":          targetIQN,
					"lun":          strconv.Itoa(int(existingLUN)),
				}, params),
			},
		}, nil
	}

	// Create new VHDX and map it
	vhdxPath, actual, err := cs.Driver.backend.CreateVirtualDisk(ctx, volName, parentDir, size)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "CreateVirtualDisk: %v", err)
	}
	targetIQN, err := cs.Driver.backend.EnsureTarget(ctx, targetName, requestedTargetIQN)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "EnsureTarget: %v", err)
	}
	if targetIQN == "" {
		return nil, status.Errorf(codes.Internal, "EnsureTarget returned an empty target IQN for target %q", targetName)
	}
	if err := cs.configureTargetChapFromSecrets(ctx, targetName, req.GetSecrets()); err != nil {
		return nil, err
	}
	lun, err := cs.Driver.backend.MapDiskToTarget(ctx, targetName, vhdxPath)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "MapDiskToTarget: %v", err)
	}

	vid := encodeVolID(volID{
		VolumeName:   volName,
		TargetPortal: targetPortalWithPort,
		TargetName:   targetName,
		TargetIQN:    targetIQN,
		LUN:          lun,
		VHDXPath:     vhdxPath,
		SizeBytes:    actual,
	})
	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      vid,
			CapacityBytes: actual,
			VolumeContext: withIscsiVolumeContextParams(map[string]string{
				"targetPortal": targetPortalWithPort,
				"iqn":          targetIQN,
				"lun":          strconv.Itoa(int(lun)),
			}, params),
		},
	}, nil
}

func (cs *ControllerServer) createNfsVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	params := req.GetParameters()
	parentDir, ok := getStringParam(params, "shareParentPath")
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "parameter shareParentPath is required")
	}
	if err := validateWindowsPathParam("shareParentPath", parentDir, true); err != nil {
		return nil, err
	}
	shareBackend, err := cs.fileShareBackendFromParams(params)
	if err != nil {
		return nil, err
	}
	nfsServer, _ := getStringParam(params, "nfsServer")
	nfsOpts, err := nfsOptionsFromParams(params)
	if err != nil {
		return nil, err
	}
	size, err := requiredBytesFromRange(req.GetCapacityRange(), 1)
	if err != nil {
		return nil, err
	}
	name := req.GetName()
	if shareBackend == fileShareBackendVHDX {
		return cs.createVHDXBackedNfsVolume(ctx, req, name, parentDir, nfsServer, size, nfsOpts)
	}
	if src := req.GetVolumeContentSource(); src != nil && src.GetSnapshot() != nil {
		return nil, status.Error(codes.InvalidArgument, "directory-backed NFS volumes do not support restore from snapshots")
	}
	exists, info, err := cs.Driver.backend.GetNfsShare(ctx, name, parentDir)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "GetNfsShare: %v", err)
	}
	if !exists {
		info, err = cs.Driver.backend.CreateNfsShare(ctx, name, parentDir, size, splitCSVParam(params["nfsClientName"]), nfsOpts)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "CreateNfsShare: %v", err)
		}
	}
	if nfsServer != "" {
		info.NfsServer = nfsServer
	}
	capacityBytes := maxInt64(info.CapacityBytes, size)
	volumeID := applyNfsOptionsToVolumeID(fileShareVolumeID(name, ProtocolNFS, shareBackend, info, capacityBytes), nfsOpts)
	vid := EncodeVolumeID(volumeID)
	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      vid,
			CapacityBytes: capacityBytes,
			VolumeContext: volumeContextForVolumeID(volumeID),
			ContentSource: req.GetVolumeContentSource(),
		},
	}, nil
}

func (cs *ControllerServer) createVHDXBackedNfsVolume(ctx context.Context, req *csi.CreateVolumeRequest, name, shareParentDir, nfsServer string, size int64, nfsOpts NfsShareOptions) (*csi.CreateVolumeResponse, error) {
	params := req.GetParameters()
	vhdxParentDir, _ := getStringParam(params, "vhdxParentPath")
	if err := validateWindowsPathParam("vhdxParentPath", vhdxParentDir, false); err != nil {
		return nil, err
	}
	sharePath := joinWindowsPath(shareParentDir, name)
	exists, info, err := cs.Driver.backend.GetNfsShare(ctx, name, shareParentDir)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "GetNfsShare: %v", err)
	}
	_, existingVHDXPath, existingSize, _, _, _, err := cs.Driver.backend.GetVolumeByName(ctx, name, vhdxParentDir)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "GetVolumeByName: %v", err)
	}
	if !exists {
		vhdxPath := existingVHDXPath
		actual := existingSize
		if src := req.GetVolumeContentSource(); src != nil && src.GetSnapshot() != nil {
			sid, err := decodeSnapID(src.GetSnapshot().GetSnapshotId())
			if err != nil {
				return nil, status.Errorf(codes.InvalidArgument, "invalid snapshotId: %v", err)
			}
			vhdxPath, err = cs.Driver.backend.ExportSnapshotAsVirtualDisk(ctx, sid.SnapshotID)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "ExportSnapshotAsVirtualDisk: %v", err)
			}
			if vi, err := cs.Driver.backend.GetVolumeInfo(ctx, vhdxPath); err == nil {
				actual = vi.SizeBytes
			}
		} else if vhdxPath == "" {
			vhdxPath, actual, err = cs.Driver.backend.CreateVirtualDisk(ctx, name, vhdxParentDir, size)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "CreateVirtualDisk: %v", err)
			}
		}
		if err := cs.Driver.backend.MountFileShareVirtualDisk(ctx, vhdxPath, sharePath); err != nil {
			return nil, status.Errorf(codes.Internal, "MountFileShareVirtualDisk: %v", err)
		}
		info, err = cs.Driver.backend.CreateNfsShare(ctx, name, shareParentDir, maxInt64(size, actual), splitCSVParam(params["nfsClientName"]), nfsOpts)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "CreateNfsShare: %v", err)
		}
		info.VHDXPath = vhdxPath
		info.SharePath = sharePath
		info.SizeBytes = actual
		info.CapacityBytes = maxInt64(info.CapacityBytes, actual)
	} else {
		info.SharePath = firstNonEmpty(info.SharePath, info.VHDXPath, sharePath)
		if existingVHDXPath == "" {
			return nil, status.Errorf(codes.FailedPrecondition, "NFS share %q exists but VHDX %q was not found", name, joinWindowsPath(vhdxParentDir, name+".vhdx"))
		}
		if err := cs.Driver.backend.MountFileShareVirtualDisk(ctx, existingVHDXPath, info.SharePath); err != nil {
			return nil, status.Errorf(codes.Internal, "MountFileShareVirtualDisk(existing): %v", err)
		}
		info.VHDXPath = existingVHDXPath
		info.SizeBytes = existingSize
		info.CapacityBytes = maxInt64(info.CapacityBytes, existingSize)
	}
	if nfsServer != "" {
		info.NfsServer = nfsServer
	}
	capacityBytes := maxInt64(info.CapacityBytes, size)
	volumeID := applyNfsOptionsToVolumeID(fileShareVolumeID(name, ProtocolNFS, fileShareBackendVHDX, info, capacityBytes), nfsOpts)
	vid := EncodeVolumeID(volumeID)
	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      vid,
			CapacityBytes: capacityBytes,
			VolumeContext: volumeContextForVolumeID(volumeID),
			ContentSource: req.GetVolumeContentSource(),
		},
	}, nil
}

func (cs *ControllerServer) createSmbVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	params := req.GetParameters()
	parentDir, ok := getStringParam(params, "shareParentPath")
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "parameter shareParentPath is required")
	}
	if err := validateWindowsPathParam("shareParentPath", parentDir, true); err != nil {
		return nil, err
	}
	shareBackend, err := cs.fileShareBackendFromParams(params)
	if err != nil {
		return nil, err
	}
	smbServer, _ := getStringParam(params, "smbServer")
	smbOpts, err := smbOptionsFromParams(params)
	if err != nil {
		return nil, err
	}
	size, err := requiredBytesFromRange(req.GetCapacityRange(), 1)
	if err != nil {
		return nil, err
	}
	name := req.GetName()
	if shareBackend == fileShareBackendVHDX {
		return cs.createVHDXBackedSmbVolume(ctx, req, name, parentDir, smbServer, size, smbOpts)
	}
	if src := req.GetVolumeContentSource(); src != nil && src.GetSnapshot() != nil {
		return nil, status.Error(codes.InvalidArgument, "directory-backed SMB volumes do not support restore from snapshots")
	}
	exists, info, err := cs.Driver.backend.GetSmbShare(ctx, name, parentDir)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "GetSmbShare: %v", err)
	}
	if !exists {
		info, err = cs.Driver.backend.CreateSmbShare(ctx, name, parentDir, size,
			splitCSVParam(params["smbFullAccess"]),
			splitCSVParam(params["smbChangeAccess"]),
			splitCSVParam(params["smbReadAccess"]),
			smbOpts)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "CreateSmbShare: %v", err)
		}
	}
	if smbServer != "" {
		info.SmbServer = smbServer
	}
	capacityBytes := maxInt64(info.CapacityBytes, size)
	vid := EncodeVolumeID(fileShareVolumeID(name, ProtocolSMB, shareBackend, info, capacityBytes))
	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      vid,
			CapacityBytes: capacityBytes,
			VolumeContext: volumeContextForVolumeID(&VolumeID{
				Protocol:     ProtocolSMB,
				SmbServer:    info.SmbServer,
				SmbShareName: info.SmbShareName,
			}),
			ContentSource: req.GetVolumeContentSource(),
		},
	}, nil
}

func (cs *ControllerServer) createVHDXBackedSmbVolume(ctx context.Context, req *csi.CreateVolumeRequest, name, shareParentDir, smbServer string, size int64, smbOpts SmbShareOptions) (*csi.CreateVolumeResponse, error) {
	params := req.GetParameters()
	vhdxParentDir, _ := getStringParam(params, "vhdxParentPath")
	if err := validateWindowsPathParam("vhdxParentPath", vhdxParentDir, false); err != nil {
		return nil, err
	}
	sharePath := joinWindowsPath(shareParentDir, name)
	exists, info, err := cs.Driver.backend.GetSmbShare(ctx, name, shareParentDir)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "GetSmbShare: %v", err)
	}
	_, existingVHDXPath, existingSize, _, _, _, err := cs.Driver.backend.GetVolumeByName(ctx, name, vhdxParentDir)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "GetVolumeByName: %v", err)
	}
	if !exists {
		vhdxPath := existingVHDXPath
		actual := existingSize
		if src := req.GetVolumeContentSource(); src != nil && src.GetSnapshot() != nil {
			sid, err := decodeSnapID(src.GetSnapshot().GetSnapshotId())
			if err != nil {
				return nil, status.Errorf(codes.InvalidArgument, "invalid snapshotId: %v", err)
			}
			vhdxPath, err = cs.Driver.backend.ExportSnapshotAsVirtualDisk(ctx, sid.SnapshotID)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "ExportSnapshotAsVirtualDisk: %v", err)
			}
			if vi, err := cs.Driver.backend.GetVolumeInfo(ctx, vhdxPath); err == nil {
				actual = vi.SizeBytes
			}
		} else if vhdxPath == "" {
			vhdxPath, actual, err = cs.Driver.backend.CreateVirtualDisk(ctx, name, vhdxParentDir, size)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "CreateVirtualDisk: %v", err)
			}
		}
		if err := cs.Driver.backend.MountFileShareVirtualDisk(ctx, vhdxPath, sharePath); err != nil {
			return nil, status.Errorf(codes.Internal, "MountFileShareVirtualDisk: %v", err)
		}
		info, err = cs.Driver.backend.CreateSmbShare(ctx, name, shareParentDir, maxInt64(size, actual),
			splitCSVParam(params["smbFullAccess"]),
			splitCSVParam(params["smbChangeAccess"]),
			splitCSVParam(params["smbReadAccess"]),
			smbOpts)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "CreateSmbShare: %v", err)
		}
		info.VHDXPath = vhdxPath
		info.SharePath = sharePath
		info.SizeBytes = actual
		info.CapacityBytes = maxInt64(info.CapacityBytes, actual)
	} else {
		info.SharePath = firstNonEmpty(info.SharePath, info.VHDXPath, sharePath)
		if existingVHDXPath == "" {
			return nil, status.Errorf(codes.FailedPrecondition, "SMB share %q exists but VHDX %q was not found", name, joinWindowsPath(vhdxParentDir, name+".vhdx"))
		}
		if err := cs.Driver.backend.MountFileShareVirtualDisk(ctx, existingVHDXPath, info.SharePath); err != nil {
			return nil, status.Errorf(codes.Internal, "MountFileShareVirtualDisk(existing): %v", err)
		}
		info.VHDXPath = existingVHDXPath
		info.SizeBytes = existingSize
		info.CapacityBytes = maxInt64(info.CapacityBytes, existingSize)
	}
	if smbServer != "" {
		info.SmbServer = smbServer
	}
	capacityBytes := maxInt64(info.CapacityBytes, size)
	vid := EncodeVolumeID(fileShareVolumeID(name, ProtocolSMB, fileShareBackendVHDX, info, capacityBytes))
	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      vid,
			CapacityBytes: capacityBytes,
			VolumeContext: volumeContextForVolumeID(&VolumeID{
				Protocol:     ProtocolSMB,
				SmbServer:    info.SmbServer,
				SmbShareName: info.SmbShareName,
			}),
			ContentSource: req.GetVolumeContentSource(),
		},
	}, nil
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (cs *ControllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "volume_id is required")
	}
	decoded, id, err := decodeAnyVolumeID(req.GetVolumeId())
	if err != nil {
		// idempotent delete
		klog.Warningf("DeleteVolume: decode error: %v", err)
		return &csi.DeleteVolumeResponse{}, nil
	}
	switch decoded.Protocol {
	case ProtocolNFS:
		sharePath := firstNonEmpty(decoded.SharePath, decoded.VHDXPath)
		if err := cs.Driver.backend.DeleteNfsShare(ctx, decoded.Name, sharePath); err != nil {
			klog.Warningf("DeleteNfsShare: %v", err)
		}
		if decoded.ShareBackend == fileShareBackendVHDX {
			if decoded.VHDXPath != "" {
				if err := cs.Driver.backend.UnmountFileShareVirtualDisk(ctx, decoded.VHDXPath, sharePath); err != nil {
					klog.Warningf("UnmountFileShareVirtualDisk: %v", err)
				}
				if err := cs.Driver.backend.DeleteVirtualDisk(ctx, decoded.VHDXPath); err != nil {
					klog.Warningf("DeleteVirtualDisk: %v", err)
				}
			}
		}
		return &csi.DeleteVolumeResponse{}, nil
	case ProtocolSMB:
		sharePath := firstNonEmpty(decoded.SharePath, decoded.VHDXPath)
		if err := cs.Driver.backend.DeleteSmbShare(ctx, decoded.Name, sharePath); err != nil {
			klog.Warningf("DeleteSmbShare: %v", err)
		}
		if decoded.ShareBackend == fileShareBackendVHDX {
			if decoded.VHDXPath != "" {
				if err := cs.Driver.backend.UnmountFileShareVirtualDisk(ctx, decoded.VHDXPath, sharePath); err != nil {
					klog.Warningf("UnmountFileShareVirtualDisk: %v", err)
				}
				if err := cs.Driver.backend.DeleteVirtualDisk(ctx, decoded.VHDXPath); err != nil {
					klog.Warningf("DeleteVirtualDisk: %v", err)
				}
			}
		}
		return &csi.DeleteVolumeResponse{}, nil
	}
	// best-effort unmap + delete
	if targetName := targetNameFromVolID(id); targetName != "" && id.VHDXPath != "" {
		if err := cs.Driver.backend.UnmapDiskFromTarget(ctx, targetName, id.VHDXPath); err != nil {
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
	if req.GetVolumeId() == "" && req.GetNodeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "volume_id and node_id are required")
	}
	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "volume_id are required")
	}
	if req.GetNodeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "node_id are required")
	}
	decoded, id, err := decodeAnyVolumeID(req.GetVolumeId())
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "invalid volume_id: %v", err)
	}
	if !cs.supportsAccessModeForProtocol(req.GetVolumeCapability().GetAccessMode().GetMode(), decoded.Protocol) {
		return nil, status.Error(codes.FailedPrecondition, "access mode is not supported by this driver")
	}
	if decoded.Protocol == ProtocolNFS || decoded.Protocol == ProtocolSMB {
		return &csi.ControllerPublishVolumeResponse{
			PublishContext: volumeContextForVolumeID(decoded),
		}, nil
	}
	initiatorIQN := req.GetNodeId()
	if !strings.HasPrefix(initiatorIQN, "iqn.") {
		return nil, status.Errorf(codes.InvalidArgument, "node_id must be an initiator IQN, got %q", initiatorIQN)
	}
	targetName := targetNameFromVolID(id)
	if targetName == "" || id.TargetIQN == "" {
		return nil, status.Error(codes.InvalidArgument, "volume_id is missing target name or target IQN")
	}
	if err := cs.configureTargetChapFromSecrets(ctx, targetName, req.GetSecrets()); err != nil {
		return nil, err
	}
	if err := cs.Driver.backend.AllowInitiator(ctx, targetName, initiatorIQN); err != nil {
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
	if req.GetVolumeId() == "" && req.GetNodeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "volume_id and node_id are required")
	}
	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "volume_id are required")
	}
	if req.GetNodeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "node_id are required")
	}
	decoded, id, err := decodeAnyVolumeID(req.GetVolumeId())
	if err != nil {
		return &csi.ControllerUnpublishVolumeResponse{}, nil
	}
	if decoded.Protocol == ProtocolNFS || decoded.Protocol == ProtocolSMB {
		return &csi.ControllerUnpublishVolumeResponse{}, nil
	}
	if targetName := targetNameFromVolID(id); targetName != "" {
		if err := cs.Driver.backend.DenyInitiator(ctx, targetName, req.GetNodeId()); err != nil {
			klog.Warningf("DenyInitiator: %v", err)
		}
	} else {
		klog.Warningf("DenyInitiator: volume_id is missing target name")
	}
	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

func (cs *ControllerServer) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	if req.GetVolumeId() == "" || len(req.GetVolumeCapabilities()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "volume_id and volume_capabilities are required")
	}
	decoded, _, err := decodeAnyVolumeID(req.GetVolumeId())
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "invalid volume_id: %v", err)
	}
	for _, vc := range req.GetVolumeCapabilities() {
		if !cs.supportsAccessModeForProtocol(vc.GetAccessMode().GetMode(), decoded.Protocol) {
			return &csi.ValidateVolumeCapabilitiesResponse{
				Message: "access mode is not supported by this driver",
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
	protocol, err := cs.protocolFromParams(params)
	if err != nil {
		return nil, err
	}
	key := "vhdxParentPath"
	required := false
	if protocol == ProtocolNFS || protocol == ProtocolSMB {
		key = "shareParentPath"
		required = true
		shareBackend, err := cs.fileShareBackendFromParams(params)
		if err != nil {
			return nil, err
		}
		if shareBackend == fileShareBackendVHDX {
			key = "vhdxParentPath"
			required = false
		}
	}
	parentDir, ok := getStringParam(params, key)
	if !ok && required {
		return nil, status.Errorf(codes.InvalidArgument, "parameter %s is required", key)
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
	decoded, vid, err := decodeAnyVolumeID(req.GetSourceVolumeId())
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "invalid source_volume_id: %v", err)
	}
	if (decoded.Protocol == ProtocolNFS || decoded.Protocol == ProtocolSMB) && decoded.ShareBackend != fileShareBackendVHDX {
		return nil, status.Error(codes.FailedPrecondition, "directory-backed NFS/SMB volumes do not support snapshots")
	}
	sourcePath := decoded.VHDXPath
	if sourcePath == "" {
		sourcePath = vid.VHDXPath
	}
	if sourcePath == "" {
		return nil, status.Errorf(codes.InvalidArgument, "source volume %q does not include a backend path", req.GetSourceVolumeId())
	}
	desc := strings.TrimSpace(req.GetName())
	snap, err := cs.Driver.backend.CreateSnapshot(ctx, sourcePath, desc)
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
	if cs.Driver != nil && cs.Driver.protocol != "" && cs.Driver.protocol != ProtocolISCSI && cs.Driver.fileShareBackend != fileShareBackendVHDX {
		return nil, status.Error(codes.Unimplemented, "ListSnapshots is only supported for iSCSI and VHDX-backed file-share snapshots")
	}

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
		decoded, vid, err := decodeAnyVolumeID(req.GetSourceVolumeId())
		if err != nil {
			return nil, status.Errorf(codes.NotFound, "invalid source_volume_id: %v", err)
		}
		if (decoded.Protocol == ProtocolNFS || decoded.Protocol == ProtocolSMB) && decoded.ShareBackend != fileShareBackendVHDX {
			return nil, status.Error(codes.FailedPrecondition, "directory-backed NFS/SMB volumes do not support snapshots")
		}
		sourcePath := decoded.VHDXPath
		if sourcePath == "" {
			sourcePath = vid.VHDXPath
		}
		if sourcePath == "" {
			return nil, status.Errorf(codes.InvalidArgument, "source volume %q does not include a backend path", req.GetSourceVolumeId())
		}
		var e error
		snaps, e = cs.Driver.backend.ListSnapshots(ctx, sourcePath)
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
	decoded, id, err := decodeAnyVolumeID(req.GetVolumeId())
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "invalid volume_id: %v", err)
	}
	want := req.GetCapacityRange().GetRequiredBytes()
	if want <= 0 {
		return nil, status.Error(codes.InvalidArgument, "required_bytes must be > 0")
	}
	if decoded.Protocol == ProtocolNFS || decoded.Protocol == ProtocolSMB {
		if decoded.VHDXPath == "" {
			return nil, status.Errorf(codes.InvalidArgument, "volume %q does not include a backing path", req.GetVolumeId())
		}
		if decoded.ShareBackend == fileShareBackendVHDX {
			actual, err := cs.Driver.backend.ResizeVirtualDisk(ctx, decoded.VHDXPath, want)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "ResizeVirtualDisk: %v", err)
			}
			if err := cs.Driver.backend.MountFileShareVirtualDisk(ctx, decoded.VHDXPath, decoded.SharePath); err != nil {
				return nil, status.Errorf(codes.Internal, "MountFileShareVirtualDisk: %v", err)
			}
			return &csi.ControllerExpandVolumeResponse{
				CapacityBytes:         actual,
				NodeExpansionRequired: false,
			}, nil
		}
		actual, err := cs.Driver.backend.ResizeFileShare(ctx, decoded.VHDXPath, want)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "ResizeFileShare: %v", err)
		}
		return &csi.ControllerExpandVolumeResponse{
			CapacityBytes:         actual,
			NodeExpansionRequired: false,
		}, nil
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
	decoded, id, err := decodeAnyVolumeID(req.GetVolumeId())
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "invalid volume_id: %v", err)
	}
	if decoded.Protocol == ProtocolNFS || decoded.Protocol == ProtocolSMB {
		return &csi.ControllerGetVolumeResponse{
			Volume: &csi.Volume{
				VolumeId:      req.GetVolumeId(),
				CapacityBytes: decoded.CapacityBytes,
				VolumeContext: volumeContextForVolumeID(decoded),
			},
			Status: &csi.ControllerGetVolumeResponse_VolumeStatus{
				VolumeCondition: &csi.VolumeCondition{Abnormal: false, Message: "OK"},
			},
		}, nil
	}
	vi, err := cs.Driver.backend.GetVolumeInfo(ctx, id.VHDXPath)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "GetVolumeInfo: %v", err)
	}
	if vi.VHDXPath == "" {
		return nil, status.Errorf(codes.NotFound, "volume not found")
	}
	published, _ := cs.Driver.backend.GetTargetInitiators(ctx, targetNameFromVolID(id))

	lunStr := volumeInfoLUNString(vi.LUN)
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
