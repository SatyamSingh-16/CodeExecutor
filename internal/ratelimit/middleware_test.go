package ratelimit

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/SatyamSingh-16/code_executor/internal/auth"
)

type mockLimiter struct {
	allowFunc func(ctx context.Context, identity string) (Result, error)
}

func (m *mockLimiter) Allow(ctx context.Context, identity string) (Result, error) {
	if m.allowFunc != nil {
		return m.allowFunc(ctx, identity)
	}
	return Result{Allowed: true, Limit: 20, Remaining: 19}, nil
}

func TestRequireRateLimit_Allowed(t *testing.T) {
	limiter := &mockLimiter{
		allowFunc: func(ctx context.Context, identity string) (Result, error) {
			if identity != "user-abc" {
				t.Errorf("expected identity 'user-abc', got %q", identity)
			}
			return Result{
				Allowed:   true,
				Limit:     20,
				Remaining: 15,
			}, nil
		},
	}
	mw := NewMiddleware(limiter)

	var nextCalled bool
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	handler := mw.RequireRateLimit(next)

	req := httptest.NewRequest(http.MethodPost, "/api/submissions", nil)
	req = req.WithContext(auth.WithUserID(req.Context(), "user-abc"))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
	if !nextCalled {
		t.Error("expected next handler to be called")
	}
	if rec.Header().Get("X-RateLimit-Limit") != "20" {
		t.Errorf("expected X-RateLimit-Limit=20, got %q", rec.Header().Get("X-RateLimit-Limit"))
	}
	if rec.Header().Get("X-RateLimit-Remaining") != "15" {
		t.Errorf("expected X-RateLimit-Remaining=15, got %q", rec.Header().Get("X-RateLimit-Remaining"))
	}
}

func TestRequireRateLimit_Exceeded_429(t *testing.T) {
	limiter := &mockLimiter{
		allowFunc: func(ctx context.Context, identity string) (Result, error) {
			return Result{
				Allowed:    false,
				Limit:      20,
				Remaining:  0,
				RetryAfter: 12 * time.Second,
			}, nil
		},
	}
	mw := NewMiddleware(limiter)

	var nextCalled bool
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
	})

	handler := mw.RequireRateLimit(next)

	req := httptest.NewRequest(http.MethodPost, "/api/submissions", nil)
	req = req.WithContext(auth.WithUserID(req.Context(), "user-rate-limited"))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("expected status 429 Too Many Requests, got %d", rec.Code)
	}
	if nextCalled {
		t.Error("expected next handler NOT to be called when rate limit exceeded")
	}
	if rec.Header().Get("Retry-After") != "12" {
		t.Errorf("expected Retry-After=12, got %q", rec.Header().Get("Retry-After"))
	}
	if rec.Header().Get("X-RateLimit-Remaining") != "0" {
		t.Errorf("expected X-RateLimit-Remaining=0, got %q", rec.Header().Get("X-RateLimit-Remaining"))
	}
}

func TestRequireRateLimit_Unauthenticated_401(t *testing.T) {
	limiter := &mockLimiter{}
	mw := NewMiddleware(limiter)

	var nextCalled bool
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
	})

	handler := mw.RequireRateLimit(next)

	// No user ID in request context
	req := httptest.NewRequest(http.MethodPost, "/api/submissions", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401 Unauthorized for unauthenticated request, got %d", rec.Code)
	}
	if nextCalled {
		t.Error("expected next handler NOT to be called")
	}
}

func TestRequireRateLimit_FailClosed_500(t *testing.T) {
	limiter := &mockLimiter{
		allowFunc: func(ctx context.Context, identity string) (Result, error) {
			return Result{}, errors.New("redis: connection timeout")
		},
	}
	mw := NewMiddleware(limiter)

	var nextCalled bool
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
	})

	handler := mw.RequireRateLimit(next)

	req := httptest.NewRequest(http.MethodPost, "/api/submissions", nil)
	req = req.WithContext(auth.WithUserID(req.Context(), "user-redis-down"))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Fail-closed policy: reject with 500
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500 on Redis failure (fail-closed), got %d", rec.Code)
	}
	if nextCalled {
		t.Error("expected next handler NOT to be called on fail-closed error")
	}
}
