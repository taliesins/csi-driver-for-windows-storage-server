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

const fileShareCopyItemPS = `function Copy-CsiDirectoryTree($s,$d){New-Item -ItemType Directory -Path $d -Force|Out-Null;@(Get-ChildItem -LiteralPath $s -Force -ErrorAction Stop)|ForEach-Object{Copy-Item -LiteralPath $_.FullName -Destination $d -Recurse -Force -ErrorAction Stop}}`

const fileShareVHDXPS = `
function Get-CsiAccessPath([string]$Path) {
  if ([string]::IsNullOrWhiteSpace($Path)) { return '' }
  $full = [System.IO.Path]::GetFullPath($Path)
  if (-not $full.EndsWith('\')) { $full += '\' }
  return $full
}
function Get-CsiPartitionByAccessPath([string]$Path) {
  $accessPath = Get-CsiAccessPath $Path
  if ([string]::IsNullOrWhiteSpace($accessPath)) { return $null }
  @(Get-Partition -ErrorAction SilentlyContinue | Where-Object { @($_.AccessPaths) -contains $accessPath } | Select-Object -First 1)[0]
}
function Test-CsiPartitionAccessPath([string]$Path) {
  $null -ne (Get-CsiPartitionByAccessPath $Path)
}
function Expand-CsiPartitionToDisk([object]$Partition) {
  if (-not $Partition) { return }
  $supported = Get-PartitionSupportedSize -DiskNumber $Partition.DiskNumber -PartitionNumber $Partition.PartitionNumber -ErrorAction SilentlyContinue
  if ($supported -and [int64]$supported.SizeMax -gt [int64]$Partition.Size) {
    Resize-Partition -DiskNumber $Partition.DiskNumber -PartitionNumber $Partition.PartitionNumber -Size $supported.SizeMax -ErrorAction Stop
  }
}
function Mount-CsiFileShareVirtualDisk([string]$VhdxPath, [string]$MountPath) {
  New-Item -ItemType Directory -Path $MountPath -Force | Out-Null
  $image = Get-DiskImage -ImagePath $VhdxPath -ErrorAction SilentlyContinue
  if (-not $image -or -not $image.Attached) {
    Mount-DiskImage -ImagePath $VhdxPath -PassThru -ErrorAction Stop | Out-Null
  }
  $disk = Get-DiskImage -ImagePath $VhdxPath -ErrorAction Stop | Get-Disk -ErrorAction Stop
  if ($disk.IsOffline) { Set-Disk -Number $disk.Number -IsOffline $false -ErrorAction Stop }
  if ($disk.IsReadOnly) { Set-Disk -Number $disk.Number -IsReadOnly $false -ErrorAction Stop }
  if ($disk.PartitionStyle -eq 'RAW') {
    Initialize-Disk -Number $disk.Number -PartitionStyle GPT -ErrorAction Stop
  }
  $partition = @(Get-Partition -DiskNumber $disk.Number -ErrorAction SilentlyContinue | Where-Object { $_.Type -ne 'Reserved' } | Sort-Object PartitionNumber | Select-Object -First 1)[0]
  if (-not $partition) {
    $partition = New-Partition -DiskNumber $disk.Number -UseMaximumSize -ErrorAction Stop
  }
  $volume = $partition | Get-Volume -ErrorAction SilentlyContinue
  if (-not $volume -or [string]::IsNullOrWhiteSpace($volume.FileSystem)) {
    Format-Volume -Partition $partition -FileSystem NTFS -NewFileSystemLabel 'CSIFileShare' -Confirm:$false -Force -ErrorAction Stop | Out-Null
  }
  $partition = Get-Partition -DiskNumber $disk.Number -PartitionNumber $partition.PartitionNumber -ErrorAction Stop
  Expand-CsiPartitionToDisk $partition
  $partition = Get-Partition -DiskNumber $disk.Number -PartitionNumber $partition.PartitionNumber -ErrorAction Stop
  $accessPath = Get-CsiAccessPath $MountPath
  if (@($partition.AccessPaths) -notcontains $accessPath) {
    Add-PartitionAccessPath -DiskNumber $disk.Number -PartitionNumber $partition.PartitionNumber -AccessPath $accessPath -ErrorAction Stop
  }
}
function Dismount-CsiFileShareVirtualDisk([string]$VhdxPath, [string]$MountPath) {
  $accessPath = Get-CsiAccessPath $MountPath
  $partition = Get-CsiPartitionByAccessPath $MountPath
  if ($partition) {
    Remove-PartitionAccessPath -DiskNumber $partition.DiskNumber -PartitionNumber $partition.PartitionNumber -AccessPath $accessPath -ErrorAction Stop
    $partition = Get-Partition -DiskNumber $partition.DiskNumber -PartitionNumber $partition.PartitionNumber -ErrorAction SilentlyContinue
    if ($partition -and @($partition.AccessPaths) -contains $accessPath) {
      throw "failed to remove partition access path '$accessPath'"
    }
  }
  $image = Get-DiskImage -ImagePath $VhdxPath -ErrorAction SilentlyContinue
  if ($image -and $image.Attached) {
    Dismount-DiskImage -ImagePath $VhdxPath -ErrorAction Stop
    $image = Get-DiskImage -ImagePath $VhdxPath -ErrorAction SilentlyContinue
    if ($image -and $image.Attached) {
      throw "failed to dismount file-share virtual disk '$VhdxPath'"
    }
  }
  if ($MountPath -and (Test-Path -LiteralPath $MountPath)) {
    Remove-Item -LiteralPath $MountPath -Force -ErrorAction Stop
    if (Test-Path -LiteralPath $MountPath) {
      throw "failed to delete file-share mount path '$MountPath'"
    }
  }
}
`

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

