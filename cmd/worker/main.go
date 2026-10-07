package main

import (
	"context"
	"database/sql"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/SatyamSingh-16/code_executor/internal/queue"
	"github.com/SatyamSingh-16/code_executor/internal/runner"
	"github.com/SatyamSingh-16/code_executor/internal/worker"
	"github.com/docker/docker/client"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
)

func main() {
	log.Println("[worker] initializing background code execution worker...")

	workerCfg := worker.LoadWorkerConfigFromEnv()
	redisCfg := queue.LoadRedisConfigFromEnv()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://satyamsingh2730@localhost:5432/code_execution_test_db?sslmode=disable"
	}

	// 1. Connect to PostgreSQL
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("[worker] failed to connect to database: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("[worker] database ping failed: %v", err)
	}

	// 2. Connect to Redis
	rdb := redis.NewClient(&redis.Options{
		Addr:     redisCfg.Addr,
		Password: redisCfg.Password,
		DB:       redisCfg.DB,
	})
	defer rdb.Close()

	if err := rdb.Ping(context.Background()).Err(); err != nil {
		log.Fatalf("[worker] redis ping failed: %v", err)
	}

	// 3. Ensure Consumer Group exists idempotently
	streamQueue := queue.NewRedisQueue(rdb, workerCfg.StreamName, workerCfg.ConsumerGroup)
	if err := streamQueue.InitConsumerGroup(context.Background()); err != nil {
		log.Fatalf("[worker] failed to init consumer group %q on stream %q: %v",
			workerCfg.ConsumerGroup, workerCfg.StreamName, err)
	}

	// 4. Initialize Docker client and DockerRunner
	dockerCli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Fatalf("[worker] failed to initialize docker client: %v", err)
	}
	defer dockerCli.Close()

	dockerRunner := runner.NewDockerRunner(dockerCli)
	defer dockerRunner.Close()

	// 5. Construct Worker components
	repo := worker.NewPostgresSubmissionRepository(db)
	consumer := worker.NewRedisStreamConsumer(rdb, workerCfg.StreamName, workerCfg.ConsumerGroup, workerCfg.ConsumerName)
	publisher := worker.NewRedisEventPublisher(rdb)

	w := worker.NewWorker(workerCfg, repo, dockerRunner, consumer, publisher)

	// 6. Setup signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	if err := w.Start(ctx); err != nil {
		log.Fatalf("[worker] failed to start worker: %v", err)
	}

	log.Printf("[worker %s] started successfully (stream=%s, group=%s, concurrency=%d)",
		workerCfg.ConsumerName, workerCfg.StreamName, workerCfg.ConsumerGroup, workerCfg.ConcurrencyLimit)

	sig := <-sigCh
	log.Printf("[worker %s] received shutdown signal %v, draining in-flight executions...", workerCfg.ConsumerName, sig)

	cancel()
	_ = w.StopWithTimeout(workerCfg.ShutdownTimeout)
	log.Printf("[worker %s] shutdown completed cleanly", workerCfg.ConsumerName)
}
