"""
Repository layer for database CRUD operations.
"""

from datetime import datetime
from typing import Optional, List
import json
import uuid

from . import db
from .models import (
    Run, RunStatus, RunSummary,
    TestResult, TestStatus, TestDetail, TERMINAL_STATES,
    StepResult, StepStatus,
    AssertionResult,
    CapturedValue,
    Suite, SuiteMode,
)


# =============================================================================
# Run Operations
# =============================================================================

def create_run(
    suite_id: Optional[int] = None,
    cli_version: Optional[str] = None,
    sdk_python_version: Optional[str] = None,
    sdk_typescript_version: Optional[str] = None,
    docker_image: Optional[str] = None,
    total_tests: int = 0,
) -> Run:
    """Create a new test run."""
    run_id = str(uuid.uuid4())
    started_at = datetime.now()

    db.execute(
        """
        INSERT INTO runs (
            run_id, suite_id, started_at, status, cli_version,
            sdk_python_version, sdk_typescript_version,
            docker_image, total_tests
        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
        """,
        (
            run_id, suite_id, started_at.isoformat(), RunStatus.RUNNING.value,
            cli_version, sdk_python_version, sdk_typescript_version,
            docker_image, total_tests,
        ),
    )
    db.commit()

    return Run(
        run_id=run_id,
        suite_id=suite_id,
        started_at=started_at,
        status=RunStatus.RUNNING,
        cli_version=cli_version,
        sdk_python_version=sdk_python_version,
        sdk_typescript_version=sdk_typescript_version,
        docker_image=docker_image,
        total_tests=total_tests,
    )


def update_run(
    run_id: str,
    status: Optional[RunStatus] = None,
    finished_at: Optional[datetime] = None,
    passed: Optional[int] = None,
    failed: Optional[int] = None,
    skipped: Optional[int] = None,
    duration_ms: Optional[int] = None,
) -> None:
    """Update a run record."""
    updates = []
    params = []

    if status is not None:
        updates.append("status = ?")
        params.append(status.value)
    if finished_at is not None:
        updates.append("finished_at = ?")
        params.append(finished_at.isoformat())
    if passed is not None:
        updates.append("passed = ?")
        params.append(passed)
    if failed is not None:
        updates.append("failed = ?")
        params.append(failed)
    if skipped is not None:
        updates.append("skipped = ?")
        params.append(skipped)
    if duration_ms is not None:
        updates.append("duration_ms = ?")
        params.append(duration_ms)

    if updates:
        params.append(run_id)
        db.execute(
            f"UPDATE runs SET {', '.join(updates)} WHERE run_id = ?",
            tuple(params),
        )
        db.commit()


def get_run(run_id: str) -> Optional[Run]:
    """Get a run by ID with suite name and computed display_name."""
    row = db.fetchone(
        """
        SELECT r.*, s.suite_name,
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
        """,
        (run_id,),
    )
    return Run.from_row(row) if row else None


def get_latest_run() -> Optional[Run]:
    """Get the most recent run with suite name and computed display_name."""
    row = db.fetchone(
        """
        SELECT r.*, s.suite_name,
            CASE
                WHEN (SELECT COUNT(*) FROM test_results tr WHERE tr.run_id = r.run_id) = 1
                    THEN (SELECT tr.test_id FROM test_results tr WHERE tr.run_id = r.run_id LIMIT 1)
                WHEN (SELECT COUNT(DISTINCT tr.use_case) FROM test_results tr WHERE tr.run_id = r.run_id) = 1
                    THEN (SELECT tr.use_case FROM test_results tr WHERE tr.run_id = r.run_id LIMIT 1)
                ELSE NULL
            END as display_name
        FROM runs r
        LEFT JOIN suites s ON r.suite_id = s.id
        ORDER BY r.started_at DESC LIMIT 1
        """
    )
    return Run.from_row(row) if row else None


