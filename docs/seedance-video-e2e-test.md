# Seedance 2.0 Asset Video E2E Test Runbook

This runbook records the known-good production test for generating a minimal
Seedance 2.0 video from an overseas material-library asset.

## Foxtoken AlbertG Filter-Off Verification

This case was verified after deploying the official Seedance 2.0 completion
billing patch.

- Verification time: `2026-07-08`
- Production image: `zooyf/new-api:nexus-20260708T034732Z-f27c7987a949`
- Production base URL: `https://llm.ai.nexus-reach.com`
- Token: production token name `AlbertG`, token ID `3`
- Required token group: `AlbertG`
- Model: `doubao-seedance-2-0-filter-off`
- Resolution: `480p`
- Duration: `5`
- Ratio: `1:1`
- `QuotaPerUnit`: `500000`

Before running this case, make sure the `AlbertG` token group is usable by the
token owner. The production issue seen during this verification was:

```text
无权访问 AlbertG 分组
```

Fix by adding `AlbertG` to `UserUsableGroups` or the corresponding special
usable-group rule, then restart/sync the active `new-api` service.

### Asset Library Proxy

Downstream requests stay on the public production domain:

```text
POST https://llm.ai.nexus-reach.com/api/v3/open/CreateAsset
POST https://llm.ai.nexus-reach.com/api/v3/open/GetAsset
```

With the `AlbertG` token, `hwdrama-proxy` routes them to:

```text
POST https://foxtoken.linkomobile.com/api/v3/open/CreateAsset
POST https://foxtoken.linkomobile.com/api/v3/open/GetAsset
```

Known-good `CreateAsset` body:

```json
{
  "model": "doubao-seedance-2-0-filter-off",
  "url": "https://img.wetoken.ai/image/2026/7/6/1/7f973ca5288a4939baf0707a179b9e1d.png?x-oss-process=image/quality,q_60/format,jpg",
  "name": "e2e-albertg-filter-off-20260708035603",
  "AssetType": "Image",
  "Moderation": {
    "Strategy": "Skip"
  }
}
```

Result:

- `CreateAsset` HTTP status: `200`
- `CreateAsset` body: `{"id":"asset-20260708115604-8nrb2"}`
- `GetAsset` final status: `Active`
- Material URL for video tests: `asset://asset-20260708115604-8nrb2`

### No Reference Video

Downstream submit endpoint:

```text
POST https://llm.ai.nexus-reach.com/v1/video/generations
```

The selected `DoubaoVideo` channel submits to:

```text
POST https://foxtoken.linkomobile.com/api/v3/contents/generations/tasks
```

Known-good request body:

```json
{
  "model": "doubao-seedance-2-0-filter-off",
  "prompt": "Create a minimal 5 second image-to-video test from the reference asset. Keep the subject stable with only subtle natural motion. No camera movement, no style change, no audio.",
  "duration": 5,
  "resolution": "480p",
  "ratio": "1:1",
  "metadata": {
    "generate_audio": false,
    "content": [
      {
        "type": "image_url",
        "role": "first_frame",
        "image_url": {
          "url": "asset://asset-20260708115604-8nrb2"
        }
      }
    ]
  }
}
```

Result:

- Local task ID: `task_OYShjFowuOFZI2cEoVyvDdobqwHfnijn`
- Upstream task ID: `cgt-20260708120029-nvwww`
- Final status: `SUCCESS`, upstream status `succeeded`
- Content check: HTTP `200`, `Content-Type: video/mp4`, size `2064411`
- Upstream usage: `total_tokens=50638`, `completion_tokens=50638`
- Official price tier: `7.0 USD / 1M tokens`

Billing formula:

```text
int(total_tokens / 1000000 * usd_per_m_tokens * quota_per_unit * group_ratio)
```

Billing calculation:

```text
int(50638 / 1000000 * 7.0 * 500000 * 1) = 177233 quota
177233 / 500000 = $0.354466
```

Check the billing adjustment in `logs.other.actual_quota`. At verification
time the `tasks.quota` field still showed the pre-charge amount, while the
balance adjustment log contained the final calculated amount:

```text
log type: refund
quota: 697767
content: token重算：tokens=50638, modelRatio=3.50, groupRatio=1.00, otherMultiplier=1.0000
other.actual_quota: 177233
other.pre_consumed_quota: 875000
```

### With Reference Video

For Seedance reference-video mode, the metadata content item must include
`role: "reference_video"`. Without this role, the upstream rejects the request:

```text
InvalidParameter: reference media mode requires video role to be reference_video
```

Known-good request body:

```json
{
  "model": "doubao-seedance-2-0-filter-off",
  "prompt": "Create a minimal 5 second video using the reference video for gentle motion guidance. Keep the output stable and simple. No audio.",
  "duration": 5,
  "resolution": "480p",
  "ratio": "1:1",
  "metadata": {
    "generate_audio": false,
    "content": [
      {
        "type": "video_url",
        "role": "reference_video",
        "video_url": {
          "url": "https://ark-doc.tos-ap-southeast-1.bytepluses.com/doc_video/r2v_tea_video1.mp4"
        }
      }
    ]
  }
}
```

