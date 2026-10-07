package queue

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sync"
	"time"
)

// PostgresStaleSubmissionFinder queries PostgreSQL for stuck QUEUED submissions.
type PostgresStaleSubmissionFinder struct {
	db *sql.DB
}

// NewPostgresStaleSubmissionFinder creates a new PostgreSQL finder.
func NewPostgresStaleSubmissionFinder(db *sql.DB) *PostgresStaleSubmissionFinder {
	return &PostgresStaleSubmissionFinder{db: db}
}

// GetStaleQueuedSubmissions fetches submissions that have been stuck in QUEUED status
// for longer than the specified cutoff timestamp.
func (f *PostgresStaleSubmissionFinder) GetStaleQueuedSubmissions(ctx context.Context, olderThan time.Time, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 100
	}

	query := `
		SELECT id FROM submissions
		WHERE status = 'QUEUED' AND created_at <= $1
		ORDER BY created_at ASC
		LIMIT $2;
	`

	rows, err := f.db.QueryContext(ctx, query, olderThan, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query stale queued submissions: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan submission id: %w", err)
		}
		ids = append(ids, id)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading submission rows: %w", err)
	}

	return ids, nil
}

// RecoverySweeper periodically detects submissions stuck in QUEUED status in PostgreSQL
// and re-publishes them to the Redis Stream to guarantee at-least-once delivery.
type RecoverySweeper struct {
	finder  StaleSubmissionFinder
	queue   SubmissionQueue
	config  SweeperConfig
	nowFunc func() time.Time

	mu      sync.Mutex
	running bool
	stopCh  chan struct{}
	doneCh  chan struct{}
}

// NewRecoverySweeper constructs a sweeper using a StaleSubmissionFinder and SubmissionQueue.
func NewRecoverySweeper(finder StaleSubmissionFinder, queue SubmissionQueue, cfg SweeperConfig) *RecoverySweeper {
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 100
	}
	if cfg.Interval <= 0 {
		cfg.Interval = 10 * time.Second
	}
	if cfg.EligibilityThreshold <= 0 {
		cfg.EligibilityThreshold = 30 * time.Second
	}

	return &RecoverySweeper{
		finder:  finder,
		queue:   queue,
		config:  cfg,
		nowFunc: time.Now,
	}
}

// NewRecoverySweeperFromDB constructs a sweeper backed directly by a PostgreSQL *sql.DB.
func NewRecoverySweeperFromDB(db *sql.DB, queue SubmissionQueue, cfg SweeperConfig) *RecoverySweeper {
	return NewRecoverySweeper(NewPostgresStaleSubmissionFinder(db), queue, cfg)
}

// SetNowFunc overrides the current time provider (primarily for deterministic unit testing).
func (s *RecoverySweeper) SetNowFunc(fn func() time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nowFunc = fn
}

func (s *RecoverySweeper) now() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.nowFunc != nil {
		return s.nowFunc()
	}
	return time.Now()
}

// SweepOnce runs a single reconciliation pass over PostgreSQL:
// 1. Calculates the eligibility cutoff (now - EligibilityThreshold).
// 2. Selects QUEUED submissions older than the cutoff up to BatchSize.
// 3. Re-enqueues eligible submissions into Redis Streams.
// 4. If Redis enqueue fails, the PostgreSQL record remains untouched in QUEUED state.
func (s *RecoverySweeper) SweepOnce(ctx context.Context) (SweepResult, error) {
	cutoff := s.now().Add(-s.config.EligibilityThreshold)

	ids, err := s.finder.GetStaleQueuedSubmissions(ctx, cutoff, s.config.BatchSize)
	if err != nil {
		return SweepResult{}, fmt.Errorf("sweeper database lookup failed: %w", err)
	}

	res := SweepResult{
		ScannedCount: len(ids),
	}

	for _, id := range ids {
		if err := s.queue.Enqueue(ctx, id); err != nil {
			res.FailedCount++
			res.Errors = append(res.Errors, fmt.Errorf("sweeper enqueue failed for submission %s: %w", id, err))
			// CRITICAL: PostgreSQL is authoritative. If Redis fails, do NOT mark SYSTEM_ERROR
			// or mutate the record. It remains QUEUED for the next sweep pass.
			continue
		}
		res.EnqueuedCount++
	}

	return res, nil
}

// Start begins the periodic recovery sweep loop in a background goroutine.
func (s *RecoverySweeper) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("sweeper is already running")
	}
	s.running = true
	s.stopCh = make(chan struct{})
	s.doneCh = make(chan struct{})
	s.mu.Unlock()

	go s.run(ctx)
	return nil
}

func (s *RecoverySweeper) run(ctx context.Context) {
	defer close(s.doneCh)

	ticker := time.NewTicker(s.config.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
			res, err := s.SweepOnce(ctx)
			if err != nil {
				log.Printf("[RecoverySweeper] sweep pass error: %v", err)
			} else if res.EnqueuedCount > 0 || res.FailedCount > 0 {
				log.Printf("[RecoverySweeper] pass completed: scanned=%d, enqueued=%d, failed=%d",
					res.ScannedCount, res.EnqueuedCount, res.FailedCount)
			}
		}
	}
}

// Stop signals the background sweeper loop to finish and waits for it to exit.
func (s *RecoverySweeper) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	close(s.stopCh)
	s.mu.Unlock()

	<-s.doneCh
}
