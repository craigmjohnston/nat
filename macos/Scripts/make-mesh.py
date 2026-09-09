#!/usr/bin/env python3
"""A true mesh gradient: a 3x3 anchor grid of colours, bilinearly
interpolated with a gentle sinusoidal warp so the flow reads organic, plus
mild dithering so the ramps don't band. Emits a base64 PNG data URI."""
import base64
import io
import math
import random
import sys

from PIL import Image

# 3x3 anchors, rows top->bottom: one violet-to-pink family flowing on the
# diagonal, dark enough to stay a dark-mode icon, no isolated pools.
GRID = [
    ["#7a58f2", "#4a35b8", "#241d4e"],
    ["#41307f", "#5c3a9e", "#a4489e"],
    ["#231c48", "#8a4192", "#e06ab2"],
]

def hx(c):
    return tuple(int(c[i:i + 2], 16) for i in (1, 3, 5))

G = [[hx(c) for c in row] for row in GRID]
SIZE = 256
rng = random.Random(7)

img = Image.new("RGB", (SIZE, SIZE))
px = img.load()
for y in range(SIZE):
    for x in range(SIZE):
        u, v = x / (SIZE - 1), y / (SIZE - 1)
        # the warp: what makes it a flowing field rather than a flat blend
        u2 = min(1, max(0, u + 0.07 * math.sin(2.6 * v * math.pi + 0.8)))
        v2 = min(1, max(0, v + 0.07 * math.sin(2.2 * u * math.pi + 2.1)))
        gu, gv = u2 * 2, v2 * 2
        i, j = min(1, int(gv)), min(1, int(gu))
        fu, fv = gu - j, gv - i
        # smoothstep the blend so anchors don't telegraph as a grid
        fu = fu * fu * (3 - 2 * fu)
        fv = fv * fv * (3 - 2 * fv)
        out = []
        for ch in range(3):
            a = G[i][j][ch] * (1 - fu) + G[i][j + 1][ch] * fu
            b = G[i + 1][j][ch] * (1 - fu) + G[i + 1][j + 1][ch] * fu
            val = a * (1 - fv) + b * fv
            out.append(max(0, min(255, round(val + rng.uniform(-1.5, 1.5)))))
        px[x, y] = tuple(out)

buf = io.BytesIO()
img.save(buf, format="PNG", optimize=True)
print("data:image/png;base64," + base64.b64encode(buf.getvalue()).decode())
