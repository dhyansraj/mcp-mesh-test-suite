// Package main is the entrypoint for tsuite-runner, the test executor binary.
// This binary executes a single test and reports detailed results (steps, assertions,
// stdout/stderr) to the API server.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/client"
	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/runner"
)

var (
	version = "0.1.0-go"

	// Flags
	testYamlPath string
	suitePath    string
	apiURL       string
	runID        string
	testID       string
	workdir      string
	logDir       string
	jsonOutput   bool
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "tsuite-runner",
		Short: "Test executor for mcp-mesh test suite",
		Long: `tsuite-runner executes a single test and reports results to the API server.
It is typically invoked by the tsuite CLI, either directly or inside a container.`,
		Version: version,
		RunE:    runTest,
	}

	// Flags (can also be set via environment variables)
	rootCmd.Flags().StringVar(&testYamlPath, "test-yaml", "", "Path to test.yaml file")
	rootCmd.Flags().StringVar(&suitePath, "suite-path", "", "Path to test suite root")
	rootCmd.Flags().StringVar(&apiURL, "api-url", "", "API server URL (env: TSUITE_API)")
	rootCmd.Flags().StringVar(&runID, "run-id", "", "Run identifier (env: TSUITE_RUN_ID)")
	rootCmd.Flags().StringVar(&testID, "test-id", "", "Test identifier (env: TSUITE_TEST_ID)")
	rootCmd.Flags().StringVar(&workdir, "workdir", "", "Working directory for test execution")
	rootCmd.Flags().StringVar(&logDir, "log-dir", "", "Directory for worker.log and mcp-mesh logs (env: TSUITE_LOG_DIR)")
	rootCmd.Flags().BoolVar(&jsonOutput, "json", false, "Output result as JSON to stdout")

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runTest(cmd *cobra.Command, args []string) error {
	// Resolve configuration from flags and environment
	if apiURL == "" {
		apiURL = os.Getenv("TSUITE_API")
	}
	if runID == "" {
		runID = os.Getenv("TSUITE_RUN_ID")
	}
	if testID == "" {
		testID = os.Getenv("TSUITE_TEST_ID")
	}
	if logDir == "" {
		logDir = os.Getenv("TSUITE_LOG_DIR")
	}

	// Validate required parameters
	if suitePath == "" {
		return fmt.Errorf("--suite-path is required")
	}

	// If test-yaml is not provided, derive from test-id and suite-path
	if testYamlPath == "" {
		if testID == "" {
			return fmt.Errorf("--test-yaml or --test-id is required")
		}
		testYamlPath = filepath.Join(suitePath, "suites", testID, "test.yaml")
	}

	// If test-id not provided, derive from test-yaml path
	if testID == "" {
		// Extract test ID from path: .../suites/uc01/tc01/test.yaml -> uc01/tc01
		dir := filepath.Dir(testYamlPath)
		tcName := filepath.Base(dir)
		ucName := filepath.Base(filepath.Dir(dir))
		testID = ucName + "/" + tcName
	}

	// Resolve suite path
	absPath, err := filepath.Abs(suitePath)
	if err != nil {
		return fmt.Errorf("failed to resolve suite path: %w", err)
	}
	absPath, _ = filepath.EvalSymlinks(absPath)

	// Check test.yaml exists
	if _, err := os.Stat(testYamlPath); os.IsNotExist(err) {
		return fmt.Errorf("test.yaml not found: %s", testYamlPath)
	}

	// Setup worker logger if log-dir is specified
	var workerLog *WorkerLogger
	if logDir != "" {
		os.MkdirAll(logDir, 0755)
		workerLog, err = NewWorkerLogger(logDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to create worker log: %v\n", err)
		} else {
			defer workerLog.Close()
			workerLog.Log("=== Test Execution Started ===")
			workerLog.Log("Test ID: %s", testID)
			workerLog.Log("Suite Path: %s", absPath)
		}

		// Set MCP_MESH_LOG_DIR for mcp-mesh agents (standalone mode)
		// This ensures agent logs go to the same location as in docker mode
		mcpLogsDir := filepath.Join(logDir, "logs")
		os.MkdirAll(mcpLogsDir, 0755)
		os.Setenv("MCP_MESH_LOG_DIR", mcpLogsDir)
	}

	// Create API client if configured
	var apiClient *client.RunnerClient
	if apiURL != "" && runID != "" {
		apiClient = client.NewRunnerClient(apiURL, runID, testID)
	}

	// Report test is running
	if apiClient != nil {
		if err := apiClient.ReportTestRunning(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to report test running: %v\n", err)
		}
	}

	// Determine workdir
	if workdir == "" {
		workdir = filepath.Join(os.TempDir(), "tsuite_runner_"+strings.ReplaceAll(testID, "/", "_"))
		os.MkdirAll(workdir, 0755)
	}

	// Create test runner and execute
	testRunner, err := runner.NewTestRunner(absPath, apiURL, runID, workdir)
	if err != nil {
		if workerLog != nil {
			workerLog.Log("ERROR: Failed to create test runner: %v", err)
		}
		reportError(apiClient, err.Error())
		return err
	}

	result, err := testRunner.RunTest(testID)
	if err != nil {
		if workerLog != nil {
			workerLog.Log("ERROR: Test execution failed: %v", err)
		}
		reportError(apiClient, err.Error())
		return err
	}

	// Log result to worker log
	if workerLog != nil {
		workerLog.LogResult(result)
	}

	// Report result to API
	if apiClient != nil {
		if result.Passed {
			if err := apiClient.ReportTestPassed(result); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: Failed to report test passed: %v\n", err)
			}
		} else {
			if err := apiClient.ReportTestFailed(result); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: Failed to report test failed: %v\n", err)
			}
		}
	}

	// Output result
	if jsonOutput {
		// Convert to JSON output
		output := convertResultToJSON(result)
		jsonBytes, _ := json.MarshalIndent(output, "", "  ")
		fmt.Println(string(jsonBytes))
	} else {
		// Human-readable output
		if result.Passed {
			fmt.Printf("PASSED: %s (%.2fs)\n", testID, result.Duration.Seconds())
		} else {
			fmt.Printf("FAILED: %s - %s (%.2fs)\n", testID, result.Error, result.Duration.Seconds())
		}

		// Print step summary
		stepsPassed := 0
		stepsFailed := 0
		for _, step := range result.Steps {
			if step.Success {
				stepsPassed++
			} else {
				stepsFailed++
			}
		}
		fmt.Printf("Steps: %d passed, %d failed\n", stepsPassed, stepsFailed)

		// Print assertion summary
		assertionsPassed := 0
		assertionsFailed := 0
		for _, assertion := range result.Assertions {
			if assertion.Passed {
				assertionsPassed++
			} else {
				assertionsFailed++
			}
		}
		if len(result.Assertions) > 0 {
			fmt.Printf("Assertions: %d passed, %d failed\n", assertionsPassed, assertionsFailed)
		}
	}

	// Exit with appropriate code
	if result.Passed {
		return nil
	}
	os.Exit(1)
	return nil
}

