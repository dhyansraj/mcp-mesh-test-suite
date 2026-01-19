"""
Report generation from SQLite database.

Supports multiple output formats:
- HTML: Visual report with expandable details
- JSON: Machine-readable for CI integration
- JUnit XML: For tooling compatibility (Jenkins, etc.)
"""

import json
import xml.etree.ElementTree as ET
from datetime import datetime
from pathlib import Path
from typing import Optional, List
from xml.dom import minidom

from . import repository as repo
from .models import Run, TestResult, TestStatus, RunStatus


def generate_report(
    run_id: str,
    output_dir: Path,
    formats: List[str] = None,
    filename_prefix: Optional[str] = None,
) -> dict:
    """
    Generate reports for a run in specified formats.

    Args:
        run_id: The run ID to generate reports for
        output_dir: Directory to write reports to
        formats: List of formats to generate (html, json, junit)
        filename_prefix: Optional prefix for filenames (default: run_id[:8])

    Returns:
        Dict mapping format to output file path
    """
    formats = formats or ["html", "json", "junit"]
    output_dir = Path(output_dir)
    output_dir.mkdir(parents=True, exist_ok=True)

    # Get run data
    run = repo.get_run(run_id)
    if not run:
        raise ValueError(f"Run not found: {run_id}")

    tests = repo.list_test_results(run_id)

    prefix = filename_prefix or run_id[:8]
    timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")

    outputs = {}

    if "html" in formats:
        path = output_dir / f"{prefix}_{timestamp}.html"
        generate_html_report(run, tests, path)
        outputs["html"] = str(path)

    if "json" in formats:
        path = output_dir / f"{prefix}_{timestamp}.json"
        generate_json_report(run, tests, path)
        outputs["json"] = str(path)

    if "junit" in formats:
        path = output_dir / f"{prefix}_{timestamp}.xml"
        generate_junit_report(run, tests, path)
        outputs["junit"] = str(path)

    return outputs


