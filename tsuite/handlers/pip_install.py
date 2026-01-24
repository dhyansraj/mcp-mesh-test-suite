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
      sdk_python_version: "0.8.0b9"  # Version for published mode
      local:
        wheels_dir: "/wheels"   # Directory containing local .whl files

Flow:
    1. Run `pip install -r requirements.txt` to install baseline dependencies
    2. Override mcp-mesh packages with target version:
       - local mode: pip install --force-reinstall /wheels/mcp_mesh*.whl
       - published mode: pip install --force-reinstall mcp-mesh=={version}
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
    Install Python dependencies from requirements.txt, then override mcp-mesh packages.

    Steps:
    1. Run pip install -r requirements.txt (baseline)
    2. Override mcp-mesh packages based on mode:
       - local: pip install --force-reinstall --no-deps /wheels/mcp_mesh*.whl
       - published: pip install --force-reinstall mcp-mesh=={version}
    """
    path = step.get("path", "/workspace")
    venv = step.get("venv", "/workspace/.venv")
    config = context.get("config", {})

    packages_config = config.get("packages", {})
    mode = packages_config.get("mode", "auto")
    wheels_dir = packages_config.get("local", {}).get("wheels_dir", "/wheels")
    sdk_version = packages_config.get("sdk_python_version", "")

    # Auto-detect mode
    original_mode = mode
    if mode == "auto":
        if os.path.exists(wheels_dir) and _has_whl_files(wheels_dir):
            mode = "local"
        else:
            mode = "published"

    pip_bin = f"{venv}/bin/pip"
    requirements_file = Path(path) / "requirements.txt"

    if not requirements_file.exists():
        return success(f"No requirements.txt found in {path} (mode={mode})")

    # Build informative header
    output_lines = [
        "=" * 60,
        "pip-install handler",
        "=" * 60,
        f"  Path: {path}",
        f"  Venv: {venv}",
        f"  Mode: {mode}" + (f" (auto-detected from '{original_mode}')" if original_mode == "auto" else ""),
    ]

    if mode == "local":
        output_lines.append(f"  Wheels dir: {wheels_dir}")
        wheels = _find_all_mcpmesh_wheels(wheels_dir)
        output_lines.append(f"  Local wheels: {', '.join(os.path.basename(w) for w in wheels) if wheels else 'none'}")
    else:
        output_lines.append(f"  Target version: {sdk_version or 'not specified'}")

    output_lines.append("-" * 60)

    # Step 1: Run baseline pip install
    try:
        cmd = [pip_bin, "install", "-r", str(requirements_file)]
        result = subprocess.run(cmd, capture_output=True, text=True, timeout=300)

        output_lines.append("Step 1: pip install -r requirements.txt (baseline)")
        output_lines.append(result.stdout)

        if result.returncode != 0:
            return StepResult(
                exit_code=result.returncode,
                stdout="\n".join(output_lines),
                stderr=result.stderr,
                success=False,
                error=f"pip install failed: {result.stderr}",
            )

    except subprocess.TimeoutExpired:
        return failure("pip install timed out after 300s", exit_code=124)
    except Exception as e:
        return failure(f"pip install failed: {e}")

    # Step 2: Override mcp-mesh packages
    try:
        if mode == "local":
            # Find all mcp_mesh wheels in wheels_dir
            wheels = _find_all_mcpmesh_wheels(wheels_dir)
            if wheels:
                output_lines.append(f"Step 2: Override with local wheels: {', '.join(os.path.basename(w) for w in wheels)}")
                cmd = [pip_bin, "install", "--force-reinstall", "--no-deps"] + wheels
                result = subprocess.run(cmd, capture_output=True, text=True, timeout=300)
                output_lines.append(result.stdout)

                if result.returncode != 0:
                    return StepResult(
                        exit_code=result.returncode,
                        stdout="\n".join(output_lines),
                        stderr=result.stderr,
                        success=False,
                        error=f"pip install (local override) failed: {result.stderr}",
                    )
            else:
                output_lines.append("Step 2: No mcp_mesh wheels found in wheels_dir, skipping override")

        elif mode == "published" and sdk_version:
            # Install specific version from PyPI
            packages_to_install = [f"mcp-mesh=={sdk_version}"]
            output_lines.append(f"Step 2: Override with published packages: {', '.join(packages_to_install)}")
            cmd = [pip_bin, "install", "--force-reinstall"] + packages_to_install
            result = subprocess.run(cmd, capture_output=True, text=True, timeout=300)
            output_lines.append(result.stdout)

            if result.returncode != 0:
                return StepResult(
                    exit_code=result.returncode,
                    stdout="\n".join(output_lines),
                    stderr=result.stderr,
                    success=False,
                    error=f"pip install (published override) failed: {result.stderr}",
                )
        else:
            output_lines.append("Step 2: No override configured, using baseline versions")

    except subprocess.TimeoutExpired:
        return failure("pip install (override) timed out after 300s", exit_code=124)
    except Exception as e:
        return failure(f"pip install (override) failed: {e}")

    return StepResult(
        exit_code=0,
        stdout="\n".join(output_lines),
        stderr="",
        success=True,
    )


def _has_whl_files(wheels_dir: str) -> bool:
    """Check if directory contains any .whl files."""
    try:
        return any(
            f.endswith('.whl')
            for f in os.listdir(wheels_dir)
            if os.path.isfile(os.path.join(wheels_dir, f))
        )
    except OSError:
        return False


def _find_all_mcpmesh_wheels(wheels_dir: str) -> list[str]:
    """Find all mcp_mesh wheels in the wheels directory."""
    wheels = []
    try:
        for file in os.listdir(wheels_dir):
            if file.startswith("mcp_mesh") and file.endswith(".whl"):
                wheels.append(os.path.join(wheels_dir, file))
    except OSError:
        pass
    return sorted(wheels)
