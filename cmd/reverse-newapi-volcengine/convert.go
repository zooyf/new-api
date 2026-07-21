package main

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

func convertSubmitRequest(body []byte) (newAPIVideoRequest, error) {
	return convertSubmitRequestWithModel(body, false)
}

func convertSeedanceOverseasSubmitRequest(body []byte) (newAPIVideoRequest, error) {
	return convertSubmitRequestWithModel(body, true)
}

func convertSubmitRequestWithModel(body []byte, normalizeOverseasModel bool) (newAPIVideoRequest, error) {
	var req volcengineSubmitRequest
	if err := common.Unmarshal(body, &req); err != nil {
		return newAPIVideoRequest{}, fmt.Errorf("invalid JSON body: %w", err)
	}

	model := strings.TrimSpace(req.Model)
	if model == "" {
		return newAPIVideoRequest{}, fmt.Errorf("model is required")
	}
	if normalizeOverseasModel {
		model = normalizeSeedanceOverseasRequestModel(model)
	}

	promptParts := make([]string, 0, len(req.Content))
	images := make([]string, 0)
	for _, item := range req.Content {
		if text := strings.TrimSpace(item.Text); text != "" {
			promptParts = append(promptParts, text)
		}
		if item.ImageURL != nil && strings.TrimSpace(item.ImageURL.URL) != "" {
			images = append(images, strings.TrimSpace(item.ImageURL.URL))
		}
	}
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		prompt = strings.Join(promptParts, "\n")
	}
	if strings.TrimSpace(prompt) == "" {
		return newAPIVideoRequest{}, fmt.Errorf("prompt is required")
	}

	var raw map[string]any
	if err := common.Unmarshal(body, &raw); err != nil {
		return newAPIVideoRequest{}, fmt.Errorf("invalid JSON body: %w", err)
	}
	delete(raw, "model")
	delete(raw, "prompt")

	resolution := strings.TrimSpace(req.Resolution)
	if resolution == "" {
		resolution = strings.TrimSpace(req.Size)
	}

	out := newAPIVideoRequest{
		Model:      model,
		Prompt:     prompt,
		Images:     images,
		Resolution: resolution,
		Metadata:   raw,
	}
	if req.Duration != nil {
		out.Duration = *req.Duration
	}
	return out, nil
}

func normalizeSeedanceOverseasRequestModel(model string) string {
	switch model {
	case "dreamina-seedance-2-0-260128", "dreamina-seedance-2-0-ep":
		return "doubao-seedance-2-0-filter-off"
	case "dreamina-seedance-2-0-fast-260128", "dreamina-seedance-2-0-fast-ep":
		return "doubao-seedance-2-0-fast-filter-off"
	default:
		return model
	}
}

func toSeedanceOverseasResponseModel(model string) string {
	switch model {
	case "doubao-seedance-2-0-filter-off", "dreamina-seedance-2-0-ep", "dreamina-seedance-2-0-260128":
		return "doubao-seedance-2-0-260128"
	case "doubao-seedance-2-0-fast-filter-off", "dreamina-seedance-2-0-fast-ep", "dreamina-seedance-2-0-fast-260128":
		return "doubao-seedance-2-0-fast-260128"
	default:
		return model
	}
}

func convertSubmitResponse(body []byte) (volcengineSubmitResponse, error) {
	var raw map[string]any
	if err := common.Unmarshal(body, &raw); err != nil {
		return volcengineSubmitResponse{}, fmt.Errorf("invalid upstream JSON body: %w", err)
	}

	taskID := firstString(raw, "id", "task_id")
	if taskID == "" {
		if data, ok := mapValue(raw["data"]); ok {
			taskID = firstString(data, "task_id", "id")
		} else if dataStr, ok := stringValue(raw["data"]); ok {
			taskID = dataStr
		}
	}
	if taskID == "" {
		return volcengineSubmitResponse{}, fmt.Errorf("task id is empty")
	}

	return volcengineSubmitResponse{ID: taskID}, nil
}

func convertFetchResponse(body []byte, requestedTaskID string) (volcengineTaskResponse, error) {
	var raw map[string]any
	if err := common.Unmarshal(body, &raw); err != nil {
		return volcengineTaskResponse{}, fmt.Errorf("invalid upstream JSON body: %w", err)
	}

	if data, ok := mapValue(raw["data"]); ok {
		return convertTaskDataResponse(data, requestedTaskID), nil
	}
	if _, hasObject := stringValue(raw["object"]); hasObject {
		return convertOpenAIVideoResponse(raw, requestedTaskID), nil
	}
	if firstString(raw, "task_id", "id") != "" && firstString(raw, "status") != "" {
		return convertVideoTaskResponse(raw, requestedTaskID), nil
	}

	return volcengineTaskResponse{}, fmt.Errorf("unsupported upstream response shape")
}

