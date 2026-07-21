package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleSubmitForwardsAuthorizationAndConvertsResponse(t *testing.T) {
	var upstreamAuthorization string
	var upstreamBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamAuthorization = r.Header.Get("Authorization")
		require.Equal(t, "/v1/video/generations", r.URL.Path)
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, common.Unmarshal(body, &upstreamBody))
		writeJSON(w, http.StatusOK, map[string]any{"id": "task_upstream"})
	}))
	defer upstream.Close()

	handler := newServer(config{
		upstreamBaseURL: upstream.URL,
		timeout:         10 * time.Second,
	})

	reqBody := mustMarshal(t, map[string]any{
		"model": "doubao-seedance-1-0-lite-t2v",
		"content": []any{
			map[string]any{"type": "text", "text": "生成视频"},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(string(reqBody)))
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "Bearer test-token", upstreamAuthorization)
	assert.Equal(t, "doubao-seedance-1-0-lite-t2v", upstreamBody["model"])
	assert.Equal(t, "生成视频", upstreamBody["prompt"])

	var response map[string]any
	require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &response))
	assert.Equal(t, "task_upstream", response["id"])
}

func TestHandleOfficialSubmitPathForwardsToNewAPIVideoEndpoint(t *testing.T) {
	var upstreamPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamPath = r.URL.Path
		writeJSON(w, http.StatusOK, map[string]any{"id": "task_official"})
	}))
	defer upstream.Close()

	handler := newServer(config{
		upstreamBaseURL: upstream.URL,
		timeout:         10 * time.Second,
	})

	reqBody := mustMarshal(t, map[string]any{
		"model": "doubao-seedance-2-0-fast-filter-off",
		"content": []any{
			map[string]any{"type": "text", "text": "generate a video"},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", strings.NewReader(string(reqBody)))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "/v1/video/generations", upstreamPath)
	var response map[string]any
	require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &response))
	assert.Equal(t, "task_official", response["id"])
}

func TestHandleSeedanceOverseasSubmitPathConvertsDocumentedRequest(t *testing.T) {
	var upstreamBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/video/generations", r.URL.Path)
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, common.Unmarshal(body, &upstreamBody))
		writeJSON(w, http.StatusOK, map[string]any{"id": "task_overseas"})
	}))
	defer upstream.Close()

	handler := newServer(config{
		upstreamBaseURL: upstream.URL,
		timeout:         10 * time.Second,
	})

	reqBody := mustMarshal(t, map[string]any{
		"model":          "dreamina-seedance-2-0-fast-260128",
		"prompt":         "镜头缓慢推进",
		"ratio":          "16:9",
		"size":           "480p",
		"duration":       5,
		"generate_audio": false,
	})
	req := httptest.NewRequest(http.MethodPost, seedanceOverseasSubmitEndpoint, strings.NewReader(string(reqBody)))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "doubao-seedance-2-0-fast-filter-off", upstreamBody["model"])
	assert.Equal(t, "镜头缓慢推进", upstreamBody["prompt"])
	assert.Equal(t, "480p", upstreamBody["resolution"])
	metadata := upstreamBody["metadata"].(map[string]any)
	assert.Equal(t, "16:9", metadata["ratio"])
	assert.Equal(t, false, metadata["generate_audio"])

	var response map[string]any
	require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &response))
	assert.Equal(t, "task_overseas", response["id"])
}

func TestHandleFetchConvertsTaskResponse(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/video/generations/task_123", r.URL.Path)
		writeJSON(w, http.StatusOK, map[string]any{
			"code": "success",
			"data": map[string]any{
				"task_id":    "task_123",
				"status":     "IN_PROGRESS",
				"created_at": 100,
			},
		})
	}))
	defer upstream.Close()

	handler := newServer(config{
		upstreamBaseURL: upstream.URL,
		timeout:         10 * time.Second,
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/video/generations/task_123", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var response map[string]any
	require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &response))
	assert.Equal(t, "task_123", response["id"])
	assert.Equal(t, "processing", response["status"])
}

func TestHandleOfficialFetchPathForwardsToNewAPIVideoEndpoint(t *testing.T) {
	var upstreamPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamPath = r.URL.Path
		writeJSON(w, http.StatusOK, map[string]any{
			"code": "success",
			"data": map[string]any{
				"task_id":    "task_123",
				"status":     "SUCCESS",
				"result_url": "https://example.com/video.mp4",
			},
		})
	}))
	defer upstream.Close()

	handler := newServer(config{
		upstreamBaseURL: upstream.URL,
		timeout:         10 * time.Second,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v3/contents/generations/tasks/task_123", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "/v1/video/generations/task_123", upstreamPath)
	var response map[string]any
	require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &response))
	assert.Equal(t, "task_123", response["id"])
	assert.Equal(t, "succeeded", response["status"])
	assert.Equal(t, "https://example.com/video.mp4", response["content"].(map[string]any)["video_url"])
}

func TestHandleSeedanceOverseasFetchPathReturnsDocumentedModel(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/video/generations/task_123", r.URL.Path)
		writeJSON(w, http.StatusOK, map[string]any{
			"code": "success",
			"data": map[string]any{
				"task_id": "task_123",
				"status":  "SUCCESS",
				"properties": map[string]any{
					"origin_model_name": "doubao-seedance-2-0-filter-off",
				},
				"data": map[string]any{
					"content": map[string]any{"video_url": "https://example.com/video.mp4"},
					"usage":   map[string]any{"total_tokens": 108900, "completion_tokens": 108900},
				},
			},
		})
	}))
	defer upstream.Close()

	handler := newServer(config{
		upstreamBaseURL: upstream.URL,
		timeout:         10 * time.Second,
	})

	req := httptest.NewRequest(http.MethodGet, seedanceOverseasFetchEndpointBase+"task_123", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var response map[string]any
	require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &response))
	assert.Equal(t, "doubao-seedance-2-0-260128", response["model"])
	assert.Equal(t, "succeeded", response["status"])
	assert.EqualValues(t, 108900, response["usage"].(map[string]any)["total_tokens"])
}
