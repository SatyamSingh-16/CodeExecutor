package queue

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type mockStaleFinder struct {
	submissions []struct {
		id        string
		createdAt time.Time
	}
}

func (m *mockStaleFinder) GetStaleQueuedSubmissions(ctx context.Context, olderThan time.Time, limit int) ([]string, error) {
	var results []string
	for _, sub := range m.submissions {
		// Submissions created at or before olderThan are considered stale
		if sub.createdAt.Before(olderThan) || sub.createdAt.Equal(olderThan) {
			results = append(results, sub.id)
			if len(results) >= limit {
				break
			}
		}
	}
	return results, nil
}

type mockSubmissionQueue struct {
	mu           sync.Mutex
	enqueued     []string
	enqueueErr   error
	enqueueCalls int
}

func (m *mockSubmissionQueue) Enqueue(ctx context.Context, submissionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.enqueueCalls++
	if m.enqueueErr != nil {
		return m.enqueueErr
	}
	m.enqueued = append(m.enqueued, submissionID)
	return nil
}

func (m *mockSubmissionQueue) InitConsumerGroup(ctx context.Context) error {
	return nil
}

func (m *mockSubmissionQueue) Close() error {
	return nil
}

func TestRecoverySweeper_EligibilityAndFiltering(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)

	// Threshold: 30 seconds
	// Cutoff: 11:59:30
	// sub-young: created at 11:59:45 (15 seconds old) -> ineligible
	// sub-exact: created at 11:59:30 (30 seconds old) -> eligible
	// sub-stale: created at 11:58:00 (120 seconds old) -> eligible
	finder := &mockStaleFinder{
		submissions: []struct {
			id        string
			createdAt time.Time
		}{
			{id: "sub-young", createdAt: now.Add(-15 * time.Second)},
			{id: "sub-exact", createdAt: now.Add(-30 * time.Second)},
			{id: "sub-stale", createdAt: now.Add(-120 * time.Second)},
		},
	}

	queue := &mockSubmissionQueue{}
	cfg := SweeperConfig{
		Interval:             10 * time.Second,
		EligibilityThreshold: 30 * time.Second,
		BatchSize:            50,
	}

	sweeper := NewRecoverySweeper(finder, queue, cfg)
	sweeper.SetNowFunc(func() time.Time { return now })

	res, err := sweeper.SweepOnce(ctx)
	if err != nil {
		t.Fatalf("unexpected SweepOnce error: %v", err)
	}

	if res.ScannedCount != 2 {
		t.Errorf("expected 2 eligible submissions, got %d", res.ScannedCount)
	}
	if res.EnqueuedCount != 2 {
		t.Errorf("expected 2 enqueued submissions, got %d", res.EnqueuedCount)
	}
	if res.FailedCount != 0 {
		t.Errorf("expected 0 failures, got %d", res.FailedCount)
	}

	queue.mu.Lock()
	defer queue.mu.Unlock()
	if len(queue.enqueued) != 2 {
		t.Fatalf("expected 2 items in queue, got %d", len(queue.enqueued))
	}
	if queue.enqueued[0] != "sub-exact" || queue.enqueued[1] != "sub-stale" {
		t.Errorf("unexpected enqueued order: %v", queue.enqueued)
	}
}

func TestRecoverySweeper_RedisFailureResilience(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)

	finder := &mockStaleFinder{
		submissions: []struct {
			id        string
			createdAt time.Time
		}{
			{id: "sub-stuck", createdAt: now.Add(-60 * time.Second)},
		},
	}

	redisDownErr := errors.New("connection refused to redis:6379")
	queue := &mockSubmissionQueue{
		enqueueErr: redisDownErr,
	}

	cfg := SweeperConfig{
		Interval:             10 * time.Second,
		EligibilityThreshold: 30 * time.Second,
		BatchSize:            10,
	}

	sweeper := NewRecoverySweeper(finder, queue, cfg)
	sweeper.SetNowFunc(func() time.Time { return now })

	res, err := sweeper.SweepOnce(ctx)
	if err != nil {
		t.Fatalf("SweepOnce returned top-level error on enqueue failure: %v", err)
	}

	if res.ScannedCount != 1 {
		t.Errorf("expected 1 scanned submission, got %d", res.ScannedCount)
	}
	if res.FailedCount != 1 {
		t.Errorf("expected 1 failed submission, got %d", res.FailedCount)
	}
	if res.EnqueuedCount != 0 {
		t.Errorf("expected 0 enqueued submissions, got %d", res.EnqueuedCount)
	}
	if len(res.Errors) != 1 {
		t.Errorf("expected 1 recorded error, got %d", len(res.Errors))
	}
}

func TestRecoverySweeper_RepeatedSweepsSafe(t *testing.T) {
	// Demonstrates at-least-once delivery where repeated sweeps safely
	// re-enqueue stuck jobs without error or state corruption.
	ctx := context.Background()
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)

	finder := &mockStaleFinder{
		submissions: []struct {
			id        string
			createdAt time.Time
		}{
			{id: "sub-stuck-1", createdAt: now.Add(-45 * time.Second)},
		},
	}

	queue := &mockSubmissionQueue{}
	cfg := SweeperConfig{
		Interval:             10 * time.Second,
		EligibilityThreshold: 30 * time.Second,
		BatchSize:            10,
	}

	sweeper := NewRecoverySweeper(finder, queue, cfg)
	sweeper.SetNowFunc(func() time.Time { return now })

	// First pass
	res1, err := sweeper.SweepOnce(ctx)
	if err != nil || res1.EnqueuedCount != 1 {
		t.Fatalf("pass 1 failed: err=%v, res=%+v", err, res1)
	}

	// Advance clock slightly (job still QUEUED because worker hasn't processed it yet)
	sweeper.SetNowFunc(func() time.Time { return now.Add(10 * time.Second) })

	// Second pass
	res2, err := sweeper.SweepOnce(ctx)
	if err != nil || res2.EnqueuedCount != 1 {
		t.Fatalf("pass 2 failed: err=%v, res=%+v", err, res2)
	}

	queue.mu.Lock()
	defer queue.mu.Unlock()
	if queue.enqueueCalls != 2 {
		t.Errorf("expected 2 enqueue attempts across 2 passes, got %d", queue.enqueueCalls)
	}
	if len(queue.enqueued) != 2 || queue.enqueued[0] != "sub-stuck-1" || queue.enqueued[1] != "sub-stuck-1" {
		t.Errorf("expected duplicate enqueue tolerated under at-least-once delivery: %v", queue.enqueued)
	}
}

func TestRecoverySweeper_Lifecycle(t *testing.T) {
	finder := &mockStaleFinder{}
	queue := &mockSubmissionQueue{}
	cfg := SweeperConfig{
		Interval:             20 * time.Millisecond,
		EligibilityThreshold: 30 * time.Second,
		BatchSize:            10,
	}

	sweeper := NewRecoverySweeper(finder, queue, cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := sweeper.Start(ctx); err != nil {
		t.Fatalf("failed to start sweeper: %v", err)
	}

	// Starting twice should return error
	if err := sweeper.Start(ctx); err == nil {
		t.Fatal("expected error starting already running sweeper, got nil")
	}

	time.Sleep(50 * time.Millisecond)
	sweeper.Stop()
}
