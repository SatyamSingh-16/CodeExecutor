package auth

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// AuthService defines user registration, login, and retrieval operations.
type AuthService interface {
	Register(ctx context.Context, req RegisterRequest) (*AuthResponse, error)
	Login(ctx context.Context, req LoginRequest) (*AuthResponse, error)
	GetUser(ctx context.Context, userID string) (*UserResponse, error)
}

// Service implements AuthService.
type Service struct {
	repo         UserRepository
	tokenManager TokenManager
}

// NewService constructs a new AuthService.
func NewService(repo UserRepository, tokenManager TokenManager) *Service {
	return &Service{
		repo:         repo,
		tokenManager: tokenManager,
	}
}

// Register validates credentials, hashes password using bcrypt, persists the user,
// and returns the public user profile with a signed JWT.
func (s *Service) Register(ctx context.Context, req RegisterRequest) (*AuthResponse, error) {
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if err := validateEmail(email); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	if len(req.Password) < 8 {
		return nil, fmt.Errorf("%w: password must be at least 8 characters", ErrInvalidInput)
	}

	// Hash password using bcrypt DefaultCost
	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	user, err := s.repo.CreateUser(ctx, email, string(hashedBytes))
	if err != nil {
		return nil, err
	}

	token, err := s.tokenManager.GenerateToken(user.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	return &AuthResponse{
		User: UserResponse{
			ID:        user.ID,
			Email:     user.Email,
			CreatedAt: user.CreatedAt,
		},
		Token: token,
	}, nil
}

// Login verifies user credentials using bcrypt and issues a signed JWT.
// Always returns generic ErrInvalidCredentials on failure to prevent user enumeration.
func (s *Service) Login(ctx context.Context, req LoginRequest) (*AuthResponse, error) {
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if email == "" || req.Password == "" {
		return nil, ErrInvalidCredentials
	}

	user, err := s.repo.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, fmt.Errorf("database query error: %w", err)
	}

	// Compare plaintext password with stored bcrypt hash
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	token, err := s.tokenManager.GenerateToken(user.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	return &AuthResponse{
		User: UserResponse{
			ID:        user.ID,
			Email:     user.Email,
			CreatedAt: user.CreatedAt,
		},
		Token: token,
	}, nil
}

// GetUser retrieves the public user profile by ID.
func (s *Service) GetUser(ctx context.Context, userID string) (*UserResponse, error) {
	if userID == "" {
		return nil, ErrInvalidInput
	}

	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &UserResponse{
		ID:        user.ID,
		Email:     user.Email,
		CreatedAt: user.CreatedAt,
	}, nil
}

func validateEmail(email string) error {
	if email == "" {
		return errors.New("email cannot be empty")
	}
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email {
		return errors.New("invalid email format")
	}
	return nil
}
