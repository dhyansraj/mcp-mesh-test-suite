"""
Command-line interface for tsuite.

Usage:
    tsuite --all                    # Run all tests (local mode)
    tsuite --all --docker           # Run all tests in Docker containers
    tsuite --uc uc01_scaffolding    # Run all tests in use case
    tsuite --tc uc01_scaffolding/tc01_python_agent  # Run specific test
    tsuite --dry-run --all          # List tests without running
    tsuite --history                # Show recent runs
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

    def __enter__(self):
        from concurrent.futures import ThreadPoolExecutor
        self.executor = ThreadPoolExecutor(max_workers=self.max_workers)
        return self

    def __exit__(self, *args):
        if self.executor:
            self.executor.shutdown(wait=True)

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


def sync_suite_to_db(suite_path: Path, config: dict, test_count: int) -> None:
    """
    Sync the suite configuration to the database.
    Called when running tests from CLI to keep DB in sync with YAML.
    """
    import json

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
    repo.upsert_suite(
        folder_path=folder_path,
        suite_name=suite_name,
        mode=mode,
        config_json=json.dumps(config),
        test_count=test_count,
    )

    console.print(f"[dim]Suite synced: {suite_name}[/dim]")


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


@click.command()
@click.option("--all", "run_all", is_flag=True, help="Run all tests")
@click.option("--uc", multiple=True, help="Run tests in specific use case(s)")
@click.option("--tc", multiple=True, help="Run specific test case(s)")
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
def main(
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
    """Run integration tests."""
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
    sync_suite_to_db(suite, config, len(all_tests))

    # Filter tests
    if retry_failed and failed_test_ids:
        # For retry-failed, filter to only failed tests
        tests = [t for t in all_tests if t.id in failed_test_ids]
    else:
        if not run_all and not uc and not tc:
            console.print("[yellow]No tests selected. Use --all, --uc, or --tc[/yellow]")
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

    # Complete run record
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
    )

    with Progress(
        SpinnerColumn(),
        TextColumn("[progress.description]{task.description}"),
        TimeElapsedColumn(),
        console=console,
    ) as progress:
        task = progress.add_task("Running tests...", total=len(tests))

        for i, test in enumerate(tests):
            progress.update(task, description=f"[{i+1}/{len(tests)}] {test.id}")

            # Mark test as running in database (updates run counters)
            current_run_id = run_id or _current_run_id
            if current_run_id:
                repo.update_test_status(
                    current_run_id,
                    test.id,
                    status=TestStatus.RUNNING,
                )

            # Emit SSE test_started event
            if current_run_id:
                sse_manager.emit_test_started(current_run_id, test.id, test.name)

            result = executor.execute(test)
            results.append(result)

            # Update test result in database (updates run counters)
            if current_run_id:
                status = TestStatus.PASSED if result.passed else TestStatus.FAILED
                repo.update_test_status(
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

    # Helper function to execute a single test (for parallel execution)
    def execute_single_test(test):
        """
        Execute a single test and return result.

        Reports CRASHED status on unexpected container/process death.
        Timeout is reported as FAILED (not CRASHED).
        """
        test_start = datetime.now()
        crashed = False

        # Mark test as running in database (updates run counters)
        if current_run_id:
            repo.update_test_status(
                current_run_id,
                test.id,
                status=TestStatus.RUNNING,
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

        # Update test result in database (updates run counters)
        if current_run_id:
            if crashed:
                status = TestStatus.CRASHED
            elif result.passed:
                status = TestStatus.PASSED
            else:
                status = TestStatus.FAILED
            repo.update_test_status(
                current_run_id,
                test.id,
                status=status,
                duration_ms=int(result.duration * 1000),
                error_message=result.error,
            )

        # Emit SSE test_completed event
        if current_run_id:
            # Determine correct status for SSE emission
            if result.passed:
                sse_status = "passed"
            elif crashed:
                sse_status = "crashed"
            else:
                sse_status = "failed"

            sse_manager.emit_test_completed(
                run_id=current_run_id,
                test_id=test.id,
                status=sse_status,
                duration_ms=int(result.duration * 1000),
                passed=result.steps_passed,
                failed=result.steps_failed,
            )

        return result, docker_result

    # Execute tests with worker pool
    stop_requested = False

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
                if stop_requested:
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
            completed = 0
            progress.update(task, description=f"[0/{len(tests)}] Running {max_workers} tests in parallel...")

            with WorkerPool(max_workers=max_workers) as pool:
                # Submit all tests
                futures = []
                for test in tests:
                    future = pool.submit(execute_single_test, test)
                    futures.append((future, test))

                # Collect results as they complete
                from concurrent.futures import as_completed
                for future in as_completed([f for f, _ in futures]):
                    completed += 1
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


if __name__ == "__main__":
    main()
