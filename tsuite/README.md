# mcp-mesh-tsuite

YAML-driven integration test framework with container isolation, real-time monitoring, and a web dashboard.

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
pip install mcp-mesh-tsuite
```

## Quick Start

```bash
# View documentation
tsuite man quickstart

# Start the dashboard
tsuite api --port 9999
```

## Usage

### Running Tests

```bash
# Run all tests in Docker mode
tsuite run --all --docker

# Run specific use case
tsuite run --uc uc01_registry --docker

# Run specific test case
tsuite run --tc uc01_registry/tc01_agent_registration --docker

# Run tests matching tags
tsuite run --tag smoke --docker

# Dry run (list tests without running)
tsuite run --dry-run --all

# View recent runs
tsuite run --history

# Generate report for a previous run
tsuite run --report-run <run_id>

# Compare two runs
tsuite run --compare <run_id_1> <run_id_2>
```

### API Server & Dashboard

Start the API server with web dashboard:

```bash
# Start on default port (9999)
tsuite api

# Start on custom port
tsuite api --port 8080

# Start with suites pre-loaded
tsuite api --suites ./my-suite,./other-suite
```

### Clear Data

```bash
# Clear all test data
tsuite clear --all

# Clear specific run
tsuite clear --run-id <run_id>
```

### Documentation

```bash
# List available topics
tsuite man --list

# View specific topic
tsuite man quickstart
tsuite man handlers
tsuite man assertions
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
tsuite run --all  # Runs without --docker flag
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

## Output Capture

Capture command output for use in assertions:

```yaml
test:
  - name: "Call API"
    handler: shell
    command: "curl http://localhost:8080/api/data"
    capture: api_response    # Captures stdout to ${captured.api_response}

  - name: "Get version"
    handler: shell
    command: "meshctl version"
    capture: version_output

assertions:
  - expr: ${captured.api_response} contains 'success'
  - expr: ${captured.version_output} contains '0.8'
```

### Capture Behavior

- `capture: <name>` stores **stdout** of the step in `${captured.<name>}`
- Captured values persist for the entire test execution
- Can be used in subsequent steps and assertions
- Available in routines (propagates to parent context)

## Expression Language

### Variable Types

| Prefix | Description | Example |
|--------|-------------|---------|
| `last.*` | Last step result | `${last.exit_code}`, `${last.stdout}`, `${last.stderr}` |
| `captured.*` | Captured output | `${captured.api_response}` |
| `config.*` | Suite configuration | `${config.packages.sdk_version}` |
| `state.*` | Shared test state | `${state.agent_port}` |
| `params.*` | Routine parameters | `${params.version}` |
| `env:*` | Environment variable | `${env:HOME}` |

### Data Access Prefixes

| Prefix | Description | Example |
|--------|-------------|---------|
| `json:` | JSONPath on last.stdout | `${json:$.data.count}` |
| `jq:` | jq query on last.stdout | `${jq:.structuredContent.content}` |
| `jq:captured.*:` | jq query on captured var | `${jq:captured.response:.data.id}` |
| `jsonfile:` | JSONPath on file | `${jsonfile:/path/file.json:$.key}` |
| `file:` | File contents | `${file:/path/to/file}` |
| `fixture:` | Fixture file contents | `${fixture:expected/output.json}` |

### jq Queries

Use `jq:` prefix for powerful JSON querying, including nested JSON parsing:

```yaml
assertions:
  # Simple path query
  - expr: ${jq:.structuredContent.content} == 'Paris'

  # Nested JSON with fromjson (parse JSON string inside JSON)
  - expr: ${jq:.content[0].text | fromjson | .content} contains 'Paris'

  # Query captured variable
  - expr: ${jq:captured.api_response:.data.items[0].name} == 'test'

  # Boolean check
  - expr: ${jq:.isError} == 'false'

  # Numeric value
  - expr: ${jq:.usage.tokens} > 0
```

### Operators

| Operator | Description | Example |
|----------|-------------|---------|
| `==` | Equals | `${exit_code} == 0` |
| `!=` | Not equals | `${status} != 'error'` |
| `>`, `<`, `>=`, `<=` | Numeric comparison | `${json:$.count} >= 5` |
| `contains` | Substring match | `${stdout} contains 'success'` |
| `not contains` | Substring not present | `${stderr} not contains 'error'` |
| `iequal` / `ieq` | Case-insensitive equals (trims whitespace) | `${jq:.content} iequal 'paris'` |
| `icontains` | Case-insensitive contains | `${stdout} icontains 'SUCCESS'` |
| `startswith` | String starts with | `${jq:.content} startswith 'Paris'` |
| `endswith` | String ends with | `${stdout} endswith 'done'` |
| `matches` | Regex match | `${stderr} matches 'Error:.*timeout'` |
| `exists` | Value is not null | `${json:$.data} exists` |
| `not exists` | Value is null | `${json:$.error} not exists` |
| `is` | Type check | `${json:$.items} is array` |
| `length` | Length comparison | `${json:$.items} length > 0` |

### Type Checking with `is`

```yaml
assertions:
  - expr: ${json:$.name} is string
  - expr: ${json:$.count} is number
  - expr: ${json:$.items} is array
  - expr: ${json:$.config} is object
  - expr: ${json:$.enabled} is boolean
  - expr: ${json:$.optional} is null
```

### Complete Example

