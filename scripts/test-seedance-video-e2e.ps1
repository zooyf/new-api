param(
    [string]$SshHost = "nexus-sg",
    [string]$BaseUrl = "https://llm.ai.nexus-reach.com",
    [string]$TokenName = "fission test",
    [string]$Model = "doubao-seedance-2-0-filter-off",
    [string]$AssetUrl = "asset://asset-20260708115604-8nrb2",
    [string]$Ratio = "9:16",
    [string]$Resolution = "480p",
    [int]$DurationSeconds = 5,
    [string]$Role = "first_frame",
    [string]$Prompt = "Create a 5 second vertical 9:16 image-to-video test from the reference asset. Keep the subject stable with subtle natural motion, no camera movement, no text, no audio.",
    [int]$PollIntervalSeconds = 5,
    [int]$TimeoutSeconds = 480,
    [string]$RemoteResultDir = "/tmp/new-api-seedance-e2e",
    [switch]$SkipContentCheck,
    [switch]$SkipRatioAssert
)

$ErrorActionPreference = "Stop"

function ConvertTo-Base64Utf8 {
    param([string]$Value)
    return [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($Value))
}

if ($DurationSeconds -le 0) {
    throw "DurationSeconds must be greater than 0."
}
if ($PollIntervalSeconds -le 0) {
    throw "PollIntervalSeconds must be greater than 0."
}
if ($TimeoutSeconds -lt $PollIntervalSeconds) {
    throw "TimeoutSeconds must be greater than or equal to PollIntervalSeconds."
}

$remoteScript = @'
set -euo pipefail

decode_b64() {
  printf '%s' "$1" | base64 -d
}

BASE_URL="$(decode_b64 "$BASE_URL_B64")"
TOKEN_NAME="$(decode_b64 "$TOKEN_NAME_B64")"
MODEL="$(decode_b64 "$MODEL_B64")"
ASSET_URL="$(decode_b64 "$ASSET_URL_B64")"
RATIO="$(decode_b64 "$RATIO_B64")"
RESOLUTION="$(decode_b64 "$RESOLUTION_B64")"
ROLE="$(decode_b64 "$ROLE_B64")"
PROMPT="$(decode_b64 "$PROMPT_B64")"
REMOTE_RESULT_DIR="$(decode_b64 "$REMOTE_RESULT_DIR_B64")"
DURATION_SECONDS="${DURATION_SECONDS}"
POLL_INTERVAL_SECONDS="${POLL_INTERVAL_SECONDS}"
TIMEOUT_SECONDS="${TIMEOUT_SECONDS}"
CHECK_CONTENT="${CHECK_CONTENT}"
ASSERT_RATIO="${ASSERT_RATIO}"

RUN_DIR="${REMOTE_RESULT_DIR%/}/$(date -u +%Y%m%dT%H%M%SZ)-$$"
mkdir -p "$RUN_DIR"
REQUEST_BODY_FILE="$RUN_DIR/request.json"
SUBMIT_RESPONSE_FILE="$RUN_DIR/submit.json"
POLL_RESPONSE_FILE="$RUN_DIR/poll.json"
PARSER_FILE="$RUN_DIR/parse_response.py"
START_TS="$(date +%s)"

cat >"$PARSER_FILE" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as f:
    body = json.load(f)

data = body.get("data", body) if isinstance(body, dict) else {}
nested = data.get("data", {}) if isinstance(data, dict) else {}
usage = nested.get("usage") or data.get("usage") or {}
content = nested.get("content") or data.get("content") or {}

summary = {
    "task_id": data.get("task_id") or data.get("id") or body.get("task_id") or body.get("id"),
    "status": data.get("status") or nested.get("status") or body.get("status") or "",
    "upstream_status": nested.get("status") or "",
    "progress": data.get("progress") or nested.get("progress") or "",
    "upstream_task_id": nested.get("id") or "",
    "ratio": nested.get("ratio") or data.get("ratio") or "",
    "resolution": nested.get("resolution") or data.get("resolution") or "",
    "duration": nested.get("duration") or data.get("duration") or "",
    "total_tokens": usage.get("total_tokens") or "",
    "completion_tokens": usage.get("completion_tokens") or "",
    "result_url": data.get("result_url") or content.get("video_url") or "",
    "fail_reason": data.get("fail_reason") or body.get("message") or "",
}

print(json.dumps(summary, ensure_ascii=False))
PY

