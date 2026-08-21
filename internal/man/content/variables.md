# Variables

> Variable interpolation syntax

## Overview

Variables allow dynamic values in test definitions. Use `${...}` syntax
anywhere in a step's string fields and in assertion expressions.

A reference that cannot be resolved is left in place verbatim (no error), so a
literal `${...}` in output usually means a typo in the variable name.

## Prefixed Sources

| Prefix | Description | Example |
|--------|-------------|---------|
| `config.` | Value from `config.yaml` (dot path) | `${config.packages.cli_version}` |
| `state.` | Shared state value (reserved; nothing populates it yet) | `${state.agent_port}` |
| `captured.` | Stdout captured by an earlier step | `${captured.version_output}` |
| `steps.` | Full result of a captured step | `${steps.build.exit_code}` |
| `last.` | Result of the immediately previous step | `${last.exit_code}` |
| `params.` | Routine parameter | `${params.username}` |
| `json:` | JSONPath query over the last step's stdout | `${json:$.agents[0].name}` |
| `jq:` | jq query (see below) | `${jq:.agents \| length}` |
| `jsonfile:` | JSONPath query over a JSON file | `${jsonfile:/workspace/out.json:$.status}` |
| `file:` | Contents of a file | `${file:/workspace/out.txt}` |
| `fixture:` | Contents of a file under the suite's `fixtures/` dir | `${fixture:expected/agents.json}` |
| `env:` | Environment variable | `${env:HOME}` |

Only these exact prefixes are recognized. In particular, environment variables
must be written `${env:API_KEY}` — a bare `${API_KEY}` is not read from the
environment.

## Bare Names

| Variable | Description |
|----------|-------------|
| `${exit_code}` | Exit code of the previous step (same as `${last.exit_code}`) |
| `${stdout}` | Stdout of the previous step |
| `${stderr}` | Stderr of the previous step |
| `${suite_path}` | Absolute path to the suite root |
| `${workdir}` | Working directory for the test (`/workspace` in Docker/K8s mode) |
| `${fixtures_dir}` | `<suite_path>/fixtures` |
| `${artifacts}` / `${artifacts_path}` | TC artifacts directory on the host |
| `${uc_artifacts}` / `${uc_artifacts_path}` | UC artifacts directory on the host |
| `${test_id}` | Current test ID, `uc_name/tc_name` |
| `${uc_name}` | Current use case directory name |
| `${tc_name}` | Current test case directory name |
| `${run_path}` | `~/.tsuite/runs/<run-id>` on the executing host |

Any other bare name is resolved, in order, against captured values, shared
state, and finally the `config.yaml` map (so `${suite.name}` also works).

Names are case-sensitive: `${TEST_ID}` is not the same as `${test_id}` and will
not resolve.

## Config Variables

Access values from `config.yaml` by dot path:

```yaml
# config.yaml
suite:
  name: My Tests
  mode: docker

docker:
  base_image: python:3.11-slim

packages:
  cli_version: 0.8.0
```

```yaml
# test.yaml
test:
  - name: Install the pinned CLI
    handler: shell
    command: pip install mcp-mesh==${config.packages.cli_version}
```

## Environment Variables

Environment variables of the process running the step are read with the `env:`
prefix:

```yaml
test:
  - name: Call API with a key from the environment
    handler: http
    url: http://localhost:8080/data
    headers:
      Authorization: Bearer ${env:API_KEY}
```

To place secrets into a test environment file, use the `secrets` handler
(`tsuite man handlers`).

## Captured Variables

`capture:` is a plain string naming the variable. It stores the step's stdout:

```yaml
test:
  - name: List agents
    handler: shell
    command: meshctl list --json
    capture: agent_list

  - name: Show what we got
    handler: shell
    command: echo "${captured.agent_list}"
```

The full step result is also available under `steps.<name>`:

```yaml
- expr: "${steps.agent_list.exit_code} == 0"
- expr: "${steps.agent_list.success} == true"
```

Available `steps.<name>` fields: `exit_code`, `stdout`, `stderr`, `success`,
`error`.

## Previous Step Results

```yaml
test:
  - name: Build
    handler: shell
    command: make build

  - name: Report
    handler: shell
    command: echo "build exited ${last.exit_code}"
```

`${exit_code}`, `${stdout}`, and `${stderr}` are shorthands for the `last.*`
values.

## JSON Queries

Captured values are strings, so drill into JSON with `jq:`, `json:`, or
`jsonfile:` rather than dotted access.

`${jq:<query>}` runs the query against the **last step's stdout**:

```yaml
test:
  - name: List agents
    handler: shell
    command: meshctl list --json

  - name: Count agents
    handler: shell
    command: echo "count=${jq:.agents | length}"
```

`${jq:captured.<name>:<query>}` runs it against a captured variable:

```yaml
test:
  - name: List agents
    handler: shell
    command: meshctl list --json
    capture: agent_list

assertions:
  - expr: "${jq:captured.agent_list:.agents | length} > 0"
    message: "At least one agent should be registered"
  - expr: "${jq:captured.agent_list:.agents[0].status} == 'running'"
```

`${jq:captured.<name>}` with no query returns the value unchanged.

`json:` uses JSONPath instead of jq and always reads the last step's stdout;
`jsonfile:/path:$.query` reads a file:

```yaml
- expr: "${json:$.status} == 'ok'"
- expr: "${jsonfile:/workspace/report.json:$.failures} == 0"
```

The `jq:` prefix requires the `jq` binary on the machine executing the step.

## Routine Parameters

Parameters passed to a routine with `params:` are read as `${params.<key>}`:

```yaml
# global/routines.yaml
routines:
  create_user:
    steps:
      - name: Create
        handler: http
        method: POST
        url: http://localhost:8080/users
        headers:
          Content-Type: application/json
        body: '{"email": "${params.email}", "role": "${params.role}"}'
```

```yaml
# test.yaml
test:
  - routine: global.create_user
    params:
      email: test@example.com
      role: admin
```

## Files and Fixtures

```yaml
test:
  - name: Compare against a fixture
    handler: shell
    command: diff <(cat /workspace/out.json) <(echo '${fixture:expected/out.json}')

  - name: Show a produced file
    handler: shell
    command: echo "${file:/workspace/build.log}"
```

`fixture:` paths are relative to `<suite_path>/fixtures`.

## String Concatenation

Variables can be combined freely with surrounding text:

```yaml
test:
  - name: Save output under the run directory
    handler: shell
    command: meshctl list --json > ${run_path}/${uc_name}-${tc_name}.json
```

## See Also

- `tsuite man handlers` - Capturing values
- `tsuite man routines` - Routine parameters
- `tsuite man assertions` - Using variables in assertions
