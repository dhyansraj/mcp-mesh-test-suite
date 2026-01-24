"""
Docker-based test execution.

Runs each test inside an isolated Docker container with:
- Test suite code mounted
- Fresh workspace directory
- Environment variables for server communication
"""

import os
import tempfile
import time
import json
from pathlib import Path
from dataclasses import dataclass, field

import docker
from docker.errors import ContainerError, ImageNotFound, APIError

from .context import runtime, TestContext, StepResult
from .discovery import TestCase


@dataclass
class ContainerConfig:
    """Configuration for test container."""
    image: str = "python:3.11-slim"
    network: str = "bridge"
    workdir: str = "/workspace"
    timeout: int = 300  # 5 minutes default
    memory_limit: str = "1g"
    cpu_limit: float = 1.0
    mounts: list = field(default_factory=list)  # Suite-level mounts from config.yaml docker.mounts


class DockerExecutor:
    """
    Executes tests inside Docker containers.

    Each test gets:
    - Fresh container from base image
    - /tsuite mounted (test-suite framework, readonly)
    - /tests mounted (test definitions, readonly)
    - /workspace mounted (writable workspace)
    - /artifacts mounted (TC-level artifacts, readonly, if exists)
    - /uc-artifacts mounted (UC-level artifacts, readonly, if exists)
    - Environment variables for server communication
    """

    def __init__(
        self,
        server_url: str,
        framework_path: Path,
        suite_path: Path,
        base_workdir: Path,
        config: ContainerConfig | None = None,
        run_id: str | None = None,
    ):
        """
        Initialize Docker executor.

        Args:
            server_url: URL of the runner server (for container callbacks)
            framework_path: Path to test-suite/ directory
            suite_path: Path to mcp-mesh-test-suites/ directory
            base_workdir: Base directory for test workspaces
            config: Container configuration
            run_id: The run ID for API status reporting (new architecture)
        """
        self.server_url = server_url
        self.framework_path = Path(framework_path)
        self.suite_path = Path(suite_path)
        self.base_workdir = Path(base_workdir)
        self.config = config or ContainerConfig()
        self.run_id = run_id

        # Initialize Docker client
        try:
            self.client = docker.from_env()
            self.client.ping()
        except Exception as e:
            raise RuntimeError(f"Failed to connect to Docker: {e}")

    def execute_test(self, test: TestCase) -> dict:
        """
        Execute a test inside a Docker container.

        Args:
            test: Test case to execute

        Returns:
            Dict with execution results
        """
        # Create workspace for this test
        test_workdir = self.base_workdir / test.id.replace("/", "_")
        test_workdir.mkdir(parents=True, exist_ok=True)

        # Create test context
        context = runtime.create_test_context(
            test_id=test.id,
            test_name=test.name,
            workdir=test_workdir,
        )

        # Get container config from test or use defaults
        container_config = test.config.get("container", {})
        image = container_config.get("image", self.config.image)
        timeout = test.config.get("timeout", self.config.timeout)

        # Prepare environment variables
        env = {
            "TSUITE_API": self.server_url,
            "TSUITE_TEST_ID": test.id,
            "PYTHONUNBUFFERED": "1",
        }

        # Add run_id for new API architecture
        if self.run_id:
            env["TSUITE_RUN_ID"] = self.run_id

        # Add env from test config
        if "env" in container_config:
            for key, value in container_config["env"].items():
                # Resolve ${env:VAR} references
                if isinstance(value, str) and value.startswith("${env:"):
                    env_var = value[6:-1]
                    env[key] = os.environ.get(env_var, "")
                else:
                    env[key] = str(value)

        # Prepare volume mounts
        volumes = {
            str(self.framework_path): {"bind": "/tsuite", "mode": "ro"},
            str(self.suite_path): {"bind": "/tests", "mode": "ro"},
            str(test_workdir): {"bind": "/workspace", "mode": "rw"},
        }

        # Auto-mount TC-local artifacts directory if it exists
        artifacts_path = self.suite_path / "suites" / test.id / "artifacts"
        if artifacts_path.exists() and artifacts_path.is_dir():
            volumes[str(artifacts_path)] = {"bind": "/artifacts", "mode": "ro"}

        # Auto-mount UC-level artifacts directory if it exists
        # test.id format: "uc01_registry/tc01_agent_registration"
        uc_name = test.id.split("/")[0] if "/" in test.id else None
        if uc_name:
            uc_artifacts_path = self.suite_path / "suites" / uc_name / "artifacts"
            if uc_artifacts_path.exists() and uc_artifacts_path.is_dir():
                volumes[str(uc_artifacts_path)] = {"bind": "/uc-artifacts", "mode": "ro"}

        # Create and mount logs directory for meshctl output
        # Structure: ~/.tsuite/runs/<run_id>/<uc>/<tc>/logs/
        if self.run_id:
            uc_name = test.id.split("/")[0] if "/" in test.id else test.id
            tc_name = test.id.split("/")[1] if "/" in test.id else "default"
            logs_path = Path.home() / ".tsuite" / "runs" / self.run_id / uc_name / tc_name / "logs"
            logs_path.mkdir(parents=True, exist_ok=True)
            volumes[str(logs_path)] = {"bind": "/root/.mcp-mesh/logs", "mode": "rw"}

        # Add suite-level mounts from config.yaml docker.mounts
        for mount in self.config.mounts:
            mount_type = mount.get("type", "host")
            container_path = mount.get("container_path")

            if mount_type == "host":
                host_path = mount.get("host_path")
                if host_path:
                    # Resolve relative paths
                    if not host_path.startswith("/"):
                        host_path = str(self.suite_path / host_path)
                    mode = "ro" if mount.get("readonly", False) else "rw"
                    volumes[host_path] = {"bind": container_path, "mode": mode}
        # Add custom mounts from test config (can override suite-level)
        for mount in container_config.get("mounts", []):
            mount_type = mount.get("type", "host")
            container_path = mount.get("container_path")

            if mount_type == "host":
                host_path = mount.get("host_path")
                if host_path:
                    # Resolve relative paths
                    if not host_path.startswith("/"):
                        host_path = str(self.suite_path / host_path)
                    mode = "ro" if mount.get("readonly", False) else "rw"
                    volumes[host_path] = {"bind": container_path, "mode": mode}

        # Build the command to run inside container
        # We'll run a Python script that executes the test steps
        command = self._build_test_command(test)

        start_time = time.time()
        exit_code = 0
        stdout = ""
        stderr = ""
        error = None

        try:
            # Pull image if needed
            try:
                self.client.images.get(image)
            except ImageNotFound:
                runtime.update_progress(test.id, 0, "pulling", f"Pulling image {image}")
                self.client.images.pull(image)

            # Run container
            runtime.update_progress(test.id, 0, "starting", "Starting container")

            container = self.client.containers.run(
                image=image,
                command=command,
                environment=env,
                volumes=volumes,
                working_dir="/workspace",
                detach=True,
                mem_limit=self.config.memory_limit,
                # cpu_quota=int(self.config.cpu_limit * 100000),
                network_mode=self.config.network,
                # For Mac/Windows, host.docker.internal works
                # For Linux, we might need --add-host
                extra_hosts={"host.docker.internal": "host-gateway"},
            )

            try:
                # Wait for container to finish
                result = container.wait(timeout=timeout)
                exit_code = result.get("StatusCode", 1)

                # Get logs
                stdout = container.logs(stdout=True, stderr=False).decode("utf-8")
                stderr = container.logs(stdout=False, stderr=True).decode("utf-8")

                # Debug: print container output on failure
                if exit_code != 0:
                    print(f"DEBUG: Container failed with exit code {exit_code}")
                    print(f"DEBUG: STDOUT (last 1000 chars):\n{stdout[-1000:]}")
                    print(f"DEBUG: STDERR (last 500 chars):\n{stderr[-500:]}")

            except Exception as e:
                error = f"Container execution failed: {e}"
                exit_code = 1

            finally:
                # Always remove container
                try:
                    container.remove(force=True)
                except:
                    pass

        except ContainerError as e:
            error = f"Container error: {e}"
            exit_code = e.exit_status
            stderr = str(e)

        except APIError as e:
            error = f"Docker API error: {e}"
            exit_code = 1

        except Exception as e:
            error = f"Unexpected error: {e}"
            exit_code = 1

        duration = time.time() - start_time

        return {
            "test_id": test.id,
            "exit_code": exit_code,
            "stdout": stdout,
            "stderr": stderr,
            "error": error,
            "duration": duration,
            "passed": exit_code == 0 and error is None,
        }

    def _build_test_command(self, test: TestCase) -> list[str]:
        """Build the command to run inside the container."""
        # We run a shell script that:
        # 1. Installs jq (for jq-based assertions)
        # 2. Installs required Python packages
        # 3. Runs the test
        script = f'''
set -e

# Install jq if not present (for jq-based assertions)
if ! command -v jq &> /dev/null; then
    apt-get update -qq && apt-get install -y -qq jq 2>/dev/null || true
fi

# Install required packages
pip install -q pyyaml requests jsonpath-ng 2>/dev/null

# Run the test
python3 -c "
import sys
sys.path.insert(0, '/tsuite')
from tsuite.container_runner import run_test
run_test('/tests/suites/{test.id}/test.yaml', '/tests')
"
'''

        return ["bash", "-c", script]


def check_docker_available() -> tuple[bool, str]:
    """Check if Docker is available and running."""
    try:
        client = docker.from_env()
        client.ping()
        version = client.version()
        return True, f"Docker {version.get('Version', 'unknown')}"
    except Exception as e:
        return False, str(e)
