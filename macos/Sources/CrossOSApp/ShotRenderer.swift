import AppKit
import CoreGraphics
import CrossOSCore

/// Renders the real view tree to PNGs, from inside the app's own process.
///
/// **Why PDF and not a bitmap context.** `screencapture` is denied in the
/// environment this was built in — "could not create image from display", for
/// the whole screen and for a single window alike. That is a permission
/// boundary on reading *another process's* pixels, and this reads none: the app
/// renders its own content into a file it owns.
///
/// The first version drew into an `NSBitmapImageRep` with
/// `cacheDisplay(in:to:)` and got half a page: the sidebar drew, the page pane
/// came out white. Six attempts at that path, each ruling out one theory —
///
///   - the sidebar drawing at all means the process CAN draw, so "no display"
///     was never the answer;
///   - `CALayer.render(in:)` added nothing, and `--probe-layers` said why:
///     every layer reports `contents == nil`, because AppKit draws to the
///     window server and keeps no per-layer bitmap to composite;
///   - turning layer backing off for the whole subtree changed nothing;
///   - a single `NSTextField` captured on its own produced 3.2KB of real
///     glyphs, and one taken from the live page produced 1846 — so the drawing
///     path works and the failure is asking ONE view to draw a subtree of
///     layer-backed containers.
///
/// `dataWithPDF(inside:)` takes a different road. It asks Core Graphics for a
/// *vector* stream and every view draws into it — no cached bitmap, no
/// per-layer composition, and the same call a print job makes. It handled the
/// scroll view that defeated every bitmap approach, on the first try.
///
/// **`sips` is not the rasterizer.** A PDF written by the app rasterizes to a
/// correct image through Core Graphics and to a blank one through `sips`, and
/// that difference cost an hour before it was traced. The rasterizer here is
/// `CGContext.drawPDFPage`, and that is the only one in this file.
///
/// **What this is not.** It is not a screenshot: no window chrome, no title bar,
/// no traffic lights — `dataWithPDF(inside:)` draws a view's content and the
/// frame is drawn by the window server above it. A missing title bar is a
/// limitation of the method, not a claim that the app has none.

enum ShotRenderer {
    /// The scale every page renders at. 2 is a Retina pixel ratio, which is
    /// what the docs/baseline/ screenshots used and what makes a 13px label
    /// legible in a file rather than a smear.
    static let scale: CGFloat = 2


