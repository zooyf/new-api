<#
.SYNOPSIS
Deploy the current new-api checkout to the production nexus-sg server.

.DESCRIPTION
By default this script packages the current local checkout as a source archive,
uploads it to the existing /opt/new-api deployment on nexus-sg, builds the
Docker image on the server, backs up the current compose file, /opt/new-api/data,
and PostgreSQL database, then recreates only the new-api application container.
It does not recreate or remove database/redis containers or Docker volumes.

Use -Yes to skip the production confirmation prompt.
Use -AllowDirty to deploy with tracked uncommitted changes.
Use -HwdramaProxyOnly to deploy or update only the Hwdrama material proxy
without recreating the running new-api application container.
Use -LocalDockerBuild to build the Docker image locally and upload an image tar
instead of uploading a source archive for remote build.
Hwdrama proxy upstream routing is read only from routes.yml and secrets.env.
reverse-newapi-volcengine is an optional sidecar that exposes Volcengine
Seedance task paths and forwards them to the active new-api video endpoint.
#>

[CmdletBinding()]
param(
    [string]$RemoteHost = "nexus-sg",
    [string]$RemoteDir = "/opt/new-api",
    [string]$ServiceName = "new-api",
    [string]$HwdramaProxyServiceName = "hwdrama-proxy",
    [string]$ReverseVolcengineServiceName = "reverse-newapi-volcengine",
    [string]$PostgresContainer = "new-api-postgres",
    [string]$ImageRepository = "zooyf/new-api",
    [string]$ImageTag = "",
    [string]$Platform = "linux/amd64",
    [int]$HealthTimeoutSeconds = 180,
    [int]$HwdramaProxyPort = 3001,
    [int]$HwdramaProxyTimeoutSeconds = 600,
    [int]$ReverseVolcenginePort = 3003,
    [int]$ReverseVolcengineTimeoutSeconds = 600,
    [string]$ReverseVolcengineUpstreamBaseUrl = "http://new-api-green:3000",
    [switch]$AllowDirty,
    [switch]$PreflightOnly,
    [switch]$SkipBackup,
    [switch]$SkipHwdramaProxy,
    [switch]$SkipReverseVolcengine,
    [switch]$HwdramaProxyOnly,
    [switch]$SkipNginxUpdate,
    [switch]$NoRollback,
    [switch]$LocalDockerBuild,
    [switch]$KeepLocalImageTar,
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

    $stageRoot = Join-Path ([IO.Path]::GetTempPath()) ("new-api-source-" + [Guid]::NewGuid().ToString("N"))
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

$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
Push-Location $RepoRoot
try {
    if ($HwdramaProxyOnly -and $SkipHwdramaProxy) {
        throw "-HwdramaProxyOnly cannot be combined with -SkipHwdramaProxy."
    }

    $branch = Get-CommandOutput git rev-parse --abbrev-ref HEAD
    $sha = Get-CommandOutput git rev-parse --short=12 HEAD
    $workingTreeDirty = Get-CommandOutput git status --porcelain
    if ($workingTreeDirty -and -not $AllowDirty) {
        throw "Working tree has uncommitted or untracked changes. Commit them first, or pass -AllowDirty to deploy this exact working tree."
    }

    if (-not $ImageTag) {
        $timestamp = (Get-Date).ToUniversalTime().ToString("yyyyMMddTHHmmssZ")
        $ImageTag = "nexus-$timestamp-$sha"
    }
    $image = "${ImageRepository}:${ImageTag}"

    if (-not $Yes -and -not $PreflightOnly) {
        Write-Host "Production deploy target: $RemoteHost $RemoteDir"
        Write-Host "Branch: $branch"
        Write-Host "Commit: $sha"
        Write-Host "Image:  $image"
        if ($LocalDockerBuild) {
            Write-Host "Build:  local Docker build, upload image tar"
        } else {
            Write-Host "Build:  upload local source tar, build on server"
        }
        if ($HwdramaProxyOnly) {
            Write-Host "Mode:   Hwdrama proxy only; running new-api will not be recreated"
        }
        $answer = Read-Host "Type DEPLOY to continue"
        if ($answer -ne "DEPLOY") {
            throw "Deployment cancelled."
        }
    }

    Write-Host "Running remote preflight checks..."
    $nginxPreflight = if ($SkipNginxUpdate) { "" } else { "; command -v nginx >/dev/null; sudo -n true; test -f /etc/nginx/sites-available/llm.ai.nexus-reach.com.conf" }
    $remoteBuildPreflight = if ($LocalDockerBuild) { "" } else { "; command -v tar >/dev/null" }
    Invoke-Remote "set -eu; test -d '$RemoteDir'; test -f '$RemoteDir/docker-compose.yml'; command -v docker >/dev/null; docker compose version >/dev/null$remoteBuildPreflight$nginxPreflight"
    if ($PreflightOnly) {
        Write-Host "Preflight OK. No deployment was performed."
        return
    }

    $deployDir = Join-Path $RepoRoot ".deploy"
    New-Item -ItemType Directory -Force -Path $deployDir | Out-Null
    $safeTag = $ImageTag -replace '[^A-Za-z0-9_.-]', '_'
    $localTar = Join-Path $deployDir "$safeTag.tar"
    $remoteTar = "$RemoteDir/releases/$safeTag.tar"
    $localSourceTar = Join-Path $deployDir "$safeTag-src.tar"
    $remoteSourceTar = "$RemoteDir/releases/$safeTag-src.tar"

    Invoke-Remote "mkdir -p '$RemoteDir/releases' '$RemoteDir/backups'"
    if ($LocalDockerBuild) {
        Write-Host "Building Docker image $image locally..."
        Invoke-External docker build --platform $Platform --pull -t $image $RepoRoot

        Write-Host "Saving image to $localTar..."
        if (Test-Path $localTar) {
            Remove-Item -LiteralPath $localTar -Force
        }
        Invoke-External docker save -o $localTar $image

        Write-Host "Uploading image archive to ${RemoteHost}:$remoteTar..."
        Invoke-External scp $localTar "${RemoteHost}:$remoteTar"
    } else {
        Write-Host "Packaging local source checkout to $localSourceTar..."
        New-SourceArchive -SourceRoot $RepoRoot -ArchivePath $localSourceTar

        Write-Host "Uploading source archive to ${RemoteHost}:$remoteSourceTar..."
        Invoke-External scp $localSourceTar "${RemoteHost}:$remoteSourceTar"
    }

    $skipBackupValue = if ($SkipBackup) { "1" } else { "0" }
    $noRollbackValue = if ($NoRollback) { "1" } else { "0" }
    $hwdramaProxyEnabledValue = if ($SkipHwdramaProxy) { "0" } else { "1" }
    $reverseVolcengineEnabledValue = if ($SkipReverseVolcengine) { "0" } else { "1" }
    $hwdramaProxyOnlyValue = if ($HwdramaProxyOnly) { "1" } else { "0" }
    $nginxUpdateEnabledValue = if ($SkipNginxUpdate) { "0" } else { "1" }
    $localDockerBuildValue = if ($LocalDockerBuild) { "1" } else { "0" }

    $remoteScript = @"
set -Eeuo pipefail

remote_dir="$RemoteDir"
service_name="$ServiceName"
proxy_service_name="$HwdramaProxyServiceName"
reverse_service_name="$ReverseVolcengineServiceName"
postgres_container="$PostgresContainer"
image="$image"
image_tar="$remoteTar"
source_tar="$remoteSourceTar"
safe_tag="$safeTag"
platform="$Platform"
health_timeout="$HealthTimeoutSeconds"
skip_backup="$skipBackupValue"
no_rollback="$noRollbackValue"
hwdrama_proxy_enabled="$hwdramaProxyEnabledValue"
reverse_volcengine_enabled="$reverseVolcengineEnabledValue"
hwdrama_proxy_only="$hwdramaProxyOnlyValue"
nginx_update_enabled="$nginxUpdateEnabledValue"
local_docker_build="$localDockerBuildValue"
hwdrama_proxy_port="$HwdramaProxyPort"
hwdrama_proxy_timeout="$HwdramaProxyTimeoutSeconds"
reverse_volcengine_port="$ReverseVolcenginePort"
reverse_volcengine_timeout="$ReverseVolcengineTimeoutSeconds"
reverse_volcengine_upstream_base_url="$ReverseVolcengineUpstreamBaseUrl"

compose_file="`$remote_dir/docker-compose.yml"
override_file="`$remote_dir/docker-compose.deploy.override.yml"
proxy_override_file="`$remote_dir/docker-compose.hwdrama-proxy.override.yml"
reverse_override_file="`$remote_dir/docker-compose.reverse-volcengine.override.yml"
proxy_env_file="`$remote_dir/hwdrama-proxy.env"
reverse_env_file="`$remote_dir/reverse-newapi-volcengine.env"
proxy_config_dir="`$remote_dir/hwdrama-proxy"
proxy_routes_file="`$proxy_config_dir/routes.yml"
proxy_secrets_file="`$proxy_config_dir/secrets.env"
backup_dir="`$remote_dir/backups/`$(date -u +%Y%m%dT%H%M%SZ)-`$service_name"

active_override_file="`$override_file"
if [ "`$hwdrama_proxy_only" = "1" ]; then
    active_override_file="`$proxy_override_file"
fi

cd "`$remote_dir"

compose() {
    docker compose -f "`$compose_file" -f "`$active_override_file" "`$@"
}

if [ ! -f "`$compose_file" ]; then
    echo "Missing compose file: `$compose_file" >&2
    exit 1
fi

previous_image="`$(docker inspect -f '{{.Config.Image}}' "`$service_name" 2>/dev/null || true)"
if [ -z "`$previous_image" ]; then
    echo "Container `$service_name not found; refusing to deploy blindly." >&2
    exit 1
fi
previous_proxy_image="`$(docker inspect -f '{{.Config.Image}}' "`$proxy_service_name" 2>/dev/null || true)"
previous_reverse_image="`$(docker inspect -f '{{.Config.Image}}' "`$reverse_service_name" 2>/dev/null || true)"

mkdir -p "`$backup_dir"
echo "Backup directory: `$backup_dir"
cp "`$compose_file" "`$backup_dir/docker-compose.yml"
if [ -f "`$override_file" ]; then
    cp "`$override_file" "`$backup_dir/docker-compose.deploy.override.yml"
fi
if [ -f "`$proxy_override_file" ]; then
    cp "`$proxy_override_file" "`$backup_dir/docker-compose.hwdrama-proxy.override.yml"
fi
if [ -f "`$reverse_override_file" ]; then
    cp "`$reverse_override_file" "`$backup_dir/docker-compose.reverse-volcengine.override.yml"
fi
if [ -f "`$reverse_env_file" ]; then
    cp "`$reverse_env_file" "`$backup_dir/reverse-newapi-volcengine.env"
fi
if [ -d "`$proxy_config_dir" ]; then
    tar -C "`$remote_dir" -czf "`$backup_dir/hwdrama-proxy-config.tgz" hwdrama-proxy
fi

if [ "`$skip_backup" != "1" ] && [ "`$hwdrama_proxy_only" != "1" ]; then
    if [ -d "`$remote_dir/data" ]; then
        tar -C "`$remote_dir" -czf "`$backup_dir/data.tgz" data
    fi

    if docker inspect "`$postgres_container" >/dev/null 2>&1; then
        pg_env="`$(docker inspect -f '{{range .Config.Env}}{{println .}}{{end}}' "`$postgres_container")"
        pg_user="`$(printf '%s\n' "`$pg_env" | awk -F= '`$1=="POSTGRES_USER"{print `$2; exit}')"
        pg_db="`$(printf '%s\n' "`$pg_env" | awk -F= '`$1=="POSTGRES_DB"{print `$2; exit}')"
        if [ -z "`$pg_user" ]; then pg_user="postgres"; fi
        if [ -z "`$pg_db" ]; then pg_db="`$pg_user"; fi
        echo "Dumping PostgreSQL database from `$postgres_container..."
        docker exec "`$postgres_container" pg_dump -U "`$pg_user" -d "`$pg_db" -Fc > "`$backup_dir/postgres.dump"
    else
        echo "PostgreSQL container `$postgres_container not found; database dump skipped." >&2
    fi
elif [ "`$hwdrama_proxy_only" = "1" ]; then
    echo "Data/database backups skipped for proxy-only deployment."
else
    echo "Backups skipped by request."
fi

if [ "`$local_docker_build" = "1" ]; then
    echo "Loading image archive..."
    docker load -i "`$image_tar"
else
    source_build_dir="`$remote_dir/builds/source-`$safe_tag"
    case "`$source_build_dir" in
        "`$remote_dir"/builds/source-*) ;;
        *)
            echo "Unexpected source build directory: `$source_build_dir" >&2
            exit 1
            ;;
    esac
    echo "Preparing source build directory: `$source_build_dir"
    rm -rf "`$source_build_dir.tmp"
    mkdir -p "`$source_build_dir.tmp"
    tar -xf "`$source_tar" -C "`$source_build_dir.tmp"
    rm -rf "`$source_build_dir"
    mv "`$source_build_dir.tmp" "`$source_build_dir"
    echo "Building Docker image on remote host: `$image"
    docker build --platform "`$platform" --pull -t "`$image" "`$source_build_dir"
fi

app_env="`$(docker inspect -f '{{range .Config.Env}}{{println .}}{{end}}' "`$service_name")"
sql_dsn="`$(printf '%s\n' "`$app_env" | sed -n 's/^SQL_DSN=//p' | head -n 1)"
tz_value="`$(printf '%s\n' "`$app_env" | sed -n 's/^TZ=//p' | head -n 1)"
if [ "`$hwdrama_proxy_enabled" = "1" ]; then
    if [ -z "`$sql_dsn" ]; then
        echo "Cannot find SQL_DSN in `$service_name container environment." >&2
        exit 1
    fi
    mkdir -p "`$proxy_config_dir"
    umask 077
    {
        printf 'SQL_DSN=%s\n' "`$sql_dsn"
        if [ -n "`$tz_value" ]; then
            printf 'TZ=%s\n' "`$tz_value"
        fi
        printf 'HWD_PROXY_PORT=%s\n' "`$hwdrama_proxy_port"
        printf 'HWD_PROXY_REQUEST_TIMEOUT_SECONDS=%s\n' "`$hwdrama_proxy_timeout"
        printf 'HWD_PROXY_ROUTES_CONFIG=/app/hwdrama-proxy/routes.yml\n'
        printf 'HWD_PROXY_SECRETS_FILE=/app/hwdrama-proxy/secrets.env\n'
    } > "`$proxy_env_file"
    chmod 600 "`$proxy_env_file"
    if [ ! -f "`$proxy_secrets_file" ]; then
        touch "`$proxy_secrets_file"
        chmod 600 "`$proxy_secrets_file"
    fi
    if [ ! -f "`$proxy_routes_file" ]; then
        cat > "`$proxy_routes_file" <<EOF
version: 1

actions:
  ark_assets_list:
    downstream_method: GET
    downstream_path: /api/v3/ark/assets
    default_upstream_method: GET
    default_upstream_path: /api/v3/ark/assets
  ark_assets_create:
    downstream_method: POST
    downstream_path: /api/v3/ark/assets
    default_upstream_method: POST
    default_upstream_path: /api/v3/ark/assets
  ark_assets_get:
    downstream_method: GET
    downstream_path: /api/v3/ark/assets/{asset_id}
    default_upstream_method: GET
    default_upstream_path: /api/v3/ark/assets/{asset_id}
  ark_asset_groups_create:
    downstream_method: POST
    downstream_path: /api/v3/ark/assets/groups
    default_upstream_method: POST
    default_upstream_path: /api/v3/ark/assets/groups
  ark_real_person_assets_create:
    downstream_method: POST
    downstream_path: /api/v3/ark/real-person/assets
    default_upstream_method: POST
    default_upstream_path: /api/v3/ark/real-person/assets
  ark_real_person_assets_get:
    downstream_method: GET
    downstream_path: /api/v3/ark/real-person/assets/{asset_id}
    default_upstream_method: GET
    default_upstream_path: /api/v3/ark/real-person/assets/{asset_id}
  ark_real_person_validate_sessions_create:
    downstream_method: POST
    downstream_path: /api/v3/ark/real-person/validate/sessions
    default_upstream_method: POST
    default_upstream_path: /api/v3/ark/real-person/validate/sessions
  ark_real_person_validate_sessions_get:
    downstream_method: GET
    downstream_path: /api/v3/ark/real-person/validate/sessions/{session_id}
    default_upstream_method: GET
    default_upstream_path: /api/v3/ark/real-person/validate/sessions/{session_id}
  seedance_open_create_asset:
    downstream_method: POST
    downstream_path: /api/v3/open/CreateAsset
    default_upstream_method: POST
    default_upstream_path: /api/v3/open/CreateAsset
  seedance_open_get_asset:
    downstream_method: POST
    downstream_path: /api/v3/open/GetAsset
    default_upstream_method: POST
    default_upstream_path: /api/v3/open/GetAsset

routes:
  - name: default-hwdrama-compatible
    all_api_keys: true
    channel_id: 1
    models:
      - "*"
    upstream_base_url: https://foxtoken.linkomobile.com
    upstream_api_key_env: HWD_HWDRAMA_API_KEY
    asset_namespace_id: hwdrama-default
    enabled_actions:
      - ark_assets_list
      - ark_assets_create
      - ark_assets_get
      - ark_asset_groups_create
      - ark_real_person_assets_create
      - ark_real_person_assets_get
      - ark_real_person_validate_sessions_create
      - ark_real_person_validate_sessions_get
      - seedance_open_create_asset
      - seedance_open_get_asset
EOF
        chmod 600 "`$proxy_routes_file"
    fi
    if ! docker run --rm --entrypoint /hwdrama-proxy --env-file "`$proxy_env_file" -v "`$proxy_config_dir:/app/hwdrama-proxy:ro" "`$image" config validate --config /app/hwdrama-proxy/routes.yml --secrets /app/hwdrama-proxy/secrets.env; then
        echo "Hwdrama proxy config validation failed. Check routes.yml and every upstream_api_key_env value in secrets.env." >&2
        exit 1
    fi
fi

if [ "`$reverse_volcengine_enabled" = "1" ]; then
    umask 077
    {
        printf 'NEW_API_BASE_URL=%s\n' "`$reverse_volcengine_upstream_base_url"
        printf 'PORT=%s\n' "`$reverse_volcengine_port"
        printf 'NEW_API_TIMEOUT_SECONDS=%s\n' "`$reverse_volcengine_timeout"
        if [ -n "`$tz_value" ]; then
            printf 'TZ=%s\n' "`$tz_value"
        fi
    } > "`$reverse_env_file"
    chmod 600 "`$reverse_env_file"
fi

if [ "`$hwdrama_proxy_only" != "1" ]; then
    cat > "`$active_override_file" <<EOF
services:
  `$service_name:
    image: `$image
EOF
else
    cat > "`$active_override_file" <<EOF
services:
EOF
fi

if [ "`$hwdrama_proxy_enabled" = "1" ]; then
    cat >> "`$active_override_file" <<EOF
  `$proxy_service_name:
    image: `$image
    entrypoint:
      - /hwdrama-proxy
    command:
      - --log-dir
      - /app/logs
    env_file:
      - ./hwdrama-proxy.env
    ports:
      - "127.0.0.1:`$hwdrama_proxy_port:`$hwdrama_proxy_port"
    volumes:
      - ./logs:/app/logs
      - ./hwdrama-proxy:/app/hwdrama-proxy
    networks:
      - new-api-network
    restart: unless-stopped
    healthcheck:
      test:
        - CMD-SHELL
        - wget -q -O - http://localhost:`$hwdrama_proxy_port/healthz | grep -q '^ok'
      interval: 30s
      timeout: 10s
      retries: 5
      start_period: 10s
EOF
fi

if [ "`$reverse_volcengine_enabled" = "1" ]; then
    cat >> "`$active_override_file" <<EOF
  `$reverse_service_name:
    image: `$image
    container_name: `$reverse_service_name
    entrypoint:
      - /reverse-newapi-volcengine
    env_file:
      - ./reverse-newapi-volcengine.env
    ports:
      - "127.0.0.1:`$reverse_volcengine_port:`$reverse_volcengine_port"
    networks:
      - new-api-network
    restart: unless-stopped
    healthcheck:
      test:
        - CMD-SHELL
        - wget -q -O - http://localhost:`$reverse_volcengine_port/healthz | grep -q '"status":"ok"'
      interval: 30s
      timeout: 10s
      retries: 5
      start_period: 10s
EOF
fi

echo "Recreating application service(s)..."
services_to_recreate=()
if [ "`$hwdrama_proxy_only" != "1" ]; then
    services_to_recreate+=("`$service_name")
fi
if [ "`$hwdrama_proxy_enabled" = "1" ]; then
    services_to_recreate+=("`$proxy_service_name")
fi
if [ "`$reverse_volcengine_enabled" = "1" ]; then
    services_to_recreate+=("`$reverse_service_name")
fi
if [ "`$hwdrama_proxy_only" = "1" ] && [ "`$hwdrama_proxy_enabled" != "1" ] && [ "`$reverse_volcengine_enabled" != "1" ]; then
    echo "No sidecar service selected for proxy-only deployment." >&2
    exit 1
fi
if [ "`$hwdrama_proxy_only" = "1" ]; then
    compose up -d --no-deps "`${services_to_recreate[@]}"
else
    compose up -d --no-deps "`${services_to_recreate[@]}"
fi

deadline=`$((SECONDS + health_timeout))
ok=0
while [ `$SECONDS -lt `$deadline ]; do
    new_api_ok=0
    proxy_ok=0
    reverse_ok=0
    if [ "`$hwdrama_proxy_only" = "1" ]; then
        new_api_ok=1
    elif curl -fsS http://127.0.0.1:3000/api/status 2>/dev/null | grep -Eq '"success"[[:space:]]*:[[:space:]]*true'; then
        new_api_ok=1
    fi
    if [ "`$hwdrama_proxy_enabled" != "1" ] || curl -fsS "http://127.0.0.1:`$hwdrama_proxy_port/healthz" 2>/dev/null | grep -q '^ok'; then
        proxy_ok=1
    fi
    if [ "`$reverse_volcengine_enabled" != "1" ] || curl -fsS "http://127.0.0.1:`$reverse_volcengine_port/healthz" 2>/dev/null | grep -q '"status":"ok"'; then
        reverse_ok=1
    fi
    if [ "`$new_api_ok" = "1" ] && [ "`$proxy_ok" = "1" ] && [ "`$reverse_ok" = "1" ]; then
        ok=1
        break
    fi
    health="healthy"
    if [ "`$hwdrama_proxy_only" != "1" ]; then
        health="`$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "`$service_name" 2>/dev/null || true)"
    fi
    proxy_health="`$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "`$proxy_service_name" 2>/dev/null || true)"
    reverse_health="`$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "`$reverse_service_name" 2>/dev/null || true)"
    if [ "`$health" = "healthy" ] && { [ "`$hwdrama_proxy_enabled" != "1" ] || [ "`$proxy_health" = "healthy" ]; } && { [ "`$reverse_volcengine_enabled" != "1" ] || [ "`$reverse_health" = "healthy" ]; }; then
        ok=1
        break
    fi
    sleep 5
