# Handlers

> Built-in handlers for test steps

## Overview

Handlers define what action a test step performs. Each step uses exactly
one handler.

## Available Handlers

| Handler | Description |
|---------|-------------|
| `http`  | Make HTTP requests |
| `exec`  | Run shell commands |
| `mesh`  | Call MCP Mesh capabilities |
| `sleep` | Wait for a duration |
| `log`   | Log a message |

## HTTP Handler

Make HTTP requests and capture responses.

```yaml
- name: Create user
  http:
    method: POST
    url: ${API_URL}/users
    headers:
      Content-Type: application/json
      Authorization: Bearer ${TOKEN}
    json:
      name: John Doe
      email: john@example.com
    timeout: 30
  capture:
    user_id: response.json.id
    status: response.status_code
    headers: response.headers
```

### HTTP Options

| Option | Description |
|--------|-------------|
| `method` | HTTP method (GET, POST, PUT, DELETE, PATCH) |
| `url` | Request URL |
| `headers` | Request headers (dict) |
| `json` | JSON body (auto-sets Content-Type) |
| `body` | Raw body string |
| `params` | Query parameters (dict) |
| `timeout` | Request timeout in seconds |

### HTTP Response Capture

| Path | Description |
|------|-------------|
| `response.status_code` | HTTP status code |
| `response.json` | Parsed JSON body |
| `response.json.field` | Specific JSON field |
| `response.text` | Raw response text |
| `response.headers` | Response headers |
| `response.headers.X-Custom` | Specific header |

## Exec Handler

Run shell commands.

```yaml
- name: Run database migration
  exec:
    command: python manage.py migrate
    cwd: /app
    env:
      DATABASE_URL: ${DB_URL}
    timeout: 60
  capture:
    output: result.stdout
    errors: result.stderr
    code: result.exit_code
```

### Exec Options

| Option | Description |
|--------|-------------|
| `command` | Command to run |
| `cwd` | Working directory |
| `env` | Environment variables (dict) |
| `timeout` | Command timeout in seconds |
| `shell` | Use shell execution (default: true) |

### Exec Result Capture

| Path | Description |
|------|-------------|
| `result.stdout` | Standard output |
| `result.stderr` | Standard error |
| `result.exit_code` | Exit code (0 = success) |

## Mesh Handler

Call MCP Mesh capabilities.

```yaml
- name: Call greeting service
  mesh:
    capability: greeting
    tool: greet
    args:
      name: World
  capture:
    greeting: response.result
```

### Mesh Options

| Option | Description |
|--------|-------------|
| `capability` | Capability name to call |
| `tool` | Tool name within capability |
| `args` | Arguments to pass (dict) |
| `tags` | Tag selector (e.g., "+prod -dev") |
| `timeout` | Call timeout in seconds |

## Sleep Handler

Wait for a specified duration.

```yaml
- name: Wait for service startup
  sleep:
    seconds: 5
```

## Log Handler

Log a message (useful for debugging).

```yaml
- name: Debug info
  log:
    message: "User ID is: ${captured.user_id}"
    level: info  # debug, info, warn, error
```

## See Also

- `tsuite man assertions` - Validating results
- `tsuite man variables` - Variable interpolation
- `tsuite man routines` - Reusing step sequences
