package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gopkg.in/yaml.v3"

	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/models"
)

// generateUUID creates a new UUID string
func generateUUID() string {
	return uuid.New().String()
}

// ==================== Suites ====================

// listSuites handles GET /api/suites
func (s *Server) listSuites(c *gin.Context) {
	suites, err := s.repo.GetAllSuites()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Ensure suites is an empty array, not null
	if suites == nil {
		suites = []models.Suite{}
	}

	c.JSON(http.StatusOK, gin.H{
		"suites": suites,
		"count":  len(suites),
	})
}

// getSuite handles GET /api/suites/:id
func (s *Server) getSuite(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid suite ID"})
		return
	}

	suite, err := s.repo.GetSuiteByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if suite == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Suite not found"})
		return
	}

	// Discover tests from filesystem
	tests, useCases, _ := DiscoverTests(suite.FolderPath)

	c.JSON(http.StatusOK, gin.H{
		"id":             suite.ID,
		"folder_path":    suite.FolderPath,
		"suite_name":     suite.SuiteName,
		"mode":           suite.Mode,
		"test_count":     suite.TestCount,
		"last_synced_at": suite.LastSyncedAt,
		"created_at":     suite.CreatedAt,
		"updated_at":     suite.UpdatedAt,
		"tests":          tests,
		"use_cases":      useCases,
	})
}

// createSuite handles POST /api/suites
func (s *Server) createSuite(c *gin.Context) {
	var req struct {
		FolderPath string `json:"folder_path"`
		Mode       string `json:"mode"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	if req.FolderPath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "folder_path is required"})
		return
	}

	// Normalize and expand path
	folderPath := req.FolderPath
	if len(folderPath) > 0 && folderPath[0] == '~' {
		if home, err := os.UserHomeDir(); err == nil {
			folderPath = filepath.Join(home, folderPath[1:])
		}
	}
	folderPath, _ = filepath.Abs(folderPath)

	// Check if directory exists
	info, err := os.Stat(folderPath)
	if os.IsNotExist(err) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Directory not found: " + folderPath})
		return
	}
	if !info.IsDir() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not a directory: " + folderPath})
		return
	}

	// Check for config.yaml
	configPath := filepath.Join(folderPath, "config.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No config.yaml found in " + folderPath})
		return
	}

	// Check if suite already exists
	existing, err := s.repo.GetSuiteByPath(folderPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if existing != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "Suite already exists", "suite": existing})
		return
	}

	// Parse config.yaml
	configData, err := os.ReadFile(configPath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read config: " + err.Error()})
		return
	}

	var config map[string]any
	if err := yaml.Unmarshal(configData, &config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to parse config: " + err.Error()})
		return
	}

	// Extract suite info from config
	suiteConfig, _ := config["suite"].(map[string]any)
	suiteName := filepath.Base(folderPath)
	mode := "docker"
	if suiteConfig != nil {
		if n, ok := suiteConfig["name"].(string); ok && n != "" {
			suiteName = n
		}
		if m, ok := suiteConfig["mode"].(string); ok && m != "" {
			mode = m
		}
	}

	// Override mode if provided in request
	if req.Mode != "" {
		if req.Mode != "docker" && req.Mode != "standalone" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid mode: " + req.Mode + ". Must be 'docker' or 'standalone'"})
			return
		}
		mode = req.Mode
	}

	// Discover tests using existing function
	tests, useCases, err := DiscoverTests(folderPath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to discover tests: " + err.Error()})
		return
	}

	// Marshal config to JSON
	configJSON, _ := json.Marshal(config)

	// Create suite
	now := time.Now()
	suite := &models.Suite{
		FolderPath:   folderPath,
		SuiteName:    suiteName,
		Mode:         models.SuiteMode(mode),
		ConfigJSON:   sql.NullString{String: string(configJSON), Valid: true},
		TestCount:    len(tests),
		LastSyncedAt: &now,
	}

	if err := s.repo.CreateSuite(suite); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":          suite.ID,
		"folder_path": suite.FolderPath,
		"suite_name":  suite.SuiteName,
		"mode":        suite.Mode,
		"test_count":  suite.TestCount,
		"tests":       tests,
		"use_cases":   useCases,
	})
}

// updateSuite handles PUT /api/suites/:id
func (s *Server) updateSuite(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid suite ID"})
		return
	}

	suite, err := s.repo.GetSuiteByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if suite == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Suite not found"})
		return
	}

	var req struct {
		SuiteName string `json:"suite_name"`
		Mode      string `json:"mode"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	if req.SuiteName != "" {
		suite.SuiteName = req.SuiteName
	}
	if req.Mode != "" {
		suite.Mode = models.SuiteMode(req.Mode)
	}

	if err := s.repo.UpdateSuite(suite); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, suite)
}