func (b *WinRMBackend) MountFileShareVirtualDisk(ctx context.Context, vhdxPath, mountPath string) error {
	s := fmt.Sprintf(`
%s
Mount-CsiFileShareVirtualDisk '%s' '%s'
@{ ok=$true }
`, fileShareVHDXPS, escapePS(vhdxPath), escapePS(mountPath))
	var out map[string]any
	return b.runPS(ctx, s, &out)
}

func (b *WinRMBackend) UnmountFileShareVirtualDisk(ctx context.Context, vhdxPath, mountPath string) error {
	s := fmt.Sprintf(`
%s
Dismount-CsiFileShareVirtualDisk '%s' '%s'
@{ ok=$true }
`, fileShareVHDXPS, escapePS(vhdxPath), escapePS(mountPath))
	var out map[string]any
	return b.runPS(ctx, s, &out)
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
		SharePath:     out.VHDXPath,
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
	return true, VolumeInfo{VolumeName: out.Name, Protocol: ProtocolNFS, NfsServer: out.NfsServer, NfsExportPath: out.NfsExportPath, VHDXPath: out.VHDXPath, SharePath: out.VHDXPath, SizeBytes: out.SizeBytes, CapacityBytes: out.CapacityBytes}, nil
}

func (b *WinRMBackend) DeleteNfsShare(ctx context.Context, name, path string) error {
	s := fmt.Sprintf(`
Import-Module NFS -ErrorAction Stop
%s
$name = '%s'
$path = '%s'
$share = Get-NfsShare -Name $name -ErrorAction SilentlyContinue
if ($share -and -not $path) {
  $path = $share.Path
}
if ($share) {
  Remove-NfsShare -Name $name -Confirm:$false -ErrorAction Stop
  if (Get-NfsShare -Name $name -ErrorAction SilentlyContinue) {
    throw "failed to delete NFS share '$name'"
  }
}
if ($path -and (Test-Path -LiteralPath $path)) {
  if (Get-Command Remove-FsrmQuota -ErrorAction SilentlyContinue) {
    if (Get-FsrmQuota -Path $path -ErrorAction SilentlyContinue) {
      Remove-FsrmQuota -Path $path -Confirm:$false -ErrorAction Stop | Out-Null
      if (Get-FsrmQuota -Path $path -ErrorAction SilentlyContinue) {
        throw "failed to delete quota for NFS share path '$path'"
      }
    }
  }
  if (-not (Test-CsiPartitionAccessPath $path)) {
    Remove-Item -LiteralPath $path -Recurse -Force -ErrorAction Stop
    if (Test-Path -LiteralPath $path) {
      throw "failed to delete NFS share path '$path'"
    }
  }
}
@{ ok=$true }
`, fileShareVHDXPS, escapePS(name), escapePS(path))
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
	return VolumeInfo{VolumeName: out.Name, Protocol: ProtocolSMB, SmbServer: out.SmbServer, SmbShareName: out.SmbShareName, VHDXPath: out.VHDXPath, SharePath: out.VHDXPath, SizeBytes: out.SizeBytes, CapacityBytes: out.CapacityBytes}, nil
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
	return true, VolumeInfo{VolumeName: out.Name, Protocol: ProtocolSMB, SmbServer: out.SmbServer, SmbShareName: out.SmbShareName, VHDXPath: out.VHDXPath, SharePath: out.VHDXPath, SizeBytes: out.SizeBytes, CapacityBytes: out.CapacityBytes}, nil
}

