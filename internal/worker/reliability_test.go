package worker

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SatyamSingh-16/code_executor/internal/runner"
)

// TestReliability_StrictConcurrencyCap verifies that under concurrent message delivery,
// active container executions strictly never exceed WORKER_CONCURRENCY.
func TestReliability_StrictConcurrencyCap(t *testing.T) {
	repo := newMockRepo()
	var msgs []StreamMessage
	for i := 1; i <= 10; i++ {
		id := "sub-cap-" + string(rune('0'+i))
		repo.submissions[id] = &Submission{
			ID:       id,
			Language: "python",
			Code:     "print('ok')",
			Status:   "QUEUED",
		}
		msgs = append(msgs, StreamMessage{MessageID: "msg-" + id, SubmissionID: id})
	}

	execRunner := &mockRunner{
		executeDelay: 25 * time.Millisecond,
	}
	consumer := &mockConsumer{messages: msgs}
	publisher := &mockPublisher{}

	cfg := WorkerConfig{
		StreamName:       "submissions:stream",
		ConsumerGroup:    "workers_group",
		ConsumerName:     "worker-cap",
		ConcurrencyLimit: 2,
		PollBatchSize:    10,
		PollBlockTimeout: 10 * time.Millisecond,
		ShutdownTimeout:  5 * time.Second,
	}

	w := NewWorker(cfg, repo, execRunner, consumer, publisher)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := w.Start(ctx); err != nil {
		t.Fatalf("failed to start worker: %v", err)
	}

	time.Sleep(300 * time.Millisecond)
	_ = w.StopWithTimeout(2 * time.Second)

	peak := atomic.LoadInt32(&execRunner.maxActive)
	if peak > 2 {
		t.Fatalf("concurrency cap violated: peak active executions was %d, expected <= 2", peak)
	}
	if peak == 0 {
		t.Fatal("no executions occurred")
	}

	repo.mu.Lock()
	completedCount := len(repo.completed)
	repo.mu.Unlock()

	if completedCount != 10 {
		t.Errorf("expected 10 completed submissions, got %d", completedCount)
	}
}

// TestReliability_DatabasePersistenceFailure verifies that if PostgreSQL persistence fails,
// the message is NOT acknowledged in Redis, ensuring it remains recoverable in the PEL.
func TestReliability_DatabasePersistenceFailure(t *testing.T) {
	repo := newMockRepo()
	repo.submissions["sub-dbfail"] = &Submission{
		ID:       "sub-dbfail",
		Language: "python",
		Code:     "print('test')",
		Status:   "QUEUED",
	}
	repo.completeErr = errors.New("postgres connection reset during update")

	execRunner := &mockRunner{}
	consumer := &mockConsumer{
		messages: []StreamMessage{{MessageID: "msg-dbfail", SubmissionID: "sub-dbfail"}},
	}
	publisher := &mockPublisher{}

	cfg := WorkerConfig{
		ConcurrencyLimit: 1,
		PollBatchSize:    1,
		PollBlockTimeout: 10 * time.Millisecond,
		ShutdownTimeout:  1 * time.Second,
	}

	w := NewWorker(cfg, repo, execRunner, consumer, publisher)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_ = w.Start(ctx)
	time.Sleep(60 * time.Millisecond)
	_ = w.StopWithTimeout(1 * time.Second)

	// Invariant 1: Message must NOT be XACKed
	consumer.mu.Lock()
	acks := len(consumer.acked)
	consumer.mu.Unlock()
	if acks != 0 {
		t.Errorf("message was prematurely XACKed despite database persistence failure!")
	}

	// Invariant 2: Database status must NOT be falsely marked as terminal (remains PROCESSING)
	repo.mu.Lock()
	sub := repo.submissions["sub-dbfail"]
	repo.mu.Unlock()
	if isTerminalStatus(sub.Status) {
		t.Errorf("submission was falsely marked terminal after persistence failure: %s", sub.Status)
	}

	// Invariant 3: No terminal event emitted to Pub/Sub
	publisher.mu.Lock()
	for _, evt := range publisher.events {
		if evt.Status != "PROCESSING" {
			t.Errorf("terminal event %q was emitted despite persistence failure", evt.Status)
		}
	}
	publisher.mu.Unlock()
}

