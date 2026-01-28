# Test Suites

> Suite structure and config.yaml

## Directory Structure

```
my-suite/
├── config.yaml           # Suite configuration
├── routines.yaml         # Suite-level reusable routines (optional)
├── artifacts/            # Suite-level shared files (optional)
└── suites/
    ├── uc01_feature_a/
    │   ├── routines.yaml # UC-level routines (optional)
    │   ├── artifacts/    # UC-level shared files (optional)
    │   ├── tc01_test/
    │   │   ├── test.yaml
    │   │   └── artifacts/
    │   └── tc02_test/
    │       └── test.yaml
    └── uc02_feature_b/
        └── tc01_test/
            └── test.yaml
```

## config.yaml

The suite configuration file defines global settings.

```yaml
suite:
  name: My Integration Tests
  mode: docker           # 'docker' or 'standalone'
  description: Tests for my application

docker:
  image: python:3.11-slim
  network: host          # or custom network name
  env:
    API_URL: http://localhost:8080
    DEBUG: "true"

defaults:
  timeout: 60            # Default test timeout in seconds
  retry: 0               # Default retry count
```

## Execution Modes

### Standalone Mode

Tests run directly on the host machine as subprocesses.

```yaml
suite:
  mode: standalone
```

- Faster execution (no container overhead)
- Tests share host environment
- Good for local development

### Docker Mode

Tests run in isolated containers.

```yaml
suite:
  mode: docker

docker:
  image: python:3.11-slim
```

- Full isolation between tests
- Reproducible environment
- Parallel execution support

## Environment Variables

Define environment variables for all tests:

```yaml
docker:
  env:
    DATABASE_URL: postgres://localhost:5432/test
    API_KEY: ${API_KEY}  # From host environment
```

## See Also

- `tsuite man usecases` - Use case organization
- `tsuite man testcases` - Test case structure
- `tsuite man docker` - Docker mode details
