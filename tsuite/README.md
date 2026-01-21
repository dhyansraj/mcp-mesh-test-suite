# tsuite - Integration Test Framework

A YAML-driven integration test framework with container isolation, real-time monitoring, and a web dashboard.

## Features

- **YAML-based test definitions** - Tests as configuration, not code
- **Container isolation** - Each test runs in a fresh Docker container
- **Parallel execution** - Worker pool for concurrent test execution (Docker mode)
- **Pluggable handlers** - Extensible test actions (shell, file, http, wait, llm)
- **Expression language** - Flexible assertions without Python
- **Reusable routines** - Define once, use anywhere (global/UC/TC scopes)
- **REST API server** - Single API server for dashboard and container communication
- **Real-time SSE streaming** - Live test execution updates via Server-Sent Events
- **SQLite database** - Persistent storage for runs, results, and suite management
- **Web dashboard** - Monitor tests, view history, edit test cases
- **Idempotent updates** - Terminal states (passed/failed/crashed) are protected

## Installation

```bash
pip install -e .
```

## Usage

```bash
# Run all tests in Docker mode
tsuite --all --docker

# Run specific use case
tsuite --uc uc01_registry --docker

# Run specific test case
tsuite --tc uc01_registry/tc01_agent_registration --docker

# Run tests matching tags
tsuite --tag smoke --docker

# Dry run (list tests without running)
tsuite --dry-run --all

# View recent runs
tsuite --history

# Generate report for a previous run
tsuite --report-run <run_id>

# Compare two runs
tsuite --compare <run_id_1> <run_id_2>
```

### Standalone API Server

Run the API server for the dashboard without running tests:

```bash
python -m tsuite.server --port 9999
python -m tsuite.server --port 9999 --suites path/to/suite1,path/to/suite2
```

## Execution Modes

### Docker Mode (Recommended)

Tests run in isolated Docker containers with optional parallel execution:

```yaml
# config.yaml
defaults:
  parallel: 4    # Number of concurrent tests (default: 1)
  timeout: 300   # Per-test timeout in seconds (default: 300)
```

### Standalone Mode

Tests run locally in sequential order. Use for development or when Docker is unavailable:

```bash
tsuite --all  # Runs without --docker flag
```

## Architecture

```
tsuite/
├── cli.py           # Command-line interface
├── server.py        # REST API server with SSE
├── discovery.py     # Test discovery from YAML files
├── executor.py      # Test execution engine
├── context.py       # Runtime context management
├── expressions.py   # Expression evaluator for assertions
├── routines.py      # Routine resolver (global/UC/TC scopes)
├── client.py        # Container client library
├── db.py            # SQLite database layer
├── models.py        # Data models and enums
├── sse.py           # Server-Sent Events manager
├── repository.py    # Data access layer
└── reporter.py      # Report generation (HTML, JSON, JUnit)
```

## Database

SQLite database stored at `~/.tsuite/results.db`.

### Schema

| Table | Description |
|-------|-------------|
| `runs` | Test run sessions with status, timestamps, and aggregate counts |
| `test_results` | Individual test case results with status, duration, errors |
| `step_results` | Step-level results with stdout/stderr and exit codes |
| `assertion_results` | Assertion outcomes with actual values |
| `captured_values` | Values captured during test execution |
| `suites` | Registered test suites with config and metadata |

### Key Fields

**runs**: `run_id`, `suite_id`, `status`, `started_at`, `finished_at`, `passed`, `failed`, `skipped`, `mode`

**test_results**: `run_id`, `test_id`, `use_case`, `test_case`, `status`, `duration_ms`, `error_message`, `steps_json`

**suites**: `folder_path`, `suite_name`, `mode`, `config_json`, `test_count`

## REST API

### Health & Config
| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/health` | Health check |
| GET | `/config` | Get full configuration |
| GET | `/config/<path>` | Get config value by dot-notation path |

### Suite Management
| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/suites` | List all registered suites |
| POST | `/api/suites` | Register new suite by folder path |
| GET | `/api/suites/<id>` | Get suite details with test list |
| PUT | `/api/suites/<id>` | Update suite settings |
| DELETE | `/api/suites/<id>` | Remove suite |
| POST | `/api/suites/<id>/sync` | Re-sync from config.yaml |
| GET | `/api/suites/<id>/tests` | List tests (supports uc/tag filters) |
| POST | `/api/suites/<id>/run` | Start test run |

