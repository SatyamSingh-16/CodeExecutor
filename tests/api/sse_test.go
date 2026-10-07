//go:build integration

package api_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/SatyamSingh-16/code_executor/internal/auth"
	"github.com/SatyamSingh-16/code_executor/internal/queue"
	"github.com/SatyamSingh-16/code_executor/internal/ratelimit"
	"github.com/SatyamSingh-16/code_executor/internal/sse"
	"github.com/SatyamSingh-16/code_executor/internal/submission"
	"github.com/SatyamSingh-16/code_executor/internal/worker"
)

func setupSSEAPITestServer(t *testing.T) (http.Handler, *auth.Service, *submission.PostgresSubmissionRepository, *worker.RedisEventPublisher) {
	t.Helper()
	db := getIntegrationDB(t)
	rdb := getIntegrationRedis(t)

	tokenMgr, err := auth.NewJWTTokenManager(auth.JWTConfig{
		Secret:     "integration-test-secret-sse-api-32b!",
		Expiration: 2 * time.Hour,
	})
	if err != nil {
		t.Fatalf("failed to create token manager: %v", err)
	}

	userRepo := auth.NewPostgresUserRepository(db)
	authSvc := auth.NewService(userRepo, tokenMgr)
	authHandler := auth.NewHandler(authSvc)
	authMiddleware := auth.NewMiddleware(tokenMgr)

	streamName := fmt.Sprintf("test_sub_stream_%d", time.Now().UnixNano())
	testQueue := queue.NewRedisQueue(rdb, streamName, "test_group")
	_ = testQueue.InitConsumerGroup(context.Background())

	limiterKeyPrefix := fmt.Sprintf("test_sse_rate_%d:", time.Now().UnixNano())
	limiter := ratelimit.NewRedisSlidingWindowLimiter(rdb, ratelimit.Config{
		Limit:     100,
		Window:    10 * time.Second,
		KeyPrefix: limiterKeyPrefix,
	})
	rateMiddleware := ratelimit.NewMiddleware(limiter)

	subRepo := submission.NewPostgresSubmissionRepository(db)
	subSvc := submission.NewService(subRepo, testQueue)
	subHandler := submission.NewHandler(subSvc)

	sseSubscriber := sse.NewRedisSubscriber(rdb)
	sseHandler := sse.NewHandler(subRepo, sseSubscriber, sse.WithHeartbeatInterval(1*time.Second))

	publisher := worker.NewRedisEventPublisher(rdb)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/register", authHandler.Register)
	mux.HandleFunc("/api/auth/login", authHandler.Login)
	mux.Handle("/api/auth/me", authMiddleware.RequireAuth(http.HandlerFunc(authHandler.Me)))

	// SSE Stream route
	mux.Handle("GET /api/submissions/{id}/stream", authMiddleware.RequireAuth(http.HandlerFunc(sseHandler.Stream)))

	// Protected submission routes
	mux.Handle("POST /api/submissions", authMiddleware.RequireAuth(rateMiddleware.RequireRateLimit(http.HandlerFunc(subHandler.Create))))
	mux.Handle("GET /api/submissions", authMiddleware.RequireAuth(http.HandlerFunc(subHandler.List)))
	mux.Handle("GET /api/submissions/{id}", authMiddleware.RequireAuth(http.HandlerFunc(subHandler.Get)))
	mux.Handle("/api/submissions/", authMiddleware.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/stream") {
			sseHandler.Stream(w, r)
			return
		}
		subHandler.RouteSubmissions(w, r)
	})))

	return mux, authSvc, subRepo, publisher
}

func registerAndLoginUser(t *testing.T, authSvc *auth.Service, prefix string) (string, string) {
	t.Helper()
	email := fmt.Sprintf("%s_%d@example.com", prefix, time.Now().UnixNano())
	regResp, err := authSvc.Register(context.Background(), auth.RegisterRequest{
		Email:    email,
		Password: "Password123!",
	})
	if err != nil {
		t.Fatalf("failed to register test user: %v", err)
	}
	return regResp.User.ID, regResp.Token
}

func parseSSEEvents(reader *bufio.Reader) ([]map[string]any, error) {
	var events []map[string]any
	var currentData string

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		line = strings.TrimRight(line, "\r\n")
		if strings.HasPrefix(line, "data: ") {
			currentData = strings.TrimPrefix(line, "data: ")
		} else if line == "" && currentData != "" {
			var parsed map[string]any
			if err := json.Unmarshal([]byte(currentData), &parsed); err == nil {
				events = append(events, parsed)
			}
			currentData = ""
		}
	}
	return events, nil
}