done

if [ "`$ok" != "1" ]; then
    echo "Health check failed. Recent logs:" >&2
    if [ "`$hwdrama_proxy_only" != "1" ]; then
        docker logs --tail 120 "`$service_name" >&2 || true
    fi
    if [ "`$hwdrama_proxy_enabled" = "1" ]; then
        docker logs --tail 120 "`$proxy_service_name" >&2 || true
    fi
    if [ "`$reverse_volcengine_enabled" = "1" ]; then
        docker logs --tail 120 "`$reverse_service_name" >&2 || true
    fi
    if [ "`$no_rollback" != "1" ]; then
        if [ "`$hwdrama_proxy_only" = "1" ]; then
            if [ -n "`$previous_proxy_image" ]; then
                echo "Rolling back `$proxy_service_name to previous image `$previous_proxy_image..." >&2
                cat > "`$active_override_file" <<EOF
services:
  `$proxy_service_name:
    image: `$previous_proxy_image
    entrypoint:
      - /hwdrama-proxy
    command:
      - --log-dir
      - /app/logs
    env_file:
      - ./hwdrama-proxy.env
    ports:
      - "127.0.0.1:`$hwdrama_proxy_port:`$hwdrama_proxy_port"
    volumes:
      - ./logs:/app/logs
      - ./hwdrama-proxy:/app/hwdrama-proxy
    networks:
      - new-api-network
    restart: unless-stopped
