package runner

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"sync"
)

// MetricsMarker is the delimiter line prefix emitted by runner instrumentation wrappers.
const MetricsMarker = "__EXECUTION_METRICS__"

// ErrInvalidMetrics is returned when metrics payload cannot be found or parsed.
var ErrInvalidMetrics = errors.New("invalid or missing execution metrics")

// ExecutionMetrics holds typed metrics extracted from runner instrumentation.
type ExecutionMetrics struct {
	WallTimeMs    int64 `json:"wall_time_ms"`
	PeakMemoryKb  int64 `json:"peak_memory_kb"`
	MemoryUsageKb int64 `json:"memory_usage_kb,omitempty"`
	ExitCode      int   `json:"exit_code"`
}

// rawMetricsJSON accommodates flexible JSON naming conventions defensively.
type rawMetricsJSON struct {
	WallTimeMs      *int64 `json:"wall_time_ms"`
	ExecutionTimeMs *int64 `json:"execution_time_ms"`
	TimeMs          *int64 `json:"time_ms"`
	PeakMemoryKb    *int64 `json:"peak_memory_kb"`
	MemoryUsageKb   *int64 `json:"memory_usage_kb"`
	MemKb           *int64 `json:"mem_kb"`
	ExitCode        *int   `json:"exit_code"`
	Exit            *int   `json:"exit"`
}

// ParseMetrics parses a string containing JSON metrics or a line containing MetricsMarker.
// It is defensive and safely handles missing fields and malformed structures without panicking.
func ParseMetrics(raw string) (*ExecutionMetrics, error) {
	idx := strings.Index(raw, "{")
	if idx == -1 {
		return nil, ErrInvalidMetrics
	}
	endIdx := strings.LastIndex(raw, "}")
	if endIdx == -1 || endIdx <= idx {
		return nil, ErrInvalidMetrics
	}

	jsonStr := raw[idx : endIdx+1]
	var payload rawMetricsJSON
	if err := json.Unmarshal([]byte(jsonStr), &payload); err != nil {
		return nil, err
	}

	metrics := &ExecutionMetrics{}
	if payload.WallTimeMs != nil {
		metrics.WallTimeMs = *payload.WallTimeMs
	} else if payload.ExecutionTimeMs != nil {
		metrics.WallTimeMs = *payload.ExecutionTimeMs
	} else if payload.TimeMs != nil {
		metrics.WallTimeMs = *payload.TimeMs
	}

	if payload.PeakMemoryKb != nil {
		metrics.PeakMemoryKb = *payload.PeakMemoryKb
		metrics.MemoryUsageKb = *payload.PeakMemoryKb
	} else if payload.MemoryUsageKb != nil {
		metrics.PeakMemoryKb = *payload.MemoryUsageKb
		metrics.MemoryUsageKb = *payload.MemoryUsageKb
	} else if payload.MemKb != nil {
		metrics.PeakMemoryKb = *payload.MemKb
		metrics.MemoryUsageKb = *payload.MemKb
	}

	if payload.ExitCode != nil {
		metrics.ExitCode = *payload.ExitCode
	} else if payload.Exit != nil {
		metrics.ExitCode = *payload.Exit
	}

	return metrics, nil
}

// ExtractMetrics scans the input string for MetricsMarker, extracts and parses the metrics,
// and returns the parsed metrics, the cleaned string without the metrics marker, and a boolean
// indicating whether the marker was found.
// It ensures that malformed JSON does not panic, normal user output is uncorrupted,
// and the metrics marker is never exposed as user output.
func ExtractMetrics(output string) (*ExecutionMetrics, string, bool) {
	markerIdx := strings.Index(output, MetricsMarker)
	if markerIdx == -1 {
		return nil, output, false
	}

	// Find the start of the line containing MetricsMarker
	lineStart := strings.LastIndex(output[:markerIdx], "\n")
	hasLeadingNewline := lineStart != -1

	// Find the end of the line containing MetricsMarker
	lineEnd := strings.Index(output[markerIdx:], "\n")
	var fullLineEnd int
	var hasTrailingNewline bool
	if lineEnd == -1 {
		fullLineEnd = len(output)
		hasTrailingNewline = false
	} else {
		fullLineEnd = markerIdx + lineEnd
		hasTrailingNewline = true
	}

	markerLine := output[markerIdx:fullLineEnd]
	metrics, _ := ParseMetrics(markerLine)
	if metrics == nil {
		metrics = &ExecutionMetrics{}
	}

	var cleanOutput string
	if hasLeadingNewline && hasTrailingNewline {
		if lineStart == 0 {
			cleanOutput = output[fullLineEnd+1:]
		} else {
			cleanOutput = output[:lineStart] + output[fullLineEnd:]
		}
	} else if hasLeadingNewline && !hasTrailingNewline {
		cleanOutput = output[:lineStart]
	} else if !hasLeadingNewline && hasTrailingNewline {
		cleanOutput = output[fullLineEnd+1:]
	} else {
		cleanOutput = ""
	}

	return metrics, cleanOutput, true
}

