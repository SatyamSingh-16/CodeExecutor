package worker

import (
	"context"
	"testing"
	"time"

	"github.com/SatyamSingh-16/code_executor/internal/runner"
)

func TestReaper_SingleRetryExecution(t *testing.T) {
	// A worker crashed while executing sub-reap-1 (status: PROCESSING, retry_count: 0)
	repo := newMockRepo()
	repo.submissions["sub-reap-1"] = &Submission{
		ID:         "sub-reap-1",
		Language:   "python",
		Code:       "print('reclaimed execution')",
		Status:     "PROCESSING",
		RetryCount: 0,
	}

	consumer := &mockConsumer{}
	publisher := &mockPublisher{}

	var dispatched []StreamMessage
	dispatch := func(ctx context.Context, msg StreamMessage) {
		dispatched = append(dispatched, msg)
	}

	cfg := WorkerConfig{
		ReaperMinIdle:   10 * time.Millisecond,
		ReaperBatchSize: 10,
	}

	reaper := NewReaper(cfg, repo, consumer, publisher, dispatch)

	// Simulate AutoClaim returning the orphaned pending message
	reaper.consumer = &reaperMockConsumer{
		mockConsumer: consumer,
		autoClaimMsgs: []StreamMessage{
			{MessageID: "pel-msg-1", SubmissionID: "sub-reap-1"},
		},
	}

	ctx := context.Background()
	res, err := reaper.ReapOnce(ctx)
	if err != nil {
		t.Fatalf("ReapOnce failed: %v", err)
	}

	if res.ClaimedCount != 1 {
		t.Errorf("expected 1 claimed message, got %d", res.ClaimedCount)
	}
	if res.RetriedCount != 1 {
		t.Errorf("expected 1 retried message, got %d", res.RetriedCount)
	}
	if res.ExhaustedCount != 0 {
		t.Errorf("expected 0 exhausted messages, got %d", res.ExhaustedCount)
	}

	// Verify retry_count incremented in PostgreSQL
	repo.mu.Lock()
	sub := repo.submissions["sub-reap-1"]
	repo.mu.Unlock()

	if sub.RetryCount != 1 {
		t.Errorf("expected retry_count to be 1, got %d", sub.RetryCount)
	}
	if sub.Status != "PROCESSING" {
		t.Errorf("expected status to be PROCESSING, got %s", sub.Status)
	}

	// Verify message was dispatched to worker execution pipeline
	if len(dispatched) != 1 || dispatched[0].SubmissionID != "sub-reap-1" {
		t.Fatalf("reclaimed message was not dispatched to execution pipeline: %v", dispatched)
	}
}

func TestReaper_ToxicJobMaxRetriesExceeded(t *testing.T) {
	// A job was already retried once (retry_count: 1), and crashed the worker again.
	// When reclaimed a second time, retries are exhausted.
	repo := newMockRepo()
	repo.submissions["sub-toxic"] = &Submission{
		ID:         "sub-toxic",
		Language:   "python",
		Code:       "crash()",
		Status:     "PROCESSING",
		RetryCount: 1, // Already at max 1 retry
	}

	consumer := &mockConsumer{}
	publisher := &mockPublisher{}

	var dispatched []StreamMessage
	dispatch := func(ctx context.Context, msg StreamMessage) {
		dispatched = append(dispatched, msg)
	}

	cfg := WorkerConfig{
		ReaperMinIdle:   10 * time.Millisecond,
		ReaperBatchSize: 10,
	}

	reaper := NewReaper(cfg, repo, consumer, publisher, dispatch)
	reaper.consumer = &reaperMockConsumer{
		mockConsumer: consumer,
		autoClaimMsgs: []StreamMessage{
			{MessageID: "pel-toxic-1", SubmissionID: "sub-toxic"},
		},
	}

	ctx := context.Background()
	res, err := reaper.ReapOnce(ctx)
	if err != nil {
		t.Fatalf("ReapOnce failed: %v", err)
	}

	if res.ClaimedCount != 1 {
		t.Errorf("expected 1 claimed message, got %d", res.ClaimedCount)
	}
	if res.ExhaustedCount != 1 {
		t.Errorf("expected 1 exhausted message, got %d", res.ExhaustedCount)
	}
	if res.RetriedCount != 0 {
		t.Errorf("expected 0 retried messages, got %d", res.RetriedCount)
	}

	// 1. Toxic job must NOT be dispatched for execution
	if len(dispatched) != 0 {
		t.Errorf("toxic job must not be dispatched for execution, was dispatched: %v", dispatched)
	}

	// 2. PostgreSQL updated to SYSTEM_ERROR
	repo.mu.Lock()
	sub := repo.submissions["sub-toxic"]
	repo.mu.Unlock()

	if sub.Status != string(runner.StatusSystemError) {
		t.Errorf("expected PostgreSQL status SYSTEM_ERROR, got %s", sub.Status)
	}

	// 3. Pub/Sub received SYSTEM_ERROR event
	publisher.mu.Lock()
	if len(publisher.events) != 1 || publisher.events[0].Status != string(runner.StatusSystemError) {
		t.Errorf("expected Pub/Sub to receive SYSTEM_ERROR event, got %v", publisher.events)
	}
	publisher.mu.Unlock()

	// 4. Toxic message acknowledged (XACK) to purge from PEL
	consumer.mu.Lock()
	if len(consumer.acked) != 1 || consumer.acked[0] != "pel-toxic-1" {
		t.Errorf("expected toxic message to be acknowledged with XACK, got %v", consumer.acked)
	}
	consumer.mu.Unlock()
}

