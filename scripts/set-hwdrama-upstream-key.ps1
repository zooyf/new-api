param(
    [Parameter(Mandatory = $true)]
    [ValidatePattern('^HWD_[A-Z0-9_]+_API_KEY$')]
    [string]$VariableName,

    [string]$UpstreamApiKey = "",

    [ValidatePattern('^[A-Za-z0-9_.@-]+$')]
    [string]$SshHost = "nexus-sg",

    [string]$UpstreamBaseUrl = "https://foxtoken.linkomobile.com",

    [string]$Model = "doubao-seedance-2-0-filter-off",

    [ValidateSet("GET", "POST")]
    [string]$AuthCheckMethod = "POST",

    [string]$AuthCheckPath = "/api/v3/open/GetAsset",

    [string]$AuthCheckBody = "",

    [ValidateRange(0, [int]::MaxValue)]
    [int]$CustomerTokenId = 0,

    [string]$DownstreamBaseUrl = "https://llm.ai.nexus-reach.com",

    [string]$RemoteSecretsFile = "/opt/new-api/hwdrama-proxy/secrets.env",

    [string]$RemoteRoutesFile = "/opt/new-api/hwdrama-proxy/routes.yml",

    [string]$ProxyContainer = "new-api-hwdrama-proxy-1",

    [string]$DatabaseContainer = "new-api-postgres",

    [string]$DatabaseUser = "newapi",

    [string]$DatabaseName = "newapi"
)

$ErrorActionPreference = "Stop"

