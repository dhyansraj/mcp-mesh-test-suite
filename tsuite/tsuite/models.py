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
    SKIPPED = "skipped"


class StepStatus(Enum):
    PENDING = "pending"
    RUNNING = "running"
    PASSED = "passed"
    FAILED = "failed"
    SKIPPED = "skipped"


@dataclass
class Run:
    """Represents a test run session."""
    run_id: str
    started_at: datetime
    finished_at: Optional[datetime] = None
    status: RunStatus = RunStatus.PENDING
    cli_version: Optional[str] = None
    sdk_python_version: Optional[str] = None
    sdk_typescript_version: Optional[str] = None
    docker_image: Optional[str] = None
    total_tests: int = 0
    passed: int = 0
    failed: int = 0
    skipped: int = 0
    duration_ms: Optional[int] = None

    def to_dict(self) -> dict:
        return {
            "run_id": self.run_id,
            "started_at": self.started_at.isoformat() if self.started_at else None,
            "finished_at": self.finished_at.isoformat() if self.finished_at else None,
            "status": self.status.value,
            "cli_version": self.cli_version,
            "sdk_python_version": self.sdk_python_version,
            "sdk_typescript_version": self.sdk_typescript_version,
            "docker_image": self.docker_image,
            "total_tests": self.total_tests,
            "passed": self.passed,
            "failed": self.failed,
            "skipped": self.skipped,
            "duration_ms": self.duration_ms,
        }

    @classmethod
    def from_row(cls, row) -> "Run":
        return cls(
            run_id=row["run_id"],
            started_at=datetime.fromisoformat(row["started_at"]) if row["started_at"] else None,
            finished_at=datetime.fromisoformat(row["finished_at"]) if row["finished_at"] else None,
            status=RunStatus(row["status"]),
            cli_version=row["cli_version"],
            sdk_python_version=row["sdk_python_version"],
            sdk_typescript_version=row["sdk_typescript_version"],
            docker_image=row["docker_image"],
            total_tests=row["total_tests"] or 0,
            passed=row["passed"] or 0,
            failed=row["failed"] or 0,
            skipped=row["skipped"] or 0,
            duration_ms=row["duration_ms"],
        )


@dataclass
class TestResult:
    """Represents a test case result."""
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
        }

    @classmethod
    def from_row(cls, row) -> "TestResult":
        tags = []
        if row["tags"]:
            try:
                tags = json.loads(row["tags"])
            except json.JSONDecodeError:
                tags = []

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

    def to_dict(self) -> dict:
        return {
            "id": self.id,
            "test_result_id": self.test_result_id,
            "assertion_index": self.assertion_index,
            "expression": self.expression,
            "message": self.message,
            "passed": self.passed,
            "actual_value": self.actual_value,
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
