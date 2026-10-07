package auth

import (
	"encoding/json"
	"net/http"
	"strings"
)

// Middleware provides HTTP middleware for JWT authentication.
type Middleware struct {
	tokenManager TokenManager
}

// NewMiddleware constructs a new auth Middleware.
func NewMiddleware(tokenManager TokenManager) *Middleware {
	return &Middleware{tokenManager: tokenManager}
}

// RequireAuth wraps an http.Handler with JWT authentication.
// It enforces the Bearer token scheme and injects the authenticated user ID into request context.
func (m *Middleware) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			writeJSONError(w, http.StatusUnauthorized, "missing authorization header")
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			writeJSONError(w, http.StatusUnauthorized, "invalid authorization header format")
			return
		}

		tokenStr := strings.TrimSpace(parts[1])
		if tokenStr == "" {
			writeJSONError(w, http.StatusUnauthorized, "missing token")
			return
		}

		userID, err := m.tokenManager.ValidateToken(tokenStr)
		if err != nil {
			writeJSONError(w, http.StatusUnauthorized, "invalid or expired token")
			return
		}

		ctx := WithUserID(r.Context(), userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
