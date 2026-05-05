#Requires -RunAsAdministrator
<#
.SYNOPSIS
Prepares a Windows Server as the iSCSI, NFS, SMB, and WinRM backend for these CSI drivers.

.DESCRIPTION
Installs the Windows Server features required for iSCSI target storage, NFS
exports, and SMB shares. The script creates the VHDX and file-share backing
directories, configures a local administrator account for WinRM, enables WinRM
over HTTPS with a self-signed certificate, and opens firewall rules for WinRM
HTTPS, iSCSI target traffic, SMB, and NFS.

.EXAMPLE
.\deploy\install-windows-machine.ps1 `
  -AllowedClient 203.0.113.10/32 `
  -IscsiTargetRemoteAddress 10.244.0.0/16 `
  -SmbRemoteAddress 10.244.0.0/16 `
  -NfsRemoteAddress 10.244.0.0/16 `
  -StoragePath E:\iSCSIVirtualDisks `
  -SharePath E:\CSIFileShares `
  -CertDnsName storage01.example.com `
  -EnableNfsKerberos `
  -NfsKerberosFlavor krb5p

.NOTES
Run this script in an elevated PowerShell session on the Windows Server that
will host the iSCSI target, NFS exports, and SMB shares. Use "Any" for firewall
remote addresses only in an isolated lab.
#>

[CmdletBinding()]
param(
    [Parameter()]
    [ValidateNotNullOrEmpty()]
    [string]$AllowedClient = "Any",

    [Parameter()]
    [ValidateNotNullOrEmpty()]
    [string]$IscsiTargetRemoteAddress = "Any",

    [Parameter()]
    [ValidateNotNullOrEmpty()]
    [string]$SmbRemoteAddress = "Any",

    [Parameter()]
    [ValidateNotNullOrEmpty()]
    [string]$NfsRemoteAddress = "Any",

    [Parameter()]
    [ValidateSet("krb5", "krb5i", "krb5p")]
    [string]$NfsKerberosFlavor = "krb5",

    [Parameter()]
    [ValidateNotNullOrEmpty()]
    [string]$WinRMUser = "csi-winrm-test",

    [Parameter()]
    [ValidateNotNullOrEmpty()]
    [string]$StoragePath = "C:\data\taliesins\csi-driver-for-windows-storage-server\vhdx",

    [Parameter()]
    [ValidateNotNullOrEmpty()]
    [string]$SharePath = "C:\data\taliesins\csi-driver-for-windows-storage-server\shares",

    [Parameter()]
    [ValidateNotNullOrEmpty()]
    [string]$CertDnsName = $env:COMPUTERNAME,

    [Parameter()]
    [securestring]$WinRMPassword,

    [Parameter()]
    [switch]$InstallMultipath,

    [Parameter()]
    [switch]$SkipWinRMUser,

    [Parameter()]
    [switch]$SkipWinRM,

    [Parameter()]
    [switch]$SkipWinRMBasicAuth,

    [Parameter()]
    [switch]$SkipIscsiFirewall,

    [Parameter()]
    [switch]$SkipSmb,

    [Parameter()]
    [switch]$SkipNfs,

    [Parameter()]
    [switch]$SkipSmbFirewall,

    [Parameter()]
    [switch]$SkipNfsFirewall,

    [Parameter()]
    [switch]$EnableNfsKerberos,

    [Parameter()]
    [switch]$SkipNfsKerberosDomainCheck,

    [Parameter()]
    [switch]$ForceNewCertificate,

    [Parameter()]
    [switch]$SkipChecks
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

function Assert-WindowsServerFeatureSupport {
    if (-not (Get-Command Install-WindowsFeature -ErrorAction SilentlyContinue)) {
        throw "Install-WindowsFeature was not found. Run this script on Windows Server with the ServerManager module available."
    }
}

function Install-FeatureIfAvailable {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Name,

        [Parameter()]
        [switch]$IncludeManagementTools
    )

    $feature = Get-WindowsFeature -Name $Name -ErrorAction SilentlyContinue
    if ($null -eq $feature) {
        Write-Warning "Windows feature '$Name' was not found on this host; skipping."
        return
    }

    if ($feature.InstallState -eq "Installed") {
        Write-Host "Windows feature '$Name' is already installed."
        return
    }

    $installParams = @{ Name = $Name }
    if ($IncludeManagementTools.IsPresent) {
        $installParams.IncludeManagementTools = $true
    }

    Write-Host "Installing Windows feature '$Name'..."
    Install-WindowsFeature @installParams | Format-Table DisplayName, Name, InstallState, Success
}

function Start-ServiceIfPresent {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Name
    )

    $service = Get-Service -Name $Name -ErrorAction SilentlyContinue
    if ($null -eq $service) {
        Write-Warning "Service '$Name' was not found; skipping."
        return
    }

    Set-Service -Name $Name -StartupType Automatic
    if ($service.Status -ne "Running") {
        Write-Host "Starting service '$Name'..."
        Start-Service -Name $Name
    } else {
        Write-Host "Service '$Name' is already running."
    }
}

function Get-AdministratorsGroupName {
    $group = Get-LocalGroup -SID "S-1-5-32-544" -ErrorAction SilentlyContinue
    if ($null -ne $group) {
        return $group.Name
    }

    return "Administrators"
}

function Ensure-LocalAdminUser {
    param(
        [Parameter(Mandatory = $true)]
        [string]$UserName,

        [Parameter(Mandatory = $true)]
        [securestring]$Password
    )

    $user = Get-LocalUser -Name $UserName -ErrorAction SilentlyContinue
    if ($null -eq $user) {
        Write-Host "Creating local WinRM user '$UserName'..."
        New-LocalUser -Name $UserName -Password $Password -PasswordNeverExpires | Out-Null
    } else {
        Write-Host "Updating local WinRM user '$UserName'..."
        Set-LocalUser -Name $UserName -Password $Password
        Enable-LocalUser -Name $UserName
    }

    $administratorsGroup = Get-AdministratorsGroupName
    $accountNames = @(
        "$env:COMPUTERNAME\$UserName",
        ".\$UserName",
        $UserName
    )
    $existingMembers = @(
        Get-LocalGroupMember -Group $administratorsGroup -ErrorAction SilentlyContinue |
            Select-Object -ExpandProperty Name
    )

    if (-not ($accountNames | Where-Object { $existingMembers -contains $_ })) {
        Write-Host "Adding '$UserName' to local '$administratorsGroup' group..."
        Add-LocalGroupMember -Group $administratorsGroup -Member $UserName
    } else {
        Write-Host "'$UserName' is already a member of local '$administratorsGroup' group."
    }
}

function Ensure-FirewallRule {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Name,

        [Parameter(Mandatory = $true)]
        [string]$DisplayName,

        [Parameter(Mandatory = $true)]
        [int]$LocalPort,

        [Parameter()]
        [ValidateSet("TCP", "UDP")]
        [string]$Protocol = "TCP",

        [Parameter(Mandatory = $true)]
        [string]$RemoteAddress
    )

    $rule = Get-NetFirewallRule -Name $Name -ErrorAction SilentlyContinue
    if ($null -eq $rule) {
        Write-Host "Creating firewall rule '$DisplayName'..."
        New-NetFirewallRule `
            -Name $Name `
            -DisplayName $DisplayName `
            -Direction Inbound `
            -Action Allow `
            -Protocol $Protocol `
            -LocalPort $LocalPort `
            -RemoteAddress $RemoteAddress | Out-Null
        return
    }

    Write-Host "Updating firewall rule '$DisplayName'..."
    Set-NetFirewallRule -Name $Name -Enabled True -Direction Inbound -Action Allow
    Get-NetFirewallPortFilter -AssociatedNetFirewallRule $rule |
        Set-NetFirewallPortFilter -Protocol $Protocol -LocalPort $LocalPort
    Get-NetFirewallAddressFilter -AssociatedNetFirewallRule $rule |
        Set-NetFirewallAddressFilter -RemoteAddress $RemoteAddress
}

