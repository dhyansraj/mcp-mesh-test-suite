"""
Wait handler.

Implements various wait operations: fixed delays, HTTP polling, file watching.

Step configurations:

Wait for seconds:
    handler: wait
    seconds: 5

Wait for HTTP endpoint to be ready:
    handler: wait
    type: http
    url: http://localhost:3000/health
    timeout: 30
    interval: 2

Wait for file to exist:
    handler: wait
    type: file
    path: /workspace/output.json
    timeout: 60
    interval: 1

Wait for condition (polling shell command):
    handler: wait
    type: command
    command: "curl -s localhost:3000/health | grep -q 'ok'"
    timeout: 30
    interval: 2
"""

import time
import subprocess
from pathlib import Path

import sys
sys.path.insert(0, str(__file__).rsplit("/handlers", 1)[0])

from tsuite.context import StepResult
from .base import success, failure


def execute(step: dict, context: dict) -> StepResult:
    """Execute a wait operation."""
    wait_type = step.get("type", "seconds")

    if wait_type == "seconds" or "seconds" in step:
        return _wait_seconds(step)
    elif wait_type == "http":
        return _wait_http(step)
    elif wait_type == "file":
        return _wait_file(step, context)
    elif wait_type == "command":
        return _wait_command(step, context)
    else:
        return failure(f"Unknown wait type: {wait_type}")


def _wait_seconds(step: dict) -> StepResult:
    """Wait for a fixed number of seconds."""
    seconds = step.get("seconds", 1)
    time.sleep(seconds)
    return success(stdout=f"Waited {seconds} seconds")


def _wait_http(step: dict) -> StepResult:
    """Wait for an HTTP endpoint to be ready."""
    try:
        import requests
    except ImportError:
        return failure("requests library not installed")

    url = step.get("url")
    if not url:
        return failure("HTTP wait requires 'url' parameter")

    timeout = step.get("timeout", 30)
    interval = step.get("interval", 2)
    expect_status = step.get("expect_status", [200, 201, 204])

    if isinstance(expect_status, int):
        expect_status = [expect_status]

    start_time = time.time()
    last_error = None

    while time.time() - start_time < timeout:
        try:
            response = requests.get(url, timeout=min(5, interval))
            if response.status_code in expect_status:
                elapsed = time.time() - start_time
                return success(
                    stdout=f"URL {url} ready after {elapsed:.1f}s (status: {response.status_code})"
                )
            last_error = f"Status {response.status_code}"
        except requests.ConnectionError:
            last_error = "Connection refused"
        except requests.Timeout:
            last_error = "Request timeout"
        except Exception as e:
            last_error = str(e)

        time.sleep(interval)

    return failure(f"URL {url} not ready after {timeout}s (last error: {last_error})")


def _wait_file(step: dict, context: dict) -> StepResult:
    """Wait for a file to exist."""
    path_str = step.get("path")
    if not path_str:
        return failure("File wait requires 'path' parameter")

    # Resolve relative paths
    if not path_str.startswith("/"):
        workdir = context.get("workdir", ".")
        path = Path(workdir) / path_str
    else:
        path = Path(path_str)

    timeout = step.get("timeout", 60)
    interval = step.get("interval", 1)

    start_time = time.time()

    while time.time() - start_time < timeout:
        if path.exists():
            elapsed = time.time() - start_time
            return success(stdout=f"File {path} exists after {elapsed:.1f}s")
        time.sleep(interval)

    return failure(f"File {path} not found after {timeout}s")


def _wait_command(step: dict, context: dict) -> StepResult:
    """Wait for a command to succeed."""
    command = step.get("command")
    if not command:
        return failure("Command wait requires 'command' parameter")

    timeout = step.get("timeout", 30)
    interval = step.get("interval", 2)
    workdir = context.get("workdir", ".")

    start_time = time.time()
    last_error = None

    while time.time() - start_time < timeout:
        try:
            result = subprocess.run(
                command,
                shell=True,
                executable="/bin/bash",  # Use bash explicitly for bashisms like &>
                cwd=workdir,
                capture_output=True,
                text=True,
                timeout=min(10, interval + 5),
            )

            if result.returncode == 0:
                elapsed = time.time() - start_time
                return success(
                    stdout=f"Command succeeded after {elapsed:.1f}s\n{result.stdout}"
                )

            last_error = f"Exit code {result.returncode}"
        except subprocess.TimeoutExpired:
            last_error = "Command timeout"
        except Exception as e:
            last_error = str(e)

        time.sleep(interval)

    return failure(f"Command did not succeed after {timeout}s (last error: {last_error})")
