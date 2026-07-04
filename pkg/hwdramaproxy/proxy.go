package hwdramaproxy

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

type TokenLookup func(key string) (bool, error)

type Config struct {
	UpstreamBaseURL string
	UpstreamAPIKey  string
	Timeout         time.Duration
	TokenLookup     TokenLookup
	Client          *http.Client
}

type Proxy struct {
	upstream       *url.URL
	upstreamAPIKey string
	client         *http.Client
	tokenLookup    TokenLookup
}

func New(config Config) (*Proxy, error) {
	upstreamBaseURL := strings.TrimSpace(config.UpstreamBaseURL)
	if upstreamBaseURL == "" {
		upstreamBaseURL = "http://ai.hwdrama.com"
	}
	upstream, err := url.Parse(upstreamBaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse upstream base url: %w", err)
	}
	if upstream.Scheme == "" || upstream.Host == "" {
		return nil, errors.New("upstream base url must include scheme and host")
	}
	upstreamAPIKey := strings.TrimSpace(config.UpstreamAPIKey)
	if upstreamAPIKey == "" {
		return nil, errors.New("upstream api key is required")
	}
	tokenLookup := config.TokenLookup
	if tokenLookup == nil {
		tokenLookup = TokenExistsInDatabase
	}
	client := config.Client
	if client == nil {
		timeout := config.Timeout
		if timeout <= 0 {
			timeout = 600 * time.Second
		}
		client = &http.Client{Timeout: timeout}
	}

	return &Proxy{
		upstream:       upstream,
		upstreamAPIKey: upstreamAPIKey,
		client:         client,
		tokenLookup:    tokenLookup,
	}, nil
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	decision := RouteDecisionFor(r.Method, r.URL.Path)
	if !decision.PathKnown {
		writeJSONError(w, http.StatusNotFound, "not_found", "route not found")
		return
	}
	if !decision.MethodAllowed {
		writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}

	key, ok := NormalizeAPIKey(r.Header.Get("Authorization"))
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "token_not_provided", "missing api key")
		return
	}
	exists, err := p.tokenLookup(key)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "database_error", "database error")
		return
	}
	if !exists {
		writeJSONError(w, http.StatusUnauthorized, "token_invalid", "invalid api key")
		return
	}

	p.proxyRequest(w, r)
}

func (p *Proxy) proxyRequest(w http.ResponseWriter, r *http.Request) {
	upstreamURL := *r.URL
	rewriteRequestURL(&upstreamURL, p.upstream)
	req, err := http.NewRequestWithContext(r.Context(), r.Method, upstreamURL.String(), r.Body)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "upstream_error", "failed to build upstream request")
		return
	}
	req.Header = r.Header.Clone()
	req.ContentLength = r.ContentLength
	removeHopByHopHeaders(req.Header)
	req.Host = p.upstream.Host
	req.Header.Set("Authorization", "Bearer "+p.upstreamAPIKey)

	resp, err := p.client.Do(req)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "upstream_error", "failed to reach upstream server")
		return
	}
	defer resp.Body.Close()

	copyHeader(w.Header(), resp.Header)
	removeHopByHopHeaders(w.Header())
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

type RouteDecision struct {
	PathKnown     bool
	MethodAllowed bool
}

func RouteDecisionFor(method string, path string) RouteDecision {
	method = strings.ToUpper(method)
	exactRoutes := map[string]map[string]bool{
		"/api/v3/ark/assets":                         {"GET": true, "POST": true},
		"/api/v3/ark/assets/groups":                  {"POST": true},
		"/api/v3/ark/real-person/assets":             {"POST": true},
		"/api/v3/ark/real-person/validate/sessions":  {"POST": true},
		"/api/v3/open/CreateAsset":                   {"POST": true},
		"/api/v3/open/GetAsset":                      {"POST": true},
	}
	if methods, ok := exactRoutes[path]; ok {
		return RouteDecision{
			PathKnown:     true,
			MethodAllowed: methods[method],
		}
	}

	getOnlyPrefixes := []string{
		"/api/v3/ark/assets/",
		"/api/v3/ark/real-person/assets/",
		"/api/v3/ark/real-person/validate/sessions/",
	}
	for _, prefix := range getOnlyPrefixes {
		if hasSinglePathSegmentAfterPrefix(path, prefix) {
			return RouteDecision{
				PathKnown:     true,
				MethodAllowed: method == http.MethodGet,
			}
		}
	}

	return RouteDecision{}
}

func NormalizeAPIKey(authHeader string) (string, bool) {
	key := strings.TrimSpace(authHeader)
	if strings.HasPrefix(strings.ToLower(key), "bearer ") {
		key = strings.TrimSpace(key[7:])
	}
	key = strings.TrimPrefix(key, "sk-")
	parts := strings.Split(key, "-")
	if len(parts) > 0 {
		key = parts[0]
	}
	if key == "" {
		return "", false
	}
	return key, true
}

func TokenExistsInDatabase(key string) (bool, error) {
	_, err := model.GetTokenByKey(key, true)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	return false, err
}

func hasSinglePathSegmentAfterPrefix(path string, prefix string) bool {
	if !strings.HasPrefix(path, prefix) {
		return false
	}
	rest := strings.TrimPrefix(path, prefix)
	return rest != "" && !strings.Contains(rest, "/")
}

func rewriteRequestURL(reqURL *url.URL, target *url.URL) {
	targetQuery := target.RawQuery
	reqURL.Scheme = target.Scheme
	reqURL.Host = target.Host
	reqURL.Path = singleJoiningSlash(target.Path, reqURL.Path)
	if targetQuery == "" || reqURL.RawQuery == "" {
		reqURL.RawQuery = targetQuery + reqURL.RawQuery
	} else {
		reqURL.RawQuery = targetQuery + "&" + reqURL.RawQuery
	}
}

func singleJoiningSlash(a string, b string) string {
	aslash := strings.HasSuffix(a, "/")
	bslash := strings.HasPrefix(b, "/")
	switch {
	case aslash && bslash:
		return a + b[1:]
	case !aslash && !bslash:
		return a + "/" + b
	default:
		return a + b
	}
}

type proxyErrorResponse struct {
	Error proxyError `json:"error"`
}

type proxyError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeJSONError(w http.ResponseWriter, status int, code string, message string) {
	body, err := common.Marshal(proxyErrorResponse{
		Error: proxyError{
			Code:    code,
			Message: message,
		},
	})
	if err != nil {
		http.Error(w, http.StatusText(status), status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func copyHeader(dst http.Header, src http.Header) {
	for key, values := range src {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func removeHopByHopHeaders(header http.Header) {
	for _, key := range []string{
		"Connection",
		"Proxy-Connection",
		"Keep-Alive",
		"Proxy-Authenticate",
		"Proxy-Authorization",
		"Te",
		"Trailer",
		"Transfer-Encoding",
		"Upgrade",
	} {
		header.Del(key)
	}
}