function Ensure-WinRMHttpsListener {
    param(
        [Parameter(Mandatory = $true)]
        [string]$HostName,

        [Parameter(Mandatory = $true)]
        [bool]$ReplaceExisting
    )

    Import-Module Microsoft.WSMan.Management -ErrorAction Stop

    $httpsListeners = @(
        Get-ChildItem WSMan:\LocalHost\Listener -ErrorAction SilentlyContinue |
            Where-Object { $_.Keys -contains "Transport=HTTPS" }
    )

    if ($ReplaceExisting -and $httpsListeners.Count -gt 0) {
        Write-Host "Removing existing WinRM HTTPS listener(s)..."
        foreach ($listener in $httpsListeners) {
            Remove-Item -Path $listener.PSPath -Recurse -Force
        }
        $httpsListeners = @()
    }

    if ($httpsListeners.Count -gt 0) {
        Write-Host "WinRM HTTPS listener already exists."
        return
    }

    if (-not (Get-Command New-SelfSignedCertificate -ErrorAction SilentlyContinue)) {
        throw "New-SelfSignedCertificate was not found. Cannot create a WinRM HTTPS certificate."
    }

    Write-Host "Creating self-signed certificate for '$HostName'..."
    $cert = New-SelfSignedCertificate `
        -DnsName $HostName `
        -CertStoreLocation "Cert:\LocalMachine\My" `
        -NotAfter (Get-Date).AddYears(2)

    Write-Host "Creating WinRM HTTPS listener for '$HostName'..."
    New-Item `
        -Path WSMan:\LocalHost\Listener `
        -Transport HTTPS `
        -Address * `
        -CertificateThumbPrint $cert.Thumbprint `
        -HostName $HostName `
        -Force | Out-Null
}

function Enable-WinRMRemoting {
    $enableParams = @{ Force = $true }
    $enableCommand = Get-Command Enable-PSRemoting -ErrorAction Stop

    if ($enableCommand.Parameters.ContainsKey("SkipNetworkProfileCheck")) {
        $enableParams.SkipNetworkProfileCheck = $true
    }

    Enable-PSRemoting @enableParams
}

function Enable-WinRMBasicAuth {
    Import-Module Microsoft.WSMan.Management -ErrorAction Stop

    Write-Host "Enabling WinRM Basic authentication for HTTPS clients..."
    Set-Item -Path WSMan:\localhost\Service\Auth\Basic -Value $true

    Write-Host "Keeping WinRM unencrypted transport disabled..."
    Set-Item -Path WSMan:\localhost\Service\AllowUnencrypted -Value $false

    Write-Host "Setting WinRM shell memory limit..."
    Set-Item -Path WSMan:\localhost\Shell\MaxMemoryPerShellMB -Value 1024
}

function Assert-CommandParameter {
    param(
        [Parameter(Mandatory = $true)]
        [string]$CommandName,

        [Parameter(Mandatory = $true)]
        [string]$ParameterName
    )

    $command = Get-Command -Name $CommandName -ErrorAction SilentlyContinue
    if ($null -eq $command) {
        throw "Required command '$CommandName' was not found."
    }
    if (-not $command.Parameters.ContainsKey($ParameterName)) {
        throw "Command '$CommandName' does not support parameter '$ParameterName'. Update the Windows Server NFS feature before enabling NFS Kerberos."
    }
}

function Assert-NfsKerberosPrerequisites {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Flavor,

        [Parameter(Mandatory = $true)]
        [bool]$SkipDomainCheck
    )

    if ($SkipNfs.IsPresent) {
        throw "NFS Kerberos support requires NFS to be installed. Remove -SkipNfs or do not use -EnableNfsKerberos."
    }

    Import-Module NFS -ErrorAction Stop
    Assert-CommandParameter -CommandName "New-NfsShare" -ParameterName "Authentication"

    if (-not $SkipDomainCheck) {
        $computerSystem = Get-CimInstance -ClassName Win32_ComputerSystem -ErrorAction Stop
        if (-not $computerSystem.PartOfDomain) {
            throw "NFS Kerberos requires this Windows Server to be domain joined. Use -SkipNfsKerberosDomainCheck only for isolated script validation."
        }
        Write-Host "NFS Kerberos domain check passed: $($computerSystem.Domain)"
    }

    Write-Host "NFS Kerberos prerequisites are available for flavor '$Flavor'."
    Write-Host "Ensure the storage server has the required NFS service principal/SPN and that Linux nodes have Kerberos client config and credentials."
}

if ($AllowedClient -eq "Any") {
    Write-Warning "AllowedClient is 'Any'. Restrict WinRM HTTPS to your dev/CI source address outside an isolated lab."
}

if (-not $SkipIscsiFirewall -and $IscsiTargetRemoteAddress -eq "Any") {
    Write-Warning "IscsiTargetRemoteAddress is 'Any'. Restrict iSCSI target access to cluster node addresses outside an isolated lab."
}

if (-not $SkipSmb.IsPresent -and -not $SkipSmbFirewall.IsPresent -and $SmbRemoteAddress -eq "Any") {
    Write-Warning "SmbRemoteAddress is 'Any'. Restrict SMB access to cluster node addresses outside an isolated lab."
}

if (-not $SkipNfs.IsPresent -and -not $SkipNfsFirewall.IsPresent -and $NfsRemoteAddress -eq "Any") {
    Write-Warning "NfsRemoteAddress is 'Any'. Restrict NFS access to cluster node addresses outside an isolated lab."
}

Assert-WindowsServerFeatureSupport
Import-Module ServerManager -ErrorAction Stop

$coreFeatures = @(
    "FS-FileServer",
    "Storage-Services",
    "FS-iSCSITarget-Server",
    "FS-Resource-Manager"
)

foreach ($featureName in $coreFeatures) {
    Install-FeatureIfAvailable -Name $featureName -IncludeManagementTools
}

if (-not $SkipNfs.IsPresent) {
    Install-FeatureIfAvailable -Name "FS-NFS-Service" -IncludeManagementTools
    Install-FeatureIfAvailable -Name "RSAT-NFS-Admin" -IncludeManagementTools
}

Install-FeatureIfAvailable -Name "MSiSCSI" -IncludeManagementTools

if ($InstallMultipath.IsPresent) {
    Install-FeatureIfAvailable -Name "Multipath-IO" -IncludeManagementTools
}

Import-Module IscsiTarget -ErrorAction Stop
Import-Module FileServerResourceManager -ErrorAction Stop
if (-not $SkipNfs.IsPresent) {
    Import-Module NFS -ErrorAction Stop
}
if (-not $SkipSmb.IsPresent) {
    Import-Module SmbShare -ErrorAction Stop
}

Write-Host "Ensuring VHDX storage directory '$StoragePath' exists..."
New-Item -ItemType Directory -Force -Path $StoragePath | Out-Null
$resolvedStoragePath = (Get-Item -LiteralPath $StoragePath).FullName
Write-Host "Setting machine CSI_VHDX_PARENT_PATH to '$resolvedStoragePath'..."
[Environment]::SetEnvironmentVariable("CSI_VHDX_PARENT_PATH", $resolvedStoragePath, "Machine")
$env:CSI_VHDX_PARENT_PATH = $resolvedStoragePath

Write-Host "Ensuring file-share backing directory '$SharePath' exists..."
New-Item -ItemType Directory -Force -Path $SharePath | Out-Null

Start-ServiceIfPresent -Name "WinTarget"
Start-ServiceIfPresent -Name "MSiSCSI"
if (-not $SkipSmb.IsPresent) {
    Start-ServiceIfPresent -Name "LanmanServer"
}
if (-not $SkipNfs.IsPresent) {
    Start-ServiceIfPresent -Name "NfsService"
}

if (-not $SkipWinRMUser) {
    if ($null -eq $WinRMPassword) {
        $WinRMPassword = Read-Host "Password for $WinRMUser" -AsSecureString
    }

    Ensure-LocalAdminUser -UserName $WinRMUser -Password $WinRMPassword

    Write-Host "Enabling full remote admin token for local SAM administrator accounts..."
    New-ItemProperty `
        -Path "HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System" `
        -Name "LocalAccountTokenFilterPolicy" `
        -PropertyType DWord `
        -Value 1 `
        -Force | Out-Null
}

