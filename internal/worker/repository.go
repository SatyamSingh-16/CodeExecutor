package worker

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/SatyamSingh-16/code_executor/internal/runner"
)

// PostgresSubmissionRepository implements SubmissionRepository using PostgreSQL.
type PostgresSubmissionRepository struct {
	db *sql.DB
}

// NewPostgresSubmissionRepository constructs a new PostgreSQL repository.
func NewPostgresSubmissionRepository(db *sql.DB) *PostgresSubmissionRepository {
	return &PostgresSubmissionRepository{db: db}
}

// GetSubmission fetches the authoritative submission entity by UUID.
func (r *PostgresSubmissionRepository) GetSubmission(ctx context.Context, id string) (*Submission, error) {
	query := `
		SELECT id, user_id, language, code, stdin, status, retry_count, created_at, updated_at
		FROM submissions
		WHERE id = $1;
	`

	sub := &Submission{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&sub.ID,
		&sub.UserID,
		&sub.Language,
		&sub.Code,
		&sub.Stdin,
		&sub.Status,
		&sub.RetryCount,
		&sub.CreatedAt,
		&sub.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrSubmissionNotFound
		}
		return nil, fmt.Errorf("failed to query submission %s: %w", id, err)
	}

	return sub, nil
}

// ClaimSubmission atomically transitions the submission state from QUEUED to PROCESSING.
// If zero rows are affected, returns false (e.g. duplicate message or already completed).
func (r *PostgresSubmissionRepository) ClaimSubmission(ctx context.Context, id string) (bool, error) {
	query := `
		UPDATE submissions
		SET status = 'PROCESSING',
		    updated_at = NOW()
		WHERE id = $1
		  AND status = 'QUEUED';
	`

	res, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return false, fmt.Errorf("failed to claim submission %s: %w", id, err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("failed to inspect rows affected for submission %s: %w", id, err)
	}

	return rows > 0, nil
}

// CompleteSubmission updates the database with the terminal execution results and telemetry metrics.
// PostgreSQL remains the authoritative persistent store for all execution artifacts.
func (r *PostgresSubmissionRepository) CompleteSubmission(ctx context.Context, id string, result *runner.ExecutionResult) error {
	if result == nil {
		return errors.New("cannot persist nil execution result")
	}

	// Resolve execution time metric
	var execTime *int64
	if result.WallTimeMs > 0 {
		val := result.WallTimeMs
		execTime = &val
	} else if result.Duration > 0 {
		val := result.Duration.Milliseconds()
		execTime = &val
	} else if result.CompileDuration > 0 {
		val := result.CompileDuration.Milliseconds()
		execTime = &val
	}

	// Resolve peak memory metric without losing peak RSS telemetry
	var memUsage *int64
	if result.PeakMemoryKb > 0 {
		val := result.PeakMemoryKb
		memUsage = &val
	} else if result.MemoryUsageKb > 0 {
		val := result.MemoryUsageKb
		memUsage = &val
	}

	var exitCode *int
	if result.Status != runner.StatusSystemError || result.ExitCode != 0 {
		val := result.ExitCode
		exitCode = &val
	}

	query := `
		UPDATE submissions
		SET status = $2,
		    stdout = $3,
		    stderr = $4,
		    compilation_output = $5,
		    stdout_truncated = $6,
		    stderr_truncated = $7,
		    exit_code = $8,
		    execution_time_ms = $9,
		    memory_usage_kb = $10,
		    updated_at = NOW()
		WHERE id = $1;
	`

	res, err := r.db.ExecContext(ctx, query,
		id,
		string(result.Status),
		result.Stdout,
		result.Stderr,
		result.CompilationOutput,
		result.StdoutTruncated,
		result.StderrTruncated,
		exitCode,
		execTime,
		memUsage,
	)
	if err != nil {
		return fmt.Errorf("failed to update execution result for submission %s: %w", id, err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to inspect affected rows for submission %s result: %w", id, err)
	}
	if rows == 0 {
		return fmt.Errorf("no submission found with id %s to complete", id)
	}

	return nil
}
