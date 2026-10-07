package queue

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	// DefaultStreamName is the canonical Redis Stream for code execution submissions.
	DefaultStreamName = "submissions:stream"

	// DefaultConsumerGroup is the consumer group for distributed workers consuming submissions.
	DefaultConsumerGroup = "workers_group"

	// FieldSubmissionID is the primary durable key passed in the Redis Stream XADD payload.
	FieldSubmissionID = "submission_id"
)

// SubmissionQueue defines the queue abstraction for publishing submissions and managing stream setup.
// Upper layers depend on this interface rather than directly on the underlying Redis client.
type SubmissionQueue interface {
	// Enqueue adds a submission ID to the delivery stream.
	Enqueue(ctx context.Context, submissionID string) error

	// InitConsumerGroup creates the worker consumer group idempotently (MKSTREAM, ignores BUSYGROUP).
	InitConsumerGroup(ctx context.Context) error

	// Close cleanly closes any underlying network connections.
	Close() error
}

// RedisClient defines the subset of go-redis commands required by RedisQueue.
// *redis.Client satisfies this interface directly.
type RedisClient interface {
	XAdd(ctx context.Context, a *redis.XAddArgs) *redis.StringCmd
	XGroupCreateMkStream(ctx context.Context, stream, group, start string) *redis.StatusCmd
	Ping(ctx context.Context) *redis.StatusCmd
	Close() error
}

// StaleSubmissionFinder queries the persistent store for submissions stuck in QUEUED state.
type StaleSubmissionFinder interface {
	GetStaleQueuedSubmissions(ctx context.Context, olderThan time.Time, limit int) ([]string, error)
}

// SweepResult provides telemetry on a single reconciliation sweeper iteration.
type SweepResult struct {
	ScannedCount  int
	EnqueuedCount int
	FailedCount   int
	Errors        []error
}