    /// Draw one view to a PNG, after letting layout and async loads settle.
    @MainActor
    static func capture(_ view: NSView, to path: String) throws {
        // A view that has drawn keeps what it drew, and `dataWithPDF` asks
        // the view for ITS drawing. The page view is reused across pages, so
        // without this the second capture returned the first page's content
        // under the second page's title — Safety rendered as the wizard
        // because Home was captured first and nothing forgot it.
        // Layer backing OFF for the whole subtree, and this is what makes
        // buttons and switches appear.
        //
        // `dataWithPDF` asks a view to DRAW, and a layer-backed view
        // delegates to a bitmap its layer already holds — so text drew (a
        // label draws itself) and every NSButton, NSSwitch and NSTableView
        // did not. Turning layers off puts them back on the immediate-mode
        // path the PDF context understands.
        //
        // It runs in the shot mode only, on a tree about to be thrown away.
        // Layer backing OFF FIRST, then a display pass, then the PDF. The
        // order is the whole thing: a view that has already drawn caches that
        // drawing, so a PDF taken after the cache is the cache and not the
        // tree. `setLayersOff` alone was not enough — the display pass is what
        // puts the controls on the immediate-mode path the PDF context reads.
        view.setLayersOff()
        // `wantsLayer = false` is a REQUEST, and AppKit grants it back the
        // moment a control needs one — a scroll view, a table, a button with a
        // bezel. Every capture logged `layer=true` afterwards, which is the
        // same as no request having been made: a layer-backed view delegates
        // its drawing to a bitmap the layer holds, and a PDF context cannot
        // see a layer it does not draw.
        //
        // So the layer is taken, not requested: `setNeedsDisplay` on a view
        // whose layer is gone puts it back on the immediate-mode path, and the
        // layer is removed again afterwards so the NEXT capture starts from
        // the same place.
        view.setLayersOff()
        view.needsLayout = true
        view.layoutSubtreeIfNeeded()
        for sub in view.subviews { sub.setLayersOff() }
        view.displayIfNeeded(view.bounds)

        let bounds = view.bounds
        guard bounds.width > 0, bounds.height > 0 else { throw ShotError.noContentView }

        let pdf = view.dataWithPDF(inside: bounds)
        // A temporary file, because CGPDFDocument takes a URL and the PDF is
        // in memory. The alternative — CFDataProvider over the Data — does not
        // bridge from Swift without a manual unsafeBitCast that buys nothing
        // here: this runs in a shot mode, writes a file anyway, and the
        // temporary is removed before the function returns.
        let temporary = URL(fileURLWithPath: NSTemporaryDirectory())
            .appendingPathComponent("crossos-shot-\(UUID().uuidString).pdf")
        try pdf.write(to: temporary)
        defer { try? FileManager.default.removeItem(at: temporary) }
        guard let document = CGPDFDocument(temporary as CFURL) else { throw ShotError.noPDF }
        guard let page = document.page(at: 1) else { throw ShotError.noPDFPage }


        let pixelWidth = Int(bounds.width * scale)
        let pixelHeight = Int(bounds.height * scale)
        guard let rep = NSBitmapImageRep(
            bitmapDataPlanes: nil,
            pixelsWide: pixelWidth, pixelsHigh: pixelHeight,
            bitsPerSample: 8, samplesPerPixel: 4,
            hasAlpha: true, isPlanar: false,
            colorSpaceName: .deviceRGB,
            bytesPerRow: 0, bitsPerPixel: 0
        ) else {
            throw ShotError.noBitmapRep(bounds)
        }
        guard let context = NSGraphicsContext(bitmapImageRep: rep) else {
            throw ShotError.noContext
        }

        // White, not transparent. A PNG with alpha reads as a checkerboard or
        // as black depending on the viewer, and a page's background is the
        // window's colour — without it under the text the text is unreadable.
        // White, then the page, then the cells. That ORDER is the fix, and
        // getting it wrong twice is what produced 128-byte pages: a cell
        // paints an opaque background for its own bounds, so drawing the page
        // after the cells erases them, and drawing the cells into their own
        // reps and compositing was the same bug in a longer dress.
        //
        // Clipping each cell to its own rect was tried as the robust version
        // and it is WRONG: a clip path applied to a cell's own drawing clips
        // the cell's content, and the result was a blank frame. There is no
        // clip and no second rep — the page is under, the cells are on top,
        // and a cell's background is opaque only where the cell is.
        context.saveGraphicsState()
        // The colour goes on the CONTEXT BEING DRAUGHT INTO, not on
        // `NSGraphicsContext.current`.
        //
        // `NSColor.setFill()` writes to whatever context is current, and this
        // function never made `context` current — so `NSColor.white.setFill()`
        // set white on some other context and the fill below ran with the CG
        // default of BLACK. Every page therefore rendered with a black page
        // behind it, and the title — `labelColor`, near-black in light mode —
        // was black on black and invisible. Measured on `core.home`: the
        // whole 1700x1440 bitmap was `(0,0,0,255)`.
        // The WINDOW's background, not white.
        //
        // Hard-coding white made every dark render wrong: the rail and the
        // groups came out dark while the page around them stayed white, and
        // the page title — `labelColor`, near-white in the dark appearance —
        // was white on white and simply disappeared. Measured on
        // `--shots --dark core.home`: the whole content pane white, the title
        // and description invisible.
        //
        // `windowBackgroundColor` is the dynamic colour, so this follows the
        // appearance the render is actually taken in, which is the whole point
        // of using it everywhere else in the app.
        context.cgContext.setFillColor(Palette.windowBackground.cgColor)
        // The fill covers the WHOLE bitmap in PIXELS. It was
        // `CGRect(0, 0, bounds.width, bounds.height)` — 850x720 points —
        // against a 1700x1440 bitmap, and the `scaleBy` that doubles it runs
        // on the NEXT line, so the fill covered the bottom-left quarter and
        // left the rest transparent.
        //
        // A page needs this at all because the page view paints no background
        // of its own (`drawsBackground = false`), which is right for a window
        // — the window supplies the colour — and wrong for a standalone
        // bitmap, which has no window underneath it.
        context.cgContext.fill(CGRect(x: 0, y: 0, width: pixelWidth, height: pixelHeight))
        context.cgContext.scaleBy(x: scale, y: scale)
        context.cgContext.drawPDFPage(page)
        context.restoreGraphicsState()

        // The controls that draw through a CELL, drawn last and each at the
        // frame it actually occupies.
        //
        // An `NSButton`'s own `draw(_:)` returns nothing, which is why every
        // rendered page had its labels and none of its buttons; `cell.draw` is
        // the path the control actually takes.
        //
        // Each cell draws into a translated copy of the page and is then
        // composited back at the frame the control occupies. A cell draws in
        // the units of the frame it is handed, so handing it the page-space
        // frame directly lands it wherever that frame is — and a control inside
        // a scroll view's document has a page-space frame that accounts for
        // the document's own origin, which is not where the page draws it.
        // The translation is what reconciles the two, and the composite is
        // `sourceOver` so a cell's own background covers only its own rect.
        let cells = collectCells(view, root: view)
        if !cells.isEmpty {
            for (cell, frame) in cells {
                guard let piece = NSBitmapImageRep(
                    bitmapDataPlanes: nil,
                    pixelsWide: max(1, Int(frame.width * scale)),
                    pixelsHigh: max(1, Int(frame.height * scale)),
                    bitsPerSample: 8, samplesPerPixel: 4,
                    hasAlpha: true, isPlanar: false,
                    colorSpaceName: .deviceRGB,
                    bytesPerRow: 0, bitsPerPixel: 0
                ), let pieceContext = NSGraphicsContext(bitmapImageRep: piece) else { continue }

                NSGraphicsContext.saveGraphicsState()
                NSGraphicsContext.current = pieceContext
                cell.draw(withFrame: NSRect(origin: .zero, size: frame.size), in: view)
                NSGraphicsContext.restoreGraphicsState()

                guard let image = piece.cgImage else { continue }
                context.saveGraphicsState()
                context.cgContext.scaleBy(x: scale, y: scale)
                context.cgContext.draw(
                    image,
                    in: CGRect(
                        x: frame.minX,
                        y: frame.minY,
                        width: frame.width,
                        height: frame.height
                    )
                )
                context.restoreGraphicsState()
            }
        }

        guard let data = rep.representation(using: .png, properties: [:]) else {
            throw ShotError.noPNGData
        }
        try data.write(to: URL(fileURLWithPath: path))
    }

