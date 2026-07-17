package hwdramaproxy

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const WildcardModel = "*"

type RoutesConfig struct {
	Version int                     `yaml:"version"`
	Actions map[string]ActionConfig `yaml:"actions"`
	Routes  []RouteConfig           `yaml:"routes"`
}

type ActionConfig struct {
	DownstreamMethod      string `yaml:"downstream_method"`
	DownstreamPath        string `yaml:"downstream_path"`
	DefaultUpstreamMethod string `yaml:"default_upstream_method"`
	DefaultUpstreamPath   string `yaml:"default_upstream_path"`
	AffinityResponseField string `yaml:"affinity_response_field,omitempty"`
}

type RouteConfig struct {
	Name                    string                          `yaml:"name"`
	APIKeyIDs               []int                           `yaml:"api_key_ids"`
	AllAPIKeys              bool                            `yaml:"all_api_keys,omitempty"`
	ChannelID               int                             `yaml:"channel_id"`
	Models                  []string                        `yaml:"models"`
	UpstreamBaseURL         string                          `yaml:"upstream_base_url"`
	UpstreamAPIKeyEnv       string                          `yaml:"upstream_api_key_env"`
	UpstreamAuthHeader      string                          `yaml:"upstream_auth_header,omitempty"`
	UpstreamAuthPrefix      string                          `yaml:"upstream_auth_prefix,omitempty"`
	AssetNamespaceID        string                          `yaml:"asset_namespace_id"`
	EnabledActions          []string                        `yaml:"enabled_actions"`
	UpstreamActionOverrides map[string]UpstreamActionConfig `yaml:"upstream_action_overrides,omitempty"`
}

type UpstreamActionConfig struct {
	UpstreamMethod string `yaml:"upstream_method"`
	UpstreamPath   string `yaml:"upstream_path"`
}

func LoadRoutesConfig(path string) (*RoutesConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var config RoutesConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parse routes config: %w", err)
	}
	return &config, nil
}

func SaveRoutesConfig(path string, config *RoutesConfig) error {
	if config == nil {
		return errors.New("routes config is nil")
	}
	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("marshal routes config: %w", err)
	}
	return os.WriteFile(path, data, 0o600)
}

func (config *RoutesConfig) Normalize() {
	if config.Version == 0 {
		config.Version = 1
	}
	for key, action := range config.Actions {
		action.DownstreamMethod = normalizeMethod(action.DownstreamMethod)
		action.DefaultUpstreamMethod = normalizeMethod(action.DefaultUpstreamMethod)
		action.DownstreamPath = strings.TrimSpace(action.DownstreamPath)
		action.DefaultUpstreamPath = strings.TrimSpace(action.DefaultUpstreamPath)
		action.AffinityResponseField = strings.TrimSpace(action.AffinityResponseField)
		config.Actions[key] = action
	}
	for i := range config.Routes {
		route := &config.Routes[i]
		route.Name = strings.TrimSpace(route.Name)
		route.UpstreamBaseURL = strings.TrimSpace(route.UpstreamBaseURL)
		route.UpstreamAPIKeyEnv = strings.TrimSpace(route.UpstreamAPIKeyEnv)
		route.UpstreamAuthHeader = strings.TrimSpace(route.UpstreamAuthHeader)
		route.UpstreamAuthPrefix = strings.TrimSpace(route.UpstreamAuthPrefix)
		if route.UpstreamAuthHeader == "" {
			route.UpstreamAuthHeader = "Authorization"
		}
		if strings.EqualFold(route.UpstreamAuthHeader, "Authorization") && route.UpstreamAuthPrefix == "" {
			route.UpstreamAuthPrefix = "Bearer"
		}
		route.AssetNamespaceID = strings.TrimSpace(route.AssetNamespaceID)
		route.Models = normalizeList(route.Models)
		route.EnabledActions = normalizeList(route.EnabledActions)
		if route.UpstreamActionOverrides != nil {
			for key, override := range route.UpstreamActionOverrides {
				override.UpstreamMethod = normalizeMethod(override.UpstreamMethod)
				override.UpstreamPath = strings.TrimSpace(override.UpstreamPath)
				route.UpstreamActionOverrides[key] = override
			}
		}
	}
}