function ConvertTo-PlainText {
    param([Security.SecureString]$SecureValue)

    $pointer = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($SecureValue)
    try {
        return [Runtime.InteropServices.Marshal]::PtrToStringBSTR($pointer)
    } finally {
        [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($pointer)
    }
}

function Get-Sha256 {
    param([string]$Value)

    $sha = [Security.Cryptography.SHA256]::Create()
    try {
        return ($sha.ComputeHash([Text.Encoding]::UTF8.GetBytes($Value)) |
            ForEach-Object { $_.ToString("x2") }) -join ""
    } finally {
        $sha.Dispose()
    }
}

function ConvertTo-Base64Utf8 {
    param([string]$Value)

    return [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($Value))
}

function Invoke-SshInput {
    param(
        [Parameter(Mandatory = $true)]
        [string]$RemoteCommand,

        [Parameter(Mandatory = $true)]
        [string]$InputText
    )

    $startInfo = [Diagnostics.ProcessStartInfo]::new()
    $startInfo.FileName = "ssh"
    $startInfo.Arguments = "$SshHost `"$RemoteCommand`""
    $startInfo.UseShellExecute = $false
    $startInfo.RedirectStandardInput = $true
    $startInfo.RedirectStandardOutput = $true
    $startInfo.RedirectStandardError = $true
    $startInfo.CreateNoWindow = $true

    $process = [Diagnostics.Process]::new()
    $process.StartInfo = $startInfo
    try {
        if (-not $process.Start()) {
            throw "Failed to start ssh."
        }
        $process.StandardInput.Write($InputText)
        $process.StandardInput.Close()
        $stdout = $process.StandardOutput.ReadToEnd()
        $stderr = $process.StandardError.ReadToEnd()
        $process.WaitForExit()
        return [pscustomobject]@{
            ExitCode = $process.ExitCode
            Stdout   = $stdout
            Stderr   = $stderr
        }
    } finally {
        $process.Dispose()
    }
}

if ([string]::IsNullOrWhiteSpace($UpstreamApiKey)) {
    $secureKey = Read-Host "Enter the full upstream API key for $VariableName" -AsSecureString
    $UpstreamApiKey = ConvertTo-PlainText $secureKey
}
$UpstreamApiKey = $UpstreamApiKey.Trim()
if ([string]::IsNullOrWhiteSpace($UpstreamApiKey)) {
    throw "The upstream API key cannot be empty."
}
if ($UpstreamApiKey -match "[\r\n]") {
    throw "The upstream API key cannot contain CR or LF characters."
}
if ($Model -notmatch '^[A-Za-z0-9._:-]+$') {
    throw "Model contains unsupported characters."
}
if (-not $AuthCheckPath.StartsWith('/') -or $AuthCheckPath.Contains('?') -or $AuthCheckPath.Contains('#')) {
    throw "AuthCheckPath must be an absolute path without query or fragment."
}

$expectedLength = [Text.Encoding]::UTF8.GetByteCount($UpstreamApiKey)
$expectedHash = Get-Sha256 $UpstreamApiKey
$encodedKey = ConvertTo-Base64Utf8 $UpstreamApiKey
$runId = [Guid]::NewGuid().ToString("N")
$remoteKeyFile = "/tmp/hwdrama-key-$runId"

# Write the Base64 payload directly to ssh stdin without PowerShell pipeline
# newline conversion. The remote file is created with mode 600.
$upload = Invoke-SshInput `
    -RemoteCommand "umask 077; base64 -d > $remoteKeyFile" `
    -InputText $encodedKey
if ($upload.ExitCode -ne 0) {
    throw "Failed to upload the candidate key: $($upload.Stderr.Trim())"
}

$template = @'
set -euo pipefail

decode() {
  printf '%s' "$1" | base64 -d
}

variable_name="__VARIABLE_NAME__"
key_file="__KEY_FILE__"
secrets_file="$(decode '__SECRETS_FILE_B64__')"
routes_file="$(decode '__ROUTES_FILE_B64__')"
upstream_base_url="$(decode '__UPSTREAM_BASE_URL_B64__')"
downstream_base_url="$(decode '__DOWNSTREAM_BASE_URL_B64__')"
model="__MODEL__"
auth_check_method="__AUTH_CHECK_METHOD__"
auth_check_path="$(decode '__AUTH_CHECK_PATH_B64__')"
auth_check_body="$(decode '__AUTH_CHECK_BODY_B64__')"
proxy_container="__PROXY_CONTAINER__"
db_container="__DB_CONTAINER__"
db_user="__DB_USER__"
db_name="__DB_NAME__"
customer_token_id="__CUSTOMER_TOKEN_ID__"
expected_length="__EXPECTED_LENGTH__"
expected_hash="__EXPECTED_HASH__"
run_id="__RUN_ID__"

config_dir=$(dirname "$secrets_file")
candidate_file="$config_dir/secrets.env.candidate.$run_id"
backup_file="$secrets_file.bak.$(date +%Y%m%d%H%M%S)"
auth_response=$(mktemp)
proxy_response=$(mktemp)
installed=0

cleanup() {
  rc=$?
  if [ "$rc" -ne 0 ] && [ "$installed" = "1" ] && [ -f "$backup_file" ]; then
    echo "Update failed after installation; restoring $backup_file" >&2
    sudo cp -p "$backup_file" "$secrets_file"
    docker exec "$proxy_container" /hwdrama-proxy config reload >/dev/null 2>&1 || true
  fi
  rm -f "$key_file" "$auth_response" "$proxy_response"
  sudo rm -f "$candidate_file"
  exit "$rc"
}
trap cleanup EXIT

test -f "$key_file"
test -f "$secrets_file"
test -f "$routes_file"

actual_length=$(wc -c < "$key_file" | tr -d '[:space:]')
actual_hash=$(sha256sum "$key_file" | awk '{print $1}')
echo "candidate_length=$actual_length"
echo "candidate_sha256=$actual_hash"
if [ "$actual_length" != "$expected_length" ] || [ "$actual_hash" != "$expected_hash" ]; then
  echo "Candidate key differs from the local input." >&2
  exit 10
fi

# Validate the credential without creating an asset. A valid credential may
# return a business-level 4xx for deliberately nonexistent data; 401 is a
# credential failure. GET checks are useful for list endpoints such as Ark.
if [ -z "$auth_check_body" ] && [ "$auth_check_method" = "POST" ]; then
  auth_check_body=$(printf '{"model":"%s","Id":"asset-key-validation-does-not-exist"}' "$model")
fi
if [ "$auth_check_method" = "GET" ]; then
  auth_http=$(curl -sS --connect-timeout 15 --max-time 60 \
    -o "$auth_response" -w '%{http_code}' \
    -X GET "$upstream_base_url$auth_check_path" \
    -H "Authorization: Bearer $(cat "$key_file")" \
    -H 'Accept: application/json')
else
  auth_http=$(curl -sS --connect-timeout 15 --max-time 60 \
    -o "$auth_response" -w '%{http_code}' \
    -X POST "$upstream_base_url$auth_check_path" \
    -H "Authorization: Bearer $(cat "$key_file")" \
    -H 'Content-Type: application/json' \
    --data-binary "$auth_check_body")
fi
echo "upstream_auth_http=$auth_http"
echo "upstream_auth_response=$(cat "$auth_response")"
if [ "$auth_http" = "401" ] || grep -qi 'invalid token' "$auth_response"; then
  echo "Upstream rejected the candidate key." >&2
  exit 11
fi

sudo install -o root -g root -m 600 /dev/null "$candidate_file"
sudo awk -F= -v target="$variable_name" '$1 != target { print }' "$secrets_file" | sudo tee "$candidate_file" >/dev/null
{
  printf '%s=' "$variable_name"
  cat "$key_file"
  printf '\n'
} | sudo tee -a "$candidate_file" >/dev/null
sudo chmod 600 "$candidate_file"

line_count=$(sudo grep -c "^${variable_name}=" "$candidate_file")
if [ "$line_count" != "1" ]; then
  echo "Candidate secrets file contains $line_count entries for $variable_name." >&2
  exit 12
fi
stored_value=$(sudo sed -n "s/^${variable_name}=//p" "$candidate_file")
stored_length=$(printf '%s' "$stored_value" | wc -c | tr -d '[:space:]')
stored_hash=$(printf '%s' "$stored_value" | sha256sum | awk '{print $1}')
echo "stored_length=$stored_length"
echo "stored_sha256=$stored_hash"
if [ "$stored_length" != "$expected_length" ] || [ "$stored_hash" != "$expected_hash" ]; then
  echo "Candidate secrets file changed the key bytes." >&2
  exit 13
fi

candidate_name=$(basename "$candidate_file")
docker exec "$proxy_container" /hwdrama-proxy config validate \
  --config /app/hwdrama-proxy/routes.yml \
  --secrets "/app/hwdrama-proxy/$candidate_name"

sudo cp -p "$secrets_file" "$backup_file"
sudo mv "$candidate_file" "$secrets_file"
sudo chmod 600 "$secrets_file"
installed=1

docker exec "$proxy_container" /hwdrama-proxy config reload
health=$(curl -fsS http://127.0.0.1:3001/healthz)
echo "proxy_health=$health"

if [ "$customer_token_id" -gt 0 ]; then
  customer_key=$(docker exec "$db_container" psql -U "$db_user" -d "$db_name" -tAc \
    "select key from tokens where id=$customer_token_id and deleted_at is null limit 1")
  customer_key=$(printf '%s' "$customer_key" | tr -d '[:space:]')
  if [ -z "$customer_key" ]; then
    echo "Customer token ID $customer_token_id was not found." >&2
    exit 14
  fi
  case "$customer_key" in
    sk-*) ;;
    *) customer_key="sk-$customer_key" ;;
  esac

  if [ "$auth_check_method" = "GET" ]; then
    proxy_http=$(curl -sS --connect-timeout 15 --max-time 60 \
      -o "$proxy_response" -w '%{http_code}' \
      -X GET "$downstream_base_url$auth_check_path" \
      -H "Authorization: Bearer $customer_key" \
      -H 'Accept: application/json')
  else
    proxy_http=$(curl -sS --connect-timeout 15 --max-time 60 \
      -o "$proxy_response" -w '%{http_code}' \
      -X POST "$downstream_base_url$auth_check_path" \
      -H "Authorization: Bearer $customer_key" \
      -H 'Content-Type: application/json' \
      --data-binary "$auth_check_body")
  fi
  echo "downstream_auth_http=$proxy_http"
  echo "downstream_auth_response=$(cat "$proxy_response")"
  if [ "$proxy_http" = "401" ] || grep -Eqi 'invalid token|no_upstream_route' "$proxy_response"; then
    echo "Downstream route did not accept the updated credential." >&2
    exit 15
  fi
fi

installed=0
echo "backup_file=$backup_file"
echo "update_status=success"
'@

$remoteScript = $template.
    Replace('__VARIABLE_NAME__', $VariableName).
    Replace('__KEY_FILE__', $remoteKeyFile).
    Replace('__SECRETS_FILE_B64__', (ConvertTo-Base64Utf8 $RemoteSecretsFile)).
    Replace('__ROUTES_FILE_B64__', (ConvertTo-Base64Utf8 $RemoteRoutesFile)).
    Replace('__UPSTREAM_BASE_URL_B64__', (ConvertTo-Base64Utf8 $UpstreamBaseUrl.TrimEnd('/'))).
    Replace('__DOWNSTREAM_BASE_URL_B64__', (ConvertTo-Base64Utf8 $DownstreamBaseUrl.TrimEnd('/'))).
    Replace('__MODEL__', $Model).
    Replace('__AUTH_CHECK_METHOD__', $AuthCheckMethod).
    Replace('__AUTH_CHECK_PATH_B64__', (ConvertTo-Base64Utf8 $AuthCheckPath)).
    Replace('__AUTH_CHECK_BODY_B64__', (ConvertTo-Base64Utf8 $AuthCheckBody)).
    Replace('__PROXY_CONTAINER__', $ProxyContainer).
    Replace('__DB_CONTAINER__', $DatabaseContainer).
    Replace('__DB_USER__', $DatabaseUser).
    Replace('__DB_NAME__', $DatabaseName).
    Replace('__CUSTOMER_TOKEN_ID__', $CustomerTokenId.ToString()).
    Replace('__EXPECTED_LENGTH__', $expectedLength.ToString()).
    Replace('__EXPECTED_HASH__', $expectedHash).
    Replace('__RUN_ID__', $runId)

try {
    $result = Invoke-SshInput -RemoteCommand "bash -s" -InputText $remoteScript
    if (-not [string]::IsNullOrWhiteSpace($result.Stdout)) {
        Write-Output $result.Stdout.TrimEnd()
    }
    if ($result.ExitCode -ne 0) {
        $errorText = $result.Stderr.Trim()
        if ([string]::IsNullOrWhiteSpace($errorText)) {
            $errorText = "remote exit code $($result.ExitCode)"
        }
        throw "Upstream key update failed: $errorText"
    }
    if (-not [string]::IsNullOrWhiteSpace($result.Stderr)) {
        Write-Warning $result.Stderr.Trim()
    }
} finally {
    $UpstreamApiKey = $null
    $encodedKey = $null
}
