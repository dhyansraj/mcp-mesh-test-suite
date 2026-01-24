# API & Dashboard

> REST API server and web dashboard

## Overview

tsuite includes a built-in API server and web dashboard for:
- Viewing test results
- Monitoring live test execution
- Managing test suites
- Browsing test history

## Starting the Server

```bash
# Start on default port (9999)
tsuite api

# Start on custom port
tsuite api --port 8080

# Start with suites pre-loaded
tsuite api --suites ./my-suite,./other-suite
```

## Web Dashboard

Access the dashboard at `http://localhost:9999`

### Dashboard Pages

| Page | URL | Description |
|------|-----|-------------|
| Home | `/` | Overview and recent runs |
| Live | `/live` | Real-time test execution |
| Runs | `/runs` | Test run history |
| Settings | `/settings` | Suite management |

### Live View

The live page shows real-time test execution:
- Test progress as tests run
- Pass/fail status updates
- Duration tracking
- Expandable test details

### Run History

Browse past test runs:
- Filter by status (passed, failed, running)
- View detailed results
- Compare runs
- Rerun tests

## REST API

### Runs

```bash
# List runs
GET /api/runs?limit=20&offset=0

# Get run details
GET /api/runs/{run_id}

# Get run tests
GET /api/runs/{run_id}/tests

# Get test tree (grouped by UC)
GET /api/runs/{run_id}/tests/tree
```

### Suites

```bash
# List suites
GET /api/suites

# Register suite
POST /api/suites
{"folder_path": "/path/to/suite"}

# Get suite details
GET /api/suites/{suite_id}

# Run suite
POST /api/suites/{suite_id}/run
{"uc": "uc01_feature", "tc": null}
```

### Statistics

```bash
# Overall stats
GET /api/stats

# Flaky tests
GET /api/stats/flaky

# Slowest tests
GET /api/stats/slowest
```

### Server-Sent Events

Real-time updates via SSE:

```bash
# Global events (all runs)
GET /api/events

# Run-specific events
GET /api/runs/{run_id}/stream
```

Event types:
- `run_started`
- `test_started`
- `test_completed`
- `run_completed`

## Running Tests via API

### Start a Test Run

```bash
# Run all tests in suite
curl -X POST http://localhost:9999/api/suites/1/run

# Run specific use case
curl -X POST http://localhost:9999/api/suites/1/run \
  -H "Content-Type: application/json" \
  -d '{"uc": "uc01_feature"}'

# Run specific test case
curl -X POST http://localhost:9999/api/suites/1/run \
  -H "Content-Type: application/json" \
  -d '{"tc": "uc01_feature/tc01_test"}'
```

### Monitor Progress

```bash
# Watch live events
curl -N http://localhost:9999/api/events
```

### Cancel a Run

```bash
curl -X POST http://localhost:9999/api/runs/{run_id}/cancel
```

## Configuration

### CORS

The API server allows cross-origin requests by default for dashboard access.

### Database

Test results are stored in SQLite:
- Default: `~/.tsuite/tsuite.db`
- Override: `TSUITE_DB_PATH` environment variable

### Clearing Data

```bash
# Clear all test data
tsuite clear

# Clear specific run
tsuite clear --run-id {run_id}
```

## Integration

### CI/CD

```yaml
# GitHub Actions example
- name: Run tests
  run: |
    tsuite api --port 9999 &
    sleep 2
    tsuite run --suite-path ./tests --all --api-url http://localhost:9999
```

### Custom Dashboard

Build custom dashboards using the REST API:

```javascript
// Fetch recent runs
const response = await fetch('http://localhost:9999/api/runs');
const { runs } = await response.json();

// Subscribe to events
const events = new EventSource('http://localhost:9999/api/events');
events.onmessage = (e) => {
  const data = JSON.parse(e.data);
  console.log('Event:', data.type);
};
```

## See Also

- `tsuite man quickstart` - Getting started
- `tsuite man docker` - Docker mode
- `tsuite man suites` - Suite configuration