if (-not $SkipWinRM) {
    Write-Host "Enabling PowerShell Remoting and WinRM..."
    Enable-WinRMRemoting
    Set-Service -Name WinRM -StartupType Automatic
    Start-Service -Name WinRM

    if (-not $SkipWinRMBasicAuth) {
        Enable-WinRMBasicAuth
    }

    Ensure-WinRMHttpsListener -HostName $CertDnsName -ReplaceExisting:$ForceNewCertificate.IsPresent

    Ensure-FirewallRule `
        -Name "CSI-WinRM-HTTPS-5986" `
        -DisplayName "WinRM HTTPS 5986 - CSI integration tests" `
        -LocalPort 5986 `
        -RemoteAddress $AllowedClient
}

if (-not $SkipIscsiFirewall) {
    Ensure-FirewallRule `
        -Name "CSI-iSCSI-Target-3260" `
        -DisplayName "iSCSI Target 3260 - CSI driver" `
        -LocalPort 3260 `
        -RemoteAddress $IscsiTargetRemoteAddress
}

if (-not $SkipSmb.IsPresent -and -not $SkipSmbFirewall.IsPresent) {
    Ensure-FirewallRule `
        -Name "CSI-SMB-445" `
        -DisplayName "SMB 445 - CSI driver" `
        -LocalPort 445 `
        -RemoteAddress $SmbRemoteAddress
}

