package runner

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

// mockDockerClient implements DockerClient for testing without a Docker daemon.
type mockDockerClient struct {
	createFunc   func(ctx context.Context, config *container.Config, hostConfig *container.HostConfig) (container.CreateResponse, error)
	copyFunc     func(ctx context.Context, containerID, dstPath string, content io.Reader) error
	copyFromFunc func(ctx context.Context, containerID, srcPath string) (io.ReadCloser, types.ContainerPathStat, error)
	attachFunc   func(ctx context.Context, container string, options container.AttachOptions) (types.HijackedResponse, error)
	startFunc    func(ctx context.Context, containerID string, options container.StartOptions) error
	waitFunc     func(ctx context.Context, containerID string, condition container.WaitCondition) (<-chan container.WaitResponse, <-chan error)
	killFunc     func(ctx context.Context, containerID, signal string) error
	removeFunc   func(ctx context.Context, containerID string, options container.RemoveOptions) error
	inspectFunc  func(ctx context.Context, containerID string) (types.ContainerJSON, error)
	closeFunc    func() error

	createdConfigs    []*container.Config
	createdHostConfig []*container.HostConfig
	removedContainers []string
}

func (m *mockDockerClient) ContainerCreate(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *v1.Platform, containerName string) (container.CreateResponse, error) {
	m.createdConfigs = append(m.createdConfigs, config)
	m.createdHostConfig = append(m.createdHostConfig, hostConfig)
	if m.createFunc != nil {
		return m.createFunc(ctx, config, hostConfig)
	}
	id := fmt.Sprintf("mock-id-%d", len(m.createdConfigs))
	return container.CreateResponse{ID: id}, nil
}

func (m *mockDockerClient) CopyToContainer(ctx context.Context, containerID, dstPath string, content io.Reader, options types.CopyToContainerOptions) error {
	if m.copyFunc != nil {
		return m.copyFunc(ctx, containerID, dstPath, content)
	}
	return nil
}

func (m *mockDockerClient) CopyFromContainer(ctx context.Context, containerID, srcPath string) (io.ReadCloser, types.ContainerPathStat, error) {
	if m.copyFromFunc != nil {
		return m.copyFromFunc(ctx, containerID, srcPath)
	}
	return io.NopCloser(bytes.NewReader([]byte("dummy-binary-tar"))), types.ContainerPathStat{}, nil
}

func (m *mockDockerClient) ContainerAttach(ctx context.Context, container string, options container.AttachOptions) (types.HijackedResponse, error) {
	if m.attachFunc != nil {
		return m.attachFunc(ctx, container, options)
	}
	c1, c2 := net.Pipe()
	_ = c2.Close()
	return types.HijackedResponse{
		Conn:   c1,
		Reader: bufio.NewReader(bytes.NewReader(nil)),
	}, nil
}

func (m *mockDockerClient) ContainerStart(ctx context.Context, containerID string, options container.StartOptions) error {
	if m.startFunc != nil {
		return m.startFunc(ctx, containerID, options)
	}
	return nil
}

func (m *mockDockerClient) ContainerWait(ctx context.Context, containerID string, condition container.WaitCondition) (<-chan container.WaitResponse, <-chan error) {
	if m.waitFunc != nil {
		return m.waitFunc(ctx, containerID, condition)
	}
	waitCh := make(chan container.WaitResponse, 1)
	errCh := make(chan error, 1)
	waitCh <- container.WaitResponse{StatusCode: 0}
	return waitCh, errCh
}

func (m *mockDockerClient) ContainerKill(ctx context.Context, containerID, signal string) error {
	if m.killFunc != nil {
		return m.killFunc(ctx, containerID, signal)
	}
	return nil
}

func (m *mockDockerClient) ContainerRemove(ctx context.Context, containerID string, options container.RemoveOptions) error {
	m.removedContainers = append(m.removedContainers, containerID)
	if m.removeFunc != nil {
		return m.removeFunc(ctx, containerID, options)
	}
	return nil
}

