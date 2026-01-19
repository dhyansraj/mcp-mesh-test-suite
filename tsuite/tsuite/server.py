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
from flask import Flask, jsonify, request, Response
import json

from werkzeug.serving import make_server

from .context import runtime
from . import repository as repo
from .models import RunStatus, TestStatus

# Suppress Flask's default logging
log = logging.getLogger("werkzeug")
log.setLevel(logging.ERROR)


def create_app() -> Flask:
    """Create the Flask application."""
    app = Flask(__name__)

    @app.route("/health", methods=["GET"])
    def health():
        """Health check endpoint."""
        return jsonify({"status": "ok"})

    @app.route("/config", methods=["GET"])
    def get_config():
        """Get full configuration."""
        return jsonify(runtime.get_config())

    @app.route("/config/<path:path>", methods=["GET"])
    def get_config_value(path: str):
        """Get specific configuration value by dot-notation path."""
        # Convert URL path to dot notation (config/packages/cli_version -> packages.cli_version)
        dot_path = path.replace("/", ".")
        value = runtime.get_config(dot_path)
        if value is None:
            return jsonify({"error": f"Config path not found: {dot_path}"}), 404
        return jsonify({"value": value})

    @app.route("/routine/<scope>/<name>", methods=["GET"])
    def get_routine(scope: str, name: str):
        """Get routine definition by scope and name."""
        routine = runtime.get_routine(scope, name)
        if routine is None:
            return jsonify({"error": f"Routine not found: {scope}.{name}"}), 404
        return jsonify(routine)

    @app.route("/routines", methods=["GET"])
    def get_all_routines():
        """Get all routines."""
        return jsonify(runtime.get_all_routines())

    @app.route("/state/<path:test_id>", methods=["GET"])
    def get_state(test_id: str):
        """Get state from a test."""
        state = runtime.get_test_state(test_id)
        return jsonify(state)

    @app.route("/state/<path:test_id>", methods=["POST"])
    def update_state(test_id: str):
        """Merge state into a test."""
        data = request.get_json() or {}
        runtime.update_test_state(test_id, data)
        return jsonify({"status": "ok"})

    @app.route("/capture/<path:test_id>", methods=["POST"])
    def capture(test_id: str):
        """Store a captured variable."""
        data = request.get_json() or {}
        for name, value in data.items():
            runtime.set_captured(test_id, name, value)
        return jsonify({"status": "ok"})

    @app.route("/progress/<path:test_id>", methods=["POST"])
    def progress(test_id: str):
        """Update progress for a test."""
        data = request.get_json() or {}
        runtime.update_progress(
            test_id,
            step=data.get("step", 0),
            status=data.get("status", "running"),
            message=data.get("message", ""),
        )
        return jsonify({"status": "ok"})

    @app.route("/progress/<path:test_id>", methods=["GET"])
    def get_progress(test_id: str):
        """Get progress for a test."""
        return jsonify(runtime.get_progress(test_id))

    @app.route("/log/<path:test_id>", methods=["POST"])
    def log_message(test_id: str):
        """Log a message from a test."""
        data = request.get_json() or {}
        level = data.get("level", "info")
        message = data.get("message", "")
        # For now, just print. Later can stream to UI.
        print(f"[{test_id}] [{level.upper()}] {message}")
        return jsonify({"status": "ok"})

    @app.route("/context/<path:test_id>", methods=["GET"])
    def get_context(test_id: str):
        """Get full test context."""
        ctx = runtime.get_test_context(test_id)
        if ctx is None:
            return jsonify({"error": f"Test context not found: {test_id}"}), 404
        return jsonify(ctx.to_dict())

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

        Clients can subscribe to get real-time status changes.
        """
        def generate():
            # Initial state
            summary = repo.get_run_summary(run_id)
            if summary:
                yield f"data: {json.dumps(summary.to_dict())}\n\n"

            # For now, just send initial state
            # Future: implement proper event streaming with polling or pubsub
            yield f"data: {json.dumps({'type': 'connected', 'run_id': run_id})}\n\n"

        return Response(
            generate(),
            mimetype="text/event-stream",
            headers={
                "Cache-Control": "no-cache",
                "Connection": "keep-alive",
            },
        )

    return app


class RunnerServer:
    """
    Manages the Flask server lifecycle.

    Runs in a background thread so tests can execute concurrently.
    """

    def __init__(self, host: str = "0.0.0.0", port: int = 9999):
        self.host = host
        self.port = port
        self.app = create_app()
        self.server = None
        self.thread = None

    def start(self):
        """Start the server in a background thread."""
        self.server = make_server(self.host, self.port, self.app, threaded=True)
        self.thread = threading.Thread(target=self.server.serve_forever)
        self.thread.daemon = True
        self.thread.start()

    def stop(self):
        """Stop the server."""
        if self.server:
            self.server.shutdown()
        if self.thread:
            self.thread.join(timeout=5)

    def get_url(self) -> str:
        """Get the server URL for containers to use."""
        # For Docker containers on Mac/Windows, use host.docker.internal
        # On Linux, we'd need to use the host's IP or --network=host
        return f"http://host.docker.internal:{self.port}"

    def __enter__(self):
        self.start()
        return self

    def __exit__(self, exc_type, exc_val, exc_tb):
        self.stop()
