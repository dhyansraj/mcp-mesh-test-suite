# Scaffold

> Generate test cases from agent directories

## Overview

The `scaffold` command auto-generates test cases by copying agent source
directories to a test suite and creating a ready-to-use `test.yaml`.

## Basic Usage

```bash
# Interactive mode (prompts for UC, TC, artifact level)
tsuite scaffold --suite ./my-suite ./path/to/agent1 ./path/to/agent2

# Non-interactive mode
tsuite scaffold --suite ./my-suite --uc uc01_tags --tc tc01_test ./agent1

# Preview without creating files
tsuite scaffold --suite ./my-suite --uc uc01_tags --tc tc01_test --dry-run ./agent1
```

## Options

| Option | Description |
|--------|-------------|
| `--suite PATH` | Path to test suite (required) |
| `--uc NAME` | Use case name (e.g., `uc01_tags`) |
| `--tc NAME` | Test case name (e.g., `tc01_test`) |
| `--artifact-level` | Where to copy artifacts: `tc` (default) or `uc` |
| `--name TEXT` | Test name (default: derived from TC name) |
| `--dry-run` | Preview without creating files |
| `--force` | Overwrite existing test case |
| `--no-interactive` | Skip prompts, use defaults |

## Agent Detection

Scaffold automatically detects agent type:

| Indicator | Type | Entry Point |
|-----------|------|-------------|
| `package.json` | TypeScript | `src/index.ts` |
| `main.py` | Python | `main.py` |

## What Gets Copied

### TypeScript Agents

Only essential files are copied (whitelist):

- `package.json`
- `tsconfig.json`
- `src/` directory
- `prompts/` directory (if exists)

Local npm references (`file:../..`) are automatically updated to use
published versions (`${config.packages.sdk_typescript_version}`).

### Python Agents

All files except common excludes (blacklist):

- Excludes: `venv/`, `__pycache__/`, `*.pyc`, `Dockerfile`, `README.md`
- Includes: `main.py`, `*.py`, `requirements.txt`, `prompts/`

## Generated test.yaml

The generated test file includes:

1. **Pre-run setup** - Appropriate routines based on agent type
2. **Copy artifacts** - Copy agents from `/artifacts/` to `/workspace/`
3. **Install dependencies** - `npm-install` or `pip-install` handlers
4. **Start agents** - `meshctl start` commands with wait steps
5. **Verify registration** - Check agents are registered with `meshctl list`
6. **Placeholder test steps** - Commented examples to customize
7. **Assertions** - Verify each agent is registered
8. **Post-run cleanup** - Stop agents and clean workspace

## Example Output

```yaml
name: "Test01 Provider Consumer"
description: "TODO: Add description"
tags:
  - scaffold
  - TODO
timeout: 300

pre_run:
  - routine: global.setup_for_typescript_agent
    params:
      meshctl_version: "${config.packages.cli_version}"

test:
  - name: "Copy artifacts to workspace"
    handler: shell
    command: |
      cp -r /artifacts/ts-provider /workspace/
      cp -r /artifacts/ts-consumer /workspace/

  - name: "Install ts-provider dependencies"
    handler: npm-install
    path: /workspace/ts-provider

  - name: "Start ts-provider"
    handler: shell
    command: "meshctl start ts-provider/src/index.ts -d"
    workdir: /workspace

  - name: "Wait for ts-provider to register"
    handler: wait
    seconds: 8

  # ... more steps ...

assertions:
  - expr: "${captured.agent_list} contains 'ts-provider'"
    message: "ts-provider should be registered"
```

## Interactive Mode

When `--uc` or `--tc` are not provided, scaffold prompts interactively:

1. **UC Selection** - Choose existing UC or create new
2. **TC Name** - Enter test case name
3. **Artifact Level** - Choose TC or UC level for artifacts
4. **Confirmation** - Review and confirm before creating

## Tips

- Use `--dry-run` first to preview what will be created
- Edit the generated `test.yaml` to add actual test logic
- Replace `TODO` tags with meaningful ones
- Check `global/routines.yaml` for available setup routines
