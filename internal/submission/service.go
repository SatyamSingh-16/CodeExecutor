package submission

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/SatyamSingh-16/code_executor/internal/queue"
)

// SubmissionService defines operations for creating, retrieving, and listing submissions.
type SubmissionService interface {
	CreateSubmission(ctx context.Context, userID string, req CreateSubmissionRequest) (*CreateSubmissionResponse, error)
	GetSubmission(ctx context.Context, userID, id string) (*SubmissionDetailResponse, error)
	ListSubmissions(ctx context.Context, userID string, limit, offset int) ([]*SubmissionDetailResponse, error)
}

// Service implements SubmissionService.
type Service struct {
	repo  SubmissionRepository
	queue queue.SubmissionQueue
}

// NewService constructs a new submission Service.
func NewService(repo SubmissionRepository, q queue.SubmissionQueue) *Service {
	return &Service{
		repo:  repo,
		queue: q,
	}
}

// CreateSubmission validates inputs, writes QUEUED record to PostgreSQL, and publishes to Redis Streams.
func (s *Service) CreateSubmission(ctx context.Context, userID string, req CreateSubmissionRequest) (*CreateSubmissionResponse, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, fmt.Errorf("%w: missing authenticated user ID", ErrInvalidInput)
	}

	lang := NormalizeLanguage(req.Language)
	if !IsValidLanguage(lang) {
		return nil, ErrUnsupportedLanguage
	}

	code := req.GetCode()
	if strings.TrimSpace(code) == "" {
		return nil, ErrEmptySourceCode
	}

	if len(code) > MaxPayloadSizeBytes {
		return nil, fmt.Errorf("%w: source code size %d exceeds limit of %d bytes", ErrPayloadTooLarge, len(code), MaxPayloadSizeBytes)
	}

	if len(req.Stdin) > MaxPayloadSizeBytes {
		return nil, fmt.Errorf("%w: stdin size %d exceeds limit of %d bytes", ErrPayloadTooLarge, len(req.Stdin), MaxPayloadSizeBytes)
	}

	// 1. Write-First: Persist to PostgreSQL with status QUEUED
	entity := &SubmissionEntity{
		UserID:   userID,
		Language: lang,
		Code:     code,
		Stdin:    req.Stdin,
	}

	created, err := s.repo.Create(ctx, entity)
	if err != nil {
		return nil, fmt.Errorf("failed to persist submission to database: %w", err)
	}

	// 2. Publish submission ID to Redis Streams
	if err := s.queue.Enqueue(ctx, created.ID); err != nil {
		// DUAL-WRITE SAFETY RULE:
		// Do NOT delete or rollback the PostgreSQL record.
		// Leave status as QUEUED so the background dual-write sweeper reconciles it.
		log.Printf("[submission] failed to enqueue submission %s to redis stream: %v (preserved in DB as QUEUED)", created.ID, err)
		return nil, fmt.Errorf("failed to enqueue submission to stream: %w", err)
	}

	return &CreateSubmissionResponse{
		ID:        created.ID,
		Status:    created.Status,
		CreatedAt: created.CreatedAt,
	}, nil
}

// GetSubmission fetches a submission by ID strictly scoped to the authenticated user.
func (s *Service) GetSubmission(ctx context.Context, userID, id string) (*SubmissionDetailResponse, error) {
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(id) == "" {
		return nil, ErrInvalidInput
	}

	entity, err := s.repo.GetByIDAndUserID(ctx, strings.TrimSpace(id), strings.TrimSpace(userID))
	if err != nil {
		return nil, err
	}

	return toDetailResponse(entity), nil
}

// ListSubmissions returns a paginated list of submissions belonging to the authenticated user.
func (s *Service) ListSubmissions(ctx context.Context, userID string, limit, offset int) ([]*SubmissionDetailResponse, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, ErrInvalidInput
	}

	entities, err := s.repo.ListByUserID(ctx, strings.TrimSpace(userID), limit, offset)
	if err != nil {
		return nil, err
	}

	responses := make([]*SubmissionDetailResponse, 0, len(entities))
	for _, entity := range entities {
		responses = append(responses, toDetailResponse(entity))
	}

	return responses, nil
}

func toDetailResponse(e *SubmissionEntity) *SubmissionDetailResponse {
	return &SubmissionDetailResponse{
		ID:                e.ID,
		Language:          e.Language,
		Status:            e.Status,
		SourceCode:        e.Code,
		Stdin:             e.Stdin,
		Stdout:            e.Stdout,
		Stderr:            e.Stderr,
		CompilationOutput: e.CompilationOutput,
		StdoutTruncated:   e.StdoutTruncated,
		StderrTruncated:   e.StderrTruncated,
		ExitCode:          e.ExitCode,
		ExecutionTimeMs:   e.ExecutionTimeMs,
		MemoryUsageKb:     e.MemoryUsageKb,
		CreatedAt:         e.CreatedAt,
		UpdatedAt:         e.UpdatedAt,
	}
}
