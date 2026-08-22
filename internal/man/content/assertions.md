# Assertions

> Assertion syntax and expressions

## Overview

Assertions validate test results after all steps complete.
All assertions must pass for the test to pass.

An assertion has exactly two fields: `expr` (required) and `message`
(optional). The operator lives **inside** the expression string; there are no
`equals:`, `contains:`, or `is_not_empty:` sub-keys.

## Expression Syntax

An expression is `${variable} operator value` as a single string.

```yaml
assertions:
  - expr: "${captured.status_code} == 200"
  - expr: "${captured.output} contains 'success'"
  - expr: "${captured.token} exists"
    message: "Login should return a token"
```

An expression that does not match this shape fails with
`Invalid expression syntax`.

### Operator Reference

| Operator | Description | Example |
|----------|-------------|---------|
| `==` | Equals (numeric-aware) | `${captured.code} == 200` |
| `!=` | Not equals | `${captured.status} != 'error'` |
| `>` | Greater than | `${captured.count} > 0` |
| `<` | Less than | `${captured.latency} < 1000` |
| `>=` | Greater or equal | `${captured.items} >= 5` |
| `<=` | Less or equal | `${captured.retries} <= 3` |
| `contains` | Contains substring | `${captured.output} contains 'success'` |
| `not contains` | Does not contain substring | `${captured.stderr} not contains 'error'` |
| `icontains` | Case-insensitive contains | `${captured.msg} icontains 'OK'` |
| `matches` | Regex match | `${captured.email} matches '^[a-z]+@'` |
| `exists` | Variable exists (no value needed) | `${captured.token} exists` |
| `not exists` | Variable does not exist | `${captured.error} not exists` |
| `is` | Type check | `${captured.count} is number` |
| `length` | Length comparison | `${captured.items} length > 5` |
| `iequal` / `ieq` | Case-insensitive equals | `${captured.status} ieq 'OK'` |
| `startswith` | Starts with prefix | `${captured.id} startswith 'usr_'` |
| `endswith` | Ends with suffix | `${captured.file} endswith '.json'` |

### Quoting Values

Values can be quoted with single or double quotes. Quotes are optional for single words without spaces.

```yaml
- expr: "${captured.name} == 'John Doe'"    # single quotes (required, has space)
- expr: '${captured.name} == "John Doe"'    # double quotes
- expr: "${captured.code} == 200"            # no quotes needed for numbers
- expr: "${captured.status} == active"       # no quotes needed for single words
```

### Variable Interpolation in Expected Values

The expected value can reference other variables:

```yaml
- expr: "${captured.output} contains '${config.packages.mesh_version}'"
- expr: "${captured.user_id} == '${env:EXPECTED_USER}'"
```

### Type Aliases for `is`

| Canonical Type | Accepted Aliases |
|---------------|------------------|
| `string` | `string`, `str` |
| `number` | `number`, `int`, `integer`, `float` |
| `bool` | `bool`, `boolean` |
| `array` | `array`, `list`, `slice` |
| `object` | `object`, `dict`, `map` |
| `null` | `null`, `none`, `nil` |

### Length Operator

The `length` operator takes a sub-expression with a comparison operator and a number:

```yaml
- expr: "${captured.items} length > 5"
- expr: "${captured.name} length == 10"
- expr: "${captured.tags} length >= 1"
- expr: "${captured.errors} length == 0"
```

Supported sub-operators: `>`, `<`, `>=`, `<=`, `==`, `!=`.

### More Examples

```yaml
assertions:
  # Existence checks
  - expr: "${captured.token} exists"
  - expr: "${captured.error} not exists"

  # Negated contains
  - expr: "${captured.stderr} not contains 'FATAL'"

  # Case-insensitive matching
  - expr: "${captured.status} icontains 'ok'"
  - expr: "${captured.env} ieq 'Production'"

  # Prefix / suffix
  - expr: "${captured.request_id} startswith 'req_'"
  - expr: "${captured.filename} endswith '.tar.gz'"

  # Type and length
  - expr: "${captured.count} is number"
  - expr: "${captured.tags} length >= 1"
```

## Variables in Assertions

Any variable form works on the left-hand side, not just `captured.`:

```yaml
assertions:
  # Previous step result
  - expr: "${last.exit_code} == 0"
  - expr: "${exit_code} == 0"

  # Full result of a named step
  - expr: "${steps.build.exit_code} == 0"

  # jq query over a captured JSON value
  - expr: "${jq:captured.agent_list:.agents | length} > 0"
  - expr: "${jq:captured.agent_list:.agents[0].status} == 'running'"

  # File contents
  - expr: "${file:/workspace/agent/main.py} exists"
```

Captured values are plain strings (the step's stdout), so use `jq:`, `json:`,
or `jsonfile:` to reach into JSON instead of dotted access such as
`${captured.response.name}`.

See `tsuite man variables` for the full list of prefixes.

## Custom Messages

Add a descriptive `message` shown when the assertion fails:

```yaml
assertions:
  - expr: "${captured.status_code} == 200"
    message: "API should return 200 OK"

  - expr: "${captured.user_email} contains '@'"
    message: "User email should be valid"
```

## Examples

### API Response Validation

```yaml
test:
  - name: Get user
    handler: http
    method: GET
    url: http://localhost:8080/users/123
    capture: body

assertions:
  - expr: "${steps.body.exit_code} == 0"
    message: "Should return a 2xx/3xx status"

  - expr: "${jq:captured.body:.success} == true"

  - expr: "${jq:captured.body:.data.id} exists"
    message: "Response should include ID"

  - expr: "${jq:captured.body:.data.created_at} matches '^\\d{4}-\\d{2}-\\d{2}'"
    message: "Created date should be ISO format"
```

### Command Output Validation

```yaml
test:
  - name: Run the tool
    handler: shell
    command: my-tool --run
    capture: tool_output

assertions:
  - expr: "${steps.tool_output.exit_code} == 0"
    message: "Command should succeed"

  - expr: "${captured.tool_output} contains 'Operation completed'"

  - expr: "${steps.tool_output.stderr} not contains 'ERROR'"
    message: "No errors expected"
```

## See Also

- `tsuite man testcases` - Test case structure
- `tsuite man variables` - Variable interpolation
- `tsuite man handlers` - Capturing values
