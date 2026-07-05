package upstreamevent

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

const rawUsageContextKey = "upstream_event_raw_usage"

type RawUsageSnapshot struct {
	Provider string                 `json:"provider"`
	Format   string                 `json:"format"`
	Usage    map[string]interface{} `json:"usage"`
}

func SetRawUsage(c *gin.Context, provider string, format string, raw any) {
	if c == nil || raw == nil {
		return
	}
	usage := anyToMap(raw)
	if len(usage) == 0 {
		return
	}
	c.Set(rawUsageContextKey, RawUsageSnapshot{
		Provider: provider,
		Format:   format,
		Usage:    usage,
	})
}

func GetRawUsage(c *gin.Context) (RawUsageSnapshot, bool) {
	if c == nil {
		return RawUsageSnapshot{}, false
	}
	value, ok := c.Get(rawUsageContextKey)
	if !ok {
		return RawUsageSnapshot{}, false
	}
	snapshot, ok := value.(RawUsageSnapshot)
	return snapshot, ok
}

func anyToMap(value any) map[string]interface{} {
	if value == nil {
		return nil
	}
	if m, ok := value.(map[string]interface{}); ok {
		return m
	}
	data, err := common.Marshal(value)
	if err != nil || len(data) == 0 {
		return nil
	}
	var decoded map[string]interface{}
	if err := common.Unmarshal(data, &decoded); err != nil {
		return nil
	}
	return decoded
}