def list_runs(limit: int = 20, offset: int = 0) -> List[Run]:
    """List runs ordered by start time (newest first), with suite name and computed display_name."""
    rows = db.fetchall(
        """
        SELECT r.*, s.suite_name,
            CASE
                WHEN (SELECT COUNT(*) FROM test_results tr WHERE tr.run_id = r.run_id) = 1
                    THEN (SELECT tr.test_id FROM test_results tr WHERE tr.run_id = r.run_id LIMIT 1)
                WHEN (SELECT COUNT(DISTINCT tr.use_case) FROM test_results tr WHERE tr.run_id = r.run_id) = 1
                    THEN (SELECT tr.use_case FROM test_results tr WHERE tr.run_id = r.run_id LIMIT 1)
                ELSE NULL
            END as display_name
        FROM runs r
        LEFT JOIN suites s ON r.suite_id = s.id
        ORDER BY r.started_at DESC
        LIMIT ? OFFSET ?
        """,
        (limit, offset),
    )
    return [Run.from_row(row) for row in rows]


def get_run_summary(run_id: str) -> Optional[RunSummary]:
    """Get a run with summary statistics."""
    run = get_run(run_id)
    if not run:
        return None

    tests = list_test_results(run_id)

    # Calculate live counts
    running_count = sum(1 for t in tests if t.status == TestStatus.RUNNING)
    pending_count = sum(1 for t in tests if t.status == TestStatus.PENDING)

    return RunSummary(
        run=run,
        tests=tests,
        running_count=running_count,
        pending_count=pending_count,
    )


def complete_run(run_id: str, duration_ms: Optional[int] = None) -> None:
    """Mark a run as completed and calculate final stats."""
    tests = list_test_results(run_id)

    passed = sum(1 for t in tests if t.status == TestStatus.PASSED)
    failed = sum(1 for t in tests if t.status == TestStatus.FAILED)
    crashed = sum(1 for t in tests if t.status == TestStatus.CRASHED)
    skipped = sum(1 for t in tests if t.status == TestStatus.SKIPPED)

    # CRASHED tests count as failures for overall status
    total_failed = failed + crashed

    # Determine overall status
    status = RunStatus.COMPLETED if total_failed == 0 else RunStatus.FAILED

    # Calculate duration if not provided
    run = get_run(run_id)
    finished_at = datetime.now()
    if duration_ms is None and run:
        duration_ms = int((finished_at - run.started_at).total_seconds() * 1000)

    update_run(
        run_id,
        status=status,
        finished_at=finished_at,
        passed=passed,
        failed=total_failed,  # Include crashed tests in failed count
        skipped=skipped,
        duration_ms=duration_ms,
    )


def create_run_with_tests(
    suite_id: int,
    tests: List[dict],
    filters: Optional[dict] = None,
    mode: str = "docker",
) -> Run:
    """
    Create a new run with all tests in PENDING state.

    Args:
        suite_id: The suite this run belongs to
        tests: List of test dicts with keys: test_id, use_case, test_case, name, tags
        filters: The filters used to select these tests
        mode: 'docker' or 'standalone'

    Returns:
        The created Run object
    """
    run_id = str(uuid.uuid4())
    started_at = datetime.now()
    total_tests = len(tests)
    filters_json = json.dumps(filters) if filters else None

    # Create the run record
    db.execute(
        """
        INSERT INTO runs (
            run_id, suite_id, started_at, status, total_tests,
            pending_count, running_count, passed, failed, skipped,
            filters, mode
        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        """,
        (
            run_id, suite_id, started_at.isoformat(), RunStatus.PENDING.value,
            total_tests, total_tests, 0, 0, 0, 0, filters_json, mode,
        ),
    )

    # Create test result records (all PENDING)
    for test in tests:
        tags_json = json.dumps(test.get("tags", [])) if test.get("tags") else None
        db.execute(
            """
            INSERT INTO test_results (
                run_id, test_id, use_case, test_case, name, tags, status
            ) VALUES (?, ?, ?, ?, ?, ?, ?)
            """,
            (
                run_id, test["test_id"], test["use_case"], test["test_case"],
                test.get("name"), tags_json, TestStatus.PENDING.value,
            ),
        )

    db.commit()

    return Run(
        run_id=run_id,
        suite_id=suite_id,
        started_at=started_at,
        status=RunStatus.PENDING,
        total_tests=total_tests,
        pending_count=total_tests,
        running_count=0,
        passed=0,
        failed=0,
        skipped=0,
        filters=filters,
        mode=mode,
    )