func convertTaskDataResponse(data map[string]any, requestedTaskID string) volcengineTaskResponse {
	out := volcengineTaskResponse{
		ID:        firstNonEmpty(firstString(data, "task_id", "id"), requestedTaskID),
		Model:     modelFromTaskData(data),
		Status:    toVolcengineStatus(firstString(data, "status")),
		CreatedAt: int64Value(data["created_at"]),
		UpdatedAt: int64Value(data["updated_at"]),
	}

	taskRawData, _ := mapValue(data["data"])
	applyVolcengineTaskFields(&out, taskRawData)

	resultURL := firstString(data, "result_url")
	if resultURL == "" && out.Status == "succeeded" {
		resultURL = firstString(data, "fail_reason")
	}
	if resultURL != "" {
		out.Content.VideoURL = resultURL
	}
	if out.Content.VideoURL == "" {
		out.Content.VideoURL = videoURLFromSource(taskRawData)
	}

	if out.Status == "failed" {
		message := firstString(data, "fail_reason")
		if message == "" && out.Error != nil {
			message = out.Error.Message
		}
		if message != "" {
			out.Error = &volcengineTaskError{
				Code:    firstNonEmpty(errorCodeFromSource(taskRawData), "upstream_task_failed"),
				Message: message,
			}
		}
	}

	return out
}

func convertOpenAIVideoResponse(raw map[string]any, requestedTaskID string) volcengineTaskResponse {
	out := volcengineTaskResponse{
		ID:        firstNonEmpty(firstString(raw, "id", "task_id"), requestedTaskID),
		Model:     firstString(raw, "model"),
		Status:    toVolcengineStatus(firstString(raw, "status")),
		CreatedAt: int64Value(raw["created_at"]),
		UpdatedAt: int64Value(raw["completed_at"]),
		Duration:  intValue(raw["seconds"]),
	}
	if metadata, ok := mapValue(raw["metadata"]); ok {
		out.Content.VideoURL = firstString(metadata, "url", "video_url")
		applyVolcengineTaskFields(&out, metadata)
	}
	if errObj, ok := mapValue(raw["error"]); ok {
		out.Error = &volcengineTaskError{
			Code:    firstString(errObj, "code"),
			Message: firstString(errObj, "message"),
		}
	}
	return out
}

func convertVideoTaskResponse(raw map[string]any, requestedTaskID string) volcengineTaskResponse {
	out := volcengineTaskResponse{
		ID:     firstNonEmpty(firstString(raw, "task_id", "id"), requestedTaskID),
		Model:  firstString(raw, "model"),
		Status: toVolcengineStatus(firstString(raw, "status")),
	}
	out.Content.VideoURL = firstString(raw, "url", "result_url")
	if metadata, ok := mapValue(raw["metadata"]); ok {
		applyVolcengineTaskFields(&out, metadata)
	}
	if errObj, ok := mapValue(raw["error"]); ok {
		out.Error = &volcengineTaskError{
			Code:    firstString(errObj, "code"),
			Message: firstString(errObj, "message"),
		}
	}
	return out
}

func modelFromTaskData(data map[string]any) string {
	if props, ok := mapValue(data["properties"]); ok {
		if modelName := firstString(props, "origin_model_name", "upstream_model_name"); modelName != "" {
			return modelName
		}
	}
	if taskRawData, ok := mapValue(data["data"]); ok {
		return firstString(taskRawData, "model")
	}
	return firstString(data, "model")
}

func applyVolcengineTaskFields(out *volcengineTaskResponse, source map[string]any) {
	if source == nil {
		return
	}
	if out.Model == "" {
		out.Model = firstString(source, "model")
	}
	if out.Content.VideoURL == "" {
		out.Content.VideoURL = videoURLFromSource(source)
	}
	if out.Seed == 0 {
		out.Seed = intValue(source["seed"])
	}
	if out.Resolution == "" {
		out.Resolution = firstString(source, "resolution")
	}
	if out.Duration == 0 {
		out.Duration = intValue(source["duration"])
	}
	if out.Ratio == "" {
		out.Ratio = firstString(source, "ratio")
	}
	if out.FramesPerSecond == 0 {
		out.FramesPerSecond = firstNonZeroInt(intValue(source["framespersecond"]), intValue(source["frames_per_second"]))
	}
	if out.ServiceTier == "" {
		out.ServiceTier = firstString(source, "service_tier")
	}
	if out.ExecutionExpiresAfter == nil {
		out.ExecutionExpiresAfter = intPointer(source, "execution_expires_after")
	}
	if out.GenerateAudio == nil {
		out.GenerateAudio = boolPointer(source, "generate_audio")
	}
	if out.Draft == nil {
		out.Draft = boolPointer(source, "draft")
	}
	if out.Priority == nil {
		out.Priority = intPointer(source, "priority")
	}
	if len(out.Tools) == 0 {
		out.Tools = toolsFromSource(source["tools"])
	}
	applyUsage(out, source)
	applyError(out, source)
}

