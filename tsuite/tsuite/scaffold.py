"""
Scaffold module for generating test cases from agent directories.

Usage:
    tsuite scaffold --suite ./my-suite --uc uc01_tags --tc tc01_test ./agent1 ./agent2
"""

import json
import re
import shutil
from dataclasses import dataclass, field
from pathlib import Path
from typing import Optional

from rich.console import Console

console = Console()


@dataclass
class AgentInfo:
    """Information about a detected agent."""
    path: Path
    name: str
    agent_type: str  # "typescript" or "python"
    entry_point: str  # e.g., "src/index.ts" or "main.py"
    has_requirements: bool = False
    package_json: Optional[dict] = None


@dataclass
class ScaffoldConfig:
    """Configuration for scaffold operation."""
    suite_path: Path
    uc_name: str
    tc_name: str
    agents: list[AgentInfo] = field(default_factory=list)
    artifact_level: str = "tc"  # "tc" or "uc"
    test_name: Optional[str] = None
    dry_run: bool = False
    force: bool = False
    skip_artifact_copy: bool = False  # Skip copying, just generate test.yaml


class ScaffoldError(Exception):
    """Error during scaffold operation."""
    pass


# =============================================================================
# Validation Functions
# =============================================================================

def validate_suite(suite_path: Path) -> None:
    """Validate that suite exists and has config.yaml."""
    if not suite_path.exists():
        raise ScaffoldError(f"Suite path does not exist: {suite_path}")

    config_file = suite_path / "config.yaml"
    if not config_file.exists():
        raise ScaffoldError(
            f"Not a valid suite: {suite_path}\n"
            f"Missing config.yaml. Create it first or use --suite with a valid suite path."
        )


def validate_agent_dir(agent_path: Path) -> AgentInfo:
    """Validate agent directory and detect type."""
    if not agent_path.exists():
        raise ScaffoldError(f"Agent directory does not exist: {agent_path}")

    if not agent_path.is_dir():
        raise ScaffoldError(f"Not a directory: {agent_path}")

    package_json = agent_path / "package.json"
    main_py = agent_path / "main.py"
    requirements_txt = agent_path / "requirements.txt"

    if package_json.exists():
        # TypeScript agent
        try:
            pkg_data = json.loads(package_json.read_text())
        except json.JSONDecodeError:
            raise ScaffoldError(f"Invalid package.json in {agent_path}")

        # Determine entry point
        entry_point = "src/index.ts"
        if (agent_path / "src" / "index.ts").exists():
            entry_point = "src/index.ts"
        elif (agent_path / "index.ts").exists():
            entry_point = "index.ts"

        return AgentInfo(
            path=agent_path,
            name=agent_path.name,
            agent_type="typescript",
            entry_point=entry_point,
            package_json=pkg_data,
        )

    elif main_py.exists():
        # Python agent
        return AgentInfo(
            path=agent_path,
            name=agent_path.name,
            agent_type="python",
            entry_point="main.py",
            has_requirements=requirements_txt.exists(),
        )

    else:
        raise ScaffoldError(
            f"Cannot detect agent type for: {agent_path}\n"
            f"Expected package.json (TypeScript) or main.py (Python)"
        )


def validate_no_parent_dirs(agent_paths: list[Path]) -> None:
    """Ensure no agent path is a parent of another."""
    resolved = [p.resolve() for p in agent_paths]

    for i, path1 in enumerate(resolved):
        for j, path2 in enumerate(resolved):
            if i != j:
                try:
                    path2.relative_to(path1)
                    raise ScaffoldError(
                        f"Parent directory conflict:\n"
                        f"  {path1} is parent of {path2}\n"
                        f"Cannot scaffold both a directory and its subdirectory."
                    )
                except ValueError:
                    pass  # Not a parent, good


def check_artifact_conflicts(
    artifacts_dir: Path,
    agents: list[AgentInfo],
    force: bool = False,
) -> None:
    """Check if any agent already exists in artifacts directory."""
    if not artifacts_dir.exists():
        return  # No conflicts possible

    conflicts = []
    for agent in agents:
        target = artifacts_dir / agent.name
        if target.exists():
            conflicts.append(agent.name)

    if conflicts and not force:
        raise ScaffoldError(
            f"Artifact conflict - the following agents already exist:\n"
            + "\n".join(f"  - {name}" for name in conflicts)
            + "\n\nUse --force to overwrite existing agents."
        )


# =============================================================================
# Artifact Copying
# =============================================================================

