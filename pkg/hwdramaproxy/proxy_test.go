package hwdramaproxy

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeAPIKey(t *testing.T) {
	tests := []struct {
		name   string
		header string
		key    string
		ok     bool
	}{
		{name: "bearer sk key", header: "Bearer sk-abc123", key: "abc123", ok: true},
		{name: "lowercase bearer", header: "bearer sk-abc123", key: "abc123", ok: true},
		{name: "specific channel suffix", header: "sk-abc123-extra", key: "abc123", ok: true},
		{name: "bare key", header: "abc123", key: "abc123", ok: true},
		{name: "empty header", header: "", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, ok := NormalizeAPIKey(tt.header)
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.key, key)
		})
	}
}

func TestRouteDecisionFor(t *testing.T) {
	tests := []struct {
		name          string
		method        string
		path          string
		pathKnown     bool
		methodAllowed bool
	}{
		{name: "list assets", method: http.MethodGet, path: "/api/v3/ark/assets", pathKnown: true, methodAllowed: true},
		{name: "create assets", method: http.MethodPost, path: "/api/v3/ark/assets", pathKnown: true, methodAllowed: true},
		{name: "asset group wrong method", method: http.MethodGet, path: "/api/v3/ark/assets/groups", pathKnown: true, methodAllowed: false},
		{name: "get asset by id", method: http.MethodGet, path: "/api/v3/ark/assets/asset-123", pathKnown: true, methodAllowed: true},
		{name: "post asset by id", method: http.MethodPost, path: "/api/v3/ark/assets/asset-123", pathKnown: true, methodAllowed: false},
		{name: "nested asset path rejected", method: http.MethodGet, path: "/api/v3/ark/assets/asset-123/extra", pathKnown: false, methodAllowed: false},
		{name: "real person asset", method: http.MethodGet, path: "/api/v3/ark/real-person/assets/asset-123", pathKnown: true, methodAllowed: true},
		{name: "validate session", method: http.MethodGet, path: "/api/v3/ark/real-person/validate/sessions/session-123", pathKnown: true, methodAllowed: true},
		{name: "seedance create asset", method: http.MethodPost, path: "/api/v3/open/CreateAsset", pathKnown: true, methodAllowed: true},
		{name: "unknown path", method: http.MethodPost, path: "/api/v3/open/DeleteAsset", pathKnown: false, methodAllowed: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := RouteDecisionFor(tt.method, tt.path)
			assert.Equal(t, tt.pathKnown, decision.PathKnown)
			assert.Equal(t, tt.methodAllowed, decision.MethodAllowed)
		})
	}
}

func TestProxyPassesThroughAllowedRequest(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		assert.Equal(t, "/api/v3/open/CreateAsset", r.URL.Path)
		assert.Equal(t, "x=1", r.URL.RawQuery)
		assert.Equal(t, "Bearer upstream-key", r.Header.Get("Authorization"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "application/json", r.Header.Get("Accept"))
		assert.JSONEq(t, `{"hello":"world"}`, string(body))

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Upstream-Trace", "trace-1")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	proxy, err := New(Config{
		UpstreamBaseURL: upstream.URL,
		UpstreamAPIKey:  "upstream-key",
		TokenLookup: func(key string) (bool, error) {
			assert.Equal(t, "clientkey", key)
			return true, nil
		},
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v3/open/CreateAsset?x=1", stringsReader(`{"hello":"world"}`))
	req.Header.Set("Authorization", "Bearer sk-clientkey-extra")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp := httptest.NewRecorder()

	proxy.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusAccepted, resp.Code)
	assert.Equal(t, "application/json", resp.Header().Get("Content-Type"))
	assert.Equal(t, "trace-1", resp.Header().Get("X-Upstream-Trace"))
	assert.JSONEq(t, `{"ok":true}`, resp.Body.String())
}

