package runner

import (
	"errors"
	"time"
)

// Language represents a supported runtime language.
type Language string

const (
	LanguagePython Language = "python"
	LanguageGo     Language = "go"
)

var (
	ErrUnsupportedLanguage = errors.New("unsupported runtime language")
	ErrEmptyCode           = errors.New("source code cannot be empty")
)

// RuntimeConfig defines container image, tmpfs mount options, and execution parameters for a language.
type RuntimeConfig struct {
	Language       Language
	Image          string
	SourceFileName string
	DefaultCommand []string
	TmpfsOptions   string
	IsCompiled     bool
	CompileCommand []string
	BinaryPath     string
}

// Default runtime configurations
var defaultRuntimeConfigs = map[Language]RuntimeConfig{
	LanguagePython: {
		Language:       LanguagePython,
		Image:          "code-executor-runner-python:latest",
		SourceFileName: "main.py",
		DefaultCommand: []string{"python3", "/tmp/main.py"},
		TmpfsOptions:   "rw,noexec,nosuid,size=64m",
		IsCompiled:     false,
	},
	LanguageGo: {
		Language:       LanguageGo,
		Image:          "code-executor-runner-go:latest",
		SourceFileName: "main.go",
		DefaultCommand: []string{"/tmp/app"},
		TmpfsOptions:   "rw,exec,nosuid,size=64m",
		IsCompiled:     true,
		CompileCommand: []string{"go", "build", "-p", "2", "-o", "/tmp/app", "/tmp/main.go"},
		BinaryPath:     "/tmp/app",
	},
}

// GetRuntimeConfig returns the runtime configuration for a given language.
func GetRuntimeConfig(lang Language) (RuntimeConfig, error) {
	cfg, ok := defaultRuntimeConfigs[lang]
	if !ok {
		return RuntimeConfig{}, ErrUnsupportedLanguage
	}
	return cfg, nil
}

// ExecutionRequest contains parameters for running user code.
type ExecutionRequest struct {
	Language           Language
	Code               string
	Stdin              string
	Timeout            time.Duration // runtime timeout (default: 2s for Go, 5s for Python)
	MemoryLimit        int64         // runtime memory limit in bytes (default 128MB)
	CPULimit           int64         // in NanoCPUs (default 1 core = 1_000_000_000)
	PidsLimit          int64         // max processes (default 64)
	CustomCommand      []string      // optional override for runtime command
	CompileTimeout     time.Duration // compilation timeout (default: 10s for compiled languages)
	CompileMemoryLimit int64         // compilation memory limit in bytes (default: 256MB for Go)
}

// ExecutionStatus represents the execution lifecycle terminal status.
type ExecutionStatus string

const (
	StatusSuccess             ExecutionStatus = "SUCCESS"
	StatusCompilationError    ExecutionStatus = "COMPILATION_ERROR"
	StatusRuntimeError        ExecutionStatus = "RUNTIME_ERROR"
	StatusTimeLimitExceeded   ExecutionStatus = "TIME_LIMIT_EXCEEDED"
	StatusMemoryLimitExceeded ExecutionStatus = "MEMORY_LIMIT_EXCEEDED"
	StatusSystemError         ExecutionStatus = "SYSTEM_ERROR"
)

// ExecutionResult contains output and metadata from a container execution.
type ExecutionResult struct {
	Stdout            string
	Stderr            string
	ExitCode          int
	TimedOut          bool
	Duration          time.Duration // runtime execution duration
	ContainerID       string
	IsCompileError    bool          // true if execution halted in compilation phase
	CompilationOutput string        // compiler diagnostic output (Phase 1)
	CompileDuration   time.Duration // duration of compilation phase

	// Ticket 04 fields:
	Status          ExecutionStatus
	StdoutTruncated bool
	StderrTruncated bool
	WallTimeMs      int64
	MemoryUsageKb   int64
	PeakMemoryKb    int64
}
