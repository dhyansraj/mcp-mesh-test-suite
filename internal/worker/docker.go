package worker

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"

	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/runner"
)

// DockerHandler wraps DockerExecutor to implement the WorkerHandler interface
type DockerHandler struct {
	serverURL   string
	suitePath   string
	baseWorkdir string
	dockerImage string
	runID       string
	network     string
	results     sync.Map // workerID -> chan *WorkerResult
	executors   sync.Map // workerID -> *runner.DockerExecutor
}

// DefaultDockerNetwork is used when docker.network is unset in config.yaml
const DefaultDockerNetwork = "bridge"

// NewDockerHandler creates a new Docker handler. An empty network falls back to
// DefaultDockerNetwork.
func NewDockerHandler(serverURL, suitePath, baseWorkdir, dockerImage, network, runID string) *DockerHandler {
	if strings.TrimSpace(network) == "" {
		network = DefaultDockerNetwork
	}
	return &DockerHandler{
		serverURL:   serverURL,
		suitePath:   suitePath,
		baseWorkdir: baseWorkdir,
		dockerImage: dockerImage,
		runID:       runID,
		network:     network,
	}
}

func (h *DockerHandler) Name() string { return "docker" }

// containerConfig builds the per-worker container configuration.
func (h *DockerHandler) containerConfig() *runner.ContainerConfig {
	return &runner.ContainerConfig{
		Image:   h.dockerImage,
		Network: h.network,
	}
}

func (h *DockerHandler) StartWorker(ctx context.Context, testID string, runID string, apiURL string) (WorkerInfo, error) {
	// Each worker gets its own docker executor (for isolation)
	dockerExec, err := runner.NewDockerExecutor(h.serverURL, h.suitePath, h.baseWorkdir, h.containerConfig(), h.runID)
	if err != nil {
		return WorkerInfo{}, fmt.Errorf("failed to create Docker executor: %w", err)
	}

	workerID := uuid.New().String()[:8]
	h.executors.Store(workerID, dockerExec)

	// Launch test in goroutine, send result to channel
	resultCh := make(chan *WorkerResult, 1)
	h.results.Store(workerID, resultCh)

	go func() {
		// Execute test (the DockerExecutor handles the full lifecycle)
		result, err := dockerExec.ExecuteTest(ctx, testID, nil)

		wr := &WorkerResult{}
		if err != nil {
			wr.Passed = false
			wr.Error = err.Error()
		} else {
			wr.Passed = result.ExitCode == 0 && result.Error == nil
			wr.Duration = result.Duration
			if result.Error != nil {
				wr.Error = result.Error.Error()
			} else if result.ExitCode != 0 {
				wr.Error = fmt.Sprintf("exit code %d", result.ExitCode)
				if result.Stderr != "" {
					lines := strings.Split(strings.TrimSpace(result.Stderr), "\n")
					if len(lines) > 3 {
						lines = lines[len(lines)-3:]
					}
					wr.Error = strings.Join(lines, "; ")
				}
			}
		}

		resultCh <- wr
	}()

	return WorkerInfo{ID: workerID, TestID: testID}, nil
}

func (h *DockerHandler) WaitForWorker(ctx context.Context, info WorkerInfo) (*WorkerResult, error) {
	val, ok := h.results.Load(info.ID)
	if !ok {
		return nil, fmt.Errorf("worker %s not found", info.ID)
	}
	resultCh := val.(chan *WorkerResult)

	select {
	case result := <-resultCh:
		return result, nil
	case <-ctx.Done():
		if ctx.Err() == context.Canceled {
			return nil, context.Canceled
		}
		return nil, ctx.Err()
	}
}

func (h *DockerHandler) CleanupWorker(ctx context.Context, info WorkerInfo) error {
	if val, ok := h.executors.LoadAndDelete(info.ID); ok {
		exec := val.(*runner.DockerExecutor)
		exec.Close()
	}
	h.results.Delete(info.ID)
	return nil
}

func (h *DockerHandler) Close() error {
	return nil
}

// Ensure DockerHandler satisfies the interface
var _ WorkerHandler = (*DockerHandler)(nil)
