package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/api"
	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/client"
	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/config"
	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/db"
	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/executor"
	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/man"
	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/runner"
	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/scaffold"
)

var (
	// version is set at build time via ldflags: -ldflags "-X main.version=X.Y.Z"
	version = "dev"
)

// Run command flags
var (
	suitePath  string
	parallel   int
	ucFilter   []string
	tcFilter   []string
	tagFilter  []string
	dryRun     bool
	apiURL     string
	runnerPath string
)

// findRunnerBinary finds the tsuite-runner binary
// It looks for the runner binary in the following locations:
// 1. Explicit path via --runner-path flag
// 2. Same directory as the current executable
// 3. Current working directory
// Returns the path to the runner binary, or empty string if not found
func findRunnerBinary() string {
	if runnerPath != "" {
		if _, err := os.Stat(runnerPath); err == nil {
			return runnerPath
		}
	}

	// Get current executable's directory
	execPath, err := os.Executable()
	if err == nil {
		execPath, _ = filepath.EvalSymlinks(execPath)
		execDir := filepath.Dir(execPath)

		// Look for tsuite-runner in the same directory
		candidates := []string{
			filepath.Join(execDir, "tsuite-runner"),
			filepath.Join(execDir, fmt.Sprintf("tsuite-runner-%s-%s", runtime.GOOS, runtime.GOARCH)),
		}

		for _, path := range candidates {
			if _, err := os.Stat(path); err == nil {
				return path
			}
		}
	}

	// Look in current working directory
	cwd, err := os.Getwd()
	if err == nil {
		candidates := []string{
			filepath.Join(cwd, "bin", "tsuite-runner"),
			filepath.Join(cwd, "tsuite-runner"),
		}
		for _, path := range candidates {
			if _, err := os.Stat(path); err == nil {
				return path
			}
		}
	}

	return ""
}

// runTestWithRunner executes a single test using the external runner binary.
// The runner reports results directly to the API, so we just need to wait for completion.
// Returns: passed, error string, duration, cancelled
func runTestWithRunner(ctx context.Context, runnerBinary, suitePath, testID, apiURL, runID, baseWorkdir string, timeout time.Duration) (bool, string, time.Duration, bool) {
	startTime := time.Now()

	// Check if already cancelled
	select {
	case <-ctx.Done():
		return false, "cancelled", 0, true
	default:
	}

	// Build command arguments
	args := []string{
		"--suite-path", suitePath,
		"--test-id", testID,
	}
	if apiURL != "" {
		args = append(args, "--api-url", apiURL)
	}
	if runID != "" {
		args = append(args, "--run-id", runID)

		// Set log directory for unified logging (standalone mode)
		// Structure: ~/.tsuite/runs/{run_id}/{uc}/{tc}/
		parts := strings.SplitN(testID, "/", 2)
		if len(parts) == 2 {
			logDir := filepath.Join(os.Getenv("HOME"), ".tsuite", "runs", runID, parts[0], parts[1])
			os.MkdirAll(logDir, 0755)
			args = append(args, "--log-dir", logDir)
		}
	}
	if baseWorkdir != "" {
		testWorkdir := filepath.Join(baseWorkdir, strings.ReplaceAll(testID, "/", "_"))
		os.MkdirAll(testWorkdir, 0755)
		args = append(args, "--workdir", testWorkdir)
	}

	// Create command with combined timeout and cancellation context
	timeoutCtx, timeoutCancel := context.WithTimeout(ctx, timeout)
	defer timeoutCancel()

	cmd := exec.CommandContext(timeoutCtx, runnerBinary, args...)
	cmd.Env = os.Environ()
	// Set process group so we can kill the whole tree
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// Capture output
	output, err := cmd.CombinedOutput()
	duration := time.Since(startTime)

	// Check if cancelled (parent context)
	if ctx.Err() == context.Canceled {
		return false, "cancelled", duration, true
	}

	if timeoutCtx.Err() == context.DeadlineExceeded {
		return false, "test timed out", duration, false
	}

	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			// Runner exited with non-zero status (test failed)
			// Extract error from output
			errMsg := "test failed"
			lines := strings.Split(strings.TrimSpace(string(output)), "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "FAILED:") {
					errMsg = strings.TrimPrefix(line, "FAILED: ")
					break
				}
				if strings.HasPrefix(line, "Error:") {
					errMsg = strings.TrimPrefix(line, "Error: ")
					break
				}
			}
			if errMsg == "test failed" && len(lines) > 0 {
				// Use last line as error
				errMsg = lines[len(lines)-1]
			}
			return false, errMsg, duration, false
		}
		return false, fmt.Sprintf("runner error: %v", err), duration, false
	}

	return true, "", duration, false
}

// runTestsWithRunnerSequential runs tests sequentially using the external runner binary
// Returns: passed, failed, skipped, failedTests, cancelled
func runTestsWithRunnerSequential(ctx context.Context, cancelFunc context.CancelFunc, runnerBinary, suitePath string, tests []string, apiURL, runID, baseWorkdir string, timeout time.Duration) (passed, failed, skipped int, failedTests []string, cancelled bool) {
	apiClient := client.NewClient(apiURL)

	// Start cancel checker goroutine
	executor.StartCancelChecker(ctx, cancelFunc, apiClient, runID)

	for _, testID := range tests {
		// Check if cancelled before starting test
		select {
		case <-ctx.Done():
			fmt.Printf("[SKIP] %s (cancelled)\n", testID)
			skipped++
			cancelled = true
			continue
		default:
		}

		fmt.Printf("\n[RUN] %s\n", testID)

		testPassed, testError, duration, wasCancelled := runTestWithRunner(ctx, runnerBinary, suitePath, testID, apiURL, runID, baseWorkdir, timeout)

		if wasCancelled {
			fmt.Printf("[SKIP] %s (cancelled)\n", testID)
			skipped++
			cancelled = true
		} else if testPassed {
			fmt.Printf("[PASS] %s (%.1fs)\n", testID, duration.Seconds())
			passed++
		} else {
			fmt.Printf("[FAIL] %s - %s (%.1fs)\n", testID, testError, duration.Seconds())
			failed++
			failedTests = append(failedTests, testID)
		}
	}
	return
}

