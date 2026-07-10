<#
.SYNOPSIS
Deploy only the Enterprise Policy Hub sidecar to the nexus-sg production host.

.DESCRIPTION
This script packages the current local checkout, uploads it to the existing
/opt/new-api deployment, builds the shared new-api image on the server, and
starts or updates only the enterprise-policy-hub container. It does not recreate
the running new-api container, database, Redis, or other sidecars.
#>

[CmdletBinding()]
param(
    [string]$RemoteHost = "nexus-sg",
    [string]$RemoteDir = "/opt/new-api",
    [string]$NewAPIServiceName = "new-api",
    [string]$EnterpriseHubServiceName = "enterprise-policy-hub",
    [string]$ImageRepository = "zooyf/new-api",
    [string]$ImageTag = "",
    [string]$Platform = "linux/amd64",
    [int]$EnterpriseHubPort = 3100,
    [string]$EnterpriseHubBasePath = "/enterprise",
    [string]$EnterpriseHubNewAPIBaseUrl = "http://new-api:3000",
    [string]$EnterpriseHubBootstrapAdminIds = "",
    [int]$EnterpriseHubLogSyncIntervalSeconds = 10,
    [string]$EnterpriseHubBudgetTimezone = "",
    [string]$TokenOperationEnabled = $env:EPH_TOKENOP_ENABLED,
    [string]$TokenOperationBaseUrl = $env:EPH_TOKENOP_BASE_URL,
    [string]$TokenOperationGatewayKey = $env:EPH_TOKENOP_GATEWAY_KEY,
    [string]$TokenOperationObjectSyncEnabled = $env:EPH_TOKENOP_OBJECT_SYNC_ENABLED,
    [string]$TokenOperationUsageEventsEnabled = $env:EPH_TOKENOP_USAGE_EVENTS_ENABLED,
    [int]$TokenOperationTimeoutSeconds = 10,
    [int]$HealthTimeoutSeconds = 120,
    [switch]$AllowDirty,
    [switch]$PreflightOnly,
    [switch]$SkipNginxUpdate,
    [switch]$SkipTests,
    [switch]$TestOnly,
    [switch]$CommittedOnly,
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

    $stageRoot = Join-Path ([IO.Path]::GetTempPath()) ("enterprise-policy-hub-source-" + [Guid]::NewGuid().ToString("N"))
    New-Item -ItemType Directory -Force -Path $stageRoot | Out-Null
    try {
        $files = & git -C $SourceRoot ls-files --cached --others --exclude-standard
        if ($LASTEXITCODE -ne 0) {
            throw "Command failed ($LASTEXITCODE): git -C $SourceRoot ls-files --cached --others --exclude-standard"
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

function New-CommittedSourceArchive {
    param(
        [Parameter(Mandatory = $true)][string]$SourceRoot,
        [Parameter(Mandatory = $true)][string]$ArchivePath
    )

    if (Test-Path -LiteralPath $ArchivePath) {
        Remove-Item -LiteralPath $ArchivePath -Force
    }
    Invoke-External git -C $SourceRoot archive --format=tar --output=$ArchivePath HEAD
}

$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
Push-Location $RepoRoot
try {
    $branch = Get-CommandOutput git rev-parse --abbrev-ref HEAD
    $sha = Get-CommandOutput git rev-parse --short=12 HEAD
    $workingTreeDirty = Get-CommandOutput git status --porcelain
    if ($workingTreeDirty -and -not $AllowDirty -and -not $CommittedOnly) {
        throw "Working tree has uncommitted or untracked changes. Commit them first, or pass -AllowDirty to deploy this exact working tree."
    }

    if (-not $ImageTag) {
        $timestamp = (Get-Date).ToUniversalTime().ToString("yyyyMMddTHHmmssZ")
        $ImageTag = "enterprise-hub-$timestamp-$sha"
    }
    $image = "${ImageRepository}:${ImageTag}"

    if (-not $Yes -and -not $PreflightOnly) {
        Write-Host "Enterprise Policy Hub deploy target: $RemoteHost $RemoteDir"
        Write-Host "Branch: $branch"
        Write-Host "Commit: $sha"
        Write-Host "Image:  $image"
        Write-Host "Service: $EnterpriseHubServiceName on 127.0.0.1:$EnterpriseHubPort"
        Write-Host "new-api will not be recreated."
        $answer = Read-Host "Type DEPLOY to continue"
        if ($answer -ne "DEPLOY") {
            throw "Deployment cancelled."
        }
    }

    Write-Host "Running remote preflight checks..."
    $nginxPreflight = if ($SkipNginxUpdate) { "" } else { "; command -v nginx >/dev/null; sudo -n true; test -f /etc/nginx/sites-available/llm.ai.nexus-reach.com.conf" }
    Invoke-Remote "set -eu; test -d '$RemoteDir'; test -f '$RemoteDir/docker-compose.yml'; command -v docker >/dev/null; command -v tar >/dev/null; docker compose version >/dev/null; docker inspect '$NewAPIServiceName' >/dev/null$nginxPreflight"
    if ($PreflightOnly) {
        Write-Host "Preflight OK. No deployment was performed."
        return
    }

    $deployDir = Join-Path $RepoRoot ".deploy"
    New-Item -ItemType Directory -Force -Path $deployDir | Out-Null
    $safeTag = $ImageTag -replace '[^A-Za-z0-9_.-]', '_'
    $localSourceTar = Join-Path $deployDir "$safeTag-enterprise-hub-src.tar"
    $remoteSourceTar = "$RemoteDir/releases/$safeTag-enterprise-hub-src.tar"

    Invoke-Remote "mkdir -p '$RemoteDir/releases' '$RemoteDir/backups'"
    if ($CommittedOnly) {
        Write-Host "Packaging committed HEAD to $localSourceTar..."
        New-CommittedSourceArchive -SourceRoot $RepoRoot -ArchivePath $localSourceTar
    }
    else {
        Write-Host "Packaging local source checkout to $localSourceTar..."
        New-SourceArchive -SourceRoot $RepoRoot -ArchivePath $localSourceTar
    }

    Write-Host "Uploading source archive to ${RemoteHost}:$remoteSourceTar..."
    Invoke-External -FilePath scp -Arguments @(
        "-C",
        "-o", "ServerAliveInterval=30",
        "-o", "ServerAliveCountMax=6",
        $localSourceTar,
        "${RemoteHost}:$remoteSourceTar"
    )

    $nginxUpdateEnabledValue = if ($SkipNginxUpdate) { "0" } else { "1" }
    $remoteScript = @"
set -Eeuo pipefail

remote_dir="$RemoteDir"
new_api_service="$NewAPIServiceName"
hub_service="$EnterpriseHubServiceName"
image="$image"
source_tar="$remoteSourceTar"
safe_tag="$safeTag"
platform="$Platform"
hub_port="$EnterpriseHubPort"
hub_base_path="$EnterpriseHubBasePath"
hub_newapi_base_url="$EnterpriseHubNewAPIBaseUrl"
hub_bootstrap_admin_ids="$EnterpriseHubBootstrapAdminIds"
hub_log_sync_interval="$EnterpriseHubLogSyncIntervalSeconds"
hub_budget_timezone="$EnterpriseHubBudgetTimezone"
tokenop_enabled="$TokenOperationEnabled"
tokenop_base_url="$TokenOperationBaseUrl"
tokenop_gateway_key="$TokenOperationGatewayKey"
tokenop_object_sync_enabled="$TokenOperationObjectSyncEnabled"
tokenop_usage_events_enabled="$TokenOperationUsageEventsEnabled"
tokenop_timeout_seconds="$TokenOperationTimeoutSeconds"
health_timeout="$HealthTimeoutSeconds"
nginx_update_enabled="$nginxUpdateEnabledValue"
skip_tests="$($SkipTests.IsPresent.ToString().ToLowerInvariant())"
test_only="$($TestOnly.IsPresent.ToString().ToLowerInvariant())"

compose_file="`$remote_dir/docker-compose.yml"
hub_override_file="`$remote_dir/docker-compose.enterprise-policy-hub.override.yml"
hub_env_file="`$remote_dir/enterprise-policy-hub.env"
backup_dir="`$remote_dir/backups/`$(date -u +%Y%m%dT%H%M%SZ)-`$hub_service"

cd "`$remote_dir"
mkdir -p "`$backup_dir"
cp "`$compose_file" "`$backup_dir/docker-compose.yml"
if [ -f "`$hub_override_file" ]; then
    cp "`$hub_override_file" "`$backup_dir/docker-compose.enterprise-policy-hub.override.yml"
fi
if [ -f "`$hub_env_file" ]; then
    cp "`$hub_env_file" "`$backup_dir/enterprise-policy-hub.env"
fi

read_existing_hub_env() {
    key="`$1"
    if [ -f "`$hub_env_file" ]; then
        sed -n "s/^`$key=//p" "`$hub_env_file" | head -n 1
    fi
}

if [ -z "`$tokenop_enabled" ]; then
    tokenop_enabled="`$(read_existing_hub_env EPH_TOKENOP_ENABLED)"
fi
if [ -z "`$tokenop_base_url" ]; then
    tokenop_base_url="`$(read_existing_hub_env EPH_TOKENOP_BASE_URL)"
fi
if [ -z "`$tokenop_gateway_key" ]; then
    tokenop_gateway_key="`$(read_existing_hub_env EPH_TOKENOP_GATEWAY_KEY)"
fi
if [ -z "`$tokenop_object_sync_enabled" ]; then
    tokenop_object_sync_enabled="`$(read_existing_hub_env EPH_TOKENOP_OBJECT_SYNC_ENABLED)"
fi
if [ -z "`$tokenop_usage_events_enabled" ]; then
    tokenop_usage_events_enabled="`$(read_existing_hub_env EPH_TOKENOP_USAGE_EVENTS_ENABLED)"
fi
if [ -z "`$tokenop_enabled" ]; then
    if [ -n "`$tokenop_base_url" ] && [ -n "`$tokenop_gateway_key" ]; then
        tokenop_enabled="true"
    else
        tokenop_enabled="false"
    fi
fi
if [ -z "`$tokenop_object_sync_enabled" ]; then
    tokenop_object_sync_enabled="true"
fi
if [ -z "`$tokenop_usage_events_enabled" ]; then
    tokenop_usage_events_enabled="false"
fi

source_build_dir="`$remote_dir/builds/source-`$safe_tag"
case "`$source_build_dir" in
    "`$remote_dir"/builds/source-*) ;;
    *)
        echo "Unexpected source build directory: `$source_build_dir" >&2
        exit 1
        ;;
esac
rm -rf "`$source_build_dir.tmp"
mkdir -p "`$source_build_dir.tmp"
tar -xf "`$source_tar" -C "`$source_build_dir.tmp"
rm -rf "`$source_build_dir"
mv "`$source_build_dir.tmp" "`$source_build_dir"
if [ "`$skip_tests" != "true" ]; then
    echo "Running Enterprise Policy Hub tests..."
    docker run --rm \
        -v enterprise-hub-go-mod-cache:/go/pkg/mod \
        -v "`$source_build_dir:/work" \
        -w /work \
        golang:1.26.1-alpine \
        /usr/local/go/bin/go test ./pkg/enterprisepolicyhub
fi
if [ "`$test_only" = "true" ]; then
    echo "Enterprise Policy Hub tests completed. No deployment was performed."
    exit 0
fi
echo "Building Docker image on remote host: `$image"
docker build --platform "`$platform" --pull -t "`$image" "`$source_build_dir"

app_env="`$(docker inspect -f '{{range .Config.Env}}{{println .}}{{end}}' "`$new_api_service")"
sql_dsn="`$(printf '%s\n' "`$app_env" | sed -n 's/^SQL_DSN=//p' | head -n 1)"
log_sql_dsn="`$(printf '%s\n' "`$app_env" | sed -n 's/^LOG_SQL_DSN=//p' | head -n 1)"
redis_conn_string="`$(printf '%s\n' "`$app_env" | sed -n 's/^REDIS_CONN_STRING=//p' | head -n 1)"
sync_frequency="`$(printf '%s\n' "`$app_env" | sed -n 's/^SYNC_FREQUENCY=//p' | head -n 1)"
tz_value="`$(printf '%s\n' "`$app_env" | sed -n 's/^TZ=//p' | head -n 1)"
if [ -z "`$sql_dsn" ]; then
    echo "Cannot find SQL_DSN in `$new_api_service container environment." >&2
    exit 1
fi
if [ -z "`$hub_budget_timezone" ]; then
    hub_budget_timezone="`$tz_value"
fi
if [ -z "`$hub_budget_timezone" ]; then
    hub_budget_timezone="UTC"
fi

umask 077
{
    printf 'SQL_DSN=%s\n' "`$sql_dsn"
    if [ -n "`$log_sql_dsn" ]; then
        printf 'LOG_SQL_DSN=%s\n' "`$log_sql_dsn"
    fi
    if [ -n "`$redis_conn_string" ]; then
        printf 'REDIS_CONN_STRING=%s\n' "`$redis_conn_string"
    fi
    if [ -n "`$sync_frequency" ]; then
        printf 'SYNC_FREQUENCY=%s\n' "`$sync_frequency"
    fi
    if [ -n "`$tz_value" ]; then
        printf 'TZ=%s\n' "`$tz_value"
    fi
    printf 'EPH_PORT=%s\n' "`$hub_port"
    printf 'EPH_BASE_PATH=%s\n' "`$hub_base_path"
    printf 'EPH_NEWAPI_BASE_URL=%s\n' "`$hub_newapi_base_url"
    printf 'EPH_LOG_SYNC_INTERVAL_SECONDS=%s\n' "`$hub_log_sync_interval"
    printf 'EPH_BUDGET_TIMEZONE=%s\n' "`$hub_budget_timezone"
    printf 'EPH_ALLOW_ANY_NEWAPI_ADMIN=false\n'
    if [ -n "`$hub_bootstrap_admin_ids" ]; then
        printf 'EPH_BOOTSTRAP_ADMIN_IDS=%s\n' "`$hub_bootstrap_admin_ids"
    fi
    printf 'EPH_TOKENOP_ENABLED=%s\n' "`$tokenop_enabled"
    if [ -n "`$tokenop_base_url" ]; then
        printf 'EPH_TOKENOP_BASE_URL=%s\n' "`$tokenop_base_url"
    fi
    if [ -n "`$tokenop_gateway_key" ]; then
        printf 'EPH_TOKENOP_GATEWAY_KEY=%s\n' "`$tokenop_gateway_key"
    fi
    printf 'EPH_TOKENOP_OBJECT_SYNC_ENABLED=%s\n' "`$tokenop_object_sync_enabled"
    printf 'EPH_TOKENOP_USAGE_EVENTS_ENABLED=%s\n' "`$tokenop_usage_events_enabled"
    printf 'EPH_TOKENOP_TIMEOUT_SECONDS=%s\n' "`$tokenop_timeout_seconds"
} > "`$hub_env_file"
chmod 600 "`$hub_env_file"

cat > "`$hub_override_file" <<EOF
services:
  `$hub_service:
    image: `$image
    container_name: `$hub_service
    entrypoint:
      - /enterprise-policy-hub
    env_file:
      - ./enterprise-policy-hub.env
    ports:
      - "127.0.0.1:`$hub_port:`$hub_port"
    networks:
      - new-api-network
    restart: unless-stopped
    healthcheck:
      test:
        - CMD-SHELL
        - wget -q -O - http://localhost:`$hub_port/healthz | grep -q '^ok'
      interval: 30s
      timeout: 10s
      retries: 5
      start_period: 10s
EOF

echo "Recreating Enterprise Policy Hub only..."
docker compose -f "`$compose_file" -f "`$hub_override_file" up -d --no-deps "`$hub_service"

deadline=`$((SECONDS + health_timeout))
ok=0
while [ `$SECONDS -lt `$deadline ]; do
    if curl -fsS "http://127.0.0.1:`$hub_port/healthz" 2>/dev/null | grep -q '^ok'; then
        ok=1
        break
    fi
    health="`$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "`$hub_service" 2>/dev/null || true)"
    if [ "`$health" = "healthy" ]; then
        ok=1
        break
    fi
    sleep 5
done
if [ "`$ok" != "1" ]; then
    echo "Enterprise Policy Hub health check failed. Recent logs:" >&2
    docker logs --tail 120 "`$hub_service" >&2 || true
    exit 1
fi

if [ "`$nginx_update_enabled" = "1" ]; then
    nginx_site="/etc/nginx/sites-available/llm.ai.nexus-reach.com.conf"
    nginx_snippet="/etc/nginx/snippets/enterprise-policy-hub.conf"
    nginx_backup="`$backup_dir/llm.ai.nexus-reach.com.conf"
    sudo cp "`$nginx_site" "`$nginx_backup"
    sudo tee "`$nginx_snippet" >/dev/null <<EOF
location ^~ /enterprise/ {
    proxy_pass http://127.0.0.1:`$hub_port;
    proxy_http_version 1.1;
    proxy_set_header Host \`$host;
    proxy_set_header X-Real-IP \`$remote_addr;
    proxy_set_header X-Forwarded-For \`$proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto \`$scheme;
    proxy_set_header X-Forwarded-Host \`$host;
    proxy_set_header X-Forwarded-Port 443;
    proxy_set_header Upgrade \`$http_upgrade;
    proxy_set_header Connection \`$connection_upgrade;
    proxy_connect_timeout 30s;
    proxy_send_timeout 300s;
    proxy_read_timeout 300s;
    proxy_buffering off;
}
EOF
    if ! sudo grep -q "enterprise-policy-hub.conf" "`$nginx_site"; then
        tmp_nginx="`$(mktemp)"
        awk '
            BEGIN { inserted = 0; ssl_server = 0 }
            /listen[[:space:]].*443/ { ssl_server = 1 }
            ssl_server && !inserted && /^[[:space:]]*location[[:space:]]+\/[[:space:]]*\{/ {
                print "    include /etc/nginx/snippets/enterprise-policy-hub.conf;"
                print ""
                inserted = 1
            }
            { print }
        ' "`$nginx_site" > "`$tmp_nginx"
        if ! grep -q "enterprise-policy-hub.conf" "`$tmp_nginx"; then
            rm -f "`$tmp_nginx"
            echo "Failed to insert Enterprise Policy Hub nginx include." >&2
            exit 1
        fi
        sudo cp "`$tmp_nginx" "`$nginx_site"
        rm -f "`$tmp_nginx"
    fi
    if ! sudo nginx -t; then
        sudo cp "`$nginx_backup" "`$nginx_site"
        sudo nginx -t || true
        echo "Nginx validation failed; restored previous site config." >&2
        exit 1
    fi
    sudo systemctl reload nginx || sudo nginx -s reload
fi

echo "Enterprise Policy Hub deployment healthy."
docker ps --filter "name=^/`$hub_service`$" --format 'table {{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Ports}}'
echo "Backup directory: `$backup_dir"
"@

    Write-Host "Deploying Enterprise Policy Hub on remote host..."
    ($remoteScript -replace "`r`n", "`n") | & ssh $RemoteHost "bash -s"
    if ($LASTEXITCODE -ne 0) {
        throw "Remote Enterprise Policy Hub deployment failed."
    }

    if (-not $KeepLocalSourceTar) {
        Remove-Item -LiteralPath $localSourceTar -Force
    }
    if ($TestOnly) {
        Write-Host "Enterprise Policy Hub test run completed."
    }
    else {
        Write-Host "Enterprise Policy Hub deployment completed: $image"
    }
}
finally {
    Pop-Location
}