def clean_npm_local_references(package_json_path: Path) -> bool:
    """
    Clean up local file: references in package.json.

    Replaces:
        "@mcpmesh/sdk": "file:../../../packages/sdk"
    With:
        "@mcpmesh/sdk": "${config.packages.sdk_typescript_version}"

    Returns True if changes were made.
    """
    try:
        content = package_json_path.read_text()
        pkg_data = json.loads(content)
    except (json.JSONDecodeError, FileNotFoundError):
        return False

    changed = False

    # Mapping of package names to config variables
    package_mapping = {
        "@mcpmesh/sdk": "${config.packages.sdk_typescript_version}",
        "@mcpmesh/cli": "${config.packages.cli_version}",
        "@mcpmesh/core": "${config.packages.core_version}",
    }

    for dep_key in ["dependencies", "devDependencies"]:
        deps = pkg_data.get(dep_key, {})
        for pkg_name, version in list(deps.items()):
            if isinstance(version, str) and version.startswith("file:"):
                # Replace with config variable or remove
                if pkg_name in package_mapping:
                    deps[pkg_name] = package_mapping[pkg_name]
                    changed = True
                else:
                    # Unknown local package - use latest
                    deps[pkg_name] = "latest"
                    changed = True

    if changed:
        package_json_path.write_text(json.dumps(pkg_data, indent=2) + "\n")

    return changed


def copy_agent_to_artifacts(
    agent: AgentInfo,
    artifacts_dir: Path,
    dry_run: bool = False,
) -> Path:
    """Copy agent directory to artifacts, copying only essential files."""
    target_path = artifacts_dir / agent.name

    if dry_run:
        console.print(f"  [dim]Would copy: {agent.path} → {target_path}[/dim]")
        return target_path

    # Remove existing if present (force mode)
    if target_path.exists():
        shutil.rmtree(target_path)

    # Create target directory
    target_path.mkdir(parents=True)

    if agent.agent_type == "typescript":
        # TypeScript: whitelist approach - only copy essential files
        _copy_typescript_agent(agent, target_path)
    else:
        # Python: blacklist approach with comprehensive exclusions
        _copy_python_agent(agent, target_path)

    return target_path


def _copy_typescript_agent(agent: AgentInfo, target_path: Path) -> None:
    """Copy TypeScript agent using whitelist approach."""
    source = agent.path

    # Essential files to copy
    essential_files = ['package.json', 'tsconfig.json']
    for filename in essential_files:
        src_file = source / filename
        if src_file.exists():
            shutil.copy2(src_file, target_path / filename)

    # Determine source directory from entry point (e.g., "src/index.ts" -> "src")
    entry_dir = Path(agent.entry_point).parts[0] if "/" in agent.entry_point else "src"
    src_dir = source / entry_dir
    if src_dir.exists() and src_dir.is_dir():
        shutil.copytree(src_dir, target_path / entry_dir)

    # Copy prompts directory if exists (for LLM agents)
    prompts_dir = source / "prompts"
    if prompts_dir.exists() and prompts_dir.is_dir():
        shutil.copytree(prompts_dir, target_path / "prompts")

    # Clean up npm local references
    pkg_json = target_path / "package.json"
    if pkg_json.exists():
        if clean_npm_local_references(pkg_json):
            console.print(f"  [dim]Cleaned local npm references in {agent.name}/package.json[/dim]")


def _copy_python_agent(agent: AgentInfo, target_path: Path) -> None:
    """Copy Python agent using blacklist approach."""
    source = agent.path

    # Comprehensive exclusion list for Python agents
    exclude_patterns = {
        # Dependency/virtual env directories
        'node_modules', '__pycache__', '.venv', 'venv', 'env', '.env',
        # Version control
        '.git', '.gitignore',
        # Python artifacts
        '.pytest_cache', '.mypy_cache', '.ruff_cache',
        '*.egg-info', 'dist', 'build',
        # IDE/editor
        '.idea', '.vscode', '.DS_Store',
        # Runtime/database files
        '*.db', '*.db-shm', '*.db-wal',
        # Deployment files (not needed for tests)
        'Dockerfile', '.dockerignore', 'docker-compose.yml',
        'helm-values.yaml', 'k8s', 'kubernetes',
        # Documentation (not needed for tests)
        'README.md', 'README.rst', 'LICENSE', 'CHANGELOG.md',
    }

    exclude_extensions = {'.pyc', '.pyo', '.db', '.db-shm', '.db-wal'}

    def ignore_patterns(directory, files):
        ignore = []
        for f in files:
            # Check exact match
            if f in exclude_patterns:
                ignore.append(f)
            # Check extension
            elif any(f.endswith(ext) for ext in exclude_extensions):
                ignore.append(f)
            # Check glob patterns
            elif f.endswith('.egg-info'):
                ignore.append(f)
        return ignore

    # Use shutil but with target already created, need to copy contents
    shutil.rmtree(target_path)  # Remove the empty dir we created
    shutil.copytree(source, target_path, ignore=ignore_patterns)