// runTestsWithRunnerParallel runs tests in parallel using the external runner binary
// Returns: passed, failed, skipped, failedTests, cancelled
func runTestsWithRunnerParallel(ctx context.Context, cancelFunc context.CancelFunc, runnerBinary, suitePath string, tests []string, workers int, apiURL, runID, baseWorkdir string, timeout time.Duration) (passed, failed, skipped int, failedTests []string, cancelled bool) {
	testCh := make(chan string, len(tests))
	resultCh := make(chan executor.TestResult, len(tests))
	apiClient := client.NewClient(apiURL)

	// Start cancel checker goroutine
	executor.StartCancelChecker(ctx, cancelFunc, apiClient, runID)

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for testID := range testCh {
				// Check if cancelled before starting test
				select {
				case <-ctx.Done():
					resultCh <- executor.TestResult{TestID: testID, Cancelled: true}
					continue
				default:
				}

				testPassed, testError, duration, wasCancelled := runTestWithRunner(ctx, runnerBinary, suitePath, testID, apiURL, runID, baseWorkdir, timeout)
				resultCh <- executor.TestResult{
					TestID:    testID,
					Passed:    testPassed,
					Error:     testError,
					Duration:  duration,
					Cancelled: wasCancelled,
				}
			}
		}()
	}

	// Send tests to workers
	for _, t := range tests {
		testCh <- t
	}
	close(testCh)

	// Wait for all workers
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// Collect results
	results := executor.CollectResults(resultCh)
	return results.Passed, results.Failed, results.Skipped, results.FailedTests, results.Cancelled
}