Result:

- Local task ID: `task_PfmsLqQurcyFPKQAUd1coHpQrBS17V9O`
- Upstream task ID: `cgt-20260708120917-2ldr2`
- Final status: `SUCCESS`, upstream status `succeeded`
- Content check: HTTP `200`, `Content-Type: video/mp4`, size `1240445`
- Upstream usage: `total_tokens=100858`, `completion_tokens=100858`
- `video_input` ratio: `0.6142857142857142`
- Official price tier: `4.3 USD / 1M tokens`

Billing calculation:

```text
int(100858 / 1000000 * 4.3 * 500000 * 1) = 216844 quota
216844 / 500000 = $0.433688
```

Adjustment log observed:

```text
log type: refund
quota: 320655
content: token重算：tokens=100858, modelRatio=3.50, groupRatio=1.00, otherMultiplier=0.6143
other.actual_quota: 216844
other.pre_consumed_quota: 537499
other.video_input: 0.6142857142857142
```

### Dreamina Mini Two Reference Images With Audio

This case verifies that multi-image reference mode must send an explicit
`role` for every image content item. The original request without `role`
failed with:

```text
InvalidParameter: role must be specified for image contents
```

For two reference images, use `role: "reference_image"` on both images. Do not
use `first_frame`/`last_frame` unless the test is specifically a strict
start/end-frame task.

Known-good request body:

```json
{
  "model": "dreamina-seedance-2-0-mini-filter-off",
  "prompt": "一个春天的早晨",
  "resolution": "480p",
  "ratio": "16:9",
  "duration": 5,
  "metadata": {
    "generate_audio": true,
    "content": [
      {
        "type": "image_url",
        "role": "reference_image",
        "image_url": {
          "url": "asset://asset-20260708115323-48kxb"
        }
      },
      {
        "type": "image_url",
        "role": "reference_image",
        "image_url": {
          "url": "asset://asset-20260708115408-j8n6s"
        }
      }
    ],
    "duration": 5
  }
}
```

Result:

- Local task ID: `task_hixqqcuIqPB53zSAAPCCSt3zDWFeCs4W`
- Final status: `SUCCESS`, upstream status `succeeded`
- Content check: HTTP `200`, `Content-Type: video/mp4`, size `1594219`
- Upstream usage: `total_tokens=50638`, `completion_tokens=50638`
- Official price tier: `3.5 USD / 1M tokens`

Billing calculation:

```text
int(50638 / 1000000 * 3.5 * 500000 * 1) = 88616 quota
88616 / 500000 = $0.177232
```

Adjustment log observed:

```text
log type: refund
quota: 348884
content: adaptor计费调整
other.actual_quota: 88616
other.pre_consumed_quota: 437500
```

### SQL Checks

Use these queries on `nexus-sg` when validating the final billing adjustment:

```bash
docker exec new-api-postgres psql -U newapi -d newapi -tAc \
  "select task_id,status,quota,private_data->'billing_context',data->'usage' from tasks where task_id='task_xxx';"

docker exec new-api-postgres psql -U newapi -d newapi -tAc \
  "select id,type,quota,content,other from logs where other like '%task_xxx%' order by id desc limit 5;"
```

## Known-Good Case

- Production base URL: `https://llm.ai.nexus-reach.com`
- Model: `doubao-seedance-2-0-fast-filter-off`
- Asset: `asset://asset-20260704235114-x565x`
- Resolution: `480p`
- Duration: `5`
- Ratio: `1:1`
- Required new-api group: `AlbertG`
- Test token name in production DB: `hwdrama-proxy-e2e-video-albertg`

Last successful verification:

- Local task ID: `task_ie7lZ9rmteopsnGIPSh1rau1zpSQ40NI`
- Upstream task ID: `task_Bs1qqKFsjxqB9CUDN3VCIks83c0nzU7e`
- Final status: `SUCCESS`, upstream status `completed`
- Progress: `100%`
- Quota charged: `3500000`
- Runtime from submit to finish: about `105s`
- Result URL:
  `https://llm.ai.nexus-reach.com/v1/videos/task_ie7lZ9rmteopsnGIPSh1rau1zpSQ40NI/content`
- Result content check: HTTP `200`, `Content-Type: video/mp4`

## Run From Local PowerShell

Run this from a machine that can SSH to `nexus-sg`. The script retrieves the
dedicated test key from the production database on the server and does not print
the key.