def start_run(run_id: str) -> None:
    """Mark a run as started (RUNNING status)."""
    db.execute(
        "UPDATE runs SET status = ? WHERE run_id = ?",
        (RunStatus.RUNNING.value, run_id),
    )
    db.commit()


def update_test_status(
    run_id: str,
    test_id: str,
    status: TestStatus,
    duration_ms: Optional[int] = None,
    error_message: Optional[str] = None,
    steps_json: Optional[List[dict]] = None,
    steps_passed: Optional[int] = None,
    steps_failed: Optional[int] = None,
    skip_reason: Optional[str] = None,
) -> Optional[TestResult]:
    """
    Update the status of a specific test in a run.

    This is the main method called by test runners to report progress.
    It also updates the run's counters.

    Idempotent: ignores updates if test is already in a terminal state
    (passed, failed, crashed, skipped). This prevents race conditions in parallel
    execution and ensures crashed tests aren't overwritten.
    """
    # Get current test state
    test = get_test_result_by_test_id(run_id, test_id)
    if not test:
        return None

    old_status = test.status

    # Idempotent - don't update terminal states
    if old_status in TERMINAL_STATES:
        # Already in terminal state, ignore update
        return test

    now = datetime.now()

    # Build update
    updates = ["status = ?"]
    params = [status.value]

    if status == TestStatus.RUNNING and not test.started_at:
        updates.append("started_at = ?")
        params.append(now.isoformat())

    if status in (TestStatus.PASSED, TestStatus.FAILED, TestStatus.CRASHED, TestStatus.SKIPPED):
        updates.append("finished_at = ?")
        params.append(now.isoformat())

    if duration_ms is not None:
        updates.append("duration_ms = ?")
        params.append(duration_ms)

    if error_message is not None:
        updates.append("error_message = ?")
        params.append(error_message)

    if steps_json is not None:
        updates.append("steps_json = ?")
        params.append(json.dumps(steps_json))

    if steps_passed is not None:
        updates.append("steps_passed = ?")
        params.append(steps_passed)

    if steps_failed is not None:
        updates.append("steps_failed = ?")
        params.append(steps_failed)

    if skip_reason is not None:
        updates.append("skip_reason = ?")
        params.append(skip_reason)

    # Update test result
    params.extend([run_id, test_id])
    db.execute(
        f"UPDATE test_results SET {', '.join(updates)} WHERE run_id = ? AND test_id = ?",
        tuple(params),
    )

    # Update run counters based on status change
    _update_run_counters(run_id, old_status, status)

    db.commit()

    return get_test_result_by_test_id(run_id, test_id)


def _update_run_counters(run_id: str, old_status: TestStatus, new_status: TestStatus) -> None:
    """Update run counters when a test status changes."""
    if old_status == new_status:
        return

    # Decrement old status counter
    if old_status == TestStatus.PENDING:
        db.execute("UPDATE runs SET pending_count = pending_count - 1 WHERE run_id = ?", (run_id,))
    elif old_status == TestStatus.RUNNING:
        db.execute("UPDATE runs SET running_count = running_count - 1 WHERE run_id = ?", (run_id,))
    elif old_status == TestStatus.PASSED:
        db.execute("UPDATE runs SET passed = passed - 1 WHERE run_id = ?", (run_id,))
    elif old_status == TestStatus.FAILED:
        db.execute("UPDATE runs SET failed = failed - 1 WHERE run_id = ?", (run_id,))
    elif old_status == TestStatus.CRASHED:
        # CRASHED counts as failed for run statistics
        db.execute("UPDATE runs SET failed = failed - 1 WHERE run_id = ?", (run_id,))
    elif old_status == TestStatus.SKIPPED:
        db.execute("UPDATE runs SET skipped = skipped - 1 WHERE run_id = ?", (run_id,))

    # Increment new status counter
    if new_status == TestStatus.PENDING:
        db.execute("UPDATE runs SET pending_count = pending_count + 1 WHERE run_id = ?", (run_id,))
    elif new_status == TestStatus.RUNNING:
        db.execute("UPDATE runs SET running_count = running_count + 1 WHERE run_id = ?", (run_id,))
    elif new_status == TestStatus.PASSED:
        db.execute("UPDATE runs SET passed = passed + 1 WHERE run_id = ?", (run_id,))
    elif new_status == TestStatus.FAILED:
        db.execute("UPDATE runs SET failed = failed + 1 WHERE run_id = ?", (run_id,))
    elif new_status == TestStatus.CRASHED:
        # CRASHED counts as failed for run statistics
        db.execute("UPDATE runs SET failed = failed + 1 WHERE run_id = ?", (run_id,))
    elif new_status == TestStatus.SKIPPED:
        db.execute("UPDATE runs SET skipped = skipped + 1 WHERE run_id = ?", (run_id,))


