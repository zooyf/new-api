<#
.SYNOPSIS
Change the production new-api blue/green traffic split on nexus-sg.

.DESCRIPTION
This script rewrites the managed Nginx upstream file and reloads Nginx. It does
not start, stop, or recreate containers. The blue slot is expected on
127.0.0.1:3000 and the green slot on 127.0.0.1:3002.

Examples:
  ./scripts/set-nexus-canary.ps1 -GreenWeight 1
  ./scripts/set-nexus-canary.ps1 -GreenWeight 50
  ./scripts/set-nexus-canary.ps1 -Promote
  ./scripts/set-nexus-canary.ps1 -Rollback
#>

[CmdletBinding()]
param(
    [string]$RemoteHost = "nexus-sg",
    [string]$UpstreamConf = "/etc/nginx/conf.d/new-api-upstream.conf",
    [int]$BluePort = 3000,
    [int]$GreenPort = 3002,
    [ValidateRange(0, 100)][int]$GreenWeight = 0,
    [switch]$Rollback,
    [switch]$Promote,
    [switch]$SkipHealthCheck,
    [switch]$DryRun,
    [switch]$Yes
)

$ErrorActionPreference = "Stop"

function Invoke-RemoteScript {
    param([Parameter(Mandatory = $true)][string]$Script)

    ($Script -replace "`r`n", "`n") | & ssh $RemoteHost "bash -s"
    if ($LASTEXITCODE -ne 0) {
        throw "Remote command failed on $RemoteHost."
    }
}

function ConvertTo-Base64 {
    param([Parameter(Mandatory = $true)][string]$Value)
    return [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($Value))
}

if ($Rollback -and $Promote) {
    throw "-Rollback and -Promote cannot be used together."
}
if ($Rollback) {
    $GreenWeight = 0
}
if ($Promote) {
    $GreenWeight = 100
}

$blueWeight = 100 - $GreenWeight
$blueLine = if ($blueWeight -gt 0) {
    "    server 127.0.0.1:${BluePort} weight=$blueWeight max_fails=1 fail_timeout=10s;"
} else {
    "    server 127.0.0.1:${BluePort} down;"
}
$greenLine = if ($GreenWeight -gt 0) {
    "    server 127.0.0.1:${GreenPort} weight=$GreenWeight max_fails=1 fail_timeout=10s;"
} else {
    "    server 127.0.0.1:${GreenPort} down;"
}

$upstreamConfig = @"
# Managed by scripts/set-nexus-canary.ps1.
upstream new_api_backend {
    zone new_api_backend 64k;
    # Keep one client on one slot so HTML and hashed frontend assets cannot mix.
    hash `$remote_addr consistent;
$blueLine
$greenLine
    keepalive 32;
}
"@

if ($DryRun) {
    Write-Host $upstreamConfig
    return
}

if (-not $Yes) {
    Write-Host "Target: $RemoteHost"
    Write-Host "Traffic split: blue=$blueWeight green=$GreenWeight"
    Write-Host "Nginx upstream: $UpstreamConf"
    $answer = Read-Host "Type CANARY to continue"
    if ($answer -ne "CANARY") {
        throw "Canary update cancelled."
    }
}

$configB64 = ConvertTo-Base64 $upstreamConfig
$skipHealthValue = if ($SkipHealthCheck) { "1" } else { "0" }

$remoteScript = @"
set -Eeuo pipefail

upstream_conf="$UpstreamConf"
config_b64="$configB64"
blue_port="$BluePort"
green_port="$GreenPort"
blue_weight="$blueWeight"
green_weight="$GreenWeight"
skip_health="$skipHealthValue"
backup_dir="/etc/nginx/backups/new-api-canary-`$(date -u +%Y%m%dT%H%M%SZ)"

check_backend() {
    name="`$1"
    port="`$2"
    curl -fsS "http://127.0.0.1:`$port/api/status" 2>/dev/null | grep -q '"success":true' || {
        echo "Backend `$name on port `$port is not healthy." >&2
        return 1
    }
}

if [ "`$skip_health" != "1" ]; then
    if [ "`$blue_weight" -gt 0 ]; then
        check_backend blue "`$blue_port"
    fi
    if [ "`$green_weight" -gt 0 ]; then
        check_backend green "`$green_port"
    fi
fi

tmp_conf="`$(mktemp)"
printf '%s' "`$config_b64" | base64 -d > "`$tmp_conf"

sudo mkdir -p "`$backup_dir"
if [ -f "`$upstream_conf" ]; then
    sudo cp "`$upstream_conf" "`$backup_dir/new-api-upstream.conf"
fi
sudo cp "`$tmp_conf" "`$upstream_conf"
rm -f "`$tmp_conf"

if ! sudo nginx -t; then
    if [ -f "`$backup_dir/new-api-upstream.conf" ]; then
        sudo cp "`$backup_dir/new-api-upstream.conf" "`$upstream_conf"
        sudo nginx -t || true
    fi
    exit 1
fi

sudo systemctl reload nginx || sudo nginx -s reload
echo "Canary traffic updated: blue=`$blue_weight green=`$green_weight"
"@

Invoke-RemoteScript $remoteScript