# =============================================================================
# Test YAML Generation
# =============================================================================

def generate_test_yaml(config: ScaffoldConfig) -> str:
    """Generate test.yaml content for the scaffold."""
    agents = config.agents

    # Determine artifact path based on level
    if config.artifact_level == "uc":
        artifact_path = "/uc-artifacts"
    else:
        artifact_path = "/artifacts"

    # Separate by type
    ts_agents = [a for a in agents if a.agent_type == "typescript"]
    py_agents = [a for a in agents if a.agent_type == "python"]

    # Build pre_run routines
    pre_run_lines = []
    if py_agents:
        pre_run_lines.append('''  - routine: global.setup_for_python_agent
    params:
      meshctl_version: "${config.packages.cli_version}"
      mcpmesh_version: "${config.packages.sdk_python_version}"''')

    if ts_agents:
        pre_run_lines.append('''  - routine: global.setup_for_typescript_agent
    params:
      meshctl_version: "${config.packages.cli_version}"''')

    pre_run = "\n".join(pre_run_lines) if pre_run_lines else "  # No setup routines needed"

    # Build copy artifacts step
    copy_commands = "\n".join(
        f"      cp -r {artifact_path}/{a.name} /workspace/"
        for a in agents
    )

    # Build install steps
    install_steps = []
    for agent in agents:
        if agent.agent_type == "typescript":
            install_steps.append(f'''  - name: "Install {agent.name} dependencies"
    handler: npm-install
    path: /workspace/{agent.name}''')
        elif agent.has_requirements:
            install_steps.append(f'''  - name: "Install {agent.name} dependencies"
    handler: pip-install
    path: /workspace/{agent.name}''')

    install_section = "\n\n".join(install_steps) if install_steps else ""

    # Build start steps
    start_steps = []
    for agent in agents:
        start_steps.append(f'''  - name: "Start {agent.name}"
    handler: shell
    command: "meshctl start {agent.name}/{agent.entry_point} -d"
    workdir: /workspace

  - name: "Wait for {agent.name} to register"
    handler: wait
    seconds: 8''')

    start_section = "\n\n".join(start_steps)

    # Build assertions
    assertions = []
    for agent in agents:
        assertions.append(f'''  - expr: "${{captured.agent_list}} contains '{agent.name}'"
    message: "{agent.name} should be registered"''')

    assertions_section = "\n\n".join(assertions)

    # Agent names for header comment
    agent_names = ", ".join(a.name for a in agents)

    # Test name
    test_name = config.test_name or config.tc_name.replace("_", " ").replace("tc", "Test").title()

    # Generate YAML
    yaml_content = f'''# Test Case: {config.tc_name}
# Auto-generated by tsuite scaffold
# Agents: {agent_names}

name: "{test_name}"
description: "TODO: Add description"
tags:
  - scaffold
  - TODO
timeout: 300

pre_run:
{pre_run}

test:
  # Copy artifacts to workspace
  - name: "Copy artifacts to workspace"
    handler: shell
    command: |
{copy_commands}
    capture: copy_output

{install_section}

{start_section}

  # Verify agents registered
  - name: "Verify agents registered"
    handler: shell
    command: "meshctl list"
    workdir: /workspace
    capture: agent_list

  # === TODO: Add your test steps below ===
  # Example: Call a capability
  # - name: "Call my_capability"
  #   handler: shell
  #   command: |
  #     RESULT=$(meshctl call my_tool '{{"arg": "value"}}' 2>&1)
  #     echo "$RESULT"
  #     if echo "$RESULT" | grep -q 'expected'; then
  #       echo "PASS"
  #     else
  #       echo "FAIL"
  #       exit 1
  #     fi
  #   workdir: /workspace
  #   capture: call_result

assertions:
{assertions_section}

  # TODO: Add your assertions here
  # - expr: "${{captured.call_result}} contains 'PASS'"
  #   message: "Should return expected result"

post_run:
  - handler: shell
    command: "meshctl stop 2>/dev/null || true"
    workdir: /workspace
    ignore_errors: true
  - routine: global.cleanup_workspace
'''

    return yaml_content


