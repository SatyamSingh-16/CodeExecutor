#!/usr/bin/env bash
# test_runner_images.sh - Automated verification suite for Ticket 01 runner base images.
set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

PASSED_COUNT=0
FAILED_COUNT=0

assert_test() {
  local desc="$1"
  local cmd="$2"
  printf "Running: %s ... " "$desc"
  if eval "$cmd" > /tmp/test_out.log 2> /tmp/test_err.log; then
    printf "${GREEN}PASS${NC}\n"
    PASSED_COUNT=$((PASSED_COUNT + 1))
  else
    printf "${RED}FAIL${NC}\n"
    echo "--- STDOUT ---"
    cat /tmp/test_out.log
    echo "--- STDERR ---"
    cat /tmp/test_err.log
    FAILED_COUNT=$((FAILED_COUNT + 1))
  fi
}

echo -e "${BLUE}=== Starting Ticket 01 Runner Base Images Verification ===${NC}"

# Test 1: Python image exists
assert_test "Check Python runner image exists" \
  "docker image inspect code-executor-runner-python:latest >/dev/null"

# Test 2: Go image exists
assert_test "Check Go runner image exists" \
  "docker image inspect code-executor-runner-go:latest >/dev/null"

# Test 3: Python execution with --read-only, --user 1000:1000, and --tmpfs /tmp
assert_test "Execute Python script in read-only container as user 1000:1000" \
  "docker run --rm --read-only --user 1000:1000 --tmpfs /tmp:rw,size=64m code-executor-runner-python:latest python3 -c 'print(\"Hello from Python\")'"

# Test 4: Verify Python user is not root (UID 1000)
assert_test "Verify Python execution user is UID 1000 (non-root)" \
  "res=\$(docker run --rm --read-only --user 1000:1000 --tmpfs /tmp:rw,size=64m code-executor-runner-python:latest id -u); [ \"\$res\" = \"1000\" ]"

# Test 5: Verify writes outside /tmp fail under read-only rootfs
assert_test "Verify writes outside /tmp fail under read-only rootfs" \
  "! docker run --rm --read-only --user 1000:1000 --tmpfs /tmp:rw,size=64m code-executor-runner-python:latest python3 -c 'open(\"/etc/test.txt\", \"w\").write(\"fail\")'"

# Test 6: Verify stdin is forwarded correctly in Python
assert_test "Verify stdin is forwarded correctly in Python" \
  "res=\$(echo 'World' | docker run --rm -i --read-only --user 1000:1000 --tmpfs /tmp:rw,size=64m code-executor-runner-python:latest python3 -c 'import sys; print(\"Hello \" + sys.stdin.read().strip())'); echo \"\$res\" | grep -q 'Hello World'"

# Test 7: Verify stdout and stderr are separated and captured
assert_test "Verify stdout and stderr are separated" \
  "docker run --rm --read-only --user 1000:1000 --tmpfs /tmp:rw,size=64m code-executor-runner-python:latest python3 -c 'import sys; sys.stdout.write(\"STDOUT_TEST\n\"); sys.stderr.write(\"STDERR_TEST\n\")' 1>/tmp/out.txt 2>/tmp/err.txt && grep -q 'STDOUT_TEST' /tmp/out.txt && grep -q 'STDERR_TEST' /tmp/err.txt"

# Test 8: Verify execution metrics are emitted with structured delimiter
assert_test "Verify execution metrics are emitted with structured delimiter" \
  "docker run --rm --read-only --user 1000:1000 --tmpfs /tmp:rw,size=64m code-executor-runner-python:latest python3 -c 'print(42)' 2>/tmp/err.txt && grep -q '__EXECUTION_METRICS__' /tmp/err.txt && grep -q '\"wall_time_ms\":' /tmp/err.txt && grep -q '\"peak_memory_kb\":' /tmp/err.txt && grep -q '\"exit_code\":0' /tmp/err.txt"

# Test 9: Verify non-zero exit code is preserved and recorded in metrics
assert_test "Verify non-zero exit code is preserved and recorded" \
  "! docker run --rm --read-only --user 1000:1000 --tmpfs /tmp:rw,size=64m code-executor-runner-python:latest python3 -c 'import sys; sys.exit(42)' 2>/tmp/err.txt && grep -q '\"exit_code\":42' /tmp/err.txt"

# Test 10: Go execution in read-only container as user 1000:1000
assert_test "Execute compiled Go binary in read-only container as user 1000:1000" \
  "docker run --rm --read-only --user 1000:1000 --tmpfs /tmp:rw,exec,size=64m code-executor-runner-go:latest sh -c 'echo \"package main; import \\\"fmt\\\"; func main() { fmt.Println(\\\"Go OK\\\") }\" > /tmp/main.go && go build -o /tmp/app /tmp/main.go && /tmp/app'"

# Test 11: Verify Go execution user is UID 1000 (non-root)
assert_test "Verify Go execution user is UID 1000 (non-root)" \
  "res=\$(docker run --rm --read-only --user 1000:1000 --tmpfs /tmp:rw,exec,size=64m code-executor-runner-go:latest id -u); [ \"\$res\" = \"1000\" ]"

# Test 12: Verify Go execution metrics are emitted with structured delimiter
assert_test "Verify Go execution metrics are emitted with structured delimiter" \
  "docker run --rm --read-only --user 1000:1000 --tmpfs /tmp:rw,exec,size=64m code-executor-runner-go:latest sh -c 'echo \"package main; import \\\"fmt\\\"; func main() { fmt.Println(\\\"Go Metrics Test\\\") }\" > /tmp/main.go && go build -o /tmp/app /tmp/main.go && /runner/entrypoint.sh /tmp/app' 2>/tmp/err.txt && grep -q '__EXECUTION_METRICS__' /tmp/err.txt && grep -q '\"exit_code\":0' /tmp/err.txt"

echo -e "\n${BLUE}=== Test Summary ===${NC}"
echo -e "Passed: ${GREEN}${PASSED_COUNT}${NC}"
echo -e "Failed: ${RED}${FAILED_COUNT}${NC}"

if [ "$FAILED_COUNT" -gt 0 ]; then
  exit 1
fi

echo -e "${GREEN}All Ticket 01 verification tests passed successfully!${NC}"