func main() {
	rootCmd := &cobra.Command{
		Use:   "tsuite",
		Short: "YAML-driven integration test framework",
		Long: `mcp-mesh-tsuite - YAML-driven integration test framework.

Features: embedded dashboard UI, Docker/standalone modes for isolation, parallel test execution.`,
		Version: version,
	}

	// API command
	apiCmd := &cobra.Command{
		Use:   "api",
		Short: "Start the API server",
		Long:  `Start the REST API server for the dashboard.`,
		RunE:  runAPIServer,
	}

	var apiPort int
	apiCmd.Flags().IntVarP(&apiPort, "port", "p", 9999, "Server port")
	apiCmd.Flags().BoolP("detach", "d", false, "Run server in background")

	rootCmd.AddCommand(apiCmd)

	// Run command
	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Run tests",
		Long:  `Run test cases from the test suite.`,
		RunE:  runTests,
	}

	runCmd.Flags().StringVarP(&suitePath, "suite-path", "s", ".", "Path to test suite")
	runCmd.Flags().IntVarP(&parallel, "parallel", "p", 1, "Number of parallel test runners")
	runCmd.Flags().StringSliceVar(&ucFilter, "uc", nil, "Filter by use case (e.g., uc01_registry)")
	runCmd.Flags().StringSliceVar(&tcFilter, "tc", nil, "Filter by test case (e.g., tc01_agent_registration)")
	runCmd.Flags().StringSliceVar(&tagFilter, "tags", nil, "Filter by tags")
	runCmd.Flags().BoolVar(&dryRun, "dry-run", false, "List tests without running")
	runCmd.Flags().StringVar(&apiURL, "api-url", "http://localhost:9999", "API server URL")
	runCmd.Flags().StringVar(&runnerPath, "runner-path", "", "Path to runner binary (default: auto-detect)")

	rootCmd.AddCommand(runCmd)

	// List command
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List tests",
		Long:  `List all test cases in the suite.`,
		RunE:  listTests,
	}

	listCmd.Flags().StringVarP(&suitePath, "suite-path", "s", ".", "Path to test suite")
	listCmd.Flags().StringSliceVar(&ucFilter, "uc", nil, "Filter by use case")
	listCmd.Flags().StringSliceVar(&tagFilter, "tags", nil, "Filter by tags")

	rootCmd.AddCommand(listCmd)

	// Version command
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("tsuite version %s\n", version)
		},
	}
	rootCmd.AddCommand(versionCmd)

	// Check command
	checkCmd := &cobra.Command{
		Use:   "check",
		Short: "Check Docker availability",
		Run: func(cmd *cobra.Command, args []string) {
			ok, msg := runner.CheckDockerAvailable()
			if ok {
				fmt.Printf("✓ Docker is available: %s\n", msg)
			} else {
				fmt.Printf("✗ Docker is not available: %s\n", msg)
				os.Exit(1)
			}
		},
	}
	rootCmd.AddCommand(checkCmd)

	// Stop command
	stopCmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop the running API server",
		Long:  `Gracefully stop the API server that was started with 'tsuite api'.`,
		RunE:  stopServer,
	}
	rootCmd.AddCommand(stopCmd)

	// Clear command
	clearCmd := &cobra.Command{
		Use:   "clear",
		Short: "Clear test data (database, logs, reports)",
		Long: `Clear test data from ~/.tsuite directory.

Examples:
  tsuite clear --all              Clear all test data
  tsuite clear --all --force      Clear without confirmation`,
		RunE: clearData,
	}
	var clearAll, clearForce bool
	clearCmd.Flags().BoolVar(&clearAll, "all", false, "Clear all test data")
	clearCmd.Flags().BoolVarP(&clearForce, "force", "f", false, "Skip confirmation prompt")
	rootCmd.AddCommand(clearCmd)

	// Scaffold command
	scaffoldCmd := &cobra.Command{
		Use:   "scaffold [agent_dirs...]",
		Short: "Generate test case from agent directories",
		Long: `Generate test case from agent directories.

Copies agent source directories to suite artifacts and generates test.yaml
with setup, agent startup, and placeholder test steps.

Examples:
  # Standard agent directory (has main.py or package.json)
  tsuite scaffold --suite ./my-suite --uc uc01_tags --tc tc01_test ./agent1 ./agent2

  # Flat directory with standalone scripts
  tsuite scaffold --suite ./my-suite --uc uc01_examples --tc tc01_simple --agent ./examples/simple --filter "*.py"

  # Preview without creating files
  tsuite scaffold --suite ./my-suite --uc uc01_tags --tc tc01_test --dry-run ./agent1`,
		Args: cobra.MinimumNArgs(0), // Allow 0 args when using --filter
		RunE: runScaffold,
	}
	var scaffoldSuite, scaffoldUC, scaffoldTC, scaffoldName, scaffoldArtifactLevel, scaffoldFilter string
	var scaffoldDryRun, scaffoldForce, scaffoldSkipCopy, scaffoldNoInteractive, scaffoldSymlink bool
	scaffoldCmd.Flags().StringVar(&scaffoldSuite, "suite", "", "Path to test suite (required)")
	scaffoldCmd.Flags().StringVar(&scaffoldUC, "uc", "", "Use case name (e.g., uc01_tags)")
	scaffoldCmd.Flags().StringVar(&scaffoldTC, "tc", "", "Test case name (e.g., tc01_test)")
	scaffoldCmd.Flags().StringVar(&scaffoldName, "name", "", "Test name (default: derived from tc)")
	scaffoldCmd.Flags().StringVar(&scaffoldArtifactLevel, "artifact-level", "tc", "Where to copy artifacts: tc or uc")
	scaffoldCmd.Flags().BoolVar(&scaffoldDryRun, "dry-run", false, "Preview without creating files")
	scaffoldCmd.Flags().BoolVar(&scaffoldForce, "force", false, "Overwrite existing TC")
	scaffoldCmd.Flags().BoolVar(&scaffoldSkipCopy, "skip-artifact-copy", false, "Skip copying artifacts")
	scaffoldCmd.Flags().BoolVar(&scaffoldSymlink, "symlink", false, "Create symlinks to agents instead of copying")
	scaffoldCmd.Flags().BoolVar(&scaffoldNoInteractive, "no-interactive", false, "Skip prompts, use defaults")
	scaffoldCmd.Flags().StringVar(&scaffoldFilter, "filter", "", "Glob for standalone scripts in flat directories (e.g., '*.py')")
	scaffoldCmd.MarkFlagRequired("suite")
	rootCmd.AddCommand(scaffoldCmd)

	// Man command
	manCmd := &cobra.Command{
		Use:   "man [topic]",
		Short: "View documentation for tsuite",
		Long: `View documentation for tsuite.

Examples:
  tsuite man --list           List all topics
  tsuite man quickstart       View quickstart guide
  tsuite man handlers         View handlers documentation
  tsuite man --raw handlers   Output raw markdown (for LLM usage)`,
		Args: cobra.MaximumNArgs(1),
		Run:  runMan,
	}
	var manListTopics, manRaw bool
	manCmd.Flags().BoolVar(&manListTopics, "list", false, "List available topics")
	manCmd.Flags().BoolVar(&manRaw, "raw", false, "Output raw markdown without formatting (for LLM usage)")
	rootCmd.AddCommand(manCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runAPIServer(cmd *cobra.Command, args []string) error {
	port, _ := cmd.Flags().GetInt("port")
	detach, _ := cmd.Flags().GetBool("detach")

	// Check if already running
	running, existingPID := isServerRunning()
	if running {
		fmt.Printf("Server already running (PID: %d)\n", existingPID)
		fmt.Println("Use 'tsuite stop' to stop it first")
		return nil
	}

	// Handle detach mode
	if detach && os.Getenv("TSUITE_DETACHED") != "1" {
		return startDetached(port)
	}

	// Set database path (use same location as Python version)
	dbPath := db.DefaultDBPath()
	if os.Getenv("TSUITE_DETACHED") != "1" {
		fmt.Printf("Using database: %s\n", dbPath)
	}

	// Write PID file
	tsuiteDir := getTsuiteHome()
	os.MkdirAll(tsuiteDir, 0755)
	pidFile := getPidFile()
	os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", os.Getpid())), 0644)

	// Also save the port for status command
	portFile := filepath.Join(tsuiteDir, "server.port")
	os.WriteFile(portFile, []byte(fmt.Sprintf("%d", port)), 0644)

	// Clean up PID file on exit
	defer os.Remove(pidFile)
	defer os.Remove(portFile)

	server, err := api.NewServer(port)
	if err != nil {
		return fmt.Errorf("failed to create server: %w", err)
	}

	return server.Run()
}

func startDetached(port int) error {
	// Get executable path
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	// Build command: tsuite api --port <port> (without --detach)
	cmdArgs := []string{"api", "--port", fmt.Sprintf("%d", port)}

	proc := exec.Command(exe, cmdArgs...)
	proc.Env = append(os.Environ(), "TSUITE_DETACHED=1")

	// Redirect stdout/stderr to log file
	tsuiteDir := getTsuiteHome()
	os.MkdirAll(tsuiteDir, 0755)
	logFile, err := os.OpenFile(filepath.Join(tsuiteDir, "server.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to create log file: %w", err)
	}
	proc.Stdout = logFile
	proc.Stderr = logFile

	// Start the detached process
	if err := proc.Start(); err != nil {
		logFile.Close()
		return fmt.Errorf("failed to start server: %w", err)
	}

	// Wait a moment and check if it's running
	time.Sleep(500 * time.Millisecond)

	running, pid := isServerRunning()
	if !running {
		// Read last few lines of log for error info
		logFile.Close()
		logContent, _ := os.ReadFile(filepath.Join(tsuiteDir, "server.log"))
		lines := strings.Split(string(logContent), "\n")
		lastLines := lines
		if len(lines) > 5 {
			lastLines = lines[len(lines)-5:]
		}
		return fmt.Errorf("server failed to start. Check %s/server.log:\n%s", tsuiteDir, strings.Join(lastLines, "\n"))
	}

	fmt.Printf("Server started in background (PID: %d, port: %d)\n", pid, port)
	fmt.Printf("Logs: %s/server.log\n", tsuiteDir)
	fmt.Println("Use 'tsuite stop' to stop the server")
	return nil
}

func runTests(cmd *cobra.Command, args []string) error {
	// Resolve suite path (including symlinks for consistent matching with database)
	absPath, err := filepath.Abs(suitePath)
	if err != nil {
		return fmt.Errorf("failed to resolve suite path: %w", err)
	}
	// Resolve symlinks to match paths stored in database
	absPath, err = filepath.EvalSymlinks(absPath)
	if err != nil {
		return fmt.Errorf("failed to resolve symlinks: %w", err)
	}

	// Load suite config to determine mode
	suiteConfig, err := config.LoadSuiteConfig(absPath)
	if err != nil {
		return fmt.Errorf("failed to load suite config: %w", err)
	}

	// Determine mode from config (default to standalone)
	mode := suiteConfig.Suite.Mode
	if mode == "" {
		mode = "standalone"
	}

	// Use config's max_workers if --parallel not explicitly set
	if !cmd.Flags().Changed("parallel") && suiteConfig.Execution.MaxWorkers > 0 {
		parallel = suiteConfig.Execution.MaxWorkers
	}

	fmt.Printf("Suite: %s (mode: %s, parallel: %d)\n", suiteConfig.Suite.Name, mode, parallel)

	// List all tests
	allTests, err := runner.ListTests(absPath)
	if err != nil {
		return fmt.Errorf("failed to list tests: %w", err)
	}

	// Filter tests
	tests := filterTests(allTests)

	if len(tests) == 0 {
		fmt.Println("No tests found matching the filters")
		return nil
	}

	fmt.Printf("Found %d test(s)\n", len(tests))

	// Dry run - just list tests
	if dryRun {
		fmt.Println("\nTests to run:")
		for _, t := range tests {
			fmt.Printf("  - %s\n", t)
		}
		return nil
	}

	// Check Docker availability if docker mode
	if mode == "docker" {
		ok, msg := runner.CheckDockerAvailable()
		if !ok {
			return fmt.Errorf("Docker not available: %s", msg)
		}
		fmt.Printf("Docker: %s\n", msg)
	}

	// Create temp workdir for test execution
	var baseWorkdir string
	tmpDir, err := os.MkdirTemp("", "tsuite_")
	if err != nil {
		return fmt.Errorf("failed to create temp workdir: %w", err)
	}
	baseWorkdir = tmpDir
	if mode == "standalone" {
		fmt.Printf("Workdir: %s\n", baseWorkdir)
	}
	defer os.RemoveAll(baseWorkdir) // Cleanup after run

	// Create API client
	apiClient := client.NewClient(apiURL)

	// Check API server health
	if err := apiClient.HealthCheck(); err != nil {
		fmt.Printf("Warning: API server not available at %s: %v\n", apiURL, err)
		fmt.Println("Results will not be saved to database. Start the API server with: tsuite api")
		apiClient = nil
	} else {
		fmt.Printf("API Server: %s\n", apiURL)
	}

	// Create run via API
	var runID string
	var suiteID int64
	if apiClient != nil {
		// Sync suite to get suite_id
		syncResp, err := apiClient.UpsertSuite(&client.SyncSuiteRequest{
			FolderPath: absPath,
			SuiteName:  suiteConfig.Suite.Name,
			Mode:       mode,
			TestCount:  len(tests),
		})
		if err != nil {
			fmt.Printf("Warning: Failed to sync suite: %v\n", err)
		} else if syncResp != nil {
			suiteID = syncResp.ID
		}

		// Build test info for API
		testInfos := make([]client.TestInfo, len(tests))
		for i, testID := range tests {
			parts := strings.Split(testID, "/")
			testInfos[i] = client.TestInfo{
				TestID:   testID,
				UseCase:  parts[0],
				TestCase: parts[1],
			}
		}

		// Build display name
		displayName := suiteConfig.Suite.Name
		if len(tests) == 1 {
			// Single test - include test name in display
			displayName = suiteConfig.Suite.Name + " / " + tests[0]
		}

		createReq := &client.CreateRunRequest{
			SuiteID:     suiteID,
			SuiteName:   suiteConfig.Suite.Name,
			DisplayName: displayName,
			TotalTests:  len(tests),
			Mode:        mode,
			Tests:       testInfos,
		}

		resp, err := apiClient.CreateRun(createReq)
		if err != nil {
			fmt.Printf("Warning: Failed to create run: %v\n", err)
		} else {
			runID = resp.RunID
			fmt.Printf("Run ID: %s\n", runID[:12])
		}
	}

	// Run tests
	startTime := time.Now()
	passed := 0
	failed := 0
	skipped := 0
	cancelled := false
	var failedTests []string

	// Get docker image from config
	dockerImage := suiteConfig.Docker.BaseImage
	if dockerImage == "" {
		dockerImage = "tsuite-mesh:local" // Default image
	}

	// Set test timeout (10 minutes default)
	testTimeout := 10 * time.Minute

	// Create context for cancellation
	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()

	if mode == "docker" {
		// Docker mode: use DockerExecutor which mounts Go runner into container
		if parallel > 1 && len(tests) > 1 {
			passed, failed, skipped, failedTests, cancelled = runTestsParallelWithDocker(ctx, cancelFunc, absPath, tests, parallel, apiClient, runID, baseWorkdir, dockerImage, apiURL)
		} else {
			passed, failed, skipped, failedTests, cancelled = runTestsSequentialWithDocker(ctx, cancelFunc, absPath, tests, apiClient, runID, baseWorkdir, dockerImage, apiURL)
		}
	} else {
		// Standalone mode: use external runner binary
		runnerBinaryPath := findRunnerBinary()
		if runnerBinaryPath == "" {
			return fmt.Errorf("runner binary not found. Build it with: make build-runner")
		}
		if parallel > 1 && len(tests) > 1 {
			passed, failed, skipped, failedTests, cancelled = runTestsWithRunnerParallel(ctx, cancelFunc, runnerBinaryPath, absPath, tests, parallel, apiURL, runID, baseWorkdir, testTimeout)
		} else {
			passed, failed, skipped, failedTests, cancelled = runTestsWithRunnerSequential(ctx, cancelFunc, runnerBinaryPath, absPath, tests, apiURL, runID, baseWorkdir, testTimeout)
		}
	}

	// Complete or cancel run via API
	if apiClient != nil && runID != "" {
		if cancelled {
			if err := apiClient.CancelRun(runID); err != nil {
				fmt.Printf("Warning: Failed to mark run as cancelled: %v\n", err)
			}
		} else {
			if err := apiClient.CompleteRun(runID); err != nil {
				fmt.Printf("Warning: Failed to complete run: %v\n", err)
			}
		}
	}

	// Print summary
	duration := time.Since(startTime)
	fmt.Println("\n" + strings.Repeat("=", 60))
	if cancelled {
		fmt.Printf("CANCELLED: %d passed, %d failed, %d skipped (%.1fs)\n", passed, failed, skipped, duration.Seconds())
	} else {
		fmt.Printf("SUMMARY: %d passed, %d failed (%.1fs)\n", passed, failed, duration.Seconds())
	}
	if len(failedTests) > 0 {
		fmt.Println("\nFailed tests:")
		for _, t := range failedTests {
			fmt.Printf("  ✗ %s\n", t)
		}
	}
	fmt.Println(strings.Repeat("=", 60))

	if failed > 0 {
		return fmt.Errorf("%d test(s) failed", failed)
	}

	return nil
}

func runTestsSequentialWithDocker(ctx context.Context, cancelFunc context.CancelFunc, suitePath string, tests []string, apiClient *client.Client, runID string, baseWorkdir string, dockerImage string, serverURL string) (passed, failed, skipped int, failedTests []string, cancelled bool) {
	// Create docker executor
	dockerConfig := &runner.ContainerConfig{
		Image:   dockerImage,
		Network: "bridge",
	}
	dockerExec, err := runner.NewDockerExecutor(serverURL, suitePath, baseWorkdir, dockerConfig, runID)
	if err != nil {
		fmt.Printf("Failed to create Docker executor: %v\n", err)
		return 0, len(tests), 0, tests, false
	}
	defer dockerExec.Close()

	// Start cancel checker goroutine
	if apiClient != nil {
		executor.StartCancelChecker(ctx, cancelFunc, apiClient, runID)
	}

	for _, testID := range tests {
		// Check if cancelled before starting test
		select {
		case <-ctx.Done():
			fmt.Printf("[SKIP] %s (cancelled)\n", testID)
			skipped++
			cancelled = true
			continue
		default:
		}

		fmt.Printf("\n[RUN] %s\n", testID)

		// Mark test as running via API
		if apiClient != nil && runID != "" {
			apiClient.UpdateTestStatus(runID, testID, &client.UpdateTestStatusRequest{
				Status: "running",
			})
		}

		// Run in Docker container (Go runner reports steps to API)
		// Use combined context with timeout
		testCtx, testCancel := context.WithTimeout(ctx, 10*time.Minute)
		result, err := dockerExec.ExecuteTest(testCtx, testID, nil)
		testCancel()

		// Check if cancelled during test
		if ctx.Err() == context.Canceled {
			fmt.Printf("[SKIP] %s (cancelled)\n", testID)
			skipped++
			cancelled = true
			continue
		}

		var testPassed bool
		var testError string
		var duration time.Duration

		if err != nil {
			testPassed = false
			testError = err.Error()
			duration = 0
			// Report failure to API since runner never started
			if apiClient != nil && runID != "" {
				apiClient.UpdateTestStatus(runID, testID, &client.UpdateTestStatusRequest{
					Status:       "failed",
					ErrorMessage: testError,
				})
			}
		} else {
			testPassed = result.ExitCode == 0 && result.Error == nil
			if result.Error != nil {
				testError = result.Error.Error()
			} else if result.ExitCode != 0 {
				testError = fmt.Sprintf("exit code %d", result.ExitCode)
				if result.Stderr != "" {
					lines := strings.Split(strings.TrimSpace(result.Stderr), "\n")
					if len(lines) > 3 {
						lines = lines[len(lines)-3:]
					}
					testError = strings.Join(lines, "; ")
				}
			}
			duration = result.Duration
		}

		if testPassed {
			fmt.Printf("[PASS] %s (%.1fs)\n", testID, duration.Seconds())
			passed++
		} else {
			fmt.Printf("[FAIL] %s - %s (%.1fs)\n", testID, testError, duration.Seconds())
			failed++
			failedTests = append(failedTests, testID)
		}
		// Note: Go runner inside container reports final status with steps to API
	}
	return
}

func runTestsParallelWithDocker(ctx context.Context, cancelFunc context.CancelFunc, suitePath string, tests []string, workers int, apiClient *client.Client, runID string, baseWorkdir string, dockerImage string, serverURL string) (passed, failed, skipped int, failedTests []string, cancelled bool) {
	testCh := make(chan string, len(tests))
	resultCh := make(chan executor.TestResult, len(tests))

	// Start cancel checker goroutine
	if apiClient != nil {
		executor.StartCancelChecker(ctx, cancelFunc, apiClient, runID)
	}

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			// Each worker gets its own docker executor (for isolation)
			dockerConfig := &runner.ContainerConfig{
				Image:   dockerImage,
				Network: "bridge",
			}
			dockerExec, err := runner.NewDockerExecutor(serverURL, suitePath, baseWorkdir, dockerConfig, runID)
			if err != nil {
				fmt.Printf("Worker %d: Failed to create Docker executor: %v\n", workerID, err)
				// Mark all remaining tests as failed
				for testID := range testCh {
					resultCh <- executor.TestResult{TestID: testID, Passed: false, Error: err.Error()}
				}
				return
			}
			defer dockerExec.Close()

			for testID := range testCh {
				// Check if cancelled before starting test
				select {
				case <-ctx.Done():
					resultCh <- executor.TestResult{TestID: testID, Cancelled: true}
					continue
				default:
				}

				// Mark test as running via API
				if apiClient != nil && runID != "" {
					apiClient.UpdateTestStatus(runID, testID, &client.UpdateTestStatusRequest{
						Status: "running",
					})
				}

				// Run in Docker container (Go runner reports steps to API)
				// Use combined context with timeout
				testCtx, testCancel := context.WithTimeout(ctx, 10*time.Minute)
				result, err := dockerExec.ExecuteTest(testCtx, testID, nil)
				testCancel()

				// Check if cancelled during test
				if ctx.Err() == context.Canceled {
					resultCh <- executor.TestResult{TestID: testID, Cancelled: true}
					continue
				}

				var testPassed bool
				var testError string
				var duration time.Duration

				if err != nil {
					testPassed = false
					testError = err.Error()
					duration = 0
					// Report failure to API since runner never started
					if apiClient != nil && runID != "" {
						apiClient.UpdateTestStatus(runID, testID, &client.UpdateTestStatusRequest{
							Status:       "failed",
							ErrorMessage: testError,
						})
					}
				} else {
					testPassed = result.ExitCode == 0 && result.Error == nil
					if result.Error != nil {
						testError = result.Error.Error()
					} else if result.ExitCode != 0 {
						testError = fmt.Sprintf("exit code %d", result.ExitCode)
						if result.Stderr != "" {
							lines := strings.Split(strings.TrimSpace(result.Stderr), "\n")
							if len(lines) > 3 {
								lines = lines[len(lines)-3:]
							}
							testError = strings.Join(lines, "; ")
						}
					}
					duration = result.Duration
				}

				resultCh <- executor.TestResult{
					TestID:   testID,
					Passed:   testPassed,
					Error:    testError,
					Duration: duration,
				}
				// Note: Go runner inside container reports final status with steps to API
			}
		}(i)
	}

	// Send tests to workers
	for _, t := range tests {
		testCh <- t
	}
	close(testCh)

	// Wait for all workers
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// Collect results
	results := executor.CollectResults(resultCh)
	return results.Passed, results.Failed, results.Skipped, results.FailedTests, results.Cancelled
}

