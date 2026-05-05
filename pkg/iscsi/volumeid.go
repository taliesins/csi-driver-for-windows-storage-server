package iscsi

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Protocol string

const (
	ProtocolISCSI Protocol = "iscsi"
	ProtocolNFS   Protocol = "nfs"
	ProtocolSMB   Protocol = "smb"
)

type VolumeID struct {
	Name                   string
	Protocol               Protocol
	ShareBackend           string
	SharePath              string
	TargetPortal           string
	TargetName             string
	TargetIQN              string
	LUN                    int
	VHDXPath               string
	NfsServer              string
	NfsExportPath          string
	NfsAuthentication      string
	NfsMountAuthentication string
	SmbServer              string
	SmbShareName           string
	CapacityBytes          int64

	nameSpecified bool
	lunSpecified  bool
}

func EncodeVolumeID(v *VolumeID) string {
	if v == nil {
		return ""
	}

	q := url.Values{}
	q.Set("name", v.Name)
	if v.CapacityBytes != 0 {
		q.Set("capacityBytes", strconv.FormatInt(v.CapacityBytes, 10))
	}
	if v.VHDXPath != "" {
		q.Set("vhdxPath", v.VHDXPath)
	}
	if v.ShareBackend != "" {
		q.Set("shareBackend", v.ShareBackend)
	}
	if v.SharePath != "" {
		q.Set("sharePath", v.SharePath)
	}
	if v.TargetName != "" {
		q.Set("targetName", v.TargetName)
	}

	switch v.Protocol {
	case ProtocolISCSI:
		host := strings.TrimSpace(v.TargetPortal)
		segments := []string{strings.TrimSpace(v.TargetIQN)}
		if shouldEncodeLUN(v) {
			segments = append(segments, "lun", strconv.Itoa(v.LUN))
		}
		return buildURI(ProtocolISCSI, host, segments, q)
	case ProtocolNFS:
		host := strings.TrimSpace(v.TargetPortal)
		if host == "" {
			host = strings.TrimSpace(v.NfsServer)
		} else {
			q.Set("targetPortal", v.TargetPortal)
		}
		if v.NfsServer != "" {
			q.Set("nfsServer", v.NfsServer)
		}
		if v.NfsAuthentication != "" {
			q.Set("nfsAuthentication", v.NfsAuthentication)
		}
		if v.NfsMountAuthentication != "" {
			q.Set("nfsMountAuthentication", v.NfsMountAuthentication)
		}
		return buildURI(ProtocolNFS, host, splitPath(v.NfsExportPath), q)
	case ProtocolSMB:
		if v.SmbServer != "" {
			q.Set("smbServer", v.SmbServer)
		}
		return buildURI(ProtocolSMB, strings.TrimSpace(v.SmbServer), []string{strings.TrimSpace(v.SmbShareName)}, q)
	default:
		return buildURI(v.Protocol, "", nil, q)
	}
}

func DecodeVolumeID(id string) (*VolumeID, error) {
	if strings.TrimSpace(id) == "" {
		return nil, fmt.Errorf("empty volume ID")
	}
	if strings.ContainsRune(id, '\x00') {
		return nil, fmt.Errorf("invalid volume ID")
	}

	proto, err := ParseProtocolFromVolumeID(id)
	if err != nil {
		return nil, err
	}
	if proto == "" {
		proto = Protocol(id[:strings.Index(id, "://")])
	}

	u, err := url.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("invalid volume ID: %w", err)
	}
	out := &VolumeID{Protocol: proto}

	q := u.Query()
	if _, ok := q["name"]; ok {
		out.Name = q.Get("name")
		out.nameSpecified = true
	}
	if capRaw := strings.TrimSpace(q.Get("capacityBytes")); capRaw != "" {
		capacity, err := strconv.ParseInt(capRaw, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid capacityBytes: %w", err)
		}
		if capacity < 0 {
			return nil, fmt.Errorf("capacityBytes must be non-negative")
		}
		out.CapacityBytes = capacity
	}
	out.VHDXPath = q.Get("vhdxPath")
	out.ShareBackend = q.Get("shareBackend")
	out.SharePath = q.Get("sharePath")
	out.TargetName = q.Get("targetName")

	switch proto {
	case ProtocolISCSI:
		out.TargetPortal = defaultPort(u.Host, "3260")
		parts := pathSegments(u.Path)
		if len(parts) > 0 {
			out.TargetIQN = parts[0]
		}
		for i := 1; i+1 < len(parts); i++ {
			if parts[i] == "lun" {
				lun, err := strconv.Atoi(parts[i+1])
				if err != nil {
					return nil, fmt.Errorf("invalid LUN: %w", err)
				}
				out.LUN = lun
				out.lunSpecified = true
				break
			}
		}
	case ProtocolNFS:
		out.TargetPortal = q.Get("targetPortal")
		out.NfsServer = q.Get("nfsServer")
		if out.NfsServer == "" {
			out.NfsServer = hostOnly(u.Host)
		}
		out.NfsAuthentication = q.Get("nfsAuthentication")
		out.NfsMountAuthentication = q.Get("nfsMountAuthentication")
		out.NfsExportPath = u.Path
	case ProtocolSMB:
		out.SmbServer = q.Get("smbServer")
		if out.SmbServer == "" {
			out.SmbServer = hostOnly(u.Host)
		}
		parts := pathSegments(u.Path)
		if len(parts) > 0 {
			out.SmbShareName = parts[0]
		}
	}

	if err := out.Validate(); err != nil {
		return nil, err
	}
	return out, nil
}

