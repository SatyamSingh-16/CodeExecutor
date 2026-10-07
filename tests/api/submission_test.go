//go:build integration

package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/SatyamSingh-16/code_executor/internal/auth"
	"github.com/SatyamSingh-16/code_executor/internal/queue"
	"github.com/SatyamSingh-16/code_executor/internal/ratelimit"
	"github.com/SatyamSingh-16/code_executor/internal/submission"
)

func setupFullAPITestServer(t *testing.T, rateLimit int) (http.Handler, *auth.Service, string) {
	t.Helper()
	db := getIntegrationDB(t)
	rdb := getIntegrationRedis(t)

	tokenMgr, err := auth.NewJWTTokenManager(auth.JWTConfig{
		Secret:     "integration-test-secret-full-api-32b!",
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

	limiterKeyPrefix := fmt.Sprintf("test_sub_rate_%d:", time.Now().UnixNano())
	if rateLimit <= 0 {
		rateLimit = 20
	}
	limiter := ratelimit.NewRedisSlidingWindowLimiter(rdb, ratelimit.Config{
		Limit:     rateLimit,
		Window:    10 * time.Second,
		KeyPrefix: limiterKeyPrefix,
	})
	rateMiddleware := ratelimit.NewMiddleware(limiter)

	subRepo := submission.NewPostgresSubmissionRepository(db)
	subSvc := submission.NewService(subRepo, testQueue)
	subHandler := submission.NewHandler(subSvc)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/register", authHandler.Register)
	mux.HandleFunc("/api/auth/login", authHandler.Login)
	mux.Handle("/api/auth/me", authMiddleware.RequireAuth(http.HandlerFunc(authHandler.Me)))

	// Protected submission routes
	mux.Handle("POST /api/submissions", authMiddleware.RequireAuth(rateMiddleware.RequireRateLimit(http.HandlerFunc(subHandler.Create))))
	mux.Handle("GET /api/submissions", authMiddleware.RequireAuth(http.HandlerFunc(subHandler.List)))
	mux.Handle("GET /api/submissions/{id}", authMiddleware.RequireAuth(http.HandlerFunc(subHandler.Get)))
	// Fallback routing
	mux.Handle("/api/submissions/", authMiddleware.RequireAuth(http.HandlerFunc(subHandler.RouteSubmissions)))
	mux.Handle("/api/submissions", authMiddleware.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			rateMiddleware.RequireRateLimit(http.HandlerFunc(subHandler.Create)).ServeHTTP(w, r)
		} else {
			subHandler.RouteSubmissions(w, r)
		}
	})))

	return mux, authSvc, streamName
}