EOF
                compose up -d --no-deps "`$proxy_service_name"
            else
                echo "Removing failed `$proxy_service_name container; no previous proxy image exists." >&2
                docker rm -f "`$proxy_service_name" >/dev/null 2>&1 || true
            fi
        else
            echo "Rolling back to previous image `$previous_image..." >&2
            if [ -n "`$previous_proxy_image" ]; then
            cat > "`$override_file" <<EOF
services:
  `$service_name:
    image: `$previous_image
  `$proxy_service_name:
    image: `$previous_proxy_image
    entrypoint:
      - /hwdrama-proxy
    command:
      - --log-dir
      - /app/logs
    env_file:
      - ./hwdrama-proxy.env
    ports:
      - "127.0.0.1:`$hwdrama_proxy_port:`$hwdrama_proxy_port"
    volumes:
      - ./logs:/app/logs
      - ./hwdrama-proxy:/app/hwdrama-proxy
    networks:
      - new-api-network
    restart: unless-stopped
EOF
            compose up -d --no-deps "`$service_name" "`$proxy_service_name"
            else
            cat > "`$override_file" <<EOF
services:
  `$service_name:
    image: `$previous_image
EOF
            compose up -d --no-deps "`$service_name"
            docker rm -f "`$proxy_service_name" >/dev/null 2>&1 || true
            fi
        fi
    fi
    exit 1
