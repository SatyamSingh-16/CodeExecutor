//go:build integration

package worker_test

import (
	"context"
	"database/sql"
	"encoding/json"
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

func getIntegrationDB(t *testing.T) *sql.DB {
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
		t.Skip("skipping integration test: PostgreSQL test DB unavailable")
	}

	migrator := database.NewMigrator(db)
	if err := migrator.Up(context.Background()); err != nil {
		t.Fatalf("failed to apply migrations: %v", err)
	}

	return db
}

func getIntegrationRunner(t *testing.T) *runner.DockerRunner {
	t.Helper()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		t.Skipf("skipping integration test: Docker client unavailable: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := cli.Ping(ctx); err != nil {
		_ = cli.Close()
		t.Skipf("skipping integration test: Docker daemon unreachable: %v", err)
	}
	return runner.NewDockerRunner(cli)
}

func ensureIntegrationUser(t *testing.T, db *sql.DB) string {
	t.Helper()
	var userID string
	err := db.QueryRow(`
		INSERT INTO users (email, password_hash)
		VALUES ('worker_int@example.com', 'hashed_pw')
		ON CONFLICT (email) DO UPDATE SET email = EXCLUDED.email
		RETURNING id;
	`).Scan(&userID)
	if err != nil {
		t.Fatalf("failed to ensure test user: %v", err)
	}
	return userID
}

func TestWorker_LiveDockerPostgresRedisPipeline(t *testing.T) {
	db := getIntegrationDB(t)
	defer db.Close()
	rdb := getIntegrationRedis(t)
	defer rdb.Close()
	dockerRunner := getIntegrationRunner(t)
	defer dockerRunner.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	userID := ensureIntegrationUser(t, db)
	testStream := fmt.Sprintf("test:worker:stream:%d", time.Now().UnixNano())
	testGroup := "test_worker_group"
	defer rdb.Del(context.Background(), testStream)

	// 1. Initialize consumer group on stream
	q := queue.NewRedisQueue(rdb, testStream, testGroup)
	if err := q.InitConsumerGroup(ctx); err != nil {
		t.Fatalf("InitConsumerGroup failed: %v", err)
	}

	// 2. Insert Python submission with status QUEUED into PostgreSQL
	var subID string
	err := db.QueryRowContext(ctx, `
		INSERT INTO submissions (user_id, language, code, stdin, status, created_at, updated_at)
		VALUES ($1, 'python', 'print("Worker Pipeline OK")', '', 'QUEUED', NOW(), NOW())
		RETURNING id;
	`, userID).Scan(&subID)
	if err != nil {
		t.Fatalf("failed to insert submission: %v", err)
	}

	// 3. Subscribe to Redis Pub/Sub events for this submission
	eventChannel := fmt.Sprintf("submissions:events:%s", subID)
	pubsub := rdb.Subscribe(ctx, eventChannel)
	defer pubsub.Close()

	// Wait for subscription confirmation
	_, err = pubsub.Receive(ctx)
	if err != nil {
		t.Fatalf("failed to subscribe to redis events: %v", err)
	}

	// 4. Enqueue submission to Redis Stream
	if err := q.Enqueue(ctx, subID); err != nil {
		t.Fatalf("failed to enqueue submission: %v", err)
	}

	// 5. Construct and Start Worker
	repo := worker.NewPostgresSubmissionRepository(db)
	consumer := worker.NewRedisStreamConsumer(rdb, testStream, testGroup, "test-worker-1")
	publisher := worker.NewRedisEventPublisher(rdb)

	cfg := worker.WorkerConfig{
		StreamName:       testStream,
		ConsumerGroup:    testGroup,
		ConsumerName:     "test-worker-1",
		ConcurrencyLimit: 2,
		PollBatchSize:    5,
		PollBlockTimeout: 500 * time.Millisecond,
		ShutdownTimeout:  5 * time.Second,
	}

	w := worker.NewWorker(cfg, repo, dockerRunner, consumer, publisher)
	if err := w.Start(ctx); err != nil {
		t.Fatalf("failed to start worker: %v", err)
	}
	defer w.Stop()

	// 6. Listen for Pub/Sub events: expect PROCESSING and SUCCESS
	var receivedStatuses []string
	ch := pubsub.Channel()

waitLoop:
	for {
		select {
		case msg := <-ch:
			var evt worker.StatusEvent
			if err := json.Unmarshal([]byte(msg.Payload), &evt); err == nil {
				receivedStatuses = append(receivedStatuses, evt.Status)
				if evt.Status == "SUCCESS" {
					break waitLoop
				}
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("timed out waiting for execution completion via Pub/Sub. Received: %v", receivedStatuses)
		}
	}

	// 7. Verify PostgreSQL row contains persisted result and metrics
	var status, stdout string
	var exitCode int
	var execTimeMs, memUsageKb sql.NullInt64

	err = db.QueryRowContext(ctx, `
		SELECT status, stdout, exit_code, execution_time_ms, memory_usage_kb
		FROM submissions
		WHERE id = $1;
	`, subID).Scan(&status, &stdout, &exitCode, &execTimeMs, &memUsageKb)
	if err != nil {
		t.Fatalf("failed to query final submission state: %v", err)
	}

	if status != "SUCCESS" {
		t.Errorf("expected status 'SUCCESS', got %s", status)
	}
	if strings.TrimSpace(stdout) != "Worker Pipeline OK" {
		t.Errorf("expected stdout 'Worker Pipeline OK', got %q", stdout)
	}
	if exitCode != 0 {
		t.Errorf("expected exitCode 0, got %d", exitCode)
	}
	if !execTimeMs.Valid || execTimeMs.Int64 <= 0 {
		t.Errorf("expected positive execution_time_ms, got %v", execTimeMs)
	}
	if !memUsageKb.Valid || memUsageKb.Int64 <= 0 {
		t.Errorf("expected positive memory_usage_kb, got %v", memUsageKb)
	}

	// 8. Verify Redis Stream message was acknowledged (Pending count = 0)
	pending, err := rdb.XPending(ctx, testStream, testGroup).Result()
	if err != nil {
		t.Fatalf("XPending check failed: %v", err)
	}
	if pending.Count != 0 {
		t.Errorf("expected 0 pending messages after XACK, got %d", pending.Count)
	}
}
