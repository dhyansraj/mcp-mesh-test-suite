# Docker Mode

> Container isolation and Docker execution

## Overview

Docker mode runs each test in an isolated container, providing:
- Reproducible environment
- Process isolation
- Clean state per test
- Parallel execution support

## Enabling Docker Mode

```yaml
# config.yaml
suite:
  mode: docker

docker:
  image: python:3.11-slim
```

## Docker Configuration

### Basic Settings

```yaml
docker:
  image: python:3.11-slim      # Required: base image
  network: host                 # Network mode
  pull: always                  # Pull policy: always, never, if-not-present
```

### Environment Variables

```yaml
docker:
  env:
    API_URL: http://host.docker.internal:8080
    DEBUG: "true"
    DATABASE_URL: postgres://db:5432/test
```

### Volume Mounts

```yaml
docker:
  volumes:
    - /host/path:/container/path:ro
    - ${HOME}/.config:/config:ro
```

### Resource Limits

```yaml
docker:
  memory: 512m
  cpus: 1.0
```

## Automatic Mounts

tsuite automatically mounts:

| Host Path | Container Path | Mode |
|-----------|----------------|------|
| Suite artifacts | `/suite-artifacts/` | Read-only |
| UC artifacts | `/uc-artifacts/` | Read-only |
| TC artifacts | `/tc-artifacts/` | Read-only |
| Output directory | `/output/` | Read-write |

## Network Modes

### Host Network

```yaml
docker:
  network: host
```

- Container shares host network
- Access services on localhost
- Simplest for local testing

### Bridge Network (Default)

```yaml
docker:
  network: bridge
```

- Isolated network
- Use `host.docker.internal` for host services
- Better isolation

### Custom Network

```yaml
docker:
  network: my-test-network
```

- Connect to existing Docker network
- Useful for service dependencies

## Accessing Host Services

From container, use:
- `host.docker.internal` (Docker Desktop)
- `172.17.0.1` (Linux default gateway)

```yaml
docker:
  env:
    API_URL: http://host.docker.internal:8080
```

## Custom Images

### Using Pre-built Images

```yaml
docker:
  image: my-registry/my-test-image:latest
```

### Building Images

For complex test environments, build a custom image:

```dockerfile
# Dockerfile
FROM python:3.11-slim

RUN pip install requests pytest
COPY test_utils.py /app/

WORKDIR /app
```

```yaml
docker:
  image: my-test-image:latest
  build:
    context: ./docker
    dockerfile: Dockerfile
```

## Parallel Execution

Docker mode supports parallel test execution:

```yaml
suite:
  mode: docker
  parallel: 4  # Run up to 4 tests concurrently
```

Each test runs in its own container with full isolation.

## Container Lifecycle

1. **Create** - Container created from image
2. **Start** - Container started
3. **Execute** - Test steps run inside container
4. **Capture** - Output and artifacts collected
5. **Stop** - Container stopped
6. **Remove** - Container removed (cleanup)

## Debugging

### Keep Container Running

For debugging, prevent automatic cleanup:

```bash
tsuite run --suite-path ./my-suite --tc uc01/tc01 --keep-container
```

### View Container Logs

```bash
docker logs tsuite-<run-id>-<test-id>
```

### Interactive Mode

```bash
tsuite run --suite-path ./my-suite --tc uc01/tc01 --interactive
```

## Comparison with Standalone

| Feature | Docker | Standalone |
|---------|--------|------------|
| Isolation | Full container | Process only |
| Reproducibility | High | Depends on host |
| Parallel execution | Yes | Sequential |
| Startup time | Slower | Faster |
| Host access | Via network | Direct |

## See Also

- `tsuite man suites` - Suite configuration
- `tsuite man artifacts` - File mounting
- `tsuite man api` - Dashboard and monitoring
