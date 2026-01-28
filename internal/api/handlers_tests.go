package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/models"
)

// ==================== Test Details ====================

// getTestDetailByNumericID handles GET /api/runs/:run_id/tests/:test_id (dashboard uses numeric DB ID)
func (s *Server) getTestDetailByNumericID(c *gin.Context) {
	runID := c.Param("run_id")
	testIDStr := c.Param("test_id")

	testID, err := strconv.ParseInt(testIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid test ID"})
		return
	}

	test, err := s.repo.GetTestResultByID(testID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if test == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Test not found"})
		return
	}

	// Verify test belongs to run
	if test.RunID != runID {
		c.JSON(http.StatusNotFound, gin.H{"error": "Test not in this run"})
		return
	}

	s.sendTestDetailResponse(c, test)
}

// getTestDetail handles GET /api/runs/:run_id/test/*test_id (CLI uses path-based test_id)
func (s *Server) getTestDetail(c *gin.Context) {
	runID := c.Param("run_id")
	testIDStr := c.Param("test_id")
	// Gin wildcard includes leading slash, strip it
	if len(testIDStr) > 0 && testIDStr[0] == '/' {
		testIDStr = testIDStr[1:]
	}

	// Look up by path-based test_id (e.g., "build/tc05_verify_artifacts")
	test, err := s.repo.GetTestResultByTestIDAndRunID(testIDStr, runID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if test == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Test not found"})
		return
	}

	s.sendTestDetailResponse(c, test)
}