// MetricsFilterWriter wraps a LimitedBuffer and intercepts lines containing MetricsMarker
// as data streams from the container. This guarantees metrics are captured even if the user
// prints more than 64 KB of output, while preventing metrics markers from accumulating in
// the user's LimitedBuffer quota.
type MetricsFilterWriter struct {
	mu      sync.Mutex
	dst     *LimitedBuffer
	metrics *ExecutionMetrics
	found   bool
	pending bytes.Buffer
}

// NewMetricsFilterWriter creates a new streaming metrics filter.
func NewMetricsFilterWriter(dst *LimitedBuffer) *MetricsFilterWriter {
	return &MetricsFilterWriter{
		dst: dst,
	}
}

// Write intercepts stream chunks, buffering lines up to 4 KB to prevent unbounded memory growth.
func (w *MetricsFilterWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	total := len(p)
	w.pending.Write(p)

	for {
		data := w.pending.Bytes()
		nlIdx := bytes.IndexByte(data, '\n')
		if nlIdx == -1 {
			// No newline in pending buffer.
			// If pending exceeds 4KB without containing MetricsMarker, flush all but the last
			// len(MetricsMarker) bytes to dst to keep memory strictly bounded.
			if w.pending.Len() > 4096 {
				marker := []byte(MetricsMarker)
				if !bytes.Contains(data, marker) {
					keep := len(marker)
					flushLen := len(data) - keep
					w.dst.Write(data[:flushLen])
					w.pending.Reset()
					w.pending.Write(data[flushLen:])
				}
			}
			break
		}

		// Line available up to nlIdx
		line := data[:nlIdx]
		if bytes.Contains(line, []byte(MetricsMarker)) {
			m, _ := ParseMetrics(string(line))
			if m != nil {
				w.metrics = m
				w.found = true
			}
			// Omit metrics line from downstream buffer
		} else {
			// Check if this line is an empty line preceding MetricsMarker delimiter
			if len(line) == 0 {
				rem := data[nlIdx+1:]
				nextNl := bytes.IndexByte(rem, '\n')
				var nextLine []byte
				if nextNl == -1 {
					nextLine = rem
				} else {
					nextLine = rem[:nextNl]
				}
				if bytes.Contains(nextLine, []byte(MetricsMarker)) {
					// Leading delimiter newline for metrics: drop it
					remaining := data[nlIdx+1:]
					tmp := make([]byte, len(remaining))
					copy(tmp, remaining)
					w.pending.Reset()
					w.pending.Write(tmp)
					continue
				}
			}
			// Normal line: write to dst with newline
			w.dst.Write(data[:nlIdx+1])
		}

		remaining := data[nlIdx+1:]
		tmp := make([]byte, len(remaining))
		copy(tmp, remaining)
		w.pending.Reset()
		w.pending.Write(tmp)
	}

	return total, nil
}

// Flush processes any remaining bytes in the pending buffer.
func (w *MetricsFilterWriter) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.pending.Len() == 0 {
		return
	}

	data := w.pending.Bytes()
	if bytes.Contains(data, []byte(MetricsMarker)) {
		m, _ := ParseMetrics(string(data))
		if m != nil {
			w.metrics = m
			w.found = true
		}
	} else {
		w.dst.Write(data)
	}
	w.pending.Reset()
}

// Metrics returns the extracted metrics if found.
func (w *MetricsFilterWriter) Metrics() (*ExecutionMetrics, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.metrics, w.found
}
