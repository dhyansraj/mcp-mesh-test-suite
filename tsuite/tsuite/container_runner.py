"""
Container-side test runner.

This module runs INSIDE the Docker container and:
1. Loads the test YAML
2. Executes pre_run, test, post_run steps
3. Evaluates assertions
4. Reports results back to the server
"""

import sys
import os
import json
import yaml
from pathlib import Path

# Add tsuite to path (mounted at /tsuite)
sys.path.insert(0, "/tsuite")

from tsuite.context import StepResult, runtime
from tsuite.expressions import ExpressionEvaluator
from tsuite.client import RunnerClient
from tsuite.discovery import load_config

# Import handlers from the handlers module
from handlers import pip_install, npm_install


def _step_result_to_dict(result: StepResult) -> dict:
    """Convert StepResult dataclass to dict for step tracking."""
    return {
        "success": result.success,
        "exit_code": result.exit_code,
        "stdout": result.stdout,
        "stderr": result.stderr,
        "error": result.error,
    }


def run_test(test_yaml_path: str, suite_path: str):
    """
    Run a test from inside a container.

    Args:
        test_yaml_path: Path to test.yaml file
        suite_path: Path to test suite root (for loading config/routines)
    """
    import time
    start_time = time.time()

    # Initialize client for server communication
    client = RunnerClient()
    test_id = os.environ.get("TSUITE_TEST_ID", "unknown")
    run_id = os.environ.get("TSUITE_RUN_ID")

    # Report test started (new API architecture)
    if run_id:
        client.report_test_running(run_id, test_id)

    # Load test configuration
    with open(test_yaml_path) as f:
        test_config = yaml.safe_load(f)

    # Load suite config
    config = load_config(Path(suite_path) / "config.yaml")

    # Load global routines
    global_routines = {}
    global_routines_path = Path(suite_path) / "global" / "routines.yaml"
    if global_routines_path.exists():
        with open(global_routines_path) as f:
            data = yaml.safe_load(f) or {}
            global_routines = data.get("routines", {})

    # Build execution context
    context = {
        "config": config,
        "state": {},
        "captured": {},
        "steps": {},  # Step results by capture name
        "last": {"exit_code": 0, "stdout": "", "stderr": ""},
        "workdir": "/workspace",
        "test_id": test_id,
    }

    results = {
        "steps": [],
        "assertions": [],
        "passed": True,
        "error": None,
    }

    # Execute pre_run
    client.progress(0, "running", "Executing pre_run")
    for i, step in enumerate(test_config.get("pre_run", [])):
        result = execute_step(step, context, global_routines, client)
        results["steps"].append({
            "phase": "pre_run",
            "index": i,
            "name": step.get("name"),
            "handler": step.get("handler", step.get("routine")),
            "result": result,
        })
        print(f"[pre_run:{i}] {step.get('handler', step.get('routine', '?'))}: {'OK' if result['success'] else 'FAIL'}")

        if not result["success"] and not step.get("ignore_errors"):
            results["passed"] = False
            results["error"] = f"pre_run step {i} failed"
            break

        # Update context with result
        context["last"] = {
            "exit_code": result.get("exit_code", 0),
            "stdout": result.get("stdout", ""),
            "stderr": result.get("stderr", ""),
        }

    # Execute test steps (if pre_run succeeded)
    if results["passed"]:
        client.progress(1, "running", "Executing test steps")
        for i, step in enumerate(test_config.get("test", [])):
            result = execute_step(step, context, global_routines, client)
            results["steps"].append({
                "phase": "test",
                "index": i,
                "name": step.get("name"),
                "handler": step.get("handler", step.get("routine")),
                "result": result,
            })
            print(f"[test:{i}] {step.get('handler', step.get('routine', '?'))}: {'OK' if result['success'] else 'FAIL'}")

            if not result["success"] and not step.get("ignore_errors"):
                results["passed"] = False
                results["error"] = f"test step {i} failed: {result.get('error', result.get('stderr', ''))}"
                break

            # Update context
            context["last"] = {
                "exit_code": result.get("exit_code", 0),
                "stdout": result.get("stdout", ""),
                "stderr": result.get("stderr", ""),
            }

            # Handle capture - store both stdout (for backward compat) and full result
            if "capture" in step:
                capture_name = step["capture"]
                # Always store full step result for ${steps.<name>.exit_code} etc.
                context["steps"][capture_name] = {
                    "exit_code": result.get("exit_code", 0),
                    "stdout": result.get("stdout", ""),
                    "stderr": result.get("stderr", ""),
                    "success": result.get("success", False),
                    "error": result.get("error"),
                }
                # Store stdout in captured for backward compatibility
                if result["success"]:
                    context["captured"][capture_name] = result.get("stdout", "")

            # Handle state
            if "state" in step and result["success"]:
                context["state"].update(step["state"])

    # Evaluate assertions
    if results["passed"]:
        client.progress(2, "running", "Evaluating assertions")
        evaluator = ExpressionEvaluator(context)

        for i, assertion in enumerate(test_config.get("assertions", [])):
            expr = assertion.get("expr", "")
            message = assertion.get("message", expr)

            passed, details, metadata = evaluator.evaluate(expr)
            results["assertions"].append({
                "index": i,
                "expr": expr,
                "message": message,
                "passed": passed,
                "details": details,
                "actual_value": metadata.get("actual_value"),
                "expected_value": metadata.get("expected_value"),
            })

            status = "PASS" if passed else "FAIL"
            print(f"[assert:{i}] {message}: {status}")

            if not passed:
                results["passed"] = False

    # Execute post_run (always)
    client.progress(3, "running", "Executing post_run")
    for i, step in enumerate(test_config.get("post_run", [])):
        step_with_ignore = {**step, "ignore_errors": True}
        result = execute_step(step_with_ignore, context, global_routines, client)
        results["steps"].append({
            "phase": "post_run",
            "index": i,
            "name": step.get("name"),
            "handler": step.get("handler", step.get("routine")),
            "result": result,
        })
        print(f"[post_run:{i}] {step.get('handler', step.get('routine', '?'))}: {'OK' if result['success'] else 'FAIL'}")

    # Report final status
    status = "passed" if results["passed"] else "failed"
    client.progress(4, status, f"Test {status}")

    # Calculate duration and step counts
    duration_ms = int((time.time() - start_time) * 1000)
    steps_passed = sum(1 for s in results["steps"] if s["result"].get("success"))
    steps_failed = sum(1 for s in results["steps"] if not s["result"].get("success"))

    # Report test completed via API (new architecture)
    if run_id:
        if results["passed"]:
            client.report_test_passed(
                run_id=run_id,
                test_id=test_id,
                duration_ms=duration_ms,
                steps_passed=steps_passed,
                steps_failed=steps_failed,
                steps=results["steps"],
                assertions=results["assertions"],
            )
        else:
            client.report_test_failed(
                run_id=run_id,
                test_id=test_id,
                duration_ms=duration_ms,
                error_message=results["error"],
                steps_passed=steps_passed,
                steps_failed=steps_failed,
                steps=results["steps"],
                assertions=results["assertions"],
            )

    # Print summary
    print(f"\n{'='*50}")
    print(f"TEST {'PASSED' if results['passed'] else 'FAILED'}")
    if results["error"]:
        print(f"Error: {results['error']}")
    print(f"{'='*50}")

    # Exit with appropriate code
    sys.exit(0 if results["passed"] else 1)


