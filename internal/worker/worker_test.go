package worker

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SatyamSingh-16/code_executor/internal/runner"
)

type mockRepo struct {
	mu          sync.Mutex
	submissions map[string]*Submission
	claimed     map[string]bool
	completed   map[string]*runner.ExecutionResult
	claimErr    error
	completeErr error
}

func newMockRepo() *mockRepo {
	return &mockRepo{
		submissions: make(map[string]*Submission),
		claimed:     make(map[string]bool),
		completed:   make(map[string]*runner.ExecutionResult),
	}
}

func (m *mockRepo) GetSubmission(ctx context.Context, id string) (*Submission, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	sub, ok := m.submissions[id]
	if !ok {
		return nil, ErrSubmissionNotFound
	}
	// Return a copy
	cp := *sub
	return &cp, nil
}

func (m *mockRepo) ClaimSubmission(ctx context.Context, id string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.claimErr != nil {
		return false, m.claimErr
	}
	sub, ok := m.submissions[id]
	if !ok || sub.Status != "QUEUED" {
		return false, nil
	}
	sub.Status = "PROCESSING"
	m.claimed[id] = true
	return true, nil
}

func (m *mockRepo) CompleteSubmission(ctx context.Context, id string, result *runner.ExecutionResult) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.completeErr != nil {
		return m.completeErr
	}
	sub, ok := m.submissions[id]
	if !ok {
		return ErrSubmissionNotFound
	}
	sub.Status = string(result.Status)
	m.completed[id] = result
	return nil
}

func (m *mockRepo) ClaimReclaimedSubmission(ctx context.Context, id string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	sub, ok := m.submissions[id]
	if !ok || sub.RetryCount >= 1 {
		return false, nil
	}
	sub.RetryCount++
	sub.Status = "PROCESSING"
	return true, nil
}

func (m *mockRepo) FailSubmissionMaxRetries(ctx context.Context, id string, stderr string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	sub, ok := m.submissions[id]
	if !ok {
		return ErrSubmissionNotFound
	}
	sub.Status = string(runner.StatusSystemError)
	return nil
}

type mockRunner struct {
	mu           sync.Mutex
	requests     []runner.ExecutionRequest
	executeFunc  func(ctx context.Context, req runner.ExecutionRequest) (*runner.ExecutionResult, error)
	activeCount  int32
	maxActive    int32
	executeDelay time.Duration
}

func (m *mockRunner) Execute(ctx context.Context, req runner.ExecutionRequest) (*runner.ExecutionResult, error) {
	cur := atomic.AddInt32(&m.activeCount, 1)
	defer atomic.AddInt32(&m.activeCount, -1)

	// Track peak concurrent active executions
	for {
		oldMax := atomic.LoadInt32(&m.maxActive)
		if cur <= oldMax || atomic.CompareAndSwapInt32(&m.maxActive, oldMax, cur) {
			break
		}
	}

	if m.executeDelay > 0 {
		time.Sleep(m.executeDelay)
	}

	m.mu.Lock()
	m.requests = append(m.requests, req)
	fn := m.executeFunc
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, req)
	}

	return &runner.ExecutionResult{
		Status:        runner.StatusSuccess,
		Stdout:        "success output",
		ExitCode:      0,
		WallTimeMs:    120,
		MemoryUsageKb: 1024,
		PeakMemoryKb:  1024,
	}, nil
}

type mockConsumer struct {
	mu            sync.Mutex
	messages      []StreamMessage
	acked         []string
	readErr       error
	ackErr        error
	autoClaimErr  error
	autoClaimMsgs []StreamMessage
}

func (c *mockConsumer) ReadMessages(ctx context.Context, count int64, block time.Duration) ([]StreamMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.readErr != nil {
		return nil, c.readErr
	}
	if len(c.messages) == 0 {
		// Non-blocking wait for test
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(20 * time.Millisecond):
			return []StreamMessage{}, nil
		}
	}
	batchSize := int(count)
	if batchSize > len(c.messages) {
		batchSize = len(c.messages)
	}
	batch := c.messages[:batchSize]
	c.messages = c.messages[batchSize:]
	return batch, nil
}

