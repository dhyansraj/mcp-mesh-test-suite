// Package models contains data structures matching the Python tsuite models.
// These must remain compatible with the existing SQLite schema in ~/.tsuite/results.db
package models

import (
	"database/sql"
	"encoding/json"
	"time"
)

// RunStatus represents the status of a test run
type RunStatus string

const (
	RunStatusPending   RunStatus = "pending"
	RunStatusRunning   RunStatus = "running"
	RunStatusCompleted RunStatus = "completed"
	RunStatusFailed    RunStatus = "failed"
	RunStatusCancelled RunStatus = "cancelled"
)

// TestStatus represents the status of a test case
type TestStatus string

const (
	TestStatusPending TestStatus = "pending"
	TestStatusRunning TestStatus = "running"
	TestStatusPassed  TestStatus = "passed"
	TestStatusFailed  TestStatus = "failed"
	TestStatusCrashed TestStatus = "crashed"
	TestStatusSkipped TestStatus = "skipped"
)

// IsTerminal returns true if the status is a terminal state (test won't change further)
func (s TestStatus) IsTerminal() bool {
	return s == TestStatusPassed || s == TestStatusFailed || s == TestStatusCrashed || s == TestStatusSkipped
}

// StepStatus represents the status of a test step
type StepStatus string

const (
	StepStatusPending StepStatus = "pending"
	StepStatusRunning StepStatus = "running"
	StepStatusPassed  StepStatus = "passed"
	StepStatusFailed  StepStatus = "failed"
	StepStatusSkipped StepStatus = "skipped"
)

// SuiteMode represents the execution mode of a suite
type SuiteMode string

const (
	SuiteModeDocker     SuiteMode = "docker"
	SuiteModeStandalone SuiteMode = "standalone"
)

// Suite represents a registered test suite
type Suite struct {
	ID           int64          `json:"id"`
	FolderPath   string         `json:"folder_path"`
	SuiteName    string         `json:"suite_name"`
	Mode         SuiteMode      `json:"mode"`
	ConfigJSON   sql.NullString `json:"-"`
	Config       any            `json:"config,omitempty"`
	TestCount    int            `json:"test_count"`
	LastSyncedAt *time.Time     `json:"last_synced_at,omitempty"`
	CreatedAt    *time.Time     `json:"created_at,omitempty"`
	UpdatedAt    *time.Time     `json:"updated_at,omitempty"`
}

// MarshalJSON customizes JSON output
func (s Suite) MarshalJSON() ([]byte, error) {
	type Alias Suite

	var config any
	if s.ConfigJSON.Valid && s.ConfigJSON.String != "" {
		_ = json.Unmarshal([]byte(s.ConfigJSON.String), &config)
	}

	return json.Marshal(&struct {
		Alias
		Config any `json:"config,omitempty"`
	}{
		Alias:  Alias(s),
		Config: config,
	})
}

// Run represents a test run session
type Run struct {
	RunID                string         `json:"run_id"`
	SuiteID              sql.NullInt64  `json:"suite_id,omitempty"`
	SuiteName            sql.NullString `json:"suite_name,omitempty"`
	DisplayName          sql.NullString `json:"display_name,omitempty"`
	StartedAt            time.Time      `json:"started_at"`
	FinishedAt           *time.Time     `json:"finished_at,omitempty"`
	Status               RunStatus      `json:"status"`
	CLIVersion           sql.NullString `json:"cli_version,omitempty"`
	SDKPythonVersion     sql.NullString `json:"sdk_python_version,omitempty"`
	SDKTypescriptVersion sql.NullString `json:"sdk_typescript_version,omitempty"`
	DockerImage          sql.NullString `json:"docker_image,omitempty"`
	TotalTests           int            `json:"total_tests"`
	PendingCount         int            `json:"pending_count"`
	RunningCount         int            `json:"running_count"`
	Passed               int            `json:"passed"`
	Failed               int            `json:"failed"`
	Skipped              int            `json:"skipped"`
	DurationMS           sql.NullInt64  `json:"duration_ms,omitempty"`
	Filters              sql.NullString `json:"-"`
	FiltersJSON          any            `json:"filters,omitempty"`
	Mode                 string         `json:"mode"`
	CancelRequested      bool           `json:"cancel_requested"`
}

// MarshalJSON customizes JSON output for Run
func (r Run) MarshalJSON() ([]byte, error) {
	var filters any
	if r.Filters.Valid && r.Filters.String != "" {
		_ = json.Unmarshal([]byte(r.Filters.String), &filters)
	}

	return json.Marshal(map[string]any{
		"run_id":                 r.RunID,
		"suite_id":               nullInt64ToAny(r.SuiteID),
		"suite_name":             nullStringToAny(r.SuiteName),
		"display_name":           nullStringToAny(r.DisplayName),
		"started_at":             r.StartedAt.Format(time.RFC3339),
		"finished_at":            timeToAny(r.FinishedAt),
		"status":                 r.Status,
		"cli_version":            nullStringToAny(r.CLIVersion),
		"sdk_python_version":     nullStringToAny(r.SDKPythonVersion),
		"sdk_typescript_version": nullStringToAny(r.SDKTypescriptVersion),
		"docker_image":           nullStringToAny(r.DockerImage),
		"total_tests":            r.TotalTests,
		"pending_count":          r.PendingCount,
		"running_count":          r.RunningCount,
		"passed":                 r.Passed,
		"failed":                 r.Failed,
		"skipped":                r.Skipped,
		"duration_ms":            nullInt64ToAny(r.DurationMS),
		"filters":                filters,
		"mode":                   r.Mode,
		"cancel_requested":       r.CancelRequested,
	})
}

