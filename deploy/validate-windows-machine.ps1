#Requires -RunAsAdministrator
<#
.SYNOPSIS
Validates that a Windows Server is ready for the CSI iSCSI, NFS, SMB, and WinRM backends.

.DESCRIPTION
Checks the installed features, services, WinRM listener, iSCSI Target cmdlets,
NFS cmdlets, SMB cmdlets, scratch storage paths, and disposable lifecycles:
iSCSI target create, initiator assignment, virtual disk create, map, resize,
snapshot, export, unmap, delete, and cleanup; NFS export create, permission
grant, readback, delete, and cleanup; SMB share create, access readback, delete,
and cleanup.

.EXAMPLE
.\deploy\validate-windows-machine.ps1 `
  -StoragePath C:\data\taliesins\csi-driver-iscsi-for-windows2\vhdx `
  -SharePath C:\data\taliesins\csi-driver-iscsi-for-windows2\shares

.EXAMPLE
.\deploy\validate-windows-machine.ps1 `
  -StoragePath E:\iSCSIVirtualDisks `
  -SharePath E:\CSIFileShares `
  -InitiatorIds "IQN:*" `
  -NfsClientNames "*" `
  -SmbChangeAccess "Everyone" `
  -KeepResources
#>

[CmdletBinding()]
param(
    [Parameter()]
    [ValidateNotNullOrEmpty()]
    [string]$StoragePath = "C:\data\taliesins\csi-driver-iscsi-for-windows2\vhdx",

    [Parameter()]
    [ValidateNotNullOrEmpty()]
    [string]$SharePath = "C:\data\taliesins\csi-driver-iscsi-for-windows2\shares",

    [Parameter()]
    [ValidateRange(1048576, [int64]::MaxValue)]
    [int64]$InitialSizeBytes = 67108864,

    [Parameter()]
    [ValidateRange(1048576, [int64]::MaxValue)]
    [int64]$ResizedSizeBytes = 134217728,

    [Parameter()]
    [string]$TargetName,

    [Parameter()]
    [string]$DiskName,

    [Parameter()]
    [string]$NfsShareName,

    [Parameter()]
    [string]$SmbShareName,

    [Parameter()]
    [ValidateNotNullOrEmpty()]
    [string[]]$InitiatorIds = @("IQN:*"),

    [Parameter()]
    [ValidateNotNullOrEmpty()]
    [string[]]$NfsClientNames = @("*"),

    [Parameter()]
    [ValidateNotNullOrEmpty()]
    [string[]]$SmbChangeAccess = @("Everyone"),

    [Parameter()]
    [ValidateNotNullOrEmpty()]
    [string]$IscsiTargetComputerName = "localhost",

    [Parameter()]
    [switch]$SkipWinRMChecks,

    [Parameter()]
    [switch]$SkipNfsChecks,

    [Parameter()]
    [switch]$SkipSmbChecks,

    [Parameter()]
    [switch]$KeepResources
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

if ($PSVersionTable.PSEdition -ne "Desktop") {
    $command = "powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\deploy\validate-windows-machine.ps1 -StoragePath `"$StoragePath`" -SharePath `"$SharePath`" -IscsiTargetComputerName `"$IscsiTargetComputerName`""
    throw @"
This validator must run in Windows PowerShell 5.1, not PowerShell $($PSVersionTable.PSVersion) ($($PSVersionTable.PSEdition)).

The IscsiTarget module is loaded through a WinPSCompatSession in PowerShell 7+,
which can cause iSCSI Target cmdlets such as Get-IscsiTargetServerSetting to
fail even when the local session is elevated.

Open an elevated Windows PowerShell session or run:

  $command
"@
}

if ($ResizedSizeBytes -le $InitialSizeBytes) {
    throw "ResizedSizeBytes must be greater than InitialSizeBytes."
}

$runID = [guid]::NewGuid().ToString("N").Substring(0, 12)
if ([string]::IsNullOrWhiteSpace($TargetName)) {
    $TargetName = "iqn.2024-01.com.example:csi-validate-$runID"
}
if ([string]::IsNullOrWhiteSpace($DiskName)) {
    $DiskName = "csi-validate-$runID"
}
if ([string]::IsNullOrWhiteSpace($NfsShareName)) {
    $NfsShareName = "csi-nfs-$runID"
}
if ([string]::IsNullOrWhiteSpace($SmbShareName)) {
    $SmbShareName = "csi-smb-$runID"
}

$diskPath = Join-Path -Path $StoragePath -ChildPath "$DiskName.vhdx"
$nfsSharePath = Join-Path -Path $SharePath -ChildPath $NfsShareName
$smbSharePath = Join-Path -Path $SharePath -ChildPath $SmbShareName
$createdTarget = $false
$createdVirtualDisks = New-Object System.Collections.Generic.List[string]
$createdSnapshots = New-Object System.Collections.Generic.List[string]
$createdNfsShares = New-Object System.Collections.Generic.List[string]
$createdSmbShares = New-Object System.Collections.Generic.List[string]
$createdShareDirectories = New-Object System.Collections.Generic.List[string]

function Write-Section {
    param([Parameter(Mandatory = $true)][string]$Message)
    Write-Host ""
    Write-Host "== $Message ==" -ForegroundColor Cyan
}

function Write-Ok {
    param([Parameter(Mandatory = $true)][string]$Message)
    Write-Host "[OK] $Message" -ForegroundColor Green
}

function Assert-CommandAvailable {
    param([Parameter(Mandatory = $true)][string]$Name)
    if (-not (Get-Command -Name $Name -ErrorAction SilentlyContinue)) {
        throw "Required command '$Name' was not found."
    }
}

function Assert-ServiceRunning {
    param([Parameter(Mandatory = $true)][string]$Name)

    $service = Get-Service -Name $Name -ErrorAction SilentlyContinue
    if ($null -eq $service) {
        throw "Required service '$Name' was not found."
    }
    if ($service.Status -ne "Running") {
        Write-Host "Starting service '$Name'..."
        Set-Service -Name $Name -StartupType Automatic
        Start-Service -Name $Name
        $service = Get-Service -Name $Name
    }
    if ($service.Status -ne "Running") {
        throw "Service '$Name' is not running."
    }
    Write-Ok "Service '$Name' is running."
}

function Assert-WindowsFeatureInstalled {
    param([Parameter(Mandatory = $true)][string]$Name)

    if (-not (Get-Command Get-WindowsFeature -ErrorAction SilentlyContinue)) {
        Write-Warning "Get-WindowsFeature is unavailable; skipping feature check for '$Name'."
        return
    }

    $feature = Get-WindowsFeature -Name $Name -ErrorAction SilentlyContinue
    if ($null -eq $feature) {
        Write-Warning "Windows feature '$Name' was not found on this host."
        return
    }
    if ($feature.InstallState -ne "Installed") {
        throw "Windows feature '$Name' is $($feature.InstallState), not Installed."
    }
    Write-Ok "Windows feature '$Name' is installed."
}

function Normalize-InitiatorId {
    param([Parameter(Mandatory = $true)][string]$Value)

    if ($Value -match '^(DNSName|IPAddress|IPv6Address|IQN|MACAddress):') {
        return $Value
    }
    return "IQN:$Value"
}

function Get-MappedTargetNames {
    param([Parameter(Mandatory = $true)][string]$Path)

    @(
        Get-IscsiServerTarget -ComputerName $IscsiTargetComputerName -ErrorAction SilentlyContinue |
            ForEach-Object {
                $targetName = $_.TargetName
                @($_.LunMappings) |
                    Where-Object { $_.Path -eq $Path } |
                    ForEach-Object { $targetName }
            }
    )
}

function Remove-TrackedVirtualDisk {
    param([Parameter(Mandatory = $true)][string]$Path)

    try {
        $virtualDisk = Get-IscsiVirtualDisk -ComputerName $IscsiTargetComputerName -Path $Path -ErrorAction SilentlyContinue
        if ($null -ne $virtualDisk) {
            foreach ($mappedTarget in @(Get-MappedTargetNames -Path $Path)) {
                try {
                    Remove-IscsiVirtualDiskTargetMapping `
                        -ComputerName $IscsiTargetComputerName `
                        -TargetName $mappedTarget `
                        -Path $Path `
                        -ErrorAction SilentlyContinue | Out-Null
                } catch {
                    Write-Warning "Could not remove mapping '$mappedTarget' from '$Path': $($_.Exception.Message)"
                }
            }

            Remove-IscsiVirtualDisk -ComputerName $IscsiTargetComputerName -Path $Path -ErrorAction SilentlyContinue | Out-Null
        }
    } catch {
        Write-Warning "Could not remove iSCSI virtual disk '$Path': $($_.Exception.Message)"
    }

    try {
        if (Test-Path -LiteralPath $Path) {
            Remove-Item -LiteralPath $Path -Force -ErrorAction SilentlyContinue
        }
    } catch {
        Write-Warning "Could not remove file '$Path': $($_.Exception.Message)"
    }

    [void]$createdVirtualDisks.Remove($Path)
}

function Remove-TrackedNfsShare {
    param([Parameter(Mandatory = $true)][string]$Name)

    try {
        if (Get-NfsShare -Name $Name -ErrorAction SilentlyContinue) {
            Remove-NfsShare -Name $Name -Confirm:$false -ErrorAction SilentlyContinue | Out-Null
        }
    } catch {
        Write-Warning "Could not remove NFS share '$Name': $($_.Exception.Message)"
    }

    [void]$createdNfsShares.Remove($Name)
}

function Remove-TrackedSmbShare {
    param([Parameter(Mandatory = $true)][string]$Name)

    try {
        if (Get-SmbShare -Name $Name -ErrorAction SilentlyContinue) {
            Remove-SmbShare -Name $Name -Force -ErrorAction SilentlyContinue | Out-Null
        }
    } catch {
        Write-Warning "Could not remove SMB share '$Name': $($_.Exception.Message)"
    }

    [void]$createdSmbShares.Remove($Name)
}

function Remove-TrackedShareDirectory {
    param([Parameter(Mandatory = $true)][string]$Path)

    try {
        if (Get-Command Remove-FsrmQuota -ErrorAction SilentlyContinue) {
            Remove-FsrmQuota -Path $Path -Confirm:$false -ErrorAction SilentlyContinue | Out-Null
        }
        if (Test-Path -LiteralPath $Path) {
            Remove-Item -LiteralPath $Path -Recurse -Force -ErrorAction SilentlyContinue
        }
    } catch {
        Write-Warning "Could not remove directory '$Path': $($_.Exception.Message)"
    }

    [void]$createdShareDirectories.Remove($Path)
}

function Invoke-Cleanup {
    if ((-not $createdTarget) -and
        $createdVirtualDisks.Count -eq 0 -and
        $createdSnapshots.Count -eq 0 -and
        $createdNfsShares.Count -eq 0 -and
        $createdSmbShares.Count -eq 0 -and
        $createdShareDirectories.Count -eq 0) {
        return
    }

    Write-Section "Cleanup"

    foreach ($snapshotID in @($createdSnapshots)) {
        try {
            Remove-IscsiVirtualDiskSnapshot -ComputerName $IscsiTargetComputerName -SnapshotId $snapshotID -ErrorAction SilentlyContinue | Out-Null
            Write-Ok "Removed snapshot '$snapshotID'."
        } catch {
            Write-Warning "Could not remove snapshot '$snapshotID': $($_.Exception.Message)"
        }
        [void]$createdSnapshots.Remove($snapshotID)
    }

    foreach ($path in @($createdVirtualDisks)) {
        Remove-TrackedVirtualDisk -Path $path
        Write-Ok "Removed virtual disk '$path'."
    }

    if ($createdTarget) {
        try {
            Remove-IscsiServerTarget -ComputerName $IscsiTargetComputerName -TargetName $TargetName -ErrorAction SilentlyContinue | Out-Null
            Write-Ok "Removed target '$TargetName'."
        } catch {
            Write-Warning "Could not remove target '$TargetName': $($_.Exception.Message)"
        }
        $script:createdTarget = $false
    }

    foreach ($shareName in @($createdNfsShares)) {
        Remove-TrackedNfsShare -Name $shareName
        Write-Ok "Removed NFS share '$shareName'."
    }

    foreach ($shareName in @($createdSmbShares)) {
        Remove-TrackedSmbShare -Name $shareName
        Write-Ok "Removed SMB share '$shareName'."
    }

    foreach ($path in @($createdShareDirectories)) {
        Remove-TrackedShareDirectory -Path $path
        Write-Ok "Removed share directory '$path'."
    }
}

function Write-IscsiTargetAccessDiagnostics {
    param(
        [Parameter(Mandatory = $true)]
        [System.Management.Automation.ErrorRecord]$AccessError
    )

    Write-Section "iSCSI Target Access Diagnostics"

    $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = New-Object Security.Principal.WindowsPrincipal($identity)
    $isAdmin = $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
    $os = Get-CimInstance -ClassName Win32_OperatingSystem -ErrorAction SilentlyContinue

    Write-Host "User:              $($identity.Name)"
    Write-Host "IsAdministrator:   $isAdmin"
    if ($null -ne $os) {
        Write-Host "OS:                $($os.Caption) $($os.Version) ProductType=$($os.ProductType)"
    }
    Write-Host "PowerShell:        $($PSVersionTable.PSVersion) ($($PSVersionTable.PSEdition))"
    Write-Host "64-bit process:    $([Environment]::Is64BitProcess)"
    Write-Host "64-bit OS:         $([Environment]::Is64BitOperatingSystem)"
    Write-Host "WinTarget status:  $((Get-Service -Name WinTarget -ErrorAction SilentlyContinue).Status)"
    Write-Host "iSCSI cmdlet host: $IscsiTargetComputerName"
    Write-Host "Error FQID:        $($AccessError.FullyQualifiedErrorId)"
    Write-Host "Error message:     $($AccessError.Exception.Message)"

    try {
        $wtClasses = @(Get-WmiObject -Namespace root\wmi -List -ErrorAction Stop |
            Where-Object { $_.Name -like "WT_*" } |
            Select-Object -ExpandProperty Name)
        Write-Host "root\wmi WT_* classes: $($wtClasses.Count)"
        if ($wtClasses.Count -gt 0) {
            $wtClasses | Sort-Object | Select-Object -First 20 | ForEach-Object {
                Write-Host "  $_"
            }
        }
    } catch {
        Write-Warning "Cannot enumerate root\wmi WT_* classes: $($_.Exception.Message)"
    }

    try {
        Get-WmiObject -Namespace root\wmi -Class WT_General -EnableAllPrivileges -ErrorAction Stop | Out-Null
        Write-Host "WT_General query:  OK"
    } catch {
        Write-Warning "WT_General query failed: $($_.Exception.Message)"
    }

    try {
        $repositoryStatus = (& winmgmt /verifyrepository) -join " "
        Write-Host "WMI repository:    $repositoryStatus"
    } catch {
        Write-Warning "Cannot verify WMI repository: $($_.Exception.Message)"
    }

    try {
        $recentEvents = @(Get-WinEvent `
            -LogName "Microsoft-Windows-IscsiTarget-Service/Admin" `
            -MaxEvents 5 `
            -ErrorAction Stop)
        if ($recentEvents.Count -gt 0) {
            Write-Host "Recent iSCSI Target service events:"
            $recentEvents | ForEach-Object {
                Write-Host "  [$($_.TimeCreated)] Id=$($_.Id) Level=$($_.LevelDisplayName) $($_.ProviderName)"
                Write-Host "    $($_.Message)"
            }
        }
    } catch {
        Write-Warning "Cannot read Microsoft-Windows-IscsiTarget-Service/Admin events: $($_.Exception.Message)"
    }
}

try {
    Write-Section "Features"
    Assert-WindowsFeatureInstalled -Name "FS-FileServer"
    Assert-WindowsFeatureInstalled -Name "Storage-Services"
    Assert-WindowsFeatureInstalled -Name "FS-iSCSITarget-Server"
    Assert-WindowsFeatureInstalled -Name "FS-Resource-Manager"
    Assert-WindowsFeatureInstalled -Name "MSiSCSI"
    if (-not $SkipNfsChecks.IsPresent) {
        Assert-WindowsFeatureInstalled -Name "FS-NFS-Service"
        Assert-WindowsFeatureInstalled -Name "RSAT-NFS-Admin"
    }

    Write-Section "Services"
    Assert-ServiceRunning -Name "WinTarget"
    Assert-ServiceRunning -Name "MSiSCSI"
    if (-not $SkipSmbChecks.IsPresent) {
        Assert-ServiceRunning -Name "LanmanServer"
    }
    if (-not $SkipNfsChecks.IsPresent) {
        Assert-ServiceRunning -Name "NfsService"
    }
    if (-not $SkipWinRMChecks.IsPresent) {
        Assert-ServiceRunning -Name "WinRM"
    }

    Write-Section "PowerShell Cmdlets"
    Import-Module IscsiTarget -ErrorAction Stop
    $requiredIscsiCmdlets = @(
        "Get-IscsiTargetServerSetting",
        "Get-IscsiServerTarget",
        "New-IscsiServerTarget",
        "Set-IscsiServerTarget",
        "Remove-IscsiServerTarget",
        "Get-IscsiVirtualDisk",
        "New-IscsiVirtualDisk",
        "Resize-IscsiVirtualDisk",
        "Remove-IscsiVirtualDisk",
        "Add-IscsiVirtualDiskTargetMapping",
        "Remove-IscsiVirtualDiskTargetMapping",
        "Checkpoint-IscsiVirtualDisk",
        "Get-IscsiVirtualDiskSnapshot",
        "Export-IscsiVirtualDiskSnapshot",
        "Remove-IscsiVirtualDiskSnapshot"
    )
    foreach ($cmdlet in $requiredIscsiCmdlets) {
        Assert-CommandAvailable -Name $cmdlet
    }
    Write-Ok "All required iSCSI Target cmdlets are available."

    Import-Module FileServerResourceManager -ErrorAction Stop
    $requiredFsrmCmdlets = @(
        "Get-FsrmQuota",
        "New-FsrmQuota",
        "Set-FsrmQuota",
        "Remove-FsrmQuota"
    )
    foreach ($cmdlet in $requiredFsrmCmdlets) {
        Assert-CommandAvailable -Name $cmdlet
    }
    Write-Ok "All required File Server Resource Manager quota cmdlets are available."

    if (-not $SkipNfsChecks.IsPresent) {
        Import-Module NFS -ErrorAction Stop
        $requiredNfsCmdlets = @(
            "Get-NfsShare",
            "New-NfsShare",
            "Grant-NfsSharePermission",
            "Remove-NfsShare"
        )
        foreach ($cmdlet in $requiredNfsCmdlets) {
            Assert-CommandAvailable -Name $cmdlet
        }
        Write-Ok "All required NFS cmdlets are available."
    }

    if (-not $SkipSmbChecks.IsPresent) {
        Import-Module SmbShare -ErrorAction Stop
        $requiredSmbCmdlets = @(
            "Get-SmbShare",
            "New-SmbShare",
            "Grant-SmbShareAccess",
            "Get-SmbShareAccess",
            "Remove-SmbShare"
        )
        foreach ($cmdlet in $requiredSmbCmdlets) {
            Assert-CommandAvailable -Name $cmdlet
        }
        Write-Ok "All required SMB cmdlets are available."
    }

    Write-Section "iSCSI Target Access"
    try {
        $settings = Get-IscsiTargetServerSetting -ComputerName $IscsiTargetComputerName -ErrorAction Stop
    } catch {
        Write-IscsiTargetAccessDiagnostics -AccessError $_
        throw @"
The iSCSI Target PowerShell module is installed, but the current session cannot
access the iSCSI Target WMI provider through -ComputerName '$IscsiTargetComputerName'.

This is the same provider path used by New-IscsiServerTarget and
New-IscsiVirtualDisk, so the CSI integration lifecycle tests cannot run until
Get-IscsiTargetServerSetting -ComputerName '$IscsiTargetComputerName' succeeds in
an elevated Windows PowerShell 5.1 session.

Most likely causes are a damaged iSCSI Target/WMI provider registration,
root\wmi namespace permissions, or a broken WMI repository.

Suggested repair sequence from an elevated Windows PowerShell 5.1 session:

  winmgmt /verifyrepository
  winmgmt /salvagerepository
  Restart-Service Winmgmt -Force
  Restart-Service WinTarget

If it still fails, remove and reinstall the iSCSI Target role:

  Uninstall-WindowsFeature FS-iSCSITarget-Server -Restart
  Install-WindowsFeature FS-iSCSITarget-Server -IncludeManagementTools

Then rerun this validator.
"@
    }
    $settings | Format-List ComputerName, Version, Portals
    Write-Ok "Current PowerShell session can access iSCSI Target server settings."

    if (-not $SkipWinRMChecks.IsPresent) {
        Write-Section "WinRM"
        $httpsListeners = @(
            Get-ChildItem WSMan:\LocalHost\Listener -ErrorAction SilentlyContinue |
                Where-Object { $_.Keys -contains "Transport=HTTPS" }
        )
        if ($httpsListeners.Count -eq 0) {
            throw "No WinRM HTTPS listener was found."
        }
        Write-Ok "WinRM HTTPS listener exists."

        $basicAuth = (Get-Item WSMan:\LocalHost\Service\Auth\Basic).Value
        $allowUnencrypted = (Get-Item WSMan:\LocalHost\Service\AllowUnencrypted).Value
        Write-Host "WinRM Basic auth: $basicAuth"
        Write-Host "WinRM AllowUnencrypted: $allowUnencrypted"
    }

    Write-Section "Storage Path"
    New-Item -ItemType Directory -Force -Path $StoragePath | Out-Null
    $probeFile = Join-Path -Path $StoragePath -ChildPath "$DiskName.probe"
    "probe" | Set-Content -LiteralPath $probeFile -Encoding ASCII
    Remove-Item -LiteralPath $probeFile -Force
    Write-Ok "Storage path '$StoragePath' is writable."

    if (-not $SkipNfsChecks.IsPresent -or -not $SkipSmbChecks.IsPresent) {
        Write-Section "Share Path"
        New-Item -ItemType Directory -Force -Path $SharePath | Out-Null
        $shareProbeFile = Join-Path -Path $SharePath -ChildPath "$DiskName.share-probe"
        "probe" | Set-Content -LiteralPath $shareProbeFile -Encoding ASCII
        Remove-Item -LiteralPath $shareProbeFile -Force
        Write-Ok "Share path '$SharePath' is writable."
    }

    if (Test-Path -LiteralPath $diskPath) {
        throw "Test disk path already exists: $diskPath"
    }

    Write-Section "Disposable iSCSI Lifecycle"
    Write-Host "TargetName: $TargetName"
    Write-Host "DiskPath:   $diskPath"

    New-IscsiServerTarget -ComputerName $IscsiTargetComputerName -TargetName $TargetName | Out-Null
    $createdTarget = $true
    Write-Ok "Created target."

    $normalizedInitiators = @($InitiatorIds | ForEach-Object { Normalize-InitiatorId -Value $_ })
    Set-IscsiServerTarget -ComputerName $IscsiTargetComputerName -TargetName $TargetName -InitiatorIds $normalizedInitiators | Out-Null
    $target = Get-IscsiServerTarget -ComputerName $IscsiTargetComputerName -TargetName $TargetName
    Write-Ok "Assigned initiator IDs: $($normalizedInitiators -join ', ')"

    New-IscsiVirtualDisk -ComputerName $IscsiTargetComputerName -Path $diskPath -SizeBytes $InitialSizeBytes | Out-Null
    [void]$createdVirtualDisks.Add($diskPath)
    $disk = Get-IscsiVirtualDisk -ComputerName $IscsiTargetComputerName -Path $diskPath
    if ([int64]$disk.Size -lt $InitialSizeBytes) {
        throw "Created disk size $($disk.Size) is smaller than expected $InitialSizeBytes."
    }
    Write-Ok "Created virtual disk."

    Add-IscsiVirtualDiskTargetMapping -ComputerName $IscsiTargetComputerName -TargetName $TargetName -Path $diskPath | Out-Null
    if (@(Get-MappedTargetNames -Path $diskPath) -notcontains $TargetName) {
        throw "Virtual disk is not mapped to target '$TargetName'."
    }
    Write-Ok "Mapped virtual disk to target."

    Resize-IscsiVirtualDisk -ComputerName $IscsiTargetComputerName -Path $diskPath -SizeBytes $ResizedSizeBytes | Out-Null
    $disk = Get-IscsiVirtualDisk -ComputerName $IscsiTargetComputerName -Path $diskPath
    if ([int64]$disk.Size -lt $ResizedSizeBytes) {
        throw "Resized disk size $($disk.Size) is smaller than expected $ResizedSizeBytes."
    }
    Write-Ok "Resized virtual disk."

    Checkpoint-IscsiVirtualDisk -ComputerName $IscsiTargetComputerName -OriginalPath $diskPath -Description "CSI validation $runID" | Out-Null
    $snapshot = Get-IscsiVirtualDiskSnapshot -ComputerName $IscsiTargetComputerName -OriginalPath $diskPath |
        Sort-Object CreationTime -Descending |
        Select-Object -First 1
    if ($null -eq $snapshot) {
        throw "Snapshot was not created."
    }
    $snapshotID = $snapshot.SnapshotId.ToString()
    [void]$createdSnapshots.Add($snapshotID)
    Write-Ok "Created snapshot '$snapshotID'."

    $exported = Export-IscsiVirtualDiskSnapshot -ComputerName $IscsiTargetComputerName -SnapshotId $snapshotID
    if ($null -eq $exported -or [string]::IsNullOrWhiteSpace($exported.Path)) {
        throw "Snapshot export did not return a virtual disk path."
    }
    $exportedPath = $exported.Path
    [void]$createdVirtualDisks.Add($exportedPath)
    Get-IscsiVirtualDisk -ComputerName $IscsiTargetComputerName -Path $exportedPath | Out-Null
    Write-Ok "Exported snapshot as '$exportedPath'."

    Remove-IscsiVirtualDiskSnapshot -ComputerName $IscsiTargetComputerName -SnapshotId $snapshotID | Out-Null
    [void]$createdSnapshots.Remove($snapshotID)
    Write-Ok "Deleted snapshot."

    Remove-TrackedVirtualDisk -Path $exportedPath
    Write-Ok "Deleted exported virtual disk."

    Remove-IscsiVirtualDiskTargetMapping -ComputerName $IscsiTargetComputerName -TargetName $TargetName -Path $diskPath -ErrorAction SilentlyContinue | Out-Null
    if (@(Get-MappedTargetNames -Path $diskPath) -contains $TargetName) {
        throw "Virtual disk is still mapped to target '$TargetName'."
    }
    Write-Ok "Unmapped virtual disk from target."

    Remove-TrackedVirtualDisk -Path $diskPath
    Write-Ok "Deleted primary virtual disk."

    Remove-IscsiServerTarget -ComputerName $IscsiTargetComputerName -TargetName $TargetName | Out-Null
    $createdTarget = $false
    Write-Ok "Deleted target."

    if (-not $SkipNfsChecks.IsPresent) {
        Write-Section "Disposable NFS Lifecycle"
        Write-Host "ShareName: $NfsShareName"
        Write-Host "SharePath: $nfsSharePath"

        if (Test-Path -LiteralPath $nfsSharePath) {
            throw "Test NFS share path already exists: $nfsSharePath"
        }
        if (Get-NfsShare -Name $NfsShareName -ErrorAction SilentlyContinue) {
            throw "Test NFS share already exists: $NfsShareName"
        }

        New-Item -ItemType Directory -Force -Path $nfsSharePath | Out-Null
        [void]$createdShareDirectories.Add($nfsSharePath)
        Write-Ok "Created NFS backing directory."

        New-FsrmQuota -Path $nfsSharePath -Size $InitialSizeBytes | Out-Null
        $nfsQuota = Get-FsrmQuota -Path $nfsSharePath -ErrorAction Stop
        if ([int64]$nfsQuota.Size -ne $InitialSizeBytes) {
            throw "NFS backing directory quota $($nfsQuota.Size) did not match expected $InitialSizeBytes."
        }
        Set-FsrmQuota -Path $nfsSharePath -Size $ResizedSizeBytes | Out-Null
        $nfsQuota = Get-FsrmQuota -Path $nfsSharePath -ErrorAction Stop
        if ([int64]$nfsQuota.Size -ne $ResizedSizeBytes) {
            throw "Resized NFS backing directory quota $($nfsQuota.Size) did not match expected $ResizedSizeBytes."
        }
        Write-Ok "Created and resized NFS backing directory quota."

        New-NfsShare -Name $NfsShareName -Path $nfsSharePath -Permission "readwrite" -AllowRootAccess $true | Out-Null
        [void]$createdNfsShares.Add($NfsShareName)
        Write-Ok "Created NFS share."

        foreach ($clientName in $NfsClientNames) {
            Grant-NfsSharePermission `
                -Name $NfsShareName `
                -ClientName $clientName `
                -ClientType "host" `
                -Permission "readwrite" `
                -AllowRootAccess $true `
                -ErrorAction SilentlyContinue | Out-Null
        }
        Write-Ok "Granted NFS client permissions: $($NfsClientNames -join ', ')"

        $nfsShare = Get-NfsShare -Name $NfsShareName -ErrorAction Stop | Select-Object -First 1
        if ($null -eq $nfsShare) {
            throw "NFS share '$NfsShareName' was not found after creation."
        }
        if ($nfsShare.Path -ne $nfsSharePath) {
            throw "NFS share '$NfsShareName' path '$($nfsShare.Path)' did not match expected '$nfsSharePath'."
        }
        Write-Ok "Read back NFS share metadata."

        Remove-TrackedNfsShare -Name $NfsShareName
        Write-Ok "Deleted NFS share."

        Remove-TrackedShareDirectory -Path $nfsSharePath
        Write-Ok "Deleted NFS backing directory."
    }

    if (-not $SkipSmbChecks.IsPresent) {
        Write-Section "Disposable SMB Lifecycle"
        Write-Host "ShareName: $SmbShareName"
        Write-Host "SharePath: $smbSharePath"

        if (Test-Path -LiteralPath $smbSharePath) {
            throw "Test SMB share path already exists: $smbSharePath"
        }
        if (Get-SmbShare -Name $SmbShareName -ErrorAction SilentlyContinue) {
            throw "Test SMB share already exists: $SmbShareName"
        }

        New-Item -ItemType Directory -Force -Path $smbSharePath | Out-Null
        [void]$createdShareDirectories.Add($smbSharePath)
        Write-Ok "Created SMB backing directory."

        New-FsrmQuota -Path $smbSharePath -Size $InitialSizeBytes | Out-Null
        $smbQuota = Get-FsrmQuota -Path $smbSharePath -ErrorAction Stop
        if ([int64]$smbQuota.Size -ne $InitialSizeBytes) {
            throw "SMB backing directory quota $($smbQuota.Size) did not match expected $InitialSizeBytes."
        }
        Set-FsrmQuota -Path $smbSharePath -Size $ResizedSizeBytes | Out-Null
        $smbQuota = Get-FsrmQuota -Path $smbSharePath -ErrorAction Stop
        if ([int64]$smbQuota.Size -ne $ResizedSizeBytes) {
            throw "Resized SMB backing directory quota $($smbQuota.Size) did not match expected $ResizedSizeBytes."
        }
        Write-Ok "Created and resized SMB backing directory quota."

        New-SmbShare -Name $SmbShareName -Path $smbSharePath -ChangeAccess $SmbChangeAccess | Out-Null
        [void]$createdSmbShares.Add($SmbShareName)
        Write-Ok "Created SMB share."

        $smbShare = Get-SmbShare -Name $SmbShareName -ErrorAction Stop | Select-Object -First 1
        if ($null -eq $smbShare) {
            throw "SMB share '$SmbShareName' was not found after creation."
        }
        if ($smbShare.Path -ne $smbSharePath) {
            throw "SMB share '$SmbShareName' path '$($smbShare.Path)' did not match expected '$smbSharePath'."
        }

        $smbAccess = @(Get-SmbShareAccess -Name $SmbShareName -ErrorAction Stop)
        if ($smbAccess.Count -eq 0) {
            throw "SMB share '$SmbShareName' did not return any share access entries."
        }
        Write-Ok "Read back SMB share metadata and access entries."

        Remove-TrackedSmbShare -Name $SmbShareName
        Write-Ok "Deleted SMB share."

        Remove-TrackedShareDirectory -Path $smbSharePath
        Write-Ok "Deleted SMB backing directory."
    }

    Write-Section "Result"
    Write-Host "Windows machine validation completed successfully." -ForegroundColor Green
} finally {
    if ($KeepResources.IsPresent) {
        Write-Warning "KeepResources was set. Test resources were not cleaned up automatically."
        Write-Warning "TargetName: $TargetName"
        Write-Warning "VirtualDisks: $($createdVirtualDisks -join ', ')"
        Write-Warning "Snapshots: $($createdSnapshots -join ', ')"
        Write-Warning "NfsShares: $($createdNfsShares -join ', ')"
        Write-Warning "SmbShares: $($createdSmbShares -join ', ')"
        Write-Warning "ShareDirectories: $($createdShareDirectories -join ', ')"
    } else {
        Invoke-Cleanup
    }
}
