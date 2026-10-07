package runner

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
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

func createStdCopyStream(stdout, stderr []byte) []byte {
	var buf bytes.Buffer
	if len(stdout) > 0 {
		header := make([]byte, 8)
		header[0] = 1 // stdout
		binary.BigEndian.PutUint32(header[4:], uint32(len(stdout)))
		buf.Write(header)
		buf.Write(stdout)
	}
	if len(stderr) > 0 {
		header := make([]byte, 8)
		header[0] = 2 // stderr
		binary.BigEndian.PutUint32(header[4:], uint32(len(stderr)))
		buf.Write(header)
		buf.Write(stderr)
	}
	return buf.Bytes()
}

func TestResultMappingAndOutputTruncationUnit(t *testing.T) {
	t.Run("Status SUCCESS on exit code 0 with metrics parsed", func(t *testing.T) {
		stream := createStdCopyStream(
			[]byte("hello world\n"),
			[]byte("\n__EXECUTION_METRICS__ {\"wall_time_ms\":45,\"peak_memory_kb\":9100,\"exit_code\":0}\n"),
		)
		mockCli := &mockDockerClient{
			attachFunc: func(ctx context.Context, container string, options container.AttachOptions) (types.HijackedResponse, error) {
				c1, _ := net.Pipe()
				return types.HijackedResponse{
					Conn:   c1,
					Reader: bufio.NewReader(bytes.NewReader(stream)),
				}, nil
			},
			waitFunc: func(ctx context.Context, containerID string, condition container.WaitCondition) (<-chan container.WaitResponse, <-chan error) {
				waitCh := make(chan container.WaitResponse, 1)
				waitCh <- container.WaitResponse{StatusCode: 0}
				return waitCh, make(chan error, 1)
			},
		}
		runner := NewDockerRunner(mockCli)
		res, err := runner.Execute(context.Background(), ExecutionRequest{
			Language: LanguagePython,
			Code:     "print('hello world')",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if res.Status != StatusSuccess {
			t.Errorf("expected Status SUCCESS, got %q", res.Status)
		}
		if res.ExitCode != 0 {
			t.Errorf("expected exit code 0, got %d", res.ExitCode)
		}
		if res.Stdout != "hello world\n" {
			t.Errorf("expected clean stdout 'hello world\\n', got %q", res.Stdout)
		}
		if res.Stderr != "" {
			t.Errorf("expected clean stderr without metrics, got %q", res.Stderr)
		}
		if res.StdoutTruncated || res.StderrTruncated {
			t.Errorf("expected truncation flags false")
		}
		if res.WallTimeMs != 45 {
			t.Errorf("expected WallTimeMs 45, got %d", res.WallTimeMs)
		}
		if res.MemoryUsageKb != 9100 || res.PeakMemoryKb != 9100 {
			t.Errorf("expected memory 9100, got usage=%d peak=%d", res.MemoryUsageKb, res.PeakMemoryKb)
		}
	})

	t.Run("Status RUNTIME_ERROR on non-zero exit code", func(t *testing.T) {
		stream := createStdCopyStream(
			nil,
			[]byte("ZeroDivisionError: division by zero\n\n__EXECUTION_METRICS__ {\"wall_time_ms\":15,\"peak_memory_kb\":4096,\"exit_code\":1}\n"),
		)
		mockCli := &mockDockerClient{
			attachFunc: func(ctx context.Context, container string, options container.AttachOptions) (types.HijackedResponse, error) {
				c1, _ := net.Pipe()
				return types.HijackedResponse{
					Conn:   c1,
					Reader: bufio.NewReader(bytes.NewReader(stream)),
				}, nil
			},
			waitFunc: func(ctx context.Context, containerID string, condition container.WaitCondition) (<-chan container.WaitResponse, <-chan error) {
				waitCh := make(chan container.WaitResponse, 1)
				waitCh <- container.WaitResponse{StatusCode: 1}
				return waitCh, make(chan error, 1)
			},
		}
		runner := NewDockerRunner(mockCli)
		res, err := runner.Execute(context.Background(), ExecutionRequest{
			Language: LanguagePython,
			Code:     "1/0",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if res.Status != StatusRuntimeError {
			t.Errorf("expected Status RUNTIME_ERROR, got %q", res.Status)
		}
		if res.ExitCode != 1 {
			t.Errorf("expected exit code 1, got %d", res.ExitCode)
		}
		if !strings.Contains(res.Stderr, "ZeroDivisionError") {
			t.Errorf("expected stderr to contain error diagnostic, got %q", res.Stderr)
		}
		if strings.Contains(res.Stderr, MetricsMarker) {
			t.Errorf("stderr must not contain MetricsMarker")
		}
	})

	t.Run("Status TIME_LIMIT_EXCEEDED on runtime timeout", func(t *testing.T) {
		mockCli := &mockDockerClient{
			waitFunc: func(ctx context.Context, containerID string, condition container.WaitCondition) (<-chan container.WaitResponse, <-chan error) {
				return make(chan container.WaitResponse), make(chan error) // blocks until timeout
			},
		}
		runner := NewDockerRunner(mockCli)
		res, err := runner.Execute(context.Background(), ExecutionRequest{
			Language: LanguagePython,
			Code:     "while True: pass",
			Timeout:  50 * time.Millisecond,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if res.Status != StatusTimeLimitExceeded {
			t.Errorf("expected Status TIME_LIMIT_EXCEEDED, got %q", res.Status)
		}
		if !res.TimedOut {
			t.Errorf("expected TimedOut true")
		}
		if res.ExitCode != 137 {
			t.Errorf("expected ExitCode 137, got %d", res.ExitCode)
		}
	})

	t.Run("Status MEMORY_LIMIT_EXCEEDED when container is OOMKilled", func(t *testing.T) {
		mockCli := &mockDockerClient{
			waitFunc: func(ctx context.Context, containerID string, condition container.WaitCondition) (<-chan container.WaitResponse, <-chan error) {
				waitCh := make(chan container.WaitResponse, 1)
				waitCh <- container.WaitResponse{StatusCode: 137}
				return waitCh, make(chan error, 1)
			},
			inspectFunc: func(ctx context.Context, containerID string) (types.ContainerJSON, error) {
				return types.ContainerJSON{
					ContainerJSONBase: &types.ContainerJSONBase{
						State: &types.ContainerState{
							OOMKilled: true,
							ExitCode:  137,
						},
					},
				}, nil
			},
		}
		runner := NewDockerRunner(mockCli)
		res, err := runner.Execute(context.Background(), ExecutionRequest{
			Language: LanguagePython,
			Code:     "a = 'x' * 1000000000",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if res.Status != StatusMemoryLimitExceeded {
			t.Errorf("expected Status MEMORY_LIMIT_EXCEEDED, got %q", res.Status)
		}
		if res.TimedOut {
			t.Errorf("expected TimedOut false for OOM kill")
		}
		if res.ExitCode != 137 {
			t.Errorf("expected exit code 137, got %d", res.ExitCode)
		}
	})

	t.Run("Status COMPILATION_ERROR on Go syntax error", func(t *testing.T) {
		mockCli := &mockDockerClient{
			waitFunc: func(ctx context.Context, containerID string, condition container.WaitCondition) (<-chan container.WaitResponse, <-chan error) {
				waitCh := make(chan container.WaitResponse, 1)
				waitCh <- container.WaitResponse{StatusCode: 2} // syntax error
				return waitCh, make(chan error, 1)
			},
		}
		runner := NewDockerRunner(mockCli)
		res, err := runner.Execute(context.Background(), ExecutionRequest{
			Language: LanguageGo,
			Code:     "package main\ninvalid syntax",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if res.Status != StatusCompilationError {
			t.Errorf("expected Status COMPILATION_ERROR, got %q", res.Status)
		}
		if !res.IsCompileError {
			t.Errorf("expected IsCompileError true")
		}
	})

	t.Run("Status SYSTEM_ERROR on Docker container creation failure", func(t *testing.T) {
		mockCli := &mockDockerClient{
			createFunc: func(ctx context.Context, config *container.Config, hostConfig *container.HostConfig) (container.CreateResponse, error) {
				return container.CreateResponse{}, errors.New("daemon connection failed")
			},
		}
		runner := NewDockerRunner(mockCli)
		res, err := runner.Execute(context.Background(), ExecutionRequest{
			Language: LanguagePython,
			Code:     "print(1)",
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if res == nil {
			t.Fatal("expected non-nil ExecutionResult with SYSTEM_ERROR status")
		}
		if res.Status != StatusSystemError {
			t.Errorf("expected Status SYSTEM_ERROR, got %q", res.Status)
		}
	})

	t.Run("Stdout truncation at 64 KB with continued consumption", func(t *testing.T) {
		limit := 64 * 1024
		// Create 100 KB stdout
		largeStdout := bytes.Repeat([]byte("A"), 100*1024)
		stream := createStdCopyStream(largeStdout, nil)

		mockCli := &mockDockerClient{
			attachFunc: func(ctx context.Context, container string, options container.AttachOptions) (types.HijackedResponse, error) {
				c1, _ := net.Pipe()
				return types.HijackedResponse{
					Conn:   c1,
					Reader: bufio.NewReader(bytes.NewReader(stream)),
				}, nil
			},
			waitFunc: func(ctx context.Context, containerID string, condition container.WaitCondition) (<-chan container.WaitResponse, <-chan error) {
				waitCh := make(chan container.WaitResponse, 1)
				waitCh <- container.WaitResponse{StatusCode: 0}
				return waitCh, make(chan error, 1)
			},
		}
		runner := NewDockerRunner(mockCli)
		res, err := runner.Execute(context.Background(), ExecutionRequest{
			Language: LanguagePython,
			Code:     "print('A' * 102400)",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !res.StdoutTruncated {
			t.Errorf("expected StdoutTruncated true")
		}
		if res.StderrTruncated {
			t.Errorf("expected StderrTruncated false")
		}
		if len(res.Stdout) != limit {
			t.Errorf("expected Stdout length exactly %d, got %d", limit, len(res.Stdout))
		}
		if res.Status != StatusSuccess {
			t.Errorf("expected Status SUCCESS, got %q", res.Status)
		}
	})

	t.Run("Stderr truncation at 64 KB with continued consumption and metrics extraction", func(t *testing.T) {
		limit := 64 * 1024
		largeStderr := bytes.Repeat([]byte("E"), 80*1024)
		metricsLine := []byte("\n__EXECUTION_METRICS__ {\"wall_time_ms\":88,\"peak_memory_kb\":6500,\"exit_code\":0}\n")
		fullStderr := append(largeStderr, metricsLine...)
		stream := createStdCopyStream(nil, fullStderr)

		mockCli := &mockDockerClient{
			attachFunc: func(ctx context.Context, container string, options container.AttachOptions) (types.HijackedResponse, error) {
				c1, _ := net.Pipe()
				return types.HijackedResponse{
					Conn:   c1,
					Reader: bufio.NewReader(bytes.NewReader(stream)),
				}, nil
			},
			waitFunc: func(ctx context.Context, containerID string, condition container.WaitCondition) (<-chan container.WaitResponse, <-chan error) {
				waitCh := make(chan container.WaitResponse, 1)
				waitCh <- container.WaitResponse{StatusCode: 0}
				return waitCh, make(chan error, 1)
			},
		}
		runner := NewDockerRunner(mockCli)
		res, err := runner.Execute(context.Background(), ExecutionRequest{
			Language: LanguagePython,
			Code:     "import sys; sys.stderr.write('E' * 81920)",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !res.StderrTruncated {
			t.Errorf("expected StderrTruncated true")
		}
		if res.StdoutTruncated {
			t.Errorf("expected StdoutTruncated false")
		}
		if len(res.Stderr) != limit {
			t.Errorf("expected Stderr length exactly %d, got %d", limit, len(res.Stderr))
		}
		if res.WallTimeMs != 88 {
			t.Errorf("expected WallTimeMs 88 even after 80KB stderr, got %d", res.WallTimeMs)
		}
		if res.MemoryUsageKb != 6500 {
			t.Errorf("expected MemoryUsageKb 6500, got %d", res.MemoryUsageKb)
		}
	})
}
