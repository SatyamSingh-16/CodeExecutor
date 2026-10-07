package runner

import (
	"bytes"
	"sync"
)

// DefaultMaxOutputBytes is the maximum captured output size (64 KB = 65,536 bytes).
const DefaultMaxOutputBytes = 64 * 1024

// LimitedBuffer is a thread-safe bounded writer that enforces an upper limit on accumulated bytes.
// It retains up to maxBytes and discards subsequent bytes, while continuing to consume
// the stream to prevent blocking writers/processes, and tracks whether truncation occurred.
type LimitedBuffer struct {
	mu        sync.Mutex
	buf       bytes.Buffer
	maxBytes  int
	truncated bool
}

// NewLimitedBuffer creates a LimitedBuffer with the specified byte limit.
func NewLimitedBuffer(maxBytes int) *LimitedBuffer {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxOutputBytes
	}
	return &LimitedBuffer{
		maxBytes: maxBytes,
	}
}

// Write accepts incoming bytes up to maxBytes. Additional bytes beyond the limit
// are discarded and mark the buffer as truncated. It always returns len(p), nil
// to allow the writer/stream to continue consuming without blocking.
func (b *LimitedBuffer) Write(p []byte) (n int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	n = len(p)
	currentLen := b.buf.Len()
	if currentLen >= b.maxBytes {
		b.truncated = true
		return n, nil
	}

	remaining := b.maxBytes - currentLen
	if len(p) > remaining {
		b.buf.Write(p[:remaining])
		b.truncated = true
	} else {
		b.buf.Write(p)
	}

	return n, nil
}

// String returns the contents of the buffer as a string.
func (b *LimitedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// Bytes returns a copy of the buffer's accumulated bytes.
func (b *LimitedBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Bytes()
}

// Len returns the current length of accumulated bytes (at most maxBytes).
func (b *LimitedBuffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Len()
}

// Truncated reports whether any incoming bytes were discarded due to exceeding maxBytes.
func (b *LimitedBuffer) Truncated() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.truncated
}

// Reset resets the buffer to empty state and clears the truncated flag.
func (b *LimitedBuffer) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf.Reset()
	b.truncated = false
}
