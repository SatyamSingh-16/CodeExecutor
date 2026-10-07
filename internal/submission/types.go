package submission

import (
	"errors"
	"strings"
	"time"
)

const (
	// MaxPayloadSizeBytes is the maximum permitted size for source code or stdin (64 KB).
	MaxPayloadSizeBytes = 64 * 1024
	// MaxRequestBodyBytes is the upper limit for incoming JSON payload (1 MB).
	MaxRequestBodyBytes = 1 << 20
)

var (
	// ErrSubmissionNotFound is returned when a submission does not exist or does not belong to the user.
	ErrSubmissionNotFound = errors.New("submission not found")
	// ErrInvalidInput is returned on client input validation failure.
	ErrInvalidInput = errors.New("invalid input")
	// ErrUnsupportedLanguage is returned when an unsupported language is requested.
	ErrUnsupportedLanguage = errors.New("unsupported language: must be python or go")
	// ErrEmptySourceCode is returned when source code is missing or whitespace.
	ErrEmptySourceCode = errors.New("source code cannot be empty")
	// ErrPayloadTooLarge is returned when source code or stdin exceeds 64 KB.
	ErrPayloadTooLarge = errors.New("payload exceeds maximum permitted size (64 KB)")
)

// Supported languages
const (
	LanguagePython = "python"
	LanguageGo     = "go"
)

// IsValidLanguage checks whether a language string is supported.
func IsValidLanguage(lang string) bool {
	l := strings.ToLower(strings.TrimSpace(lang))
	return l == LanguagePython || l == LanguageGo
}

// NormalizeLanguage returns the canonical lower-case representation of the language.
func NormalizeLanguage(lang string) string {
	return strings.ToLower(strings.TrimSpace(lang))
}

// CreateSubmissionRequest represents the incoming JSON payload for POST /api/submissions.
type CreateSubmissionRequest struct {
	Language   string `json:"language"`
	SourceCode string `json:"source_code"`
	Code       string `json:"code,omitempty"` // Fallback alias
	Stdin      string `json:"stdin,omitempty"`
}

// GetCode returns SourceCode if set, otherwise falls back to Code.
func (r *CreateSubmissionRequest) GetCode() string {
	if r.SourceCode != "" {
		return r.SourceCode
	}
	return r.Code
}

// CreateSubmissionResponse represents the HTTP 202 response body.
type CreateSubmissionResponse struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at,omitempty"`
}

// SubmissionDetailResponse represents the full submission details for GET /api/submissions/{id}.
type SubmissionDetailResponse struct {
	ID                string     `json:"id"`
	Language          string     `json:"language"`
	Status            string     `json:"status"`
	SourceCode        string     `json:"source_code,omitempty"`
	Stdin             string     `json:"stdin,omitempty"`
	Stdout            string     `json:"stdout"`
	Stderr            string     `json:"stderr"`
	CompilationOutput string     `json:"compilation_output"`
	StdoutTruncated   bool       `json:"stdout_truncated"`
	StderrTruncated   bool       `json:"stderr_truncated"`
	ExitCode          *int       `json:"exit_code"`
	ExecutionTimeMs   *int64     `json:"execution_time_ms"`
	MemoryUsageKb     *int64     `json:"memory_usage_kb"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// SubmissionEntity represents the database record in the submissions table.
type SubmissionEntity struct {
	ID                string
	UserID            string
	Language          string
	Code              string
	Stdin             string
	Status            string
	Stdout            string
	Stderr            string
	CompilationOutput string
	StdoutTruncated   bool
	StderrTruncated   bool
	ExitCode          *int
	ExecutionTimeMs   *int64
	MemoryUsageKb     *int64
	RetryCount        int
	CreatedAt         time.Time
	UpdatedAt         time.Time
}
