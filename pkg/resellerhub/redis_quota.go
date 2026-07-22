package resellerhub

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/go-redis/redis/v8"
)

const quotaRedisScript = `
if redis.call('EXISTS', KEYS[2]) == 1 then
    return redis.call('GET', KEYS[2])
end
if redis.call('EXISTS', KEYS[1]) == 0 then
    redis.call('SET', KEYS[2], 'cache_absent', 'EX', ARGV[3])
    return 'cache_absent'
end
local current = tonumber(redis.call('HGET', KEYS[1], ARGV[1]))
if current == nil then
    redis.call('DEL', KEYS[1])
    redis.call('SET', KEYS[2], 'cache_invalidated', 'EX', ARGV[3])
    return 'cache_invalidated'
end
local delta = tonumber(ARGV[2])
if delta < 0 and current + delta < 0 then
    redis.call('DEL', KEYS[1])
    redis.call('SET', KEYS[2], 'cache_invalidated', 'EX', ARGV[3])
    return 'cache_invalidated'
end
redis.call('HINCRBY', KEYS[1], ARGV[1], delta)
if ARGV[4] ~= '' then
    redis.call('HSET', KEYS[1], 'Status', ARGV[4])
end
redis.call('SET', KEYS[2], 'applied', 'EX', ARGV[3])
return 'applied'
`

func redisHealthy(ctx context.Context) error {
	if !common.RedisEnabled {
		return nil
	}
	if common.RDB == nil {
		return errors.New("Redis is enabled but client is unavailable")
	}
	return common.RDB.Ping(ctx).Err()
}

func readCachedQuota(ctx context.Context, cacheKey, field string) (int64, bool, error) {
	if !common.RedisEnabled {
		return 0, false, nil
	}
	exists, err := common.RDB.Exists(ctx, cacheKey).Result()
	if err != nil {
		return 0, false, err
	}
	if exists == 0 {
		return 0, false, nil
	}
	value, err := common.RDB.HGet(ctx, cacheKey, field).Result()
	if errors.Is(err, redis.Nil) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	quota, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, false, fmt.Errorf("invalid cached quota: %w", err)
	}
	return quota, true, nil
}

func (a *App) applyRedisQuotaEvent(ctx context.Context, cacheKey, field, eventID string, delta int, status *int) (string, error) {
	if !common.RedisEnabled {
		return "redis_disabled", nil
	}
	statusValue := ""
	if status != nil {
		statusValue = strconv.Itoa(*status)
	}
	marker := "reseller_hub:quota_event:" + eventID
	result, err := common.RDB.Eval(ctx, quotaRedisScript, []string{cacheKey, marker}, field, delta, int(a.config.RedisEventMarkerTTL.Seconds()), statusValue).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return "", err
	}
	return fmt.Sprint(result), nil
}
