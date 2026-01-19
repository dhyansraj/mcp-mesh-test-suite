"""
Base handler interface and utilities.

All handlers should implement:
    execute(step: dict, context: dict) -> StepResult
"""

from dataclasses import dataclass
from typing import Any, Protocol

import sys
sys.path.insert(0, str(__file__).rsplit("/handlers", 1)[0])

from tsuite.context import StepResult


class Handler(Protocol):
    """Protocol for handler implementations."""

    def execute(self, step: dict, context: dict) -> StepResult:
        """
        Execute a step.

        Args:
            step: Step configuration from YAML (already interpolated)
            context: Execution context with config, state, captured, last, etc.

        Returns:
            StepResult with exit_code, stdout, stderr, success, error
        """
        ...


def success(stdout: str = "", stderr: str = "") -> StepResult:
    """Create a successful step result."""
    return StepResult(
        exit_code=0,
        stdout=stdout,
        stderr=stderr,
        success=True,
    )


def failure(error: str, exit_code: int = 1, stdout: str = "", stderr: str = "") -> StepResult:
    """Create a failed step result."""
    return StepResult(
        exit_code=exit_code,
        stdout=stdout,
        stderr=stderr,
        success=False,
        error=error,
    )
