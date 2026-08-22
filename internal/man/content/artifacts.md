# Artifacts

> Test artifacts and file mounting

## Overview

Artifacts are files used by tests. They can be defined at UC or TC level and
are automatically mounted into the container in Docker and K8s modes.

## Artifact Levels

| Level | Directory | Mount Path | Host Path Variable |
|-------|-----------|------------|--------------------|
| UC | `artifacts/` (UC dir) | `/uc-artifacts/` | `${uc_artifacts}` |
| TC | `artifacts/` (TC dir) | `/artifacts/` | `${artifacts}` |

## Directory Structure

```
my-suite/
├── config.yaml
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

### In Docker and K8s Modes

Each entry inside an `artifacts/` directory is mounted individually (symlinks
are resolved), TC artifacts under `/artifacts/` and UC artifacts under
`/uc-artifacts/`:

```yaml
# test.yaml
test:
  - name: Load test data
    handler: shell
    command: cat /artifacts/request.json

  - name: Use shared template
    handler: shell
    command: cat /uc-artifacts/user_template.json

  - name: Copy agents into the workspace
    handler: shell
    command: cp -r /artifacts/my-agent /workspace/
```

### In Standalone Mode

There is no mount, so use the host paths:

```yaml
test:
  - name: Load test data
    handler: shell
    command: cat ${artifacts}/request.json

  - name: Use shared template
    handler: shell
    command: cat ${uc_artifacts}/user_template.json
```

Path variables (`${artifacts_path}` and `${uc_artifacts_path}` are aliases):
- `${artifacts}` - Path to this test case's artifacts
- `${uc_artifacts}` - Path to the use case's artifacts

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
    handler: http
    method: POST
    url: http://localhost:8080/users
    headers:
      Content-Type: application/json
    body: ${file:/artifacts/create_user.json}
```

### Expected Response Comparison

```yaml
test:
  - name: Get user
    handler: http
    method: GET
    url: http://localhost:8080/users/123
    capture: response

  - name: Load expected
    handler: shell
    command: cat /artifacts/expected.json
    capture: expected

assertions:
  - expr: "${jq:captured.response:.name} == '${jq:captured.expected:.name}'"
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

UC-level artifacts for configuration shared by every test case in the UC:

```yaml
test:
  - name: Stage app config
    handler: shell
    command: cp /uc-artifacts/app_config.yaml /workspace/config.yaml
```

## Binary Files

Artifacts can be binary files (images, PDFs, etc.). Reference them by their
mount path from a shell step:

```yaml
test:
  - name: Upload image
    handler: shell
    command: curl -sf -F file=@/artifacts/test_image.png http://localhost:8080/upload
```

## Generated Artifacts

Everything a test writes under `/workspace` lives in a per-test working
directory that is discarded after the run. To keep output, write it to the
mounted log directory, which maps to `~/.tsuite/runs/<run-id>/<uc>/<tc>/` on
the host:

```yaml
test:
  - name: Generate report
    handler: shell
    command: python generate_report.py --output /var/log/tsuite/report.html
```

In standalone mode use `${run_path}` for the same purpose.

## See Also

- `tsuite man suites` - Suite structure
- `tsuite man usecases` - Use case organization
- `tsuite man docker` - Docker mode details