func (c *mockConsumer) AckMessage(ctx context.Context, messageID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ackErr != nil {
		return c.ackErr
	}
	c.acked = append(c.acked, messageID)
	return nil
}

func (c *mockConsumer) AutoClaim(ctx context.Context, minIdle time.Duration, start string, count int64) ([]StreamMessage, string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.autoClaimErr != nil {
		return nil, "", c.autoClaimErr
	}
	msgs := c.autoClaimMsgs
	c.autoClaimMsgs = nil
	return msgs, "0-0", nil
}

type mockPublisher struct {
	mu        sync.Mutex
	events    []StatusEvent
	pubErr    error
	callOrder []string
}

func (p *mockPublisher) PublishStatusEvent(ctx context.Context, submissionID string, status string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pubErr != nil {
		return p.pubErr
	}
	p.events = append(p.events, StatusEvent{SubmissionID: submissionID, Status: status})
	p.callOrder = append(p.callOrder, status)
	return nil
}

func TestWorker_EndToEndProcessing(t *testing.T) {
	repo := newMockRepo()
	repo.submissions["sub-1"] = &Submission{
		ID:       "sub-1",
		UserID:   "user-1",
		Language: "python",
		Code:     "print('hello from DB')",
		Stdin:    "test input",
		Status:   "QUEUED",
	}

	execRunner := &mockRunner{}
	consumer := &mockConsumer{
		messages: []StreamMessage{
			{MessageID: "1000-0", SubmissionID: "sub-1"},
		},
	}
	publisher := &mockPublisher{}

	cfg := WorkerConfig{
		StreamName:       "submissions:stream",
		ConsumerGroup:    "workers_group",
		ConsumerName:     "worker-test",
		ConcurrencyLimit: 2,
		PollBatchSize:    5,
		PollBlockTimeout: 10 * time.Millisecond,
		ShutdownTimeout:  2 * time.Second,
	}

	w := NewWorker(cfg, repo, execRunner, consumer, publisher)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := w.Start(ctx); err != nil {
		t.Fatalf("failed to start worker: %v", err)
	}

	// Wait for processing to complete
	time.Sleep(100 * time.Millisecond)
	_ = w.StopWithTimeout(1 * time.Second)

	// Verification 1: Runner received authoritative data from PostgreSQL
	execRunner.mu.Lock()
	if len(execRunner.requests) != 1 {
		t.Fatalf("expected 1 execution request, got %d", len(execRunner.requests))
	}
	req := execRunner.requests[0]
	execRunner.mu.Unlock()

	if req.Code != "print('hello from DB')" || req.Stdin != "test input" || req.Language != runner.LanguagePython {
		t.Errorf("runner did not receive authoritative PostgreSQL data: %+v", req)
	}

	// Verification 2: PostgreSQL status updated to SUCCESS
	repo.mu.Lock()
	sub := repo.submissions["sub-1"]
	completedRes := repo.completed["sub-1"]
	repo.mu.Unlock()

	if sub.Status != "SUCCESS" {
		t.Errorf("expected PostgreSQL status SUCCESS, got %s", sub.Status)
	}
	if completedRes == nil || completedRes.Status != runner.StatusSuccess {
		t.Errorf("expected completed result with StatusSuccess, got %+v", completedRes)
	}

	// Verification 3: Redis Pub/Sub received events in order: PROCESSING, then SUCCESS
	publisher.mu.Lock()
	if len(publisher.events) != 2 {
		t.Fatalf("expected 2 pub/sub events, got %d", len(publisher.events))
	}
	if publisher.events[0].Status != "PROCESSING" || publisher.events[1].Status != "SUCCESS" {
		t.Errorf("unexpected pub/sub event order: %v", publisher.events)
	}
	publisher.mu.Unlock()

	// Verification 4: Stream message acknowledged via XACK
	consumer.mu.Lock()
	if len(consumer.acked) != 1 || consumer.acked[0] != "1000-0" {
		t.Errorf("expected message 1000-0 to be acknowledged, got %v", consumer.acked)
	}
	consumer.mu.Unlock()
}