func (m *mockDockerClient) ContainerInspect(ctx context.Context, containerID string) (types.ContainerJSON, error) {
	if m.inspectFunc != nil {
		return m.inspectFunc(ctx, containerID)
	}
	return types.ContainerJSON{}, nil
}

func (m *mockDockerClient) Close() error {
	if m.closeFunc != nil {
		return m.closeFunc()
	}
	return nil
}

func TestExecuteValidation(t *testing.T) {
	runner := NewDockerRunner(nil)

	t.Run("Empty code validation", func(t *testing.T) {
		req := ExecutionRequest{
			Language: LanguagePython,
			Code:     "",
		}
		_, err := runner.Execute(context.Background(), req)
		if err != ErrEmptyCode {
			t.Errorf("expected ErrEmptyCode, got %v", err)
		}
	})

	t.Run("Unsupported language validation", func(t *testing.T) {
		req := ExecutionRequest{
			Language: Language("ruby"),
			Code:     "puts 'hello'",
		}
		_, err := runner.Execute(context.Background(), req)
		if err != ErrUnsupportedLanguage {
			t.Errorf("expected ErrUnsupportedLanguage, got %v", err)
		}
	})
}

func TestContainerLifecycleUnit(t *testing.T) {
	t.Run("Container creation failure", func(t *testing.T) {
		mockErr := errors.New("daemon creation error")
		mockCli := &mockDockerClient{
			createFunc: func(ctx context.Context, config *container.Config, hostConfig *container.HostConfig) (container.CreateResponse, error) {
				return container.CreateResponse{}, mockErr
			},
		}

		runner := NewDockerRunner(mockCli)
		_, err := runner.Execute(context.Background(), ExecutionRequest{
			Language: LanguagePython,
			Code:     "print(1)",
		})
		if err == nil || !errors.Is(err, mockErr) {
			t.Fatalf("expected error containing %v, got %v", mockErr, err)
		}
		if len(mockCli.removedContainers) != 0 {
			t.Errorf("did not expect ContainerRemove to be called on create error")
		}
	})

	t.Run("Code copy failure triggers cleanup", func(t *testing.T) {
		mockErr := errors.New("copy failed")
		mockCli := &mockDockerClient{
			copyFunc: func(ctx context.Context, containerID, dstPath string, content io.Reader) error {
				return mockErr
			},
		}

		runner := NewDockerRunner(mockCli)
		_, err := runner.Execute(context.Background(), ExecutionRequest{
			Language: LanguagePython,
			Code:     "print(1)",
		})
		if err == nil {
			t.Fatalf("expected copy error, got nil")
		}
		if len(mockCli.removedContainers) != 1 || mockCli.removedContainers[0] != "mock-id-1" {
			t.Errorf("expected container mock-id-1 to be removed on copy error, got %v", mockCli.removedContainers)
		}
	})

	t.Run("Resource defaults applied properly for Python", func(t *testing.T) {
		var capturedHostCfg *container.HostConfig
		mockCli := &mockDockerClient{
			createFunc: func(ctx context.Context, config *container.Config, hostConfig *container.HostConfig) (container.CreateResponse, error) {
				capturedHostCfg = hostConfig
				return container.CreateResponse{ID: "test-id"}, nil
			},
			copyFunc: func(ctx context.Context, containerID, dstPath string, content io.Reader) error {
				return errors.New("stop execution early")
			},
		}

		runner := NewDockerRunner(mockCli)
		_, _ = runner.Execute(context.Background(), ExecutionRequest{
			Language: LanguagePython,
			Code:     "print(1)",
		})

		if capturedHostCfg == nil {
			t.Fatalf("expected hostConfig to be captured")
		}

		// Defaults: 128 MB RAM, 1 core CPU, 64 PidsLimit, ReadonlyRootfs true, CapDrop ALL
		if capturedHostCfg.Memory != 128*1024*1024 {
			t.Errorf("expected 128MB default, got %d", capturedHostCfg.Memory)
		}
		if capturedHostCfg.MemorySwap != 128*1024*1024 {
			t.Errorf("expected 128MB swap default, got %d", capturedHostCfg.MemorySwap)
		}
		if capturedHostCfg.NanoCPUs != 1_000_000_000 {
			t.Errorf("expected 1 CPU default, got %d", capturedHostCfg.NanoCPUs)
		}
		if *capturedHostCfg.PidsLimit != 64 {
			t.Errorf("expected 64 pids limit default, got %d", *capturedHostCfg.PidsLimit)
		}
		if !capturedHostCfg.ReadonlyRootfs {
			t.Errorf("expected ReadonlyRootfs true")
		}
		if string(capturedHostCfg.NetworkMode) != "none" {
			t.Errorf("expected NetworkMode none")
		}
	})

	t.Run("Runner Close cleans up client", func(t *testing.T) {
		closed := false
		mockCli := &mockDockerClient{
			closeFunc: func() error {
				closed = true
				return nil
			},
		}
		runner := NewDockerRunner(mockCli)
		if err := runner.Close(); err != nil {
			t.Errorf("unexpected error on Close: %v", err)
		}
		if !closed {
			t.Errorf("expected underlying client Close to be called")
		}
	})
}

