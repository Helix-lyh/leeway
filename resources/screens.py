#!/usr/bin/env python3
"""Compose the screenshot board from the window shots in build/screenshots.

    python3 resources/screens.py build/screenshots

Every cell is one window at 1040x700, scaled to CELL wide, in the order the
board reads: the overview, the provider pages, the history page, settings,
then the first-run and dark passes.
"""

import sys
from pathlib import Path

from PIL import Image, ImageDraw

CELL = 640
GAP = 16
ORDER = [
    "overview.png",
    "overview-trend.png",
    "overview-burn.png",
    "provider.png",
    "provider-trend.png",
    "provider-many.png",
    "provider-many-burn.png",
    "history-many.png",
    "history-burn.png",
    "settings.png",
    "settings-data.png",
    "overview-first-run.png",
    "overview-dark.png",
    "provider-many-dark.png",
    "about.png",
]


def main() -> int:
    root = Path(sys.argv[1] if len(sys.argv) > 1 else "build/screenshots")
    shots = [(name, Image.open(root / name).convert("RGB")) for name in ORDER if (root / name).exists()]
    if not shots:
        print("no screenshots in", root, file=sys.stderr)
        return 1
    cols = 4
    rows = (len(shots) + cols - 1) // cols
    height = round(shots[0][1].height * CELL / shots[0][1].width)
    board = Image.new("RGB", (cols * CELL + (cols + 1) * GAP, rows * (height + 22) + (rows + 1) * GAP), "#101014")
    draw = ImageDraw.Draw(board)
    for i, (name, img) in enumerate(shots):
        x = GAP + (i % cols) * (CELL + GAP)
        y = GAP + (i // cols) * (height + 22 + GAP)
        board.paste(img.resize((CELL, height), Image.LANCZOS), (x, y))
        draw.text((x + 2, y + height + 5), name.removesuffix(".png"), fill="#8b8b96")
    out = root / "all-screens.png"
    board.save(out)
    print(f"{out}: {board.width}x{board.height}, {len(shots)} screens")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
