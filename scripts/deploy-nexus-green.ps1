<#
.SYNOPSIS
Backward-compatible wrapper for deploying specifically to the Green slot.

.DESCRIPTION
New automation should call deploy-nexus-slot.ps1 with the default -Slot Auto.
This wrapper preserves existing commands while retaining the inactive-slot
safety check, so it refuses to overwrite Green while Green serves traffic.
#>

[CmdletBinding()]
param(
    [string]$RemoteHost = "nexus-sg",
    [string]$RemoteDir = "/opt/new-api",
    [string]$ActiveServiceName = "new-api",
    [string]$GreenServiceName = "new-api-green",
    [string]$ImageRepository = "zooyf/new-api",
    [string]$ImageTag = "",
    [string]$Platform = "linux/amd64",
    [int]$GreenHostPort = 3002,
    [int]$ContainerPort = 3000,
    [int]$HealthTimeoutSeconds = 180,
    [ValidateSet("Direct", "Batch")][string]$BatchUpdateMode = "Direct",
    [ValidateRange(1, 300)][int]$BatchUpdateInterval = 5,
    [ValidateRange(1, 365)][int]$BuildCacheRetentionDays = 7,
    [ValidatePattern('^[1-9][0-9]*(B|KB|MB|GB|TB)$')][string]$BuildCacheMaxUsedSpace = "40GB",
    [ValidatePattern('^[1-9][0-9]*(B|KB|MB|GB|TB)$')][string]$BuildCacheReservedSpace = "20GB",
    [switch]$SkipBuildCacheCleanup,
    [switch]$AllowDirty,
    [switch]$BuildOnRemote,
    [switch]$PreflightOnly,
    [switch]$KeepLocalImageTar,
    [switch]$Yes
)

$parameters = @{
    Slot                 = "Green"
    RemoteHost           = $RemoteHost
    RemoteDir            = $RemoteDir
    BlueServiceName      = $ActiveServiceName
    GreenServiceName     = $GreenServiceName
    ImageRepository      = $ImageRepository
    ImageTag             = $ImageTag
    Platform             = $Platform
    GreenHostPort        = $GreenHostPort
    ContainerPort        = $ContainerPort
    HealthTimeoutSeconds = $HealthTimeoutSeconds
    BatchUpdateMode      = $BatchUpdateMode
    BatchUpdateInterval  = $BatchUpdateInterval
    BuildCacheRetentionDays = $BuildCacheRetentionDays
    BuildCacheMaxUsedSpace  = $BuildCacheMaxUsedSpace
    BuildCacheReservedSpace = $BuildCacheReservedSpace
    SkipBuildCacheCleanup   = $SkipBuildCacheCleanup.IsPresent
    AllowDirty           = $AllowDirty.IsPresent
    BuildOnRemote        = $BuildOnRemote.IsPresent
    PreflightOnly        = $PreflightOnly.IsPresent
    KeepLocalImageTar    = $KeepLocalImageTar.IsPresent
    Yes                  = $Yes.IsPresent
}

& (Join-Path $PSScriptRoot "deploy-nexus-slot.ps1") @parameters