func listTests(cmd *cobra.Command, args []string) error {
	absPath, err := filepath.Abs(suitePath)
	if err != nil {
		return fmt.Errorf("failed to resolve suite path: %w", err)
	}

	allTests, err := runner.ListTests(absPath)
	if err != nil {
		return fmt.Errorf("failed to list tests: %w", err)
	}

	tests := filterTests(allTests)

	if len(tests) == 0 {
		fmt.Println("No tests found")
		return nil
	}

	fmt.Printf("Found %d test(s):\n", len(tests))
	for _, t := range tests {
		fmt.Printf("  - %s\n", t)
	}

	return nil
}

func filterTests(tests []string) []string {
	var filtered []string

	for _, testID := range tests {
		parts := strings.Split(testID, "/")
		if len(parts) < 2 {
			continue
		}
		ucName := parts[0]
		tcName := parts[1]

		// Filter by use case
		if len(ucFilter) > 0 {
			match := false
			for _, uc := range ucFilter {
				if strings.Contains(ucName, uc) {
					match = true
					break
				}
			}
			if !match {
				continue
			}
		}

		// Filter by test case
		// Supports both formats:
		// - Full path: --tc uc01_registry/tc01_agent (exact match on testID)
		// - TC name only: --tc tc01_agent (substring match on tcName)
		if len(tcFilter) > 0 {
			match := false
			for _, tc := range tcFilter {
				if strings.Contains(tc, "/") {
					// Full path format - exact match on testID
					if testID == tc {
						match = true
						break
					}
				} else {
					// TC name only - substring match
					if strings.Contains(tcName, tc) {
						match = true
						break
					}
				}
			}
			if !match {
				continue
			}
		}

		// TODO: Filter by tags (requires loading test.yaml)

		filtered = append(filtered, testID)
	}

	return filtered
}

