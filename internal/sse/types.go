package sse

import (
	"context"
	"time"

	"github.com/SatyamSingh-16/code_executor/internal/submission"
)

// DefaultHeartbeatInterval is the default interval for sending keepalive heartbeats.
const DefaultHeartbeatInterval = 15 * time.Second

// EventSubscriber defines the contract for subscribing to submission status events.
type EventSubscriber interface {
	// Subscribe opens a subscription to events for the given submission ID.
	Subscribe(ctx context.Context, submissionID string) (Subscription, error)
}

// Subscription represents an active subscription to a submission's event channel.
type Subscription interface {
	// Channel returns a channel that yields incoming messages.
	Channel() <-chan string
	// Close closes the subscription and releases underlying resources.
	Close() error
}

// SubmissionRepository defines the DB operations needed by the SSE handler.
type SubmissionRepository interface {
	GetByIDAndUserID(ctx context.Context, id, userID string) (*submission.SubmissionEntity, error)
}

// IsTerminalStatus returns true if the status represents a final execution outcome.
func IsTerminalStatus(status string) bool {
	switch status {
	case "SUCCESS",
		"COMPILATION_ERROR",
		"RUNTIME_ERROR",
		"TIME_LIMIT_EXCEEDED",
		"MEMORY_LIMIT_EXCEEDED",
		"SYSTEM_ERROR":
		return true
	default:
		return false
	}
}

// SubmissionEventPayload represents the JSON payload sent in the SSE event data.
// It matches the clean SubmissionDetailResponse structure without exposing internal Redis/Docker IDs.
type SubmissionEventPayload struct {
	ID                string    `json:"id"`
	Status            string    `json:"status"`
	Stdout            string    `json:"stdout"`
	Stderr            string    `json:"stderr"`
	CompilationOutput string    `json:"compilation_output"`
	StdoutTruncated   bool      `json:"stdout_truncated"`
	StderrTruncated   bool      `json:"stderr_truncated"`
	ExitCode          *int      `json:"exit_code"`
	ExecutionTimeMs   *int64    `json:"execution_time_ms"`
	MemoryUsageKb     *int64    `json:"memory_usage_kb"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// PayloadFromEntity converts a database entity into a client-safe event payload.
func PayloadFromEntity(entity *submission.SubmissionEntity) SubmissionEventPayload {
	return SubmissionEventPayload{
		ID:                entity.ID,
		Status:            entity.Status,
		Stdout:            entity.Stdout,
		Stderr:            entity.Stderr,
		CompilationOutput: entity.CompilationOutput,
		StdoutTruncated:   entity.StdoutTruncated,
		StderrTruncated:   entity.StderrTruncated,
		ExitCode:          entity.ExitCode,
		ExecutionTimeMs:   entity.ExecutionTimeMs,
		MemoryUsageKb:     entity.MemoryUsageKb,
		CreatedAt:         entity.CreatedAt,
		UpdatedAt:         entity.UpdatedAt,
	}
}
