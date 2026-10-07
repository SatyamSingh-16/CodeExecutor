//go:build integration

package adversarial

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/SatyamSingh-16/code_executor/internal/runner"
	"github.com/docker/docker/client"
)

// setupRunner initializes a Docker client and DockerRunner for integration tests.
func setupRunner(t *testing.T) (*runner.DockerRunner, *client.Client, context.Context) {
	t.Helper()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		t.Fatalf("failed to connect to Docker daemon: %v", err)
	}
	t.Cleanup(func() {
		_ = cli.Close()
	})

	r := runner.NewDockerRunner(cli)
	t.Cleanup(func() {
		_ = r.Close()
	})

	ctx := context.Background()
	return r, cli, ctx
}

// 1. TIME LIMIT EXCEEDED
func TestAdversarialTimeoutKill(t *testing.T) {
	r, cli, ctx := setupRunner(t)

	t.Run("Python infinite loop terminated under hard deadline", func(t *testing.T) {
		req := runner.ExecutionRequest{
			Language: runner.LanguagePython,
			Code:     "while True:\n    pass",
			Timeout:  2 * time.Second,
		}

		start := time.Now()
		res, err := r.Execute(ctx, req)
		elapsed := time.Since(start)

		if err != nil {
			t.Fatalf("unexpected execution error: %v", err)
		}

		if !res.TimedOut {
			t.Errorf("expected TimedOut true for infinite loop")
		}
		if res.Status != runner.StatusTimeLimitExceeded {
			t.Errorf("expected Status TIME_LIMIT_EXCEEDED, got %q", res.Status)
		}
		if res.ExitCode != 137 {
			t.Errorf("expected ExitCode 137 (SIGKILL), got %d", res.ExitCode)
		}
		if elapsed > 4*time.Second {
			t.Errorf("expected timeout execution to terminate in ~2s, took %v", elapsed)
		}

		// Verify container is removed from Docker daemon
		_, inspectErr := cli.ContainerInspect(ctx, res.ContainerID)
		if !client.IsErrNotFound(inspectErr) {
			t.Errorf("expected container %s to be removed after timeout, inspect err: %v", res.ContainerID, inspectErr)
		}
	})

	t.Run("Go runtime infinite loop terminated under hard deadline", func(t *testing.T) {
		req := runner.ExecutionRequest{
			Language: runner.LanguageGo,
			Code: `package main
func main() {
	for {}
}`,
			Timeout: 2 * time.Second,
		}

		start := time.Now()
		res, err := r.Execute(ctx, req)
		elapsed := time.Since(start)

		if err != nil {
			t.Fatalf("unexpected execution error: %v", err)
		}

		if res.IsCompileError {
			t.Fatalf("expected compilation to succeed, got: %s", res.CompilationOutput)
		}
		if !res.TimedOut {
			t.Errorf("expected TimedOut true for Go infinite loop")
		}
		if res.Status != runner.StatusTimeLimitExceeded {
			t.Errorf("expected Status TIME_LIMIT_EXCEEDED, got %q", res.Status)
		}
		if res.ExitCode != 137 {
			t.Errorf("expected ExitCode 137 (SIGKILL), got %d", res.ExitCode)
		}
		if res.Duration > 4*time.Second {
			t.Errorf("expected Go runtime duration to terminate in ~2s, took %v", res.Duration)
		}
		if elapsed > 10*time.Second {
			t.Errorf("expected Go two-phase execution to complete within 10s, took %v", elapsed)
		}

		// Verify runtime container is removed
		_, inspectErr := cli.ContainerInspect(ctx, res.ContainerID)
		if !client.IsErrNotFound(inspectErr) {
			t.Errorf("expected runtime container %s to be removed, inspect err: %v", res.ContainerID, inspectErr)
		}
	})
}