func TestIntegration_Submission_CreateAndGet(t *testing.T) {
	db := getIntegrationDB(t)
	defer db.Close()
	rdb := getIntegrationRedis(t)
	defer rdb.Close()

	ctx := context.Background()
	router, authSvc, streamName := setupFullAPITestServer(t, 20)
	defer rdb.Del(ctx, streamName)

	// 1. Register User A
	emailA := fmt.Sprintf("user_a_%d@example.com", time.Now().UnixNano())
	regRespA, err := authSvc.Register(ctx, auth.RegisterRequest{
		Email:    emailA,
		Password: "Password12345!",
	})
	if err != nil {
		t.Fatalf("failed to register user A: %v", err)
	}

	// 2. Submit Python Code: POST /api/submissions
	subPayload, _ := json.Marshal(submission.CreateSubmissionRequest{
		Language:   "python",
		SourceCode: "print('hello from full api integration')",
		Stdin:      "sample_stdin",
	})
	postReq := httptest.NewRequest(http.MethodPost, "/api/submissions", bytes.NewReader(subPayload))
	postReq.Header.Set("Authorization", "Bearer "+regRespA.Token)
	postRec := httptest.NewRecorder()

	router.ServeHTTP(postRec, postReq)

	if postRec.Code != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted, got %d. Body: %s", postRec.Code, postRec.Body.String())
	}

	var postResp submission.CreateSubmissionResponse
	if err := json.Unmarshal(postRec.Body.Bytes(), &postResp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if postResp.ID == "" {
		t.Fatal("expected non-empty submission ID")
	}
	if postResp.Status != "QUEUED" {
		t.Errorf("expected status QUEUED, got %s", postResp.Status)
	}

	// 3. Directly inspect PostgreSQL: Verify record persists with status QUEUED and user_id == userA
	var dbStatus, dbLang, dbUserID string
	err = db.QueryRowContext(ctx, "SELECT status, language, user_id FROM submissions WHERE id = $1;", postResp.ID).Scan(
		&dbStatus, &dbLang, &dbUserID,
	)
	if err != nil {
		t.Fatalf("failed to query database for submission: %v", err)
	}
	if dbStatus != "QUEUED" {
		t.Errorf("expected DB status QUEUED, got %s", dbStatus)
	}
	if dbLang != "python" {
		t.Errorf("expected DB language python, got %s", dbLang)
	}
	if dbUserID != regRespA.User.ID {
		t.Errorf("expected DB user_id %s, got %s", regRespA.User.ID, dbUserID)
	}

	// 4. Directly inspect Redis Stream: Verify XADD was emitted with submission_id
	entries, err := rdb.XRevRange(ctx, streamName, "+", "-").Result()
	if err != nil {
		t.Fatalf("failed to read redis stream: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least 1 entry in redis stream, got 0")
	}
	enqueuedID, ok := entries[0].Values["submission_id"].(string)
	if !ok || enqueuedID != postResp.ID {
		t.Errorf("expected enqueued submission_id %s, got %v", postResp.ID, entries[0].Values)
	}

	// 5. Query submission detail: GET /api/submissions/{id}
	getReq := httptest.NewRequest(http.MethodGet, "/api/submissions/"+postResp.ID, nil)
	getReq.Header.Set("Authorization", "Bearer "+regRespA.Token)
	getRec := httptest.NewRecorder()

	router.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on GET /api/submissions/{id}, got %d. Body: %s", getRec.Code, getRec.Body.String())
	}

	var detailResp submission.SubmissionDetailResponse
	if err := json.Unmarshal(getRec.Body.Bytes(), &detailResp); err != nil {
		t.Fatalf("failed to decode detail response: %v", err)
	}
	if detailResp.ID != postResp.ID {
		t.Errorf("expected ID %s, got %s", postResp.ID, detailResp.ID)
	}
	if detailResp.Status != "QUEUED" {
		t.Errorf("expected status QUEUED, got %s", detailResp.Status)
	}
	if detailResp.Language != "python" {
		t.Errorf("expected language python, got %s", detailResp.Language)
	}
}

func TestIntegration_Submission_OwnershipSecurity_UserACannotBeSeenByUserB(t *testing.T) {
	db := getIntegrationDB(t)
	defer db.Close()
	rdb := getIntegrationRedis(t)
	defer rdb.Close()

	ctx := context.Background()
	router, authSvc, streamName := setupFullAPITestServer(t, 20)
	defer rdb.Del(ctx, streamName)

	// User A
	regA, _ := authSvc.Register(ctx, auth.RegisterRequest{
		Email:    fmt.Sprintf("user_a_%d@example.com", time.Now().UnixNano()),
		Password: "Password12345!",
	})

	// User B
	regB, _ := authSvc.Register(ctx, auth.RegisterRequest{
		Email:    fmt.Sprintf("user_b_%d@example.com", time.Now().UnixNano()),
		Password: "Password12345!",
	})

	// User A creates submission
	subPayload, _ := json.Marshal(submission.CreateSubmissionRequest{
		Language:   "python",
		SourceCode: "print('user A secret payload')",
	})
	postReq := httptest.NewRequest(http.MethodPost, "/api/submissions", bytes.NewReader(subPayload))
	postReq.Header.Set("Authorization", "Bearer "+regA.Token)
	postRec := httptest.NewRecorder()
	router.ServeHTTP(postRec, postReq)

	var postResp submission.CreateSubmissionResponse
	_ = json.Unmarshal(postRec.Body.Bytes(), &postResp)

	// User B attempts to access User A's submission: GET /api/submissions/{id}
	getReqB := httptest.NewRequest(http.MethodGet, "/api/submissions/"+postResp.ID, nil)
	getReqB.Header.Set("Authorization", "Bearer "+regB.Token)
	getRecB := httptest.NewRecorder()
	router.ServeHTTP(getRecB, getReqB)

	// MUST RETURN 404 NOT FOUND (do not leak existence)
	if getRecB.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found when User B accesses User A's submission, got %d. Body: %s",
			getRecB.Code, getRecB.Body.String())
	}

	var errResp map[string]string
	_ = json.Unmarshal(getRecB.Body.Bytes(), &errResp)
	if errResp["error"] != "submission not found" {
		t.Errorf("expected error 'submission not found', got %q", errResp["error"])
	}

	// User B lists submissions: GET /api/submissions
	listReqB := httptest.NewRequest(http.MethodGet, "/api/submissions", nil)
	listReqB.Header.Set("Authorization", "Bearer "+regB.Token)
	listRecB := httptest.NewRecorder()
	router.ServeHTTP(listRecB, listReqB)

	if listRecB.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on list, got %d", listRecB.Code)
	}

	var listB []submission.SubmissionDetailResponse
	_ = json.Unmarshal(listRecB.Body.Bytes(), &listB)
	if len(listB) != 0 {
		t.Errorf("expected 0 submissions for user B, got %d (User A's submission leaked into User B list!)", len(listB))
	}
}

