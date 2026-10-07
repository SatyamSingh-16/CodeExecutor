package ratelimit

import (
	"encoding/json"
	"log"
	"math"
	"net/http"
	"strconv"

	"github.com/SatyamSingh-16/code_executor/internal/auth"
)

// Middleware provides HTTP middleware for rate limiting authenticated users.
type Middleware struct {
	limiter RateLimiter
}

// NewMiddleware constructs a new rate limit Middleware.
func NewMiddleware(limiter RateLimiter) *Middleware {
	return &Middleware{limiter: limiter}
}

// RequireRateLimit wraps an http.Handler with per-user sliding window rate limiting.
// It extracts the authenticated user ID from context. If rate limits are exceeded,
// it responds with HTTP 429 Too Many Requests and a Retry-After header.
// On Redis failure, it applies a fail-closed policy (HTTP 500).
func (m *Middleware) RequireRateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok || userID == "" {
			writeJSONError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		res, err := m.limiter.Allow(r.Context(), userID)
		if err != nil {
			// Fail-closed security design: protect workers during Redis outages
			log.Printf("[ratelimit] rate limiter error for user %s: %v", userID, err)
			writeJSONError(w, http.StatusInternalServerError, "rate limit service unavailable")
			return
		}

		// Inject standard rate limit headers
		w.Header().Set("X-RateLimit-Limit", strconv.Itoa(res.Limit))
		w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(res.Remaining))

		if !res.Allowed {
			retrySeconds := int(math.Ceil(res.RetryAfter.Seconds()))
			if retrySeconds <= 0 {
				retrySeconds = 1
			}
			w.Header().Set("Retry-After", strconv.Itoa(retrySeconds))
			writeJSONError(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}

		next.ServeHTTP(w, r)
	})
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