// 2. MEMORY LIMIT EXCEEDED
func TestAdversarialMemoryLimitExceeded(t *testing.T) {
	r, cli, ctx := setupRunner(t)

	t.Run("Python 500MB allocation killed by cgroup 128MB limit", func(t *testing.T) {
		req := runner.ExecutionRequest{
			Language: runner.LanguagePython,
			// Allocate 500 MB under 128 MB sandbox limit
			Code: `
import sys
try:
    a = bytearray(500 * 1024 * 1024)
    print("ALLOCATED")
except MemoryError:
    print("PYTHON_MEMORY_ERROR")
    sys.exit(1)
`,
			Timeout: 5 * time.Second,
		}

		res, err := r.Execute(ctx, req)
		if err != nil {
			t.Fatalf("unexpected execution error: %v", err)
		}

		if res.Status != runner.StatusMemoryLimitExceeded && res.Status != runner.StatusRuntimeError {
			t.Errorf("expected Status MEMORY_LIMIT_EXCEEDED or RUNTIME_ERROR, got %q", res.Status)
		}
		if res.ExitCode == 0 {
			t.Errorf("expected non-zero exit code due to OOM kill, got 0")
		}
		if strings.Contains(res.Stdout, "ALLOCATED") {
			t.Errorf("hostile program must not successfully allocate 500 MB under 128 MB limit")
		}

		// Verify container is removed
		_, inspectErr := cli.ContainerInspect(ctx, res.ContainerID)
		if !client.IsErrNotFound(inspectErr) {
			t.Errorf("expected container %s to be removed after OOM, inspect err: %v", res.ContainerID, inspectErr)
		}
	})

	t.Run("Go 300MB heap allocation killed by cgroup 128MB limit", func(t *testing.T) {
		req := runner.ExecutionRequest{
			Language: runner.LanguageGo,
			Code: `package main
func main() {
	buf := make([][]byte, 300)
	for i := range buf {
		buf[i] = make([]byte, 1024*1024)
		for j := range buf[i] {
			buf[i][j] = 1
		}
	}
}`,
			Timeout: 5 * time.Second,
		}

		res, err := r.Execute(ctx, req)
		if err != nil {
			t.Fatalf("unexpected execution error: %v", err)
		}

		if res.IsCompileError {
			t.Fatalf("expected successful compilation, got compile error: %s", res.CompilationOutput)
		}
		if res.Status != runner.StatusMemoryLimitExceeded && res.Status != runner.StatusRuntimeError {
			t.Errorf("expected Status MEMORY_LIMIT_EXCEEDED or RUNTIME_ERROR, got %q", res.Status)
		}
		if res.ExitCode == 0 {
			t.Errorf("expected non-zero exit code, got 0")
		}

		// Verify container is removed
		_, inspectErr := cli.ContainerInspect(ctx, res.ContainerID)
		if !client.IsErrNotFound(inspectErr) {
			t.Errorf("expected container %s to be removed, inspect err: %v", res.ContainerID, inspectErr)
		}
	})
}

