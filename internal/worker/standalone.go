package worker

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// StandaloneHandler runs tests using the external runner binary on the host
type StandaloneHandler struct {
	runnerBinary string
	suitePath    string
	baseWorkdir  string
	timeout      time.Duration
	processes    sync.Map // WorkerInfo.ID -> *exec.Cmd
}

// NewStandaloneHandler creates a new standalone handler
func NewStandaloneHandler(runnerBinary, suitePath, baseWorkdir string, timeout time.Duration) *StandaloneHandler {
	return &StandaloneHandler{
		runnerBinary: runnerBinary,
		suitePath:    suitePath,
		baseWorkdir:  baseWorkdir,
		timeout:      timeout,
	}
}

func (h *StandaloneHandler) Name() string { return "standalone" }

func (h *StandaloneHandler) StartWorker(ctx context.Context, testID string, runID string, apiURL string) (WorkerInfo, error) {
	// Build command arguments
	args := []string{
		"--suite-path", h.suitePath,
		"--test-id", testID,
	}
	if apiURL != "" {
		args = append(args, "--api-url", apiURL)
	}
	if runID != "" {
		args = append(args, "--run-id", runID)

		// Set log directory for unified logging
		parts := strings.SplitN(testID, "/", 2)
		if len(parts) == 2 {
			logDir := filepath.Join(os.Getenv("HOME"), ".tsuite", "runs", runID, parts[0], parts[1])
			os.MkdirAll(logDir, 0755)
			args = append(args, "--log-dir", logDir)
		}
	}
	if h.baseWorkdir != "" {
		testWorkdir := filepath.Join(h.baseWorkdir, strings.ReplaceAll(testID, "/", "_"))
		os.MkdirAll(testWorkdir, 0755)
		args = append(args, "--workdir", testWorkdir)
	}

	cmd := exec.CommandContext(ctx, h.runnerBinary, args...)
	cmd.Env = os.Environ()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return WorkerInfo{}, fmt.Errorf("failed to start runner: %w", err)
	}

	id := fmt.Sprintf("%d", cmd.Process.Pid)
	h.processes.Store(id, cmd)

	return WorkerInfo{ID: id, TestID: testID}, nil
}

func (h *StandaloneHandler) WaitForWorker(ctx context.Context, info WorkerInfo) (*WorkerResult, error) {
	val, ok := h.processes.Load(info.ID)
	if !ok {
		return nil, fmt.Errorf("process %s not found", info.ID)
	}
	cmd := val.(*exec.Cmd)
	startTime := time.Now()

	// Wait for process
	err := cmd.Wait()
	duration := time.Since(startTime)

	// Check if cancelled (parent context)
	if ctx.Err() == context.Canceled {
		return nil, context.Canceled
	}

	if ctx.Err() == context.DeadlineExceeded {
		return &WorkerResult{
			Passed:   false,
			Error:    "test timed out",
			Duration: duration,
		}, nil
	}

	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			// Runner exited with non-zero status (test failed)
			// The runner reports results directly to the API,
			// so we just indicate failure
			return &WorkerResult{
				Passed:   false,
				Error:    "test failed",
				Duration: duration,
			}, nil
		}
		return &WorkerResult{
			Passed:   false,
			Error:    fmt.Sprintf("runner error: %v", err),
			Duration: duration,
		}, nil
	}

	return &WorkerResult{
		Passed:   true,
		Duration: duration,
	}, nil
}

func (h *StandaloneHandler) CleanupWorker(ctx context.Context, info WorkerInfo) error {
	val, ok := h.processes.LoadAndDelete(info.ID)
	if !ok {
		return nil
	}
	cmd := val.(*exec.Cmd)

	// If process is still running, kill the process group
	if cmd.Process != nil && cmd.ProcessState == nil {
		// Kill process group
		pgid, err := syscall.Getpgid(cmd.Process.Pid)
		if err == nil {
			syscall.Kill(-pgid, syscall.SIGKILL)
		}
	}

	return nil
}

func (h *StandaloneHandler) Close() error {
	return nil
}
