#!/bin/sh
# entrypoint.sh - In-container instrumentation wrapper for code execution.
# Executes user code, forwards standard streams, and outputs execution metrics.

if [ $# -eq 0 ]; then
  echo "Usage: /runner/entrypoint.sh <command> [args...]" >&2
  exit 1
fi

TIME_FILE="/tmp/.time.log"

# Record start time in nanoseconds
START_NS=$(date +%s%N 2>/dev/null || date +%s)

# Execute user command under GNU time to capture peak memory usage (in KB)
set +e
/usr/bin/time -o "$TIME_FILE" -f "peak_memory_kb=%M" "$@"
EXIT_CODE=$?

# Record end time in nanoseconds
END_NS=$(date +%s%N 2>/dev/null || date +%s)

# Calculate elapsed wall time in milliseconds
if [ "$START_NS" -gt 1000000000000000 ] 2>/dev/null; then
  WALL_TIME_MS=$(( (END_NS - START_NS) / 1000000 ))
else
  # Fallback if nanoseconds not supported (seconds * 1000)
  WALL_TIME_MS=$(( (END_NS - START_NS) * 1000 ))
fi

# Extract peak memory in KB from time output
PEAK_MEM_KB=0
if [ -f "$TIME_FILE" ]; then
  PEAK_MEM_KB=$(grep -oE "peak_memory_kb=[0-9]+" "$TIME_FILE" | cut -d= -f2 || echo 0)
  rm -f "$TIME_FILE" 2>/dev/null || true
fi

# Ensure peak memory is an integer
case "$PEAK_MEM_KB" in
  ''|*[!0-9]*) PEAK_MEM_KB=0 ;;
esac

# Format JSON metrics payload
METRICS_JSON="{\"wall_time_ms\":$WALL_TIME_MS,\"peak_memory_kb\":$PEAK_MEM_KB,\"exit_code\":$EXIT_CODE}"

# Emit structured metrics delimiter to stderr
printf "\n__EXECUTION_METRICS__ %s\n" "$METRICS_JSON" >&2

# Write metrics JSON file to /tmp if writable
echo "$METRICS_JSON" > /tmp/.metrics.json 2>/dev/null || true

exit "$EXIT_CODE"
