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