// 3. PID / FORK BOMB RESISTANCE
func TestAdversarialForkBombContainment(t *testing.T) {
	r, cli, ctx := setupRunner(t)

	t.Run("PidsLimit strictly blocks process explosion and prevents host starvation", func(t *testing.T) {
		req := runner.ExecutionRequest{
			Language: runner.LanguagePython,
			Code: `
import os, sys, time
created = 0
for i in range(200):
    try:
        pid = os.fork()
        if pid == 0:
            time.sleep(2)
            sys.exit(0)
        created += 1
    except (BlockingIOError, OSError) as e:
        print(f"FORK_BLOCKED_AT_{created}:{type(e).__name__}")
        sys.exit(0)
print(f"SPAWNED_{created}")
`,
			Timeout: 5 * time.Second,
		}

		res, err := r.Execute(ctx, req)
		if err != nil {
			t.Fatalf("unexpected execution error: %v", err)
		}

		// Verify fork was blocked before reaching 200 processes
		if strings.Contains(res.Stdout, "SPAWNED_200") {
			t.Errorf("sandbox failed to contain fork bomb; spawned 200 processes!")
		}
		if !strings.Contains(res.Stdout, "FORK_BLOCKED_AT_") {
			t.Errorf("expected fork to be blocked by PidsLimit, got stdout: %q, stderr: %q", res.Stdout, res.Stderr)
		}

		// Verify container is cleaned up
		_, inspectErr := cli.ContainerInspect(ctx, res.ContainerID)
		if !client.IsErrNotFound(inspectErr) {
			t.Errorf("expected container %s to be removed, inspect err: %v", res.ContainerID, inspectErr)
		}
	})

	t.Run("Aggressive fork bomb loop safely contained and killed by timeout", func(t *testing.T) {
		req := runner.ExecutionRequest{
			Language: runner.LanguagePython,
			Code: `
import os, time
while True:
    try:
        os.fork()
    except Exception:
        pass
    time.sleep(0.005)
`,
			Timeout: 2 * time.Second,
		}

		start := time.Now()
		res, err := r.Execute(ctx, req)
		elapsed := time.Since(start)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if elapsed > 4*time.Second {
			t.Errorf("host responsiveness degraded; took %v to contain fork loop", elapsed)
		}

		if !res.TimedOut {
			t.Errorf("expected TimedOut true for infinite fork loop")
		}

		// Verify container is removed
		_, inspectErr := cli.ContainerInspect(ctx, res.ContainerID)
		if !client.IsErrNotFound(inspectErr) {
			t.Errorf("expected container %s to be removed, inspect err: %v", res.ContainerID, inspectErr)
		}
	})
}

// 4. NETWORK ISOLATION
func TestAdversarialNetworkIsolation(t *testing.T) {
	r, cli, ctx := setupRunner(t)

	t.Run("Python outbound TCP connection fails deterministically", func(t *testing.T) {
		req := runner.ExecutionRequest{
			Language: runner.LanguagePython,
			Code: `
import socket, sys
try:
    s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    s.settimeout(2.0)
    s.connect(("1.1.1.1", 80))
    print("CONNECTED")
except OSError as e:
    print(f"NETWORK_BLOCKED:{type(e).__name__}:{e.errno}")
`,
			Timeout: 5 * time.Second,
		}

		res, err := r.Execute(ctx, req)
		if err != nil {
			t.Fatalf("unexpected execution error: %v", err)
		}

		if strings.Contains(res.Stdout, "CONNECTED") {
			t.Errorf("hostile program breached network isolation: connected to external IP!")
		}
		if !strings.Contains(res.Stdout, "NETWORK_BLOCKED") {
			t.Errorf("expected socket connection to fail with network error, got stdout: %q, stderr: %q", res.Stdout, res.Stderr)
		}

		// Verify container is cleaned up
		_, inspectErr := cli.ContainerInspect(ctx, res.ContainerID)
		if !client.IsErrNotFound(inspectErr) {
			t.Errorf("expected container %s to be removed, inspect err: %v", res.ContainerID, inspectErr)
		}
	})

	t.Run("Network interface inspection shows only loopback interface", func(t *testing.T) {
		req := runner.ExecutionRequest{
			Language: runner.LanguagePython,
			Code: `
import os
try:
    with open("/proc/net/dev", "r") as f:
        interfaces = f.read()
    print("INTERFACES:\n" + interfaces)
except Exception as e:
    print(f"ERR:{e}")
`,
			Timeout: 5 * time.Second,
		}

		res, err := r.Execute(ctx, req)
		if err != nil {
			t.Fatalf("unexpected execution error: %v", err)
		}

		if strings.Contains(res.Stdout, "eth0") || strings.Contains(res.Stdout, "ens") {
			t.Errorf("external network interface detected in sandbox! stdout: %s", res.Stdout)
		}
		if !strings.Contains(res.Stdout, "lo:") {
			t.Errorf("expected loopback interface present in /proc/net/dev, got: %s", res.Stdout)
		}

		// Verify container cleaned up
		_, inspectErr := cli.ContainerInspect(ctx, res.ContainerID)
		if !client.IsErrNotFound(inspectErr) {
			t.Errorf("expected container %s to be removed, inspect err: %v", res.ContainerID, inspectErr)
		}
	})
}

