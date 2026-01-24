"""
Runtime context management for test execution.

The context holds all state during a test run:
- Configuration
- Test metadata
- Captured values
- Shared state between tests
- Last step results
"""

from dataclasses import dataclass, field
from typing import Any
from pathlib import Path
import threading


@dataclass
class StepResult:
    """Result of a single step execution."""
    exit_code: int = 0
    stdout: str = ""
    stderr: str = ""
    duration: float = 0.0
    success: bool = True
    error: str | None = None


@dataclass
class TestContext:
    """Context for a single test execution."""
    test_id: str
    test_name: str
    uc: str
    tc: str
    workdir: Path
    captured: dict[str, Any] = field(default_factory=dict)
    state: dict[str, Any] = field(default_factory=dict)
    steps: dict[str, dict] = field(default_factory=dict)  # Step results by capture name
    last: StepResult = field(default_factory=StepResult)

    def to_dict(self) -> dict:
        """Convert to dictionary for API responses."""
        return {
            "test_id": self.test_id,
            "test_name": self.test_name,
            "uc": self.uc,
            "tc": self.tc,
            "workdir": str(self.workdir),
            "captured": self.captured,
            "state": self.state,
            "steps": self.steps,
            "last": {
                "exit_code": self.last.exit_code,
                "stdout": self.last.stdout,
                "stderr": self.last.stderr,
                "duration": self.last.duration,
                "success": self.last.success,
                "error": self.last.error,
            }
        }


class RuntimeContext:
    """
    Global runtime context for the test suite.

    Thread-safe singleton that holds:
    - Loaded configuration
    - All test contexts
    - Progress information
    - Routine definitions
    """

    _instance = None
    _lock = threading.Lock()

    def __new__(cls):
        if cls._instance is None:
            with cls._lock:
                if cls._instance is None:
                    cls._instance = super().__new__(cls)
                    cls._instance._initialized = False
        return cls._instance

    def __init__(self):
        if self._initialized:
            return

        self._initialized = True
        self._config: dict = {}
        self._routines: dict[str, dict] = {}  # scope -> {name -> routine}
        self._tests: dict[str, TestContext] = {}  # test_id -> TestContext
        self._progress: dict[str, dict] = {}  # test_id -> progress info
        self._lock = threading.Lock()

    def reset(self):
        """Reset all state (useful for testing)."""
        with self._lock:
            self._config = {}
            self._routines = {}
            self._tests = {}
            self._progress = {}

    # Configuration
    def set_config(self, config: dict):
        """Set the loaded configuration."""
        with self._lock:
            self._config = config

    def get_config(self, path: str | None = None) -> Any:
        """
        Get configuration value.

        Args:
            path: Dot-notation path (e.g., "packages.cli_version")
                  If None, returns entire config.
        """
        if path is None:
            return self._config

        parts = path.split(".")
        value = self._config
        for part in parts:
            if isinstance(value, dict) and part in value:
                value = value[part]
            else:
                return None
        return value

    # Routines
    def set_routines(self, scope: str, routines: dict):
        """Set routines for a scope (global, uc, tc)."""
        with self._lock:
            self._routines[scope] = routines

    def get_routine(self, scope: str, name: str) -> dict | None:
        """Get a routine definition."""
        return self._routines.get(scope, {}).get(name)

    def get_all_routines(self) -> dict:
        """Get all routines."""
        return self._routines.copy()

    # Test contexts
    def create_test_context(self, test_id: str, test_name: str, workdir: Path) -> TestContext:
        """Create a new test context."""
        parts = test_id.split("/")
        uc = parts[0] if len(parts) > 0 else ""
        tc = parts[1] if len(parts) > 1 else ""

        ctx = TestContext(
            test_id=test_id,
            test_name=test_name,
            uc=uc,
            tc=tc,
            workdir=workdir,
        )

        with self._lock:
            self._tests[test_id] = ctx

        return ctx

    def get_test_context(self, test_id: str) -> TestContext | None:
        """Get test context by ID."""
        return self._tests.get(test_id)

    def get_test_state(self, test_id: str) -> dict:
        """Get state from a test."""
        ctx = self._tests.get(test_id)
        return ctx.state if ctx else {}

    def update_test_state(self, test_id: str, state: dict):
        """Merge state into a test context."""
        with self._lock:
            if test_id in self._tests:
                self._tests[test_id].state.update(state)

    def set_captured(self, test_id: str, name: str, value: Any):
        """Set a captured variable for a test."""
        with self._lock:
            if test_id in self._tests:
                self._tests[test_id].captured[name] = value

    # Progress
    def update_progress(self, test_id: str, step: int, status: str, message: str = ""):
        """Update progress for a test."""
        with self._lock:
            self._progress[test_id] = {
                "step": step,
                "status": status,
                "message": message,
            }

    def get_progress(self, test_id: str) -> dict:
        """Get progress for a test."""
        return self._progress.get(test_id, {})


# Global instance
runtime = RuntimeContext()
