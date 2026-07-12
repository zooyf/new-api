package upstreamevent

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/model"
)

func tokenOperationBatchID(rows []model.UpstreamEventOutbox) string {
	if len(rows) == 0 {
		return "new-api-provider-batch-empty"
	}
	var key strings.Builder
	for _, row := range rows {
		key.WriteString("|")
		key.WriteString(row.EventID)
	}
	sum := sha256.Sum256([]byte(key.String()))
	return fmt.Sprintf("new-api-provider-batch-%d-%d-%s", rows[0].ID, rows[len(rows)-1].ID, hex.EncodeToString(sum[:8]))
}

func tokenOperationProviderEventFrom(event ProviderEvent) tokenOperationProviderEvent {
	extra := tokenOperationExtra(event)
	return tokenOperationProviderEvent{
		IdempotencyKey:   event.EventID,
		SourceSystem:     event.SourceSystem,
		EventID:          event.EventID,
		EventType:        tokenOperationEventType(event),
		OccurredAt:       event.OccurredAt,
		RequestID:        tokenOperationRequestID(event),
		CustomerContext:  event.CustomerContext,
		RoutingContext:   tokenOperationRoutingContext(event),
		RequestBodyJSON:  cloneExtra(event.UsageContext.RequestMetadataJSON),
		ResponseBodyJSON: cloneExtra(event.UsageContext.ResponseMetadataJSON),
		RawUsageJSON:     tokenOperationRawUsage(event),
		UsageJSON:        cloneExtra(event.UsageContext.UsageJSON),
		PayloadHashes:    event.PayloadHashes,
		ExtraJSON:        extra,
	}
}

func tokenOperationEventType(event ProviderEvent) string {
	switch event.EventType {
	case EventTaskSubmitRequest, EventTaskSubmitResponse:
		return "upstream.task_submitted"
	case EventTaskCompleted:
		return "upstream.task_succeeded"
	case EventTaskFailed:
		return "upstream.task_failed"
	default:
		return event.EventType
	}
}

func tokenOperationRequestID(event ProviderEvent) string {
	if strings.TrimSpace(event.RequestID) != "" {
		return event.RequestID
	}
	if strings.TrimSpace(event.TaskID) != "" {
		return event.TaskID
	}
	if strings.TrimSpace(event.UpstreamTaskID) != "" {
		return event.UpstreamTaskID
	}
	return event.EventID
}

func tokenOperationRoutingContext(event ProviderEvent) RoutingContext {
	routing := event.RoutingContext
	routing.CallType = tokenOperationCallType(routing.CallType)
	routing.RelayFormat = tokenOperationRelayFormat(event, routing)
	routing.RelayMode = tokenOperationRelayMode(event, routing)
	return routing
}

func tokenOperationCallType(callType string) string {
	switch strings.TrimSpace(callType) {
	case "", "unknown":
		return "unknown"
	case "chat_completion":
		return "text_generation"
	default:
		return callType
	}
}

func tokenOperationRelayFormat(event ProviderEvent, routing RoutingContext) string {
	relayFormat := strings.TrimSpace(strings.ToLower(routing.RelayFormat))
	if isSeedanceVideoEvent(event) {
		return "volcengine-ark"
	}
	switch relayFormat {
	case "claude":
		return "anthropic"
	case "openai", "openai_responses":
		return "openai-compatible"
	default:
		return routing.RelayFormat
	}
}

func tokenOperationRelayMode(event ProviderEvent, routing RoutingContext) string {
	relayMode := strings.TrimSpace(strings.ToLower(routing.RelayMode))
	if isSeedanceVideoEvent(event) {
		return "async_video"
	}
	if strings.EqualFold(routing.RelayFormat, "claude") || strings.EqualFold(routing.RelayFormat, "anthropic") {
		return "messages"
	}
	switch relayMode {
	case "chat_completions", "completions":
		return "chat"
	case "responses", "responses_compact":
		return "responses"
	case "gemini":
		return "generate_content"
	default:
		return routing.RelayMode
	}
}