// 5. FILESYSTEM INTEGRITY
func TestAdversarialFilesystemIntegrity(t *testing.T) {
	r, cli, ctx := setupRunner(t)

	t.Run("Writes to protected root filesystem locations fail with Read-only file system", func(t *testing.T) {
		req := runner.ExecutionRequest{
			Language: runner.LanguagePython,
			Code: `
import sys
targets = ["/etc/hacked", "/bin/evil", "/usr/local/bin/backdoor", "/evil.txt"]
blocked_count = 0
for t in targets:
    try:
        with open(t, "w") as f:
            f.write("owned")
        print(f"BREACH:{t}")
    except OSError as e:
        if "Read-only file system" in str(e) or e.errno == 30:
            blocked_count += 1
            print(f"PROTECTED:{t}")
        else:
            print(f"FAILED_OTHER:{t}:{e}")

# Verify /tmp is writable as configured
try:
    with open("/tmp/test_write.txt", "w") as f:
        f.write("ok")
    print("TMP_WRITABLE_OK")
except Exception as e:
    print(f"TMP_FAILED:{e}")

if blocked_count == len(targets):
    print("ALL_PROTECTED_OK")
`,
			Timeout: 5 * time.Second,
		}

		res, err := r.Execute(ctx, req)
		if err != nil {
			t.Fatalf("unexpected execution error: %v", err)
		}

		if strings.Contains(res.Stdout, "BREACH:") {
			t.Errorf("hostile program breached read-only rootfs! stdout: %s", res.Stdout)
		}
		if !strings.Contains(res.Stdout, "ALL_PROTECTED_OK") {
			t.Errorf("expected all protected locations to be read-only, stdout: %s", res.Stdout)
		}
		if !strings.Contains(res.Stdout, "TMP_WRITABLE_OK") {
			t.Errorf("expected /tmp to be writable, stdout: %s", res.Stdout)
		}

		// Verify container cleaned up
		_, inspectErr := cli.ContainerInspect(ctx, res.ContainerID)
		if !client.IsErrNotFound(inspectErr) {
			t.Errorf("expected container %s to be removed, inspect err: %v", res.ContainerID, inspectErr)
		}
	})

	t.Run("Go binary cannot write to protected root filesystem", func(t *testing.T) {
		req := runner.ExecutionRequest{
			Language: runner.LanguageGo,
			Code: `package main
import (
	"fmt"
	"os"
)
func main() {
	err := os.WriteFile("/etc/hacked", []byte("bad"), 0644)
	if err != nil {
		fmt.Printf("GO_PROTECTED:%v\n", err)
	} else {
		fmt.Println("GO_BREACH")
	}
}`,
			Timeout: 5 * time.Second,
		}

		res, err := r.Execute(ctx, req)
		if err != nil {
			t.Fatalf("unexpected execution error: %v", err)
		}

		if strings.Contains(res.Stdout, "GO_BREACH") {
			t.Errorf("Go binary breached read-only rootfs!")
		}
		if !strings.Contains(res.Stdout, "GO_PROTECTED") {
			t.Errorf("expected Go write to fail on read-only rootfs, stdout: %s", res.Stdout)
		}

		// Verify container cleaned up
		_, inspectErr := cli.ContainerInspect(ctx, res.ContainerID)
		if !client.IsErrNotFound(inspectErr) {
			t.Errorf("expected container %s to be removed, inspect err: %v", res.ContainerID, inspectErr)
		}
	})
}

