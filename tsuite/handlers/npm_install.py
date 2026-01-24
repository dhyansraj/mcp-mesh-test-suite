"""
Npm install handler.

Installs Node.js dependencies with support for local/published package modes.

Step configuration:
    handler: npm-install
    path: /workspace/agent      # Directory with package.json

Context configuration (from suite config.yaml):
    packages:
      mode: "auto"              # "local", "published", or "auto"
      sdk_typescript_version: "0.8.0-beta.9"  # Version for published mode
      local:
        packages_dir: "/packages"  # Directory containing local .tgz files

Flow:
    1. Run `npm install` to install baseline dependencies from package.json
    2. Override @mcpmesh/* packages with target version:
       - local mode: install all .tgz files from packages_dir
       - published mode: install specific version from config
"""

import subprocess
import os
import glob
from pathlib import Path

import sys
sys.path.insert(0, str(__file__).rsplit("/handlers", 1)[0])

from tsuite.context import StepResult
from .base import success, failure


def execute(step: dict, context: dict) -> StepResult:
    """
    Install Node.js dependencies from package.json, then override @mcpmesh/* packages.

    Steps:
    1. Run npm install (baseline - installs whatever versions are in package.json)
    2. Override @mcpmesh/* packages based on mode:
       - local: npm install /packages/mcpmesh-*.tgz
       - published: npm install @mcpmesh/sdk@{version} @mcpmesh/core@{version}
    """
    path = step.get("path", "/workspace")
    config = context.get("config", {})

    packages_config = config.get("packages", {})
    mode = packages_config.get("mode", "auto")
    packages_dir = packages_config.get("local", {}).get("packages_dir", "/packages")
    sdk_version = packages_config.get("sdk_typescript_version", "")

    # Auto-detect mode
    original_mode = mode
    if mode == "auto":
        if os.path.exists(packages_dir) and _has_tgz_files(packages_dir):
            mode = "local"
        else:
            mode = "published"

    package_json_path = Path(path) / "package.json"

    if not package_json_path.exists():
        return success(f"No package.json found in {path} (mode={mode})")

    # Build informative header
    output_lines = [
        "=" * 60,
        "npm-install handler",
        "=" * 60,
        f"  Path: {path}",
        f"  Mode: {mode}" + (f" (auto-detected from '{original_mode}')" if original_mode == "auto" else ""),
    ]

    if mode == "local":
        output_lines.append(f"  Packages dir: {packages_dir}")
        tarballs = _find_all_mcpmesh_tarballs(packages_dir)
        output_lines.append(f"  Local tarballs: {', '.join(os.path.basename(t) for t in tarballs) if tarballs else 'none'}")
    else:
        output_lines.append(f"  Target version: {sdk_version or 'not specified'}")

    output_lines.append("-" * 60)

    # Step 1: Run baseline npm install
    try:
        result = subprocess.run(
            ["npm", "install"],
            cwd=path,
            capture_output=True,
            text=True,
            timeout=300,
        )

        output_lines.append("Step 1: npm install (baseline)")
        output_lines.append(result.stdout)

        if result.returncode != 0:
            return StepResult(
                exit_code=result.returncode,
                stdout="\n".join(output_lines),
                stderr=result.stderr,
                success=False,
                error=f"npm install failed: {result.stderr}",
            )

    except subprocess.TimeoutExpired:
        return failure("npm install timed out after 300s", exit_code=124)
    except Exception as e:
        return failure(f"npm install failed: {e}")

    # Step 2: Override @mcpmesh/* packages
    try:
        if mode == "local":
            # Find all mcpmesh tarballs in packages_dir
            tarballs = _find_all_mcpmesh_tarballs(packages_dir)
            if tarballs:
                output_lines.append(f"Step 2: Override with local packages: {', '.join(os.path.basename(t) for t in tarballs)}")
                result = subprocess.run(
                    ["npm", "install", "--save"] + tarballs,
                    cwd=path,
                    capture_output=True,
                    text=True,
                    timeout=300,
                )
                output_lines.append(result.stdout)

                if result.returncode != 0:
                    return StepResult(
                        exit_code=result.returncode,
                        stdout="\n".join(output_lines),
                        stderr=result.stderr,
                        success=False,
                        error=f"npm install (local override) failed: {result.stderr}",
                    )
            else:
                output_lines.append("Step 2: No mcpmesh tarballs found in packages_dir, skipping override")

        elif mode == "published" and sdk_version:
            # Install specific versions from npm registry
            packages_to_install = [
                f"@mcpmesh/sdk@{sdk_version}",
                f"@mcpmesh/core@{sdk_version}",
            ]
            output_lines.append(f"Step 2: Override with published packages: {', '.join(packages_to_install)}")
            result = subprocess.run(
                ["npm", "install", "--save"] + packages_to_install,
                cwd=path,
                capture_output=True,
                text=True,
                timeout=300,
            )
            output_lines.append(result.stdout)

            if result.returncode != 0:
                return StepResult(
                    exit_code=result.returncode,
                    stdout="\n".join(output_lines),
                    stderr=result.stderr,
                    success=False,
                    error=f"npm install (published override) failed: {result.stderr}",
                )
        else:
            output_lines.append("Step 2: No override configured, using baseline versions")

    except subprocess.TimeoutExpired:
        return failure("npm install (override) timed out after 300s", exit_code=124)
    except Exception as e:
        return failure(f"npm install (override) failed: {e}")

    return StepResult(
        exit_code=0,
        stdout="\n".join(output_lines),
        stderr="",
        success=True,
    )


def _has_tgz_files(packages_dir: str) -> bool:
    """Check if directory contains any .tgz files."""
    try:
        return any(
            f.endswith('.tgz')
            for f in os.listdir(packages_dir)
            if os.path.isfile(os.path.join(packages_dir, f))
        )
    except OSError:
        return False


def _find_all_mcpmesh_tarballs(packages_dir: str) -> list[str]:
    """Find all mcpmesh tarballs in the packages directory."""
    tarballs = []
    try:
        for file in os.listdir(packages_dir):
            if file.startswith("mcpmesh-") and file.endswith(".tgz"):
                tarballs.append(os.path.join(packages_dir, file))
    except OSError:
        pass
    return sorted(tarballs)
