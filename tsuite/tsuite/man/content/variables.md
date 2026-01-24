# Variables

> Variable interpolation syntax

## Overview

Variables allow dynamic values in test definitions. Use `${variable_name}`
syntax for interpolation.

## Variable Sources

| Source | Syntax | Description |
|--------|--------|-------------|
| Config | `${config.key}` | From config.yaml |
| Environment | `${ENV_VAR}` | From environment |
| Captured | `${captured.name}` | From previous steps |
| Routine params | `${param}` | From routine `with:` |

## Config Variables

Access values from config.yaml:

```yaml
# config.yaml
suite:
  name: My Tests

docker:
  env:
    API_URL: http://localhost:8080
    DB_HOST: localhost

custom:
  timeout: 30
  retries: 3
```

```yaml
# test.yaml
test:
  - name: Call API
    http:
      url: ${config.docker.env.API_URL}/users
      timeout: ${config.custom.timeout}
```

## Environment Variables

Access host environment variables:

```yaml
test:
  - name: Use API key
    http:
      url: ${API_URL}/data
      headers:
        Authorization: Bearer ${API_KEY}
```

Environment variables are resolved at runtime from:
1. Host environment
2. `docker.env` in config.yaml
3. Step-level `env` settings

## Captured Variables

Access values captured from previous steps:

```yaml
test:
  - name: Login
    http:
      method: POST
      url: ${API_URL}/login
      json:
        username: admin
        password: secret
    capture:
      token: response.json.access_token
      user_id: response.json.user.id

  - name: Get profile
    http:
      url: ${API_URL}/users/${captured.user_id}
      headers:
        Authorization: Bearer ${captured.token}
```

### Nested Access

```yaml
capture:
  user: response.json.user

# Later...
- name: Check user
  log:
    message: "User name: ${captured.user.name}"
```

### Array Access

```yaml
capture:
  items: response.json.items

# Access by index
- name: First item
  log:
    message: "First: ${captured.items[0].name}"
```

## Routine Parameters

Parameters passed to routines:

```yaml
# routines.yaml
routines:
  create_user:
    - name: Create
      http:
        method: POST
        url: ${API_URL}/users
        json:
          email: ${email}      # From with.email
          role: ${role}        # From with.role
```

```yaml
# test.yaml
test:
  - routine: suite.create_user
    with:
      email: test@example.com
      role: admin
```

## Built-in Variables

| Variable | Description |
|----------|-------------|
| `${TEST_ID}` | Current test ID (uc/tc) |
| `${RUN_ID}` | Current run ID |
| `${TC_ARTIFACTS}` | TC artifacts path |
| `${UC_ARTIFACTS}` | UC artifacts path |
| `${SUITE_ARTIFACTS}` | Suite artifacts path |

## Default Values

Provide fallback for undefined variables:

```yaml
test:
  - name: Call API
    http:
      url: ${API_URL:-http://localhost:8080}/users
      timeout: ${TIMEOUT:-30}
```

## Escaping

To use literal `${...}`, escape with double dollar:

```yaml
test:
  - name: Echo literal
    exec:
      command: echo '$${NOT_A_VARIABLE}'
```

## String Concatenation

Variables can be combined with text:

```yaml
test:
  - name: Build URL
    http:
      url: ${API_URL}/users/${captured.user_id}/profile
      headers:
        X-Request-ID: req-${RUN_ID}-${TEST_ID}
```

## Conditional Values

Use ternary-like syntax:

```yaml
test:
  - name: Call endpoint
    http:
      url: ${API_URL}/${USE_V2:+v2/}users
```

## See Also

- `tsuite man handlers` - Capturing values
- `tsuite man routines` - Routine parameters
- `tsuite man assertions` - Using variables in assertions
