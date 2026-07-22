<#
.SYNOPSIS
Build and deploy only the Reseller Hub sidecar on nexus-sg.

.DESCRIPTION
The default deploy path archives the current local checkout, uploads it to the
server, builds the shared image there, runs the Reseller Hub migration command,
and recreates only the reseller-hub service. The script discovers the active
Blue/Green new-api container from the managed Nginx upstream and reuses only
its database, Redis, timezone, and gateway connection settings.

The active new-api service is never recreated or restarted. Its container ID,
start time, and Nginx traffic state are compared before and after every change.
Rollback restores only the Reseller Hub env/Compose files and the Nginx site
and snippet; already-applied database migrations are intentionally not undone.
#>

[CmdletBinding()]
param(
    [string]$RemoteHost = "nexus-sg",
    [string]$RemoteDir = "/opt/new-api",
    [string]$UpstreamConf = "/etc/nginx/conf.d/new-api-upstream.conf",
    [string]$NginxSitePath = "/etc/nginx/sites-available/llm.ai.nexus-reach.com.conf",
    [string]$BlueServiceName = "new-api",
    [string]$GreenServiceName = "new-api-green",
    [ValidateRange(1, 65535)][int]$BlueHostPort = 3000,
    [ValidateRange(1, 65535)][int]$GreenHostPort = 3002,
    [ValidateRange(1, 65535)][int]$GatewayContainerPort = 3000,
    [string]$ResellerHubServiceName = "reseller-hub",
    [string]$ImageRepository = "zooyf/new-api",
    [string]$ImageTag = "",
    [string]$Platform = "linux/amd64",
    [ValidateRange(1, 65535)][int]$ResellerHubPort = 3200,
    [ValidateRange(1, 86400)][int]$ReconcileIntervalSeconds = 60,
    [ValidateRange(1, 86400)][int]$ConsistencyGraceSeconds = 180,
    [ValidateRange(0, 2147483647)][int]$CarrierLowQuota = 0,
    [ValidateRange(0, 1000000)][int]$KeyQPSAlertThreshold = 0,
    [ValidateRange(1, 3600)][int]$MigrationTimeoutSeconds = 120,
    [ValidateRange(1, 300)][int]$MigrationLockTimeoutSeconds = 10,
    [ValidateRange(1, 3600)][int]$HealthTimeoutSeconds = 120,
    [ValidatePattern('^[A-Za-z0-9_.-]*$')][string]$RollbackBackupName = "",
    [switch]$AllowDirty,
    [switch]$PreflightOnly,
    [switch]$Rollback,
    [switch]$SkipNginxUpdate,
    [switch]$KeepLocalSourceTar,
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
        throw "Remote Reseller Hub operation failed on $RemoteHost."
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

function New-SourceArchive {
    param(
        [Parameter(Mandatory = $true)][string]$SourceRoot,
        [Parameter(Mandatory = $true)][string]$ArchivePath
    )

    $stageRoot = Join-Path ([IO.Path]::GetTempPath()) ("reseller-hub-source-" + [Guid]::NewGuid().ToString("N"))
    New-Item -ItemType Directory -Force -Path $stageRoot | Out-Null
    try {
        $files = & git -C $SourceRoot ls-files --cached --others --exclude-standard
        if ($LASTEXITCODE -ne 0) {
            throw "Unable to enumerate the local source checkout."
        }

        foreach ($relativePath in $files) {
            if ([string]::IsNullOrWhiteSpace($relativePath)) {
                continue
            }
            $sourcePath = Join-Path $SourceRoot ($relativePath -replace '/', [IO.Path]::DirectorySeparatorChar)
            if (-not (Test-Path -LiteralPath $sourcePath -PathType Leaf)) {
                continue
            }
            $destinationPath = Join-Path $stageRoot ($relativePath -replace '/', [IO.Path]::DirectorySeparatorChar)
            $destinationDir = Split-Path -Parent $destinationPath
            New-Item -ItemType Directory -Force -Path $destinationDir | Out-Null
            Copy-Item -LiteralPath $sourcePath -Destination $destinationPath -Force
        }

        if (Test-Path -LiteralPath $ArchivePath) {
            Remove-Item -LiteralPath $ArchivePath -Force
        }
        Invoke-External tar -cf $ArchivePath -C $stageRoot .
    }
    finally {
        if (Test-Path -LiteralPath $stageRoot) {
            Remove-Item -LiteralPath $stageRoot -Recurse -Force
        }
    }
}

if ($PreflightOnly -and $Rollback) {
    throw "-PreflightOnly and -Rollback cannot be used together."
}
if ($RollbackBackupName -and -not $Rollback) {
    throw "-RollbackBackupName requires -Rollback."
}

$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$localSourceTar = $null

Push-Location $RepoRoot
try {
    $branch = Get-CommandOutput git rev-parse --abbrev-ref HEAD
    $sha = Get-CommandOutput git rev-parse --short=12 HEAD
    $workingTreeDirty = Get-CommandOutput git status --porcelain

    if (-not $Rollback) {
        if (-not (Test-Path -LiteralPath (Join-Path $RepoRoot "Dockerfile") -PathType Leaf)) {
            throw "Dockerfile is missing."
        }
        if (-not (Test-Path -LiteralPath (Join-Path $RepoRoot "cmd/reseller-hub") -PathType Container)) {
            throw "cmd/reseller-hub is missing; the Reseller Hub binary must be implemented before deployment."
        }
        if ($workingTreeDirty -and -not $AllowDirty) {
            throw "The working tree has changes. Commit them first, or pass -AllowDirty to deploy this exact local checkout."
        }
        if (-not $ImageTag) {
            $timestamp = (Get-Date).ToUniversalTime().ToString("yyyyMMddTHHmmssZ")
            $baseVersion = (& git describe --tags --abbrev=0 HEAD 2>$null)
            if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace(($baseVersion -join ""))) {
                $baseVersion = (Get-Content -LiteralPath (Join-Path $RepoRoot "VERSION") -Raw).Trim()
            } else {
                $baseVersion = ($baseVersion -join "").Trim()
            }
            $dirtySuffix = if ($workingTreeDirty) { ".dirty" } else { "" }
            $ImageTag = "$baseVersion.reseller-hub.$timestamp.g$sha$dirtySuffix"
        }
        if ($ImageTag -notmatch '^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$') {
            throw "ImageTag is not Docker-safe."
        }
    }

    $image = if ($Rollback) { "" } else { "${ImageRepository}:${ImageTag}" }
    $nginxUpdateEnabled = if ($SkipNginxUpdate) { "0" } else { "1" }
    $mode = if ($Rollback) { "rollback" } elseif ($PreflightOnly) { "preflight" } else { "deploy" }

    Write-Host "Running Reseller Hub remote preflight checks..."
    $nginxPreflight = if ($SkipNginxUpdate) { "" } else { "; command -v nginx >/dev/null; test -f '$NginxSitePath'" }
    Invoke-Remote "set -eu; test -d '$RemoteDir'; test -f '$RemoteDir/docker-compose.yml'; test -f '$UpstreamConf'; command -v docker >/dev/null; command -v tar >/dev/null; command -v curl >/dev/null; command -v timeout >/dev/null; sudo -n true; docker compose version >/dev/null; docker inspect '$BlueServiceName' >/dev/null 2>&1 || docker inspect '$GreenServiceName' >/dev/null 2>&1$nginxPreflight"

    if (-not $Yes -and -not $PreflightOnly) {
        if ($Rollback) {
            Write-Host "Rollback target: $RemoteHost $RemoteDir"
            Write-Host "Only Reseller Hub and its Nginx route will be restored."
            $answer = Read-Host "Type ROLLBACK to continue"
            if ($answer -ne "ROLLBACK") {
                throw "Rollback cancelled."
            }
        } else {
            Write-Host "Reseller Hub deploy target: $RemoteHost $RemoteDir"
            Write-Host "Branch: $branch"
            Write-Host "Commit: $sha"
            Write-Host "Image:  $image"
            Write-Host "Only $ResellerHubServiceName will be migrated and recreated."
            Write-Host "The active Blue/Green new-api container will not be restarted."
            $answer = Read-Host "Type DEPLOY to continue"
            if ($answer -ne "DEPLOY") {
                throw "Deployment cancelled."
            }
        }
    }

    if ($mode -eq "deploy") {
        $deployDir = Join-Path $RepoRoot ".deploy"
        New-Item -ItemType Directory -Force -Path $deployDir | Out-Null
        $safeTag = $ImageTag -replace '[^A-Za-z0-9_.-]', '_'
        $localSourceTar = Join-Path $deployDir "$safeTag-reseller-hub-source.tar"
        $remoteSourceTar = "$RemoteDir/releases/$safeTag-reseller-hub-source.tar"

        Write-Host "Packaging the current local checkout..."
        New-SourceArchive -SourceRoot $RepoRoot -ArchivePath $localSourceTar
        Invoke-Remote "mkdir -p '$RemoteDir/releases' '$RemoteDir/builds' '$RemoteDir/backups'"
        Write-Host "Uploading the source archive..."
        Invoke-External -FilePath scp -Arguments @(
            "-C",
            "-o", "ServerAliveInterval=30",
            "-o", "ServerAliveCountMax=6",
            $localSourceTar,
            "${RemoteHost}:$remoteSourceTar"
        )
    } else {
        $safeTag = ""
        $remoteSourceTar = ""
    }

    $remoteScript = @"
set -Eeuo pipefail

mode="$mode"
remote_dir="$RemoteDir"
compose_file="`$remote_dir/docker-compose.yml"
upstream_conf="$UpstreamConf"
nginx_site="$NginxSitePath"
nginx_snippet="/etc/nginx/snippets/reseller-hub.conf"
blue_service="$BlueServiceName"
green_service="$GreenServiceName"
blue_port="$BlueHostPort"
green_port="$GreenHostPort"
gateway_container_port="$GatewayContainerPort"
hub_service="$ResellerHubServiceName"
hub_port="$ResellerHubPort"
image="$image"
platform="$Platform"
source_tar="$remoteSourceTar"
safe_tag="$safeTag"
reconcile_interval="$ReconcileIntervalSeconds"
consistency_grace="$ConsistencyGraceSeconds"
carrier_low_quota="$CarrierLowQuota"
key_qps_alert_threshold="$KeyQPSAlertThreshold"
migration_timeout="$MigrationTimeoutSeconds"
migration_lock_timeout="$MigrationLockTimeoutSeconds"
health_timeout="$HealthTimeoutSeconds"
nginx_update_enabled="$nginxUpdateEnabled"
rollback_backup_name="$RollbackBackupName"
hub_override_file="`$remote_dir/docker-compose.reseller-hub.override.yml"
hub_env_file="`$remote_dir/reseller-hub.env"

cd "`$remote_dir"

slot_weight() {
    port="`$1"
    line="`$(sudo grep -E "^[[:space:]]*server[[:space:]]+127\\.0\\.0\\.1:`${port}([[:space:]]|;)" "`$upstream_conf" | head -n 1 || true)"
    if [ -z "`$line" ]; then
        echo "Missing Blue/Green server on port `$port in `$upstream_conf." >&2
        return 1
    fi
    if printf '%s' "`$line" | grep -Eq '(^|[[:space:]])down([[:space:]]|;)'; then
        printf '0'
    elif printf '%s' "`$line" | grep -Eq '(^|[[:space:]])weight=[0-9]+([[:space:]]|;)'; then
        printf '%s' "`$line" | sed -E 's/.*(^|[[:space:]])weight=([0-9]+)([[:space:]]|;).*/\2/'
    else
        printf '1'
    fi
}

discover_active() {
    blue_weight="`$(slot_weight "`$blue_port")"
    green_weight="`$(slot_weight "`$green_port")"
    active_services=""
    if [ "`$blue_weight" -gt 0 ]; then
        active_services="`$blue_service"
    fi
    if [ "`$green_weight" -gt 0 ]; then
        active_services="`$active_services `$green_service"
    fi
    active_services="`$(printf '%s' "`$active_services" | xargs)"
    if [ -z "`$active_services" ]; then
        echo "Neither Blue nor Green is receiving traffic." >&2
        return 1
    fi
    if [ "`$green_weight" -gt "`$blue_weight" ]; then
        config_service="`$green_service"
    elif [ "`$blue_weight" -gt "`$green_weight" ]; then
        config_service="`$blue_service"
    elif [ "`$blue_weight" -gt 0 ]; then
        echo "Blue and Green have equal positive traffic weights; refusing to choose an ambiguous configuration source." >&2
        return 1
    fi
    for service in `$active_services; do
        docker inspect "`$service" >/dev/null
        state="`$(docker inspect -f '{{.State.Status}}' "`$service")"
        if [ "`$state" != "running" ]; then
            echo "Active service `$service is not running." >&2
            return 1
        fi
    done
}

active_snapshot() {
    discover_active
    printf 'blue_weight=%s|green_weight=%s' "`$blue_weight" "`$green_weight"
    for service in `$active_services; do
        docker inspect -f '|{{.Name}}|{{.Id}}|{{.State.StartedAt}}' "`$service"
    done
}

assert_active_unchanged() {
    current="`$(active_snapshot)"
    if [ "`$current" != "`$active_before" ]; then
        echo "Active Blue/Green identity, start time, or traffic state changed during the Sidecar operation." >&2
        echo "The operation is considered failed; active new-api was not restarted by this script." >&2
        return 1
    fi
}

discover_active
active_before="`$(active_snapshot)"
app_env="`$(docker inspect -f '{{range .Config.Env}}{{println .}}{{end}}' "`$config_service")"
if ! printf '%s\n' "`$app_env" | grep -q '^SQL_DSN='; then
    echo "The selected active service does not expose SQL_DSN." >&2
    exit 1
fi

if [ "`$mode" = "preflight" ]; then
    echo "Preflight OK. Configuration source: `$config_service; active services: `$active_services."
    echo "No file, container, database, or Nginx change was made."
    exit 0
fi

restore_files_from_backup() {
    source_dir="`$1"
    if [ -f "`$source_dir/had-hub-override" ]; then
        cp "`$source_dir/docker-compose.reseller-hub.override.yml" "`$hub_override_file"
    else
        rm -f "`$hub_override_file"
    fi
    if [ -f "`$source_dir/had-hub-env" ]; then
        cp "`$source_dir/reseller-hub.env" "`$hub_env_file"
        chmod 600 "`$hub_env_file"
    else
        rm -f "`$hub_env_file"
    fi
    if [ "`$nginx_update_enabled" = "1" ]; then
        sudo cp "`$source_dir/nginx-site.conf" "`$nginx_site"
        if [ -f "`$source_dir/had-nginx-snippet" ]; then
            sudo cp "`$source_dir/reseller-hub.conf" "`$nginx_snippet"
        else
            sudo rm -f "`$nginx_snippet"
        fi
    fi
}

apply_restored_sidecar() {
    source_dir="`$1"
    if [ -f "`$source_dir/had-hub-override" ]; then
        docker compose -f "`$compose_file" -f "`$hub_override_file" up -d --no-deps "`$hub_service"
    else
        docker compose -f "`$compose_file" -f "`$hub_override_file" rm -sf "`$hub_service" 2>/dev/null || docker rm -f "`$hub_service" 2>/dev/null || true
    fi
}

capture_backup() {
    target_dir="`$1"
    mkdir -p "`$target_dir"
    chmod 700 "`$target_dir"
    if [ -f "`$hub_override_file" ]; then
        cp "`$hub_override_file" "`$target_dir/docker-compose.reseller-hub.override.yml"
        touch "`$target_dir/had-hub-override"
    fi
    if [ -f "`$hub_env_file" ]; then
        cp "`$hub_env_file" "`$target_dir/reseller-hub.env"
        chmod 600 "`$target_dir/reseller-hub.env"
        touch "`$target_dir/had-hub-env"
    fi
    if [ "`$nginx_update_enabled" = "1" ]; then
        sudo cp "`$nginx_site" "`$target_dir/nginx-site.conf"
        if sudo test -f "`$nginx_snippet"; then
            sudo cp "`$nginx_snippet" "`$target_dir/reseller-hub.conf"
            touch "`$target_dir/had-nginx-snippet"
        fi
    fi
    printf '%s\n' "`$active_before" > "`$target_dir/active-before.txt"
}

if [ "`$mode" = "rollback" ]; then
    if [ -n "`$rollback_backup_name" ]; then
        rollback_dir="`$remote_dir/backups/`$rollback_backup_name"
    else
        rollback_dir="`$(find "`$remote_dir/backups" -mindepth 1 -maxdepth 1 -type d -name 'reseller-hub-*' ! -name 'reseller-hub-rollback-guard-*' -printf '%T@ %p\n' | sort -nr | head -n 1 | cut -d' ' -f2-)"
    fi
    case "`$rollback_dir" in
        "`$remote_dir"/backups/reseller-hub-*) ;;
        *) echo "Invalid or missing Reseller Hub backup directory." >&2; exit 1 ;;
    esac
    if [ ! -d "`$rollback_dir" ] || { [ "`$nginx_update_enabled" = "1" ] && [ ! -f "`$rollback_dir/nginx-site.conf" ]; }; then
        echo "Rollback backup is incomplete: `$rollback_dir" >&2
        exit 1
    fi

    guard_dir="`$remote_dir/backups/reseller-hub-rollback-guard-`$(date -u +%Y%m%dT%H%M%SZ)"
    capture_backup "`$guard_dir"
    restore_files_from_backup "`$rollback_dir"
    apply_restored_sidecar "`$rollback_dir"
    if [ "`$nginx_update_enabled" = "1" ]; then
        if ! sudo nginx -t; then
            restore_files_from_backup "`$guard_dir"
            apply_restored_sidecar "`$guard_dir"
            sudo nginx -t || true
            echo "Rollback Nginx validation failed; the pre-rollback Sidecar and Nginx files were restored." >&2
            exit 1
        fi
        sudo systemctl reload nginx || sudo nginx -s reload
    fi
    assert_active_unchanged
    echo "Reseller Hub and its Nginx route were restored from `$rollback_dir."
    echo "No active new-api container was restarted. Database migrations were not reversed."
    exit 0
