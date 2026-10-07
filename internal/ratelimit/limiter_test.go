package ratelimit

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func getTestRedis(t *testing.T) *redis.Client {
	t.Helper()
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}
	rdb := redis.NewClient(&redis.Options{Addr: addr})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("skipping Redis test: Redis unavailable: %v", err)
	}
	return rdb
}

func TestRedisSlidingWindowLimiter_Allow(t *testing.T) {
	rdb := getTestRedis(t)
	defer rdb.Close()
	ctx := context.Background()

	prefix := fmt.Sprintf("test_rate_%d:", time.Now().UnixNano())
	cfg := Config{
		Limit:     3,
		Window:    10 * time.Second,
		KeyPrefix: prefix,
	}
	limiter := NewRedisSlidingWindowLimiter(rdb, cfg)

	userID := "user-123"
	key := prefix + userID
	defer rdb.Del(ctx, key)

	// 1. First request allowed
	res1, err := limiter.Allow(ctx, userID)
	if err != nil {
		t.Fatalf("unexpected error on request 1: %v", err)
	}
	if !res1.Allowed || res1.Remaining != 2 {
		t.Errorf("expected allowed=true, remaining=2, got allowed=%v, remaining=%d", res1.Allowed, res1.Remaining)
	}

	// 2. Second request allowed
	res2, err := limiter.Allow(ctx, userID)
	if err != nil {
		t.Fatalf("unexpected error on request 2: %v", err)
	}
	if !res2.Allowed || res2.Remaining != 1 {
		t.Errorf("expected allowed=true, remaining=1, got allowed=%v, remaining=%d", res2.Allowed, res2.Remaining)
	}

	// 3. Third request allowed (reaches limit)
	res3, err := limiter.Allow(ctx, userID)
	if err != nil {
		t.Fatalf("unexpected error on request 3: %v", err)
	}
	if !res3.Allowed || res3.Remaining != 0 {
		t.Errorf("expected allowed=true, remaining=0, got allowed=%v, remaining=%d", res3.Allowed, res3.Remaining)
	}

	// 4. Fourth request rejected (limit exceeded)
	res4, err := limiter.Allow(ctx, userID)
	if err != nil {
		t.Fatalf("unexpected error on request 4: %v", err)
	}
	if res4.Allowed {
		t.Errorf("expected allowed=false, got allowed=true")
	}
	if res4.Remaining != 0 {
		t.Errorf("expected remaining=0, got %d", res4.Remaining)
	}
	if res4.RetryAfter <= 0 {
		t.Errorf("expected positive RetryAfter duration, got %v", res4.RetryAfter)
	}
}

func TestRedisSlidingWindowLimiter_SlidingWindowExpiry(t *testing.T) {
	rdb := getTestRedis(t)
	defer rdb.Close()
	ctx := context.Background()

	prefix := fmt.Sprintf("test_expire_%d:", time.Now().UnixNano())
	cfg := Config{
		Limit:     2,
		Window:    5 * time.Second,
		KeyPrefix: prefix,
	}
	limiter := NewRedisSlidingWindowLimiter(rdb, cfg)

	userID := "user-slide"
	key := prefix + userID
	defer rdb.Del(ctx, key)

	baseTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	currentTime := baseTime

	limiter.SetNowFunc(func() time.Time {
		return currentTime
	})

	// Fill quota
	_, _ = limiter.Allow(ctx, userID)
	_, _ = limiter.Allow(ctx, userID)

	// Third attempt rejected
	res, _ := limiter.Allow(ctx, userID)
	if res.Allowed {
		t.Fatal("expected request to be rejected at limit")
	}

	// Advance time by 6 seconds (past the 5s window)
	currentTime = baseTime.Add(6 * time.Second)

	// Now should be allowed again
	resAfter, err := limiter.Allow(ctx, userID)
	if err != nil {
		t.Fatalf("unexpected error after sliding window: %v", err)
	}
	if !resAfter.Allowed {
		t.Errorf("expected request to be allowed after sliding window advanced, got rejected")
	}
	if resAfter.Remaining != 1 {
		t.Errorf("expected remaining=1, got %d", resAfter.Remaining)
	}
}

func TestRedisSlidingWindowLimiter_IndependentUsers(t *testing.T) {
	rdb := getTestRedis(t)
	defer rdb.Close()
	ctx := context.Background()

	prefix := fmt.Sprintf("test_users_%d:", time.Now().UnixNano())
	cfg := Config{
		Limit:     1,
		Window:    10 * time.Second,
		KeyPrefix: prefix,
	}
	limiter := NewRedisSlidingWindowLimiter(rdb, cfg)

	userA := "user-alpha"
	userB := "user-beta"
	defer rdb.Del(ctx, prefix+userA, prefix+userB)

	// User A consumes quota
	resA1, _ := limiter.Allow(ctx, userA)
	if !resA1.Allowed {
		t.Fatal("expected user A request 1 to be allowed")
	}

	// User A rejected on request 2
	resA2, _ := limiter.Allow(ctx, userA)
	if resA2.Allowed {
		t.Fatal("expected user A request 2 to be rejected")
	}

	// User B is independent and must be allowed!
	resB1, err := limiter.Allow(ctx, userB)
	if err != nil {
		t.Fatalf("unexpected error for user B: %v", err)
	}
	if !resB1.Allowed {
		t.Errorf("expected user B to be allowed independently of user A, got rejected")
	}
}

func TestRedisSlidingWindowLimiter_InputValidation(t *testing.T) {
	rdb := getTestRedis(t)
	defer rdb.Close()

	limiter := NewRedisSlidingWindowLimiter(rdb, Config{})
	_, err := limiter.Allow(context.Background(), "")
	if err == nil {
		t.Fatal("expected error on empty identity, got nil")
	}
}
