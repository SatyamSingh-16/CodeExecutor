package runner

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

// DockerRunner manages the execution of code within isolated ephemeral Docker containers.
type DockerRunner struct {
	client DockerClient
}

// NewDockerRunner creates an execution runner with a given DockerClient.
func NewDockerRunner(cli DockerClient) *DockerRunner {
	return &DockerRunner{client: cli}
}

// NewDefaultDockerRunner creates a runner connected to the local Docker daemon via environment options.
func NewDefaultDockerRunner() (*DockerRunner, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}
	return NewDockerRunner(cli), nil
}

// Close closes the underlying Docker client connection.
func (r *DockerRunner) Close() error {
	if r.client != nil {
		return r.client.Close()
	}
	return nil
}

// removeContainer cleans up a container and its volumes forcefully within a dedicated timeout context.
func removeContainer(cli DockerClient, containerID string) {
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = cli.ContainerRemove(cleanupCtx, containerID, container.RemoveOptions{
		Force:         true,
		RemoveVolumes: true,
	})
}

// Execute runs code in sandboxed ephemeral containers, applying two-phase compilation/execution
// for compiled languages (Go) or single-phase execution for interpreted languages (Python).
func (r *DockerRunner) Execute(ctx context.Context, req ExecutionRequest) (*ExecutionResult, error) {
	if req.Code == "" {
		return nil, ErrEmptyCode
	}

	cfg, err := GetRuntimeConfig(req.Language)
	if err != nil {
		return nil, err
	}

	// Two-phase workflow for compiled languages (Phase 1 Compilation -> Phase 2 Runtime)
	if cfg.IsCompiled {
		binaryTar, compileRes, err := r.compile(ctx, req, cfg)
		if err != nil {
			if compileRes != nil {
				return compileRes, err
			}
			return &ExecutionResult{Status: StatusSystemError, Stderr: err.Error()}, err
		}
		// If compilation failed (syntax error, compile timeout, etc.), return compile result directly
		if compileRes != nil && compileRes.IsCompileError {
			return compileRes, nil
		}

		var compileDuration time.Duration
		if compileRes != nil {
			compileDuration = compileRes.CompileDuration
		}
		return r.executeRuntime(ctx, req, cfg, binaryTar, compileDuration)
	}

	// Single-phase workflow for interpreted languages
	res, err := r.executeRuntime(ctx, req, cfg, nil, 0)
	if err != nil && res == nil {
		return &ExecutionResult{Status: StatusSystemError, Stderr: err.Error()}, err
	}
	return res, err
}