func TestIntegration_SSE_OwnershipEnforcement(t *testing.T) {
	handler, authSvc, subRepo, _ := setupSSEAPITestServer(t)
	server := httptest.NewServer(handler)
	defer server.Close()

	userA_ID, tokenA := registerAndLoginUser(t, authSvc, "userA")
	_, tokenB := registerAndLoginUser(t, authSvc, "userB")

	// User A creates submission
	created, err := subRepo.Create(context.Background(), &submission.SubmissionEntity{
		UserID:   userA_ID,
		Language: "python",
		Code:     "print('hello from A')",
	})
	if err != nil {
		t.Fatalf("failed to create submission for user A: %v", err)
	}

	// 1. Unauthenticated request -> 401
	req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/submissions/%s/stream", server.URL, created.ID), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated stream request, got %d", resp.StatusCode)
	}
	_ = resp.Body.Close()

	// 2. User B requests User A's submission -> 404 (does not leak existence)
	reqB, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/submissions/%s/stream", server.URL, created.ID), nil)
	reqB.Header.Set("Authorization", "Bearer "+tokenB)
	respB, err := http.DefaultClient.Do(reqB)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if respB.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for non-owner, got %d", respB.StatusCode)
	}
	_ = respB.Body.Close()

	// 3. Request nonexistent submission -> 404
	reqNonexistent, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/submissions/00000000-0000-0000-0000-000000000000/stream", server.URL), nil)
	reqNonexistent.Header.Set("Authorization", "Bearer "+tokenA)
	respNone, err := http.DefaultClient.Do(reqNonexistent)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if respNone.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for nonexistent submission, got %d", respNone.StatusCode)
	}
	_ = respNone.Body.Close()
}

func TestIntegration_SSE_AlreadyTerminalClosesImmediately(t *testing.T) {
	handler, authSvc, _, _ := setupSSEAPITestServer(t)
	server := httptest.NewServer(handler)
	defer server.Close()
	db := getIntegrationDB(t)

	userID, token := registerAndLoginUser(t, authSvc, "terminal_user")

	// Insert already terminal submission directly in DB
	exitCode := 0
	execTime := 45
	memKB := 12500
	var subID string
	err := db.QueryRow(`
		INSERT INTO submissions (
			user_id, language, code, status, stdout, stderr, exit_code, execution_time_ms, memory_usage_kb, created_at, updated_at
		) VALUES (
			$1, 'python', 'print("done")', 'SUCCESS', $2, '', $3, $4, $5, NOW(), NOW()
		) RETURNING id;
	`, userID, "output line\n", exitCode, execTime, memKB).Scan(&subID)
	if err != nil {
		t.Fatalf("failed to insert terminal submission: %v", err)
	}

	req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/submissions/%s/stream", server.URL, subID), nil)
	req.Header.Set("Authorization", "Bearer "+token)

	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("expected Content-Type text/event-stream, got %s", ct)
	}

	reader := bufio.NewReader(resp.Body)
	events, err := parseSSEEvents(reader)
	if err != nil {
		t.Fatalf("failed to parse SSE events: %v", err)
	}

	elapsed := time.Since(start)
	if elapsed > 2*time.Second {
		t.Fatalf("stream took too long to terminate (%v), expected immediate termination", elapsed)
	}

	if len(events) != 1 {
		t.Fatalf("expected exactly 1 terminal event, got %d: %+v", len(events), events)
	}
	evt := events[0]
	if evt["status"] != "SUCCESS" {
		t.Fatalf("expected status SUCCESS, got %v", evt["status"])
	}
	if stdout, ok := evt["stdout"].(string); !ok || strings.TrimSpace(stdout) != "output line" {
		t.Fatalf("expected stdout 'output line', got %q", evt["stdout"])
	}
	if fmt.Sprintf("%v", evt["exit_code"]) != "0" {
		t.Fatalf("expected exit_code 0, got %v", evt["exit_code"])
	}
}

