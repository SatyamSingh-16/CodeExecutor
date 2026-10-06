package runner

import (
	"bytes"
	"context"
	"fmt"
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

// Execute runs code in a sandboxed ephemeral container and returns execution results.
func (r *DockerRunner) Execute(ctx context.Context, req ExecutionRequest) (*ExecutionResult, error) {
	if req.Code == "" {
		return nil, ErrEmptyCode
	}

	cfg, err := GetRuntimeConfig(req.Language)
	if err != nil {
		return nil, err
	}

	// Apply default resource limits if not explicitly configured
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
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
		return nil, fmt.Errorf("failed to create container: %w", err)
	}

	containerID := createResp.ID

	// 4. Ensure container cleanup on all return paths (defer-based removal)
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = r.client.ContainerRemove(cleanupCtx, containerID, container.RemoveOptions{
			Force:         true,
			RemoveVolumes: true,
		})
	}()

	// 5. Inject source code via in-memory TAR stream (no host file mount)
	tarReader, err := BuildTarArchive(cfg.SourceFileName, []byte(req.Code))
	if err != nil {
		return nil, fmt.Errorf("failed to build in-memory tar archive: %w", err)
	}

	if err := r.client.CopyToContainer(ctx, containerID, "/tmp", tarReader, types.CopyToContainerOptions{}); err != nil {
		return nil, fmt.Errorf("failed to copy source code to container: %w", err)
	}

	// 6. Attach to streams before starting container to ensure no output is lost
	attachResp, err := r.client.ContainerAttach(ctx, containerID, container.AttachOptions{
		Stream: true,
		Stdin:  true,
		Stdout: true,
		Stderr: true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to attach to container: %w", err)
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

	// Multiplex container stdout and stderr concurrently
	var stdoutBuf, stderrBuf bytes.Buffer
	copyDone := make(chan error, 1)
	go func() {
		_, copyErr := stdcopy.StdCopy(&stdoutBuf, &stderrBuf, attachResp.Reader)
		copyDone <- copyErr
	}()

	// 7. Start Container
	startTime := time.Now()
	if err := r.client.ContainerStart(ctx, containerID, container.StartOptions{}); err != nil {
		return nil, fmt.Errorf("failed to start container: %w", err)
	}

	// 8. Wait for completion with hard deadline enforcement
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
			return nil, fmt.Errorf("error waiting for container: %w", waitErr)
		}

	case waitResp := <-waitCh:
		exitCode = int(waitResp.StatusCode)
	}

	duration := time.Since(startTime)

	// Wait for stdout/stderr demux to finish with a short deadline
	select {
	case <-copyDone:
	case <-time.After(1 * time.Second):
	}

	return &ExecutionResult{
		Stdout:      stdoutBuf.String(),
		Stderr:      stderrBuf.String(),
		ExitCode:    exitCode,
		TimedOut:    timedOut,
		Duration:    duration,
		ContainerID: containerID,
	}, nil
}
