package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/lib/pq"
)

// UserRepository defines database access methods for user entities.
type UserRepository interface {
	CreateUser(ctx context.Context, email, passwordHash string) (*User, error)
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	GetUserByID(ctx context.Context, id string) (*User, error)
}

// PostgresUserRepository implements UserRepository using PostgreSQL.
type PostgresUserRepository struct {
	db *sql.DB
}

// NewPostgresUserRepository constructs a new PostgresUserRepository.
func NewPostgresUserRepository(db *sql.DB) *PostgresUserRepository {
	return &PostgresUserRepository{db: db}
}

// CreateUser inserts a new user record with hashed password.
func (r *PostgresUserRepository) CreateUser(ctx context.Context, email, passwordHash string) (*User, error) {
	query := `
		INSERT INTO users (email, password_hash, created_at, updated_at)
		VALUES ($1, $2, NOW(), NOW())
		RETURNING id, email, password_hash, created_at, updated_at;
	`

	u := &User{}
	err := r.db.QueryRowContext(ctx, query, email, passwordHash).Scan(
		&u.ID,
		&u.Email,
		&u.PasswordHash,
		&u.CreatedAt,
		&u.UpdatedAt,
	)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			// Unique constraint violation (duplicate email)
			return nil, ErrDuplicateEmail
		}
		return nil, fmt.Errorf("failed to insert user: %w", err)
	}

	return u, nil
}

// GetUserByEmail queries a user record by email.
func (r *PostgresUserRepository) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	query := `
		SELECT id, email, password_hash, created_at, updated_at
		FROM users
		WHERE LOWER(email) = LOWER($1);
	`

	u := &User{}
	err := r.db.QueryRowContext(ctx, query, strings.TrimSpace(email)).Scan(
		&u.ID,
		&u.Email,
		&u.PasswordHash,
		&u.CreatedAt,
		&u.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to query user by email: %w", err)
	}

	return u, nil
}

// GetUserByID queries a user record by ID.
func (r *PostgresUserRepository) GetUserByID(ctx context.Context, id string) (*User, error) {
	query := `
		SELECT id, email, password_hash, created_at, updated_at
		FROM users
		WHERE id = $1;
	`

	u := &User{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&u.ID,
		&u.Email,
		&u.PasswordHash,
		&u.CreatedAt,
		&u.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to query user by id: %w", err)
	}

	return u, nil
}
