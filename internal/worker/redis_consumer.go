package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/SatyamSingh-16/code_executor/internal/queue"
	"github.com/redis/go-redis/v9"
)

// RedisClient defines the subset of Redis commands required by the worker consumer and publisher.
// *redis.Client satisfies this interface directly.
type RedisClient interface {
	XReadGroup(ctx context.Context, a *redis.XReadGroupArgs) *redis.XStreamSliceCmd
	XAck(ctx context.Context, stream, group string, ids ...string) *redis.IntCmd
	XAutoClaim(ctx context.Context, a *redis.XAutoClaimArgs) *redis.XAutoClaimCmd
	Publish(ctx context.Context, channel string, message interface{}) *redis.IntCmd
}

// RedisStreamConsumer implements StreamConsumer using Redis Streams XREADGROUP and XACK.
type RedisStreamConsumer struct {
	client        RedisClient
	streamName    string
	consumerGroup string
	consumerName  string
}

// NewRedisStreamConsumer constructs a new Redis stream consumer.
func NewRedisStreamConsumer(client RedisClient, streamName, consumerGroup, consumerName string) *RedisStreamConsumer {
	if streamName == "" {
		streamName = queue.DefaultStreamName
	}
	if consumerGroup == "" {
		consumerGroup = queue.DefaultConsumerGroup
	}
	return &RedisStreamConsumer{
		client:        client,
		streamName:    streamName,
		consumerGroup: consumerGroup,
		consumerName:  consumerName,
	}
}

// ReadMessages pulls a batch of new messages from the stream consumer group using '>' identifier.
func (c *RedisStreamConsumer) ReadMessages(ctx context.Context, count int64, block time.Duration) ([]StreamMessage, error) {
	args := &redis.XReadGroupArgs{
		Group:    c.consumerGroup,
		Consumer: c.consumerName,
		Streams:  []string{c.streamName, ">"},
		Count:    count,
		Block:    block,
	}

	cmd := c.client.XReadGroup(ctx, args)
	streams, err := cmd.Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return []StreamMessage{}, nil
		}
		return nil, fmt.Errorf("XREADGROUP failed on stream %q: %w", c.streamName, err)
	}

	var messages []StreamMessage
	for _, stream := range streams {
		for _, msg := range stream.Messages {
			var subID string
			if val, ok := msg.Values[queue.FieldSubmissionID]; ok {
				subID = fmt.Sprintf("%v", val)
			}
			messages = append(messages, StreamMessage{
				MessageID:    msg.ID,
				SubmissionID: subID,
			})
		}
	}

	return messages, nil
}

// AckMessage acknowledges a completed stream message via XACK.
func (c *RedisStreamConsumer) AckMessage(ctx context.Context, messageID string) error {
	cmd := c.client.XAck(ctx, c.streamName, c.consumerGroup, messageID)
	if err := cmd.Err(); err != nil {
		return fmt.Errorf("XACK failed for message %s: %w", messageID, err)
	}
	return nil
}

// AutoClaim reclaims orphaned pending messages in the PEL using XAUTOCLAIM.
func (c *RedisStreamConsumer) AutoClaim(ctx context.Context, minIdle time.Duration, start string, count int64) ([]StreamMessage, string, error) {
	if start == "" {
		start = "0-0"
	}
	if count <= 0 {
		count = 10
	}

	args := &redis.XAutoClaimArgs{
		Stream:   c.streamName,
		Group:    c.consumerGroup,
		Consumer: c.consumerName,
		MinIdle:  minIdle,
		Start:    start,
		Count:    count,
	}

	cmd := c.client.XAutoClaim(ctx, args)
	msgs, nextStart, err := cmd.Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return []StreamMessage{}, "0-0", nil
		}
		return nil, "", fmt.Errorf("XAUTOCLAIM failed on stream %q: %w", c.streamName, err)
	}

	var streamMsgs []StreamMessage
	for _, msg := range msgs {
		var subID string
		if val, ok := msg.Values[queue.FieldSubmissionID]; ok {
			subID = fmt.Sprintf("%v", val)
		}
		streamMsgs = append(streamMsgs, StreamMessage{
			MessageID:    msg.ID,
			SubmissionID: subID,
		})
	}

	return streamMsgs, nextStart, nil
}

// RedisEventPublisher implements EventPublisher publishing transition events to Redis Pub/Sub.
type RedisEventPublisher struct {
	client RedisClient
}

// NewRedisEventPublisher constructs a new Redis Pub/Sub event publisher.
func NewRedisEventPublisher(client RedisClient) *RedisEventPublisher {
	return &RedisEventPublisher{client: client}
}

// PublishStatusEvent publishes a status transition event to submissions:events:<submissionID>.
func (p *RedisEventPublisher) PublishStatusEvent(ctx context.Context, submissionID string, status string) error {
	channel := fmt.Sprintf("submissions:events:%s", submissionID)
	evt := StatusEvent{
		SubmissionID: submissionID,
		Status:       status,
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
	}

	payload, err := json.Marshal(evt)
	if err != nil {
		return fmt.Errorf("failed to marshal status event for %s: %w", submissionID, err)
	}

	cmd := p.client.Publish(ctx, channel, payload)
	if err := cmd.Err(); err != nil {
		return fmt.Errorf("failed to publish status event on channel %s: %w", channel, err)
	}

	return nil
}
