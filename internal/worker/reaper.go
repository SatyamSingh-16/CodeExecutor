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

// DispatchFunc defines the callback for delivering a reclaimed job to the worker's execution pipeline.
type DispatchFunc func(ctx context.Context, msg StreamMessage)

// Reaper runs a periodic background loop using XAUTOCLAIM to reclaim orphaned messages
// left in the Redis Streams Pending Entries List (PEL) due to worker crashes.
type Reaper struct {
	config       WorkerConfig
	repo         SubmissionRepository
	consumer     StreamConsumer
	publisher    EventPublisher
	dispatchFunc DispatchFunc

	mu      sync.Mutex
	running bool
	stopCh  chan struct{}
	doneCh  chan struct{}
}

// NewReaper constructs a new Reaper instance.
func NewReaper(
	cfg WorkerConfig,
	repo SubmissionRepository,
	consumer StreamConsumer,
	publisher EventPublisher,
	dispatch DispatchFunc,
) *Reaper {
	if cfg.ReaperInterval <= 0 {
		cfg.ReaperInterval = 15 * time.Second
	}
	if cfg.ReaperMinIdle <= 0 {
		cfg.ReaperMinIdle = 30 * time.Second
	}
	if cfg.ReaperBatchSize <= 0 {
		cfg.ReaperBatchSize = 10
	}

	return &Reaper{
		config:       cfg,
		repo:         repo,
		consumer:     consumer,
		publisher:    publisher,
		dispatchFunc: dispatch,
	}
}

// ReapOnce performs a single orphan reclamation pass:
// 1. Calls XAUTOCLAIM on the stream consumer group for messages idle past ReaperMinIdle.
// 2. For each reclaimed message:
//    a. Inspects the corresponding submission in PostgreSQL.
//    b. If already in a terminal state: acknowledges (XACK) the stale message.
//    c. If retry_count >= 1 (retries exhausted):
//       - Updates PostgreSQL to SYSTEM_ERROR with stderr explaining worker crash limit reached.
//       - Emits SYSTEM_ERROR event to Redis Pub/Sub.
//       - Calls XACK to permanently purge toxic message from stream.
//    d. If retry_count < 1:
//       - Atomically claims and increments retry_count in PostgreSQL.
//       - Dispatches to worker's execution pipeline for a single retry attempt.
func (r *Reaper) ReapOnce(ctx context.Context) (ReapResult, error) {
	messages, _, err := r.consumer.AutoClaim(ctx, r.config.ReaperMinIdle, "0-0", r.config.ReaperBatchSize)
	if err != nil {
		return ReapResult{}, fmt.Errorf("reaper autoclaim error on stream %q: %w", r.config.StreamName, err)
	}

	res := ReapResult{
		ClaimedCount: len(messages),
	}

	for _, msg := range messages {
		if msg.SubmissionID == "" {
			_ = r.consumer.AckMessage(context.Background(), msg.MessageID)
			continue
		}

		sub, err := r.repo.GetSubmission(ctx, msg.SubmissionID)
		if err != nil {
			if errors.Is(err, ErrSubmissionNotFound) {
				// Record does not exist in DB: ACK and remove invalid poison entry from stream
				log.Printf("[Reaper %s] submission %s not found in DB, purging message %s",
					r.config.ConsumerName, msg.SubmissionID, msg.MessageID)
				_ = r.consumer.AckMessage(context.Background(), msg.MessageID)
				continue
			}
			res.Errors = append(res.Errors, fmt.Errorf("failed to lookup submission %s: %w", msg.SubmissionID, err))
			continue
		}

		// 1. If submission already completed in PostgreSQL (crashed after DB write but before XACK):
		if isTerminalStatus(sub.Status) {
			if ackErr := r.consumer.AckMessage(context.Background(), msg.MessageID); ackErr != nil {
				res.Errors = append(res.Errors, fmt.Errorf("failed to ACK terminal submission %s: %w", msg.SubmissionID, ackErr))
			} else {
				res.TerminalAckCount++
			}
			continue
		}

		// 2. Check retry count: single retry policy (max 1 retry)
		if sub.RetryCount >= 1 {
			// Toxic job: worker crashed during or after retry
			log.Printf("[Reaper %s] submission %s exceeded retry limit (retry_count=%d), failing with SYSTEM_ERROR",
				r.config.ConsumerName, msg.SubmissionID, sub.RetryCount)

			errMsg := "Execution aborted due to worker node failure (max retries exceeded)"
			if err := r.repo.FailSubmissionMaxRetries(ctx, msg.SubmissionID, errMsg); err != nil {
				res.Errors = append(res.Errors, fmt.Errorf("failed to transition toxic submission %s to SYSTEM_ERROR: %w", msg.SubmissionID, err))
				continue
			}

			// Broadcast SYSTEM_ERROR event to Redis Pub/Sub
			if r.publisher != nil {
				if pubErr := r.publisher.PublishStatusEvent(ctx, msg.SubmissionID, string(runner.StatusSystemError)); pubErr != nil {
					log.Printf("[Reaper %s] failed to publish SYSTEM_ERROR event for %s: %v",
						r.config.ConsumerName, msg.SubmissionID, pubErr)
				}
			}

			// Permanently remove toxic message from stream PEL
			if ackErr := r.consumer.AckMessage(context.Background(), msg.MessageID); ackErr != nil {
				res.Errors = append(res.Errors, fmt.Errorf("failed to ACK toxic message %s: %w", msg.MessageID, ackErr))
			} else {
				res.ExhaustedCount++
			}
			continue
		}

		// 3. RetryCount < 1: Eligible for retry attempt
		claimed, err := r.repo.ClaimReclaimedSubmission(ctx, msg.SubmissionID)
		if err != nil {
			res.Errors = append(res.Errors, fmt.Errorf("failed to claim reclaimed submission %s: %w", msg.SubmissionID, err))
			continue
		}
		if !claimed {
			// Already claimed or state changed concurrently
			continue
		}

		res.RetriedCount++

		// Deliver to worker execution pipeline
		if r.dispatchFunc != nil {
			r.dispatchFunc(ctx, msg)
		}
	}

	return res, nil
}

// Start launches the periodic reclamation loop in a background goroutine.
func (r *Reaper) Start(ctx context.Context) error {
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return fmt.Errorf("reaper is already running")
	}
	r.running = true
	r.stopCh = make(chan struct{})
	r.doneCh = make(chan struct{})
	r.mu.Unlock()

	go r.run(ctx)
	return nil
}

func (r *Reaper) run(ctx context.Context) {
	defer close(r.doneCh)

	ticker := time.NewTicker(r.config.ReaperInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-r.stopCh:
			return
		case <-ticker.C:
			res, err := r.ReapOnce(ctx)
			if err != nil {
				log.Printf("[Reaper %s] error during reap pass: %v", r.config.ConsumerName, err)
			} else if res.ClaimedCount > 0 {
				log.Printf("[Reaper %s] pass completed: claimed=%d, retried=%d, exhausted=%d, terminal_acked=%d",
					r.config.ConsumerName, res.ClaimedCount, res.RetriedCount, res.ExhaustedCount, res.TerminalAckCount)
			}
		}
	}
}

// Stop signals the background reaper loop to exit.
func (r *Reaper) Stop() {
	r.mu.Lock()
	if !r.running {
		r.mu.Unlock()
		return
	}
	r.running = false
	close(r.stopCh)
	r.mu.Unlock()

	<-r.doneCh
}
