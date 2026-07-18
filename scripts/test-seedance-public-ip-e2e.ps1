[CmdletBinding()]
param(
    [string]$BaseUrl = "https://124.174.0.221",
    [string]$SshHost = "124.174.0.221",
    [string]$SshUser = "root",
    [string]$SshKeyPath = "D:\codex\BPlatform.pem",
    [int]$TokenId = 1,
    [string]$TokenName = "seedance-assets-production",
    [string]$ImageUrl = "https://images.clipsafari.com/6rhxknsi0s4gqoqot0z9u2593bf6?filename=cartoon-woman.png",
    [string]$ResultRoot = ".deploy\seedance-public-ip-e2e",
    [int]$PollIntervalSeconds = 10,
    [int]$TimeoutSeconds = 900
)

$ErrorActionPreference = "Stop"
$utf8NoBom = New-Object System.Text.UTF8Encoding($false)
$runName = (Get-Date).ToUniversalTime().ToString("yyyyMMddTHHmmssZ")
$runDir = Join-Path $ResultRoot $runName
New-Item -ItemType Directory -Force -Path $runDir | Out-Null

function Write-Utf8Json {
    param(
        [Parameter(Mandatory = $true)][string]$Path,
        [Parameter(Mandatory = $true)]$Value
    )

    $json = $Value | ConvertTo-Json -Depth 30
    [IO.File]::WriteAllText($Path, $json, $utf8NoBom)
}

function Invoke-JsonEndpoint {
    param(
        [Parameter(Mandatory = $true)][string]$Name,
        [Parameter(Mandatory = $true)][ValidateSet("GET", "POST")][string]$Method,
        [Parameter(Mandatory = $true)][string]$Path,
        $Body = $null
    )

    $headersPath = Join-Path $runDir "$Name.response.headers"
    $responsePath = Join-Path $runDir "$Name.response.json"
    $arguments = @(
        "-sS",
        "--connect-timeout", "15",
        "--max-time", "120",
        "-D", $headersPath,
        "-o", $responsePath,
        "-w", "%{http_code}",
        "-X", $Method,
        "$BaseUrl$Path",
        "-H", "Authorization: Bearer $apiKey"
    )

    $requestPath = $null
    if ($null -ne $Body) {
        $requestPath = Join-Path $runDir "$Name.request.json"
        Write-Utf8Json -Path $requestPath -Value $Body
        $arguments += @("-H", "Content-Type: application/json", "--data-binary", "@$requestPath")
    }

    $httpStatus = (& curl.exe @arguments).Trim()
    if ($LASTEXITCODE -ne 0) {
        throw "$Name failed at the TLS/HTTP transport layer (curl exit $LASTEXITCODE)."
    }
    $rawBody = [IO.File]::ReadAllText($responsePath, [Text.Encoding]::UTF8)
    $parsedBody = $null
    if (-not [string]::IsNullOrWhiteSpace($rawBody)) {
        try {
            $parsedBody = $rawBody | ConvertFrom-Json
        }
        catch {
            throw "$Name returned non-JSON content: $rawBody"
        }
    }

    Write-Host "$Name http=$httpStatus"
    return [pscustomobject]@{
        Name = $Name
        Method = $Method
        Url = "$BaseUrl$Path"
        HttpStatus = [int]$httpStatus
        RequestPath = $requestPath
        ResponsePath = $responsePath
        HeadersPath = $headersPath
        Body = $parsedBody
    }
}

function Assert-Http200 {
    param([Parameter(Mandatory = $true)]$Result)
    if ($Result.HttpStatus -ne 200) {
        throw "$($Result.Name) returned HTTP $($Result.HttpStatus). See $($Result.ResponsePath)."
    }
}

function Assert-BusinessSuccess {
    param([Parameter(Mandatory = $true)]$Result)
    Assert-Http200 -Result $Result
    if ($Result.Body.state -ne 1) {
        throw "$($Result.Name) returned state=$($Result.Body.state). See $($Result.ResponsePath)."
    }
}

$sshArgs = @(
    "-o", "BatchMode=yes",
    "-o", "StrictHostKeyChecking=accept-new",
    "-i", $SshKeyPath,
    "$SshUser@$SshHost",
    "docker exec -i new-api-seedance-postgres-1 psql -U newapi -d newapi -X -q -t -A"
)
$escapedTokenName = $TokenName.Replace("'", "''")
$tokenSql = "select key from tokens where id = $TokenId and name = '$escapedTokenName' and deleted_at is null;"
$tokenValue = ($tokenSql | & ssh @sshArgs | Out-String).Trim()
if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($tokenValue)) {
    throw "Unable to load the E2E API token from the production database."
}
$apiKey = if ($tokenValue.StartsWith("sk-")) { $tokenValue } else { "sk-$tokenValue" }

