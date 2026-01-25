"""
Command-line interface for tsuite (mcp-mesh-tsuite).

Usage:
    tsuite run --all                    # Run all tests (local mode)
    tsuite run --all --docker           # Run all tests in Docker containers
    tsuite run --uc uc01_scaffolding    # Run all tests in a use case folder
    tsuite run --tc uc01/tc01           # Run specific test (uc_folder/tc_folder)
    tsuite run --dry-run --all          # List tests without running

    tsuite api                          # Start API server with dashboard
    tsuite api --port 8080              # Start on custom port

    tsuite clear                        # Clear all test data
    tsuite clear --run-id <id>          # Clear specific run

    tsuite man --list                   # List available topics
    tsuite man quickstart               # View man page

Note: Test IDs use the format <uc_folder>/<tc_folder> (e.g., uc05_meshctl/tc05_status_healthy_resolution)
"""

import sys
import time
import subprocess
import tempfile
from pathlib import Path
from dataclasses import dataclass
from datetime import datetime
from typing import Optional

import click
import requests
from rich.console import Console
from rich.table import Table
from rich.panel import Panel
from rich.progress import Progress, SpinnerColumn, TextColumn, TimeElapsedColumn

from .context import runtime
from .discovery import TestDiscovery, load_config
from .executor import TestExecutor, TestResult
from .routines import RoutineResolver
from . import db
from . import repository as repo
from .models import RunStatus, TestStatus
from . import reporter
from .sse import sse_manager
from .models import SuiteMode

console = Console()

# Global run_id for current execution
_current_run_id: Optional[str] = None

# Default report directory
DEFAULT_REPORT_DIR = Path.home() / ".tsuite" / "reports"


@dataclass
class DockerTestResult:
    """Result from Docker-based test execution."""
    test_id: str
    test_name: str
    passed: bool
    duration: float
    stdout: str
    stderr: str
    error: str | None


# =============================================================================
# API Server Helper Functions
# =============================================================================

def health_check(api_url: str, timeout: int = 5) -> bool:
    """
    Check if API server is running.

    Args:
        api_url: URL of the API server (e.g., "http://localhost:9999")
        timeout: Request timeout in seconds

    Returns:
        True if server is healthy, False otherwise.
    """
    try:
        resp = requests.get(f"{api_url}/health", timeout=timeout)
        return resp.status_code == 200
    except requests.RequestException:
        return False


def ensure_api_server(port: int = 9999, timeout: int = 10) -> str:
    """
    Start API server if not running, return URL.

    Args:
        port: Port for the API server
        timeout: Max seconds to wait for server to start

    Returns:
        API server URL

    Raises:
        RuntimeError: If server fails to start within timeout
    """
    api_url = f"http://localhost:{port}"

    # Check if already running
    if health_check(api_url):
        return api_url

    console.print(f"[dim]Starting API server on port {port}...[/dim]")

    # Start server in background
    subprocess.Popen(
        [sys.executable, "-m", "tsuite.server", "--port", str(port)],
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    )

    # Wait for server to be ready
    start_time = time.time()
    while time.time() - start_time < timeout:
        if health_check(api_url, timeout=2):
            console.print(f"[dim]API server started at {api_url}[/dim]")
            return api_url
        time.sleep(0.2)

    raise RuntimeError(f"Failed to start API server on port {port} within {timeout}s")


def create_run_via_api(api_url: str, suite_id: int, tests: list, filters: dict | None = None, mode: str = "docker") -> str:
    """
    Create a new test run via API.

    Args:
        api_url: URL of the API server
        suite_id: Database ID of the suite
        tests: List of test objects to run
        filters: Optional filters used for test selection
        mode: Execution mode ("docker" or "standalone")

    Returns:
        run_id: The created run's ID

    Raises:
        RuntimeError: If API call fails
    """
    # Build test list for API
    test_data = [
        {
            "test_id": t.id,
            "use_case": t.uc,
            "test_case": t.tc,
            "name": t.name,
            "tags": t.tags or [],
        }
        for t in tests
    ]

    payload = {
        "suite_id": suite_id,
        "tests": test_data,
        "mode": mode,
    }
    if filters:
        payload["filters"] = filters

    try:
        resp = requests.post(f"{api_url}/api/runs", json=payload, timeout=30)
        resp.raise_for_status()
        data = resp.json()
        return data["run_id"]
    except requests.RequestException as e:
        raise RuntimeError(f"Failed to create run via API: {e}")


def start_run_via_api(api_url: str, run_id: str) -> None:
    """Signal that a run has started."""
    try:
        resp = requests.post(f"{api_url}/api/runs/{run_id}/start", timeout=10)
        resp.raise_for_status()
    except requests.RequestException as e:
        console.print(f"[yellow]Warning: Failed to signal run start: {e}[/yellow]")


def complete_run_via_api(api_url: str, run_id: str, duration_ms: int | None = None) -> None:
    """Signal that a run has completed."""
    try:
        payload = {}
        if duration_ms is not None:
            payload["duration_ms"] = duration_ms
        resp = requests.post(f"{api_url}/api/runs/{run_id}/complete", json=payload, timeout=10)
        resp.raise_for_status()
    except requests.RequestException as e:
        console.print(f"[yellow]Warning: Failed to signal run complete: {e}[/yellow]")


def update_test_status_via_api(
    api_url: str,
    run_id: str,
    test_id: str,
    status: str,
    duration_ms: int | None = None,
    error_message: str | None = None,
    steps_passed: int | None = None,
    steps_failed: int | None = None,
) -> None:
    """Update test status via API."""
    try:
        payload = {"status": status}
        if duration_ms is not None:
            payload["duration_ms"] = duration_ms
        if error_message is not None:
            payload["error_message"] = error_message
        if steps_passed is not None:
            payload["steps_passed"] = steps_passed
        if steps_failed is not None:
            payload["steps_failed"] = steps_failed

        resp = requests.patch(f"{api_url}/api/runs/{run_id}/tests/{test_id}", json=payload, timeout=10)
        resp.raise_for_status()
    except requests.RequestException as e:
        console.print(f"[yellow]Warning: Failed to update test status: {e}[/yellow]")


def check_cancel_requested(api_url: str, run_id: str) -> bool:
    """
    Check if cancellation has been requested for a run.

    Called before starting each test for cooperative cancellation.
    """
    try:
        resp = requests.get(f"{api_url}/api/runner/should-cancel/{run_id}", timeout=5)
        if resp.status_code == 200:
            return resp.json().get("cancel_requested", False)
    except requests.RequestException:
        pass  # On error, continue execution
    return False


def finalize_cancelled_run(run_id: str, skip_reason: str = "Run cancelled") -> int:
    """
    Mark all pending tests as skipped and complete the run as cancelled.

    Returns the number of tests that were skipped.
    """
    return repo.cancel_run_with_skip(run_id, skip_reason)


# =============================================================================
# Worker Pool for Parallel Execution
# =============================================================================

class TestTimeoutError(Exception):
    """Raised when a test exceeds its timeout."""
    pass