// sendTestDetailResponse sends the test detail JSON response
func (s *Server) sendTestDetailResponse(c *gin.Context, test *models.TestResult) {
	// Get steps
	steps, err := s.repo.GetStepResultsByTestID(test.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Get assertions
	assertions, err := s.repo.GetAssertionsByTestID(test.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Get captured values
	captured, err := s.repo.GetCapturedValuesByTestID(test.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":            test.ID,
		"run_id":        test.RunID,
		"test_id":       test.TestID,
		"use_case":      test.UseCase,
		"test_case":     test.TestCase,
		"name":          nullStringValue(test.Name),
		"status":        test.Status,
		"started_at":    test.StartedAt,
		"finished_at":   test.FinishedAt,
		"duration_ms":   nullInt64Value(test.DurationMS),
		"error_message": nullStringValue(test.ErrorMessage),
		"error_step":    nullInt64Value(test.ErrorStep),
		"skip_reason":   nullStringValue(test.SkipReason),
		"steps_passed":  test.StepsPassed,
		"steps_failed":  test.StepsFailed,
		"steps":         steps,
		"assertions":    assertions,
		"captured":      captured,
	})
}

// ==================== Test Status Updates ====================

// StepReport represents a step result from the runner
// Supports both flat format (Go runner) and nested result format (Python runner)
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

// UnmarshalJSON handles both flat and nested result formats
func (sr *StepReport) UnmarshalJSON(data []byte) error {
	// First try to unmarshal into a map to check for nested "result"
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	// Extract top-level fields
	if v, ok := raw["phase"]; ok {
		json.Unmarshal(v, &sr.Phase)
	}
	if v, ok := raw["index"]; ok {
		json.Unmarshal(v, &sr.Index)
	}
	if v, ok := raw["handler"]; ok {
		json.Unmarshal(v, &sr.Handler)
	}
	if v, ok := raw["name"]; ok {
		json.Unmarshal(v, &sr.Name)
	}
	if v, ok := raw["duration_ms"]; ok {
		json.Unmarshal(v, &sr.DurationMS)
	}

	// Check if there's a nested "result" object (Python format)
	if resultRaw, ok := raw["result"]; ok {
		var result struct {
			Success  bool   `json:"success"`
			ExitCode int    `json:"exit_code"`
			Stdout   string `json:"stdout"`
			Stderr   string `json:"stderr"`
			Error    string `json:"error"`
		}
		if err := json.Unmarshal(resultRaw, &result); err == nil {
			sr.Success = result.Success
			sr.ExitCode = result.ExitCode
			sr.Stdout = result.Stdout
			sr.Stderr = result.Stderr
			sr.Error = result.Error
			return nil
		}
	}

	// Otherwise, extract flat fields (Go runner format)
	if v, ok := raw["success"]; ok {
		json.Unmarshal(v, &sr.Success)
	}
	if v, ok := raw["exit_code"]; ok {
		json.Unmarshal(v, &sr.ExitCode)
	}
	if v, ok := raw["stdout"]; ok {
		json.Unmarshal(v, &sr.Stdout)
	}
	if v, ok := raw["stderr"]; ok {
		json.Unmarshal(v, &sr.Stderr)
	}
	if v, ok := raw["error"]; ok {
		json.Unmarshal(v, &sr.Error)
	}

	return nil
}

// AssertionReport represents an assertion result from the runner
type AssertionReport struct {
	Index    int    `json:"index"`
	Expr     string `json:"expr"`
	Message  string `json:"message"`
	Passed   bool   `json:"passed"`
	Actual   string `json:"actual"`
	Expected string `json:"expected"`
}

// updateTestStatus handles PATCH /api/runs/:run_id/test/*test_id
func (s *Server) updateTestStatus(c *gin.Context) {
	runID := c.Param("run_id")
	testID := c.Param("test_id")
	// Gin wildcard includes leading slash, strip it
	if len(testID) > 0 && testID[0] == '/' {
		testID = testID[1:]
	}
	s.doUpdateTestStatus(c, runID, testID)
}

// doUpdateTestStatus is the shared implementation for updating test status
func (s *Server) doUpdateTestStatus(c *gin.Context, runID, testID string) {
	var req struct {
		Status       string            `json:"status"`
		DurationMS   *int64            `json:"duration_ms"`
		ErrorMessage string            `json:"error_message"`
		StepsPassed  *int              `json:"steps_passed"`
		StepsFailed  *int              `json:"steps_failed"`
		Steps        []StepReport      `json:"steps"`
		Assertions   []AssertionReport `json:"assertions"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	// Get test result
	tr, err := s.repo.GetTestResultByTestIDAndRunID(testID, runID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if tr == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Test not found"})
		return
	}

	// Capture old status for incremental counter update
	oldStatus := tr.Status

	// Idempotency check: ignore updates if test is already in a terminal state
	// This prevents race conditions in parallel execution
	if oldStatus.IsTerminal() && req.Status != "" {
		// Already in terminal state, return success but don't update
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"test_id": testID,
			"status":  tr.Status,
			"skipped": true,
			"reason":  "test already in terminal state",
		})
		return
	}

	// Update fields
	now := time.Now()
	newStatus := oldStatus
	if req.Status != "" {
		newStatus = models.TestStatus(req.Status)
		tr.Status = newStatus

		if req.Status == "running" {
			tr.StartedAt = &now
		} else if req.Status == "passed" || req.Status == "failed" || req.Status == "crashed" || req.Status == "skipped" {
			tr.FinishedAt = &now
		}
	}

	if req.DurationMS != nil {
		tr.DurationMS = sql.NullInt64{Int64: *req.DurationMS, Valid: true}
	}

	if req.ErrorMessage != "" {
		tr.ErrorMessage = sql.NullString{String: req.ErrorMessage, Valid: true}
	}

	if req.StepsPassed != nil {
		tr.StepsPassed = *req.StepsPassed
	}

	if req.StepsFailed != nil {
		tr.StepsFailed = *req.StepsFailed
	}

	// Store steps as JSON in steps_json column
	if len(req.Steps) > 0 {
		stepsJSON, err := json.Marshal(req.Steps)
		if err == nil {
			tr.StepsJSON = sql.NullString{String: string(stepsJSON), Valid: true}
		}
	}

	if err := s.repo.UpdateTestResult(tr); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update test: " + err.Error()})
		return
	}

	// Store step results in step_results table
	if len(req.Steps) > 0 {
		for _, step := range req.Steps {
			stepResult := &models.StepResult{
				TestResultID: tr.ID,
				StepIndex:    step.Index,
				Phase:        step.Phase,
				Handler:      step.Handler,
				Description:  sql.NullString{String: step.Name, Valid: step.Name != ""},
				ExitCode:     sql.NullInt64{Int64: int64(step.ExitCode), Valid: true},
				Stdout:       sql.NullString{String: step.Stdout, Valid: step.Stdout != ""},
				Stderr:       sql.NullString{String: step.Stderr, Valid: step.Stderr != ""},
				ErrorMessage: sql.NullString{String: step.Error, Valid: step.Error != ""},
				DurationMS:   sql.NullInt64{Int64: step.DurationMS, Valid: step.DurationMS > 0},
			}
			if step.Success {
				stepResult.Status = models.StepStatusPassed
			} else {
				stepResult.Status = models.StepStatusFailed
			}
			if err := s.repo.CreateStepResult(stepResult); err != nil {
				// Log error but continue (best effort)
				fmt.Printf("Warning: Failed to insert step result: %v\n", err)
			}
		}
	}

	// Store assertion results
	if len(req.Assertions) > 0 {
		for _, assertion := range req.Assertions {
			assertionResult := &models.AssertionResult{
				TestResultID:   tr.ID,
				AssertionIndex: assertion.Index,
				Expression:     assertion.Expr,
				Message:        sql.NullString{String: assertion.Message, Valid: assertion.Message != ""},
				Passed:         assertion.Passed,
				ActualValue:    sql.NullString{String: assertion.Actual, Valid: assertion.Actual != ""},
				ExpectedValue:  sql.NullString{String: assertion.Expected, Valid: assertion.Expected != ""},
			}
			// Ignore errors on assertion insertion (best effort)
			s.repo.CreateAssertionResult(assertionResult)
		}
	}

	// Update run counters incrementally (idempotent, avoids race conditions)
	if err := s.repo.UpdateRunCountersIncremental(runID, oldStatus, newStatus); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update run counters: " + err.Error()})
		return
	}

	// Emit SSE event for status change
	if req.Status == "running" {
		testName := ""
		if tr.Name.Valid {
			testName = tr.Name.String
		}
		s.sseHub.EmitTestStarted(runID, testID, testName)
	} else if req.Status == "passed" || req.Status == "failed" || req.Status == "crashed" || req.Status == "skipped" {
		durationMS := int64(0)
		if req.DurationMS != nil {
			durationMS = *req.DurationMS
		}
		stepsPassed := 0
		stepsFailed := 0
		if req.StepsPassed != nil {
			stepsPassed = *req.StepsPassed
		}
		if req.StepsFailed != nil {
			stepsFailed = *req.StepsFailed
		}
		s.sseHub.EmitTestCompleted(runID, testID, req.Status, durationMS, stepsPassed, stepsFailed)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"test_id": testID,
		"status":  tr.Status,
	})
}

// updateTestStatusByPath handles PATCH /api/runs/:run_id/tests/*test_id
// This is used by the Python runner which sends test_id as a path with slashes
func (s *Server) updateTestStatusByPath(c *gin.Context) {
	runID := c.Param("run_id")
	testID := c.Param("test_id")
	// Gin wildcard includes leading slash, strip it
	if len(testID) > 0 && testID[0] == '/' {
		testID = testID[1:]
	}
	s.doUpdateTestStatus(c, runID, testID)
}