```powershell
@'
set -euo pipefail

BASE_URL="https://llm.ai.nexus-reach.com"
TOKEN_NAME="hwdrama-proxy-e2e-video-albertg"
MODEL="doubao-seedance-2-0-fast-filter-off"
ASSET_URL="asset://asset-20260704235114-x565x"

KEY_RAW=$(docker exec new-api-postgres psql -U newapi -d newapi -tAc \
  "select key from tokens where name='${TOKEN_NAME}' and deleted_at is null order by id desc limit 1")
KEY_RAW=$(printf '%s' "$KEY_RAW" | tr -d '[:space:]')

if [ -z "$KEY_RAW" ]; then
  echo "Missing production test token: ${TOKEN_NAME}" >&2
  exit 1
fi

REQUEST_BODY=$(cat <<JSON
{
  "model": "${MODEL}",
  "prompt": "Use the reference portrait as the subject. Create a minimal 5 second video with only a subtle blink, gentle breathing, and a tiny natural head movement. Keep the face identity and background stable. No camera movement, no style change.",
  "duration": 5,
  "resolution": "480p",
  "ratio": "1:1",
  "metadata": {
    "generate_audio": false,
    "content": [
      {
        "type": "image_url",
        "role": "first_frame",
        "image_url": {
          "url": "${ASSET_URL}"
        }
      }
    ]
  }
}
JSON
)

SUBMIT_RESPONSE=$(curl -sS -X POST "${BASE_URL}/v1/video/generations" \
  -H "Authorization: Bearer sk-${KEY_RAW}" \
  -H "Content-Type: application/json" \
  -d "$REQUEST_BODY")

TASK_ID=$(python3 -c 'import json,sys
body=json.load(sys.stdin)
task_id=body.get("task_id") or body.get("id") or body.get("data",{}).get("task_id")
if not task_id:
    raise SystemExit("submit response did not contain task_id: " + json.dumps(body, ensure_ascii=False))
print(task_id)' <<< "$SUBMIT_RESPONSE")

echo "submitted_task_id=${TASK_ID}"

for i in $(seq 1 45); do
  QUERY_RESPONSE=$(curl -sS \
    -H "Authorization: Bearer sk-${KEY_RAW}" \
    "${BASE_URL}/v1/video/generations/${TASK_ID}")

  POLL_SUMMARY=$(python3 -c 'import json,sys
body=json.load(sys.stdin)
data=body.get("data", body)
status=data.get("status") or data.get("data",{}).get("status") or ""
upstream_status=data.get("data",{}).get("status") or ""
progress=data.get("progress") or data.get("data",{}).get("progress") or ""
result_url=data.get("result_url") or data.get("data",{}).get("metadata",{}).get("url") or ""
fail_reason=data.get("fail_reason") or data.get("error",{}).get("message") or ""
print(json.dumps({
    "status": status,
    "upstream_status": upstream_status,
    "progress": progress,
    "result_url": result_url,
    "fail_reason": fail_reason,
}, ensure_ascii=False))' <<< "$QUERY_RESPONSE")

  echo "poll_${i}=${POLL_SUMMARY}"

  STATUS=$(python3 -c 'import json,sys
print(json.load(sys.stdin).get("status",""))' <<< "$POLL_SUMMARY")

  if [ "$STATUS" = "SUCCESS" ] || [ "$STATUS" = "completed" ] || [ "$STATUS" = "succeeded" ]; then
    RESULT_URL=$(python3 -c 'import json,sys
print(json.load(sys.stdin).get("result_url",""))' <<< "$POLL_SUMMARY")

    if [ -n "$RESULT_URL" ]; then
      CONTENT_CHECK=$(curl -sS -L -o /dev/null -w "%{http_code} %{content_type} %{size_download}" \
        -H "Authorization: Bearer sk-${KEY_RAW}" "$RESULT_URL")
      echo "content_check=${CONTENT_CHECK}"
    fi

    exit 0
  fi

  if [ "$STATUS" = "FAILURE" ] || [ "$STATUS" = "failed" ]; then
    echo "task failed" >&2
    exit 1
  fi

  sleep 3
done

echo "task did not finish before polling timeout: ${TASK_ID}" >&2
exit 1
'@ | ssh nexus-sg "bash -s"
```

## Run Directly From A Customer Machine

Use these commands when testing from a customer's own Windows, Linux, or macOS
machine. Replace `sk-TEST_NEW_API_KEY` with a new-api key that belongs to the
`AlbertG` group.

### Linux Or macOS

Submit the video task:

```bash
export NEW_API_KEY="sk-TEST_NEW_API_KEY"
export BASE_URL="https://llm.ai.nexus-reach.com"

curl -sS -X POST "${BASE_URL}/v1/video/generations" \
  -H "Authorization: Bearer ${NEW_API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "doubao-seedance-2-0-fast-filter-off",
    "prompt": "Use the reference portrait as the subject. Create a minimal 5 second video with only a subtle blink, gentle breathing, and a tiny natural head movement. Keep the face identity and background stable. No camera movement, no style change.",
    "duration": 5,
    "resolution": "480p",
    "ratio": "1:1",
    "metadata": {
      "generate_audio": false,
      "content": [
        {
          "type": "image_url",
          "role": "first_frame",
          "image_url": {
            "url": "asset://asset-20260704235114-x565x"
          }
        }
      ]
    }
  }'
```

Query the task. Replace `task_xxx` with the `id` or `task_id` returned by the
submit response.

```bash
curl -sS \
  -H "Authorization: Bearer ${NEW_API_KEY}" \
  "${BASE_URL}/v1/video/generations/task_xxx"
```

Download or verify the generated video after the task status becomes `SUCCESS`.

```bash
curl -L \
  -H "Authorization: Bearer ${NEW_API_KEY}" \
  -o seedance-test.mp4 \
  "${BASE_URL}/v1/videos/task_xxx/content"
```