class WorkerPool:
    """
    Thread pool for parallel test execution.

    Used in docker mode to run multiple containers concurrently.
    Standalone mode should always use max_workers=1.
    """

    def __init__(self, max_workers: int = 1):
        from concurrent.futures import ThreadPoolExecutor
        self.max_workers = max(1, max_workers)
        self.executor: ThreadPoolExecutor | None = None
        self._cancelled = False

    def __enter__(self):
        from concurrent.futures import ThreadPoolExecutor
        self.executor = ThreadPoolExecutor(max_workers=self.max_workers)
        return self

    def __exit__(self, *args):
        if self.executor:
            # If cancelled, don't wait for running tasks
            self.executor.shutdown(wait=not self._cancelled, cancel_futures=self._cancelled)

    def cancel(self):
        """Mark the pool as cancelled - exit won't wait for futures."""
        self._cancelled = True

    def submit(self, fn, *args, **kwargs):
        """Submit a task to the pool."""
        if self.executor is None:
            raise RuntimeError("WorkerPool not entered as context manager")
        return self.executor.submit(fn, *args, **kwargs)

    def map_unordered(self, fn, items):
        """
        Execute fn for each item, yielding results as they complete.

        Results may be returned in any order (not necessarily input order).
        """
        from concurrent.futures import as_completed

        if self.executor is None:
            raise RuntimeError("WorkerPool not entered as context manager")

        futures = {self.executor.submit(fn, item): item for item in items}
        for future in as_completed(futures):
            try:
                yield future.result()
            except Exception as e:
                # Return error result for this item
                item = futures[future]
                yield {"test_id": getattr(item, 'id', str(item)), "error": str(e), "passed": False}


def execute_with_timeout(fn, timeout_seconds: int):
    """
    Execute a function with a timeout.

    Args:
        fn: Callable to execute
        timeout_seconds: Maximum execution time in seconds

    Returns:
        Result of fn()

    Raises:
        TestTimeoutError: If execution exceeds timeout
    """
    from concurrent.futures import ThreadPoolExecutor, TimeoutError as FuturesTimeoutError

    with ThreadPoolExecutor(max_workers=1) as executor:
        future = executor.submit(fn)
        try:
            return future.result(timeout=timeout_seconds)
        except FuturesTimeoutError:
            raise TestTimeoutError(f"Execution exceeded {timeout_seconds}s timeout")


def get_handlers() -> dict:
    """Load all available handlers."""
    from handlers import shell, file, routine, http, wait, llm, pip_install, npm_install

    return {
        "shell": shell.execute,
        "file": file.execute,
        "routine": routine.execute,
        "http": http.execute,
        "wait": wait.execute,
        "llm": llm.execute,
        "pip-install": pip_install.execute,
        "npm-install": npm_install.execute,
    }


def sync_suite_to_db(suite_path: Path, config: dict, test_count: int):
    """
    Sync the suite configuration to the database.
    Called when running tests from CLI to keep DB in sync with YAML.

    Returns:
        Suite object with id for linking to runs
    """
    import json
    from .models import Suite

    folder_path = str(suite_path.resolve())

    # Get suite info from config
    suite_config = config.get("suite", {})
    suite_name = suite_config.get("name", suite_path.name)
    mode_str = suite_config.get("mode", "docker")

    try:
        mode = SuiteMode(mode_str)
    except ValueError:
        mode = SuiteMode.DOCKER

    # Upsert suite (create or update)
    suite = repo.upsert_suite(
        folder_path=folder_path,
        suite_name=suite_name,
        mode=mode,
        config_json=json.dumps(config),
        test_count=test_count,
    )

    console.print(f"[dim]Suite synced: {suite_name}[/dim]")
    return suite


def print_banner(config: dict, test_count: int):
    """Print the startup banner."""
    version = config.get("packages", {}).get("cli_version", "unknown")
    console.print(Panel(
        f"[bold blue]MCP Mesh Integration Test Suite[/bold blue]\n"
        f"Version: {version} | Tests: {test_count}",
        expand=False,
    ))


def print_test_result(result: TestResult, verbose: bool = False):
    """Print result of a single test."""
    status = "[green]PASSED[/green]" if result.passed else "[red]FAILED[/red]"
    console.print(f"\n[bold]{result.test_id}[/bold] - {status} ({result.duration:.1f}s)")

    if not result.passed or verbose:
        # Show step results
        for sr in result.step_results:
            step_status = "[green]OK[/green]" if sr["result"].success else "[red]FAIL[/red]"
            step_name = sr["step"].get("handler") or sr["step"].get("routine", "unknown")
            console.print(f"  [{sr['phase']}] {step_name}: {step_status}")

            if not sr["result"].success:
                if sr["result"].error:
                    console.print(f"    [red]Error: {sr['result'].error}[/red]")
                if sr["result"].stderr:
                    console.print(f"    [dim]stderr: {sr['result'].stderr[:200]}[/dim]")

        # Show assertion results
        for ar in result.assertion_results:
            a_status = "[green]PASS[/green]" if ar["passed"] else "[red]FAIL[/red]"
            console.print(f"  [assert] {ar['message']}: {a_status}")
            if not ar["passed"]:
                console.print(f"    [dim]{ar['details']}[/dim]")

    if result.error:
        console.print(f"  [red]Error: {result.error}[/red]")


def print_summary(results: list[TestResult], run_id: Optional[str] = None):
    """Print summary of all test results."""
    passed = sum(1 for r in results if r.passed)
    failed = sum(1 for r in results if not r.passed)
    total_time = sum(r.duration for r in results)

    console.print("\n" + "=" * 60)
    console.print(
        f"[bold]SUMMARY:[/bold] "
        f"[green]{passed} passed[/green], "
        f"[red]{failed} failed[/red] "
        f"({total_time:.1f}s total)"
    )
    if run_id:
        console.print(f"[dim]Run ID: {run_id}[/dim]")
    console.print("=" * 60)


def print_history(limit: int = 10):
    """Print recent test runs."""
    runs = repo.list_runs(limit=limit)

    if not runs:
        console.print("[yellow]No test runs found[/yellow]")
        return

    table = Table(title="Recent Test Runs")
    table.add_column("Run ID", style="cyan", max_width=12)
    table.add_column("Started", style="dim")
    table.add_column("Status")
    table.add_column("Tests", justify="right")
    table.add_column("Passed", style="green", justify="right")
    table.add_column("Failed", style="red", justify="right")
    table.add_column("Duration", justify="right")

    for run in runs:
        status_color = {
            RunStatus.COMPLETED: "green",
            RunStatus.FAILED: "red",
            RunStatus.RUNNING: "yellow",
            RunStatus.PENDING: "dim",
            RunStatus.CANCELLED: "dim",
        }.get(run.status, "white")

        duration = f"{run.duration_ms / 1000:.1f}s" if run.duration_ms else "-"
        started = run.started_at.strftime("%Y-%m-%d %H:%M") if run.started_at else "-"

        table.add_row(
            run.run_id[:12],
            started,
            f"[{status_color}]{run.status.value}[/{status_color}]",
            str(run.total_tests),
            str(run.passed),
            str(run.failed),
            duration,
        )

    console.print(table)


