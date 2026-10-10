#!/usr/bin/env python3
"""Draw resources/icon.png.

The mark is a brush stroke wound into a spiral, solid where it is spent and
coming apart into threads where it is not — 尾, the tail, which is what leeway
means: the length you have not had to spend yet.

Why a spiral and not a quota bar. A quota bar is a line, and an icon that
redraws the app's own progress bar is an icon of a UI component, which is
exactly what Apple's HIG says not to ship ("prefer illustrations to photos and
avoid replicating UI components"). Winding the line up says the half of the
product a bar cannot: quota comes round again, it resets, it cycles. The solid
head is the part used; the threads are the part still open.

Why the threads rather than a dotted line. A dotted continuation was tried and
drawn correctly, and at 16px its dashes read as the teeth of a gear. Threads
that fan and thin read as one bold stroke at 16px and as fur at 512px.

macOS geometry: a 1024 canvas, an 824 superellipse body centred in it, corner
exponent 5 (Apple's squircle, not a rounded rectangle). The plate is one clean
vertical ramp — no glow, no bevel — and the mark is measured and re-centred on
its own ink, because a spiral with a tail is not symmetric. Everything is drawn
at 4x and scaled down, which is what keeps the edges clean.

    python3 resources/icon.py
"""

import math
import random

from PIL import Image, ImageDraw

S = 4096  # working canvas, 4x the 1024 the bundle wants
CANVAS = 1024
BODY = round(824 / 1024 * S)
N = 5.0  # superellipse exponent
VIOLET_TOP = (124, 92, 255)
VIOLET_BOTTOM = (74, 34, 217)
WHITE = (255, 255, 255)

# The stroke. How bold this can be is not a taste decision, it is arithmetic:
# the squircle is 412px from the centre on the axes, so R1 + WIDTH/2 has to stay
# under about 380, and the turns have to fit inside that. More turns therefore
# force a thinner stroke — 1.6 turns caps the width near 100, 1.1 turns allows
# 145. A bolder stroke is worth more than an extra half turn.
R0, R1 = 102, 307
TURNS = 1.10
WIDTH = 145
START = -math.pi / 2
TAPER = 0.80  # how much thinner each thread gets by its end; the head is
              # unaffected, because the taper only begins at the split
# 飞白: the stroke is bristles, not a ribbon. A duty below 1 leaves a fine gap
# between them along the whole stroke, the way a dry brush does, instead of the
# stroke only coming apart at its end. The gaps close up into solid tone as the
# icon is scaled down, which is what keeps 16px clean.
DUTY = 0.78
# Per thread: where it stops, where it starts to come away from the others, and
# how far it spreads. Every length is different and the lengths step down
# monotonically, so the tips trace a taper rather than a guillotine cut; the
# steps vary, and the split points are staggered, so the threads come apart one
# after another instead of along one straight seam. That is the difference
# between fur and a comb — the order is in the envelope, the scatter is in the
# detail.
THREAD_SPEC = (
    # end    split  fan
    (0.992, 0.52, 1.00),
    (0.962, 0.47, 1.08),
    (0.936, 0.50, 0.96),
    (0.900, 0.45, 1.06),
    (0.858, 0.54, 1.00),
)
# 散毛: hairs that escape the brush. These are single hairs, not strokes — a
# few pixels wide against a 145px body. Thicker than this and they stop reading
# as hairs and start competing with the stroke they came off.
#   from, to, how far out it drifts by the end, thickness
STRAY_SPEC = (
    (0.88, 1.00, 26, 4.0),
    (0.92, 1.02, 32, 3.0),
    (0.85, 0.95, 17, 5.0),
)
# 墨点: ink the brush threw off. Random, but from a fixed seed — an icon that
# redraws itself differently every run is not a build artifact.
INK_SEED = 7
INK_PER_TIP = 6
FLING = 88  # how far a fleck can travel before it has spent its momentum


