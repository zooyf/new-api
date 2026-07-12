package upstreamevent

import "time"

const (
	SchemaVersion = "v1"

	EventUpstreamResponseReceived = "upstream.response_received"
	EventTaskSubmitRequest        = "task.submit_request"
	EventTaskSubmitResponse       = "task.submit_response"
	EventTaskCompleted            = "task.completed"
	EventTaskFailed               = "task.failed"
	EventBillingDownstreamDelta   = "billing.downstream_delta"

	PriorityCritical = 0
	PriorityHigh     = 1
	PriorityNormal   = 2
)

type ProviderEvent struct {
	SchemaVersion     string                 `json:"schema_version"`
	SourceSystem      string                 `json:"source_system"`
	EventID           string                 `json:"event_id"`
	EventType         string                 `json:"event_type"`
	OccurredAt        string                 `json:"occurred_at"`
	RequestID         string                 `json:"request_id,omitempty"`
	UpstreamRequestID string                 `json:"upstream_request_id,omitempty"`
	TaskID            string                 `json:"task_id,omitempty"`
	UpstreamTaskID    string                 `json:"upstream_task_id,omitempty"`
	CustomerContext   CustomerContext        `json:"customer_context"`
	RoutingContext    RoutingContext         `json:"routing_context"`
	UsageContext      UsageContext           `json:"usage_context,omitempty"`
	PayloadHashes     PayloadHashes          `json:"payload_hashes,omitempty"`
	Extra             map[string]interface{} `json:"extra_json,omitempty"`
}

type CustomerContext struct {
	GatewayCustomerID string `json:"gateway_customer_id,omitempty"`
	GatewayUserID     string `json:"gateway_user_id,omitempty"`
	TokenID           string `json:"token_id,omitempty"`
	APIKeyFingerprint string `json:"api_key_fingerprint,omitempty"`
	APIKeyLast4       string `json:"api_key_last4,omitempty"`
	APIKeyRedacted    bool   `json:"api_key_redacted,omitempty"`
	Group             string `json:"group,omitempty"`
}

type RoutingContext struct {
	ChannelID         string `json:"channel_id,omitempty"`
	ChannelType       string `json:"channel_type,omitempty"`
	ChannelName       string `json:"channel_name,omitempty"`
	ModelName         string `json:"model_name,omitempty"`
	OriginModelName   string `json:"origin_model_name,omitempty"`
	UpstreamModelName string `json:"upstream_model_name,omitempty"`
	CallType          string `json:"call_type,omitempty"`
	RelayMode         string `json:"relay_mode,omitempty"`
	RelayFormat       string `json:"relay_format,omitempty"`
	Method            string `json:"method,omitempty"`
	Path              string `json:"path,omitempty"`
	UpstreamBaseURL   string `json:"upstream_base_url,omitempty"`
	IsStream          bool   `json:"is_stream,omitempty"`
	IsModelMapped     bool   `json:"is_model_mapped,omitempty"`
}

type UsageContext struct {
	RawUsageJSON         map[string]interface{} `json:"raw_usage_json,omitempty"`
	UsageJSON            map[string]interface{} `json:"usage_json,omitempty"`
	RequestMetadataJSON  map[string]interface{} `json:"request_metadata_json,omitempty"`
	ResponseMetadataJSON map[string]interface{} `json:"response_metadata_json,omitempty"`
	ExtraJSON            map[string]interface{} `json:"extra_json,omitempty"`
}

type PayloadHashes struct {
	RequestBodyHash  string `json:"request_body_hash,omitempty"`
	ResponseBodyHash string `json:"response_body_hash,omitempty"`
}

type tokenOperationBulkRequest struct {
	BatchID        string                        `json:"batchId,omitempty"`
	ProviderEvents []tokenOperationProviderEvent `json:"providerEvents"`
}

type tokenOperationProviderEvent struct {
	IdempotencyKey   string                 `json:"idempotency_key,omitempty"`
	SourceSystem     string                 `json:"source_system"`
	EventID          string                 `json:"event_id"`
	EventType        string                 `json:"event_type"`
	OccurredAt       string                 `json:"occurred_at"`
	RequestID        string                 `json:"request_id,omitempty"`
	CustomerContext  CustomerContext        `json:"customer_context"`
	RoutingContext   RoutingContext         `json:"routing_context"`
	RequestBodyJSON  map[string]interface{} `json:"request_body_json,omitempty"`
	ResponseBodyJSON map[string]interface{} `json:"response_body_json,omitempty"`
	RawUsageJSON     map[string]interface{} `json:"raw_usage_json,omitempty"`
	UsageJSON        map[string]interface{} `json:"usage_json,omitempty"`
	PayloadHashes    PayloadHashes          `json:"payload_hashes,omitempty"`
	ExtraJSON        map[string]interface{} `json:"extra_json,omitempty"`
}

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}
