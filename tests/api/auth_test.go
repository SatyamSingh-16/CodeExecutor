//go:build integration

package api_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/SatyamSingh-16/code_executor/internal/auth"
	"github.com/SatyamSingh-16/code_executor/internal/database"
	_ "github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
)

func getIntegrationDB(t *testing.T) *sql.DB {
	t.Helper()
	urls := []string{
		os.Getenv("DATABASE_URL"),
		"postgres://satyamsingh2730@localhost:5432/code_execution_test_db?sslmode=disable",
		"postgres://postgres@localhost:5432/code_execution_test_db?sslmode=disable",
		"postgres://localhost:5432/code_execution_test_db?sslmode=disable",
	}

	var db *sql.DB
	for _, u := range urls {
		if u == "" {
			continue
		}
		cand, err := sql.Open("postgres", u)
		if err != nil {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err = cand.PingContext(ctx)
		cancel()
		if err == nil {
			db = cand
			break
		}
		_ = cand.Close()
	}

	if db == nil {
		t.Skip("skipping integration test: PostgreSQL test DB unavailable")
	}

	migrator := database.NewMigrator(db)
	if err := migrator.Up(context.Background()); err != nil {
		t.Fatalf("failed to apply migrations: %v", err)
	}

	return db
}

func setupAPITestServer(t *testing.T, db *sql.DB) (http.Handler, *auth.JWTTokenManager) {
	t.Helper()
	tokenMgr, err := auth.NewJWTTokenManager(auth.JWTConfig{
		Secret:     "integration-test-secret-32-chars-long!",
		Expiration: 2 * time.Hour,
	})
	if err != nil {
		t.Fatalf("failed to create token manager: %v", err)
	}

	repo := auth.NewPostgresUserRepository(db)
	svc := auth.NewService(repo, tokenMgr)
	handler := auth.NewHandler(svc)
	middleware := auth.NewMiddleware(tokenMgr)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/register", handler.Register)
	mux.HandleFunc("/api/auth/login", handler.Login)
	mux.Handle("/api/auth/me", middleware.RequireAuth(http.HandlerFunc(handler.Me)))

	return mux, tokenMgr
}

func TestIntegration_Auth_RegisterAndLogin(t *testing.T) {
	db := getIntegrationDB(t)
	defer db.Close()

	router, _ := setupAPITestServer(t, db)

	email := fmt.Sprintf("testuser_%d@example.com", time.Now().UnixNano())
	password := "CorrectHorseBatteryStaple123!"

	// 1. Register User via POST /api/auth/register
	regPayload, _ := json.Marshal(auth.RegisterRequest{
		Email:    email,
		Password: password,
	})
	regReq := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewReader(regPayload))
	regRec := httptest.NewRecorder()
	router.ServeHTTP(regRec, regReq)

	if regRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d. Body: %s", regRec.Code, regRec.Body.String())
	}

	var regResp auth.AuthResponse
	if err := json.Unmarshal(regRec.Body.Bytes(), &regResp); err != nil {
		t.Fatalf("failed to parse register response: %v", err)
	}

	if regResp.User.Email != email {
		t.Errorf("expected email %q, got %q", email, regResp.User.Email)
	}
	if regResp.User.ID == "" {
		t.Error("expected non-empty user ID")
	}
	if regResp.Token == "" {
		t.Error("expected non-empty JWT token")
	}

	// 2. Direct PostgreSQL Inspection: Verify plaintext password was never stored
	var storedHash string
	err := db.QueryRow("SELECT password_hash FROM users WHERE id = $1;", regResp.User.ID).Scan(&storedHash)
	if err != nil {
		t.Fatalf("failed to query stored password hash: %v", err)
	}

	if storedHash == password {
		t.Fatal("CRITICAL SECURITY VIOLATION: plaintext password stored in database!")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(password)); err != nil {
		t.Fatalf("stored password is not a valid bcrypt hash of the password: %v", err)
	}

	// 3. Login with Correct Credentials: POST /api/auth/login
	loginPayload, _ := json.Marshal(auth.LoginRequest{
		Email:    email,
		Password: password,
	})
	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(loginPayload))
	loginRec := httptest.NewRecorder()
	router.ServeHTTP(loginRec, loginReq)

	if loginRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for valid login, got %d. Body: %s", loginRec.Code, loginRec.Body.String())
	}

	var loginResp auth.AuthResponse
	if err := json.Unmarshal(loginRec.Body.Bytes(), &loginResp); err != nil {
		t.Fatalf("failed to parse login response: %v", err)
	}
	if loginResp.User.ID != regResp.User.ID {
		t.Errorf("expected user ID %q, got %q", regResp.User.ID, loginResp.User.ID)
	}
	if loginResp.Token == "" {
		t.Error("expected non-empty JWT token on login")
	}

	// 4. Login with Wrong Password: POST /api/auth/login
	wrongPwPayload, _ := json.Marshal(auth.LoginRequest{
		Email:    email,
		Password: "IncorrectPassword456!",
	})
	wrongReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(wrongPwPayload))
	wrongRec := httptest.NewRecorder()
	router.ServeHTTP(wrongRec, wrongReq)

	if wrongRec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized for wrong password, got %d", wrongRec.Code)
	}

	// 5. Login with Non-Existent Email: POST /api/auth/login (generic failure)
	unknownEmailPayload, _ := json.Marshal(auth.LoginRequest{
		Email:    "unknown_ghost_user@example.com",
		Password: "SomePassword123!",
	})
	unknownReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(unknownEmailPayload))
	unknownRec := httptest.NewRecorder()
	router.ServeHTTP(unknownRec, unknownReq)

	if unknownRec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized for non-existent email, got %d", unknownRec.Code)
	}

	// Verify both 401 responses return identical generic error messages
	var wrongMsg, unknownMsg map[string]string
	_ = json.Unmarshal(wrongRec.Body.Bytes(), &wrongMsg)
	_ = json.Unmarshal(unknownRec.Body.Bytes(), &unknownMsg)
	if wrongMsg["error"] != "invalid email or password" || unknownMsg["error"] != "invalid email or password" {
		t.Errorf("expected generic error message 'invalid email or password', got %q and %q",
			wrongMsg["error"], unknownMsg["error"])
	}
}

