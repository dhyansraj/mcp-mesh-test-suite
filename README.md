# MCP Mesh Test Suite

Integration test suite for [MCP Mesh](https://github.com/dhyansraj/mcp-mesh).

## Structure

```
mcp-mesh-test-suite/
├── tsuite/          # Test framework (Python package)
├── dashboard/       # Web dashboard (Next.js)
├── lib-tests/       # Library tests & Docker image builder
└── integration/     # Integration test suites
```

## Components

### tsuite

The `tsuite` Python package provides the test framework for running integration tests. It supports:
- YAML-based test definitions
- Docker container execution
- Test result database and reporting
- Routines and reusable test patterns

### dashboard

Web dashboard for monitoring test runs and managing test suites:
- Real-time test execution monitoring via SSE
- Test result history and filtering
- Test case editor with YAML preview

### lib-tests

Builds the `tsuite-mesh` Docker image with pre-installed MCP Mesh packages:
- meshctl CLI
- Python SDK (mcp-mesh)
- Node.js for TypeScript agents

### integration

Integration test suites organized by use case:
- `uc01_*` - Registry and scaffolding tests
- `uc02_*` - Agent lifecycle and tools tests
- `uc03_*` - Capabilities and tag matching tests

## Quick Start

```bash
# 1. Setup (first time only)
cd tsuite && python3.11 -m venv venv && source venv/bin/activate && pip install -r requirements.txt -e .
cd ../dashboard && npm install

# 2. Build the Docker image (lib-tests)
./start.sh --cli --suite-path lib-tests

# 3. Run integration tests
./start.sh --cli --docker
```

## Running the Suite

Use `start.sh` to run CLI, API server, or dashboard:

```bash
./start.sh --api          # Start API server (http://localhost:9999)
./start.sh --ui           # Start dashboard UI (http://localhost:3000)
./start.sh --cli          # Run CLI with default suite (integration/suites)
./start.sh --all          # Start API + UI in background, then wait
```

### CLI Options

```bash
./start.sh --cli                              # Run all tests
./start.sh --cli --uc uc01_registry           # Run specific use case
./start.sh --cli --tc tc01_simple             # Run specific test case
./start.sh --cli --docker                     # Run tests in Docker mode
./start.sh --cli --standalone                 # Run tests without Docker
./start.sh --cli --suite-path /path/to/suite  # Run specific suite
```

## Dashboard

The web dashboard provides real-time test monitoring and suite management.

```bash
# Terminal 1: Start API server
./start.sh --api

# Terminal 2: Start dashboard
./start.sh --ui
```

The dashboard will be available at http://localhost:3000

Add test suites via Settings → Add Suite in the dashboard UI.

## Configuration

Each component has a `config.yaml` for version configuration:

```yaml
packages:
  cli_version: "0.8.0-beta.8"
  sdk_python_version: "0.8.0b8"
  sdk_typescript_version: "0.8.0-beta.8"
```

## License

MIT
