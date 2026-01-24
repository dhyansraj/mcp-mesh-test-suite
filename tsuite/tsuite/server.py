"""
REST API server for container <-> host communication.

The server runs on the host and provides endpoints for:
- Configuration access
- State management
- Progress reporting
- Logging
- Dashboard API (reporting data)
"""

import threading
import logging
from flask import Flask, jsonify, request, Response, send_from_directory
import json
from pathlib import Path

from werkzeug.serving import make_server

from .context import runtime
from . import repository as repo
from . import db
from .models import RunStatus, TestStatus
from .sse import sse_manager, stream_events, SSEEvent
from .discovery import TestDiscovery, load_config

# Suppress Flask's default logging
log = logging.getLogger("werkzeug")
log.setLevel(logging.ERROR)


def create_app() -> Flask:
    """Create the Flask application."""
    app = Flask(__name__)

    @app.teardown_appcontext
    def close_db_connection(exception=None):
        """Close database connection at end of each request to prevent FD leaks."""
        db.close_connection()

    @app.after_request
    def add_cors_headers(response):
        """Add CORS headers to all responses for dashboard access."""
        response.headers["Access-Control-Allow-Origin"] = "*"
        response.headers["Access-Control-Allow-Methods"] = "GET, POST, PUT, PATCH, DELETE, OPTIONS"
        response.headers["Access-Control-Allow-Headers"] = "Content-Type"
        return response

    @app.route("/api/<path:path>", methods=["OPTIONS"])
    def handle_options(path):
        """Handle CORS preflight requests."""
        return "", 204

    @app.route("/health", methods=["GET"])
    def health():
        """Health check endpoint."""
        return jsonify({"status": "ok"})

    # =========================================================================
    # Runner API Endpoints
    # These endpoints are called by test containers/subprocesses.
    # =========================================================================

    @app.route("/api/runner/config", methods=["GET"])
    def api_runner_get_config():
        """
        Get full configuration for runner.

        Query params:
            run_id: str (optional) - The run ID for tracking
        """
        return jsonify(runtime.get_config())

    @app.route("/api/runner/config/<path:path>", methods=["GET"])
    def api_runner_get_config_value(path: str):
        """
        Get specific configuration value by dot-notation path.

        Query params:
            run_id: str (optional) - The run ID for tracking
        """
        dot_path = path.replace("/", ".")
        value = runtime.get_config(dot_path)
        if value is None:
            return jsonify({"error": f"Config path not found: {dot_path}"}), 404
        return jsonify({"value": value})

    @app.route("/api/runner/routine/<scope>/<name>", methods=["GET"])
    def api_runner_get_routine(scope: str, name: str):
        """
        Get routine definition by scope and name.

        Query params:
            run_id: str (optional) - The run ID for tracking
        """
        routine = runtime.get_routine(scope, name)
        if routine is None:
            return jsonify({"error": f"Routine not found: {scope}.{name}"}), 404
        return jsonify(routine)

    @app.route("/api/runner/routines", methods=["GET"])
    def api_runner_get_all_routines():
        """
        Get all routines.

        Query params:
            run_id: str (optional) - The run ID for tracking
        """
        return jsonify(runtime.get_all_routines())

    @app.route("/api/runner/state/<path:test_id>", methods=["GET"])
    def api_runner_get_state(test_id: str):
        """
        Get state for a test.

        Query params:
            run_id: str (optional) - The run ID for tracking
        """
        state = runtime.get_test_state(test_id)
        return jsonify(state)

    @app.route("/api/runner/state/<path:test_id>", methods=["POST"])
    def api_runner_update_state(test_id: str):
        """
        Merge state into a test.

        Query params:
            run_id: str (optional) - The run ID for tracking
        """
        data = request.get_json() or {}
        runtime.update_test_state(test_id, data)
        return jsonify({"status": "ok"})

    @app.route("/api/runner/capture/<path:test_id>", methods=["POST"])
    def api_runner_capture(test_id: str):
        """
        Store captured variables for a test.

        Query params:
            run_id: str (optional) - The run ID for tracking
        """
        data = request.get_json() or {}
        for name, value in data.items():
            runtime.set_captured(test_id, name, value)
        return jsonify({"status": "ok"})

    @app.route("/api/runner/progress/<path:test_id>", methods=["POST"])
    def api_runner_progress(test_id: str):
        """
        Update progress for a test.

        Query params:
            run_id: str (optional) - The run ID for tracking
        """
        data = request.get_json() or {}
        runtime.update_progress(
            test_id,
            step=data.get("step", 0),
            status=data.get("status", "running"),
            message=data.get("message", ""),
        )
        return jsonify({"status": "ok"})

    @app.route("/api/runner/progress/<path:test_id>", methods=["GET"])
    def api_runner_get_progress(test_id: str):
        """
        Get progress for a test.

        Query params:
            run_id: str (optional) - The run ID for tracking
        """
        return jsonify(runtime.get_progress(test_id))

    @app.route("/api/runner/log/<path:test_id>", methods=["POST"])
    def api_runner_log_message(test_id: str):
        """
        Log a message from a test.

        Query params:
            run_id: str (optional) - The run ID for tracking
        """
        data = request.get_json() or {}
        level = data.get("level", "info")
        message = data.get("message", "")
        print(f"[{test_id}] [{level.upper()}] {message}")
        return jsonify({"status": "ok"})

    @app.route("/api/runner/context/<path:test_id>", methods=["GET"])
    def api_runner_get_context(test_id: str):
        """
        Get full test context.

        Query params:
            run_id: str (optional) - The run ID for tracking
        """
        ctx = runtime.get_test_context(test_id)
        if ctx is None:
            return jsonify({"error": f"Test context not found: {test_id}"}), 404
        return jsonify(ctx.to_dict())

    @app.route("/api/runner/should-cancel/<run_id>", methods=["GET"])
    def api_runner_should_cancel(run_id: str):
        """
        Check if a run has been requested to cancel.

        CLI calls this before starting each test to support cooperative cancellation.

        Returns:
            cancel_requested: bool - True if cancellation has been requested
        """
        cancel_requested = repo.is_cancel_requested(run_id)
        return jsonify({"cancel_requested": cancel_requested})

    # =========================================================================
    # Dashboard API Endpoints
    # =========================================================================

    @app.route("/api/runs", methods=["GET"])
    def api_list_runs():
        """
        List test runs (paginated).

        Query params:
            limit: Number of runs to return (default: 20, max: 100)
            offset: Number of runs to skip (default: 0)
            status: Filter by status (optional)
        """
        limit = min(int(request.args.get("limit", 20)), 100)
        offset = int(request.args.get("offset", 0))

        runs = repo.list_runs(limit=limit, offset=offset)
        return jsonify({
            "runs": [r.to_dict() for r in runs],
            "count": len(runs),
            "limit": limit,
            "offset": offset,
        })

    @app.route("/api/runs/latest", methods=["GET"])
    def api_latest_run():
        """Get the most recent run."""
        run = repo.get_latest_run()
        if not run:
            return jsonify({"error": "No runs found"}), 404
        return jsonify(run.to_dict())

    @app.route("/api/runs/<run_id>", methods=["GET"])
    def api_get_run(run_id: str):
        """
        Get run details with summary.

        Returns run metadata plus aggregated test counts.
        """
        summary = repo.get_run_summary(run_id)
        if not summary:
            return jsonify({"error": f"Run not found: {run_id}"}), 404
        return jsonify(summary.to_dict())

    @app.route("/api/runs/<run_id>/tests", methods=["GET"])
    def api_get_run_tests(run_id: str):
        """
        Get all test results for a run.

        Query params:
            status: Filter by status (passed, failed, running, pending)
        """
        run = repo.get_run(run_id)
        if not run:
            return jsonify({"error": f"Run not found: {run_id}"}), 404

        tests = repo.list_test_results(run_id)

        # Optional status filter
        status_filter = request.args.get("status")
        if status_filter:
            try:
                status = TestStatus(status_filter)
                tests = [t for t in tests if t.status == status]
            except ValueError:
                pass  # Invalid status, ignore filter

        return jsonify({
            "run_id": run_id,
            "tests": [t.to_dict() for t in tests],
            "count": len(tests),
        })

    @app.route("/api/runs/<run_id>/tests/<int:test_id>", methods=["GET"])
    def api_get_test_detail(run_id: str, test_id: int):
        """
        Get detailed test result with steps and assertions.
        """
        detail = repo.get_test_detail(test_id)
        if not detail:
            return jsonify({"error": f"Test not found: {test_id}"}), 404

        # Verify test belongs to run
        if detail.test.run_id != run_id:
            return jsonify({"error": f"Test {test_id} not in run {run_id}"}), 404

        return jsonify(detail.to_dict())

    # =========================================================================
    # Run Management API
    # =========================================================================

    @app.route("/api/runs", methods=["POST"])
    def api_create_run():
        """
        Create a new test run with all tests in PENDING state.

        Request body:
            suite_id: int - The suite to run tests from
            filters: dict (optional) - Filters to select tests
                uc: list[str] - Use cases to include
                tc: list[str] - Test cases to include
                tags: list[str] - Tags to filter by

        Returns:
            run_id: str
            total_tests: int
            tests: list of test objects with pending status
        """
        data = request.get_json()
        if not data:
            return jsonify({"error": "No JSON body"}), 400

        suite_id = data.get("suite_id")
        if not suite_id:
            return jsonify({"error": "Missing 'suite_id' field"}), 400

        suite = repo.get_suite(suite_id)
        if not suite:
            return jsonify({"error": f"Suite not found: {suite_id}"}), 404

        # Get filters
        filters = data.get("filters", {})
        uc_filter = filters.get("uc", [])
        tc_filter = filters.get("tc", [])
        tag_filter = filters.get("tags", [])

        # Load and resolve tests from suite
        from .discovery import TestDiscovery
        import os

        if not os.path.isdir(suite.folder_path):
            return jsonify({"error": f"Suite directory not found: {suite.folder_path}"}), 400

        try:
            discovery = TestDiscovery(suite.folder_path)
            all_tests = discovery.discover_tests()
        except Exception as e:
            return jsonify({"error": f"Failed to load suite: {str(e)}"}), 500

        # Resolve tests based on filters
        resolved_tests = []
        for test in all_tests:
            # Apply filters
            include = True

            if tc_filter:
                include = test.id in tc_filter

            if uc_filter and include:
                include = test.uc in uc_filter

            if tag_filter and include:
                include = any(tag in (test.tags or []) for tag in tag_filter)

            if include:
                resolved_tests.append({
                    "test_id": test.id,
                    "use_case": test.uc,
                    "test_case": test.tc,
                    "name": test.name,
                    "tags": test.tags,
                })

        if not resolved_tests:
            return jsonify({"error": "No tests match the specified filters"}), 400

        # Create run with tests
        run = repo.create_run_with_tests(
            suite_id=suite_id,
            tests=resolved_tests,
            filters=filters if filters else None,
            mode=suite.mode.value,
        )

        # Get created tests
        tests = repo.list_test_results(run.run_id)

        return jsonify({
            "run_id": run.run_id,
            "suite_id": suite_id,
            "total_tests": run.total_tests,
            "status": run.status.value,
            "mode": run.mode,
            "tests": [t.to_dict() for t in tests],
        }), 201

    @app.route("/api/runs/<run_id>/start", methods=["POST"])
    def api_start_run(run_id: str):
        """
        Start a previously created run.

        This marks the run as RUNNING and can optionally dispatch to a runner.
        """
        run = repo.get_run(run_id)
        if not run:
            return jsonify({"error": f"Run not found: {run_id}"}), 404

        if run.status != RunStatus.PENDING:
            return jsonify({"error": f"Run already started or completed"}), 400

        repo.start_run(run_id)

        # Emit SSE event
        sse_manager.emit_run_started(run_id, run.total_tests)

        return jsonify({
            "run_id": run_id,
            "status": "running",
            "message": "Run started",
        })

    @app.route("/api/runs/<run_id>/tests/tree", methods=["GET"])
    def api_get_run_tests_tree(run_id: str):
        """
        Get test results grouped by use case (tree structure).

        Returns hierarchical structure for dashboard display.
        """
        run = repo.get_run(run_id)
        if not run:
            return jsonify({"error": f"Run not found: {run_id}"}), 404

        grouped = repo.get_tests_by_usecase(run_id)

        # Convert to list and serialize
        use_cases = []
        for uc_id, uc_data in sorted(grouped.items()):
            use_cases.append({
                "use_case": uc_data["use_case"],
                "tests": [t.to_dict() for t in uc_data["tests"]],
                "pending": uc_data["pending"],
                "running": uc_data["running"],
                "passed": uc_data["passed"],
                "failed": uc_data["failed"],
                "crashed": uc_data["crashed"],
                "skipped": uc_data["skipped"],
                "total": uc_data["total"],
            })

        return jsonify({
            "run_id": run_id,
            "run": run.to_dict(),
            "use_cases": use_cases,
        })

    @app.route("/api/runs/<run_id>/tests/<path:test_id>", methods=["PATCH"])
    def api_update_test_status(run_id: str, test_id: str):
        """
        Update the status of a specific test.

        Called by test runners to report progress.

        Request body:
            status: str - 'running', 'passed', 'failed', 'skipped'
            duration_ms: int (optional)
            error_message: str (optional)
            steps: list (optional) - Step results
            assertions: list (optional) - Assertion results
            skip_reason: str (optional)
        """
        run = repo.get_run(run_id)
        if not run:
            return jsonify({"error": f"Run not found: {run_id}"}), 404

        data = request.get_json()
        if not data:
            return jsonify({"error": "No JSON body"}), 400

        status_str = data.get("status")
        if not status_str:
            return jsonify({"error": "Missing 'status' field"}), 400

        try:
            status = TestStatus(status_str)
        except ValueError:
            return jsonify({"error": f"Invalid status: {status_str}"}), 400

        # Update test status
        test = repo.update_test_status(
            run_id=run_id,
            test_id=test_id,
            status=status,
            duration_ms=data.get("duration_ms"),
            error_message=data.get("error_message"),
            steps_json=data.get("steps"),
            steps_passed=data.get("steps_passed"),
            steps_failed=data.get("steps_failed"),
            skip_reason=data.get("skip_reason"),
        )

        if not test:
            return jsonify({"error": f"Test not found: {test_id}"}), 404

        # Store assertion results if provided
        assertions = data.get("assertions")
        if assertions and test.id:
            for assertion in assertions:
                repo.create_assertion_result(
                    test_result_id=test.id,
                    assertion_index=assertion.get("index", 0),
                    expression=assertion.get("expr", ""),
                    message=assertion.get("message"),
                    passed=assertion.get("passed", False),
                    actual_value=assertion.get("actual_value"),
                    expected_value=assertion.get("expected_value"),
                )

        # Emit SSE event
        if status == TestStatus.RUNNING:
            sse_manager.emit_test_started(run_id, test_id, test.name or test_id)
        elif status in (TestStatus.PASSED, TestStatus.FAILED, TestStatus.CRASHED, TestStatus.SKIPPED):
            sse_manager.emit_test_completed(
                run_id, test_id, status.value,
                data.get("duration_ms", 0),
                data.get("steps_passed", 0),
                data.get("steps_failed", 0),
            )

        return jsonify(test.to_dict())

    @app.route("/api/runs/<run_id>/complete", methods=["POST"])
    def api_complete_run(run_id: str):
        """
        Mark a run as completed.

        Request body:
            duration_ms: int (optional) - Total run duration
        """
        run = repo.get_run(run_id)
        if not run:
            return jsonify({"error": f"Run not found: {run_id}"}), 404

        data = request.get_json() or {}
        duration_ms = data.get("duration_ms")

        repo.complete_run(run_id, duration_ms=duration_ms)

        # Get updated run
        run = repo.get_run(run_id)

        # Emit SSE event
        sse_manager.emit_run_completed(
            run_id,
            passed=run.passed,
            failed=run.failed,
            skipped=run.skipped,
            duration_ms=run.duration_ms or 0,
        )

        return jsonify(run.to_dict())

    @app.route("/api/runs/<run_id>/cancel", methods=["POST"])
    def api_cancel_run(run_id: str):
        """
        Request cancellation of a running test run.

        Sets cancel_requested flag. CLI will check this before starting each test.
        """
        run = repo.get_run(run_id)
        if not run:
            return jsonify({"error": f"Run not found: {run_id}"}), 404

        if run.status not in (RunStatus.PENDING, RunStatus.RUNNING):
            return jsonify({"error": f"Cannot cancel run with status: {run.status.value}"}), 400

        # Set cancel_requested flag
        repo.request_cancel(run_id)

        # Emit SSE event for UI update
        from .sse import SSEEvent
        sse_manager.emit(
            SSEEvent(type="cancel_requested", data={"run_id": run_id}),
            run_id=run_id,
        )

        return jsonify({"success": True, "run_id": run_id, "cancel_requested": True})

    @app.route("/api/stats", methods=["GET"])
    def api_stats():
        """
        Get aggregate statistics across all runs.

        Returns:
            total_runs: Number of completed runs
            total_tests_executed: Total test executions
            total_passed: Total passed tests
            total_failed: Total failed tests
            avg_run_duration_ms: Average run duration
            pass_rate: Overall pass rate percentage
        """
        stats = repo.get_run_stats()

        # Calculate pass rate
        total = (stats.get("total_passed") or 0) + (stats.get("total_failed") or 0)
        pass_rate = (stats.get("total_passed") or 0) / total * 100 if total > 0 else 0

        return jsonify({
            **stats,
            "pass_rate": round(pass_rate, 2),
        })

    @app.route("/api/stats/flaky", methods=["GET"])
    def api_flaky_tests():
        """
        Get tests with mixed pass/fail results (flaky tests).

        Query params:
            limit: Number of tests to return (default: 20)
        """
        limit = int(request.args.get("limit", 20))
        flaky = repo.get_flaky_tests(limit=limit)
        return jsonify({
            "tests": flaky,
            "count": len(flaky),
        })

    @app.route("/api/stats/slowest", methods=["GET"])
    def api_slowest_tests():
        """
        Get tests with highest average duration.

        Query params:
            limit: Number of tests to return (default: 10)
        """
        limit = int(request.args.get("limit", 10))
        slowest = repo.get_slowest_tests(limit=limit)
        return jsonify({
            "tests": slowest,
            "count": len(slowest),
        })

    @app.route("/api/compare/<run_id_1>/<run_id_2>", methods=["GET"])
    def api_compare_runs(run_id_1: str, run_id_2: str):
        """
        Compare test results between two runs.

        Returns tests with their status in each run and change type:
        - same: Status unchanged
        - regression: Passed -> Failed
        - fixed: Failed -> Passed
        - changed: Other status change
        """
        # Verify both runs exist
        run1 = repo.get_run(run_id_1)
        run2 = repo.get_run(run_id_2)

        if not run1:
            return jsonify({"error": f"Run not found: {run_id_1}"}), 404
        if not run2:
            return jsonify({"error": f"Run not found: {run_id_2}"}), 404

        comparison = repo.compare_runs(run_id_1, run_id_2)

        # Count by change type
        regressions = sum(1 for c in comparison if c.get("change_type") == "regression")
        fixed = sum(1 for c in comparison if c.get("change_type") == "fixed")

        return jsonify({
            "run_1": run1.to_dict(),
            "run_2": run2.to_dict(),
            "comparison": comparison,
            "summary": {
                "total": len(comparison),
                "regressions": regressions,
                "fixed": fixed,
                "same": len(comparison) - regressions - fixed,
            },
        })

    @app.route("/api/runs/<run_id>/stream", methods=["GET"])
    def api_run_stream(run_id: str):
        """
        Server-Sent Events stream for live run updates.

        Clients can subscribe to get real-time status changes for a specific run.
        """
        def generate():
            # Subscribe to run events
            queue = sse_manager.subscribe_run(run_id)

            try:
                # Send initial state
                summary = repo.get_run_summary(run_id)
                if summary:
                    yield f"data: {json.dumps({'type': 'initial_state', 'run_id': run_id, **summary.to_dict()})}\n\n"

                # Send connected event
                yield f"data: {json.dumps({'type': 'connected', 'run_id': run_id})}\n\n"

                # Stream events
                for event in stream_events(queue):
                    yield event

            finally:
                # Clean up subscription
                sse_manager.unsubscribe_run(run_id, queue)

        return Response(
            generate(),
            mimetype="text/event-stream",
            headers={
                "Cache-Control": "no-cache",
                "Connection": "keep-alive",
                "X-Accel-Buffering": "no",
            },
        )

    @app.route("/api/events", methods=["GET"])
    def api_global_events():
        """
        Server-Sent Events stream for all test events.

        Clients can subscribe to get real-time updates across all runs.
        Used by the dashboard live view.

        Query params:
            run_id: Optional filter to only receive events for a specific run
        """
        filter_run_id = request.args.get("run_id")

        def generate():
            # Subscribe to global events
            queue = sse_manager.subscribe_global()

            try:
                # Send connected event
                current_run = sse_manager.get_current_run()
                yield f"data: {json.dumps({'type': 'connected', 'current_run_id': current_run})}\n\n"

                # Replay cached events for current run (for late subscribers)
                if current_run:
                    cached_events = sse_manager.get_cached_events(current_run)
                    for cached_event in cached_events:
                        yield cached_event

                # Stream events
                for event in stream_events(queue):
                    # If filtering by run_id, check if event matches
                    if filter_run_id and event.startswith("data:"):
                        try:
                            event_data = json.loads(event[5:].strip())
                            if event_data.get("run_id") != filter_run_id:
                                continue
                        except (json.JSONDecodeError, IndexError):
                            pass
                    yield event

            finally:
                # Clean up subscription
                sse_manager.unsubscribe_global(queue)

        return Response(
            generate(),
            mimetype="text/event-stream",
            headers={
                "Cache-Control": "no-cache",
                "Connection": "keep-alive",
                "X-Accel-Buffering": "no",
            },
        )

    @app.route("/api/events/emit", methods=["POST"])
    def api_emit_event():
        """
        Receive events from external processes (like CLI subprocesses) and broadcast via SSE.

        This endpoint allows the CLI running in a subprocess to send events
        to the API server, which then broadcasts them to all SSE subscribers.

        Request body:
            type: Event type (run_started, test_started, test_completed, run_completed, etc.)
            run_id: The run ID
            ... other event-specific fields
        """
        data = request.get_json()
        if not data:
            return jsonify({"error": "No JSON body"}), 400

        event_type = data.get("type")
        run_id = data.get("run_id")

        if not event_type:
            return jsonify({"error": "Missing 'type' field"}), 400

        # Create and emit the event
        event = SSEEvent(type=event_type, data=data)
        sse_manager.emit(event, run_id=run_id)

        # Update current run tracking
        if event_type == "run_started":
            sse_manager.set_current_run(run_id)
        elif event_type == "run_completed":
            sse_manager.set_current_run(None)
            # Clear event cache after run completes (allow time for late subscribers)
            # Note: Don't clear immediately in case clients are still catching up
            # The cache will be naturally replaced when next run starts

        return jsonify({"ok": True}), 200

    # =========================================================================
    # Suite Management API Endpoints
    # =========================================================================

    @app.route("/api/suites", methods=["GET"])
    def api_list_suites():
        """List all registered test suites."""
        suites = repo.list_suites()
        return jsonify({
            "suites": [s.to_dict() for s in suites],
            "count": len(suites),
        })

    @app.route("/api/suites", methods=["POST"])
    def api_create_suite():
        """
        Register a new test suite by folder path.

        Request body:
            folder_path: Absolute path to the suite folder
            mode: 'docker' or 'standalone' (optional, defaults to config or 'docker')
        """
        data = request.get_json() or {}
        folder_path = data.get("folder_path")

        if not folder_path:
            return jsonify({"error": "folder_path is required"}), 400

        # Normalize and validate path
        import os
        folder_path = os.path.abspath(os.path.expanduser(folder_path))

        if not os.path.isdir(folder_path):
            return jsonify({"error": f"Directory not found: {folder_path}"}), 400

        # Check for config.yaml
        config_path = os.path.join(folder_path, "config.yaml")
        if not os.path.isfile(config_path):
            return jsonify({"error": f"No config.yaml found in {folder_path}"}), 400

        # Check if suite already exists
        existing = repo.get_suite_by_path(folder_path)
        if existing:
            return jsonify({"error": "Suite already exists", "suite": existing.to_dict()}), 409

        # Parse config and discover tests
        try:
            suite_info = _parse_suite_config(folder_path)
        except Exception as e:
            return jsonify({"error": f"Failed to parse config: {str(e)}"}), 400

        # Override mode if provided
        mode_str = data.get("mode") or suite_info.get("mode", "docker")
        from .models import SuiteMode
        try:
            mode = SuiteMode(mode_str)
        except ValueError:
            return jsonify({"error": f"Invalid mode: {mode_str}. Must be 'docker' or 'standalone'"}), 400

        # Create suite
        suite = repo.create_suite(
            folder_path=folder_path,
            suite_name=suite_info.get("name", os.path.basename(folder_path)),
            mode=mode,
            config_json=json.dumps(suite_info.get("config", {})),
            test_count=suite_info.get("test_count", 0),
        )

        return jsonify({
            **suite.to_dict(),
            "tests": suite_info.get("tests", []),
        }), 201

    @app.route("/api/suites/<int:suite_id>", methods=["GET"])
    def api_get_suite(suite_id: int):
        """Get suite details with test list."""
        suite = repo.get_suite(suite_id)
        if not suite:
            return jsonify({"error": f"Suite not found: {suite_id}"}), 404

        # Parse tests from folder
        import os
        tests = []
        use_cases = []
        if os.path.isdir(suite.folder_path):
            try:
                suite_info = _parse_suite_config(suite.folder_path)
                tests = suite_info.get("tests", [])
                use_cases = suite_info.get("use_cases", [])
            except Exception:
                pass

        return jsonify({
            **suite.to_dict(),
            "tests": tests,
            "use_cases": use_cases,
        })

    @app.route("/api/suites/<int:suite_id>", methods=["PUT"])
    def api_update_suite(suite_id: int):
        """Update suite settings."""
        suite = repo.get_suite(suite_id)
        if not suite:
            return jsonify({"error": f"Suite not found: {suite_id}"}), 404

        data = request.get_json() or {}

        mode = None
        if "mode" in data:
            from .models import SuiteMode
            try:
                mode = SuiteMode(data["mode"])
            except ValueError:
                return jsonify({"error": f"Invalid mode: {data['mode']}"}), 400

        repo.update_suite(
            suite_id,
            suite_name=data.get("suite_name"),
            mode=mode,
        )

        return jsonify(repo.get_suite(suite_id).to_dict())

    @app.route("/api/suites/<int:suite_id>", methods=["DELETE"])
    def api_delete_suite(suite_id: int):
        """Remove a suite from settings (does not delete files)."""
        suite = repo.get_suite(suite_id)
        if not suite:
            return jsonify({"error": f"Suite not found: {suite_id}"}), 404

        repo.delete_suite(suite_id)
        return jsonify({"deleted": True, "id": suite_id})

    @app.route("/api/suites/<int:suite_id>/sync", methods=["POST"])
    def api_sync_suite(suite_id: int):
        """Re-read config.yaml and update cached config."""
        suite = repo.get_suite(suite_id)
        if not suite:
            return jsonify({"error": f"Suite not found: {suite_id}"}), 404

        import os
        if not os.path.isdir(suite.folder_path):
            return jsonify({"error": f"Directory not found: {suite.folder_path}"}), 400

        try:
            suite_info = _parse_suite_config(suite.folder_path)
        except Exception as e:
            return jsonify({"error": f"Failed to parse config: {str(e)}"}), 400

        # Update mode from config if present
        from .models import SuiteMode
        mode_str = suite_info.get("mode", suite.mode.value)
        try:
            mode = SuiteMode(mode_str)
        except ValueError:
            mode = suite.mode

        repo.update_suite(
            suite_id,
            suite_name=suite_info.get("name", suite.suite_name),
            mode=mode,
            config_json=json.dumps(suite_info.get("config", {})),
            test_count=suite_info.get("test_count", 0),
        )

        updated = repo.get_suite(suite_id)
        return jsonify({
            "synced": True,
            "test_count": updated.test_count,
            "last_synced_at": updated.last_synced_at.isoformat() if updated.last_synced_at else None,
        })

    @app.route("/api/suites/<int:suite_id>/resolve", methods=["POST"])
    def api_resolve_tests(suite_id: int):
        """
        Resolve (dry-run) test selection without creating a run.

        This endpoint allows clients to preview which tests would be selected
        based on the provided filters before actually creating a run.

        Request body:
            filters: dict (optional) - Filters to select tests
                uc: list[str] - Use cases to include
                tc: list[str] - Test cases to include
                tags: list[str] - Tags to filter by

        Returns:
            suite_id: int
            filters: dict - Applied filters
            tests: list - Resolved test objects
            count: int - Number of tests
            use_cases: list - Grouped by use case with counts
        """
        suite = repo.get_suite(suite_id)
        if not suite:
            return jsonify({"error": f"Suite not found: {suite_id}"}), 404

        import os
        if not os.path.isdir(suite.folder_path):
            return jsonify({"error": f"Directory not found: {suite.folder_path}"}), 400

        # Load suite data
        from .discovery import TestDiscovery
        try:
            discovery = TestDiscovery(suite.folder_path)
            all_tests = discovery.discover_tests()
        except Exception as e:
            return jsonify({"error": f"Failed to load suite: {str(e)}"}), 500

        # Get filters from request body
        data = request.get_json() or {}
        filters = data.get("filters", {})
        uc_filter = filters.get("uc", [])
        tc_filter = filters.get("tc", [])
        tag_filter = filters.get("tags", [])

        # Resolve tests based on filters
        resolved_tests = []
        use_case_counts = {}

        for test in all_tests:
            include = True

            if tc_filter:
                include = test.id in tc_filter

            if uc_filter and include:
                include = test.uc in uc_filter

            if tag_filter and include:
                include = any(tag in (test.tags or []) for tag in tag_filter)

            if include:
                test_info = {
                    "test_id": test.id,
                    "use_case": test.uc,
                    "test_case": test.tc,
                    "name": test.name,
                    "tags": test.tags,
                    "description": test.config.get("description") if test.config else None,
                }
                resolved_tests.append(test_info)

                # Track use case counts
                uc = test.uc
                if uc not in use_case_counts:
                    use_case_counts[uc] = {"use_case": uc, "count": 0}
                use_case_counts[uc]["count"] += 1

        return jsonify({
            "suite_id": suite_id,
            "filters": filters if filters else None,
            "tests": resolved_tests,
            "count": len(resolved_tests),
            "use_cases": list(use_case_counts.values()),
        })

    @app.route("/api/suites/<int:suite_id>/tests", methods=["GET"])
    def api_suite_tests(suite_id: int):
        """List all tests in a suite."""
        suite = repo.get_suite(suite_id)
        if not suite:
            return jsonify({"error": f"Suite not found: {suite_id}"}), 404

        import os
        if not os.path.isdir(suite.folder_path):
            return jsonify({"error": f"Directory not found: {suite.folder_path}"}), 400

        try:
            suite_info = _parse_suite_config(suite.folder_path)
        except Exception as e:
            return jsonify({"error": f"Failed to parse config: {str(e)}"}), 400

        tests = suite_info.get("tests", [])

        # Apply filters
        uc_filter = request.args.get("uc")
        tag_filter = request.args.get("tag")

        if uc_filter:
            tests = [t for t in tests if t.get("use_case") == uc_filter]
        if tag_filter:
            tests = [t for t in tests if tag_filter in t.get("tags", [])]

        return jsonify({
            "suite_id": suite_id,
            "tests": tests,
            "count": len(tests),
        })

    # =========================================================================
    # Test Case Editor API Endpoints
    # =========================================================================

    # =========================================================================
    # Suite Config Editor API Endpoints
    # =========================================================================

    @app.route("/api/suites/<int:suite_id>/config", methods=["GET"])
    def api_get_suite_config(suite_id: int):
        """
        Get suite config.yaml content (for config editor).

        Returns the structure of config.yaml with support for editing.
        """
        import os
        from .yaml_utils import load_yaml_file, get_test_case_structure

        suite = repo.get_suite(suite_id)
        if not suite:
            return jsonify({"error": f"Suite not found: {suite_id}"}), 404

        config_path = os.path.join(suite.folder_path, "config.yaml")

        if not os.path.isfile(config_path):
            return jsonify({"error": f"Config not found: {config_path}"}), 404

        try:
            # Load with comment preservation
            yaml_data = load_yaml_file(config_path)
            structure = get_test_case_structure(yaml_data)

            # Read raw content for display
            with open(config_path, 'r') as f:
                raw_yaml = f.read()

            return jsonify({
                "suite_id": suite_id,
                "path": config_path,
                "raw_yaml": raw_yaml,
                "structure": structure,
            })
        except Exception as e:
            return jsonify({"error": f"Failed to read config: {str(e)}"}), 500

    @app.route("/api/suites/<int:suite_id>/config", methods=["PUT"])
    def api_update_suite_config(suite_id: int):
        """
        Update suite config.yaml content.

        Request body:
            updates: dict - Structured field updates (preserves comments)
        """
        import os
        from .yaml_utils import load_yaml_file, save_yaml_file, merge_yaml_updates

        suite = repo.get_suite(suite_id)
        if not suite:
            return jsonify({"error": f"Suite not found: {suite_id}"}), 404

        config_path = os.path.join(suite.folder_path, "config.yaml")

        if not os.path.isfile(config_path):
            return jsonify({"error": f"Config not found: {config_path}"}), 404

        data = request.get_json()
        if not data:
            return jsonify({"error": "No JSON body"}), 400

        if "updates" not in data:
            return jsonify({"error": "Must provide 'updates'"}), 400

        try:
            # Load existing, merge updates, save (preserves comments)
            yaml_data = load_yaml_file(config_path)
            merge_yaml_updates(yaml_data, data["updates"])
            save_yaml_file(yaml_data, config_path)

            # Return updated content
            with open(config_path, 'r') as f:
                raw_yaml = f.read()

            return jsonify({
                "success": True,
                "suite_id": suite_id,
                "raw_yaml": raw_yaml,
            })
        except Exception as e:
            return jsonify({"error": f"Failed to save config: {str(e)}"}), 500

    # =========================================================================
    # Test Case Editor API Endpoints
    # =========================================================================

    @app.route("/api/suites/<int:suite_id>/tests/<path:test_id>/yaml", methods=["GET"])
    def api_get_test_yaml(suite_id: int, test_id: str):
        """
        Get test case YAML content (for editor).

        Returns the raw YAML content and parsed structure.
        """
        import os
        from .yaml_utils import load_yaml_file, get_test_case_structure

        suite = repo.get_suite(suite_id)
        if not suite:
            return jsonify({"error": f"Suite not found: {suite_id}"}), 404

        # Build path to test.yaml
        # test_id is like "uc01_registry/tc01_agent_registration"
        test_path = os.path.join(suite.folder_path, "suites", test_id, "test.yaml")

        if not os.path.isfile(test_path):
            return jsonify({"error": f"Test not found: {test_id}"}), 404

        try:
            # Load with comment preservation
            yaml_data = load_yaml_file(test_path)
            structure = get_test_case_structure(yaml_data)

            # Read raw content for display
            with open(test_path, 'r') as f:
                raw_yaml = f.read()

            return jsonify({
                "suite_id": suite_id,
                "test_id": test_id,
                "path": test_path,
                "raw_yaml": raw_yaml,
                "structure": structure,
            })
        except Exception as e:
            return jsonify({"error": f"Failed to read test: {str(e)}"}), 500

    @app.route("/api/suites/<int:suite_id>/tests/<path:test_id>/yaml", methods=["PUT"])
    def api_update_test_yaml(suite_id: int, test_id: str):
        """
        Update test case YAML content.

        Can update either raw YAML or structured fields (with comment preservation).

        Request body:
            raw_yaml: str (optional) - Complete YAML content to write
            updates: dict (optional) - Structured field updates (preserves comments)
        """
        import os
        from .yaml_utils import (
            load_yaml_file, save_yaml_file, string_to_yaml,
            merge_yaml_updates, yaml_to_string
        )

        suite = repo.get_suite(suite_id)
        if not suite:
            return jsonify({"error": f"Suite not found: {suite_id}"}), 404

        test_path = os.path.join(suite.folder_path, "suites", test_id, "test.yaml")

        if not os.path.isfile(test_path):
            return jsonify({"error": f"Test not found: {test_id}"}), 404

        data = request.get_json()
        if not data:
            return jsonify({"error": "No JSON body"}), 400

        try:
            if "raw_yaml" in data:
                # Write raw YAML directly (no comment preservation)
                yaml_data = string_to_yaml(data["raw_yaml"])
                save_yaml_file(yaml_data, test_path)
            elif "updates" in data:
                # Load existing, merge updates, save (preserves comments)
                yaml_data = load_yaml_file(test_path)
                merge_yaml_updates(yaml_data, data["updates"])
                save_yaml_file(yaml_data, test_path)
            else:
                return jsonify({"error": "Must provide 'raw_yaml' or 'updates'"}), 400

            # Return updated content
            updated_yaml = load_yaml_file(test_path)
            with open(test_path, 'r') as f:
                raw_yaml = f.read()

            return jsonify({
                "success": True,
                "test_id": test_id,
                "raw_yaml": raw_yaml,
            })
        except Exception as e:
            return jsonify({"error": f"Failed to save test: {str(e)}"}), 500

    @app.route("/api/suites/<int:suite_id>/tests/<path:test_id>/steps", methods=["GET"])
    def api_get_test_steps(suite_id: int, test_id: str):
        """
        Get test steps in a structured format for the step editor.
        """
        import os
        from .yaml_utils import load_yaml_file, get_test_case_structure

        suite = repo.get_suite(suite_id)
        if not suite:
            return jsonify({"error": f"Suite not found: {suite_id}"}), 404

        test_path = os.path.join(suite.folder_path, "suites", test_id, "test.yaml")

        if not os.path.isfile(test_path):
            return jsonify({"error": f"Test not found: {test_id}"}), 404

        try:
            yaml_data = load_yaml_file(test_path)
            structure = get_test_case_structure(yaml_data)

            return jsonify({
                "test_id": test_id,
                "pre_run": structure.get("pre_run", []),
                "test": structure.get("test", []),
                "post_run": structure.get("post_run", []),
                "assertions": structure.get("assertions", []),
            })
        except Exception as e:
            return jsonify({"error": f"Failed to read steps: {str(e)}"}), 500

    @app.route("/api/suites/<int:suite_id>/tests/<path:test_id>/steps/<phase>/<int:index>", methods=["PUT"])
    def api_update_step(suite_id: int, test_id: str, phase: str, index: int):
        """
        Update a single step in a test case.

        Request body: Step fields to update
        """
        import os
        from .yaml_utils import load_yaml_file, save_yaml_file, update_step

        if phase not in ("pre_run", "test", "post_run"):
            return jsonify({"error": f"Invalid phase: {phase}"}), 400

        suite = repo.get_suite(suite_id)
        if not suite:
            return jsonify({"error": f"Suite not found: {suite_id}"}), 404

        test_path = os.path.join(suite.folder_path, "suites", test_id, "test.yaml")

        if not os.path.isfile(test_path):
            return jsonify({"error": f"Test not found: {test_id}"}), 404

        data = request.get_json()
        if not data:
            return jsonify({"error": "No JSON body"}), 400

        try:
            yaml_data = load_yaml_file(test_path)

            if not update_step(yaml_data, phase, index, data):
                return jsonify({"error": f"Step not found: {phase}[{index}]"}), 404

            save_yaml_file(yaml_data, test_path)

            return jsonify({
                "success": True,
                "phase": phase,
                "index": index,
            })
        except Exception as e:
            return jsonify({"error": f"Failed to update step: {str(e)}"}), 500

    @app.route("/api/suites/<int:suite_id>/tests/<path:test_id>/steps/<phase>", methods=["POST"])
    def api_add_step(suite_id: int, test_id: str, phase: str):
        """
        Add a new step to a phase.

        Request body:
            step: Step definition
            index: Optional index to insert at (appends if not provided)
        """
        import os
        from .yaml_utils import load_yaml_file, save_yaml_file, add_step

        if phase not in ("pre_run", "test", "post_run"):
            return jsonify({"error": f"Invalid phase: {phase}"}), 400

        suite = repo.get_suite(suite_id)
        if not suite:
            return jsonify({"error": f"Suite not found: {suite_id}"}), 404

        test_path = os.path.join(suite.folder_path, "suites", test_id, "test.yaml")

        if not os.path.isfile(test_path):
            return jsonify({"error": f"Test not found: {test_id}"}), 404

        data = request.get_json()
        if not data or "step" not in data:
            return jsonify({"error": "Must provide 'step' field"}), 400

        try:
            yaml_data = load_yaml_file(test_path)
            add_step(yaml_data, phase, data["step"], data.get("index"))
            save_yaml_file(yaml_data, test_path)

            return jsonify({
                "success": True,
                "phase": phase,
            }), 201
        except Exception as e:
            return jsonify({"error": f"Failed to add step: {str(e)}"}), 500

    @app.route("/api/suites/<int:suite_id>/tests/<path:test_id>/steps/<phase>/<int:index>", methods=["DELETE"])
    def api_delete_step(suite_id: int, test_id: str, phase: str, index: int):
        """
        Delete a step from a phase.
        """
        import os
        from .yaml_utils import load_yaml_file, save_yaml_file, remove_step

        if phase not in ("pre_run", "test", "post_run"):
            return jsonify({"error": f"Invalid phase: {phase}"}), 400

        suite = repo.get_suite(suite_id)
        if not suite:
            return jsonify({"error": f"Suite not found: {suite_id}"}), 404

        test_path = os.path.join(suite.folder_path, "suites", test_id, "test.yaml")

        if not os.path.isfile(test_path):
            return jsonify({"error": f"Test not found: {test_id}"}), 404

        try:
            yaml_data = load_yaml_file(test_path)

            if not remove_step(yaml_data, phase, index):
                return jsonify({"error": f"Step not found: {phase}[{index}]"}), 404

            save_yaml_file(yaml_data, test_path)

            return jsonify({
                "success": True,
                "phase": phase,
                "index": index,
            })
        except Exception as e:
            return jsonify({"error": f"Failed to delete step: {str(e)}"}), 500

    @app.route("/api/runs/<run_id>/rerun", methods=["POST"])
    def api_rerun(run_id: str):
        """
        Rerun a previous test run with the same filters.

        Reads the original run's suite_id and filters, then launches CLI
        with the same configuration.

        Returns:
            started: True if subprocess launched
            pid: Process ID of CLI subprocess
            description: What's being run
            original_run_id: The run being rerun
        """
        import subprocess
        import sys
        import os
        import tempfile
        from pathlib import Path

        # Get original run
        run = repo.get_run(run_id)
        if not run:
            return jsonify({"error": f"Run not found: {run_id}"}), 404

        if not run.suite_id:
            return jsonify({"error": "Cannot rerun: no suite_id associated with this run"}), 400

        suite = repo.get_suite(run.suite_id)
        if not suite:
            return jsonify({"error": f"Suite not found: {run.suite_id}"}), 404

        if not os.path.isdir(suite.folder_path):
            return jsonify({"error": f"Directory not found: {suite.folder_path}"}), 400

        # Get test IDs from original run's test_results
        original_tests = repo.list_test_results(run_id)
        if not original_tests:
            return jsonify({"error": "No tests found in original run"}), 400

        # Determine the scope: single tc, single uc, or full suite
        test_ids = [t.test_id for t in original_tests]
        use_cases = set(t.use_case for t in original_tests)

        if len(test_ids) == 1:
            # Single test case
            run_scope = ("tc", test_ids[0])
        elif len(use_cases) == 1:
            # Multiple tests but all in same use case
            run_scope = ("uc", list(use_cases)[0])
        else:
            # Multiple use cases - run full suite
            run_scope = ("all", None)

        # Determine tsuite venv path (relative to this package)
        tsuite_dir = Path(__file__).parent.parent
        venv_python = tsuite_dir / "venv" / "bin" / "python"

        if not venv_python.exists():
            # Fallback to system python
            venv_python = sys.executable

        # Build CLI command (uses --api-url to call back to this server)
        api_url = f"http://{request.host}"
        cmd = [
            str(venv_python),
            "-m", "tsuite.cli",
            "run",  # CLI requires 'run' subcommand
            "--suite-path", suite.folder_path,
            "--api-url", api_url,
        ]

        # Add scope flag based on original run
        scope_type, scope_value = run_scope
        if scope_type == "tc":
            cmd.extend(["--tc", scope_value])
        elif scope_type == "uc":
            cmd.extend(["--uc", scope_value])
        else:
            cmd.append("--all")

        # Add docker flag if mode is docker
        if suite.mode.value == "docker":
            cmd.append("--docker")

        # Start subprocess (non-blocking) - CLI will create run and update via API
        try:
            output_path = tempfile.mktemp(prefix='tsuite_run_', suffix='.log')
            output_file = open(output_path, 'w')

            process = subprocess.Popen(
                cmd,
                stdout=output_file,
                stderr=subprocess.STDOUT,
                cwd=suite.folder_path,
                env={
                    **os.environ,
                    "PYTHONUNBUFFERED": "1",
                },
            )

            # Close file handle - subprocess inherits the FD and can write independently
            output_file.close()

            # Build description of what's running
            if scope_type == "tc":
                description = f"Rerunning test case: {scope_value}"
            elif scope_type == "uc":
                description = f"Rerunning use case: {scope_value}"
            else:
                description = f"Rerunning all tests in: {suite.suite_name}"

            return jsonify({
                "started": True,
                "pid": process.pid,
                "description": description,
                "mode": suite.mode.value,
                "log_file": output_path,
                "original_run_id": run_id,
            }), 202

        except Exception as e:
            return jsonify({"error": f"Failed to start rerun: {str(e)}"}), 500

    @app.route("/api/suites/<int:suite_id>/run", methods=["POST"])
    def api_run_suite(suite_id: int):
        """
        Run tests in a suite (dumb launcher - Phase 3).

        API server just builds CLI command and launches subprocess.
        CLI will create run_id and update status via API (eventual consistency).

        Request body (optional):
            uc: Use case to run (e.g., "uc01_scaffolding")
            tc: Test case to run (e.g., "uc01_scaffolding/tc01_python_agent")
            tags: List of tags to filter by
            skip_tags: List of tags to skip

        Returns:
            started: True if subprocess launched
            pid: Process ID of CLI subprocess
            description: What's being run
        """
        import subprocess
        import sys
        import os
        import tempfile
        from pathlib import Path

        suite = repo.get_suite(suite_id)
        if not suite:
            return jsonify({"error": f"Suite not found: {suite_id}"}), 404

        if not os.path.isdir(suite.folder_path):
            return jsonify({"error": f"Directory not found: {suite.folder_path}"}), 400

        data = request.get_json() or {}
        uc = data.get("uc")
        tc = data.get("tc")
        tags = data.get("tags", [])
        skip_tags = data.get("skip_tags", [])

        # Determine tsuite venv path (relative to this package)
        tsuite_dir = Path(__file__).parent.parent
        venv_python = tsuite_dir / "venv" / "bin" / "python"

        if not venv_python.exists():
            # Fallback to system python
            venv_python = sys.executable

        # Build CLI command (uses --api-url to call back to this server)
        api_url = f"http://{request.host}"
        cmd = [
            str(venv_python),
            "-m", "tsuite.cli",
            "run",  # CLI requires 'run' subcommand
            "--suite-path", suite.folder_path,
            "--api-url", api_url,
        ]

        # Add filter flags
        if tc:
            cmd.extend(["--tc", tc])
        elif uc:
            cmd.extend(["--uc", uc])
        else:
            cmd.append("--all")

        # Add tag filters
        for tag in tags:
            cmd.extend(["--tag", tag])
        for skip_tag in skip_tags:
            cmd.extend(["--skip-tag", skip_tag])

        # Add docker flag if mode is docker
        if suite.mode.value == "docker":
            cmd.append("--docker")

        # Start subprocess (non-blocking) - CLI will create run and update via API
        try:
            output_path = tempfile.mktemp(prefix='tsuite_run_', suffix='.log')
            output_file = open(output_path, 'w')

            process = subprocess.Popen(
                cmd,
                stdout=output_file,
                stderr=subprocess.STDOUT,
                cwd=suite.folder_path,
                env={
                    **os.environ,
                    "PYTHONUNBUFFERED": "1",
                },
            )

            # Close file handle - subprocess inherits the FD and can write independently
            output_file.close()

            # Build description of what's running
            if tc:
                description = f"Running test case: {tc}"
            elif uc:
                description = f"Running use case: {uc}"
            else:
                description = f"Running all tests in: {suite.suite_name}"

            return jsonify({
                "started": True,
                "pid": process.pid,
                "description": description,
                "mode": suite.mode.value,
                "log_file": output_path,
            }), 202

        except Exception as e:
            return jsonify({"error": f"Failed to start run: {str(e)}"}), 500

    # =========================================================================
    # File Browser API Endpoint
    # =========================================================================

    @app.route("/api/browse", methods=["GET"])
    def api_browse_folders():
        """
        Browse directories for folder selection.

        Query params:
            path: Directory path to list (defaults to home directory)

        Returns:
            path: Current path
            parent: Parent directory path (null if at root)
            directories: List of subdirectories with metadata
            is_suite: Whether current path is a valid test suite
        """
        import os
        from pathlib import Path

        # Get requested path, default to home
        requested_path = request.args.get("path", "")

        if not requested_path:
            # Default to home directory
            path = Path.home()
        else:
            path = Path(os.path.expanduser(requested_path))

        # Normalize path
        try:
            path = path.resolve()
        except Exception:
            return jsonify({"error": "Invalid path"}), 400

        # Security: don't allow browsing certain system directories
        restricted_prefixes = ["/proc", "/sys", "/dev", "/etc/shadow"]
        path_str = str(path)
        for prefix in restricted_prefixes:
            if path_str.startswith(prefix):
                return jsonify({"error": "Access denied"}), 403

        if not path.exists():
            return jsonify({"error": f"Path does not exist: {path}"}), 404

        if not path.is_dir():
            return jsonify({"error": f"Not a directory: {path}"}), 400

        # Check if this is a valid test suite
        is_suite = (path / "config.yaml").exists() and (path / "suites").is_dir()

        # Get parent directory
        parent = str(path.parent) if path.parent != path else None

        # List directories
        directories = []
        try:
            for entry in sorted(path.iterdir()):
                if entry.is_dir() and not entry.name.startswith("."):
                    # Check if subdir is a suite
                    subdir_is_suite = (
                        (entry / "config.yaml").exists() and
                        (entry / "suites").is_dir()
                    )
                    directories.append({
                        "name": entry.name,
                        "path": str(entry),
                        "is_suite": subdir_is_suite,
                    })
        except PermissionError:
            return jsonify({"error": "Permission denied"}), 403

        return jsonify({
            "path": str(path),
            "parent": parent,
            "directories": directories,
            "is_suite": is_suite,
        })

    # =========================================================================
    # Static Dashboard Serving
    # Serves the pre-built Next.js static export bundled with the package.
    # =========================================================================

    # Get the dashboard static files directory (bundled with package)
    dashboard_dir = Path(__file__).parent / "dashboard"

    @app.route("/")
    def serve_dashboard_index():
        """Serve the dashboard index page."""
        if not dashboard_dir.exists():
            return jsonify({
                "error": "Dashboard not installed",
                "message": "Static dashboard files not found. Run 'npm run build' in dashboard/ and copy 'out/' to tsuite/tsuite/dashboard/"
            }), 404
        return send_from_directory(dashboard_dir, "index.html")

    @app.route("/<path:path>")
    def serve_dashboard_static(path):
        """Serve dashboard static files or fallback to index.html for SPA routing."""
        if not dashboard_dir.exists():
            return jsonify({"error": "Dashboard not installed"}), 404

        # Check if the requested file exists
        file_path = dashboard_dir / path

        # If it's a file, serve it directly
        if file_path.is_file():
            return send_from_directory(dashboard_dir, path)

        # If it's a directory with index.html (Next.js static export pattern)
        index_path = file_path / "index.html"
        if index_path.is_file():
            return send_from_directory(file_path, "index.html")

        # Fallback to main index.html for SPA client-side routing
        return send_from_directory(dashboard_dir, "index.html")

    return app


