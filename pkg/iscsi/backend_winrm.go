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

// SnapshotInfo and VolumeInfo are shared with controllerserver.go (same package).
// type SnapshotInfo struct { ... }
// type VolumeInfo  struct { ... }

type WinRMBackend struct {
	Endpoint *winrm.Endpoint
	User     string
	Pass     string

	// Optional knobs
	PSModuleImport string        // default: "Import-Module IscsiTarget"
	Timeout        time.Duration // default: 60s
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
		PSModuleImport: "Import-Module IscsiTarget",
		Timeout:        timeout,
	}
}

// runPS executes a PowerShell script that MUST produce JSON on stdout.
// The script is wrapped with error handling and ConvertTo-Json.
func (b *WinRMBackend) runPS(ctx context.Context, script string, out any) error {
	client, err := winrm.NewClient(b.Endpoint, b.User, b.Pass)
	if err != nil {
		return fmt.Errorf("winrm.NewClient: %w", err)
	}
	if b.PSModuleImport == "" {
		b.PSModuleImport = "Import-Module IscsiTarget"
	}
	ps := fmt.Sprintf(`$ErrorActionPreference='Stop'; %s; try { %s | ConvertTo-Json -Compress -Depth 6 } catch { Write-Error $_; exit 1 }`,
		b.PSModuleImport, script)

	var stdout, stderr string
	done := make(chan struct{})
	var runErr error

	go func() {
		defer close(done)
		_, runErr = client.Run(ps, &stringWriter{&stdout}, &stringWriter{&stderr})
	}()

	select {
	case <-ctx.Done():
		return fmt.Errorf("winrm: context canceled: %w", ctx.Err())
	case <-done:
	}

	if runErr != nil {
		return fmt.Errorf("winrm run failed: %w; stderr=%s", runErr, stderr)
	}
	if out != nil {
		// Empty output is valid for some actions; only unmarshal if non-empty.
		if strings.TrimSpace(stdout) != "" {
			if err := json.Unmarshal([]byte(stdout), out); err != nil {
				return fmt.Errorf("json unmarshal failed: %w; raw=%s", err, stdout)
			}
		}
	}
	return nil
}

type stringWriter struct{ s *string }

func (w *stringWriter) Write(p []byte) (int, error) {
	*w.s += string(p)
	return len(p), nil
}

