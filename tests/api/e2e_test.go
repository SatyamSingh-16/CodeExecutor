//go:build integration

package api_test

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/SatyamSingh-16/code_executor/internal/auth"
	"github.com/SatyamSingh-16/code_executor/internal/queue"
	"github.com/SatyamSingh-16/code_executor/internal/ratelimit"
	"github.com/SatyamSingh-16/code_executor/internal/runner"
	"github.com/SatyamSingh-16/code_executor/internal/sse"
	"github.com/SatyamSingh-16/code_executor/internal/submission"
	"github.com/SatyamSingh-16/code_executor/internal/worker"
	"github.com/docker/docker/client"
	"github.com/redis/go-redis/v9"
)

// e2eTestEnvironment bundles the live API server, worker, background queue, and database.
type e2eTestEnvironment struct {
	server       *httptest.Server
	authSvc      *auth.Service
	db           *sql.DB
	rdb          *redis.Client
	streamName   string
	groupName    string
	worker       *worker.Worker
	dockerRunner *runner.DockerRunner
	cancelFunc   context.CancelFunc
}

func setupE2EEnvironment(t *testing.T, rateLimit int) *e2eTestEnvironment {
	t.Helper()

	db := getIntegrationDB(t)
	rdb := getIntegrationRedis(t)

	// Docker Client & Runner
	dockerCli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		t.Fatalf("failed to connect to Docker daemon for E2E: %v", err)
	}
	dockerRunner := runner.NewDockerRunner(dockerCli)

	// Auth components
	tokenMgr, err := auth.NewJWTTokenManager(auth.JWTConfig{
		Secret:     "e2e-integration-test-secret-32b-key!",
		Expiration: 2 * time.Hour,
	})
	if err != nil {
		t.Fatalf("failed to create JWT token manager: %v", err)
	}

	userRepo := auth.NewPostgresUserRepository(db)
	authSvc := auth.NewService(userRepo, tokenMgr)
	authHandler := auth.NewHandler(authSvc)
	authMiddleware := auth.NewMiddleware(tokenMgr)

	// Unique stream and group for test isolation
	uid := time.Now().UnixNano()
	streamName := fmt.Sprintf("e2e_stream_%d", uid)
	groupName := fmt.Sprintf("e2e_group_%d", uid)

	q := queue.NewRedisQueue(rdb, streamName, groupName)
	if err := q.InitConsumerGroup(context.Background()); err != nil {
		t.Fatalf("failed to init consumer group for stream %s: %v", streamName, err)
	}

	// Rate limiter
	if rateLimit <= 0 {
		rateLimit = 50
	}
	limiterPrefix := fmt.Sprintf("e2e_rate_%d:", uid)
	limiter := ratelimit.NewRedisSlidingWindowLimiter(rdb, ratelimit.Config{
		Limit:     rateLimit,
		Window:    5 * time.Second,
		KeyPrefix: limiterPrefix,
	})
	rateMiddleware := ratelimit.NewMiddleware(limiter)

	// Submission components
	subRepo := submission.NewPostgresSubmissionRepository(db)
	subSvc := submission.NewService(subRepo, q)
	subHandler := submission.NewHandler(subSvc)

	// SSE components
	sseSub := sse.NewRedisSubscriber(rdb)
	sseHandler := sse.NewHandler(subRepo, sseSub, sse.WithHeartbeatInterval(1*time.Second))

	// Router setup identical to cmd/api/main.go
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/register", authHandler.Register)
	mux.HandleFunc("/api/auth/login", authHandler.Login)
	mux.Handle("/api/auth/me", authMiddleware.RequireAuth(http.HandlerFunc(authHandler.Me)))

	mux.Handle("GET /api/submissions/{id}/stream", authMiddleware.RequireAuth(http.HandlerFunc(sseHandler.Stream)))
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

	server := httptest.NewServer(mux)

	// Worker setup
	workerRepo := worker.NewPostgresSubmissionRepository(db)
	workerConsumer := worker.NewRedisStreamConsumer(rdb, streamName, groupName, fmt.Sprintf("worker-%d", uid))
	workerPublisher := worker.NewRedisEventPublisher(rdb)

	workerCfg := worker.WorkerConfig{
		StreamName:       streamName,
		ConsumerGroup:    groupName,
		ConsumerName:     fmt.Sprintf("e2e-worker-%d", uid),
		ConcurrencyLimit: 4,
		PollBatchSize:    5,
		PollBlockTimeout: 200 * time.Millisecond,
		ShutdownTimeout:  5 * time.Second,
	}

	w := worker.NewWorker(workerCfg, workerRepo, dockerRunner, workerConsumer, workerPublisher)
	workerCtx, cancelWorker := context.WithCancel(context.Background())
	if err := w.Start(workerCtx); err != nil {
		t.Fatalf("failed to start worker: %v", err)
	}

	env := &e2eTestEnvironment{
		server:       server,
		authSvc:      authSvc,
		db:           db,
		rdb:          rdb,
		streamName:   streamName,
		groupName:    groupName,
		worker:       w,
		dockerRunner: dockerRunner,
		cancelFunc:   cancelWorker,
	}

	t.Cleanup(func() {
		w.Stop()
		cancelWorker()
		server.Close()
		_ = dockerRunner.Close()
		_ = dockerCli.Close()
		_ = rdb.Del(context.Background(), streamName)
		_ = db.Close()
		_ = rdb.Close()
	})

	return env
}

