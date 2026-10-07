package worker

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/SatyamSingh-16/code_executor/internal/runner"
)

// Worker orchestrates asynchronous code execution by pulling jobs from Redis Streams,
// loading authoritative records from PostgreSQL, running sandboxes via ExecutionRunner,
// persisting terminal results, and publishing status events.
type Worker struct {
	config    WorkerConfig
	repo      SubmissionRepository
	runner    ExecutionRunner
	consumer  StreamConsumer
	publisher EventPublisher

	semaphore chan struct{}
	mu        sync.Mutex
	running   bool
	stopCh    chan struct{}
	doneCh    chan struct{}
	wg        sync.WaitGroup
}

// NewWorker constructs a new Worker instance with bounded concurrency.
func NewWorker(
	cfg WorkerConfig,
	repo SubmissionRepository,
	execRunner ExecutionRunner,
	consumer StreamConsumer,
	publisher EventPublisher,
) *Worker {
	if cfg.ConcurrencyLimit <= 0 {
		cfg.ConcurrencyLimit = 4
	}
	if cfg.PollBatchSize <= 0 {
		cfg.PollBatchSize = 10
	}
	if cfg.PollBlockTimeout <= 0 {
		cfg.PollBlockTimeout = 2000 * time.Millisecond
	}
	if cfg.ShutdownTimeout <= 0 {
		cfg.ShutdownTimeout = 30 * time.Second
	}

	return &Worker{
		config:    cfg,
		repo:      repo,
		runner:    execRunner,
		consumer:  consumer,
		publisher: publisher,
		semaphore: make(chan struct{}, cfg.ConcurrencyLimit),
	}
}

// Start begins the stream consumer loop in a background goroutine.
func (w *Worker) Start(ctx context.Context) error {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return fmt.Errorf("worker is already running")
	}
	w.running = true
	w.stopCh = make(chan struct{})
	w.doneCh = make(chan struct{})
	w.mu.Unlock()

	go w.run(ctx)
	return nil
}

// run executes the main consumer loop until stopped or context is cancelled.
func (w *Worker) run(ctx context.Context) {
	defer close(w.doneCh)

	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stopCh:
			return
		default:
		}

		// Read new messages from the consumer group
		messages, err := w.consumer.ReadMessages(ctx, w.config.PollBatchSize, w.config.PollBlockTimeout)
		if err != nil {
			select {
			case <-ctx.Done():
				return
			case <-w.stopCh:
				return
			default:
				log.Printf("[Worker %s] read stream error: %v", w.config.ConsumerName, err)
				time.Sleep(200 * time.Millisecond)
				continue
			}
		}

		for _, msg := range messages {
			// Acquire concurrency semaphore slot BEFORE claiming submission in DB
			// or spawning execution goroutine. This guarantees that only active execution
			// slots enter the system and PostgreSQL records do not sit in PROCESSING indefinitely.
			select {
			case <-ctx.Done():
				return
			case <-w.stopCh:
				return
			case w.semaphore <- struct{}{}:
			}

			w.wg.Add(1)
			go func(m StreamMessage) {
				defer func() {
					<-w.semaphore
					w.wg.Done()
				}()
				w.processMessage(ctx, m)
			}(msg)
		}
	}
}