// compile executes Phase 1 of the two-phase lifecycle inside an ephemeral compilation container.
func (r *DockerRunner) compile(ctx context.Context, req ExecutionRequest, cfg RuntimeConfig) ([]byte, *ExecutionResult, error) {
	compileTimeout := req.CompileTimeout
	if compileTimeout <= 0 {
		compileTimeout = 10 * time.Second
	}

	compileMem := req.CompileMemoryLimit
	if compileMem <= 0 {
		compileMem = 256 * 1024 * 1024 // 256 MB
	}

	cpuLimit := req.CPULimit
	if cpuLimit <= 0 {
		cpuLimit = 1_000_000_000 // 1 core (NanoCPUs)
	}

	pidsLimit := req.PidsLimit
	if pidsLimit <= 0 {
		pidsLimit = 64
	}

	containerConfig := &container.Config{
		Image:        cfg.Image,
		Cmd:          cfg.CompileCommand,
		WorkingDir:   "/tmp",
		User:         "1000:1000",
		AttachStdout: true,
		AttachStderr: true,
		Volumes: map[string]struct{}{
			"/tmp": {},
		},
	}

	hostConfig := &container.HostConfig{
		NetworkMode: "none",
		Resources: container.Resources{
			Memory:     compileMem,
			MemorySwap: compileMem,
			NanoCPUs:   cpuLimit,
			PidsLimit:  &pidsLimit,
		},
		ReadonlyRootfs: true,
		CapDrop:        []string{"ALL"},
	}

	createResp, err := r.client.ContainerCreate(ctx, containerConfig, hostConfig, nil, nil, "")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create compilation container: %w", err)
	}

	containerID := createResp.ID
	defer removeContainer(r.client, containerID)

	// Inject source code into compilation container via in-memory TAR
	tarReader, err := BuildTarArchive(cfg.SourceFileName, []byte(req.Code))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to build compilation in-memory tar archive: %w", err)
	}

	if err := r.client.CopyToContainer(ctx, containerID, "/tmp", tarReader, types.CopyToContainerOptions{}); err != nil {
		return nil, nil, fmt.Errorf("failed to copy source code to compilation container: %w", err)
	}

	// Attach to streams to capture compiler diagnostics
	attachResp, err := r.client.ContainerAttach(ctx, containerID, container.AttachOptions{
		Stream: true,
		Stdout: true,
		Stderr: true,
	})
	if err != nil {
		return nil, &ExecutionResult{Status: StatusSystemError, Stderr: err.Error()}, fmt.Errorf("failed to attach to compilation container: %w", err)
	}
	defer attachResp.Close()

	stdoutLimited := NewLimitedBuffer(DefaultMaxOutputBytes)
	stderrLimited := NewLimitedBuffer(DefaultMaxOutputBytes)
	copyDone := make(chan error, 1)
	go func() {
		_, copyErr := stdcopy.StdCopy(stdoutLimited, stderrLimited, attachResp.Reader)
		copyDone <- copyErr
	}()

	startTime := time.Now()
	if err := r.client.ContainerStart(ctx, containerID, container.StartOptions{}); err != nil {
		return nil, &ExecutionResult{Status: StatusSystemError, Stderr: err.Error()}, fmt.Errorf("failed to start compilation container: %w", err)
	}

	waitCh, errCh := r.client.ContainerWait(ctx, containerID, container.WaitConditionNotRunning)

	timeoutTimer := time.NewTimer(compileTimeout)
	defer timeoutTimer.Stop()

	var exitCode int
	var timedOut bool

	select {
	case <-timeoutTimer.C:
		timedOut = true
		killCtx, killCancel := context.WithTimeout(context.Background(), 3*time.Second)
		_ = r.client.ContainerKill(killCtx, containerID, "SIGKILL")
		killCancel()
		exitCode = 137

	case <-ctx.Done():
		timedOut = true
		killCtx, killCancel := context.WithTimeout(context.Background(), 3*time.Second)
		_ = r.client.ContainerKill(killCtx, containerID, "SIGKILL")
		killCancel()
		exitCode = 137

	case waitErr := <-errCh:
		if waitErr != nil {
			return nil, &ExecutionResult{Status: StatusSystemError, Stderr: waitErr.Error()}, fmt.Errorf("error waiting for compilation container: %w", waitErr)
		}

	case waitResp := <-waitCh:
		exitCode = int(waitResp.StatusCode)
	}

	duration := time.Since(startTime)

	select {
	case <-copyDone:
	case <-time.After(1 * time.Second):
	}

	stdoutStr := stdoutLimited.String()
	stderrStr := stderrLimited.String()
	compilerOutput := strings.TrimSpace(stderrStr + "\n" + stdoutStr)

	if timedOut {
		msg := fmt.Sprintf("compilation timed out after %v", compileTimeout)
		if compilerOutput != "" {
			msg = compilerOutput + "\n" + msg
		}
		return nil, &ExecutionResult{
			Status:            StatusCompilationError,
			Stdout:            stdoutStr,
			Stderr:            stderrStr,
			StdoutTruncated:   stdoutLimited.Truncated(),
			StderrTruncated:   stderrLimited.Truncated(),
			ExitCode:          exitCode,
			TimedOut:          true,
			Duration:          duration,
			ContainerID:       containerID,
			IsCompileError:    true,
			CompilationOutput: msg,
			CompileDuration:   duration,
			WallTimeMs:        duration.Milliseconds(),
		}, nil
	}

	if exitCode != 0 {
		return nil, &ExecutionResult{
			Status:            StatusCompilationError,
			Stdout:            stdoutStr,
			Stderr:            stderrStr,
			StdoutTruncated:   stdoutLimited.Truncated(),
			StderrTruncated:   stderrLimited.Truncated(),
			ExitCode:          exitCode,
			TimedOut:          false,
			Duration:          duration,
			ContainerID:       containerID,
			IsCompileError:    true,
			CompilationOutput: compilerOutput,
			CompileDuration:   duration,
			WallTimeMs:        duration.Milliseconds(),
		}, nil
	}

	// Compilation succeeded: extract compiled binary from compilation container
	reader, _, err := r.client.CopyFromContainer(ctx, containerID, cfg.BinaryPath)
	if err != nil {
		return nil, &ExecutionResult{Status: StatusSystemError, Stderr: err.Error()}, fmt.Errorf("failed to extract compiled binary from container: %w", err)
	}
	defer reader.Close()

	binaryTar, err := io.ReadAll(reader)
	if err != nil {
		return nil, &ExecutionResult{Status: StatusSystemError, Stderr: err.Error()}, fmt.Errorf("failed to read binary tar stream: %w", err)
	}

	return binaryTar, &ExecutionResult{
		Status:          StatusSuccess,
		CompileDuration: duration,
	}, nil
}

