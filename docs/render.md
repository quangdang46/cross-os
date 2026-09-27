# What a render can and cannot show

`CrossOS --shots` draws the real view tree to a PNG per page, from inside the
app's own process. Fifteen pages, no Wails, no webview, no React.

It exists because `screencapture` is denied in the environment this was built
in — "could not create image from display", for the whole screen and for a
single window alike. That is a permission boundary on reading **another
process's** pixels, and `--shots` reads none: the app renders its own content
into a file it owns, through `NSView.dataWithPDF(inside:)` and a Core Graphics
rasterizer. It is the same call a print job makes.

## What it shows

Text, card fills and borders, SF Symbols, the wizard's step numbers, the
readiness rows, the colour of a state chip, the position of a row, whether two
controls overlap, whether a label wrapped.

That is enough to have found, and these were all real:

- **Twenty controls across fifteen pages rendered as holes.** Every card row
  measured 0pt tall, so its content was laid out at y=-162 — below its own
  card, outside the clip view. The frame said 162pt and was right; the content
  was outside the frame and nothing said so.
- **Safety rendered Home's wizard above its own three buttons**, because
  `rebuild` cleared the page stack to a count of three and the page id is the
  third chrome view, added at the *end* of the method.
- **A row at 26pt** where System Settings uses 30, and a selected row in the
  deprecated `.sourceList` style — a solid accent bar, the 2010 look.

## What it does not show

`NSButton`, `NSSwitch` and `NSTableView` — and the reason is measured rather
than assumed.

A button draws through its **cell**. `btn.draw(bounds)` produces nothing;
`btn.cell?.draw(withFrame: btn.bounds, in: view)` produces 2041 bytes of real
pixels where a blank frame of the same size is 903. So the path exists and
`--shots` walks the tree calling it.

Then it stops, and the reason is the scroll view:

| | button renders |
|---|---|
| button in a plain view | **yes** — 2804 bytes, bezel, label, correct position |
| button inside an `NSScrollView` | **no** — 1046 bytes, blank |

Every page in this app is a page inside a scroll view, so every page is in the
second row. The scroll view is layer-backed, `wantsLayer = false` is a request
rather than a command, AppKit grants it back for any control that needs one,
and every capture logs `layer=true` afterwards — which is the same as the
request never having been made.

Three things were tried and measured, all recorded here so the next person does
not re-run them:

- **Clip each cell to its own rect.** Wrong: a clip applied to a cell's own
  drawing clips the cell's content, and the result is a blank frame.
- **Draw cells after the page.** A cell paints an opaque background for its
  own bounds, so this erases the page beneath it — 83KB down to 128 bytes.
- **Give each cell its own bitmap and composite.** The same bug in a longer
  dress: compositing a cell over the page with the cell's own opaque
  background still replaces what is under it.

**This is a limit of the method, and the controls are not broken.** The
running app draws them with the system's own focus ring, press animation,
disabled appearance and HiDPI rendering — none of which a drawn replacement
would have, and all of which `NSButton` has for free. Replacing a system
control with a hand-drawn view so a screenshot can see it trades the
product's quality for the tool's convenience, and that trade is never worth
making.

It is also why the shell uses a checkbox rather than an `NSSwitch` where a
row has a label and a sentence: a checkbox draws (measured, 753 bytes of
pixels), and it is what such a row uses anyway. `NSSwitch` is 31x19 and reads
better in a narrow pane.

## The rule

**`--shots` is a measurement instrument, not the specification.** It is right
about geometry, colour and text, and silent about the controls people click.
Those are confirmed by running the app:

```bash
./scripts/run.sh              # the real window
cd macos && swift run CoreTests
cd macos && ./.build/debug/CrossOS --audit     # exits non-zero on a FAIL
cd macos && ./.build/debug/CrossOS --describe  # the live view tree
cd macos && ./.build/debug/CrossOS --geometry  # the switcher's arithmetic
```

`--audit` is the one that gates a build. It measures contrast, rhythm, fit
and duplication arithmetically, so it needs no display and runs in CI against
a live daemon — and it is the check the React shell could not have: jsdom does
not load a stylesheet, so a green run there meant a page nobody had looked at.

## How the renderer got here

Six attempts at a bitmap context, each ruling out one theory, in the order they
happened:

| Attempt | What it ruled out |
|---|---|
| `cacheDisplay` on `contentView` | The sidebar drew and the page did not — so "no display" was never the answer |
| `cacheDisplay` on the page's view | Not the split view item |
| `CALayer.render(in:)` | `--probe-layers`: every layer reports `contents == nil`, because AppKit draws to the window server and keeps no per-layer bitmap |
| Layer backing off for the subtree | Not a layer-composition problem |
| `cacheDisplay` on a single `NSTextField` | 3.2KB of real glyphs — the drawing path works; the failure is asking one view to draw a subtree of layer-backed containers |
| `dataWithPDF(inside:)` | — it worked on the first try |

And one detail that cost an hour: **a PDF written by the app rasterizes
correctly through `CGContext.drawPDFPage` and blank through `sips`.** The PDF
was never wrong. `sips` is not the rasterizer for vector text.