fi

backup_dir="`$remote_dir/backups/reseller-hub-`$(date -u +%Y%m%dT%H%M%SZ)-`$safe_tag"
capture_backup "`$backup_dir"
rollback_ready=1

rollback_failed_deploy() {
    if [ "`$rollback_ready" != "1" ]; then
        return
    fi
    echo "Restoring the previous Reseller Hub and Nginx files after deployment failure..." >&2
    restore_files_from_backup "`$backup_dir"
    apply_restored_sidecar "`$backup_dir"
    if [ "`$nginx_update_enabled" = "1" ]; then
        sudo nginx -t && (sudo systemctl reload nginx || sudo nginx -s reload) || true
    fi
}

on_deploy_error() {
    code="`$?"
    trap - ERR
    set +e
    rollback_failed_deploy
    assert_active_unchanged
    echo "The Sidecar deployment failed. Existing new-api containers were not intentionally changed." >&2
    exit "`$code"
}
trap on_deploy_error ERR

source_build_dir="`$remote_dir/builds/reseller-hub-`$safe_tag"
case "`$source_build_dir" in
    "`$remote_dir"/builds/reseller-hub-*) ;;
    *) echo "Unexpected source build directory." >&2; exit 1 ;;
esac
rm -rf "`$source_build_dir.tmp"
mkdir -p "`$source_build_dir.tmp"
tar -xf "`$source_tar" -C "`$source_build_dir.tmp"
rm -rf "`$source_build_dir"
mv "`$source_build_dir.tmp" "`$source_build_dir"

