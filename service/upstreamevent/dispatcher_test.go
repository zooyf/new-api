package upstreamevent

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPostEventsUsesTokenOperationGatewayHeaders(t *testing.T) {
	var receivedHeaders http.Header
	var receivedBody tokenOperationBulkRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		require.NoError(t, common.DecodeJson(r.Body, &receivedBody))
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	body, err := common.Marshal(tokenOperationBulkRequest{
		BatchID: "batch_1",
		ProviderEvents: []tokenOperationProviderEvent{{
			SourceSystem: "new-api",
			EventID:      "evt_1",
			EventType:    EventUpstreamResponseReceived,
			OccurredAt:   "2026-07-06T00:00:00Z",
		}},
	})
	require.NoError(t, err)

	cfg := Config{
		WebhookURL:               server.URL,
		GatewayKey:               "gw_secret",
		GatewayID:                "new-api-green",
		SchemaVersion:            tokenOperationProviderEventBulkSchema,
		DispatcherRequestTimeout: time.Second,
	}
	statusCode, responsePreview, err := postEvents(cfg, "batch_1", body)

	require.NoError(t, err)
	assert.Equal(t, http.StatusAccepted, statusCode)
	assert.Contains(t, responsePreview, `"ok":true`)
	assert.Equal(t, "gw_secret", receivedHeaders.Get("x-gateway-key"))
	assert.Equal(t, "batch_1", receivedHeaders.Get("idempotency-key"))
	assert.Equal(t, tokenOperationProviderEventBulkSchema, receivedHeaders.Get("x-schema-version"))
	assert.Equal(t, "new-api-green", receivedHeaders.Get("x-gateway-id"))
	assert.Equal(t, "batch_1", receivedBody.BatchID)
	require.Len(t, receivedBody.ProviderEvents, 1)
	assert.Equal(t, "evt_1", receivedBody.ProviderEvents[0].EventID)
}

func TestLoadConfigBuildsTokenOperationBulkURLFromBaseURL(t *testing.T) {
	t.Setenv("UPSTREAM_EVENT_TOKENOP_BASE_URL", "https://ops.ai.p35q.cn/")
	t.Setenv("UPSTREAM_EVENT_GATEWAY_KEY", "gw_secret")
	t.Setenv("NODE_NAME", "new-api-green")

	cfg := LoadConfig()

	assert.Equal(t, defaultTokenOperationProviderSourceSystem, cfg.SourceSystem)
	assert.Equal(t, "new-api-green", cfg.GatewayID)
	assert.Equal(t, "gw_secret", cfg.GatewayKey)
	assert.Equal(t, "https://ops.ai.p35q.cn/api/v1/gateway/provider-events/bulk", cfg.WebhookURL)
	assert.Equal(t, tokenOperationProviderEventBulkSchema, cfg.SchemaVersion)
}

func TestTokenOperationProviderEventMapsClaudeContractFields(t *testing.T) {
	event := ProviderEvent{
		SourceSystem: "new-api",
		EventID:      "evt_claude",
		EventType:    EventUpstreamResponseReceived,
		OccurredAt:   "2026-07-06T00:00:00Z",
		RequestID:    "req_claude",
		CustomerContext: CustomerContext{
			GatewayCustomerID: "456",
			GatewayUserID:     "456",
			TokenID:           "123",
		},
		RoutingContext: RoutingContext{
			ChannelID:       "789",
			ModelName:       "claude-sonnet-4",
			CallType:        "chat_completion",
			RelayMode:       "chat_completions",
			RelayFormat:     "claude",
			IsModelMapped:   true,
			OriginModelName: "claude-sonnet-4",
		},
		UsageContext: UsageContext{
			RawUsageJSON: map[string]interface{}{
				"provider": "anthropic",
				"format":   "claude_messages",
				"usage": map[string]interface{}{
					"input_tokens":  1000,
					"output_tokens": 200,
				},
			},
		},
	}

	payload := tokenOperationProviderEventFrom(event)

	assert.Equal(t, "evt_claude", payload.IdempotencyKey)
	assert.Equal(t, "new-api", payload.SourceSystem)
	assert.Equal(t, "text_generation", payload.RoutingContext.CallType)
	assert.Equal(t, "anthropic", payload.RoutingContext.RelayFormat)
	assert.Equal(t, "messages", payload.RoutingContext.RelayMode)
	assert.Equal(t, event.UsageContext.RawUsageJSON, payload.RawUsageJSON)
	assert.Nil(t, payload.RequestBodyJSON)
	assert.Nil(t, payload.ResponseBodyJSON)
}