def generate_report_for_run(
    run_id: str,
    report_dir: str | None,
    formats: tuple,
):
    """Generate reports for a specific run."""
    output_dir = Path(report_dir) if report_dir else DEFAULT_REPORT_DIR

    # If run_id is partial, try to find matching run
    if len(run_id) < 36:
        runs = repo.list_runs(limit=100)
        matching = [r for r in runs if r.run_id.startswith(run_id)]
        if not matching:
            console.print(f"[red]No run found matching: {run_id}[/red]")
            return
        if len(matching) > 1:
            console.print(f"[yellow]Multiple runs match '{run_id}':[/yellow]")
            for r in matching[:5]:
                console.print(f"  {r.run_id}")
            return
        run_id = matching[0].run_id

    formats_list = list(formats) if formats else ["html", "json", "junit"]

    console.print(f"Generating reports for run [cyan]{run_id[:12]}[/cyan]...")

    try:
        outputs = reporter.generate_report(
            run_id=run_id,
            output_dir=output_dir,
            formats=formats_list,
        )

        console.print("[green]Reports generated:[/green]")
        for fmt, path in outputs.items():
            console.print(f"  {fmt}: {path}")

    except ValueError as e:
        console.print(f"[red]Error: {e}[/red]")


def generate_comparison(
    run_id_1: str,
    run_id_2: str,
    report_dir: str | None,
):
    """Generate comparison report between two runs."""
    output_dir = Path(report_dir) if report_dir else DEFAULT_REPORT_DIR

    # Resolve partial run IDs
    def resolve_run_id(partial: str) -> str | None:
        if len(partial) >= 36:
            return partial
        runs = repo.list_runs(limit=100)
        matching = [r for r in runs if r.run_id.startswith(partial)]
        if len(matching) == 1:
            return matching[0].run_id
        return None

    resolved_1 = resolve_run_id(run_id_1)
    resolved_2 = resolve_run_id(run_id_2)

    if not resolved_1:
        console.print(f"[red]Could not resolve run ID: {run_id_1}[/red]")
        return
    if not resolved_2:
        console.print(f"[red]Could not resolve run ID: {run_id_2}[/red]")
        return

    console.print(f"Comparing [cyan]{resolved_1[:12]}[/cyan] vs [cyan]{resolved_2[:12]}[/cyan]...")

    try:
        # Generate both HTML and JSON
        html_path = reporter.generate_comparison_report(
            resolved_1, resolved_2, output_dir, format="html"
        )
        json_path = reporter.generate_comparison_report(
            resolved_1, resolved_2, output_dir, format="json"
        )

        console.print("[green]Comparison reports generated:[/green]")
        console.print(f"  html: {html_path}")
        console.print(f"  json: {json_path}")

    except ValueError as e:
        console.print(f"[red]Error: {e}[/red]")


@click.group(invoke_without_command=True)
@click.pass_context
@click.version_option(version="0.1.0", prog_name="mcp-mesh-tsuite")
def main(ctx):
    """
    mcp-mesh-tsuite - YAML-driven integration test framework.

    Run 'tsuite <command> --help' for command-specific help.
    """
    if ctx.invoked_subcommand is None:
        click.echo(ctx.get_help())


