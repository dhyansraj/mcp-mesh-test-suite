# Use Cases

> Organizing tests into use cases

## Overview

Use cases (UCs) group related test cases together. They represent a feature
or functional area being tested.

## Naming Convention

```
suites/
├── uc01_user_registration/
├── uc02_authentication/
├── uc03_api_endpoints/
└── uc04_data_export/
```

Prefix with `uc##_` for ordering. The name after the prefix describes the feature.

## Use Case Structure

```
uc01_user_registration/
├── routines.yaml      # UC-level reusable routines (optional)
├── artifacts/         # Shared files for all TCs in this UC (optional)
├── tc01_valid_email/
│   └── test.yaml
├── tc02_invalid_email/
│   └── test.yaml
└── tc03_duplicate_user/
    └── test.yaml
```

## UC-Level Routines

Define routines shared across test cases in the UC:

```yaml
# uc01_user_registration/routines.yaml
routines:
  create_user:
    steps:
      - name: Register user via API
        handler: http
        method: POST
        url: http://localhost:8080/users
        headers:
          Content-Type: application/json
        body: '{"email": "${params.email}", "password": "${params.password}"}'
        capture: user_response
```

Use in test cases (UC routines are referenced by their bare name):

```yaml
# tc01_valid_email/test.yaml
test:
  - routine: create_user
    params:
      email: test@example.com
      password: secret123
```

## UC-Level Artifacts

Files in `artifacts/` are available to all test cases:

```
uc01_user_registration/
├── artifacts/
│   ├── sample_users.json
│   └── test_data.csv
└── tc01_valid_email/
    └── test.yaml
```

In Docker and K8s modes these are mounted at `/uc-artifacts/`:

```yaml
test:
  - name: Load test data
    handler: shell
    command: cat /uc-artifacts/sample_users.json
```

## Running Use Cases

```bash
# Run all tests in a use case
tsuite run --suite-path ./my-suite --uc uc01_user_registration

# Run multiple use cases
tsuite run --suite-path ./my-suite --uc uc01_user_registration --uc uc02_authentication
```

## See Also

- `tsuite man testcases` - Test case structure
- `tsuite man routines` - Reusable routines
- `tsuite man artifacts` - Test artifacts
