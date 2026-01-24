"""
Client library for container <-> server communication.

Used by handlers and test scripts running inside containers to
communicate with the runner server on the host.

Usage:
    from tsuite.client import RunnerClient

    client = RunnerClient()  # reads TSUITE_API from env
    version = client.get_config("packages.cli_version")
    client.set_state("agent_port", 3000)
    client.progress(step=2, message="Installing dependencies")
"""

import os
from typing import Any

import requests


class RunnerClient:
    """
    Client for communicating with the runner server.

    Reads server URL from TSUITE_API environment variable.
    Reads test ID from TSUITE_TEST_ID environment variable.
    """

    def __init__(self, api_url: str | None = None, test_id: str | None = None):
        """
        Initialize the client.

        Args:
            api_url: Server URL (defaults to TSUITE_API env var)
            test_id: Test ID (defaults to TSUITE_TEST_ID env var)
        """
        self.api_url = api_url or os.environ.get("TSUITE_API", "http://localhost:9999")
        self.test_id = test_id or os.environ.get("TSUITE_TEST_ID", "")
        self._timeout = 10  # seconds

    def _get(self, path: str) -> Any:
        """Make a GET request."""
        url = f"{self.api_url}{path}"
        try:
            response = requests.get(url, timeout=self._timeout)
            response.raise_for_status()
            return response.json()
        except requests.RequestException as e:
            return None

    def _post(self, path: str, data: dict) -> bool:
        """Make a POST request."""
        url = f"{self.api_url}{path}"
        try:
            response = requests.post(url, json=data, timeout=self._timeout)
            response.raise_for_status()
            return True
        except requests.RequestException:
            return False

    # Health check
    def health(self) -> bool:
        """Check if server is healthy."""
        result = self._get("/health")
        return result is not None and result.get("status") == "ok"

    # Configuration (uses /api/runner/* endpoints)
    def get_config(self, path: str | None = None) -> Any:
        """
        Get configuration value.

        Args:
            path: Dot-notation path (e.g., "packages.cli_version")
                  If None, returns entire config.

        Returns:
            Configuration value or None if not found.
        """
        if path is None:
            return self._get("/api/runner/config")

        # Convert dot notation to URL path
        url_path = path.replace(".", "/")
        result = self._get(f"/api/runner/config/{url_path}")
        return result.get("value") if result else None

    # Routines (uses /api/runner/* endpoints)
    def get_routine(self, scope: str, name: str) -> dict | None:
        """Get routine definition by scope and name."""
        return self._get(f"/api/runner/routine/{scope}/{name}")

    def get_all_routines(self) -> dict:
        """Get all routines."""
        return self._get("/api/runner/routines") or {}

    # State management (uses /api/runner/* endpoints)
    def get_state(self, test_id: str | None = None) -> dict:
        """
        Get state from a test.

        Args:
            test_id: Test ID (defaults to current test)

        Returns:
            State dictionary.
        """
        tid = test_id or self.test_id
        return self._get(f"/api/runner/state/{tid}") or {}

    def set_state(self, key: str, value: Any, test_id: str | None = None) -> bool:
        """
        Set a state value for a test.

        Args:
            key: State key
            value: State value
            test_id: Test ID (defaults to current test)

        Returns:
            True if successful.
        """
        tid = test_id or self.test_id
        return self._post(f"/api/runner/state/{tid}", {key: value})

    def update_state(self, state: dict, test_id: str | None = None) -> bool:
        """
        Merge state into a test.

        Args:
            state: Dictionary to merge
            test_id: Test ID (defaults to current test)

        Returns:
            True if successful.
        """
        tid = test_id or self.test_id
        return self._post(f"/api/runner/state/{tid}", state)

    # Captured variables (uses /api/runner/* endpoints)
    def capture(self, name: str, value: Any, test_id: str | None = None) -> bool:
        """
        Store a captured variable.

        Args:
            name: Variable name
            value: Variable value
            test_id: Test ID (defaults to current test)

        Returns:
            True if successful.
        """
        tid = test_id or self.test_id
        return self._post(f"/api/runner/capture/{tid}", {name: value})

    # Progress reporting (uses /api/runner/* endpoints)
    def progress(
        self,
        step: int,
        status: str = "running",
        message: str = "",
        test_id: str | None = None,
    ) -> bool:
        """
        Report progress for a test.

        Args:
            step: Current step number
            status: Status string (running, completed, failed)
            message: Optional message
            test_id: Test ID (defaults to current test)

        Returns:
            True if successful.
        """
        tid = test_id or self.test_id
        return self._post(f"/api/runner/progress/{tid}", {
            "step": step,
            "status": status,
            "message": message,
        })

    def get_progress(self, test_id: str | None = None) -> dict:
        """Get progress for a test."""
        tid = test_id or self.test_id
        return self._get(f"/api/runner/progress/{tid}") or {}

    # Logging (uses /api/runner/* endpoints)
    def log(
        self,
        message: str,
        level: str = "info",
        test_id: str | None = None,
    ) -> bool:
        """
        Send a log message.

        Args:
            message: Log message
            level: Log level (debug, info, warning, error)
            test_id: Test ID (defaults to current test)

        Returns:
            True if successful.
        """
        tid = test_id or self.test_id
        return self._post(f"/api/runner/log/{tid}", {
            "level": level,
            "message": message,
        })

    def debug(self, message: str) -> bool:
        """Log a debug message."""
        return self.log(message, "debug")

    def info(self, message: str) -> bool:
        """Log an info message."""
        return self.log(message, "info")

    def warning(self, message: str) -> bool:
        """Log a warning message."""
        return self.log(message, "warning")

    def error(self, message: str) -> bool:
        """Log an error message."""
        return self.log(message, "error")

    # Context (uses /api/runner/* endpoints)
    def get_context(self, test_id: str | None = None) -> dict:
        """Get full test context."""
        tid = test_id or self.test_id
        return self._get(f"/api/runner/context/{tid}") or {}

    # =========================================================================
    # Test Status Reporting Methods
    # =========================================================================

    def _patch(self, path: str, data: dict) -> dict | None:
        """Make a PATCH request."""
        url = f"{self.api_url}{path}"
        try:
            response = requests.patch(url, json=data, timeout=self._timeout)
            response.raise_for_status()
            return response.json()
        except requests.RequestException:
            return None

    def report_test_running(
        self,
        run_id: str,
        test_id: str | None = None,
    ) -> dict | None:
        """
        Report that a test has started running.

        Args:
            run_id: The run ID
            test_id: Test ID (defaults to TSUITE_TEST_ID env var)

        Returns:
            Updated test result dict or None on error.
        """
        tid = test_id or self.test_id
        return self._patch(f"/api/runs/{run_id}/tests/{tid}", {
            "status": "running",
        })

    def report_test_passed(
        self,
        run_id: str,
        test_id: str | None = None,
        duration_ms: int | None = None,
        steps_passed: int | None = None,
        steps_failed: int | None = None,
        steps: list | None = None,
        assertions: list | None = None,
    ) -> dict | None:
        """
        Report that a test has passed.

        Args:
            run_id: The run ID
            test_id: Test ID (defaults to TSUITE_TEST_ID env var)
            duration_ms: Test duration in milliseconds
            steps_passed: Number of steps that passed
            steps_failed: Number of steps that failed
            steps: Detailed step results
            assertions: Assertion results with actual/expected values

        Returns:
            Updated test result dict or None on error.
        """
        tid = test_id or self.test_id
        data = {"status": "passed"}
        if duration_ms is not None:
            data["duration_ms"] = duration_ms
        if steps_passed is not None:
            data["steps_passed"] = steps_passed
        if steps_failed is not None:
            data["steps_failed"] = steps_failed
        if steps is not None:
            data["steps"] = steps
        if assertions is not None:
            data["assertions"] = assertions
        return self._patch(f"/api/runs/{run_id}/tests/{tid}", data)

    def report_test_failed(
        self,
        run_id: str,
        test_id: str | None = None,
        duration_ms: int | None = None,
        error_message: str | None = None,
        steps_passed: int | None = None,
        steps_failed: int | None = None,
        steps: list | None = None,
        assertions: list | None = None,
    ) -> dict | None:
        """
        Report that a test has failed.

        Args:
            run_id: The run ID
            test_id: Test ID (defaults to TSUITE_TEST_ID env var)
            duration_ms: Test duration in milliseconds
            error_message: Error message
            steps_passed: Number of steps that passed
            steps_failed: Number of steps that failed
            steps: Detailed step results
            assertions: Assertion results with actual/expected values

        Returns:
            Updated test result dict or None on error.
        """
        tid = test_id or self.test_id
        data = {"status": "failed"}
        if duration_ms is not None:
            data["duration_ms"] = duration_ms
        if error_message is not None:
            data["error_message"] = error_message
        if steps_passed is not None:
            data["steps_passed"] = steps_passed
        if steps_failed is not None:
            data["steps_failed"] = steps_failed
        if steps is not None:
            data["steps"] = steps
        if assertions is not None:
            data["assertions"] = assertions
        return self._patch(f"/api/runs/{run_id}/tests/{tid}", data)

    def report_test_skipped(
        self,
        run_id: str,
        test_id: str | None = None,
        skip_reason: str | None = None,
    ) -> dict | None:
        """
        Report that a test was skipped.

        Args:
            run_id: The run ID
            test_id: Test ID (defaults to TSUITE_TEST_ID env var)
            skip_reason: Reason for skipping

        Returns:
            Updated test result dict or None on error.
        """
        tid = test_id or self.test_id
        data = {"status": "skipped"}
        if skip_reason is not None:
            data["skip_reason"] = skip_reason
        return self._patch(f"/api/runs/{run_id}/tests/{tid}", data)

    def report_test_status(
        self,
        run_id: str,
        status: str,
        test_id: str | None = None,
        duration_ms: int | None = None,
        error_message: str | None = None,
        skip_reason: str | None = None,
        steps_passed: int | None = None,
        steps_failed: int | None = None,
        steps: list | None = None,
        assertions: list | None = None,
    ) -> dict | None:
        """
        Generic method to report test status.

        Args:
            run_id: The run ID
            status: Test status (running, passed, failed, skipped)
            test_id: Test ID (defaults to TSUITE_TEST_ID env var)
            duration_ms: Test duration in milliseconds
            error_message: Error message (for failed tests)
            skip_reason: Reason for skipping (for skipped tests)
            steps_passed: Number of steps that passed
            steps_failed: Number of steps that failed
            steps: Detailed step results
            assertions: Assertion results with actual/expected values

        Returns:
            Updated test result dict or None on error.
        """
        tid = test_id or self.test_id
        data = {"status": status}
        if duration_ms is not None:
            data["duration_ms"] = duration_ms
        if error_message is not None:
            data["error_message"] = error_message
        if skip_reason is not None:
            data["skip_reason"] = skip_reason
        if steps_passed is not None:
            data["steps_passed"] = steps_passed
        if steps_failed is not None:
            data["steps_failed"] = steps_failed
        if steps is not None:
            data["steps"] = steps
        if assertions is not None:
            data["assertions"] = assertions
        return self._patch(f"/api/runs/{run_id}/tests/{tid}", data)

    def start_run(self, run_id: str) -> dict | None:
        """
        Signal that a run has started.

        Args:
            run_id: The run ID

        Returns:
            Response dict or None on error.
        """
        url = f"{self.api_url}/api/runs/{run_id}/start"
        try:
            response = requests.post(url, timeout=self._timeout)
            response.raise_for_status()
            return response.json()
        except requests.RequestException:
            return None

    def complete_run(
        self,
        run_id: str,
        duration_ms: int | None = None,
    ) -> dict | None:
        """
        Signal that a run has completed.

        Args:
            run_id: The run ID
            duration_ms: Total run duration in milliseconds

        Returns:
            Response dict or None on error.
        """
        url = f"{self.api_url}/api/runs/{run_id}/complete"
        data = {}
        if duration_ms is not None:
            data["duration_ms"] = duration_ms
        try:
            response = requests.post(url, json=data, timeout=self._timeout)
            response.raise_for_status()
            return response.json()
        except requests.RequestException:
            return None

    def get_run(self, run_id: str) -> dict | None:
        """
        Get run details.

        Args:
            run_id: The run ID

        Returns:
            Run dict or None on error.
        """
        return self._get(f"/api/runs/{run_id}")

    def get_run_tests(self, run_id: str) -> dict | None:
        """
        Get all tests for a run.

        Args:
            run_id: The run ID

        Returns:
            Dict with tests list or None on error.
        """
        return self._get(f"/api/runs/{run_id}/tests")

    def get_run_tests_tree(self, run_id: str) -> dict | None:
        """
        Get tests grouped by use case (tree structure).

        Args:
            run_id: The run ID

        Returns:
            Dict with use_cases list or None on error.
        """
        return self._get(f"/api/runs/{run_id}/tests/tree")


# Convenience instance
_default_client: RunnerClient | None = None


def get_client() -> RunnerClient:
    """Get the default client instance."""
    global _default_client
    if _default_client is None:
        _default_client = RunnerClient()
    return _default_client
