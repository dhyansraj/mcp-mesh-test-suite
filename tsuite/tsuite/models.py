"""
Data models for test reporting.
"""

from dataclasses import dataclass, field
from datetime import datetime
from typing import Optional, List
from enum import Enum
import json


class RunStatus(Enum):
    PENDING = "pending"
    RUNNING = "running"
    COMPLETED = "completed"
    FAILED = "failed"
    CANCELLED = "cancelled"


class TestStatus(Enum):
    PENDING = "pending"
    RUNNING = "running"
    PASSED = "passed"
    FAILED = "failed"
    CRASHED = "crashed"  # subprocess/container died unexpectedly
    SKIPPED = "skipped"


# Terminal states that cannot be overwritten (idempotent updates)
TERMINAL_STATES = {TestStatus.PASSED, TestStatus.FAILED, TestStatus.CRASHED, TestStatus.SKIPPED}


class StepStatus(Enum):
    PENDING = "pending"
    RUNNING = "running"
    PASSED = "passed"
    FAILED = "failed"
    SKIPPED = "skipped"


class SuiteMode(Enum):
    DOCKER = "docker"
    STANDALONE = "standalone"


@dataclass
class Run:
    """Represents a test run session."""
    run_id: str
    started_at: datetime
    finished_at: Optional[datetime] = None
    status: RunStatus = RunStatus.PENDING
    suite_id: Optional[int] = None
    suite_name: Optional[str] = None  # Populated from join with suites table
    display_name: Optional[str] = None  # Computed: tc name, uc name, or suite name
    cli_version: Optional[str] = None
    sdk_python_version: Optional[str] = None
    sdk_typescript_version: Optional[str] = None
    docker_image: Optional[str] = None
    total_tests: int = 0
    pending_count: int = 0
    running_count: int = 0
    passed: int = 0
    failed: int = 0
    skipped: int = 0
    duration_ms: Optional[int] = None
    filters: Optional[dict] = None
    mode: str = "docker"
    cancel_requested: bool = False

    def to_dict(self) -> dict:
        return {
            "run_id": self.run_id,
            "suite_id": self.suite_id,
            "suite_name": self.suite_name,
            "display_name": self.display_name,
            "started_at": self.started_at.isoformat() if self.started_at else None,
            "finished_at": self.finished_at.isoformat() if self.finished_at else None,
            "status": self.status.value,
            "cli_version": self.cli_version,
            "sdk_python_version": self.sdk_python_version,
            "sdk_typescript_version": self.sdk_typescript_version,
            "docker_image": self.docker_image,
            "total_tests": self.total_tests,
            "pending_count": self.pending_count,
            "running_count": self.running_count,
            "passed": self.passed,
            "failed": self.failed,
            "skipped": self.skipped,
            "duration_ms": self.duration_ms,
            "filters": self.filters,
            "mode": self.mode,
            "cancel_requested": self.cancel_requested,
        }

    @classmethod
    def from_row(cls, row) -> "Run":
        filters = None
        if row["filters"]:
            try:
                filters = json.loads(row["filters"])
            except json.JSONDecodeError:
                filters = None

        # Handle suite_name from joined query (may not be present in all queries)
        suite_name = None
        try:
            suite_name = row["suite_name"]
        except (KeyError, IndexError):
            pass

        # Handle cancel_requested (may not exist in older DBs before migration)
        cancel_requested = False
        try:
            cancel_requested = bool(row["cancel_requested"])
        except (KeyError, IndexError):
            pass

        # Handle display_name from computed query (may not be present in all queries)
        display_name = None
        try:
            display_name = row["display_name"]
        except (KeyError, IndexError):
            pass

        return cls(
            run_id=row["run_id"],
            suite_id=row["suite_id"],
            suite_name=suite_name,
            display_name=display_name,
            started_at=datetime.fromisoformat(row["started_at"]) if row["started_at"] else None,
            finished_at=datetime.fromisoformat(row["finished_at"]) if row["finished_at"] else None,
            status=RunStatus(row["status"]),
            cli_version=row["cli_version"],
            sdk_python_version=row["sdk_python_version"],
            sdk_typescript_version=row["sdk_typescript_version"],
            docker_image=row["docker_image"],
            total_tests=row["total_tests"] or 0,
            pending_count=row["pending_count"] or 0,
            running_count=row["running_count"] or 0,
            passed=row["passed"] or 0,
            failed=row["failed"] or 0,
            skipped=row["skipped"] or 0,
            duration_ms=row["duration_ms"],
            filters=filters,
            mode=row["mode"] or "docker",
            cancel_requested=cancel_requested,
        )


