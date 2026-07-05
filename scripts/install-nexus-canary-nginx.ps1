<#
.SYNOPSIS
Prepare production Nginx on nexus-sg for new-api blue/green canary traffic.

.DESCRIPTION
This script creates /etc/nginx/conf.d/new-api-upstream.conf and rewrites the
llm.ai.nexus-reach.com site from a direct proxy_pass to 127.0.0.1:3000 into
proxy_pass http://new_api_backend. The initial traffic split defaults to
100% blue and 0% green. It backs up changed files, runs nginx -t, and reloads
Nginx only after validation passes.
#>

[CmdletBinding()]
param(
    [string]$RemoteHost = "nexus-sg",
    [string]$SitePath = "/etc/nginx/sites-available/llm.ai.nexus-reach.com.conf",
    [string]$UpstreamConf = "/etc/nginx/conf.d/new-api-upstream.conf",
    [int]$BluePort = 3000,
    [int]$GreenPort = 3002,
    [ValidateRange(0, 100)][int]$GreenWeight = 0,
    [switch]$SkipHealthCheck,
    [switch]$SkipReload,
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
$blueLine
$greenLine
    keepalive 32;
}
"@

if ($DryRun) {
    Write-Host "Would install upstream config:"
    Write-Host $upstreamConfig
    Write-Host "Would replace proxy_pass http://127.0.0.1:${BluePort}; with proxy_pass http://new_api_backend; in $SitePath"
    return
}

if (-not $Yes) {
    Write-Host "Target: $RemoteHost"
    Write-Host "Site:   $SitePath"
    Write-Host "Upstream conf: $UpstreamConf"
    Write-Host "Initial split: blue=$blueWeight green=$GreenWeight"
    $answer = Read-Host "Type INSTALL to continue"
    if ($answer -ne "INSTALL") {
        throw "Nginx canary install cancelled."
    }
}

$configB64 = ConvertTo-Base64 $upstreamConfig
$skipHealthValue = if ($SkipHealthCheck) { "1" } else { "0" }
$skipReloadValue = if ($SkipReload) { "1" } else { "0" }

$remoteScript = @"
set -Eeuo pipefail

site_path="$SitePath"
upstream_conf="$UpstreamConf"
config_b64="$configB64"
blue_port="$BluePort"
green_port="$GreenPort"
blue_weight="$blueWeight"
green_weight="$GreenWeight"
skip_health="$skipHealthValue"
skip_reload="$skipReloadValue"
backup_dir="/etc/nginx/backups/new-api-canary-install-`$(date -u +%Y%m%dT%H%M%SZ)"

if [ ! -f "`$site_path" ]; then
    echo "Missing Nginx site: `$site_path" >&2
    exit 1
fi

if [ "`$skip_health" != "1" ]; then
    curl -fsS "http://127.0.0.1:`$blue_port/api/status" 2>/dev/null | grep -q '"success":true' || {
        echo "Blue backend on port `$blue_port is not healthy." >&2
        exit 1
    }
    if [ "`$green_weight" -gt 0 ]; then
        curl -fsS "http://127.0.0.1:`$green_port/api/status" 2>/dev/null | grep -q '"success":true' || {
            echo "Green backend on port `$green_port is not healthy." >&2
            exit 1
        }
    fi
fi

sudo mkdir -p "`$backup_dir"
sudo cp "`$site_path" "`$backup_dir/llm.ai.nexus-reach.com.conf"
if [ -f "`$upstream_conf" ]; then
    sudo cp "`$upstream_conf" "`$backup_dir/new-api-upstream.conf"
fi

tmp_conf="`$(mktemp)"
printf '%s' "`$config_b64" | base64 -d > "`$tmp_conf"
sudo cp "`$tmp_conf" "`$upstream_conf"
rm -f "`$tmp_conf"

if sudo grep -q 'proxy_pass http://new_api_backend;' "`$site_path"; then
    echo "Site already uses new_api_backend."
else
    if ! sudo grep -q "proxy_pass http://127\\.0\\.0\\.1:`$blue_port;" "`$site_path"; then
        echo "Could not find proxy_pass http://127.0.0.1:`$blue_port; in `$site_path" >&2
        exit 1
    fi
    sudo perl -0pi -e "s#proxy_pass http://127\\.0\\.0\\.1:`$blue_port;#proxy_pass http://new_api_backend;#g" "`$site_path"
fi

if ! sudo nginx -t; then
    sudo cp "`$backup_dir/llm.ai.nexus-reach.com.conf" "`$site_path"
    if [ -f "`$backup_dir/new-api-upstream.conf" ]; then
        sudo cp "`$backup_dir/new-api-upstream.conf" "`$upstream_conf"
    fi
    sudo nginx -t || true
    exit 1
fi

if [ "`$skip_reload" != "1" ]; then
    sudo systemctl reload nginx || sudo nginx -s reload
fi

echo "Nginx canary upstream installed. Backup: `$backup_dir"
"@

Invoke-RemoteScript $remoteScript