// helper to register a new user and login, returning userID, token, and email
func (env *e2eTestEnvironment) createAndLoginUser(t *testing.T, prefix string) (string, string, string) {
	t.Helper()
	email := fmt.Sprintf("%s_%d@example.com", prefix, time.Now().UnixNano())
	regResp, err := env.authSvc.Register(context.Background(), auth.RegisterRequest{
		Email:    email,
		Password: "Password123!",
	})
	if err != nil {
		t.Fatalf("failed to register test user %s: %v", email, err)
	}
	return regResp.User.ID, regResp.Token, email
}

// helper to POST /api/submissions
func (env *e2eTestEnvironment) postSubmission(t *testing.T, token, lang, code, stdin string) (int, submission.CreateSubmissionResponse, string) {
	t.Helper()
	payload, _ := json.Marshal(submission.CreateSubmissionRequest{
		Language:   lang,
		SourceCode: code,
		Stdin:      stdin,
	})

	req, err := http.NewRequest(http.MethodPost, env.server.URL+"/api/submissions", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /api/submissions failed: %v", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	var createResp submission.CreateSubmissionResponse
	_ = json.Unmarshal(bodyBytes, &createResp)

	return resp.StatusCode, createResp, string(bodyBytes)
}

// helper to GET /api/submissions/:id
func (env *e2eTestEnvironment) getSubmission(t *testing.T, token, id string) (int, submission.SubmissionDetailResponse, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/submissions/%s", env.server.URL, id), nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /api/submissions/%s failed: %v", id, err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	var detail submission.SubmissionDetailResponse
	_ = json.Unmarshal(bodyBytes, &detail)

	return resp.StatusCode, detail, string(bodyBytes)
}

// helper to connect to GET /api/submissions/:id/stream and collect events until stream closes
func (env *e2eTestEnvironment) streamSubmission(t *testing.T, token, id string, timeout time.Duration) (int, []map[string]any, error) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/submissions/%s/stream", env.server.URL, id), nil)
	if err != nil {
		return 0, nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, nil, nil
	}

	reader := bufio.NewReader(resp.Body)
	events, err := parseSSEEvents(reader)
	return resp.StatusCode, events, err
}

