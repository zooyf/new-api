<#
.SYNOPSIS
Deploy the current checkout to the inactive green slot on nexus-sg.

.DESCRIPTION
This script builds a Docker image from the current working tree, uploads it to
the production server, writes a green-slot Compose override, and recreates only
the new-api-green container on 127.0.0.1:3002. It does not change Nginx traffic.
Use scripts/set-nexus-canary.ps1 after health checks to shift traffic.
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
    [switch]$AllowDirty,
    [switch]$PreflightOnly,
    [switch]$KeepLocalImageTar,
    [switch]$Yes
)

$ErrorActionPreference = "Stop"

function Invoke-External {
    param(
        [Parameter(Mandatory = $true)][string]$FilePath,
        [Parameter(ValueFromRemainingArguments = $true)][string[]]$Arguments
    )

    & $FilePath @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "Command failed ($LASTEXITCODE): $FilePath $($Arguments -join ' ')"
    }
}

function Invoke-Remote {
    param([Parameter(Mandatory = $true)][string]$Command)
    Invoke-External ssh $RemoteHost $Command
}

function Invoke-RemoteScript {
    param([Parameter(Mandatory = $true)][string]$Script)

    ($Script -replace "`r`n", "`n") | & ssh $RemoteHost "bash -s"
    if ($LASTEXITCODE -ne 0) {
        throw "Remote command failed on $RemoteHost."
    }
}

function Get-CommandOutput {
    param(
        [Parameter(Mandatory = $true)][string]$FilePath,
        [Parameter(ValueFromRemainingArguments = $true)][string[]]$Arguments
    )

    $output = & $FilePath @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "Command failed ($LASTEXITCODE): $FilePath $($Arguments -join ' ')"
    }
    return ($output -join "`n").Trim()
}