// TestReliability_PubSubFailure_PreservesDB verifies that if Redis Pub/Sub broadcast fails
// after successful PostgreSQL result persistence, the database result is intact and message is XACKed.
func TestReliability_PubSubFailure_PreservesDB(t *testing.T) {
	repo := newMockRepo()
	repo.submissions["sub-pubfail"] = &Submission{
		ID:       "sub-pubfail",
		Language: "python",
		Code:     "print('ok')",
		Status:   "QUEUED",
	}

	execRunner := &mockRunner{}
	consumer := &mockConsumer{
		messages: []StreamMessage{{MessageID: "msg-pubfail", SubmissionID: "sub-pubfail"}},
	}
	publisher := &mockPublisher{
		pubErr: errors.New("redis pubsub down"),
	}

	cfg := WorkerConfig{
		ConcurrencyLimit: 1,
		PollBatchSize:    1,
		PollBlockTimeout: 10 * time.Millisecond,
		ShutdownTimeout:  1 * time.Second,
	}

	w := NewWorker(cfg, repo, execRunner, consumer, publisher)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_ = w.Start(ctx)
	time.Sleep(60 * time.Millisecond)
	_ = w.StopWithTimeout(1 * time.Second)

	// Invariant 1: DB result is preserved as SUCCESS
	repo.mu.Lock()
	sub := repo.submissions["sub-pubfail"]
	repo.mu.Unlock()
	if sub.Status != "SUCCESS" {
		t.Errorf("expected DB status SUCCESS, got %s", sub.Status)
	}

	// Invariant 2: Message was XACKed since DB write succeeded
	consumer.mu.Lock()
	acks := len(consumer.acked)
	consumer.mu.Unlock()
	if acks != 1 {
		t.Errorf("expected message to be XACKed, got %d", acks)
	}
}

// TestReliability_RedisStreamReadFailure_GracefulRetry verifies that transient Redis read
// errors in the polling loop do not crash the worker.
func TestReliability_RedisStreamReadFailure_GracefulRetry(t *testing.T) {
	repo := newMockRepo()
	execRunner := &mockRunner{}
	consumer := &mockConsumer{
		readErr: errors.New("redis: connection refused"),
	}
	publisher := &mockPublisher{}

	cfg := WorkerConfig{
		ConcurrencyLimit: 1,
		PollBatchSize:    1,
		PollBlockTimeout: 10 * time.Millisecond,
		ShutdownTimeout:  1 * time.Second,
	}

	w := NewWorker(cfg, repo, execRunner, consumer, publisher)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := w.Start(ctx); err != nil {
		t.Fatalf("failed to start worker: %v", err)
	}

	// Run during active read errors
	time.Sleep(60 * time.Millisecond)

	// Now restore read capability
	consumer.mu.Lock()
	consumer.readErr = nil
	repo.submissions["sub-recovered"] = &Submission{
		ID:       "sub-recovered",
		Language: "python",
		Code:     "print(1)",
		Status:   "QUEUED",
	}
	consumer.messages = []StreamMessage{{MessageID: "msg-recovered", SubmissionID: "sub-recovered"}}
	consumer.mu.Unlock()

	time.Sleep(300 * time.Millisecond)
	_ = w.StopWithTimeout(1 * time.Second)

	repo.mu.Lock()
	sub := repo.submissions["sub-recovered"]
	repo.mu.Unlock()
	if sub.Status != "SUCCESS" {
		t.Errorf("worker failed to recover after transient Redis read error: status is %s", sub.Status)
	}
}