echo "Building the shared image from the uploaded local checkout..."
docker build --platform "`$platform" --pull --build-arg "BUILD_VERSION=$ImageTag" -t "`$image" "`$source_build_dir"
assert_active_unchanged

read_active_env() {
    key="`$1"
    printf '%s\n' "`$app_env" | sed -n "s/^`$key=//p" | head -n 1
}

sql_dsn="`$(read_active_env SQL_DSN)"
log_sql_dsn="`$(read_active_env LOG_SQL_DSN)"
redis_conn_string="`$(read_active_env REDIS_CONN_STRING)"
tz_value="`$(read_active_env TZ)"
gateway_base_url="http://`$config_service:`$gateway_container_port"

umask 077
{
    printf 'SQL_DSN=%s\n' "`$sql_dsn"
    if [ -n "`$log_sql_dsn" ]; then printf 'LOG_SQL_DSN=%s\n' "`$log_sql_dsn"; fi
    if [ -n "`$redis_conn_string" ]; then printf 'REDIS_CONN_STRING=%s\n' "`$redis_conn_string"; fi
    if [ -n "`$tz_value" ]; then printf 'TZ=%s\n' "`$tz_value"; fi
    printf 'SQL_MAX_IDLE_CONNS=5\n'
    printf 'SQL_MAX_OPEN_CONNS=20\n'
    printf 'RESELLER_HUB_PORT=%s\n' "`$hub_port"
    printf 'RESELLER_HUB_BASE_PATH=/reseller\n'
    printf 'RESELLER_HUB_GATEWAY_BASE_URL=%s\n' "`$gateway_base_url"
    printf 'RESELLER_HUB_RECONCILE_INTERVAL_SECONDS=%s\n' "`$reconcile_interval"
    printf 'RESELLER_HUB_CONSISTENCY_GRACE_SECONDS=%s\n' "`$consistency_grace"
    printf 'RESELLER_HUB_MIGRATION_TIMEOUT_SECONDS=%s\n' "`$migration_timeout"
    printf 'RESELLER_HUB_MIGRATION_LOCK_TIMEOUT_SECONDS=%s\n' "`$migration_lock_timeout"
    printf 'RESELLER_HUB_AUTO_MIGRATE=false\n'
    printf 'RESELLER_HUB_CARRIER_LOW_QUOTA=%s\n' "`$carrier_low_quota"
    printf 'RESELLER_HUB_KEY_QPS_ALERT_THRESHOLD=%s\n' "`$key_qps_alert_threshold"
    printf 'RESELLER_HUB_REDIS_PREFIX=reseller_hub:\n'
} > "`$hub_env_file"
chmod 600 "`$hub_env_file"

