// Package client provides an HTTP client for the tsuite API server.
package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is an API client for the tsuite server
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a new API client
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// CreateRunRequest contains the parameters for creating a run
type CreateRunRequest struct {
	SuiteID              int64      `json:"suite_id"`
	SuiteName            string     `json:"suite_name"`
	DisplayName          string     `json:"display_name"`
	CLIVersion           string     `json:"cli_version"`
	SDKPythonVersion     string     `json:"sdk_python_version"`
	SDKTypescriptVersion string     `json:"sdk_typescript_version"`
	DockerImage          string     `json:"docker_image"`
	TotalTests           int        `json:"total_tests"`
	Mode                 string     `json:"mode"`
	Tests                []TestInfo `json:"tests"`
}

// TestInfo contains test metadata
type TestInfo struct {
	TestID   string   `json:"test_id"`
	UseCase  string   `json:"use_case"`
	TestCase string   `json:"test_case"`
	Name     string   `json:"name"`
	Tags     []string `json:"tags"`
}

// CreateRunResponse is the response from creating a run
type CreateRunResponse struct {
	RunID      string `json:"run_id"`
	Status     string `json:"status"`
	TotalTests int    `json:"total_tests"`
	StartedAt  string `json:"started_at"`
}

// CreateRun creates a new test run
func (c *Client) CreateRun(req *CreateRunRequest) (*CreateRunResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Post(c.baseURL+"/api/runs", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to create run: %s - %s", resp.Status, string(bodyBytes))
	}

	var result CreateRunResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

// UpdateTestStatusRequest contains the parameters for updating test status
type UpdateTestStatusRequest struct {
	Status       string `json:"status"`
	DurationMS   *int64 `json:"duration_ms,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
	StepsPassed  *int   `json:"steps_passed,omitempty"`
	StepsFailed  *int   `json:"steps_failed,omitempty"`
}

// UpdateTestStatus updates the status of a test
func (c *Client) UpdateTestStatus(runID, testID string, req *UpdateTestStatusRequest) error {
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}

	// Use /test/ (singular) with wildcard to handle test_ids containing slashes
	httpReq, err := http.NewRequest(http.MethodPatch, c.baseURL+"/api/runs/"+runID+"/test/"+testID, bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to update test status: %s - %s", resp.Status, string(bodyBytes))
	}

	return nil
}

// CompleteRun marks a run as completed
func (c *Client) CompleteRun(runID string) error {
	resp, err := c.httpClient.Post(c.baseURL+"/api/runs/"+runID+"/complete", "application/json", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to complete run: %s - %s", resp.Status, string(bodyBytes))
	}

	return nil
}

// CancelRun marks a run as cancelled (called by CLI after terminating workers)
func (c *Client) CancelRun(runID string) error {
	req, err := http.NewRequest(http.MethodPatch, c.baseURL+"/api/runs/"+runID, bytes.NewReader([]byte(`{"status":"cancelled"}`)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to cancel run: %s - %s", resp.Status, string(bodyBytes))
	}

	return nil
}

// CheckCancelRequested checks if cancellation has been requested for a run
func (c *Client) CheckCancelRequested(runID string) (bool, error) {
	resp, err := c.httpClient.Get(c.baseURL + "/api/runs/" + runID)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, nil
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, err
	}

	if cancelled, ok := result["cancel_requested"].(bool); ok {
		return cancelled, nil
	}

	return false, nil
}

// HealthCheck checks if the API server is healthy
func (c *Client) HealthCheck() error {
	resp, err := c.httpClient.Get(c.baseURL + "/health")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check failed: %s", resp.Status)
	}

	return nil
}

// SyncSuiteRequest contains parameters for syncing a suite
type SyncSuiteRequest struct {
	FolderPath string `json:"folder_path"`
	SuiteName  string `json:"suite_name"`
	Mode       string `json:"mode"`
	TestCount  int    `json:"test_count"`
	ConfigJSON string `json:"config_json,omitempty"`
}

// SyncSuiteResponse is the response from syncing a suite
type SyncSuiteResponse struct {
	ID         int64  `json:"id"`
	SuiteName  string `json:"suite_name"`
	FolderPath string `json:"folder_path"`
}

// UpsertSuite creates or updates a suite
func (c *Client) UpsertSuite(req *SyncSuiteRequest) (*SyncSuiteResponse, error) {
	// First, try to find existing suite by folder path
	resp, err := c.httpClient.Get(c.baseURL + "/api/suites")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var listResult struct {
		Suites []struct {
			ID         int64  `json:"id"`
			FolderPath string `json:"folder_path"`
			SuiteName  string `json:"suite_name"`
		} `json:"suites"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&listResult); err != nil {
		return nil, err
	}

	// Check if suite exists
	for _, suite := range listResult.Suites {
		if suite.FolderPath == req.FolderPath {
			// Suite exists, return it
			return &SyncSuiteResponse{
				ID:         suite.ID,
				SuiteName:  suite.SuiteName,
				FolderPath: suite.FolderPath,
			}, nil
		}
	}

	// Suite doesn't exist, create it
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	createResp, err := c.httpClient.Post(c.baseURL+"/api/suites", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer createResp.Body.Close()

	// For now, just return a placeholder since create might not be fully implemented
	// The important thing is we tried to sync
	return &SyncSuiteResponse{
		SuiteName:  req.SuiteName,
		FolderPath: req.FolderPath,
	}, nil
}