if (-not $SkipNfs.IsPresent -and -not $SkipNfsFirewall.IsPresent) {
    Ensure-FirewallRule `
        -Name "CSI-NFS-2049-TCP" `
        -DisplayName "NFS 2049 TCP - CSI driver" `
        -LocalPort 2049 `
        -Protocol TCP `
        -RemoteAddress $NfsRemoteAddress

    Ensure-FirewallRule `
        -Name "CSI-NFS-2049-UDP" `
        -DisplayName "NFS 2049 UDP - CSI driver" `
        -LocalPort 2049 `
        -Protocol UDP `
        -RemoteAddress $NfsRemoteAddress

    Ensure-FirewallRule `
        -Name "CSI-NFS-111-TCP" `
        -DisplayName "NFS RPC portmapper 111 TCP - CSI driver" `
        -LocalPort 111 `
        -Protocol TCP `
        -RemoteAddress $NfsRemoteAddress

    Ensure-FirewallRule `
        -Name "CSI-NFS-111-UDP" `
        -DisplayName "NFS RPC portmapper 111 UDP - CSI driver" `
        -LocalPort 111 `
        -Protocol UDP `
        -RemoteAddress $NfsRemoteAddress
}

if ($EnableNfsKerberos.IsPresent) {
    Assert-NfsKerberosPrerequisites `
        -Flavor $NfsKerberosFlavor `
        -SkipDomainCheck:$SkipNfsKerberosDomainCheck.IsPresent
}

