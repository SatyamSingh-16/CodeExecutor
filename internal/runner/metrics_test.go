package runner

import (
	"bytes"
	"strings"
	"testing"
)

func TestMetricsParsing(t *testing.T) {
	t.Run("Valid metrics marker JSON parsing", func(t *testing.T) {
		input := `__EXECUTION_METRICS__ {"wall_time_ms":45,"peak_memory_kb":14200,"exit_code":0}`
		metrics, err := ParseMetrics(input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if metrics.WallTimeMs != 45 {
			t.Errorf("expected WallTimeMs 45, got %d", metrics.WallTimeMs)
		}
		if metrics.PeakMemoryKb != 14200 {
			t.Errorf("expected PeakMemoryKb 14200, got %d", metrics.PeakMemoryKb)
		}
		if metrics.MemoryUsageKb != 14200 {
			t.Errorf("expected MemoryUsageKb 14200, got %d", metrics.MemoryUsageKb)
		}
		if metrics.ExitCode != 0 {
			t.Errorf("expected ExitCode 0, got %d", metrics.ExitCode)
		}
	})

	t.Run("Malformed JSON does not panic and returns error", func(t *testing.T) {
		input := `__EXECUTION_METRICS__ {invalid-json-content`
		metrics, err := ParseMetrics(input)
		if err == nil {
			t.Errorf("expected error on malformed JSON, got nil")
		}
		if metrics != nil {
			t.Errorf("expected nil metrics on error, got %+v", metrics)
		}
	})

	t.Run("Missing fields are handled safely", func(t *testing.T) {
		input := `__EXECUTION_METRICS__ {"wall_time_ms":12}`
		metrics, err := ParseMetrics(input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if metrics.WallTimeMs != 12 {
			t.Errorf("expected WallTimeMs 12, got %d", metrics.WallTimeMs)
		}
		if metrics.PeakMemoryKb != 0 {
			t.Errorf("expected default PeakMemoryKb 0, got %d", metrics.PeakMemoryKb)
		}
		if metrics.ExitCode != 0 {
			t.Errorf("expected default ExitCode 0, got %d", metrics.ExitCode)
		}
	})

	t.Run("ExtractMetrics separates marker from user output", func(t *testing.T) {
		output := "hello world\n__EXECUTION_METRICS__ {\"wall_time_ms\":50,\"peak_memory_kb\":8192,\"exit_code\":0}\n"
		metrics, cleanOutput, found := ExtractMetrics(output)
		if !found {
			t.Fatalf("expected metrics to be found")
		}
		if metrics.WallTimeMs != 50 {
			t.Errorf("expected WallTimeMs 50, got %d", metrics.WallTimeMs)
		}
		if metrics.PeakMemoryKb != 8192 {
			t.Errorf("expected PeakMemoryKb 8192, got %d", metrics.PeakMemoryKb)
		}
		if cleanOutput != "hello world\n" {
			t.Errorf("expected clean output %q, got %q", "hello world\n", cleanOutput)
		}
		if strings.Contains(cleanOutput, MetricsMarker) {
			t.Errorf("clean output must not contain MetricsMarker")
		}
	})

	t.Run("ExtractMetrics with content before and after marker", func(t *testing.T) {
		output := "line 1\n__EXECUTION_METRICS__ {\"wall_time_ms\":30,\"peak_memory_kb\":4096,\"exit_code\":1}\nline 2\n"
		metrics, cleanOutput, found := ExtractMetrics(output)
		if !found {
			t.Fatalf("expected metrics to be found")
		}
		if metrics.ExitCode != 1 {
			t.Errorf("expected ExitCode 1, got %d", metrics.ExitCode)
		}
		if cleanOutput != "line 1\nline 2\n" {
			t.Errorf("expected clean output %q, got %q", "line 1\nline 2\n", cleanOutput)
		}
	})

	t.Run("ExtractMetrics with only marker and newlines", func(t *testing.T) {
		output := "\n__EXECUTION_METRICS__ {\"wall_time_ms\":10,\"peak_memory_kb\":2048,\"exit_code\":0}\n"
		metrics, cleanOutput, found := ExtractMetrics(output)
		if !found {
			t.Fatalf("expected metrics to be found")
		}
		if metrics.WallTimeMs != 10 {
			t.Errorf("expected WallTimeMs 10, got %d", metrics.WallTimeMs)
		}
		if cleanOutput != "" {
			t.Errorf("expected empty clean output, got %q", cleanOutput)
		}
	})

	t.Run("ExtractMetrics with malformed JSON preserves user output", func(t *testing.T) {
		output := "user text\n__EXECUTION_METRICS__ {bad json}\n"
		metrics, cleanOutput, found := ExtractMetrics(output)
		if !found {
			t.Fatalf("expected found true")
		}
		if metrics == nil {
			t.Fatalf("expected non-nil default metrics struct")
		}
		if cleanOutput != "user text\n" {
			t.Errorf("expected clean output %q, got %q", "user text\n", cleanOutput)
		}
	})

	t.Run("ExtractMetrics returns unchanged when marker absent", func(t *testing.T) {
		output := "regular output without metrics\n"
		metrics, cleanOutput, found := ExtractMetrics(output)
		if found {
			t.Errorf("expected found false")
		}
		if metrics != nil {
			t.Errorf("expected nil metrics, got %+v", metrics)
		}
		if cleanOutput != output {
			t.Errorf("expected unchanged output")
		}
	})
}

func TestMetricsFilterWriter(t *testing.T) {
	t.Run("Intercepts metrics line and writes only user output to LimitedBuffer", func(t *testing.T) {
		lim := NewLimitedBuffer(64 * 1024)
		filter := NewMetricsFilterWriter(lim)

		stream := "first line\n__EXECUTION_METRICS__ {\"wall_time_ms\":42,\"peak_memory_kb\":5000,\"exit_code\":0}\nsecond line\n"
		_, err := filter.Write([]byte(stream))
		if err != nil {
			t.Fatalf("write error: %v", err)
		}
		filter.Flush()

		m, found := filter.Metrics()
		if !found {
			t.Fatalf("expected metrics found")
		}
		if m.WallTimeMs != 42 || m.PeakMemoryKb != 5000 {
			t.Errorf("unexpected metrics: %+v", m)
		}
		if lim.String() != "first line\nsecond line\n" {
			t.Errorf("expected buffer 'first line\\nsecond line\\n', got %q", lim.String())
		}
		if lim.Truncated() {
			t.Errorf("expected not truncated")
		}
	})

	t.Run("Captures trailing metrics even when user output exceeds 64 KB", func(t *testing.T) {
		lim := NewLimitedBuffer(64 * 1024)
		filter := NewMetricsFilterWriter(lim)

		// Write 100 KB of user output
		largeChunk := bytes.Repeat([]byte("error log line\n"), 7000) // ~105 KB
		_, err := filter.Write(largeChunk)
		if err != nil {
			t.Fatalf("write error: %v", err)
		}

		// Trailing metrics from entrypoint
		metricsLine := []byte("\n__EXECUTION_METRICS__ {\"wall_time_ms\":99,\"peak_memory_kb\":12000,\"exit_code\":0}\n")
		_, err = filter.Write(metricsLine)
		if err != nil {
			t.Fatalf("write error: %v", err)
		}
		filter.Flush()

		// Buffer was capped at 64 KB
		if lim.Len() != 64*1024 {
			t.Errorf("expected buffer capped at 64 KB, got %d", lim.Len())
		}
		if !lim.Truncated() {
			t.Errorf("expected buffer truncated")
		}

		// Metrics were still successfully captured!
		m, found := filter.Metrics()
		if !found {
			t.Fatalf("expected metrics captured even after 64 KB user output")
		}
		if m.WallTimeMs != 99 || m.PeakMemoryKb != 12000 {
			t.Errorf("unexpected metrics: %+v", m)
		}
	})
}
