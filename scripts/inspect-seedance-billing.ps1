param(
    [Parameter(Mandatory = $true)]
    [ValidatePattern('^[A-Za-z0-9_.:-]+$')]
    [string]$TaskId,

    [string]$SshHost = "nexus-sg",

    [string]$DatabaseContainer = "new-api-postgres",

    [string]$DatabaseUser = "newapi",

    [string]$DatabaseName = "newapi",

    [ValidateSet("Text", "Json")]
    [string]$Format = "Text"
)

$ErrorActionPreference = "Stop"
$quotaPerUnit = 500000.0

$sql = @"
select json_build_object(
  'task', json_build_object(
    'id', t.id,
    'task_id', t.task_id,
    'upstream_task_id', t.private_data->>'upstream_task_id',
    'created_at', t.created_at,
    'updated_at', t.updated_at,
    'status', t.status,
    'user_id', t.user_id,
    'username', u.username,
    'token_id', t.private_data->>'token_id',
    'group', t."group",
    'channel_id', t.channel_id,
    'channel_name', c.name,
    'channel_type', c.type,
    'stored_quota', t.quota,
    'properties', t.properties,
    'billing_context', t.private_data->'billing_context',
    'data', t.data
  ),
  'logs', coalesce((
    select json_agg(json_build_object(
      'id', l.id,
      'created_at', l.created_at,
      'type', l.type,
      'quota', l.quota,
      'content', l.content,
      'token_name', l.token_name,
      'model_name', l.model_name,
      'other', l.other::jsonb
    ) order by l.id)
    from logs l
    where l.other::jsonb->>'task_id' = '$TaskId'
  ), '[]'::json)
)::text
from tasks t
left join users u on u.id = t.user_id
left join channels c on c.id = t.channel_id
where t.task_id = '$TaskId';
"@

$remoteCommand = "docker exec -i $DatabaseContainer psql -U $DatabaseUser -d $DatabaseName -X -q -t -A"
$raw = $sql | & ssh $SshHost $remoteCommand
if ($LASTEXITCODE -ne 0) {
    throw "Production read-only query failed with exit code $LASTEXITCODE."
}

$rawJson = ($raw -join "`n").Trim()
if ([string]::IsNullOrWhiteSpace($rawJson)) {
    throw "Task not found: $TaskId"
}

$record = $rawJson | ConvertFrom-Json
$task = $record.task
$billing = $task.billing_context
$data = $task.data

$modelName = [string]$billing.origin_model_name
if ([string]::IsNullOrWhiteSpace($modelName)) {
    $modelName = [string]$task.properties.origin_model_name
}

$groupRatio = if ($null -ne $billing.group_ratio) { [double]$billing.group_ratio } else { 0.0 }
$modelRatio = if ($null -ne $billing.model_ratio) { [double]$billing.model_ratio } else { 0.0 }
$totalTokens = 0L
if ($null -ne $data.usage.total_tokens) {
    $totalTokens = [long]$data.usage.total_tokens
}

$resolution = [string]$data.resolution
$duration = if ($null -ne $data.duration) { [int]$data.duration } else { 0 }
$aspectRatio = [string]$data.ratio

$basePrices = @{
    "doubao-seedance-2-0-filter-off" = 7.0
    "doubao-seedance-2-0-fast-filter-off" = 5.6
    "dreamina-seedance-2-0-mini-filter-off" = 3.5
}

$videoInputRatio = 1.0
$hasVideoInput = $false
if ($null -ne $billing.other_ratios -and $null -ne $billing.other_ratios.video_input) {
    $videoInputRatio = [double]$billing.other_ratios.video_input
    $hasVideoInput = $videoInputRatio -ne 1.0
}

$unitPrice = $null
$calculatedQuota = $null
$calculatedUsd = $null
if ($basePrices.ContainsKey($modelName) -and $totalTokens -gt 0 -and $groupRatio -gt 0) {
    $unitPrice = [math]::Round([double]$basePrices[$modelName] * $videoInputRatio, 10)
    $calculatedQuota = [long][math]::Truncate(
        ([double]$totalTokens / 1000000.0) * $unitPrice * $quotaPerUnit * $groupRatio
    )
    $calculatedUsd = [double]$calculatedQuota / $quotaPerUnit
}