// deleteSuite handles DELETE /api/suites/:id
func (s *Server) deleteSuite(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid suite ID"})
		return
	}

	suite, err := s.repo.GetSuiteByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if suite == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Suite not found"})
		return
	}

	if err := s.repo.DeleteSuite(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"deleted": true, "id": id})
}

// syncSuite handles POST /api/suites/:id/sync
// Re-reads config.yaml and updates the cached config in the database
func (s *Server) syncSuite(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid suite ID"})
		return
	}

	suite, err := s.repo.GetSuiteByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if suite == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Suite not found"})
		return
	}

	// Check if directory still exists
	if _, err := os.Stat(suite.FolderPath); os.IsNotExist(err) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Directory not found: " + suite.FolderPath})
		return
	}

	// Parse config.yaml
	configPath := filepath.Join(suite.FolderPath, "config.yaml")
	configData, err := os.ReadFile(configPath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read config: " + err.Error()})
		return
	}

	var config map[string]any
	if err := yaml.Unmarshal(configData, &config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to parse config: " + err.Error()})
		return
	}

	// Extract suite info from config
	suiteConfig, _ := config["suite"].(map[string]any)
	suiteName := suite.SuiteName // Keep existing name as default
	mode := string(suite.Mode)   // Keep existing mode as default
	if suiteConfig != nil {
		if n, ok := suiteConfig["name"].(string); ok && n != "" {
			suiteName = n
		}
		if m, ok := suiteConfig["mode"].(string); ok && m != "" {
			mode = m
		}
	}

	// Discover tests
	tests, _, err := DiscoverTests(suite.FolderPath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to discover tests: " + err.Error()})
		return
	}

	// Marshal config to JSON
	configJSON, _ := json.Marshal(config)

	// Update suite in database
	now := time.Now()
	suite.SuiteName = suiteName
	suite.Mode = models.SuiteMode(mode)
	suite.ConfigJSON = sql.NullString{String: string(configJSON), Valid: true}
	suite.TestCount = len(tests)
	suite.LastSyncedAt = &now

	if err := s.repo.UpdateSuite(suite); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, suite)
}

// getSuiteTests handles GET /api/suites/:id/tests
func (s *Server) getSuiteTests(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid suite ID"})
		return
	}

	suite, err := s.repo.GetSuiteByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if suite == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Suite not found"})
		return
	}

	// Discover tests from filesystem
	tests, _, err := DiscoverTests(suite.FolderPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Apply filters
	ucFilter := c.Query("uc")
	tagFilter := c.Query("tag")

	if ucFilter != "" || tagFilter != "" {
		filtered := make([]TestInfo, 0)
		for _, t := range tests {
			if ucFilter != "" && t.UseCase != ucFilter {
				continue
			}
			if tagFilter != "" {
				hasTag := false
				for _, tag := range t.Tags {
					if tag == tagFilter {
						hasTag = true
						break
					}
				}
				if !hasTag {
					continue
				}
			}
			filtered = append(filtered, t)
		}
		tests = filtered
	}

	c.JSON(http.StatusOK, gin.H{
		"suite_id": id,
		"tests":    tests,
		"count":    len(tests),
	})
}

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

	// Get test results
	tests, err := s.repo.GetTestResultsByRunID(runID)
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

	tests, err := s.repo.GetTestResultsByRunID(runID)
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
		"run_id": runID,
		"tests":  tests,
		"count":  len(tests),
	})
}