fi

if [ "`$nginx_update_enabled" = "1" ] && { [ "`$hwdrama_proxy_enabled" = "1" ] || [ "`$reverse_volcengine_enabled" = "1" ]; }; then
    nginx_site="/etc/nginx/sites-available/llm.ai.nexus-reach.com.conf"
    nginx_common_snippet="/etc/nginx/snippets/hwdrama-proxy-common.conf"
    nginx_reverse_snippet="/etc/nginx/snippets/reverse-volcengine-common.conf"
    nginx_locations_snippet="/etc/nginx/snippets/hwdrama-proxy-locations.conf"
    nginx_backup="`$backup_dir/llm.ai.nexus-reach.com.conf"
    sudo cp "`$nginx_site" "`$nginx_backup"
    sudo tee "`$nginx_common_snippet" >/dev/null <<EOF
proxy_pass http://127.0.0.1:`$hwdrama_proxy_port;
proxy_http_version 1.1;
proxy_set_header Host \`$host;
proxy_set_header X-Real-IP \`$remote_addr;
proxy_set_header X-Forwarded-For \`$proxy_add_x_forwarded_for;
proxy_set_header X-Forwarded-Proto \`$scheme;
proxy_set_header X-Forwarded-Host \`$host;
proxy_set_header X-Forwarded-Port 443;
proxy_set_header Upgrade \`$http_upgrade;
proxy_set_header Connection \`$connection_upgrade;
proxy_connect_timeout 60s;
proxy_send_timeout 3600s;
proxy_read_timeout 3600s;
proxy_buffering off;
proxy_request_buffering off;
proxy_cache off;
proxy_next_upstream off;
EOF
    sudo tee "`$nginx_reverse_snippet" >/dev/null <<EOF