func TestIntegration_Auth_DuplicateRegistrationConflict(t *testing.T) {
	db := getIntegrationDB(t)
	defer db.Close()

	router, _ := setupAPITestServer(t, db)

	email := fmt.Sprintf("conflict_user_%d@example.com", time.Now().UnixNano())
	payload, _ := json.Marshal(auth.RegisterRequest{
		Email:    email,
		Password: "Password12345!",
	})

	// Initial registration
	req1 := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewReader(payload))
	rec1 := httptest.NewRecorder()
	router.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusCreated {
		t.Fatalf("first registration failed: %d", rec1.Code)
	}

	// Duplicate registration attempt (using uppercase variant of email)
	upperPayload, _ := json.Marshal(auth.RegisterRequest{
		Email:    email,
		Password: "DifferentPassword123!",
	})
	req2 := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewReader(upperPayload))
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict on duplicate registration, got %d. Body: %s", rec2.Code, rec2.Body.String())
	}

	var errResp map[string]string
	_ = json.Unmarshal(rec2.Body.Bytes(), &errResp)
	if errResp["error"] != "email already registered" {
		t.Errorf("expected 'email already registered', got %q", errResp["error"])
	}
}

func TestIntegration_Auth_ProtectedEndpoint(t *testing.T) {
	db := getIntegrationDB(t)
	defer db.Close()

	router, _ := setupAPITestServer(t, db)

	// Register user to obtain valid token
	email := fmt.Sprintf("protected_%d@example.com", time.Now().UnixNano())
	regPayload, _ := json.Marshal(auth.RegisterRequest{
		Email:    email,
		Password: "SecurePassword123!",
	})
	regReq := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewReader(regPayload))
	regRec := httptest.NewRecorder()
	router.ServeHTTP(regRec, regReq)

	var regResp auth.AuthResponse
	_ = json.Unmarshal(regRec.Body.Bytes(), &regResp)

	// 1. Call GET /api/auth/me without Authorization header -> 401
	unauthReq := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	unauthRec := httptest.NewRecorder()
	router.ServeHTTP(unauthRec, unauthReq)
	if unauthRec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without auth header, got %d", unauthRec.Code)
	}

	// 2. Call GET /api/auth/me with invalid token -> 401
	badTokenReq := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	badTokenReq.Header.Set("Authorization", "Bearer invalid.token.value")
	badTokenRec := httptest.NewRecorder()
	router.ServeHTTP(badTokenRec, badTokenReq)
	if badTokenRec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 with invalid token, got %d", badTokenRec.Code)
	}

	// 3. Call GET /api/auth/me with valid Bearer token -> 200 OK
	authReq := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	authReq.Header.Set("Authorization", "Bearer "+regResp.Token)
	authRec := httptest.NewRecorder()
	router.ServeHTTP(authRec, authReq)

	if authRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK with valid token, got %d. Body: %s", authRec.Code, authRec.Body.String())
	}

	var meResp map[string]auth.UserResponse
	if err := json.Unmarshal(authRec.Body.Bytes(), &meResp); err != nil {
		t.Fatalf("failed to decode /api/auth/me response: %v", err)
	}
	if meResp["user"].ID != regResp.User.ID {
		t.Errorf("expected authenticated user ID %q, got %q", regResp.User.ID, meResp["user"].ID)
	}
	if meResp["user"].Email != email {
		t.Errorf("expected authenticated user email %q, got %q", email, meResp["user"].Email)
	}
}
