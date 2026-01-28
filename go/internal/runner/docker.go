package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	dockercontext "github.com/docker/go-sdk/context"
)

// ContainerConfig holds configuration for a test container
type ContainerConfig struct {
	Image       string
	Network     string
	Workdir     string
	Timeout     time.Duration
	MemoryLimit int64
	CPUQuota    int64
	Mounts      []MountConfig
}

// MountConfig holds a volume mount configuration
type MountConfig struct {
	Type          string // "host" or "volume"
	HostPath      string
	ContainerPath string
	ReadOnly      bool
}

// DefaultContainerConfig returns a default container configuration
func DefaultContainerConfig() ContainerConfig {
	return ContainerConfig{
		Image:       "python:3.11-slim",
		Network:     "bridge",
		Workdir:     "/workspace",
		Timeout:     5 * time.Minute,
		MemoryLimit: 1024 * 1024 * 1024, // 1GB
	}
}

// DockerExecutor runs tests inside Docker containers
type DockerExecutor struct {
	client      *client.Client
	serverURL   string
	suitePath   string
	baseWorkdir string
	config      ContainerConfig
	runID       string
	runnerPath  string // Path to Go runner binary (Linux version for container)
}

// NewDockerExecutor creates a new Docker executor
func NewDockerExecutor(serverURL, suitePath, baseWorkdir string, config *ContainerConfig, runID string) (*DockerExecutor, error) {
	// Get Docker host from context configuration using official Docker SDK
	// This handles Docker Desktop, rootless Docker, DOCKER_HOST/DOCKER_CONTEXT env vars, etc.
	dockerHost, err := dockercontext.CurrentDockerHost()
	if err != nil {
		// Fall back to default (FromEnv behavior)
		dockerHost = ""
	}

	var cli *client.Client
	if dockerHost != "" {
		cli, err = client.NewClientWithOpts(client.WithHost(dockerHost), client.WithAPIVersionNegotiation())
	} else {
		cli, err = client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}

	// Ping Docker to ensure it's available
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = cli.Ping(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Docker: %w", err)
	}

	cfg := DefaultContainerConfig()
	if config != nil {
		if config.Image != "" {
			cfg.Image = config.Image
		}
		if config.Network != "" {
			cfg.Network = config.Network
		}
		if config.Workdir != "" {
			cfg.Workdir = config.Workdir
		}
		if config.Timeout > 0 {
			cfg.Timeout = config.Timeout
		}
		if config.MemoryLimit > 0 {
			cfg.MemoryLimit = config.MemoryLimit
		}
		cfg.Mounts = config.Mounts
	}

	// Find the Go runner binary for Linux (container architecture)
	runnerPath, err := findRunnerBinaryForDocker()
	if err != nil {
		cli.Close()
		return nil, fmt.Errorf("failed to find runner binary for docker: %w", err)
	}

	return &DockerExecutor{
		client:      cli,
		serverURL:   serverURL,
		suitePath:   suitePath,
		baseWorkdir: baseWorkdir,
		config:        cfg,
		runID:         runID,
		runnerPath:    runnerPath,
	}, nil
}

// findRunnerBinaryForDocker finds the Go runner binary for Linux containers
func findRunnerBinaryForDocker() (string, error) {
	// Get the directory of the current executable
	execPath, err := os.Executable()
	if err != nil {
		return "", err
	}
	execDir := filepath.Dir(execPath)

	// Look for tsuite-runner-linux (built with GOOS=linux, uses host architecture)
	runnerPath := filepath.Join(execDir, "tsuite-runner-linux")
	if _, err := os.Stat(runnerPath); err == nil {
		return runnerPath, nil
	}

	return "", fmt.Errorf("runner binary not found. Run 'make build-runner-linux' to build it. Expected: %s", runnerPath)
}

// ContainerResult holds the result of running a container
type ContainerResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
	Error    error
	Duration time.Duration
}

