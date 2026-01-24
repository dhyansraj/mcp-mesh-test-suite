"""
Terminal renderer for man page markdown content.

Uses rich library for nice terminal output.
"""

from rich.console import Console
from rich.markdown import Markdown
from rich.panel import Panel
from rich.table import Table
from rich.text import Text

from . import ManPage, list_pages


def render_page(page: ManPage, console: Console | None = None) -> None:
    """Render a man page to the terminal."""
    if console is None:
        console = Console()

    content = page.get_content()

    # Render markdown with rich
    md = Markdown(content)
    console.print()
    console.print(md)
    console.print()


def render_list(console: Console | None = None) -> None:
    """Render the list of available man pages."""
    if console is None:
        console = Console()

    console.print()
    console.print(Text("Available Topics", style="bold cyan"))
    console.print(Text("─" * 72, style="cyan"))
    console.print()

    for page in list_pages():
        # Topic name in yellow/bold
        name_text = Text(f"  {page.name}", style="bold yellow")

        # Aliases in dim
        if page.aliases:
            aliases_str = ", ".join(page.aliases)
            name_text.append(f" ({aliases_str})", style="dim")

        console.print(name_text)

        # Description indented
        console.print(Text(f"    {page.description}", style=""))
        console.print()


def render_not_found(topic: str, console: Console | None = None) -> None:
    """Render a 'topic not found' message with suggestions."""
    if console is None:
        console = Console()

    console.print()
    console.print(Text(f"Topic not found: {topic}", style="bold red"))
    console.print()
    console.print("Use 'tsuite man --list' to see available topics.")
    console.print()