def get_tests_by_usecase(run_id: str) -> dict:
    """
    Get tests for a run grouped by use case.

    Returns dict like:
    {
        "uc01_registry": {
            "use_case": "uc01_registry",
            "tests": [TestResult, ...],
            "pending": 2,
            "running": 1,
            "passed": 3,
            "failed": 0,
            "total": 6,
        },
        ...
    }
    """
    tests = list_test_results(run_id)

    grouped = {}
    for test in tests:
        uc = test.use_case
        if uc not in grouped:
            grouped[uc] = {
                "use_case": uc,
                "tests": [],
                "pending": 0,
                "running": 0,
                "passed": 0,
                "failed": 0,
                "crashed": 0,
                "skipped": 0,
                "total": 0,
            }

        grouped[uc]["tests"].append(test)
        grouped[uc]["total"] += 1

        if test.status == TestStatus.PENDING:
            grouped[uc]["pending"] += 1
        elif test.status == TestStatus.RUNNING:
            grouped[uc]["running"] += 1
        elif test.status == TestStatus.PASSED:
            grouped[uc]["passed"] += 1
        elif test.status == TestStatus.FAILED:
            grouped[uc]["failed"] += 1
        elif test.status == TestStatus.CRASHED:
            grouped[uc]["crashed"] += 1
        elif test.status == TestStatus.SKIPPED:
            grouped[uc]["skipped"] += 1

    return grouped


# =============================================================================
# Test Result Operations
# =============================================================================

def create_test_result(
    run_id: str,
    test_id: str,
    use_case: str,
    test_case: str,
    name: Optional[str] = None,
    tags: Optional[List[str]] = None,
) -> TestResult:
    """Create a new test result record."""
    tags_json = json.dumps(tags) if tags else None

    cursor = db.execute(
        """
        INSERT INTO test_results (
            run_id, test_id, use_case, test_case, name, tags, status
        ) VALUES (?, ?, ?, ?, ?, ?, ?)
        """,
        (run_id, test_id, use_case, test_case, name, tags_json, TestStatus.PENDING.value),
    )
    db.commit()

    return TestResult(
        id=cursor.lastrowid,
        run_id=run_id,
        test_id=test_id,
        use_case=use_case,
        test_case=test_case,
        name=name,
        tags=tags or [],
        status=TestStatus.PENDING,
    )