// TestResult represents a test case result
type TestResult struct {
	ID           int64          `json:"id"`
	RunID        string         `json:"run_id"`
	TestID       string         `json:"test_id"`
	UseCase      string         `json:"use_case"`
	TestCase     string         `json:"test_case"`
	Name         sql.NullString `json:"name,omitempty"`
	Tags         sql.NullString `json:"-"`
	TagsList     []string       `json:"tags"`
	Status       TestStatus     `json:"status"`
	StartedAt    *time.Time     `json:"started_at,omitempty"`
	FinishedAt   *time.Time     `json:"finished_at,omitempty"`
	DurationMS   sql.NullInt64  `json:"duration_ms,omitempty"`
	ErrorMessage sql.NullString `json:"error_message,omitempty"`
	ErrorStep    sql.NullInt64  `json:"error_step,omitempty"`
	SkipReason   sql.NullString `json:"skip_reason,omitempty"`
	StepsJSON    sql.NullString `json:"-"`
	Steps        any            `json:"steps,omitempty"`
	StepsPassed  int            `json:"steps_passed"`
	StepsFailed  int            `json:"steps_failed"`
}

// MarshalJSON customizes JSON output for TestResult
func (t TestResult) MarshalJSON() ([]byte, error) {
	var tags []string
	if t.Tags.Valid && t.Tags.String != "" {
		_ = json.Unmarshal([]byte(t.Tags.String), &tags)
	}

	var steps any
	if t.StepsJSON.Valid && t.StepsJSON.String != "" {
		_ = json.Unmarshal([]byte(t.StepsJSON.String), &steps)
	}

	return json.Marshal(map[string]any{
		"id":            t.ID,
		"run_id":        t.RunID,
		"test_id":       t.TestID,
		"use_case":      t.UseCase,
		"test_case":     t.TestCase,
		"name":          nullStringToAny(t.Name),
		"tags":          tags,
		"status":        t.Status,
		"started_at":    timeToAny(t.StartedAt),
		"finished_at":   timeToAny(t.FinishedAt),
		"duration_ms":   nullInt64ToAny(t.DurationMS),
		"error_message": nullStringToAny(t.ErrorMessage),
		"error_step":    nullInt64ToAny(t.ErrorStep),
		"skip_reason":   nullStringToAny(t.SkipReason),
		"steps":         steps,
		"steps_passed":  t.StepsPassed,
		"steps_failed":  t.StepsFailed,
	})
}

// StepResult represents a step execution result
type StepResult struct {
	ID           int64          `json:"id"`
	TestResultID int64          `json:"test_result_id"`
	StepIndex    int            `json:"step_index"`
	Phase        string         `json:"phase"`
	Handler      string         `json:"handler"`
	Description  sql.NullString `json:"description,omitempty"`
	Status       StepStatus     `json:"status"`
	StartedAt    *time.Time     `json:"started_at,omitempty"`
	FinishedAt   *time.Time     `json:"finished_at,omitempty"`
	DurationMS   sql.NullInt64  `json:"duration_ms,omitempty"`
	ExitCode     sql.NullInt64  `json:"exit_code,omitempty"`
	Stdout       sql.NullString `json:"stdout,omitempty"`
	Stderr       sql.NullString `json:"stderr,omitempty"`
	ErrorMessage sql.NullString `json:"error_message,omitempty"`
}

// MarshalJSON customizes JSON output for StepResult
func (s StepResult) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"id":             s.ID,
		"test_result_id": s.TestResultID,
		"step_index":     s.StepIndex,
		"phase":          s.Phase,
		"handler":        s.Handler,
		"description":    nullStringToAny(s.Description),
		"status":         s.Status,
		"started_at":     timeToAny(s.StartedAt),
		"finished_at":    timeToAny(s.FinishedAt),
		"duration_ms":    nullInt64ToAny(s.DurationMS),
		"exit_code":      nullInt64ToAny(s.ExitCode),
		"stdout":         nullStringToAny(s.Stdout),
		"stderr":         nullStringToAny(s.Stderr),
		"error_message":  nullStringToAny(s.ErrorMessage),
	})
}

// AssertionResult represents an assertion result
type AssertionResult struct {
	ID             int64          `json:"id"`
	TestResultID   int64          `json:"test_result_id"`
	AssertionIndex int            `json:"assertion_index"`
	Expression     string         `json:"expression"`
	Message        sql.NullString `json:"message,omitempty"`
	Passed         bool           `json:"passed"`
	ActualValue    sql.NullString `json:"actual_value,omitempty"`
	ExpectedValue  sql.NullString `json:"expected_value,omitempty"`
}

// MarshalJSON customizes JSON output for AssertionResult
func (a AssertionResult) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"id":              a.ID,
		"test_result_id":  a.TestResultID,
		"assertion_index": a.AssertionIndex,
		"expression":      a.Expression,
		"message":         nullStringToAny(a.Message),
		"passed":          a.Passed,
		"actual_value":    nullStringToAny(a.ActualValue),
		"expected_value":  nullStringToAny(a.ExpectedValue),
	})
}

// CapturedValue represents a captured value during execution
type CapturedValue struct {
	ID           int64          `json:"id"`
	TestResultID int64          `json:"test_result_id"`
	Key          string         `json:"key"`
	Value        sql.NullString `json:"value,omitempty"`
	CapturedAt   *time.Time     `json:"captured_at,omitempty"`
}

// Helper functions for JSON marshaling

func nullStringToAny(ns sql.NullString) any {
	if ns.Valid {
		return ns.String
	}
	return nil
}

func nullInt64ToAny(ni sql.NullInt64) any {
	if ni.Valid {
		return ni.Int64
	}
	return nil
}

func timeToAny(t *time.Time) any {
	if t != nil {
		return t.Format(time.RFC3339)
	}
	return nil
}