func (b *WinRMBackend) DeleteSmbShare(ctx context.Context, name, path string) error {
	s := fmt.Sprintf(`
Import-Module SmbShare -ErrorAction Stop
%s
$name = '%s'
$path = '%s'
$share = Get-SmbShare -Name $name -ErrorAction SilentlyContinue
if ($share -and -not $path) {
  $path = $share.Path
}
if ($share) {
  Remove-SmbShare -Name $name -Force -ErrorAction Stop
  if (Get-SmbShare -Name $name -ErrorAction SilentlyContinue) {
    throw "failed to delete SMB share '$name'"
  }
}
if ($path -and (Test-Path -LiteralPath $path)) {
  if (Get-Command Remove-FsrmQuota -ErrorAction SilentlyContinue) {
    if (Get-FsrmQuota -Path $path -ErrorAction SilentlyContinue) {
      Remove-FsrmQuota -Path $path -Confirm:$false -ErrorAction Stop | Out-Null
      if (Get-FsrmQuota -Path $path -ErrorAction SilentlyContinue) {
        throw "failed to delete quota for SMB share path '$path'"
      }
    }
  }
  if (-not (Test-CsiPartitionAccessPath $path)) {
    Remove-Item -LiteralPath $path -Recurse -Force -ErrorAction Stop
    if (Test-Path -LiteralPath $path) {
      throw "failed to delete SMB share path '$path'"
    }
  }
}
@{ ok=$true }
`, fileShareVHDXPS, escapePS(name), escapePS(path))
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
	if handle, ok, err := decodeFileShareSnapshotHandle(snapshotID); ok {
		if err != nil {
			return err
		}
		s := fmt.Sprintf(`
%s
%s
$shadowID = '%s'
$sourceRelativePath = '%s'
$storedShadowPath = '%s'
$shadow = Get-CsiShadowCopy $shadowID
if (-not $shadow) {
  throw "file-share VSS snapshot was not found: $shadowID"
}
if (-not [string]::IsNullOrWhiteSpace($sourceRelativePath)) {
  $sourcePath = Join-CsiShadowPath ([string]$shadow.DeviceObject) $sourceRelativePath
} else {
  $sourcePath = $storedShadowPath
}
if (-not (Test-Path -LiteralPath $sourcePath -PathType Container)) {
  throw "file-share snapshot path not found: $sourcePath"
}
Copy-CsiDirectoryTree $sourcePath '%s'
@{ ok=$true }
`, fileShareCopyItemPS, fileShareShadowCopyPS, escapePS(handle.ShadowID), escapePS(handle.SourceRelativePath), escapePS(handle.ShadowPath), escapePS(destinationPath))
		var out map[string]any
		return b.runPS(ctx, s, &out)
	}

	s := fmt.Sprintf(`
%s
%s
%s
$snapshotID = '%s'
$sourcePath = $snapshotID
$isVssSnapshot = $false
if (Test-Path -LiteralPath $snapshotID -PathType Leaf) {
  $meta = Get-Content -LiteralPath $snapshotID -Raw | ConvertFrom-Json
  if ($meta.snapshotType -eq 'vss') {
    $isVssSnapshot = $true
    $shadow = Get-CsiShadowCopy ([string]$meta.shadowId)
    if (-not $shadow) {
      throw "file-share VSS snapshot was not found: $($meta.shadowId)"
    }
    $sourcePath = [string]$meta.shadowPath
    if ([string]::IsNullOrWhiteSpace($sourcePath) -and $meta.sourceRelativePath) {
      $sourcePath = Join-CsiShadowPath ([string]$shadow.DeviceObject) ([string]$meta.sourceRelativePath)
    }
  } elseif ($meta.snapshotId) {
    $sourcePath = [string]$meta.snapshotId
  }
}
if (-not (Test-Path -LiteralPath $sourcePath -PathType Container)) {
  throw "file-share snapshot path not found: $sourcePath"
}
if ($isVssSnapshot) {
  Copy-CsiDirectoryTree $sourcePath '%s'
} else {
  Copy-CsiDirectoryMirror $sourcePath '%s'
}
@{ ok=$true }
`, fileShareCopyPS, fileShareCopyItemPS, fileShareShadowCopyPS, escapePS(snapshotID), escapePS(destinationPath), escapePS(destinationPath))
	var out map[string]any
	return b.runPS(ctx, s, &out)
}
