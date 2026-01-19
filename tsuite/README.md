# tsuite - Integration Test Framework

A YAML-driven integration test framework with container isolation.

## Features

- **YAML-based test definitions** - Tests as configuration, not code
- **Container isolation** - Each test runs in a fresh Docker container
- **Pluggable handlers** - Extensible test actions (shell, file, http, etc.)
- **Expression language** - Flexible assertions without Python
- **Reusable routines** - Define once, use anywhere (global/UC/TC scopes)
- **REST API server** - Container ↔ host communication

## Installation

```bash
pip install -e .
```

## Usage

```bash
# Run with tsuite CLI
tsuite --all --suite-path /path/to/test-suites

# Or use the run.py in your test suite
cd /path/to/test-suites
./run.py --all
```

## Architecture

```
test-suite/
├── tsuite/              # Core framework
│   ├── cli.py          # Command-line interface
│   ├── server.py       # REST API server
│   ├── discovery.py    # Test discovery
│   ├── executor.py     # Test execution
│   ├── context.py      # Runtime context
│   ├── expressions.py  # Expression evaluator
│   ├── routines.py     # Routine resolver
│   └── client.py       # Container client library
│
└── handlers/           # Pluggable handlers
    ├── shell.py       # Shell commands
    ├── file.py        # File operations
    └── routine.py     # Routine invocation
```

## Expression Language

```yaml
assertions:
  # Exact match
  - expr: ${exit_code} == 0

  # String contains
  - expr: ${stdout} contains 'success'

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
# global/routines.yaml
routines:
  install_deps:
    params:
      version: { type: string, required: true }
    steps:
      - handler: shell
        command: "npm install -g package@${params.version}"
```

Usage in tests:
```yaml
pre_run:
  - routine: global.install_deps
    params:
      version: "1.0.0"
```

## Server API

The framework runs a REST server for container communication:

| Endpoint | Description |
|----------|-------------|
| `GET /config` | Get configuration |
| `GET /state/{test_id}` | Get test state |
| `POST /state/{test_id}` | Update test state |
| `POST /progress/{test_id}` | Report progress |

## Extending

Add new handlers in `handlers/`:

```python
# handlers/myhandler.py
from tsuite.context import StepResult

def execute(step: dict, context: dict) -> StepResult:
    # Implement your handler
    return StepResult(success=True, exit_code=0)
```

Register in `handlers/__init__.py` and use in tests:

```yaml
test:
  - handler: myhandler
    param1: value1
```
