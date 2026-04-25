// backend_winrm.go
package iscsi

import (
	"context"
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

	client, err := winrm.NewClientWithParameters(b.Endpoint, b.User, b.Pass, params)
	if err != nil {
		return fmt.Errorf("winrm.NewClient: %w", err)
	}
	if b.PSModuleImport == "" {
		b.PSModuleImport = "Import-Module IscsiTarget"
	}
	ps := fmt.Sprintf(`$ErrorActionPreference='Stop'; %s; $IscsiTargetComputerName='localhost'; function Get-MappedIscsiTargetNames([string]$Path) { @(Get-IscsiServerTarget -ComputerName $IscsiTargetComputerName -ErrorAction SilentlyContinue | ForEach-Object { $targetName = $_.TargetName; @($_.LunMappings) | Where-Object { $_.Path -eq $Path } | ForEach-Object { $targetName } }) }; function Get-IscsiInitiatorIQNValues($InitiatorIds) { @($InitiatorIds | ForEach-Object { $raw = ''; if ($_ -is [string]) { $raw = $_ } elseif ($_.PSObject.Properties['IQN'] -and $_.IQN) { $raw = $_.IQN } elseif ($_.PSObject.Properties['Value'] -and $_.Method -eq 'Iqn') { $raw = $_.Value }; if (-not [string]::IsNullOrWhiteSpace($raw)) { if ($raw -like 'IQN:*') { $raw.Substring(4) } else { $raw } } }) }; try { $result = & { %s }; $result | ConvertTo-Json -Compress -Depth 6 } catch { Write-Error $_; exit 1 }`,
		b.PSModuleImport, script)

	done := make(chan struct{})
	var runErr error
	var stdout, stderr string
	var exitCode int

	go func() {
		defer close(done)
		stdout, stderr, exitCode, runErr = client.RunPSWithContext(ctx, ps)
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

func (b *WinRMBackend) EnsureTarget(ctx context.Context, targetIQN string) error {
	s := fmt.Sprintf(`
$t = Get-IscsiServerTarget -ComputerName $IscsiTargetComputerName -TargetName '%s' -ErrorAction SilentlyContinue
if (-not $t) { New-IscsiServerTarget -ComputerName $IscsiTargetComputerName -TargetName '%s' | Out-Null }
@{ ok = $true }`, escapePS(targetIQN), escapePS(targetIQN))
	var out map[string]any
	return b.runPS(ctx, s, &out)
}

func (b *WinRMBackend) CreateVirtualDisk(ctx context.Context, name, parentDir string, sizeBytes int64) (string, int64, error) {
	s := fmt.Sprintf(`
$path = Join-Path -Path '%s' -ChildPath ('%s' + '.vhdx')
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

func (b *WinRMBackend) MapDiskToTarget(ctx context.Context, targetIQN, vhdxPath string) (int32, error) {
	s := fmt.Sprintf(`
$vd = Get-IscsiVirtualDisk -ComputerName $IscsiTargetComputerName -Path '%s'
$mappedTargets = @(Get-MappedIscsiTargetNames -Path '%s')
if ($mappedTargets -notcontains '%s') {
  Add-IscsiVirtualDiskTargetMapping -ComputerName $IscsiTargetComputerName -TargetName '%s' -Path '%s' | Out-Null
}
# Single-disk target → LUN 0
@{ lun = 0 }
`, escapePS(vhdxPath), escapePS(vhdxPath), escapePS(targetIQN), escapePS(targetIQN), escapePS(vhdxPath))
	var out struct {
		LUN int32 `json:"lun"`
	}
	if err := b.runPS(ctx, s, &out); err != nil {
		return 0, err
	}
	return out.LUN, nil
}

func (b *WinRMBackend) UnmapDiskFromTarget(ctx context.Context, targetIQN, vhdxPath string) error {
	s := fmt.Sprintf(`
if (Get-IscsiVirtualDisk -ComputerName $IscsiTargetComputerName -Path '%s' -ErrorAction SilentlyContinue) {
  Remove-IscsiVirtualDiskTargetMapping -ComputerName $IscsiTargetComputerName -TargetName '%s' -Path '%s' -ErrorAction SilentlyContinue
}
@{ ok = $true }
`, escapePS(vhdxPath), escapePS(targetIQN), escapePS(vhdxPath))
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

func (b *WinRMBackend) GetVolumeByName(ctx context.Context, name, parentDir string) (bool, string, int64, string, int32, error) {
	s := fmt.Sprintf(`
$path = Join-Path -Path '%s' -ChildPath ('%s' + '.vhdx')
if (-not (Test-Path -LiteralPath $path)) {
  @{ exists=$false }
} else {
  $vd = Get-IscsiVirtualDisk -ComputerName $IscsiTargetComputerName -Path $path
  $targets = @(Get-MappedIscsiTargetNames -Path $path)
  $lun = if ($targets.Count -gt 0) { 0 } else { -1 }
  $targetIQN = ''
  if ($targets.Count -gt 0) {
    $targetIQN = [string]$targets[0]
  }
  @{ exists=$true; path=$path; sizeBytes=[int64]$vd.Size; targetIQN=$targetIQN; lun = $lun }
}
`, escapePS(parentDir), escapePS(name))
	var out struct {
		Exists    bool   `json:"exists"`
		Path      string `json:"path"`
		SizeBytes int64  `json:"sizeBytes"`
		TargetIQN string `json:"targetIQN"`
		LUN       int32  `json:"lun"`
	}
	if err := b.runPS(ctx, s, &out); err != nil {
		return false, "", 0, "", -1, err
	}
	if !out.Exists {
		return false, "", 0, "", -1, nil
	}
	return out.Exists, out.Path, out.SizeBytes, out.TargetIQN, out.LUN, nil
}

func (b *WinRMBackend) AllowInitiator(ctx context.Context, targetIQN, initiatorIQN string) error {
	s := fmt.Sprintf(`
$t = Get-IscsiServerTarget -ComputerName $IscsiTargetComputerName -TargetName '%s'
$iqns = @(Get-IscsiInitiatorIQNValues $t.InitiatorIds)
if ($iqns -notcontains '%s') {
  $iqns = $iqns + '%s'
  $initiatorIds = @($iqns | %% { if ($_ -like 'IQN:*') { $_ } else { 'IQN:' + $_ } })
  Set-IscsiServerTarget -ComputerName $IscsiTargetComputerName -TargetName '%s' -InitiatorIds $initiatorIds | Out-Null
}
@{ ok = $true }
`, escapePS(targetIQN), escapePS(initiatorIQN), escapePS(initiatorIQN), escapePS(targetIQN))
	var out map[string]any
	return b.runPS(ctx, s, &out)
}

func (b *WinRMBackend) DenyInitiator(ctx context.Context, targetIQN, initiatorIQN string) error {
	s := fmt.Sprintf(`
$t = Get-IscsiServerTarget -ComputerName $IscsiTargetComputerName -TargetName '%s' -ErrorAction SilentlyContinue
if ($t) {
  $iqns = @(Get-IscsiInitiatorIQNValues $t.InitiatorIds) | Where-Object { $_ -ne '%s' }
  $initiatorIds = @($iqns | %% { if ($_ -like 'IQN:*') { $_ } else { 'IQN:' + $_ } })
  Set-IscsiServerTarget -ComputerName $IscsiTargetComputerName -TargetName '%s' -InitiatorIds $initiatorIds | Out-Null
}
@{ ok = $true }
`, escapePS(targetIQN), escapePS(initiatorIQN), escapePS(targetIQN))
	var out map[string]any
	return b.runPS(ctx, s, &out)
}

func (b *WinRMBackend) GetTargetInitiators(ctx context.Context, targetIQN string) ([]string, error) {
	s := fmt.Sprintf(`
$t = Get-IscsiServerTarget -ComputerName $IscsiTargetComputerName -TargetName '%s' -ErrorAction SilentlyContinue
if (-not $t) { @{ iqns=@() } } else { @{ iqns = @(Get-IscsiInitiatorIQNValues $t.InitiatorIds) } }
`, escapePS(targetIQN))
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
$item = Get-Item -LiteralPath '%s'
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

func (b *WinRMBackend) CreateSnapshot(ctx context.Context, vhdxPath, description string) (SnapshotInfo, error) {
	var s string
	if strings.HasSuffix(strings.ToLower(strings.TrimSpace(vhdxPath)), ".vhdx") {
		s = fmt.Sprintf(`
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
	} else {
		s = fmt.Sprintf(`
$sourcePath = '%s'
$description = '%s'
%s
if (-not (Test-Path -LiteralPath $sourcePath -PathType Container)) { throw "file-share snapshot source path not found: $sourcePath" }
$parent = Split-Path -Parent $sourcePath
if ([string]::IsNullOrWhiteSpace($parent)) { $parent = $sourcePath }
$root = Join-Path -Path $parent -ChildPath '.csi-snapshots'
New-Item -ItemType Directory -Path $root -Force | Out-Null
$safe = ($description -replace '[^A-Za-z0-9_.-]', '_').Trim('_')
if ([string]::IsNullOrWhiteSpace($safe)) { $safe = [guid]::NewGuid().ToString('N') }
$snapshotPath = Join-Path -Path $root -ChildPath $safe
if (Test-Path -LiteralPath $snapshotPath) {
  $snapshotPath = Join-Path -Path $root -ChildPath "$safe-$([guid]::NewGuid().ToString('N').Substring(0,8))"
}
Copy-CsiDirectoryMirror $sourcePath $snapshotPath
$createdAt = (Get-Date).ToUniversalTime().ToString('o')
$meta = @{ snapshotId=$snapshotPath; originalPath=$sourcePath; description=$description; createdAt=$createdAt; sizeBytes=[int64]0 }
$meta | ConvertTo-Json -Compress | Set-Content -LiteralPath (Join-Path -Path $snapshotPath -ChildPath '.csi-snapshot.json') -Encoding UTF8
$meta
`, escapePS(vhdxPath), escapePS(description), fileShareCopyPS)
	}
	var out struct {
		SnapshotID   string    `json:"snapshotId"`
		OriginalPath string    `json:"originalPath"`
		Description  string    `json:"description"`
		CreatedAt    time.Time `json:"createdAt"`
		SizeBytes    int64     `json:"sizeBytes"`
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
	s := fmt.Sprintf(`
$snapshotID = '%s'
$snapshotGuid = [guid]::Empty
if ([guid]::TryParse($snapshotID, [ref]$snapshotGuid) -and (Get-IscsiVirtualDiskSnapshot -ComputerName $IscsiTargetComputerName -SnapshotId $snapshotID -ErrorAction SilentlyContinue)) {
  Remove-IscsiVirtualDiskSnapshot -ComputerName $IscsiTargetComputerName -SnapshotId $snapshotID -ErrorAction SilentlyContinue
} elseif (Test-Path -LiteralPath $snapshotID -PathType Container) {
  Remove-Item -LiteralPath $snapshotID -Recurse -Force -ErrorAction SilentlyContinue
}
@{ ok=$true }
`, escapePS(snapshotID))
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
		s = fmt.Sprintf(`
$sourcePath = '%s'
$root = Join-Path -Path (Split-Path -Parent $sourcePath) -ChildPath '.csi-snapshots'
if (Test-Path -LiteralPath $root -PathType Container) {
  $sn = @(Get-ChildItem -LiteralPath $root -Directory -ErrorAction SilentlyContinue | ForEach-Object {
    $metaPath = Join-Path -Path $_.FullName -ChildPath '.csi-snapshot.json'
    if (Test-Path -LiteralPath $metaPath) {
      $meta = Get-Content -LiteralPath $metaPath -Raw | ConvertFrom-Json
      if ($meta.originalPath -eq $sourcePath) {
        @{ snapshotId=[string]$meta.snapshotId; originalPath=[string]$meta.originalPath; description=[string]$meta.description; createdAt=[string]$meta.createdAt; sizeBytes=[int64]$meta.sizeBytes }
      }
    }
  })
} else {
  $sn = @()
}
@{ snapshots=$sn }
`, escapePS(vhdxPath))
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
