package auth

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type mockUserRepo struct {
	mu    sync.Mutex
	users map[string]*User // keyed by lower(email)
	byID  map[string]*User // keyed by ID
}

func newMockUserRepo() *mockUserRepo {
	return &mockUserRepo{
		users: make(map[string]*User),
		byID:  make(map[string]*User),
	}
}

func (m *mockUserRepo) CreateUser(ctx context.Context, email, passwordHash string) (*User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	norm := strings.ToLower(email)
	if _, exists := m.users[norm]; exists {
		return nil, ErrDuplicateEmail
	}

	u := &User{
		ID:           "generated-uuid-" + norm,
		Email:        norm,
		PasswordHash: passwordHash,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	m.users[norm] = u
	m.byID[u.ID] = u
	return u, nil
}

func (m *mockUserRepo) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	norm := strings.ToLower(email)
	u, exists := m.users[norm]
	if !exists {
		return nil, ErrUserNotFound
	}
	return u, nil
}

func (m *mockUserRepo) GetUserByID(ctx context.Context, id string) (*User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	u, exists := m.byID[id]
	if !exists {
		return nil, ErrUserNotFound
	}
	return u, nil
}

func TestAuthService_Register_Success(t *testing.T) {
	repo := newMockUserRepo()
	tokenMgr, _ := NewJWTTokenManager(JWTConfig{Secret: "test-secret", Expiration: time.Hour})
	svc := NewService(repo, tokenMgr)

	req := RegisterRequest{
		Email:    "Alice@Example.COM",
		Password: "SecurePassword123!",
	}

	res, err := svc.Register(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected registration error: %v", err)
	}

	if res.User.Email != "alice@example.com" {
		t.Errorf("expected normalized email alice@example.com, got %q", res.User.Email)
	}
	if res.User.ID == "" {
		t.Error("expected non-empty user ID")
	}
	if res.Token == "" {
		t.Error("expected non-empty JWT token")
	}

	// Verify plaintext password was NOT stored and password_hash is valid bcrypt
	repo.mu.Lock()
	storedUser := repo.users["alice@example.com"]
	repo.mu.Unlock()

	if storedUser == nil {
		t.Fatal("user was not saved in repo")
	}
	if storedUser.PasswordHash == "SecurePassword123!" {
		t.Fatal("CRITICAL: plaintext password was stored directly in database!")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(storedUser.PasswordHash), []byte("SecurePassword123!")); err != nil {
		t.Errorf("stored hash is not a valid bcrypt hash of the password: %v", err)
	}
}

func TestAuthService_Register_DuplicateEmail(t *testing.T) {
	repo := newMockUserRepo()
	tokenMgr, _ := NewJWTTokenManager(JWTConfig{Secret: "test-secret", Expiration: time.Hour})
	svc := NewService(repo, tokenMgr)

	req := RegisterRequest{Email: "bob@example.com", Password: "Password123"}
	_, err := svc.Register(context.Background(), req)
	if err != nil {
		t.Fatalf("first registration failed: %v", err)
	}

	// Second registration with same email (different case)
	req2 := RegisterRequest{Email: "BOB@example.com", Password: "Password456"}
	_, err = svc.Register(context.Background(), req2)
	if err == nil {
		t.Fatal("expected error on duplicate email registration, got nil")
	}
	if !errors.Is(err, ErrDuplicateEmail) {
		t.Errorf("expected ErrDuplicateEmail, got: %v", err)
	}
}

func TestAuthService_Register_InvalidInputs(t *testing.T) {
	repo := newMockUserRepo()
	tokenMgr, _ := NewJWTTokenManager(JWTConfig{Secret: "test-secret", Expiration: time.Hour})
	svc := NewService(repo, tokenMgr)

	testCases := []struct {
		name     string
		email    string
		password string
	}{
		{"empty email", "", "Password123"},
		{"invalid email format", "not-an-email", "Password123"},
		{"missing domain", "user@", "Password123"},
		{"password too short", "valid@example.com", "short"},
		{"empty password", "valid@example.com", ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Register(context.Background(), RegisterRequest{Email: tc.email, Password: tc.password})
			if err == nil {
				t.Fatalf("expected error for case %q, got nil", tc.name)
			}
			if !errors.Is(err, ErrInvalidInput) {
				t.Errorf("expected ErrInvalidInput, got: %v", err)
			}
		})
	}
}

func TestAuthService_Login_Success(t *testing.T) {
	repo := newMockUserRepo()
	tokenMgr, _ := NewJWTTokenManager(JWTConfig{Secret: "test-secret", Expiration: time.Hour})
	svc := NewService(repo, tokenMgr)

	_, err := svc.Register(context.Background(), RegisterRequest{
		Email:    "charlie@example.com",
		Password: "MySecretPassword123",
	})
	if err != nil {
		t.Fatalf("registration failed: %v", err)
	}

	// Successful login
	loginRes, err := svc.Login(context.Background(), LoginRequest{
		Email:    "CHARLIE@example.com", // Case insensitive
		Password: "MySecretPassword123",
	})
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}

	if loginRes.User.Email != "charlie@example.com" {
		t.Errorf("expected email charlie@example.com, got %q", loginRes.User.Email)
	}
	if loginRes.Token == "" {
		t.Error("expected non-empty JWT token")
	}

	// Verify token contains correct user ID
	uid, err := tokenMgr.ValidateToken(loginRes.Token)
	if err != nil || uid != loginRes.User.ID {
		t.Errorf("token validation failed or mismatched ID: %v, %q", err, uid)
	}
}

func TestAuthService_Login_GenericAuthenticationFailure(t *testing.T) {
	repo := newMockUserRepo()
	tokenMgr, _ := NewJWTTokenManager(JWTConfig{Secret: "test-secret", Expiration: time.Hour})
	svc := NewService(repo, tokenMgr)

	_, _ = svc.Register(context.Background(), RegisterRequest{
		Email:    "existing@example.com",
		Password: "CorrectPassword123",
	})

	testCases := []struct {
		name     string
		email    string
		password string
	}{
		{"wrong password", "existing@example.com", "WrongPassword123"},
		{"non-existent email", "nonexistent@example.com", "SomePassword123"},
		{"empty email", "", "Password123"},
		{"empty password", "existing@example.com", ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Login(context.Background(), LoginRequest{Email: tc.email, Password: tc.password})
			if err == nil {
				t.Fatalf("expected login failure for case %q, got nil", tc.name)
			}
			// Must return generic ErrInvalidCredentials in both wrong-password and unknown-email cases
			if !errors.Is(err, ErrInvalidCredentials) {
				t.Errorf("expected generic ErrInvalidCredentials, got: %v", err)
			}
		})
	}
}

func TestAuthService_GetUser(t *testing.T) {
	repo := newMockUserRepo()
	tokenMgr, _ := NewJWTTokenManager(JWTConfig{Secret: "test-secret", Expiration: time.Hour})
	svc := NewService(repo, tokenMgr)

	reg, _ := svc.Register(context.Background(), RegisterRequest{
		Email:    "user@example.com",
		Password: "Password123",
	})

	user, err := svc.GetUser(context.Background(), reg.User.ID)
	if err != nil {
		t.Fatalf("failed to get user: %v", err)
	}
	if user.Email != "user@example.com" {
		t.Errorf("expected email user@example.com, got %q", user.Email)
	}

	_, err = svc.GetUser(context.Background(), "unknown-id")
	if !errors.Is(err, ErrUserNotFound) {
		t.Errorf("expected ErrUserNotFound, got %v", err)
	}
}