@dataclass
class TestResult:
    """Represents a test case result (also used for live tracking)."""
    id: Optional[int] = None
    run_id: str = ""
    test_id: str = ""
    use_case: str = ""
    test_case: str = ""
    name: Optional[str] = None
    tags: List[str] = field(default_factory=list)
    status: TestStatus = TestStatus.PENDING
    started_at: Optional[datetime] = None
    finished_at: Optional[datetime] = None
    duration_ms: Optional[int] = None
    error_message: Optional[str] = None
    error_step: Optional[int] = None
    skip_reason: Optional[str] = None
    steps_json: Optional[List[dict]] = None
    steps_passed: int = 0
    steps_failed: int = 0

    def to_dict(self) -> dict:
        return {
            "id": self.id,
            "run_id": self.run_id,
            "test_id": self.test_id,
            "use_case": self.use_case,
            "test_case": self.test_case,
            "name": self.name,
            "tags": self.tags,
            "status": self.status.value,
            "started_at": self.started_at.isoformat() if self.started_at else None,
            "finished_at": self.finished_at.isoformat() if self.finished_at else None,
            "duration_ms": self.duration_ms,
            "error_message": self.error_message,
            "error_step": self.error_step,
            "skip_reason": self.skip_reason,
            "steps": self.steps_json,
            "steps_passed": self.steps_passed,
            "steps_failed": self.steps_failed,
        }

    @classmethod
    def from_row(cls, row) -> "TestResult":
        tags = []
        if row["tags"]:
            try:
                tags = json.loads(row["tags"])
            except json.JSONDecodeError:
                tags = []

        steps_json = None
        if row["steps_json"]:
            try:
                steps_json = json.loads(row["steps_json"])
            except json.JSONDecodeError:
                steps_json = None

        return cls(
            id=row["id"],
            run_id=row["run_id"],
            test_id=row["test_id"],
            use_case=row["use_case"],
            test_case=row["test_case"],
            name=row["name"],
            tags=tags,
            status=TestStatus(row["status"]),
            started_at=datetime.fromisoformat(row["started_at"]) if row["started_at"] else None,
            finished_at=datetime.fromisoformat(row["finished_at"]) if row["finished_at"] else None,
            duration_ms=row["duration_ms"],
            error_message=row["error_message"],
            error_step=row["error_step"],
            skip_reason=row["skip_reason"],
            steps_json=steps_json,
            steps_passed=row["steps_passed"] or 0,
            steps_failed=row["steps_failed"] or 0,
        )


@dataclass
class StepResult:
    """Represents a step execution result."""
    id: Optional[int] = None
    test_result_id: int = 0
    step_index: int = 0
    phase: str = "test"  # pre_run, test, post_run
    handler: str = ""
    description: Optional[str] = None
    status: StepStatus = StepStatus.PENDING
    started_at: Optional[datetime] = None
    finished_at: Optional[datetime] = None
    duration_ms: Optional[int] = None
    exit_code: Optional[int] = None
    stdout: Optional[str] = None
    stderr: Optional[str] = None
    error_message: Optional[str] = None

    def to_dict(self) -> dict:
        return {
            "id": self.id,
            "test_result_id": self.test_result_id,
            "step_index": self.step_index,
            "phase": self.phase,
            "handler": self.handler,
            "description": self.description,
            "status": self.status.value,
            "started_at": self.started_at.isoformat() if self.started_at else None,
            "finished_at": self.finished_at.isoformat() if self.finished_at else None,
            "duration_ms": self.duration_ms,
            "exit_code": self.exit_code,
            "stdout": self.stdout,
            "stderr": self.stderr,
            "error_message": self.error_message,
        }

    @classmethod
    def from_row(cls, row) -> "StepResult":
        return cls(
            id=row["id"],
            test_result_id=row["test_result_id"],
            step_index=row["step_index"],
            phase=row["phase"],
            handler=row["handler"],
            description=row["description"],
            status=StepStatus(row["status"]),
            started_at=datetime.fromisoformat(row["started_at"]) if row["started_at"] else None,
            finished_at=datetime.fromisoformat(row["finished_at"]) if row["finished_at"] else None,
            duration_ms=row["duration_ms"],
            exit_code=row["exit_code"],
            stdout=row["stdout"],
            stderr=row["stderr"],
            error_message=row["error_message"],
        )