func ValidateVolumeID(id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("empty volume ID")
	}
	if strings.ContainsRune(id, '\x00') {
		return fmt.Errorf("invalid volume ID")
	}
	_, err := DecodeVolumeID(id)
	return err
}

func ParseProtocolFromVolumeID(id string) (Protocol, error) {
	if strings.TrimSpace(id) == "" {
		return "", fmt.Errorf("missing scheme")
	}
	if strings.ContainsRune(id, '\x00') {
		return "", fmt.Errorf("invalid volume ID")
	}

	sep := strings.Index(id, "://")
	if sep < 0 {
		return "", fmt.Errorf("invalid scheme")
	}
	if sep == 0 {
		return "", fmt.Errorf("missing scheme")
	}
	if sep+3 >= len(id) {
		return "", fmt.Errorf("invalid volume ID")
	}
	if strings.HasPrefix(id[sep+3:], "/") {
		return "", nil
	}

	switch proto := Protocol(id[:sep]); proto {
	case ProtocolISCSI, ProtocolNFS, ProtocolSMB:
		return proto, nil
	default:
		return "", fmt.Errorf("unknown protocol: %s", proto)
	}
}

func (v *VolumeID) Validate() error {
	if v == nil {
		return fmt.Errorf("empty volume ID")
	}
	if v.nameSpecified && strings.TrimSpace(v.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if v.CapacityBytes < 0 {
		return fmt.Errorf("capacityBytes must be non-negative")
	}

	switch v.Protocol {
	case ProtocolISCSI:
		if strings.TrimSpace(v.TargetPortal) == "" {
			return fmt.Errorf("targetPortal is required")
		}
		if strings.TrimSpace(v.TargetIQN) == "" {
			return fmt.Errorf("targetIQN is required")
		}
		if !v.lunSpecified {
			return fmt.Errorf("LUN is required")
		}
	case ProtocolNFS:
		if strings.TrimSpace(v.NfsServer) == "" {
			return fmt.Errorf("nfsServer is required")
		}
		if strings.TrimSpace(v.NfsExportPath) == "" || strings.TrimSpace(v.NfsExportPath) == "/" {
			return fmt.Errorf("nfsExportPath is required")
		}
	case ProtocolSMB:
		if strings.TrimSpace(v.SmbServer) == "" {
			return fmt.Errorf("smbServer is required")
		}
		if strings.TrimSpace(v.SmbShareName) == "" {
			return fmt.Errorf("smbShareName is required")
		}
	default:
		return fmt.Errorf("unknown protocol: %s", v.Protocol)
	}
	return nil
}

type SelectableBackend interface {
	GetProtocol() Protocol
}

type IscsiBackend struct{}
type NfsBackend struct{}
type SmbBackend struct{}

func (*IscsiBackend) GetProtocol() Protocol { return ProtocolISCSI }
func (*NfsBackend) GetProtocol() Protocol   { return ProtocolNFS }
func (*SmbBackend) GetProtocol() Protocol   { return ProtocolSMB }

type BackendSelector struct {
	iscsi SelectableBackend
	nfs   SelectableBackend
	smb   SelectableBackend
}

func NewBackendSelector(iscsi, nfs, smb SelectableBackend) *BackendSelector {
	return &BackendSelector{iscsi: iscsi, nfs: nfs, smb: smb}
}

func (s *BackendSelector) Select(protocol Protocol) (SelectableBackend, error) {
	var backend SelectableBackend
	switch protocol {
	case ProtocolISCSI:
		backend = s.iscsi
	case ProtocolNFS:
		backend = s.nfs
	case ProtocolSMB:
		backend = s.smb
	default:
		return nil, status.Errorf(codes.InvalidArgument, "unknown protocol: %s", protocol)
	}
	if backend == nil {
		return nil, status.Errorf(codes.NotFound, "%s backend not configured", protocol)
	}
	return backend, nil
}

func (s *BackendSelector) GetProtocol(volumeID string) (Protocol, error) {
	return ParseProtocolFromVolumeID(volumeID)
}

type NfsPermissions string

const (
	NfsPermReadOnly   NfsPermissions = "ro"
	NfsPermReadWrite  NfsPermissions = "rw"
	NfsPermRootAccess NfsPermissions = "no_root_squash"
)

type SmbPermissions string

const (
	SmbPermFullControl SmbPermissions = "FullControl"
	SmbPermReadWrite   SmbPermissions = "ReadWrite"
	SmbPermReadOnly    SmbPermissions = "ReadOnly"
)

type NfsShareInfo struct {
	Name        string
	Path        string
	Permissions NfsPermissions
	Clients     []string
}

func (i NfsShareInfo) Validate() error {
	if strings.TrimSpace(i.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if strings.TrimSpace(i.Path) == "" {
		return fmt.Errorf("path is required")
	}
	return nil
}

type SmbAclEntry struct {
	User        string
	Permissions SmbPermissions
}

type SmbShareInfo struct {
	Name   string
	Path   string
	Server string
	ACL    []SmbAclEntry
}

func (i SmbShareInfo) Validate() error {
	if strings.TrimSpace(i.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if strings.TrimSpace(i.Path) == "" {
		return fmt.Errorf("path is required")
	}
	if strings.TrimSpace(i.Server) == "" {
		return fmt.Errorf("server is required")
	}
	return nil
}

type NfsExportInfo struct {
	Server     string
	ExportPath string
	Clients    []string
}

func (i NfsExportInfo) Validate() error {
	if strings.TrimSpace(i.Server) == "" {
		return fmt.Errorf("server is required")
	}
	if strings.TrimSpace(i.ExportPath) == "" {
		return fmt.Errorf("exportPath is required")
	}
	return nil
}

type SmbMappingInfo struct {
	Server string
	Share  string
	User   string
}

func (i SmbMappingInfo) Validate() error {
	if strings.TrimSpace(i.Server) == "" {
		return fmt.Errorf("server is required")
	}
	if strings.TrimSpace(i.Share) == "" {
		return fmt.Errorf("share is required")
	}
	return nil
}

func (s SnapshotInfo) Validate() error {
	if strings.TrimSpace(s.SnapshotID) == "" {
		return fmt.Errorf("snapshotID is required")
	}
	if strings.TrimSpace(s.SourceVolume) == "" {
		return fmt.Errorf("sourceVolume is required")
	}
	return nil
}

func (v VolumeInfo) MarshalJSON() ([]byte, error) {
	type volumeInfo VolumeInfo
	return json.Marshal(volumeInfo(v))
}

func (v *VolumeInfo) UnmarshalJSON(data []byte) error {
	type volumeInfo VolumeInfo
	var out volumeInfo
	if err := json.Unmarshal(data, &out); err != nil {
		return err
	}
	if lun, ok := normalizeLUN(out.LUN); ok {
		out.LUN = lun
	}
	*v = VolumeInfo(out)
	return nil
}

func volumeInfoLUNString(lun any) string {
	v, ok := normalizeLUN(lun)
	if !ok {
		return ""
	}
	return strconv.Itoa(v)
}

func normalizeLUN(lun any) (int, bool) {
	switch v := lun.(type) {
	case nil:
		return 0, false
	case int:
		return v, true
	case int32:
		return int(v), true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	case *int32:
		if v == nil {
			return 0, false
		}
		return int(*v), true
	case *int:
		if v == nil {
			return 0, false
		}
		return *v, true
	default:
		return 0, false
	}
}

func shouldEncodeLUN(v *VolumeID) bool {
	if v.LUN != 0 {
		return true
	}
	return v.VHDXPath != "" || v.CapacityBytes != 0
}

func buildURI(protocol Protocol, host string, segments []string, q url.Values) string {
	escaped := make([]string, 0, len(segments))
	for _, segment := range segments {
		if segment = strings.TrimSpace(segment); segment != "" {
			escaped = append(escaped, url.PathEscape(segment))
		}
	}
	uri := string(protocol) + "://" + host
	if len(escaped) > 0 {
		uri += "/" + strings.Join(escaped, "/")
	}
	if encoded := q.Encode(); encoded != "" {
		uri += "?" + encoded
	}
	return uri
}

func splitPath(path string) []string {
	return pathSegments(path)
}

func pathSegments(path string) []string {
	raw := strings.Split(strings.Trim(path, "/"), "/")
	segments := raw[:0]
	for _, segment := range raw {
		if segment != "" {
			if decoded, err := url.PathUnescape(segment); err == nil {
				segment = decoded
			}
			segments = append(segments, segment)
		}
	}
	return segments
}

func defaultPort(host, port string) string {
	if host == "" {
		return ""
	}
	if strings.Contains(host, ":") {
		return host
	}
	return net.JoinHostPort(host, port)
}

func hostOnly(host string) string {
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	return host
}