func TestTokenOperationProviderEventMapsGeminiUsageMetadata(t *testing.T) {
	event := ProviderEvent{
		SourceSystem: "new-api",
		EventID:      "evt_gemini",
		EventType:    EventUpstreamResponseReceived,
		OccurredAt:   "2026-07-06T00:00:00Z",
		RequestID:    "req_gemini",
		CustomerContext: CustomerContext{
			GatewayCustomerID: "456",
			TokenID:           "123",
		},
		RoutingContext: RoutingContext{
			ChannelID:   "789",
			ModelName:   "gemini-2.5-pro",
			CallType:    "text_generation",
			RelayMode:   "gemini",
			RelayFormat: "gemini",
		},
		UsageContext: UsageContext{
			RawUsageJSON: map[string]interface{}{
				"provider": "google",
				"format":   "gemini_generate_content",
				"usage": map[string]interface{}{
					"promptTokenCount":     800,
					"candidatesTokenCount": 300,
				},
			},
		},
	}

	payload := tokenOperationProviderEventFrom(event)

	assert.Equal(t, "generate_content", payload.RoutingContext.RelayMode)
	assert.Equal(t, "gemini", payload.RoutingContext.RelayFormat)
	assert.Equal(t, payload.RawUsageJSON["usage"], payload.RawUsageJSON["usageMetadata"])
}

func TestTokenOperationProviderEventMapsSeedanceTaskEvent(t *testing.T) {
	event := ProviderEvent{
		SourceSystem:   "new-api",
		EventID:        "evt_seedance",
		EventType:      EventTaskCompleted,
		OccurredAt:     "2026-07-06T00:00:00Z",
		TaskID:         "task_public",
		UpstreamTaskID: "seedance_task_1",
		CustomerContext: CustomerContext{
			GatewayCustomerID: "456",
			TokenID:           "123",
		},
		RoutingContext: RoutingContext{
			ChannelID:         "789",
			ModelName:         "doubao-seedance-2-0-filter-off",
			UpstreamModelName: "doubao-seedance-2-0-filter-off",
			CallType:          "video_generation",
			RelayMode:         "video_task",
			RelayFormat:       "task",
		},
		UsageContext: UsageContext{
			RequestMetadataJSON: map[string]interface{}{
				"duration":   5,
				"resolution": "720p",
				"ratio":      "16:9",
			},
			ResponseMetadataJSON: map[string]interface{}{
				"id":     "seedance_task_1",
				"status": "SUCCESS",
			},
		},
	}

	payload := tokenOperationProviderEventFrom(event)

	assert.Equal(t, "upstream.task_succeeded", payload.EventType)
	assert.Equal(t, "volcengine-ark", payload.RoutingContext.RelayFormat)
	assert.Equal(t, "async_video", payload.RoutingContext.RelayMode)
	assert.Equal(t, "task_public", payload.RequestID)
	require.NotNil(t, payload.RawUsageJSON)
	usage, ok := payload.RawUsageJSON["usage"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, 1, usage["request_count"])
	assert.Equal(t, 5, usage["duration_seconds"])
	assert.Equal(t, "SUCCESS", usage["status"])
	assert.Equal(t, "seedance_task_1", payload.ExtraJSON["upstream_task_id"])
	assert.Equal(t, "SUCCESS", payload.ExtraJSON["provider_status"])
}