// ExecuteTest runs a test inside a Docker container
func (e *DockerExecutor) ExecuteTest(ctx context.Context, testID string, testConfig map[string]any) (*ContainerResult, error) {
	startTime := time.Now()

	// Create workspace for this test
	testWorkdir := filepath.Join(e.baseWorkdir, strings.ReplaceAll(testID, "/", "_"))
	if err := os.MkdirAll(testWorkdir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create test workdir: %w", err)
	}

	// Get container config from test or use defaults
	containerConfigMap, _ := testConfig["container"].(map[string]any)
	imageName := e.config.Image
	if img, ok := containerConfigMap["image"].(string); ok {
		imageName = img
	}

	timeout := e.config.Timeout
	if t, ok := testConfig["timeout"].(int); ok {
		timeout = time.Duration(t) * time.Second
	}

	// Prepare environment variables
	// Convert localhost to host.docker.internal for container access to host
	containerAPIURL := strings.Replace(e.serverURL, "localhost", "host.docker.internal", 1)
	containerAPIURL = strings.Replace(containerAPIURL, "127.0.0.1", "host.docker.internal", 1)
	env := []string{
		fmt.Sprintf("TSUITE_API=%s", containerAPIURL),
		fmt.Sprintf("TSUITE_TEST_ID=%s", testID),
	}
	if e.runID != "" {
		env = append(env, fmt.Sprintf("TSUITE_RUN_ID=%s", e.runID))
	}

	// Add env from test config
	if envMap, ok := containerConfigMap["env"].(map[string]any); ok {
		for k, v := range envMap {
			value := fmt.Sprintf("%v", v)
			// Resolve ${env:VAR} references
			if strings.HasPrefix(value, "${env:") && strings.HasSuffix(value, "}") {
				envVar := value[6 : len(value)-1]
				value = os.Getenv(envVar)
			}
			env = append(env, fmt.Sprintf("%s=%s", k, value))
		}
	}

	// Prepare volume mounts
	mounts := []mount.Mount{
		{
			Type:     mount.TypeBind,
			Source:   e.runnerPath,
			Target:   "/usr/local/bin/tsuite-runner",
			ReadOnly: true,
		},
		{
			Type:     mount.TypeBind,
			Source:   e.suitePath,
			Target:   "/tests",
			ReadOnly: true,
		},
		{
			Type:     mount.TypeBind,
			Source:   testWorkdir,
			Target:   "/workspace",
			ReadOnly: false,
		},
	}

	// Auto-mount TC-local artifacts directory if it exists
	// Mount each item inside artifacts separately, resolving symlinks
	artifactsPath := filepath.Join(e.suitePath, "suites", testID, "artifacts")
	mounts = append(mounts, mountArtifactsDir(artifactsPath, "/artifacts")...)

	// Auto-mount UC-level artifacts directory if it exists
	// Mount each item inside artifacts separately, resolving symlinks
	parts := strings.Split(testID, "/")
	if len(parts) >= 1 {
		ucArtifactsPath := filepath.Join(e.suitePath, "suites", parts[0], "artifacts")
		mounts = append(mounts, mountArtifactsDir(ucArtifactsPath, "/uc-artifacts")...)
	}

	// Create and mount logs directory for unified logging
	// Structure: ~/.tsuite/runs/{run_id}/{uc}/{tc}/
	//   - worker.log: runner execution trace
	//   - logs/: mcp-mesh agent logs
	if e.runID != "" && len(parts) >= 2 {
		testLogDir := filepath.Join(os.Getenv("HOME"), ".tsuite", "runs", e.runID, parts[0], parts[1])
		logsPath := filepath.Join(testLogDir, "logs")
		if err := os.MkdirAll(logsPath, 0755); err == nil {
			// Mount parent directory for worker.log
			mounts = append(mounts, mount.Mount{
				Type:     mount.TypeBind,
				Source:   testLogDir,
				Target:   "/var/log/tsuite",
				ReadOnly: false,
			})
			// Mount logs subdir for mcp-mesh agent logs
			mounts = append(mounts, mount.Mount{
				Type:     mount.TypeBind,
				Source:   logsPath,
				Target:   "/root/.mcp-mesh/logs",
				ReadOnly: false,
			})
			// Set env var for runner to know where to write worker.log
			env = append(env, "TSUITE_LOG_DIR=/var/log/tsuite")
		}
	}

	// Add suite-level mounts from config
	for _, m := range e.config.Mounts {
		source := m.HostPath
		if !filepath.IsAbs(source) {
			source = filepath.Join(e.suitePath, source)
		}
		mounts = append(mounts, mount.Mount{
			Type:     mount.TypeBind,
			Source:   source,
			Target:   m.ContainerPath,
			ReadOnly: m.ReadOnly,
		})
	}

	// Build the command to run inside container
	command := e.buildTestCommand(testID)

	// Pull image if needed
	if err := e.ensureImage(ctx, imageName); err != nil {
		return nil, fmt.Errorf("failed to pull image: %w", err)
	}

	// Create container
	containerConfig := &container.Config{
		Image:      imageName,
		Cmd:        command,
		Env:        env,
		WorkingDir: "/workspace",
	}

	hostConfig := &container.HostConfig{
		Mounts:      mounts,
		NetworkMode: container.NetworkMode(e.config.Network),
		Resources: container.Resources{
			Memory: e.config.MemoryLimit,
		},
		ExtraHosts: []string{"host.docker.internal:host-gateway"},
	}

	resp, err := e.client.ContainerCreate(ctx, containerConfig, hostConfig, nil, nil, "")
	if err != nil {
		return nil, fmt.Errorf("failed to create container: %w", err)
	}

	containerID := resp.ID
	defer func() {
		// Always remove container
		removeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		e.client.ContainerRemove(removeCtx, containerID, container.RemoveOptions{Force: true})
	}()

	// Start container
	if err := e.client.ContainerStart(ctx, containerID, container.StartOptions{}); err != nil {
		return nil, fmt.Errorf("failed to start container: %w", err)
	}

	// Wait for container with timeout
	waitCtx, waitCancel := context.WithTimeout(ctx, timeout)
	defer waitCancel()

	statusCh, errCh := e.client.ContainerWait(waitCtx, containerID, container.WaitConditionNotRunning)

	var exitCode int
	select {
	case err := <-errCh:
		if err != nil {
			// Timeout or other error - kill container
			killCtx, killCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer killCancel()
			e.client.ContainerKill(killCtx, containerID, "SIGKILL")
			return &ContainerResult{
				ExitCode: 124,
				Error:    fmt.Errorf("container execution failed: %w", err),
				Duration: time.Since(startTime),
			}, nil
		}
	case status := <-statusCh:
		exitCode = int(status.StatusCode)
	}

	// Get logs
	logsReader, err := e.client.ContainerLogs(ctx, containerID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
	})
	if err != nil {
		return &ContainerResult{
			ExitCode: exitCode,
			Error:    fmt.Errorf("failed to get container logs: %w", err),
			Duration: time.Since(startTime),
		}, nil
	}
	defer logsReader.Close()

	var stdout, stderr strings.Builder
	_, _ = stdcopy.StdCopy(&stdout, &stderr, logsReader)

	return &ContainerResult{
		ExitCode: exitCode,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Duration: time.Since(startTime),
	}, nil
}