func TestIntegration_Submission_DisallowUnknownFields_ClientUserIDRejected(t *testing.T) {
	router, authSvc, _ := setupFullAPITestServer(t, 20)
	ctx := context.Background()

	reg, _ := authSvc.Register(ctx, auth.RegisterRequest{
		Email:    fmt.Sprintf("spoof_%d@example.com", time.Now().UnixNano()),
		Password: "Password12345!",
	})

	// Client maliciously specifies "user_id" in body
	payload := `{"language":"python","source_code":"print(1)","user_id":"fake-user-id"}`
	req := httptest.NewRequest(http.MethodPost, "/api/submissions", bytes.NewReader([]byte(payload)))
	req.Header.Set("Authorization", "Bearer "+reg.Token)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request when user_id is injected into JSON body, got %d", rec.Code)
	}
}

func TestIntegration_Submission_RateLimitingEnforced(t *testing.T) {
	router, authSvc, _ := setupFullAPITestServer(t, 3) // Quota = 3 requests
	ctx := context.Background()

	reg, _ := authSvc.Register(ctx, auth.RegisterRequest{
		Email:    fmt.Sprintf("rl_%d@example.com", time.Now().UnixNano()),
		Password: "Password12345!",
	})

	subPayload, _ := json.Marshal(submission.CreateSubmissionRequest{
		Language:   "python",
		SourceCode: "print(1)",
	})

	// Send 3 requests (all allowed)
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/submissions", bytes.NewReader(subPayload))
		req.Header.Set("Authorization", "Bearer "+reg.Token)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusAccepted {
			t.Fatalf("request %d failed: %d", i+1, rec.Code)
		}
	}

	// 4th request must be rejected with 429 Too Many Requests
	req := httptest.NewRequest(http.MethodPost, "/api/submissions", bytes.NewReader(subPayload))
	req.Header.Set("Authorization", "Bearer "+reg.Token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 Too Many Requests on 4th submission, got %d", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("expected Retry-After header on 429 response")
	}
}

func TestIntegration_Submission_UnauthenticatedRejected(t *testing.T) {
	router, _, _ := setupFullAPITestServer(t, 20)

	// 1. POST without auth
	req1 := httptest.NewRequest(http.MethodPost, "/api/submissions", bytes.NewReader([]byte(`{"language":"python","source_code":"print(1)"}`)))
	rec1 := httptest.NewRecorder()
	router.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for unauthenticated POST, got %d", rec1.Code)
	}

	// 2. GET detail without auth
	req2 := httptest.NewRequest(http.MethodGet, "/api/submissions/some-id", nil)
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for unauthenticated GET detail, got %d", rec2.Code)
	}

	// 3. GET list without auth
	req3 := httptest.NewRequest(http.MethodGet, "/api/submissions", nil)
	rec3 := httptest.NewRecorder()
	router.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for unauthenticated GET list, got %d", rec3.Code)
	}
}