cat > "`$hub_override_file" <<EOF
services:
  `$hub_service:
    image: `$image
    container_name: `$hub_service
    entrypoint:
      - /reseller-hub
    command:
      - serve
    env_file:
      - ./reseller-hub.env
    ports:
      - "127.0.0.1:`$hub_port:`$hub_port"
    networks:
      - new-api-network
    restart: unless-stopped
    cpus: "0.50"
    mem_limit: 512m
    healthcheck:
      test:
        - CMD-SHELL
        - wget -q -O - http://localhost:`$hub_port/healthz | grep -q '"status":"ok"'
      interval: 30s
      timeout: 10s
      retries: 5
      start_period: 10s
EOF

assert_active_unchanged
echo "Running the isolated Reseller Hub schema migration..."
timeout --foreground "`$migration_timeout"s docker compose -f "`$compose_file" -f "`$hub_override_file" run --rm --no-deps "`$hub_service" migrate
assert_active_unchanged

echo "Starting only the Reseller Hub service..."
docker compose -f "`$compose_file" -f "`$hub_override_file" up -d --no-deps "`$hub_service"

deadline=`$((SECONDS + health_timeout))
healthy=0
while [ `$SECONDS -lt `$deadline ]; do
    if curl -fsS "http://127.0.0.1:`$hub_port/healthz" 2>/dev/null | grep -q '"status":"ok"'; then
        healthy=1
        break
    fi
    sleep 3
