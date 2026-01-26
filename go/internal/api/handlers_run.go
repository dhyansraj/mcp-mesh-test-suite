package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/models"
)

// ==================== Runs ====================

// listRuns handles GET /api/runs
func (s *Server) listRuns(c *gin.Context) {
	limit := 20
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
			if limit > 100 {
				limit = 100
			}
		}
	}

	var suiteID *int64
	if sid := c.Query("suite_id"); sid != "" {
		if parsed, err := strconv.ParseInt(sid, 10, 64); err == nil {
			suiteID = &parsed
		}
	}

	runs, err := s.repo.GetAllRuns(suiteID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Ensure runs is an empty array, not null, for JSON serialization
	if runs == nil {
		runs = []models.Run{}
	}

	c.JSON(http.StatusOK, gin.H{
		"runs":  runs,
		"count": len(runs),
		"limit": limit,
	})
}

// getLatestRun handles GET /api/runs/latest
func (s *Server) getLatestRun(c *gin.Context) {
	run, err := s.repo.GetLatestRun()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if run == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "No runs found"})
		return
	}

	c.JSON(http.StatusOK, run)
}

// getRun handles GET /api/runs/:run_id
func (s *Server) getRun(c *gin.Context) {
	run, ok := s.getRunByIDParam(c)
	if !ok {
		return
	}

	// Get test results
	tests, err := s.repo.GetTestResultsByRunID(run.RunID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Build response matching Python's RunSummary
	c.JSON(http.StatusOK, gin.H{
		"run_id":                 run.RunID,
		"suite_id":               nullInt64Value(run.SuiteID),
		"suite_name":             nullStringValue(run.SuiteName),
		"display_name":           nullStringValue(run.DisplayName),
		"started_at":             run.StartedAt,
		"finished_at":            run.FinishedAt,
		"status":                 run.Status,
		"cli_version":            nullStringValue(run.CLIVersion),
		"sdk_python_version":     nullStringValue(run.SDKPythonVersion),
		"sdk_typescript_version": nullStringValue(run.SDKTypescriptVersion),
		"docker_image":           nullStringValue(run.DockerImage),
		"total_tests":            run.TotalTests,
		"pending_count":          run.PendingCount,
		"running_count":          run.RunningCount,
		"passed":                 run.Passed,
		"failed":                 run.Failed,
		"skipped":                run.Skipped,
		"duration_ms":            nullInt64Value(run.DurationMS),
		"mode":                   run.Mode,
		"cancel_requested":       run.CancelRequested,
		"tests":                  tests,
	})
}

// getRunTests handles GET /api/runs/:run_id/tests
func (s *Server) getRunTests(c *gin.Context) {
	run, ok := s.getRunByIDParam(c)
	if !ok {
		return
	}

	tests, err := s.repo.GetTestResultsByRunID(run.RunID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Ensure tests is an empty array, not null
	if tests == nil {
		tests = []models.TestResult{}
	}

	// Optional status filter
	if statusFilter := c.Query("status"); statusFilter != "" {
		filtered := make([]models.TestResult, 0)
		for _, t := range tests {
			if string(t.Status) == statusFilter {
				filtered = append(filtered, t)
			}
		}
		tests = filtered
	}

	c.JSON(http.StatusOK, gin.H{
		"run_id": run.RunID,
		"tests":  tests,
		"count":  len(tests),
	})
}

// getRunTestsTree handles GET /api/runs/:run_id/tests/tree
func (s *Server) getRunTestsTree(c *gin.Context) {
	run, ok := s.getRunByIDParam(c)
	if !ok {
		return
	}

	tests, err := s.repo.GetTestResultsByRunID(run.RunID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Group by use case
	useCaseMap := make(map[string]*useCaseGroup)

	for _, t := range tests {
		uc := t.UseCase
		if _, ok := useCaseMap[uc]; !ok {
			useCaseMap[uc] = &useCaseGroup{
				UseCase: uc,
				Tests:   []models.TestResult{},
			}
		}
		group := useCaseMap[uc]
		group.Tests = append(group.Tests, t)
		group.Total++

		switch t.Status {
		case models.TestStatusPending:
			group.Pending++
		case models.TestStatusRunning:
			group.Running++
		case models.TestStatusPassed:
			group.Passed++
		case models.TestStatusFailed:
			group.Failed++
		case models.TestStatusCrashed:
			group.Crashed++
		case models.TestStatusSkipped:
			group.Skipped++
		}
	}

	// Convert to slice and sort alphabetically by use case name
	useCases := make([]useCaseGroup, 0, len(useCaseMap))
	for _, uc := range useCaseMap {
		// Sort tests within use case: running tests by started_at (oldest first), then by test_case
		sort.Slice(uc.Tests, func(i, j int) bool {
			ti, tj := uc.Tests[i], uc.Tests[j]
			// Running tests sorted by started_at (oldest = longest running first)
			if ti.Status == models.TestStatusRunning && tj.Status == models.TestStatusRunning {
				if ti.StartedAt != nil && tj.StartedAt != nil {
					return ti.StartedAt.Before(*tj.StartedAt)
				}
			}
			// Running tests come before other statuses
			if ti.Status == models.TestStatusRunning && tj.Status != models.TestStatusRunning {
				return true
			}
			if ti.Status != models.TestStatusRunning && tj.Status == models.TestStatusRunning {
				return false
			}
			// Otherwise sort by test_case alphabetically
			return ti.TestCase < tj.TestCase
		})
		useCases = append(useCases, *uc)
	}

	// Sort use cases alphabetically
	sort.Slice(useCases, func(i, j int) bool {
		return useCases[i].UseCase < useCases[j].UseCase
	})

	c.JSON(http.StatusOK, gin.H{
		"run_id":    run.RunID,
		"run":       run,
		"use_cases": useCases,
	})
}

// createRun handles POST /api/runs
func (s *Server) createRun(c *gin.Context) {
	var req struct {
		SuiteID              int64    `json:"suite_id"`
		SuiteName            string   `json:"suite_name"`
		DisplayName          string   `json:"display_name"`
		CLIVersion           string   `json:"cli_version"`
		SDKPythonVersion     string   `json:"sdk_python_version"`
		SDKTypescriptVersion string   `json:"sdk_typescript_version"`
		DockerImage          string   `json:"docker_image"`
		TotalTests           int      `json:"total_tests"`
		Mode                 string   `json:"mode"`
		Tests                []struct {
			TestID   string   `json:"test_id"`
			UseCase  string   `json:"use_case"`
			TestCase string   `json:"test_case"`
			Name     string   `json:"name"`
			Tags     []string `json:"tags"`
		} `json:"tests"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body: " + err.Error()})
		return
	}

	// Generate run ID
	runID := generateUUID()

	// Create run
	run := &models.Run{
		RunID:                runID,
		SuiteID:              sql.NullInt64{Int64: req.SuiteID, Valid: req.SuiteID > 0},
		SuiteName:            sql.NullString{String: req.SuiteName, Valid: req.SuiteName != ""},
		DisplayName:          sql.NullString{String: req.DisplayName, Valid: req.DisplayName != ""},
		StartedAt:            time.Now(),
		Status:               models.RunStatusRunning,
		CLIVersion:           sql.NullString{String: req.CLIVersion, Valid: req.CLIVersion != ""},
		SDKPythonVersion:     sql.NullString{String: req.SDKPythonVersion, Valid: req.SDKPythonVersion != ""},
		SDKTypescriptVersion: sql.NullString{String: req.SDKTypescriptVersion, Valid: req.SDKTypescriptVersion != ""},
		DockerImage:          sql.NullString{String: req.DockerImage, Valid: req.DockerImage != ""},
		TotalTests:           req.TotalTests,
		PendingCount:         req.TotalTests,
		Mode:                 req.Mode,
	}

	if err := s.repo.CreateRun(run); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create run: " + err.Error()})
		return
	}

	// Create test result records if provided
	for _, t := range req.Tests {
		tagsJSON, _ := json.Marshal(t.Tags)
		tr := &models.TestResult{
			RunID:    runID,
			TestID:   t.TestID,
			UseCase:  t.UseCase,
			TestCase: t.TestCase,
			Name:     sql.NullString{String: t.Name, Valid: t.Name != ""},
			Tags:     sql.NullString{String: string(tagsJSON), Valid: true},
			Status:   models.TestStatusPending,
		}
		if err := s.repo.CreateTestResult(tr); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create test result: " + err.Error()})
			return
		}
	}

	// Emit SSE run_started event
	s.sseHub.EmitRunStarted(runID, run.TotalTests)

	c.JSON(http.StatusCreated, gin.H{
		"run_id":      runID,
		"status":      run.Status,
		"total_tests": run.TotalTests,
		"started_at":  run.StartedAt.Format(time.RFC3339),
	})
}

// completeRun handles POST /api/runs/:run_id/complete
func (s *Server) completeRun(c *gin.Context) {
	run, ok := s.getRunByIDParam(c)
	if !ok {
		return
	}

	if err := s.repo.CompleteRun(run.RunID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to complete run: " + err.Error()})
		return
	}

	// Get updated run
	run, _ = s.repo.GetRunByID(run.RunID)

	// Emit SSE run_completed event
	durationMS := int64(0)
	if run.DurationMS.Valid {
		durationMS = run.DurationMS.Int64
	}
	s.sseHub.EmitRunCompleted(run.RunID, run.Passed, run.Failed, run.Skipped, durationMS)

	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"run_id":      run.RunID,
		"status":      run.Status,
		"passed":      run.Passed,
		"failed":      run.Failed,
		"duration_ms": nullInt64Value(run.DurationMS),
	})
}

// rerunTests handles POST /api/runs/:run_id/rerun
// Like Python, this spawns CLI subprocess to actually run tests
func (s *Server) rerunTests(c *gin.Context) {
	run, ok := s.getRunByIDParam(c)
	if !ok {
		return
	}

	if !run.SuiteID.Valid {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot rerun: no suite_id associated with this run"})
		return
	}

	suite, err := s.repo.GetSuiteByID(run.SuiteID.Int64)
	if err != nil || suite == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Suite not found"})
		return
	}

	// Check suite folder exists
	if _, err := os.Stat(suite.FolderPath); os.IsNotExist(err) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Suite directory not found: " + suite.FolderPath})
		return
	}

	// Get tests from original run to determine scope
	tests, err := s.repo.GetTestResultsByRunID(run.RunID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if len(tests) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No tests found in original run"})
		return
	}

	// Determine scope: single tc, single uc, or full suite
	testIDs := make([]string, len(tests))
	useCases := make(map[string]bool)
	for i, t := range tests {
		testIDs[i] = t.TestID
		useCases[t.UseCase] = true
	}

	var scopeType, scopeValue string
	if len(testIDs) == 1 {
		scopeType = "tc"
		scopeValue = testIDs[0]
	} else if len(useCases) == 1 {
		scopeType = "uc"
		for uc := range useCases {
			scopeValue = uc
		}
	} else {
		scopeType = "all"
	}

	// Find tsuite binary (same directory as running server)
	execPath, err := os.Executable()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Cannot find executable path"})
		return
	}
	execPath, _ = filepath.EvalSymlinks(execPath)

	// Build CLI command
	apiURL := fmt.Sprintf("http://%s", c.Request.Host)
	cmd := []string{
		execPath,
		"run",
		"--suite-path", suite.FolderPath,
		"--api-url", apiURL,
	}

	// Add scope flag
	switch scopeType {
	case "tc":
		cmd = append(cmd, "--tc", scopeValue)
	case "uc":
		cmd = append(cmd, "--uc", scopeValue)
	}

	// Create log file for output
	logFile, err := os.CreateTemp("", "tsuite_rerun_*.log")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create log file"})
		return
	}
	logPath := logFile.Name()

	// Start subprocess
	process := newExecCommand(cmd[0], cmd[1:]...)
	process.Stdout = logFile
	process.Stderr = logFile
	process.Dir = suite.FolderPath

	if err := process.Start(); err != nil {
		logFile.Close()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start CLI: " + err.Error()})
		return
	}

	// Close log file - subprocess has inherited the FD
	logFile.Close()

	// Don't wait for process - let it run in background
	go func() {
		process.Wait()
	}()

	// Build description
	var description string
	switch scopeType {
	case "tc":
		description = "Rerunning test case: " + scopeValue
	case "uc":
		description = "Rerunning use case: " + scopeValue
	default:
		description = "Rerunning all tests in: " + suite.SuiteName
	}

	c.JSON(http.StatusAccepted, gin.H{
		"started":         true,
		"pid":             process.Process.Pid,
		"description":     description,
		"mode":            suite.Mode,
		"log_file":        logPath,
		"original_run_id": run.RunID,
	})
}

// updateRunStatus handles PATCH /api/runs/:run_id
// Used by CLI to mark run as cancelled after terminating workers
func (s *Server) updateRunStatus(c *gin.Context) {
	runID := c.Param("run_id")

	var req struct {
		Status string `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	run, err := s.repo.GetRunByID(runID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if run == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Run not found"})
		return
	}

	// Only allow updating to 'cancelled' status
	if req.Status != "cancelled" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Only 'cancelled' status is supported via PATCH"})
		return
	}

	if err := s.repo.MarkRunCancelled(runID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to cancel run: " + err.Error()})
		return
	}

	// Get updated run
	run, _ = s.repo.GetRunByID(runID)

	// Emit SSE run_cancelled event
	durationMS := int64(0)
	if run.DurationMS.Valid {
		durationMS = run.DurationMS.Int64
	}
	s.sseHub.EmitRunCancelled(runID, run.Passed, run.Failed, run.Skipped, durationMS)

	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"run_id":      runID,
		"status":      run.Status,
		"passed":      run.Passed,
		"failed":      run.Failed,
		"skipped":     run.Skipped,
		"duration_ms": nullInt64Value(run.DurationMS),
	})
}

// cancelRun handles POST /api/runs/:run_id/cancel
func (s *Server) cancelRun(c *gin.Context) {
	runID := c.Param("run_id")

	run, err := s.repo.GetRunByID(runID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if run == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Run not found"})
		return
	}

	if run.Status != models.RunStatusPending && run.Status != models.RunStatusRunning {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot cancel run with status: " + string(run.Status)})
		return
	}

	if err := s.repo.SetCancelRequested(runID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Emit SSE cancel_requested event
	s.sseHub.EmitCancelRequested(runID)

	c.JSON(http.StatusOK, gin.H{
		"success":          true,
		"run_id":           runID,
		"cancel_requested": true,
	})
}

// deleteRun handles DELETE /api/runs/:run_id
func (s *Server) deleteRun(c *gin.Context) {
	runID := c.Param("run_id")

	run, err := s.repo.GetRunByID(runID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if run == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Run not found"})
		return
	}

	if err := s.repo.DeleteRun(runID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete run: " + err.Error()})
		return
	}

	// Emit SSE event to notify clients
	s.sseHub.Emit(&SSEEvent{
		Type: "run_deleted",
		Data: map[string]any{
			"run_id": runID,
		},
	}, runID)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"run_id":  runID,
		"deleted": true,
	})
}