def execute_step(step: dict, context: dict, routines: dict, client: RunnerClient) -> dict:
    """Execute a single step."""
    # Check if this is a routine call
    if "routine" in step:
        return execute_routine(step, context, routines, client)

    # Get handler
    handler_name = step.get("handler")
    if not handler_name:
        return {"success": False, "error": "Step missing 'handler' or 'routine'"}

    # Interpolate step parameters
    evaluator = ExpressionEvaluator(context)
    interpolated = interpolate_step(step, evaluator)

    # Execute handler
    if handler_name == "shell":
        return execute_shell(interpolated, context)
    elif handler_name == "file":
        return execute_file(interpolated, context)
    elif handler_name == "http":
        return execute_http(interpolated, context)
    elif handler_name == "wait":
        return execute_wait(interpolated, context)
    elif handler_name == "pip-install":
        return _step_result_to_dict(pip_install.execute(interpolated, context))
    elif handler_name == "npm-install":
        return _step_result_to_dict(npm_install.execute(interpolated, context))
    else:
        return {"success": False, "error": f"Unknown handler: {handler_name}"}


def execute_routine(step: dict, context: dict, routines: dict, client: RunnerClient) -> dict:
    """Execute a routine."""
    routine_ref = step["routine"]
    params = step.get("params", {})

    # Resolve routine name (handle global. prefix)
    if routine_ref.startswith("global."):
        routine_name = routine_ref[7:]
    else:
        routine_name = routine_ref

    routine = routines.get(routine_name)
    if not routine:
        return {"success": False, "error": f"Routine not found: {routine_ref}"}

    # Interpolate params
    evaluator = ExpressionEvaluator(context)
    interpolated_params = {}
    for key, value in params.items():
        if isinstance(value, str):
            interpolated_params[key] = evaluator.interpolate(value)
        else:
            interpolated_params[key] = value

    # Apply defaults
    for param_name, schema in routine.get("params", {}).items():
        if param_name not in interpolated_params and "default" in schema:
            interpolated_params[param_name] = schema["default"]

    # Create routine context
    routine_context = {
        **context,
        "params": interpolated_params,
    }

    # Execute routine steps
    for i, routine_step in enumerate(routine.get("steps", [])):
        result = execute_step(routine_step, routine_context, routines, client)

        if not result.get("success") and not routine_step.get("ignore_errors"):
            return result

        # Update routine context
        routine_context["last"] = {
            "exit_code": result.get("exit_code", 0),
            "stdout": result.get("stdout", ""),
            "stderr": result.get("stderr", ""),
        }

        if "capture" in routine_step:
            capture_name = routine_step["capture"]
            # Always store full step result for ${steps.<name>.exit_code} etc.
            step_result = {
                "exit_code": result.get("exit_code", 0),
                "stdout": result.get("stdout", ""),
                "stderr": result.get("stderr", ""),
                "success": result.get("success", False),
                "error": result.get("error"),
            }
            routine_context["steps"][capture_name] = step_result
            context["steps"][capture_name] = step_result
            # Store stdout in captured for backward compatibility
            if result.get("success"):
                routine_context["captured"][capture_name] = result.get("stdout", "")
                context["captured"][capture_name] = result.get("stdout", "")

    return {"success": True, "exit_code": 0, "stdout": "", "stderr": ""}