// getRunTestsTree handles GET /api/runs/:run_id/tests/tree
func (s *Server) getRunTestsTree(c *gin.Context) {
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

	tests, err := s.repo.GetTestResultsByRunID(runID)
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
		"run_id":    runID,
		"run":       run,
		"use_cases": useCases,
	})
}

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
func (s *StepReport) UnmarshalJSON(data []byte) error {
	// First try to unmarshal into a map to check for nested "result"
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	// Extract top-level fields
	if v, ok := raw["phase"]; ok {
		json.Unmarshal(v, &s.Phase)
	}
	if v, ok := raw["index"]; ok {
		json.Unmarshal(v, &s.Index)
	}
	if v, ok := raw["handler"]; ok {
		json.Unmarshal(v, &s.Handler)
	}
	if v, ok := raw["name"]; ok {
		json.Unmarshal(v, &s.Name)
	}
	if v, ok := raw["duration_ms"]; ok {
		json.Unmarshal(v, &s.DurationMS)
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
			s.Success = result.Success
			s.ExitCode = result.ExitCode
			s.Stdout = result.Stdout
			s.Stderr = result.Stderr
			s.Error = result.Error
			return nil
		}
	}

	// Otherwise, extract flat fields (Go runner format)
	if v, ok := raw["success"]; ok {
		json.Unmarshal(v, &s.Success)
	}
	if v, ok := raw["exit_code"]; ok {
		json.Unmarshal(v, &s.ExitCode)
	}
	if v, ok := raw["stdout"]; ok {
		json.Unmarshal(v, &s.Stdout)
	}
	if v, ok := raw["stderr"]; ok {
		json.Unmarshal(v, &s.Stderr)
	}
	if v, ok := raw["error"]; ok {
		json.Unmarshal(v, &s.Error)
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

// completeRun handles POST /api/runs/:run_id/complete
func (s *Server) completeRun(c *gin.Context) {
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

	if err := s.repo.CompleteRun(runID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to complete run: " + err.Error()})
		return
	}

	// Get updated run
	run, _ = s.repo.GetRunByID(runID)

	// Emit SSE run_completed event
	durationMS := int64(0)
	if run.DurationMS.Valid {
		durationMS = run.DurationMS.Int64
	}
	s.sseHub.EmitRunCompleted(runID, run.Passed, run.Failed, run.Skipped, durationMS)

	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"run_id":      runID,
		"status":      run.Status,
		"passed":      run.Passed,
		"failed":      run.Failed,
		"duration_ms": nullInt64Value(run.DurationMS),
	})
}