// Docker execution support
func runTestInDocker(ctx context.Context, suitePath string, testID string) (*runner.TestResult, error) {
	// This would use DockerExecutor to run tests in containers
	// For now, we run tests locally
	testRunner, err := runner.NewTestRunner(suitePath, "", "", "") // Empty baseWorkdir for docker mode
	if err != nil {
		return nil, err
	}
	return testRunner.RunTest(testID)
}

// =============================================================================
// Stop Command
// =============================================================================

func getTsuiteHome() string {
	return filepath.Join(os.Getenv("HOME"), ".tsuite")
}

func getPidFile() string {
	return filepath.Join(getTsuiteHome(), "server.pid")
}

func isServerRunning() (bool, int) {
	pidFile := getPidFile()
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return false, 0
	}

	var pid int
	_, err = fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &pid)
	if err != nil {
		return false, 0
	}

	// Check if process exists by sending signal 0
	process, err := os.FindProcess(pid)
	if err != nil {
		os.Remove(pidFile)
		return false, 0
	}

	// On Unix, FindProcess always succeeds, so we need to send signal 0
	err = process.Signal(syscall.Signal(0))
	if err != nil {
		os.Remove(pidFile)
		return false, 0
	}

	return true, pid
}

func stopServer(cmd *cobra.Command, args []string) error {
	running, pid := isServerRunning()

	if !running {
		fmt.Println("No server running")
		return nil
	}

	fmt.Printf("Stopping server (PID: %d)...\n", pid)

	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find process: %w", err)
	}

	// Send SIGTERM
	err = process.Signal(syscall.SIGTERM)
	if err != nil {
		// Process might already be dead
		os.Remove(getPidFile())
		fmt.Println("Server already stopped")
		return nil
	}

	// Wait for process to terminate (up to 5 seconds)
	for i := 0; i < 50; i++ {
		err = process.Signal(syscall.Signal(0))
		if err != nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Clean up PID file
	os.Remove(getPidFile())
	fmt.Println("Server stopped")
	return nil
}