def update_test_result(
    test_result_id: int,
    status: Optional[TestStatus] = None,
    started_at: Optional[datetime] = None,
    finished_at: Optional[datetime] = None,
    duration_ms: Optional[int] = None,
    error_message: Optional[str] = None,
    error_step: Optional[int] = None,
) -> None:
    """
    Update a test result record.

    Idempotent: ignores status updates if test is already in a
    terminal state (passed, failed, crashed, skipped).
    """
    # Check current status for idempotency
    if status is not None:
        current = get_test_result(test_result_id)
        if current and current.status in TERMINAL_STATES:
            # Already in terminal state, ignore status update
            # But still allow other field updates (duration, error_message, etc.)
            status = None

    updates = []
    params = []

    if status is not None:
        updates.append("status = ?")
        params.append(status.value)
    if started_at is not None:
        updates.append("started_at = ?")
        params.append(started_at.isoformat())
    if finished_at is not None:
        updates.append("finished_at = ?")
        params.append(finished_at.isoformat())
    if duration_ms is not None:
        updates.append("duration_ms = ?")
        params.append(duration_ms)
    if error_message is not None:
        updates.append("error_message = ?")
        params.append(error_message)
    if error_step is not None:
        updates.append("error_step = ?")
        params.append(error_step)

    if updates:
        params.append(test_result_id)
        db.execute(
            f"UPDATE test_results SET {', '.join(updates)} WHERE id = ?",
            tuple(params),
        )
        db.commit()


def get_test_result(test_result_id: int) -> Optional[TestResult]:
    """Get a test result by ID."""
    row = db.fetchone("SELECT * FROM test_results WHERE id = ?", (test_result_id,))
    return TestResult.from_row(row) if row else None


def get_test_result_by_test_id(run_id: str, test_id: str) -> Optional[TestResult]:
    """Get a test result by run_id and test_id."""
    row = db.fetchone(
        "SELECT * FROM test_results WHERE run_id = ? AND test_id = ?",
        (run_id, test_id),
    )
    return TestResult.from_row(row) if row else None


def list_test_results(run_id: str) -> List[TestResult]:
    """List all test results for a run."""
    rows = db.fetchall(
        "SELECT * FROM test_results WHERE run_id = ? ORDER BY id",
        (run_id,),
    )
    return [TestResult.from_row(row) for row in rows]


def get_test_detail(test_result_id: int) -> Optional[TestDetail]:
    """Get detailed test result with steps and assertions."""
    test = get_test_result(test_result_id)
    if not test:
        return None

    # First try normalized step_results table
    steps = list_step_results(test_result_id)

    # If no normalized steps, convert from embedded steps_json
    if not steps and test.steps_json:
        steps = _convert_embedded_steps(test_result_id, test.steps_json)

    assertions = list_assertion_results(test_result_id)
    captured = list_captured_values(test_result_id)

    return TestDetail(
        test=test,
        steps=steps,
        assertions=assertions,
        captured=captured,
    )


def _convert_embedded_steps(test_result_id: int, steps_json: List[dict]) -> List[StepResult]:
    """Convert embedded steps_json to StepResult objects."""
    results = []
    for step in steps_json:
        phase = step.get("phase", "test")
        index = step.get("index", 0)
        name = step.get("name")  # Optional step name from YAML
        handler = step.get("handler")  # Handler from step, not result
        result = step.get("result", {})

        # Determine status from result
        success = result.get("success", False)
        status = StepStatus.PASSED if success else StepStatus.FAILED

        # Extract error message
        error_message = result.get("error")
        if not error_message and not success:
            # Try to get error from stderr
            stderr = result.get("stderr", "")
            if stderr:
                error_message = stderr[:500]  # Truncate long errors

        step_result = StepResult(
            id=None,  # Not persisted in normalized table
            test_result_id=test_result_id,
            step_index=index,
            phase=phase,
            handler=handler or result.get("handler", ""),
            description=name,  # Use step name as description
            status=status,
            started_at=None,
            finished_at=None,
            duration_ms=None,
            exit_code=result.get("exit_code"),
            stdout=result.get("stdout"),
            stderr=result.get("stderr"),
            error_message=error_message,
        )
        results.append(step_result)

    return results


# =============================================================================
# Step Result Operations
# =============================================================================

def create_step_result(
    test_result_id: int,
    step_index: int,
    phase: str,
    handler: str,
    description: Optional[str] = None,
) -> StepResult:
    """Create a new step result record."""
    cursor = db.execute(
        """
        INSERT INTO step_results (
            test_result_id, step_index, phase, handler, description, status
        ) VALUES (?, ?, ?, ?, ?, ?)
        """,
        (test_result_id, step_index, phase, handler, description, StepStatus.PENDING.value),
    )
    db.commit()

    return StepResult(
        id=cursor.lastrowid,
        test_result_id=test_result_id,
        step_index=step_index,
        phase=phase,
        handler=handler,
        description=description,
        status=StepStatus.PENDING,
    )