// rerunTests handles POST /api/runs/:run_id/rerun
// Like Python, this spawns CLI subprocess to actually run tests
func (s *Server) rerunTests(c *gin.Context) {
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
	tests, err := s.repo.GetTestResultsByRunID(runID)
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
	process := exec.Command(cmd[0], cmd[1:]...)
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
		"original_run_id": runID,
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

// ==================== Stats ====================

// getStats handles GET /api/stats
func (s *Server) getStats(c *gin.Context) {
	stats, err := s.repo.GetRunStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// ==================== SSE Events ====================

// streamEvents handles GET /api/events
func (s *Server) streamEvents(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	// Subscribe to global events
	eventCh := s.sseHub.SubscribeGlobal()
	defer s.sseHub.UnsubscribeGlobal(eventCh)

	// Get current running run
	currentRunID := s.sseHub.GetCurrentRun()
	if currentRunID == "" {
		// Try to find from DB
		runningRun, _ := s.repo.GetRunningRun()
		if runningRun != nil {
			currentRunID = runningRun.RunID
		}
	}

	// Send connected event
	connected := map[string]any{
		"type":           "connected",
		"current_run_id": currentRunID,
	}
	connectedJSON, _ := json.Marshal(connected)
	c.Writer.WriteString("data: " + string(connectedJSON) + "\n\n")
	c.Writer.Flush()

	// Send cached events for current run (for late subscribers)
	if currentRunID != "" {
		cachedEvents := s.sseHub.GetCachedEvents(currentRunID)
		for _, event := range cachedEvents {
			c.Writer.WriteString(event)
			c.Writer.Flush()
		}
	}

	// Keep connection alive with heartbeat and stream events
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	clientGone := c.Request.Context().Done()

	for {
		select {
		case <-clientGone:
			return
		case event, ok := <-eventCh:
			if !ok {
				return
			}
			c.Writer.WriteString(event)
			c.Writer.Flush()
		case <-ticker.C:
			// Send heartbeat
			c.Writer.WriteString(": heartbeat\n\n")
			c.Writer.Flush()
		}
	}
}

// streamRunEvents handles GET /api/runs/:run_id/stream
func (s *Server) streamRunEvents(c *gin.Context) {
	runID := c.Param("run_id")

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	// Subscribe to run-specific events
	eventCh := s.sseHub.SubscribeRun(runID)
	defer s.sseHub.UnsubscribeRun(runID, eventCh)

	// Send initial state
	run, err := s.repo.GetRunByID(runID)
	if err == nil && run != nil {
		tests, _ := s.repo.GetTestResultsByRunID(runID)
		initial := map[string]any{
			"type":   "initial_state",
			"run_id": runID,
			"run":    run,
			"tests":  tests,
		}
		initialJSON, _ := json.Marshal(initial)
		c.Writer.WriteString("data: " + string(initialJSON) + "\n\n")
	}

	// Send connected event
	connected := map[string]any{
		"type":   "connected",
		"run_id": runID,
	}
	connectedJSON, _ := json.Marshal(connected)
	c.Writer.WriteString("data: " + string(connectedJSON) + "\n\n")
	c.Writer.Flush()

	// Send cached events for this run
	cachedEvents := s.sseHub.GetCachedEvents(runID)
	for _, event := range cachedEvents {
		c.Writer.WriteString(event)
		c.Writer.Flush()
	}

	// Keep connection alive with heartbeat and stream events
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	clientGone := c.Request.Context().Done()

	for {
		select {
		case <-clientGone:
			return
		case event, ok := <-eventCh:
			if !ok {
				return
			}
			c.Writer.WriteString(event)
			c.Writer.Flush()
		case <-ticker.C:
			// Send heartbeat
			c.Writer.WriteString(": heartbeat\n\n")
			c.Writer.Flush()
		}
	}
}

// ==================== Helpers ====================

type useCaseGroup struct {
	UseCase string              `json:"use_case"`
	Tests   []models.TestResult `json:"tests"`
	Pending int                 `json:"pending"`
	Running int                 `json:"running"`
	Passed  int                 `json:"passed"`
	Failed  int                 `json:"failed"`
	Crashed int                 `json:"crashed"`
	Skipped int                 `json:"skipped"`
	Total   int                 `json:"total"`
}

func nullStringValue(ns sql.NullString) any {
	if ns.Valid {
		return ns.String
	}
	return nil
}

func nullInt64Value(ni sql.NullInt64) any {
	if ni.Valid {
		return ni.Int64
	}
	return nil
}

// ==================== Suite Config ====================

// getSuiteConfig handles GET /api/suites/:id/config
func (s *Server) getSuiteConfig(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid suite ID"})
		return
	}

	suite, err := s.repo.GetSuiteByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if suite == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Suite not found"})
		return
	}

	configPath := filepath.Join(suite.FolderPath, "config.yaml")

	// Check if config.yaml exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Config not found: " + configPath})
		return
	}

	// Read raw content
	rawYAML, err := os.ReadFile(configPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read config: " + err.Error()})
		return
	}

	// Parse YAML into structure
	var structure map[string]any
	if err := yaml.Unmarshal(rawYAML, &structure); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse config: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"suite_id":  id,
		"path":      configPath,
		"raw_yaml":  string(rawYAML),
		"structure": structure,
	})
}

