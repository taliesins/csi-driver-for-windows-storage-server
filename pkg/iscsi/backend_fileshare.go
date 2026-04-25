package iscsi

import (
	"context"
	"fmt"
	"strings"
)

func psStringArray(values []string) string {
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			cleaned = append(cleaned, "'"+escapePS(value)+"'")
		}
	}
	if len(cleaned) == 0 {
		return "@()"
	}
	return "@(" + strings.Join(cleaned, ",") + ")"
}

func psBoolLiteral(value bool) string {
	if value {
		return "$true"
	}
	return "$false"
}

func firstNfsShareOptions(opts []NfsShareOptions) NfsShareOptions {
	if len(opts) == 0 {
		return NfsShareOptions{}
	}
	return opts[0]
}

func firstSmbShareOptions(opts []SmbShareOptions) SmbShareOptions {
	if len(opts) == 0 {
		return SmbShareOptions{}
	}
	return opts[0]
}

func nfsShareParamsScript(opt NfsShareOptions) string {
	lines := []string{}
	if len(opt.Authentication) > 0 {
		lines = append(lines, "$params.Authentication = "+psStringArray(opt.Authentication))
	}
	if opt.AnonymousUID != nil {
		lines = append(lines, fmt.Sprintf("$params.AnonymousUid = %d", *opt.AnonymousUID))
	}
	if opt.AnonymousGID != nil {
		lines = append(lines, fmt.Sprintf("$params.AnonymousGid = %d", *opt.AnonymousGID))
	}
	if opt.LanguageEncoding != "" {
		lines = append(lines, fmt.Sprintf("$params.LanguageEncoding = '%s'", escapePS(opt.LanguageEncoding)))
	}
	if opt.EnableAnonymousAccess != nil {
		lines = append(lines, "$params.EnableAnonymousAccess = "+psBoolLiteral(*opt.EnableAnonymousAccess))
	}
	if opt.EnableUnmappedAccess != nil {
		lines = append(lines, "$params.EnableUnmappedAccess = "+psBoolLiteral(*opt.EnableUnmappedAccess))
	}
	return strings.Join(lines, "\n  ")
}

func smbShareParamsScript(opt SmbShareOptions) string {
	lines := []string{}
	if len(opt.NoAccess) > 0 {
		lines = append(lines, "$params.NoAccess = "+psStringArray(opt.NoAccess))
	}
	if opt.Description != "" {
		lines = append(lines, fmt.Sprintf("$params.Description = '%s'", escapePS(opt.Description)))
	}
	if opt.EncryptData != nil {
		lines = append(lines, "$params.EncryptData = "+psBoolLiteral(*opt.EncryptData))
	}
	if opt.CompressData != nil {
		lines = append(lines, "if ((Get-Command New-SmbShare).Parameters.ContainsKey('CompressData')) { $params.CompressData = "+psBoolLiteral(*opt.CompressData)+" }")
	}
	if opt.ContinuouslyAvailable != nil {
		lines = append(lines, "$params.ContinuouslyAvailable = "+psBoolLiteral(*opt.ContinuouslyAvailable))
	}
	if opt.CachingMode != "" {
		lines = append(lines, fmt.Sprintf("$params.CachingMode = '%s'", escapePS(opt.CachingMode)))
	}
	if opt.FolderEnumerationMode != "" {
		lines = append(lines, fmt.Sprintf("$params.FolderEnumerationMode = '%s'", escapePS(opt.FolderEnumerationMode)))
	}
	if opt.ConcurrentUserLimit != 0 {
		lines = append(lines, fmt.Sprintf("$params.ConcurrentUserLimit = %d", opt.ConcurrentUserLimit))
	}
	return strings.Join(lines, "\n  ")
}

const fileShareCopyPS = `function Copy-CsiDirectoryMirror($s,$d){New-Item -ItemType Directory -Path $d -Force|Out-Null;robocopy $s $d /MIR /R:1 /W:1 /NFL /NDL /NJH /NJS /NP|Out-Null;$c=$LASTEXITCODE;$global:LASTEXITCODE=0;if($c -gt 7){throw "robocopy failed with exit code $c"}}`