func TestIntegration_SSE_LivePubSubFanout(t *testing.T) {
	handler, authSvc, subRepo, publisher := setupSSEAPITestServer(t)
	server := httptest.NewServer(handler)
	defer server.Close()
	db := getIntegrationDB(t)

	userID, token := registerAndLoginUser(t, authSvc, "live_user")

	created, err := subRepo.Create(context.Background(), &submission.SubmissionEntity{
		UserID:   userID,
		Language: "python",
		Code:     "import time; time.sleep(0.1); print('live')",
	})
	if err != nil {
		t.Fatalf("failed to create submission: %v", err)
	}

	req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/submissions/%s/stream", server.URL, created.ID), nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to connect to stream: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}

	reader := bufio.NewReader(resp.Body)

	// In a background goroutine, simulate worker lifecycle:
	// 1. DB -> PROCESSING, Publish Pub/Sub event
	// 2. DB -> SUCCESS, Publish Pub/Sub event
	go func() {
		time.Sleep(100 * time.Millisecond)

		// Transition to PROCESSING
		_, _ = db.Exec(`UPDATE submissions SET status = 'PROCESSING', updated_at = NOW() WHERE id = $1`, created.ID)
		_ = publisher.PublishStatusEvent(context.Background(), created.ID, "PROCESSING")

		time.Sleep(100 * time.Millisecond)

		// Transition to SUCCESS
		_, _ = db.Exec(`
			UPDATE submissions
			SET status = 'SUCCESS', stdout = $2, exit_code = 0, execution_time_ms = 85, updated_at = NOW()
			WHERE id = $1
		`, created.ID, "live output\n")
		_ = publisher.PublishStatusEvent(context.Background(), created.ID, "SUCCESS")
	}()

	events, err := parseSSEEvents(reader)
	if err != nil {
		t.Fatalf("failed to read events: %v", err)
	}

	if len(events) < 2 {
		t.Fatalf("expected at least 2 events (QUEUED + SUCCESS or QUEUED + PROCESSING + SUCCESS), got %d: %+v", len(events), events)
	}

	// First event should be QUEUED
	if events[0]["status"] != "QUEUED" {
		t.Errorf("expected first event to be QUEUED, got %v", events[0]["status"])
	}

	// Last event must be terminal SUCCESS with authoritative DB data
	lastEvt := events[len(events)-1]
	if lastEvt["status"] != "SUCCESS" {
		t.Fatalf("expected final event status SUCCESS, got %v", lastEvt["status"])
	}
	if stdout, ok := lastEvt["stdout"].(string); !ok || strings.TrimSpace(stdout) != "live output" {
		t.Fatalf("expected authoritative stdout 'live output', got %q", lastEvt["stdout"])
	}
	if fmt.Sprintf("%v", lastEvt["exit_code"]) != "0" {
		t.Fatalf("expected authoritative exit_code 0, got %v", lastEvt["exit_code"])
	}
}

func TestIntegration_SSE_MultipleSubscribersIndependent(t *testing.T) {
	handler, authSvc, subRepo, publisher := setupSSEAPITestServer(t)
	server := httptest.NewServer(handler)
	defer server.Close()
	db := getIntegrationDB(t)

	userID, token := registerAndLoginUser(t, authSvc, "multi_user")

	created, err := subRepo.Create(context.Background(), &submission.SubmissionEntity{
		UserID:   userID,
		Language: "python",
		Code:     "print('multi')",
	})
	if err != nil {
		t.Fatalf("failed to create submission: %v", err)
	}

	// Connect 3 concurrent SSE clients for the same submission
	numClients := 3
	var wg sync.WaitGroup
	results := make([][]map[string]any, numClients)

	for i := 0; i < numClients; i++ {
		wg.Add(1)
		clientIdx := i
		go func() {
			defer wg.Done()

			req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/submissions/%s/stream", server.URL, created.ID), nil)
			req.Header.Set("Authorization", "Bearer "+token)

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Errorf("client %d request failed: %v", clientIdx, err)
				return
			}
			defer resp.Body.Close()

			reader := bufio.NewReader(resp.Body)
			events, err := parseSSEEvents(reader)
			if err != nil {
				t.Errorf("client %d parse failed: %v", clientIdx, err)
				return
			}
			results[clientIdx] = events
		}()
	}

	// Give clients time to connect
	time.Sleep(150 * time.Millisecond)

	// Transition to SUCCESS
	_, _ = db.Exec(`
		UPDATE submissions
		SET status = 'SUCCESS', stdout = $2, exit_code = 0, updated_at = NOW()
		WHERE id = $1
	`, created.ID, "multi done\n")
	_ = publisher.PublishStatusEvent(context.Background(), created.ID, "SUCCESS")

	wg.Wait()

	for i := 0; i < numClients; i++ {
		evts := results[i]
		if len(evts) == 0 {
			t.Fatalf("client %d received no events", i)
		}
		last := evts[len(evts)-1]
		if last["status"] != "SUCCESS" {
			t.Errorf("client %d last event status %v != SUCCESS", i, last["status"])
		}
		if stdout, ok := last["stdout"].(string); !ok || strings.TrimSpace(stdout) != "multi done" {
			t.Errorf("client %d last event stdout %q != 'multi done'", i, last["stdout"])
		}
	}
}