proxy_pass http://127.0.0.1:`$reverse_volcengine_port;
proxy_http_version 1.1;
proxy_set_header Host \`$host;
proxy_set_header X-Real-IP \`$remote_addr;
proxy_set_header X-Forwarded-For \`$proxy_add_x_forwarded_for;
proxy_set_header X-Forwarded-Proto \`$scheme;
proxy_set_header X-Forwarded-Host \`$host;
proxy_set_header X-Forwarded-Port 443;
proxy_set_header Connection "";
client_max_body_size 512m;
client_body_timeout 600s;
proxy_connect_timeout 60s;
proxy_send_timeout 3600s;
proxy_read_timeout 3600s;
proxy_buffering off;
proxy_request_buffering off;
proxy_cache off;
proxy_next_upstream off;
EOF
    sudo tee "`$nginx_locations_snippet" >/dev/null <<EOF
location = /api/v3/contents/generations/tasks { include `$nginx_reverse_snippet; }
location ^~ /api/v3/contents/generations/tasks/ { include `$nginx_reverse_snippet; }
location ^~ /api/v3/ark/ { include `$nginx_common_snippet; }
location ^~ /api/v3/open/ { include `$nginx_common_snippet; }
EOF
    if ! sudo grep -q "hwdrama-proxy-locations.conf" "`$nginx_site"; then
        tmp_nginx="`$(mktemp)"
        awk '
            BEGIN { inserted = 0; ssl_server = 0 }
            /listen[[:space:]].*443/ { ssl_server = 1 }
            ssl_server && !inserted && /^[[:space:]]*location[[:space:]]+\/[[:space:]]*\{/ {
                print "    include /etc/nginx/snippets/hwdrama-proxy-locations.conf;"
                print ""
                inserted = 1
            }
            { print }
        ' "`$nginx_site" > "`$tmp_nginx"
        if ! grep -q "hwdrama-proxy-locations.conf" "`$tmp_nginx"; then
            rm -f "`$tmp_nginx"
            echo "Failed to insert hwdrama proxy nginx include." >&2
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

echo "Deployment healthy."
docker ps --filter "name=^/`$service_name`$" --format 'table {{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Ports}}'
if [ "`$hwdrama_proxy_enabled" = "1" ]; then
    docker ps --filter "name=^/`$proxy_service_name`$" --format 'table {{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Ports}}'
fi
if [ "`$reverse_volcengine_enabled" = "1" ]; then
    docker ps --filter "name=^/`$reverse_service_name`$" --format 'table {{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Ports}}'
fi
echo "Backup directory: `$backup_dir"
"@

    Write-Host "Deploying on remote host..."
    ($remoteScript -replace "`r`n", "`n") | & ssh $RemoteHost "bash -s"
    if ($LASTEXITCODE -ne 0) {
        throw "Remote deployment failed."
    }

    if ($LocalDockerBuild) {
        if (-not $KeepLocalImageTar) {
            Remove-Item -LiteralPath $localTar -Force
        }
    } elseif (-not $KeepLocalSourceTar) {
        Remove-Item -LiteralPath $localSourceTar -Force
    }

    Write-Host "Deployment completed: $image"
}
finally {
    Pop-Location
}
