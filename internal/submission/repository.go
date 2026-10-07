package submission

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// SubmissionRepository defines database operations for submissions.
type SubmissionRepository interface {
	Create(ctx context.Context, sub *SubmissionEntity) (*SubmissionEntity, error)
	GetByIDAndUserID(ctx context.Context, id, userID string) (*SubmissionEntity, error)
	ListByUserID(ctx context.Context, userID string, limit, offset int) ([]*SubmissionEntity, error)
}

// PostgresSubmissionRepository implements SubmissionRepository using PostgreSQL.
type PostgresSubmissionRepository struct {
	db *sql.DB
}

// NewPostgresSubmissionRepository constructs a new PostgreSQL submission repository.
func NewPostgresSubmissionRepository(db *sql.DB) *PostgresSubmissionRepository {
	return &PostgresSubmissionRepository{db: db}
}

// Create inserts a new submission into PostgreSQL with status QUEUED.
func (r *PostgresSubmissionRepository) Create(ctx context.Context, sub *SubmissionEntity) (*SubmissionEntity, error) {
	query := `
		INSERT INTO submissions (
			user_id, language, code, stdin, status, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, 'QUEUED', NOW(), NOW()
		)
		RETURNING
			id, user_id, language, code, stdin, status,
			stdout, stderr, compilation_output,
			stdout_truncated, stderr_truncated,
			exit_code, execution_time_ms, memory_usage_kb,
			retry_count, created_at, updated_at;
	`

	created := &SubmissionEntity{}
	err := r.db.QueryRowContext(ctx, query,
		sub.UserID,
		sub.Language,
		sub.Code,
		sub.Stdin,
	).Scan(
		&created.ID,
		&created.UserID,
		&created.Language,
		&created.Code,
		&created.Stdin,
		&created.Status,
		&created.Stdout,
		&created.Stderr,
		&created.CompilationOutput,
		&created.StdoutTruncated,
		&created.StderrTruncated,
		&created.ExitCode,
		&created.ExecutionTimeMs,
		&created.MemoryUsageKb,
		&created.RetryCount,
		&created.CreatedAt,
		&created.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to insert submission into postgres: %w", err)
	}

	return created, nil
}

// GetByIDAndUserID retrieves a submission by ID strictly scoped to the authenticated user ID.
// Returns ErrSubmissionNotFound if no row matches.
func (r *PostgresSubmissionRepository) GetByIDAndUserID(ctx context.Context, id, userID string) (*SubmissionEntity, error) {
	query := `
		SELECT
			id, user_id, language, code, stdin, status,
			stdout, stderr, compilation_output,
			stdout_truncated, stderr_truncated,
			exit_code, execution_time_ms, memory_usage_kb,
			retry_count, created_at, updated_at
		FROM submissions
		WHERE id = $1 AND user_id = $2;
	`

	sub := &SubmissionEntity{}
	err := r.db.QueryRowContext(ctx, query, id, userID).Scan(
		&sub.ID,
		&sub.UserID,
		&sub.Language,
		&sub.Code,
		&sub.Stdin,
		&sub.Status,
		&sub.Stdout,
		&sub.Stderr,
		&sub.CompilationOutput,
		&sub.StdoutTruncated,
		&sub.StderrTruncated,
		&sub.ExitCode,
		&sub.ExecutionTimeMs,
		&sub.MemoryUsageKb,
		&sub.RetryCount,
		&sub.CreatedAt,
		&sub.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrSubmissionNotFound
		}
		return nil, fmt.Errorf("failed to query submission by id: %w", err)
	}

	return sub, nil
}

// ListByUserID retrieves a paginated list of submissions for the authenticated user, ordered newest first.
func (r *PostgresSubmissionRepository) ListByUserID(ctx context.Context, userID string, limit, offset int) ([]*SubmissionEntity, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	query := `
		SELECT
			id, user_id, language, code, stdin, status,
			stdout, stderr, compilation_output,
			stdout_truncated, stderr_truncated,
			exit_code, execution_time_ms, memory_usage_kb,
			retry_count, created_at, updated_at
		FROM submissions
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3;
	`

	rows, err := r.db.QueryContext(ctx, query, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list submissions: %w", err)
	}
	defer rows.Close()

	var submissions []*SubmissionEntity
	for rows.Next() {
		sub := &SubmissionEntity{}
		if err := rows.Scan(
			&sub.ID,
			&sub.UserID,
			&sub.Language,
			&sub.Code,
			&sub.Stdin,
			&sub.Status,
			&sub.Stdout,
			&sub.Stderr,
			&sub.CompilationOutput,
			&sub.StdoutTruncated,
			&sub.StderrTruncated,
			&sub.ExitCode,
			&sub.ExecutionTimeMs,
			&sub.MemoryUsageKb,
			&sub.RetryCount,
			&sub.CreatedAt,
			&sub.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan submission row: %w", err)
		}
		submissions = append(submissions, sub)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading submission rows: %w", err)
	}

	return submissions, nil
}
