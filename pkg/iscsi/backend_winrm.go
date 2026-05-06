// backend_winrm.go
package iscsi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/masterzen/winrm"
)

type Endpoint = winrm.Endpoint

// SnapshotInfo and VolumeInfo are shared with controllerserver.go (same package).
// type SnapshotInfo struct { ... }
// type VolumeInfo  struct { ... }

type powerShellRunner func(ctx context.Context, script string, out any) error

type winRMClient interface {
	RunPSWithContextWithString(ctx context.Context, command string, stdin string) (string, string, int, error)
}

var newWinRMClientWithParameters = func(endpoint *winrm.Endpoint, user, password string, params *winrm.Parameters) (winRMClient, error) {
	return winrm.NewClientWithParameters(endpoint, user, password, params)
}

type winRMSnapshotOutput struct {
	SnapshotID   string    `json:"snapshotId"`
	OriginalPath string    `json:"originalPath"`
	Description  string    `json:"description"`
	CreatedAt    time.Time `json:"createdAt"`
	SizeBytes    int64     `json:"sizeBytes"`
}

type winRMSnapshotListOutput struct {
	Snapshots []winRMSnapshotOutput `json:"snapshots"`
}

const fileShareSnapshotIDPrefix = "vss://"

type fileShareSnapshotHandle struct {
	SnapshotType       string    `json:"snapshotType"`
	ShadowID           string    `json:"shadowId"`
	ShadowDeviceObject string    `json:"shadowDeviceObject"`
	ShadowPath         string    `json:"shadowPath"`
	SourceRelativePath string    `json:"sourceRelativePath"`
	OriginalPath       string    `json:"originalPath"`
	Description        string    `json:"description"`
	CreatedAt          time.Time `json:"createdAt"`
	SizeBytes          int64     `json:"sizeBytes"`
}

func encodeFileShareSnapshotHandle(handle fileShareSnapshotHandle) (string, error) {
	if strings.TrimSpace(handle.SnapshotType) == "" {
		handle.SnapshotType = "vss"
	}
	data, err := json.Marshal(handle)
	if err != nil {
		return "", err
	}
	return fileShareSnapshotIDPrefix + base64.RawURLEncoding.EncodeToString(data), nil
}

func decodeFileShareSnapshotHandle(snapshotID string) (fileShareSnapshotHandle, bool, error) {
	if !strings.HasPrefix(snapshotID, fileShareSnapshotIDPrefix) {
		return fileShareSnapshotHandle{}, false, nil
	}
	encoded := strings.TrimPrefix(snapshotID, fileShareSnapshotIDPrefix)
	data, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return fileShareSnapshotHandle{}, true, fmt.Errorf("invalid file-share snapshot handle: %w", err)
	}
	var handle fileShareSnapshotHandle
	if err := json.Unmarshal(data, &handle); err != nil {
		return fileShareSnapshotHandle{}, true, fmt.Errorf("invalid file-share snapshot handle: %w", err)
	}
	if strings.TrimSpace(handle.SnapshotType) == "" {
		handle.SnapshotType = "vss"
	}
	if !strings.EqualFold(handle.SnapshotType, "vss") {
		return fileShareSnapshotHandle{}, true, fmt.Errorf("unsupported file-share snapshot type %q", handle.SnapshotType)
	}
	if strings.TrimSpace(handle.ShadowID) == "" {
		return fileShareSnapshotHandle{}, true, fmt.Errorf("file-share snapshot handle is missing shadowId")
	}
	return handle, true, nil
}

type WinRMBackend struct {
	Endpoint *winrm.Endpoint
	User     string
	Pass     string
	Auth     string // default: "basic"; supported: "basic", "ntlm"

	// Optional knobs
	PSModuleImport string        // default: "Import-Module IscsiTarget"
	Timeout        time.Duration // default: 60s

	psRunner powerShellRunner
}

