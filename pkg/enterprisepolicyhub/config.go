package enterprisepolicyhub

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

type Config struct {
	Port                 string
	BasePath             string
	NewAPIBaseURL        string
	AuthTimeout          time.Duration
	LogSyncInterval      time.Duration
	BootstrapAdminIDs    map[int]bool
	AllowAnyNewAPIAdmin  bool
	EnableBackgroundSync bool
	BudgetTimezone       string
	TokenOperation       TokenOperationConfig
}

type TokenOperationConfig struct {
	Enabled            bool
	BaseURL            string
	GatewayKey         string
	ObjectSyncEnabled  bool
	UsageEventsEnabled bool
	Timeout            time.Duration
}

func LoadConfig() Config {
	port := strings.TrimSpace(os.Getenv("EPH_PORT"))
	if port == "" {
		port = "3100"
	}
	basePath := strings.TrimSpace(os.Getenv("EPH_BASE_PATH"))
	if basePath == "" {
		basePath = "/enterprise"
	}
	if !strings.HasPrefix(basePath, "/") {
		basePath = "/" + basePath
	}
	basePath = strings.TrimRight(basePath, "/")
	if basePath == "" {
		basePath = "/enterprise"
	}

	authTimeoutSeconds := common.GetEnvOrDefault("EPH_AUTH_TIMEOUT_SECONDS", 10)
	syncIntervalSeconds := common.GetEnvOrDefault("EPH_LOG_SYNC_INTERVAL_SECONDS", 10)
	tokenOperationTimeoutSeconds := common.GetEnvOrDefault("EPH_TOKENOP_TIMEOUT_SECONDS", 10)
	budgetTimezone := strings.TrimSpace(os.Getenv("EPH_BUDGET_TIMEZONE"))
	if budgetTimezone == "" {
		budgetTimezone = "Asia/Shanghai"
	}

	bootstrap := make(map[int]bool)
	for _, part := range strings.Split(os.Getenv("EPH_BOOTSTRAP_ADMIN_IDS"), ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := strconv.Atoi(part)
		if err == nil && id > 0 {
			bootstrap[id] = true
		}
	}

	return Config{
		Port:                 port,
		BasePath:             basePath,
		NewAPIBaseURL:        strings.TrimRight(strings.TrimSpace(os.Getenv("EPH_NEWAPI_BASE_URL")), "/"),
		AuthTimeout:          time.Duration(authTimeoutSeconds) * time.Second,
		LogSyncInterval:      time.Duration(syncIntervalSeconds) * time.Second,
		BootstrapAdminIDs:    bootstrap,
		AllowAnyNewAPIAdmin:  strings.EqualFold(os.Getenv("EPH_ALLOW_ANY_NEWAPI_ADMIN"), "true"),
		EnableBackgroundSync: !strings.EqualFold(os.Getenv("EPH_DISABLE_BACKGROUND_SYNC"), "true"),
		BudgetTimezone:       budgetTimezone,
		TokenOperation: TokenOperationConfig{
			Enabled:            strings.EqualFold(os.Getenv("EPH_TOKENOP_ENABLED"), "true"),
			BaseURL:            strings.TrimRight(strings.TrimSpace(os.Getenv("EPH_TOKENOP_BASE_URL")), "/"),
			GatewayKey:         strings.TrimSpace(os.Getenv("EPH_TOKENOP_GATEWAY_KEY")),
			ObjectSyncEnabled:  !strings.EqualFold(os.Getenv("EPH_TOKENOP_OBJECT_SYNC_ENABLED"), "false"),
			UsageEventsEnabled: strings.EqualFold(os.Getenv("EPH_TOKENOP_USAGE_EVENTS_ENABLED"), "true"),
			Timeout:            time.Duration(tokenOperationTimeoutSeconds) * time.Second,
		},
	}
}
