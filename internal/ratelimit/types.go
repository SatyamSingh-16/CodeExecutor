package ratelimit

import (
	"context"
	"time"
)

// Default limits according to spec: 20 submissions per minute
const (
	DefaultRateLimit   = 20
	DefaultWindow      = 1 * time.Minute
	DefaultKeyPrefix   = "submission_rate:"
)

// Result represents the outcome of a rate limiting evaluation.
type Result struct {
	Allowed    bool
	Limit      int
	Remaining  int
	RetryAfter time.Duration
}

// RateLimiter defines the interface for evaluating rate limits for an identity.
type RateLimiter interface {
	Allow(ctx context.Context, identity string) (Result, error)
}

// Config holds configuration parameters for the sliding-window rate limiter.
type Config struct {
	Limit     int           // Max requests allowed within Window (default: 20)
	Window    time.Duration // Sliding window duration (default: 1 minute)
	KeyPrefix string        // Redis key prefix (default: "submission_rate:")
}