def update_step_result(
    step_result_id: int,
    status: Optional[StepStatus] = None,
    started_at: Optional[datetime] = None,
    finished_at: Optional[datetime] = None,
    duration_ms: Optional[int] = None,
    exit_code: Optional[int] = None,
    stdout: Optional[str] = None,
    stderr: Optional[str] = None,
    error_message: Optional[str] = None,
) -> None:
    """Update a step result record."""
    updates = []
    params = []

    if status is not None:
        updates.append("status = ?")
        params.append(status.value)
    if started_at is not None:
        updates.append("started_at = ?")
        params.append(started_at.isoformat())
    if finished_at is not None:
        updates.append("finished_at = ?")
        params.append(finished_at.isoformat())
    if duration_ms is not None:
        updates.append("duration_ms = ?")
        params.append(duration_ms)
    if exit_code is not None:
        updates.append("exit_code = ?")
        params.append(exit_code)
    if stdout is not None:
        updates.append("stdout = ?")
        params.append(stdout)
    if stderr is not None:
        updates.append("stderr = ?")
        params.append(stderr)
    if error_message is not None:
        updates.append("error_message = ?")
        params.append(error_message)

    if updates:
        params.append(step_result_id)
        db.execute(
            f"UPDATE step_results SET {', '.join(updates)} WHERE id = ?",
            tuple(params),
        )
        db.commit()


def list_step_results(test_result_id: int) -> List[StepResult]:
    """List all step results for a test."""
    rows = db.fetchall(
        "SELECT * FROM step_results WHERE test_result_id = ? ORDER BY phase, step_index",
        (test_result_id,),
    )
    return [StepResult.from_row(row) for row in rows]


# =============================================================================
# Assertion Result Operations
# =============================================================================

def create_assertion_result(
    test_result_id: int,
    assertion_index: int,
    expression: str,
    message: Optional[str] = None,
    passed: bool = False,
    actual_value: Optional[str] = None,
    expected_value: Optional[str] = None,
) -> AssertionResult:
    """Create a new assertion result record."""
    cursor = db.execute(
        """
        INSERT INTO assertion_results (
            test_result_id, assertion_index, expression, message, passed, actual_value, expected_value
        ) VALUES (?, ?, ?, ?, ?, ?, ?)
        """,
        (test_result_id, assertion_index, expression, message, 1 if passed else 0, actual_value, expected_value),
    )
    db.commit()

    return AssertionResult(
        id=cursor.lastrowid,
        test_result_id=test_result_id,
        assertion_index=assertion_index,
        expression=expression,
        message=message,
        passed=passed,
        actual_value=actual_value,
        expected_value=expected_value,
    )


def list_assertion_results(test_result_id: int) -> List[AssertionResult]:
    """List all assertion results for a test."""
    rows = db.fetchall(
        "SELECT * FROM assertion_results WHERE test_result_id = ? ORDER BY assertion_index",
        (test_result_id,),
    )
    return [AssertionResult.from_row(row) for row in rows]


# =============================================================================
# Captured Value Operations
# =============================================================================

def create_captured_value(
    test_result_id: int,
    key: str,
    value: Optional[str] = None,
) -> CapturedValue:
    """Create a new captured value record."""
    captured_at = datetime.now()

    cursor = db.execute(
        """
        INSERT OR REPLACE INTO captured_values (
            test_result_id, key, value, captured_at
        ) VALUES (?, ?, ?, ?)
        """,
        (test_result_id, key, value, captured_at.isoformat()),
    )
    db.commit()

    return CapturedValue(
        id=cursor.lastrowid,
        test_result_id=test_result_id,
        key=key,
        value=value,
        captured_at=captured_at,
    )


