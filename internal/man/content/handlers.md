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
| `probe` | Poll a command until it reports readiness |
| `file` | File operations (exists, read, write, delete, mkdir) |
| `http` | Make an HTTP request |
| `pip-install` | Install Python packages or a requirements.txt |
| `npm-install` | Run `npm install` for a Node project |
| `maven-install` | Resolve Maven dependencies for a project |
| `gradle-install` | Resolve Gradle dependencies for a project |
| `secrets` | Write configured secrets into an env file |
| `runner` | Download the embedded runner binaries from the API server |

## Duration Options

Every duration option below (`timeout`, `interval`) accepts either a number of
seconds or a duration string such as `500ms`, `30s`, `5m`, or `1m30s`. The two
forms are interchangeable, so `timeout: 1200` and `timeout: 20m` are the same
deadline.

```yaml
- name: Build the agent image
  handler: shell
  command: docker build -t agent .
  timeout: 20m
```

Leave the option out to get the handler's default. A value that is present but
unparseable (`timeout: fivem`) fails the step with an error naming the field,
rather than quietly reverting to the default and running with a deadline you did
not ask for. `0` and negative values are rejected the same way; there is no
"no timeout" value.

The `wait` handler's `seconds:` is a plain number of seconds, not a duration
string.

## shell Handler

Run a command with `bash -c`. Stdout, stderr, and the exit code are captured.

| Option | Description | Required / Default |
|--------|-------------|--------------------|
| `command` | Command to run | required |
| `workdir` | Working directory | default `/workspace` |
| `timeout` | Timeout, in seconds or as a duration string | default `120` (seconds) |

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
| `interval` | Time between polls (`type: http`), in seconds or as a duration string | default `2` (seconds) |
| `timeout` | How long to wait for the URL (`type: http`), in seconds or as a duration string | default `30` (seconds) |
| `insecure_tls` | Skip TLS certificate verification (`type: http`) | default `false` |
| `ca_cert` | Path to a PEM CA bundle to trust (`type: http`) | optional |

A URL is considered ready when it responds with a status below 400. When the
wait gives up, the error reports why the last poll failed - the transport error,
or the status that kept coming back - so a refused connection, a rejected
certificate and a server stuck on 503 are not the same message.

```yaml
- name: Wait for registry
  handler: wait
  type: http
  url: http://localhost:8000/health
  interval: 2
  timeout: 60
```

```yaml
- name: Wait for registry over TLS
  handler: wait
  type: http
  url: https://localhost:8443/health
  ca_cert: ${env:HOME}/.mcp-mesh/tls/ca.pem
  timeout: 60
```

```yaml
- name: Settle
  handler: wait
  type: seconds
  seconds: 5
```

## probe Handler

Poll a command until it reports readiness, instead of hand-writing a retry loop
in bash. The command runs exactly like a `shell` step, once per attempt, until
the success condition holds or the deadline passes.

| Option | Description | Required / Default |
|--------|-------------|--------------------|
| `command` | Command to poll | required |
| `workdir` | Working directory | default `/workspace` |
| `interval` | Time between attempts, in seconds or as a duration string | default `2` (seconds) |
| `timeout` | Overall deadline, in seconds or as a duration string | default `60` (seconds) |
| `until` | Assertion expression deciding success | default: exit code `0` |
| `success_threshold` | Consecutive passes required | default `1` |
| `on_failure` | Command run once when the probe gives up | optional |

A non-zero exit is a failed attempt, not a fatal error. When `until` is set it
is evaluated with the same syntax as `assertions:`, against the latest attempt's
`${stdout}`, `${stderr}`, and `${exit_code}`; `${stdout}` and `${stderr}` have
trailing whitespace trimmed, so `${stdout} == 3` matches jq's `3\n`.

On success the step's stdout is the final attempt's output verbatim, so
`capture` behaves exactly as it does for `shell`, and the poll trace is written
to stderr. On timeout the step fails with an error naming the probe, the elapsed
time, the attempt count, and the last attempt's output, and any `on_failure`
output is appended to stderr.