// processMessage executes the core claim-execute-persist-ack pipeline for a single message.
func (w *Worker) processMessage(ctx context.Context, msg StreamMessage) {
	if msg.SubmissionID == "" {
		log.Printf("[Worker %s] received message %s with empty submission ID, discarding", w.config.ConsumerName, msg.MessageID)
		_ = w.consumer.AckMessage(context.Background(), msg.MessageID)
		return
	}

	// 1. Load authoritative submission from PostgreSQL
	sub, err := w.repo.GetSubmission(ctx, msg.SubmissionID)
	if err != nil {
		if errors.Is(err, ErrSubmissionNotFound) {
			// Submission does not exist in database (invalid/stale stream message).
			// ACK to remove invalid message from stream PEL and avoid infinite retry.
			log.Printf("[Worker %s] submission %s not found in database, acknowledging invalid message %s",
				w.config.ConsumerName, msg.SubmissionID, msg.MessageID)
			_ = w.consumer.AckMessage(context.Background(), msg.MessageID)
			return
		}

		// Database read error: do NOT acknowledge message so it can be retried
		log.Printf("[Worker %s] database lookup error for submission %s: %v",
			w.config.ConsumerName, msg.SubmissionID, err)
		return
	}

	// 2. Check current status for idempotency / duplicate delivery
	if sub.Status != "QUEUED" {
		if isTerminalStatus(sub.Status) {
			// Already reached terminal state (e.g. duplicate delivery).
			// Safe to acknowledge stale message.
			_ = w.consumer.AckMessage(context.Background(), msg.MessageID)
		}
		return
	}

	// 3. Atomically transition state: QUEUED -> PROCESSING
	claimed, err := w.repo.ClaimSubmission(ctx, msg.SubmissionID)
	if err != nil {
		log.Printf("[Worker %s] error attempting to claim submission %s: %v",
			w.config.ConsumerName, msg.SubmissionID, err)
		return
	}
	if !claimed {
		// Zero rows affected: Another worker claimed it or status transitioned.
		// Do NOT execute.
		curr, getErr := w.repo.GetSubmission(ctx, msg.SubmissionID)
		if getErr == nil && curr != nil && isTerminalStatus(curr.Status) {
			_ = w.consumer.AckMessage(context.Background(), msg.MessageID)
		}
		return
	}

	// 4. Publish PROCESSING state event to Redis Pub/Sub
	if w.publisher != nil {
		if pubErr := w.publisher.PublishStatusEvent(ctx, msg.SubmissionID, "PROCESSING"); pubErr != nil {
			log.Printf("[Worker %s] failed to publish PROCESSING event for %s: %v",
				w.config.ConsumerName, msg.SubmissionID, pubErr)
		}
	}

	// 5. Execute code in isolated container sandbox
	req := runner.ExecutionRequest{
		Language: runner.Language(sub.Language),
		Code:     sub.Code,
		Stdin:    sub.Stdin,
	}

	execResult, execErr := w.runner.Execute(ctx, req)
	if execResult == nil {
		errMsg := "execution failed without result"
		if execErr != nil {
			errMsg = execErr.Error()
		}
		execResult = &runner.ExecutionResult{
			Status: runner.StatusSystemError,
			Stderr: errMsg,
		}
	} else if execErr != nil && execResult.Status == "" {
		execResult.Status = runner.StatusSystemError
		execResult.Stderr = execErr.Error()
	}

	// 6. Persist execution result to PostgreSQL (PostgreSQL is authoritative)
	if err := w.repo.CompleteSubmission(ctx, msg.SubmissionID, execResult); err != nil {
		log.Printf("[Worker %s] failed to persist execution result for submission %s: %v",
			w.config.ConsumerName, msg.SubmissionID, err)
		// CRITICAL: Do NOT ACK message if result persistence failed.
		return
	}

	// 7. Publish terminal event to Redis Pub/Sub
	if w.publisher != nil {
		if pubErr := w.publisher.PublishStatusEvent(ctx, msg.SubmissionID, string(execResult.Status)); pubErr != nil {
			log.Printf("[Worker %s] failed to publish terminal event for %s: %v",
				w.config.ConsumerName, msg.SubmissionID, pubErr)
			// Non-fatal: PostgreSQL persistence is already durable
		}
	}

	// 8. Acknowledge message in Redis Streams (XACK)
	if ackErr := w.consumer.AckMessage(context.Background(), msg.MessageID); ackErr != nil {
		log.Printf("[Worker %s] failed to ACK message %s for submission %s: %v",
			w.config.ConsumerName, msg.MessageID, msg.SubmissionID, ackErr)
	}
}

// Stop initiates graceful shutdown, waiting for active container executions to complete.
func (w *Worker) Stop() {
	_ = w.StopWithTimeout(w.config.ShutdownTimeout)
}

// StopWithTimeout stops message ingestion and waits up to timeout for running executions to finish.
func (w *Worker) StopWithTimeout(timeout time.Duration) error {
	w.mu.Lock()
	if !w.running {
		w.mu.Unlock()
		return nil
	}
	w.running = false
	close(w.stopCh)
	w.mu.Unlock()

	// Wait for consumer poll loop to exit
	<-w.doneCh

	// Wait for active executions with timeout
	done := make(chan struct{})
	go func() {
		w.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("worker shutdown timed out waiting for active executions after %v", timeout)
	}
}

// isTerminalStatus checks if a status represents a final terminal execution state.
func isTerminalStatus(status string) bool {
	switch runner.ExecutionStatus(status) {
	case runner.StatusSuccess,
		runner.StatusCompilationError,
		runner.StatusRuntimeError,
		runner.StatusTimeLimitExceeded,
		runner.StatusMemoryLimitExceeded,
		runner.StatusSystemError:
		return true
	default:
		return false
	}
}
