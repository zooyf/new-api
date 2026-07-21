package main

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertSubmitRequest(t *testing.T) {
	body := mustMarshal(t, map[string]any{
		"model": "doubao-seedance-1-0-lite-t2v",
		"content": []any{
			map[string]any{"type": "text", "text": "第一段"},
			map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://example.com/a.png"}},
			map[string]any{"type": "text", "text": "第二段"},
		},
		"duration":   5,
		"resolution": "720p",
		"seed":       0,
		"watermark":  false,
	})

	got, err := convertSubmitRequest(body)
	require.NoError(t, err)
	assert.Equal(t, "doubao-seedance-1-0-lite-t2v", got.Model)
	assert.Equal(t, "第一段\n第二段", got.Prompt)
	assert.Equal(t, []string{"https://example.com/a.png"}, got.Images)
	assert.Equal(t, 5, got.Duration)
	assert.NotContains(t, got.Metadata, "model")
	assert.Equal(t, "720p", got.Metadata["resolution"])
	assert.Equal(t, false, got.Metadata["watermark"])
	assert.Contains(t, got.Metadata, "content")
}

func TestConvertSubmitRequestRequiresModelAndPrompt(t *testing.T) {
	_, err := convertSubmitRequest(mustMarshal(t, map[string]any{
		"content": []any{map[string]any{"type": "text", "text": "提示词"}},
	}))
	require.ErrorContains(t, err, "model is required")

	_, err = convertSubmitRequest(mustMarshal(t, map[string]any{
		"model":   "doubao-seedance-1-0-lite-t2v",
		"content": []any{map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://example.com/a.png"}}},
	}))
	require.ErrorContains(t, err, "prompt is required")
}

func TestConvertSeedanceOverseasSubmitRequest(t *testing.T) {
	body := mustMarshal(t, map[string]any{
		"model":          "dreamina-seedance-2-0-260128",
		"prompt":         "女孩睁开眼，温柔地看向镜头",
		"size":           "480p",
		"ratio":          "9:16",
		"duration":       5,
		"generate_audio": false,
		"content": []any{
			map[string]any{
				"type":      "image_url",
				"role":      "first_frame",
				"image_url": map[string]any{"url": "asset://asset-123"},
			},
		},
	})

	got, err := convertSeedanceOverseasSubmitRequest(body)
	require.NoError(t, err)
	assert.Equal(t, "doubao-seedance-2-0-filter-off", got.Model)
	assert.Equal(t, "女孩睁开眼，温柔地看向镜头", got.Prompt)
	assert.Equal(t, "480p", got.Resolution)
	assert.Equal(t, 5, got.Duration)
	assert.NotContains(t, got.Metadata, "model")
	assert.NotContains(t, got.Metadata, "prompt")
	assert.Equal(t, "9:16", got.Metadata["ratio"])
	assert.Equal(t, false, got.Metadata["generate_audio"])
	assert.Contains(t, got.Metadata, "content")
}

func TestSeedanceOverseasModelAliases(t *testing.T) {
	assert.Equal(t, "doubao-seedance-2-0-filter-off", normalizeSeedanceOverseasRequestModel("dreamina-seedance-2-0-ep"))
	assert.Equal(t, "doubao-seedance-2-0-fast-filter-off", normalizeSeedanceOverseasRequestModel("dreamina-seedance-2-0-fast-260128"))
	assert.Equal(t, "custom-model", normalizeSeedanceOverseasRequestModel("custom-model"))

	assert.Equal(t, "doubao-seedance-2-0-260128", toSeedanceOverseasResponseModel("doubao-seedance-2-0-filter-off"))
	assert.Equal(t, "doubao-seedance-2-0-fast-260128", toSeedanceOverseasResponseModel("dreamina-seedance-2-0-fast-ep"))
	assert.Equal(t, "custom-model", toSeedanceOverseasResponseModel("custom-model"))
}

func TestConvertSubmitResponse(t *testing.T) {
	got, err := convertSubmitResponse(mustMarshal(t, map[string]any{
		"id":      "task_123",
		"object":  "video",
		"task_id": "task_legacy",
	}))
	require.NoError(t, err)
	assert.Equal(t, "task_123", got.ID)

	got, err = convertSubmitResponse(mustMarshal(t, map[string]any{
		"code": "success",
		"data": map[string]any{"task_id": "task_data"},
	}))
	require.NoError(t, err)
	assert.Equal(t, "task_data", got.ID)
}

func TestConvertFetchResponseFromTaskData(t *testing.T) {
	body := mustMarshal(t, map[string]any{
		"code": "success",
		"data": map[string]any{
			"task_id":    "task_123",
			"status":     "SUCCESS",
			"result_url": "https://example.com/result.mp4",
			"created_at": 100,
			"updated_at": 200,
			"properties": map[string]any{
				"origin_model_name": "doubao-seedance-1-0-lite-t2v",
			},
			"data": map[string]any{
				"model":                   "upstream-model",
				"seed":                    7,
				"resolution":              "720p",
				"duration":                5,
				"ratio":                   "16:9",
				"execution_expires_after": 172800,
				"generate_audio":          false,
				"draft":                   false,
				"priority":                0,
				"usage": map[string]any{
					"completion_tokens": 11,
					"total_tokens":      13,
					"tool_usage": map[string]any{
						"web_search": 2,
					},
				},
			},
		},
	})

	got, err := convertFetchResponse(body, "task_123")
	require.NoError(t, err)
	assert.Equal(t, "task_123", got.ID)
	assert.Equal(t, "doubao-seedance-1-0-lite-t2v", got.Model)
	assert.Equal(t, "succeeded", got.Status)
	assert.Equal(t, "https://example.com/result.mp4", got.Content.VideoURL)
	assert.Equal(t, 7, got.Seed)
	assert.Equal(t, "720p", got.Resolution)
	assert.Equal(t, 5, got.Duration)
	assert.Equal(t, "16:9", got.Ratio)
	assert.Equal(t, 11, got.Usage.CompletionTokens)
	assert.Equal(t, 13, got.Usage.TotalTokens)
	assert.Equal(t, 2, got.Usage.ToolUsage.WebSearch)
	require.NotNil(t, got.ExecutionExpiresAfter)
	assert.Equal(t, 172800, *got.ExecutionExpiresAfter)
	require.NotNil(t, got.GenerateAudio)
	assert.False(t, *got.GenerateAudio)
	require.NotNil(t, got.Draft)
	assert.False(t, *got.Draft)
	require.NotNil(t, got.Priority)
	assert.Zero(t, *got.Priority)
	assert.EqualValues(t, 100, got.CreatedAt)
	assert.EqualValues(t, 200, got.UpdatedAt)
}

func TestConvertFetchResponseFromOpenAIVideo(t *testing.T) {
	body := mustMarshal(t, map[string]any{
		"id":         "task_openai",
		"object":     "video",
		"model":      "sora-2",
		"status":     "completed",
		"created_at": 300,
		"metadata": map[string]any{
			"url": "https://example.com/openai.mp4",
		},
	})

	got, err := convertFetchResponse(body, "")
	require.NoError(t, err)
	assert.Equal(t, "task_openai", got.ID)
	assert.Equal(t, "sora-2", got.Model)
	assert.Equal(t, "succeeded", got.Status)
	assert.Equal(t, "https://example.com/openai.mp4", got.Content.VideoURL)
}

func mustMarshal(t *testing.T, value any) []byte {
	t.Helper()
	body, err := common.Marshal(value)
	require.NoError(t, err)
	return body
}
