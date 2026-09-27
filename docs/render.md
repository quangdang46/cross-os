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

## Controls: buttons and checkboxes yes, table cells no

A button draws through its **cell**. `btn.draw(bounds)` produces nothing;
`btn.cell?.draw(withFrame: btn.bounds, in: view)` produces 2041 bytes of real
pixels where a blank frame of the same size is 903. `--shots` walks the tree
calling the second form, drawing each cell into its own bitmap and compositing
it back at the frame the control occupies.

Two things that is not:

- **A cell inside a table's row is skipped.** An `NSTableRowView` reports the
  frame of the ROW, so a checkbox in the last column drew a 700pt-wide box at
  the row's own x — a stripe down the middle of the table, sitting exactly
  where the scrollbar is. A table's rows come through `dataWithPDF` instead,
  which is why the matrix's chords and actions are legible and its column of
  checkboxes is not drawn by this file. The running app draws them; AppKit
  does.
- **`NSTableView` itself does not draw through this path.** Its cell draws its
  scroller chrome over its own rows.

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
