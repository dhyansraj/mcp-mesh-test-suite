# Assertions

> Assertion syntax and expressions

## Overview

Assertions validate test results after all steps complete.
All assertions must pass for the test to pass.

There are two assertion styles: **inline expressions** (most common) and **field-based assertions** (structured YAML fields).

## Inline Expression Assertions

Inline assertions use the syntax `${variable} operator value` as a single string expression.

```yaml
assertions:
  - expr: "${captured.status_code} == 200"
  - expr: "${captured.output} contains 'success'"
  - expr: "${captured.token} exists"
```

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
- expr: "${captured.user_id} == '${env.EXPECTED_USER}'"
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

### Inline Expression Examples

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

## Field-Based Assertions

Field-based assertions use separate YAML fields for the expression and operator.

```yaml
assertions:
  - expr: captured.status_code
    equals: 200

  - expr: captured.response.name
    equals: "John Doe"

  - expr: captured.items
    is_not_empty: true
```

### Equality

```yaml
# Exact match
- expr: captured.value
  equals: 42

# Not equal
- expr: captured.status
  not_equals: "error"
```

### Comparison

```yaml
# Greater than
- expr: captured.count
  greater_than: 0

# Less than
- expr: captured.latency_ms
  less_than: 1000

# Greater than or equal
- expr: captured.items | length
  gte: 5

# Less than or equal
- expr: captured.retry_count
  lte: 3
```

### String Matching

```yaml
# Contains substring
- expr: captured.message
  contains: "success"

# Starts with
- expr: captured.id
  starts_with: "user_"

# Ends with
- expr: captured.filename
  ends_with: ".json"

# Regex match
- expr: captured.email
  matches: "^[a-z]+@example\\.com$"
```

### Type and Existence

```yaml
# Not empty (strings, lists, dicts)
- expr: captured.items
  is_not_empty: true

# Is empty
- expr: captured.errors
  is_empty: true

# Is null
- expr: captured.optional_field
  is_null: true

# Is not null
- expr: captured.required_field
  is_not_null: true

# Type check
- expr: captured.count
  is_type: int

- expr: captured.items
  is_type: list
```

### List Assertions

```yaml
# Length check
- expr: captured.items | length
  equals: 5

# Contains element
- expr: captured.tags
  contains: "important"

# All items match
- expr: captured.statuses
  all_equal: "active"
```

### Expression Syntax

```yaml
# Direct access
- expr: captured.user_id
  is_not_null: true

# Nested access
- expr: captured.response.data.items[0].name
  equals: "First Item"

# Array indexing
- expr: captured.list[0]
  equals: "first"

- expr: captured.list[-1]
  equals: "last"

# Length filter
- expr: captured.items | length
  greater_than: 0

# JSON path (for complex queries)
- expr: captured.data | jsonpath('$.users[*].active')
  all_equal: true
```

## Custom Messages

Add descriptive messages for assertion failures:

```yaml
assertions:
  # Inline style
  - expr: "${captured.status_code} == 200"
    message: "API should return 200 OK"

  # Field-based style
  - expr: captured.user.email
    contains: "@"
    message: "User email should be valid"
```

## Examples

### API Response Validation (Inline Style)

```yaml
assertions:
  - expr: "${captured.status} == 200"
    message: "Should return 200"

  - expr: "${captured.body.success} == true"

  - expr: "${captured.body.data.id} exists"
    message: "Response should include ID"

  - expr: "${captured.body.data.created_at} matches '^\\d{4}-\\d{2}-\\d{2}'"
    message: "Created date should be ISO format"

  - expr: "${captured.body.errors} not exists"
```

### Command Output Validation (Field-Based Style)

```yaml
assertions:
  - expr: captured.exit_code
    equals: 0
    message: "Command should succeed"

  - expr: captured.stdout
    contains: "Operation completed"

  - expr: captured.stderr
    is_empty: true
    message: "No errors expected"
```

## See Also

- `tsuite man testcases` - Test case structure
- `tsuite man variables` - Variable interpolation
- `tsuite man handlers` - Capturing values