func (config *RoutesConfig) Validate(secretLookup func(string) string) error {
	if config == nil {
		return errors.New("routes config is nil")
	}
	config.Normalize()
	if config.Version != 1 {
		return fmt.Errorf("unsupported routes config version %d", config.Version)
	}
	if len(config.Actions) == 0 {
		return errors.New("actions cannot be empty")
	}

	actionByDownstream := make(map[string]string, len(config.Actions))
	for actionKey, action := range config.Actions {
		if strings.TrimSpace(actionKey) == "" {
			return errors.New("action key cannot be empty")
		}
		if err := validateMethod(action.DownstreamMethod, "action "+actionKey+" downstream_method"); err != nil {
			return err
		}
		if err := validateMethod(action.DefaultUpstreamMethod, "action "+actionKey+" default_upstream_method"); err != nil {
			return err
		}
		if err := validatePathTemplate(action.DownstreamPath, "action "+actionKey+" downstream_path"); err != nil {
			return err
		}
		if err := validatePathTemplate(action.DefaultUpstreamPath, "action "+actionKey+" default_upstream_path"); err != nil {
			return err
		}
		if isPrivateBillingPath(action.DefaultUpstreamPath) {
			return fmt.Errorf("action %s cannot expose the private Seedance billing endpoint", actionKey)
		}
		downstreamKey := action.DownstreamMethod + " " + normalizePathTemplate(action.DownstreamPath)
		if existing, ok := actionByDownstream[downstreamKey]; ok {
			return fmt.Errorf("actions %s and %s share downstream endpoint %s", existing, actionKey, downstreamKey)
		}
		actionByDownstream[downstreamKey] = actionKey
	}

	if len(config.Routes) == 0 {
		return errors.New("routes cannot be empty")
	}
	routeNames := make(map[string]bool, len(config.Routes))
	routeKeys := make(map[string]string)
	for i, route := range config.Routes {
		prefix := fmt.Sprintf("route[%d]", i)
		if route.Name == "" {
			return fmt.Errorf("%s name cannot be empty", prefix)
		}
		if routeNames[route.Name] {
			return fmt.Errorf("route name %q is duplicated", route.Name)
		}
		routeNames[route.Name] = true
		if len(route.APIKeyIDs) == 0 && !route.AllAPIKeys {
			return fmt.Errorf("route %s api_key_ids cannot be empty unless all_api_keys is true", route.Name)
		}
		for _, id := range route.APIKeyIDs {
			if id <= 0 {
				return fmt.Errorf("route %s api_key_ids must be positive integers", route.Name)
			}
		}
		if route.ChannelID <= 0 {
			return fmt.Errorf("route %s channel_id must be positive", route.Name)
		}
		if len(route.Models) == 0 {
			return fmt.Errorf("route %s models cannot be empty", route.Name)
		}
		if len(route.Models) > 1 && hasString(route.Models, WildcardModel) {
			return fmt.Errorf("route %s cannot combine %q with specific models", route.Name, WildcardModel)
		}
		if route.UpstreamBaseURL == "" {
			return fmt.Errorf("route %s upstream_base_url cannot be empty", route.Name)
		}
		baseURL, err := parseBaseURL(route.UpstreamBaseURL)
		if err != nil {
			return fmt.Errorf("route %s upstream_base_url: %w", route.Name, err)
		}
		if route.UpstreamAPIKeyEnv == "" {
			return fmt.Errorf("route %s upstream_api_key_env cannot be empty", route.Name)
		}
		if !validHeaderName(route.UpstreamAuthHeader) {
			return fmt.Errorf("route %s upstream_auth_header is invalid", route.Name)
		}
		if strings.ContainsAny(route.UpstreamAuthPrefix, "\r\n") {
			return fmt.Errorf("route %s upstream_auth_prefix is invalid", route.Name)
		}
		if secretLookup != nil && strings.TrimSpace(secretLookup(route.UpstreamAPIKeyEnv)) == "" {
			return fmt.Errorf("route %s upstream api key env %s is empty", route.Name, route.UpstreamAPIKeyEnv)
		}
		if len(route.EnabledActions) == 0 {
			return fmt.Errorf("route %s enabled_actions cannot be empty", route.Name)
		}
		for _, actionKey := range route.EnabledActions {
			action, ok := config.Actions[actionKey]
			if !ok {
				return fmt.Errorf("route %s references unknown action %s", route.Name, actionKey)
			}
			if action.AffinityResponseField != "" && route.AssetNamespaceID == "" {
				return fmt.Errorf("route %s asset_namespace_id is required for affinity action %s", route.Name, actionKey)
			}
			upstreamPath := action.DefaultUpstreamPath
			if override, hasOverride := route.UpstreamActionOverrides[actionKey]; hasOverride && override.UpstreamPath != "" {
				upstreamPath = override.UpstreamPath
			}
			if isPrivateBillingPath(singleJoiningSlash(baseURL.Path, upstreamPath)) {
				return fmt.Errorf("route %s cannot expose the private Seedance billing endpoint", route.Name)
			}
		}
		for actionKey, override := range route.UpstreamActionOverrides {
			if _, ok := config.Actions[actionKey]; !ok {
				return fmt.Errorf("route %s override references unknown action %s", route.Name, actionKey)
			}
			if !hasString(route.EnabledActions, actionKey) {
				return fmt.Errorf("route %s override references disabled action %s", route.Name, actionKey)
			}
			if override.UpstreamMethod != "" {
				if err := validateMethod(override.UpstreamMethod, "route "+route.Name+" override "+actionKey+" upstream_method"); err != nil {
					return err
				}
			}
			if override.UpstreamPath != "" {
				if err := validatePathTemplate(override.UpstreamPath, "route "+route.Name+" override "+actionKey+" upstream_path"); err != nil {
					return err
				}
				if isPrivateBillingPath(override.UpstreamPath) {
					return fmt.Errorf("route %s cannot expose the private Seedance billing endpoint", route.Name)
				}
			}
		}
		for _, apiKeyID := range routeAPIKeyIDs(route) {
			for _, model := range route.Models {
				for _, actionKey := range route.EnabledActions {
					key := fmt.Sprintf("%d\x00%s\x00%s", apiKeyID, model, actionKey)
					if existing, ok := routeKeys[key]; ok {
						return fmt.Errorf("routes %s and %s both match api_key_id=%d model=%s action=%s", existing, route.Name, apiKeyID, model, actionKey)
					}
					routeKeys[key] = route.Name
				}
			}
		}
	}
	return nil
}

