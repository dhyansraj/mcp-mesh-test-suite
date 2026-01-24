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
    exec:
      command: python mock_server.py &

# Main test steps
test:
  - name: Login request
    http:
      method: POST
      url: ${API_URL}/login
      json:
        username: testuser
        password: secret
    capture:
      token: response.json.access_token
      status: response.status_code

  - name: Access protected resource
    http:
      method: GET
      url: ${API_URL}/profile
      headers:
        Authorization: Bearer ${captured.token}
    capture:
      profile: response.json

# Cleanup steps (always run, even on failure)
post_run:
  - name: Stop mock server
    exec:
      command: pkill -f mock_server.py

# Assertions (evaluated after test steps)
assertions:
  - expr: captured.status
    equals: 200
  - expr: captured.token
    is_not_empty: true
  - expr: captured.profile.username
    equals: testuser
```

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

Use tags to categorize and filter tests:

```yaml
tags:
  - smoke
  - regression
  - slow
```

Run by tag:

```bash
tsuite run --suite-path ./my-suite --tag smoke
tsuite run --suite-path ./my-suite --skip-tag slow
```

## Timeout

Set per-test timeout:

```yaml
name: Long Running Test
timeout: 300  # 5 minutes
```

## Skip Tests

Conditionally skip tests:

```yaml
name: Linux-only Test
skip:
  reason: Only runs on Linux
  when: ${OS} != "linux"
```

## See Also

- `tsuite man handlers` - Available handlers
- `tsuite man assertions` - Assertion syntax
- `tsuite man variables` - Variable interpolation