### Windows PowerShell

PowerShell aliases `curl` to `Invoke-WebRequest` on some systems, so use
`curl.exe` explicitly.

Submit the video task:

```powershell
$Env:NEW_API_KEY = "sk-TEST_NEW_API_KEY"
$BaseUrl = "https://llm.ai.nexus-reach.com"

$Body = @'
{
  "model": "doubao-seedance-2-0-fast-filter-off",
  "prompt": "Use the reference portrait as the subject. Create a minimal 5 second video with only a subtle blink, gentle breathing, and a tiny natural head movement. Keep the face identity and background stable. No camera movement, no style change.",
  "duration": 5,
  "resolution": "480p",
  "ratio": "1:1",
  "metadata": {
    "generate_audio": false,
    "content": [
      {
        "type": "image_url",
        "role": "first_frame",
        "image_url": {
          "url": "asset://asset-20260704235114-x565x"
        }
      }
    ]
  }
}
'@

curl.exe -sS -X POST "$BaseUrl/v1/video/generations" `
  -H "Authorization: Bearer $Env:NEW_API_KEY" `
  -H "Content-Type: application/json" `
  --data $Body
```

## Wetoken CreateAsset With Moderation Skip Success

This records the successful production material-library test using the `wetoken`
new-api key, `Moderation.Strategy=Skip`, and a wetoken-hosted image URL.

- Production base URL: `https://llm.ai.nexus-reach.com`
- Token name in production DB: `wetoken`
- Token group: `globalSDTest2`
- Selected channel: `wetoken`, channel ID `3`, channel type `54`
- Upstream base URL: `https://wetoken.ai`
- Model: `doubao-seedance-2-0-fast-filter-off`
- Image URL:
  `https://img.wetoken.ai/image/2026/7/6/1/7f973ca5288a4939baf0707a179b9e1d.png?x-oss-process=image/quality,q_60/format,jpg`
- Moderation: `{"Strategy":"Skip"}`

Verification result on 2026-07-06: `CreateAsset` and `GetAsset` both
succeeded. The asset became active.

- Asset ID: `asset-20260706201813-f864h`
- Asset name: `e2e-wetoken-image-skip-20260706121811`
- Final asset status: `Active`
- `URL` present: yes
- Error: empty

Successful `CreateAsset` response:

```json
{
  "id": "asset-20260706201813-f864h"
}
```

Successful `GetAsset` response shape:

```json
{
  "AssetType": "Image",
  "Error": {
    "code": "",
    "message": ""
  },
  "GroupId": "group-20260702224302-h72vf",
  "Id": "asset-20260706201813-f864h",
  "Name": "e2e-wetoken-image-skip-20260706121811",
  "Status": "Active",
  "URL": "https://ark-media-asset-ap-southeast-1.tos-ap-southeast-1.volces.com/..."
}
```

The same image also succeeds without `Moderation.Strategy=Skip`.

- Asset ID: `asset-20260706202616-dbpdp`
- Asset name: `e2e-wetoken-image-default-20260706122613`
- Final asset status: `Active`
- `URL` present: yes
- Error: empty

The no-moderation `CreateAsset` request body is:

```json
{
  "model": "doubao-seedance-2-0-fast-filter-off",
  "url": "https://img.wetoken.ai/image/2026/7/6/1/7f973ca5288a4939baf0707a179b9e1d.png?x-oss-process=image/quality,q_60/format,jpg",
  "name": "e2e-wetoken-image-default-20260706122613",
  "AssetType": "Image"
}
```

## AlbertG CreateAsset With And Without Moderation Skip

This records the same material-library test using the new-api token whose name
is `AlbertG`.

- Production base URL: `https://llm.ai.nexus-reach.com`
- Token name in production DB: `AlbertG`
- Token group: `AlbertG`
- Model: `doubao-seedance-2-0-fast-filter-off`
- Image URL:
  `https://img.wetoken.ai/image/2026/7/6/1/7f973ca5288a4939baf0707a179b9e1d.png?x-oss-process=image/quality,q_60/format,jpg`

Verification result on 2026-07-06: both cases succeeded.

Without `Moderation.Strategy=Skip`:

- Asset ID: `asset-20260706202857-45wzh`
- Asset name: `e2e-albertg-default-20260706122856`
- Final asset status: `Active`
- `URL` present: yes
- Error: empty

With `Moderation.Strategy=Skip`:

- Asset ID: `asset-20260706202902-tf8xw`
- Asset name: `e2e-albertg-skip-20260706122901`
- Final asset status: `Active`
- `URL` present: yes
- Error: empty

The no-moderation `CreateAsset` request body was:

```json
{
  "model": "doubao-seedance-2-0-fast-filter-off",
  "url": "https://img.wetoken.ai/image/2026/7/6/1/7f973ca5288a4939baf0707a179b9e1d.png?x-oss-process=image/quality,q_60/format,jpg",
  "name": "e2e-albertg-default-20260706122856",
  "AssetType": "Image"
}
```

The moderation-skip `CreateAsset` request body was:

