"""
Test execution engine.

Handles:
- Docker container lifecycle
- Step execution
- Pre/post run hooks
- Assertion evaluation
"""

import tempfile
import time
import subprocess
from pathlib import Path
from dataclasses import dataclass, field
from typing import Callable

from .context import runtime, TestContext, StepResult
from .discovery import TestCase
from .expressions import ExpressionEvaluator
from .routines import RoutineResolver, ResolvedRoutine, create_routine_context


@dataclass
class TestResult:
    """Result of a test execution."""
    test_id: str
    test_name: str
    passed: bool
    duration: float
    steps_passed: int
    steps_failed: int
    assertions_passed: int
    assertions_failed: int
    error: str | None = None
    step_results: list[dict] = field(default_factory=list)
    assertion_results: list[dict] = field(default_factory=list)


class StepExecutor:
    """
    Executes individual test steps.

    Steps can be:
    - handler: shell/file/http/etc.
    - routine: reference to a reusable routine
    """

    def __init__(
        self,
        handlers: dict[str, Callable],
        routine_resolver: RoutineResolver,
        context: TestContext,
        server_url: str,
        suite_path: Path | None = None,
    ):
        self.handlers = handlers
        self.routine_resolver = routine_resolver
        self.context = context
        self.server_url = server_url
        self.suite_path = suite_path

    def execute_step(self, step: dict, step_index: int) -> StepResult:
        """Execute a single step."""
        start_time = time.time()

        # Check if this is a routine call
        if "routine" in step:
            return self._execute_routine(step, step_index)

        # Get handler
        handler_name = step.get("handler")
        if not handler_name:
            return StepResult(
                exit_code=1,
                success=False,
                error="Step missing 'handler' or 'routine' key",
            )

        handler = self.handlers.get(handler_name)
        if not handler:
            return StepResult(
                exit_code=1,
                success=False,
                error=f"Unknown handler: {handler_name}",
            )

        # Build execution context
        exec_context = self._build_exec_context()

        # Interpolate step parameters
        interpolated_step = self._interpolate_step(step, exec_context)

        # Execute handler
        try:
            result = handler(interpolated_step, exec_context)
        except Exception as e:
            result = StepResult(
                exit_code=1,
                success=False,
                error=str(e),
            )

        result.duration = time.time() - start_time

        # Update context with result
        self.context.last = result

        # Handle capture - store both stdout (for backward compat) and full result
        if "capture" in step:
            capture_name = step["capture"]
            # Always store full step result for ${steps.<name>.exit_code} etc.
            self.context.steps[capture_name] = {
                "exit_code": result.exit_code,
                "stdout": result.stdout,
                "stderr": result.stderr,
                "success": result.success,
                "duration": result.duration,
                "error": result.error,
            }
            # Store stdout in captured for backward compatibility
            if result.success:
                self.context.captured[capture_name] = result.stdout

        # Handle state updates
        if "state" in step and result.success:
            self.context.state.update(step["state"])

        return result

    def _execute_routine(self, step: dict, step_index: int) -> StepResult:
        """Execute a routine call."""
        routine_ref = step["routine"]
        params = step.get("params", {})

        # Resolve routine
        routine = self.routine_resolver.resolve(
            routine_ref,
            self.context.uc,
            self.context.test_id,
        )

        if not routine:
            return StepResult(
                exit_code=1,
                success=False,
                error=f"Routine not found: {routine_ref}",
            )

        # Validate and apply defaults
        errors = self.routine_resolver.validate_params(routine, params)
        if errors:
            return StepResult(
                exit_code=1,
                success=False,
                error=f"Routine param errors: {', '.join(errors)}",
            )

        params = self.routine_resolver.apply_defaults(routine, params)

        # Interpolate params
        exec_context = self._build_exec_context()
        evaluator = ExpressionEvaluator(exec_context)
        interpolated_params = {}
        for key, value in params.items():
            if isinstance(value, str):
                interpolated_params[key] = evaluator.interpolate(value)
            else:
                interpolated_params[key] = value

        # Execute routine steps
        routine_context = create_routine_context(routine, interpolated_params, exec_context)

        for i, routine_step in enumerate(routine.steps):
            # Create a new step executor with routine context
            step_with_params = dict(routine_step)

            # Interpolate with routine params
            routine_evaluator = ExpressionEvaluator(routine_context)
            step_with_params = self._interpolate_step(step_with_params, routine_context)

            result = self.execute_step(step_with_params, step_index)
            if not result.success and not step.get("ignore_errors"):
                return result

            # Update routine context with captured values
            routine_context["captured"] = self.context.captured
            routine_context["state"] = self.context.state
            routine_context["steps"] = self.context.steps
            routine_context["last"] = {
                "exit_code": result.exit_code,
                "stdout": result.stdout,
                "stderr": result.stderr,
            }

        return StepResult(success=True, exit_code=0)

    def _build_exec_context(self) -> dict:
        """Build execution context for handlers/expressions."""
        ctx = {
            "config": runtime.get_config(),
            "state": self.context.state,
            "captured": self.context.captured,
            "steps": self.context.steps,  # Step results by capture name
            "last": {
                "exit_code": self.context.last.exit_code,
                "stdout": self.context.last.stdout,
                "stderr": self.context.last.stderr,
            },
            "workdir": str(self.context.workdir),
            "test_id": self.context.test_id,
            "server_url": self.server_url,
        }

        # Add suite_path and artifacts_path for standalone mode
        if self.suite_path:
            ctx["suite_path"] = str(self.suite_path)
            # artifacts_path: suite_path/suites/<test_id>/artifacts
            artifacts_path = self.suite_path / "suites" / self.context.test_id / "artifacts"
            if artifacts_path.exists():
                ctx["artifacts_path"] = str(artifacts_path)
            else:
                ctx["artifacts_path"] = ""
            # uc_artifacts_path: suite_path/suites/<uc>/artifacts
            uc_artifacts_path = self.suite_path / "suites" / self.context.uc / "artifacts"
            if uc_artifacts_path.exists():
                ctx["uc_artifacts_path"] = str(uc_artifacts_path)
            else:
                ctx["uc_artifacts_path"] = ""
        else:
            # No suite_path - set empty values
            ctx["suite_path"] = ""
            ctx["artifacts_path"] = ""
            ctx["uc_artifacts_path"] = ""

        return ctx

    def _interpolate_step(self, step: dict, context: dict) -> dict:
        """Interpolate variables in step parameters."""
        evaluator = ExpressionEvaluator(context)
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
                result[key] = self._interpolate_step(value, context)
            else:
                result[key] = value

        return result


