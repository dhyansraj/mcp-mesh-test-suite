# Assertions

> Assertion syntax and expressions

## Overview

Assertions validate test results after all steps complete.
All assertions must pass for the test to pass.

## Basic Syntax

```yaml
assertions:
  - expr: captured.status_code
    equals: 200

  - expr: captured.response.name
    equals: "John Doe"

  - expr: captured.items
    is_not_empty: true
```

## Assertion Operators

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

## Expression Syntax

### Accessing Captured Values

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
```

### Filters

```yaml
# Length filter
- expr: captured.items | length
  greater_than: 0

# JSON path (for complex queries)
- expr: captured.data | jsonpath('$.users[*].active')
  all_equal: true
```

## Custom Messages

Add descriptive messages for failures:

```yaml
assertions:
  - expr: captured.status_code
    equals: 200
    message: "API should return 200 OK"

  - expr: captured.user.email
    contains: "@"
    message: "User email should be valid"
```

## Conditional Assertions

Skip assertions based on conditions:

```yaml
assertions:
  - expr: captured.premium_features
    is_not_empty: true
    when: ${USER_TYPE} == "premium"
```

## Examples

### API Response Validation

```yaml
assertions:
  - expr: captured.status
    equals: 200
    message: "Should return 200"

  - expr: captured.body.success
    equals: true

  - expr: captured.body.data.id
    is_not_null: true
    message: "Response should include ID"

  - expr: captured.body.data.created_at
    matches: "^\\d{4}-\\d{2}-\\d{2}"
    message: "Created date should be ISO format"
```

### Command Output Validation

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
