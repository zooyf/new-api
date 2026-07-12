<#
.SYNOPSIS
Deploy the current checkout to the inactive Blue or Green slot on nexus-sg.

.DESCRIPTION
By default the script reads the managed Nginx upstream and automatically
selects the slot marked down. It refuses to deploy while both slots receive
traffic, or when the requested slot is active. It builds a Docker image from
the current commit, writes that slot's Compose override, and recreates only the
inactive container. It never changes Nginx traffic.
#>

[CmdletBinding()]
param(
    [ValidateSet("Auto", "Blue", "Green")][string]$Slot = "Auto",
    [string]$RemoteHost = "nexus-sg",
    [string]$RemoteDir = "/opt/new-api",
    [string]$UpstreamConf = "/etc/nginx/conf.d/new-api-upstream.conf",
    [string]$BlueServiceName = "new-api",
    [string]$GreenServiceName = "new-api-green",
    [string]$ImageRepository = "zooyf/new-api",
    [string]$ImageTag = "",
    [string]$Platform = "linux/amd64",
    [int]$BlueHostPort = 3000,
    [int]$GreenHostPort = 3002,
    [int]$ContainerPort = 3000,
    [int]$HealthTimeoutSeconds = 180,
    [ValidateSet("Direct", "Batch")][string]$BatchUpdateMode = "Direct",
    [ValidateRange(1, 300)][int]$BatchUpdateInterval = 5,
    [switch]$AllowDirty,
    [switch]$BuildOnRemote,
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

function Get-UpstreamSlotState {
    param(
        [Parameter(Mandatory = $true)][string]$Config,
        [Parameter(Mandatory = $true)][int]$BluePort,
        [Parameter(Mandatory = $true)][int]$GreenPort
    )

    $weights = @{}
    foreach ($entry in @(@{ Name = "Blue"; Port = $BluePort }, @{ Name = "Green"; Port = $GreenPort })) {
        $pattern = "(?m)^\s*server\s+127\.0\.0\.1:$($entry.Port)\s+([^;]+);"
        $match = [regex]::Match($Config, $pattern)
        if (-not $match.Success) {
            throw "Cannot find the $($entry.Name) slot on port $($entry.Port) in the managed Nginx upstream."
        }
        $arguments = $match.Groups[1].Value
        if ($arguments -match '(^|\s)down($|\s)') {
            $weights[$entry.Name] = 0
        } elseif ($arguments -match '(^|\s)weight=(\d+)($|\s)') {
            $weights[$entry.Name] = [int]$Matches[2]
        } else {
            $weights[$entry.Name] = 1
        }
    }
    return $weights
}

$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
Push-Location $RepoRoot
try {
    $branch = Get-CommandOutput git rev-parse --abbrev-ref HEAD
    $sha = Get-CommandOutput git rev-parse --short=12 HEAD
    $useRemoteBuild = $BuildOnRemote -or -not [bool](Get-Command docker -ErrorAction SilentlyContinue)
    $workingTreeDirty = Get-CommandOutput git status --porcelain
    if ($workingTreeDirty -and -not $AllowDirty) {
        throw "The working tree has uncommitted or untracked changes. Commit them first, or pass -AllowDirty to deploy this exact working tree."
    }
    if ($workingTreeDirty -and $AllowDirty -and $useRemoteBuild -and -not $PreflightOnly) {
        throw "Remote build uses git archive HEAD and cannot include working-tree changes. Commit first, or run on a machine with local Docker."
    }

    $nginxConfig = Get-CommandOutput ssh $RemoteHost "sudo cat '$UpstreamConf'"
    $slotWeights = Get-UpstreamSlotState -Config $nginxConfig -BluePort $BlueHostPort -GreenPort $GreenHostPort
    if ($Slot -eq "Auto") {
        if ($slotWeights.Blue -eq 0 -and $slotWeights.Green -gt 0) {
            $targetSlot = "Blue"
        } elseif ($slotWeights.Green -eq 0 -and $slotWeights.Blue -gt 0) {
            $targetSlot = "Green"
        } else {
            throw "Cannot auto-select an inactive slot while traffic is blue=$($slotWeights.Blue), green=$($slotWeights.Green). Complete or roll back the canary first, or explicitly mark one slot down."
        }
    } else {
        $targetSlot = $Slot
        if ($slotWeights[$targetSlot] -ne 0) {
            throw "Refusing to deploy the active $targetSlot slot (weight=$($slotWeights[$targetSlot])). Mark it down in Nginx before deployment."
        }
        $otherSlot = if ($targetSlot -eq "Blue") { "Green" } else { "Blue" }
        if ($slotWeights[$otherSlot] -le 0) {
            throw "Refusing deployment because the other slot is not serving traffic."
        }
    }

    $sourceSlot = if ($targetSlot -eq "Blue") { "Green" } else { "Blue" }
    $targetService = if ($targetSlot -eq "Blue") { $BlueServiceName } else { $GreenServiceName }
    $sourceService = if ($sourceSlot -eq "Blue") { $BlueServiceName } else { $GreenServiceName }
    $targetHostPort = if ($targetSlot -eq "Blue") { $BlueHostPort } else { $GreenHostPort }
    $slotName = $targetSlot.ToLowerInvariant()
    $nodeName = "nexus-sg-new-api-$slotName"
    $batchUpdateEnabled = if ($BatchUpdateMode -eq "Batch") { "true" } else { "false" }

    if (-not $ImageTag) {
        $timestamp = (Get-Date).ToUniversalTime().ToString("yyyyMMddTHHmmssZ")
        $ImageTag = "$slotName-$timestamp-$sha"
    }
    $image = "${ImageRepository}:${ImageTag}"

    if (-not $Yes -and -not $PreflightOnly) {
        Write-Host "Inactive slot selected: $targetSlot (blue=$($slotWeights.Blue), green=$($slotWeights.Green))"
        Write-Host "Deploy target: $RemoteHost $RemoteDir"
        Write-Host "Branch: $branch"
        Write-Host "Commit: $sha"
        Write-Host "Image:  $image"
        Write-Host "Build:  $(if ($useRemoteBuild) { 'remote Docker build from git archive' } else { 'local Docker build + upload' })"
        Write-Host "$targetSlot slot: 127.0.0.1:$targetHostPort -> container :$ContainerPort"
        Write-Host "Batch update mode: $BatchUpdateMode"
        Write-Host "Nginx traffic will NOT be changed by this script."
        $answer = Read-Host "Type $($targetSlot.ToUpperInvariant()) to continue"
        if ($answer -ne $targetSlot.ToUpperInvariant()) {
            throw "$targetSlot deployment cancelled."
        }
    }

    Write-Host "Running remote preflight checks..."
    Invoke-Remote "set -eu; test -d '$RemoteDir'; test -f '$RemoteDir/docker-compose.yml'; test -f '$UpstreamConf'; command -v docker >/dev/null; command -v base64 >/dev/null; docker compose version >/dev/null; docker inspect '$sourceService' >/dev/null; curl -fsS 'http://127.0.0.1:$(if ($sourceSlot -eq 'Blue') { $BlueHostPort } else { $GreenHostPort })/api/status' >/dev/null"
    if ($PreflightOnly) {
        Write-Host "Preflight OK. Auto-selected inactive slot: $targetSlot. No deployment was performed."
        return
    }

    $deployDir = Join-Path $RepoRoot ".deploy"
    New-Item -ItemType Directory -Force -Path $deployDir | Out-Null
    $safeTag = $ImageTag -replace '[^A-Za-z0-9_.-]', '_'
    $localTar = Join-Path $deployDir "$safeTag.tar"
    $localSourceTar = Join-Path $deployDir "$safeTag-source.tar"
    $remoteTar = "$RemoteDir/releases/$safeTag.tar"
    $remoteSourceTar = "$RemoteDir/releases/$safeTag-source.tar"
    $remoteBuildDir = "$RemoteDir/builds/$slotName-$safeTag-src"

    Invoke-Remote "mkdir -p '$RemoteDir/releases' '$RemoteDir/builds'"
    if ($useRemoteBuild) {
        Write-Host "Local Docker is unavailable or -BuildOnRemote was set; building $image on $RemoteHost..."
        if (Test-Path $localSourceTar) {
            Remove-Item -LiteralPath $localSourceTar -Force
        }
        Invoke-External git archive "--format=tar" "--output=$localSourceTar" HEAD
        Write-Host "Uploading source archive to ${RemoteHost}:$remoteSourceTar..."
        Invoke-External scp $localSourceTar "${RemoteHost}:$remoteSourceTar"
    } else {
        Write-Host "Building Docker image $image..."
        Invoke-External docker build --platform $Platform --pull --build-arg "BUILD_VERSION=$ImageTag" -t $image $RepoRoot

        Write-Host "Saving image to $localTar..."
        if (Test-Path $localTar) {
            Remove-Item -LiteralPath $localTar -Force
        }
        Invoke-External docker save -o $localTar $image

        Write-Host "Uploading image archive to ${RemoteHost}:$remoteTar..."
        Invoke-External scp $localTar "${RemoteHost}:$remoteTar"
    }

    $remoteScript = @"
set -Eeuo pipefail

remote_dir="$RemoteDir"
upstream_conf="$UpstreamConf"
target_slot="$slotName"
source_service="$sourceService"
target_service="$targetService"
image="$image"
image_tar="$remoteTar"
source_tar="$remoteSourceTar"
build_dir="$remoteBuildDir"
build_on_remote="$($useRemoteBuild.ToString().ToLowerInvariant())"
platform="$Platform"
target_host_port="$targetHostPort"
container_port="$ContainerPort"
health_timeout="$HealthTimeoutSeconds"
batch_update_enabled="$batchUpdateEnabled"
batch_update_interval="$BatchUpdateInterval"

compose_file="`$remote_dir/docker-compose.yml"
slot_override_file="`$remote_dir/docker-compose.`$target_slot.override.yml"
slot_env_file="`$remote_dir/new-api-`$target_slot.env"
slot_env_basename="new-api-`$target_slot.env"

cd "`$remote_dir"

if [ ! -f "`$compose_file" ]; then
    echo "Missing compose file: `$compose_file" >&2
    exit 1
fi
if ! docker inspect "`$source_service" >/dev/null 2>&1; then
    echo "Active source container `$source_service not found; refusing to deploy blindly." >&2
    exit 1
fi

# Re-check the target immediately before changing containers. This closes the
# race between local auto-selection and a concurrent Nginx weight change.
target_line="`$(sudo grep -E "^[[:space:]]*server[[:space:]]+127\\.0\\.0\\.1:`$target_host_port([[:space:]]|;)" "`$upstream_conf" | head -n 1 || true)"
if [ -z "`$target_line" ] || ! printf '%s' "`$target_line" | grep -Eq '(^|[[:space:]])down([[:space:]]|;)'; then
    echo "Target slot `$target_slot is no longer marked down in `$upstream_conf; refusing deployment." >&2
    exit 1
fi

if [ "`$build_on_remote" = "true" ]; then
    echo "Building image on remote host from source archive..."
    rm -rf "`$build_dir"
    mkdir -p "`$build_dir"
    tar -xf "`$source_tar" -C "`$build_dir"
    docker build --platform "`$platform" --pull --build-arg "BUILD_VERSION=$ImageTag" -t "`$image" "`$build_dir"
else
    echo "Loading image archive..."
    docker load -i "`$image_tar"
fi

app_env="`$(docker inspect -f '{{range .Config.Env}}{{println .}}{{end}}' "`$source_service")"
if [ -z "`$app_env" ]; then
    echo "Cannot read environment from `$source_service." >&2
    exit 1
fi

umask 077
{
    printf '%s\n' "`$app_env" | awk -F= '
        `$1!="HOSTNAME" &&
        `$1!="PATH" &&
        `$1!="PORT" &&
        `$1!="NODE_NAME" &&
        `$1!="BATCH_UPDATE_ENABLED" &&
        `$1!="BATCH_UPDATE_INTERVAL" {
            print
        }
    '
    printf 'PORT=%s\n' "`$container_port"
    printf 'NODE_NAME=%s\n' "$nodeName"
    printf 'BATCH_UPDATE_ENABLED=%s\n' "`$batch_update_enabled"
    printf 'BATCH_UPDATE_INTERVAL=%s\n' "`$batch_update_interval"
} > "`$slot_env_file"

if [ "`$target_slot" = "blue" ]; then
cat > "`$slot_override_file" <<EOF
services:
  `$target_service:
    image: `$image
    container_name: `$target_service
    env_file:
      - ./`$slot_env_basename
    environment:
      PORT: "`$container_port"
      NODE_NAME: "$nodeName"
      BATCH_UPDATE_ENABLED: "`$batch_update_enabled"
      BATCH_UPDATE_INTERVAL: "`$batch_update_interval"
EOF
else
cat > "`$slot_override_file" <<EOF
services:
  `$target_service:
    image: `$image
    container_name: `$target_service
    entrypoint:
      - /new-api
    command:
      - --log-dir
      - /app/logs
    env_file:
      - ./`$slot_env_basename
    environment:
      PORT: "`$container_port"
      NODE_NAME: "$nodeName"
      BATCH_UPDATE_ENABLED: "`$batch_update_enabled"
      BATCH_UPDATE_INTERVAL: "`$batch_update_interval"
    ports:
      - "127.0.0.1:`$target_host_port:`$container_port"
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
fi

docker compose -f "`$compose_file" -f "`$slot_override_file" up -d --no-deps "`$target_service"

deadline=`$((SECONDS + health_timeout))
while [ `$SECONDS -lt `$deadline ]; do
    status_body="`$(curl -fsS "http://127.0.0.1:`$target_host_port/api/status" 2>/dev/null || true)"
    if printf '%s' "`$status_body" | grep -q '"success":true'; then
        health="`$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "`$target_service" 2>/dev/null || true)"
        if [ "`$health" = "healthy" ] || [ "`$health" = "running" ]; then
            if ! printf '%s' "`$status_body" | grep -q "\"enable_batch_update\":`$batch_update_enabled"; then
                echo "Slot health succeeded but BATCH_UPDATE_ENABLED does not match `$batch_update_enabled." >&2
                exit 1
            fi
            runtime_version="`$(docker exec "`$target_service" /new-api --version 2>/dev/null || true)"
            if [ "`$runtime_version" != "$ImageTag" ]; then
                echo "Runtime version mismatch: expected $ImageTag, got `$runtime_version." >&2
                exit 1
            fi
            echo "$targetSlot slot is healthy on 127.0.0.1:`$target_host_port with batch mode $BatchUpdateMode."
            exit 0
        fi
    fi
    sleep 3
done

echo "$targetSlot slot did not become healthy within `$health_timeout seconds." >&2
docker ps --filter "name=`$target_service" --format 'table {{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Ports}}' >&2 || true
docker logs --tail 120 "`$target_service" >&2 || true
exit 1
"@

    Invoke-RemoteScript $remoteScript
}
finally {
    if (-not $KeepLocalImageTar -and $localTar -and (Test-Path $localTar)) {
        Remove-Item -LiteralPath $localTar -Force
    }
    if (-not $KeepLocalImageTar -and $localSourceTar -and (Test-Path $localSourceTar)) {
        Remove-Item -LiteralPath $localSourceTar -Force
    }
    Pop-Location
}