// pollDatabaseUntilTerminal polls PostgreSQL with a deadline
func (env *e2eTestEnvironment) pollDatabaseUntilTerminal(t *testing.T, id string, timeout time.Duration) *submission.SubmissionEntity {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var sub submission.SubmissionEntity
		err := env.db.QueryRow(`
			SELECT id, user_id, language, code, status, stdout, stderr,
			       compilation_output, stdout_truncated, stderr_truncated,
			       exit_code, execution_time_ms, memory_usage_kb
			FROM submissions WHERE id = $1
		`, id).Scan(
			&sub.ID, &sub.UserID, &sub.Language, &sub.Code, &sub.Status,
			&sub.Stdout, &sub.Stderr, &sub.CompilationOutput,
			&sub.StdoutTruncated, &sub.StderrTruncated,
			&sub.ExitCode, &sub.ExecutionTimeMs, &sub.MemoryUsageKb,
		)
		if err == nil && sse.IsTerminalStatus(sub.Status) {
			return &sub
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for submission %s to reach terminal status in PostgreSQL", id)
	return nil
}

// ============================================================================
// TEST 1: Full Lifecycle for Valid Python Code
// Covers: Register, Login, JWT, POST, QUEUED, Worker execution, Docker sandbox,
//         PostgreSQL result, Redis Pub/Sub, SSE delivery, GET authoritative fallback,
//         stdout, stderr, execution metrics.
// ============================================================================
func TestE2E_FullLifecycle_ValidPython(t *testing.T) {
	env := setupE2EEnvironment(t, 20)

	// 1. Register & Login
	_, token, _ := env.createAndLoginUser(t, "py_user")

	// 2. Submit Python program that prints output and accepts stdin
	pyCode := `
import sys
name = sys.stdin.read().strip()
print(f"Hello, {name}!")
sys.stderr.write("python_diag_log\n")
`
	status, createResp, body := env.postSubmission(t, token, "python", pyCode, "Antigravity")
	if status != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted, got %d. Body: %s", status, body)
	}
	if createResp.ID == "" {
		t.Fatalf("expected submission ID, got empty")
	}
	if createResp.Status != "QUEUED" {
		t.Errorf("expected initial status QUEUED, got %s", createResp.Status)
	}

	subID := createResp.ID

	// 3. Connect to SSE stream and observe live transition to terminal
	streamStatus, events, err := env.streamSubmission(t, token, subID, 10*time.Second)
	if err != nil {
		t.Fatalf("streaming failed: %v", err)
	}
	if streamStatus != http.StatusOK {
		t.Fatalf("expected SSE stream status 200, got %d", streamStatus)
	}
	if len(events) == 0 {
		t.Fatalf("expected SSE events, got none")
	}

	// Verify terminal event was received over SSE
	lastEvt := events[len(events)-1]
	if lastEvt["status"] != "SUCCESS" {
		t.Fatalf("expected terminal event SUCCESS over SSE, got %v", lastEvt["status"])
	}
	if stdout, _ := lastEvt["stdout"].(string); strings.TrimSpace(stdout) != "Hello, Antigravity!" {
		t.Errorf("expected SSE stdout 'Hello, Antigravity!', got %q", stdout)
	}
	if stderr, _ := lastEvt["stderr"].(string); strings.TrimSpace(stderr) != "python_diag_log" {
		t.Errorf("expected SSE stderr 'python_diag_log', got %q", stderr)
	}

	// 4. Verify authoritative GET /api/submissions/:id
	getStatus, detail, _ := env.getSubmission(t, token, subID)
	if getStatus != http.StatusOK {
		t.Fatalf("expected GET status 200, got %d", getStatus)
	}
	if detail.Status != "SUCCESS" {
		t.Errorf("expected GET detail status SUCCESS, got %s", detail.Status)
	}
	if strings.TrimSpace(detail.Stdout) != "Hello, Antigravity!" {
		t.Errorf("expected GET stdout 'Hello, Antigravity!', got %q", detail.Stdout)
	}
	if strings.TrimSpace(detail.Stderr) != "python_diag_log" {
		t.Errorf("expected GET stderr 'python_diag_log', got %q", detail.Stderr)
	}
	if detail.ExitCode == nil || *detail.ExitCode != 0 {
		t.Errorf("expected exit_code 0, got %v", detail.ExitCode)
	}
	if detail.ExecutionTimeMs == nil || *detail.ExecutionTimeMs <= 0 {
		t.Errorf("expected positive execution_time_ms, got %v", detail.ExecutionTimeMs)
	}
	if detail.MemoryUsageKb == nil || *detail.MemoryUsageKb <= 0 {
		t.Errorf("expected positive memory_usage_kb, got %v", detail.MemoryUsageKb)
	}

	// 5. Verify PostgreSQL directly
	dbEntity := env.pollDatabaseUntilTerminal(t, subID, 2*time.Second)
	if dbEntity.Status != "SUCCESS" {
		t.Errorf("expected DB status SUCCESS, got %s", dbEntity.Status)
	}
}

