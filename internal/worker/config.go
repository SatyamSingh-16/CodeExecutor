package worker

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/SatyamSingh-16/code_executor/internal/queue"
)

// WorkerConfig holds execution and queue polling settings for the background worker.
type WorkerConfig struct {
	StreamName       string
	ConsumerGroup    string
	ConsumerName     string
	ConcurrencyLimit int
	PollBatchSize    int64
	PollBlockTimeout time.Duration
	ShutdownTimeout  time.Duration

	// Ticket 09 Reaper & XAUTOCLAIM configuration
	ReaperInterval  time.Duration
	ReaperMinIdle   time.Duration
	ReaperBatchSize int64
}

// DefaultWorkerConfig returns the default configuration with concurrency 4.
func DefaultWorkerConfig() WorkerConfig {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = fmt.Sprintf("worker-%d", time.Now().UnixNano())
	}

	return WorkerConfig{
		StreamName:       queue.DefaultStreamName,
		ConsumerGroup:    queue.DefaultConsumerGroup,
		ConsumerName:     hostname,
		ConcurrencyLimit: 4,
		PollBatchSize:    10,
		PollBlockTimeout: 2000 * time.Millisecond,
		ShutdownTimeout:  30 * time.Second,
		ReaperInterval:   15 * time.Second,
		ReaperMinIdle:    30 * time.Second,
		ReaperBatchSize:  10,
	}
}

// LoadWorkerConfigFromEnv creates a WorkerConfig using environment variables where provided.
func LoadWorkerConfigFromEnv() WorkerConfig {
	cfg := DefaultWorkerConfig()

	if stream := os.Getenv("REDIS_STREAM_NAME"); stream != "" {
		cfg.StreamName = stream
	}
	if group := os.Getenv("REDIS_CONSUMER_GROUP"); group != "" {
		cfg.ConsumerGroup = group
	}
	if name := os.Getenv("WORKER_CONSUMER_NAME"); name != "" {
		cfg.ConsumerName = name
	} else if id := os.Getenv("WORKER_ID"); id != "" {
		cfg.ConsumerName = id
	}

	if concStr := os.Getenv("WORKER_CONCURRENCY"); concStr != "" {
		if c, err := strconv.Atoi(concStr); err == nil && c > 0 {
			cfg.ConcurrencyLimit = c
		}
	}
	if batchStr := os.Getenv("WORKER_BATCH_SIZE"); batchStr != "" {
		if b, err := strconv.ParseInt(batchStr, 10, 64); err == nil && b > 0 {
			cfg.PollBatchSize = b
		}
	}
	if blockStr := os.Getenv("WORKER_BLOCK_TIMEOUT_MS"); blockStr != "" {
		if ms, err := strconv.Atoi(blockStr); err == nil && ms > 0 {
			cfg.PollBlockTimeout = time.Duration(ms) * time.Millisecond
		}
	}
	if shutStr := os.Getenv("WORKER_SHUTDOWN_TIMEOUT_SECONDS"); shutStr != "" {
		if s, err := strconv.Atoi(shutStr); err == nil && s > 0 {
			cfg.ShutdownTimeout = time.Duration(s) * time.Second
		}
	}
	if reapIntStr := os.Getenv("REAPER_INTERVAL_SECONDS"); reapIntStr != "" {
		if s, err := strconv.Atoi(reapIntStr); err == nil && s > 0 {
			cfg.ReaperInterval = time.Duration(s) * time.Second
		}
	}
	if reapIdleStr := os.Getenv("REAPER_MIN_IDLE_SECONDS"); reapIdleStr != "" {
		if s, err := strconv.Atoi(reapIdleStr); err == nil && s > 0 {
			cfg.ReaperMinIdle = time.Duration(s) * time.Second
		}
	}
	if reapBatchStr := os.Getenv("REAPER_BATCH_SIZE"); reapBatchStr != "" {
		if b, err := strconv.ParseInt(reapBatchStr, 10, 64); err == nil && b > 0 {
			cfg.ReaperBatchSize = b
		}
	}

	return cfg
}