func reportError(apiClient *client.RunnerClient, errMsg string) {
	if apiClient != nil {
		apiClient.ReportTestFailed(&runner.TestResult{
			TestID: testID,
			Passed: false,
			Error:  errMsg,
		})
	}
	fmt.Fprintf(os.Stderr, "Error: %s\n", errMsg)
}

// convertResultToJSON converts TestResult to a JSON-serializable map
func convertResultToJSON(result *runner.TestResult) map[string]any {
	steps := make([]map[string]any, len(result.Steps))
	for i, step := range result.Steps {
		steps[i] = map[string]any{
			"phase":     step.Phase,
			"index":     step.Index,
			"handler":   step.Handler,
			"name":      step.Name,
			"success":   step.Success,
			"exit_code": step.ExitCode,
			"stdout":    step.Stdout,
			"stderr":    step.Stderr,
			"error":     step.Error,
		}
	}

	assertions := make([]map[string]any, len(result.Assertions))
	for i, assertion := range result.Assertions {
		assertions[i] = map[string]any{
			"index":    assertion.Index,
			"expr":     assertion.Expr,
			"message":  assertion.Message,
			"passed":   assertion.Passed,
			"actual":   assertion.Actual,
			"expected": assertion.Expected,
		}
	}

	// Count steps
	stepsPassed := 0
	stepsFailed := 0
	for _, step := range result.Steps {
		if step.Success {
			stepsPassed++
		} else {
			stepsFailed++
		}
	}

	return map[string]any{
		"test_id":      result.TestID,
		"test_name":    result.TestName,
		"passed":       result.Passed,
		"error":        result.Error,
		"duration_ms":  result.Duration.Milliseconds(),
		"steps_passed": stepsPassed,
		"steps_failed": stepsFailed,
		"steps":        steps,
		"assertions":   assertions,
	}
}

