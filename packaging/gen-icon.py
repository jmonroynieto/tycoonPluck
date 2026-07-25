#!/usr/bin/env python3
"""Regenerates packaging/icons/com.local.tycoonpluck.svg — three chunky
arrows spiraling outward from the center, in the app's own accent color.
Run this and re-rasterize (see `make icons`) after changing the design.
"""
import math
import os

CX, CY = 128.0, 128.0
BG_R = 118.0
BG_FILL = "#171512"
BG_FILL2 = "#0c0b0a"

# Matches internal/ui/theme.go palette.primary (warm gold).
ARROW_COLOR = "#D6BA7C"

STROKE_W = 30.0
R0, R1 = 27.0, 96.0
THETA0, THETA1 = math.radians(-15), math.radians(95)
N = 48

ARROWHEAD_LEN = 38.0
ARROWHEAD_HALF_W = 23.0

OUT_PATH = os.path.join(os.path.dirname(__file__), "icons", "com.local.tycoonpluck.svg")


def spiral_point(t):
    theta = THETA0 + (THETA1 - THETA0) * t
    r = R0 + (R1 - R0) * t
    return (CX + r * math.cos(theta), CY + r * math.sin(theta))


def blade_path_and_head():
    pts = [spiral_point(i / N) for i in range(N + 1)]
    d = "M " + " L ".join(f"{x:.2f},{y:.2f}" for x, y in pts)

    # Tangent at the tip via finite difference, used to aim the arrowhead.
    x1, y1 = spiral_point(0.965)
    x2, y2 = spiral_point(1.0)
    dx, dy = x2 - x1, y2 - y1
    norm = math.hypot(dx, dy)
    dx, dy = dx / norm, dy / norm
    px, py = -dy, dx

    apex = (x2 + dx * ARROWHEAD_LEN * 0.55, y2 + dy * ARROWHEAD_LEN * 0.55)
    base_center = (x2 - dx * ARROWHEAD_LEN * 0.30, y2 - dy * ARROWHEAD_LEN * 0.30)
    left = (base_center[0] + px * ARROWHEAD_HALF_W, base_center[1] + py * ARROWHEAD_HALF_W)
    right = (base_center[0] - px * ARROWHEAD_HALF_W, base_center[1] - py * ARROWHEAD_HALF_W)

    tri = f"M {apex[0]:.2f},{apex[1]:.2f} L {left[0]:.2f},{left[1]:.2f} L {right[0]:.2f},{right[1]:.2f} Z"
    return d, tri


def main():
    blade_d, head_d = blade_path_and_head()

    blades = []
    for angle in (0, 120, 240):
        blades.append(f'''
    <g transform="rotate({angle} {CX} {CY})">
      <path d="{blade_d}" fill="none" stroke="{ARROW_COLOR}"
            stroke-width="{STROKE_W}" stroke-linecap="round" stroke-linejoin="round"/>
      <path d="{head_d}" fill="{ARROW_COLOR}"/>
    </g>''')

    svg = f'''<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 256 256" width="256" height="256">
  <defs>
    <radialGradient id="bg" cx="35%" cy="30%" r="85%">
      <stop offset="0%" stop-color="{BG_FILL}"/>
      <stop offset="100%" stop-color="{BG_FILL2}"/>
    </radialGradient>
  </defs>
  <circle cx="{CX}" cy="{CY}" r="{BG_R}" fill="url(#bg)"/>
  {"".join(blades)}
</svg>
'''

    os.makedirs(os.path.dirname(OUT_PATH), exist_ok=True)
    with open(OUT_PATH, "w") as f:
        f.write(svg)
    print(f"wrote {OUT_PATH}")


if __name__ == "__main__":
    main()