def generate_html_report(run: Run, tests: List[TestResult], output_path: Path) -> None:
    """Generate an HTML report."""
    # Calculate stats
    passed = sum(1 for t in tests if t.status == TestStatus.PASSED)
    failed = sum(1 for t in tests if t.status == TestStatus.FAILED)
    skipped = sum(1 for t in tests if t.status == TestStatus.SKIPPED)
    total = len(tests)
    pass_rate = (passed / total * 100) if total > 0 else 0
    duration_sec = (run.duration_ms / 1000) if run.duration_ms else 0

    # Status colors
    status_colors = {
        TestStatus.PASSED: "#22c55e",
        TestStatus.FAILED: "#ef4444",
        TestStatus.SKIPPED: "#6b7280",
        TestStatus.RUNNING: "#eab308",
        TestStatus.PENDING: "#9ca3af",
    }

    # Generate test rows
    test_rows = []
    for t in tests:
        color = status_colors.get(t.status, "#6b7280")
        duration = f"{t.duration_ms / 1000:.2f}s" if t.duration_ms else "-"
        error_row = ""
        if t.error_message:
            escaped_error = _escape_html(t.error_message)
            error_row = f'<tr class="error-row"><td colspan="4"><div class="error">{escaped_error}</div></td></tr>'

        test_rows.append(f"""
        <tr class="test-row" data-status="{t.status.value}">
            <td class="test-id">{_escape_html(t.test_id)}</td>
            <td class="test-name">{_escape_html(t.name or "")}</td>
            <td class="test-status" style="color: {color}; font-weight: bold;">
                {t.status.value.upper()}
            </td>
            <td class="test-duration">{duration}</td>
        </tr>
        {error_row}
        """)

    # Build skipped button separately to avoid backslash issues in f-string
    skipped_btn = ""
    if skipped > 0:
        skipped_btn = f'<button class="filter-btn" onclick="filterTests(\'skipped\')">Skipped ({skipped})</button>'

    html = f"""<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Test Report - {run.run_id[:8]}</title>
    <style>
        * {{ box-sizing: border-box; margin: 0; padding: 0; }}
        body {{
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: #f5f5f5;
            color: #1f2937;
            line-height: 1.5;
            padding: 2rem;
        }}
        .container {{ max-width: 1200px; margin: 0 auto; }}
        h1 {{ font-size: 1.5rem; margin-bottom: 1rem; }}
        .summary {{
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
            gap: 1rem;
            margin-bottom: 2rem;
        }}
        .stat {{
            background: white;
            padding: 1rem;
            border-radius: 8px;
            box-shadow: 0 1px 3px rgba(0,0,0,0.1);
            text-align: center;
        }}
        .stat-value {{
            font-size: 2rem;
            font-weight: bold;
        }}
        .stat-label {{ color: #6b7280; font-size: 0.875rem; }}
        .passed {{ color: #22c55e; }}
        .failed {{ color: #ef4444; }}
        .meta {{
            background: white;
            padding: 1rem;
            border-radius: 8px;
            margin-bottom: 2rem;
            font-size: 0.875rem;
            color: #6b7280;
        }}
        .meta strong {{ color: #1f2937; }}
        table {{
            width: 100%;
            background: white;
            border-collapse: collapse;
            border-radius: 8px;
            overflow: hidden;
            box-shadow: 0 1px 3px rgba(0,0,0,0.1);
        }}
        th, td {{ padding: 0.75rem 1rem; text-align: left; }}
        th {{
            background: #f9fafb;
            font-weight: 600;
            border-bottom: 1px solid #e5e7eb;
        }}
        tr:hover {{ background: #f9fafb; }}
        .test-id {{ font-family: monospace; font-size: 0.875rem; }}
        .test-duration {{ text-align: right; color: #6b7280; }}
        .error {{
            background: #fef2f2;
            color: #b91c1c;
            padding: 0.5rem;
            font-family: monospace;
            font-size: 0.75rem;
            white-space: pre-wrap;
            border-radius: 4px;
        }}
        .error-row td {{ padding: 0 1rem 0.75rem; }}
        .filters {{
            margin-bottom: 1rem;
            display: flex;
            gap: 0.5rem;
        }}
        .filter-btn {{
            padding: 0.5rem 1rem;
            border: 1px solid #e5e7eb;
            background: white;
            border-radius: 4px;
            cursor: pointer;
            font-size: 0.875rem;
        }}
        .filter-btn:hover {{ background: #f9fafb; }}
        .filter-btn.active {{ background: #1f2937; color: white; border-color: #1f2937; }}
    </style>
</head>
<body>
    <div class="container">
        <h1>Test Report</h1>

        <div class="summary">
            <div class="stat">
                <div class="stat-value">{total}</div>
                <div class="stat-label">Total Tests</div>
            </div>
            <div class="stat">
                <div class="stat-value passed">{passed}</div>
                <div class="stat-label">Passed</div>
            </div>
            <div class="stat">
                <div class="stat-value failed">{failed}</div>
                <div class="stat-label">Failed</div>
            </div>
            <div class="stat">
                <div class="stat-value">{pass_rate:.1f}%</div>
                <div class="stat-label">Pass Rate</div>
            </div>
            <div class="stat">
                <div class="stat-value">{duration_sec:.1f}s</div>
                <div class="stat-label">Duration</div>
            </div>
        </div>

        <div class="meta">
            <strong>Run ID:</strong> {run.run_id}<br>
            <strong>Started:</strong> {run.started_at.strftime("%Y-%m-%d %H:%M:%S") if run.started_at else "-"}<br>
            <strong>Status:</strong> {run.status.value}<br>
            <strong>CLI Version:</strong> {run.cli_version or "-"}<br>
            <strong>Docker Image:</strong> {run.docker_image or "-"}
        </div>

        <div class="filters">
            <button class="filter-btn active" onclick="filterTests('all')">All ({total})</button>
            <button class="filter-btn" onclick="filterTests('passed')">Passed ({passed})</button>
            <button class="filter-btn" onclick="filterTests('failed')">Failed ({failed})</button>
            {skipped_btn}
        </div>

        <table>
            <thead>
                <tr>
                    <th>Test ID</th>
                    <th>Name</th>
                    <th>Status</th>
                    <th style="text-align: right;">Duration</th>
                </tr>
            </thead>
            <tbody id="test-tbody">
                {''.join(test_rows)}
            </tbody>
        </table>
    </div>

    <script>
        function filterTests(status) {{
            const rows = document.querySelectorAll('.test-row');
            const errorRows = document.querySelectorAll('.error-row');
            const buttons = document.querySelectorAll('.filter-btn');

            buttons.forEach(btn => btn.classList.remove('active'));
            event.target.classList.add('active');

            rows.forEach((row, i) => {{
                const testStatus = row.getAttribute('data-status');
                const show = status === 'all' || testStatus === status;
                row.style.display = show ? '' : 'none';
                if (errorRows[i]) {{
                    errorRows[i].style.display = show ? '' : 'none';
                }}
            }});
        }}
    </script>
</body>
</html>
"""

    output_path.write_text(html)


