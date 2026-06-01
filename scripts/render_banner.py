"""Render the Loaded Price Monitor banner to a PNG for the README.

Usage:
    python scripts/render_banner.py [output.png]

Mirrors the gradient logic from python/src/banner.py exactly so the PNG
matches what a user sees in their terminal.
"""
from __future__ import annotations

import sys
from pathlib import Path

from PIL import Image, ImageDraw, ImageFont

REPO_ROOT = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(REPO_ROOT / "python"))
from src.banner import LINES, TAGLINE, GRADIENT_START, GRADIENT_END  # noqa: E402

FONT_CANDIDATES = [
    r"C:\Windows\Fonts\CascadiaMono.ttf",
    r"C:\Windows\Fonts\CascadiaCode.ttf",
    r"C:\Windows\Fonts\consola.ttf",
    "/Library/Fonts/Menlo.ttc",
    "/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf",
]

BG = (15, 18, 24)
TAGLINE_COLOR = (148, 163, 184)
FONT_SIZE = 20
PAD_X = 28
PAD_Y = 24
LINE_GAP = -4  # tighten rows so block-art glyphs meet seamlessly


def find_font(size: int) -> ImageFont.FreeTypeFont:
    for path in FONT_CANDIDATES:
        if Path(path).exists():
            return ImageFont.truetype(path, size)
    raise SystemExit("No suitable monospace font found.")


def lerp(a: int, b: int, t: float) -> int:
    return max(0, min(255, round(a + (b - a) * t)))


def gradient(idx: int, total: int) -> tuple[int, int, int]:
    t = 0.0 if total <= 1 else idx / (total - 1)
    return (
        lerp(GRADIENT_START[0], GRADIENT_END[0], t),
        lerp(GRADIENT_START[1], GRADIENT_END[1], t),
        lerp(GRADIENT_START[2], GRADIENT_END[2], t),
    )


def render(out: Path) -> None:
    font = find_font(FONT_SIZE)
    all_rows = list(LINES) + ["", TAGLINE]

    # Measure
    ascent, descent = font.getmetrics()
    line_h = ascent + descent + LINE_GAP
    max_w = 0
    for row in all_rows:
        bbox = font.getbbox(row)
        max_w = max(max_w, bbox[2] - bbox[0])

    w = max_w + PAD_X * 2
    h = line_h * len(all_rows) + PAD_Y * 2

    img = Image.new("RGB", (w, h), BG)
    draw = ImageDraw.Draw(img)

    total = len(LINES)
    y = PAD_Y
    for i, line in enumerate(LINES):
        draw.text((PAD_X, y), line, font=font, fill=gradient(i, total))
        y += line_h
    y += line_h  # blank spacer
    draw.text((PAD_X, y), TAGLINE, font=font, fill=TAGLINE_COLOR)

    img.save(out, optimize=True)
    print(f"Wrote {out}  ({w}x{h})")


if __name__ == "__main__":
    target = Path(sys.argv[1]) if len(sys.argv) > 1 else REPO_ROOT / "docs" / "banner.png"
    target.parent.mkdir(parents=True, exist_ok=True)
    render(target)
