# mcp-mesh-tsuite

YAML-driven integration test framework with container isolation and real-time monitoring.

## Installation

```bash
pip install mcp-mesh-tsuite
```

## Quick Start

```bash
# View the quickstart guide
tsuite man quickstart

# Start the dashboard (includes API server)
tsuite api --port 9999

# Open http://localhost:9999 in your browser
```

## Commands

### Run Tests

```bash
# Run all tests in Docker mode
tsuite run --suite ./my-suite --all --docker

# Run specific use case
tsuite run --suite ./my-suite --uc uc01_registry --docker

# Run specific test case
tsuite run --suite ./my-suite --tc uc01_registry/tc01_agent_registration --docker

# Run tests matching tags
tsuite run --suite ./my-suite --tag smoke --docker

# Dry run (list tests without executing)
tsuite run --suite ./my-suite --dry-run --all
```

### Dashboard & API Server

```bash
# Start on default port (9999)
tsuite api

# Start on custom port
tsuite api --port 8080

# Start in background (detached)
tsuite api --detach

# Stop background server
tsuite stop
```

### Scaffold Test Cases

Generate test cases from agent directories:

```bash
# Interactive mode
tsuite scaffold --suite ./my-suite ./path/to/agent1 ./path/to/agent2

# Non-interactive mode
tsuite scaffold --suite ./my-suite --uc uc01_tags --tc tc01_test ./agent1 ./agent2

# Preview without creating files
tsuite scaffold --suite ./my-suite --uc uc01_tags --tc tc01_test --dry-run ./agent1
```

### Documentation

```bash
# List available topics
tsuite man --list

# View specific topic
tsuite man quickstart
tsuite man handlers
tsuite man assertions
tsuite man routines
```

### Clear Data

```bash
# Clear all test data
tsuite clear --all

# Clear specific run
tsuite clear --run-id <run_id>
```

## Features

- **YAML-based test definitions** - Tests as configuration, not code
- **Container isolation** - Each test runs in a fresh Docker container
- **Parallel execution** - Worker pool for concurrent test runs
- **Web dashboard** - Real-time monitoring, history, and test editor
- **Pluggable handlers** - shell, http, file, wait, pip-install, npm-install
- **Expression language** - Flexible assertions with jq, JSONPath support
- **Reusable routines** - Define once, use across tests
- **Scaffold command** - Auto-generate test cases from agent directories

## Test Suite Structure

```
my-suite/
├── config.yaml              # Suite configuration
├── global/
│   └── routines.yaml        # Global reusable routines
└── suites/
    └── uc01_example/        # Use case folder
        ├── routines.yaml    # UC-level routines (optional)
        └── tc01_test/       # Test case folder
            ├── test.yaml    # Test definition
            └── artifacts/   # Test artifacts (agents, fixtures)
```

## Example Test

```yaml
name: "Agent Registration Test"
description: "Verify agent registers with mesh"
tags: [smoke, registry]
timeout: 300

pre_run:
  - routine: global.setup_for_python_agent
    params:
      meshctl_version: "${config.packages.cli_version}"

test:
  - name: "Copy agent to workspace"
    handler: shell
    command: "cp -r /artifacts/my-agent /workspace/"

  - name: "Start agent"
    handler: shell
    command: "meshctl start my-agent/main.py -d"
    workdir: /workspace

  - name: "Wait for registration"
    handler: wait
    seconds: 5

  - name: "Verify agent registered"
    handler: shell
    command: "meshctl list"
    capture: agent_list

assertions:
  - expr: "${captured.agent_list} contains 'my-agent'"
    message: "Agent should be registered"

post_run:
  - handler: shell
    command: "meshctl stop || true"
    workdir: /workspace
```

## Documentation Topics

Run `tsuite man <topic>` for detailed documentation:

| Topic | Description |
|-------|-------------|
| quickstart | Getting started guide |
| suites | Suite structure and config.yaml |
| testcases | Test case structure and test.yaml |
| handlers | Built-in handlers (shell, http, file, etc.) |
| routines | Reusable test routines |
| assertions | Assertion syntax and expressions |
| variables | Variable interpolation syntax |
| docker | Docker mode and container isolation |
| api | API server and dashboard |

## License

MIT
