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

    # Configuration
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
            return self._get("/config")

        # Convert dot notation to URL path
        url_path = path.replace(".", "/")
        result = self._get(f"/config/{url_path}")
        return result.get("value") if result else None

    # Routines
    def get_routine(self, scope: str, name: str) -> dict | None:
        """Get routine definition by scope and name."""
        return self._get(f"/routine/{scope}/{name}")

    def get_all_routines(self) -> dict:
        """Get all routines."""
        return self._get("/routines") or {}

    # State management
    def get_state(self, test_id: str | None = None) -> dict:
        """
        Get state from a test.

        Args:
            test_id: Test ID (defaults to current test)

        Returns:
            State dictionary.
        """
        tid = test_id or self.test_id
        return self._get(f"/state/{tid}") or {}

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
        return self._post(f"/state/{tid}", {key: value})

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
        return self._post(f"/state/{tid}", state)

    # Captured variables
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
        return self._post(f"/capture/{tid}", {name: value})

    # Progress reporting
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
        return self._post(f"/progress/{tid}", {
            "step": step,
            "status": status,
            "message": message,
        })

    def get_progress(self, test_id: str | None = None) -> dict:
        """Get progress for a test."""
        tid = test_id or self.test_id
        return self._get(f"/progress/{tid}") or {}

    # Logging
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
        return self._post(f"/log/{tid}", {
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

    # Context
    def get_context(self, test_id: str | None = None) -> dict:
        """Get full test context."""
        tid = test_id or self.test_id
        return self._get(f"/context/{tid}") or {}


# Convenience instance
_default_client: RunnerClient | None = None


def get_client() -> RunnerClient:
    """Get the default client instance."""
    global _default_client
    if _default_client is None:
        _default_client = RunnerClient()
    return _default_client
