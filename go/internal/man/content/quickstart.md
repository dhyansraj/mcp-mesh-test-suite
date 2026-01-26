# Quick Start

> Get started with tsuite in minutes

## Installation

```bash
pip install mcp-mesh-tsuite
```

## 1. Start the Dashboard

```bash
# Start API server with web dashboard
tsuite api --port 9999
```

Open http://localhost:9999 in your browser.

## 2. Create a Test Suite

```bash
mkdir my-tests
cd my-tests

# Create suite structure
mkdir -p suites/uc01_basic/tc01_hello
```

## 3. Create config.yaml

```yaml
# my-tests/config.yaml
suite:
  name: My Test Suite
  mode: standalone  # or 'docker' for container isolation

docker:
  image: python:3.11-slim
```

## 4. Create Your First Test

```yaml
# my-tests/suites/uc01_basic/tc01_hello/test.yaml
name: Hello World Test
description: A simple test that runs a command

test:
  - name: Say Hello
    exec:
      command: echo "Hello, World!"
    capture:
      output: result.stdout

assertions:
  - expr: captured.output
    contains: "Hello"
```

## 5. Run the Test

```bash
# Run all tests
tsuite run --suite-path ./my-tests --all

# Run specific test case
tsuite run --suite-path ./my-tests --tc uc01_basic/tc01_hello
```

## 6. View Results

Results are visible in:
- Terminal output (pass/fail summary)
- Dashboard at http://localhost:9999

## Next Steps

- `tsuite man suites` - Learn about suite structure
- `tsuite man handlers` - Available test handlers
- `tsuite man assertions` - Assertion syntax
