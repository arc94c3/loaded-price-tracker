"""ASCII banner for the Loaded Price Monitor CLI."""
from __future__ import annotations

import os
import sys

from rich.console import Console
from rich.text import Text

LINES = [
    "  ██╗      ██████╗  █████╗ ██████╗ ███████╗██████╗ ",
    "  ██║     ██╔═══██╗██╔══██╗██╔══██╗██╔════╝██╔══██╗",
    "  ██║     ██║   ██║███████║██║  ██║█████╗  ██║  ██║",
    "  ██║     ██║   ██║██╔══██║██║  ██║██╔══╝  ██║  ██║",
    "  ███████╗╚██████╔╝██║  ██║██████╔╝███████╗██████╔╝",
    "  ╚══════╝ ╚═════╝ ╚═╝  ╚═╝╚═════╝ ╚══════╝╚═════╝ ",
    "  ██████╗ ██████╗ ██╗ ██████╗███████╗    ███╗   ███╗ ██████╗ ███╗   ██╗██╗████████╗ ██████╗ ██████╗ ",
    "  ██╔══██╗██╔══██╗██║██╔════╝██╔════╝    ████╗ ████║██╔═══██╗████╗  ██║██║╚══██╔══╝██╔═══██╗██╔══██╗",
    "  ██████╔╝██████╔╝██║██║     █████╗      ██╔████╔██║██║   ██║██╔██╗ ██║██║   ██║   ██║   ██║██████╔╝",
    "  ██╔═══╝ ██╔══██╗██║██║     ██╔══╝      ██║╚██╔╝██║██║   ██║██║╚██╗██║██║   ██║   ██║   ██║██╔══██╗",
    "  ██║     ██║  ██║██║╚██████╗███████╗    ██║ ╚═╝ ██║╚██████╔╝██║ ╚████║██║   ██║   ╚██████╔╝██║  ██║",
    "  ╚═╝     ╚═╝  ╚═╝╚═╝ ╚═════╝╚══════╝    ╚═╝     ╚═╝ ╚═════╝ ╚═╝  ╚═══╝╚═╝   ╚═╝    ╚═════╝╚═╝  ╚═╝",
]

TAGLINE = "  > track game key prices on loaded.com"

GRADIENT_START = (0xFF, 0x2E, 0x2E)
GRADIENT_END = (0xFF, 0xA8, 0x3C)


def _color_enabled() -> bool:
    if os.environ.get("NO_COLOR"):
        return False
    if os.environ.get("FORCE_COLOR") or os.environ.get("CLICOLOR_FORCE"):
        return True
    return sys.stdout.isatty()


def _lerp(a: int, b: int, t: float) -> int:
    return max(0, min(255, int(round(a + (b - a) * t))))


def _gradient_color(idx: int, total: int) -> str:
    t = 0.0 if total <= 1 else idx / (total - 1)
    r = _lerp(GRADIENT_START[0], GRADIENT_END[0], t)
    g = _lerp(GRADIENT_START[1], GRADIENT_END[1], t)
    b = _lerp(GRADIENT_START[2], GRADIENT_END[2], t)
    return f"#{r:02x}{g:02x}{b:02x}"


def print_banner(force: bool = False) -> None:
    """Print the banner. Suppressed unless TTY or `force=True`."""
    if not force and not _color_enabled() and not sys.stdout.isatty():
        return

    console = Console(no_color=not _color_enabled(), force_terminal=force or None)
    console.print()
    total = len(LINES)
    for i, line in enumerate(LINES):
        if _color_enabled():
            text = Text(line, style=f"bold {_gradient_color(i, total)}")
            console.print(text)
        else:
            console.print(line)
    if _color_enabled():
        console.print(Text(TAGLINE, style="#64748b"))
    else:
        console.print(TAGLINE)
    console.print()


if __name__ == "__main__":
    print_banner(force=True)
