//go:build integration

package worker_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/SatyamSingh-16/code_executor/internal/database"
	"github.com/SatyamSingh-16/code_executor/internal/queue"
	"github.com/SatyamSingh-16/code_executor/internal/runner"
	"github.com/SatyamSingh-16/code_executor/internal/worker"
	"github.com/docker/docker/client"
	_ "github.com/lib/pq"
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
		t.Skipf("skipping test: Redis unreachable at %s: %v", addr, err)
	}
	return rdb
}

func getTestDB(t *testing.T) *sql.DB {
	t.Helper()
	urls := []string{
		os.Getenv("DATABASE_URL"),
		"postgres://satyamsingh2730@localhost:5432/code_execution_test_db?sslmode=disable",
		"postgres://postgres@localhost:5432/code_execution_test_db?sslmode=disable",
		"postgres://localhost:5432/code_execution_test_db?sslmode=disable",
	}

	var db *sql.DB
	for _, u := range urls {
		if u == "" {
			continue
		}
		cand, err := sql.Open("postgres", u)
		if err != nil {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err = cand.PingContext(ctx)
		cancel()
		if err == nil {
			db = cand
			break
		}
		_ = cand.Close()
	}

	if db == nil {
		t.Skip("skipping test: PostgreSQL test DB unreachable")
	}

	migrator := database.NewMigrator(db)
	if err := migrator.Up(context.Background()); err != nil {
		t.Fatalf("failed to apply migrations: %v", err)
	}

	return db
}

func getTestRunner(t *testing.T) *runner.DockerRunner {
	t.Helper()
	var opts []client.Opt
	opts = append(opts, client.WithAPIVersionNegotiation())

	if os.Getenv("DOCKER_HOST") != "" {
		opts = append(opts, client.FromEnv)
	} else {
		homeDir, _ := os.UserHomeDir()
		desktopSock := homeDir + "/.docker/run/docker.sock"
		if _, err := os.Stat(desktopSock); err == nil {
			opts = append(opts, client.WithHost("unix://"+desktopSock))
		} else {
			opts = append(opts, client.FromEnv)
		}
	}

	cli, err := client.NewClientWithOpts(opts...)
	if err != nil {
		t.Skipf("skipping test: Docker client unavailable: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := cli.Ping(ctx); err != nil {
		_ = cli.Close()
		t.Skipf("skipping test: Docker daemon unreachable: %v", err)
	}
	return runner.NewDockerRunner(cli)
}

func ensureTestUser(t *testing.T, db *sql.DB) string {
	t.Helper()
	var userID string
	err := db.QueryRow(`
		INSERT INTO users (email, password_hash)
		VALUES ('worker_reliability@example.com', 'hashed_pw')
		ON CONFLICT (email) DO UPDATE SET email = EXCLUDED.email
		RETURNING id;
	`).Scan(&userID)
	if err != nil {
		t.Fatalf("failed to ensure test user: %v", err)
	}
	return userID
}

// 1. CONCURRENCY SEMAPHORE TEST (Real Docker Infrastructure)
// Verifies worker throttles 6 concurrent sleeping jobs with WORKER_CONCURRENCY=2.
func TestWorkerConcurrency_RealInfrastructure(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()
	rdb := getTestRedis(t)
	defer rdb.Close()
	dockerRunner := getTestRunner(t)
	defer dockerRunner.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	userID := ensureTestUser(t, db)
	testStream := fmt.Sprintf("test:conc:stream:%d", time.Now().UnixNano())
	testGroup := "test_conc_group"
	defer rdb.Del(context.Background(), testStream)

	q := queue.NewRedisQueue(rdb, testStream, testGroup)
	if err := q.InitConsumerGroup(ctx); err != nil {
		t.Fatalf("InitConsumerGroup failed: %v", err)
	}

	const jobCount = 6
	subIDs := make([]string, jobCount)
	for i := 0; i < jobCount; i++ {
		code := fmt.Sprintf("import time; time.sleep(0.5); print('done-%d')", i)
		err := db.QueryRowContext(ctx, `
			INSERT INTO submissions (user_id, language, code, stdin, status, retry_count, created_at, updated_at)
			VALUES ($1, 'python', $2, '', 'QUEUED', 0, NOW(), NOW())
			RETURNING id;
		`, userID, code).Scan(&subIDs[i])
		if err != nil {
			t.Fatalf("failed to insert submission %d: %v", i, err)
		}
		if err := q.Enqueue(ctx, subIDs[i]); err != nil {
			t.Fatalf("failed to enqueue submission %d: %v", i, err)
		}
	}

	repo := worker.NewPostgresSubmissionRepository(db)
	consumer := worker.NewRedisStreamConsumer(rdb, testStream, testGroup, "worker-conc")
	publisher := worker.NewRedisEventPublisher(rdb)

	cfg := worker.WorkerConfig{
		StreamName:       testStream,
		ConsumerGroup:    testGroup,
		ConsumerName:     "worker-conc",
		ConcurrencyLimit: 2, // WORKER_CONCURRENCY = 2
		PollBatchSize:    6,
		PollBlockTimeout: 200 * time.Millisecond,
		ShutdownTimeout:  10 * time.Second,
	}

	w := worker.NewWorker(cfg, repo, dockerRunner, consumer, publisher)
	if err := w.Start(ctx); err != nil {
		t.Fatalf("failed to start worker: %v", err)
	}
	defer w.Stop()

	// Wait for all 6 jobs to finish
	deadline := time.Now().Add(20 * time.Second)
	allCompleted := false
	for time.Now().Before(deadline) {
		var completedCount int
		err := db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM submissions WHERE id = ANY($1) AND status = 'SUCCESS';
		`, fmt.Sprintf("{%s}", strings.Join(subIDs, ","))).Scan(&completedCount)
		if err == nil && completedCount == jobCount {
			allCompleted = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if !allCompleted {
		t.Fatalf("timed out waiting for all %d jobs to reach SUCCESS", jobCount)
	}

	// Verify all 6 jobs have outputs and positive metrics
	for i, id := range subIDs {
		var status, stdout string
		var exitCode int
		var execTimeMs, memUsageKb sql.NullInt64
		err := db.QueryRowContext(ctx, `
			SELECT status, stdout, exit_code, execution_time_ms, memory_usage_kb
			FROM submissions WHERE id = $1;
		`, id).Scan(&status, &stdout, &exitCode, &execTimeMs, &memUsageKb)
		if err != nil {
			t.Fatalf("failed to inspect submission %s: %v", id, err)
		}
		if status != "SUCCESS" {
			t.Errorf("submission %d (%s) expected SUCCESS, got %s", i, id, status)
		}
		if !strings.Contains(stdout, fmt.Sprintf("done-%d", i)) {
			t.Errorf("submission %d expected stdout 'done-%d', got %q", i, i, stdout)
		}
		if exitCode != 0 {
			t.Errorf("submission %d exit code expected 0, got %d", i, exitCode)
		}
		if !execTimeMs.Valid || execTimeMs.Int64 <= 0 {
			t.Errorf("submission %d execution_time_ms missing or non-positive: %v", i, execTimeMs)
		}
		if !memUsageKb.Valid || memUsageKb.Int64 <= 0 {
			t.Errorf("submission %d memory_usage_kb missing or non-positive: %v", i, memUsageKb)
		}
	}

	// Verify PEL is clean (zero pending messages)
	pending, err := rdb.XPending(ctx, testStream, testGroup).Result()
	if err != nil {
		t.Fatalf("XPending check failed: %v", err)
	}
	if pending.Count != 0 {
		t.Errorf("expected 0 pending messages in PEL after all completions, got %d", pending.Count)
	}
}

// 2. MULTIPLE WORKERS LOAD DISTRIBUTION & ATOMIC CLAIMING
// Worker A and Worker B consume from the same group. Verifies distribution and zero duplicate runs.
func TestWorkerPool_LoadDistribution_NoDuplicates(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()
	rdb := getTestRedis(t)
	defer rdb.Close()
	dockerRunner := getTestRunner(t)
	defer dockerRunner.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	userID := ensureTestUser(t, db)
	testStream := fmt.Sprintf("test:pool:stream:%d", time.Now().UnixNano())
	testGroup := "test_pool_group"
	defer rdb.Del(context.Background(), testStream)

	q := queue.NewRedisQueue(rdb, testStream, testGroup)
	if err := q.InitConsumerGroup(ctx); err != nil {
		t.Fatalf("InitConsumerGroup failed: %v", err)
	}

	const jobCount = 8
	subIDs := make([]string, jobCount)
	for i := 0; i < jobCount; i++ {
		code := fmt.Sprintf("print('pool-job-%d')", i)
		err := db.QueryRowContext(ctx, `
			INSERT INTO submissions (user_id, language, code, stdin, status, retry_count, created_at, updated_at)
			VALUES ($1, 'python', $2, '', 'QUEUED', 0, NOW(), NOW())
			RETURNING id;
		`, userID, code).Scan(&subIDs[i])
		if err != nil {
			t.Fatalf("failed to insert submission %d: %v", i, err)
		}
		if err := q.Enqueue(ctx, subIDs[i]); err != nil {
			t.Fatalf("failed to enqueue: %v", err)
		}
	}

	repo := worker.NewPostgresSubmissionRepository(db)

	consumerA := worker.NewRedisStreamConsumer(rdb, testStream, testGroup, "worker-pool-A")
	publisherA := worker.NewRedisEventPublisher(rdb)
	cfgA := worker.WorkerConfig{
		StreamName:       testStream,
		ConsumerGroup:    testGroup,
		ConsumerName:     "worker-pool-A",
		ConcurrencyLimit: 2,
		PollBatchSize:    4,
		PollBlockTimeout: 200 * time.Millisecond,
		ShutdownTimeout:  5 * time.Second,
	}
	workerA := worker.NewWorker(cfgA, repo, dockerRunner, consumerA, publisherA)

	consumerB := worker.NewRedisStreamConsumer(rdb, testStream, testGroup, "worker-pool-B")
	publisherB := worker.NewRedisEventPublisher(rdb)
	cfgB := worker.WorkerConfig{
		StreamName:       testStream,
		ConsumerGroup:    testGroup,
		ConsumerName:     "worker-pool-B",
		ConcurrencyLimit: 2,
		PollBatchSize:    4,
		PollBlockTimeout: 200 * time.Millisecond,
		ShutdownTimeout:  5 * time.Second,
	}
	workerB := worker.NewWorker(cfgB, repo, dockerRunner, consumerB, publisherB)

	if err := workerA.Start(ctx); err != nil {
		t.Fatalf("failed to start worker A: %v", err)
	}
	defer workerA.Stop()

	if err := workerB.Start(ctx); err != nil {
		t.Fatalf("failed to start worker B: %v", err)
	}
	defer workerB.Stop()

	// Wait for all 8 jobs to complete
	deadline := time.Now().Add(15 * time.Second)
	allDone := false
	for time.Now().Before(deadline) {
		var doneCount int
		err := db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM submissions WHERE id = ANY($1) AND status = 'SUCCESS';
		`, fmt.Sprintf("{%s}", strings.Join(subIDs, ","))).Scan(&doneCount)
		if err == nil && doneCount == jobCount {
			allDone = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if !allDone {
		t.Fatalf("timed out waiting for worker pool to complete all %d jobs", jobCount)
	}

	// Verify all 8 submissions have retry_count = 0 (executed exactly once)
	for i, id := range subIDs {
		var retryCount int
		var status string
		_ = db.QueryRowContext(ctx, "SELECT status, retry_count FROM submissions WHERE id = $1;", id).Scan(&status, &retryCount)
		if status != "SUCCESS" {
			t.Errorf("job %d (%s) status is %s, expected SUCCESS", i, id, status)
		}
		if retryCount != 0 {
			t.Errorf("job %d (%s) was retried %d times, expected exactly 0 retries", i, id, retryCount)
		}
	}

	// Verify PEL is clean
	pending, err := rdb.XPending(ctx, testStream, testGroup).Result()
	if err != nil {
		t.Fatalf("XPending check failed: %v", err)
	}
	if pending.Count != 0 {
		t.Errorf("expected 0 pending messages in PEL, got %d", pending.Count)
	}
}

// 3. DUPLICATE DELIVERY IDEMPOTENCY
// Injects multiple duplicate messages for the same submission ID; asserts only one execution occurs.
func TestWorker_DuplicateDelivery_IdempotentClaim(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()
	rdb := getTestRedis(t)
	defer rdb.Close()
	dockerRunner := getTestRunner(t)
	defer dockerRunner.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	userID := ensureTestUser(t, db)
	testStream := fmt.Sprintf("test:dup:stream:%d", time.Now().UnixNano())
	testGroup := "test_dup_group"
	defer rdb.Del(context.Background(), testStream)

	q := queue.NewRedisQueue(rdb, testStream, testGroup)
	if err := q.InitConsumerGroup(ctx); err != nil {
		t.Fatalf("InitConsumerGroup failed: %v", err)
	}

	var subID string
	err := db.QueryRowContext(ctx, `
		INSERT INTO submissions (user_id, language, code, stdin, status, retry_count, created_at, updated_at)
		VALUES ($1, 'python', 'print("dedup")', '', 'QUEUED', 0, NOW(), NOW())
		RETURNING id;
	`, userID).Scan(&subID)
	if err != nil {
		t.Fatalf("failed to insert submission: %v", err)
	}

	// Enqueue 3 duplicate messages for the SAME submission
	for i := 0; i < 3; i++ {
		if err := q.Enqueue(ctx, subID); err != nil {
			t.Fatalf("failed to enqueue duplicate %d: %v", i, err)
		}
	}

	repo := worker.NewPostgresSubmissionRepository(db)
	consumer := worker.NewRedisStreamConsumer(rdb, testStream, testGroup, "worker-dup")
	publisher := worker.NewRedisEventPublisher(rdb)

	cfg := worker.WorkerConfig{
		StreamName:       testStream,
		ConsumerGroup:    testGroup,
		ConsumerName:     "worker-dup",
		ConcurrencyLimit: 2,
		PollBatchSize:    5,
		PollBlockTimeout: 200 * time.Millisecond,
		ShutdownTimeout:  5 * time.Second,
		ReaperInterval:   100 * time.Millisecond,
		ReaperMinIdle:    200 * time.Millisecond,
		ReaperBatchSize:  10,
	}

	w := worker.NewWorker(cfg, repo, dockerRunner, consumer, publisher)
	if err := w.Start(ctx); err != nil {
		t.Fatalf("failed to start worker: %v", err)
	}
	defer w.Stop()

	// Wait for processing
	deadline := time.Now().Add(8 * time.Second)
	var finalStatus string
	for time.Now().Before(deadline) {
		_ = db.QueryRowContext(ctx, "SELECT status FROM submissions WHERE id = $1;", subID).Scan(&finalStatus)
		if finalStatus == "SUCCESS" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if finalStatus != "SUCCESS" {
		t.Fatalf("expected status SUCCESS, got %s", finalStatus)
	}

	// Verify PEL is clean (all duplicates safely processed and acknowledged by worker and its reaper)
	deadline = time.Now().Add(5 * time.Second)
	var pendingCount int64 = -1
	for time.Now().Before(deadline) {
		pending, err := rdb.XPending(ctx, testStream, testGroup).Result()
		if err == nil {
			pendingCount = pending.Count
			if pendingCount == 0 {
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
	}

	if pendingCount != 0 {
		t.Errorf("expected 0 pending messages in PEL after duplicate acks, got %d", pendingCount)
	}
}

// 4. CRASH RECOVERY CONCURRENT REAPERS
// Multiple orphaned messages in PEL with two workers running reapers concurrently; asserts no duplicate executions.
func TestWorkerCrashRecovery_ConcurrentReapers(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()
	rdb := getTestRedis(t)
	defer rdb.Close()
	dockerRunner := getTestRunner(t)
	defer dockerRunner.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	userID := ensureTestUser(t, db)
	testStream := fmt.Sprintf("test:concreap:stream:%d", time.Now().UnixNano())
	testGroup := "test_concreap_group"
	defer rdb.Del(context.Background(), testStream)

	q := queue.NewRedisQueue(rdb, testStream, testGroup)
	if err := q.InitConsumerGroup(ctx); err != nil {
		t.Fatalf("InitConsumerGroup failed: %v", err)
	}

	const orphanCount = 4
	subIDs := make([]string, orphanCount)
	for i := 0; i < orphanCount; i++ {
		code := fmt.Sprintf("print('rescued-%d')", i)
		err := db.QueryRowContext(ctx, `
			INSERT INTO submissions (user_id, language, code, stdin, status, retry_count, created_at, updated_at)
			VALUES ($1, 'python', $2, '', 'PROCESSING', 0, NOW(), NOW())
			RETURNING id;
		`, userID, code).Scan(&subIDs[i])
		if err != nil {
			t.Fatalf("failed to insert orphan %d: %v", i, err)
		}
		if err := q.Enqueue(ctx, subIDs[i]); err != nil {
			t.Fatalf("failed to enqueue: %v", err)
		}
	}

	// Simulate crashed worker reading all 4 messages
	crashedConsumer := "crashed-worker-batch"
	readRes, err := rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    testGroup,
		Consumer: crashedConsumer,
		Streams:  []string{testStream, ">"},
		Count:    orphanCount,
		Block:    1 * time.Second,
	}).Result()
	if err != nil || len(readRes) == 0 || len(readRes[0].Messages) != orphanCount {
		t.Fatalf("failed to simulate crashed worker batch read: %v", err)
	}

	// Wait past idle threshold (60ms)
	time.Sleep(60 * time.Millisecond)

	repo := worker.NewPostgresSubmissionRepository(db)

	// Start two workers with active reapers
	consumer1 := worker.NewRedisStreamConsumer(rdb, testStream, testGroup, "reaper-1")
	publisher1 := worker.NewRedisEventPublisher(rdb)
	cfg1 := worker.WorkerConfig{
		StreamName:       testStream,
		ConsumerGroup:    testGroup,
		ConsumerName:     "reaper-1",
		ConcurrencyLimit: 2,
		ReaperInterval:   50 * time.Millisecond,
		ReaperMinIdle:    40 * time.Millisecond,
		ReaperBatchSize:  5,
	}
	w1 := worker.NewWorker(cfg1, repo, dockerRunner, consumer1, publisher1)

	consumer2 := worker.NewRedisStreamConsumer(rdb, testStream, testGroup, "reaper-2")
	publisher2 := worker.NewRedisEventPublisher(rdb)
	cfg2 := worker.WorkerConfig{
		StreamName:       testStream,
		ConsumerGroup:    testGroup,
		ConsumerName:     "reaper-2",
		ConcurrencyLimit: 2,
		ReaperInterval:   50 * time.Millisecond,
		ReaperMinIdle:    40 * time.Millisecond,
		ReaperBatchSize:  5,
	}
	w2 := worker.NewWorker(cfg2, repo, dockerRunner, consumer2, publisher2)

	if err := w1.Start(ctx); err != nil {
		t.Fatalf("failed to start w1: %v", err)
	}
	defer w1.Stop()

	if err := w2.Start(ctx); err != nil {
		t.Fatalf("failed to start w2: %v", err)
	}
	defer w2.Stop()

	// Wait for all 4 orphans to be reclaimed and reach SUCCESS
	deadline := time.Now().Add(12 * time.Second)
	allRescued := false
	for time.Now().Before(deadline) {
		var successCount int
		err := db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM submissions WHERE id = ANY($1) AND status = 'SUCCESS' AND retry_count = 1;
		`, fmt.Sprintf("{%s}", strings.Join(subIDs, ","))).Scan(&successCount)
		if err == nil && successCount == orphanCount {
			allRescued = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if !allRescued {
		t.Fatalf("timed out waiting for all 4 orphans to be rescued")
	}

	// Verify PEL is clean
	pending, err := rdb.XPending(ctx, testStream, testGroup).Result()
	if err != nil {
		t.Fatalf("XPending check failed: %v", err)
	}
	if pending.Count != 0 {
		t.Errorf("expected 0 pending messages in PEL after concurrent reaper rescue, got %d", pending.Count)
	}
}

// 5. NORMAL USER-CODE FAILURES DO NOT CONSUME RETRY
// Tests Compilation Error, Runtime Error, TLE, MLE. Asserts retry_count strictly remains 0 for all 4.
func TestWorker_NormalUserFailuresDoNotConsumeRetry(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()
	rdb := getTestRedis(t)
	defer rdb.Close()
	dockerRunner := getTestRunner(t)
	defer dockerRunner.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()

	userID := ensureTestUser(t, db)
	testStream := fmt.Sprintf("test:failures:stream:%d", time.Now().UnixNano())
	testGroup := "test_failures_group"
	defer rdb.Del(context.Background(), testStream)

	q := queue.NewRedisQueue(rdb, testStream, testGroup)
	if err := q.InitConsumerGroup(ctx); err != nil {
		t.Fatalf("InitConsumerGroup failed: %v", err)
	}

	testCases := []struct {
		name           string
		language       string
		code           string
		expectedStatus string
	}{
		{
			name:           "CompilationError",
			language:       "go",
			code:           "package main\nfunc main() { this is invalid syntax }",
			expectedStatus: "COMPILATION_ERROR",
		},
		{
			name:           "RuntimeError",
			language:       "python",
			code:           "x = 1 / 0",
			expectedStatus: "RUNTIME_ERROR",
		},
		{
			name:           "TimeLimitExceeded",
			language:       "python",
			code:           "while True: pass",
			expectedStatus: "TIME_LIMIT_EXCEEDED",
		},
		{
			name:           "MemoryLimitExceeded",
			language:       "python",
			code:           "x = 'a' * (300 * 1024 * 1024)",
			expectedStatus: "MEMORY_LIMIT_EXCEEDED",
		},
	}

	subIDs := make([]string, len(testCases))
	for i, tc := range testCases {
		err := db.QueryRowContext(ctx, `
			INSERT INTO submissions (user_id, language, code, stdin, status, retry_count, created_at, updated_at)
			VALUES ($1, $2, $3, '', 'QUEUED', 0, NOW(), NOW())
			RETURNING id;
		`, userID, tc.language, tc.code).Scan(&subIDs[i])
		if err != nil {
			t.Fatalf("failed to insert submission %s: %v", tc.name, err)
		}
		if err := q.Enqueue(ctx, subIDs[i]); err != nil {
			t.Fatalf("failed to enqueue submission %s: %v", tc.name, err)
		}
	}

	repo := worker.NewPostgresSubmissionRepository(db)
	consumer := worker.NewRedisStreamConsumer(rdb, testStream, testGroup, "worker-failures")
	publisher := worker.NewRedisEventPublisher(rdb)

	cfg := worker.WorkerConfig{
		StreamName:       testStream,
		ConsumerGroup:    testGroup,
		ConsumerName:     "worker-failures",
		ConcurrencyLimit: 4,
		PollBatchSize:    4,
		PollBlockTimeout: 200 * time.Millisecond,
		ShutdownTimeout:  10 * time.Second,
	}

	w := worker.NewWorker(cfg, repo, dockerRunner, consumer, publisher)
	if err := w.Start(ctx); err != nil {
		t.Fatalf("failed to start worker: %v", err)
	}
	defer w.Stop()

	// Wait for all 4 submissions to reach terminal status
	deadline := time.Now().Add(25 * time.Second)
	allDone := false
	for time.Now().Before(deadline) {
		var nonTerminalCount int
		err := db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM submissions WHERE id = ANY($1) AND status IN ('QUEUED', 'PROCESSING');
		`, fmt.Sprintf("{%s}", strings.Join(subIDs, ","))).Scan(&nonTerminalCount)
		if err == nil && nonTerminalCount == 0 {
			allDone = true
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	if !allDone {
		t.Fatalf("timed out waiting for execution failure test cases to reach terminal state")
	}

	// Verify each status matches expected and retry_count strictly remains 0!
	for i, tc := range testCases {
		var status string
		var retryCount int
		err := db.QueryRowContext(ctx, `
			SELECT status, retry_count FROM submissions WHERE id = $1;
		`, subIDs[i]).Scan(&status, &retryCount)
		if err != nil {
			t.Fatalf("failed to query status for %s: %v", tc.name, err)
		}

		if status != tc.expectedStatus {
			t.Errorf("test case %s expected status %s, got %s", tc.name, tc.expectedStatus, status)
		}
		if retryCount != 0 {
			t.Errorf("test case %s consumed a retry! retry_count was %d, expected 0", tc.name, retryCount)
		}
	}
}

// 6. GRACEFUL SHUTDOWN
// Verifies worker shutdown drains in-flight container executions cleanly without leaving orphan Docker containers.
func TestWorker_GracefulShutdown_DrainsInFlight(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()
	rdb := getTestRedis(t)
	defer rdb.Close()
	dockerRunner := getTestRunner(t)
	defer dockerRunner.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	userID := ensureTestUser(t, db)
	testStream := fmt.Sprintf("test:shutdown:stream:%d", time.Now().UnixNano())
	testGroup := "test_shutdown_group"
	defer rdb.Del(context.Background(), testStream)

	q := queue.NewRedisQueue(rdb, testStream, testGroup)
	if err := q.InitConsumerGroup(ctx); err != nil {
		t.Fatalf("InitConsumerGroup failed: %v", err)
	}

	var subID string
	err := db.QueryRowContext(ctx, `
		INSERT INTO submissions (user_id, language, code, stdin, status, retry_count, created_at, updated_at)
		VALUES ($1, 'python', 'import time; time.sleep(0.8); print("shutdown-safe")', '', 'QUEUED', 0, NOW(), NOW())
		RETURNING id;
	`, userID).Scan(&subID)
	if err != nil {
		t.Fatalf("failed to insert submission: %v", err)
	}
	if err := q.Enqueue(ctx, subID); err != nil {
		t.Fatalf("failed to enqueue: %v", err)
	}

	repo := worker.NewPostgresSubmissionRepository(db)
	consumer := worker.NewRedisStreamConsumer(rdb, testStream, testGroup, "worker-shutdown")
	publisher := worker.NewRedisEventPublisher(rdb)

	cfg := worker.WorkerConfig{
		StreamName:       testStream,
		ConsumerGroup:    testGroup,
		ConsumerName:     "worker-shutdown",
		ConcurrencyLimit: 2,
		PollBatchSize:    2,
		PollBlockTimeout: 100 * time.Millisecond,
		ShutdownTimeout:  5 * time.Second,
	}

	w := worker.NewWorker(cfg, repo, dockerRunner, consumer, publisher)
	if err := w.Start(ctx); err != nil {
		t.Fatalf("failed to start worker: %v", err)
	}

	// Wait until job enters PROCESSING
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var status string
		_ = db.QueryRowContext(ctx, "SELECT status FROM submissions WHERE id = $1;", subID).Scan(&status)
		if status == "PROCESSING" {
			break
		}
		time.Sleep(30 * time.Millisecond)
	}

	// Trigger shutdown while job is running in container
	shutdownStart := time.Now()
	if err := w.StopWithTimeout(5 * time.Second); err != nil {
		t.Fatalf("StopWithTimeout failed: %v", err)
	}
	shutdownDuration := time.Since(shutdownStart)

	// Verify job was allowed to complete cleanly
	var finalStatus, stdout string
	err = db.QueryRowContext(ctx, "SELECT status, stdout FROM submissions WHERE id = $1;", subID).Scan(&finalStatus, &stdout)
	if err != nil {
		t.Fatalf("failed to query final submission state: %v", err)
	}

	if finalStatus != "SUCCESS" {
		t.Errorf("expected final status SUCCESS after graceful shutdown, got %s", finalStatus)
	}
	if !strings.Contains(stdout, "shutdown-safe") {
		t.Errorf("expected stdout 'shutdown-safe', got %q", stdout)
	}

	t.Logf("shutdown drained execution cleanly in %v", shutdownDuration)
}
