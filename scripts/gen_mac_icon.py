#!/usr/bin/env python3
"""Make assets/AppIcon.icns, the macOS icon, from assets/Icon.png.

Icon.png fills its square, as Android and iOS want: they cut the shape
themselves. macOS shows an icon as drawn, so this one keeps to the macOS
icon grid: an 824px rounded square on a 1024px canvas, with the template's
drop shadow. Run from the repository root; needs Pillow and iconutil.
"""

import math
import os
import subprocess
import tempfile

from PIL import Image, ImageChops, ImageDraw, ImageFilter

CANVAS, BODY = 1024, 824
SCALE = 4  # supersampling of the mask's edge


def squircle(size):
    """The macOS icon's continuous-corner square as an L mask: a superellipse."""
    big = size * SCALE
    mask = Image.new("L", (big, big), 0)
    n, r = 5.0, big / 2
    points = []
    steps = 2048
    for i in range(steps):
        t = 2 * math.pi * i / steps
        c, s = math.cos(t), math.sin(t)
        x = r + r * (abs(c) ** (2 / n)) * (1 if c >= 0 else -1)
        y = r + r * (abs(s) ** (2 / n)) * (1 if s >= 0 else -1)
        points.append((x, y))
    ImageDraw.Draw(mask).polygon(points, fill=255)
    return mask.resize((size, size), Image.LANCZOS)


def main():
    art = Image.open("assets/Icon.png").convert("RGBA").resize((BODY, BODY), Image.LANCZOS)
    mask = squircle(BODY)
    art.putalpha(ImageChops.multiply(art.getchannel("A"), mask))

    offset = (CANVAS - BODY) // 2
    shadow = Image.new("RGBA", (CANVAS, CANVAS), (0, 0, 0, 0))
    shade = Image.new("RGBA", (BODY, BODY), (0, 0, 0, 77))
    shade.putalpha(ImageChops.multiply(shade.getchannel("A"), mask))
    shadow.paste(shade, (offset, offset + 10), shade)
    icon = shadow.filter(ImageFilter.GaussianBlur(10))
    icon.alpha_composite(art, (offset, offset))

    with tempfile.TemporaryDirectory() as tmp:
        iconset = os.path.join(tmp, "AppIcon.iconset")
        os.mkdir(iconset)
        for size in (16, 32, 128, 256, 512):
            icon.resize((size, size), Image.LANCZOS).save(os.path.join(iconset, f"icon_{size}x{size}.png"))
            icon.resize((size * 2, size * 2), Image.LANCZOS).save(os.path.join(iconset, f"icon_{size}x{size}@2x.png"))
        subprocess.run(["iconutil", "-c", "icns", iconset, "-o", "assets/AppIcon.icns"], check=True)


if __name__ == "__main__":
    main()