TOKEN_ROW="$(docker exec -i new-api-postgres psql -U newapi -d newapi \
  -v token_name="$TOKEN_NAME" -F $'\t' -A -t <<'SQL'
select t.id,t.key,t."group",coalesce(u.username,''),t.user_id
from tokens t
left join users u on u.id = t.user_id
where t.name = :'token_name' and t.deleted_at is null
order by t.id desc
limit 1;
SQL
)"

if [ -z "$TOKEN_ROW" ]; then
  echo "missing_token_name=$TOKEN_NAME" >&2
  exit 1
fi

IFS=$'\t' read -r TOKEN_ID KEY_RAW TOKEN_GROUP USERNAME USER_ID <<<"$TOKEN_ROW"
KEY_RAW="$(printf '%s' "$KEY_RAW" | tr -d '[:space:]')"
AUTH_KEY="$KEY_RAW"
case "$AUTH_KEY" in
  sk-*) ;;
  *) AUTH_KEY="sk-${AUTH_KEY}" ;;
esac

export MODEL PROMPT DURATION_SECONDS RESOLUTION RATIO ASSET_URL ROLE
python3 - <<'PY' >"$REQUEST_BODY_FILE"
import json
import os

asset_url = os.environ["ASSET_URL"]
metadata = {
    "ratio": os.environ["RATIO"],
    "generate_audio": False,
}
if asset_url:
    metadata["content"] = [
        {
            "type": "image_url",
            "role": os.environ["ROLE"],
            "image_url": {
                "url": asset_url,
            },
        }
    ]

body = {
    "model": os.environ["MODEL"],
    "prompt": os.environ["PROMPT"],
    "duration": int(os.environ["DURATION_SECONDS"]),
    "resolution": os.environ["RESOLUTION"],
    "metadata": metadata,
}

print(json.dumps(body, ensure_ascii=False, indent=2))
PY

echo "run_dir=$RUN_DIR"
echo "token_id=$TOKEN_ID"
echo "token_name=$TOKEN_NAME"
echo "token_group=$TOKEN_GROUP"
echo "username=$USERNAME"
echo "request_body_path=$REQUEST_BODY_FILE"
echo "request_body_begin"
cat "$REQUEST_BODY_FILE"
echo
echo "request_body_end"

SUBMIT_HTTP="$(curl -sS -o "$SUBMIT_RESPONSE_FILE" -w "%{http_code}" \
  -X POST "$BASE_URL/v1/video/generations" \
  -H "Authorization: Bearer ${AUTH_KEY}" \
  -H "Content-Type: application/json" \
  --data-binary @"$REQUEST_BODY_FILE")"

echo "submit_http=$SUBMIT_HTTP"
echo "submit_response_path=$SUBMIT_RESPONSE_FILE"
cat "$SUBMIT_RESPONSE_FILE"
echo

case "$SUBMIT_HTTP" in
  2*) ;;
  *) echo "submit_failed" >&2; exit 1 ;;
esac

TASK_ID="$(python3 - "$SUBMIT_RESPONSE_FILE" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as f:
    body = json.load(f)

task_id = body.get("task_id") or body.get("id") or body.get("data", {}).get("task_id") or body.get("data", {}).get("id")
if not task_id:
    raise SystemExit("submit response did not contain task_id")
print(task_id)
PY
)"

echo "task_id=$TASK_ID"

MAX_POLLS=$(( (TIMEOUT_SECONDS + POLL_INTERVAL_SECONDS - 1) / POLL_INTERVAL_SECONDS ))
FINAL_SUMMARY=""

for i in $(seq 1 "$MAX_POLLS"); do
  POLL_HTTP="$(curl -sS -o "$POLL_RESPONSE_FILE" -w "%{http_code}" \
    -H "Authorization: Bearer ${AUTH_KEY}" \
    "$BASE_URL/v1/video/generations/$TASK_ID")"

  SUMMARY="$(python3 "$PARSER_FILE" "$POLL_RESPONSE_FILE")"
  echo "poll_$i http=$POLL_HTTP summary=$SUMMARY"

  STATUS="$(printf '%s' "$SUMMARY" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("status",""))')"
  case "$STATUS" in
    SUCCESS|completed|succeeded|success)
      FINAL_SUMMARY="$SUMMARY"
      break
      ;;
    FAILURE|failed|cancelled|canceled)
      FINAL_SUMMARY="$SUMMARY"
      echo "task_failed" >&2
      break
      ;;
  esac

  sleep "$POLL_INTERVAL_SECONDS"
