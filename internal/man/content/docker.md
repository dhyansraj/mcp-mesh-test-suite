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
  base_image: python:3.11-slim
```

## Docker Configuration

`docker` accepts exactly two settings:

```yaml
docker:
  base_image: python:3.11-slim  # Image workers run in
  network: bridge               # Network mode
```

There are no `image`, `env`, `volumes`, `pull`, `memory`, or `cpus` settings;
anything else under `docker:` is ignored. Per-test environment values belong in
the step itself (`handler: shell` with `VAR=... cmd`) or in the `secrets`
handler.

## Automatic Mounts

tsuite automatically mounts:

| Host Path | Container Path | Mode |
|-----------|----------------|------|
| Suite directory | `/tests/` | Read-only |
| Per-test working directory | `/workspace/` | Read-write |
| UC artifacts (per entry) | `/uc-artifacts/` | Read-only |
| TC artifacts (per entry) | `/artifacts/` | Read-only |
| `~/.tsuite/runs/<run>/<uc>/<tc>/` | `/var/log/tsuite/` | Read-write |

Artifact directories are mounted one entry at a time so that symlinks inside
them are resolved to their targets.

## Networking

Worker containers run on the default `bridge` network. To reach a service on
the host from inside a test, use:

- `host.docker.internal` (Docker Desktop)
- `172.17.0.1` (Linux default gateway)

The API URL handed to the runner already has `localhost`/`127.0.0.1` rewritten
to `host.docker.internal`.

## Custom Images

Point `base_image` at any image that has the toolchains your tests need:

```yaml
docker:
  base_image: my-registry/my-test-image:latest
```

Build it beforehand (tsuite does not build images for you):

```dockerfile
# Dockerfile
FROM python:3.11-slim

RUN pip install requests pytest
COPY test_utils.py /app/

WORKDIR /app
```

```bash
docker build -t my-test-image:latest .
```

## Parallel Execution

Run several containers at once with `--parallel`, or set `execution.max_workers`
in `config.yaml`:

```bash
tsuite run --suite-path ./my-suite --parallel 4
```

Each test runs in its own container with full isolation.

## Container Lifecycle

1. **Create** - Container created from `base_image`
2. **Start** - Container started, runner binary injected
3. **Execute** - Test steps run inside the container
4. **Capture** - Output streamed back to the API server
5. **Stop** - Container stopped
6. **Remove** - Container removed (cleanup)

## Debugging

Worker logs for each test are written to the host at
`~/.tsuite/runs/<run-id>/<uc>/<tc>/worker.log`, and step output is visible in
the dashboard and the terminal. Containers are removed after each test, so
inspect those logs rather than the container.

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