def superellipse(canvas, body, n, steps=8192):
    """A centred squircle mask, as Apple draws an icon body."""
    a, c = body / 2, canvas / 2
    pts = []
    for i in range(steps):
        t = 2 * math.pi * i / steps
        ct, st = math.cos(t), math.sin(t)
        pts.append((c + a * math.copysign(abs(ct) ** (2 / n), ct),
                    c + a * math.copysign(abs(st) ** (2 / n), st)))
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


def plate():
    body = superellipse(S, BODY, N)
    img = vertical_gradient(S, VIOLET_TOP, VIOLET_BOTTOM).convert("RGBA")
    out = Image.new("RGBA", (S, S), (0, 0, 0, 0))
    out.paste(img, (0, 0), body)
    return out


def arc(cx, cy, r, a0, a1, steps=40):
    return [(cx + r * math.cos(a0 + (a1 - a0) * i / steps),
             cy + r * math.sin(a0 + (a1 - a0) * i / steps))
            for i in range(steps + 1)]


def band(dr, wf, t0, t1, steps=900):
    """One thread, as a closed polygon between two offset curves.

    The cap at each end is a half-disc centred on the centreline, swept from the
    OUTER edge (angle a) through the tangent to the INNER edge (a+pi). Sweeping
    a +/- pi/2 instead centres the half-disc on a and reaches neither edge; the
    polygon then self-intersects and even-odd fill bites a semicircular notch
    out of every thread end.
    """
    cx = cy = CANVAS / 2
    n = max(4, int(steps * (t1 - t0)))
    outer, inner = [], []
    for i in range(n + 1):
        t = t0 + (t1 - t0) * i / n
        r = R0 + (R1 - R0) * t + dr(t)
        a = START + 2 * math.pi * TURNS * t
        half = WIDTH * wf(t) / 2
        ca, sa = math.cos(a), math.sin(a)
        outer.append((cx + (r + half) * ca, cy + (r + half) * sa))
        inner.append((cx + (r - half) * ca, cy + (r - half) * sa))

    def end(t):
        r = R0 + (R1 - R0) * t + dr(t)
        a = START + 2 * math.pi * TURNS * t
        return cx + r * math.cos(a), cy + r * math.sin(a), a, WIDTH * wf(t) / 2

    x1, y1, a1, h1 = end(t1)
    x0, y0, a0, h0 = end(t0)
    return (outer + arc(x1, y1, h1, a1, a1 + math.pi)
            + inner[::-1] + arc(x0, y0, h0, a0 + math.pi, a0 + 2 * math.pi))


def circle(cx, cy, r, steps=28):
    return [(cx + r * math.cos(2 * math.pi * i / steps),
             cy + r * math.sin(2 * math.pi * i / steps)) for i in range(steps)]


def speck(cx, cy, r, rng, stretch=1.0, angle=0.0):
    """One fleck of ink, optionally streaked along `angle`.

    Mostly a lumpy blob with at most a couple of short burrs. Giving every
    vertex a spike produces a symmetrical star, and a field of stars reads as
    glitter, not as ink. Stretching the fleck along the direction it was thrown
    is what turns a scatter of dots into something that looks flung.
    """
    n = rng.randint(5, 8)
    spiked = set(rng.sample(range(n), rng.randint(0, 2)))
    ca, sa = math.cos(angle), math.sin(angle)
    pts = []
    for i in range(n):
        a = 2 * math.pi * i / n + rng.uniform(-0.14, 0.14)
        base = r * rng.uniform(0.55, 1.15)
        ring, offs = [base], [0.0]
        if i in spiked:
            ring = [base, base * rng.uniform(1.5, 2.1), base]
            offs = [-0.09, 0.0, 0.09]
        for rr, da in zip(ring, offs):
            lx = rr * stretch * math.cos(a + da)
            ly = rr * math.sin(a + da)
            pts.append((cx + lx * ca - ly * sa, cy + lx * sa + ly * ca))
    return pts


def stray(from_t, to_t, drift, thickness):
    """A hair that leaves the stroke. It starts inside the stroke, so it only
    becomes visible once it has drifted past the edge — which is how a hair
    actually comes off a brush."""
    span = to_t - from_t

    def dr(t):
        f = max(0.0, min(1.0, (t - from_t) / span))
        return drift * f * f

    def wf(t):
        return thickness / WIDTH

    return band(dr, wf, from_t, to_t)


