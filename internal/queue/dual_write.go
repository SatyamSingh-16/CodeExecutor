package queue

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// SubmissionInsertInput contains the parameters to persist a new submission.
type SubmissionInsertInput struct {
	ID        string // Optional UUID; if blank, PostgreSQL generates gen_random_uuid()
	UserID    string
	Language  string
	Code      string
	Stdin     string
	CreatedAt *time.Time // Optional override for testing
}

// SubmissionRecord represents the persisted submission state.
type SubmissionRecord struct {
	ID        string
	UserID    string
	Language  string
	Status    string
	CreatedAt time.Time
}

// DualWriteSubmit implements the write-first dual-write pattern:
// 1. Authoritative Step: Inserts the submission into PostgreSQL with status = 'QUEUED'.
// 2. Delivery Step: Attempts to publish the submission ID to the Redis Stream via Queue.Enqueue().
// 3. Fallback Handling: If Redis enqueue fails:
//    - The PostgreSQL record REMAINS strictly in 'QUEUED' status.
//    - The record is NEVER deleted or set to 'SYSTEM_ERROR'.
//    - The error is returned to the caller so the recovery sweeper can pick it up.
func DualWriteSubmit(ctx context.Context, db *sql.DB, q SubmissionQueue, input SubmissionInsertInput) (*SubmissionRecord, error) {
	if input.UserID == "" {
		return nil, errors.New("user_id is required")
	}
	if input.Language == "" {
		return nil, errors.New("language is required")
	}
	if input.Code == "" {
		return nil, errors.New("code cannot be empty")
	}

	var query string
	var args []interface{}

	if input.ID != "" && input.CreatedAt != nil {
		query = `
			INSERT INTO submissions (id, user_id, language, code, stdin, status, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, 'QUEUED', $6, $6)
			RETURNING id, user_id, language, status, created_at;
		`
		args = []interface{}{input.ID, input.UserID, input.Language, input.Code, input.Stdin, *input.CreatedAt}
	} else if input.ID != "" {
		query = `
			INSERT INTO submissions (id, user_id, language, code, stdin, status, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, 'QUEUED', NOW(), NOW())
			RETURNING id, user_id, language, status, created_at;
		`
		args = []interface{}{input.ID, input.UserID, input.Language, input.Code, input.Stdin}
	} else if input.CreatedAt != nil {
		query = `
			INSERT INTO submissions (user_id, language, code, stdin, status, created_at, updated_at)
			VALUES ($1, $2, $3, $4, 'QUEUED', $5, $5)
			RETURNING id, user_id, language, status, created_at;
		`
		args = []interface{}{input.UserID, input.Language, input.Code, input.Stdin, *input.CreatedAt}
	} else {
		query = `
			INSERT INTO submissions (user_id, language, code, stdin, status, created_at, updated_at)
			VALUES ($1, $2, $3, $4, 'QUEUED', NOW(), NOW())
			RETURNING id, user_id, language, status, created_at;
		`
		args = []interface{}{input.UserID, input.Language, input.Code, input.Stdin}
	}

	record := &SubmissionRecord{}
	err := db.QueryRowContext(ctx, query, args...).Scan(
		&record.ID,
		&record.UserID,
		&record.Language,
		&record.Status,
		&record.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to persist submission in PostgreSQL: %w", err)
	}

	// Step 2: Attempt immediate Redis stream enqueue
	if q != nil {
		if err := q.Enqueue(ctx, record.ID); err != nil {
			// Step 4: Redis failure does NOT modify PostgreSQL state.
			// PostgreSQL remains authoritative, record remains QUEUED.
			return record, fmt.Errorf("submission persisted as QUEUED in PostgreSQL, but Redis enqueue failed: %w", err)
		}
	}

	return record, nil
}