// ensureImage checks if an image exists locally.
// All images must be pre-built by running src-tests or lib-tests first.
// We never pull from Docker Hub - images are always local.
func (e *DockerExecutor) ensureImage(ctx context.Context, imageName string) error {
	_, _, err := e.client.ImageInspectWithRaw(ctx, imageName)
	if err == nil {
		return nil // Image exists
	}

	// Check if it's a "not found" error vs connection/other error
	if client.IsErrNotFound(err) {
		return fmt.Errorf("image %q not found locally. Run src-tests or lib-tests first to build it", imageName)
	}

	// For other errors (connection refused, timeout, etc.), include the original error
	return fmt.Errorf("failed to check image %q: %w", imageName, err)
}

// buildTestCommand creates the command to run inside the container
func (e *DockerExecutor) buildTestCommand(testID string) []string {
	// Run the Go runner binary (mounted at /usr/local/bin/tsuite-runner)
	// Environment variables TSUITE_API, TSUITE_RUN_ID, TSUITE_TEST_ID, TSUITE_LOG_DIR are already set
	script := fmt.Sprintf(`
set -e

# Install jq if not present (for jq-based assertions)
if ! command -v jq &> /dev/null; then
    apt-get update -qq && apt-get install -y -qq jq 2>/dev/null || true
fi

# Run the Go test runner (uses TSUITE_LOG_DIR env var for logging)
/usr/local/bin/tsuite-runner \
    --test-yaml /tests/suites/%s/test.yaml \
    --suite-path /tests
`, testID)

	return []string{"bash", "-c", script}
}

// Close closes the Docker client
func (e *DockerExecutor) Close() error {
	return e.client.Close()
}

// CheckDockerAvailable checks if Docker is available and running
func CheckDockerAvailable() (bool, string) {
	// Get Docker host from context configuration using official Docker SDK
	// This handles Docker Desktop, rootless Docker, DOCKER_HOST/DOCKER_CONTEXT env vars, etc.
	dockerHost, err := dockercontext.CurrentDockerHost()
	if err != nil {
		// Fall back to default (FromEnv behavior)
		dockerHost = ""
	}

	var cli *client.Client
	if dockerHost != "" {
		cli, err = client.NewClientWithOpts(client.WithHost(dockerHost), client.WithAPIVersionNegotiation())
	} else {
		cli, err = client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	}
	if err != nil {
		return false, err.Error()
	}
	defer cli.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ping, err := cli.Ping(ctx)
	if err != nil {
		return false, err.Error()
	}

	version, err := cli.ServerVersion(ctx)
	if err != nil {
		return true, fmt.Sprintf("Docker available (API %s)", ping.APIVersion)
	}

	return true, fmt.Sprintf("Docker %s (API %s)", version.Version, ping.APIVersion)
}

// mountArtifactsDir mounts each item inside an artifacts directory separately.
// This ensures symlinks inside the artifacts directory are resolved to their
// actual targets, which is necessary for Docker bind mounts.
func mountArtifactsDir(artifactsPath string, containerBasePath string) []mount.Mount {
	var mounts []mount.Mount

	// Check if artifacts directory exists
	info, err := os.Stat(artifactsPath)
	if err != nil || !info.IsDir() {
		return mounts
	}

	// Read directory entries
	entries, err := os.ReadDir(artifactsPath)
	if err != nil {
		return mounts
	}

	// Mount each item separately, resolving symlinks
	for _, entry := range entries {
		itemPath := filepath.Join(artifactsPath, entry.Name())

		// Resolve symlinks to get the actual path
		resolved, err := filepath.EvalSymlinks(itemPath)
		if err != nil {
			continue
		}

		// Verify the resolved path exists
		if _, err := os.Stat(resolved); err != nil {
			continue
		}

		// Mount both files and directories
		mounts = append(mounts, mount.Mount{
			Type:     mount.TypeBind,
			Source:   resolved,
			Target:   filepath.Join(containerBasePath, entry.Name()),
			ReadOnly: true,
		})
	}

	return mounts
}
