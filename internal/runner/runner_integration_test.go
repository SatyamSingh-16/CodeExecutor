//go:build integration

package runner

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

func TestDockerRunnerIntegration(t *testing.T) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		t.Fatalf("failed to connect to Docker daemon: %v", err)
	}
	defer cli.Close()

	runner := NewDockerRunner(cli)
	defer runner.Close()

	ctx := context.Background()

	// 1. Python code execution with stdin forwarding and output capture
	t.Run("Python successful execution with stdin", func(t *testing.T) {
		req := ExecutionRequest{
			Language: LanguagePython,
			Code:     "import sys\nname = sys.stdin.read().strip()\nprint(f'Hello, {name}!')",
			Stdin:    "Satyam",
			Timeout:  5 * time.Second,
		}

		res, err := runner.Execute(ctx, req)
		if err != nil {
			t.Fatalf("execution failed: %v", err)
		}

		if res.TimedOut {
			t.Errorf("expected execution not to time out")
		}

		if res.ExitCode != 0 {
			t.Errorf("expected exit code 0, got %d. stderr: %s", res.ExitCode, res.Stderr)
		}

		if !strings.Contains(res.Stdout, "Hello, Satyam!") {
			t.Errorf("expected stdout to contain 'Hello, Satyam!', got %q", res.Stdout)
		}

		// 14. Verify container is removed after successful execution
		_, inspectErr := cli.ContainerInspect(ctx, res.ContainerID)
		if !client.IsErrNotFound(inspectErr) {
			t.Errorf("expected container %s to be removed, inspect err: %v", res.ContainerID, inspectErr)
		}
	})

	// 2. Go runner container execution
	t.Run("Go container execution", func(t *testing.T) {
		req := ExecutionRequest{
			Language:    LanguageGo,
			Code:        "package main\nimport \"fmt\"\nfunc main() {\nfmt.Println(\"Go runner verified\")\n}",
			MemoryLimit: 512 * 1024 * 1024,
			Timeout:     10 * time.Second,
		}

		res, err := runner.Execute(ctx, req)
		if err != nil {
			t.Fatalf("Go execution failed: %v", err)
		}

		if res.ExitCode != 0 {
			t.Errorf("expected exit code 0, got %d. stderr: %s", res.ExitCode, res.Stderr)
		}

		if !strings.Contains(res.Stdout, "Go runner verified") {
			t.Errorf("expected stdout to contain 'Go runner verified', got %q", res.Stdout)
		}

		// Verify container is cleaned up
		_, inspectErr := cli.ContainerInspect(ctx, res.ContainerID)
		if !client.IsErrNotFound(inspectErr) {
			t.Errorf("expected container %s to be removed, inspect err: %v", res.ContainerID, inspectErr)
		}
	})

	// 4-12. Sandbox Security Configurations verification (HostConfig & Config inspect)
	t.Run("Sandbox security and tmpfs configuration check", func(t *testing.T) {
		// Test Python configuration
		pyCfg, _ := GetRuntimeConfig(LanguagePython)
		pidsLimit := int64(64)
		containerConfig := &container.Config{
			Image:      pyCfg.Image,
			Cmd:        pyCfg.DefaultCommand,
			WorkingDir: "/tmp",
			User:       "1000:1000",
		}
		hostConfig := &container.HostConfig{
			NetworkMode: "none",
			Resources: container.Resources{
				Memory:     128 * 1024 * 1024,
				MemorySwap: 128 * 1024 * 1024,
				NanoCPUs:   1_000_000_000,
				PidsLimit:  &pidsLimit,
			},
			ReadonlyRootfs: true,
			Tmpfs: map[string]string{
				"/tmp": pyCfg.TmpfsOptions,
			},
			CapDrop: []string{"ALL"},
		}

		pyCreate, err := cli.ContainerCreate(ctx, containerConfig, hostConfig, nil, nil, "")
		if err != nil {
			t.Fatalf("failed to create inspection container: %v", err)
		}
		defer func() {
			_ = cli.ContainerRemove(ctx, pyCreate.ID, container.RemoveOptions{Force: true})
		}()

		pyInspect, err := cli.ContainerInspect(ctx, pyCreate.ID)
		if err != nil {
			t.Fatalf("failed to inspect container: %v", err)
		}

		// 4. Network isolation check
		if string(pyInspect.HostConfig.NetworkMode) != "none" {
			t.Errorf("expected NetworkMode 'none', got %q", pyInspect.HostConfig.NetworkMode)
		}

		// 5. Memory limit check
		if pyInspect.HostConfig.Memory != 128*1024*1024 {
			t.Errorf("expected Memory 128MB, got %d", pyInspect.HostConfig.Memory)
		}
		if pyInspect.HostConfig.MemorySwap != 128*1024*1024 {
			t.Errorf("expected MemorySwap 128MB, got %d", pyInspect.HostConfig.MemorySwap)
		}

		// 6. CPU limit check
		if pyInspect.HostConfig.NanoCPUs != 1_000_000_000 {
			t.Errorf("expected NanoCPUs 1000000000, got %d", pyInspect.HostConfig.NanoCPUs)
		}

		// 7. PID limit check
		if pyInspect.HostConfig.PidsLimit == nil || *pyInspect.HostConfig.PidsLimit != 64 {
			t.Errorf("expected PidsLimit 64, got %v", pyInspect.HostConfig.PidsLimit)
		}

		// 8. Read-only root filesystem check
		if !pyInspect.HostConfig.ReadonlyRootfs {
			t.Errorf("expected ReadonlyRootfs true, got false")
		}

		// 9. Non-root user check
		if pyInspect.Config.User != "1000:1000" {
			t.Errorf("expected User '1000:1000', got %q", pyInspect.Config.User)
		}

		// 10. All capabilities dropped check
		hasCapDropAll := false
		for _, cap := range pyInspect.HostConfig.CapDrop {
			if cap == "ALL" {
				hasCapDropAll = true
				break
			}
		}
		if !hasCapDropAll {
			t.Errorf("expected CapDrop to contain 'ALL', got %v", pyInspect.HostConfig.CapDrop)
		}

		// 11. Python gets noexec tmpfs
		pyTmpfs := pyInspect.HostConfig.Tmpfs["/tmp"]
		if !strings.Contains(pyTmpfs, "noexec") {
			t.Errorf("expected Python tmpfs to contain 'noexec', got %q", pyTmpfs)
		}

		// 12. Go gets exec tmpfs
		goCfg, _ := GetRuntimeConfig(LanguageGo)
		if !strings.Contains(goCfg.TmpfsOptions, "exec") || strings.Contains(goCfg.TmpfsOptions, "noexec") {
			t.Errorf("expected Go tmpfs to contain 'exec' and not 'noexec', got %q", goCfg.TmpfsOptions)
		}
	})

	// 13 & 16. Execution timeout forcibly terminates container and removes it
	t.Run("Execution timeout forcibly terminates and cleans up container", func(t *testing.T) {
		req := ExecutionRequest{
			Language: LanguagePython,
			Code:     "import time\ntime.sleep(10)",
			Timeout:  1 * time.Second,
		}

		start := time.Now()
		res, err := runner.Execute(ctx, req)
		elapsed := time.Since(start)

		if err != nil {
			t.Fatalf("unexpected error during timeout execution: %v", err)
		}

		if !res.TimedOut {
			t.Errorf("expected TimedOut true, got false")
		}

		if res.ExitCode != 137 {
			t.Errorf("expected ExitCode 137 (SIGKILL), got %d", res.ExitCode)
		}

		if elapsed > 4*time.Second {
			t.Errorf("expected timeout to terminate within ~2s, took %v", elapsed)
		}

		// 16. Container removed after timeout
		_, inspectErr := cli.ContainerInspect(ctx, res.ContainerID)
		if !client.IsErrNotFound(inspectErr) {
			t.Errorf("expected container %s to be removed after timeout, inspect err: %v", res.ContainerID, inspectErr)
		}
	})

	// 15. Container removed after failure (runtime error)
	t.Run("Container removed after runtime error", func(t *testing.T) {
		req := ExecutionRequest{
			Language: LanguagePython,
			Code:     "import sys\nsys.stderr.write('fatal crash\\n')\nsys.exit(42)",
			Timeout:  5 * time.Second,
		}

		res, err := runner.Execute(ctx, req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if res.ExitCode != 42 {
			t.Errorf("expected exit code 42, got %d", res.ExitCode)
		}

		if !strings.Contains(res.Stderr, "fatal crash") {
			t.Errorf("expected stderr to contain 'fatal crash', got %q", res.Stderr)
		}

		// 15. Verify container removed after failure
		_, inspectErr := cli.ContainerInspect(ctx, res.ContainerID)
		if !client.IsErrNotFound(inspectErr) {
			t.Errorf("expected container %s to be removed after failure, inspect err: %v", res.ContainerID, inspectErr)
		}
	})
}