func TestReaper_AlreadyTerminalSubmissionACKed(t *testing.T) {
	// Worker completed execution and wrote SUCCESS to PostgreSQL, but crashed before XACK.
	repo := newMockRepo()
	repo.submissions["sub-done"] = &Submission{
		ID:         "sub-done",
		Language:   "python",
		Status:     "SUCCESS", // Already terminal!
		RetryCount: 0,
	}

	consumer := &mockConsumer{}
	publisher := &mockPublisher{}

	var dispatched []StreamMessage
	dispatch := func(ctx context.Context, msg StreamMessage) {
		dispatched = append(dispatched, msg)
	}

	cfg := WorkerConfig{}
	reaper := NewReaper(cfg, repo, consumer, publisher, dispatch)
	reaper.consumer = &reaperMockConsumer{
		mockConsumer: consumer,
		autoClaimMsgs: []StreamMessage{
			{MessageID: "pel-done-1", SubmissionID: "sub-done"},
		},
	}

	ctx := context.Background()
	res, err := reaper.ReapOnce(ctx)
	if err != nil {
		t.Fatalf("ReapOnce failed: %v", err)
	}

	if res.TerminalAckCount != 1 {
		t.Errorf("expected 1 terminal acked message, got %d", res.TerminalAckCount)
	}
	if len(dispatched) != 0 {
		t.Errorf("terminal job must not be dispatched for re-execution")
	}

	// Acknowledged via XACK
	consumer.mu.Lock()
	if len(consumer.acked) != 1 || consumer.acked[0] != "pel-done-1" {
		t.Errorf("expected stale terminal message to be XACKed, got %v", consumer.acked)
	}
	consumer.mu.Unlock()
}

func TestReaper_NonExistentSubmissionPurged(t *testing.T) {
	repo := newMockRepo() // empty
	consumer := &mockConsumer{}
	publisher := &mockPublisher{}

	cfg := WorkerConfig{}
	reaper := NewReaper(cfg, repo, consumer, publisher, nil)
	reaper.consumer = &reaperMockConsumer{
		mockConsumer: consumer,
		autoClaimMsgs: []StreamMessage{
			{MessageID: "pel-ghost-1", SubmissionID: "non-existent-id"},
		},
	}

	ctx := context.Background()
	res, err := reaper.ReapOnce(ctx)
	if err != nil {
		t.Fatalf("ReapOnce failed: %v", err)
	}

	if res.ClaimedCount != 1 {
		t.Errorf("expected 1 claimed message, got %d", res.ClaimedCount)
	}

	// Poison entry acknowledged and purged from stream
	consumer.mu.Lock()
	if len(consumer.acked) != 1 || consumer.acked[0] != "pel-ghost-1" {
		t.Errorf("expected ghost message to be XACKed, got %v", consumer.acked)
	}
	consumer.mu.Unlock()
}

type reaperMockConsumer struct {
	*mockConsumer
	autoClaimMsgs []StreamMessage
}

func (r *reaperMockConsumer) AutoClaim(ctx context.Context, minIdle time.Duration, start string, count int64) ([]StreamMessage, string, error) {
	msgs := r.autoClaimMsgs
	r.autoClaimMsgs = nil
	return msgs, "0-0", nil
}