// ============================================================================
// TEST 2: Full Lifecycle for Valid Go Code (Two-Phase Pipeline)
// Covers: Go compile -> Go run -> Docker sandbox -> metrics extraction -> SSE -> GET
// ============================================================================
func TestE2E_FullLifecycle_ValidGo(t *testing.T) {
	env := setupE2EEnvironment(t, 20)

	_, token, _ := env.createAndLoginUser(t, "go_user")

	goCode := `package main
import (
	"fmt"
	"io"
	"os"
)
func main() {
	in, _ := io.ReadAll(os.Stdin)
	fmt.Printf("Go Echo: %s\n", string(in))
}`

	status, createResp, body := env.postSubmission(t, token, "go", goCode, "E2E_GO_STDIN")
	if status != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", status, body)
	}
	subID := createResp.ID

	// Wait for worker execution and stream SSE
	streamStatus, events, err := env.streamSubmission(t, token, subID, 15*time.Second)
	if err != nil {
		t.Fatalf("streaming failed: %v", err)
	}
	if streamStatus != http.StatusOK {
		t.Fatalf("expected 200, got %d", streamStatus)
	}
	if len(events) == 0 {
		t.Fatalf("expected SSE events, got none")
	}

	lastEvt := events[len(events)-1]
	if lastEvt["status"] != "SUCCESS" {
		t.Fatalf("expected final event status SUCCESS, got %v (events: %+v)", lastEvt["status"], events)
	}
	if stdout, _ := lastEvt["stdout"].(string); strings.TrimSpace(stdout) != "Go Echo: E2E_GO_STDIN" {
		t.Errorf("expected Go stdout 'Go Echo: E2E_GO_STDIN', got %q", stdout)
	}

	// GET verification
	getStatus, detail, _ := env.getSubmission(t, token, subID)
	if getStatus != http.StatusOK || detail.Status != "SUCCESS" {
		t.Fatalf("GET mismatch: status=%d, detail.Status=%s", getStatus, detail.Status)
	}
	if strings.TrimSpace(detail.Stdout) != "Go Echo: E2E_GO_STDIN" {
		t.Errorf("expected detail stdout match, got %q", detail.Stdout)
	}
}

// ============================================================================
// TEST 3: Compilation Error (Go syntax error)
// ============================================================================
func TestE2E_Outcome_CompilationError(t *testing.T) {
	env := setupE2EEnvironment(t, 20)
	_, token, _ := env.createAndLoginUser(t, "compile_err_user")

	badGoCode := `package main
func main() {
	this is a deliberate syntax error
}`

	_, createResp, _ := env.postSubmission(t, token, "go", badGoCode, "")
	subID := createResp.ID

	streamStatus, events, err := env.streamSubmission(t, token, subID, 12*time.Second)
	if err != nil {
		t.Fatalf("stream err: %v", err)
	}
	if streamStatus != http.StatusOK {
		t.Fatalf("expected 200, got %d", streamStatus)
	}

	lastEvt := events[len(events)-1]
	if lastEvt["status"] != "COMPILATION_ERROR" {
		t.Fatalf("expected COMPILATION_ERROR, got %v", lastEvt["status"])
	}

	// Verify GET
	_, detail, _ := env.getSubmission(t, token, subID)
	if detail.Status != "COMPILATION_ERROR" {
		t.Errorf("expected detail status COMPILATION_ERROR, got %s", detail.Status)
	}
	if detail.CompilationOutput == "" {
		t.Errorf("expected non-empty CompilationOutput")
	}
}

// ============================================================================
// TEST 4: Runtime Error (Python zero division)
// ============================================================================
func TestE2E_Outcome_RuntimeError(t *testing.T) {
	env := setupE2EEnvironment(t, 20)
	_, token, _ := env.createAndLoginUser(t, "runtime_err_user")

	badPyCode := `
x = 1 / 0
`
	_, createResp, _ := env.postSubmission(t, token, "python", badPyCode, "")
	subID := createResp.ID

	_, events, err := env.streamSubmission(t, token, subID, 10*time.Second)
	if err != nil {
		t.Fatalf("stream err: %v", err)
	}

	lastEvt := events[len(events)-1]
	if lastEvt["status"] != "RUNTIME_ERROR" {
		t.Fatalf("expected RUNTIME_ERROR, got %v", lastEvt["status"])
	}
	if stderr, _ := lastEvt["stderr"].(string); !strings.Contains(stderr, "ZeroDivisionError") {
		t.Errorf("expected stderr to mention ZeroDivisionError, got %q", stderr)
	}

	// Authoritative GET
	_, detail, _ := env.getSubmission(t, token, subID)
	if detail.Status != "RUNTIME_ERROR" {
		t.Errorf("expected detail status RUNTIME_ERROR, got %s", detail.Status)
	}
	if detail.ExitCode == nil || *detail.ExitCode == 0 {
		t.Errorf("expected non-zero exit code, got %v", detail.ExitCode)
	}
}

