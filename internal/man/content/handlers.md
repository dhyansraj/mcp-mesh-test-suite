# Handlers

> Built-in handlers for test steps

## Overview

Handlers define what action a test step performs. Each step names exactly one
handler via the `handler:` field, and the remaining fields configure it.

Any step may add `capture: <name>` to save the handler's stdout under that
name. The captured value is then available to later steps and to assertions as
`${captured.<name>}`.

## Available Handlers

| Handler | Description |
|---------|-------------|
| `shell` | Run a shell command with bash |
| `wait` | Pause for N seconds or poll a URL until ready |
| `file` | File operations (exists, read, write, delete, mkdir) |
| `http` | Make an HTTP request |
| `pip-install` | Install Python packages or a requirements.txt |
| `npm-install` | Run `npm install` for a Node project |
| `maven-install` | Resolve Maven dependencies for a project |
| `gradle-install` | Resolve Gradle dependencies for a project |
| `secrets` | Write configured secrets into an env file |

> `runner` is an internal handler that extracts embedded runner binaries from
> the API server for distributed execution; it is not meant to be written in a
> `test.yaml` by hand.

## shell Handler

Run a command with `bash -c`. Stdout, stderr, and the exit code are captured.

| Option | Description | Required / Default |
|--------|-------------|--------------------|
| `command` | Command to run | required |
| `workdir` | Working directory | default `/workspace` |
| `timeout` | Timeout in seconds | default `120` |

```yaml
- name: Print mesh version
  handler: shell
  command: meshctl version
  workdir: /workspace/examples
  capture: version_output
```

## wait Handler

Pause execution, either for a fixed duration or until a URL becomes reachable.

| Option | Description | Required / Default |
|--------|-------------|--------------------|
| `type` | `seconds` or `http` | default `seconds` |
| `seconds` | Seconds to wait (`type: seconds`) | default `1` |
| `url` | URL to poll (`type: http`) | required for `http` |
| `interval` | Seconds between polls (`type: http`) | default `2` |
| `timeout` | Max seconds to wait for the URL (`type: http`) | default `30` |

A URL is considered ready when it responds with a status below 400.

```yaml
- name: Wait for registry
  handler: wait
  type: http
  url: http://localhost:8000/health
  interval: 2
  timeout: 60
```

```yaml
- name: Settle
  handler: wait
  type: seconds
  seconds: 5
```

## file Handler

Perform a file operation. Relative paths are resolved against the working
directory (default `/workspace`).

| Option | Description | Required / Default |
|--------|-------------|--------------------|
| `operation` | One of `exists`, `read`, `write`, `delete`, `mkdir` | default `exists` |
| `path` | Target file or directory | required |
| `content` | Content to write (`operation: write`) | optional |

`read` returns the file contents as stdout (capture it to use the value).
`write` creates parent directories as needed.

```yaml
- name: Write config
  handler: file
  operation: write
  path: /workspace/config.yaml
  content: |
    log_level: debug
    port: 8080
```

```yaml
- name: Read result
  handler: file
  operation: read
  path: /workspace/output.txt
  capture: result_text
```

## http Handler

Make an HTTP request. The response body is returned as stdout; the step fails
on a status of 400 or above.

| Option | Description | Required / Default |
|--------|-------------|--------------------|
| `method` | HTTP method | default `GET` |
| `url` | Request URL | required |
| `headers` | Request headers (map) | optional |
| `body` | Request body: a string, or a map sent as JSON | optional |
| `timeout` | Timeout in seconds | default `30` |

When `body` is a map it is marshaled to JSON and `Content-Type:
application/json` is set automatically unless you provide it yourself.

```yaml
- name: Register agent
  handler: http
  method: POST
  url: http://localhost:8000/agents/register
  headers:
    Content-Type: application/json
  body:
    name: greeter
    capabilities:
      - greeting
  capture: register_response
```

## pip-install Handler

Install Python packages with `pip`, either from a `requirements.txt` or from an
explicit package list. Provide one of `path` or `packages`.

| Option | Description | Required / Default |
|--------|-------------|--------------------|
| `path` | Path to a `requirements.txt` (or a directory containing one) | required unless `packages` |
| `packages` | List of package specs to install | required unless `path` |
| `timeout` | Timeout in seconds | default `300` |

If a `/wheels` directory is present, the handler installs from those local
wheels first (offline mode) before resolving the rest.

```yaml
- name: Install requirements
  handler: pip-install
  path: examples/simple_agent
```

```yaml
- name: Install packages
  handler: pip-install
  packages:
    - mcp-mesh
    - pytest
```

## npm-install Handler

Run `npm install --legacy-peer-deps` for a Node project (a directory containing
`package.json`). `node_modules` is cleared first to avoid platform mismatches.

| Option | Description | Required / Default |
|--------|-------------|--------------------|
| `path` | Directory containing `package.json` | required |
| `replace_file_deps` | Replace `file:` dependencies with a resolvable version | default `true` |
| `timeout` | Timeout in seconds | default `300` |

By default `file:` dependencies (which point at host paths that don't exist in
the container) are rewritten so `npm install` can resolve them. If a `/packages`
directory of local `.tgz` tarballs is present, matching dependencies are rewired
to those tarballs so direct and transitive deps resolve offline.

```yaml
- name: Install node deps
  handler: npm-install
  path: examples/typescript_agent
```

## maven-install Handler

Resolve Maven dependencies by running `mvn dependency:resolve` for a project (a
directory containing `pom.xml`).

| Option | Description | Required / Default |
|--------|-------------|--------------------|
| `path` | Directory containing `pom.xml` | required |
| `strip_file_repos` | Remove `<repository>` blocks with `file://` URLs | default `true` |
| `align_version` | Align `<mcp-mesh.version>` to the SDK found in the local m2 repo | default `true` |
| `m2_repo` | Local m2 repository path used for version alignment | default `/root/.m2/repository` |
| `timeout` | Timeout in seconds | default `300` |

```yaml
- name: Resolve maven deps
  handler: maven-install
  path: examples/java_agent
```

## gradle-install Handler

Resolve Gradle dependencies by running `gradle dependencies` for a project (a
directory containing `build.gradle` or `build.gradle.kts`). The `gradlew`
wrapper is used when present, otherwise the system `gradle`.

| Option | Description | Required / Default |
|--------|-------------|--------------------|
| `path` | Directory containing a Gradle build file | required |
| `strip_file_repos` | Remove `maven { ... }` repositories with `file://` URLs | default `true` |
| `timeout` | Timeout in seconds | default `300` |

```yaml
- name: Resolve gradle deps
  handler: gradle-install
  path: examples/kotlin_agent
```

## secrets Handler

Fetch secrets configured on the API server and write them as `KEY=value` lines
into a target env file (mode `0600`). Requires the `TSUITE_API` environment
variable to be set (the runner sets this automatically).

| Option | Description | Required / Default |
|--------|-------------|--------------------|
| `target` | Env file to write | required |
| `source` | File whose contents are prepended before the secrets | optional |
| `keys` | List of secret keys to include (default: all) | optional |

```yaml
- name: Inject secrets
  handler: secrets
  source: examples/.env.example
  target: examples/.env
  keys:
    - OPENAI_API_KEY
    - DATABASE_URL
```

## See Also

- `tsuite man testcases` - Test case structure
- `tsuite man assertions` - Validating results
- `tsuite man variables` - Variable interpolation
- `tsuite man routines` - Reusing step sequences
