// Package client provides HTTP clients for the tsuite API server.
package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/runner"
	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/textutil"
)

// RunnerClient is an API client specifically for the runner binary.
// It reports detailed test results including steps, assertions, and output.
type RunnerClient struct {
	baseURL    string
	runID      string
	testID     string
	httpClient *http.Client
}

// NewRunnerClient creates a new runner API client
func NewRunnerClient(baseURL, runID, testID string) *RunnerClient {
	return &RunnerClient{
		baseURL: baseURL,
		runID:   runID,
		testID:  testID,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// StepReport represents a step result for API reporting
type StepReport struct {
	Phase      string `json:"phase"`
	Index      int    `json:"index"`
	Handler    string `json:"handler"`
	Name       string `json:"name"`
	Success    bool   `json:"success"`
	ExitCode   int    `json:"exit_code"`
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	Error      string `json:"error,omitempty"`
	DurationMS int64  `json:"duration_ms,omitempty"`
}

// newStepReport converts a step result for transmission, capping stdout/stderr
// so a chatty step cannot write an unbounded blob into step_results. The
// runner's own result keeps the full output.
func newStepReport(result runner.StepResult) StepReport {
	return StepReport{
		Phase:    result.Phase,
		Index:    result.Index,
		Handler:  result.Handler,
		Name:     result.Name,
		Success:  result.Success,
		ExitCode: result.ExitCode,
		Stdout:   textutil.TruncateOutput(result.Stdout, textutil.MaxStepOutput),
		Stderr:   textutil.TruncateOutput(result.Stderr, textutil.MaxStepOutput),
		Error:    result.Error,
	}
}

// AssertionReport represents an assertion result for API reporting
type AssertionReport struct {
	Index    int    `json:"index"`
	Expr     string `json:"expr"`
	Message  string `json:"message"`
	Passed   bool   `json:"passed"`
	Actual   string `json:"actual"`
	Expected string `json:"expected"`
}

// TestStatusReport is the full request body for reporting test status
type TestStatusReport struct {
	Status       string            `json:"status"`
	DurationMS   *int64            `json:"duration_ms,omitempty"`
	ErrorMessage string            `json:"error_message,omitempty"`
	StepsPassed  *int              `json:"steps_passed,omitempty"`
	StepsFailed  *int              `json:"steps_failed,omitempty"`
	Steps        []StepReport      `json:"steps,omitempty"`
	Assertions   []AssertionReport `json:"assertions,omitempty"`
}

// ReportTestRunning reports that the test has started running
func (c *RunnerClient) ReportTestRunning() error {
	return c.sendStatusUpdate(&TestStatusReport{
		Status: "running",
	})
}

// ReportTestPassed reports that the test passed with full details
func (c *RunnerClient) ReportTestPassed(result *runner.TestResult) error {
	report := c.buildReport(result, "passed")
	return c.sendStatusUpdate(report)
}

// ReportTestFailed reports that the test failed with full details
func (c *RunnerClient) ReportTestFailed(result *runner.TestResult) error {
	report := c.buildReport(result, "failed")
	return c.sendStatusUpdate(report)
}

// buildReport converts a TestResult to a TestStatusReport
func (c *RunnerClient) buildReport(result *runner.TestResult, status string) *TestStatusReport {
	// Convert steps
	steps := make([]StepReport, len(result.Steps))
	stepsPassed := 0
	stepsFailed := 0
	for i, step := range result.Steps {
		steps[i] = newStepReport(step)
		if step.Success {
			stepsPassed++
		} else {
			stepsFailed++
		}
	}

	// Convert assertions
	assertions := make([]AssertionReport, len(result.Assertions))
	for i, assertion := range result.Assertions {
		assertions[i] = AssertionReport{
			Index:    assertion.Index,
			Expr:     assertion.Expr,
			Message:  assertion.Message,
			Passed:   assertion.Passed,
			Actual:   assertion.Actual,
			Expected: assertion.Expected,
		}
	}

	durationMS := result.Duration.Milliseconds()

	return &TestStatusReport{
		Status:       status,
		DurationMS:   &durationMS,
		ErrorMessage: result.Error,
		StepsPassed:  &stepsPassed,
		StepsFailed:  &stepsFailed,
		Steps:        steps,
		Assertions:   assertions,
	}
}

// sendStatusUpdate sends a status update to the API
func (c *RunnerClient) sendStatusUpdate(report *TestStatusReport) error {
	body, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("failed to marshal report: %w", err)
	}

	// Use /test/ (singular) with wildcard to handle test_ids containing slashes
	url := fmt.Sprintf("%s/api/runs/%s/test/%s", c.baseURL, c.runID, c.testID)
	req, err := http.NewRequest(http.MethodPatch, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error: %s - %s", resp.Status, string(bodyBytes))
	}

	return nil
}

// ReportStepStarted reports that a step has started executing
func (c *RunnerClient) ReportStepStarted(phase string, index int, name string, handler string) error {
	report := map[string]any{
		"phase":   phase,
		"index":   index,
		"name":    name,
		"handler": handler,
		"status":  "running",
	}
	body, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("failed to marshal step start: %w", err)
	}

	url := fmt.Sprintf("%s/api/runs/%s/test-steps/%s", c.baseURL, c.runID, c.testID)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error: %s - %s", resp.Status, string(bodyBytes))
	}

	return nil
}

// OnStepStarted implements runner.StepReporter by reporting step start to the API
func (c *RunnerClient) OnStepStarted(phase string, index int, name string, handler string) {
	if err := c.ReportStepStarted(phase, index, name, handler); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to report step started: %v\n", err)
	}
}

// ReportStepCompleted reports a single step result to the API server
func (c *RunnerClient) ReportStepCompleted(step StepReport) error {
	body, err := json.Marshal(step)
	if err != nil {
		return fmt.Errorf("failed to marshal step: %w", err)
	}

	url := fmt.Sprintf("%s/api/runs/%s/test-steps/%s", c.baseURL, c.runID, c.testID)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error: %s - %s", resp.Status, string(bodyBytes))
	}

	return nil
}

// OnStepCompleted implements runner.StepReporter by reporting to the API
func (c *RunnerClient) OnStepCompleted(result runner.StepResult) {
	if err := c.ReportStepCompleted(newStepReport(result)); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to report step completed: %v\n", err)
	}
}
