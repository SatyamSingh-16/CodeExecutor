package ratelimit

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

var slidingWindowLua = redis.NewScript(`
local key = KEYS[1]
local now = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])
local member = ARGV[4]
local ttl = tonumber(ARGV[5])

local clear_before = now - window

-- 1. Remove entries older than the sliding window
redis.call('ZREMRANGEBYSCORE', key, '-inf', clear_before)

-- 2. Count current entries in window
local count = redis.call('ZCARD', key)

-- 3. If count >= limit, compute retry-after from oldest entry and reject
if count >= limit then
    local oldest = redis.call('ZRANGE', key, 0, 0, 'WITHSCORES')
    local retry_after_ms = 0
    if #oldest >= 2 then
        local oldest_time = tonumber(oldest[2])
        retry_after_ms = (oldest_time + window) - now
        if retry_after_ms < 0 then
            retry_after_ms = 0
        end
    else
        retry_after_ms = window
    end
    return {0, count, retry_after_ms}
end

-- 4. Otherwise, admit request and add current entry
redis.call('ZADD', key, now, member)
redis.call('EXPIRE', key, ttl)

return {1, count + 1, 0}
`)

// RedisSlidingWindowLimiter implements RateLimiter using Redis Sorted Sets with an atomic Lua script.
type RedisSlidingWindowLimiter struct {
	client    *redis.Client
	config    Config
	nowFunc   func() time.Time
	mu        sync.RWMutex
}

// NewRedisSlidingWindowLimiter constructs a new rate limiter with the provided Redis client and config.
func NewRedisSlidingWindowLimiter(client *redis.Client, cfg Config) *RedisSlidingWindowLimiter {
	if cfg.Limit <= 0 {
		cfg.Limit = DefaultRateLimit
	}
	if cfg.Window <= 0 {
		cfg.Window = DefaultWindow
	}
	if cfg.KeyPrefix == "" {
		cfg.KeyPrefix = DefaultKeyPrefix
	}

	return &RedisSlidingWindowLimiter{
		client:  client,
		config:  cfg,
		nowFunc: time.Now,
	}
}

// SetNowFunc overrides the current time provider (primarily for deterministic testing).
func (l *RedisSlidingWindowLimiter) SetNowFunc(fn func() time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.nowFunc = fn
}

func (l *RedisSlidingWindowLimiter) now() time.Time {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.nowFunc != nil {
		return l.nowFunc()
	}
	return time.Now()
}

// Allow evaluates whether an action for the given identity is permitted under the rate limit.
func (l *RedisSlidingWindowLimiter) Allow(ctx context.Context, identity string) (Result, error) {
	if strings.TrimSpace(identity) == "" {
		return Result{}, errors.New("identity cannot be empty")
	}

	now := l.now()
	nowMs := now.UnixNano() / int64(time.Millisecond)
	windowMs := l.config.Window.Milliseconds()

	// Member must be unique to prevent collisions in the sorted set
	uniqueID := generateUniqueID()
	member := fmt.Sprintf("%d:%s", nowMs, uniqueID)

	key := l.config.KeyPrefix + identity
	ttlSeconds := int64(math.Ceil(l.config.Window.Seconds())) + 2

	keys := []string{key}
	args := []interface{}{nowMs, windowMs, l.config.Limit, member, ttlSeconds}

	res, err := slidingWindowLua.Run(ctx, l.client, keys, args...).Slice()
	if err != nil {
		return Result{}, fmt.Errorf("rate limit evaluation error: %w", err)
	}

	if len(res) < 3 {
		return Result{}, fmt.Errorf("unexpected lua result format: %v", res)
	}

	allowedInt, _ := toInt64(res[0])
	countInt, _ := toInt64(res[1])
	retryAfterMs, _ := toInt64(res[2])

	allowed := allowedInt == 1
	remaining := l.config.Limit - int(countInt)
	if remaining < 0 {
		remaining = 0
	}

	var retryAfter time.Duration
	if !allowed && retryAfterMs > 0 {
		retryAfter = time.Duration(retryAfterMs) * time.Millisecond
	}

	return Result{
		Allowed:    allowed,
		Limit:      l.config.Limit,
		Remaining:  remaining,
		RetryAfter: retryAfter,
	}, nil
}

func generateUniqueID() string {
	b := make([]byte, 8)
	_, err := rand.Read(b)
	if err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

func toInt64(v interface{}) (int64, bool) {
	switch val := v.(type) {
	case int64:
		return val, true
	case int:
		return int64(val), true
	case float64:
		return int64(val), true
	default:
		return 0, false
	}
}