def ink_specks(spec=THREAD_SPEC):
    """Ink thrown off the tips of the threads, as polygons.

    Each fleck leaves along its tip's own tangent — the direction the stroke was
    actually travelling — so the spray sweeps with the rotation instead of
    spraying evenly in all directions. Flecks are dense and large just off the
    tips and thin out, spread out and streak out with distance, which is what
    reads as force rather than as decoration. Random, from a fixed seed.
    """
    rng = random.Random(INK_SEED)
    n = len(spec)
    pitch = WIDTH / n
    mid = (n - 1) / 2
    k = (R1 - R0) / (2 * math.pi * TURNS)
    polys = []
    for i, (end, split, fan) in enumerate(spec):
        r = R0 + (R1 - R0) * end + (i - mid) * pitch * fan
        a = START + 2 * math.pi * TURNS * end
        ca, sa = math.cos(a), math.sin(a)
        vx, vy = k * ca - r * sa, k * sa + r * ca
        norm = math.hypot(vx, vy) or 1.0
        vx, vy = vx / norm, vy / norm
        tipx, tipy = CANVAS / 2 + r * ca, CANVAS / 2 + r * sa
        for _ in range(INK_PER_TIP):
            d = rng.uniform(0.0, 1.0) ** 1.5 * FLING
            spread = 5.0 + d * 0.42
            polys.append(speck(tipx + vx * d - vy * rng.gauss(0.0, spread),
                               tipy + vy * d + vx * rng.gauss(0.0, spread),
                               rng.uniform(3.0, 10.0) * (1.0 - d / FLING * 0.50) / 2,
                               rng,
                               1.0 + d / FLING * 2.3,
                               math.atan2(vy, vx)))
    return polys


def mark(spec=THREAD_SPEC):
    """The whole mark as polygons.

    Only the stroke is measured for centring. The ink is thrown off it and the
    hairs trail from it, so letting them into the measurement would allow one
    stray fleck to drag the whole spiral off centre.
    """
    n = len(spec)
    pitch = WIDTH / n
    mid = (n - 1) / 2
    body = []
    for i, (end, split, fan) in enumerate(spec):
        off = (i - mid) * pitch

        def dr(t, off=off, split=split, fan=fan):
            return off * fan * max(0.0, min(1.0, (t - split) / (1.0 - split)))

        def wf(t, split=split):
            f = max(0.0, min(1.0, (t - split) / (1.0 - split)))
            return (DUTY / n) * (1.0 - TAPER * f)

        body.append(band(dr, wf, 0.0, end))

    xs = [p[0] for poly in body for p in poly]
    ys = [p[1] for poly in body for p in poly]
    dx = CANVAS / 2 - (min(xs) + max(xs)) / 2
    dy = CANVAS / 2 - (min(ys) + max(ys)) / 2

    shifted = [[(x + dx, y + dy) for x, y in poly] for poly in body]
    for from_t, to_t, drift, thickness in STRAY_SPEC:
        shifted.append([(x + dx, y + dy)
                        for x, y in stray(from_t, to_t, drift, thickness)])
    for poly in ink_specks(spec):
        shifted.append([(x + dx, y + dy) for x, y in poly])
    return shifted


def main():
    canvas = plate()
    ink = Image.new("RGBA", (S, S), (0, 0, 0, 0))
    draw = ImageDraw.Draw(ink)
    for poly in mark():
        draw.polygon([(x / CANVAS * S, y / CANVAS * S) for x, y in poly], fill=WHITE)
    canvas.alpha_composite(ink)

    canvas.resize((CANVAS, CANVAS), Image.LANCZOS).save("resources/icon.png")
    for size in (256, 128, 32, 16):
        canvas.resize((size, size), Image.LANCZOS).save(f"/tmp/icon-{size}.png")


if __name__ == "__main__":
    main()