// updateSuiteConfig handles PUT /api/suites/:id/config
func (s *Server) updateSuiteConfig(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid suite ID"})
		return
	}

	suite, err := s.repo.GetSuiteByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if suite == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Suite not found"})
		return
	}

	configPath := filepath.Join(suite.FolderPath, "config.yaml")

	// Check if config.yaml exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Config not found: " + configPath})
		return
	}

	var req struct {
		Updates map[string]any `json:"updates"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	if req.Updates == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Must provide 'updates'"})
		return
	}

	// Use YAMLDocument to preserve comments and key ordering
	doc, err := LoadYAMLFile(configPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read config: " + err.Error()})
		return
	}

	// Merge updates preserving structure
	if err := doc.MergeUpdates(req.Updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to merge updates: " + err.Error()})
		return
	}

	// Write back
	newYAML, err := doc.ToBytes()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to marshal config: " + err.Error()})
		return
	}

	if err := os.WriteFile(configPath, newYAML, 0644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to write config: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"raw_yaml": string(newYAML),
	})
}

// mergeUpdates recursively merges updates into the config
func mergeUpdates(config map[string]any, updates map[string]any) {
	for key, value := range updates {
		// Handle "__DELETE__" marker
		if strVal, ok := value.(string); ok && strVal == "__DELETE__" {
			delete(config, key)
			continue
		}

		// If both are maps, merge recursively
		if existingMap, ok := config[key].(map[string]any); ok {
			if updateMap, ok := value.(map[string]any); ok {
				mergeUpdates(existingMap, updateMap)
				continue
			}
		}

		// Otherwise, replace
		config[key] = value
	}
}

// ==================== Test Case YAML Editor ====================

// Helper to strip leading slash from wildcard params
func stripLeadingSlash(s string) string {
	if len(s) > 0 && s[0] == '/' {
		return s[1:]
	}
	return s
}

// getTestYAMLHandler handles GET /api/suites/:id/test-yaml/*test_id
func (s *Server) getTestYAMLHandler(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid suite ID"})
		return
	}
	testID := stripLeadingSlash(c.Param("test_id"))
	s.getTestYAML(c, id, testID)
}

// updateTestYAMLHandler handles PUT /api/suites/:id/test-yaml/*test_id
func (s *Server) updateTestYAMLHandler(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid suite ID"})
		return
	}
	testID := stripLeadingSlash(c.Param("test_id"))
	s.updateTestYAML(c, id, testID)
}

// getTestStepsHandler handles GET /api/suites/:id/test-steps/*test_id
func (s *Server) getTestStepsHandler(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid suite ID"})
		return
	}
	testID := stripLeadingSlash(c.Param("test_id"))
	s.getTestSteps(c, id, testID)
}

// updateTestStepHandler handles PUT /api/suites/:id/test-step/:phase/:index/*test_id
func (s *Server) updateTestStepHandler(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid suite ID"})
		return
	}
	phase := c.Param("phase")
	index, err := strconv.Atoi(c.Param("index"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid index"})
		return
	}
	testID := stripLeadingSlash(c.Param("test_id"))
	s.updateTestStep(c, id, testID, phase, index)
}

// addTestStepHandler handles POST /api/suites/:id/test-step/:phase/*test_id
func (s *Server) addTestStepHandler(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid suite ID"})
		return
	}
	phase := c.Param("phase")
	testID := stripLeadingSlash(c.Param("test_id"))
	s.addTestStep(c, id, testID, phase)
}

// deleteTestStepHandler handles DELETE /api/suites/:id/test-step/:phase/:index/*test_id
func (s *Server) deleteTestStepHandler(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid suite ID"})
		return
	}
	phase := c.Param("phase")
	index, err := strconv.Atoi(c.Param("index"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid index"})
		return
	}
	testID := stripLeadingSlash(c.Param("test_id"))
	s.deleteTestStep(c, id, testID, phase, index)
}

// getTestYAML returns test YAML content
func (s *Server) getTestYAML(c *gin.Context, suiteID int64, testID string) {
	suite, err := s.repo.GetSuiteByID(suiteID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if suite == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Suite not found"})
		return
	}

	// Build path to test.yaml
	testPath := filepath.Join(suite.FolderPath, "suites", testID, "test.yaml")

	// Check if test.yaml exists
	if _, err := os.Stat(testPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Test not found: " + testID})
		return
	}

	// Read raw content
	rawYAML, err := os.ReadFile(testPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read test: " + err.Error()})
		return
	}

	// Parse YAML into structure
	var structure map[string]any
	if err := yaml.Unmarshal(rawYAML, &structure); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse test: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"suite_id":  suiteID,
		"test_id":   testID,
		"path":      testPath,
		"raw_yaml":  string(rawYAML),
		"structure": structure,
	})
}

// updateTestYAML updates test YAML content (preserves comments and key ordering)
func (s *Server) updateTestYAML(c *gin.Context, suiteID int64, testID string) {
	suite, err := s.repo.GetSuiteByID(suiteID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if suite == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Suite not found"})
		return
	}

	testPath := filepath.Join(suite.FolderPath, "suites", testID, "test.yaml")

	// Check if test.yaml exists
	if _, err := os.Stat(testPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Test not found: " + testID})
		return
	}

	var req struct {
		RawYAML string         `json:"raw_yaml"`
		Updates map[string]any `json:"updates"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	var newYAML []byte

	if req.RawYAML != "" {
		// Write raw YAML directly
		// Validate it's valid YAML first
		var test map[string]any
		if err := yaml.Unmarshal([]byte(req.RawYAML), &test); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid YAML: " + err.Error()})
			return
		}
		newYAML = []byte(req.RawYAML)
	} else if req.Updates != nil {
		// Use YAMLDocument to preserve comments and key ordering
		doc, err := LoadYAMLFile(testPath)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read test: " + err.Error()})
			return
		}

		if err := doc.MergeUpdates(req.Updates); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to merge updates: " + err.Error()})
			return
		}

		newYAML, err = doc.ToBytes()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to marshal test: " + err.Error()})
			return
		}
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Must provide 'raw_yaml' or 'updates'"})
		return
	}

	if err := os.WriteFile(testPath, newYAML, 0644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to write test: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"test_id":  testID,
		"raw_yaml": string(newYAML),
	})
}

