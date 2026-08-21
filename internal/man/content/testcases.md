# Test Cases

> Test case structure and test.yaml

## Overview

A test case (TC) is the smallest unit of testing. Each TC has its own
directory with a `test.yaml` file.

## Directory Structure

```
tc01_valid_login/
├── test.yaml          # Test definition (required)
└── artifacts/         # Test-specific files (optional)
    ├── request.json
    └── expected.json
```

## test.yaml Structure

```yaml
name: Valid Login Test
description: Verify user can login with correct credentials
tags:
  - auth
  - smoke

# Setup steps (run before test)
pre_run:
  - name: Start mock server
    handler: shell
    command: python mock_server.py &

# Main test steps
test:
  - name: Login request
    handler: http
    method: POST
    url: http://localhost:8080/login
    headers:
      Content-Type: application/json
    body: '{"username": "testuser", "password": "secret"}'
    capture: login_response

  - name: Access protected resource
    handler: http
    method: GET
    url: http://localhost:8080/profile
    headers:
      Authorization: Bearer ${jq:captured.login_response:.access_token}
    capture: profile

# Cleanup steps (always run, even on failure)
post_run:
  - name: Stop mock server
    handler: shell
    command: pkill -f mock_server.py

# Assertions (evaluated after test steps)
assertions:
  - expr: "${steps.login_response.exit_code} == 0"
  - expr: "${jq:captured.login_response:.access_token} exists"
  - expr: "${jq:captured.profile:.username} == 'testuser'"
```

Each step names exactly one `handler:`; the remaining keys configure it.
`capture: <name>` is a plain string that stores the step's stdout as
`${captured.<name>}` (and the full result as `${steps.<name>.exit_code}`,
`${steps.<name>.stdout}`, `${steps.<name>.stderr}`, `${steps.<name>.success}`).

## Test Phases

### pre_run
Setup steps that run before the main test. If any step fails,
the test is skipped.

### test
Main test steps. Failures here mark the test as failed.

### post_run
Cleanup steps that always run, regardless of test outcome.
Failures are logged but don't affect test status.

### assertions
Evaluated after all test steps complete. All must pass for
the test to pass.

## Tags

Use tags to categorize tests for discovery and display:

```yaml
tags:
  - smoke
  - regression
  - slow
```

Tags are surfaced in the dashboard and can be used to filter the test list
via the discovery API (`?tag=smoke`).

## Run Filters

Select which tests to run with `--uc` (use case) or `--tc` (test case). Both
accept multiple comma-separated values:

```bash
# Run specific test cases
tsuite run --suite-path ./my-suite --tc tc01_agent_registration,tc03_heartbeat

# Run an entire use case
tsuite run --suite-path ./my-suite --uc uc01_registry
```

`--uc` and `--tc` are mutually exclusive; use only one per run.

## Timeout

Set per-test timeout:

```yaml
name: Long Running Test
timeout: 300  # 5 minutes
```

## Disable Tests

Skip a test case without deleting it:

```yaml
name: Work In Progress Test
disabled: true
```

A use case can be disabled the same way with `disabled: true` in its `uc.yaml`.

## See Also

- `tsuite man handlers` - Available handlers
- `tsuite man assertions` - Assertion syntax
- `tsuite man variables` - Variable interpolation
