package resellerhub

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const (
	defaultMinDiscountBPS = 100
	defaultMaxDiscountBPS = 50000
)

type Config struct {
	Port                     string
	BasePath                 string
	GatewayBaseURL           string
	AuthTimeout              time.Duration
	ReconcileInterval        time.Duration
	ConsistencyGrace         time.Duration
	RetirementObservation    time.Duration
	LeaderLeaseDuration      time.Duration
	InstanceID               string
	AutoMigrate              bool
	MinDiscountBPS           int
	MaxDiscountBPS           int
	RedisEventMarkerTTL      time.Duration
	CarrierLowQuota          int
	KeyQPSAlertThreshold     int
	DisableBackgroundWorkers bool
}

func LoadConfig() Config {
	port := strings.TrimSpace(os.Getenv("RESELLER_HUB_PORT"))
	if port == "" {
		port = "3200"
	}
	basePath := normalizeBasePath(os.Getenv("RESELLER_HUB_BASE_PATH"))
	instanceID := strings.TrimSpace(os.Getenv("RESELLER_HUB_INSTANCE_ID"))
	if instanceID == "" {
		hostname, _ := os.Hostname()
		instanceID = hostname + "-" + strconv.Itoa(os.Getpid())
	}

	return Config{
		Port:                     port,
		BasePath:                 basePath,
		GatewayBaseURL:           strings.TrimRight(strings.TrimSpace(os.Getenv("RESELLER_HUB_GATEWAY_BASE_URL")), "/"),
		AuthTimeout:              seconds("RESELLER_HUB_AUTH_TIMEOUT_SECONDS", 10),
		ReconcileInterval:        seconds("RESELLER_HUB_RECONCILE_INTERVAL_SECONDS", 60),
		ConsistencyGrace:         seconds("RESELLER_HUB_CONSISTENCY_GRACE_SECONDS", 180),
		RetirementObservation:    seconds("RESELLER_HUB_RETIREMENT_OBSERVATION_SECONDS", 86400),
		LeaderLeaseDuration:      seconds("RESELLER_HUB_LEADER_LEASE_SECONDS", 30),
		InstanceID:               instanceID,
		AutoMigrate:              envBool("RESELLER_HUB_AUTO_MIGRATE", false),
		MinDiscountBPS:           envInt("RESELLER_HUB_MIN_DISCOUNT_BPS", defaultMinDiscountBPS),
		MaxDiscountBPS:           envInt("RESELLER_HUB_MAX_DISCOUNT_BPS", defaultMaxDiscountBPS),
		RedisEventMarkerTTL:      seconds("RESELLER_HUB_REDIS_EVENT_TTL_SECONDS", max(common.RedisKeyCacheSeconds()*3, 86400)),
		CarrierLowQuota:          envInt("RESELLER_HUB_CARRIER_LOW_QUOTA", 0),
		KeyQPSAlertThreshold:     envInt("RESELLER_HUB_KEY_QPS_ALERT_THRESHOLD", 0),
		DisableBackgroundWorkers: envBool("RESELLER_HUB_DISABLE_BACKGROUND_WORKERS", false),
	}
}

func normalizeBasePath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "/reseller"
	}
	if !strings.HasPrefix(value, "/") {
		value = "/" + value
	}
	value = strings.TrimRight(value, "/")
	if value == "" {
		return "/reseller"
	}
	return value
}

func seconds(name string, fallback int) time.Duration {
	return time.Duration(envInt(name, fallback)) * time.Second
}

func envInt(name string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envBool(name string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}