```json
{
  "model": "doubao-seedance-2-0-fast-filter-off",
  "url": "https://img.wetoken.ai/image/2026/7/6/1/7f973ca5288a4939baf0707a179b9e1d.png?x-oss-process=image/quality,q_60/format,jpg",
  "name": "e2e-albertg-skip-20260706122901",
  "AssetType": "Image",
  "Moderation": {
    "Strategy": "Skip"
  }
}
```

### Run From Local PowerShell Through The Server

This version reads the `wetoken` new-api key from the production DB and does not
print it.

```powershell
@'
set -euo pipefail

BASE_URL="https://llm.ai.nexus-reach.com"
TOKEN_NAME="wetoken"
MODEL="doubao-seedance-2-0-fast-filter-off"
IMAGE_URL="https://img.wetoken.ai/image/2026/7/6/1/7f973ca5288a4939baf0707a179b9e1d.png?x-oss-process=image/quality,q_60/format,jpg"
ASSET_NAME="e2e-wetoken-image-skip-$(date -u +%Y%m%d%H%M%S)"

KEY_RAW=$(docker exec new-api-postgres psql -U newapi -d newapi -tAc \
  "select key from tokens where name='${TOKEN_NAME}' and deleted_at is null order by id desc limit 1")
KEY_RAW=$(printf '%s' "$KEY_RAW" | tr -d '[:space:]')

CREATE_BODY=$(cat <<JSON
{
  "model": "${MODEL}",
  "url": "${IMAGE_URL}",
  "name": "${ASSET_NAME}",
  "AssetType": "Image",
  "Moderation": {
    "Strategy": "Skip"
  }
}
JSON
)

CREATE_RESPONSE=$(curl -sS -X POST "${BASE_URL}/api/v3/open/CreateAsset" \
  -H "Authorization: Bearer sk-${KEY_RAW}" \
  -H "Content-Type: application/json" \
  -d "$CREATE_BODY")

echo "$CREATE_RESPONSE"
ASSET_ID=$(printf '%s' "$CREATE_RESPONSE" | python3 -c 'import json,sys
body=json.load(sys.stdin)
print(body.get("id") or body.get("Id") or body.get("asset_id") or "")')

GET_BODY=$(cat <<JSON
{
  "model": "${MODEL}",
  "Id": "${ASSET_ID}"
}
JSON
)

for i in $(seq 1 20); do
  GET_RESPONSE=$(curl -sS -X POST "${BASE_URL}/api/v3/open/GetAsset" \
    -H "Authorization: Bearer sk-${KEY_RAW}" \
    -H "Content-Type: application/json" \
    -d "$GET_BODY")
  echo "$GET_RESPONSE"

  STATUS=$(printf '%s' "$GET_RESPONSE" | python3 -c 'import json,sys
body=json.load(sys.stdin)
print(body.get("Status") or body.get("status") or "")')

  if [ "$STATUS" = "Active" ] || [ "$STATUS" = "Failed" ]; then
    break
  fi

  sleep 3
done
'@ | ssh nexus-sg "tr -d '\r' | bash"
```

### Run Directly From Linux Or macOS

Replace `sk-WETOKEN_NEW_API_KEY` with the customer-facing new-api key.

```bash
export NEW_API_KEY="sk-WETOKEN_NEW_API_KEY"
export BASE_URL="https://llm.ai.nexus-reach.com"
export MODEL="doubao-seedance-2-0-fast-filter-off"
export ASSET_NAME="e2e-wetoken-image-skip-$(date -u +%Y%m%d%H%M%S)"

curl -sS -X POST "${BASE_URL}/api/v3/open/CreateAsset" \
  -H "Authorization: Bearer ${NEW_API_KEY}" \
  -H "Content-Type: application/json" \
  -d "{
    \"model\": \"${MODEL}\",
    \"url\": \"https://img.wetoken.ai/image/2026/7/6/1/7f973ca5288a4939baf0707a179b9e1d.png?x-oss-process=image/quality,q_60/format,jpg\",
    \"name\": \"${ASSET_NAME}\",
    \"AssetType\": \"Image\",
    \"Moderation\": {
      \"Strategy\": \"Skip\"
    }
  }"
```

Then query the asset. Replace `asset_xxx` with the `id` returned by
`CreateAsset`.

```bash
curl -sS -X POST "${BASE_URL}/api/v3/open/GetAsset" \
  -H "Authorization: Bearer ${NEW_API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "doubao-seedance-2-0-fast-filter-off",
    "Id": "asset_xxx"
  }'
```

### Run Directly From Windows PowerShell

PowerShell aliases `curl` to `Invoke-WebRequest` on some systems, so use
`curl.exe` explicitly.

```powershell
$Env:NEW_API_KEY = "sk-WETOKEN_NEW_API_KEY"
$BaseUrl = "https://llm.ai.nexus-reach.com"
$AssetName = "e2e-wetoken-image-skip-$((Get-Date).ToUniversalTime().ToString('yyyyMMddHHmmss'))"

$CreateBody = @"
{
  "model": "doubao-seedance-2-0-fast-filter-off",
  "url": "https://img.wetoken.ai/image/2026/7/6/1/7f973ca5288a4939baf0707a179b9e1d.png?x-oss-process=image/quality,q_60/format,jpg",
  "name": "$AssetName",
  "AssetType": "Image",
  "Moderation": {
    "Strategy": "Skip"
  }
}
"@

curl.exe -sS -X POST "$BaseUrl/api/v3/open/CreateAsset" `
  -H "Authorization: Bearer $Env:NEW_API_KEY" `
  -H "Content-Type: application/json" `
  --data $CreateBody
