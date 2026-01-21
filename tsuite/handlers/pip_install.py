"""
Pip install handler.

Installs Python dependencies with support for local/published package modes.

Step configuration:
    handler: pip-install
    path: /workspace/agent      # Directory with requirements.txt
    venv: /workspace/.venv      # Optional, defaults to /workspace/.venv

Context configuration (from suite config.yaml):
    packages:
      mode: "auto"              # "local", "published", or "auto"
      local:
        wheels_dir: "/wheels"   # Directory containing local .whl files
"""

import subprocess
import os
from pathlib import Path

import sys
sys.path.insert(0, str(__file__).rsplit("/handlers", 1)[0])

from tsuite.context import StepResult
from .base import success, failure


def execute(step: dict, context: dict) -> StepResult:
    """
    Install Python dependencies from requirements.txt.

    Checks config.packages.mode:
    - "local": Uses --find-links with wheels directory
    - "published": Normal pip install from PyPI
    - "auto": Auto-detect (local if wheels dir exists and non-empty)
    """
    path = step.get("path", "/workspace")
    venv = step.get("venv", "/workspace/.venv")
    config = context.get("config", {})

    packages_config = config.get("packages", {})
    mode = packages_config.get("mode", "auto")
    wheels_dir = packages_config.get("local", {}).get("wheels_dir", "/wheels")

    # Auto-detect mode
    if mode == "auto":
        if os.path.exists(wheels_dir) and os.listdir(wheels_dir):
            mode = "local"
        else:
            mode = "published"

    pip_bin = f"{venv}/bin/pip"
    requirements_file = Path(path) / "requirements.txt"

    if not requirements_file.exists():
        return success(f"No requirements.txt found in {path} (mode={mode})")

    # Build command
    cmd = [pip_bin, "install", "-r", str(requirements_file)]

    if mode == "local":
        cmd.extend(["--find-links", wheels_dir])
        # Prefer local packages but fallback to PyPI for other deps
        cmd.extend(["--extra-index-url", "https://pypi.org/simple/"])

    try:
        result = subprocess.run(cmd, capture_output=True, text=True, timeout=300)

        if result.returncode == 0:
            return StepResult(
                exit_code=0,
                stdout=f"[mode={mode}] {result.stdout}",
                stderr=result.stderr,
                success=True,
            )
        else:
            return StepResult(
                exit_code=result.returncode,
                stdout=result.stdout,
                stderr=result.stderr,
                success=False,
                error=f"pip install failed (mode={mode}): {result.stderr}",
            )

    except subprocess.TimeoutExpired:
        return failure("pip install timed out after 300s", exit_code=124)
    except Exception as e:
        return failure(f"pip install failed: {e}")