// =============================================================================
// Clear Command
// =============================================================================

func clearData(cmd *cobra.Command, args []string) error {
	clearAll, _ := cmd.Flags().GetBool("all")
	force, _ := cmd.Flags().GetBool("force")

	if !clearAll {
		fmt.Println("Use --all to clear all test data")
		fmt.Println("  tsuite clear --all           Clear database, logs, and reports")
		fmt.Println("  tsuite clear --all --force   Clear without confirmation")
		return nil
	}

	tsuiteDir := getTsuiteHome()
	if _, err := os.Stat(tsuiteDir); os.IsNotExist(err) {
		fmt.Println("Nothing to clear (~/.tsuite does not exist)")
		return nil
	}

	if !force {
		fmt.Print("Delete ALL test data (database, logs, reports)? This cannot be undone. [y/N]: ")
		var response string
		fmt.Scanln(&response)
		if strings.ToLower(response) != "y" && strings.ToLower(response) != "yes" {
			fmt.Println("Aborted.")
			return nil
		}
	}

	var cleared []string

	// Clear database files
	patterns := []string{"*.db", "*.db-*"}
	for _, pattern := range patterns {
		matches, _ := filepath.Glob(filepath.Join(tsuiteDir, pattern))
		for _, f := range matches {
			if err := os.Remove(f); err == nil {
				cleared = append(cleared, filepath.Base(f))
			}
		}
	}

	// Clear runs directory
	runsDir := filepath.Join(tsuiteDir, "runs")
	if _, err := os.Stat(runsDir); err == nil {
		if err := os.RemoveAll(runsDir); err == nil {
			cleared = append(cleared, "runs/")
		}
	}

	// Clear reports directory
	reportsDir := filepath.Join(tsuiteDir, "reports")
	if _, err := os.Stat(reportsDir); err == nil {
		if err := os.RemoveAll(reportsDir); err == nil {
			cleared = append(cleared, "reports/")
		}
	}

	// Clear server log
	serverLog := filepath.Join(tsuiteDir, "server.log")
	if err := os.Remove(serverLog); err == nil {
		cleared = append(cleared, "server.log")
	}

	// Clear PID file
	pidFile := getPidFile()
	if err := os.Remove(pidFile); err == nil {
		cleared = append(cleared, "server.pid")
	}

	if len(cleared) > 0 {
		fmt.Printf("Cleared: %s\n", strings.Join(cleared, ", "))
	} else {
		fmt.Println("Nothing to clear")
	}

	return nil
}

