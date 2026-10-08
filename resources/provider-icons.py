#!/usr/bin/env python3
"""Turn each provider's official icon into the asset the window embeds.

The window shows a provider by its own mark, never by a letter we drew, so the
sources below are the providers' own published icons. This script is a
maintainer tool: it normalises whatever each provider publishes (16-bit ICO,
full-bleed app icon, transparent glyph) into one square, 256px PNG with the
corners rounded, which is what `internal/gui/icons/` holds and what the window
embeds. Run it only when a provider changes its mark.

    python3 resources/provider-icons.py            # uses /tmp/icons as the cache
    python3 resources/provider-icons.py --raw DIR  # read the sources from DIR

Sources (see internal/gui/icons/SOURCES.md for what each one is):

  deepseek      the whale, from https://www.deepseek.com/favicon.ico
  kimi          https://www.kimi.com/pwa-192.png
  grok          https://grok.com/images/android-chrome-512x512.png
  codex         the OpenAI knot, in the style the maintainer asked for: cut out
                of codex-knot.jpg and drawn white, as the family is
  cursor        https://cursor.com/apple-touch-icon.png
  opencode-go   https://opencode.ai/apple-touch-icon.png, glyph only
  commandcode   https://commandcode.ai/apple-touch-icon.png
"""

import argparse
import pathlib
import subprocess
import sys

from PIL import Image, ImageChops, ImageDraw

# The app draws a mark with this radius, as a fraction of its side, so the
# asset is cut with the same one: the window's rounding and the asset's agree
# instead of fighting.
RADIUS = 0.26
SIDE = 256

# Provider id -> (raw file name, how to read it).
SOURCES = {
    "deepseek": ("deepseek.ico", "ico"),
    "kimi": ("kimi.png", "png"),
    "grok": ("grok.png", "png"),
    "codex": ("codex-knot.jpg", "png"),
    "cursor": ("cursor.png", "png"),
    "opencode-go": ("opencode.png", "png"),
    "commandcode": ("cc.png", "png"),
}

# The providers whose published icon is the mark alone, with no tile of its own.
# Their glyph is cut out and drawn in the window's tile, at the size the
# providers that do publish a tile draw theirs — 0.62 of the side. "colour"
# keeps the mark's own colours (the whale is blue, the terminal mark is two
# tones); "white" is for a mark that was drawn dark on light and has to join a
# dark tile.
GLYPHS = {
    "deepseek": ("deepseek.ico", "colour"),
    "codex": ("codex-knot.jpg", "white"),
    "opencode-go": ("opencode.png", "colour"),
}
GLYPH_SIDE = 0.62


def load(raw: pathlib.Path, kind: str) -> Image.Image:
    if kind == "ico":
        # A 32-bit BMP inside an ICO: macOS ImageIO reads it, Pillow does not.
        out = raw.with_suffix(".decoded.png")
        subprocess.run(["sips", "-s", "format", "png", str(raw), "--out", str(out)],
                       check=True, capture_output=True)
        return Image.open(out)
    return Image.open(raw)


def glyph(img: Image.Image, mode: str) -> Image.Image:
    """The mark alone: whatever the picture's background is, take it away and
    leave the ink, at the size a published tile draws its own glyph."""
    img = img.convert("RGB")
    ring = Image.new("RGB", (img.width, 8))
    ring.paste(img.crop((0, 0, img.width, 4)), (0, 0))
    ring.paste(img.crop((0, img.height - 4, img.width, img.height)), (0, 4))
    bg = ring.resize((1, 1), Image.BOX).getpixel((0, 0))
    alpha = ImageChops.difference(img, Image.new("RGB", img.size, bg)).convert("L")
    # A soft edge: JPEG and icon edges are not clean steps.
    alpha = alpha.point(lambda v: 0 if v < 12 else min(255, (v - 12) * 8))
    ink = Image.new("RGB", img.size, (255, 255, 255)) if mode == "white" else img
    out = Image.merge("RGBA", (*ink.split(), alpha))
    box = out.getchannel("A").getbbox()
    if box is None:
        return out
    out = out.crop(box)
    side = int(SIDE * GLYPH_SIDE)
    scale = side / max(out.size)
    out = out.resize((max(1, round(out.width * scale)), max(1, round(out.height * scale))),
                     Image.LANCZOS)
    canvas = Image.new("RGBA", (SIDE, SIDE), (0, 0, 0, 0))
    canvas.paste(out, ((SIDE - out.width) // 2, (SIDE - out.height) // 2), out)
    return canvas


def square(img: Image.Image) -> Image.Image:
    """The largest centred square, so nothing is squashed."""
    img = img.convert("RGBA")
    side = min(img.size)
    left = (img.width - side) // 2
    top = (img.height - side) // 2
    return img.crop((left, top, left + side, top + side))


def rounded(img: Image.Image, radius: float = RADIUS) -> Image.Image:
    mask = Image.new("L", img.size, 0)
    ImageDraw.Draw(mask).rounded_rectangle(
        (0, 0, img.width - 1, img.height - 1), radius=int(img.width * radius), fill=255)
    img = img.copy()
    img.putalpha(Image.composite(img.getchannel("A"),
                                 Image.new("L", img.size, 0), mask))
    return img


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--raw", default="/tmp/icons", help="directory holding the sources")
    ap.add_argument("--out", default="internal/gui/icons", help="where the assets go")
    args = ap.parse_args()

    raw_dir = pathlib.Path(args.raw)
    out_dir = pathlib.Path(args.out)
    out_dir.mkdir(parents=True, exist_ok=True)
    for provider, (name, kind) in SOURCES.items():
        if provider in GLYPHS:
            name, kind = GLYPHS[provider][0], "ico" if name.endswith(".ico") else "png"
        src = raw_dir / name
        if not src.exists():
            print(f"skip {provider}: {src} is missing (see the header for its URL)",
                  file=sys.stderr)
            continue
        img = load(src, kind)
        if provider in GLYPHS:
            # The window draws these in its own tile, so the asset is the mark
            # on nothing and the tile is drawn behind it.
            img = glyph(img, GLYPHS[provider][1])
        else:
            img = rounded(square(img).resize((SIDE, SIDE), Image.LANCZOS))
        dst = out_dir / f"{provider}.png"
        img.save(dst, optimize=True)
        print(f"{provider:14} {img.size[0]}x{img.size[1]} {dst.stat().st_size:>6}B  <- {name}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
