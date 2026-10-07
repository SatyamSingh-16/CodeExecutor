package runner

import (
	"bytes"
	"sync"
	"testing"
)

func TestLimitedBuffer(t *testing.T) {
	t.Run("Zero bytes written", func(t *testing.T) {
		buf := NewLimitedBuffer(64 * 1024)
		if buf.Len() != 0 {
			t.Errorf("expected Len 0, got %d", buf.Len())
		}
		if buf.Truncated() {
			t.Errorf("expected Truncated false")
		}
		if buf.String() != "" {
			t.Errorf("expected empty string, got %q", buf.String())
		}
	})

	t.Run("Output below 64 KB", func(t *testing.T) {
		buf := NewLimitedBuffer(64 * 1024)
		data := []byte("hello world\n")
		n, err := buf.Write(data)
		if err != nil {
			t.Fatalf("unexpected write error: %v", err)
		}
		if n != len(data) {
			t.Errorf("expected n %d, got %d", len(data), n)
		}
		if buf.Len() != len(data) {
			t.Errorf("expected Len %d, got %d", len(data), buf.Len())
		}
		if buf.Truncated() {
			t.Errorf("expected Truncated false for output below limit")
		}
		if buf.String() != string(data) {
			t.Errorf("expected string %q, got %q", string(data), buf.String())
		}
	})

	t.Run("Output exactly 64 KB", func(t *testing.T) {
		limit := 64 * 1024 // 65,536 bytes
		buf := NewLimitedBuffer(limit)
		data := bytes.Repeat([]byte("A"), limit)

		n, err := buf.Write(data)
		if err != nil {
			t.Fatalf("unexpected write error: %v", err)
		}
		if n != limit {
			t.Errorf("expected n %d, got %d", limit, n)
		}
		if buf.Len() != limit {
			t.Errorf("expected Len %d, got %d", limit, buf.Len())
		}
		if buf.Truncated() {
			t.Errorf("expected Truncated false when output exactly equals limit")
		}
		if len(buf.Bytes()) != limit {
			t.Errorf("expected %d bytes, got %d", limit, len(buf.Bytes()))
		}
	})

	t.Run("Output 64 KB + 1 byte", func(t *testing.T) {
		limit := 64 * 1024
		buf := NewLimitedBuffer(limit)
		data := bytes.Repeat([]byte("B"), limit+1)

		n, err := buf.Write(data)
		if err != nil {
			t.Fatalf("unexpected write error: %v", err)
		}
		// Returns full length of data to avoid blocking upstream writer
		if n != len(data) {
			t.Errorf("expected n %d, got %d", len(data), n)
		}
		if buf.Len() != limit {
			t.Errorf("expected Len %d, got %d", limit, buf.Len())
		}
		if !buf.Truncated() {
			t.Errorf("expected Truncated true when output exceeds limit by 1 byte")
		}
	})

	t.Run("Large stream 10 MB in chunks continues consuming without memory growth", func(t *testing.T) {
		limit := 64 * 1024
		buf := NewLimitedBuffer(limit)
		chunkSize := 1024 * 1024 // 1 MB chunks
		totalBytes := 10 * 1024 * 1024 // 10 MB total
		chunk := bytes.Repeat([]byte("X"), chunkSize)

		written := 0
		for written < totalBytes {
			n, err := buf.Write(chunk)
			if err != nil {
				t.Fatalf("unexpected write error: %v", err)
			}
			if n != chunkSize {
				t.Fatalf("expected chunk write %d, got %d", chunkSize, n)
			}
			written += n
		}

		if buf.Len() != limit {
			t.Errorf("expected Len capped at %d, got %d", limit, buf.Len())
		}
		if !buf.Truncated() {
			t.Errorf("expected Truncated true for 10 MB stream")
		}
	})

	t.Run("Concurrent writes are thread safe", func(t *testing.T) {
		buf := NewLimitedBuffer(1000)
		var wg sync.WaitGroup
		goroutines := 10
		writesPerGoroutine := 100

		for i := 0; i < goroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < writesPerGoroutine; j++ {
					_, _ = buf.Write([]byte("abcdefghij"))
				}
			}()
		}
		wg.Wait()

		if buf.Len() != 1000 {
			t.Errorf("expected Len 1000, got %d", buf.Len())
		}
		if !buf.Truncated() {
			t.Errorf("expected Truncated true")
		}
	})
}