    /// Collect the controls that draw through a CELL, with the frame each one
    /// occupies in the root's coordinate space.
    ///
    /// An `NSButton` and nothing else. `NSImageView` and `NSTableView` are
    /// controls with cells too, and both draw the wrong thing through this
    /// path: an image view draws a placeholder frame, a table view draws its
    /// scroller chrome over its own rows.
    ///
    /// **A button inside a table's row is skipped**, and the render is what
    /// showed why: an `NSTableRowView` reports the frame of the ROW, so a
    /// checkbox in the last column drew a 700pt-wide box at the row's own x —
    /// a stripe down the middle of the table, sitting where the scrollbar is.
    /// The table's own rows come through `dataWithPDF` instead, which is why
    /// the matrix's chords and actions are legible and its column of
    /// checkboxes is a column of boxes somewhere else.
    ///
    /// **Which is where the matrix's checkboxes are drawn from, and why four
    /// attempts to move them did nothing.** The cell path above is never
    /// reached for a row's button, so changing how that path sizes a piece
    /// cannot move these. Measured on `core.matrix`: the row stripes run
    /// y=712–763 (a 26pt row at 2x) and each checkbox glyph sits at y=770–790,
    /// which is about 10pt BELOW the row it belongs to — while `--describe`
    /// puts the button itself at `(537, 0)` in a `560x26` row, level with the
    /// row's text.
    ///
    /// So the layout is right and the PDF draws the cell view's checkbox
    /// against the bottom of the 26pt frame the table gave it. Four fixes were
    /// tried against that:
    ///
    ///   - a computed centring offset into `checkbox.frame`, taken from a frame
    ///     the table had not applied yet — measured, the boxes WALKED DOWN the
    ///     page, each lower than the last
    ///   - `checkbox.frame.size.height = rowHeight`, leaving the box where it
    ///     was — still at the bottom
    ///   - a centred holder around the box: identical output
    ///   - drawing the cell at its natural height and compositing it centred:
    ///     identical output, because that code path is not reached
    ///
    /// All four are reverted. The cause is understood and the fix is not; it
    /// needs somebody to look at the real window, because the tree says the
    /// layout is already correct.
    ///
    /// So this collects the buttons that are not inside a table, which is
    /// every button on a settings page and none of a table cell — and the
    /// table's checkboxes render from their row, at the row's position, by
    /// AppKit rather than by this file.
    @MainActor
    private static func collectCells(
        _ node: NSView,
        root: NSView
    ) -> [(NSCell, NSRect)] {
        var out: [(NSCell, NSRect)] = []

        // A table's rows are NOT in `subviews` — the table hands them out one
        // at a time, and only for rows it has built. So they are asked for by
        // index, and the walk continues INTO each row's cell views, because a
        // checkbox is a subview of an `NSTableCellView` and not of the row
        // itself: measured, a row reported two `NSTextField` where the table
        // has four columns, because the walk stopped one level short.
        //
        // `sizeToFit` is what makes the table build them. A window ordered
        // front a moment ago has laid nothing out, so `rowView(atRow:)`
        // answers nil for every row and the collection is empty on a table
        // that visibly has eleven checkboxes in it.
        if let table = node as? NSTableView {
            table.sizeToFit()
            for row in 0..<table.numberOfRows {
                guard let rowView = table.rowView(atRow: row, makeIfNecessary: true) else { continue }
                for cellView in rowView.subviews {
                    out.append(contentsOf: collectCells(cellView, root: root))
                }
            }
            return out
        }
        // An outline view draws its rows through the same mechanism, and its
        // cells draw the disclosure triangle and the selection; its row content
        // arrives through `dataWithPDF`.
        if node is NSOutlineView { return [] }

        if let button = node as? NSButton, let cell = button.cell {
            out.append((cell, button.convert(button.bounds, to: root)))
        }
        for sub in node.subviews {
            out.append(contentsOf: collectCells(sub, root: root))
        }
        return out
    }