def list_captured_values(test_result_id: int) -> List[CapturedValue]:
    """List all captured values for a test."""
    rows = db.fetchall(
        "SELECT * FROM captured_values WHERE test_result_id = ? ORDER BY id",
        (test_result_id,),
    )
    return [CapturedValue.from_row(row) for row in rows]


# =============================================================================
# Statistics & Comparison Queries
# =============================================================================

def get_flaky_tests(limit: int = 20) -> List[dict]:
    """Find tests that have mixed pass/fail results across runs."""
    rows = db.fetchall(
        """
        SELECT
            test_id,
            COUNT(*) as total_runs,
            SUM(CASE WHEN status = 'passed' THEN 1 ELSE 0 END) as passes,
            SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END) as failures
        FROM test_results
        GROUP BY test_id
        HAVING passes > 0 AND failures > 0
        ORDER BY failures DESC
        LIMIT ?
        """,
        (limit,),
    )
    return [dict(row) for row in rows]


def get_slowest_tests(limit: int = 10) -> List[dict]:
    """Find tests with highest average duration."""
    rows = db.fetchall(
        """
        SELECT
            test_id,
            name,
            AVG(duration_ms) as avg_duration_ms,
            COUNT(*) as run_count
        FROM test_results
        WHERE status = 'passed' AND duration_ms IS NOT NULL
        GROUP BY test_id
        ORDER BY avg_duration_ms DESC
        LIMIT ?
        """,
        (limit,),
    )
    return [dict(row) for row in rows]


def compare_runs(run_id_1: str, run_id_2: str) -> List[dict]:
    """Compare test results between two runs."""
    rows = db.fetchall(
        """
        SELECT
            t1.test_id,
            t1.status as run1_status,
            t1.duration_ms as run1_duration_ms,
            t2.status as run2_status,
            t2.duration_ms as run2_duration_ms,
            CASE
                WHEN t1.status = t2.status THEN 'same'
                WHEN t1.status = 'passed' AND t2.status = 'failed' THEN 'regression'
                WHEN t1.status = 'failed' AND t2.status = 'passed' THEN 'fixed'
                ELSE 'changed'
            END as change_type
        FROM test_results t1
        JOIN test_results t2 ON t1.test_id = t2.test_id
        WHERE t1.run_id = ? AND t2.run_id = ?
        ORDER BY
            CASE change_type
                WHEN 'regression' THEN 1
                WHEN 'fixed' THEN 2
                WHEN 'changed' THEN 3
                ELSE 4
            END,
            t1.test_id
        """,
        (run_id_1, run_id_2),
    )
    return [dict(row) for row in rows]


def get_run_stats() -> dict:
    """Get aggregate statistics across all runs."""
    row = db.fetchone(
        """
        SELECT
            COUNT(*) as total_runs,
            SUM(total_tests) as total_tests_executed,
            SUM(passed) as total_passed,
            SUM(failed) as total_failed,
            AVG(duration_ms) as avg_run_duration_ms
        FROM runs
        WHERE status IN ('completed', 'failed')
        """
    )
    return dict(row) if row else {}


# =============================================================================
# Suite Operations
# =============================================================================

def create_suite(
    folder_path: str,
    suite_name: str,
    mode: SuiteMode = SuiteMode.DOCKER,
    config_json: Optional[str] = None,
    test_count: int = 0,
) -> Suite:
    """Create a new test suite record."""
    now = datetime.now()

    cursor = db.execute(
        """
        INSERT INTO suites (
            folder_path, suite_name, mode, config_json, test_count,
            last_synced_at, created_at, updated_at
        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
        """,
        (
            folder_path, suite_name, mode.value, config_json, test_count,
            now.isoformat(), now.isoformat(), now.isoformat(),
        ),
    )
    db.commit()

    return Suite(
        id=cursor.lastrowid,
        folder_path=folder_path,
        suite_name=suite_name,
        mode=mode,
        config_json=config_json,
        test_count=test_count,
        last_synced_at=now,
        created_at=now,
        updated_at=now,
    )


