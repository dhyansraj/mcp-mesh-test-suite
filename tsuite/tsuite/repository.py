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
    TestResult, TestStatus, TestDetail,
    StepResult, StepStatus,
    AssertionResult,
    CapturedValue,
)


# =============================================================================
# Run Operations
# =============================================================================

def create_run(
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
            run_id, started_at, status, cli_version,
            sdk_python_version, sdk_typescript_version,
            docker_image, total_tests
        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
        """,
        (
            run_id, started_at.isoformat(), RunStatus.RUNNING.value,
            cli_version, sdk_python_version, sdk_typescript_version,
            docker_image, total_tests,
        ),
    )
    db.commit()

    return Run(
        run_id=run_id,
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
    """Get a run by ID."""
    row = db.fetchone("SELECT * FROM runs WHERE run_id = ?", (run_id,))
    return Run.from_row(row) if row else None


def get_latest_run() -> Optional[Run]:
    """Get the most recent run."""
    row = db.fetchone("SELECT * FROM runs ORDER BY started_at DESC LIMIT 1")
    return Run.from_row(row) if row else None


def list_runs(limit: int = 20, offset: int = 0) -> List[Run]:
    """List runs ordered by start time (newest first)."""
    rows = db.fetchall(
        "SELECT * FROM runs ORDER BY started_at DESC LIMIT ? OFFSET ?",
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


def complete_run(run_id: str) -> None:
    """Mark a run as completed and calculate final stats."""
    tests = list_test_results(run_id)

    passed = sum(1 for t in tests if t.status == TestStatus.PASSED)
    failed = sum(1 for t in tests if t.status == TestStatus.FAILED)
    skipped = sum(1 for t in tests if t.status == TestStatus.SKIPPED)

    # Determine overall status
    status = RunStatus.COMPLETED if failed == 0 else RunStatus.FAILED

    # Calculate duration
    run = get_run(run_id)
    finished_at = datetime.now()
    duration_ms = int((finished_at - run.started_at).total_seconds() * 1000) if run else None

    update_run(
        run_id,
        status=status,
        finished_at=finished_at,
        passed=passed,
        failed=failed,
        skipped=skipped,
        duration_ms=duration_ms,
    )


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
    """Update a test result record."""
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

    steps = list_step_results(test_result_id)
    assertions = list_assertion_results(test_result_id)
    captured = list_captured_values(test_result_id)

    return TestDetail(
        test=test,
        steps=steps,
        assertions=assertions,
        captured=captured,
    )


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
) -> AssertionResult:
    """Create a new assertion result record."""
    cursor = db.execute(
        """
        INSERT INTO assertion_results (
            test_result_id, assertion_index, expression, message, passed, actual_value
        ) VALUES (?, ?, ?, ?, ?, ?)
        """,
        (test_result_id, assertion_index, expression, message, 1 if passed else 0, actual_value),
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