def interpolate_step(step: dict, evaluator: ExpressionEvaluator) -> dict:
    """Interpolate variables in step parameters."""
    result = {}
    for key, value in step.items():
        if isinstance(value, str):
            result[key] = evaluator.interpolate(value)
        elif isinstance(value, list):
            result[key] = [
                evaluator.interpolate(v) if isinstance(v, str) else v
                for v in value
            ]
        elif isinstance(value, dict):
            result[key] = interpolate_step(value, evaluator)
        else:
            result[key] = value
    return result


def execute_shell(step: dict, context: dict) -> dict:
    """Execute a shell command."""
    import subprocess

    command = step.get("command", "")
    workdir = step.get("workdir", context.get("workdir", "/workspace"))
    timeout = step.get("timeout", 120)

    try:
        result = subprocess.run(
            command,
            shell=True,
            executable="/bin/bash",  # Use bash explicitly for bashisms like &>
            cwd=workdir,
            capture_output=True,
            text=True,
            timeout=timeout,
            env={**os.environ, "TSUITE_API": os.environ.get("TSUITE_API", "")},
        )

        return {
            "success": result.returncode == 0,
            "exit_code": result.returncode,
            "stdout": result.stdout,
            "stderr": result.stderr,
            "error": None if result.returncode == 0 else f"Exit code: {result.returncode}",
        }
    except subprocess.TimeoutExpired:
        return {"success": False, "exit_code": 124, "error": f"Timeout after {timeout}s"}
    except Exception as e:
        return {"success": False, "exit_code": 1, "error": str(e)}