// ============================================================================
// TEST 5: Timeout (Python infinite loop)
// ============================================================================
func TestE2E_Outcome_TimeLimitExceeded(t *testing.T) {
	env := setupE2EEnvironment(t, 20)
	_, token, _ := env.createAndLoginUser(t, "tle_user")

	infiniteLoopCode := `
while True:
    pass
`
	_, createResp, _ := env.postSubmission(t, token, "python", infiniteLoopCode, "")
	subID := createResp.ID

	// Wait up to 15s (default Python runtime timeout is 5s)
	_, events, err := env.streamSubmission(t, token, subID, 15*time.Second)
	if err != nil {
		t.Fatalf("stream err: %v", err)
	}

	lastEvt := events[len(events)-1]
	if lastEvt["status"] != "TIME_LIMIT_EXCEEDED" {
		t.Fatalf("expected TIME_LIMIT_EXCEEDED, got %v", lastEvt["status"])
	}

	_, detail, _ := env.getSubmission(t, token, subID)
	if detail.Status != "TIME_LIMIT_EXCEEDED" {
		t.Errorf("expected detail status TIME_LIMIT_EXCEEDED, got %s", detail.Status)
	}
	if detail.ExitCode == nil || *detail.ExitCode != 137 {
		t.Errorf("expected exit code 137 for SIGKILL, got %v", detail.ExitCode)
	}
}

// ============================================================================
// TEST 6: Memory Limit Exceeded (Python heap bomb > 128MB)
// ============================================================================
func TestE2E_Outcome_MemoryLimitExceeded(t *testing.T) {
	env := setupE2EEnvironment(t, 20)
	_, token, _ := env.createAndLoginUser(t, "mle_user")

	oomCode := `
import sys
# Allocate 400 MB to breach the 128 MB cgroup limit
a = bytearray(400 * 1024 * 1024)
print("ALLOCATED")
`
	_, createResp, _ := env.postSubmission(t, token, "python", oomCode, "")
	subID := createResp.ID

	_, events, err := env.streamSubmission(t, token, subID, 12*time.Second)
	if err != nil {
		t.Fatalf("stream err: %v", err)
	}

	lastEvt := events[len(events)-1]
	status := fmt.Sprintf("%v", lastEvt["status"])
	if status != "MEMORY_LIMIT_EXCEEDED" && status != "RUNTIME_ERROR" {
		t.Fatalf("expected MEMORY_LIMIT_EXCEEDED or RUNTIME_ERROR, got %s", status)
	}

	_, detail, _ := env.getSubmission(t, token, subID)
	if detail.Status != "MEMORY_LIMIT_EXCEEDED" && detail.Status != "RUNTIME_ERROR" {
		t.Errorf("expected detail status MEMORY_LIMIT_EXCEEDED or RUNTIME_ERROR, got %s", detail.Status)
	}
	if strings.Contains(detail.Stdout, "ALLOCATED") {
		t.Errorf("hostile program must not have printed ALLOCATED")
	}
}

// ============================================================================
// TEST 7: 64KB Output Truncation
// ============================================================================
func TestE2E_Outcome_OutputTruncation64KB(t *testing.T) {
	env := setupE2EEnvironment(t, 20)
	_, token, _ := env.createAndLoginUser(t, "trunc_user")

	floodCode := `
import sys
# Output ~1 MB of text
for _ in range(10000):
    sys.stdout.write("B" * 100 + "\n")
`
	_, createResp, _ := env.postSubmission(t, token, "python", floodCode, "")
	subID := createResp.ID

	_, events, err := env.streamSubmission(t, token, subID, 12*time.Second)
	if err != nil {
		t.Fatalf("stream err: %v", err)
	}

	lastEvt := events[len(events)-1]
	if lastEvt["status"] != "SUCCESS" {
		t.Fatalf("expected SUCCESS, got %v", lastEvt["status"])
	}
	if trunc, _ := lastEvt["stdout_truncated"].(bool); !trunc {
		t.Errorf("expected stdout_truncated true")
	}

	_, detail, _ := env.getSubmission(t, token, subID)
	if !detail.StdoutTruncated {
		t.Errorf("expected detail.StdoutTruncated true")
	}
	if len(detail.Stdout) != 64*1024 {
		t.Errorf("expected detail.Stdout length exactly 65536, got %d", len(detail.Stdout))
	}
}