### Run Management
| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/runs` | List runs (paginated, filterable) |
| GET | `/api/runs/latest` | Get most recent run |
| POST | `/api/runs` | Create new run with filters |
| GET | `/api/runs/<id>` | Get run details with summary |
| POST | `/api/runs/<id>/start` | Start a pending run |
| POST | `/api/runs/<id>/complete` | Mark run as completed |
| GET | `/api/runs/<id>/tests` | Get all test results |
| GET | `/api/runs/<id>/tests/tree` | Get results grouped by use case |
| GET | `/api/runs/<id>/tests/<test_id>` | Get detailed test result |
| PATCH | `/api/runs/<id>/tests/<test_id>` | Update test status |

### Test Case Editor
| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/suites/<id>/tests/<test_id>/yaml` | Get test YAML for editing |
| PUT | `/api/suites/<id>/tests/<test_id>/yaml` | Update test YAML |
| PUT | `/api/suites/<id>/tests/<test_id>/steps/<phase>/<index>` | Update single step |
| POST | `/api/suites/<id>/tests/<test_id>/steps/<phase>` | Add new step |
| DELETE | `/api/suites/<id>/tests/<test_id>/steps/<phase>/<index>` | Delete step |

### Analytics
| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/stats` | Aggregate statistics |
| GET | `/api/stats/flaky` | Flaky tests (mixed results) |
| GET | `/api/stats/slowest` | Slowest tests by duration |
| GET | `/api/compare/<id1>/<id2>` | Compare two runs |

### Server-Sent Events (SSE)
| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/runs/<id>/stream` | SSE stream for specific run |
| GET | `/api/events` | Global SSE stream (supports run_id filter) |

### Container Communication
| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/state/<test_id>` | Get test state |
| POST | `/state/<test_id>` | Update test state |
| POST | `/capture/<test_id>` | Store captured variable |
| POST | `/progress/<test_id>` | Report progress |
| POST | `/log/<test_id>` | Log message from container |

## Data Models

### Enums

```python
class RunStatus(Enum):
    PENDING, RUNNING, COMPLETED, FAILED, CANCELLED

class TestStatus(Enum):
    PENDING, RUNNING, PASSED, FAILED, CRASHED, SKIPPED

class SuiteMode(Enum):
    DOCKER, STANDALONE
```

### SSE Events

| Event Type | Payload | Description |
|------------|---------|-------------|
| `run_started` | `run_id`, `total_tests` | Run began |
| `test_started` | `run_id`, `test_id`, `name` | Test began |
| `test_completed` | `run_id`, `test_id`, `status`, `duration_ms`, `steps_passed`, `steps_failed` | Test finished |
| `step_completed` | `run_id`, `test_id`, `step_index`, `phase`, `status`, `duration_ms`, `handler` | Step finished |
| `run_completed` | `run_id`, `passed`, `failed`, `skipped`, `duration_ms` | Run finished |

## Expression Language

```yaml
assertions:
  # Exact match
  - expr: ${exit_code} == 0

  # String contains
  - expr: ${stdout} contains 'success'

  # Captured variable
  - expr: ${captured.my_var} contains 'expected'

  # File exists
  - expr: ${file:/path/to/file} exists

  # JSONPath
  - expr: ${json:$.data.count} >= 5

  # Regex match
  - expr: ${stderr} matches 'Error:.*'
```

## Routines

Reusable step sequences defined at different scopes:

```yaml
# global/routines.yaml - Available everywhere
# uc01_registry/routines.yaml - Available in use case
# tc01_test/routines.yaml - Available in test case

routines:
  setup_environment:
    params:
      version: { type: string, required: true }
    steps:
      - handler: shell
        command: "pip install package==${params.version}"
```

Usage:
```yaml
pre_run:
  - routine: global.setup_environment
    params:
      version: "1.0.0"
```

## Handlers

| Handler | Description |
|---------|-------------|
| `shell` | Execute shell commands |
| `file` | File operations (create, copy, delete) |
| `http` | HTTP requests |
| `wait` | Wait for duration or condition |
| `routine` | Invoke a routine |
| `llm` | LLM-based operations |

### Adding Custom Handlers

```python
# handlers/myhandler.py
from tsuite.context import StepResult

def execute(step: dict, context: dict) -> StepResult:
    # Implement your handler
    return StepResult(success=True, exit_code=0)
```

## CLI Options

```
tsuite [OPTIONS]

Run Selection:
  --all                 Run all tests
  --uc TEXT             Run use case(s) [multiple]
  --tc TEXT             Run test case(s) [multiple]
  --tag TEXT            Filter by tag(s) [multiple]
  --skip-tag TEXT       Skip tests with tag(s) [multiple]
  --pattern TEXT        Filter by glob pattern

Execution:
  --docker              Run in Docker containers
  --image TEXT          Override Docker image
  --api-url TEXT        API server URL [default: http://localhost:9999]
  --stop-on-fail        Stop on first failure
  --retry-failed        Retry failed tests from last run
  --mock-llm            Use mock LLM responses

Output:
  --dry-run             List tests without running
  --verbose, -v         Verbose output
  --history             Show recent runs

Reporting:
  --report              Generate reports after run
  --report-dir PATH     Report output directory
  --report-format TEXT  Report format: html, json, junit [multiple]
  --report-run TEXT     Generate report for run ID
  --compare ID1 ID2     Compare two runs
```

## License

MIT