@main.command("run")
@click.option("--all", "run_all", is_flag=True, help="Run all tests")
@click.option("--uc", multiple=True, help="Run all tests in use case folder(s) (e.g., --uc uc01_scaffolding)")
@click.option("--tc", multiple=True, help="Run specific test(s) using uc_folder/tc_folder format (e.g., --tc uc05_meshctl/tc05_status)")
@click.option("--tag", multiple=True, help="Filter by tag(s)")
@click.option("--pattern", help="Filter by glob pattern")
@click.option("--dry-run", is_flag=True, help="List tests without running")
@click.option("--verbose", "-v", is_flag=True, help="Verbose output")
@click.option("--stop-on-fail", is_flag=True, help="Stop on first failure")
@click.option("--suite-path", type=click.Path(exists=True), help="Path to test suite")
@click.option("--docker", is_flag=True, help="Run tests in Docker containers")
@click.option("--image", default=None, help="Docker image to use (overrides config)")
@click.option("--db-path", type=click.Path(), help="Path to results database")
@click.option("--history", is_flag=True, help="Show recent test runs")
@click.option("--report", is_flag=True, help="Generate reports after run")
@click.option("--report-dir", type=click.Path(), help="Directory for reports")
@click.option("--report-format", multiple=True, help="Report formats: html, json, junit")
@click.option("--report-run", help="Generate report for a specific run ID")
@click.option("--compare", nargs=2, help="Compare two runs (provide two run IDs)")
@click.option("--retry-failed", is_flag=True, help="Retry failed tests from last run")
@click.option("--mock-llm", is_flag=True, help="Use mock LLM responses (no API calls)")
@click.option("--skip-tag", multiple=True, help="Skip tests with specific tag(s)")
@click.option("--api-url", default="http://localhost:9999", help="API server URL for SSE event forwarding")
def run_cmd(
    run_all: bool,
    uc: tuple,
    tc: tuple,
    tag: tuple,
    pattern: str | None,
    dry_run: bool,
    verbose: bool,
    stop_on_fail: bool,
    suite_path: str | None,
    docker: bool,
    image: str | None,
    db_path: str | None,
    history: bool,
    report: bool,
    report_dir: str | None,
    report_format: tuple,
    report_run: str | None,
    compare: tuple | None,
    retry_failed: bool,
    mock_llm: bool,
    skip_tag: tuple,
    api_url: str,
):
    """Run integration tests.

    Examples:
        tsuite run --all                     Run all tests
        tsuite run --all --docker            Run in Docker containers
        tsuite run --uc uc01_users           Run use case
        tsuite run --tc uc01/tc01            Run specific test
        tsuite run --dry-run --all           List tests without running
    """
    global _current_run_id

    # Set mock LLM mode
    if mock_llm:
        import os
        os.environ["TSUITE_MOCK_LLM"] = "true"
        console.print("[dim]Mock LLM mode enabled[/dim]")

    # Configure SSE event forwarding to API server
    sse_manager.set_event_server(api_url)
    console.print(f"[dim]SSE forwarding: {api_url}[/dim]")

    # Initialize database
    if db_path:
        db.set_db_path(Path(db_path))
    db.init_db()

    # Ensure API server is running (required for docker mode and SSE forwarding)
    # Parse port from api_url
    from urllib.parse import urlparse
    parsed = urlparse(api_url)
    api_port = parsed.port or 9999
    try:
        api_url = ensure_api_server(port=api_port, timeout=15)
    except RuntimeError as e:
        console.print(f"[yellow]Warning: {e}[/yellow]")
        console.print("[dim]Continuing without API server (SSE events will not be forwarded)[/dim]")

    # Show history and exit
    if history:
        print_history()
        sys.exit(0)

    # Generate report for historical run
    if report_run:
        generate_report_for_run(report_run, report_dir, report_format)
        sys.exit(0)

    # Compare two runs
    if compare:
        generate_comparison(compare[0], compare[1], report_dir)
        sys.exit(0)

    # Handle retry-failed - get failed test IDs from last run
    failed_test_ids = None
    if retry_failed:
        latest_run = repo.get_latest_run()
        if not latest_run:
            console.print("[red]No previous run found[/red]")
            sys.exit(1)

        failed_tests = [
            t for t in repo.list_test_results(latest_run.run_id)
            if t.status == TestStatus.FAILED
        ]

        if not failed_tests:
            console.print(f"[green]No failed tests in last run ({latest_run.run_id[:8]})[/green]")
            sys.exit(0)

        failed_test_ids = [t.test_id for t in failed_tests]
        console.print(f"[yellow]Retrying {len(failed_test_ids)} failed test(s) from run {latest_run.run_id[:8]}[/yellow]")
    # Determine suite path (always resolve to absolute for Docker volume mounts)
    if suite_path:
        suite = Path(suite_path).resolve()
    else:
        # Try to find suite in current directory or parent
        cwd = Path.cwd()
        if (cwd / "config.yaml").exists():
            suite = cwd
        elif (cwd / "integration" / "config.yaml").exists():
            suite = cwd / "integration"
        else:
            console.print("[red]Error: Could not find test suite. Use --suite-path[/red]")
            sys.exit(1)

    # Load configuration
    config = load_config(suite / "config.yaml")
    runtime.set_config(config)

    # Discover tests and routines
    discovery = TestDiscovery(suite)
    all_tests = discovery.discover_tests()
    routine_sets = discovery.discover_routines()

    # Sync suite to database (keeps DB in sync with YAML for dashboard)
    suite_record = sync_suite_to_db(suite, config, len(all_tests))

    # Filter tests
    if retry_failed and failed_test_ids:
        # For retry-failed, filter to only failed tests
        tests = [t for t in all_tests if t.id in failed_test_ids]
    else:
        if not run_all and not uc and not tc:
            console.print("[yellow]No tests selected. Use --all, --uc, or --tc[/yellow]")
            console.print("[dim]Examples:[/dim]")
            console.print("[dim]  tsuite --all                           # Run all tests[/dim]")
            console.print("[dim]  tsuite --uc uc01_scaffolding           # Run all tests in use case[/dim]")
            console.print("[dim]  tsuite --tc uc05_meshctl/tc05_status   # Run specific test (uc_folder/tc_folder)[/dim]")
            sys.exit(0)

        tests = discovery.filter_tests(
            all_tests,
            uc=list(uc) if uc else None,
            tc=list(tc) if tc else None,
            tags=list(tag) if tag else None,
            pattern=pattern,
        )

        # Filter out tests with skip tags
        if skip_tag:
            skip_tags = set(skip_tag)
            before_count = len(tests)
            tests = [t for t in tests if not any(tag in skip_tags for tag in t.tags)]
            skipped_count = before_count - len(tests)
            if skipped_count > 0:
                console.print(f"[dim]Skipped {skipped_count} test(s) with tags: {', '.join(skip_tags)}[/dim]")

    if not tests:
        console.print("[yellow]No tests match the criteria[/yellow]")
        console.print("[dim]Tip: Use --dry-run --all to list available tests[/dim]")
        console.print("[dim]Test IDs use format: uc_folder/tc_folder (e.g., uc05_meshctl/tc05_status_healthy_resolution)[/dim]")
        sys.exit(0)

    # Dry run: just list tests
    if dry_run:
        table = Table(title="Tests to run")
        table.add_column("ID", style="cyan")
        table.add_column("Name")
        table.add_column("Tags")

        for test in tests:
            table.add_row(test.id, test.name, ", ".join(test.tags))

        console.print(table)
        console.print(f"\n[bold]{len(tests)} test(s) would run[/bold]")
        sys.exit(0)

    # Print banner
    print_banner(config, len(tests))

    # Create temp workdir
    workdir = Path(tempfile.mkdtemp(prefix="tsuite_"))
    console.print(f"[dim]Workdir: {workdir}[/dim]")

    # Get docker image for database record
    docker_config = config.get("docker", {})
    docker_image = image or docker_config.get("base_image", "python:3.11-slim")

    # Create run record in database
    packages = config.get("packages", {})
    run = repo.create_run(
        suite_id=suite_record.id,
        cli_version=packages.get("cli_version"),
        sdk_python_version=packages.get("sdk_python_version"),
        sdk_typescript_version=packages.get("sdk_typescript_version"),
        docker_image=docker_image if docker else None,
        total_tests=len(tests),
    )
    _current_run_id = run.run_id
    console.print(f"[dim]Run ID: {run.run_id[:12]}...[/dim]")

    # Emit SSE run_started event
    sse_manager.emit_run_started(run.run_id, len(tests))

    # Create test result records for all tests (PENDING status)
    for test in tests:
        parts = test.id.split("/")
        use_case = parts[0] if len(parts) > 0 else ""
        test_case = parts[1] if len(parts) > 1 else ""

        repo.create_test_result(
            run_id=run.run_id,
            test_id=test.id,
            use_case=use_case,
            test_case=test_case,
            name=test.name,
            tags=test.tags,
        )

    # Docker mode or local mode
    if docker:
        results = run_docker_mode(
            tests=tests,
            config=config,
            suite=suite,
            workdir=workdir,
            verbose=verbose,
            stop_on_fail=stop_on_fail,
            image_override=image,
            run_id=run.run_id,
            api_url=api_url,
        )
    else:
        results = run_local_mode(
            tests=tests,
            routine_sets=routine_sets,
            suite=suite,
            workdir=workdir,
            verbose=verbose,
            stop_on_fail=stop_on_fail,
            run_id=run.run_id,
            api_url=api_url,
        )

    # Complete run record (unless already cancelled)
    current_run = repo.get_run(run.run_id)
    if current_run and current_run.status != RunStatus.CANCELLED:
        repo.complete_run(run.run_id)

        # Emit SSE run_completed event
        passed_count = sum(1 for r in results if r.passed)
        failed_count = sum(1 for r in results if not r.passed)
        total_duration_ms = int(sum(r.duration for r in results) * 1000)
        sse_manager.emit_run_completed(
            run_id=run.run_id,
            passed=passed_count,
            failed=failed_count,
            skipped=0,
            duration_ms=total_duration_ms,
        )

    # Print summary
    print_summary(results, run.run_id)

    # Generate reports if requested
    if report:
        output_dir = Path(report_dir) if report_dir else DEFAULT_REPORT_DIR
        formats_list = list(report_format) if report_format else ["html", "json", "junit"]

        console.print("\n[dim]Generating reports...[/dim]")
        try:
            outputs = reporter.generate_report(
                run_id=run.run_id,
                output_dir=output_dir,
                formats=formats_list,
            )
            console.print("[green]Reports:[/green]")
            for fmt, path in outputs.items():
                console.print(f"  {fmt}: {path}")
        except Exception as e:
            console.print(f"[red]Failed to generate reports: {e}[/red]")

    # Exit with appropriate code
    failed = sum(1 for r in results if not r.passed)
    sys.exit(1 if failed > 0 else 0)


