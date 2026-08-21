# Routines

> Reusable test step sequences

## Overview

Routines are reusable sequences of test steps. Define once, use in
multiple test cases.

## Routine Scopes

| Scope | File Location | Reference | Description |
|-------|---------------|-----------|-------------|
| Global | `global/routines.yaml` | `global.<name>` | Available to all tests |
| UC | `routines.yaml` (UC dir) | `<name>` (bare) | Available to tests in that UC |

A bare name is looked up in the use case's own `routines.yaml` first, then falls
back to the global routines. Use the `global.` prefix to force the global one.

## Defining Routines

Each routine is a map with a `steps:` list (and optional `name`/`description`).

### Global Routines

```yaml
# my-suite/global/routines.yaml
routines:
  login:
    description: Fetch an auth token
    steps:
      - name: Get auth token
        handler: http
        method: POST
        url: http://localhost:8080/auth/login
        headers:
          Content-Type: application/json
        body: '{"username": "${params.username}", "password": "${params.password}"}'
        capture: login_response

  create_resource:
    steps:
      - name: Create via API
        handler: http
        method: POST
        url: http://localhost:8080/resources
        headers:
          Content-Type: application/json
          Authorization: Bearer ${params.token}
        body: '{"name": "${params.name}", "type": "${params.type}"}'
        capture: create_response
```

### UC-Level Routines

```yaml
# my-suite/suites/uc01_users/routines.yaml
routines:
  create_user:
    steps:
      - name: Register user
        handler: http
        method: POST
        url: http://localhost:8080/users
        headers:
          Content-Type: application/json
        body: '{"email": "${params.email}", "name": "${params.name}"}'
        capture: user_response
```

## Using Routines

### Basic Usage

```yaml
# test.yaml
test:
  - routine: global.login
    params:
      username: admin
      password: secret123

  - routine: create_user      # bare name: UC routine, else global
    params:
      email: test@example.com
      name: Test User
```

### Using Captured Values

Values captured inside a routine are copied back into the calling test, so
later steps can use them:

```yaml
test:
  - routine: global.login
    params:
      username: admin
      password: secret

  # captured.login_response is now available
  - name: Use token
    handler: http
    method: GET
    url: http://localhost:8080/profile
    headers:
      Authorization: Bearer ${jq:captured.login_response:.access_token}
```

### Chaining Routines

```yaml
test:
  - routine: global.login
    params:
      username: admin
      password: secret

  - routine: global.create_resource
    params:
      token: ${jq:captured.login_response:.access_token}
      name: My Resource
      type: document

  - name: Verify resource
    handler: http
    method: GET
    url: http://localhost:8080/resources/${jq:captured.create_response:.id}
```

## Routine Parameters

Parameters are passed via `params:` and read inside the routine as
`${params.<key>}`:

```yaml
# global/routines.yaml
routines:
  send_notification:
    steps:
      - name: Send email
        handler: http
        method: POST
        url: http://localhost:8080/notifications
        headers:
          Content-Type: application/json
        body: '{"to": "${params.recipient}", "subject": "${params.subject}", "text": "${params.body}"}'
```

```yaml
# test.yaml
test:
  - routine: global.send_notification
    params:
      recipient: user@example.com
      subject: Test Subject
      body: Hello, this is a test
```

Parameter values are interpolated in the caller's context before the routine
runs, so they may themselves reference `${captured.*}`, `${config.*}`, and so on.

## See Also

- `tsuite man testcases` - Test case structure
- `tsuite man handlers` - Available handlers
- `tsuite man variables` - Variable interpolation