@dataclass
class AssertionResult:
    """Represents an assertion result."""
    id: Optional[int] = None
    test_result_id: int = 0
    assertion_index: int = 0
    expression: str = ""
    message: Optional[str] = None
    passed: bool = False
    actual_value: Optional[str] = None
    expected_value: Optional[str] = None

    def to_dict(self) -> dict:
        return {
            "id": self.id,
            "test_result_id": self.test_result_id,
            "assertion_index": self.assertion_index,
            "expression": self.expression,
            "message": self.message,
            "passed": self.passed,
            "actual_value": self.actual_value,
            "expected_value": self.expected_value,
        }

    @classmethod
    def from_row(cls, row) -> "AssertionResult":
        return cls(
            id=row["id"],
            test_result_id=row["test_result_id"],
            assertion_index=row["assertion_index"],
            expression=row["expression"],
            message=row["message"],
            passed=bool(row["passed"]),
            actual_value=row["actual_value"],
            expected_value=row.get("expected_value") if hasattr(row, "get") else row["expected_value"] if "expected_value" in row.keys() else None,
        )


@dataclass
class CapturedValue:
    """Represents a captured value during test execution."""
    id: Optional[int] = None
    test_result_id: int = 0
    key: str = ""
    value: Optional[str] = None
    captured_at: Optional[datetime] = None

    def to_dict(self) -> dict:
        return {
            "id": self.id,
            "test_result_id": self.test_result_id,
            "key": self.key,
            "value": self.value,
            "captured_at": self.captured_at.isoformat() if self.captured_at else None,
        }

    @classmethod
    def from_row(cls, row) -> "CapturedValue":
        return cls(
            id=row["id"],
            test_result_id=row["test_result_id"],
            key=row["key"],
            value=row["value"],
            captured_at=datetime.fromisoformat(row["captured_at"]) if row["captured_at"] else None,
        )


@dataclass
class Suite:
    """Represents a registered test suite."""
    id: Optional[int] = None
    folder_path: str = ""
    suite_name: str = ""
    mode: SuiteMode = SuiteMode.DOCKER
    config_json: Optional[str] = None
    test_count: int = 0
    last_synced_at: Optional[datetime] = None
    created_at: Optional[datetime] = None
    updated_at: Optional[datetime] = None

    def to_dict(self) -> dict:
        return {
            "id": self.id,
            "folder_path": self.folder_path,
            "suite_name": self.suite_name,
            "mode": self.mode.value,
            "config_json": self.config_json,
            "config": json.loads(self.config_json) if self.config_json else None,
            "test_count": self.test_count,
            "last_synced_at": self.last_synced_at.isoformat() if self.last_synced_at else None,
            "created_at": self.created_at.isoformat() if self.created_at else None,
            "updated_at": self.updated_at.isoformat() if self.updated_at else None,
        }

    @classmethod
    def from_row(cls, row) -> "Suite":
        return cls(
            id=row["id"],
            folder_path=row["folder_path"],
            suite_name=row["suite_name"],
            mode=SuiteMode(row["mode"]),
            config_json=row["config_json"],
            test_count=row["test_count"] or 0,
            last_synced_at=datetime.fromisoformat(row["last_synced_at"]) if row["last_synced_at"] else None,
            created_at=datetime.fromisoformat(row["created_at"]) if row["created_at"] else None,
            updated_at=datetime.fromisoformat(row["updated_at"]) if row["updated_at"] else None,
        )


@dataclass
class RunSummary:
    """Summary of a test run with aggregated stats."""
    run: Run
    tests: List[TestResult] = field(default_factory=list)
    running_count: int = 0
    pending_count: int = 0

    def to_dict(self) -> dict:
        return {
            **self.run.to_dict(),
            "tests": [t.to_dict() for t in self.tests],
            "running_count": self.running_count,
            "pending_count": self.pending_count,
        }


@dataclass
class TestDetail:
    """Detailed view of a test result with steps and assertions."""
    test: TestResult
    steps: List[StepResult] = field(default_factory=list)
    assertions: List[AssertionResult] = field(default_factory=list)
    captured: List[CapturedValue] = field(default_factory=list)

    def to_dict(self) -> dict:
        return {
            **self.test.to_dict(),
            "steps": [s.to_dict() for s in self.steps],
            "assertions": [a.to_dict() for a in self.assertions],
            "captured": [c.to_dict() for c in self.captured],
        }