def _parse_suite_config(folder_path: str) -> dict:
    """
    Parse config.yaml and discover tests in a suite folder.

    Returns:
        dict with keys: name, mode, config, tests, test_count, use_cases
    """
    import os
    import yaml

    config_path = os.path.join(folder_path, "config.yaml")

    with open(config_path, "r") as f:
        config = yaml.safe_load(f) or {}

    # Extract suite info
    suite_config = config.get("suite", {})
    name = suite_config.get("name", os.path.basename(folder_path))
    mode = suite_config.get("mode", "docker")

    # Discover tests
    tests = []
    use_cases = {}
    suites_dir = os.path.join(folder_path, "suites")

    if os.path.isdir(suites_dir):
        for uc_name in sorted(os.listdir(suites_dir)):
            uc_path = os.path.join(suites_dir, uc_name)
            if not os.path.isdir(uc_path) or uc_name.startswith("."):
                continue

            uc_tests = []
            for tc_name in sorted(os.listdir(uc_path)):
                tc_path = os.path.join(uc_path, tc_name)
                test_yaml = os.path.join(tc_path, "test.yaml")

                if not os.path.isfile(test_yaml):
                    continue

                # Parse test.yaml for metadata
                try:
                    with open(test_yaml, "r") as f:
                        test_config = yaml.safe_load(f) or {}
                except Exception:
                    test_config = {}

                test_id = f"{uc_name}/{tc_name}"
                test_info = {
                    "test_id": test_id,
                    "use_case": uc_name,
                    "test_case": tc_name,
                    "name": test_config.get("name", tc_name),
                    "description": test_config.get("description", ""),
                    "tags": test_config.get("tags", []),
                    "steps_count": len(test_config.get("steps", [])),
                }
                tests.append(test_info)
                uc_tests.append(test_info)

            if uc_tests:
                use_cases[uc_name] = {
                    "id": uc_name,
                    "name": uc_name.replace("_", " ").title(),
                    "test_count": len(uc_tests),
                }

    return {
        "name": name,
        "mode": mode,
        "config": config,
        "tests": tests,
        "test_count": len(tests),
        "use_cases": list(use_cases.values()),
    }


