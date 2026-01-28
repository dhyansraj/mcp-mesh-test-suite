package db

import (
	"database/sql"
	"time"

	"github.com/dhyansraj/mcp-mesh-test-suite/go/internal/models"
)

// Repository provides database operations
type Repository struct {
	db *sql.DB
}

// NewRepository creates a new repository
func NewRepository() (*Repository, error) {
	db, err := GetDB()
	if err != nil {
		return nil, err
	}
	return &Repository{db: db}, nil
}

// ==================== Suites ====================

// GetAllSuites returns all registered suites
func (r *Repository) GetAllSuites() ([]models.Suite, error) {
	rows, err := r.db.Query(`
		SELECT id, folder_path, suite_name, mode, config_json, test_count,
		       last_synced_at, created_at, updated_at
		FROM suites
		ORDER BY suite_name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var suites []models.Suite
	for rows.Next() {
		var s models.Suite
		var lastSynced, created, updated sql.NullString

		err := rows.Scan(&s.ID, &s.FolderPath, &s.SuiteName, &s.Mode, &s.ConfigJSON,
			&s.TestCount, &lastSynced, &created, &updated)
		if err != nil {
			return nil, err
		}

		s.LastSyncedAt = parseTime(lastSynced)
		s.CreatedAt = parseTime(created)
		s.UpdatedAt = parseTime(updated)

		suites = append(suites, s)
	}

	return suites, rows.Err()
}

// GetSuiteByID returns a suite by ID
func (r *Repository) GetSuiteByID(id int64) (*models.Suite, error) {
	var s models.Suite
	var lastSynced, created, updated sql.NullString

	err := r.db.QueryRow(`
		SELECT id, folder_path, suite_name, mode, config_json, test_count,
		       last_synced_at, created_at, updated_at
		FROM suites WHERE id = ?
	`, id).Scan(&s.ID, &s.FolderPath, &s.SuiteName, &s.Mode, &s.ConfigJSON,
		&s.TestCount, &lastSynced, &created, &updated)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	s.LastSyncedAt = parseTime(lastSynced)
	s.CreatedAt = parseTime(created)
	s.UpdatedAt = parseTime(updated)

	return &s, nil
}

// GetSuiteByPath returns a suite by folder path
func (r *Repository) GetSuiteByPath(folderPath string) (*models.Suite, error) {
	var s models.Suite
	var lastSynced, created, updated sql.NullString

	err := r.db.QueryRow(`
		SELECT id, folder_path, suite_name, mode, config_json, test_count,
		       last_synced_at, created_at, updated_at
		FROM suites WHERE folder_path = ?
	`, folderPath).Scan(&s.ID, &s.FolderPath, &s.SuiteName, &s.Mode, &s.ConfigJSON,
		&s.TestCount, &lastSynced, &created, &updated)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	s.LastSyncedAt = parseTime(lastSynced)
	s.CreatedAt = parseTime(created)
	s.UpdatedAt = parseTime(updated)

	return &s, nil
}

// CreateSuite creates a new suite
func (r *Repository) CreateSuite(s *models.Suite) error {
	result, err := r.db.Exec(`
		INSERT INTO suites (folder_path, suite_name, mode, config_json, test_count, last_synced_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, s.FolderPath, s.SuiteName, s.Mode, s.ConfigJSON, s.TestCount,
		formatTime(s.LastSyncedAt))

	if err != nil {
		return err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	s.ID = id

	return nil
}

// UpdateSuite updates a suite
func (r *Repository) UpdateSuite(s *models.Suite) error {
	_, err := r.db.Exec(`
		UPDATE suites SET
			folder_path = ?,
			suite_name = ?,
			mode = ?,
			config_json = ?,
			test_count = ?,
			last_synced_at = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, s.FolderPath, s.SuiteName, s.Mode, s.ConfigJSON, s.TestCount,
		formatTime(s.LastSyncedAt), s.ID)

	return err
}

// DeleteSuite deletes a suite
func (r *Repository) DeleteSuite(id int64) error {
	_, err := r.db.Exec(`DELETE FROM suites WHERE id = ?`, id)
	return err
}

// ==================== Runs ====================

// GetAllRuns returns all runs, optionally filtered by suite
func (r *Repository) GetAllRuns(suiteID *int64, limit int) ([]models.Run, error) {
	query := `
		SELECT r.run_id, r.suite_id, COALESCE(r.suite_name, s.suite_name) as suite_name, r.started_at, r.finished_at,
		       r.status, r.cli_version, r.sdk_python_version, r.sdk_typescript_version,
		       r.docker_image, r.total_tests, r.pending_count, r.running_count,
		       r.passed, r.failed, r.skipped, r.duration_ms, r.filters, r.mode,
		       r.cancel_requested,
		       CASE
		           WHEN (SELECT COUNT(*) FROM test_results tr WHERE tr.run_id = r.run_id) = 1
		               THEN (SELECT tr.test_id FROM test_results tr WHERE tr.run_id = r.run_id LIMIT 1)
		           WHEN (SELECT COUNT(DISTINCT tr.use_case) FROM test_results tr WHERE tr.run_id = r.run_id) = 1
		               THEN (SELECT tr.use_case FROM test_results tr WHERE tr.run_id = r.run_id LIMIT 1)
		           ELSE NULL
		       END as display_name
		FROM runs r
		LEFT JOIN suites s ON r.suite_id = s.id
	`

	var args []any
	if suiteID != nil {
		query += " WHERE r.suite_id = ?"
		args = append(args, *suiteID)
	}

	query += " ORDER BY r.started_at DESC"

	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []models.Run
	for rows.Next() {
		var run models.Run
		var startedAt string
		var finishedAt sql.NullString

		err := rows.Scan(
			&run.RunID, &run.SuiteID, &run.SuiteName, &startedAt, &finishedAt,
			&run.Status, &run.CLIVersion, &run.SDKPythonVersion, &run.SDKTypescriptVersion,
			&run.DockerImage, &run.TotalTests, &run.PendingCount, &run.RunningCount,
			&run.Passed, &run.Failed, &run.Skipped, &run.DurationMS, &run.Filters,
			&run.Mode, &run.CancelRequested, &run.DisplayName,
		)
		if err != nil {
			return nil, err
		}

		run.StartedAt, _ = time.Parse(time.RFC3339, startedAt)
		run.FinishedAt = parseTime(finishedAt)

		runs = append(runs, run)
	}

	return runs, rows.Err()
}

// GetRunByID returns a run by ID
func (r *Repository) GetRunByID(runID string) (*models.Run, error) {
	var run models.Run
	var startedAt string
	var finishedAt sql.NullString

	err := r.db.QueryRow(`
		SELECT r.run_id, r.suite_id, COALESCE(r.suite_name, s.suite_name) as suite_name, r.started_at, r.finished_at,
		       r.status, r.cli_version, r.sdk_python_version, r.sdk_typescript_version,
		       r.docker_image, r.total_tests, r.pending_count, r.running_count,
		       r.passed, r.failed, r.skipped, r.duration_ms, r.filters, r.mode,
		       r.cancel_requested,
		       CASE
		           WHEN (SELECT COUNT(*) FROM test_results tr WHERE tr.run_id = r.run_id) = 1
		               THEN (SELECT tr.test_id FROM test_results tr WHERE tr.run_id = r.run_id LIMIT 1)
		           WHEN (SELECT COUNT(DISTINCT tr.use_case) FROM test_results tr WHERE tr.run_id = r.run_id) = 1
		               THEN (SELECT tr.use_case FROM test_results tr WHERE tr.run_id = r.run_id LIMIT 1)
		           ELSE NULL
		       END as display_name
		FROM runs r
		LEFT JOIN suites s ON r.suite_id = s.id
		WHERE r.run_id = ?
	`, runID).Scan(
		&run.RunID, &run.SuiteID, &run.SuiteName, &startedAt, &finishedAt,
		&run.Status, &run.CLIVersion, &run.SDKPythonVersion, &run.SDKTypescriptVersion,
		&run.DockerImage, &run.TotalTests, &run.PendingCount, &run.RunningCount,
		&run.Passed, &run.Failed, &run.Skipped, &run.DurationMS, &run.Filters,
		&run.Mode, &run.CancelRequested, &run.DisplayName,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	run.StartedAt, _ = time.Parse(time.RFC3339, startedAt)
	run.FinishedAt = parseTime(finishedAt)

	return &run, nil
}

// GetLatestRun returns the most recent run
func (r *Repository) GetLatestRun() (*models.Run, error) {
	runs, err := r.GetAllRuns(nil, 1)
	if err != nil {
		return nil, err
	}
	if len(runs) == 0 {
		return nil, nil
	}
	return &runs[0], nil
}

// GetRunningRun returns the currently running run (if any)
func (r *Repository) GetRunningRun() (*models.Run, error) {
	var run models.Run
	var startedAt string
	var finishedAt sql.NullString

	err := r.db.QueryRow(`
		SELECT r.run_id, r.suite_id, COALESCE(r.suite_name, s.suite_name) as suite_name, r.started_at, r.finished_at,
		       r.status, r.cli_version, r.sdk_python_version, r.sdk_typescript_version,
		       r.docker_image, r.total_tests, r.pending_count, r.running_count,
		       r.passed, r.failed, r.skipped, r.duration_ms, r.filters, r.mode,
		       r.cancel_requested,
		       CASE
		           WHEN (SELECT COUNT(*) FROM test_results tr WHERE tr.run_id = r.run_id) = 1
		               THEN (SELECT tr.test_id FROM test_results tr WHERE tr.run_id = r.run_id LIMIT 1)
		           WHEN (SELECT COUNT(DISTINCT tr.use_case) FROM test_results tr WHERE tr.run_id = r.run_id) = 1
		               THEN (SELECT tr.use_case FROM test_results tr WHERE tr.run_id = r.run_id LIMIT 1)
		           ELSE NULL
		       END as display_name
		FROM runs r
		LEFT JOIN suites s ON r.suite_id = s.id
		WHERE r.status = 'running'
		ORDER BY r.started_at DESC
		LIMIT 1
	`).Scan(
		&run.RunID, &run.SuiteID, &run.SuiteName, &startedAt, &finishedAt,
		&run.Status, &run.CLIVersion, &run.SDKPythonVersion, &run.SDKTypescriptVersion,
		&run.DockerImage, &run.TotalTests, &run.PendingCount, &run.RunningCount,
		&run.Passed, &run.Failed, &run.Skipped, &run.DurationMS, &run.Filters,
		&run.Mode, &run.CancelRequested, &run.DisplayName,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	run.StartedAt, _ = time.Parse(time.RFC3339, startedAt)
	run.FinishedAt = parseTime(finishedAt)

	return &run, nil
}

// SetCancelRequested sets the cancel flag for a run
func (r *Repository) SetCancelRequested(runID string) error {
	_, err := r.db.Exec(`UPDATE runs SET cancel_requested = 1 WHERE run_id = ?`, runID)
	return err
}

// MarkRunCancelled marks a run as cancelled (called by CLI after terminating workers)
// Also marks remaining pending and running tests as skipped
func (r *Repository) MarkRunCancelled(runID string) error {
	now := time.Now().UTC().Format(time.RFC3339)

	// Mark all pending tests as skipped
	_, err := r.db.Exec(`
		UPDATE test_results SET
			status = 'skipped',
			skip_reason = 'Run cancelled'
		WHERE run_id = ? AND status = 'pending'
	`, runID)
	if err != nil {
		return err
	}

	// Mark all running tests as skipped (they were terminated)
	_, err = r.db.Exec(`
		UPDATE test_results SET
			status = 'skipped',
			finished_at = ?,
			skip_reason = 'Run cancelled (terminated)'
		WHERE run_id = ? AND status = 'running'
	`, now, runID)
	if err != nil {
		return err
	}

	// Update counts on run (pending and running become 0, skipped gets updated)
	_, err = r.db.Exec(`
		UPDATE runs SET
			pending_count = 0,
			running_count = 0,
			skipped = (SELECT COUNT(*) FROM test_results WHERE run_id = ? AND status = 'skipped')
		WHERE run_id = ?
	`, runID, runID)
	if err != nil {
		return err
	}

	// Mark run as cancelled
	_, err = r.db.Exec(`
		UPDATE runs SET
			status = 'cancelled',
			finished_at = ?,
			duration_ms = CAST(
				(julianday(?) - julianday(started_at)) * 24 * 60 * 60 * 1000 AS INTEGER
			)
		WHERE run_id = ?
	`, now, now, runID)
	return err
}

// ==================== Test Results ====================

// GetTestResultsByRunID returns all test results for a run
func (r *Repository) GetTestResultsByRunID(runID string) ([]models.TestResult, error) {
	rows, err := r.db.Query(`
		SELECT id, run_id, test_id, use_case, test_case, name, tags, status,
		       started_at, finished_at, duration_ms, error_message, error_step,
		       skip_reason, steps_json, steps_passed, steps_failed
		FROM test_results
		WHERE run_id = ?
		ORDER BY use_case, test_case
	`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []models.TestResult
	for rows.Next() {
		var t models.TestResult
		var startedAt, finishedAt sql.NullString

		err := rows.Scan(
			&t.ID, &t.RunID, &t.TestID, &t.UseCase, &t.TestCase, &t.Name, &t.Tags,
			&t.Status, &startedAt, &finishedAt, &t.DurationMS, &t.ErrorMessage,
			&t.ErrorStep, &t.SkipReason, &t.StepsJSON, &t.StepsPassed, &t.StepsFailed,
		)
		if err != nil {
			return nil, err
		}

		t.StartedAt = parseTime(startedAt)
		t.FinishedAt = parseTime(finishedAt)

		results = append(results, t)
	}

	return results, rows.Err()
}

// GetTestResultByID returns a test result by ID
func (r *Repository) GetTestResultByID(id int64) (*models.TestResult, error) {
	var t models.TestResult
	var startedAt, finishedAt sql.NullString

	err := r.db.QueryRow(`
		SELECT id, run_id, test_id, use_case, test_case, name, tags, status,
		       started_at, finished_at, duration_ms, error_message, error_step,
		       skip_reason, steps_json, steps_passed, steps_failed
		FROM test_results
		WHERE id = ?
	`, id).Scan(
		&t.ID, &t.RunID, &t.TestID, &t.UseCase, &t.TestCase, &t.Name, &t.Tags,
		&t.Status, &startedAt, &finishedAt, &t.DurationMS, &t.ErrorMessage,
		&t.ErrorStep, &t.SkipReason, &t.StepsJSON, &t.StepsPassed, &t.StepsFailed,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	t.StartedAt = parseTime(startedAt)
	t.FinishedAt = parseTime(finishedAt)

	return &t, nil
}

// ==================== Step Results ====================

// GetStepResultsByTestID returns all step results for a test
func (r *Repository) GetStepResultsByTestID(testResultID int64) ([]models.StepResult, error) {
	rows, err := r.db.Query(`
		SELECT id, test_result_id, step_index, phase, handler, description, status,
		       started_at, finished_at, duration_ms, exit_code, stdout, stderr, error_message
		FROM step_results
		WHERE test_result_id = ?
		ORDER BY phase, step_index
	`, testResultID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []models.StepResult
	for rows.Next() {
		var s models.StepResult
		var startedAt, finishedAt sql.NullString

		err := rows.Scan(
			&s.ID, &s.TestResultID, &s.StepIndex, &s.Phase, &s.Handler, &s.Description,
			&s.Status, &startedAt, &finishedAt, &s.DurationMS, &s.ExitCode,
			&s.Stdout, &s.Stderr, &s.ErrorMessage,
		)
		if err != nil {
			return nil, err
		}

		s.StartedAt = parseTime(startedAt)
		s.FinishedAt = parseTime(finishedAt)

		results = append(results, s)
	}

	return results, rows.Err()
}

// ==================== Assertions ====================

// GetAssertionsByTestID returns all assertions for a test
func (r *Repository) GetAssertionsByTestID(testResultID int64) ([]models.AssertionResult, error) {
	rows, err := r.db.Query(`
		SELECT id, test_result_id, assertion_index, expression, message, passed,
		       actual_value, expected_value
		FROM assertion_results
		WHERE test_result_id = ?
		ORDER BY assertion_index
	`, testResultID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []models.AssertionResult
	for rows.Next() {
		var a models.AssertionResult

		err := rows.Scan(
			&a.ID, &a.TestResultID, &a.AssertionIndex, &a.Expression, &a.Message,
			&a.Passed, &a.ActualValue, &a.ExpectedValue,
		)
		if err != nil {
			return nil, err
		}

		results = append(results, a)
	}

	return results, rows.Err()
}

// ==================== Captured Values ====================

// GetCapturedValuesByTestID returns all captured values for a test
func (r *Repository) GetCapturedValuesByTestID(testResultID int64) ([]models.CapturedValue, error) {
	rows, err := r.db.Query(`
		SELECT id, test_result_id, key, value, captured_at
		FROM captured_values
		WHERE test_result_id = ?
		ORDER BY key
	`, testResultID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []models.CapturedValue
	for rows.Next() {
		var c models.CapturedValue
		var capturedAt sql.NullString

		err := rows.Scan(&c.ID, &c.TestResultID, &c.Key, &c.Value, &capturedAt)
		if err != nil {
			return nil, err
		}

		c.CapturedAt = parseTime(capturedAt)

		results = append(results, c)
	}

	return results, rows.Err()
}

// ==================== Helpers ====================

func parseTime(ns sql.NullString) *time.Time {
	if !ns.Valid || ns.String == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, ns.String)
	if err != nil {
		// Try ISO format without timezone
		t, err = time.Parse("2006-01-02T15:04:05", ns.String)
		if err != nil {
			return nil
		}
	}
	return &t
}

func formatTime(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return t.Format(time.RFC3339)
}

// ==================== Run Creation ====================

// CreateRun creates a new test run
func (r *Repository) CreateRun(run *models.Run) error {
	_, err := r.db.Exec(`
		INSERT INTO runs (
			run_id, suite_id, suite_name, started_at, status,
			cli_version, sdk_python_version, sdk_typescript_version, docker_image,
			total_tests, pending_count, running_count, passed, failed, skipped,
			mode, cancel_requested
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		run.RunID,
		nullInt64(run.SuiteID),
		nullString(run.SuiteName),
		run.StartedAt.Format(time.RFC3339),
		run.Status,
		nullString(run.CLIVersion),
		nullString(run.SDKPythonVersion),
		nullString(run.SDKTypescriptVersion),
		nullString(run.DockerImage),
		run.TotalTests,
		run.PendingCount,
		run.RunningCount,
		run.Passed,
		run.Failed,
		run.Skipped,
		run.Mode,
		run.CancelRequested,
	)
	return err
}

// CreateTestResult creates a new test result record
func (r *Repository) CreateTestResult(tr *models.TestResult) error {
	result, err := r.db.Exec(`
		INSERT INTO test_results (
			run_id, test_id, use_case, test_case, name, tags, status,
			steps_passed, steps_failed
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		tr.RunID,
		tr.TestID,
		tr.UseCase,
		tr.TestCase,
		nullString(tr.Name),
		nullString(tr.Tags),
		tr.Status,
		tr.StepsPassed,
		tr.StepsFailed,
	)
	if err != nil {
		return err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	tr.ID = id
	return nil
}

// UpdateTestResult updates an existing test result
func (r *Repository) UpdateTestResult(tr *models.TestResult) error {
	_, err := r.db.Exec(`
		UPDATE test_results SET
			status = ?,
			started_at = ?,
			finished_at = ?,
			duration_ms = ?,
			error_message = ?,
			error_step = ?,
			steps_passed = ?,
			steps_failed = ?,
			steps_json = ?
		WHERE id = ?
	`,
		tr.Status,
		formatTime(tr.StartedAt),
		formatTime(tr.FinishedAt),
		nullInt64(tr.DurationMS),
		nullString(tr.ErrorMessage),
		nullInt64(tr.ErrorStep),
		tr.StepsPassed,
		tr.StepsFailed,
		nullString(tr.StepsJSON),
		tr.ID,
	)
	return err
}

// GetTestResultByTestIDAndRunID gets a test result by test_id and run_id
func (r *Repository) GetTestResultByTestIDAndRunID(testID, runID string) (*models.TestResult, error) {
	var t models.TestResult
	var startedAt, finishedAt sql.NullString

	err := r.db.QueryRow(`
		SELECT id, run_id, test_id, use_case, test_case, name, tags, status,
		       started_at, finished_at, duration_ms, error_message, error_step,
		       skip_reason, steps_json, steps_passed, steps_failed
		FROM test_results
		WHERE test_id = ? AND run_id = ?
	`, testID, runID).Scan(
		&t.ID, &t.RunID, &t.TestID, &t.UseCase, &t.TestCase, &t.Name, &t.Tags,
		&t.Status, &startedAt, &finishedAt, &t.DurationMS, &t.ErrorMessage,
		&t.ErrorStep, &t.SkipReason, &t.StepsJSON, &t.StepsPassed, &t.StepsFailed,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	t.StartedAt = parseTime(startedAt)
	t.FinishedAt = parseTime(finishedAt)

	return &t, nil
}

// UpdateRunCounters updates the test count fields on a run (full recount - use sparingly)
func (r *Repository) UpdateRunCounters(runID string) error {
	_, err := r.db.Exec(`
		UPDATE runs SET
			pending_count = (SELECT COUNT(*) FROM test_results WHERE run_id = ? AND status = 'pending'),
			running_count = (SELECT COUNT(*) FROM test_results WHERE run_id = ? AND status = 'running'),
			passed = (SELECT COUNT(*) FROM test_results WHERE run_id = ? AND status = 'passed'),
			failed = (SELECT COUNT(*) FROM test_results WHERE run_id = ? AND status IN ('failed', 'crashed')),
			skipped = (SELECT COUNT(*) FROM test_results WHERE run_id = ? AND status = 'skipped')
		WHERE run_id = ?
	`, runID, runID, runID, runID, runID, runID)
	return err
}

// UpdateRunCountersIncremental updates run counters based on status transition (idempotent)
// This is the preferred method during test execution to avoid race conditions
func (r *Repository) UpdateRunCountersIncremental(runID string, oldStatus, newStatus models.TestStatus) error {
	if oldStatus == newStatus {
		return nil
	}

	// Decrement old status counter
	switch oldStatus {
	case models.TestStatusPending:
		r.db.Exec("UPDATE runs SET pending_count = pending_count - 1 WHERE run_id = ? AND pending_count > 0", runID)
	case models.TestStatusRunning:
		r.db.Exec("UPDATE runs SET running_count = running_count - 1 WHERE run_id = ? AND running_count > 0", runID)
	case models.TestStatusPassed:
		r.db.Exec("UPDATE runs SET passed = passed - 1 WHERE run_id = ? AND passed > 0", runID)
	case models.TestStatusFailed, models.TestStatusCrashed:
		// Both count as failed
		r.db.Exec("UPDATE runs SET failed = failed - 1 WHERE run_id = ? AND failed > 0", runID)
	case models.TestStatusSkipped:
		r.db.Exec("UPDATE runs SET skipped = skipped - 1 WHERE run_id = ? AND skipped > 0", runID)
	}

	// Increment new status counter
	switch newStatus {
	case models.TestStatusPending:
		r.db.Exec("UPDATE runs SET pending_count = pending_count + 1 WHERE run_id = ?", runID)
	case models.TestStatusRunning:
		r.db.Exec("UPDATE runs SET running_count = running_count + 1 WHERE run_id = ?", runID)
	case models.TestStatusPassed:
		r.db.Exec("UPDATE runs SET passed = passed + 1 WHERE run_id = ?", runID)
	case models.TestStatusFailed, models.TestStatusCrashed:
		// Both count as failed
		r.db.Exec("UPDATE runs SET failed = failed + 1 WHERE run_id = ?", runID)
	case models.TestStatusSkipped:
		r.db.Exec("UPDATE runs SET skipped = skipped + 1 WHERE run_id = ?", runID)
	}

	return nil
}

// CompleteRun marks a run as completed and calculates duration
func (r *Repository) CompleteRun(runID string) error {
	now := time.Now().Format(time.RFC3339)

	// Determine status based on test results
	var failed int
	err := r.db.QueryRow(`
		SELECT COUNT(*) FROM test_results
		WHERE run_id = ? AND status IN ('failed', 'crashed')
	`, runID).Scan(&failed)
	if err != nil {
		return err
	}

	status := models.RunStatusCompleted
	if failed > 0 {
		status = models.RunStatusFailed
	}

	// Calculate duration as wall-clock time (finished - started), not sum of test durations
	// With parallel execution, sum would be much larger than actual elapsed time
	_, err = r.db.Exec(`
		UPDATE runs SET
			status = ?,
			finished_at = ?,
			duration_ms = CAST(
				(julianday(?) - julianday(started_at)) * 24 * 60 * 60 * 1000 AS INTEGER
			)
		WHERE run_id = ?
	`, status, now, now, runID)

	return err
}

// UpdateRunStatus updates the status of a run
func (r *Repository) UpdateRunStatus(runID string, status models.RunStatus) error {
	_, err := r.db.Exec(`UPDATE runs SET status = ? WHERE run_id = ?`, status, runID)
	return err
}

// DeleteRun deletes a run and all associated records (test results, steps, assertions, captured values)
func (r *Repository) DeleteRun(runID string) error {
	// Start a transaction
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Delete step_results for all tests in this run
	_, err = tx.Exec(`
		DELETE FROM step_results
		WHERE test_result_id IN (SELECT id FROM test_results WHERE run_id = ?)
	`, runID)
	if err != nil {
		return err
	}

	// Delete assertion_results for all tests in this run
	_, err = tx.Exec(`
		DELETE FROM assertion_results
		WHERE test_result_id IN (SELECT id FROM test_results WHERE run_id = ?)
	`, runID)
	if err != nil {
		return err
	}

	// Delete captured_values for all tests in this run
	_, err = tx.Exec(`
		DELETE FROM captured_values
		WHERE test_result_id IN (SELECT id FROM test_results WHERE run_id = ?)
	`, runID)
	if err != nil {
		return err
	}

	// Delete test_results for this run
	_, err = tx.Exec(`DELETE FROM test_results WHERE run_id = ?`, runID)
	if err != nil {
		return err
	}

	// Delete the run itself
	_, err = tx.Exec(`DELETE FROM runs WHERE run_id = ?`, runID)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// ==================== Step Results ====================

// CreateStepResult creates a new step result record
func (r *Repository) CreateStepResult(sr *models.StepResult) error {
	result, err := r.db.Exec(`
		INSERT INTO step_results (
			test_result_id, step_index, phase, handler, description, status,
			started_at, finished_at, duration_ms, exit_code, stdout, stderr, error_message
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		sr.TestResultID,
		sr.StepIndex,
		sr.Phase,
		sr.Handler,
		nullString(sr.Description),
		sr.Status,
		formatTime(sr.StartedAt),
		formatTime(sr.FinishedAt),
		nullInt64(sr.DurationMS),
		nullInt64(sr.ExitCode),
		nullString(sr.Stdout),
		nullString(sr.Stderr),
		nullString(sr.ErrorMessage),
	)
	if err != nil {
		return err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	sr.ID = id
	return nil
}

// ==================== Assertion Results ====================

// CreateAssertionResult creates a new assertion result record
func (r *Repository) CreateAssertionResult(ar *models.AssertionResult) error {
	result, err := r.db.Exec(`
		INSERT INTO assertion_results (
			test_result_id, assertion_index, expression, message, passed,
			actual_value, expected_value
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`,
		ar.TestResultID,
		ar.AssertionIndex,
		ar.Expression,
		nullString(ar.Message),
		ar.Passed,
		nullString(ar.ActualValue),
		nullString(ar.ExpectedValue),
	)
	if err != nil {
		return err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	ar.ID = id
	return nil
}

// RunStats holds aggregate statistics across all runs
type RunStats struct {
	TotalRuns          int64   `json:"total_runs"`
	TotalTestsExecuted int64   `json:"total_tests_executed"`
	TotalPassed        int64   `json:"total_passed"`
	TotalFailed        int64   `json:"total_failed"`
	AvgRunDurationMS   *int64  `json:"avg_run_duration_ms"`
	PassRate           float64 `json:"pass_rate"`
}

// GetRunStats returns aggregate statistics across all completed/failed runs
func (r *Repository) GetRunStats() (*RunStats, error) {
	row := r.db.QueryRow(`
		SELECT
			COUNT(*) as total_runs,
			COALESCE(SUM(total_tests), 0) as total_tests_executed,
			COALESCE(SUM(passed), 0) as total_passed,
			COALESCE(SUM(failed), 0) as total_failed,
			AVG(duration_ms) as avg_run_duration_ms
		FROM runs
		WHERE status IN ('completed', 'failed')
	`)

	stats := &RunStats{}
	var avgDuration sql.NullFloat64

	err := row.Scan(
		&stats.TotalRuns,
		&stats.TotalTestsExecuted,
		&stats.TotalPassed,
		&stats.TotalFailed,
		&avgDuration,
	)
	if err != nil {
		return nil, err
	}

	if avgDuration.Valid {
		avg := int64(avgDuration.Float64)
		stats.AvgRunDurationMS = &avg
	}

	// Calculate pass rate
	total := stats.TotalPassed + stats.TotalFailed
	if total > 0 {
		stats.PassRate = float64(stats.TotalPassed) / float64(total) * 100
		// Round to 2 decimal places
		stats.PassRate = float64(int(stats.PassRate*100)) / 100
	}

	return stats, nil
}

// Helper functions for null values
func nullString(ns sql.NullString) interface{} {
	if ns.Valid {
		return ns.String
	}
	return nil
}

func nullInt64(ni sql.NullInt64) interface{} {
	if ni.Valid {
		return ni.Int64
	}
	return nil
}
