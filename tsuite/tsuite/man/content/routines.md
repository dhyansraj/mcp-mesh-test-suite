# Routines

> Reusable test step sequences

## Overview

Routines are reusable sequences of test steps. Define once, use in
multiple test cases.

## Routine Scopes

| Scope | File Location | Prefix | Description |
|-------|---------------|--------|-------------|
| Suite | `routines.yaml` (root) | `suite.` | Available to all tests |
| UC | `routines.yaml` (UC dir) | `uc.` | Available to tests in that UC |

## Defining Routines

### Suite-Level Routines

```yaml
# my-suite/routines.yaml
routines:
  login:
    - name: Get auth token
      http:
        method: POST
        url: ${API_URL}/auth/login
        json:
          username: ${username}
          password: ${password}
      capture:
        token: response.json.access_token

  create_resource:
    - name: Create via API
      http:
        method: POST
        url: ${API_URL}/resources
        headers:
          Authorization: Bearer ${token}
        json:
          name: ${name}
          type: ${type}
      capture:
        resource_id: response.json.id
```

### UC-Level Routines

```yaml
# my-suite/suites/uc01_users/routines.yaml
routines:
  create_user:
    - name: Register user
      http:
        method: POST
        url: ${API_URL}/users
        json:
          email: ${email}
          name: ${name}
      capture:
        user_id: response.json.id
```

## Using Routines

### Basic Usage

```yaml
# test.yaml
test:
  - routine: suite.login
    with:
      username: admin
      password: secret123

  - routine: uc.create_user
    with:
      email: test@example.com
      name: Test User
```

### Using Captured Values

Routines can capture values that subsequent steps can use:

```yaml
test:
  - routine: suite.login
    with:
      username: admin
      password: secret

  # captured.token is now available
  - name: Use token
    http:
      method: GET
      url: ${API_URL}/profile
      headers:
        Authorization: Bearer ${captured.token}
```

### Chaining Routines

```yaml
test:
  - routine: suite.login
    with:
      username: admin
      password: secret

  - routine: suite.create_resource
    with:
      token: ${captured.token}
      name: My Resource
      type: document

  - name: Verify resource
    http:
      method: GET
      url: ${API_URL}/resources/${captured.resource_id}
```

## Routine Parameters

Parameters are passed via `with:` and available as variables:

```yaml
# routines.yaml
routines:
  send_notification:
    - name: Send email
      http:
        method: POST
        url: ${API_URL}/notifications
        json:
          to: ${recipient}        # From with.recipient
          subject: ${subject}     # From with.subject
          body: ${body}           # From with.body
```

```yaml
# test.yaml
test:
  - routine: suite.send_notification
    with:
      recipient: user@example.com
      subject: Test Subject
      body: Hello, this is a test
```

## See Also

- `tsuite man testcases` - Test case structure
- `tsuite man handlers` - Available handlers
- `tsuite man variables` - Variable interpolation
