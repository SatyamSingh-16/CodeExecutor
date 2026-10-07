package queue

import (
	"os"
	"strconv"
	"time"
)

// RedisConfig holds connection and stream naming configuration for Redis.
type RedisConfig struct {
	Addr          string
	Password      string
	DB            int
	StreamName    string
	ConsumerGroup string
}

// DefaultRedisConfig returns the default configuration for local Redis.
func DefaultRedisConfig() RedisConfig {
	return RedisConfig{
		Addr:          "localhost:6379",
		Password:      "",
		DB:            0,
		StreamName:    DefaultStreamName,
		ConsumerGroup: DefaultConsumerGroup,
	}
}

// LoadRedisConfigFromEnv loads RedisConfig with overrides from environment variables.
func LoadRedisConfigFromEnv() RedisConfig {
	cfg := DefaultRedisConfig()

	if addr := os.Getenv("REDIS_ADDR"); addr != "" {
		cfg.Addr = addr
	}
	if pwd := os.Getenv("REDIS_PASSWORD"); pwd != "" {
		cfg.Password = pwd
	}
	if dbStr := os.Getenv("REDIS_DB"); dbStr != "" {
		if db, err := strconv.Atoi(dbStr); err == nil {
			cfg.DB = db
		}
	}
	if stream := os.Getenv("REDIS_STREAM_NAME"); stream != "" {
		cfg.StreamName = stream
	}
	if group := os.Getenv("REDIS_CONSUMER_GROUP"); group != "" {
		cfg.ConsumerGroup = group
	}

	return cfg
}

// SweeperConfig holds timing and batch limits for the dual-write recovery sweeper.
type SweeperConfig struct {
	// Interval is how frequently the background sweeper polls for stuck submissions.
	Interval time.Duration

	// EligibilityThreshold is how long a submission must have been QUEUED before being eligible for recovery.
	EligibilityThreshold time.Duration

	// BatchSize is the maximum number of submissions fetched per sweep pass.
	BatchSize int
}

// DefaultSweeperConfig returns production defaults (10s interval, 30s threshold, batch size 100).
func DefaultSweeperConfig() SweeperConfig {
	return SweeperConfig{
		Interval:             10 * time.Second,
		EligibilityThreshold: 30 * time.Second,
		BatchSize:            100,
	}
}

// LoadSweeperConfigFromEnv loads SweeperConfig with overrides from environment variables.
func LoadSweeperConfigFromEnv() SweeperConfig {
	cfg := DefaultSweeperConfig()

	if intStr := os.Getenv("SWEEPER_INTERVAL_SECONDS"); intStr != "" {
		if s, err := strconv.Atoi(intStr); err == nil && s > 0 {
			cfg.Interval = time.Duration(s) * time.Second
		}
	}
	if threshStr := os.Getenv("SWEEPER_THRESHOLD_SECONDS"); threshStr != "" {
		if s, err := strconv.Atoi(threshStr); err == nil && s > 0 {
			cfg.EligibilityThreshold = time.Duration(s) * time.Second
		}
	}
	if batchStr := os.Getenv("SWEEPER_BATCH_SIZE"); batchStr != "" {
		if b, err := strconv.Atoi(batchStr); err == nil && b > 0 {
			cfg.BatchSize = b
		}
	}

	return cfg
}