```

Then query the asset. Replace `asset_xxx` with the `id` returned by
`CreateAsset`.

```powershell
$GetBody = @'
{
  "model": "doubao-seedance-2-0-fast-filter-off",
  "Id": "asset_xxx"
}
'@

curl.exe -sS -X POST "$BaseUrl/api/v3/open/GetAsset" `
  -H "Authorization: Bearer $Env:NEW_API_KEY" `
  -H "Content-Type: application/json" `
  --data $GetBody
```

## Wetoken Replacement Image 0.5s Attempt

This records the follow-up production attempt with the same `wetoken` key, but
using the replacement Xinhua image URL.

- Production base URL: `https://llm.ai.nexus-reach.com`
- Token name in production DB: `wetoken`
- Token group: `globalSDTest2`
- Selected channel: `wetoken`, channel ID `3`, channel type `54`
- Upstream base URL: `https://wetoken.ai`
- Requested model: `doubao-seedance-2-0-fast-filter-off`
- Upstream returned model: `dreamina-seedance-2-0-fast-260128`
- Image URL:
  `https://english.news.cn/20260705/26e48b12c5cd46e393a3c841724f8fff/2026070526e48b12c5cd46e393a3c841724f8fff_20260705a72e50108c95436ea94849e6c5382aaf.jpg`
- Requested seconds: `0.5`
- Upstream returned duration: `5`
- Requested ratio: `1:1`
- Upstream returned ratio: `4:3`
- Resolution: `480p`

Verification result on 2026-07-06: submit and generation succeeded.

- Local task ID: `task_wFlVH3JNPhOe5KvKSi4MtByCHarigIrn`
- Upstream task ID: `cgt-20260706171937-cn4g6`
- Final status: `SUCCESS`, upstream status `succeeded`
- Progress: `100%`
- Task quota: `700000`
- Runtime from submit to finish: about `105s`
- Upstream usage: `total_tokens=49761`, `completion_tokens=49761`
- Result content check through new-api proxy: HTTP `200`, `Content-Type: video/mp4`,
  size `1169980` bytes
- Result URL:
  `https://llm.ai.nexus-reach.com/v1/videos/task_wFlVH3JNPhOe5KvKSi4MtByCHarigIrn/content`

Important caveat: this confirms the replacement image can generate successfully,
but it does not confirm a true upstream `0.5s` video. The current production
`wetoken` channel uses the DoubaoVideo task adaptor. That adaptor integer-parses
`seconds`, so `seconds: "0.5"` is not forwarded as a fractional duration. The
upstream result showed `duration: 5`.

### Run From Local PowerShell Through The Server

This version reads the `wetoken` new-api key from the production DB and does not
print it.

```powershell
@'
set -euo pipefail

BASE_URL="https://llm.ai.nexus-reach.com"
TOKEN_NAME="wetoken"
MODEL="doubao-seedance-2-0-fast-filter-off"
IMAGE_URL="https://english.news.cn/20260705/26e48b12c5cd46e393a3c841724f8fff/2026070526e48b12c5cd46e393a3c841724f8fff_20260705a72e50108c95436ea94849e6c5382aaf.jpg"

KEY_RAW=$(docker exec new-api-postgres psql -U newapi -d newapi -tAc \
  "select key from tokens where name='${TOKEN_NAME}' and deleted_at is null order by id desc limit 1")
KEY_RAW=$(printf '%s' "$KEY_RAW" | tr -d '[:space:]')

REQUEST_BODY=$(cat <<JSON
{
  "model": "${MODEL}",
  "prompt": "Create an ultra-short 0.5 second image-to-video test from the reference image. Keep the scene almost still, with only a very subtle natural motion. No camera movement, no style change, no audio.",
  "seconds": "0.5",
  "resolution": "480p",
  "ratio": "1:1",
  "metadata": {
    "generate_audio": false,
    "content": [
      {
        "type": "image_url",
        "role": "first_frame",
        "image_url": {
          "url": "${IMAGE_URL}"
        }
      }
    ]
  }
}
JSON
)

SUBMIT_RESPONSE=$(curl -sS -X POST "${BASE_URL}/v1/video/generations" \
  -H "Authorization: Bearer sk-${KEY_RAW}" \
  -H "Content-Type: application/json" \
  -d "$REQUEST_BODY")

echo "$SUBMIT_RESPONSE"
TASK_ID=$(printf '%s' "$SUBMIT_RESPONSE" | python3 -c 'import json,sys
body=json.load(sys.stdin)
data=body.get("data") if isinstance(body.get("data"), dict) else {}
print(body.get("task_id") or body.get("id") or data.get("task_id") or data.get("id") or "")')

for i in $(seq 1 80); do
  QUERY_RESPONSE=$(curl -sS \
    -H "Authorization: Bearer sk-${KEY_RAW}" \
    "${BASE_URL}/v1/video/generations/${TASK_ID}")
  echo "$QUERY_RESPONSE"

  STATUS=$(printf '%s' "$QUERY_RESPONSE" | python3 -c 'import json,sys
body=json.load(sys.stdin)
data=body.get("data", body)
print(data.get("status", ""))')

  if [ "$STATUS" = "SUCCESS" ] || [ "$STATUS" = "FAILURE" ]; then
    break
  fi

  sleep 3
done

curl -sS -L -o /dev/null -w "%{http_code} %{content_type} %{size_download}\n" \
  -H "Authorization: Bearer sk-${KEY_RAW}" \
  "${BASE_URL}/v1/videos/${TASK_ID}/content"
'@ | ssh nexus-sg "tr -d '\r' | bash"
```