def generate_json_report(run: Run, tests: List[TestResult], output_path: Path) -> None:
    """Generate a JSON report."""
    report = {
        "generated_at": datetime.now().isoformat(),
        "run": run.to_dict(),
        "tests": [t.to_dict() for t in tests],
        "summary": {
            "total": len(tests),
            "passed": sum(1 for t in tests if t.status == TestStatus.PASSED),
            "failed": sum(1 for t in tests if t.status == TestStatus.FAILED),
            "skipped": sum(1 for t in tests if t.status == TestStatus.SKIPPED),
            "pass_rate": (
                sum(1 for t in tests if t.status == TestStatus.PASSED) / len(tests) * 100
                if tests else 0
            ),
        },
    }

    output_path.write_text(json.dumps(report, indent=2, default=str))


def generate_junit_report(run: Run, tests: List[TestResult], output_path: Path) -> None:
    """Generate a JUnit XML report for CI tooling compatibility."""
    # Create root element
    testsuites = ET.Element("testsuites")
    testsuites.set("name", f"tsuite-{run.run_id[:8]}")
    testsuites.set("tests", str(len(tests)))
    testsuites.set("failures", str(sum(1 for t in tests if t.status == TestStatus.FAILED)))
    testsuites.set("errors", "0")
    testsuites.set("time", str((run.duration_ms or 0) / 1000))

    # Group tests by use case
    by_uc = {}
    for t in tests:
        uc = t.use_case or "default"
        if uc not in by_uc:
            by_uc[uc] = []
        by_uc[uc].append(t)

    # Create a testsuite for each use case
    for uc, uc_tests in by_uc.items():
        testsuite = ET.SubElement(testsuites, "testsuite")
        testsuite.set("name", uc)
        testsuite.set("tests", str(len(uc_tests)))
        testsuite.set("failures", str(sum(1 for t in uc_tests if t.status == TestStatus.FAILED)))
        testsuite.set("errors", "0")
        testsuite.set("skipped", str(sum(1 for t in uc_tests if t.status == TestStatus.SKIPPED)))
        testsuite.set("time", str(sum((t.duration_ms or 0) for t in uc_tests) / 1000))

        for t in uc_tests:
            testcase = ET.SubElement(testsuite, "testcase")
            testcase.set("name", t.test_case or t.test_id)
            testcase.set("classname", t.use_case or "tsuite")
            testcase.set("time", str((t.duration_ms or 0) / 1000))

            if t.status == TestStatus.FAILED:
                failure = ET.SubElement(testcase, "failure")
                failure.set("message", t.error_message or "Test failed")
                failure.text = t.error_message or ""

            elif t.status == TestStatus.SKIPPED:
                skipped = ET.SubElement(testcase, "skipped")
                skipped.set("message", "Test skipped")

    # Pretty print
    xml_str = minidom.parseString(ET.tostring(testsuites, encoding="unicode")).toprettyxml(indent="  ")
    # Remove extra blank lines
    xml_str = "\n".join(line for line in xml_str.split("\n") if line.strip())

    output_path.write_text(xml_str)


def generate_comparison_report(
    run_id_1: str,
    run_id_2: str,
    output_dir: Path,
    format: str = "html",
) -> str:
    """Generate a comparison report between two runs."""
    output_dir = Path(output_dir)
    output_dir.mkdir(parents=True, exist_ok=True)

    run1 = repo.get_run(run_id_1)
    run2 = repo.get_run(run_id_2)

    if not run1 or not run2:
        raise ValueError("One or both runs not found")

    comparison = repo.compare_runs(run_id_1, run_id_2)

    timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
    output_path = output_dir / f"compare_{run_id_1[:8]}_{run_id_2[:8]}_{timestamp}.{format}"

    if format == "html":
        _generate_comparison_html(run1, run2, comparison, output_path)
    elif format == "json":
        _generate_comparison_json(run1, run2, comparison, output_path)

    return str(output_path)