func tokenOperationRawUsage(event ProviderEvent) map[string]interface{} {
	rawUsage := cloneExtra(event.UsageContext.RawUsageJSON)
	if len(rawUsage) == 0 {
		rawUsage = map[string]interface{}{}
	}
	if tokenOperationRawUsageFormat(rawUsage, event) == "gemini" {
		if usage, ok := rawUsage["usage"].(map[string]interface{}); ok {
			if _, exists := rawUsage["usageMetadata"]; !exists {
				rawUsage["usageMetadata"] = usage
			}
		}
	}
	if isSeedanceVideoEvent(event) {
		usage, ok := rawUsage["usage"].(map[string]interface{})
		if !ok || usage == nil {
			usage = map[string]interface{}{}
			rawUsage["usage"] = usage
		} else {
			usage = cloneExtra(usage)
			rawUsage["usage"] = usage
		}
		if _, ok := usage["request_count"]; !ok {
			usage["request_count"] = 1
		}
		if _, ok := usage["duration_seconds"]; !ok {
			if duration, ok := firstEventValue(
				event.UsageContext.ResponseMetadataJSON,
				event.UsageContext.RequestMetadataJSON,
				"duration",
				"seconds",
			); ok {
				usage["duration_seconds"] = duration
			}
		}
		if _, ok := usage["status"]; !ok {
			if status := tokenOperationProviderStatus(event); status != "" {
				usage["status"] = status
			}
		}
	}
	if len(rawUsage) == 0 {
		return nil
	}
	return rawUsage
}

func tokenOperationRawUsageFormat(rawUsage map[string]interface{}, event ProviderEvent) string {
	format := strings.ToLower(fmt.Sprint(rawUsage["format"]))
	if strings.Contains(format, "gemini") {
		return "gemini"
	}
	relayFormat := strings.ToLower(event.RoutingContext.RelayFormat)
	if strings.Contains(relayFormat, "gemini") {
		return "gemini"
	}
	return ""
}

func tokenOperationExtra(event ProviderEvent) map[string]interface{} {
	extra := cloneExtra(event.Extra)
	if extra == nil {
		extra = map[string]interface{}{}
	}
	for k, v := range event.UsageContext.ExtraJSON {
		extra[k] = v
	}
	if event.UpstreamRequestID != "" {
		extra["upstream_request_id"] = event.UpstreamRequestID
	}
	if event.TaskID != "" {
		extra["task_id"] = event.TaskID
	}
	if event.UpstreamTaskID != "" {
		extra["upstream_task_id"] = event.UpstreamTaskID
	}
	if status := tokenOperationProviderStatus(event); status != "" {
		extra["provider_status"] = status
	}
	if len(extra) == 0 {
		return nil
	}
	return extra
}

func tokenOperationProviderStatus(event ProviderEvent) string {
	if status, ok := firstEventValue(event.UsageContext.ResponseMetadataJSON, event.UsageContext.ExtraJSON, "status", "task_status"); ok {
		return fmt.Sprint(status)
	}
	switch event.EventType {
	case EventTaskCompleted:
		return "success"
	case EventTaskFailed:
		return "failed"
	case EventTaskSubmitRequest, EventTaskSubmitResponse:
		return "submitted"
	default:
		return ""
	}
}

func isSeedanceVideoEvent(event ProviderEvent) bool {
	if tokenOperationCallType(event.RoutingContext.CallType) != "video_generation" {
		return false
	}
	needle := strings.ToLower(strings.Join([]string{
		event.RoutingContext.ModelName,
		event.RoutingContext.OriginModelName,
		event.RoutingContext.UpstreamModelName,
		event.RoutingContext.ChannelType,
		event.RoutingContext.RelayMode,
		event.RoutingContext.RelayFormat,
	}, "|"))
	return strings.Contains(needle, "seedance") || strings.Contains(needle, "volc") || strings.Contains(needle, "doubao")
}

func firstEventValue(first map[string]interface{}, second map[string]interface{}, keys ...string) (interface{}, bool) {
	for _, key := range keys {
		if first != nil {
			if value, ok := first[key]; ok && value != nil && fmt.Sprint(value) != "" {
				return value, true
			}
		}
		if second != nil {
			if value, ok := second[key]; ok && value != nil && fmt.Sprint(value) != "" {
				return value, true
			}
		}
	}
	return nil, false
}
