package database

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"

	"github.com/SatyamSingh-16/code_executor/migrations"
)

// Migration represents a single versioned migration pair.
type Migration struct {
	Version  int
	Name     string
	UpFile   string
	DownFile string
}

// Migrator manages schema migrations against a PostgreSQL database.
type Migrator struct {
	db *sql.DB
}

// NewMigrator constructs a new database migrator.
func NewMigrator(db *sql.DB) *Migrator {
	return &Migrator{db: db}
}

// EnsureSchemaMigrationsTable creates the migration history table if not present.
func (m *Migrator) EnsureSchemaMigrationsTable(ctx context.Context) error {
	query := `
	CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);`
	_, err := m.db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to create schema_migrations table: %w", err)
	}
	return nil
}

// GetAppliedVersions returns all migration versions already applied to the database.
func (m *Migrator) GetAppliedVersions(ctx context.Context) (map[int]bool, error) {
	if err := m.EnsureSchemaMigrationsTable(ctx); err != nil {
		return nil, err
	}

	rows, err := m.db.QueryContext(ctx, "SELECT version FROM schema_migrations ORDER BY version ASC;")
	if err != nil {
		return nil, fmt.Errorf("failed to query applied migration versions: %w", err)
	}
	defer rows.Close()

	applied := make(map[int]bool)
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		applied[v] = true
	}
	return applied, rows.Err()
}

// DiscoverMigrations parses migration files embedded in the migrations package.
func DiscoverMigrations() ([]Migration, error) {
	entries, err := fs.ReadDir(migrations.FS, ".")
	if err != nil {
		return nil, fmt.Errorf("failed to read embedded migrations: %w", err)
	}

	migMap := make(map[int]*Migration)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}

		parts := strings.SplitN(name, "_", 2)
		if len(parts) < 2 {
			continue
		}

		v, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}

		mig, ok := migMap[v]
		if !ok {
			mig = &Migration{Version: v}
			migMap[v] = mig
		}

		if strings.HasSuffix(name, ".up.sql") {
			mig.UpFile = name
			mig.Name = strings.TrimSuffix(name, ".up.sql")
		} else if strings.HasSuffix(name, ".down.sql") {
			mig.DownFile = name
		}
	}

	var list []Migration
	for _, mig := range migMap {
		list = append(list, *mig)
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i].Version < list[j].Version
	})

	return list, nil
}

// Up applies all pending migrations in ascending version order.
func (m *Migrator) Up(ctx context.Context) error {
	if err := m.EnsureSchemaMigrationsTable(ctx); err != nil {
		return err
	}

	applied, err := m.GetAppliedVersions(ctx)
	if err != nil {
		return err
	}

	allMigrations, err := DiscoverMigrations()
	if err != nil {
		return err
	}

	for _, mig := range allMigrations {
		if applied[mig.Version] {
			continue
		}
		if mig.UpFile == "" {
			return fmt.Errorf("missing up migration file for version %d", mig.Version)
		}

		content, err := fs.ReadFile(migrations.FS, mig.UpFile)
		if err != nil {
			return fmt.Errorf("failed to read up migration file %s: %w", mig.UpFile, err)
		}

		tx, err := m.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("failed to begin transaction for migration %s: %w", mig.Name, err)
		}

		if _, err := tx.ExecContext(ctx, string(content)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("failed to apply migration %s: %w", mig.Name, err)
		}

		if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations (version, name) VALUES ($1, $2);", mig.Version, mig.Name); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("failed to record migration %s in schema_migrations: %w", mig.Name, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit migration %s: %w", mig.Name, err)
		}
	}

	return nil
}

// Down rolls back all applied migrations in descending version order.
func (m *Migrator) Down(ctx context.Context) error {
	if err := m.EnsureSchemaMigrationsTable(ctx); err != nil {
		return err
	}

	applied, err := m.GetAppliedVersions(ctx)
	if err != nil {
		return err
	}

	allMigrations, err := DiscoverMigrations()
	if err != nil {
		return err
	}

	// Reverse order for rollback
	for i := len(allMigrations) - 1; i >= 0; i-- {
		mig := allMigrations[i]
		if !applied[mig.Version] {
			continue
		}

		if mig.DownFile == "" {
			return fmt.Errorf("missing down migration file for version %d", mig.Version)
		}

		content, err := fs.ReadFile(migrations.FS, mig.DownFile)
		if err != nil {
			return fmt.Errorf("failed to read down migration file %s: %w", mig.DownFile, err)
		}

		tx, err := m.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("failed to begin transaction for down migration %s: %w", mig.Name, err)
		}

		if _, err := tx.ExecContext(ctx, string(content)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("failed to execute rollback for migration %s: %w", mig.Name, err)
		}

		if _, err := tx.ExecContext(ctx, "DELETE FROM schema_migrations WHERE version = $1;", mig.Version); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("failed to delete migration %s from schema_migrations: %w", mig.Name, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit rollback of migration %s: %w", mig.Name, err)
		}
	}

	return nil
}

// MigrateTo migrates forward or backward to reach the specified targetVersion.
func (m *Migrator) MigrateTo(ctx context.Context, targetVersion int) error {
	if err := m.EnsureSchemaMigrationsTable(ctx); err != nil {
		return err
	}

	applied, err := m.GetAppliedVersions(ctx)
	if err != nil {
		return err
	}

	allMigrations, err := DiscoverMigrations()
	if err != nil {
		return err
	}

	// Forward migration up to targetVersion
	for _, mig := range allMigrations {
		if mig.Version <= targetVersion && !applied[mig.Version] {
			if mig.UpFile == "" {
				return fmt.Errorf("missing up migration file for version %d", mig.Version)
			}
			content, err := fs.ReadFile(migrations.FS, mig.UpFile)
			if err != nil {
				return err
			}
			tx, err := m.db.BeginTx(ctx, nil)
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, string(content)); err != nil {
				_ = tx.Rollback()
				return err
			}
			if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations (version, name) VALUES ($1, $2);", mig.Version, mig.Name); err != nil {
				_ = tx.Rollback()
				return err
			}
			if err := tx.Commit(); err != nil {
				return err
			}
		}
	}

	// Rollback if applied above targetVersion
	for i := len(allMigrations) - 1; i >= 0; i-- {
		mig := allMigrations[i]
		if mig.Version > targetVersion && applied[mig.Version] {
			if mig.DownFile == "" {
				return fmt.Errorf("missing down migration file for version %d", mig.Version)
			}
			content, err := fs.ReadFile(migrations.FS, mig.DownFile)
			if err != nil {
				return err
			}
			tx, err := m.db.BeginTx(ctx, nil)
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, string(content)); err != nil {
				_ = tx.Rollback()
				return err
			}
			if _, err := tx.ExecContext(ctx, "DELETE FROM schema_migrations WHERE version = $1;", mig.Version); err != nil {
				_ = tx.Rollback()
				return err
			}
			if err := tx.Commit(); err != nil {
				return err
			}
		}
	}

	return nil
}

