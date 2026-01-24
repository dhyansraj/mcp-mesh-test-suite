# Artifacts

> Test artifacts and file mounting

## Overview

Artifacts are files used by tests. They can be defined at suite, UC,
or TC level and are automatically mounted into containers in Docker mode.

## Artifact Levels

| Level | Directory | Mount Path | Description |
|-------|-----------|------------|-------------|
| Suite | `artifacts/` (root) | `/suite-artifacts/` | Shared across all tests |
| UC | `artifacts/` (UC dir) | `/uc-artifacts/` | Shared within use case |
| TC | `artifacts/` (TC dir) | `/tc-artifacts/` | Specific to test case |

## Directory Structure

```
my-suite/
├── config.yaml
├── artifacts/                    # Suite-level
│   ├── global_config.json
│   └── shared_data.csv
└── suites/
    └── uc01_users/
        ├── artifacts/            # UC-level
        │   └── user_template.json
        └── tc01_create/
            ├── test.yaml
            └── artifacts/        # TC-level
                ├── request.json
                └── expected.json
```

## Using Artifacts

### In Docker Mode

Artifacts are mounted as read-only volumes:

```yaml
# test.yaml
test:
  - name: Load test data
    exec:
      command: cat /tc-artifacts/request.json

  - name: Use shared template
    exec:
      command: cat /uc-artifacts/user_template.json

  - name: Access global config
    exec:
      command: cat /suite-artifacts/global_config.json
```

### In Standalone Mode

Artifacts are accessed via absolute paths:

```yaml
test:
  - name: Load test data
    exec:
      command: cat ${TC_ARTIFACTS}/request.json
```

Environment variables:
- `${SUITE_ARTIFACTS}` - Path to suite artifacts
- `${UC_ARTIFACTS}` - Path to UC artifacts
- `${TC_ARTIFACTS}` - Path to TC artifacts

## Common Use Cases

### JSON Request Bodies

```
tc01_create/
├── test.yaml
└── artifacts/
    └── create_user.json
```

```json
// artifacts/create_user.json
{
  "name": "Test User",
  "email": "test@example.com"
}
```

```yaml
# test.yaml
test:
  - name: Create user
    http:
      method: POST
      url: ${API_URL}/users
      body_file: /tc-artifacts/create_user.json
```

### Expected Response Comparison

```yaml
test:
  - name: Get user
    http:
      method: GET
      url: ${API_URL}/users/123
    capture:
      response: response.json

  - name: Load expected
    exec:
      command: cat /tc-artifacts/expected.json
    capture:
      expected: result.stdout | json

assertions:
  - expr: captured.response
    equals: ${captured.expected}
```

### Shared Test Data

UC-level artifacts for data shared across related tests:

```
uc01_users/
├── artifacts/
│   └── test_users.csv
├── tc01_create/
├── tc02_update/
└── tc03_delete/
```

### Configuration Files

Suite-level artifacts for global configuration:

```yaml
# config.yaml
docker:
  env:
    CONFIG_PATH: /suite-artifacts/app_config.yaml
```

## Binary Files

Artifacts can be binary files (images, PDFs, etc.):

```yaml
test:
  - name: Upload image
    http:
      method: POST
      url: ${API_URL}/upload
      file: /tc-artifacts/test_image.png
```

## Generated Artifacts

Tests can write to a designated output directory:

```yaml
test:
  - name: Generate report
    exec:
      command: python generate_report.py --output /output/report.html
```

Output artifacts are preserved for later inspection.

## See Also

- `tsuite man suites` - Suite structure
- `tsuite man usecases` - Use case organization
- `tsuite man docker` - Docker mode details
