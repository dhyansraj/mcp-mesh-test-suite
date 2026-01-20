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
# 1. Setup Python environment (Python 3.11 required)
cd lib-tests
python3.11 -m venv venv
source venv/bin/activate
pip install -e ../tsuite

# 2. Build the Docker image
tsuite --all

# 3. Run integration tests
cd ../integration
python3.11 -m venv venv
source venv/bin/activate
pip install -e ../tsuite

tsuite --all --docker
```

## Dashboard

The test suite includes a web dashboard for viewing test results and managing test suites.

### Starting the Dashboard

```bash
# Terminal 1: Start the API server
cd mcp-mesh-test-suite
source tsuite/venv/bin/activate
python -m tsuite.server --port 9999

# Terminal 2: Start the web server
cd mcp-mesh-test-suite/dashboard
npm install  # first time only
npm run dev
```

The dashboard will be available at http://localhost:3000

You can add test suites via Settings → Add Suite in the dashboard UI.

Alternatively, pre-sync suites at startup:
```bash
python -m tsuite.server --port 9999 --suites integration,lib-tests
```

### Running Tests with Dashboard

When running tests while the dashboard server is running, use a different port:

```bash
# In integration/ or lib-tests/ directory
tsuite --all --docker --port 9998
```

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