```yaml
test:
  - name: "Call LLM provider"
    handler: shell
    command: "meshctl call gemini_provider '{\"request\": {\"messages\": [{\"role\": \"user\", \"content\": \"What is 2+2?\"}]}}'"
    capture: llm_response

assertions:
  # Basic string check
  - expr: ${captured.llm_response} contains '4'
    message: "Response should contain 4"

  # jq with case-insensitive comparison
  - expr: ${jq:captured.llm_response:.structuredContent.content} icontains 'four'
    message: "Content should mention four"

  # Nested JSON parsing
  - expr: ${jq:captured.llm_response:.content[0].text | fromjson | .role} == 'assistant'
    message: "Role should be assistant"

  # Boolean check
  - expr: ${jq:captured.llm_response:.isError} == 'false'
    message: "Should not be an error"

  # Numeric check
  - expr: ${jq:captured.llm_response:.structuredContent._mesh_usage.prompt_tokens} > 0
    message: "Should have prompt tokens"

  # Model validation
  - expr: ${jq:captured.llm_response:.structuredContent._mesh_usage.model} startswith 'gemini'
    message: "Model should be gemini"
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

### shell

Execute shell commands with bash:

```yaml
- name: "Run command"
  handler: shell
  command: "meshctl list -t"
  workdir: /workspace           # Optional: working directory
  timeout: 120                  # Optional: timeout in seconds (default: 120)
  capture: output               # Optional: capture stdout

- name: "Multi-line command"
  handler: shell
  command: |
    echo "Step 1"
    meshctl start agent/main.py -d
    echo "Step 2"
```

### wait

Wait for time duration or HTTP endpoint:

```yaml
# Wait for seconds
- name: "Wait for agent startup"
  handler: wait
  seconds: 5

# Wait for HTTP endpoint
- name: "Wait for API ready"
  handler: wait
  type: http
  url: "http://localhost:8080/health"
  timeout: 30                   # Max wait time in seconds
  interval: 2                   # Polling interval in seconds
```

### file

File operations:

```yaml
# Check if file exists
- name: "Check config exists"
  handler: file
  operation: exists
  path: "/workspace/config.yaml"

# Read file contents
- name: "Read config"
  handler: file
  operation: read
  path: "/workspace/config.yaml"
  capture: config_content

# Write file
- name: "Create config"
  handler: file
  operation: write
  path: "/workspace/config.yaml"
  content: |
    name: test
    version: 1.0
```

### http

HTTP requests:

```yaml
# GET request
- name: "Health check"
  handler: http
  method: GET
  url: "http://localhost:8080/health"
  timeout: 10
  capture: health_response

# POST request with JSON body
- name: "Create resource"
  handler: http
  method: POST
  url: "http://localhost:8080/api/items"
  headers:
    Content-Type: "application/json"
    Authorization: "Bearer ${env:API_TOKEN}"
  body:
    name: "test-item"
    value: 42
  capture: create_response

# PUT request
- name: "Update resource"
  handler: http
  method: PUT
  url: "http://localhost:8080/api/items/1"
  body:
    name: "updated-item"
```

### pip-install

Install Python packages:

```yaml
# Install from requirements.txt
- name: "Install dependencies"
  handler: pip-install
  requirements: "/workspace/requirements.txt"

# Install specific packages
- name: "Install packages"
  handler: pip-install
  packages:
    - requests>=2.28.0
    - pyyaml
```

### npm-install

Install Node.js packages:

```yaml
# Install from package.json
- name: "Install dependencies"
  handler: npm-install
  workdir: /workspace/my-agent

# Install specific packages
- name: "Install packages"
  handler: npm-install
  packages:
    - typescript
    - "@mcpmesh/sdk@0.8.0"
```

### routine

Invoke reusable routines:

```yaml
# Call global routine with parameters
- routine: global.setup_for_python_agent
  params:
    meshctl_version: "0.8.0"
    sdk_version: "0.8.0"

# Call use-case level routine
- routine: uc01.start_registry
  params:
    port: 8000
```

### Handler Summary

| Handler | Description | Key Parameters |
|---------|-------------|----------------|
| `shell` | Execute bash commands | `command`, `workdir`, `timeout`, `capture` |
| `wait` | Wait for time/condition | `seconds`, `type`, `url`, `timeout`, `interval` |
| `file` | File operations | `operation`, `path`, `content` |
| `http` | HTTP requests | `method`, `url`, `headers`, `body`, `timeout` |
| `pip-install` | Install Python packages | `requirements`, `packages` |
| `npm-install` | Install Node.js packages | `workdir`, `packages` |
| `routine` | Invoke routines | `params` |

### Adding Custom Handlers

Create a new handler in the `handlers/` directory:

```python
# handlers/myhandler.py
from tsuite.context import StepResult

def execute(step: dict, context: dict) -> StepResult:
    """
    Execute custom handler logic.

    Args:
        step: Step configuration from test.yaml
        context: Execution context with config, state, captured, last, workdir

    Returns:
        StepResult with success, exit_code, stdout, stderr, error
    """
    my_param = step.get("my_param", "default")

    # Your implementation here

    return StepResult(
        success=True,
        exit_code=0,
        stdout="Handler output",
        stderr="",
        error=None
    )
```

Register in `container_runner.py`:

```python
elif handler_name == "myhandler":
    from handlers import myhandler
    return myhandler.execute(step, context)
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