def run_local_mode(
    tests: list,
    routine_sets: dict,
    suite: Path,
    workdir: Path,
    verbose: bool,
    stop_on_fail: bool,
    run_id: str | None = None,
    api_url: str | None = None,
) -> list[TestResult]:
    """
    Run tests in local/standalone mode (no Docker).

    Always sequential (max_workers=1) because tests share the same
    filesystem and may have dependencies on each other.
    Use docker mode for parallel execution.
    """
    # Setup routine resolver
    routine_resolver = RoutineResolver(routine_sets)

    # Load handlers - framework is relative to this file's location
    framework_path = Path(__file__).parent.parent
    sys.path.insert(0, str(framework_path))
    handlers = get_handlers()

    results = []

    # Use API server URL
    server_url = api_url or "http://localhost:9999"
    console.print(f"[dim]API Server: {server_url}[/dim]")
    console.print(f"[dim]Mode: local (standalone)[/dim]\n")

    executor = TestExecutor(
        handlers=handlers,
        routine_resolver=routine_resolver,
        server_url=server_url,
        base_workdir=workdir,
        suite_path=suite,
    )

    cancel_requested = False

    with Progress(
        SpinnerColumn(),
        TextColumn("[progress.description]{task.description}"),
        TimeElapsedColumn(),
        console=console,
    ) as progress:
        task = progress.add_task("Running tests...", total=len(tests))

        for i, test in enumerate(tests):
            current_run_id = run_id or _current_run_id

            # Check for cancellation before starting each test
            if current_run_id and check_cancel_requested(server_url, current_run_id):
                console.print("\n[yellow]Cancellation requested - skipping remaining tests[/yellow]")
                cancel_requested = True
                break

            progress.update(task, description=f"[{i+1}/{len(tests)}] {test.id}")

            # Mark test as running via API (updates run counters)
            if current_run_id:
                update_test_status_via_api(
                    server_url,
                    current_run_id,
                    test.id,
                    status="running",
                )

            # Emit SSE test_started event
            if current_run_id:
                sse_manager.emit_test_started(current_run_id, test.id, test.name)

            result = executor.execute(test)
            results.append(result)

            # Update test result via API (updates run counters)
            if current_run_id:
                status = "passed" if result.passed else "failed"
                update_test_status_via_api(
                    server_url,
                    current_run_id,
                    test.id,
                    status=status,
                    duration_ms=int(result.duration * 1000),
                    error_message=result.error,
                )

            # Emit SSE test_completed event
            if current_run_id:
                sse_manager.emit_test_completed(
                    run_id=current_run_id,
                    test_id=test.id,
                    status="passed" if result.passed else "failed",
                    duration_ms=int(result.duration * 1000),
                    passed=result.steps_passed,
                    failed=result.steps_failed,
                )

            print_test_result(result, verbose)

            if not result.passed and stop_on_fail:
                console.print("\n[red]Stopping on first failure[/red]")
                break

            progress.advance(task)

    # If cancelled, finalize the run with skipped tests
    if cancel_requested and current_run_id:
        skipped = finalize_cancelled_run(current_run_id)
        console.print(f"[yellow]Skipped {skipped} remaining test(s)[/yellow]")
        # Emit SSE event for cancellation
        from .sse import SSEEvent
        sse_manager.emit(
            SSEEvent(type="run_cancelled", data={"run_id": current_run_id, "skipped_count": skipped}),
            run_id=current_run_id,
        )

    return results