class TestExecutor:
    """
    Executes a complete test case.

    Handles the full lifecycle:
    1. Create workspace
    2. Run pre_run steps
    3. Run test steps
    4. Evaluate assertions
    5. Run post_run steps (cleanup)
    """

    def __init__(
        self,
        handlers: dict[str, Callable],
        routine_resolver: RoutineResolver,
        server_url: str,
        base_workdir: Path,
        suite_path: Path | None = None,
    ):
        self.handlers = handlers
        self.routine_resolver = routine_resolver
        self.server_url = server_url
        self.base_workdir = base_workdir
        self.suite_path = suite_path

    def execute(self, test: TestCase) -> TestResult:
        """Execute a test case."""
        start_time = time.time()

        # Create workspace
        workdir = self.base_workdir / test.id.replace("/", "_")
        workdir.mkdir(parents=True, exist_ok=True)

        # Create test context
        context = runtime.create_test_context(
            test_id=test.id,
            test_name=test.name,
            workdir=workdir,
        )

        # Create step executor
        step_executor = StepExecutor(
            handlers=self.handlers,
            routine_resolver=self.routine_resolver,
            context=context,
            server_url=self.server_url,
            suite_path=self.suite_path,
        )

        step_results = []
        steps_passed = 0
        steps_failed = 0
        error = None

        # Execute pre_run
        pre_run = test.config.get("pre_run", [])
        for i, step in enumerate(pre_run):
            runtime.update_progress(test.id, i, "pre_run", f"Running pre_run step {i+1}")
            result = step_executor.execute_step(step, i)
            step_results.append({
                "phase": "pre_run",
                "index": i,
                "step": step,
                "result": result,
            })

            if result.success:
                steps_passed += 1
            else:
                steps_failed += 1
                if not step.get("ignore_errors"):
                    error = f"pre_run step {i+1} failed: {result.error or result.stderr}"
                    break

        # Execute test steps (if pre_run succeeded)
        if not error:
            test_steps = test.config.get("test", [])
            for i, step in enumerate(test_steps):
                runtime.update_progress(test.id, i, "test", f"Running test step {i+1}")
                result = step_executor.execute_step(step, i)
                step_results.append({
                    "phase": "test",
                    "index": i,
                    "step": step,
                    "result": result,
                })

                if result.success:
                    steps_passed += 1
                else:
                    steps_failed += 1
                    if not step.get("ignore_errors"):
                        error = f"test step {i+1} failed: {result.error or result.stderr}"
                        break

        # Evaluate assertions
        assertion_results = []
        assertions_passed = 0
        assertions_failed = 0

        if not error:
            assertions = test.config.get("assertions", [])
            exec_context = step_executor._build_exec_context()
            evaluator = ExpressionEvaluator(exec_context)

            for i, assertion in enumerate(assertions):
                expr = assertion.get("expr", "")
                message = assertion.get("message", expr)

                passed, details, values = evaluator.evaluate(expr)
                assertion_results.append({
                    "index": i,
                    "expr": expr,
                    "message": message,
                    "passed": passed,
                    "details": details,
                    "expected_value": values.get("expected_value"),
                    "actual_value": values.get("actual_value"),
                })

                if passed:
                    assertions_passed += 1
                else:
                    assertions_failed += 1

        # Execute post_run (always runs)
        post_run = test.config.get("post_run", [])
        for i, step in enumerate(post_run):
            runtime.update_progress(test.id, i, "post_run", f"Running post_run step {i+1}")
            # Always ignore errors in post_run
            step_with_ignore = {**step, "ignore_errors": True}
            result = step_executor.execute_step(step_with_ignore, i)
            step_results.append({
                "phase": "post_run",
                "index": i,
                "step": step,
                "result": result,
            })

        duration = time.time() - start_time

        # Determine overall pass/fail
        passed = (
            error is None and
            steps_failed == 0 and
            assertions_failed == 0
        )

        return TestResult(
            test_id=test.id,
            test_name=test.name,
            passed=passed,
            duration=duration,
            steps_passed=steps_passed,
            steps_failed=steps_failed,
            assertions_passed=assertions_passed,
            assertions_failed=assertions_failed,
            error=error,
            step_results=step_results,
            assertion_results=assertion_results,
        )