def main():
    """
    Run the API server standalone for the dashboard.

    Usage:
        python -m tsuite.server [--port PORT] [--suites PATHS]

    Examples:
        python -m tsuite.server --port 9999 --suites integration,lib-tests
    """
    import argparse
    from pathlib import Path

    parser = argparse.ArgumentParser(description="Run tsuite API server for dashboard")
    parser.add_argument("--port", type=int, default=9999, help="Server port (default: 9999)")
    parser.add_argument("--suites", type=str, help="Comma-separated list of suite paths to sync")
    args = parser.parse_args()

    # Initialize database
    db.init_db()

    # Sync suites if provided
    if args.suites:
        for suite_path in args.suites.split(","):
            suite_dir = Path(suite_path.strip()).resolve()
            config_file = suite_dir / "config.yaml"
            if suite_dir.exists() and config_file.exists():
                config = load_config(config_file)
                if config:
                    # Count tests
                    discovery = TestDiscovery(suite_dir)
                    test_count = len(discovery.discover_tests())

                    # Sync to DB
                    from .cli import sync_suite_to_db
                    sync_suite_to_db(suite_dir, config, test_count)
                    print(f"Synced suite: {suite_dir.name} ({test_count} tests)")

    # Start server (blocking)
    print(f"Starting API server on http://localhost:{args.port}")
    app = create_app()
    server = make_server("0.0.0.0", args.port, app, threaded=True)
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        print("\nShutting down...")
        server.shutdown()


if __name__ == "__main__":
    main()
