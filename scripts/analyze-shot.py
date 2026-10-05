#!/usr/bin/env python3
"""Measure a rendered page's layout from its own pixels.

**Why this exists.** The view tree says where AppKit *intends* to put things;
this says where the ink actually landed. Those have differed before — three
pages were measured wrong this way — so a layout opinion is worth exactly
nothing until it is checked against the raster.

Everything here reads the PNG the app wrote. No window server, no `sips`,
no re-render, and no assumption about what the renderer meant to do.

    python3 scripts/analyze-shot.py /tmp/shots7/core-home.png

The output answers four questions a design review actually asks:

  - where is the ink, and how much of the frame does it use
  - are the horizontal margins even (a ragged right edge is the most common
    complaint about a settings pane, and it is invisible in a view tree)
  - are the vertical gaps between blocks the same size (rhythm)
  - is anything touching the edges when it should be inset
"""

import sys
from PIL import Image

# A pixel counts as ink when it differs from the page background. The
# background is sampled from the corners rather than assumed white, because
# `--dark` renders on near-black and assuming white marks the whole frame as
# ink.
def load(path):
    im = Image.open(path).convert("RGB")
    w, h = im.size
    px = im.load()
    corners = [px[0, 0], px[w - 1, 0], px[0, h - 1], px[w - 1, h - 1]]
    bg = tuple(sum(c[i] for c in corners) // 4 for i in range(3))
    return im, px, w, h, bg


def ink_map(path, tol=28):
    """Boolean grid: is this pixel not the background?"""
    im, px, w, h, bg = load(path)
    rows = []
    for y in range(h):
        row = bytearray(w)
        for x in range(w):
            r, g, b = px[x, y]
            if abs(r - bg[0]) + abs(g - bg[1]) + abs(b - bg[2]) > tol:
                row[x] = 1
        rows.append(row)
    return rows, w, h, bg


def bands(row_flags):
    """Contiguous runs of True, as (start, end) inclusive-exclusive."""
    out, start = [], None
    for i, on in enumerate(row_flags):
        if on and start is None:
            start = i
        elif not on and start is not None:
            out.append((start, i))
            start = None
    if start is not None:
        out.append((start, len(row_flags)))
    return out


def main(path):
    rows, w, h, bg = ink_map(path)

    any_row = [any(r) for r in rows]
    any_col = [any(rows[y][x] for y in range(h)) for x in range(w)]

    if not any(any_row):
        print(f"{path}: BLANK — no ink at all")
        return

    y0 = next(i for i, v in enumerate(any_row) if v)
    y1 = h - next(i for i, v in enumerate(reversed(any_row)) if v)
    x0 = next(i for i, v in enumerate(any_col) if v)
    x1 = w - next(i for i, v in enumerate(reversed(any_col)) if v)

    print(f"{path}")
    print(f"  frame       {w} x {h}px   background rgb{bg}")
    print(f"  ink box     x {x0}..{x1}  y {y0}..{y1}")
    print(f"  left margin {x0}px   right margin {w - x1}px   top {y0}px   bottom {h - y1}px")
    print(f"  fills       x {100 * (x1 - x0) / w:.0f}%   y {100 * (y1 - y0) / h:.0f}%")

    # Vertical rhythm: the gaps BETWEEN blocks of ink. Block height is noise;
    # spacing is the design.
    gap_sizes = []
    blocks = bands(any_row)
    for a, b in zip(blocks, blocks[1:]):
        gap = a[1] - b[0]
        gap_sizes.append(gap)
    if gap_sizes:
        # Ignore the sub-pixel gaps inside a paragraph of text (<= 4px).
        real = [g for g in gap_sizes if g > 4]
        if real:
            uniq = sorted(set(real))
            print(f"  gaps        {len(real)} between blocks: {uniq}")
            if len(uniq) > 4:
                print("                ^ many distinct gap sizes — the vertical rhythm is not one value")

    # Horizontal: for each ink band, where does its content start and end?
    # This is what a ragged right edge looks like numerically.
    lefts, rights = [], []
    for a, b in bands(any_row):
        cols = [any(rows[y][x] for y in range(a, b)) for x in range(w)]
        if not any(cols):
            continue
        l = next(i for i, v in enumerate(cols) if v)
        r = w - next(i for i, v in enumerate(reversed(cols)) if v)
        lefts.append(l)
        rights.append(r)
    if lefts:
        lset, rset = sorted(set(lefts)), sorted(set(rights))
        print(f"  band lefts  {lset[:8]}{' …' if len(lset) > 8 else ''}")
        print(f"  band rights {rset[-8:]}{' …' if len(rset) > 8 else ''}")
        if len(lset) == 1:
            print("                ^ every band starts at the same x — the left edge is aligned")
        if len(rset) > 6:
            print(f"                ^ {len(rset)} distinct right edges")


if __name__ == "__main__":
    for p in sys.argv[1:]:
        main(p)
        print()