// 6. OUTPUT FLOODING
func TestAdversarialOutputFlooding(t *testing.T) {
	r, cli, ctx := setupRunner(t)

	t.Run("10MB stdout log bomb strictly truncated to 64KB without process deadlock", func(t *testing.T) {
		req := runner.ExecutionRequest{
			Language: runner.LanguagePython,
			Code: `
import sys
# Output ~10 MB of text
for _ in range(100000):
    sys.stdout.write("A" * 100 + "\n")
`,
			Timeout: 5 * time.Second,
		}

		res, err := r.Execute(ctx, req)
		if err != nil {
			t.Fatalf("unexpected execution error: %v", err)
		}

		if res.Status != runner.StatusSuccess {
			t.Errorf("expected Status SUCCESS, got %q", res.Status)
		}
		if !res.StdoutTruncated {
			t.Errorf("expected StdoutTruncated true for 10MB flood")
		}
		if res.StderrTruncated {
			t.Errorf("expected StderrTruncated false")
		}
		if len(res.Stdout) != 64*1024 {
			t.Errorf("expected captured stdout length exactly 65536, got %d", len(res.Stdout))
		}

		// Verify container cleaned up
		_, inspectErr := cli.ContainerInspect(ctx, res.ContainerID)
		if !client.IsErrNotFound(inspectErr) {
			t.Errorf("expected container %s to be removed, inspect err: %v", res.ContainerID, inspectErr)
		}
	})

	t.Run("10MB stderr log bomb strictly truncated to 64KB and metrics still parsed", func(t *testing.T) {
		req := runner.ExecutionRequest{
			Language: runner.LanguagePython,
			Code: `
import sys
# Output ~10 MB of error text
for _ in range(100000):
    sys.stderr.write("E" * 100 + "\n")
`,
			Timeout: 5 * time.Second,
		}

		res, err := r.Execute(ctx, req)
		if err != nil {
			t.Fatalf("unexpected execution error: %v", err)
		}

		if res.Status != runner.StatusSuccess {
			t.Errorf("expected Status SUCCESS, got %q", res.Status)
		}
		if !res.StderrTruncated {
			t.Errorf("expected StderrTruncated true for 10MB flood")
		}
		if len(res.Stderr) != 64*1024 {
			t.Errorf("expected captured stderr length exactly 65536, got %d", len(res.Stderr))
		}
		if res.WallTimeMs <= 0 {
			t.Errorf("expected valid parsed WallTimeMs even after 10MB stderr flood, got %d", res.WallTimeMs)
		}

		// Verify container cleaned up
		_, inspectErr := cli.ContainerInspect(ctx, res.ContainerID)
		if !client.IsErrNotFound(inspectErr) {
			t.Errorf("expected container %s to be removed, inspect err: %v", res.ContainerID, inspectErr)
		}
	})
}

// 7. PRIVILEGE CONTAINMENT (Non-root user verification)
func TestAdversarialPrivilegeContainment(t *testing.T) {
	r, cli, ctx := setupRunner(t)

	t.Run("Process executes strictly as UID/GID 1000 and cannot escalate", func(t *testing.T) {
		req := runner.ExecutionRequest{
			Language: runner.LanguagePython,
			Code: `
import os, sys
uid = os.getuid()
gid = os.getgid()
print(f"UID:{uid},GID:{gid}")

# Attempt to escalate to root
try:
    os.setuid(0)
    print("ESCALATED_ROOT")
except PermissionError:
    print("SETUID_DENIED")
except Exception as e:
    print(f"SETUID_ERROR:{e}")
`,
			Timeout: 5 * time.Second,
		}

		res, err := r.Execute(ctx, req)
		if err != nil {
			t.Fatalf("unexpected execution error: %v", err)
		}

		if !strings.Contains(res.Stdout, "UID:1000,GID:1000") {
			t.Errorf("expected process to run as UID:1000,GID:1000, got stdout: %s", res.Stdout)
		}
		if strings.Contains(res.Stdout, "ESCALATED_ROOT") {
			t.Errorf("privilege escalation vulnerability: process setuid(0) succeeded!")
		}
		if !strings.Contains(res.Stdout, "SETUID_DENIED") {
			t.Errorf("expected setuid(0) to be denied with PermissionError, got: %s", res.Stdout)
		}

		// Verify container cleaned up
		_, inspectErr := cli.ContainerInspect(ctx, res.ContainerID)
		if !client.IsErrNotFound(inspectErr) {
			t.Errorf("expected container %s to be removed, inspect err: %v", res.ContainerID, inspectErr)
		}
	})
}