def run_docker_mode(
    tests: list,
    config: dict,
    suite: Path,
    workdir: Path,
    verbose: bool,
    stop_on_fail: bool,
    image_override: str | None = None,
    run_id: str | None = None,
    api_url: str | None = None,
) -> list[TestResult]:
    """
    Run tests in Docker containers with optional parallel execution.

    Uses WorkerPool for parallel test execution.
    max_workers is read from config.execution.max_workers (default: 1).
    """
    from .docker_executor import DockerExecutor, ContainerConfig, check_docker_available

    # Check Docker availability
    available, info = check_docker_available()
    if not available:
        console.print(f"[red]Docker not available: {info}[/red]")
        console.print("[yellow]Falling back to local mode...[/yellow]")
        from .discovery import TestDiscovery
        discovery = TestDiscovery(suite)
        routine_sets = discovery.discover_routines()
        return run_local_mode(tests, routine_sets, suite, workdir, verbose, stop_on_fail, run_id, api_url)

    console.print(f"[dim]Docker: {info}[/dim]")

    # Get framework path - relative to this file's location
    framework_path = Path(__file__).parent.parent

    # Configure container
    docker_config = config.get("docker", {})
    container_config = ContainerConfig(
        image=image_override or docker_config.get("base_image", "python:3.11-slim"),
        network=docker_config.get("network", "bridge"),
        mounts=docker_config.get("mounts", []),
    )

    # Get execution settings from config
    execution = config.get("execution", {})
    max_workers = execution.get("max_workers", 1)
    test_timeout = execution.get("timeout", 300)  # seconds per test

    results = []
    current_run_id = run_id or _current_run_id

    # Use host.docker.internal for Docker containers to reach host's API server
    api_server_url = "http://host.docker.internal:9999"
    console.print(f"[dim]API Server: {api_server_url}[/dim]")
    console.print(f"[dim]Mode: docker ({container_config.image})[/dim]")
    console.print(f"[dim]Workers: {max_workers}, Timeout: {test_timeout}s[/dim]\n")

    executor = DockerExecutor(
        server_url=api_server_url,
        framework_path=framework_path,
        suite_path=suite,
        base_workdir=workdir,
        config=container_config,
        run_id=run_id,
    )

    # API URL for status updates (use host URL, not container URL)
    host_api_url = api_url or "http://localhost:9999"

    # Helper function to execute a single test (for parallel execution)
    def execute_single_test(test):
        """
        Execute a single test and return result.

        Reports CRASHED status on unexpected container/process death.
        Timeout is reported as FAILED (not CRASHED).
        """
        test_start = datetime.now()
        crashed = False

        # Mark test as running via API (updates run counters)
        if current_run_id:
            update_test_status_via_api(
                host_api_url,
                current_run_id,
                test.id,
                status="running",
            )

        # Emit SSE test_started event
        if current_run_id:
            sse_manager.emit_test_started(current_run_id, test.id, test.name)

        # Execute test with timeout and crash detection
        try:
            docker_result = execute_with_timeout(
                lambda: executor.execute_test(test),
                test_timeout
            )
        except TestTimeoutError as e:
            # Timeout is FAILED, not CRASHED
            docker_result = {
                "test_id": test.id,
                "passed": False,
                "duration": test_timeout,
                "error": str(e),
                "stdout": "",
                "stderr": f"Test exceeded {test_timeout}s timeout",
            }
        except Exception as e:
            # Unexpected error = container/process crashed
            crashed = True
            docker_result = {
                "test_id": test.id,
                "passed": False,
                "duration": (datetime.now() - test_start).total_seconds(),
                "error": f"Container/process crashed: {e}",
                "stdout": "",
                "stderr": str(e),
            }

        # Convert to TestResult
        result = TestResult(
            test_id=docker_result["test_id"],
            test_name=test.name,
            passed=docker_result["passed"],
            duration=docker_result.get("duration", 0),
            steps_passed=0,
            steps_failed=0 if docker_result["passed"] else 1,
            assertions_passed=0,
            assertions_failed=0,
            error=docker_result.get("error"),
            step_results=[],
            assertion_results=[],
        )

        # Update test result via API (updates run counters)
        if current_run_id:
            if crashed:
                status = "crashed"
            elif result.passed:
                status = "passed"
            else:
                status = "failed"
            update_test_status_via_api(
                host_api_url,
                current_run_id,
                test.id,
                status=status,
                duration_ms=int(result.duration * 1000),
                error_message=result.error,
            )

        # Emit SSE test_completed event
        if current_run_id:
            sse_manager.emit_test_completed(
                run_id=current_run_id,
                test_id=test.id,
                status=status if current_run_id else ("passed" if result.passed else "failed"),
                duration_ms=int(result.duration * 1000),
                passed=result.steps_passed,
                failed=result.steps_failed,
            )

        return result, docker_result

    # Execute tests with worker pool
    stop_requested = False
    cancel_requested = False

    with Progress(
        SpinnerColumn(),
        TextColumn("[progress.description]{task.description}"),
        TimeElapsedColumn(),
        console=console,
    ) as progress:
        task = progress.add_task("Running tests...", total=len(tests))

        if max_workers == 1:
            # Sequential execution (same as before)
            for i, test in enumerate(tests):
                if stop_requested or cancel_requested:
                    break

                # Check for cancellation before starting each test
                if current_run_id and check_cancel_requested(host_api_url, current_run_id):
                    console.print("\n[yellow]Cancellation requested - skipping remaining tests[/yellow]")
                    cancel_requested = True
                    break

                progress.update(task, description=f"[{i+1}/{len(tests)}] {test.id}")

                result, docker_result = execute_single_test(test)
                results.append(result)

                print_docker_result(docker_result, verbose)

                if not result.passed and stop_on_fail:
                    console.print("\n[red]Stopping on first failure[/red]")
                    stop_requested = True

                progress.advance(task)
        else:
            # Parallel execution using WorkerPool
            # Interleave submission and collection - submit new tests only as workers become free
            from concurrent.futures import as_completed

            completed = 0
            submitted = 0
            test_iter = iter(tests)
            progress.update(task, description=f"[0/{len(tests)}] Running {max_workers} tests in parallel...")

            with WorkerPool(max_workers=max_workers) as pool:
                pending_futures = {}  # future -> test mapping

                # Submit initial batch (up to max_workers)
                for _ in range(max_workers):
                    try:
                        test = next(test_iter)
                        # Check for cancellation before submitting
                        if current_run_id and check_cancel_requested(host_api_url, current_run_id):
                            console.print("\n[yellow]Cancellation requested - not submitting more tests[/yellow]")
                            cancel_requested = True
                            break
                        future = pool.submit(execute_single_test, test)
                        pending_futures[future] = test
                        submitted += 1
                    except StopIteration:
                        break  # No more tests to submit

                # Process results and submit new tests as workers become free
                while pending_futures:
                    # Wait for any future to complete
                    done_futures = as_completed(pending_futures.keys())
                    for future in done_futures:
                        completed += 1
                        test = pending_futures.pop(future)

                        try:
                            result, docker_result = future.result()
                            results.append(result)
                            print_docker_result(docker_result, verbose)

                            if not result.passed and stop_on_fail:
                                console.print("\n[red]Failure detected (remaining tests will complete)[/red]")
                                stop_requested = True

                        except Exception as e:
                            console.print(f"[red]Test execution error: {e}[/red]")

                        progress.update(task, description=f"[{completed}/{len(tests)}] Running...")
                        progress.advance(task)

                        # Check for cancellation - if cancelled, don't submit new tests
                        if not cancel_requested and current_run_id and check_cancel_requested(host_api_url, current_run_id):
                            console.print("\n[yellow]Cancellation requested - waiting for running tests to complete[/yellow]")
                            cancel_requested = True

                        # Submit next test if not cancelled/stopped and there are more tests
                        if not cancel_requested and not stop_requested:
                            try:
                                next_test = next(test_iter)
                                future = pool.submit(execute_single_test, next_test)
                                pending_futures[future] = next_test
                                submitted += 1
                            except StopIteration:
                                pass  # No more tests

                        # Break inner loop to re-check pending_futures
                        break

    # If cancelled, finalize the run with skipped tests
    if cancel_requested and current_run_id:
        skipped = finalize_cancelled_run(current_run_id)
        console.print(f"[yellow]Skipped {skipped} remaining test(s)[/yellow]")
        # Emit SSE event for cancellation
        from .sse import SSEEvent
        sse_manager.emit(
            SSEEvent(type="run_cancelled", data={"run_id": current_run_id, "skipped_count": skipped}),
            run_id=current_run_id,
        )

    return results


def print_docker_result(result: dict, verbose: bool = False):
    """Print result from Docker execution."""
    status = "[green]PASSED[/green]" if result["passed"] else "[red]FAILED[/red]"
    console.print(f"\n[bold]{result['test_id']}[/bold] - {status} ({result['duration']:.1f}s)")

    if not result["passed"] or verbose:
        if result.get("error"):
            console.print(f"  [red]Error: {result['error']}[/red]")

        # Parse and show container output
        stdout = result.get("stdout", "")
        if stdout:
            for line in stdout.strip().split("\n"):
                if line.startswith("[") and "]" in line:
                    # Format step/assertion output
                    if "FAIL" in line or "Error" in line:
                        console.print(f"  [red]{line}[/red]")
                    elif "OK" in line or "PASS" in line:
                        console.print(f"  [green]{line}[/green]")
                    else:
                        console.print(f"  {line}")
                elif verbose:
                    console.print(f"  [dim]{line}[/dim]")

        if result.get("stderr") and verbose:
            console.print(f"  [dim]stderr: {result['stderr'][:500]}[/dim]")


# =============================================================================
# API Command
# =============================================================================

# Default home directory for tsuite
TSUITE_HOME = Path.home() / ".tsuite"


def get_pid_file() -> Path:
    """Get path to PID file."""
    return TSUITE_HOME / "server.pid"


def get_log_file() -> Path:
    """Get path to log file."""
    return TSUITE_HOME / "server.log"


def is_server_running() -> tuple[bool, int | None]:
    """Check if server is running. Returns (is_running, pid)."""
    pid_file = get_pid_file()
    if not pid_file.exists():
        return False, None

    try:
        pid = int(pid_file.read_text().strip())
        # Check if process exists
        import os
        os.kill(pid, 0)  # Signal 0 just checks if process exists
        return True, pid
    except (ValueError, ProcessLookupError, PermissionError):
        # PID file exists but process is dead - clean up
        pid_file.unlink(missing_ok=True)
        return False, None