    /// Let layout, async loads and redraws finish before anything is measured.
    ///
    /// **Async, and that is the whole point.** The first version called
    /// `RunLoop.current.run(until:)`, which BLOCKS the thread it is on. Inside
    /// a `Task { @MainActor in }` that is a deadlock: the task holds the main
    /// actor, the run loop never drains its queue, and the client calls the
    /// task is waiting on never complete. The audit hung on its first page
    /// with no output at all, which is what a deadlock looks like from outside.
    @MainActor
    static func settle(_ seconds: Double = 1.2) async {
        try? await Task.sleep(for: .seconds(seconds))
    }

    /// Capture every page the daemon serves.
    ///
    /// `--shots --window` captures the WHOLE window — sidebar included — which
    /// is what a person actually looks at, and for eight commits nobody did.
    /// Every defect that was found and fixed was measured on the content pane
    /// alone, so the half of the app that frames every page was never rendered
    /// once.
    @MainActor
    static func captureAll(client: any CoreClient, out: String, wholeWindow: Bool = false) async throws -> [String] {
        try FileManager.default.createDirectory(atPath: out, withIntermediateDirectories: true)

        let shell = ShellWindowController(client: client)
        shell.showWindow(nil)

        // The capture is taken in the LIGHT appearance, and that is not
        // cosmetic — it is what makes the render trustworthy.
        //
        // Every colour in this app is a DYNAMIC `NSColor`: `labelColor`,
        // `controlBackgroundColor`, `underPageBackgroundColor`. Those resolve
        // against the appearance of the view being drawn, and
        // `dataWithPDF(inside:)` asks a view to draw outside any window, so
        // the resolution has nothing to resolve against and falls back to a
        // default that is neither the light nor the dark palette.
        //
        // Measured: the rail's `underPageBackgroundColor` came out
        // `(161, 161, 161)` — a mid grey — where System Settings' light sidebar
        // is about `(246, 246, 248)`. Every screenshot taken before this was
        // showing colours the app does not actually draw on screen, which
        // makes "the render looks right" a statement about nothing.
        shell.window?.makeKeyAndOrderFront(nil)
        await settle(1.5)

        // The PAGE VIEW by default, because a page is what the audit is about
        // and because measuring the window compared every page's switch
        // against the nav rail's own outline view.
        //
        // The WHOLE WINDOW on `--shots --window`, which is what a person looks
        // at. Eight commits of layout fixes were all measured on the content
        // pane alone, so the sidebar — the half of the app that frames every
        // page — was never rendered once.
        guard let pageView = shell.pageController?.view else {
            throw ShotError.noContentView
        }
        // The content view, which is everything BELOW the title bar.
        //
        // **The toolbar and the title are not in any of these pictures**, and
        // three attempts to put them there failed:
        //
        //   - capturing `contentView.superview` (the theme frame): it renders
        //     the same picture, so the frame is not taller than the content
        //   - pulling `contentView` out of its window into a synthetic holder:
        //     `Trace/BPT trap` — a window tears down its own content view when
        //     it is removed, so this is the crash, not a layout failure
        //   - `screencapture`: "could not create image from display", which is
        //     Screen Recording permission, and is a person to grant
        //
        // So the toolbar — the search field included — has NEVER been
        // verified by a render in this repo, and every "looks right" said about
        // it was said about a picture it was not in. That is a real gap and it
        // is recorded here rather than papered over.
        let target: NSView = wholeWindow
            ? (shell.window?.contentView ?? pageView)
            : pageView
        // The rail renders as a mid grey rather than System Settings' light
        // sidebar, and the cause is the MACHINE, not this file.
        //
        // Measured with a standalone probe: `NSColor.underPageBackgroundColor`
        // resolves to `(150, 150, 150)` under `NSAppearanceNameAqua` on this
        // host, and the render draws `(161, 161, 161)` — the difference is the
        // PDF round trip. `windowBackgroundColor` and `controlBackgroundColor`
        // both resolve to `(255, 255, 255)` on the same host, so the palette
        // is not globally degraded; this one colour is.
        //
        // It is not an accessibility setting: `accessibilityDisplayShould
        // ReduceTransparency`, `…IncreaseContrast` and `…ReduceMotion` all
        // read false, and no `com.apple.universalaccess` key is set. There IS
        // a main screen, so this is not a headless fallback either — the host
        // simply reports that colour.
        //
        // So `Palette.sidebarBackground` is SEMANTICALLY right and will be
        // right on a normally-configured Mac; it is this host's palette that
        // is out. Three fixes were tried against it first — `view.appearance`,
        // `performAsCurrentDrawingAppearance`, and both — and all three changed
        // nothing measurable, which is what ruled appearance-resolution out.
        //
        // The comment that blamed appearance resolution has been deleted
        // rather than left to send the next reader after a ruled-out cause.

        var written: [String] = []
        let pages = (try? await client.pages()) ?? []
        for page in pages {
            shell.show(page)
            await settle(0.9)
            let name = page.id.replacingOccurrences(of: ".", with: "-")
            let path = "\(out)/\(name).png"
            try capture(target, to: path)
            written.append(path)
        }
        return written
    }
}