done

if [ -z "$FINAL_SUMMARY" ]; then
  FINAL_SUMMARY="$(python3 "$PARSER_FILE" "$POLL_RESPONSE_FILE")"
  echo "task_timeout=$TASK_ID" >&2
  echo "final_summary=$FINAL_SUMMARY"
  exit 1
fi

echo "final_response_path=$POLL_RESPONSE_FILE"
echo "final_response_begin"
cat "$POLL_RESPONSE_FILE"
echo
echo "final_response_end"
echo "final_summary=$FINAL_SUMMARY"

FINAL_STATUS="$(printf '%s' "$FINAL_SUMMARY" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("status",""))')"
FINAL_RATIO="$(printf '%s' "$FINAL_SUMMARY" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("ratio",""))')"
RESULT_URL="$(printf '%s' "$FINAL_SUMMARY" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("result_url",""))')"

if [ "$ASSERT_RATIO" = "1" ] && [ -n "$FINAL_RATIO" ] && [ "$FINAL_RATIO" != "$RATIO" ]; then
  echo "ratio_mismatch expected=$RATIO actual=$FINAL_RATIO" >&2
  exit 1
fi

if [ "$CHECK_CONTENT" = "1" ] && [ -n "$RESULT_URL" ]; then
  CONTENT_CHECK="$(curl -sS -L -o /dev/null -w "%{http_code} %{content_type} %{size_download}" "$RESULT_URL")"
  echo "content_check=$CONTENT_CHECK"
fi

echo "db_task_begin"
docker exec -i new-api-postgres psql -U newapi -d newapi \
  -v task_id="$TASK_ID" -F $'\t' -A -t <<'SQL'
select
  t.id,
  t.task_id,
  t.user_id,
  t."group",
  t.channel_id,
  c.name as channel_name,
  c.type as channel_type,
  c.base_url as channel_base_url,
  t.quota,
  t.status,
  t.progress,
  t.properties,
  t.data
from tasks t
left join channels c on c.id = t.channel_id
where t.task_id = :'task_id';
SQL
echo "db_task_end"

echo "billing_logs_begin"
docker exec -i new-api-postgres psql -U newapi -d newapi \
  -v token_name="$TOKEN_NAME" -v start_ts="$START_TS" -F $'\t' -A -t <<'SQL'
select id,created_at,type,quota,content,other
from logs
where token_name = :'token_name' and created_at >= :start_ts
order by id;
SQL
echo "billing_logs_end"

case "$FINAL_STATUS" in
  SUCCESS|completed|succeeded|success) exit 0 ;;
  *) exit 1 ;;
esac
'@

$checkContent = if ($SkipContentCheck) { "0" } else { "1" }
$assertRatio = if ($SkipRatioAssert) { "0" } else { "1" }

$remoteEnv = @(
    "BASE_URL_B64=$(ConvertTo-Base64Utf8 $BaseUrl)",
    "TOKEN_NAME_B64=$(ConvertTo-Base64Utf8 $TokenName)",
    "MODEL_B64=$(ConvertTo-Base64Utf8 $Model)",
    "ASSET_URL_B64=$(ConvertTo-Base64Utf8 $AssetUrl)",
    "RATIO_B64=$(ConvertTo-Base64Utf8 $Ratio)",
    "RESOLUTION_B64=$(ConvertTo-Base64Utf8 $Resolution)",
    "ROLE_B64=$(ConvertTo-Base64Utf8 $Role)",
    "PROMPT_B64=$(ConvertTo-Base64Utf8 $Prompt)",
    "REMOTE_RESULT_DIR_B64=$(ConvertTo-Base64Utf8 $RemoteResultDir)",
    "DURATION_SECONDS=$DurationSeconds",
    "POLL_INTERVAL_SECONDS=$PollIntervalSeconds",
    "TIMEOUT_SECONDS=$TimeoutSeconds",
    "CHECK_CONTENT=$checkContent",
    "ASSERT_RATIO=$assertRatio"
) -join " "

$remoteCommand = "tr -d '\r' | env $remoteEnv bash"
$remoteScript | ssh $SshHost $remoteCommand

if ($LASTEXITCODE -ne 0) {
    throw "Seedance video E2E test failed with exit code $LASTEXITCODE."
}
