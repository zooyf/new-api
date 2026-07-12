package upstreamevent

import (
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	writeModeSync   = "sync"
	writeModeAsync  = "async"
	writeModeHybrid = "hybrid"

	bodyModeMetadata = "metadata"
	bodyModeRedacted = "redacted"

	tokenOperationProviderEventBulkPath       = "/api/v1/gateway/provider-events/bulk"
	tokenOperationProviderEventBulkSchema     = "provider-event-bulk-v1"
	defaultTokenOperationProviderSourceSystem = "new-api"
)

type Config struct {
	Enabled                  bool
	SourceSystem             string
	GatewayID                string
	GatewayKey               string
	WebhookURL               string
	WebhookSecret            string
	SchemaVersion            string
	WriteMode                string
	BodyMode                 string
	SyncTimeout              time.Duration
	AsyncQueueSize           int
	AsyncFlushInterval       time.Duration
	AsyncFlushBatchSize      int
	DropLowPriorityWhenFull  bool
	DispatcherBatchSize      int
	DispatcherInterval       time.Duration
	DispatcherMaxRetry       int
	DispatcherRequestTimeout time.Duration
	RetentionDays            int
}

func LoadConfig() Config {
	nodeName := strings.TrimSpace(os.Getenv("NODE_NAME"))
	if nodeName == "" {
		nodeName = "new-api"
	}
	sourceSystem := strings.TrimSpace(os.Getenv("UPSTREAM_EVENT_SOURCE_SYSTEM"))
	if sourceSystem == "" {
		sourceSystem = defaultTokenOperationProviderSourceSystem
	}
	gatewayID := strings.TrimSpace(os.Getenv("UPSTREAM_EVENT_GATEWAY_ID"))
	if gatewayID == "" {
		gatewayID = nodeName
	}
	webhookURL := strings.TrimSpace(os.Getenv("UPSTREAM_EVENT_WEBHOOK_URL"))
	if webhookURL == "" {
		baseURL := strings.TrimSpace(os.Getenv("UPSTREAM_EVENT_TOKENOP_BASE_URL"))
		if baseURL != "" {
			webhookURL = strings.TrimRight(baseURL, "/") + tokenOperationProviderEventBulkPath
		}
	}
	schemaVersion := strings.TrimSpace(os.Getenv("UPSTREAM_EVENT_SCHEMA_VERSION"))
	if schemaVersion == "" {
		schemaVersion = tokenOperationProviderEventBulkSchema
	}
	writeMode := strings.ToLower(strings.TrimSpace(os.Getenv("UPSTREAM_EVENT_WRITE_MODE")))
	if writeMode == "" {
		writeMode = writeModeHybrid
	}
	if writeMode != writeModeSync && writeMode != writeModeAsync && writeMode != writeModeHybrid {
		writeMode = writeModeHybrid
	}
	bodyMode := strings.ToLower(strings.TrimSpace(os.Getenv("UPSTREAM_EVENT_BODY_MODE")))
	if bodyMode == "" {
		bodyMode = bodyModeMetadata
	}
	if bodyMode != bodyModeMetadata && bodyMode != bodyModeRedacted {
		bodyMode = bodyModeMetadata
	}
	return Config{
		Enabled:                  strings.EqualFold(os.Getenv("UPSTREAM_EVENT_ENABLED"), "true"),
		SourceSystem:             sourceSystem,
		GatewayID:                gatewayID,
		GatewayKey:               os.Getenv("UPSTREAM_EVENT_GATEWAY_KEY"),
		WebhookURL:               webhookURL,
		WebhookSecret:            os.Getenv("UPSTREAM_EVENT_WEBHOOK_SECRET"),
		SchemaVersion:            schemaVersion,
		WriteMode:                writeMode,
		BodyMode:                 bodyMode,
		SyncTimeout:              time.Duration(envInt("UPSTREAM_EVENT_SYNC_TIMEOUT_MS", 100)) * time.Millisecond,
		AsyncQueueSize:           envInt("UPSTREAM_EVENT_ASYNC_QUEUE_SIZE", 10000),
		AsyncFlushInterval:       time.Duration(envInt("UPSTREAM_EVENT_ASYNC_FLUSH_INTERVAL_MS", 1000)) * time.Millisecond,
		AsyncFlushBatchSize:      envInt("UPSTREAM_EVENT_ASYNC_FLUSH_BATCH_SIZE", 100),
		DropLowPriorityWhenFull:  !strings.EqualFold(os.Getenv("UPSTREAM_EVENT_DROP_LOW_PRIORITY_WHEN_FULL"), "false"),
		DispatcherBatchSize:      envInt("UPSTREAM_EVENT_DISPATCH_BATCH_SIZE", 100),
		DispatcherInterval:       time.Duration(envInt("UPSTREAM_EVENT_DISPATCH_INTERVAL_MS", 5000)) * time.Millisecond,
		DispatcherMaxRetry:       envInt("UPSTREAM_EVENT_MAX_RETRY", 10),
		DispatcherRequestTimeout: time.Duration(envInt("UPSTREAM_EVENT_DISPATCH_TIMEOUT_SECONDS", 30)) * time.Second,
		RetentionDays:            envInt("UPSTREAM_EVENT_RETENTION_DAYS", 30),
	}
}

func envInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}
