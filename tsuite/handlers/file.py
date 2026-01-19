"""
File operation handler.

Handles file-related operations: exists, read, write, delete.

Step configurations:

Check if file exists:
    handler: file
    operation: exists
    path: /workspace/test-agent/main.py

Read file content:
    handler: file
    operation: read
    path: /workspace/output.json
    capture: file_content

Write file:
    handler: file
    operation: write
    path: /workspace/config.json
    content: '{"key": "value"}'

Delete file:
    handler: file
    operation: delete
    path: /workspace/temp.txt
"""

from pathlib import Path

import sys
sys.path.insert(0, str(__file__).rsplit("/handlers", 1)[0])

from tsuite.context import StepResult
from .base import success, failure


def execute(step: dict, context: dict) -> StepResult:
    """Execute a file operation."""
    operation = step.get("operation", "exists")
    path_str = step.get("path")

    if not path_str:
        return failure("File handler requires 'path' parameter")

    # Resolve path relative to workdir if not absolute
    if not path_str.startswith("/"):
        workdir = context.get("workdir", ".")
        path = Path(workdir) / path_str
    else:
        path = Path(path_str)

    if operation == "exists":
        return _check_exists(path)

    elif operation == "read":
        return _read_file(path)

    elif operation == "write":
        content = step.get("content", "")
        return _write_file(path, content)

    elif operation == "delete":
        return _delete_file(path)

    elif operation == "mkdir":
        return _mkdir(path)

    else:
        return failure(f"Unknown file operation: {operation}")


def _check_exists(path: Path) -> StepResult:
    """Check if a file or directory exists."""
    exists = path.exists()
    return StepResult(
        exit_code=0 if exists else 1,
        stdout=str(exists),
        stderr="",
        success=exists,
        error=None if exists else f"Path does not exist: {path}",
    )


def _read_file(path: Path) -> StepResult:
    """Read file contents."""
    try:
        content = path.read_text()
        return success(stdout=content)
    except FileNotFoundError:
        return failure(f"File not found: {path}")
    except PermissionError:
        return failure(f"Permission denied: {path}")
    except Exception as e:
        return failure(f"Failed to read file: {e}")


def _write_file(path: Path, content: str) -> StepResult:
    """Write content to file."""
    try:
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(content)
        return success(stdout=f"Wrote {len(content)} bytes to {path}")
    except PermissionError:
        return failure(f"Permission denied: {path}")
    except Exception as e:
        return failure(f"Failed to write file: {e}")


def _delete_file(path: Path) -> StepResult:
    """Delete a file."""
    try:
        if path.is_file():
            path.unlink()
            return success(stdout=f"Deleted {path}")
        elif path.is_dir():
            import shutil
            shutil.rmtree(path)
            return success(stdout=f"Deleted directory {path}")
        else:
            return success(stdout=f"Path does not exist: {path}")
    except PermissionError:
        return failure(f"Permission denied: {path}")
    except Exception as e:
        return failure(f"Failed to delete: {e}")


def _mkdir(path: Path) -> StepResult:
    """Create a directory."""
    try:
        path.mkdir(parents=True, exist_ok=True)
        return success(stdout=f"Created directory {path}")
    except PermissionError:
        return failure(f"Permission denied: {path}")
    except Exception as e:
        return failure(f"Failed to create directory: {e}")
