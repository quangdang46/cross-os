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
        NSColor.white.setFill()
        context.cgContext.fill(CGRect(x: 0, y: 0, width: bounds.width, height: bounds.height))
        context.cgContext.scaleBy(x: scale, y: scale)
        context.cgContext.drawPDFPage(page)
        context.restoreGraphicsState()


        guard let data = rep.representation(using: .png, properties: [:]) else {
            throw ShotError.noPNGData
        }
        try data.write(to: URL(fileURLWithPath: path))
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
    @MainActor
    static func captureAll(client: any CoreClient, out: String) async throws -> [String] {
        try FileManager.default.createDirectory(atPath: out, withIntermediateDirectories: true)

        let shell = ShellWindowController(client: client)
        shell.showWindow(nil)
        shell.window?.makeKeyAndOrderFront(nil)
        await settle(1.5)

        // The PAGE VIEW, not the window's content view: the content view is
        // the whole window including the sidebar, and a page is what this is a
        // picture of. The audit runs on the same view for the same reason —
        // measuring the window compared every page's switch against the nav
        // rail's own outline view, which cannot happen to a person.
        guard let pageView = shell.pageController?.view else {
            throw ShotError.noContentView
        }

        var written: [String] = []
        let pages = (try? await client.pages()) ?? []
        for page in pages {
            shell.show(page)
            await settle(0.9)
            let name = page.id.replacingOccurrences(of: ".", with: "-")
            let path = "\(out)/\(name).png"
            try capture(pageView, to: path)
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
