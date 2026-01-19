# MCP Mesh Test Suite

Integration test suite for [MCP Mesh](https://github.com/dhyansraj/mcp-mesh).

## Structure

```
mcp-mesh-test-suite/
├── tsuite/          # Test framework (Python package)
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