if (-not $SkipChecks) {
    Write-Host ""
    Write-Host "Installed Windows features:"
    $featureNames = @(
        "FS-FileServer",
        "Storage-Services",
        "FS-iSCSITarget-Server",
        "FS-Resource-Manager",
        "FS-NFS-Service",
        "RSAT-NFS-Admin",
        "MSiSCSI",
        "Multipath-IO"
    )
    Get-WindowsFeature -Name $featureNames -ErrorAction SilentlyContinue |
        Format-Table DisplayName, Name, InstallState

    if (-not $SkipWinRM) {
        Write-Host ""
        Write-Host "WinRM listeners:"
        Get-ChildItem WSMan:\LocalHost\Listener

        Write-Host ""
        Write-Host "Testing local WinRM HTTPS endpoint:"
        try {
            Test-WSMan -ComputerName $CertDnsName -UseSSL -ErrorAction Stop
        } catch {
            Write-Warning "Local WinRM HTTPS check failed: $($_.Exception.Message)"
            Write-Warning "If this host uses the self-signed certificate created by this script, external clients should use WINRM_INSECURE=true."
        }
    }
}

Write-Host ""
Write-Host "Windows machine setup complete."
Write-Host "Use these values from the development or controller environment:"
Write-Host "  WINRM_HOST=$CertDnsName"
Write-Host "  WINRM_USER=$WinRMUser"
Write-Host "  WINRM_PORT=5986"
Write-Host "  WINRM_TLS=true"
Write-Host "  WINRM_INSECURE=true"
Write-Host "  CSI_VHDX_PARENT_PATH=$resolvedStoragePath"
Write-Host "  WINRM_TEST_PARENT_DIR=$StoragePath"
Write-Host "  WINRM_TEST_SHARE_DIR=$SharePath"
if ($EnableNfsKerberos.IsPresent) {
    Write-Host "  NFS_KERBEROS_AUTHENTICATION=$NfsKerberosFlavor"
    Write-Host "  StorageClass nfsAuthentication=$NfsKerberosFlavor"
    Write-Host "  StorageClass nfsMountAuthentication=$NfsKerberosFlavor"
}
