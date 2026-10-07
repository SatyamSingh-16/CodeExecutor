//go:build integration

package api_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SatyamSingh-16/code_executor/internal/auth"
	"github.com/SatyamSingh-16/code_executor/internal/ratelimit"
	"github.com/redis/go-redis/v9"
)

func getIntegrationRedis(t *testing.T) *redis.Client {
	t.Helper()
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}
	rdb := redis.NewClient(&redis.Options{Addr: addr})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("skipping integration test: Redis unavailable: %v", err)
	}
	return rdb
}

func TestIntegration_RateLimit_ConcurrentBurst(t *testing.T) {
	db := getIntegrationDB(t)
	defer db.Close()
	rdb := getIntegrationRedis(t)
	defer rdb.Close()

	ctx := context.Background()

	// 1. Initialize Auth components and create a user
	tokenMgr, _ := auth.NewJWTTokenManager(auth.JWTConfig{
		Secret:     "ratelimit-integration-test-secret-32b!",
		Expiration: 2 * time.Hour,
	})
	userRepo := auth.NewPostgresUserRepository(db)
	authSvc := auth.NewService(userRepo, tokenMgr)
	authMiddleware := auth.NewMiddleware(tokenMgr)

	userEmail := fmt.Sprintf("ratelimit_user_%d@example.com", time.Now().UnixNano())
	regResp, err := authSvc.Register(ctx, auth.RegisterRequest{
		Email:    userEmail,
		Password: "Password12345!",
	})
	if err != nil {
		t.Fatalf("failed to register user: %v", err)
	}
	userID := regResp.User.ID
	token := regResp.Token

	// Clean up Redis key after test
	keyPrefix := fmt.Sprintf("test_burst_%d:submission_rate:", time.Now().UnixNano())
	defer rdb.Del(ctx, keyPrefix+userID)

	// 2. Initialize Rate Limiter: limit = 20 requests per 10 seconds
	const limit = 20
	limiter := ratelimit.NewRedisSlidingWindowLimiter(rdb, ratelimit.Config{
		Limit:     limit,
		Window:    10 * time.Second,
		KeyPrefix: keyPrefix,
	})
	rateMiddleware := ratelimit.NewMiddleware(limiter)

	// Protected mock endpoint
	var handlerHits int64
	mockEndpoint := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&handlerHits, 1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	protectedHandler := authMiddleware.RequireAuth(rateMiddleware.RequireRateLimit(mockEndpoint))

	// 3. Fire 50 concurrent requests simultaneously
	const totalRequests = 50
	var admittedCount int64
	var rejectedCount int64

	var startBarrier sync.WaitGroup
	startBarrier.Add(1)

	var wg sync.WaitGroup
	wg.Add(totalRequests)

	for i := 0; i < totalRequests; i++ {
		go func() {
			defer wg.Done()
			startBarrier.Wait() // Synchronize start to maximize concurrency collision

			req := httptest.NewRequest(http.MethodPost, "/api/submissions", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			rec := httptest.NewRecorder()

			protectedHandler.ServeHTTP(rec, req)

			switch rec.Code {
			case http.StatusOK:
				atomic.AddInt64(&admittedCount, 1)
			case http.StatusTooManyRequests:
				atomic.AddInt64(&rejectedCount, 1)
				// Verify Retry-After header exists on 429
				if rec.Header().Get("Retry-After") == "" {
					t.Errorf("missing Retry-After header on 429 response")
				}
			default:
				t.Errorf("unexpected status code: %d", rec.Code)
			}
		}()
	}

	startBarrier.Done() // Unleash all 50 goroutines
	wg.Wait()

	// 4. Assert exact limits: exactly 20 admitted, exactly 30 rejected
	if admittedCount != limit {
		t.Errorf("expected exactly %d admitted requests under high concurrency, got %d", limit, admittedCount)
	}
	if rejectedCount != (totalRequests - limit) {
		t.Errorf("expected exactly %d rejected requests, got %d", totalRequests-limit, rejectedCount)
	}
	if handlerHits != limit {
		t.Errorf("expected handlerHits=%d, got %d", limit, handlerHits)
	}

	// 5. Inspect Redis Sorted Set directly
	zcard, err := rdb.ZCard(ctx, keyPrefix+userID).Result()
	if err != nil {
		t.Fatalf("failed to query ZCard: %v", err)
	}
	if zcard != limit {
		t.Errorf("expected ZCARD to be exactly %d, got %d", limit, zcard)
	}
}

func TestIntegration_RateLimit_MultiUserIsolation(t *testing.T) {
	db := getIntegrationDB(t)
	defer db.Close()
	rdb := getIntegrationRedis(t)
	defer rdb.Close()

	ctx := context.Background()

	tokenMgr, _ := auth.NewJWTTokenManager(auth.JWTConfig{
		Secret:     "ratelimit-multiuser-test-secret-32b!!",
		Expiration: 2 * time.Hour,
	})
	userRepo := auth.NewPostgresUserRepository(db)
	authSvc := auth.NewService(userRepo, tokenMgr)
	authMiddleware := auth.NewMiddleware(tokenMgr)

	// Create User 1 and User 2
	u1, _ := authSvc.Register(ctx, auth.RegisterRequest{
		Email:    fmt.Sprintf("user1_%d@example.com", time.Now().UnixNano()),
		Password: "Password12345!",
	})
	u2, _ := authSvc.Register(ctx, auth.RegisterRequest{
		Email:    fmt.Sprintf("user2_%d@example.com", time.Now().UnixNano()),
		Password: "Password12345!",
	})

	keyPrefix := fmt.Sprintf("test_multi_%d:submission_rate:", time.Now().UnixNano())
	defer rdb.Del(ctx, keyPrefix+u1.User.ID, keyPrefix+u2.User.ID)

	limiter := ratelimit.NewRedisSlidingWindowLimiter(rdb, ratelimit.Config{
		Limit:     2,
		Window:    10 * time.Second,
		KeyPrefix: keyPrefix,
	})
	rateMiddleware := ratelimit.NewMiddleware(limiter)

	endpoint := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := authMiddleware.RequireAuth(rateMiddleware.RequireRateLimit(endpoint))

	// User 1 uses all 2 slots
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/submissions", nil)
		req.Header.Set("Authorization", "Bearer "+u1.Token)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("user 1 request %d failed: %d", i+1, rec.Code)
		}
	}

	// User 1 attempt 3 -> 429
	req1 := httptest.NewRequest(http.MethodPost, "/api/submissions", nil)
	req1.Header.Set("Authorization", "Bearer "+u1.Token)
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusTooManyRequests {
		t.Fatalf("expected user 1 request 3 to receive 429, got %d", rec1.Code)
	}

	// User 2 must still be allowed!
	req2 := httptest.NewRequest(http.MethodPost, "/api/submissions", nil)
	req2.Header.Set("Authorization", "Bearer "+u2.Token)
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("expected user 2 to be allowed with 200, got %d", rec2.Code)
	}
}
