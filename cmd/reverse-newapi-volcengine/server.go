package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const (
	submitEndpoint     = "/v1/video/generations"
	fetchEndpointBase  = "/v1/video/generations/"
	defaultMaxBodySize = 32 << 20
)

type reverseServer struct {
	upstreamBaseURL string
	client          *http.Client
}

func newServer(cfg config) http.Handler {
	s := &reverseServer{
		upstreamBaseURL: cfg.upstreamBaseURL,
		client: &http.Client{
			Timeout: cfg.timeout,
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc(submitEndpoint, s.handleSubmit)
	mux.HandleFunc(fetchEndpointBase, s.handleFetch)
	return mux
}

func (s *reverseServer) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *reverseServer) handleSubmit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only POST is allowed")
		return
	}

	body, err := readRequestBody(w, r)
	if err != nil {
		status := http.StatusBadRequest
		code := "invalid_request"
		if errors.Is(err, errRequestBodyTooLarge) {
			status = http.StatusRequestEntityTooLarge
			code = "request_body_too_large"
		}
		writeError(w, status, code, err.Error())
		return
	}

	newAPIReq, err := convertSubmitRequest(body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	upstreamBody, err := common.Marshal(newAPIReq)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "marshal_request_failed", err.Error())
		return
	}

	resp, err := s.doUpstream(r, http.MethodPost, submitEndpoint, bytes.NewReader(upstreamBody))
	if err != nil {
		writeError(w, http.StatusBadGateway, "upstream_request_failed", err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		copyUpstreamResponse(w, resp)
		return
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		writeError(w, http.StatusBadGateway, "read_upstream_response_failed", err.Error())
		return
	}
	volcResp, err := convertSubmitResponse(respBody)
	if err != nil {
		writeError(w, http.StatusBadGateway, "invalid_response", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, volcResp)
}

func (s *reverseServer) handleFetch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET is allowed")
		return
	}

	taskID := strings.TrimPrefix(r.URL.Path, fetchEndpointBase)
	taskID = strings.Trim(taskID, "/")
	if taskID == "" || strings.Contains(taskID, "/") {
		writeError(w, http.StatusBadRequest, "invalid_request", "task_id is required")
		return
	}

	endpoint := fetchEndpointBase + url.PathEscape(taskID)
	resp, err := s.doUpstream(r, http.MethodGet, endpoint, nil)
	if err != nil {
		writeError(w, http.StatusBadGateway, "upstream_request_failed", err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		copyUpstreamResponse(w, resp)
		return
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		writeError(w, http.StatusBadGateway, "read_upstream_response_failed", err.Error())
		return
	}
	volcResp, err := convertFetchResponse(respBody, taskID)
	if err != nil {
		writeError(w, http.StatusBadGateway, "invalid_response", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, volcResp)
}

func (s *reverseServer) doUpstream(src *http.Request, method string, endpoint string, body io.Reader) (*http.Response, error) {
	upstreamURL, err := joinUpstreamURL(s.upstreamBaseURL, endpoint)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(src.Context(), method, upstreamURL, body)
	if err != nil {
		return nil, err
	}
	copyProxyHeaders(req.Header, src.Header)
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
	}
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/json")
	}
	return s.client.Do(req)
}

func joinUpstreamURL(baseURL string, endpoint string) (string, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	basePath := strings.TrimRight(parsed.Path, "/")
	parsed.Path = basePath + endpoint
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func copyProxyHeaders(dst http.Header, src http.Header) {
	for name, values := range src {
		canonical := http.CanonicalHeaderKey(name)
		if isHopByHopHeader(canonical) {
			continue
		}
		for _, value := range values {
			dst.Add(canonical, value)
		}
	}
}

func isHopByHopHeader(name string) bool {
	switch strings.ToLower(name) {
	case "connection", "keep-alive", "proxy-authenticate", "proxy-authorization",
		"te", "trailer", "transfer-encoding", "upgrade", "host", "content-length",
		"accept-encoding":
		return true
	default:
		return false
	}
}

var errRequestBodyTooLarge = errors.New("request body too large")

func readRequestBody(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	reader := http.MaxBytesReader(w, r.Body, defaultMaxBodySize)
	defer reader.Close()
	body, err := io.ReadAll(reader)
	if err != nil {
		if strings.Contains(err.Error(), "http: request body too large") {
			return nil, errRequestBodyTooLarge
		}
		return nil, err
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return nil, fmt.Errorf("request body is empty")
	}
	return body, nil
}

func copyUpstreamResponse(w http.ResponseWriter, resp *http.Response) {
	for name, values := range resp.Header {
		if isHopByHopHeader(name) {
			continue
		}
		for _, value := range values {
			w.Header().Add(name, value)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

type apiErrorBody struct {
	Error apiError `json:"error"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeError(w http.ResponseWriter, status int, code string, message string) {
	writeJSON(w, status, apiErrorBody{
		Error: apiError{
			Code:    code,
			Message: message,
		},
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	body, err := common.Marshal(payload)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Date", time.Now().UTC().Format(http.TimeFormat))
	w.WriteHeader(status)
	_, _ = w.Write(body)
}