func applyUsage(out *volcengineTaskResponse, source map[string]any) {
	usage, ok := mapValue(source["usage"])
	if !ok {
		return
	}
	out.Usage.CompletionTokens = intValue(usage["completion_tokens"])
	out.Usage.TotalTokens = intValue(usage["total_tokens"])
	if toolUsage, ok := mapValue(usage["tool_usage"]); ok {
		out.Usage.ToolUsage.WebSearch = intValue(toolUsage["web_search"])
	}
}

func applyError(out *volcengineTaskResponse, source map[string]any) {
	errObj, ok := mapValue(source["error"])
	if !ok {
		return
	}
	code := firstString(errObj, "code")
	message := firstString(errObj, "message")
	if code == "" && message == "" {
		return
	}
	out.Error = &volcengineTaskError{Code: code, Message: message}
}

func videoURLFromSource(source map[string]any) string {
	if source == nil {
		return ""
	}
	if content, ok := mapValue(source["content"]); ok {
		if url := firstString(content, "video_url", "url"); url != "" {
			return url
		}
	}
	return firstString(source, "url", "video_url", "result_url")
}

func errorCodeFromSource(source map[string]any) string {
	if source == nil {
		return ""
	}
	if errObj, ok := mapValue(source["error"]); ok {
		return firstString(errObj, "code")
	}
	return ""
}

func toolsFromSource(value any) []volcengineTool {
	items, ok := value.([]any)
	if !ok || len(items) == 0 {
		return nil
	}
	tools := make([]volcengineTool, 0, len(items))
	for _, item := range items {
		itemMap, ok := mapValue(item)
		if !ok {
			continue
		}
		if toolType := firstString(itemMap, "type"); toolType != "" {
			tools = append(tools, volcengineTool{Type: toolType})
		}
	}
	return tools
}

func toVolcengineStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "not_start", "submitted", "queued", "pending":
		return "queued"
	case "in_progress", "processing", "running":
		return "processing"
	case "success", "completed", "succeeded":
		return "succeeded"
	case "failure", "failed":
		return "failed"
	default:
		return "processing"
	}
}

func firstString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := stringValue(values[key]); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func stringValue(value any) (string, bool) {
	switch v := value.(type) {
	case string:
		return v, true
	case fmt.Stringer:
		return v.String(), true
	case float64:
		if v == float64(int64(v)) {
			return fmt.Sprintf("%d", int64(v)), true
		}
		return fmt.Sprintf("%g", v), true
	case int:
		return fmt.Sprintf("%d", v), true
	case int64:
		return fmt.Sprintf("%d", v), true
	default:
		return "", false
	}
}

func mapValue(value any) (map[string]any, bool) {
	if value == nil {
		return nil, false
	}
	if m, ok := value.(map[string]any); ok {
		return m, true
	}
	if s, ok := value.(string); ok && strings.TrimSpace(s) != "" {
		var m map[string]any
		if err := common.Unmarshal([]byte(s), &m); err == nil {
			return m, true
		}
	}
	return nil, false
}

func intValue(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case string:
		var parsed int
		if _, err := fmt.Sscanf(v, "%d", &parsed); err == nil {
			return parsed
		}
		return 0
	default:
		return 0
	}
}

func intPointer(source map[string]any, key string) *int {
	value, ok := source[key]
	if !ok || value == nil {
		return nil
	}
	parsed := intValue(value)
	return &parsed
}

func boolPointer(source map[string]any, key string) *bool {
	value, ok := source[key]
	if !ok || value == nil {
		return nil
	}
	parsed, ok := value.(bool)
	if !ok {
		return nil
	}
	return &parsed
}

func int64Value(value any) int64 {
	switch v := value.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	case string:
		var parsed int64
		if _, err := fmt.Sscanf(v, "%d", &parsed); err == nil {
			return parsed
		}
		return 0
	default:
		return 0
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstNonZeroInt(values ...int) int {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}