// executeRuntime executes user code (or compiled binary) inside a fresh, isolated runtime container.
func (r *DockerRunner) executeRuntime(ctx context.Context, req ExecutionRequest, cfg RuntimeConfig, binaryTar []byte, compileDuration time.Duration) (*ExecutionResult, error) {
	timeout := req.Timeout
	if timeout <= 0 {
		if cfg.IsCompiled {
			timeout = 2 * time.Second // 2s hard runtime timeout for Go
		} else {
			timeout = 5 * time.Second // 5s timeout for Python
		}
	}

	memoryLimit := req.MemoryLimit
	if memoryLimit <= 0 {
		memoryLimit = 128 * 1024 * 1024 // 128 MB
	}

	cpuLimit := req.CPULimit
	if cpuLimit <= 0 {
		cpuLimit = 1_000_000_000 // 1 core (NanoCPUs)
	}

	pidsLimit := req.PidsLimit
	if pidsLimit <= 0 {
		pidsLimit = 64
	}

	cmd := cfg.DefaultCommand
	if len(req.CustomCommand) > 0 {
		cmd = req.CustomCommand
	}

	// 1. Build Container Configuration
	containerConfig := &container.Config{
		Image:        cfg.Image,
		Cmd:          cmd,
		WorkingDir:   "/tmp",
		User:         "1000:1000",
		Tty:          false,
		OpenStdin:    true,
		StdinOnce:    true,
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Volumes: map[string]struct{}{
			"/tmp": {},
		},
	}

	// 2. Build Host Configuration with strict sandbox boundaries
	hostConfig := &container.HostConfig{
		NetworkMode: "none",
		Resources: container.Resources{
			Memory:     memoryLimit,
			MemorySwap: memoryLimit, // swap disabled (equal to memory)
			NanoCPUs:   cpuLimit,
			PidsLimit:  &pidsLimit,
		},
		ReadonlyRootfs: true,
		CapDrop:        []string{"ALL"},
	}

	// 3. Create Container
	createResp, err := r.client.ContainerCreate(ctx, containerConfig, hostConfig, nil, nil, "")
	if err != nil {
		return &ExecutionResult{Status: StatusSystemError, Stderr: err.Error()}, fmt.Errorf("failed to create runtime container: %w", err)
	}

	containerID := createResp.ID
	defer removeContainer(r.client, containerID)

	// 4. Inject payload into runtime container via in-memory TAR stream
	if binaryTar != nil {
		// Compiled binary TAR extracted from Phase 1
		if err := r.client.CopyToContainer(ctx, containerID, "/tmp", bytes.NewReader(binaryTar), types.CopyToContainerOptions{}); err != nil {
			return &ExecutionResult{Status: StatusSystemError, Stderr: err.Error()}, fmt.Errorf("failed to copy compiled binary to runtime container: %w", err)
		}
	} else {
		// Interpreted source code
		tarReader, err := BuildTarArchive(cfg.SourceFileName, []byte(req.Code))
		if err != nil {
			return &ExecutionResult{Status: StatusSystemError, Stderr: err.Error()}, fmt.Errorf("failed to build in-memory tar archive: %w", err)
		}
		if err := r.client.CopyToContainer(ctx, containerID, "/tmp", tarReader, types.CopyToContainerOptions{}); err != nil {
			return &ExecutionResult{Status: StatusSystemError, Stderr: err.Error()}, fmt.Errorf("failed to copy source code to runtime container: %w", err)
		}
	}

	// 5. Attach to streams before starting container
	attachResp, err := r.client.ContainerAttach(ctx, containerID, container.AttachOptions{
		Stream: true,
		Stdin:  true,
		Stdout: true,
		Stderr: true,
	})
	if err != nil {
		return &ExecutionResult{Status: StatusSystemError, Stderr: err.Error()}, fmt.Errorf("failed to attach to runtime container: %w", err)
	}
	defer attachResp.Close()

	// Write stdin and close write end so process receives EOF
	if req.Stdin != "" {
		go func() {
			defer attachResp.CloseWrite()
			_, _ = attachResp.Conn.Write([]byte(req.Stdin))
		}()
	} else {
		_ = attachResp.CloseWrite()
	}

	// Multiplex container stdout and stderr concurrently with bounded buffers and metrics filtering
	stdoutLimited := NewLimitedBuffer(DefaultMaxOutputBytes)
	stderrLimited := NewLimitedBuffer(DefaultMaxOutputBytes)
	stdoutFilter := NewMetricsFilterWriter(stdoutLimited)
	stderrFilter := NewMetricsFilterWriter(stderrLimited)

	copyDone := make(chan error, 1)
	go func() {
		_, copyErr := stdcopy.StdCopy(stdoutFilter, stderrFilter, attachResp.Reader)
		copyDone <- copyErr
	}()

	// 6. Start Container
	startTime := time.Now()
	if err := r.client.ContainerStart(ctx, containerID, container.StartOptions{}); err != nil {
		return &ExecutionResult{Status: StatusSystemError, Stderr: err.Error()}, fmt.Errorf("failed to start runtime container: %w", err)
	}

	// 7. Wait for completion with hard deadline enforcement
	waitCh, errCh := r.client.ContainerWait(ctx, containerID, container.WaitConditionNotRunning)

	timeoutTimer := time.NewTimer(timeout)
	defer timeoutTimer.Stop()

	var exitCode int
	var timedOut bool

	select {
	case <-timeoutTimer.C:
		timedOut = true
		killCtx, killCancel := context.WithTimeout(context.Background(), 3*time.Second)
		_ = r.client.ContainerKill(killCtx, containerID, "SIGKILL")
		killCancel()
		exitCode = 137

	case <-ctx.Done():
		timedOut = true
		killCtx, killCancel := context.WithTimeout(context.Background(), 3*time.Second)
		_ = r.client.ContainerKill(killCtx, containerID, "SIGKILL")
		killCancel()
		exitCode = 137

	case waitErr := <-errCh:
		if waitErr != nil {
			return &ExecutionResult{Status: StatusSystemError, Stderr: waitErr.Error()}, fmt.Errorf("error waiting for runtime container: %w", waitErr)
		}

	case waitResp := <-waitCh:
		exitCode = int(waitResp.StatusCode)
	}

	// Inspect container for OOM killer termination before cleanup removes it
	var oomKilled bool
	inspectCtx, inspectCancel := context.WithTimeout(context.Background(), 2*time.Second)
	inspect, inspectErr := r.client.ContainerInspect(inspectCtx, containerID)
	inspectCancel()
	if inspectErr == nil && inspect.ContainerJSONBase != nil && inspect.State != nil {
		oomKilled = inspect.State.OOMKilled
	}

	duration := time.Since(startTime)

	// Wait for stdout/stderr demux to finish
	select {
	case <-copyDone:
	case <-time.After(1 * time.Second):
	}

	stdoutFilter.Flush()
	stderrFilter.Flush()

	var metrics *ExecutionMetrics
	if m, found := stderrFilter.Metrics(); found {
		metrics = m
	} else if m, found := stdoutFilter.Metrics(); found {
		metrics = m
	}

	stdoutStr := stdoutLimited.String()
	stderrStr := stderrLimited.String()

	// Strip any remaining internal metrics marker from user-visible strings
	if m, cleanStderr, found := ExtractMetrics(stderrStr); found {
		stderrStr = cleanStderr
		if metrics == nil {
			metrics = m
		}
	}
	if m, cleanStdout, found := ExtractMetrics(stdoutStr); found {
		stdoutStr = cleanStdout
		if metrics == nil {
			metrics = m
		}
	}

	// If metrics were extracted, ensure any stray delimiter newlines are not exposed as user output
	if metrics != nil && strings.TrimSpace(stderrStr) == "" {
		stderrStr = ""
	}
	if metrics != nil && strings.TrimSpace(stdoutStr) == "" {
		stdoutStr = ""
	}

	var status ExecutionStatus
	if timedOut {
		status = StatusTimeLimitExceeded
	} else if oomKilled {
		status = StatusMemoryLimitExceeded
	} else if exitCode != 0 {
		status = StatusRuntimeError
	} else {
		status = StatusSuccess
	}

	var wallTimeMs int64
	var memUsageKb int64
	if metrics != nil {
		wallTimeMs = metrics.WallTimeMs
		memUsageKb = metrics.PeakMemoryKb
		if exitCode == 0 && metrics.ExitCode != 0 {
			exitCode = metrics.ExitCode
		}
	} else {
		wallTimeMs = duration.Milliseconds()
	}

	return &ExecutionResult{
		Status:          status,
		Stdout:          stdoutStr,
		Stderr:          stderrStr,
		StdoutTruncated: stdoutLimited.Truncated(),
		StderrTruncated: stderrLimited.Truncated(),
		ExitCode:        exitCode,
		TimedOut:        timedOut,
		Duration:        duration,
		ContainerID:     containerID,
		IsCompileError:  false,
		CompileDuration: compileDuration,
		WallTimeMs:      wallTimeMs,
		MemoryUsageKb:   memUsageKb,
		PeakMemoryKb:    memUsageKb,
	}, nil
}
