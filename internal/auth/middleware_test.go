package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestJWTMiddleware_RequireAuth(t *testing.T) {
	tokenMgr, _ := NewJWTTokenManager(JWTConfig{Secret: "test-secret-key", Expiration: time.Hour})
	mw := NewMiddleware(tokenMgr)

	validToken, _ := tokenMgr.GenerateToken("authenticated-user-42")

	expiredMgr, _ := NewJWTTokenManager(JWTConfig{Secret: "test-secret-key", Expiration: -1 * time.Second})
	expiredToken, _ := expiredMgr.GenerateToken("user-expired")

	wrongSecretMgr, _ := NewJWTTokenManager(JWTConfig{Secret: "wrong-secret-key", Expiration: time.Hour})
	wrongSecretToken, _ := wrongSecretMgr.GenerateToken("user-wrong-secret")

	testCases := []struct {
		name           string
		authHeader     string
		expectedStatus int
		expectCalled   bool
		expectedUserID string
	}{
		{
			name:           "valid Bearer token",
			authHeader:     "Bearer " + validToken,
			expectedStatus: http.StatusOK,
			expectCalled:   true,
			expectedUserID: "authenticated-user-42",
		},
		{
			name:           "missing Authorization header",
			authHeader:     "",
			expectedStatus: http.StatusUnauthorized,
			expectCalled:   false,
		},
		{
			name:           "missing Bearer prefix",
			authHeader:     validToken,
			expectedStatus: http.StatusUnauthorized,
			expectCalled:   false,
		},
		{
			name:           "Basic scheme instead of Bearer",
			authHeader:     "Basic dXNlcjpwYXNz",
			expectedStatus: http.StatusUnauthorized,
			expectCalled:   false,
		},
		{
			name:           "empty token after Bearer",
			authHeader:     "Bearer ",
			expectedStatus: http.StatusUnauthorized,
			expectCalled:   false,
		},
		{
			name:           "expired token",
			authHeader:     "Bearer " + expiredToken,
			expectedStatus: http.StatusUnauthorized,
			expectCalled:   false,
		},
		{
			name:           "token signed with wrong secret",
			authHeader:     "Bearer " + wrongSecretToken,
			expectedStatus: http.StatusUnauthorized,
			expectCalled:   false,
		},
		{
			name:           "garbage token string",
			authHeader:     "Bearer not.a.valid.jwt",
			expectedStatus: http.StatusUnauthorized,
			expectCalled:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var handlerCalled bool
			var extractedID string

			nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				handlerCalled = true
				if id, ok := UserIDFromContext(r.Context()); ok {
					extractedID = id
				}
				w.WriteHeader(http.StatusOK)
			})

			wrappedHandler := mw.RequireAuth(nextHandler)

			req := httptest.NewRequest(http.MethodGet, "/protected", nil)
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}
			rec := httptest.NewRecorder()

			wrappedHandler.ServeHTTP(rec, req)

			if rec.Code != tc.expectedStatus {
				t.Errorf("expected HTTP status %d, got %d", tc.expectedStatus, rec.Code)
			}
			if handlerCalled != tc.expectCalled {
				t.Errorf("expected handlerCalled=%v, got %v", tc.expectCalled, handlerCalled)
			}
			if tc.expectCalled && extractedID != tc.expectedUserID {
				t.Errorf("expected user ID %q in context, got %q", tc.expectedUserID, extractedID)
			}
		})
	}
}
