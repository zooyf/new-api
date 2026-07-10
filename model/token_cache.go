package model

import (
	"context"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
)

func cacheSetToken(token Token) error {
	key := common.GenerateHMAC(token.Key)
	token.Clean()
	err := common.RedisHSetObj(fmt.Sprintf("token:%s", key), &token, time.Duration(common.RedisKeyCacheSeconds())*time.Second)
	if err != nil {
		return err
	}
	return nil
}

func cacheDeleteToken(key string) error {
	key = common.GenerateHMAC(key)
	err := common.RedisDelKey(fmt.Sprintf("token:%s", key))
	if err != nil {
		return err
	}
	return nil
}

func cacheIncrTokenQuota(key string, increment int64) error {
	key = common.GenerateHMAC(key)
	err := common.RedisHIncrBy(fmt.Sprintf("token:%s", key), constant.TokenFiledRemainQuota, increment)
	if err != nil {
		return err
	}
	return nil
}

func cacheDecrTokenQuota(key string, decrement int64) error {
	return cacheIncrTokenQuota(key, -decrement)
}

func cacheSetTokenField(key string, field string, value string) error {
	key = common.GenerateHMAC(key)
	err := common.RedisHSetField(fmt.Sprintf("token:%s", key), field, value)
	if err != nil {
		return err
	}
	return nil
}

// CacheGetTokenByKey 从缓存中获取 token，如果缓存中不存在，则从数据库中获取
func cacheGetTokenByKey(key string) (*Token, error) {
	hmacKey := common.GenerateHMAC(key)
	if !common.RedisEnabled {
		return nil, fmt.Errorf("redis is not enabled")
	}
	var token Token
	err := common.RedisHGetObj(fmt.Sprintf("token:%s", hmacKey), &token)
	if err != nil {
		return nil, err
	}
	token.Key = key
	return &token, nil
}

// UpdateTokenCacheAfterExternalWrite applies metadata changed by a sidecar while
// preserving quota deltas that may still be buffered by the main process.
func UpdateTokenCacheAfterExternalWrite(oldKey string, token Token, remainQuotaDelta int, replaceRemainQuota bool) error {
	if !common.RedisEnabled || common.RDB == nil || oldKey == "" {
		return nil
	}
	oldRedisKey := fmt.Sprintf("token:%s", common.GenerateHMAC(oldKey))
	newRedisKey := fmt.Sprintf("token:%s", common.GenerateHMAC(token.Key))
	replaceQuotaFlag := "0"
	if replaceRemainQuota {
		replaceQuotaFlag = "1"
	}
	script := `
if redis.call('EXISTS', KEYS[1]) == 0 then
    return 0
end
if KEYS[1] ~= KEYS[2] then
    redis.call('RENAME', KEYS[1], KEYS[2])
end
redis.call('HSET', KEYS[2],
    'UserId', ARGV[1],
    'Name', ARGV[2],
    'Status', ARGV[3],
    'ExpiredTime', ARGV[4],
    'UnlimitedQuota', ARGV[5],
    'ModelLimitsEnabled', ARGV[6],
    'ModelLimits', ARGV[7],
    'Group', ARGV[8])
if ARGV[10] == '1' then
    redis.call('HSET', KEYS[2], 'RemainQuota', ARGV[9])
elseif tonumber(ARGV[9]) ~= 0 then
    redis.call('HINCRBY', KEYS[2], 'RemainQuota', ARGV[9])
end
return 1
`
	_, err := common.RDB.Eval(context.Background(), script, []string{oldRedisKey, newRedisKey},
		fmt.Sprint(token.UserId),
		token.Name,
		fmt.Sprint(token.Status),
		fmt.Sprint(token.ExpiredTime),
		fmt.Sprint(token.UnlimitedQuota),
		fmt.Sprint(token.ModelLimitsEnabled),
		token.ModelLimits,
		token.Group,
		fmt.Sprint(remainQuotaDelta),
		replaceQuotaFlag,
	).Result()
	return err
}
