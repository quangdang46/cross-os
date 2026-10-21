#!/usr/bin/env python3
"""Does a rendered page's SPACING carry the grouping, or only its boxes?

**Why this exists.** The design removed the boxes on purpose — a group draws
nothing, and the hierarchy is carried by type and rhythm. That is a real macOS
choice. It is also a bet: **if spacing does not separate the roles, removing
the box removes the only thing that was doing the work**, and the page reads as
a list of equally-weighted lines.

That bet is checkable, and it is checked here — on the pixels, because "the gap
between section frames is 40pt" is not what a reader measures. What a reader
measures is the gap between the last glyph of one group and the first glyph of
the next: the frame gap plus the space inside the two frames above and below
their text.

**Where the roles come from, and why not from the pixels.**

The first version bucketed every ink gap into "within" or "between" by
comparing its magnitude against two brackets — and that is circular, because
whether the roles separate BY SIZE is the thing under test. It failed loudly:
raising the section gap from 40pt to 56pt made the measured "between" figure
go *down* on twelve of fifteen pages, which is impossible if the bucket means
what it says. It was measuring whichever gap happened to land in a bracket.

So the roles come from the view tree, which knows which view is a group, and
the distances come from the pixels, which know where the ink landed. Each
instrument does the half it is good at.

    python3 scripts/check-rhythm.py docs/screenshots/*.png

Needs the `<page>.layout.json` sidecar the shell writes beside each PNG.
"""

import importlib.util
import json
import os
import statistics
import sys

_HERE = os.path.dirname(os.path.abspath(__file__))
_spec = importlib.util.spec_from_file_location("analyze_shot", os.path.join(_HERE, "analyze-shot.py"))
_module = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(_module)
bands, ink_map = _module.bands, _module.ink_map

# A group must be this much further from its neighbour than its own rows are
# from each other, or the two are the same distance and grouping is carried by
# nothing at all. System Settings sits near 3x; below 2.5x a reader cannot
# tell a section from a row.
MIN_RATIO = 2.5

# Tolerance, in pixels, for "this ink belongs to that group". A group's frame
# is the ink plus its padding, so ink sits strictly inside it; the slack covers
# antialiasing at the edges.
SLACK = 8


def ink_bands(path, x_range=None):
    """Contiguous runs of ink, merged across intra-block leading."""
    rows, _w, _h, _bg = ink_map(path, x_range=x_range)
    found = bands([any(r) for r in rows])
    merged = []
    for a, b in found:
        if merged and a - merged[-1][1] <= 6:
            merged[-1] = (merged[-1][0], b)
        else:
            merged.append((a, b))
    return merged


def group_rects(png_path):
    """Outer group rects from the sidecar, in pixels: (x, y0, y1, w).

    A group is reported twice — once for `CardControl`, once for the `Card` it
    wraps — and the two share a top edge. Keeping the taller one per top edge
    leaves the outer rect, which is the boundary that matters: an ink gap
    cannot be inside a group and also between two of them.
    """
    sidecar = os.path.splitext(png_path)[0] + ".layout.json"
    if not os.path.exists(sidecar):
        return None
    with open(sidecar) as handle:
        data = json.load(handle)
    best = {}
    for g in data.get("groups", []):
        y0, y1 = g["y0"], g["y1"]
        x = g.get("x", 0)
        w = g.get("w", 0)
        if y0 not in best or y1 > best[y0][2]:
            best[y0] = (x, y0, y1, w)
    rects = sorted(best.values())
    # **A group INSIDE another group is not a second group.**
    #
    # The tree reports a group twice: the card and the control that wraps
    # it. When the control has a heading above the card the two do NOT
    # share a top edge — the heading puts the card lower — so the dedup
    # that keeps one per top edge kept both, and the classifier saw two
    # overlapping groups and gave up: "0 within, 0 between — nothing to
    # compare" on `core.commands` and `core.rules`, the two pages with the
    # most groups on them.
    #
    # So CONTAINMENT decides, not the top edge: the outer rect is the
    # boundary that matters, and an ink gap cannot be inside a group and
    # also between two of them.
    outer = []
    for rect in rects:
        inside = any(
            other[1] <= rect[1] and other[2] >= rect[2]
            and other[0] <= rect[0] and other[0] + other[3] >= rect[0] + rect[3]
            for other in rects if other is not rect
        )
        if not inside:
            outer.append(rect)
    return outer


def groups_for(png_path):
    """The group rects as (y0, y1) pairs — what the classifier works on."""
    rects = group_rects(png_path)
    if rects is None:
        return None
    return sorted((y0, y1) for _x, y0, y1, _w in rects)