$settlementLog = $null
foreach ($log in $record.logs) {
    if ($null -ne $log.other -and $null -ne $log.other.actual_quota) {
        $settlementLog = $log
    }
}

$preConsumedQuota = [long]$task.stored_quota
$actualQuota = $calculatedQuota
$adjustmentQuota = $null
$adjustmentType = "none"
$logMatchesCalculation = $null
if ($null -ne $settlementLog) {
    $preConsumedQuota = [long]$settlementLog.other.pre_consumed_quota
    $actualQuota = [long]$settlementLog.other.actual_quota
    $adjustmentQuota = [long]$settlementLog.quota
    if ([int]$settlementLog.type -eq 6) {
        $adjustmentType = "refund"
    } elseif ([int]$settlementLog.type -eq 2) {
        $adjustmentType = "consume"
    } else {
        $adjustmentType = "log_type_$($settlementLog.type)"
    }
    if ($null -ne $calculatedQuota) {
        $logMatchesCalculation = $actualQuota -eq $calculatedQuota
    }
}

$result = [ordered]@{
    task_id = [string]$task.task_id
    upstream_task_id = [string]$task.upstream_task_id
    status = [string]$task.status
    user = [string]$task.username
    token_id = [string]$task.token_id
    token_name = if ($record.logs.Count -gt 0) { [string]$record.logs[0].token_name } else { "" }
    group = [string]$task.group
    channel = "$($task.channel_name) ($($task.channel_id), type=$($task.channel_type))"
    billing_model = $modelName
    upstream_model = [string]$task.properties.upstream_model_name
    resolution = $resolution
    duration_seconds = $duration
    aspect_ratio = $aspectRatio
    has_video_input = $hasVideoInput
    total_tokens = $totalTokens
    usd_per_million_tokens = $unitPrice
    group_ratio = $groupRatio
    quota_per_usd = [long]$quotaPerUnit
    calculated_quota = $calculatedQuota
    logged_actual_quota = $actualQuota
    final_usd = if ($null -ne $actualQuota) { [double]$actualQuota / $quotaPerUnit } else { $null }
    pre_consumed_quota = $preConsumedQuota
    pre_consumed_usd = [double]$preConsumedQuota / $quotaPerUnit
    adjustment_type = $adjustmentType
    adjustment_quota = $adjustmentQuota
    adjustment_usd = if ($null -ne $adjustmentQuota) { [double]$adjustmentQuota / $quotaPerUnit } else { $null }
    settlement_log_id = if ($null -ne $settlementLog) { [long]$settlementLog.id } else { $null }
    log_matches_calculation = $logMatchesCalculation
}

if ($Format -eq "Json") {
    $result | ConvertTo-Json -Depth 5
    exit 0
}

Write-Output "Task:              $($result.task_id)"
Write-Output "Upstream task:     $($result.upstream_task_id)"
Write-Output "Status:            $($result.status)"
Write-Output "User / token:      $($result.user) / $($result.token_name) (token_id=$($result.token_id))"
Write-Output "Group:             $($result.group) (ratio=$($result.group_ratio))"
Write-Output "Channel:           $($result.channel)"
Write-Output "Billing model:     $($result.billing_model)"
Write-Output "Upstream model:    $($result.upstream_model)"
Write-Output "Output:            $($result.resolution), $($result.duration_seconds)s, $($result.aspect_ratio)"
Write-Output "Video input:       $($result.has_video_input)"
Write-Output "Usage:             $($result.total_tokens) tokens"

if ($null -ne $unitPrice) {
    Write-Output "Unit price:        $unitPrice USD / 1M tokens"
    Write-Output "Formula:           int($totalTokens / 1000000 * $unitPrice * $([long]$quotaPerUnit) * $groupRatio)"
    Write-Output "Calculated quota:  $calculatedQuota"
} else {
    Write-Output "Formula:           unavailable (unsupported model or missing billing evidence)"
}

Write-Output "Pre-consumed:      $preConsumedQuota quota ($($result.pre_consumed_usd) USD)"
if ($null -ne $adjustmentQuota) {
    Write-Output "Adjustment:        $adjustmentType $adjustmentQuota quota ($($result.adjustment_usd) USD)"
}
Write-Output "Final charge:      $actualQuota quota ($($result.final_usd) USD)"
Write-Output "Settlement log:    $($result.settlement_log_id)"
Write-Output "Log matches calc:  $($result.log_matches_calculation)"
