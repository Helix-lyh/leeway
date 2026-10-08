#!/usr/bin/env python3
"""Draw resources/icon.png.

The icon is the thing leeway is about: a quota bar with the part that is left
still visible. One dominant bar in the app's violet, its remaining headroom as
a translucent track, and a second, quieter bar under it — the same shape the
overview draws for the provider that is closest to its limit.

macOS geometry: a 1024 canvas, an 824 superellipse body centred in it, corner
exponent 5 (Apple's squircle, not a plain rounded rectangle). Everything is
drawn at 2x and scaled down, which is what keeps the edges clean.

    python3 resources/icon.py
"""

from PIL import Image, ImageChops, ImageDraw
import math

S = 2048  # working canvas, 2x the 1024 the bundle wants
BODY = 824 / 1024 * S
N = 5.0  # superellipse exponent
VIOLET_TOP = (124, 92, 255)
VIOLET_BOTTOM = (74, 34, 217)
TRACK = (23, 12, 60, 58)  # a recessed well, the way the app draws a track
FILL = (255, 255, 255, 255)


def superellipse(canvas, body, n, steps=4096):
    """A centred squircle mask, as Apple draws an icon body."""
    a = body / 2
    c = canvas / 2
    pts = []
    for i in range(steps):
        t = 2 * math.pi * i / steps
        ct, st = math.cos(t), math.sin(t)
        pts.append((
            c + a * math.copysign(abs(ct) ** (2 / n), ct),
            c + a * math.copysign(abs(st) ** (2 / n), st),
        ))
    mask = Image.new("L", (canvas, canvas), 0)
    ImageDraw.Draw(mask).polygon(pts, fill=255)
    return mask


def vertical_gradient(size, top, bottom):
    grad = Image.new("RGB", (1, size))
    px = grad.load()
    for y in range(size):
        f = y / (size - 1)
        px[0, y] = tuple(round(top[i] + (bottom[i] - top[i]) * f) for i in range(3))
    return grad.resize((size, size), Image.BILINEAR)


def bar(canvas, x, y, w, h, fill_w, radius):
    """One capsule: the recessed track, then the part that is used up."""
    well = Image.new("RGBA", (int(w) + 2, int(h) + 2), (0, 0, 0, 0))
    ImageDraw.Draw(well).rounded_rectangle([0, 0, w + 1, h + 1], radius=radius, fill=TRACK)
    canvas.alpha_composite(well, (int(x), int(y)))
    if fill_w <= 0:
        return
    fill = Image.new("RGBA", (int(w) + 2, int(h) + 2), (0, 0, 0, 0))
    fd = ImageDraw.Draw(fill)
    fd.rounded_rectangle([0, 0, fill_w + 1, h + 1], radius=radius, fill=FILL)
    # A touch of depth along the bottom of the fill, so the bar has a top face
    # rather than being a flat white sticker.
    shade = Image.new("RGBA", fill.size, (0, 0, 0, 0))
    ImageDraw.Draw(shade).rounded_rectangle(
        [0, h * 0.5, fill_w + 1, h + 1], radius=radius, fill=(70, 48, 150, 26))
    fill.alpha_composite(shade)
    canvas.alpha_composite(fill, (int(x), int(y)))


def main():
    canvas = Image.new("RGBA", (S, S), (0, 0, 0, 0))
    body = superellipse(S, BODY, N)

    # The body: violet, lighter at the top, with a soft highlight in the
    # upper-left corner so the plate reads as material rather than as a swatch.
    plate = vertical_gradient(S, VIOLET_TOP, VIOLET_BOTTOM).convert("RGBA")
    glow = Image.new("L", (S, S), 0)
    gd = ImageDraw.Draw(glow)
    cx, cy = S * 0.32, S * 0.22
    for i in range(30, -1, -1):  # largest first, so the middle stays brightest
        f = i / 30
        r = BODY * 0.92 * f
        gd.ellipse([cx - r, cy - r, cx + r, cy + r], fill=int(40 * (1 - f) ** 1.5))
    plate = Image.composite(Image.new("RGBA", (S, S), (255, 255, 255, 255)), plate, glow)
    canvas.paste(plate, (0, 0), body)

    # A hairline of light along the top edge, and a darker bottom edge: the
    # two strokes every macOS plate has.
    outer = superellipse(S, BODY, N)
    inner = superellipse(S, BODY - 6, N)
    ring = ImageChops.subtract(outer, inner)
    top_half = Image.new("L", (S, S), 0)
    ImageDraw.Draw(top_half).rectangle([0, 0, S, S // 2], fill=255)
    edge = Image.new("RGBA", (S, S), (0, 0, 0, 0))
    edge.paste((255, 255, 255, 70), (0, 0), ImageChops.multiply(ring, top_half))
    edge.paste((0, 0, 0, 46), (0, 0), ImageChops.multiply(ring, ImageChops.invert(top_half)))
    canvas = Image.alpha_composite(canvas, Image.composite(edge, Image.new("RGBA", (S, S), (0, 0, 0, 0)), body))

    # The quota: one bar, with the part that is left still visible. This is the
    # object the window is built around, drawn the way the window draws it.
    width = BODY * 0.70
    height = BODY * 0.16
    left = S * 0.5 - width / 2
    top = S * 0.5 - height / 2 - BODY * 0.01
    bar(canvas, left, top, width, height, width * 0.64, height / 2)

    canvas.resize((1024, 1024), Image.LANCZOS).save("resources/icon.png")
    for size in (256, 128, 32, 16):
        canvas.resize((size, size), Image.LANCZOS).save(f"/tmp/icon-{size}.png")


if __name__ == "__main__":
    main()
