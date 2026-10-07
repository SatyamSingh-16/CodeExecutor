package queue_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/SatyamSingh-16/code_executor/internal/database"
	"github.com/SatyamSingh-16/code_executor/internal/queue"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
)

func getRealRedis(t *testing.T) *redis.Client {
	t.Helper()
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}
	rdb := redis.NewClient(&redis.Options{
		Addr: addr,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("skipping live Redis test: Redis unavailable at %s: %v", addr, err)
	}
	return rdb
}

func getRealDB(t *testing.T) *sql.DB {
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
		t.Skip("skipping live PostgreSQL test: test DB unavailable")
	}

	// Apply migrations to ensure latest schema and indexes exist
	migrator := database.NewMigrator(db)
	if err := migrator.Up(context.Background()); err != nil {
		t.Fatalf("failed to apply migrations up for integration test: %v", err)
	}

	return db
}

func ensureTestUser(t *testing.T, db *sql.DB) string {
	t.Helper()
	var userID string
	err := db.QueryRow(`
		INSERT INTO users (email, password_hash)
		VALUES ('queue_test@example.com', 'hashed_pw')
		ON CONFLICT (email) DO UPDATE SET email = EXCLUDED.email
		RETURNING id;
	`).Scan(&userID)
	if err != nil {
		t.Fatalf("failed to ensure test user: %v", err)
	}
	return userID
}

func TestRedisQueue_LiveStreamAndConsumerGroup(t *testing.T) {
	rdb := getRealRedis(t)
	defer rdb.Close()

	ctx := context.Background()
	testStream := fmt.Sprintf("test:submissions:%d", time.Now().UnixNano())
	testGroup := "test_workers_group"

	// Cleanup test stream upon completion
	defer rdb.Del(ctx, testStream)

	q := queue.NewRedisQueue(rdb, testStream, testGroup)

	// 1. Consumer group creation on non-existent stream (MKSTREAM creates it)
	if err := q.InitConsumerGroup(ctx); err != nil {
		t.Fatalf("InitConsumerGroup failed: %v", err)
	}

	// 2. Repeated initialization must be idempotent (BUSYGROUP ignored)
	if err := q.InitConsumerGroup(ctx); err != nil {
		t.Fatalf("repeated InitConsumerGroup failed: %v", err)
	}

	// 3. Enqueue submission
	testSubID := "sub-real-live-1234"
	if err := q.Enqueue(ctx, testSubID); err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	// 4. Verify message in Redis Stream using XRevRange
	msgs, err := rdb.XRevRange(ctx, testStream, "+", "-").Result()
	if err != nil {
		t.Fatalf("XRevRange failed: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message in stream, got %d", len(msgs))
	}
	if msgs[0].Values[queue.FieldSubmissionID] != testSubID {
		t.Errorf("expected stream value %s, got %v", testSubID, msgs[0].Values[queue.FieldSubmissionID])
	}

	// 5. Duplicate Enqueue succeeds (at-least-once delivery)
	if err := q.Enqueue(ctx, testSubID); err != nil {
		t.Fatalf("duplicate Enqueue failed: %v", err)
	}
	msgsAfterDup, _ := rdb.XRevRange(ctx, testStream, "+", "-").Result()
	if len(msgsAfterDup) != 2 {
		t.Fatalf("expected 2 messages after duplicate enqueue, got %d", len(msgsAfterDup))
	}
}

type failingQueue struct {
	failErr error
}

func (f *failingQueue) Enqueue(ctx context.Context, submissionID string) error {
	return f.failErr
}

func (f *failingQueue) InitConsumerGroup(ctx context.Context) error {
	return nil
}

func (f *failingQueue) Close() error {
	return nil
}

func TestDualWrite_RedisFailurePreservesPostgresState(t *testing.T) {
	db := getRealDB(t)
	defer db.Close()

	ctx := context.Background()
	userID := ensureTestUser(t, db)

	// Simulate Redis outage during enqueue
	mockFailingQueue := &failingQueue{
		failErr: errors.New("redis connection refused: port 6379"),
	}

	record, err := queue.DualWriteSubmit(ctx, db, mockFailingQueue, queue.SubmissionInsertInput{
		UserID:   userID,
		Language: "python",
		Code:     "print('hello world')",
	})

	// Verification C1: Redis enqueue failure is returned to caller
	if err == nil {
		t.Fatal("expected error from DualWriteSubmit when Redis fails, got nil")
	}
	if record == nil {
		t.Fatal("expected record to be returned with created ID even when Redis enqueue fails")
	}

	// Verification C2: PostgreSQL submission MUST remain QUEUED and NOT SYSTEM_ERROR
	var status string
	err = db.QueryRowContext(ctx, "SELECT status FROM submissions WHERE id = $1;", record.ID).Scan(&status)
	if err != nil {
		t.Fatalf("failed to query submission after dual-write failure: %v", err)
	}

	if status != "QUEUED" {
		t.Fatalf("expected status 'QUEUED', but found %q. Database state was improperly mutated!", status)
	}
}