func TestTwoPhaseGoUnit(t *testing.T) {
	t.Run("Two-phase Go compilation success and runtime execution", func(t *testing.T) {
		mockCli := &mockDockerClient{}
		runner := NewDockerRunner(mockCli)

		res, err := runner.Execute(context.Background(), ExecutionRequest{
			Language: LanguageGo,
			Code:     "package main\nfunc main() {}",
		})
		if err != nil {
			t.Fatalf("unexpected execution error: %v", err)
		}

		if res.IsCompileError {
			t.Errorf("expected IsCompileError false on clean build")
		}

		// Verify 2 containers were created (Phase 1 Compile, Phase 2 Runtime)
		if len(mockCli.createdConfigs) != 2 {
			t.Fatalf("expected 2 containers created, got %d", len(mockCli.createdConfigs))
		}

		// Phase 1 verification
		compileCfg := mockCli.createdConfigs[0]
		compileHost := mockCli.createdHostConfig[0]
		if compileCfg.Cmd[0] != "go" || compileCfg.Cmd[1] != "build" {
			t.Errorf("expected compile command [go build ...], got %v", compileCfg.Cmd)
		}
		if compileHost.Memory != 256*1024*1024 {
			t.Errorf("expected Phase 1 compile memory 256MB, got %d", compileHost.Memory)
		}

		// Phase 2 verification
		runtimeCfg := mockCli.createdConfigs[1]
		runtimeHost := mockCli.createdHostConfig[1]
		if runtimeCfg.Cmd[0] != "/tmp/app" {
			t.Errorf("expected runtime command [/tmp/app], got %v", runtimeCfg.Cmd)
		}
		if runtimeHost.Memory != 128*1024*1024 {
			t.Errorf("expected Phase 2 runtime memory 128MB, got %d", runtimeHost.Memory)
		}

		// Verify both containers removed
		if len(mockCli.removedContainers) != 2 {
			t.Errorf("expected 2 containers removed, got %d: %v", len(mockCli.removedContainers), mockCli.removedContainers)
		}
	})

	t.Run("Two-phase Go compilation failure halts before runtime", func(t *testing.T) {
		mockCli := &mockDockerClient{
			waitFunc: func(ctx context.Context, containerID string, condition container.WaitCondition) (<-chan container.WaitResponse, <-chan error) {
				waitCh := make(chan container.WaitResponse, 1)
				errCh := make(chan error, 1)
				// Compilation fails with exit code 2
				waitCh <- container.WaitResponse{StatusCode: 2}
				return waitCh, errCh
			},
		}
		runner := NewDockerRunner(mockCli)

		res, err := runner.Execute(context.Background(), ExecutionRequest{
			Language: LanguageGo,
			Code:     "package main\ninvalid syntax",
		})
		if err != nil {
			t.Fatalf("unexpected execution error: %v", err)
		}

		if !res.IsCompileError {
			t.Errorf("expected IsCompileError true on compilation failure")
		}
		if res.ExitCode != 2 {
			t.Errorf("expected exit code 2, got %d", res.ExitCode)
		}

		// Verify ONLY 1 container created (Phase 1 only, Phase 2 never created)
		if len(mockCli.createdConfigs) != 1 {
			t.Errorf("expected exactly 1 container created (Phase 1), got %d", len(mockCli.createdConfigs))
		}

		// Verify compilation container cleaned up
		if len(mockCli.removedContainers) != 1 {
			t.Errorf("expected compilation container removed, got %d", len(mockCli.removedContainers))
		}
	})

	t.Run("Two-phase Go compilation timeout halts before runtime", func(t *testing.T) {
		mockCli := &mockDockerClient{
			waitFunc: func(ctx context.Context, containerID string, condition container.WaitCondition) (<-chan container.WaitResponse, <-chan error) {
				// Block indefinitely until timeout occurs
				waitCh := make(chan container.WaitResponse)
				errCh := make(chan error)
				return waitCh, errCh
			},
		}
		runner := NewDockerRunner(mockCli)

		res, err := runner.Execute(context.Background(), ExecutionRequest{
			Language:       LanguageGo,
			Code:           "package main\nfunc main() {}",
			CompileTimeout: 50 * time.Millisecond,
		})
		if err != nil {
			t.Fatalf("unexpected execution error: %v", err)
		}

		if !res.IsCompileError {
			t.Errorf("expected IsCompileError true on compilation timeout")
		}
		if !res.TimedOut {
			t.Errorf("expected TimedOut true")
		}
		if res.ExitCode != 137 {
			t.Errorf("expected ExitCode 137, got %d", res.ExitCode)
		}
		if !strings.Contains(res.CompilationOutput, "compilation timed out") {
			t.Errorf("expected compilation output to mention timeout, got %q", res.CompilationOutput)
		}

		// Phase 2 container was never created
		if len(mockCli.createdConfigs) != 1 {
			t.Errorf("expected only 1 container created, got %d", len(mockCli.createdConfigs))
		}
		// Compilation container was killed and cleaned up
		if len(mockCli.removedContainers) != 1 {
			t.Errorf("expected compilation container removed, got %d", len(mockCli.removedContainers))
		}
	})

	t.Run("Two-phase Go independent limits configuration", func(t *testing.T) {
		mockCli := &mockDockerClient{}
		runner := NewDockerRunner(mockCli)

		_, err := runner.Execute(context.Background(), ExecutionRequest{
			Language:           LanguageGo,
			Code:               "package main\nfunc main() {}",
			MemoryLimit:        64 * 1024 * 1024,  // custom runtime memory (64MB)
			CompileMemoryLimit: 512 * 1024 * 1024, // custom compile memory (512MB)
			Timeout:            1 * time.Second,   // custom runtime timeout
			CompileTimeout:     15 * time.Second,  // custom compile timeout
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(mockCli.createdHostConfig) != 2 {
			t.Fatalf("expected 2 containers created, got %d", len(mockCli.createdHostConfig))
		}

		compileHost := mockCli.createdHostConfig[0]
		runtimeHost := mockCli.createdHostConfig[1]

		if compileHost.Memory != 512*1024*1024 {
			t.Errorf("expected compile memory 512MB, got %d", compileHost.Memory)
		}
		if runtimeHost.Memory != 64*1024*1024 {
			t.Errorf("expected runtime memory 64MB, got %d", runtimeHost.Memory)
		}
	})
}
