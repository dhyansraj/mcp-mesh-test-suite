"""
Test and routine discovery.

Finds test.yaml and routines.yaml files in the test suite directory.
"""

import yaml
from pathlib import Path
from dataclasses import dataclass, field


@dataclass
class TestCase:
    """Represents a discovered test case."""
    id: str  # e.g., "uc01_scaffolding/tc01_python_agent"
    name: str
    path: Path  # Path to test.yaml
    tags: list[str] = field(default_factory=list)
    timeout: int = 120
    config: dict = field(default_factory=dict)  # Full parsed YAML

    @property
    def uc(self) -> str:
        """Use case directory name."""
        parts = self.id.split("/")
        return parts[0] if parts else ""

    @property
    def tc(self) -> str:
        """Test case directory name."""
        parts = self.id.split("/")
        return parts[1] if len(parts) > 1 else ""


@dataclass
class RoutineSet:
    """Collection of routines at a specific scope."""
    scope: str  # "global", "uc01_scaffolding", "uc01_scaffolding/tc01_python_agent"
    path: Path  # Path to routines.yaml
    routines: dict[str, dict] = field(default_factory=dict)


class TestDiscovery:
    """
    Discovers tests and routines in a test suite directory.

    Expected structure:
        suites/
            uc01_scaffolding/
                routines.yaml  (optional, UC-level)
                tc01_python_agent/
                    test.yaml
                    routines.yaml  (optional, TC-level)
        global/
            routines.yaml  (global-level)
    """

    def __init__(self, suite_path: Path):
        self.suite_path = Path(suite_path)
        self.suites_path = self.suite_path / "suites"
        self.global_path = self.suite_path / "global"

    def discover_tests(self) -> list[TestCase]:
        """Find all test.yaml files and parse them."""
        tests = []

        if not self.suites_path.exists():
            return tests

        # Find all test.yaml files
        for test_yaml in self.suites_path.glob("**/test.yaml"):
            rel_path = test_yaml.parent.relative_to(self.suites_path)
            test_id = str(rel_path)

            try:
                with open(test_yaml) as f:
                    config = yaml.safe_load(f) or {}

                tests.append(TestCase(
                    id=test_id,
                    name=config.get("name", test_id),
                    path=test_yaml,
                    tags=config.get("tags", []),
                    timeout=config.get("timeout", 120),
                    config=config,
                ))
            except Exception as e:
                print(f"Warning: Failed to parse {test_yaml}: {e}")

        # Sort by ID for consistent ordering
        tests.sort(key=lambda t: t.id)
        return tests

    def discover_routines(self) -> dict[str, RoutineSet]:
        """
        Find all routines.yaml files and parse them.

        Returns dict mapping scope to RoutineSet:
            "global" -> global routines
            "uc01_scaffolding" -> UC-level routines
            "uc01_scaffolding/tc01_python_agent" -> TC-level routines
        """
        routine_sets = {}

        # Global routines
        global_routines = self.global_path / "routines.yaml"
        if global_routines.exists():
            try:
                with open(global_routines) as f:
                    data = yaml.safe_load(f) or {}
                routine_sets["global"] = RoutineSet(
                    scope="global",
                    path=global_routines,
                    routines=data.get("routines", {}),
                )
            except Exception as e:
                print(f"Warning: Failed to parse {global_routines}: {e}")

        # UC and TC level routines
        if self.suites_path.exists():
            for routines_yaml in self.suites_path.glob("**/routines.yaml"):
                rel_path = routines_yaml.parent.relative_to(self.suites_path)
                scope = str(rel_path)

                try:
                    with open(routines_yaml) as f:
                        data = yaml.safe_load(f) or {}
                    routine_sets[scope] = RoutineSet(
                        scope=scope,
                        path=routines_yaml,
                        routines=data.get("routines", {}),
                    )
                except Exception as e:
                    print(f"Warning: Failed to parse {routines_yaml}: {e}")

        return routine_sets

    def filter_tests(
        self,
        tests: list[TestCase],
        uc: list[str] | None = None,
        tc: list[str] | None = None,
        tags: list[str] | None = None,
        pattern: str | None = None,
    ) -> list[TestCase]:
        """
        Filter tests by various criteria.

        Args:
            uc: List of use case IDs to include
            tc: List of test case IDs to include (full path like "uc01/tc01")
            tags: List of tags (test must have ALL tags)
            pattern: Glob pattern to match test IDs
        """
        import fnmatch

        filtered = tests

        # Filter by use case
        if uc:
            filtered = [t for t in filtered if t.uc in uc]

        # Filter by test case ID
        if tc:
            filtered = [t for t in filtered if t.id in tc]

        # Filter by tags (must have ALL specified tags)
        if tags:
            filtered = [t for t in filtered if all(tag in t.tags for tag in tags)]

        # Filter by pattern
        if pattern:
            filtered = [t for t in filtered if fnmatch.fnmatch(t.id, pattern)]

        return filtered


def load_config(config_path: Path) -> dict:
    """Load the main config.yaml file."""
    if not config_path.exists():
        return {}

    with open(config_path) as f:
        return yaml.safe_load(f) or {}