# =============================================================================
# Main Scaffold Function
# =============================================================================

def run_scaffold(config: ScaffoldConfig) -> None:
    """Execute the scaffold operation."""
    suite_path = config.suite_path
    suites_dir = suite_path / "suites"
    uc_dir = suites_dir / config.uc_name
    tc_dir = uc_dir / config.tc_name

    # Determine artifacts directory
    if config.artifact_level == "uc":
        artifacts_dir = uc_dir / "artifacts"
    else:
        artifacts_dir = tc_dir / "artifacts"

    # Check for TC conflict
    if tc_dir.exists() and not config.force:
        raise ScaffoldError(
            f"Test case already exists: {tc_dir}\n"
            f"Use --force to overwrite."
        )

    # Check for artifact conflicts (skip if not copying)
    if not config.skip_artifact_copy:
        check_artifact_conflicts(artifacts_dir, config.agents, config.force)

    if config.dry_run:
        console.print("\n[bold cyan]Dry run - no files will be created[/bold cyan]\n")

    # Create UC if needed
    uc_created = False
    if not uc_dir.exists():
        if config.dry_run:
            console.print(f"[dim]Would create UC: {uc_dir}[/dim]")
        else:
            uc_dir.mkdir(parents=True)
            uc_created = True
            console.print(f"[green]✓[/green] Created UC: {config.uc_name}/")

    # Create TC
    tc_created = False
    if not tc_dir.exists():
        if config.dry_run:
            console.print(f"[dim]Would create TC: {tc_dir}[/dim]")
        else:
            tc_dir.mkdir(parents=True)
            tc_created = True
            console.print(f"[green]✓[/green] Created TC: {config.uc_name}/{config.tc_name}/")
    elif config.force:
        console.print(f"[yellow]![/yellow] Overwriting TC: {config.uc_name}/{config.tc_name}/")

    # Create artifacts directory (skip if not copying)
    if not config.skip_artifact_copy:
        if not artifacts_dir.exists():
            if config.dry_run:
                console.print(f"[dim]Would create artifacts: {artifacts_dir}[/dim]")
            else:
                artifacts_dir.mkdir(parents=True)

        # Copy agents
        console.print(f"[green]✓[/green] Copying artifacts:")
        for agent in config.agents:
            copy_agent_to_artifacts(agent, artifacts_dir, config.dry_run)
            type_label = "TypeScript" if agent.agent_type == "typescript" else "Python"
            console.print(f"    - {agent.name} ({type_label})")
    else:
        console.print(f"[yellow]![/yellow] Skipping artifact copy (--skip-artifact-copy)")
        for agent in config.agents:
            type_label = "TypeScript" if agent.agent_type == "typescript" else "Python"
            console.print(f"    - {agent.name} ({type_label})")

    # Generate test.yaml
    test_yaml_path = tc_dir / "test.yaml"
    test_yaml_content = generate_test_yaml(config)

    if config.dry_run:
        console.print(f"\n[dim]Would create: {test_yaml_path}[/dim]")
        console.print("\n[bold]Generated test.yaml:[/bold]")
        console.print(test_yaml_content)
    else:
        test_yaml_path.write_text(test_yaml_content)
        console.print(f"[green]✓[/green] Generated: {config.uc_name}/{config.tc_name}/test.yaml")

    # Print completion message
    if not config.dry_run:
        print_completion_message(config, uc_created, tc_created)


def print_completion_message(
    config: ScaffoldConfig,
    uc_created: bool,
    tc_created: bool,
) -> None:
    """Print helpful completion message."""
    console.print("\n[bold green]Scaffold complete![/bold green]\n")

    console.print("[bold]Next steps:[/bold]")
    console.print(f"  1. Edit test.yaml to add your test steps and assertions")
    console.print(f"  2. Run with: tsuite run --suite {config.suite_path} --tc {config.uc_name}/{config.tc_name}")

    console.print("\n[bold]Notes:[/bold]")
    console.print("  • Setup routines are referenced from global/routines.yaml")
    console.print("  • To create UC-level routines, add routines.yaml in the UC directory")
    console.print("  • See existing UC routines for examples:")
    console.print("      suites/uc02_agent_lifecycle/routines.yaml")
    console.print("      suites/uc04_llm_integration/routines.yaml")

    # Check if any npm cleanup was done
    ts_agents = [a for a in config.agents if a.agent_type == "typescript"]
    if ts_agents:
        console.print("  • Local npm references (file:...) have been updated to use")
        console.print("    published versions (${config.packages.*})")