func TestWorker_ConcurrencySemaphore(t *testing.T) {
	// Verify that with ConcurrencyLimit = 2 and 10 submissions,
	// at no point do active concurrent executions exceed 2.
	repo := newMockRepo()
	var msgs []StreamMessage
	for i := 1; i <= 10; i++ {
		id := "sub-" + string(rune('0'+i))
		repo.submissions[id] = &Submission{
			ID:       id,
			Language: "python",
			Code:     "print(1)",
			Status:   "QUEUED",
		}
		msgs = append(msgs, StreamMessage{MessageID: "msg-" + id, SubmissionID: id})
	}

	execRunner := &mockRunner{
		executeDelay: 30 * time.Millisecond, // Each execution takes 30ms
	}
	consumer := &mockConsumer{messages: msgs}
	publisher := &mockPublisher{}

	cfg := WorkerConfig{
		StreamName:       "submissions:stream",
		ConsumerGroup:    "workers_group",
		ConsumerName:     "concurrency-worker",
		ConcurrencyLimit: 2, // Strict limit = 2
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

	// Wait enough for all 10 items (5 pairs * 30ms = ~150ms + overhead)
	time.Sleep(350 * time.Millisecond)
	_ = w.StopWithTimeout(2 * time.Second)

	peak := atomic.LoadInt32(&execRunner.maxActive)
	if peak > 2 {
		t.Fatalf("concurrency semaphore violated! Max concurrent executions was %d, expected <= 2", peak)
	}
	if peak == 0 {
		t.Fatal("no executions occurred")
	}

	repo.mu.Lock()
	completedCount := len(repo.completed)
	repo.mu.Unlock()

	if completedCount != 10 {
		t.Errorf("expected all 10 submissions to complete, got %d", completedCount)
	}
}

func TestWorker_DuplicateDeliveryIdempotency(t *testing.T) {
	repo := newMockRepo()
	repo.submissions["sub-dup"] = &Submission{
		ID:       "sub-dup",
		Language: "python",
		Code:     "print('dup')",
		Status:   "QUEUED",
	}

	execRunner := &mockRunner{}
	// Deliver the SAME submission ID twice
	consumer := &mockConsumer{
		messages: []StreamMessage{
			{MessageID: "msg-1", SubmissionID: "sub-dup"},
			{MessageID: "msg-2", SubmissionID: "sub-dup"},
		},
	}
	publisher := &mockPublisher{}

	cfg := WorkerConfig{
		ConcurrencyLimit: 2,
		PollBatchSize:    10,
		PollBlockTimeout: 10 * time.Millisecond,
		ShutdownTimeout:  2 * time.Second,
	}

	w := NewWorker(cfg, repo, execRunner, consumer, publisher)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_ = w.Start(ctx)
	time.Sleep(100 * time.Millisecond)
	_ = w.StopWithTimeout(1 * time.Second)

	execRunner.mu.Lock()
	execCalls := len(execRunner.requests)
	execRunner.mu.Unlock()

	// Atomic claim ensures runner is only called ONCE despite duplicate stream messages
	if execCalls != 1 {
		t.Fatalf("expected runner to be called exactly once for duplicate submission, got %d", execCalls)
	}
}

func TestWorker_ResultStatusMappings(t *testing.T) {
	testCases := []struct {
		name           string
		runnerStatus   runner.ExecutionStatus
		expectedStatus string
	}{
		{"SUCCESS", runner.StatusSuccess, "SUCCESS"},
		{"RUNTIME_ERROR", runner.StatusRuntimeError, "RUNTIME_ERROR"},
		{"TIME_LIMIT_EXCEEDED", runner.StatusTimeLimitExceeded, "TIME_LIMIT_EXCEEDED"},
		{"MEMORY_LIMIT_EXCEEDED", runner.StatusMemoryLimitExceeded, "MEMORY_LIMIT_EXCEEDED"},
		{"COMPILATION_ERROR", runner.StatusCompilationError, "COMPILATION_ERROR"},
		{"SYSTEM_ERROR", runner.StatusSystemError, "SYSTEM_ERROR"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			repo := newMockRepo()
			subID := "sub-" + tc.name
			repo.submissions[subID] = &Submission{
				ID:       subID,
				Language: "python",
				Code:     "print(1)",
				Status:   "QUEUED",
			}

			execRunner := &mockRunner{
				executeFunc: func(ctx context.Context, req runner.ExecutionRequest) (*runner.ExecutionResult, error) {
					return &runner.ExecutionResult{
						Status:        tc.runnerStatus,
						ExitCode:      1,
						WallTimeMs:    50,
						MemoryUsageKb: 512,
						PeakMemoryKb:  512,
					}, nil
				},
			}

			consumer := &mockConsumer{
				messages: []StreamMessage{{MessageID: "msg-" + tc.name, SubmissionID: subID}},
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
			time.Sleep(50 * time.Millisecond)
			_ = w.StopWithTimeout(1 * time.Second)

			repo.mu.Lock()
			res := repo.completed[subID]
			sub := repo.submissions[subID]
			repo.mu.Unlock()

			if sub.Status != tc.expectedStatus {
				t.Errorf("expected PostgreSQL status %s, got %s", tc.expectedStatus, sub.Status)
			}
			if string(res.Status) != tc.expectedStatus {
				t.Errorf("expected result status %s, got %s", tc.expectedStatus, res.Status)
			}
		})
	}
}

func TestWorker_PubSubFailureDoesNotCorruptDB(t *testing.T) {
	repo := newMockRepo()
	repo.submissions["sub-puberr"] = &Submission{
		ID:       "sub-puberr",
		Language: "python",
		Code:     "print('pub err')",
		Status:   "QUEUED",
	}

	execRunner := &mockRunner{}
	consumer := &mockConsumer{
		messages: []StreamMessage{{MessageID: "msg-puberr", SubmissionID: "sub-puberr"}},
	}
	publisher := &mockPublisher{
		pubErr: errors.New("redis pubsub connection reset"),
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

	// Database record must still be completed as SUCCESS despite Pub/Sub failure
	repo.mu.Lock()
	sub := repo.submissions["sub-puberr"]
	repo.mu.Unlock()

	if sub.Status != "SUCCESS" {
		t.Errorf("PostgreSQL status was improperly altered on Pub/Sub failure: %s", sub.Status)
	}

	// Message should still be acknowledged because DB is durable
	consumer.mu.Lock()
	acked := len(consumer.acked)
	consumer.mu.Unlock()
	if acked != 1 {
		t.Errorf("expected stream message to be acknowledged after durable DB save, got %d acks", acked)
	}
}

func TestWorker_DBPersistenceFailureDoesNotACK(t *testing.T) {
	repo := newMockRepo()
	repo.submissions["sub-dberr"] = &Submission{
		ID:       "sub-dberr",
		Language: "python",
		Code:     "print('db err')",
		Status:   "QUEUED",
	}
	repo.completeErr = errors.New("postgres disk full: cannot update row")

	execRunner := &mockRunner{}
	consumer := &mockConsumer{
		messages: []StreamMessage{{MessageID: "msg-dberr", SubmissionID: "sub-dberr"}},
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

	// CRITICAL: When DB persistence fails, message MUST NOT be acknowledged in Redis
	consumer.mu.Lock()
	acked := len(consumer.acked)
	consumer.mu.Unlock()

	if acked != 0 {
		t.Errorf("stream message was prematurely acknowledged despite database persistence failure!")
	}
}

func TestWorker_InvalidOrMissingSubmissionACK(t *testing.T) {
	repo := newMockRepo() // Empty repo: submission does not exist

	execRunner := &mockRunner{}
	consumer := &mockConsumer{
		messages: []StreamMessage{{MessageID: "msg-missing", SubmissionID: "sub-does-not-exist"}},
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

	// Runner must NOT be executed for non-existent submission
	execRunner.mu.Lock()
	execCalls := len(execRunner.requests)
	execRunner.mu.Unlock()
	if execCalls != 0 {
		t.Errorf("runner was called for non-existent submission")
	}

	// Invalid message acknowledged to remove from stream PEL
	consumer.mu.Lock()
	acked := len(consumer.acked)
	consumer.mu.Unlock()
	if acked != 1 {
		t.Errorf("invalid stream message was not acknowledged: %d acks", acked)
	}
}
