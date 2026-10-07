package worker

import (
	"context"
	"errors"
	"time"

	"github.com/SatyamSingh-16/code_executor/internal/runner"
)

var (
	// ErrSubmissionNotFound is returned when the submission record does not exist in PostgreSQL.
	ErrSubmissionNotFound = errors.New("submission not found")
)

// Submission represents the authoritative submission entity loaded from PostgreSQL.
type Submission struct {
	ID         string
	UserID     string
	Language   string
	Code       string
	Stdin      string
	Status     string
	RetryCount int
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// StreamMessage represents an asynchronous job message pulled from Redis Streams.
type StreamMessage struct {
	MessageID    string
	SubmissionID string
}

// StatusEvent represents the minimal payload published to Redis Pub/Sub for SSE/API updates.
type StatusEvent struct {
	SubmissionID string `json:"submission_id"`
	Status       string `json:"status"`
	Timestamp    string `json:"timestamp"`
}

// SubmissionRepository defines database operations for the worker execution lifecycle.
type SubmissionRepository interface {
	// GetSubmission retrieves the complete authoritative submission record from PostgreSQL.
	GetSubmission(ctx context.Context, id string) (*Submission, error)

	// ClaimSubmission atomically transitions the submission status from QUEUED to PROCESSING.
	// Returns true if the status transition was applied, or false if 0 rows were affected.
	ClaimSubmission(ctx context.Context, id string) (bool, error)

	// CompleteSubmission persists final execution status, captured outputs, and metrics to PostgreSQL.
	CompleteSubmission(ctx context.Context, id string, result *runner.ExecutionResult) error
}

// StreamConsumer defines the contract for reading from and acknowledging messages in Redis Streams.
type StreamConsumer interface {
	// ReadMessages reads a batch of pending/new messages from the stream consumer group.
	ReadMessages(ctx context.Context, count int64, block time.Duration) ([]StreamMessage, error)

	// AckMessage acknowledges a processed message via XACK.
	AckMessage(ctx context.Context, messageID string) error
}

// EventPublisher publishes status transition events to Redis Pub/Sub.
type EventPublisher interface {
	// PublishStatusEvent publishes a state change event to submissions:events:<id>.
	PublishStatusEvent(ctx context.Context, submissionID string, status string) error
}

// ExecutionRunner executes code inside an isolated container sandbox.
// *runner.DockerRunner satisfies this interface.
type ExecutionRunner interface {
	Execute(ctx context.Context, req runner.ExecutionRequest) (*runner.ExecutionResult, error)
}