```yaml
- name: Wait for calendar-agent to serve
  handler: probe
  command: curl -sf http://localhost:9092/health
  interval: 2
  timeout: 120
```

```yaml
- name: Wait for 3 agents to register
  handler: probe
  command: meshctl list --json | jq '.agents | length'
  until: ${stdout} >= 3
  interval: 3
  timeout: 120
  success_threshold: 2
  on_failure: meshctl logs registry | tail -50
```

> Keep `timeout` below the test's own `timeout:`, which hard-kills the whole
> test; the probe enforces its own deadline and reports a diagnostic failure.

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
| `body` | Request body (string, or a map/list sent as JSON) | optional |
| `timeout` | Timeout, in seconds or as a duration string | default `30` (seconds) |
| `insecure_tls` | Skip TLS certificate verification | default `false` |
| `ca_cert` | Path to a PEM CA bundle to trust | optional |

A string `body` is sent verbatim, so set `Content-Type` yourself. A `body`
written as YAML structure is marshalled to JSON and `Content-Type:
application/json` is added unless `headers` already sets it. Header values and
body values are interpolated.

```yaml
- name: Register agent
  handler: http
  method: POST
  url: http://localhost:8000/agents/register
  headers:
    Content-Type: application/json
  body: '{"name": "greeter", "capabilities": ["greeting"]}'
  capture: register_response
```

```yaml
- name: Register agent (structured body)
  handler: http
  method: POST
  url: http://localhost:8000/agents/register
  body:
    name: greeter
    capabilities:
      - greeting
  capture: register_response
```

### TLS

`insecure_tls` and `ca_cert` are available on both the `http` handler and `wait`
with `type: http`. Neither is set by default: an `https://` URL is verified
against the system trust store, exactly as `curl` would.

`ca_cert` names a PEM bundle to trust **in addition to** the system roots, so a
step that pins a private CA can still reach public-CA hosts. The path is
interpolated like `url`, so it can be written relative to an environment
variable. A path that does not exist, or a file with no certificate in it, fails
the step rather than quietly falling back to default trust.

```yaml
- name: Query registry over TLS
  handler: http
  url: https://localhost:8443/agents
  ca_cert: ${env:HOME}/.mcp-mesh/tls/ca.pem
  capture: agents
```

`insecure_tls: true` turns verification off entirely. Setting it together with
`ca_cert` is an error, not a precedence rule - "trust nothing" and "trust
exactly this CA" are contradictory, so the step fails instead of guessing.

```yaml
- name: Query a self-signed dev endpoint
  handler: http
  url: https://localhost:8443/agents
  insecure_tls: true
```

## pip-install Handler

Install Python packages with `pip`, either from a `requirements.txt` or from an
explicit package list. Provide one of `path` or `packages`.

| Option | Description | Required / Default |
|--------|-------------|--------------------|
| `path` | Path to a `requirements.txt` (or a directory containing one) | required unless `packages` |
| `packages` | List of package specs to install | required unless `path` |
| `timeout` | Timeout, in seconds or as a duration string | default `300` (seconds) |

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
| `timeout` | Timeout, in seconds or as a duration string | default `300` (seconds) |

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
| `timeout` | Timeout, in seconds or as a duration string | default `300` (seconds) |

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
| `timeout` | Timeout, in seconds or as a duration string | default `300` (seconds) |

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

## runner Handler

Download every runner binary embedded in the API server into a directory and
mark it executable. This is for tests that build or exercise a nested tsuite
setup; ordinary tests never need it. Requires the `TSUITE_API` environment
variable (the runner sets this automatically).

| Option | Description | Required / Default |
|--------|-------------|--------------------|
| `dest` | Directory to write the binaries into | required |

Relative `dest` paths are resolved against the working directory (default
`/workspace`), and the directory is created if missing. The step fails if the
API server has no embedded runners — build tsuite with `make build-with-runners`
to embed them.

```yaml
- name: Stage runner binaries
  handler: runner
  dest: /workspace/bin/runners
```

## See Also

- `tsuite man testcases` - Test case structure
- `tsuite man assertions` - Validating results
- `tsuite man variables` - Variable interpolation
- `tsuite man routines` - Reusing step sequences
