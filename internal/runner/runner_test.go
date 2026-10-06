package runner

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

// mockDockerClient implements DockerClient for testing without a Docker daemon.
type mockDockerClient struct {
	createFunc  func(ctx context.Context, config *container.Config, hostConfig *container.HostConfig) (container.CreateResponse, error)
	copyFunc    func(ctx context.Context, containerID, dstPath string, content io.Reader) error
	attachFunc  func(ctx context.Context, container string, options container.AttachOptions) (types.HijackedResponse, error)
	startFunc   func(ctx context.Context, containerID string, options container.StartOptions) error
	waitFunc    func(ctx context.Context, containerID string, condition container.WaitCondition) (<-chan container.WaitResponse, <-chan error)
	killFunc    func(ctx context.Context, containerID, signal string) error
	removeFunc  func(ctx context.Context, containerID string, options container.RemoveOptions) error
	inspectFunc func(ctx context.Context, containerID string) (types.ContainerJSON, error)
	closeFunc   func() error

	removedContainers []string
}

func (m *mockDockerClient) ContainerCreate(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *v1.Platform, containerName string) (container.CreateResponse, error) {
	if m.createFunc != nil {
		return m.createFunc(ctx, config, hostConfig)
	}
	return container.CreateResponse{ID: "mock-id-123"}, nil
}

func (m *mockDockerClient) CopyToContainer(ctx context.Context, containerID, dstPath string, content io.Reader, options types.CopyToContainerOptions) error {
	if m.copyFunc != nil {
		return m.copyFunc(ctx, containerID, dstPath, content)
	}
	return nil
}

func (m *mockDockerClient) ContainerAttach(ctx context.Context, container string, options container.AttachOptions) (types.HijackedResponse, error) {
	if m.attachFunc != nil {
		return m.attachFunc(ctx, container, options)
	}
	return types.HijackedResponse{}, nil
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
		if len(mockCli.removedContainers) != 1 || mockCli.removedContainers[0] != "mock-id-123" {
			t.Errorf("expected container mock-id-123 to be removed on copy error, got %v", mockCli.removedContainers)
		}
	})

	t.Run("Resource defaults applied properly", func(t *testing.T) {
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