func TestProxyUsesDynamicRouteConfig(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		assert.Equal(t, "/v1/assets/create", r.URL.Path)
		assert.Equal(t, "q=1", r.URL.RawQuery)
		assert.Equal(t, "Bearer dynamic-upstream-key", r.Header.Get("Authorization"))
		assert.JSONEq(t, `{"model":"doubao-seedance-2-0-fast-filter-off","url":"https://example.com/a.jpg"}`, string(body))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"asset-1"}`))
	}))
	defer upstream.Close()

	configPath := filepath.Join(t.TempDir(), "routes.yml")
	config := RoutesConfig{
		Version: 1,
		Actions: map[string]ActionConfig{
			"seedance_open_create_asset": {
				DownstreamMethod:      "POST",
				DownstreamPath:        "/api/v3/open/CreateAsset",
				DefaultUpstreamMethod: "POST",
				DefaultUpstreamPath:   "/api/v3/open/CreateAsset",
			},
		},
		Routes: []RouteConfig{
			{
				Name:              "wetoken",
				APIKeyIDs:         []int{7},
				ChannelID:         3,
				Models:            []string{"doubao-seedance-2-0-fast-filter-off"},
				UpstreamBaseURL:   upstream.URL,
				UpstreamAPIKeyEnv: "HWD_TEST_UPSTREAM_KEY",
				EnabledActions:    []string{"seedance_open_create_asset"},
				UpstreamActionOverrides: map[string]UpstreamActionConfig{
					"seedance_open_create_asset": {
						UpstreamMethod: "POST",
						UpstreamPath:   "/v1/assets/create",
					},
				},
			},
		},
	}
	require.NoError(t, SaveRoutesConfig(configPath, &config))
	t.Setenv("HWD_TEST_UPSTREAM_KEY", "dynamic-upstream-key")

	proxy, err := New(Config{
		RoutesConfigPath: configPath,
		UpstreamBaseURL:  "://legacy-value-is-ignored-in-dynamic-mode",
		TokenResolver: func(key string) (*TokenIdentity, error) {
			assert.Equal(t, "clientkey", key)
			return &TokenIdentity{ID: 7}, nil
		},
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v3/open/CreateAsset?q=1", stringsReader(`{"model":"doubao-seedance-2-0-fast-filter-off","url":"https://example.com/a.jpg"}`))
	req.Header.Set("Authorization", "Bearer sk-clientkey")
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	proxy.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusCreated, resp.Code)
	assert.JSONEq(t, `{"id":"asset-1"}`, resp.Body.String())
}

func TestProxyRejectsInvalidInputs(t *testing.T) {
	proxy, err := New(Config{
		UpstreamBaseURL: "http://example.com",
		UpstreamAPIKey:  "upstream-key",
		TokenLookup: func(key string) (bool, error) {
			return key == "valid", nil
		},
	})
	require.NoError(t, err)

	tests := []struct {
		name   string
		method string
		path   string
		auth   string
		status int
	}{
		{name: "missing key", method: http.MethodGet, path: "/api/v3/ark/assets", status: http.StatusUnauthorized},
		{name: "unknown key", method: http.MethodGet, path: "/api/v3/ark/assets", auth: "Bearer sk-missing", status: http.StatusUnauthorized},
		{name: "wrong method", method: http.MethodPatch, path: "/api/v3/ark/assets", auth: "Bearer sk-valid", status: http.StatusMethodNotAllowed},
		{name: "unknown path", method: http.MethodGet, path: "/api/v3/ark/not-assets", auth: "Bearer sk-valid", status: http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			if tt.auth != "" {
				req.Header.Set("Authorization", tt.auth)
			}
			resp := httptest.NewRecorder()

			proxy.ServeHTTP(resp, req)

			assert.Equal(t, tt.status, resp.Code)
			assert.Equal(t, "application/json", resp.Header().Get("Content-Type"))
		})
	}
}

func TestProxyReturnsDatabaseError(t *testing.T) {
	proxy, err := New(Config{
		UpstreamBaseURL: "http://example.com",
		UpstreamAPIKey:  "upstream-key",
		TokenLookup: func(_ string) (bool, error) {
			return false, errors.New("database unavailable")
		},
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v3/ark/assets", nil)
	req.Header.Set("Authorization", "Bearer sk-valid")
	resp := httptest.NewRecorder()

	proxy.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusInternalServerError, resp.Code)
}

func TestTokenExistsInDatabaseOnlyRequiresRecordPresence(t *testing.T) {
	tempDir := t.TempDir()
	originalSQLDSN, hadSQLDSN := os.LookupEnv("SQL_DSN")
	originalSQLitePath := common.SQLitePath
	originalIsMasterNode := common.IsMasterNode
	originalRedisEnabled := common.RedisEnabled
	t.Cleanup(func() {
		if hadSQLDSN {
			require.NoError(t, os.Setenv("SQL_DSN", originalSQLDSN))
		} else {
			require.NoError(t, os.Unsetenv("SQL_DSN"))
		}
		common.SQLitePath = originalSQLitePath
		common.IsMasterNode = originalIsMasterNode
		common.RedisEnabled = originalRedisEnabled
		if model.DB != nil {
			_ = model.CloseDB()
		}
	})

	require.NoError(t, os.Setenv("SQL_DSN", "local"))
	common.SQLitePath = filepath.Join(tempDir, "new-api.db")
	common.IsMasterNode = true
	common.RedisEnabled = false
	require.NoError(t, model.InitDB())
	model.LOG_DB = model.DB

	expiredAt := common.GetTimestamp() - 100
	tokens := []model.Token{
		{UserId: 1, Key: "disabled", Status: common.TokenStatusDisabled, ExpiredTime: -1, RemainQuota: 100},
		{UserId: 1, Key: "expired", Status: common.TokenStatusEnabled, ExpiredTime: expiredAt, RemainQuota: 100},
		{UserId: 1, Key: "empty-quota", Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 0},
		{UserId: 1, Key: "deleted", Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 100},
	}
	for i := range tokens {
		require.NoError(t, model.DB.Create(&tokens[i]).Error)
	}
	require.NoError(t, model.DB.Delete(&tokens[3]).Error)

	for _, key := range []string{"disabled", "expired", "empty-quota"} {
		exists, err := TokenExistsInDatabase(key)
		require.NoError(t, err)
		assert.True(t, exists)
	}

	exists, err := TokenExistsInDatabase("missing")
	require.NoError(t, err)
	assert.False(t, exists)

	exists, err = TokenExistsInDatabase("deleted")
	require.NoError(t, err)
	assert.False(t, exists)
}

func stringsReader(s string) io.Reader {
	return strings.NewReader(s)
}