### Run Directly From Linux Or macOS

Replace `sk-WETOKEN_NEW_API_KEY` with the customer-facing new-api key.

```bash
export NEW_API_KEY="sk-WETOKEN_NEW_API_KEY"
export BASE_URL="https://llm.ai.nexus-reach.com"

curl -sS -X POST "${BASE_URL}/v1/video/generations" \
  -H "Authorization: Bearer ${NEW_API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "doubao-seedance-2-0-fast-filter-off",
    "prompt": "Create an ultra-short 0.5 second image-to-video test from the reference image. Keep the scene almost still, with only a very subtle natural motion. No camera movement, no style change, no audio.",
    "seconds": "0.5",
    "resolution": "480p",
    "ratio": "1:1",
    "metadata": {
      "generate_audio": false,
      "content": [
        {
          "type": "image_url",
          "role": "first_frame",
          "image_url": {
            "url": "https://english.news.cn/20260705/26e48b12c5cd46e393a3c841724f8fff/2026070526e48b12c5cd46e393a3c841724f8fff_20260705a72e50108c95436ea94849e6c5382aaf.jpg"
          }
        }
      ]
    }
  }'
```

### Run Directly From Windows PowerShell

```powershell
$Env:NEW_API_KEY = "sk-WETOKEN_NEW_API_KEY"
$BaseUrl = "https://llm.ai.nexus-reach.com"

$Body = @'
{
  "model": "doubao-seedance-2-0-fast-filter-off",
  "prompt": "Create an ultra-short 0.5 second image-to-video test from the reference image. Keep the scene almost still, with only a very subtle natural motion. No camera movement, no style change, no audio.",
  "seconds": "0.5",
  "resolution": "480p",
  "ratio": "1:1",
  "metadata": {
    "generate_audio": false,
    "content": [
      {
        "type": "image_url",
        "role": "first_frame",
        "image_url": {
          "url": "https://english.news.cn/20260705/26e48b12c5cd46e393a3c841724f8fff/2026070526e48b12c5cd46e393a3c841724f8fff_20260705a72e50108c95436ea94849e6c5382aaf.jpg"
        }
      }
    ]
  }
}
'@

curl.exe -sS -X POST "$BaseUrl/v1/video/generations" `
  -H "Authorization: Bearer $Env:NEW_API_KEY" `
  -H "Content-Type: application/json" `
  --data $Body
```

## Query Or Download A Submitted Task

Query the task. Replace `task_xxx` with the `id` or `task_id` returned by the
submit response.

```powershell
curl.exe -sS `
  -H "Authorization: Bearer $Env:NEW_API_KEY" `
  "$BaseUrl/v1/video/generations/task_xxx"
```

Download or verify the generated video after the task status becomes `SUCCESS`.

```powershell
curl.exe -L `
  -H "Authorization: Bearer $Env:NEW_API_KEY" `
  -o seedance-test.mp4 `
  "$BaseUrl/v1/videos/task_xxx/content"
```

## Expected Output

The successful path should show:

```text
submitted_task_id=task_xxx
poll_N={"status":"SUCCESS","upstream_status":"completed","progress":"100%","result_url":"https://llm.ai.nexus-reach.com/v1/videos/task_xxx/content","fail_reason":""}
content_check=200 video/mp4 ...
```

## Notes

- This test creates a real upstream Seedance video task and consumes quota.
- Keep the request at `480p`, `5` seconds, and `generate_audio=false` for the
  lowest practical test cost.
- The query endpoint returns a wrapper shaped like `{"code":"success","data":{...}}`.
  Polling scripts must read task status from `data.status`.
- Do not send the downstream test key to the upstream provider. The production
  new-api channel handles upstream authentication.

## Wetoken Direct Image 0.5s Attempt

This records the production attempt using the `wetoken` new-api key and a direct
public image URL instead of a material-library `asset://...` reference.

- Production base URL: `https://llm.ai.nexus-reach.com`
- Token name in production DB: `wetoken`
- Token group: `globalSDTest2`
- Selected channel: `wetoken`, channel ID `3`, channel type `54`
- Upstream base URL: `https://wetoken.ai`
- Model: `doubao-seedance-2-0-fast-filter-off`
- Image URL:
  `https://tu.duoduocdn.com/uploads/day_260706/202607061050473326_720.jpg`