// NewWinRMBackend creates a backend connected to the Windows (Storage Server) host.
func NewWinRMBackend(host string, port int, https, insecure bool, user, pass string, timeout time.Duration) *WinRMBackend {
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	ep := &winrm.Endpoint{
		Host:     host,
		Port:     port,
		HTTPS:    https,
		Insecure: insecure,
		// TLSClientConfig/CACert may be set by caller via Endpoint if needed.
		Timeout: timeout,
	}
	return &WinRMBackend{
		Endpoint:       ep,
		User:           user,
		Pass:           pass,
		Auth:           "basic",
		PSModuleImport: "Import-Module IscsiTarget",
		Timeout:        timeout,
	}
}

func (b *WinRMBackend) clientParameters() (*winrm.Parameters, error) {
	params := *winrm.DefaultParameters
	switch normalizeWinRMAuth(b.Auth) {
	case "basic":
		return &params, nil
	case "ntlm":
		params.TransportDecorator = func() winrm.Transporter {
			return &winrm.ClientNTLM{}
		}
		return &params, nil
	default:
		return nil, fmt.Errorf("unsupported WinRM auth mode %q; supported values are basic and ntlm", b.Auth)
	}
}

func normalizeWinRMAuth(auth string) string {
	switch strings.ToLower(strings.TrimSpace(auth)) {
	case "", "basic":
		return "basic"
	case "ntlm", "negotiate":
		return "ntlm"
	default:
		return strings.ToLower(strings.TrimSpace(auth))
	}
}

// runPS executes a PowerShell script that MUST produce JSON on stdout.
// The script is wrapped with error handling and ConvertTo-Json.
func (b *WinRMBackend) runPS(ctx context.Context, script string, out any) error {
	if b.psRunner != nil {
		return b.psRunner(ctx, script, out)
	}

	params, err := b.clientParameters()
	if err != nil {
		return err
	}

	client, err := newWinRMClientWithParameters(b.Endpoint, b.User, b.Pass, params)
	if err != nil {
		return fmt.Errorf("winrm.NewClient: %w", err)
	}
	if b.PSModuleImport == "" {
		b.PSModuleImport = "Import-Module IscsiTarget"
	}
	ps := fmt.Sprintf(`$ErrorActionPreference='Stop'; %s; $IscsiTargetComputerName='localhost'; function Resolve-CsiVHDXParentPath([string]$ParentDir) { $path = $ParentDir; if ([string]::IsNullOrWhiteSpace($path)) { $path = [Environment]::GetEnvironmentVariable('CSI_VHDX_PARENT_PATH', 'Machine') }; if ([string]::IsNullOrWhiteSpace($path)) { $path = [Environment]::GetEnvironmentVariable('CSI_VHDX_PARENT_PATH', 'Process') }; if ([string]::IsNullOrWhiteSpace($path)) { $systemDrive = $env:SystemDrive; if ([string]::IsNullOrWhiteSpace($systemDrive)) { $systemDrive = 'C:' }; $path = $systemDrive.TrimEnd('\') + '\iSCSIVirtualDisks' }; New-Item -ItemType Directory -Force -Path $path | Out-Null; return (Get-Item -LiteralPath $path).FullName.TrimEnd([char[]]@('\','/')) }; function Get-MappedIscsiTargets([string]$Path) { @(Get-IscsiServerTarget -ComputerName $IscsiTargetComputerName -ErrorAction SilentlyContinue | ForEach-Object { $target = $_; @($target.LunMappings) | Where-Object { $_.Path -eq $Path } | ForEach-Object { [pscustomobject]@{ TargetName=[string]$target.TargetName; TargetIQN=[string]$target.TargetIqn } } }) }; function Get-MappedIscsiTargetNames([string]$Path) { @(Get-MappedIscsiTargets -Path $Path | ForEach-Object { $_.TargetName }) }; function Get-IscsiInitiatorIQNValues($InitiatorIds) { @($InitiatorIds | ForEach-Object { $raw = ''; if ($_ -is [string]) { $raw = $_ } elseif ($_.PSObject.Properties['IQN'] -and $_.IQN) { $raw = $_.IQN } elseif ($_.PSObject.Properties['Value'] -and $_.Method -eq 'Iqn') { $raw = $_.Value }; if (-not [string]::IsNullOrWhiteSpace($raw)) { if ($raw -like 'IQN:*') { $raw.Substring(4) } else { $raw } } }) }; try { $result = & { %s }; $result | ConvertTo-Json -Compress -Depth 6 } catch { Write-Error $_; exit 1 }`,
		b.PSModuleImport, script)

	done := make(chan struct{})
	var runErr error
	var stdout, stderr string
	var exitCode int

	go func() {
		defer close(done)
		stdout, stderr, exitCode, runErr = client.RunPSWithContextWithString(ctx, "Invoke-Expression ([Console]::In.ReadToEnd())", ps)
	}()

	select {
	case <-ctx.Done():
		return fmt.Errorf("winrm run failed: context canceled: %w", ctx.Err())
	case <-done:
	}

	if runErr != nil {
		return fmt.Errorf("winrm run failed: %w; stderr=%s", runErr, stderr)
	}
	if exitCode != 0 {
		return fmt.Errorf("winrm run failed: exit code %d; stderr=%s", exitCode, stderr)
	}
	if out != nil {
		// Empty output is valid for some actions; only unmarshal if non-empty.
		if strings.TrimSpace(stdout) != "" {
			if err := json.Unmarshal([]byte(stdout), out); err != nil {
				return fmt.Errorf("json unmarshal failed: %w; raw=%s", err, stdout)
			}
		} else {
			return fmt.Errorf("winrm run failed: expected JSON output but stdout was empty; stderr=%s", stderr)
		}
	}
	return nil
}