func splitCSVParam(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func (b *WinRMBackend) CreateNfsShare(ctx context.Context, name, parentDir string, sizeBytes int64, clients []string, opts ...NfsShareOptions) (VolumeInfo, error) {
	if len(clients) == 0 {
		clients = []string{"*"}
	}
	opt := firstNfsShareOptions(opts)
	permission := strings.TrimSpace(opt.Permission)
	if permission == "" {
		permission = "readwrite"
	}
	clientType := strings.TrimSpace(opt.ClientType)
	if clientType == "" {
		clientType = "host"
	}
	allowRootAccess := true
	if opt.AllowRootAccess != nil {
		allowRootAccess = *opt.AllowRootAccess
	}
	s := fmt.Sprintf(`
Import-Module NFS -ErrorAction Stop
$name = '%s'
$path = Join-Path -Path '%s' -ChildPath $name
New-Item -ItemType Directory -Path $path -Force | Out-Null
$share = Get-NfsShare -Name $name -ErrorAction SilentlyContinue
if (-not $share) {
  $params = @{ Name=$name; Path=$path; Permission='%s'; AllowRootAccess=%s }
  %s
  New-NfsShare @params | Out-Null
}
$clients = %s
foreach ($client in $clients) {
  Grant-NfsSharePermission -Name $name -ClientName $client -ClientType '%s' -Permission '%s' -AllowRootAccess %s -ErrorAction SilentlyContinue | Out-Null
}
@{ name=$name; protocol='nfs'; nfsServer=$env:COMPUTERNAME; nfsExportPath=('/' + $name); vhdxPath=$path; sizeBytes=[int64]0; capacityBytes=[int64]%d }
`, escapePS(name), escapePS(parentDir), escapePS(permission), psBoolLiteral(allowRootAccess), nfsShareParamsScript(opt), psStringArray(clients), escapePS(clientType), escapePS(permission), psBoolLiteral(allowRootAccess), sizeBytes)
	var out struct {
		Name          string `json:"name"`
		Protocol      string `json:"protocol"`
		NfsServer     string `json:"nfsServer"`
		NfsExportPath string `json:"nfsExportPath"`
		VHDXPath      string `json:"vhdxPath"`
		SizeBytes     int64  `json:"sizeBytes"`
		CapacityBytes int64  `json:"capacityBytes"`
	}
	if err := b.runPS(ctx, s, &out); err != nil {
		return VolumeInfo{}, err
	}
	if sizeBytes > 0 {
		actual, err := b.ResizeFileShare(ctx, out.VHDXPath, sizeBytes)
		if err != nil {
			return VolumeInfo{}, err
		}
		out.CapacityBytes = actual
	}
	return VolumeInfo{
		VolumeName:    out.Name,
		Protocol:      ProtocolNFS,
		NfsServer:     out.NfsServer,
		NfsExportPath: out.NfsExportPath,
		VHDXPath:      out.VHDXPath,
		SizeBytes:     out.SizeBytes,
		CapacityBytes: out.CapacityBytes,
	}, nil
}

func (b *WinRMBackend) GetNfsShare(ctx context.Context, name, parentDir string) (bool, VolumeInfo, error) {
	s := fmt.Sprintf(`
Import-Module NFS -ErrorAction Stop
$name = '%s'
$path = Join-Path -Path '%s' -ChildPath $name
$share = Get-NfsShare -Name $name -ErrorAction SilentlyContinue
if (-not $share) {
  @{ exists=$false }
} else {
  $cap = [int64]0
  $metaPath = Join-Path -Path $share.Path -ChildPath '.csi-volume.json'
  if (Test-Path -LiteralPath $metaPath) { $cap = [int64]((Get-Content -LiteralPath $metaPath -Raw | ConvertFrom-Json).capacityBytes) }
  @{ exists=$true; name=$name; protocol='nfs'; nfsServer=$env:COMPUTERNAME; nfsExportPath=('/' + $name); vhdxPath=$share.Path; sizeBytes=[int64]0; capacityBytes=$cap }
}
`, escapePS(name), escapePS(parentDir))
	var out struct {
		Exists        bool   `json:"exists"`
		Name          string `json:"name"`
		NfsServer     string `json:"nfsServer"`
		NfsExportPath string `json:"nfsExportPath"`
		VHDXPath      string `json:"vhdxPath"`
		SizeBytes     int64  `json:"sizeBytes"`
		CapacityBytes int64  `json:"capacityBytes"`
	}
	if err := b.runPS(ctx, s, &out); err != nil {
		return false, VolumeInfo{}, err
	}
	if !out.Exists {
		return false, VolumeInfo{}, nil
	}
	return true, VolumeInfo{VolumeName: out.Name, Protocol: ProtocolNFS, NfsServer: out.NfsServer, NfsExportPath: out.NfsExportPath, VHDXPath: out.VHDXPath, SizeBytes: out.SizeBytes, CapacityBytes: out.CapacityBytes}, nil
}

func (b *WinRMBackend) DeleteNfsShare(ctx context.Context, name, path string) error {
	s := fmt.Sprintf(`
Import-Module NFS -ErrorAction Stop
$name = '%s'
$path = '%s'
$share = Get-NfsShare -Name $name -ErrorAction SilentlyContinue
if ($share -and -not $path) {
  $path = $share.Path
}
if ($share) {
  Remove-NfsShare -Name $name -Confirm:$false -ErrorAction SilentlyContinue
}
if ($path -and (Test-Path -LiteralPath $path)) {
  if (Get-Command Remove-FsrmQuota -ErrorAction SilentlyContinue) { Remove-FsrmQuota -Path $path -Confirm:$false -ErrorAction SilentlyContinue | Out-Null }
  Remove-Item -LiteralPath $path -Recurse -Force -ErrorAction SilentlyContinue
}
@{ ok=$true }
`, escapePS(name), escapePS(path))
	var out map[string]any
	return b.runPS(ctx, s, &out)
}

func (b *WinRMBackend) CreateSmbShare(ctx context.Context, name, parentDir string, sizeBytes int64, fullAccess, changeAccess, readAccess []string, opts ...SmbShareOptions) (VolumeInfo, error) {
	if len(fullAccess) == 0 && len(changeAccess) == 0 && len(readAccess) == 0 {
		fullAccess = []string{"Administrators"}
		changeAccess = []string{"Everyone"}
	}
	opt := firstSmbShareOptions(opts)
	s := fmt.Sprintf(`
Import-Module SmbShare -ErrorAction Stop
$name = '%s'
$path = Join-Path -Path '%s' -ChildPath $name
New-Item -ItemType Directory -Path $path -Force | Out-Null
$share = Get-SmbShare -Name $name -ErrorAction SilentlyContinue
if (-not $share) {
  $params = @{ Name=$name; Path=$path }
  $full = %s
  $change = %s
  $read = %s
  if ($full.Count -gt 0) { $params.FullAccess = $full }
  if ($change.Count -gt 0) { $params.ChangeAccess = $change }
  if ($read.Count -gt 0) { $params.ReadAccess = $read }
  %s
  New-SmbShare @params | Out-Null
}
@{ name=$name; protocol='smb'; smbServer=$env:COMPUTERNAME; smbShareName=$name; vhdxPath=$path; sizeBytes=[int64]0; capacityBytes=[int64]%d }
`, escapePS(name), escapePS(parentDir), psStringArray(fullAccess), psStringArray(changeAccess), psStringArray(readAccess), smbShareParamsScript(opt), sizeBytes)
	var out struct {
		Name          string `json:"name"`
		SmbServer     string `json:"smbServer"`
		SmbShareName  string `json:"smbShareName"`
		VHDXPath      string `json:"vhdxPath"`
		SizeBytes     int64  `json:"sizeBytes"`
		CapacityBytes int64  `json:"capacityBytes"`
	}
	if err := b.runPS(ctx, s, &out); err != nil {
		return VolumeInfo{}, err
	}
	if sizeBytes > 0 {
		actual, err := b.ResizeFileShare(ctx, out.VHDXPath, sizeBytes)
		if err != nil {
			return VolumeInfo{}, err
		}
		out.CapacityBytes = actual
	}
	return VolumeInfo{VolumeName: out.Name, Protocol: ProtocolSMB, SmbServer: out.SmbServer, SmbShareName: out.SmbShareName, VHDXPath: out.VHDXPath, SizeBytes: out.SizeBytes, CapacityBytes: out.CapacityBytes}, nil
}

func (b *WinRMBackend) GetSmbShare(ctx context.Context, name, parentDir string) (bool, VolumeInfo, error) {
	s := fmt.Sprintf(`
Import-Module SmbShare -ErrorAction Stop
$name = '%s'
$share = Get-SmbShare -Name $name -ErrorAction SilentlyContinue
if (-not $share) {
  @{ exists=$false }
} else {
  $cap = [int64]0
  $metaPath = Join-Path -Path $share.Path -ChildPath '.csi-volume.json'
  if (Test-Path -LiteralPath $metaPath) { $cap = [int64]((Get-Content -LiteralPath $metaPath -Raw | ConvertFrom-Json).capacityBytes) }
  @{ exists=$true; name=$name; protocol='smb'; smbServer=$env:COMPUTERNAME; smbShareName=$name; vhdxPath=$share.Path; sizeBytes=[int64]0; capacityBytes=$cap }
}
`, escapePS(name))
	var out struct {
		Exists        bool   `json:"exists"`
		Name          string `json:"name"`
		SmbServer     string `json:"smbServer"`
		SmbShareName  string `json:"smbShareName"`
		VHDXPath      string `json:"vhdxPath"`
		SizeBytes     int64  `json:"sizeBytes"`
		CapacityBytes int64  `json:"capacityBytes"`
	}
	if err := b.runPS(ctx, s, &out); err != nil {
		return false, VolumeInfo{}, err
	}
	if !out.Exists {
		return false, VolumeInfo{}, nil
	}
	return true, VolumeInfo{VolumeName: out.Name, Protocol: ProtocolSMB, SmbServer: out.SmbServer, SmbShareName: out.SmbShareName, VHDXPath: out.VHDXPath, SizeBytes: out.SizeBytes, CapacityBytes: out.CapacityBytes}, nil
}

func (b *WinRMBackend) DeleteSmbShare(ctx context.Context, name, path string) error {
	s := fmt.Sprintf(`
Import-Module SmbShare -ErrorAction Stop
$name = '%s'
$path = '%s'
$share = Get-SmbShare -Name $name -ErrorAction SilentlyContinue
if ($share -and -not $path) {
  $path = $share.Path
}
if ($share) {
  Remove-SmbShare -Name $name -Force -ErrorAction SilentlyContinue
}
if ($path -and (Test-Path -LiteralPath $path)) {
  if (Get-Command Remove-FsrmQuota -ErrorAction SilentlyContinue) { Remove-FsrmQuota -Path $path -Confirm:$false -ErrorAction SilentlyContinue | Out-Null }
  Remove-Item -LiteralPath $path -Recurse -Force -ErrorAction SilentlyContinue
}
@{ ok=$true }
`, escapePS(name), escapePS(path))
	var out map[string]any
	return b.runPS(ctx, s, &out)
}

func (b *WinRMBackend) ResizeFileShare(ctx context.Context, path string, newSizeBytes int64) (int64, error) {
	s := fmt.Sprintf(`
$path = '%s'
$bytes = [int64]%d
New-Item -ItemType Directory -Path $path -Force | Out-Null
$quotaEnforced = $false
if (Get-Command New-FsrmQuota -ErrorAction SilentlyContinue) {
  Import-Module FileServerResourceManager -ErrorAction SilentlyContinue
  try {
    $quota = Get-FsrmQuota -Path $path -ErrorAction SilentlyContinue
    if ($quota) { Set-FsrmQuota -Path $path -Size $bytes -ErrorAction Stop | Out-Null } else { New-FsrmQuota -Path $path -Size $bytes -ErrorAction Stop | Out-Null }
    $quotaEnforced = $true
  } catch { $quotaEnforced = $false }
}
@{ capacityBytes=$bytes; quotaEnforced=[bool]$quotaEnforced; updatedAt=(Get-Date).ToUniversalTime().ToString('o') } | ConvertTo-Json -Compress | Set-Content -LiteralPath (Join-Path -Path $path -ChildPath '.csi-volume.json') -Encoding UTF8
@{ capacityBytes=$bytes }
`, escapePS(path), newSizeBytes)
	var out struct {
		CapacityBytes int64 `json:"capacityBytes"`
	}
	if err := b.runPS(ctx, s, &out); err != nil {
		return 0, err
	}
	return out.CapacityBytes, nil
}

func (b *WinRMBackend) RestoreSnapshotAsFileShare(ctx context.Context, snapshotID, destinationPath string) error {
	s := fmt.Sprintf(`
%s
if (-not (Test-Path -LiteralPath '%s' -PathType Container)) {
  throw "file-share snapshot path not found: %s"
}
Copy-CsiDirectoryMirror '%s' '%s'
@{ ok=$true }
`, fileShareCopyPS, escapePS(snapshotID), escapePS(snapshotID), escapePS(snapshotID), escapePS(destinationPath))
	var out map[string]any
	return b.runPS(ctx, s, &out)
}