@main.command("api")
@click.option("--port", default=9999, help="Port to run the server on (default: 9999)")
@click.option("--suites", help="Comma-separated list of suite paths to sync on startup")
@click.option("--detach", "-d", is_flag=True, help="Run in background (detached mode)")
def api_cmd(port: int, suites: str | None, detach: bool):
    """Start the API server with web dashboard.

    The server provides:
    - REST API for test management
    - Web dashboard at http://localhost:<port>
    - Real-time updates via SSE

    Examples:
        tsuite api                     Start on default port 9999
        tsuite api --port 8080         Start on custom port
        tsuite api --detach            Run in background
        tsuite api -d --port 8080      Background on custom port
        tsuite api --suites ./my-suite Sync suite on startup
    """
    # Ensure home directory exists
    TSUITE_HOME.mkdir(parents=True, exist_ok=True)

    # Check if already running
    running, existing_pid = is_server_running()
    if running:
        console.print(f"[yellow]Server already running (PID: {existing_pid})[/yellow]")
        console.print(f"[dim]Use 'tsuite stop' to stop it first[/dim]")
        sys.exit(1)

    if detach:
        # Start in background
        import os

        log_file = get_log_file()
        pid_file = get_pid_file()

        # Build command
        cmd = [sys.executable, "-m", "tsuite.server", "--port", str(port)]
        if suites:
            cmd.extend(["--suites", suites])

        # Open log file for output
        log_fd = open(log_file, "a")
        log_fd.write(f"\n{'='*60}\n")
        log_fd.write(f"Server started at {datetime.now().isoformat()}\n")
        log_fd.write(f"Port: {port}\n")
        log_fd.write(f"{'='*60}\n")
        log_fd.flush()

        # Start detached process
        process = subprocess.Popen(
            cmd,
            stdout=log_fd,
            stderr=log_fd,
            start_new_session=True,  # Detach from terminal
        )

        # Write PID file
        pid_file.write_text(str(process.pid))

        console.print(f"[bold green]API server started in background[/bold green]")
        console.print(f"[dim]PID: {process.pid}[/dim]")
        console.print(f"[dim]Dashboard: http://localhost:{port}[/dim]")
        console.print(f"[dim]Logs: {log_file}[/dim]")
        console.print(f"[dim]Use 'tsuite stop' to stop the server[/dim]")
        return

    # Foreground mode
    from .server import create_app
    from werkzeug.serving import make_server

    # Initialize database
    db.init_db()

    # Sync suites if provided
    if suites:
        for suite_path in suites.split(","):
            suite_dir = Path(suite_path.strip()).resolve()
            config_file = suite_dir / "config.yaml"
            if suite_dir.exists() and config_file.exists():
                config = load_config(config_file)
                if config:
                    discovery = TestDiscovery(suite_dir)
                    test_count = len(discovery.discover_tests())
                    sync_suite_to_db(suite_dir, config, test_count)
                    console.print(f"[dim]Synced suite: {suite_dir.name} ({test_count} tests)[/dim]")

    # Write PID file for foreground mode too (so stop command works)
    import os
    pid_file = get_pid_file()
    pid_file.write_text(str(os.getpid()))

    # Start server
    console.print(f"[bold green]Starting API server on http://localhost:{port}[/bold green]")
    console.print(f"[dim]Dashboard: http://localhost:{port}[/dim]")
    console.print(f"[dim]API: http://localhost:{port}/api[/dim]")
    console.print("[dim]Press Ctrl+C to stop[/dim]\n")

    app = create_app()
    server = make_server("0.0.0.0", port, app, threaded=True)
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        console.print("\n[dim]Shutting down...[/dim]")
        server.shutdown()
    finally:
        # Clean up PID file
        pid_file.unlink(missing_ok=True)


# =============================================================================
# Stop Command
# =============================================================================

@main.command("stop")
def stop_cmd():
    """Stop the running API server.

    Gracefully stops the server that was started with 'tsuite api'.

    Examples:
        tsuite stop                    Stop the running server
    """
    import signal

    running, pid = is_server_running()

    if not running:
        console.print("[yellow]No server running[/yellow]")
        return

    console.print(f"[dim]Stopping server (PID: {pid})...[/dim]")

    try:
        import os
        os.kill(pid, signal.SIGTERM)

        # Wait for process to terminate (up to 5 seconds)
        import time
        for _ in range(50):
            try:
                os.kill(pid, 0)
                time.sleep(0.1)
            except ProcessLookupError:
                break

        # Clean up PID file
        get_pid_file().unlink(missing_ok=True)
        console.print("[green]Server stopped[/green]")

    except ProcessLookupError:
        console.print("[green]Server already stopped[/green]")
        get_pid_file().unlink(missing_ok=True)
    except PermissionError:
        console.print(f"[red]Permission denied. Try: kill {pid}[/red]")


# =============================================================================
# Clear Command
# =============================================================================

@main.command("clear")
@click.option("--all", "clear_all", is_flag=True, help="Clear all test data")
@click.option("--force", "-f", is_flag=True, help="Skip confirmation prompt")
def clear_cmd(clear_all: bool, force: bool):
    """Clear test data (database, logs, reports).

    Examples:
        tsuite clear --all              Clear all test data
        tsuite clear --all --force      Clear without confirmation
    """
    import shutil

    tsuite_dir = Path.home() / ".tsuite"

    if not clear_all:
        console.print("[yellow]Use --all to clear all test data[/yellow]")
        console.print("  tsuite clear --all           Clear database, logs, and reports")
        console.print("  tsuite clear --all --force   Clear without confirmation")
        return

    if not tsuite_dir.exists():
        console.print("[yellow]Nothing to clear (~/.tsuite does not exist)[/yellow]")
        return

    if not force:
        click.confirm("Delete ALL test data (database, logs, reports)? This cannot be undone.", abort=True)

    cleared = []

    # Clear database files (including backups)
    for pattern in ["*.db", "*.db-*", "*.db.*"]:
        for f in tsuite_dir.glob(pattern):
            f.unlink()
            cleared.append(f.name)

    # Clear runs directory
    runs_dir = tsuite_dir / "runs"
    if runs_dir.exists():
        shutil.rmtree(runs_dir)
        cleared.append("runs/")

    # Clear reports directory
    reports_dir = tsuite_dir / "reports"
    if reports_dir.exists():
        shutil.rmtree(reports_dir)
        cleared.append("reports/")

    # Clear server log
    server_log = tsuite_dir / "server.log"
    if server_log.exists():
        server_log.unlink()
        cleared.append("server.log")

    if cleared:
        console.print(f"[green]Cleared: {', '.join(cleared)}[/green]")
    else:
        console.print("[yellow]Nothing to clear[/yellow]")


# =============================================================================
# Man Command
# =============================================================================

@main.command("man")
@click.argument("topic", required=False)
@click.option("--list", "list_topics", is_flag=True, help="List available topics")
def man_cmd(topic: str | None, list_topics: bool):
    """View documentation for tsuite.

    Examples:
        tsuite man --list           List all topics
        tsuite man quickstart       View quickstart guide
        tsuite man handlers         View handlers documentation
    """
    from .man import get_page, list_pages
    from .man.renderer import render_page, render_list, render_not_found

    if list_topics or topic is None:
        render_list(console)
        return

    page = get_page(topic)
    if page is None:
        render_not_found(topic, console)
        sys.exit(1)

    render_page(page, console)


# =============================================================================
# Scaffold Command
# =============================================================================

@main.command("scaffold")
@click.option("--suite", "suite_path", required=True, type=click.Path(exists=True),
              help="Path to test suite (must have config.yaml)")
@click.option("--uc", "uc_name", help="Use case name (e.g., uc01_tags). Interactive if not provided.")
@click.option("--tc", "tc_name", help="Test case name (e.g., tc01_test). Interactive if not provided.")
@click.option("--artifact-level", type=click.Choice(["tc", "uc"]), default=None,
              help="Where to copy artifacts: tc (default) or uc level")
