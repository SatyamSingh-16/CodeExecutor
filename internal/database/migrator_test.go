package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

func getTestDB(t *testing.T) *sql.DB {
	t.Helper()
	urls := []string{
		os.Getenv("DATABASE_URL"),
		"postgres://satyamsingh2730@localhost:5432/code_execution_test_db?sslmode=disable",
		"postgres://postgres@localhost:5432/code_execution_test_db?sslmode=disable",
		"postgres://localhost:5432/code_execution_test_db?sslmode=disable",
	}

	var db *sql.DB
	var pingErr error
	for _, u := range urls {
		if u == "" {
			continue
		}
		candidate, err := sql.Open("postgres", u)
		if err != nil {
			pingErr = err
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err = candidate.PingContext(ctx)
		cancel()
		if err == nil {
			db = candidate
			break
		}
		_ = candidate.Close()
		pingErr = err
	}

	if db == nil {
		t.Skipf("skipping test: no reachable PostgreSQL database found: %v", pingErr)
	}

	t.Cleanup(func() {
		_ = db.Close()
	})

	return db
}

func TestMigrationsLifecycle(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()
	migrator := NewMigrator(db)

	// Clean slate before test: run Down
	_ = migrator.Down(ctx)

	// 1. Clean database can apply migrations successfully
	t.Run("Clean database applies migrations up", func(t *testing.T) {
		if err := migrator.Up(ctx); err != nil {
			t.Fatalf("failed to apply migrations up: %v", err)
		}

		applied, err := migrator.GetAppliedVersions(ctx)
		if err != nil {
			t.Fatalf("failed to get applied versions: %v", err)
		}
		if !applied[1] || !applied[2] {
			t.Errorf("expected versions 1 and 2 applied, got %v", applied)
		}
	})

	// 2. Users table exists with expected schema and constraints
	t.Run("Users table schema and constraints", func(t *testing.T) {
		var exists bool
		query := `SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'users');`
		if err := db.QueryRowContext(ctx, query).Scan(&exists); err != nil || !exists {
			t.Fatalf("users table does not exist: %v", err)
		}

		// Insert user with generated UUID
		var userID string
		insertQuery := `
		INSERT INTO users (email, password_hash)
		VALUES ('test@example.com', '$2a$12$hashedpassword')
		RETURNING id;`
		if err := db.QueryRowContext(ctx, insertQuery).Scan(&userID); err != nil {
			t.Fatalf("failed to insert user: %v", err)
		}
		if userID == "" {
			t.Errorf("expected non-empty generated UUID")
		}

		// Duplicate email is rejected
		dupQuery := `
		INSERT INTO users (email, password_hash)
		VALUES ('test@example.com', '$2a$12$secondhash');`
		_, err := db.ExecContext(ctx, dupQuery)
		if err == nil {
			t.Errorf("expected error on duplicate email, got nil")
		}
		if !strings.Contains(err.Error(), "uq_users_email") && !strings.Contains(err.Error(), "duplicate key") {
			t.Errorf("expected unique constraint violation on email, got %v", err)
		}

		// Empty email is rejected
		emptyEmailQuery := `
		INSERT INTO users (email, password_hash)
		VALUES ('', '$2a$12$somehash');`
		_, err = db.ExecContext(ctx, emptyEmailQuery)
		if err == nil {
			t.Errorf("expected error on empty email check constraint")
		}
	})

	// 3. Submissions table schema, foreign keys, and status constraints
	t.Run("Submissions table constraints and foreign key", func(t *testing.T) {
		// Retrieve existing user ID
		var userID string
		if err := db.QueryRowContext(ctx, "SELECT id FROM users LIMIT 1;").Scan(&userID); err != nil {
			t.Fatalf("failed to find user: %v", err)
		}

		// Submission referencing nonexistent user is rejected
		fakeUserID := "00000000-0000-0000-0000-000000000000"
		badFKQuery := fmt.Sprintf(`
		INSERT INTO submissions (user_id, language, code, status)
		VALUES ('%s', 'python', 'print(1)', 'QUEUED');`, fakeUserID)
		_, err := db.ExecContext(ctx, badFKQuery)
		if err == nil {
			t.Errorf("expected foreign key violation for nonexistent user_id")
		}
		if !strings.Contains(err.Error(), "foreign key") && !strings.Contains(err.Error(), "violates foreign key constraint") {
			t.Errorf("expected foreign key violation error message, got %v", err)
		}

		// Invalid status is rejected
		badStatusQuery := fmt.Sprintf(`
		INSERT INTO submissions (user_id, language, code, status)
		VALUES ('%s', 'python', 'print(1)', 'INVALID_STATUS');`, userID)
		_, err = db.ExecContext(ctx, badStatusQuery)
		if err == nil {
			t.Errorf("expected check constraint error for invalid status")
		}

		// Invalid language is rejected
		badLangQuery := fmt.Sprintf(`
		INSERT INTO submissions (user_id, language, code, status)
		VALUES ('%s', 'ruby', 'puts 1', 'QUEUED');`, userID)
		_, err = db.ExecContext(ctx, badLangQuery)
		if err == nil {
			t.Errorf("expected check constraint error for invalid language")
		}

		// Negative execution_time_ms is rejected
		badExecTimeQuery := fmt.Sprintf(`
		INSERT INTO submissions (user_id, language, code, status, execution_time_ms)
		VALUES ('%s', 'python', 'print(1)', 'QUEUED', -10);`, userID)
		_, err = db.ExecContext(ctx, badExecTimeQuery)
		if err == nil {
			t.Errorf("expected check constraint error for negative execution_time_ms")
		}

		// Negative memory_usage_kb is rejected
		badMemQuery := fmt.Sprintf(`
		INSERT INTO submissions (user_id, language, code, status, memory_usage_kb)
		VALUES ('%s', 'python', 'print(1)', 'QUEUED', -50);`, userID)
		_, err = db.ExecContext(ctx, badMemQuery)
		if err == nil {
			t.Errorf("expected check constraint error for negative memory_usage_kb")
		}

		// Negative retry_count is rejected
		badRetryQuery := fmt.Sprintf(`
		INSERT INTO submissions (user_id, language, code, status, retry_count)
		VALUES ('%s', 'python', 'print(1)', 'QUEUED', -1);`, userID)
		_, err = db.ExecContext(ctx, badRetryQuery)
		if err == nil {
			t.Errorf("expected check constraint error for negative retry_count")
		}

		// All 8 canonical domain statuses are valid
		statuses := []string{
			"QUEUED",
			"PROCESSING",
			"SUCCESS",
			"COMPILATION_ERROR",
			"RUNTIME_ERROR",
			"TIME_LIMIT_EXCEEDED",
			"MEMORY_LIMIT_EXCEEDED",
			"SYSTEM_ERROR",
		}

		for _, status := range statuses {
			insertSubQuery := `
			INSERT INTO submissions (user_id, language, code, stdin, status, stdout, stderr, execution_time_ms, memory_usage_kb, retry_count)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			RETURNING id;`
			var subID string
			err := db.QueryRowContext(ctx, insertSubQuery, userID, "python", "print('hello')", "", status, "hello\n", "", 50, 4096, 0).Scan(&subID)
			if err != nil {
				t.Errorf("failed to insert submission with valid status %q: %v", status, err)
			}
			if subID == "" {
				t.Errorf("expected non-empty submission UUID for status %q", status)
			}
		}

		// Foreign key deletion restriction: attempting to delete a user with submissions fails
		deleteUserQuery := fmt.Sprintf("DELETE FROM users WHERE id = '%s';", userID)
		_, err = db.ExecContext(ctx, deleteUserQuery)
		if err == nil {
			t.Errorf("expected foreign key RESTRICT error when deleting user with existing submissions, got nil")
		}
	})

	// 4. User submission history query ordering and index usage
	t.Run("User submission history query ordered by newest first", func(t *testing.T) {
		var userID string
		if err := db.QueryRowContext(ctx, "SELECT id FROM users LIMIT 1;").Scan(&userID); err != nil {
			t.Fatalf("failed to query user: %v", err)
		}

		query := `
		SELECT id, status, created_at
		FROM submissions
		WHERE user_id = $1
		ORDER BY created_at DESC;`

		rows, err := db.QueryContext(ctx, query, userID)
		if err != nil {
			t.Fatalf("failed to query user submissions: %v", err)
		}
		defer rows.Close()

		var count int
		var prevTime time.Time
		for rows.Next() {
			var id, status string
			var createdAt time.Time
			if err := rows.Scan(&id, &status, &createdAt); err != nil {
				t.Fatalf("scan error: %v", err)
			}
			count++
			if !prevTime.IsZero() && createdAt.After(prevTime) {
				t.Errorf("submissions out of order: expected newest first")
			}
			prevTime = createdAt
		}
		if count < 8 {
			t.Errorf("expected at least 8 submissions, found %d", count)
		}

		// Verify index existence
		indexQuery := `
		SELECT indexname
		FROM pg_indexes
		WHERE tablename = 'submissions' AND indexname = 'idx_submissions_user_id_created_at';`
		var indexName string
		if err := db.QueryRowContext(ctx, indexQuery).Scan(&indexName); err != nil || indexName == "" {
			t.Errorf("expected index idx_submissions_user_id_created_at to exist on submissions: %v", err)
		}
	})

	// 5. Down migrations roll back cleanly
	t.Run("Roll back down migrations cleanly", func(t *testing.T) {
		if err := migrator.Down(ctx); err != nil {
			t.Fatalf("failed to roll back down migrations: %v", err)
		}

		// Verify tables are dropped
		var submissionsExists, usersExists bool
		_ = db.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'submissions');").Scan(&submissionsExists)
		_ = db.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'users');").Scan(&usersExists)

		if submissionsExists {
			t.Errorf("expected submissions table to be dropped by down migration")
		}
		if usersExists {
			t.Errorf("expected users table to be dropped by down migration")
		}

		// Verify migrations can be reapplied from scratch
		if err := migrator.Up(ctx); err != nil {
			t.Fatalf("failed to reapply migrations up after down rollback: %v", err)
		}
	})
}

