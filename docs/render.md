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

## Buttons and checkboxes draw; a table's checkboxes do not

An `NSButton`'s own `draw(_:)` returns nothing — which is why every rendered
page had its labels and none of its buttons. `cell.draw` is the path the
control actually takes, and it produces real pixels: **2041 bytes** for one
button against **903** for a blank frame of the same size. Buttons and the
checkboxes outside tables render, and the Safety page's PANIC STOP, its two
companions and the trial's own button are all in the picture.

**A checkbox inside a table's cell does not.** The collector finds the
checkboxes — measured, eleven of them at a 44x26 frame in the right column,
each piece a bitmap with real content — and the composite does not land them.
Thirteen attempts at this, each ruling out one theory, and the ones that
survived measurement are the ones worth recording:

| Tried | Ruled out by |
|---|---|
| clip the cell to its own rect | a clip on a cell's drawing clips the cell's content |
| draw the cells before the page | a cell paints an opaque background and erases it |
| one bitmap per cell, composited | the same bug in a longer dress |
| `rowViews` | **not a property of `NSTableView`** — compiles, evaluates to nothing |
| `reloadData` / `layoutSubtreeIfNeeded` / `sizeToFit` | `rowView(atRow:)` was returning the rows all along |
| scale the dest rect | the piece is already at `scale`; scaling again drew it 2x |
| zero the canvas origin | the origin was already the page's |

The last measurement is the honest one: the same button converts to
`(753, 444)` in the page view and `(1003, 444)` in the window — 250pt apart,
which is the sidebar's width. The rects are right in the window and the
canvas is drawn from the page view's own origin, and the two have not been
reconciled. The button is in the running app, in the right column, at the
right size; it is not in this tool's picture.

## What this tool cannot see

- a table cell's own control
- the window chrome — title bar, traffic lights, split divider
- anything that composites against the window server rather than drawing

None of those are the app's problem. They are the tool's, and they are the
reason `--shots` is a measurement instrument and not the specification.

## What the measurement tool was

`ls -la` in this shell reports the wrong size for every file. A page that
`ls` called 114 bytes was 162,897 — the whole of the "the render is blank"
conclusion, repeated for hours, was a conclusion about `ls`. `python3 -c
"import os; print(os.path.getsize(p))"` is the tool that works, and
`ShellRenderer.capture` logs the count it wrote for exactly this reason.

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
