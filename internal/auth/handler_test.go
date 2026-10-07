package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func setupTestHandler() (*Handler, *Service, *mockUserRepo) {
	repo := newMockUserRepo()
	tokenMgr, _ := NewJWTTokenManager(JWTConfig{Secret: "test-secret-123", Expiration: time.Hour})
	service := NewService(repo, tokenMgr)
	handler := NewHandler(service)
	return handler, service, repo
}

func TestHandler_Register_Success(t *testing.T) {
	h, _, _ := setupTestHandler()

	body, _ := json.Marshal(RegisterRequest{
		Email:    "newuser@example.com",
		Password: "SecurePassword123",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h.Register(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201 Created, got %d. Body: %s", rec.Code, rec.Body.String())
	}

	var resp AuthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.User.Email != "newuser@example.com" {
		t.Errorf("expected email newuser@example.com, got %q", resp.User.Email)
	}
	if resp.Token == "" {
		t.Error("expected non-empty token")
	}
}

func TestHandler_Register_DuplicateEmail_409(t *testing.T) {
	h, _, _ := setupTestHandler()

	body, _ := json.Marshal(RegisterRequest{Email: "dup@example.com", Password: "Password123"})
	req1 := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewReader(body))
	rec1 := httptest.NewRecorder()
	h.Register(rec1, req1)

	// Second attempt with same email
	req2 := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewReader(body))
	rec2 := httptest.NewRecorder()
	h.Register(rec2, req2)

	if rec2.Code != http.StatusConflict {
		t.Fatalf("expected status 409 Conflict, got %d. Body: %s", rec2.Code, rec2.Body.String())
	}

	var errResp map[string]string
	_ = json.Unmarshal(rec2.Body.Bytes(), &errResp)
	if errResp["error"] != "email already registered" {
		t.Errorf("expected 'email already registered', got %q", errResp["error"])
	}
}

func TestHandler_Register_InvalidInputs_400(t *testing.T) {
	h, _, _ := setupTestHandler()

	testCases := []struct {
		name string
		json string
	}{
		{"invalid json", "{bad-json"},
		{"unknown fields", `{"email":"a@b.com","password":"Password123","admin":true}`},
		{"short password", `{"email":"a@b.com","password":"short"}`},
		{"invalid email", `{"email":"not-an-email","password":"Password123"}`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewReader([]byte(tc.json)))
			rec := httptest.NewRecorder()

			h.Register(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected status 400 Bad Request for %s, got %d. Body: %s", tc.name, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestHandler_Register_MethodNotAllowed(t *testing.T) {
	h, _, _ := setupTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/auth/register", nil)
	rec := httptest.NewRecorder()

	h.Register(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405 Method Not Allowed, got %d", rec.Code)
	}
}

func TestHandler_Login_Success(t *testing.T) {
	h, svc, _ := setupTestHandler()

	_, _ = svc.Register(context.Background(), RegisterRequest{
		Email:    "loginuser@example.com",
		Password: "CorrectPassword123",
	})

	body, _ := json.Marshal(LoginRequest{
		Email:    "loginuser@example.com",
		Password: "CorrectPassword123",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h.Login(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d. Body: %s", rec.Code, rec.Body.String())
	}

	var resp AuthResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Token == "" {
		t.Error("expected non-empty token")
	}
}

func TestHandler_Login_InvalidCredentials_401(t *testing.T) {
	h, svc, _ := setupTestHandler()

	_, _ = svc.Register(context.Background(), RegisterRequest{
		Email:    "user@example.com",
		Password: "CorrectPassword123",
	})

	testCases := []struct {
		name     string
		email    string
		password string
	}{
		{"wrong password", "user@example.com", "WrongPassword"},
		{"unknown email", "unknown@example.com", "CorrectPassword123"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(LoginRequest{Email: tc.email, Password: tc.password})
			req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
			rec := httptest.NewRecorder()

			h.Login(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("expected status 401 Unauthorized for %s, got %d. Body: %s", tc.name, rec.Code, rec.Body.String())
			}

			var errResp map[string]string
			_ = json.Unmarshal(rec.Body.Bytes(), &errResp)
			if errResp["error"] != "invalid email or password" {
				t.Errorf("expected generic error 'invalid email or password', got %q", errResp["error"])
			}
		})
	}
}

func TestHandler_Me_Endpoint(t *testing.T) {
	h, svc, _ := setupTestHandler()

	reg, _ := svc.Register(context.Background(), RegisterRequest{
		Email:    "me@example.com",
		Password: "Password123",
	})

	// 1. Authenticated request with user ID in context
	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	ctx := WithUserID(req.Context(), reg.User.ID)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	h.Me(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d. Body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]UserResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["user"].Email != "me@example.com" {
		t.Errorf("expected email me@example.com, got %q", resp["user"].Email)
	}

	// 2. Unauthenticated request (no user ID in context)
	unauthReq := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	unauthRec := httptest.NewRecorder()

	h.Me(unauthRec, unauthReq)

	if unauthRec.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401 Unauthorized for missing context, got %d", unauthRec.Code)
	}
}
