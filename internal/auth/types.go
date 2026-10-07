package auth

import (
	"errors"
	"time"
)

var (
	// ErrDuplicateEmail is returned when attempting to register an email that already exists.
	ErrDuplicateEmail = errors.New("email already registered")
	// ErrInvalidCredentials is returned when login authentication fails (generic to avoid enumeration).
	ErrInvalidCredentials = errors.New("invalid email or password")
	// ErrUserNotFound is returned when a user record does not exist.
	ErrUserNotFound = errors.New("user not found")
	// ErrInvalidInput is returned when input validation fails.
	ErrInvalidInput = errors.New("invalid input")
	// ErrInvalidToken is returned when a JWT token is missing, malformed, or invalid.
	ErrInvalidToken = errors.New("invalid or expired token")
)

// User represents the authoritative user entity from PostgreSQL.
type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"` // Never serialized in JSON
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// UserResponse represents the public user information returned to clients.
type UserResponse struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

// RegisterRequest represents the registration payload.
type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// LoginRequest represents the login payload.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// AuthResponse represents the authentication result containing user details and JWT.
type AuthResponse struct {
	User  UserResponse `json:"user"`
	Token string       `json:"token"`
}
