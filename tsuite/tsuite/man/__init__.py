"""
Man pages for tsuite.

Provides documentation accessible via `tsuite man <topic>`.
"""

from pathlib import Path
from typing import Optional

# Man page content directory
CONTENT_DIR = Path(__file__).parent / "content"


class ManPage:
    """Represents a man page topic."""

    def __init__(
        self,
        name: str,
        title: str,
        description: str,
        aliases: list[str] | None = None,
    ):
        self.name = name
        self.title = title
        self.description = description
        self.aliases = aliases or []

    @property
    def file_path(self) -> Path:
        """Path to the markdown content file."""
        return CONTENT_DIR / f"{self.name}.md"

    def exists(self) -> bool:
        """Check if the content file exists."""
        return self.file_path.exists()

    def get_content(self) -> str:
        """Read and return the markdown content."""
        if not self.exists():
            return f"# {self.title}\n\nContent not yet available."
        return self.file_path.read_text()


# Registry of all man pages
PAGES: dict[str, ManPage] = {
    "quickstart": ManPage(
        name="quickstart",
        title="Quick Start",
        description="Get started with tsuite in minutes",
        aliases=["quick", "start"],
    ),
    "suites": ManPage(
        name="suites",
        title="Test Suites",
        description="Suite structure and config.yaml",
        aliases=["suite", "config"],
    ),
    "usecases": ManPage(
        name="usecases",
        title="Use Cases",
        description="Organizing tests into use cases",
        aliases=["uc", "usecase"],
    ),
    "testcases": ManPage(
        name="testcases",
        title="Test Cases",
        description="Test case structure and test.yaml",
        aliases=["tc", "testcase", "test"],
    ),
    "handlers": ManPage(
        name="handlers",
        title="Handlers",
        description="Built-in handlers (http, exec, mesh, etc.)",
        aliases=["handler"],
    ),
    "routines": ManPage(
        name="routines",
        title="Routines",
        description="Reusable test routines",
        aliases=["routine"],
    ),
    "assertions": ManPage(
        name="assertions",
        title="Assertions",
        description="Assertion syntax and expressions",
        aliases=["assert", "assertion"],
    ),
    "artifacts": ManPage(
        name="artifacts",
        title="Artifacts",
        description="Test artifacts and file mounting",
        aliases=["artifact", "files"],
    ),
    "variables": ManPage(
        name="variables",
        title="Variables",
        description="Variable interpolation syntax",
        aliases=["vars", "interpolate", "interpolation"],
    ),
    "docker": ManPage(
        name="docker",
        title="Docker Mode",
        description="Container isolation and Docker execution",
        aliases=["container", "containers"],
    ),
    "api": ManPage(
        name="api",
        title="API & Dashboard",
        description="REST API server and web dashboard",
        aliases=["server", "dashboard", "ui"],
    ),
    "scaffold": ManPage(
        name="scaffold",
        title="Scaffold",
        description="Generate test cases from agent directories",
        aliases=["generate", "gen"],
    ),
}

# Build alias lookup
_ALIAS_MAP: dict[str, str] = {}
for page_name, page in PAGES.items():
    _ALIAS_MAP[page_name] = page_name
    for alias in page.aliases:
        _ALIAS_MAP[alias] = page_name


def get_page(name: str) -> Optional[ManPage]:
    """Get a man page by name or alias."""
    canonical = _ALIAS_MAP.get(name.lower())
    if canonical:
        return PAGES.get(canonical)
    return None


def list_pages() -> list[ManPage]:
    """List all available man pages."""
    return list(PAGES.values())