// ============================================================================
// TEST 8: Security & Ownership Isolation (User A vs User B)
// Covers:
// - Unauthenticated submission rejected (401)
// - Unauthenticated GET rejected (401)
// - Unauthenticated SSE rejected (401)
// - User B cannot access User A's submission via GET (404)
// - User B cannot stream User A's submission via SSE (404)
// ============================================================================
func TestE2E_Security_OwnershipIsolation(t *testing.T) {
	env := setupE2EEnvironment(t, 20)

	_, tokenA, _ := env.createAndLoginUser(t, "user_a")
	_, tokenB, _ := env.createAndLoginUser(t, "user_b")

	// 1. Unauthenticated POST rejected
	code, _, _ := env.postSubmission(t, "", "python", "print('hack')", "")
	if code != http.StatusUnauthorized {
		t.Errorf("expected 401 for unauthenticated POST, got %d", code)
	}

	// 2. User A creates submission
	code, createRespA, _ := env.postSubmission(t, tokenA, "python", "print('secret_of_user_a')", "")
	if code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", code)
	}
	subIDA := createRespA.ID

	// 3. Unauthenticated GET rejected
	getCode, _, _ := env.getSubmission(t, "", subIDA)
	if getCode != http.StatusUnauthorized {
		t.Errorf("expected 401 for unauthenticated GET, got %d", getCode)
	}

	// 4. Unauthenticated SSE rejected
	sseCode, _, _ := env.streamSubmission(t, "", subIDA, 2*time.Second)
	if sseCode != http.StatusUnauthorized {
		t.Errorf("expected 401 for unauthenticated SSE, got %d", sseCode)
	}

	// 5. User B requests User A's submission -> 404 (does not leak existence)
	getCodeB, _, _ := env.getSubmission(t, tokenB, subIDA)
	if getCodeB != http.StatusNotFound {
		t.Errorf("expected 404 for User B accessing User A's submission, got %d", getCodeB)
	}

	// 6. User B requests User A's SSE stream -> 404
	sseCodeB, _, _ := env.streamSubmission(t, tokenB, subIDA, 2*time.Second)
	if sseCodeB != http.StatusNotFound {
		t.Errorf("expected 404 for User B opening User A's stream, got %d", sseCodeB)
	}

	// 7. User A can access it cleanly
	getCodeA, detailA, _ := env.getSubmission(t, tokenA, subIDA)
	if getCodeA != http.StatusOK {
		t.Errorf("expected 200 for User A, got %d", getCodeA)
	}
	if detailA.ID != subIDA {
		t.Errorf("expected sub ID %s, got %s", subIDA, detailA.ID)
	}
}

// ============================================================================
// TEST 9: Rate Limiting Enforcement
// ============================================================================
func TestE2E_RateLimitingEnforcement(t *testing.T) {
	// Configure tight rate limit of 3 submissions per 5-second window
	env := setupE2EEnvironment(t, 3)

	_, token, _ := env.createAndLoginUser(t, "ratelimited_user")

	// Submit 3 requests (within limit)
	for i := 0; i < 3; i++ {
		code, _, _ := env.postSubmission(t, token, "python", fmt.Sprintf("print(%d)", i), "")
		if code != http.StatusAccepted {
			t.Fatalf("request %d within limit was rejected with %d", i+1, code)
		}
	}

	// 4th request must be rejected with 429 Too Many Requests
	code, _, body := env.postSubmission(t, token, "python", "print('blocked')", "")
	if code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 Too Many Requests on 4th request, got %d. Body: %s", code, body)
	}
}

// ============================================================================
// TEST 10: Already-Terminal SSE Fallback (Late Subscriber)
// ============================================================================
func TestE2E_AlreadyTerminal_SSEFallback(t *testing.T) {
	env := setupE2EEnvironment(t, 20)
	_, token, _ := env.createAndLoginUser(t, "late_subscriber_user")

	_, createResp, _ := env.postSubmission(t, token, "python", "print('quick done')", "")
	subID := createResp.ID

	// Wait for worker to finish execution in DB before connecting SSE client
	env.pollDatabaseUntilTerminal(t, subID, 10*time.Second)

	// Now connect late SSE client
	start := time.Now()
	sseCode, events, err := env.streamSubmission(t, token, subID, 3*time.Second)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("stream err: %v", err)
	}
	if sseCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", sseCode)
	}
	if elapsed > 2*time.Second {
		t.Errorf("expected already-terminal stream to terminate almost immediately, took %v", elapsed)
	}

	if len(events) != 1 {
		t.Fatalf("expected exactly 1 terminal event for already-terminal submission, got %d: %+v", len(events), events)
	}
	if events[0]["status"] != "SUCCESS" {
		t.Errorf("expected status SUCCESS, got %v", events[0]["status"])
	}
	if stdout, _ := events[0]["stdout"].(string); strings.TrimSpace(stdout) != "quick done" {
		t.Errorf("expected stdout 'quick done', got %q", stdout)
	}
}