// TestReliability_RedisAckFailure_DoesNotCorruptDB verifies that if XACK fails,
// the PostgreSQL execution result remains intact.
func TestReliability_RedisAckFailure_DoesNotCorruptDB(t *testing.T) {
	repo := newMockRepo()
	repo.submissions["sub-ackfail"] = &Submission{
		ID:       "sub-ackfail",
		Language: "python",
		Code:     "print(1)",
		Status:   "QUEUED",
	}

	execRunner := &mockRunner{}
	consumer := &mockConsumer{
		messages: []StreamMessage{{MessageID: "msg-ackfail", SubmissionID: "sub-ackfail"}},
		ackErr:   errors.New("redis: XACK failed due to network partition"),
	}
	publisher := &mockPublisher{}

	cfg := WorkerConfig{
		ConcurrencyLimit: 1,
		PollBatchSize:    1,
		PollBlockTimeout: 10 * time.Millisecond,
		ShutdownTimeout:  1 * time.Second,
	}

	w := NewWorker(cfg, repo, execRunner, consumer, publisher)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_ = w.Start(ctx)
	time.Sleep(60 * time.Millisecond)
	_ = w.StopWithTimeout(1 * time.Second)

	repo.mu.Lock()
	sub := repo.submissions["sub-ackfail"]
	repo.mu.Unlock()

	if sub.Status != "SUCCESS" {
		t.Errorf("DB state corrupted on XACK failure: %s", sub.Status)
	}
}

// TestReliability_AutoClaimFailure_GracefulRetry verifies that transient Redis errors
// during XAUTOCLAIM do not crash the background reaper loop.
func TestReliability_AutoClaimFailure_GracefulRetry(t *testing.T) {
	repo := newMockRepo()
	consumer := &mockConsumer{
		autoClaimErr: errors.New("redis: XAUTOCLAIM timeout"),
	}
	publisher := &mockPublisher{}

	cfg := WorkerConfig{
		ReaperInterval: 20 * time.Millisecond,
		ReaperMinIdle:  30 * time.Millisecond,
	}

	reaper := NewReaper(cfg, repo, consumer, publisher, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := reaper.Start(ctx); err != nil {
		t.Fatalf("failed to start reaper: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	// Now clear the error
	consumer.mu.Lock()
	consumer.autoClaimErr = nil
	consumer.mu.Unlock()

	time.Sleep(50 * time.Millisecond)
	reaper.Stop()
}

// TestReliability_TerminalStateReclaim_ImmediateAck verifies that if a job already reached
// a terminal state (e.g. SUCCESS), XAUTOCLAIM immediately XACKs it without re-executing.
func TestReliability_TerminalStateReclaim_ImmediateAck(t *testing.T) {
	terminalStatuses := []runner.ExecutionStatus{
		runner.StatusSuccess,
		runner.StatusCompilationError,
		runner.StatusRuntimeError,
		runner.StatusTimeLimitExceeded,
		runner.StatusMemoryLimitExceeded,
		runner.StatusSystemError,
	}

	for _, termStatus := range terminalStatuses {
		t.Run(string(termStatus), func(t *testing.T) {
			repo := newMockRepo()
			subID := "sub-term-" + string(termStatus)
			repo.submissions[subID] = &Submission{
				ID:         subID,
				Language:   "python",
				Status:     string(termStatus),
				RetryCount: 0,
			}

			consumer := &mockConsumer{
				autoClaimMsgs: []StreamMessage{{MessageID: "msg-" + subID, SubmissionID: subID}},
			}
			publisher := &mockPublisher{}

			var dispatchedCount int
			dispatch := func(ctx context.Context, msg StreamMessage) {
				dispatchedCount++
			}

			reaper := NewReaper(WorkerConfig{}, repo, consumer, publisher, dispatch)
			res, err := reaper.ReapOnce(context.Background())
			if err != nil {
				t.Fatalf("ReapOnce failed: %v", err)
			}

			if res.TerminalAckCount != 1 {
				t.Errorf("expected 1 terminal ack, got %d", res.TerminalAckCount)
			}
			if dispatchedCount != 0 {
				t.Errorf("terminal job %s must not be re-dispatched for execution", termStatus)
			}

			consumer.mu.Lock()
			acks := len(consumer.acked)
			consumer.mu.Unlock()
			if acks != 1 {
				t.Errorf("expected terminal job to be XACKed, got %d", acks)
			}
		})
	}
}
