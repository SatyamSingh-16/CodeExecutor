package queue

import (
	"context"
	"errors"
	"testing"

	"github.com/redis/go-redis/v9"
)

type mockRedisClient struct {
	xaddFunc   func(ctx context.Context, a *redis.XAddArgs) *redis.StringCmd
	xgroupFunc func(ctx context.Context, stream, group, start string) *redis.StatusCmd
	pingFunc   func(ctx context.Context) *redis.StatusCmd
	closeFunc  func() error
}

func (m *mockRedisClient) XAdd(ctx context.Context, a *redis.XAddArgs) *redis.StringCmd {
	if m.xaddFunc != nil {
		return m.xaddFunc(ctx, a)
	}
	cmd := redis.NewStringCmd(ctx)
	cmd.SetVal("1700000000000-0")
	return cmd
}

func (m *mockRedisClient) XGroupCreateMkStream(ctx context.Context, stream, group, start string) *redis.StatusCmd {
	if m.xgroupFunc != nil {
		return m.xgroupFunc(ctx, stream, group, start)
	}
	cmd := redis.NewStatusCmd(ctx)
	cmd.SetVal("OK")
	return cmd
}

func (m *mockRedisClient) Ping(ctx context.Context) *redis.StatusCmd {
	if m.pingFunc != nil {
		return m.pingFunc(ctx)
	}
	cmd := redis.NewStatusCmd(ctx)
	cmd.SetVal("PONG")
	return cmd
}

func (m *mockRedisClient) Close() error {
	if m.closeFunc != nil {
		return m.closeFunc()
	}
	return nil
}

func TestRedisQueue_Enqueue(t *testing.T) {
	ctx := context.Background()

	t.Run("successful enqueue produces XADD with minimal identifier", func(t *testing.T) {
		var capturedArgs *redis.XAddArgs
		mock := &mockRedisClient{
			xaddFunc: func(ctx context.Context, a *redis.XAddArgs) *redis.StringCmd {
				capturedArgs = a
				cmd := redis.NewStringCmd(ctx)
				cmd.SetVal("1000-0")
				return cmd
			},
		}

		q := NewRedisQueue(mock, "custom:stream", "custom_group")
		err := q.Enqueue(ctx, "sub-12345")
		if err != nil {
			t.Fatalf("unexpected enqueue error: %v", err)
		}

		if capturedArgs == nil {
			t.Fatal("expected XAddArgs to be captured")
		}
		if capturedArgs.Stream != "custom:stream" {
			t.Errorf("expected stream 'custom:stream', got %q", capturedArgs.Stream)
		}
		vals, ok := capturedArgs.Values.(map[string]interface{})
		if !ok {
			t.Fatalf("expected map[string]interface{}, got %T", capturedArgs.Values)
		}
		if vals[FieldSubmissionID] != "sub-12345" {
			t.Errorf("expected submission_id 'sub-12345', got %v", vals[FieldSubmissionID])
		}
		// Confirm source code or bulky state is NOT put into Redis stream
		if _, exists := vals["code"]; exists {
			t.Errorf("source code must not be included in Redis stream payload")
		}
	})

	t.Run("empty submission ID is rejected", func(t *testing.T) {
		mock := &mockRedisClient{}
		q := NewRedisQueue(mock, "", "")
		if err := q.Enqueue(ctx, "   "); err == nil {
			t.Fatal("expected error for whitespace submission ID, got nil")
		}
	})

	t.Run("redis XADD failure is returned to caller", func(t *testing.T) {
		mock := &mockRedisClient{
			xaddFunc: func(ctx context.Context, a *redis.XAddArgs) *redis.StringCmd {
				cmd := redis.NewStringCmd(ctx)
				cmd.SetErr(errors.New("connection refused"))
				return cmd
			},
		}

		q := NewRedisQueue(mock, DefaultStreamName, DefaultConsumerGroup)
		err := q.Enqueue(ctx, "sub-failure")
		if err == nil {
			t.Fatal("expected enqueue error when Redis fails, got nil")
		}
	})
}

func TestRedisQueue_InitConsumerGroup(t *testing.T) {
	ctx := context.Background()

	t.Run("creates consumer group with MKSTREAM", func(t *testing.T) {
		var capturedStream, capturedGroup, capturedStart string
		mock := &mockRedisClient{
			xgroupFunc: func(ctx context.Context, stream, group, start string) *redis.StatusCmd {
				capturedStream = stream
				capturedGroup = group
				capturedStart = start
				cmd := redis.NewStatusCmd(ctx)
				cmd.SetVal("OK")
				return cmd
			},
		}

		q := NewRedisQueue(mock, "submissions:stream", "workers_group")
		if err := q.InitConsumerGroup(ctx); err != nil {
			t.Fatalf("unexpected InitConsumerGroup error: %v", err)
		}

		if capturedStream != "submissions:stream" || capturedGroup != "workers_group" || capturedStart != "$" {
			t.Errorf("unexpected XGroup params: %s, %s, %s", capturedStream, capturedGroup, capturedStart)
		}
	})

	t.Run("repeated initialization is safe and ignores BUSYGROUP", func(t *testing.T) {
		callCount := 0
		mock := &mockRedisClient{
			xgroupFunc: func(ctx context.Context, stream, group, start string) *redis.StatusCmd {
				callCount++
				cmd := redis.NewStatusCmd(ctx)
				if callCount > 1 {
					cmd.SetErr(errors.New("BUSYGROUP Consumer Group name already exists"))
				} else {
					cmd.SetVal("OK")
				}
				return cmd
			},
		}

		q := NewRedisQueue(mock, "submissions:stream", "workers_group")
		// First call
		if err := q.InitConsumerGroup(ctx); err != nil {
			t.Fatalf("first InitConsumerGroup failed: %v", err)
		}
		// Second call (simulating restart or concurrent worker)
		if err := q.InitConsumerGroup(ctx); err != nil {
			t.Fatalf("second InitConsumerGroup failed on BUSYGROUP: %v", err)
		}
	})

	t.Run("non-BUSYGROUP errors are reported", func(t *testing.T) {
		mock := &mockRedisClient{
			xgroupFunc: func(ctx context.Context, stream, group, start string) *redis.StatusCmd {
				cmd := redis.NewStatusCmd(ctx)
				cmd.SetErr(errors.New("NOPERM user has no permission for XGROUP"))
				return cmd
			},
		}

		q := NewRedisQueue(mock, "submissions:stream", "workers_group")
		if err := q.InitConsumerGroup(ctx); err == nil {
			t.Fatal("expected error on NOPERM, got nil")
		}
	})
}
