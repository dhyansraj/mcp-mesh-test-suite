package worker

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/config"
)

// SSHHandler runs tests on a remote host via SSH
type SSHHandler struct {
	sshHost         string
	runnerDir       string        // remote directory for runner binary (default: /tmp/tsuite)
	suitePath       string        // local suite path
	remoteSuitePath string        // translated suite path for remote host
	apiURL          string        // API URL reachable from remote host
	timeout         time.Duration
	processes       sync.Map      // workerID -> *exec.Cmd (local ssh process)
	staged          bool          // runner binary has been staged on remote
}

// NewSSHHandler creates a new SSH handler for remote test execution
func NewSSHHandler(cfg *config.SuiteConfig, suitePath string, apiURL string, timeout time.Duration) (*SSHHandler, error) {
	if cfg.SSH.Host == "" {
		return nil, fmt.Errorf("ssh.host is required in config.yaml for remote standalone mode")
	}

	runnerDir := cfg.SSH.RunnerDir
	if runnerDir == "" {
		runnerDir = "/tmp/tsuite"
	}

	// Translate suite path for remote host if NFS mount mapping is configured
	remoteSuitePath := suitePath
	if cfg.SSH.LocalPath != "" && cfg.SSH.MountPath != "" {
		remoteSuitePath = strings.Replace(suitePath, cfg.SSH.LocalPath, cfg.SSH.MountPath, 1)
	}

	h := &SSHHandler{
		sshHost:         cfg.SSH.Host,
		runnerDir:       runnerDir,
		suitePath:       suitePath,
		remoteSuitePath: remoteSuitePath,
		apiURL:          apiURL,
		timeout:         timeout,
	}

	// Verify SSH connectivity
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ssh", "-o", "ConnectTimeout=5", "-o", "BatchMode=yes", h.sshHost, "echo ok")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("SSH connectivity check failed for %s: %v (output: %s)", h.sshHost, err, strings.TrimSpace(string(out)))
	}
	if !strings.Contains(string(out), "ok") {
		return nil, fmt.Errorf("SSH connectivity check unexpected output for %s: %s", h.sshHost, strings.TrimSpace(string(out)))
	}

	return h, nil
}

func (h *SSHHandler) Name() string { return "ssh" }

// StageRunner downloads the runner binary onto the remote host
func (h *SSHHandler) StageRunner() error {
	if h.staged {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	remoteCmd := fmt.Sprintf(
		"mkdir -p %s && rm -f %s/tsuite-runner && curl -sf -o %s/tsuite-runner %s/api/runners/tsuite-runner-linux-amd64 && chmod +x %s/tsuite-runner",
		h.runnerDir, h.runnerDir, h.runnerDir, h.apiURL, h.runnerDir,
	)

	cmd := exec.CommandContext(ctx, "ssh", h.sshHost, remoteCmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to stage runner on %s: %v (output: %s)", h.sshHost, err, strings.TrimSpace(string(out)))
	}

	h.staged = true
	return nil
}

func (h *SSHHandler) StartWorker(ctx context.Context, testID string, runID string, apiURL string) (WorkerInfo, error) {
	if err := h.StageRunner(); err != nil {
		return WorkerInfo{}, err
	}

	// Build remote command – use h.apiURL (the remote-reachable address)
	// instead of the apiURL parameter which may be localhost.
	// Skip --log-dir: the Mac-local path doesn't exist on the remote host;
	// the runner will report results back via the API.
	remoteCmd := fmt.Sprintf("%s/tsuite-runner --suite-path %s --test-id %s --api-url %s --run-id %s",
		h.runnerDir, h.remoteSuitePath, testID, h.apiURL, runID,
	)

	cmd := exec.CommandContext(ctx, "ssh", h.sshHost, remoteCmd)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return WorkerInfo{}, fmt.Errorf("failed to start SSH runner: %w", err)
	}

	id := fmt.Sprintf("%d", cmd.Process.Pid)
	h.processes.Store(id, cmd)

	return WorkerInfo{ID: id, TestID: testID, NodeName: h.sshHost}, nil
}

func (h *SSHHandler) WaitForWorker(ctx context.Context, info WorkerInfo) (*WorkerResult, error) {
	val, ok := h.processes.Load(info.ID)
	if !ok {
		return nil, fmt.Errorf("process %s not found", info.ID)
	}
	cmd := val.(*exec.Cmd)
	startTime := time.Now()

	err := cmd.Wait()
	duration := time.Since(startTime)

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

func (h *SSHHandler) CleanupWorker(ctx context.Context, info WorkerInfo) error {
	val, ok := h.processes.LoadAndDelete(info.ID)
	if !ok {
		return nil
	}
	cmd := val.(*exec.Cmd)

	if cmd.Process != nil && cmd.ProcessState == nil {
		pgid, err := syscall.Getpgid(cmd.Process.Pid)
		if err == nil {
			syscall.Kill(-pgid, syscall.SIGKILL)
		}
	}

	return nil
}

func (h *SSHHandler) Close() error {
	return nil
}