@click.option("--name", "test_name", help="Test name (default: derived from tc)")
@click.option("--no-interactive", is_flag=True, help="Skip prompts, use defaults")
@click.option("--dry-run", is_flag=True, help="Preview without creating files")
@click.option("--force", is_flag=True, help="Overwrite existing TC")
@click.option("--skip-artifact-copy", is_flag=True, help="Skip copying artifacts, just generate test.yaml")
@click.argument("agent_dirs", nargs=-1, required=True, type=click.Path(exists=True))
def scaffold_cmd(
    suite_path: str,
    uc_name: str | None,
    tc_name: str | None,
    artifact_level: str | None,
    test_name: str | None,
    no_interactive: bool,
    dry_run: bool,
    force: bool,
    skip_artifact_copy: bool,
    agent_dirs: tuple,
):
    """Generate test case from agent directories.

    Copies agent source directories to suite artifacts and generates test.yaml
    with setup, agent startup, and placeholder test steps.

    Examples:
        tsuite scaffold --suite ./my-suite --uc uc01_tags --tc tc01_test ./agent1 ./agent2

        tsuite scaffold --suite ./my-suite --uc uc01_tags --tc tc01_test \\
            --artifact-level uc --dry-run ./agent1

        tsuite scaffold --suite ./my-suite --uc uc01_tags --tc tc01_test \\
            --force --no-interactive ./agent1 ./agent2

        # Interactive mode (prompts for UC, TC, artifact level):
        tsuite scaffold --suite ./my-suite ./agent1 ./agent2
    """
    from .scaffold import (
        ScaffoldConfig,
        ScaffoldError,
        validate_suite,
        validate_agent_dir,
        validate_no_parent_dirs,
        run_scaffold,
    )

    suite = Path(suite_path).resolve()

    try:
        # Validate suite
        validate_suite(suite)

        # Validate and detect agents
        agent_paths = [Path(d).resolve() for d in agent_dirs]
        validate_no_parent_dirs(agent_paths)

        agents = []
        for agent_path in agent_paths:
            agent_info = validate_agent_dir(agent_path)
            agents.append(agent_info)

        # Show detected agents
        console.print(f"\n[bold]Suite:[/bold] {suite}")
        console.print(f"[bold]Detected agents:[/bold]")
        for agent in agents:
            type_label = "TypeScript" if agent.agent_type == "typescript" else "Python"
            console.print(f"  - {agent.name} ({type_label})")
        console.print()

        # Interactive mode: prompt for UC if not provided
        if uc_name is None:
            if no_interactive:
                console.print("[red]Error: --uc is required in non-interactive mode[/red]")
                sys.exit(1)
            uc_name = _interactive_uc_selection(suite)

        # Interactive mode: prompt for TC if not provided
        if tc_name is None:
            if no_interactive:
                console.print("[red]Error: --tc is required in non-interactive mode[/red]")
                sys.exit(1)
            tc_name = _interactive_tc_prompt(suite, uc_name)

        # Interactive mode: prompt for artifact level if not provided
        if artifact_level is None:
            if no_interactive:
                artifact_level = "tc"  # Default
            else:
                artifact_level = _interactive_artifact_level_prompt(suite, uc_name, tc_name)

        # Confirmation prompt in interactive mode
        if not no_interactive and not dry_run:
            console.print(f"\n[bold]Summary:[/bold]")
            console.print(f"  Use Case:       {uc_name}")
            console.print(f"  Test Case:      {tc_name}")
            console.print(f"  Artifact Level: {artifact_level}")
            console.print(f"  Agents:         {', '.join(a.name for a in agents)}")
            if not click.confirm("\nProceed?", default=True):
                console.print("[yellow]Aborted.[/yellow]")
                sys.exit(0)

        # Create config
        config = ScaffoldConfig(
            suite_path=suite,
            uc_name=uc_name,
            tc_name=tc_name,
            agents=agents,
            artifact_level=artifact_level,
            test_name=test_name,
            dry_run=dry_run,
            force=force,
            skip_artifact_copy=skip_artifact_copy,
        )

        # Run scaffold
        run_scaffold(config)

    except ScaffoldError as e:
        console.print(f"[red]Error: {e}[/red]")
        sys.exit(1)


def _interactive_uc_selection(suite: Path) -> str:
    """Interactive UC selection/creation."""
    suites_dir = suite / "suites"

    # Find existing UCs
    existing_ucs = []
    if suites_dir.exists():
        existing_ucs = sorted([
            d.name for d in suites_dir.iterdir()
            if d.is_dir() and d.name.startswith("uc")
        ])

    if existing_ucs:
        console.print("[bold]Use Case:[/bold]")
        for i, uc in enumerate(existing_ucs, 1):
            console.print(f"  [{i}] {uc}")
        console.print(f"  [{len(existing_ucs) + 1}] Create new UC")

        while True:
            choice = click.prompt("Select", type=int, default=len(existing_ucs) + 1)
            if 1 <= choice <= len(existing_ucs):
                return existing_ucs[choice - 1]
            elif choice == len(existing_ucs) + 1:
                break
            else:
                console.print("[red]Invalid choice[/red]")

    # Create new UC
    while True:
        uc_name = click.prompt("New UC name (e.g., uc03_tags)")
        if uc_name.startswith("uc") and "_" in uc_name:
            return uc_name
        console.print("[red]UC name should follow pattern: uc##_description[/red]")


def _interactive_tc_prompt(suite: Path, uc_name: str) -> str:
    """Interactive TC name prompt."""
    uc_dir = suite / "suites" / uc_name

    # Find existing TCs
    existing_tcs = []
    if uc_dir.exists():
        existing_tcs = sorted([
            d.name for d in uc_dir.iterdir()
            if d.is_dir() and d.name.startswith("tc")
        ])

    if existing_tcs:
        console.print(f"\n[bold]Existing TCs in {uc_name}:[/bold]")
        for tc in existing_tcs:
            console.print(f"  - {tc}")

    while True:
        tc_name = click.prompt("\nTest Case name (e.g., tc01_test)")
        if tc_name.startswith("tc") and "_" in tc_name:
            if tc_name in existing_tcs:
                console.print(f"[yellow]Warning: {tc_name} already exists (use --force to overwrite)[/yellow]")
            return tc_name
        console.print("[red]TC name should follow pattern: tc##_description[/red]")


def _interactive_artifact_level_prompt(suite: Path, uc_name: str, tc_name: str) -> str:
    """Interactive artifact level prompt."""
    tc_path = f"suites/{uc_name}/{tc_name}/artifacts/"
    uc_path = f"suites/{uc_name}/artifacts/"

    console.print(f"\n[bold]Artifact level:[/bold]")
    console.print(f"  [1] TC level ({tc_path})")
    console.print(f"  [2] UC level ({uc_path})")

    while True:
        choice = click.prompt("Select", type=int, default=1)
        if choice == 1:
            return "tc"
        elif choice == 2:
            return "uc"
        console.print("[red]Invalid choice[/red]")


if __name__ == "__main__":
    main()