func escapePS(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// ------------------- Backend implementation -------------------

func (b *WinRMBackend) EnsureTarget(ctx context.Context, targetIQN string) error {
	s := fmt.Sprintf(`
$t = Get-IscsiServerTarget -TargetName '%s' -ErrorAction SilentlyContinue
if (-not $t) { New-IscsiServerTarget -TargetName '%s' | Out-Null }
@{ ok = $true }`, escapePS(targetIQN), escapePS(targetIQN))
	var out map[string]any
	return b.runPS(ctx, s, &out)
}

func (b *WinRMBackend) CreateVirtualDisk(ctx context.Context, name, parentDir string, sizeBytes int64) (string, int64, error) {
	s := fmt.Sprintf(`
$path = Join-Path -Path '%s' -ChildPath ('%s' + '.vhdx')
if (-not (Test-Path -LiteralPath $path)) {
  New-IscsiVirtualDisk -Path $path -SizeBytes %d | Out-Null
}
$vd = Get-IscsiVirtualDisk -Path $path
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
$vd = Get-IscsiVirtualDisk -Path '%s'
$mappedTargets = @($vd.TargetNames)
if ($mappedTargets -notcontains '%s') {
  Add-IscsiVirtualDiskTargetMapping -TargetName '%s' -Path '%s' | Out-Null
}
# Single-disk target → LUN 0
@{ lun = 0 }
`, escapePS(vhdxPath), escapePS(targetIQN), escapePS(targetIQN), escapePS(vhdxPath))
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
if (Get-IscsiVirtualDisk -Path '%s' -ErrorAction SilentlyContinue) {
  Remove-IscsiVirtualDiskTargetMapping -TargetName '%s' -Path '%s' -ErrorAction SilentlyContinue
}
@{ ok = $true }
`, escapePS(vhdxPath), escapePS(targetIQN), escapePS(vhdxPath))
	var out map[string]any
	return b.runPS(ctx, s, &out)
}

func (b *WinRMBackend) DeleteVirtualDisk(ctx context.Context, vhdxPath string) error {
	s := fmt.Sprintf(`
if (Get-IscsiVirtualDisk -Path '%s' -ErrorAction SilentlyContinue) {
  Remove-IscsiVirtualDisk -Path '%s' -ErrorAction SilentlyContinue
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
  $vd = Get-IscsiVirtualDisk -Path $path
  $targets = @($vd.TargetNames)
  $lun = if ($targets.Count -gt 0) { 0 } else { -1 }
  @{ exists=$true; path=$path; sizeBytes=[int64]$vd.Size; targetIQN=($targets | Select-Object -First 1); lun = $lun }
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
	return out.Exists, out.Path, out.SizeBytes, out.TargetIQN, out.LUN, nil
}

func (b *WinRMBackend) AllowInitiator(ctx context.Context, targetIQN, initiatorIQN string) error {
	s := fmt.Sprintf(`
$t = Get-IscsiServerTarget -TargetName '%s'
$iqns = @($t.InitiatorIds | %% { $_.IQN })
if ($iqns -notcontains '%s') {
  $iqns = $iqns + '%s'
  Set-IscsiServerTarget -TargetName '%s' -InitiatorId $iqns | Out-Null
}
@{ ok = $true }
`, escapePS(targetIQN), escapePS(initiatorIQN), escapePS(initiatorIQN), escapePS(targetIQN))
	var out map[string]any
	return b.runPS(ctx, s, &out)
}

func (b *WinRMBackend) DenyInitiator(ctx context.Context, targetIQN, initiatorIQN string) error {
	s := fmt.Sprintf(`
$t = Get-IscsiServerTarget -TargetName '%s' -ErrorAction SilentlyContinue
if ($t) {
  $iqns = @($t.InitiatorIds | %% { $_.IQN }) | Where-Object { $_ -ne '%s' }
  Set-IscsiServerTarget -TargetName '%s' -InitiatorId $iqns | Out-Null
}
@{ ok = $true }
`, escapePS(targetIQN), escapePS(initiatorIQN), escapePS(targetIQN))
	var out map[string]any
	return b.runPS(ctx, s, &out)
}

func (b *WinRMBackend) GetTargetInitiators(ctx context.Context, targetIQN string) ([]string, error) {
	s := fmt.Sprintf(`
$t = Get-IscsiServerTarget -TargetName '%s' -ErrorAction SilentlyContinue
if (-not $t) { @{ iqns=@() } } else { @{ iqns = @($t.InitiatorIds | %% { $_.IQN }) } }
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
	s := fmt.Sprintf(`
$null = Checkpoint-IscsiVirtualDisk -OriginalPath '%s' -Description '%s'
$g = Get-IscsiVirtualDiskSnapshot -OriginalPath '%s' | Sort-Object CreationTime -Descending | Select-Object -First 1
@{
  snapshotId   = $g.SnapshotId.ToString()
  originalPath = '%s'
  description  = $g.Description
  createdAt    = $g.CreationTime.ToUniversalTime()
  sizeBytes    = 0
}
`, escapePS(vhdxPath), escapePS(description), escapePS(vhdxPath), escapePS(vhdxPath))
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
Remove-IscsiVirtualDiskSnapshot -SnapshotId '%s' -ErrorAction SilentlyContinue
@{ ok=$true }
`, escapePS(snapshotID))
	var out map[string]any
	return b.runPS(ctx, s, &out)
}

func (b *WinRMBackend) ListSnapshots(ctx context.Context, vhdxPath string) ([]SnapshotInfo, error) {
	s := fmt.Sprintf(`
$sn = Get-IscsiVirtualDiskSnapshot -OriginalPath '%s' -ErrorAction SilentlyContinue
$sn | Select-Object @{n='snapshotId';e={$_.SnapshotId.ToString()}},
                     @{n='originalPath';e={'%s'}},
                     @{n='description';e={$_.Description}},
                     @{n='createdAt';e={$_.CreationTime.ToUniversalTime()}},
                     @{n='sizeBytes';e={0}}
`, escapePS(vhdxPath), escapePS(vhdxPath))
	// Let JSON map directly into []SnapshotInfo; field names match via json tags
	var out []struct {
		SnapshotID   string    `json:"snapshotId"`
		OriginalPath string    `json:"originalPath"`
		Description  string    `json:"description"`
		CreatedAt    time.Time `json:"createdAt"`
		SizeBytes    int64     `json:"sizeBytes"`
	}
	if err := b.runPS(ctx, s, &out); err != nil {
		return nil, err
	}
	res := make([]SnapshotInfo, 0, len(out))
	for _, s := range out {
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
$vd = Export-IscsiVirtualDiskSnapshot -SnapshotId '%s'
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
Resize-IscsiVirtualDisk -Path '%s' -SizeBytes %d | Out-Null
$vd = Get-IscsiVirtualDisk -Path '%s'
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
  $vd = Get-IscsiVirtualDisk -Path '%s'
  $targets = @($vd.TargetNames)
  $lun = if ($targets.Count -gt 0) { 0 } else { $null }
  @{ path='%s'; sizeBytes=[int64]$vd.Size; targets=$targets; lun=$lun }
}
`, escapePS(vhdxPath), escapePS(vhdxPath), escapePS(vhdxPath))
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