// =============================================================================
// Worker Logger
// =============================================================================

// WorkerLogger writes execution trace to worker.log
type WorkerLogger struct {
	file   *os.File
	writer io.Writer
}

// NewWorkerLogger creates a new worker logger
func NewWorkerLogger(logDir string) (*WorkerLogger, error) {
	logPath := filepath.Join(logDir, "worker.log")
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}
	return &WorkerLogger{
		file:   file,
		writer: file,
	}, nil
}

// Close closes the log file
func (w *WorkerLogger) Close() error {
	if w.file != nil {
		return w.file.Close()
	}
	return nil
}

// Log writes a formatted message to the log
func (w *WorkerLogger) Log(format string, args ...any) {
	timestamp := time.Now().Format("2006-01-02 15:04:05.000")
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(w.writer, "[%s] %s\n", timestamp, msg)
}

// LogResult writes the test result to the log
func (w *WorkerLogger) LogResult(result *runner.TestResult) {
	w.Log("=== Test Result ===")
	if result.Passed {
		w.Log("Status: PASSED")
	} else {
		w.Log("Status: FAILED")
		if result.Error != "" {
			w.Log("Error: %s", result.Error)
		}
	}
	w.Log("Duration: %.2fs", result.Duration.Seconds())

	// Log steps
	if len(result.Steps) > 0 {
		w.Log("")
		w.Log("--- Steps ---")
		for _, step := range result.Steps {
			status := "✓"
			if !step.Success {
				status = "✗"
			}
			w.Log("[%s] %s: %s (%s)", status, step.Phase, step.Name, step.Handler)
			if step.Stdout != "" {
				w.Log("  stdout: %s", truncate(step.Stdout, 500))
			}
			if step.Stderr != "" {
				w.Log("  stderr: %s", truncate(step.Stderr, 500))
			}
			if step.Error != "" {
				w.Log("  error: %s", step.Error)
			}
		}
	}

	// Log assertions
	if len(result.Assertions) > 0 {
		w.Log("")
		w.Log("--- Assertions ---")
		for _, assertion := range result.Assertions {
			status := "✓"
			if !assertion.Passed {
				status = "✗"
			}
			w.Log("[%s] %s", status, assertion.Expr)
			if assertion.Message != "" {
				w.Log("  message: %s", assertion.Message)
			}
			if !assertion.Passed {
				w.Log("  actual: %s", assertion.Actual)
				w.Log("  expected: %s", assertion.Expected)
			}
		}
	}

	w.Log("")
	w.Log("=== Test Execution Completed ===")
}

// truncate truncates a string to maxLen characters
func truncate(s string, maxLen int) string {
	// Replace newlines with spaces for single-line output
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