// getTestSteps returns test steps
func (s *Server) getTestSteps(c *gin.Context, suiteID int64, testID string) {
	suite, err := s.repo.GetSuiteByID(suiteID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if suite == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Suite not found"})
		return
	}

	testPath := filepath.Join(suite.FolderPath, "suites", testID, "test.yaml")

	if _, err := os.Stat(testPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Test not found: " + testID})
		return
	}

	rawYAML, err := os.ReadFile(testPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read test: " + err.Error()})
		return
	}

	var structure map[string]any
	if err := yaml.Unmarshal(rawYAML, &structure); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse test: " + err.Error()})
		return
	}

	// Extract step arrays (return empty arrays if not present)
	preRun, _ := structure["pre_run"].([]any)
	test, _ := structure["test"].([]any)
	postRun, _ := structure["post_run"].([]any)
	assertions, _ := structure["assertions"].([]any)

	if preRun == nil {
		preRun = []any{}
	}
	if test == nil {
		test = []any{}
	}
	if postRun == nil {
		postRun = []any{}
	}
	if assertions == nil {
		assertions = []any{}
	}

	c.JSON(http.StatusOK, gin.H{
		"test_id":    testID,
		"pre_run":    preRun,
		"test":       test,
		"post_run":   postRun,
		"assertions": assertions,
	})
}

