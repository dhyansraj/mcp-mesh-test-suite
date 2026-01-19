"""
Routine invocation handler.

This handler is a passthrough - routine execution is handled by the StepExecutor.
The handler exists to provide the interface, but actual routine execution
happens in executor.py via _execute_routine().

Step configuration:
    routine: global.install_meshctl
    params:
      version: "0.7.21"
"""

import sys
sys.path.insert(0, str(__file__).rsplit("/handlers", 1)[0])

from tsuite.context import StepResult
from .base import success


def execute(step: dict, context: dict) -> StepResult:
    """
    Routine handler placeholder.

    Note: Actual routine execution is handled by StepExecutor._execute_routine().
    This handler should not be called directly.
    """
    # This should never be called directly - routines are handled specially
    # by the StepExecutor. If we get here, something is wrong.
    routine_name = step.get("routine", "unknown")
    return success(stdout=f"Routine '{routine_name}' executed (placeholder)")