def page_columns(png_path):
    """The columns the PAGE occupies, in pixels.

    **The rail is not the page.** The nav rail's labels run the whole height
    of the window, so a scan of the whole picture finds ink on every row and
    every band merges into one — the check then reported "0 within, 0
    between — nothing to compare" and skipped every page of the fifteen. A
    check that measures the rail measures nothing, and the thing it was
    written to catch (spacing that carries the grouping) was therefore never
    checked at all.

    So the scan is limited to the columns the page's own groups occupy, plus
    a margin for the title and the description, which sit beside the cards
    rather than inside them.
    """
    rects = group_rects(png_path)
    if not rects:
        return None
    # Eight pixels, not sixty-four: the rail's own edge is a vertical line
    # of grey against the page's white, and a margin wide enough to reach it
    # puts ink on every row of the picture again — which is the thing this
    # scan was narrowed to avoid. The cards start at the content pane's
    # inset, so the page's title and description are in range already.
    margin = 8
    left = min(r[0] for r in rects) - margin
    right = max(r[0] + r[3] for r in rects) + margin
    return (max(0, left), right)


def classify(merged, groups):
    """Split the page's spacing into within-group and between-group distances.

    **BETWEEN is the span from the last ink inside one group to the first ink
    inside the next**, not the empty space between the two frames.

    That distinction is not pedantry. A page is laid out as
    `[cap] [card] [cap] [card]` — a section cap sits BETWEEN two cards and
    belongs to neither. Measuring the empty space between the frames therefore
    finds two short gaps around that cap and classifies both as "outside every
    group", which is exactly what the first run did: 22 gaps within, ZERO
    between, on a page with two large groups plainly separated by a wide band
    of air. A section cap is the most visible thing in the gap, so it is the
    thing a reader measures across.

    WITHIN is every gap whose two ends sit inside one group.

    Returns `(within, between, outside)` in pixels.
    """
    if len(groups) < 2:
        # One group is a page with nothing to separate. Reporting a ratio here
        # would mean comparing a gap to itself.
        within = []
        for y0, y1 in groups:
            for i in range(1, len(merged)):
                if merged[i - 1][1] >= y0 and merged[i][0] <= y1:
                    within.append(merged[i][0] - merged[i - 1][1])
        return within, [], []

    starts = [b[0] for b in merged]
    ends = [b[1] for b in merged]
    within, between, outside = [], [], []

    # Overlapping groups mean the tree's nesting and the ink disagree — a
    # table's rows sit inside a group whose frame the tree reports differently
    # from the one that drew. `core.commands` produced a BETWEEN distance of
    # -98pt, which is not a tight gap, it is the classifier walking past its
    # own boundary. A page it cannot order is a page it cannot measure.
    for a, b in zip(groups, groups[1:]):
        if b[0] < a[1]:
            return [], [], []

    for i in range(len(groups)):
        y0, y1 = groups[i]
        for j in range(1, len(merged)):
            if merged[j - 1][1] >= y0 and merged[j][0] <= y1:
                within.append(merged[j][0] - merged[j - 1][1])

    for i in range(len(groups) - 1):
        last = max((e for e in ends if e <= groups[i][1]), default=None)
        nxt = min((s for s in starts if s >= groups[i + 1][0]), default=None)
        if last is not None and nxt is not None:
            between.append(nxt - last)

    first_ink = starts[0] if starts else 0
    if first_ink < groups[0][0]:
        outside.append(groups[0][0] - first_ink)

    return within, between, outside


def main(paths):
    failed, missing_sidecar = [], []
    for png in sorted(paths):
        layout = groups_for(png)
        if layout is None:
            # Not a skip to report twice: a page with no sidecar was never
            # rendered by a shell that writes one, which is a different
            # situation from a page that was rendered and cannot be measured.
            missing_sidecar.append(png)
            continue

        merged = ink_bands(png, x_range=page_columns(png))
        within, between, _outside = classify(merged, layout)

        if not within or not between:
            # WHY it skipped, because "0 within, 0 between" reads like a
            # measurement of nothing where the truth is that the page has
            # nothing to measure: one group is a page with nothing to
            # separate, and a ratio there would compare a gap to itself.
            why = ("one group — nothing to separate"
                   if len(layout) < 2 else
                   f"{len(within)} within, {len(between)} between — nothing to compare")
            print(f"  skip {os.path.basename(png)}: {why}")
            continue

        w = statistics.mean(within) / 2
        b = statistics.mean(between) / 2
        ratio = b / w if w else 0.0
        ok = ratio >= MIN_RATIO
        print(f"  {'ok  ' if ok else 'FAIL'} {os.path.basename(png)}: "
              f"within {w:.0f}pt, between {b:.0f}pt, ratio {ratio:.2f}x "
              f"(needs >= {MIN_RATIO}x)")
        if not ok:
            failed.append(png)

    for png in missing_sidecar:
        print(f"  skip {os.path.basename(png)}: no .layout.json sidecar — "
              f"re-render with ./scripts/screenshots.sh")

    if failed:
        print(f"\n  {len(failed)} page(s) where spacing does not separate the roles:")
        for png in failed:
            print(f"    {png}")
        print("\n  A group draws no box — that is the design. So the gap IS the grouping.")
        print("  Below the floor, the page is a list, not a pane.")
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))