func TestStepByStepMigration(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()
	migrator := NewMigrator(db)

	// Clean slate
	_ = migrator.Down(ctx)

	// 1. Step up to version 1: only users table should exist
	if err := migrator.MigrateTo(ctx, 1); err != nil {
		t.Fatalf("failed to migrate to version 1: %v", err)
	}

	var usersExists, submissionsExists bool
	_ = db.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'users');").Scan(&usersExists)
	_ = db.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'submissions');").Scan(&submissionsExists)

	if !usersExists {
		t.Errorf("expected users table to exist at version 1")
	}
	if submissionsExists {
		t.Errorf("expected submissions table NOT to exist at version 1")
	}

	// 2. Step up to version 2: both users and submissions should exist
	if err := migrator.MigrateTo(ctx, 2); err != nil {
		t.Fatalf("failed to migrate to version 2: %v", err)
	}

	_ = db.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'submissions');").Scan(&submissionsExists)
	if !submissionsExists {
		t.Errorf("expected submissions table to exist at version 2")
	}

	// 3. Roll back to version 1: submissions should be dropped, users still exists
	if err := migrator.MigrateTo(ctx, 1); err != nil {
		t.Fatalf("failed to roll back to version 1: %v", err)
	}

	_ = db.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'users');").Scan(&usersExists)
	_ = db.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'submissions');").Scan(&submissionsExists)

	if !usersExists {
		t.Errorf("expected users table to still exist after rolling back to version 1")
	}
	if submissionsExists {
		t.Errorf("expected submissions table to be dropped after rolling back to version 1")
	}

	// 4. Roll back to version 0: all tables dropped
	if err := migrator.MigrateTo(ctx, 0); err != nil {
		t.Fatalf("failed to roll back to version 0: %v", err)
	}

	_ = db.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'users');").Scan(&usersExists)
	if usersExists {
		t.Errorf("expected users table to be dropped after rolling back to version 0")
	}

	// Reapply full schema up for subsequent tasks
	if err := migrator.Up(ctx); err != nil {
		t.Fatalf("failed to reapply migrations up: %v", err)
	}
}