done
if [ "`$healthy" != "1" ]; then
    echo "Reseller Hub did not become healthy within `$health_timeout seconds." >&2
    false
fi

if [ "`$nginx_update_enabled" = "1" ]; then
    sudo tee "`$nginx_snippet" >/dev/null <<EOF
location ^~ /reseller/ {
    proxy_pass http://127.0.0.1:`$hub_port;
    proxy_http_version 1.1;
    proxy_set_header Host \`$host;
    proxy_set_header X-Real-IP \`$remote_addr;
    proxy_set_header X-Forwarded-For \`$proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto \`$scheme;
    proxy_connect_timeout 30s;
    proxy_send_timeout 300s;
    proxy_read_timeout 300s;
    proxy_buffering off;
}
EOF
    if ! sudo grep -q 'snippets/reseller-hub.conf' "`$nginx_site"; then
        tmp_nginx="`$(mktemp)"
        awk '
            BEGIN { inserted = 0; ssl_server = 0 }
            /listen[[:space:]].*443/ { ssl_server = 1 }
            ssl_server && !inserted && /^[[:space:]]*location[[:space:]]+\/[[:space:]]*\{/ {
                print "    include /etc/nginx/snippets/reseller-hub.conf;"
                print ""
                inserted = 1
            }
            { print }
        ' "`$nginx_site" > "`$tmp_nginx"
        if ! grep -q 'snippets/reseller-hub.conf' "`$tmp_nginx"; then
            rm -f "`$tmp_nginx"
            echo "Unable to insert the Reseller Hub Nginx include before location /." >&2
            false
        fi
        sudo cp "`$tmp_nginx" "`$nginx_site"
        rm -f "`$tmp_nginx"
    fi
    sudo nginx -t
    sudo systemctl reload nginx || sudo nginx -s reload
fi

assert_active_unchanged
trap - ERR
rollback_ready=0
echo "Reseller Hub deployment completed and is healthy."
echo "Backup directory: `$backup_dir"
echo "Active new-api identity and start time are unchanged."
docker ps --filter "name=^/`$hub_service`$" --format 'table {{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Ports}}'
"@

    Invoke-RemoteScript -Script $remoteScript
}
finally {
    if ($localSourceTar -and (Test-Path -LiteralPath $localSourceTar) -and -not $KeepLocalSourceTar) {
        Remove-Item -LiteralPath $localSourceTar -Force
    }
    Pop-Location
}
