"""
Shell command handler.

Executes shell commands and captures output.

Step configuration:
    handler: shell
    command: "echo 'hello world'"
    workdir: /workspace  # optional
    timeout: 60  # optional, seconds
    capture: output_var  # optional, capture stdout to variable
    ignore_errors: false  # optional
"""

import subprocess
import os
from pathlib import Path

import sys
sys.path.insert(0, str(__file__).rsplit("/handlers", 1)[0])

from tsuite.context import StepResult
from .base import success, failure


def execute(step: dict, context: dict) -> StepResult:
    """Execute a shell command."""
    command = step.get("command")
    if not command:
        return failure("Shell handler requires 'command' parameter")

    workdir = step.get("workdir", context.get("workdir", "."))
    timeout = step.get("timeout", 120)
    env = os.environ.copy()

    # Add context to environment
    env["TSUITE_API"] = context.get("server_url", "")
    env["TSUITE_TEST_ID"] = context.get("test_id", "")

    # Add any additional env from step
    if "env" in step:
        env.update(step["env"])

    try:
        result = subprocess.run(
            command,
            shell=True,
            executable="/bin/bash",  # Use bash explicitly for bashisms like &>
            cwd=workdir,
            capture_output=True,
            text=True,
            timeout=timeout,
            env=env,
        )

        return StepResult(
            exit_code=result.returncode,
            stdout=result.stdout,
            stderr=result.stderr,
            success=result.returncode == 0,
            error=None if result.returncode == 0 else f"Exit code: {result.returncode}",
        )

    except subprocess.TimeoutExpired:
        return failure(f"Command timed out after {timeout}s", exit_code=124)
    except Exception as e:
        return failure(f"Command execution failed: {e}")
