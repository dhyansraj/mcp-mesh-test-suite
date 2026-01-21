"""
Npm install handler.

Installs Node.js dependencies with support for local/published package modes.

Step configuration:
    handler: npm-install
    path: /workspace/agent      # Directory with package.json

Context configuration (from suite config.yaml):
    packages:
      mode: "auto"              # "local", "published", or "auto"
      local:
        packages_dir: "/packages"  # Directory containing local .tgz files
"""

import subprocess
import os
import json
from pathlib import Path

import sys
sys.path.insert(0, str(__file__).rsplit("/handlers", 1)[0])

from tsuite.context import StepResult
from .base import success, failure


def execute(step: dict, context: dict) -> StepResult:
    """
    Install Node.js dependencies from package.json.

    Checks config.packages.mode:
    - "local": Replaces @mcpmesh/* versions with file: references
    - "published": Normal npm install from registry
    - "auto": Auto-detect (local if packages dir exists and has .tgz files)
    """
    path = step.get("path", "/workspace")
    config = context.get("config", {})

    packages_config = config.get("packages", {})
    mode = packages_config.get("mode", "auto")
    packages_dir = packages_config.get("local", {}).get("packages_dir", "/packages")

    # Auto-detect mode
    if mode == "auto":
        if os.path.exists(packages_dir) and _has_tgz_files(packages_dir):
            mode = "local"
        else:
            mode = "published"

    package_json_path = Path(path) / "package.json"

    if not package_json_path.exists():
        return success(f"No package.json found in {path} (mode={mode})")

    # If local mode, modify package.json to use file: references
    modified_deps = []
    if mode == "local":
        try:
            with open(package_json_path) as f:
                package_data = json.load(f)

            for dep_type in ["dependencies", "devDependencies"]:
                deps = package_data.get(dep_type, {})
                for pkg_name, version in list(deps.items()):
                    if pkg_name.startswith("@mcpmesh/"):
                        # Find matching tarball
                        tarball = _find_tarball(packages_dir, pkg_name)
                        if tarball:
                            deps[pkg_name] = f"file:{tarball}"
                            modified_deps.append(f"{pkg_name} -> file:{tarball}")

            if modified_deps:
                with open(package_json_path, "w") as f:
                    json.dump(package_data, f, indent=2)

        except Exception as e:
            return failure(f"Failed to modify package.json: {e}")

    # Run npm install
    try:
        result = subprocess.run(
            ["npm", "install"],
            cwd=path,
            capture_output=True,
            text=True,
            timeout=300,
        )

        stdout = result.stdout
        if modified_deps:
            stdout = f"[mode={mode}] Modified: {', '.join(modified_deps)}\n{stdout}"
        else:
            stdout = f"[mode={mode}] {stdout}"

        if result.returncode == 0:
            return StepResult(
                exit_code=0,
                stdout=stdout,
                stderr=result.stderr,
                success=True,
            )
        else:
            return StepResult(
                exit_code=result.returncode,
                stdout=stdout,
                stderr=result.stderr,
                success=False,
                error=f"npm install failed (mode={mode}): {result.stderr}",
            )

    except subprocess.TimeoutExpired:
        return failure("npm install timed out after 300s", exit_code=124)
    except Exception as e:
        return failure(f"npm install failed: {e}")


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


def _find_tarball(packages_dir: str, package_name: str) -> str | None:
    """Find tarball for a package in the packages directory."""
    # @mcpmesh/sdk -> mcpmesh-sdk-*.tgz
    normalized = package_name.replace("@", "").replace("/", "-")

    try:
        for file in os.listdir(packages_dir):
            if file.startswith(normalized) and file.endswith(".tgz"):
                return os.path.join(packages_dir, file)
    except OSError:
        pass

    return None