func TestRecoverySweeper_LiveReconciliation(t *testing.T) {
	db := getRealDB(t)
	defer db.Close()
	rdb := getRealRedis(t)
	defer rdb.Close()

	ctx := context.Background()
	userID := ensureTestUser(t, db)

	testStream := fmt.Sprintf("test:recovery:stream:%d", time.Now().UnixNano())
	testGroup := "test_recovery_group"
	defer rdb.Del(ctx, testStream)

	q := queue.NewRedisQueue(rdb, testStream, testGroup)
	if err := q.InitConsumerGroup(ctx); err != nil {
		t.Fatalf("failed to init consumer group: %v", err)
	}

	now := time.Now()
	staleCreatedAt := now.Add(-60 * time.Second) // 60 seconds old -> eligible
	youngCreatedAt := now.Add(-5 * time.Second)  // 5 seconds old -> ineligible

	// 1. Insert stale submission directly into PostgreSQL with status = 'QUEUED'
	var staleID string
	err := db.QueryRowContext(ctx, `
		INSERT INTO submissions (user_id, language, code, status, created_at, updated_at)
		VALUES ($1, 'python', 'print("stale")', 'QUEUED', $2, $2)
		RETURNING id;
	`, userID, staleCreatedAt).Scan(&staleID)
	if err != nil {
		t.Fatalf("failed to insert stale submission: %v", err)
	}

	// 2. Insert young submission into PostgreSQL with status = 'QUEUED'
	var youngID string
	err = db.QueryRowContext(ctx, `
		INSERT INTO submissions (user_id, language, code, status, created_at, updated_at)
		VALUES ($1, 'python', 'print("young")', 'QUEUED', $2, $2)
		RETURNING id;
	`, userID, youngCreatedAt).Scan(&youngID)
	if err != nil {
		t.Fatalf("failed to insert young submission: %v", err)
	}

	// Configure sweeper with 30s threshold
	cfg := queue.SweeperConfig{
		Interval:             10 * time.Second,
		EligibilityThreshold: 30 * time.Second,
		BatchSize:            100,
	}

	sweeper := queue.NewRecoverySweeperFromDB(db, q, cfg)
	sweeper.SetNowFunc(func() time.Time { return now })

	// Run sweep pass
	res, err := sweeper.SweepOnce(ctx)
	if err != nil {
		t.Fatalf("SweepOnce failed: %v", err)
	}

	if res.EnqueuedCount < 1 {
		t.Errorf("expected at least 1 enqueued submission, got %d", res.EnqueuedCount)
	}

	// Check Redis stream for the stale ID
	msgs, err := rdb.XRevRange(ctx, testStream, "+", "-").Result()
	if err != nil {
		t.Fatalf("XRevRange failed: %v", err)
	}

	foundStale := false
	foundYoung := false
	for _, m := range msgs {
		subID := fmt.Sprintf("%v", m.Values[queue.FieldSubmissionID])
		if subID == staleID {
			foundStale = true
		}
		if subID == youngID {
			foundYoung = true
		}
	}

	if !foundStale {
		t.Errorf("stale submission %s was not enqueued into Redis Stream by sweeper", staleID)
	}
	if foundYoung {
		t.Errorf("young submission %s (< 30s old) should NOT have been swept into Redis Stream", youngID)
	}

	// Verify PostgreSQL record status is still QUEUED (not modified to SYSTEM_ERROR)
	var finalStaleStatus string
	_ = db.QueryRowContext(ctx, "SELECT status FROM submissions WHERE id = $1;", staleID).Scan(&finalStaleStatus)
	if finalStaleStatus != "QUEUED" {
		t.Errorf("expected PostgreSQL status to remain 'QUEUED', got %s", finalStaleStatus)
	}
}