try {
    $callbackUrl = "$BaseUrl/apidocs/apidocs-example-callback.html"

    $visualSession = Invoke-JsonEndpoint `
        -Name "01-create-visual-session" `
        -Method POST `
        -Path "/api/v3/open/CreateVisualValidateSession" `
        -Body ([ordered]@{ CallbackURL = $callbackUrl })
    Assert-BusinessSuccess -Result $visualSession

    if ($null -ne $visualSession.Body.data.Result) {
        $visualSession.Body.data.Result.BytedToken = "<redacted>"
        $visualSession.Body.data.Result.H5Link = "<redacted>"
    }
    if ($null -ne $visualSession.Body.data.ResponseMetadata.RequestId) {
        $visualSession.Body.data.ResponseMetadata.RequestId = "<redacted>"
    }
    Write-Utf8Json -Path $visualSession.ResponsePath -Value $visualSession.Body

    $invalidVisualResult = Invoke-JsonEndpoint `
        -Name "02-get-visual-result-invalid-token" `
        -Method POST `
        -Path "/api/v3/open/GetVisualValidateResult" `
        -Body ([ordered]@{ BytedToken = "invalid-e2e-$runName" })
    Assert-Http200 -Result $invalidVisualResult
    if ($invalidVisualResult.Body.state -ne 0) {
        throw "The invalid-token visual-result check unexpectedly returned state=$($invalidVisualResult.Body.state)."
    }

    $assetGroup = Invoke-JsonEndpoint `
        -Name "03-create-asset-group" `
        -Method POST `
        -Path "/api/v3/open/CreateAssetGroup" `
        -Body ([ordered]@{
            Name = "nexus-ip-e2e-$runName"
            Description = "Public IP Seedance 2.0 E2E validation"
        })
    Assert-BusinessSuccess -Result $assetGroup
    $groupId = [string]$assetGroup.Body.data.Id
    if ([string]::IsNullOrWhiteSpace($groupId)) {
        throw "CreateAssetGroup did not return data.Id."
    }

    $asset = Invoke-JsonEndpoint `
        -Name "04-create-image-asset" `
        -Method POST `
        -Path "/api/v3/open/CreateAsset" `
        -Body ([ordered]@{
            GroupId = $groupId
            URL = $ImageUrl
            Name = "nexus-ip-e2e-virtual-avatar-$runName"
            AssetType = "Image"
        })
    Assert-BusinessSuccess -Result $asset
    $assetId = [string]$asset.Body.data.Id
    if ([string]::IsNullOrWhiteSpace($assetId)) {
        throw "CreateAsset did not return data.Id."
    }

    $assetDeadline = (Get-Date).AddSeconds($TimeoutSeconds)
    $assetPoll = 0
    $assetStatus = ""
    do {
        $assetPoll++
        $assetResult = Invoke-JsonEndpoint `
            -Name ("05-get-asset-{0:d3}" -f $assetPoll) `
            -Method POST `
            -Path "/api/v3/open/GetAsset" `
            -Body ([ordered]@{ Id = $assetId })
        Assert-BusinessSuccess -Result $assetResult
        $assetStatus = [string]$assetResult.Body.data.Status
        Write-Output "asset_status=$assetStatus"
        if ($assetStatus -eq "Failed") {
            throw "The E2E image asset failed processing."
        }
        if ($assetStatus -ne "Active") {
            Start-Sleep -Seconds $PollIntervalSeconds
        }
    } while ($assetStatus -ne "Active" -and (Get-Date) -lt $assetDeadline)
    if ($assetStatus -ne "Active") {
        throw "The E2E image asset did not become Active before timeout."
    }

    $privateBillRoute = Invoke-JsonEndpoint `
        -Name "06-private-bill-route-must-not-exist" `
        -Method POST `
        -Path "/api/v3/open/ListSplitBillDetail" `
        -Body ([ordered]@{})
    if ($privateBillRoute.HttpStatus -ne 404) {
        throw "ListSplitBillDetail must remain private, but the public route returned HTTP $($privateBillRoute.HttpStatus)."
    }

    $videoBody = [ordered]@{
        model = "doubao-seedance-2-0-260128"
        content = @(
            [ordered]@{
                type = "image_url"
                image_url = [ordered]@{ url = "asset://$assetId" }
                role = "reference_image"
            },
            [ordered]@{
                type = "text"
                text = "图片1中的虚拟人物自然眨眼并轻微呼吸，镜头固定，无文字。"
            }
        )
        audio_status = 0
        resolution = "720p"
        ratio = "16:9"
        dur = 4
    }
    $videoSubmit = Invoke-JsonEndpoint `
        -Name "07-create-video" `
        -Method POST `
        -Path "/v1/video/generations" `
        -Body $videoBody
    Assert-Http200 -Result $videoSubmit
    $taskId = [string]$videoSubmit.Body.task_id
    if ([string]::IsNullOrWhiteSpace($taskId)) {
        $taskId = [string]$videoSubmit.Body.id
    }
    if ([string]::IsNullOrWhiteSpace($taskId)) {
        throw "Video submission did not return a public task ID."
    }

    $taskDeadline = (Get-Date).AddSeconds($TimeoutSeconds)
    $taskPoll = 0
    $taskStatus = ""
    do {
        $taskPoll++
        $taskResult = Invoke-JsonEndpoint `
            -Name ("08-get-video-task-{0:d3}" -f $taskPoll) `
            -Method GET `
            -Path "/v1/video/generations/$taskId"
        Assert-Http200 -Result $taskResult
        $taskStatus = [string]$taskResult.Body.data.status
        Write-Output "video_status=$taskStatus progress=$($taskResult.Body.data.progress)"
        if ($taskStatus -eq "FAILURE") {
            throw "The E2E video task failed: $($taskResult.Body.data.fail_reason)"
        }
        if ($taskStatus -ne "SUCCESS") {
            Start-Sleep -Seconds $PollIntervalSeconds
        }
    } while ($taskStatus -ne "SUCCESS" -and (Get-Date) -lt $taskDeadline)
    if ($taskStatus -ne "SUCCESS") {
        throw "The E2E video task did not succeed before timeout."
    }

    $openAiTask = Invoke-JsonEndpoint `
        -Name "09-get-video-task-openai" `
        -Method GET `
        -Path "/v1/videos/$taskId"
    Assert-Http200 -Result $openAiTask
    if ([string]$openAiTask.Body.status -ne "completed") {
        throw "The OpenAI-compatible task query did not return completed."
    }

    $contentHeadersPath = Join-Path $runDir "10-download-video.response.headers"
    $contentPath = Join-Path $runDir "10-download-video.mp4"
    $contentHttp = (& curl.exe `
        -sS `
        --connect-timeout 15 `
        --max-time 240 `
        -D $contentHeadersPath `
        -o $contentPath `
        -w "%{http_code}" `
        "$BaseUrl/v1/videos/$taskId/content" `
        -H "Authorization: Bearer $apiKey").Trim()
    if ($LASTEXITCODE -ne 0 -or [int]$contentHttp -ne 200) {
        throw "Video content download failed with HTTP $contentHttp and curl exit $LASTEXITCODE."
    }
    $contentInfo = Get-Item -LiteralPath $contentPath
    if ($contentInfo.Length -le 0) {
        throw "Video content download returned an empty file."
    }
    $firstBytes = [IO.File]::ReadAllBytes($contentPath)
    $prefixLength = [Math]::Min(16, $firstBytes.Length)
    $prefixHex = -join ($firstBytes[0..($prefixLength - 1)] | ForEach-Object { $_.ToString("x2") })
    $contentHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $contentPath).Hash.ToLowerInvariant()

    $summary = [ordered]@{
        run_name = $runName
        base_url = $BaseUrl
        token_id = $TokenId
        group_id = $groupId
        asset_id = $assetId
        asset_status = $assetStatus
        task_id = $taskId
        task_status = $taskStatus
        video_spec = [ordered]@{
            resolution = "720p"
            duration_seconds = 4
            ratio = "16:9"
            audio_status = 0
            has_video_input = $false
            estimated_tokens = 86400
            cny_per_million_tokens = 46
            estimated_cost_cny = 3.9744
        }
        content = [ordered]@{
            bytes = $contentInfo.Length
            sha256 = $contentHash
            first_16_bytes_hex = $prefixHex
            headers_path = $contentHeadersPath
            file_path = $contentPath
        }
        result_directory = $runDir
    }
    $summaryPath = Join-Path $runDir "summary.json"
    Write-Utf8Json -Path $summaryPath -Value $summary
    Write-Output "E2E_SUCCESS summary=$summaryPath task_id=$taskId asset_id=$assetId video_bytes=$($contentInfo.Length) sha256=$contentHash"
}
finally {
    $apiKey = $null
    $tokenValue = $null
}