- Requested seconds: `0.5`
- Resolution: `480p`
- Ratio: `1:1`

Verification result on 2026-07-06: submit failed before a task ID was created.
The upstream rejected the input image as likely containing a real person:

```json
{
  "code": "fail_to_fetch_task",
  "message": "{\"code\":\"upstream_error\",\"message\":\"{\\\"error\\\":{\\\"code\\\":\\\"InputImageSensitiveContentDetected.PrivacyInformation\\\",\\\"message\\\":\\\"The request failed because the input image may contain real person. Request id: 0217833292866659e96f8c80ac47960a454270e9930f9545e39d5\\\",\\\"param\\\":\\\"\\\",\\\"type\\\":\\\"BadRequest\\\"}}\",\"data\":null}",
  "data": null
}
```

Current implementation note: the production `wetoken` channel uses the
DoubaoVideo task adaptor. That adaptor converts `seconds` with integer parsing,
so `seconds: "0.5"` is not preserved as a fractional duration in the upstream
Doubao request body. The request below records the intended 0.5s customer input;
strict fractional-duration pass-through would require an adaptor change or an
OpenAI-compatible/Sora-style upstream channel that forwards fractional seconds.

### Run From Local PowerShell Through The Server

This version reads the `wetoken` new-api key from the production DB and does not
print it.

```powershell
@'
set -euo pipefail

BASE_URL="https://llm.ai.nexus-reach.com"
TOKEN_NAME="wetoken"
MODEL="doubao-seedance-2-0-fast-filter-off"
IMAGE_URL="https://tu.duoduocdn.com/uploads/day_260706/202607061050473326_720.jpg"

KEY_RAW=$(docker exec new-api-postgres psql -U newapi -d newapi -tAc \
  "select key from tokens where name='${TOKEN_NAME}' and deleted_at is null order by id desc limit 1")
KEY_RAW=$(printf '%s' "$KEY_RAW" | tr -d '[:space:]')

REQUEST_BODY=$(cat <<JSON
{
  "model": "${MODEL}",
  "prompt": "Create an ultra-short 0.5 second image-to-video test from the reference image. Keep the subject almost still, with only a very subtle natural blink and tiny breathing motion. No camera movement, no style change, no audio.",
  "seconds": "0.5",
  "resolution": "480p",
  "ratio": "1:1",
  "metadata": {
    "generate_audio": false,
    "content": [
      {
        "type": "image_url",
        "role": "first_frame",
        "image_url": {
          "url": "${IMAGE_URL}"
        }
      }
    ]
  }
}
JSON
)

curl -sS -X POST "${BASE_URL}/v1/video/generations" \
  -H "Authorization: Bearer sk-${KEY_RAW}" \
  -H "Content-Type: application/json" \
  -d "$REQUEST_BODY"
'@ | ssh nexus-sg "tr -d '\r' | bash"
```

### Run Directly From Linux Or macOS

Replace `sk-WETOKEN_NEW_API_KEY` with the customer-facing new-api key.

```bash
export NEW_API_KEY="sk-WETOKEN_NEW_API_KEY"
export BASE_URL="https://llm.ai.nexus-reach.com"

curl -sS -X POST "${BASE_URL}/v1/video/generations" \
  -H "Authorization: Bearer ${NEW_API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "doubao-seedance-2-0-fast-filter-off",
    "prompt": "Create an ultra-short 0.5 second image-to-video test from the reference image. Keep the subject almost still, with only a very subtle natural blink and tiny breathing motion. No camera movement, no style change, no audio.",
    "seconds": "0.5",
    "resolution": "480p",
    "ratio": "1:1",
    "metadata": {
      "generate_audio": false,
      "content": [
        {
          "type": "image_url",
          "role": "first_frame",
          "image_url": {
            "url": "https://tu.duoduocdn.com/uploads/day_260706/202607061050473326_720.jpg"
          }
        }
      ]
    }
  }'
```

### Run Directly From Windows PowerShell

PowerShell aliases `curl` to `Invoke-WebRequest` on some systems, so use
`curl.exe` explicitly.

```powershell
$Env:NEW_API_KEY = "sk-WETOKEN_NEW_API_KEY"
$BaseUrl = "https://llm.ai.nexus-reach.com"

$Body = @'
{
  "model": "doubao-seedance-2-0-fast-filter-off",
  "prompt": "Create an ultra-short 0.5 second image-to-video test from the reference image. Keep the subject almost still, with only a very subtle natural blink and tiny breathing motion. No camera movement, no style change, no audio.",
  "seconds": "0.5",
  "resolution": "480p",
  "ratio": "1:1",
  "metadata": {
    "generate_audio": false,
    "content": [
      {
        "type": "image_url",
        "role": "first_frame",
        "image_url": {
          "url": "https://tu.duoduocdn.com/uploads/day_260706/202607061050473326_720.jpg"
        }
      }
    ]
  }
}
'@

curl.exe -sS -X POST "$BaseUrl/v1/video/generations" `
  -H "Authorization: Bearer $Env:NEW_API_KEY" `
  -H "Content-Type: application/json" `
  --data $Body
```