// updateTestStep updates a single step (preserves comments and key ordering)
func (s *Server) updateTestStep(c *gin.Context, suiteID int64, testID, phase string, index int) {
	if phase != "pre_run" && phase != "test" && phase != "post_run" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid phase: " + phase})
		return
	}

	suite, err := s.repo.GetSuiteByID(suiteID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if suite == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Suite not found"})
		return
	}

	testPath := filepath.Join(suite.FolderPath, "suites", testID, "test.yaml")

	if _, err := os.Stat(testPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Test not found: " + testID})
		return
	}

	var updates map[string]any
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	// Use YAMLDocument to preserve comments and key ordering
	doc, err := LoadYAMLFile(testPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read test: " + err.Error()})
		return
	}

	// Update the step at the given index
	if err := doc.UpdateSequenceItem(phase, index, updates); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	// Save back
	newYAML, err := doc.ToBytes()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to marshal test: " + err.Error()})
		return
	}

	if err := os.WriteFile(testPath, newYAML, 0644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to write test: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// addTestStep adds a new step
func (s *Server) addTestStep(c *gin.Context, suiteID int64, testID, phase string) {
	if phase != "pre_run" && phase != "test" && phase != "post_run" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid phase: " + phase})
		return
	}

	suite, err := s.repo.GetSuiteByID(suiteID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if suite == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Suite not found"})
		return
	}

	testPath := filepath.Join(suite.FolderPath, "suites", testID, "test.yaml")

	if _, err := os.Stat(testPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Test not found: " + testID})
		return
	}

	var req struct {
		Step  map[string]any `json:"step"`
		Index *int           `json:"index"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	if req.Step == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "step is required"})
		return
	}

	// Use YAMLDocument to preserve comments and key ordering
	doc, err := LoadYAMLFile(testPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read test: " + err.Error()})
		return
	}

	// Add step at index or append
	if err := doc.AddSequenceItem(phase, req.Step, req.Index); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add step: " + err.Error()})
		return
	}

	// Save back
	newYAML, err := doc.ToBytes()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to marshal test: " + err.Error()})
		return
	}

	if err := os.WriteFile(testPath, newYAML, 0644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to write test: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// deleteTestStep deletes a step
func (s *Server) deleteTestStep(c *gin.Context, suiteID int64, testID, phase string, index int) {
	if phase != "pre_run" && phase != "test" && phase != "post_run" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid phase: " + phase})
		return
	}

	suite, err := s.repo.GetSuiteByID(suiteID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if suite == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Suite not found"})
		return
	}

	testPath := filepath.Join(suite.FolderPath, "suites", testID, "test.yaml")

	if _, err := os.Stat(testPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Test not found: " + testID})
		return
	}

	// Use YAMLDocument to preserve comments and key ordering
	doc, err := LoadYAMLFile(testPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read test: " + err.Error()})
		return
	}

	// Remove the step at index
	if err := doc.RemoveSequenceItem(phase, index); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	// Save back
	newYAML, err := doc.ToBytes()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to marshal test: " + err.Error()})
		return
	}

	if err := os.WriteFile(testPath, newYAML, 0644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to write test: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ==================== Suite Run ====================

// runSuite handles POST /api/suites/:id/run
// Launches the Go CLI as a subprocess to run tests
func (s *Server) runSuite(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid suite ID"})
		return
	}

	suite, err := s.repo.GetSuiteByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if suite == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Suite not found"})
		return
	}

	// Check if suite folder exists
	if _, err := os.Stat(suite.FolderPath); os.IsNotExist(err) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Directory not found: " + suite.FolderPath})
		return
	}

	// Parse request body for filters
	var req struct {
		UC       string   `json:"uc"`
		TC       string   `json:"tc"`
		Tags     []string `json:"tags"`
		SkipTags []string `json:"skip_tags"`
	}
	c.ShouldBindJSON(&req) // Optional body

	// Find tsuite executable
	execPath, err := os.Executable()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to find executable: " + err.Error()})
		return
	}

	// Build CLI command
	apiURL := "http://localhost:" + strconv.Itoa(s.port)
	args := []string{
		"run",
		"--suite-path", suite.FolderPath,
		"--api-url", apiURL,
	}

	// Add filter flags
	if req.TC != "" {
		args = append(args, "--tc", req.TC)
	} else if req.UC != "" {
		args = append(args, "--uc", req.UC)
	}
	// If no filters, run all tests (default behavior)

	// Add tag filters
	for _, tag := range req.Tags {
		args = append(args, "--tags", tag)
	}
	// Note: skip_tags not implemented in Go CLI yet

	// Create log file
	logFile, err := os.CreateTemp("", "tsuite_run_*.log")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create log file: " + err.Error()})
		return
	}
	logPath := logFile.Name()

	// Start subprocess
	cmd := newExecCommand(execPath, args...)
	cmd.Dir = suite.FolderPath
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		logFile.Close()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start CLI: " + err.Error()})
		return
	}

	// Close log file - subprocess has inherited the FD
	logFile.Close()

	// Build description
	var description string
	if req.TC != "" {
		description = "Running test case: " + req.TC
	} else if req.UC != "" {
		description = "Running use case: " + req.UC
	} else {
		description = "Running all tests in: " + suite.SuiteName
	}

	// Don't wait for subprocess - let it run independently
	go func() {
		cmd.Wait()
	}()

	c.JSON(http.StatusOK, gin.H{
		"started":     true,
		"pid":         cmd.Process.Pid,
		"description": description,
		"log_file":    logPath,
	})
}

// newExecCommand creates a new exec.Cmd - extracted for testing
var newExecCommand = func(name string, args ...string) *execCmd {
	cmd := &execCmd{Cmd: exec.Command(name, args...)}
	return cmd
}

// execCmd wraps exec.Cmd for easier testing
type execCmd struct {
	*exec.Cmd
}

// ==================== SSE Event Emit ====================

// emitEvent handles POST /api/events/emit
// This endpoint receives events from CLI subprocesses and broadcasts via SSE
func (s *Server) emitEvent(c *gin.Context) {
	var req map[string]any

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	eventType, _ := req["type"].(string)
	if eventType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing 'type' field"})
		return
	}

	runID, _ := req["run_id"].(string)

	// Create and emit the event
	event := NewSSEEvent(eventType, req)
	s.sseHub.Emit(event, runID)

	// Update current run tracking
	if eventType == "run_started" {
		s.sseHub.SetCurrentRun(runID)
	} else if eventType == "run_completed" {
		s.sseHub.SetCurrentRun("")
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ==================== File Browser ====================

// browseFolders handles GET /api/browse
// Browse directories for folder selection in the dashboard
func (s *Server) browseFolders(c *gin.Context) {
	requestedPath := c.Query("path")

	var path string
	if requestedPath == "" {
		// Default to home directory
		home, err := os.UserHomeDir()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Cannot get home directory"})
			return
		}
		path = home
	} else {
		// Expand ~ if present
		if len(requestedPath) > 0 && requestedPath[0] == '~' {
			home, err := os.UserHomeDir()
			if err == nil {
				requestedPath = filepath.Join(home, requestedPath[1:])
			}
		}
		// Resolve to absolute path
		absPath, err := filepath.Abs(requestedPath)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid path"})
			return
		}
		path = absPath
	}

	// Security: don't allow browsing certain system directories
	restrictedPrefixes := []string{"/proc", "/sys", "/dev", "/etc/shadow"}
	for _, prefix := range restrictedPrefixes {
		if len(path) >= len(prefix) && path[:len(prefix)] == prefix {
			c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
			return
		}
	}

	// Check if path exists
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Path does not exist: " + path})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !info.IsDir() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not a directory: " + path})
		return
	}

	// Check if this is a valid test suite
	configPath := filepath.Join(path, "config.yaml")
	suitesPath := filepath.Join(path, "suites")
	_, configErr := os.Stat(configPath)
	suitesInfo, suitesErr := os.Stat(suitesPath)
	isSuite := configErr == nil && suitesErr == nil && suitesInfo.IsDir()

	// Get parent directory
	parent := filepath.Dir(path)
	var parentPtr *string
	if parent != path {
		parentPtr = &parent
	}

	// List directories
	type DirEntry struct {
		Name    string `json:"name"`
		Path    string `json:"path"`
		IsSuite bool   `json:"is_suite"`
	}
	var directories []DirEntry

	entries, err := os.ReadDir(path)
	if os.IsPermission(err) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Permission denied"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() || entry.Name()[0] == '.' {
			continue
		}

		entryPath := filepath.Join(path, entry.Name())

		// Check if subdir is a suite
		subConfigPath := filepath.Join(entryPath, "config.yaml")
		subSuitesPath := filepath.Join(entryPath, "suites")
		_, subConfigErr := os.Stat(subConfigPath)
		subSuitesInfo, subSuitesErr := os.Stat(subSuitesPath)
		subdirIsSuite := subConfigErr == nil && subSuitesErr == nil && subSuitesInfo.IsDir()

		directories = append(directories, DirEntry{
			Name:    entry.Name(),
			Path:    entryPath,
			IsSuite: subdirIsSuite,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"path":        path,
		"parent":      parentPtr,
		"directories": directories,
		"is_suite":    isSuite,
	})
}
