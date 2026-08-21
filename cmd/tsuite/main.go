package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/api"
	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/client"
	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/config"
	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/db"
	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/man"
	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/runner"
	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/scaffold"
	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/scheduler"
	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/worker"
)

var (
	// version is set at build time via ldflags: -ldflags "-X main.version=X.Y.Z"
	version = "dev"
)

// Run command flags
var (
	suitePath   string
	parallel    int
	ucFilter    []string
	tcFilter    []string
	dryRun      bool
	apiURL      string
	runnerPath  string
	tcFile      string
	executeMode bool
	runIDFlag   string
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

// detectHostIP returns the host's outbound IP address
func detectHostIP() string {
	conn, err := net.DialTimeout("udp", "8.8.8.8:80", time.Second)
	if err != nil {
		return ""
	}
	defer conn.Close()
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
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
	runCmd.Flags().BoolVar(&dryRun, "dry-run", false, "List tests without running")
	runCmd.Flags().StringVar(&apiURL, "api-url", "http://localhost:9999", "API server URL")
	runCmd.Flags().StringVar(&runnerPath, "runner-path", "", "Path to runner binary (default: auto-detect)")
	runCmd.Flags().StringVar(&tcFile, "tc-file", "", "File containing test IDs to run (one per line)")
	runCmd.Flags().BoolVar(&executeMode, "execute", false, "Direct execution mode (used by API server)")
	runCmd.Flags().StringVar(&runIDFlag, "run-id", "", "Pre-assigned run ID (used by API server)")
	runCmd.Flags().MarkHidden("execute")
	runCmd.Flags().MarkHidden("run-id")
	runCmd.MarkFlagsMutuallyExclusive("uc", "tc")

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
		Short: "Check Docker, K8s, and SSH availability",
		Run: func(cmd *cobra.Command, args []string) {
			// Check Docker
			ok, msg := runner.CheckDockerAvailable()
			if ok {
				fmt.Printf("Docker: %s\n", msg)
			} else {
				fmt.Printf("Docker: not available (%s)\n", msg)
			}

			// Check K8s
			k8sOk, k8sMsg := runner.CheckK8sAvailable()
			if k8sOk {
				fmt.Printf("K8s: %s\n", k8sMsg)
			} else {
				fmt.Printf("K8s: not available (%s)\n", k8sMsg)
			}

			// Check SSH if suite-path provided
			checkSuitePath, _ := cmd.Flags().GetString("suite-path")
			if checkSuitePath != "" {
				absPath, err := filepath.Abs(checkSuitePath)
				if err == nil {
					absPath, _ = filepath.EvalSymlinks(absPath)
				}
				if err == nil {
					suiteConfig, err := config.LoadSuiteConfig(absPath)
					if err == nil && suiteConfig.Standalone.Type == "remote" && suiteConfig.SSH.Host != "" {
						sshCmd := exec.Command("ssh", "-o", "ConnectTimeout=5", "-o", "BatchMode=yes", suiteConfig.SSH.Host, "uname -srm")
						output, err := sshCmd.CombinedOutput()
						if err != nil {
							fmt.Printf("SSH (%s): not available (%v)\n", suiteConfig.SSH.Host, err)
						} else {
							fmt.Printf("SSH (%s): Available (%s)\n", suiteConfig.SSH.Host, strings.TrimSpace(string(output)))
						}
					}
				}
			}
		},
	}
	checkCmd.Flags().StringP("suite-path", "s", "", "Path to test suite (for SSH check)")
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
		Short: "Clear test data",
		Long: `Clear test run history or all tsuite data.

Examples:
  tsuite clear --runs             Clear only test run history (preserves suites & secrets)
  tsuite clear --runs --force     Clear runs without confirmation
  tsuite clear --all              Clear ALL data (database, logs, reports)
  tsuite clear --all --force      Clear without confirmation`,
		RunE: clearData,
	}
	var clearAll, clearRuns, clearForce bool
	clearCmd.Flags().BoolVar(&clearAll, "all", false, "Clear all test data (database, logs, reports, secrets, suites)")
	clearCmd.Flags().BoolVar(&clearRuns, "runs", false, "Clear only test run history (preserves suites and secrets)")
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

	api.Version = version

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
	if executeMode {
		return executeTests(cmd, args)
	}
	return delegateToAPI(cmd, args)
}

