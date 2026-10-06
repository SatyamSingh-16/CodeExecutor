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
}

// Default runtime configurations
var defaultRuntimeConfigs = map[Language]RuntimeConfig{
	LanguagePython: {
		Language:       LanguagePython,
		Image:          "code-executor-runner-python:latest",
		SourceFileName: "main.py",
		DefaultCommand: []string{"python3", "/tmp/main.py"},
		TmpfsOptions:   "rw,noexec,nosuid,size=64m",
	},
	LanguageGo: {
		Language:       LanguageGo,
		Image:          "code-executor-runner-go:latest",
		SourceFileName: "main.go",
		DefaultCommand: []string{"go", "run", "/tmp/main.go"},
		TmpfsOptions:   "rw,exec,nosuid,size=64m",
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
	Language       Language
	Code           string
	Stdin          string
	Timeout        time.Duration
	MemoryLimit    int64 // in bytes (default 128MB)
	CPULimit       int64 // in NanoCPUs (default 1 core = 1_000_000_000)
	PidsLimit      int64 // max processes (default 64)
	CustomCommand  []string // optional override for entrypoint args
}

// ExecutionResult contains output and metadata from a container execution.
type ExecutionResult struct {
	Stdout      string
	Stderr      string
	ExitCode    int
	TimedOut    bool
	Duration    time.Duration
	ContainerID string
}