$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
Push-Location $RepoRoot
try {
    $branch = Get-CommandOutput git rev-parse --abbrev-ref HEAD
    $sha = Get-CommandOutput git rev-parse --short=12 HEAD
    $trackedDirty = Get-CommandOutput git status --porcelain --untracked-files=no
    if ($trackedDirty -and -not $AllowDirty) {
        throw "Tracked files have uncommitted changes. Commit them first, or pass -AllowDirty to deploy this exact working tree."
    }

    if (-not $ImageTag) {
        $timestamp = (Get-Date).ToUniversalTime().ToString("yyyyMMddTHHmmssZ")
        $ImageTag = "green-$timestamp-$sha"
    }
    $image = "${ImageRepository}:${ImageTag}"

    if (-not $Yes -and -not $PreflightOnly) {
        Write-Host "Green deploy target: $RemoteHost $RemoteDir"
        Write-Host "Branch: $branch"
        Write-Host "Commit: $sha"
        Write-Host "Image:  $image"
        Write-Host "Green slot: 127.0.0.1:$GreenHostPort -> container :$ContainerPort"
        Write-Host "Nginx traffic will NOT be changed by this script."
        $answer = Read-Host "Type GREEN to continue"
        if ($answer -ne "GREEN") {
            throw "Green deployment cancelled."
        }
    }

    Write-Host "Running remote preflight checks..."
    Invoke-Remote "set -eu; test -d '$RemoteDir'; test -f '$RemoteDir/docker-compose.yml'; command -v docker >/dev/null; command -v base64 >/dev/null; docker compose version >/dev/null; docker inspect '$ActiveServiceName' >/dev/null"
    if ($PreflightOnly) {
        Write-Host "Preflight OK. No deployment was performed."
        return
    }

    $deployDir = Join-Path $RepoRoot ".deploy"
    New-Item -ItemType Directory -Force -Path $deployDir | Out-Null
    $safeTag = $ImageTag -replace '[^A-Za-z0-9_.-]', '_'
    $localTar = Join-Path $deployDir "$safeTag.tar"
    $remoteTar = "$RemoteDir/releases/$safeTag.tar"

    Write-Host "Building Docker image $image..."
    Invoke-External docker build --platform $Platform --pull -t $image $RepoRoot

    Write-Host "Saving image to $localTar..."
    if (Test-Path $localTar) {
        Remove-Item -LiteralPath $localTar -Force
    }
    Invoke-External docker save -o $localTar $image

    Write-Host "Uploading image archive to ${RemoteHost}:$remoteTar..."
    Invoke-Remote "mkdir -p '$RemoteDir/releases'"
    Invoke-External scp $localTar "${RemoteHost}:$remoteTar"

    $remoteScript = @"
set -Eeuo pipefail

remote_dir="$RemoteDir"
active_service="$ActiveServiceName"
green_service="$GreenServiceName"
image="$image"
image_tar="$remoteTar"
green_host_port="$GreenHostPort"
container_port="$ContainerPort"
health_timeout="$HealthTimeoutSeconds"

compose_file="`$remote_dir/docker-compose.yml"
green_override_file="`$remote_dir/docker-compose.green.override.yml"
green_env_file="`$remote_dir/new-api-green.env"

cd "`$remote_dir"

if [ ! -f "`$compose_file" ]; then
    echo "Missing compose file: `$compose_file" >&2
    exit 1
fi
if ! docker inspect "`$active_service" >/dev/null 2>&1; then
    echo "Active service container `$active_service not found; refusing to deploy blindly." >&2
    exit 1
fi

echo "Loading image archive..."
docker load -i "`$image_tar"

app_env="`$(docker inspect -f '{{range .Config.Env}}{{println .}}{{end}}' "`$active_service")"
if [ -z "`$app_env" ]; then
    echo "Cannot read environment from `$active_service." >&2
    exit 1
fi

umask 077
{
    printf '%s\n' "`$app_env" | awk -F= '
        `$1!="HOSTNAME" &&
        `$1!="PATH" &&
        `$1!="PORT" &&
        `$1!="NODE_NAME" &&
        `$1!="BATCH_UPDATE_ENABLED" {
            print
        }
    '
    printf 'PORT=%s\n' "`$container_port"
    printf 'NODE_NAME=nexus-sg-new-api-green\n'
    printf 'BATCH_UPDATE_ENABLED=false\n'
} > "`$green_env_file"

cat > "`$green_override_file" <<EOF
services:
  `$green_service:
    image: `$image
    container_name: `$green_service
    entrypoint:
      - /new-api
    command:
      - --log-dir
      - /app/logs
    env_file:
      - ./new-api-green.env
    ports:
      - "127.0.0.1:`$green_host_port:`$container_port"
    volumes:
      - ./data:/data
      - ./logs:/app/logs
    networks:
      - new-api-network
    restart: unless-stopped
    healthcheck:
      test:
        - CMD-SHELL
        - wget -q -O - http://localhost:`$container_port/api/status | grep -q '"success":true'
      interval: 30s
      timeout: 10s
      retries: 5
      start_period: 30s
EOF

docker compose -f "`$compose_file" -f "`$green_override_file" up -d --no-deps "`$green_service"

deadline=`$((SECONDS + health_timeout))
while [ `$SECONDS -lt `$deadline ]; do
    if curl -fsS "http://127.0.0.1:`$green_host_port/api/status" 2>/dev/null | grep -q '"success":true'; then
        health="`$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "`$green_service" 2>/dev/null || true)"
        if [ "`$health" = "healthy" ] || [ "`$health" = "running" ]; then
            echo "Green slot is healthy on 127.0.0.1:`$green_host_port."
            exit 0
        fi
    fi
    sleep 3
done

echo "Green slot did not become healthy within `$health_timeout seconds." >&2
docker ps --filter "name=`$green_service" --format 'table {{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Ports}}' >&2 || true
docker logs --tail 120 "`$green_service" >&2 || true
exit 1
"@

    Invoke-RemoteScript $remoteScript
}
finally {
    if (-not $KeepLocalImageTar -and $localTar -and (Test-Path $localTar)) {
        Remove-Item -LiteralPath $localTar -Force
    }
    Pop-Location
}
