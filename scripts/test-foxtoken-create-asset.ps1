param(
    [string]$UpstreamBaseUrl = "https://foxtoken.linkomobile.com",

    [string]$UpstreamApiKey = $env:FOXTOKEN_API_KEY,

    [string]$Model = "doubao-seedance-2-0-filter-off",

    [string]$ImageUrl = "http://115.190.59.253:8701/files/uploads/878e08ea8bcb49118d313ceaefc31fd3.png",

    [string]$AssetName = "ChatGPT Image Jul 2, 2026, 07_53_13 PM.png",

    [ValidateRange(1, 3600)]
    [int]$TimeoutSeconds = 180,

    [ValidateRange(1, 300)]
    [int]$PollIntervalSeconds = 3,

    [ValidateRange(1, 3600)]
    [int]$PollTimeoutSeconds = 90,

    [switch]$SkipPoll
)

$ErrorActionPreference = "Stop"

if ([string]::IsNullOrWhiteSpace($UpstreamApiKey)) {
    $secureKey = Read-Host "Enter the full foxtoken upstream API key" -AsSecureString
    $keyPointer = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secureKey)
    try {
        $UpstreamApiKey = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($keyPointer)
    } finally {
        [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($keyPointer)
    }
}

$UpstreamApiKey = $UpstreamApiKey.Trim()
if ([string]::IsNullOrWhiteSpace($UpstreamApiKey)) {
    throw "The upstream API key cannot be empty."
}

$UpstreamBaseUrl = $UpstreamBaseUrl.TrimEnd('/')
$createEndpoint = "$UpstreamBaseUrl/api/v3/open/CreateAsset"
$getEndpoint = "$UpstreamBaseUrl/api/v3/open/GetAsset"

# Build the JSON as text because the upstream example deliberately contains
# case-distinct duplicate fields (url/URL and name/Name).
$modelJson = ConvertTo-Json -InputObject $Model -Compress
$imageUrlJson = ConvertTo-Json -InputObject $ImageUrl -Compress
$assetNameJson = ConvertTo-Json -InputObject $AssetName -Compress
$createBody = @"
{
  "model": $modelJson,
  "url": $imageUrlJson,
  "URL": $imageUrlJson,
  "AssetType": "Image",
  "name": $assetNameJson,
  "Name": $assetNameJson
}
"@

Add-Type -AssemblyName System.Net.Http
$client = [System.Net.Http.HttpClient]::new()
$client.Timeout = [TimeSpan]::FromSeconds($TimeoutSeconds)

function Invoke-JsonPost {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Uri,

        [Parameter(Mandatory = $true)]
        [string]$JsonBody
    )

    $request = [System.Net.Http.HttpRequestMessage]::new(
        [System.Net.Http.HttpMethod]::Post,
        $Uri
    )
    try {
        $request.Headers.Authorization =
            [System.Net.Http.Headers.AuthenticationHeaderValue]::new(
                "Bearer",
                $UpstreamApiKey
            )
        $request.Headers.Accept.Add(
            [System.Net.Http.Headers.MediaTypeWithQualityHeaderValue]::new(
                "application/json"
            )
        )
        $request.Content = [System.Net.Http.StringContent]::new(
            $JsonBody,
            [Text.Encoding]::UTF8,
            "application/json"
        )

        $response = $client.SendAsync($request).GetAwaiter().GetResult()
        try {
            $responseBody = $response.Content.ReadAsStringAsync().GetAwaiter().GetResult()
            return [pscustomobject]@{
                StatusCode = [int]$response.StatusCode
                IsSuccess  = $response.IsSuccessStatusCode
                Body       = $responseBody
            }
        } finally {
            $response.Dispose()
        }
    } finally {
        $request.Dispose()
    }
}

try {
    Write-Output "create_endpoint=$createEndpoint"
    Write-Output "create_request_begin"
    Write-Output $createBody
    Write-Output "create_request_end"

    $createResult = Invoke-JsonPost -Uri $createEndpoint -JsonBody $createBody
    Write-Output "create_http=$($createResult.StatusCode)"
    Write-Output "create_response=$($createResult.Body)"

    if (-not $createResult.IsSuccess) {
        exit 1
    }

    try {
        $createResponse = $createResult.Body | ConvertFrom-Json
    } catch {
        throw "CreateAsset returned a non-JSON success response: $($createResult.Body)"
    }

    $assetId = [string]$createResponse.id
    if ([string]::IsNullOrWhiteSpace($assetId)) {
        $assetId = [string]$createResponse.Id
    }
    if ([string]::IsNullOrWhiteSpace($assetId)) {
        $assetId = [string]$createResponse.asset_id
    }
    if ([string]::IsNullOrWhiteSpace($assetId)) {
        throw "CreateAsset succeeded but did not return an asset ID."
    }

    Write-Output "asset_id=$assetId"
    if ($SkipPoll) {
        exit 0
    }

    $getBody = [ordered]@{
        model = $Model
        Id    = $assetId
    } | ConvertTo-Json -Compress

    $deadline = [DateTimeOffset]::UtcNow.AddSeconds($PollTimeoutSeconds)
    $pollNumber = 0
    while ([DateTimeOffset]::UtcNow -lt $deadline) {
        $pollNumber++
        $getResult = Invoke-JsonPost -Uri $getEndpoint -JsonBody $getBody
        Write-Output "poll=$pollNumber get_http=$($getResult.StatusCode) response=$($getResult.Body)"

        if (-not $getResult.IsSuccess) {
            exit 2
        }

        try {
            $asset = $getResult.Body | ConvertFrom-Json
            $status = [string]$asset.Status
            if ([string]::IsNullOrWhiteSpace($status)) {
                $status = [string]$asset.status
            }
        } catch {
            throw "GetAsset returned invalid JSON: $($getResult.Body)"
        }

        if ($status -eq "Active") {
            Write-Output "final_status=Active"
            exit 0
        }
        if ($status -in @("Failed", "Error")) {
            Write-Output "final_status=$status"
            exit 3
        }

        Start-Sleep -Seconds $PollIntervalSeconds
    }

    Write-Output "final_status=Timeout"
    exit 4
} finally {
    $client.Dispose()
    $UpstreamApiKey = $null
}