// =============================================================================
// Scaffold Command
// =============================================================================

func runScaffold(cmd *cobra.Command, args []string) error {
	suitePath, _ := cmd.Flags().GetString("suite")
	ucName, _ := cmd.Flags().GetString("uc")
	tcName, _ := cmd.Flags().GetString("tc")
	testName, _ := cmd.Flags().GetString("name")
	artifactLevel, _ := cmd.Flags().GetString("artifact-level")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	force, _ := cmd.Flags().GetBool("force")
	skipCopy, _ := cmd.Flags().GetBool("skip-artifact-copy")
	useSymlinks, _ := cmd.Flags().GetBool("symlink")
	noInteractive, _ := cmd.Flags().GetBool("no-interactive")
	filter, _ := cmd.Flags().GetString("filter")

	// Resolve suite path
	absPath, err := filepath.Abs(suitePath)
	if err != nil {
		return fmt.Errorf("failed to resolve suite path: %w", err)
	}

	// Validate suite
	if err := scaffold.ValidateSuite(absPath); err != nil {
		return err
	}

	var agents []scaffold.AgentInfo
	var flatScriptDir string // For --filter mode: the directory containing flat scripts

	if filter != "" {
		// Filter mode: scan directory for matching scripts
		if len(args) != 1 {
			return fmt.Errorf("--filter requires exactly one directory argument")
		}

		flatScriptDir = args[0]
		scripts, err := scaffold.DiscoverScriptsByFilter(flatScriptDir, filter)
		if err != nil {
			return err
		}
		if len(scripts) == 0 {
			return fmt.Errorf("no files matching '%s' found in %s", filter, flatScriptDir)
		}
		agents = scripts
	} else {
		// Standard mode: validate agent directories
		var agentPaths []string
		for _, arg := range args {
			if strings.TrimSpace(arg) != "" {
				agentPaths = append(agentPaths, arg)
			}
		}
		if len(agentPaths) == 0 {
			return fmt.Errorf("at least one agent directory is required")
		}
		if err := scaffold.ValidateNoParentDirs(agentPaths); err != nil {
			return err
		}

		for _, agentPath := range agentPaths {
			agent, err := scaffold.ValidateAgentDir(agentPath)
			if err != nil {
				return err
			}
			agents = append(agents, *agent)
		}
	}

	// Show detected agents/scripts
	fmt.Printf("\nSuite: %s\n", absPath)
	if filter != "" {
		fmt.Printf("Discovered scripts in %s (filter: %s):\n", flatScriptDir, filter)
		for _, agent := range agents {
			fmt.Printf("  - %s\n", agent.EntryPoint)
		}
	} else {
		fmt.Println("Detected agents:")
		for _, agent := range agents {
			typeLabel := "Python"
			if agent.AgentType == "typescript" {
				typeLabel = "TypeScript"
			}
			fmt.Printf("  - %s (%s)\n", agent.Name, typeLabel)
		}
	}
	fmt.Println()

	// Require UC and TC in non-interactive mode
	if noInteractive {
		if ucName == "" {
			return fmt.Errorf("--uc is required in non-interactive mode")
		}
		if tcName == "" {
			return fmt.Errorf("--tc is required in non-interactive mode")
		}
	} else {
		// Interactive mode prompts
		if ucName == "" {
			fmt.Print("Use case name (e.g., uc01_tags): ")
			fmt.Scanln(&ucName)
		}
		if tcName == "" {
			fmt.Print("Test case name (e.g., tc01_test): ")
			fmt.Scanln(&tcName)
		}
	}

	// Validate UC and TC names
	if !strings.HasPrefix(ucName, "uc") {
		return fmt.Errorf("UC name should start with 'uc' (e.g., uc01_tags)")
	}
	if !strings.HasPrefix(tcName, "tc") {
		return fmt.Errorf("TC name should start with 'tc' (e.g., tc01_test)")
	}

	config := &scaffold.Config{
		SuitePath:        absPath,
		UCName:           ucName,
		TCName:           tcName,
		Agents:           agents,
		ArtifactLevel:    artifactLevel,
		TestName:         testName,
		DryRun:           dryRun,
		Force:            force,
		SkipArtifactCopy: skipCopy,
		UseSymlinks:      useSymlinks,
		FlatScriptDir:    flatScriptDir,
		Filter:           filter,
	}

	return scaffold.Run(config)
}

// =============================================================================
// Man Command
// =============================================================================

func runMan(cmd *cobra.Command, args []string) {
	listTopics, _ := cmd.Flags().GetBool("list")
	raw, _ := cmd.Flags().GetBool("raw")
	renderer := man.NewRenderer(os.Stdout)

	if listTopics || len(args) == 0 {
		renderer.RenderList()
		return
	}

	topic := args[0]
	page := man.GetPage(topic)
	if page == nil {
		renderer.RenderNotFound(topic)
		os.Exit(1)
	}

	if raw {
		// Output raw markdown for LLM usage
		content, err := page.GetContent()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Print(content)
		return
	}

	if err := renderer.RenderPage(page); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