func executeTests(cmd *cobra.Command, args []string) error {
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

	// Check if suite is disabled
	if suiteConfig.Suite.Disabled {
		return fmt.Errorf("suite %q is disabled", suiteConfig.Suite.Name)
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

	// List all tests with disabled info
	allTests, disabledSet, err := runner.ListTestsWithDisabled(absPath)
	if err != nil {
		return fmt.Errorf("failed to list tests: %w", err)
	}

	// Filter tests
	tests := filterTests(allTests)

	// Separate disabled tests from active
	var activeTests []string
	var disabledTests []string
	for _, t := range tests {
		if disabledSet[t] {
			disabledTests = append(disabledTests, t)
		} else {
			activeTests = append(activeTests, t)
		}
	}

	// Build dependency graph if any test has depends_on
	depLoader := func(testID string) []string {
		parts := strings.Split(testID, "/")
		if len(parts) != 2 {
			return nil
		}
		testPath := filepath.Join(absPath, "suites", parts[0], parts[1])
		tc, err := config.LoadTestConfig(testPath)
		if err != nil || tc == nil {
			return nil
		}
		return tc.DependsOn
	}
	testDAG, err := scheduler.Build(activeTests, depLoader)
	if err != nil {
		return fmt.Errorf("dependency graph error: %w", err)
	}
	if err := testDAG.Validate(); err != nil {
		return fmt.Errorf("dependency validation failed: %w", err)
	}
	if testDAG.HasDependencies() {
		fmt.Printf("Dependencies: %d test(s) have depends_on constraints\n", testDAG.DependentCount())
	}

	if len(activeTests) == 0 && len(disabledTests) == 0 {
		fmt.Println("No tests found matching the filters")
		return nil
	}

	if len(disabledTests) > 0 {
		fmt.Printf("Found %d of %d test(s) (%d disabled)\n", len(activeTests), len(tests), len(disabledTests))
	} else {
		fmt.Printf("Found %d test(s)\n", len(activeTests))
	}

	// Dry run - just list tests
	if dryRun {
		fmt.Println("\nTests to run:")
		for _, t := range activeTests {
			node := testDAG.GetNode(t)
			if node != nil && len(node.DependsOn) > 0 {
				fmt.Printf("  - %s (depends on: %s)\n", t, strings.Join(node.DependsOn, ", "))
			} else {
				fmt.Printf("  - %s\n", t)
			}
		}
		if len(disabledTests) > 0 {
			fmt.Println("\nDisabled tests:")
			for _, t := range disabledTests {
				fmt.Printf("  - %s (disabled)\n", t)
			}
		}
		return nil
	}

	// Check Docker availability based on mode
	if mode == "docker" {
		ok, msg := runner.CheckDockerAvailable()
		if !ok {
			return fmt.Errorf("Docker not available: %s", msg)
		}
		fmt.Printf("Docker: %s\n", msg)
	}

	if mode == "k8s" {
		ok, msg := runner.CheckK8sAvailable()
		if !ok {
			return fmt.Errorf("Kubernetes not available: %s", msg)
		}
		fmt.Printf("K8s: %s\n", msg)
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

	// Resolve config paths using secrets (e.g., REMOTE_WORKSPACE_ROOT for k8s NFS / SSH mount paths)
	if apiClient != nil {
		if secrets, err := apiClient.GetSecretValues(); err == nil {
			suiteConfig.ResolveWithSecrets(secrets, absPath)
		}
	}

	var runID string
	var suiteID int64

	if apiClient != nil {
		// Direct execution - create run via API
		// Sync suite to get suite_id
		syncResp, err := apiClient.UpsertSuite(&client.SyncSuiteRequest{
			FolderPath: absPath,
			SuiteName:  suiteConfig.Suite.Name,
			Mode:       mode,
			TestCount:  len(tests),
		})
		if err != nil {
			fmt.Printf("Warning: Failed to sync suite %q at %s: %v\n", suiteConfig.Suite.Name, absPath, err)
		} else if syncResp != nil {
			suiteID = syncResp.ID
		}
		if suiteID == 0 {
			fmt.Printf("Warning: No suite ID after sync for %q at %s - run will not be linked to a suite\n", suiteConfig.Suite.Name, absPath)
		}

		// Build test info for API (include ALL tests: active + disabled)
		testInfos := make([]client.TestInfo, len(tests))
		for i, testID := range tests {
			parts := strings.Split(testID, "/")
			testInfos[i] = client.TestInfo{
				TestID:   testID,
				UseCase:  parts[0],
				TestCase: parts[1],
			}
		}

		// Build display name based on filters
		var displayName string
		if tcFile != "" {
			// Multi-select from file
			displayName = fmt.Sprintf("%d selected tests", len(tests))
		} else if len(tests) == 1 {
			// Single test - include test name in display
			displayName = tests[0]
		}
		// If no specific filter, displayName stays empty and SQL CASE will compute it

		// Build filters JSON for the run record
		var filtersJSON string
		if tcFile != "" {
			filterData := map[string]any{}
			ids := make([]string, len(tests))
			copy(ids, tests)
			filterData["test_ids"] = ids
			if data, err := json.Marshal(filterData); err == nil {
				filtersJSON = string(data)
			}
		}

		createReq := &client.CreateRunRequest{
			RunID:       runIDFlag,
			SuiteID:     suiteID,
			SuiteName:   suiteConfig.Suite.Name,
			DisplayName: displayName,
			Filters:     filtersJSON,
			TotalTests:  len(tests),
			Mode:        mode,
			Tests:       testInfos,
		}

		if mode == "docker" && suiteConfig.Docker.BaseImage != "" {
			createReq.DockerImage = suiteConfig.Docker.BaseImage
		}

		// Extract version fields from suite config
		if pkgs, ok := suiteConfig.Raw["packages"].(map[string]any); ok {
			if v, ok := pkgs["cli_version"].(string); ok {
				createReq.CLIVersion = v
			}
			if v, ok := pkgs["sdk_python_version"].(string); ok {
				createReq.SDKPythonVersion = v
			}
			if v, ok := pkgs["sdk_typescript_version"].(string); ok {
				createReq.SDKTypescriptVersion = v
			}
		}

		resp, err := apiClient.CreateRun(createReq)
		if err != nil {
			fmt.Printf("Warning: Failed to create run: %v\n", err)
		} else {
			runID = resp.RunID
			fmt.Printf("Run ID: %s\n", runID[:12])
		}

		// Report disabled tests as skipped
		if runID != "" {
			for _, testID := range disabledTests {
				apiClient.UpdateTestStatus(runID, testID, &client.UpdateTestStatusRequest{
					Status:       "skipped",
					ErrorMessage: "disabled",
				})
			}
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

	var handler worker.WorkerHandler
	switch mode {
	case "docker":
		handler = worker.NewDockerHandler(apiURL, absPath, baseWorkdir, dockerImage, runID)
	case "k8s":
		if suiteConfig.K8s.NFSServer == "" || suiteConfig.K8s.NFSPath == "" {
			return fmt.Errorf("k8s.nfs_server and k8s.nfs_path are required in config.yaml")
		}
		// Resolve API URL for K8s (pods need to reach the API server)
		k8sAPIURL := apiURL
		if suiteConfig.K8s.APIUrl != "" {
			k8sAPIURL = suiteConfig.K8s.APIUrl
		} else {
			// Auto-detect: use host's outbound IP
			hostIP := detectHostIP()
			if hostIP != "" {
				k8sAPIURL = fmt.Sprintf("http://%s:9999", hostIP)
				fmt.Printf("K8s: auto-detected API URL: %s\n", k8sAPIURL)
			}
		}
		apiURL = k8sAPIURL
		k8sHandler, err := worker.NewK8sHandler(suiteConfig, absPath)
		if err != nil {
			return fmt.Errorf("K8s handler: %w", err)
		}
		handler = k8sHandler
	default:
		standaloneType := suiteConfig.Standalone.Type
		if standaloneType == "remote" {
			sshAPIURL := apiURL
			if suiteConfig.SSH.APIUrl != "" {
				sshAPIURL = suiteConfig.SSH.APIUrl
			} else if hostIP := detectHostIP(); hostIP != "" {
				sshAPIURL = fmt.Sprintf("http://%s:9999", hostIP)
			}
			sshHandler, sshErr := worker.NewSSHHandler(suiteConfig, absPath, sshAPIURL, testTimeout)
			if sshErr != nil {
				return fmt.Errorf("SSH handler: %w", sshErr)
			}
			handler = sshHandler
			fmt.Printf("SSH Host: %s (runner: %s)\n", suiteConfig.SSH.Host, suiteConfig.SSH.RunnerDir)
		} else {
			runnerBinaryPath := findRunnerBinary()
			if runnerBinaryPath == "" {
				return fmt.Errorf("runner binary not found. Build it with: make build-runner")
			}
			handler = worker.NewStandaloneHandler(runnerBinaryPath, absPath, baseWorkdir, testTimeout)
		}
	}
	defer handler.Close()

	var dagPtr *scheduler.DAG
	if testDAG.HasDependencies() {
		dagPtr = testDAG
	}
	result := scheduler.RunScheduled(ctx, cancelFunc, worker.PoolConfig{
		Handler:   handler,
		Tests:     activeTests,
		Workers:   parallel,
		APIURL:    apiURL,
		RunID:     runID,
		Timeout:   testTimeout,
		APIClient: apiClient,
	}, dagPtr)
	passed = result.Passed
	failed = result.Failed
	skipped = result.Skipped
	failedTests = result.FailedTests
	cancelled = result.Cancelled

	// Complete or cancel run via API
	if apiClient != nil && runID != "" {
		if cancelled {
			if err := apiClient.FinalizeCancelled(runID); err != nil {
				fmt.Printf("Warning: Failed to mark run as cancelled: %v\n", err)
			}
		} else {
			if err := apiClient.CompleteRun(runID); err != nil {
				fmt.Printf("Warning: Failed to complete run: %v\n", err)
			}
		}
	}

	// Add disabled count to skipped
	skipped += len(disabledTests)

	// Print summary
	duration := time.Since(startTime)
	fmt.Println("\n" + strings.Repeat("=", 60))
	if cancelled {
		fmt.Printf("CANCELLED: %d passed, %d failed, %d skipped (%.1fs)\n", passed, failed, skipped, duration.Seconds())
	} else if skipped > 0 {
		fmt.Printf("SUMMARY: %d passed, %d failed, %d skipped (%.1fs)\n", passed, failed, skipped, duration.Seconds())
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

func delegateToAPI(cmd *cobra.Command, args []string) error {
	// Resolve suite path
	absPath, err := filepath.Abs(suitePath)
	if err != nil {
		return fmt.Errorf("failed to resolve suite path: %w", err)
	}
	absPath, err = filepath.EvalSymlinks(absPath)
	if err != nil {
		return fmt.Errorf("failed to resolve symlinks: %w", err)
	}

	// Load suite config
	suiteConfig, err := config.LoadSuiteConfig(absPath)
	if err != nil {
		return fmt.Errorf("failed to load suite config: %w", err)
	}

	if suiteConfig.Suite.Disabled {
		return fmt.Errorf("suite %q is disabled", suiteConfig.Suite.Name)
	}

	mode := suiteConfig.Suite.Mode
	if mode == "" {
		mode = "standalone"
	}

	// Determine mode display
	modeDisplay := mode
	if mode == "standalone" && suiteConfig.Standalone.Type == "remote" {
		modeDisplay = "standalone/remote"
	}

	// Create API client and check health
	apiClient := client.NewClient(apiURL)
	if err := apiClient.HealthCheck(); err != nil {
		fmt.Printf("Warning: API server not available at %s\n", apiURL)
		fmt.Println("Running tests directly (start API with: tsuite api)")
		// Fall back to direct execution
		executeMode = true
		return executeTests(cmd, args)
	}

	// List tests for validation / dry-run
	allTests, disabledSet, err := runner.ListTestsWithDisabled(absPath)
	if err != nil {
		return fmt.Errorf("failed to list tests: %w", err)
	}
	tests := filterTests(allTests)

	var activeTests []string
	for _, t := range tests {
		if !disabledSet[t] {
			activeTests = append(activeTests, t)
		}
	}

	if len(activeTests) == 0 {
		fmt.Println("No tests found matching the filters")
		return nil
	}

	// Dry run
	if dryRun {
		// Build DAG for dependency display
		depLoader := func(testID string) []string {
			parts := strings.Split(testID, "/")
			if len(parts) != 2 {
				return nil
			}
			testPath := filepath.Join(absPath, "suites", parts[0], parts[1])
			tc, err := config.LoadTestConfig(testPath)
			if err != nil || tc == nil {
				return nil
			}
			return tc.DependsOn
		}
		testDAG, _ := scheduler.Build(activeTests, depLoader)

		fmt.Printf("Suite: %s (mode: %s)\n", suiteConfig.Suite.Name, modeDisplay)
		if testDAG != nil && testDAG.HasDependencies() {
			fmt.Printf("Dependencies: %d test(s) have depends_on constraints\n", testDAG.DependentCount())
		}
		fmt.Printf("Found %d test(s)\n", len(activeTests))
		fmt.Println("\nTests to run:")
		for _, t := range activeTests {
			if testDAG != nil {
				node := testDAG.GetNode(t)
				if node != nil && len(node.DependsOn) > 0 {
					fmt.Printf("  - %s (depends on: %s)\n", t, strings.Join(node.DependsOn, ", "))
				} else {
					fmt.Printf("  - %s\n", t)
				}
			} else {
				fmt.Printf("  - %s\n", t)
			}
		}
		return nil
	}

	// Upsert suite
	syncResp, err := apiClient.UpsertSuite(&client.SyncSuiteRequest{
		FolderPath: absPath,
		SuiteName:  suiteConfig.Suite.Name,
		Mode:       mode,
		TestCount:  len(tests),
	})
	if err != nil {
		return fmt.Errorf("failed to sync suite %q at %s: %w", suiteConfig.Suite.Name, absPath, err)
	}

	suiteID := syncResp.ID
	if suiteID == 0 {
		return fmt.Errorf("failed to get suite ID after sync: server returned suite %q (path %s) with id 0", syncResp.SuiteName, syncResp.FolderPath)
	}

	// Build trigger request
	triggerReq := &client.TriggerRunRequest{}
	if len(ucFilter) > 0 {
		triggerReq.UC = strings.Join(ucFilter, ",")
	}
	if len(tcFilter) > 0 {
		triggerReq.TC = strings.Join(tcFilter, ",")
	}
	if tcFile != "" {
		// Read test IDs from file for test_ids field
		content, err := os.ReadFile(tcFile)
		if err == nil {
			for _, line := range strings.Split(strings.TrimSpace(string(content)), "\n") {
				line = strings.TrimSpace(line)
				if line != "" {
					triggerReq.TestIDs = append(triggerReq.TestIDs, line)
				}
			}
		}
	}

	// Trigger run via API
	triggerResp, err := apiClient.TriggerRun(suiteID, triggerReq)
	if err != nil {
		return fmt.Errorf("failed to trigger run: %w", err)
	}

	runID := triggerResp.RunID

	// Print header
	if suiteConfig.SSH.Host != "" && suiteConfig.Standalone.Type == "remote" {
		fmt.Printf("Suite: %s (mode: %s, host: %s)\n", suiteConfig.Suite.Name, modeDisplay, suiteConfig.SSH.Host)
	} else {
		fmt.Printf("Suite: %s (mode: %s)\n", suiteConfig.Suite.Name, modeDisplay)
	}
	fmt.Printf("Run ID: %s\n", runID[:min(12, len(runID))])
	fmt.Println("Watching progress...")
	fmt.Println()

	// Set up signal handling.
	// SIGINT (interactive Ctrl-C): gracefully request cancellation; the driver
	// tears workers down and finalizes the run.
	// SIGTERM (watcher killed by a supervisor/harness): just detach and exit,
	// leaving the run to continue in the background.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		if sig == syscall.SIGINT {
			fmt.Println("\nCancelling run (graceful)...")
			apiClient.RequestCancel(runID)
		} else {
			fmt.Println("\nDetaching from run (still running in background)...")
		}
		os.Exit(0)
	}()

	// Poll for progress
	return watchProgress(apiClient, runID)
}

func watchProgress(apiClient *client.Client, runID string) error {
	printedTests := make(map[string]bool)
	startTime := time.Now()

	for {
		runData, err := apiClient.GetRunWithTests(runID)
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}

		// Print newly completed tests
		for _, test := range runData.Tests {
			if printedTests[test.TestID] {
				continue
			}
			// Only print completed tests (not pending/running)
			if test.Status == "pending" || test.Status == "running" {
				continue
			}

			printedTests[test.TestID] = true
			duration := ""
			if test.DurationMS != nil {
				duration = fmt.Sprintf(" (%.1fs)", float64(*test.DurationMS)/1000)
			}

			switch test.Status {
			case "passed":
				fmt.Printf("[PASS] %s%s\n", test.TestID, duration)
			case "failed", "crashed":
				errMsg := ""
				if test.ErrorMessage != "" {
					errMsg = " - " + test.ErrorMessage
				}
				fmt.Printf("[FAIL] %s%s%s\n", test.TestID, errMsg, duration)
			case "skipped":
				fmt.Printf("[SKIP] %s\n", test.TestID)
			}
		}

		// Check if run is finished
		if runData.Status == "completed" || runData.Status == "cancelled" || runData.Status == "failed" {
			duration := time.Since(startTime)
			fmt.Println("\n" + strings.Repeat("=", 60))
			if runData.Status == "cancelled" {
				fmt.Printf("CANCELLED: %d passed, %d failed, %d skipped (%.1fs)\n",
					runData.Passed, runData.Failed, runData.Skipped, duration.Seconds())
			} else if runData.Skipped > 0 {
				fmt.Printf("SUMMARY: %d passed, %d failed, %d skipped (%.1fs)\n",
					runData.Passed, runData.Failed, runData.Skipped, duration.Seconds())
			} else {
				fmt.Printf("SUMMARY: %d passed, %d failed (%.1fs)\n",
					runData.Passed, runData.Failed, duration.Seconds())
			}

			// Print failed tests
			var failedTests []string
			for _, test := range runData.Tests {
				if test.Status == "failed" || test.Status == "crashed" {
					failedTests = append(failedTests, test.TestID)
				}
			}
			if len(failedTests) > 0 {
				fmt.Println("\nFailed tests:")
				for _, t := range failedTests {
					fmt.Printf("  ✗ %s\n", t)
				}
			}
			fmt.Println(strings.Repeat("=", 60))

			if runData.Failed > 0 {
				return fmt.Errorf("%d test(s) failed", runData.Failed)
			}
			return nil
		}

		time.Sleep(2 * time.Second)
	}
}

func listTests(cmd *cobra.Command, args []string) error {
	absPath, err := filepath.Abs(suitePath)
	if err != nil {
		return fmt.Errorf("failed to resolve suite path: %w", err)
	}

	allTests, disabledSet, err := runner.ListTestsWithDisabled(absPath)
	if err != nil {
		return fmt.Errorf("failed to list tests: %w", err)
	}

	tests := filterTests(allTests)

	if len(tests) == 0 {
		fmt.Println("No tests found")
		return nil
	}

	disabledCount := 0
	for _, t := range tests {
		if disabledSet[t] {
			disabledCount++
		}
	}

	if disabledCount > 0 {
		fmt.Printf("Found %d test(s) (%d disabled):\n", len(tests), disabledCount)
	} else {
		fmt.Printf("Found %d test(s):\n", len(tests))
	}
	for _, t := range tests {
		if disabledSet[t] {
			fmt.Printf("  - %s (disabled)\n", t)
		} else {
			fmt.Printf("  - %s\n", t)
		}
	}

	return nil
}

func filterTests(tests []string) []string {
	// If tc-file is specified, filter by exact test ID match
	if tcFile != "" {
		content, err := os.ReadFile(tcFile)
		if err != nil {
			fmt.Printf("Warning: Failed to read tc-file: %v\n", err)
			return tests
		}
		allowedIDs := make(map[string]bool)
		for _, line := range strings.Split(strings.TrimSpace(string(content)), "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				allowedIDs[line] = true
			}
		}
		var filtered []string
		for _, testID := range tests {
			if allowedIDs[testID] {
				filtered = append(filtered, testID)
			}
		}
		return filtered
	}

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

		filtered = append(filtered, testID)
	}

	return filtered
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
	clearRuns, _ := cmd.Flags().GetBool("runs")
	force, _ := cmd.Flags().GetBool("force")

	if !clearAll && !clearRuns {
		fmt.Println("Use --runs or --all to clear data")
		fmt.Println("  tsuite clear --runs          Clear run history only (preserves suites & secrets)")
		fmt.Println("  tsuite clear --all           Clear everything (database, logs, reports)")
		return nil
	}

	if clearAll && clearRuns {
		return fmt.Errorf("--all and --runs are mutually exclusive")
	}

	tsuiteDir := getTsuiteHome()
	if _, err := os.Stat(tsuiteDir); os.IsNotExist(err) {
		fmt.Println("Nothing to clear (~/.tsuite does not exist)")
		return nil
	}

	// --runs path: delete run history but keep suites and secrets
	if clearRuns {
		if !force {
			fmt.Print("Delete all test run history (keeping suites and secrets)? This cannot be undone. [y/N]: ")
			var response string
			fmt.Scanln(&response)
			if strings.ToLower(response) != "y" && strings.ToLower(response) != "yes" {
				fmt.Println("Aborted.")
				return nil
			}
		}

		dbPath := filepath.Join(tsuiteDir, "results.db")
		if _, err := os.Stat(dbPath); os.IsNotExist(err) {
			fmt.Println("No database found, nothing to clear")
			return nil
		}

		repo, err := db.NewRepository()
		if err != nil {
			return fmt.Errorf("opening database: %w", err)
		}

		if err := repo.DeleteAllRuns(); err != nil {
			return fmt.Errorf("deleting runs: %w", err)
		}

		var cleared []string
		cleared = append(cleared, "run history from database")

		// Also clear per-run files (runs/ directory and reports/)
		runsDir := filepath.Join(tsuiteDir, "runs")
		if _, err := os.Stat(runsDir); err == nil {
			if err := os.RemoveAll(runsDir); err == nil {
				cleared = append(cleared, "runs/")
			}
		}
		reportsDir := filepath.Join(tsuiteDir, "reports")
		if _, err := os.Stat(reportsDir); err == nil {
			if err := os.RemoveAll(reportsDir); err == nil {
				cleared = append(cleared, "reports/")
			}
		}

		fmt.Printf("Cleared: %s\n", strings.Join(cleared, ", "))
		fmt.Println("Preserved: suites, secrets")
		return nil
	}

	// --all path: original behavior
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
