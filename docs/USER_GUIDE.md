# tsuite User Guide

A comprehensive guide to creating and running tests with the tsuite test framework.

## Table of Contents

1. [Overview](#overview)
2. [Creating a Suite](#creating-a-suite)
3. [Creating Use Cases](#creating-use-cases)
4. [Creating Test Cases](#creating-test-cases)
5. [Handlers](#handlers)
6. [Routines](#routines)
7. [Variables & Interpolation](#variables--interpolation)
8. [Assertions](#assertions)
9. [Artifacts](#artifacts)
10. [Examples](#examples)

---

## Overview

### What is tsuite?

tsuite is a YAML-based test framework designed for integration testing. It supports:
- Declarative test definitions in YAML
- Docker container execution for isolated, parallel testing
- Standalone mode for local debugging
- Reusable routines and shared artifacts
- Rich assertion expressions

### Hierarchy

Tests are organized in a three-level hierarchy:

```
suite/
├── config.yaml          # Suite configuration
├── global/
│   └── routines.yaml    # Global routines (optional)
└── suites/
    ├── uc01_feature_a/          # Use Case
    │   ├── artifacts/           # UC-level artifacts (optional)
    │   ├── routines.yaml        # UC-level routines (optional)
    │   ├── tc01_basic/          # Test Case
    │   │   ├── test.yaml        # Test definition
    │   │   └── artifacts/       # TC-level artifacts (optional)
    │   └── tc02_advanced/
    │       └── test.yaml
    └── uc02_feature_b/
        └── tc01_simple/
            └── test.yaml
```

- **Suite**: Top-level container with configuration
- **Use Case (UC)**: Groups related test cases (e.g., `uc01_registry`, `uc02_lifecycle`)
- **Test Case (TC)**: Individual test with steps and assertions

---

## Creating a Suite

A suite is defined by a `config.yaml` file at the root directory.

### Basic config.yaml

```yaml
suite:
  name: "My Integration Tests"
  mode: "docker"  # or "standalone"

packages:
  cli_version: "0.8.0-beta.9"
  sdk_python_version: "0.8.0b9"
  sdk_typescript_version: "0.8.0-beta.9"

docker:
  base_image: "tsuite-mesh:0.8.0-beta.9"

execution:
  max_workers: 4
  timeout: 300
```

### Configuration Options

| Field | Description | Default |
|-------|-------------|---------|
| `suite.name` | Human-readable suite name | Required |
| `suite.mode` | Execution mode: `docker` or `standalone` | `docker` |
| `packages.*` | Package versions for interpolation | - |
| `docker.base_image` | Docker image for test containers | Required for docker mode |
| `execution.max_workers` | Parallel workers (docker mode only) | `4` |
| `execution.timeout` | Default test timeout (seconds) | `300` |

### Modes

**Docker Mode** (`mode: docker`)
- Tests run in isolated containers
- Parallel execution with worker pool
- Clean environment for each test
- Requires Docker installed

**Standalone Mode** (`mode: standalone`)
- Tests run directly on host
- Sequential execution only
- Useful for debugging
- No Docker required

---

## Creating Use Cases

Use cases are directories under `suites/` that group related test cases.

### Directory Structure

```
suites/
└── uc01_registry/
    ├── artifacts/           # Shared artifacts (mounted as /uc-artifacts)
    ├── routines.yaml        # UC-scoped routines (optional)
    ├── tc01_start_registry/
    │   └── test.yaml
    └── tc02_agent_register/
        └── test.yaml
```

### Naming Convention

Use cases follow the pattern: `ucNN_descriptive_name`
- `uc01_registry` - Registry tests
- `uc02_lifecycle` - Agent lifecycle tests
- `uc03_capabilities` - Capability matching tests

### UC-Level Routines

Create `routines.yaml` in the UC directory for routines shared across TCs in that UC:

```yaml
routines:
  start_registry:
    description: "Start the mesh registry"
    steps:
      - handler: shell
        command: "mesh-registry &"
      - handler: wait
        seconds: 2
```

Reference in tests as `uc.start_registry`.

---

## Creating Test Cases

Each test case is a directory containing a `test.yaml` file.

### Basic test.yaml

```yaml
name: "Agent Start and Stop"
description: "Test starting and stopping a Python agent"
tags:
  - lifecycle
  - python
timeout: 120

test:
  - name: "Start agent"
    handler: shell
    command: "meshctl start agent/main.py -d"
    workdir: /workspace
    capture: start_output

  - name: "Check agent running"
    handler: shell
    command: "meshctl list"
    capture: list_output

assertions:
  - expr: "${captured.list_output} contains 'agent'"
    message: "Agent should be listed"
```

### Test Case Fields

| Field | Description | Required |
|-------|-------------|----------|
| `name` | Test name | Yes |
| `description` | Detailed description | No |
| `tags` | List of tags for filtering | No |
| `timeout` | Test timeout in seconds | No (uses suite default) |
| `pre_run` | Setup steps (run before test) | No |
| `test` | Main test steps | Yes |
| `assertions` | Validation expressions | No |
| `post_run` | Cleanup steps (always runs) | No |

### Test Phases

```yaml
pre_run:
  # Setup phase - runs first
  - routine: global.setup_environment

test:
  # Main test phase
  - name: "Execute test logic"
    handler: shell
    command: "run-test.sh"

assertions:
  # Evaluated after test phase
  - expr: ${last.exit_code} == 0
    message: "Test should pass"

post_run:
  # Cleanup phase - ALWAYS runs (even if test fails)
  - handler: shell
    command: "cleanup.sh"
    ignore_errors: true
```

### Step Fields

| Field | Description |
|-------|-------------|
| `name` | Step description |
| `handler` | Step type (shell, wait, file, etc.) |
| `command` | Command to execute (for shell handler) |
| `workdir` | Working directory |
| `capture` | Variable name to store stdout |
| `timeout` | Step timeout in seconds |
| `ignore_errors` | Continue on failure (default: false) |
| `env` | Environment variables (map) |

---

## Handlers

Handlers define what action a step performs.

### shell

Execute shell commands.

```yaml
- name: "Run command"
  handler: shell
  command: "meshctl list --json"
  workdir: /workspace
  capture: output
  timeout: 30
  env:
    DEBUG: "true"
```

Multi-line commands:

```yaml
- name: "Complex script"
  handler: shell
  command: |
    cd /workspace
    if [ -f config.json ]; then
      cat config.json | jq '.version'
    else
      echo "not found"
    fi
  capture: result
```

### wait

Wait for conditions.

**Wait for seconds:**
```yaml
- handler: wait
  seconds: 5
```

**Wait for HTTP endpoint:**
```yaml
- handler: wait
  type: http
  url: "http://localhost:3000/health"
  timeout: 30
  interval: 2
  expect_status: [200]
```

**Wait for file:**
```yaml
- handler: wait
  type: file
  path: /workspace/output.json
  timeout: 60
```

**Wait for command to succeed:**
```yaml
- handler: wait
  type: command
  command: "curl -s localhost:3000/health | grep -q ok"
  timeout: 30
  interval: 2
```

### file

File operations.

```yaml
# Check existence
- handler: file
  operation: exists
  path: /workspace/agent/main.py

# Read file
- handler: file
  operation: read
  path: /workspace/config.json
  capture: config_content

# Write file
- handler: file
  operation: write
  path: /workspace/test.txt
  content: "Hello, World!"

# Delete file
- handler: file
  operation: delete
  path: /workspace/temp.txt

# Create directory
- handler: file
  operation: mkdir
  path: /workspace/output
```

### npm-install

Install Node.js dependencies with automatic package overrides.

```yaml
- name: "Install dependencies"
  handler: npm-install
  path: /workspace/ts-agent
```

This handler:
1. Runs `npm install` in the specified path
2. Automatically overrides `@mcpmesh/*` packages based on suite's package mode

### pip-install

Install Python dependencies with automatic package overrides.

```yaml
- name: "Install dependencies"
  handler: pip-install
  path: /workspace/py-agent
  venv: /workspace/.venv  # optional, defaults to /workspace/.venv
```

This handler:
1. Creates/uses virtual environment
2. Runs `pip install -r requirements.txt`
3. Automatically overrides `mcp-mesh` packages based on suite's package mode

### http

Make HTTP requests.

```yaml
- name: "Call API"
  handler: http
  method: POST
  url: "http://localhost:3000/api/agents"
  headers:
    Content-Type: "application/json"
  json:
    name: "test-agent"
  capture: response
```

---

## Routines

Routines are reusable step sequences with parameters.

### Defining Routines

**Global routines** (`global/routines.yaml`):

```yaml
routines:
  setup_for_python_agent:
    description: "Setup Python environment for agent testing"
    params:
      meshctl_version:
        type: string
        required: true
      mcpmesh_version:
        type: string
        required: true
    steps:
      - handler: shell
        command: "python3 -m venv /workspace/.venv"

      - handler: shell
        command: |
          source /workspace/.venv/bin/activate
          pip install meshctl==${params.meshctl_version}
          pip install mcp-mesh==${params.mcpmesh_version}

  cleanup_workspace:
    description: "Clean up workspace directory"
    steps:
      - handler: shell
        command: "rm -rf /workspace/*"
        ignore_errors: true
```

**UC-level routines** (`suites/uc01_feature/routines.yaml`):

```yaml
routines:
  scaffold_agent:
    params:
      name:
        type: string
        required: true
      lang:
        type: string
        default: "python"
    steps:
      - handler: shell
        command: "meshctl scaffold --name ${params.name} --lang ${params.lang}"
        workdir: /workspace
```

### Using Routines

```yaml
pre_run:
  # Global routine
  - routine: global.setup_for_python_agent
    params:
      meshctl_version: "${config.packages.cli_version}"
      mcpmesh_version: "${config.packages.sdk_python_version}"

test:
  # UC-level routine
  - routine: uc.scaffold_agent
    params:
      name: "my-agent"
      lang: "python"

post_run:
  - routine: global.cleanup_workspace
```

### Routine Scope

| Prefix | Location | Availability |
|--------|----------|--------------|
| `global.` | `global/routines.yaml` | All tests in suite |
| `uc.` | `suites/<uc>/routines.yaml` | Tests in that UC only |

---

## Variables & Interpolation

Variables are resolved using `${expression}` syntax.

### Variable Sources

**Config values:**
```yaml
command: "pip install meshctl==${config.packages.cli_version}"
```

**Captured output:**
```yaml
- name: "Get version"
  handler: shell
  command: "meshctl --version"
  capture: version

- name: "Use version"
  handler: shell
  command: "echo Version is ${captured.version}"
```

**Routine parameters:**
```yaml
# In routine definition
command: "meshctl scaffold --name ${params.name}"
```

**Environment variables:**
```yaml
command: "echo Home is ${env:HOME}"
```

**Last step result:**
```yaml
assertions:
  - expr: ${last.exit_code} == 0
  - expr: "${last.stdout} contains 'success'"
```

### Special Prefixes

| Prefix | Description | Example |
|--------|-------------|---------|
| `captured.` | Captured step output | `${captured.list_output}` |
| `config.` | Suite configuration | `${config.packages.cli_version}` |
| `params.` | Routine parameters | `${params.name}` |
| `env:` | Environment variable | `${env:HOME}` |
| `file:` | File contents | `${file:/workspace/out.txt}` |
| `jq:` | JSON query on variable | `${jq:captured.json:.items[0].name}` |
| `last.` | Last step result | `${last.exit_code}` |

### JSON Queries

Use `jq:` prefix for JSON path queries:

```yaml
- name: "Get JSON"
  handler: shell
  command: "meshctl list --json"
  capture: list_json

assertions:
  # Query array length
  - expr: ${jq:captured.list_json:.agents | length} > 0
    message: "Should have agents"

  # Query nested field
  - expr: "${jq:captured.list_json:.agents[0].status}" == "running"
    message: "First agent should be running"
```

---

## Assertions

Assertions validate test results after the test phase completes.

### Syntax

```yaml
assertions:
  - expr: <expression>
    message: "Human-readable failure message"
```

### Operators

**String operators:**

```yaml
# Contains substring
- expr: "${captured.output} contains 'success'"

# Does not contain
- expr: "${captured.output} not contains 'error'"

# Equals (case-sensitive)
- expr: "${captured.status}" == "running"

# Equals (case-insensitive)
- expr: "${captured.status} iequal 'RUNNING'"

# Starts/ends with
- expr: "${captured.output} startswith 'OK'"
- expr: "${captured.output} endswith 'done'"

# Regex match
- expr: "${captured.version} matches '^[0-9]+\\.[0-9]+\\.[0-9]+$'"
```

**Comparison operators:**

```yaml
# Numeric comparison
- expr: ${last.exit_code} == 0
- expr: ${jq:captured.json:.count} > 5
- expr: ${jq:captured.json:.count} >= 1
- expr: ${jq:captured.json:.count} < 100
- expr: ${jq:captured.json:.count} != 0
```

**File operators:**

```yaml
# File exists
- expr: ${file:/workspace/agent/main.py} exists

# File does not exist
- expr: ${file:/workspace/temp.txt} not exists
```

### Examples

```yaml
assertions:
  # Command succeeded
  - expr: ${last.exit_code} == 0
    message: "Command should succeed"

  # Output contains expected text
  - expr: "${captured.list_output} contains 'my-agent'"
    message: "Agent should be listed"

  # JSON field check
  - expr: ${jq:captured.json:.agents | length} > 0
    message: "Should have at least one agent"

  # File was created
  - expr: ${file:/workspace/output.json} exists
    message: "Output file should be created"

  # Multiple conditions (each is separate assertion)
  - expr: "${captured.status} iequal 'healthy'"
    message: "Status should be healthy"

  - expr: "${captured.endpoint} not contains ':0'"
    message: "Port should not be 0"
```

---

## Artifacts

Artifacts are static files needed by tests, automatically mounted into containers.

### TC-Level Artifacts

Place files in `<tc>/artifacts/` directory:

```
tc01_basic_test/
├── test.yaml
└── artifacts/
    ├── agent-code/
    │   ├── main.py
    │   └── requirements.txt
    └── config.json
```

Mounted as `/artifacts` in the container:

```yaml
test:
  - name: "Copy agent code"
    handler: shell
    command: "cp -r /artifacts/agent-code /workspace/"
```

### UC-Level Artifacts

Place files in `<uc>/artifacts/` directory:

```
uc02_lifecycle/
├── artifacts/
│   └── shared-config.yaml
├── tc01_start/
│   └── test.yaml
└── tc02_stop/
    └── test.yaml
```

Mounted as `/uc-artifacts` in the container:

```yaml
test:
  - name: "Copy shared config"
    handler: shell
    command: "cp /uc-artifacts/shared-config.yaml /workspace/"
```

### Artifact Paths

| Location | Container Mount | Variable |
|----------|-----------------|----------|
| `<tc>/artifacts/` | `/artifacts` | `${artifacts_path}` |
| `<uc>/artifacts/` | `/uc-artifacts` | `${uc_artifacts_path}` |

---

## Examples

### Example 1: Simple Shell Test

```yaml
name: "Verify meshctl version"
description: "Check that meshctl is installed and reports correct version"
tags:
  - smoke
  - meshctl
timeout: 30

test:
  - name: "Check meshctl version"
    handler: shell
    command: "meshctl --version"
    capture: version_output

assertions:
  - expr: ${last.exit_code} == 0
    message: "meshctl should be installed"

  - expr: "${captured.version_output} contains '${config.packages.cli_version}'"
    message: "Version should match config"
```

### Example 2: Agent Lifecycle Test

```yaml
name: "Python Agent Lifecycle"
description: "Test starting, checking, and stopping a Python agent"
tags:
  - lifecycle
  - python
timeout: 120

pre_run:
  - routine: global.setup_for_python_agent
    params:
      meshctl_version: "${config.packages.cli_version}"
      mcpmesh_version: "${config.packages.sdk_python_version}"

test:
  - name: "Copy agent from artifacts"
    handler: shell
    command: "cp -r /artifacts/py-agent /workspace/"

  - name: "Install agent dependencies"
    handler: pip-install
    path: /workspace/py-agent

  - name: "Start agent"
    handler: shell
    command: "meshctl start py-agent/main.py -d"
    workdir: /workspace
    capture: start_output

  - name: "Wait for agent to register"
    handler: wait
    seconds: 5

  - name: "Check agent status"
    handler: shell
    command: "meshctl list --json"
    workdir: /workspace
    capture: list_json

  - name: "Stop agent"
    handler: shell
    command: "meshctl stop py-agent"
    workdir: /workspace
    capture: stop_output

assertions:
  - expr: "${captured.start_output} contains 'py-agent'"
    message: "Agent should start"

  - expr: ${jq:captured.list_json:.agents | length} > 0
    message: "Agent should be registered"

  - expr: "${captured.stop_output} contains 'stopped'"
    message: "Agent should stop"

post_run:
  - handler: shell
    command: "meshctl stop 2>/dev/null || true"
    ignore_errors: true
  - routine: global.cleanup_workspace
```

### Example 3: HTTP API Test

```yaml
name: "Agent HTTP Endpoint"
description: "Verify agent exposes HTTP endpoint correctly"
tags:
  - http
  - api
timeout: 60

test:
  - name: "Start agent with HTTP endpoint"
    handler: shell
    command: "meshctl start agent/main.py -d"
    workdir: /workspace

  - name: "Wait for HTTP endpoint"
    handler: wait
    type: http
    url: "http://localhost:3000/health"
    timeout: 30
    expect_status: [200]

  - name: "Call agent endpoint"
    handler: http
    method: GET
    url: "http://localhost:3000/api/status"
    capture: status_response

assertions:
  - expr: "${captured.status_response} contains 'healthy'"
    message: "Agent should report healthy status"
```

### Example 4: Using Routines

**global/routines.yaml:**
```yaml
routines:
  scaffold_and_start:
    params:
      name:
        type: string
        required: true
      lang:
        type: string
        default: "python"
    steps:
      - handler: shell
        command: "meshctl scaffold --name ${params.name} --lang ${params.lang}"
        workdir: /workspace

      - handler: shell
        command: "meshctl start ${params.name}/main.py -d"
        workdir: /workspace

      - handler: wait
        seconds: 5
```

**test.yaml:**
```yaml
name: "Multi-Agent Test"
description: "Test with multiple agents using routines"
tags:
  - multi-agent
timeout: 180

test:
  - routine: global.scaffold_and_start
    params:
      name: "agent-1"
      lang: "python"

  - routine: global.scaffold_and_start
    params:
      name: "agent-2"
      lang: "typescript"

  - name: "Verify both agents"
    handler: shell
    command: "meshctl list"
    capture: list_output

assertions:
  - expr: "${captured.list_output} contains 'agent-1'"
    message: "Agent 1 should be running"

  - expr: "${captured.list_output} contains 'agent-2'"
    message: "Agent 2 should be running"
```

---

## Running Tests

### CLI Commands

```bash
# Run all tests in suite
tsuite

# Run specific use case
tsuite --uc uc01_registry

# Run specific test case
tsuite --tc uc01_registry/tc01_start

# Run tests with specific tag
tsuite --tag smoke

# Run in standalone mode (override config)
tsuite --standalone

# Run in docker mode
tsuite --docker

# Dry run (list tests without executing)
tsuite --dry-run

# Verbose output
tsuite -v
```

### Using start.sh

```bash
# Run with CLI
./start.sh --cli --suite-path integration

# Run specific test
./start.sh --cli --tc uc01_registry/tc01_start

# Start dashboard
./start.sh --ui

# Start API server
./start.sh --api
```

---

## Tips & Best Practices

1. **Use descriptive names**: Test and step names should clearly describe what they do.

2. **Always capture output**: Use `capture:` for commands whose output you'll need in assertions.

3. **Use routines for repetition**: If you copy-paste steps, create a routine instead.

4. **Clean up in post_run**: Always clean up resources in `post_run` with `ignore_errors: true`.

5. **Use tags for filtering**: Tag tests by feature, type (smoke, regression), or component.

6. **Keep tests independent**: Each test should be able to run in isolation.

7. **Use artifacts for test data**: Store test files in `artifacts/` rather than generating them in steps.

8. **Prefer JSON output**: Use `--json` flags when available for easier assertion with `jq:`.

9. **Set appropriate timeouts**: Don't use default timeouts for tests that need more/less time.

10. **Document complex tests**: Use `description` field to explain non-obvious test logic.
