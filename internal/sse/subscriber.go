package sse

import (
	"context"
	"fmt"
	"sync"

	"github.com/redis/go-redis/v9"
)

// RedisClient defines the subset of redis.Client operations used by the RedisSubscriber.
type RedisClient interface {
	Subscribe(ctx context.Context, channels ...string) *redis.PubSub
}

// RedisSubscriber implements EventSubscriber using Redis Pub/Sub.
type RedisSubscriber struct {
	client RedisClient
}

// NewRedisSubscriber creates a new RedisSubscriber instance.
func NewRedisSubscriber(client RedisClient) *RedisSubscriber {
	return &RedisSubscriber{client: client}
}

// Subscribe subscribes to the canonical channel submissions:events:<submissionID>.
func (s *RedisSubscriber) Subscribe(ctx context.Context, submissionID string) (Subscription, error) {
	channelName := fmt.Sprintf("submissions:events:%s", submissionID)
	pubsub := s.client.Subscribe(ctx, channelName)

	// Verify the subscription is established by receiving the confirmation message
	// or returning an error if context is cancelled or Redis is down.
	_, err := pubsub.Receive(ctx)
	if err != nil {
		_ = pubsub.Close()
		return nil, fmt.Errorf("failed to subscribe to channel %s: %w", channelName, err)
	}

	msgChan := make(chan string, 16)
	redisCh := pubsub.Channel()

	sub := &redisSubscription{
		pubsub:  pubsub,
		msgChan: msgChan,
		done:    make(chan struct{}),
	}

	// Forward messages in a background goroutine
	sub.wg.Add(1)
	go func() {
		defer sub.wg.Done()
		defer close(msgChan)

		for {
			select {
			case <-sub.done:
				return
			case msg, ok := <-redisCh:
				if !ok {
					return
				}
				select {
				case msgChan <- msg.Payload:
				case <-sub.done:
					return
				}
			}
		}
	}()

	return sub, nil
}

type redisSubscription struct {
	pubsub  *redis.PubSub
	msgChan chan string
	done    chan struct{}
	once    sync.Once
	wg      sync.WaitGroup
}

func (s *redisSubscription) Channel() <-chan string {
	return s.msgChan
}

func (s *redisSubscription) Close() error {
	var err error
	s.once.Do(func() {
		close(s.done)
		err = s.pubsub.Close()
		s.wg.Wait()
	})
	return err
}