func escapePS(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// ------------------- Backend implementation -------------------

func (b *WinRMBackend) EnsureTarget(ctx context.Context, targetName, targetIQN string) (string, error) {
	targetName = strings.TrimSpace(targetName)
	targetIQN = strings.TrimSpace(targetIQN)
	if targetName == "" {
		return "", fmt.Errorf("target name is required")
	}
	s := fmt.Sprintf(`
$targetName = '%s'
$targetIQN = '%s'
$t = Get-IscsiServerTarget -ComputerName $IscsiTargetComputerName -TargetName $targetName -ErrorAction SilentlyContinue
if (-not $t) {
  $t = New-IscsiServerTarget -ComputerName $IscsiTargetComputerName -TargetName $targetName
}
if (-not [string]::IsNullOrWhiteSpace($targetIQN) -and [string]$t.TargetIqn -ne $targetIQN) {
  $t = Set-IscsiServerTarget -ComputerName $IscsiTargetComputerName -TargetName $targetName -TargetIqn $targetIQN -PassThru
  if (-not $t) {
    $t = Get-IscsiServerTarget -ComputerName $IscsiTargetComputerName -TargetName $targetName
  }
}
@{ targetName=[string]$t.TargetName; targetIQN=[string]$t.TargetIqn }`, escapePS(targetName), escapePS(targetIQN))
	var out struct {
		TargetName string `json:"targetName"`
		TargetIQN  string `json:"targetIQN"`
	}
	if err := b.runPS(ctx, s, &out); err != nil {
		return "", err
	}
	if strings.TrimSpace(out.TargetIQN) == "" {
		return "", fmt.Errorf("target %q has no TargetIqn", targetName)
	}
	return out.TargetIQN, nil
}

func (b *WinRMBackend) ConfigureTargetChap(ctx context.Context, targetName string, opts TargetChapOptions) error {
	targetName = strings.TrimSpace(targetName)
	if targetName == "" {
		return fmt.Errorf("target name is required")
	}
	if !opts.Enabled() {
		return nil
	}
	s := fmt.Sprintf(`
$targetName = '%s'
$params = @{ ComputerName=$IscsiTargetComputerName; TargetName=$targetName }
if (-not [string]::IsNullOrWhiteSpace('%s')) {
  $chapSecret = ConvertTo-SecureString -String '%s' -AsPlainText -Force
  $params.EnableChap = $true
  $params.Chap = [pscredential]::new('%s', $chapSecret)
}
if (-not [string]::IsNullOrWhiteSpace('%s')) {
  $reverseChapSecret = ConvertTo-SecureString -String '%s' -AsPlainText -Force
  $params.EnableReverseChap = $true
  $params.ReverseChap = [pscredential]::new('%s', $reverseChapSecret)
}
Set-IscsiServerTarget @params | Out-Null
@{ ok = $true }
`, escapePS(targetName), escapePS(opts.ChapUser), escapePS(opts.ChapSecret), escapePS(opts.ChapUser), escapePS(opts.ReverseChapUser), escapePS(opts.ReverseChapSecret), escapePS(opts.ReverseChapUser))
	var out map[string]any
	return b.runPS(ctx, s, &out)
}

func (b *WinRMBackend) CreateVirtualDisk(ctx context.Context, name, parentDir string, sizeBytes int64) (string, int64, error) {
	s := fmt.Sprintf(`
$parentDir = Resolve-CsiVHDXParentPath '%s'
$path = Join-Path -Path $parentDir -ChildPath ('%s' + '.vhdx')
if (-not (Test-Path -LiteralPath $path)) {
  New-IscsiVirtualDisk -ComputerName $IscsiTargetComputerName -Path $path -SizeBytes %d | Out-Null
}
$vd = Get-IscsiVirtualDisk -ComputerName $IscsiTargetComputerName -Path $path
@{ path = $path; sizeBytes = [int64]$vd.Size }
`, escapePS(parentDir), escapePS(name), sizeBytes)
	var out struct {
		Path      string `json:"path"`
		SizeBytes int64  `json:"sizeBytes"`
	}
	if err := b.runPS(ctx, s, &out); err != nil {
		return "", 0, err
	}
	return out.Path, out.SizeBytes, nil
}

func (b *WinRMBackend) MapDiskToTarget(ctx context.Context, targetName, vhdxPath string) (int32, error) {
	s := fmt.Sprintf(`
$vd = Get-IscsiVirtualDisk -ComputerName $IscsiTargetComputerName -Path '%s'
$mappedTargets = @(Get-MappedIscsiTargetNames -Path '%s')
if ($mappedTargets -notcontains '%s') {
  Add-IscsiVirtualDiskTargetMapping -ComputerName $IscsiTargetComputerName -TargetName '%s' -Path '%s' | Out-Null
}
# Single-disk target → LUN 0
@{ lun = 0 }
`, escapePS(vhdxPath), escapePS(vhdxPath), escapePS(targetName), escapePS(targetName), escapePS(vhdxPath))
	var out struct {
		LUN int32 `json:"lun"`
	}
	if err := b.runPS(ctx, s, &out); err != nil {
		return 0, err
	}
	return out.LUN, nil
}

func (b *WinRMBackend) UnmapDiskFromTarget(ctx context.Context, targetName, vhdxPath string) error {
	s := fmt.Sprintf(`
if (Get-IscsiVirtualDisk -ComputerName $IscsiTargetComputerName -Path '%s' -ErrorAction SilentlyContinue) {
  Remove-IscsiVirtualDiskTargetMapping -ComputerName $IscsiTargetComputerName -TargetName '%s' -Path '%s' -ErrorAction SilentlyContinue
}
@{ ok = $true }
`, escapePS(vhdxPath), escapePS(targetName), escapePS(vhdxPath))
	var out map[string]any
	return b.runPS(ctx, s, &out)
}

func (b *WinRMBackend) DeleteVirtualDisk(ctx context.Context, vhdxPath string) error {
	s := fmt.Sprintf(`
if (Get-IscsiVirtualDisk -ComputerName $IscsiTargetComputerName -Path '%s' -ErrorAction SilentlyContinue) {
  Remove-IscsiVirtualDisk -ComputerName $IscsiTargetComputerName -Path '%s' -ErrorAction SilentlyContinue
}
if (Test-Path -LiteralPath '%s') { Remove-Item -LiteralPath '%s' -Force -ErrorAction SilentlyContinue }
@{ ok = $true }
`, escapePS(vhdxPath), escapePS(vhdxPath), escapePS(vhdxPath), escapePS(vhdxPath))
	var out map[string]any
	return b.runPS(ctx, s, &out)
}

func (b *WinRMBackend) GetVolumeByName(ctx context.Context, name, parentDir string) (bool, string, int64, string, string, int32, error) {
	s := fmt.Sprintf(`
$parentDir = Resolve-CsiVHDXParentPath '%s'
$path = Join-Path -Path $parentDir -ChildPath ('%s' + '.vhdx')
if (-not (Test-Path -LiteralPath $path)) {
  @{ exists=$false }
} else {
  $vd = Get-IscsiVirtualDisk -ComputerName $IscsiTargetComputerName -Path $path
  $targets = @(Get-MappedIscsiTargets -Path $path)
  $lun = if ($targets.Count -gt 0) { 0 } else { -1 }
  $targetName = ''
  $targetIQN = ''
  if ($targets.Count -gt 0) {
    $targetName = [string]$targets[0].TargetName
    $targetIQN = [string]$targets[0].TargetIQN
  }
  @{ exists=$true; path=$path; sizeBytes=[int64]$vd.Size; targetName=$targetName; targetIQN=$targetIQN; lun = $lun }
}
`, escapePS(parentDir), escapePS(name))
	var out struct {
		Exists     bool   `json:"exists"`
		Path       string `json:"path"`
		SizeBytes  int64  `json:"sizeBytes"`
		TargetName string `json:"targetName"`
		TargetIQN  string `json:"targetIQN"`
		LUN        int32  `json:"lun"`
	}
	if err := b.runPS(ctx, s, &out); err != nil {
		return false, "", 0, "", "", -1, err
	}
	if !out.Exists {
		return false, "", 0, "", "", -1, nil
	}
	return out.Exists, out.Path, out.SizeBytes, out.TargetName, out.TargetIQN, out.LUN, nil
}

func (b *WinRMBackend) AllowInitiator(ctx context.Context, targetName, initiatorIQN string) error {
	s := fmt.Sprintf(`
$t = Get-IscsiServerTarget -ComputerName $IscsiTargetComputerName -TargetName '%s'
$iqns = @(Get-IscsiInitiatorIQNValues $t.InitiatorIds)
if ($iqns -notcontains '%s') {
  $iqns = $iqns + '%s'
  $initiatorIds = @($iqns | %% { if ($_ -like 'IQN:*') { $_ } else { 'IQN:' + $_ } })
  Set-IscsiServerTarget -ComputerName $IscsiTargetComputerName -TargetName '%s' -InitiatorIds $initiatorIds | Out-Null
}
@{ ok = $true }
`, escapePS(targetName), escapePS(initiatorIQN), escapePS(initiatorIQN), escapePS(targetName))
	var out map[string]any
	return b.runPS(ctx, s, &out)
}

func (b *WinRMBackend) DenyInitiator(ctx context.Context, targetName, initiatorIQN string) error {
	s := fmt.Sprintf(`
$t = Get-IscsiServerTarget -ComputerName $IscsiTargetComputerName -TargetName '%s' -ErrorAction SilentlyContinue
if ($t) {
  $iqns = @(Get-IscsiInitiatorIQNValues $t.InitiatorIds) | Where-Object { $_ -ne '%s' }
  $initiatorIds = @($iqns | %% { if ($_ -like 'IQN:*') { $_ } else { 'IQN:' + $_ } })
  Set-IscsiServerTarget -ComputerName $IscsiTargetComputerName -TargetName '%s' -InitiatorIds $initiatorIds | Out-Null
}
@{ ok = $true }
`, escapePS(targetName), escapePS(initiatorIQN), escapePS(targetName))
	var out map[string]any
	return b.runPS(ctx, s, &out)
}

func (b *WinRMBackend) GetTargetInitiators(ctx context.Context, targetName string) ([]string, error) {
	s := fmt.Sprintf(`
$t = Get-IscsiServerTarget -ComputerName $IscsiTargetComputerName -TargetName '%s' -ErrorAction SilentlyContinue
if (-not $t) { @{ iqns=@() } } else { @{ iqns = @(Get-IscsiInitiatorIQNValues $t.InitiatorIds) } }
`, escapePS(targetName))
	var out struct {
		IQNs []string `json:"iqns"`
	}
	if err := b.runPS(ctx, s, &out); err != nil {
		return nil, err
	}
	return out.IQNs, nil
}

func (b *WinRMBackend) GetDirectoryFreeCapacity(ctx context.Context, parentDir string) (int64, error) {
	s := fmt.Sprintf(`
$parentDir = Resolve-CsiVHDXParentPath '%s'
$item = Get-Item -LiteralPath $parentDir
$drive = $item.PSDrive.Name
$psd = Get-PSDrive -Name $drive
@{ free=[int64]$psd.Free }
`, escapePS(parentDir))
	var out struct {
		Free int64 `json:"free"`
	}
	if err := b.runPS(ctx, s, &out); err != nil {
		return 0, err
	}
	return out.Free, nil
}

// ------------------- Snapshots -------------------

const fileShareShadowCopyPS = `
function Get-CsiShadowCopy([string]$ShadowId) {
  @(Get-CimInstance -ClassName Win32_ShadowCopy -ErrorAction SilentlyContinue | Where-Object { $_.ID -eq $ShadowId } | Select-Object -First 1)[0]
}
function New-CsiShadowCopy([string]$VolumeRoot) {
  Invoke-CimMethod -ClassName Win32_ShadowCopy -MethodName Create -Arguments @{ Volume=$VolumeRoot; Context='ClientAccessible' }
}
function Remove-CsiShadowCopy([string]$ShadowId) {
  $shadow = Get-CsiShadowCopy $ShadowId
  if ($shadow) { $shadow | Remove-CimInstance -ErrorAction SilentlyContinue | Out-Null }
}
function Join-CsiShadowPath([string]$DeviceObject, [string]$RelativePath) {
  $root = $DeviceObject.TrimEnd('\')
  if ([string]::IsNullOrWhiteSpace($RelativePath)) { return ($root + '\') }
  return ($root + '\' + $RelativePath.TrimStart('\'))
}
`

func (b *WinRMBackend) CreateSnapshot(ctx context.Context, vhdxPath, description string) (SnapshotInfo, error) {
	isVirtualDisk := strings.HasSuffix(strings.ToLower(strings.TrimSpace(vhdxPath)), ".vhdx")
	if !isVirtualDisk {
		return SnapshotInfo{}, fmt.Errorf("directory-backed file-share snapshots are not supported; use a VHDX-backed NFS/SMB driver")
	}

	s := fmt.Sprintf(`
$description = '%s'
$null = Checkpoint-IscsiVirtualDisk -ComputerName $IscsiTargetComputerName -OriginalPath '%s' -Description $description
$g = $null
for ($i = 0; $i -lt 20; $i++) {
  $g = Get-IscsiVirtualDiskSnapshot -ComputerName $IscsiTargetComputerName -OriginalPath '%s' -ErrorAction SilentlyContinue |
    Where-Object { $_.Description -eq $description } |
    Sort-Object CreationTime -Descending |
    Select-Object -First 1
  if ($g) { break }
  Start-Sleep -Milliseconds 250
}
if (-not $g) {
  throw "Snapshot was not found after Checkpoint-IscsiVirtualDisk completed."
}
$createdAt = (Get-Date).ToUniversalTime().ToString('o')
if ($null -ne $g.CreationTime) {
  $createdAt = $g.CreationTime.ToUniversalTime().ToString('o')
}
@{
  snapshotId   = [string]$g.SnapshotId
  originalPath = '%s'
  description  = $g.Description
  createdAt    = $createdAt
  sizeBytes    = 0
}
`, escapePS(description), escapePS(vhdxPath), escapePS(vhdxPath), escapePS(vhdxPath))
	var out struct {
		SnapshotID         string    `json:"snapshotId"`
		SnapshotType       string    `json:"snapshotType"`
		ShadowID           string    `json:"shadowId"`
		ShadowDeviceObject string    `json:"shadowDeviceObject"`
		ShadowPath         string    `json:"shadowPath"`
		SourceRelativePath string    `json:"sourceRelativePath"`
		OriginalPath       string    `json:"originalPath"`
		Description        string    `json:"description"`
		CreatedAt          time.Time `json:"createdAt"`
		SizeBytes          int64     `json:"sizeBytes"`
	}
	if err := b.runPS(ctx, s, &out); err != nil {
		return SnapshotInfo{}, err
	}
	return SnapshotInfo{
		SnapshotID:   out.SnapshotID,
		OriginalPath: out.OriginalPath,
		Description:  out.Description,
		CreatedAt:    out.CreatedAt,
		SizeBytes:    out.SizeBytes,
	}, nil
}

func (b *WinRMBackend) DeleteSnapshot(ctx context.Context, snapshotID string) error {
	if handle, ok, err := decodeFileShareSnapshotHandle(snapshotID); ok {
		if err != nil {
			return err
		}
		s := fmt.Sprintf(`
%s
Remove-CsiShadowCopy '%s'
@{ ok=$true }
`, fileShareShadowCopyPS, escapePS(handle.ShadowID))
		var out map[string]any
		return b.runPS(ctx, s, &out)
	}

	s := fmt.Sprintf(`
%s
$snapshotID = '%s'
$snapshotGuid = [guid]::Empty
if ([guid]::TryParse($snapshotID, [ref]$snapshotGuid) -and (Get-IscsiVirtualDiskSnapshot -ComputerName $IscsiTargetComputerName -SnapshotId $snapshotID -ErrorAction SilentlyContinue)) {
  Remove-IscsiVirtualDiskSnapshot -ComputerName $IscsiTargetComputerName -SnapshotId $snapshotID -ErrorAction SilentlyContinue
} elseif (Test-Path -LiteralPath $snapshotID -PathType Leaf) {
  $meta = Get-Content -LiteralPath $snapshotID -Raw | ConvertFrom-Json
  if ($meta.snapshotType -eq 'vss' -and $meta.shadowId) {
    Remove-CsiShadowCopy ([string]$meta.shadowId)
  }
  Remove-Item -LiteralPath $snapshotID -Force -ErrorAction SilentlyContinue
} elseif (Test-Path -LiteralPath $snapshotID -PathType Container) {
  Remove-Item -LiteralPath $snapshotID -Recurse -Force -ErrorAction SilentlyContinue
}
@{ ok=$true }
`, fileShareShadowCopyPS, escapePS(snapshotID))
	var out map[string]any
	return b.runPS(ctx, s, &out)
}

func (b *WinRMBackend) ListSnapshots(ctx context.Context, vhdxPath string) ([]SnapshotInfo, error) {
	var s string
	if strings.HasSuffix(strings.ToLower(strings.TrimSpace(vhdxPath)), ".vhdx") {
		s = fmt.Sprintf(`
$sn = @(Get-IscsiVirtualDiskSnapshot -ComputerName $IscsiTargetComputerName -OriginalPath '%s' -ErrorAction SilentlyContinue |
  Select-Object @{n='snapshotId';e={$_.SnapshotId.ToString()}},
                @{n='originalPath';e={'%s'}},
                @{n='description';e={$_.Description}},
                @{n='createdAt';e={if ($null -ne $_.CreationTime) { $_.CreationTime.ToUniversalTime().ToString('o') } else { (Get-Date).ToUniversalTime().ToString('o') }}},
                @{n='sizeBytes';e={0}})
@{ snapshots=$sn }
`, escapePS(vhdxPath), escapePS(vhdxPath))
	} else {
		return []SnapshotInfo{}, nil
	}
	var out winRMSnapshotListOutput
	if err := b.runPS(ctx, s, &out); err != nil {
		return nil, err
	}
	res := make([]SnapshotInfo, 0, len(out.Snapshots))
	for _, s := range out.Snapshots {
		res = append(res, SnapshotInfo{
			SnapshotID:   s.SnapshotID,
			OriginalPath: s.OriginalPath,
			Description:  s.Description,
			CreatedAt:    s.CreatedAt,
			SizeBytes:    s.SizeBytes,
		})
	}
	return res, nil
}

// Export snapshot as a "virtual disk object" and return its Path
func (b *WinRMBackend) ExportSnapshotAsVirtualDisk(ctx context.Context, snapshotID string) (string, error) {
	s := fmt.Sprintf(`
$vd = Export-IscsiVirtualDiskSnapshot -ComputerName $IscsiTargetComputerName -SnapshotId '%s'
@{ path = $vd.Path }
`, escapePS(snapshotID))
	var out struct {
		Path string `json:"path"`
	}
	if err := b.runPS(ctx, s, &out); err != nil {
		return "", err
	}
	return out.Path, nil
}

// ------------------- Expansion / Query -------------------

func (b *WinRMBackend) ResizeVirtualDisk(ctx context.Context, vhdxPath string, newSizeBytes int64) (int64, error) {
	s := fmt.Sprintf(`
Resize-IscsiVirtualDisk -ComputerName $IscsiTargetComputerName -Path '%s' -SizeBytes %d | Out-Null
$vd = Get-IscsiVirtualDisk -ComputerName $IscsiTargetComputerName -Path '%s'
@{ sizeBytes=[int64]$vd.Size }
`, escapePS(vhdxPath), newSizeBytes, escapePS(vhdxPath))
	var out struct {
		SizeBytes int64 `json:"sizeBytes"`
	}
	if err := b.runPS(ctx, s, &out); err != nil {
		return 0, err
	}
	return out.SizeBytes, nil
}

func (b *WinRMBackend) GetVolumeInfo(ctx context.Context, vhdxPath string) (VolumeInfo, error) {
	s := fmt.Sprintf(`
if (-not (Test-Path -LiteralPath '%s')) {
  @{ path=''; sizeBytes=0; targets=@(); lun=$null }
} else {
  $vd = Get-IscsiVirtualDisk -ComputerName $IscsiTargetComputerName -Path '%s'
  $targets = @(Get-MappedIscsiTargetNames -Path '%s')
  $lun = if ($targets.Count -gt 0) { 0 } else { $null }
  @{ path='%s'; sizeBytes=[int64]$vd.Size; targets=$targets; lun=$lun }
}
`, escapePS(vhdxPath), escapePS(vhdxPath), escapePS(vhdxPath), escapePS(vhdxPath))
	var out struct {
		Path      string   `json:"path"`
		SizeBytes int64    `json:"sizeBytes"`
		Targets   []string `json:"targets"`
		LUN       *int32   `json:"lun"`
	}
	if err := b.runPS(ctx, s, &out); err != nil {
		return VolumeInfo{}, err
	}
	return VolumeInfo{
		VHDXPath:  out.Path,
		SizeBytes: out.SizeBytes,
		Targets:   out.Targets,
		LUN:       out.LUN,
	}, nil
}