func isPrivateBillingPath(path string) bool {
	path = strings.ToLower(strings.TrimRight(strings.TrimSpace(path), "/"))
	return strings.HasSuffix(path, "/listsplitbilldetail")
}

func validHeaderName(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') {
			continue
		}
		switch char {
		case '!', '#', '$', '%', '&', '\'', '*', '+', '-', '.', '^', '_', '`', '|', '~':
			continue
		default:
			return false
		}
	}
	return true
}

func routeAPIKeyIDs(route RouteConfig) []int {
	if route.AllAPIKeys {
		return []int{0}
	}
	return route.APIKeyIDs
}

func parseBaseURL(rawURL string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil, err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, errors.New("must include scheme and host")
	}
	return parsed, nil
}

func normalizeMethod(method string) string {
	return strings.ToUpper(strings.TrimSpace(method))
}

func validateMethod(method string, field string) error {
	switch normalizeMethod(method) {
	case "GET", "POST", "PUT", "PATCH", "DELETE":
		return nil
	default:
		return fmt.Errorf("%s must be one of GET, POST, PUT, PATCH, DELETE", field)
	}
}

func validatePathTemplate(path string, field string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("%s cannot be empty", field)
	}
	if strings.Contains(path, "://") {
		return fmt.Errorf("%s must be a path, not a full URL", field)
	}
	if !strings.HasPrefix(path, "/") {
		return fmt.Errorf("%s must start with /", field)
	}
	if strings.Contains(path, "?") || strings.Contains(path, "#") {
		return fmt.Errorf("%s must not include query or fragment", field)
	}
	if _, err := parsePathTemplate(path); err != nil {
		return fmt.Errorf("%s: %w", field, err)
	}
	return nil
}

func normalizePathTemplate(path string) string {
	segments := splitPath(strings.TrimSpace(path))
	for i, segment := range segments {
		if isTemplateVar(segment) {
			segments[i] = "{}"
		}
	}
	return "/" + strings.Join(segments, "/")
}

func normalizeList(values []string) []string {
	seen := make(map[string]bool, len(values))
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		normalized = append(normalized, value)
	}
	sort.Strings(normalized)
	return normalized
}

func hasString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