def execute_file(step: dict, context: dict) -> dict:
    """Execute a file operation."""
    operation = step.get("operation", "exists")
    path_str = step.get("path", "")

    if not path_str.startswith("/"):
        path_str = os.path.join(context.get("workdir", "/workspace"), path_str)

    path = Path(path_str)

    if operation == "exists":
        exists = path.exists()
        return {
            "success": exists,
            "exit_code": 0 if exists else 1,
            "stdout": str(exists),
            "error": None if exists else f"Path does not exist: {path}",
        }
    elif operation == "read":
        try:
            content = path.read_text()
            return {"success": True, "exit_code": 0, "stdout": content}
        except Exception as e:
            return {"success": False, "exit_code": 1, "error": str(e)}
    elif operation == "write":
        try:
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(step.get("content", ""))
            return {"success": True, "exit_code": 0, "stdout": f"Wrote to {path}"}
        except Exception as e:
            return {"success": False, "exit_code": 1, "error": str(e)}

    return {"success": False, "error": f"Unknown operation: {operation}"}


def execute_http(step: dict, context: dict) -> dict:
    """Execute an HTTP request."""
    import requests

    method = step.get("method", "GET").upper()
    url = step.get("url", "")
    headers = step.get("headers", {})
    body = step.get("body")
    timeout = step.get("timeout", 30)

    try:
        if method == "GET":
            response = requests.get(url, headers=headers, timeout=timeout)
        elif method == "POST":
            response = requests.post(url, json=body, headers=headers, timeout=timeout)
        elif method == "PUT":
            response = requests.put(url, json=body, headers=headers, timeout=timeout)
        elif method == "DELETE":
            response = requests.delete(url, headers=headers, timeout=timeout)
        else:
            return {"success": False, "error": f"Unknown HTTP method: {method}"}

        return {
            "success": response.status_code < 400,
            "exit_code": 0 if response.status_code < 400 else 1,
            "stdout": response.text,
            "stderr": "",
            "status_code": response.status_code,
        }
    except requests.Timeout:
        return {"success": False, "exit_code": 1, "error": f"Request timeout after {timeout}s"}
    except Exception as e:
        return {"success": False, "exit_code": 1, "error": str(e)}


def execute_wait(step: dict, context: dict) -> dict:
    """Execute a wait operation."""
    import time
    import requests

    wait_type = step.get("type", "seconds")

    if wait_type == "seconds":
        seconds = step.get("seconds", 1)
        time.sleep(seconds)
        return {"success": True, "exit_code": 0, "stdout": f"Waited {seconds}s"}

    elif wait_type == "http":
        url = step.get("url", "")
        timeout = step.get("timeout", 30)
        interval = step.get("interval", 2)

        start = time.time()
        while time.time() - start < timeout:
            try:
                response = requests.get(url, timeout=5)
                if response.status_code < 400:
                    return {"success": True, "exit_code": 0, "stdout": f"URL {url} is ready"}
            except:
                pass
            time.sleep(interval)

        return {"success": False, "exit_code": 1, "error": f"URL {url} not ready after {timeout}s"}

    return {"success": False, "error": f"Unknown wait type: {wait_type}"}


if __name__ == "__main__":
    if len(sys.argv) < 3:
        print("Usage: python container_runner.py <test.yaml> <suite_path>")
        sys.exit(1)

    run_test(sys.argv[1], sys.argv[2])