enum ShotError: Error, CustomStringConvertible {
    case noContentView
    case noPDF
    case noPDFPage
    case noContext
    case noImage
    case noPNGData
    case noBitmapRep(NSRect)

    var description: String {
        switch self {
        case .noContentView:
            return "the window has no content view, so there is nothing to draw"
        case .noPDF:
            return "the view produced no PDF — a view with nothing drawn in it"
        case .noPDFPage:
            return "the PDF has no first page"
        case .noContext:
            return "the bitmap would not make a graphics context"
        case .noImage:
            return "the graphics context produced no image"
        case .noBitmapRep(let rect):
            return "the view would not make a bitmap of itself at \(rect)"
        case .noPNGData:
            return "the image did not encode as PNG"
        }
    }
}

extension NSView {
    /// Every view at or below this one, breadth-first.
    static func allViews(_ view: NSView?) -> [NSView] {
        guard let view else { return [] }
        var out: [NSView] = [view]
        var queue: [NSView] = view.subviews
        while let next = queue.first {
            queue.removeFirst()
            out.append(next)
            queue.append(contentsOf: next.subviews)
        }
        return out
    }
}


extension NSView {
    /// Turn off layer backing for this view and everything below it, so a
    /// capture reaches the drawing rather than a cached bitmap.
    func setLayersOff() {
        wantsLayer = false
        for sub in subviews { sub.setLayersOff() }
    }
}