def upsert_suite(
    folder_path: str,
    suite_name: str,
    mode: SuiteMode = SuiteMode.DOCKER,
    config_json: Optional[str] = None,
    test_count: int = 0,
) -> Suite:
    """Create or update a suite by folder_path."""
    existing = get_suite_by_path(folder_path)

    if existing:
        update_suite(
            existing.id,
            suite_name=suite_name,
            mode=mode,
            config_json=config_json,
            test_count=test_count,
        )
        return get_suite(existing.id)
    else:
        return create_suite(
            folder_path=folder_path,
            suite_name=suite_name,
            mode=mode,
            config_json=config_json,
            test_count=test_count,
        )


def update_suite(
    suite_id: int,
    suite_name: Optional[str] = None,
    mode: Optional[SuiteMode] = None,
    config_json: Optional[str] = None,
    test_count: Optional[int] = None,
) -> None:
    """Update a suite record."""
    updates = ["updated_at = ?"]
    params = [datetime.now().isoformat()]

    if suite_name is not None:
        updates.append("suite_name = ?")
        params.append(suite_name)
    if mode is not None:
        updates.append("mode = ?")
        params.append(mode.value)
    if config_json is not None:
        updates.append("config_json = ?")
        params.append(config_json)
        updates.append("last_synced_at = ?")
        params.append(datetime.now().isoformat())
    if test_count is not None:
        updates.append("test_count = ?")
        params.append(test_count)

    params.append(suite_id)
    db.execute(
        f"UPDATE suites SET {', '.join(updates)} WHERE id = ?",
        tuple(params),
    )
    db.commit()


def get_suite(suite_id: int) -> Optional[Suite]:
    """Get a suite by ID."""
    row = db.fetchone("SELECT * FROM suites WHERE id = ?", (suite_id,))
    return Suite.from_row(row) if row else None


def get_suite_by_path(folder_path: str) -> Optional[Suite]:
    """Get a suite by folder path."""
    row = db.fetchone("SELECT * FROM suites WHERE folder_path = ?", (folder_path,))
    return Suite.from_row(row) if row else None


def list_suites() -> List[Suite]:
    """List all registered suites."""
    rows = db.fetchall("SELECT * FROM suites ORDER BY suite_name")
    return [Suite.from_row(row) for row in rows]


def delete_suite(suite_id: int) -> bool:
    """Delete a suite by ID. Returns True if deleted."""
    cursor = db.execute("DELETE FROM suites WHERE id = ?", (suite_id,))
    db.commit()
    return cursor.rowcount > 0


# =============================================================================
# Cancellation Operations
# =============================================================================

def request_cancel(run_id: str) -> None:
    """Set the cancel_requested flag on a run."""
    db.execute(
        "UPDATE runs SET cancel_requested = 1 WHERE run_id = ?",
        (run_id,),
    )
    db.commit()


def is_cancel_requested(run_id: str) -> bool:
    """Check if cancellation has been requested for a run."""
    row = db.fetchone(
        "SELECT cancel_requested FROM runs WHERE run_id = ?",
        (run_id,),
    )
    return bool(row["cancel_requested"]) if row else False


def cancel_run_with_skip(run_id: str, skip_reason: str = "Run cancelled") -> int:
    """
    Mark all pending tests as skipped and complete the run as cancelled.

    Returns the number of tests that were skipped.
    """
    # Get all pending tests
    pending_tests = db.fetchall(
        "SELECT test_id FROM test_results WHERE run_id = ? AND status = ?",
        (run_id, TestStatus.PENDING.value),
    )

    skipped_count = 0
    for row in pending_tests:
        update_test_status(
            run_id=run_id,
            test_id=row["test_id"],
            status=TestStatus.SKIPPED,
            skip_reason=skip_reason,
        )
        skipped_count += 1

    # Mark the run as cancelled
    db.execute(
        """
        UPDATE runs SET
            status = ?,
            finished_at = ?
        WHERE run_id = ?
        """,
        (RunStatus.CANCELLED.value, datetime.now().isoformat(), run_id),
    )
    db.commit()

    return skipped_count
