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
	RunID                string     `json:"run_id,omitempty"`
	SuiteID              int64      `json:"suite_id"`
	SuiteName            string     `json:"suite_name"`
	DisplayName          string     `json:"display_name"`
	Filters              string     `json:"filters,omitempty"`
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
	PodName      string `json:"pod_name,omitempty"`
	NodeName     string `json:"node_name,omitempty"`
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

// UpdateTestMeta updates pod/node/image metadata for a test
func (c *Client) UpdateTestMeta(runID, testID, podName, nodeName, imageID string) error {
	payload := map[string]string{
		"pod_name":  podName,
		"node_name": nodeName,
	}
	if imageID != "" {
		payload["image_id"] = imageID
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPatch, c.baseURL+"/api/runs/"+runID+"/test-meta/"+testID, bytes.NewReader(body))
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
		return fmt.Errorf("failed to update test meta: %s - %s", resp.Status, string(bodyBytes))
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

// FinalizeCancelled marks an already-torn-down run as cancelled (FINALIZE path).
// Called by the driver AFTER workers have been torn down: it immediately sets
// status='cancelled', finished_at, and marks pending/running tests as skipped.
func (c *Client) FinalizeCancelled(runID string) error {
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
		return fmt.Errorf("failed to finalize run as cancelled: %s - %s", resp.Status, string(bodyBytes))
	}

	return nil
}

// RequestCancel requests graceful cancellation of a run (GRACEFUL path).
// It sets cancel_requested only; the driver's CancelChecker observes the flag,
// tears workers down, and finalizes the run itself.
func (c *Client) RequestCancel(runID string) error {
	resp, err := c.httpClient.Post(c.baseURL+"/api/runs/"+runID+"/cancel", "application/json", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to request run cancellation: %s - %s", resp.Status, string(bodyBytes))
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

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to list suites: %s - %s", resp.Status, string(bodyBytes))
	}

	var listResult struct {
		Suites []struct {
			ID         int64  `json:"id"`
			FolderPath string `json:"folder_path"`
			SuiteName  string `json:"suite_name"`
		} `json:"suites"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&listResult); err != nil {
		return nil, fmt.Errorf("failed to decode suite list: %w", err)
	}

	// Check if suite exists
	for _, suite := range listResult.Suites {
		if suite.FolderPath == req.FolderPath {
			// Suite exists, return it
			if suite.ID == 0 {
				return nil, fmt.Errorf("suite %q at %s exists but server returned id 0", suite.SuiteName, suite.FolderPath)
			}
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

	createBody, err := io.ReadAll(createResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read create suite response: %w", err)
	}

	switch {
	case createResp.StatusCode >= 200 && createResp.StatusCode < 300:
		var result SyncSuiteResponse
		if err := json.Unmarshal(createBody, &result); err != nil {
			return nil, fmt.Errorf("failed to decode create suite response: %w - %s", err, string(createBody))
		}
		if result.ID == 0 {
			return nil, fmt.Errorf("create suite returned %s without a suite id - %s", createResp.Status, string(createBody))
		}
		return &result, nil

	case createResp.StatusCode == http.StatusConflict:
		// Server replies {"error": "Suite already exists", "suite": {...}} - an
		// existing suite is a successful upsert, so return its ID.
		var conflict struct {
			Error string             `json:"error"`
			Suite *SyncSuiteResponse `json:"suite"`
		}
		if err := json.Unmarshal(createBody, &conflict); err != nil {
			return nil, fmt.Errorf("failed to decode create suite conflict response: %w - %s", err, string(createBody))
		}
		if conflict.Suite == nil || conflict.Suite.ID == 0 {
			return nil, fmt.Errorf("create suite returned %s without a suite id - %s", createResp.Status, string(createBody))
		}
		return conflict.Suite, nil

	default:
		var errResult struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(createBody, &errResult); err == nil && errResult.Error != "" {
			return nil, fmt.Errorf("failed to create suite: %s - %s", createResp.Status, errResult.Error)
		}
		return nil, fmt.Errorf("failed to create suite: %s - %s", createResp.Status, string(createBody))
	}
}

// TriggerRunRequest contains parameters for triggering a run via the API
type TriggerRunRequest struct {
	UC      string   `json:"uc,omitempty"`
	TC      string   `json:"tc,omitempty"`
	TestIDs []string `json:"test_ids,omitempty"`
}

// TriggerRunResponse is the response from triggering a run
type TriggerRunResponse struct {
	Started     bool   `json:"started"`
	RunID       string `json:"run_id"`
	PID         int    `json:"pid"`
	Description string `json:"description"`
	LogFile     string `json:"log_file"`
}

// TriggerRun starts a test run via the API
func (c *Client) TriggerRun(suiteID int64, req *TriggerRunRequest) (*TriggerRunResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Post(
		fmt.Sprintf("%s/api/suites/%d/run", c.baseURL, suiteID),
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to trigger run: %s - %s", resp.Status, string(bodyBytes))
	}

	var result TriggerRunResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

// RunWithTestsResponse contains run info and test results for progress display
type RunWithTestsResponse struct {
	RunID   string `json:"run_id"`
	Status  string `json:"status"`
	Passed  int    `json:"passed"`
	Failed  int    `json:"failed"`
	Skipped int    `json:"skipped"`
	Tests   []struct {
		TestID       string `json:"test_id"`
		Status       string `json:"status"`
		DurationMS   *int64 `json:"duration_ms"`
		ErrorMessage string `json:"error_message"`
	} `json:"tests"`
}

// GetRunWithTests gets run details including test results
func (c *Client) GetRunWithTests(runID string) (*RunWithTestsResponse, error) {
	resp, err := c.httpClient.Get(c.baseURL + "/api/runs/" + runID)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get run: %s - %s", resp.Status, string(bodyBytes))
	}

	var result RunWithTestsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

// GetSecretValues fetches all secret values from the API server
func (c *Client) GetSecretValues() (map[string]string, error) {
	resp, err := c.httpClient.Get(c.baseURL + "/api/secrets/values")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get secret values: %s - %s", resp.Status, string(bodyBytes))
	}

	var result map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result, nil
}