def _generate_comparison_html(
    run1: Run, run2: Run, comparison: list, output_path: Path
) -> None:
    """Generate HTML comparison report."""
    regressions = [c for c in comparison if c.get("change_type") == "regression"]
    fixed = [c for c in comparison if c.get("change_type") == "fixed"]
    same = [c for c in comparison if c.get("change_type") == "same"]

    def status_badge(status):
        colors = {
            "passed": "#22c55e",
            "failed": "#ef4444",
            "skipped": "#6b7280",
        }
        return f'<span style="color: {colors.get(status, "#6b7280")}; font-weight: bold;">{status.upper()}</span>'

    rows = []
    for c in comparison:
        change = c.get("change_type", "same")
        bg_color = {
            "regression": "#fef2f2",
            "fixed": "#f0fdf4",
            "same": "white",
        }.get(change, "white")

        rows.append(f"""
        <tr style="background: {bg_color};">
            <td class="test-id">{c.get('test_id', '')}</td>
            <td>{status_badge(c.get('run1_status', ''))}</td>
            <td>{status_badge(c.get('run2_status', ''))}</td>
            <td>{change.upper()}</td>
        </tr>
        """)

    html = f"""<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>Run Comparison</title>
    <style>
        body {{ font-family: -apple-system, sans-serif; padding: 2rem; background: #f5f5f5; }}
        .container {{ max-width: 1000px; margin: 0 auto; }}
        h1 {{ font-size: 1.5rem; margin-bottom: 1rem; }}
        .summary {{ display: flex; gap: 1rem; margin-bottom: 2rem; }}
        .stat {{ background: white; padding: 1rem; border-radius: 8px; text-align: center; flex: 1; }}
        .stat-value {{ font-size: 1.5rem; font-weight: bold; }}
        .regression {{ color: #ef4444; }}
        .fixed {{ color: #22c55e; }}
        table {{ width: 100%; background: white; border-collapse: collapse; border-radius: 8px; overflow: hidden; }}
        th, td {{ padding: 0.75rem 1rem; text-align: left; border-bottom: 1px solid #e5e7eb; }}
        th {{ background: #f9fafb; }}
        .test-id {{ font-family: monospace; font-size: 0.875rem; }}
    </style>
</head>
<body>
    <div class="container">
        <h1>Run Comparison</h1>
        <p style="color: #6b7280; margin-bottom: 1rem;">
            Comparing <strong>{run1.run_id[:8]}</strong> vs <strong>{run2.run_id[:8]}</strong>
        </p>

        <div class="summary">
            <div class="stat">
                <div class="stat-value">{len(comparison)}</div>
                <div>Total Tests</div>
            </div>
            <div class="stat">
                <div class="stat-value regression">{len(regressions)}</div>
                <div>Regressions</div>
            </div>
            <div class="stat">
                <div class="stat-value fixed">{len(fixed)}</div>
                <div>Fixed</div>
            </div>
            <div class="stat">
                <div class="stat-value">{len(same)}</div>
                <div>Unchanged</div>
            </div>
        </div>

        <table>
            <thead>
                <tr>
                    <th>Test ID</th>
                    <th>Run 1</th>
                    <th>Run 2</th>
                    <th>Change</th>
                </tr>
            </thead>
            <tbody>
                {''.join(rows)}
            </tbody>
        </table>
    </div>
</body>
</html>
"""
    output_path.write_text(html)


def _generate_comparison_json(
    run1: Run, run2: Run, comparison: list, output_path: Path
) -> None:
    """Generate JSON comparison report."""
    report = {
        "generated_at": datetime.now().isoformat(),
        "run_1": run1.to_dict(),
        "run_2": run2.to_dict(),
        "comparison": comparison,
        "summary": {
            "total": len(comparison),
            "regressions": sum(1 for c in comparison if c.get("change_type") == "regression"),
            "fixed": sum(1 for c in comparison if c.get("change_type") == "fixed"),
            "same": sum(1 for c in comparison if c.get("change_type") == "same"),
        },
    }
    output_path.write_text(json.dumps(report, indent=2, default=str))


def _escape_html(text: str) -> str:
    """Escape HTML special characters."""
    if not text:
        return ""
    return (
        text.replace("&", "&amp;")
        .replace("<", "&lt;")
        .replace(">", "&gt;")
        .replace('"', "&quot;")
        .replace("'", "&#39;")
    )