// ============================================================================
// TEST 11: Concurrent Submissions from Same User
// ============================================================================
func TestE2E_ConcurrentSubmissions_SameUser(t *testing.T) {
	env := setupE2EEnvironment(t, 50)
	_, token, _ := env.createAndLoginUser(t, "concurrent_user")

	concurrency := 4
	var wg sync.WaitGroup
	subIDs := make([]string, concurrency)
	errs := make([]error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		idx := i
		go func() {
			defer wg.Done()
			code, createResp, body := env.postSubmission(t, token, "python", fmt.Sprintf("print('concur_%d')", idx), "")
			if code != http.StatusAccepted {
				errs[idx] = fmt.Errorf("client %d POST failed: %d (%s)", idx, code, body)
				return
			}
			subIDs[idx] = createResp.ID
		}()
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("concurrent submission %d error: %v", i, err)
		}
	}

	// Verify all submissions reach SUCCESS in PostgreSQL and have correct stdout
	for i, id := range subIDs {
		entity := env.pollDatabaseUntilTerminal(t, id, 15*time.Second)
		if entity.Status != "SUCCESS" {
			t.Errorf("submission %d (%s) expected SUCCESS, got %s", i, id, entity.Status)
		}
		expectedOut := fmt.Sprintf("concur_%d", i)
		if strings.TrimSpace(entity.Stdout) != expectedOut {
			t.Errorf("submission %d expected stdout %q, got %q", i, expectedOut, entity.Stdout)
		}
	}
}

// ============================================================================
// TEST 12: Multiple Users Isolated Under Load
// ============================================================================
func TestE2E_MultipleUsers_IsolatedConcurrency(t *testing.T) {
	env := setupE2EEnvironment(t, 50)

	numUsers := 3
	type userTest struct {
		userID string
		token  string
		subID  string
	}
	users := make([]userTest, numUsers)

	for i := 0; i < numUsers; i++ {
		uID, token, _ := env.createAndLoginUser(t, fmt.Sprintf("multi_iso_%d", i))
		users[i] = userTest{userID: uID, token: token}
	}

	// Each user submits their own job
	var wg sync.WaitGroup
	for i := 0; i < numUsers; i++ {
		wg.Add(1)
		idx := i
		go func() {
			defer wg.Done()
			code, resp, _ := env.postSubmission(t, users[idx].token, "python", fmt.Sprintf("print('user_payload_%d')", idx), "")
			if code != http.StatusAccepted {
				t.Errorf("user %d post failed with code %d", idx, code)
				return
			}
			users[idx].subID = resp.ID
		}()
	}
	wg.Wait()

	// Verify each user can only see their own submission
	for i := 0; i < numUsers; i++ {
		curr := users[i]
		entity := env.pollDatabaseUntilTerminal(t, curr.subID, 15*time.Second)
		if entity.Status != "SUCCESS" {
			t.Errorf("user %d submission expected SUCCESS, got %s", i, entity.Status)
		}

		// Owner check
		codeOwner, detail, _ := env.getSubmission(t, curr.token, curr.subID)
		if codeOwner != http.StatusOK {
			t.Errorf("user %d failed to GET own submission: %d", i, codeOwner)
		}
		if strings.TrimSpace(detail.Stdout) != fmt.Sprintf("user_payload_%d", i) {
			t.Errorf("user %d stdout mismatch", i)
		}

		// Cross-user check: other users must get 404
		for j := 0; j < numUsers; j++ {
			if i == j {
				continue
			}
			codeCross, _, _ := env.getSubmission(t, users[j].token, curr.subID)
			if codeCross != http.StatusNotFound {
				t.Errorf("user %d was able to see user %d's submission: got %d", j, i, codeCross)
			}
		}
	}
}
