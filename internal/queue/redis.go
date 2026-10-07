package queue

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/redis/go-redis/v9"
)

// RedisQueue implements SubmissionQueue backed by Redis Streams.
type RedisQueue struct {
	client        RedisClient
	streamName    string
	consumerGroup string
}

// NewRedisQueue constructs a RedisQueue with an injected RedisClient and stream names.
func NewRedisQueue(client RedisClient, streamName, consumerGroup string) *RedisQueue {
	if streamName == "" {
		streamName = DefaultStreamName
	}
	if consumerGroup == "" {
		consumerGroup = DefaultConsumerGroup
	}
	return &RedisQueue{
		client:        client,
		streamName:    streamName,
		consumerGroup: consumerGroup,
	}
}

// NewRedisQueueFromConfig initializes a new *redis.Client and wraps it in a RedisQueue.
func NewRedisQueueFromConfig(cfg RedisConfig) (*RedisQueue, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	return NewRedisQueue(rdb, cfg.StreamName, cfg.ConsumerGroup), nil
}

// Enqueue publishes the submission ID to the Redis Stream using XADD.
// Only durable immutable identifiers are passed, keeping source code and full
// state in PostgreSQL.
func (q *RedisQueue) Enqueue(ctx context.Context, submissionID string) error {
	if strings.TrimSpace(submissionID) == "" {
		return errors.New("submission ID cannot be empty")
	}

	args := &redis.XAddArgs{
		Stream: q.streamName,
		Values: map[string]interface{}{
			FieldSubmissionID: submissionID,
		},
	}

	cmd := q.client.XAdd(ctx, args)
	if err := cmd.Err(); err != nil {
		return fmt.Errorf("redis XADD failed for stream %q: %w", q.streamName, err)
	}

	return nil
}

// InitConsumerGroup idempotently creates the worker consumer group.
// It issues XGROUP CREATE <stream> <group> $ MKSTREAM.
// If the consumer group already exists (BUSYGROUP error), it returns nil without error.
func (q *RedisQueue) InitConsumerGroup(ctx context.Context) error {
	cmd := q.client.XGroupCreateMkStream(ctx, q.streamName, q.consumerGroup, "$")
	err := cmd.Err()
	if err == nil {
		return nil
	}

	if isBusyGroupError(err) {
		// Idempotent: Group already exists from a prior startup or peer worker
		return nil
	}

	return fmt.Errorf("failed to create consumer group %q on stream %q: %w", q.consumerGroup, q.streamName, err)
}

// StreamName returns the configured stream name.
func (q *RedisQueue) StreamName() string {
	return q.streamName
}

// ConsumerGroup returns the configured consumer group name.
func (q *RedisQueue) ConsumerGroup() string {
	return q.consumerGroup
}

// Close closes the underlying Redis client connection.
func (q *RedisQueue) Close() error {
	if q.client != nil {
		return q.client.Close()
	}
	return nil
}

// isBusyGroupError checks if a Redis error indicates the consumer group already exists.
func isBusyGroupError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToUpper(err.Error()), "BUSYGROUP")
}
