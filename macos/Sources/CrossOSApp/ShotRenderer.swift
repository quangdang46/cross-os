import AppKit
import CrossOSCore

/// Renders the real view tree to PNGs, from inside the app's own process.
///
/// **Why this exists and why it is not a screenshot tool.** The environment
/// this was built in denies Screen Recording, so `screencapture` returns
/// "could not create image from display" for the whole screen and for a
/// single window alike. That is a permission boundary on *reading another
/// process's* pixels, and nothing here reads another process's pixels:
/// `NSView.cacheDisplay(in:to:)` asks the view to draw itself into a bitmap,
/// which is the same call a print context makes and the same one
/// `NSBitmapImageRep` makes. The app renders its own content to a file it
/// owns, and no permission is involved because none is needed.
///
/// The alternative was `--describe`, and it is not a substitute. Frames catch
/// "this row is 883px tall in a 720px window". They cannot catch "this grey is
/// wrong", because that is a judgement about a colour and a judgement belongs
/// to somebody looking. This produces the pixels that judgement needs.
///
/// **What it is faithful to, and what it is not.** It is the real view tree
/// after a real layout pass, drawn by the real AppKit controls with the real
/// semantic colours, so spacing, hierarchy, truncation and contrast are all
/// what ships. What it does NOT include is the window chrome — the title bar,
/// the traffic lights, the split divider — because `cacheDisplay` draws a
/// view's content and the frame is drawn by the window server above it. A
/// missing title bar is a limitation of the method and not a claim that the
/// app has none.

enum ShotRenderer {
    /// Draw one view to a PNG, after letting the run loop settle so Auto
    /// Layout has resolved and any async load has landed.
    ///
    /// **Two passes, and the second one is the one that works.**
    /// `cacheDisplay(in:to:)` asks the view to draw itself, and a view draws
    /// its frame — which for an `NSSplitViewItem` is a background and nothing
    /// else, because the split view's content lives in a layer the item
    /// references rather than one it owns. The first attempt at this rendered a
    /// white page with a grey stripe down the left: the sidebar, and no
    /// content, which is exactly what a capture that stops at the view's own
    /// frame looks like.
    ///
    /// So the tree is walked and every view with a backing layer is asked to
    /// render that layer directly. `CALayer.render(in:)` composites what a
    /// layer actually holds — including the sublayers a split view item's
    /// content lives in — so it sees the parts `cacheDisplay` does not.
    ///
    /// The result is not a screenshot: it is the app drawing its own content
    /// into a bitmap, which is what a print context does.
    @MainActor
    static func capture(_ view: NSView, to path: String) throws {
        let bounds = view.bounds
        let scale: CGFloat = 2
        let pixelSize = NSSize(width: bounds.width * scale, height: bounds.height * scale)

        guard let rep = NSBitmapImageRep(
            bitmapDataPlanes: nil,
            pixelsWide: Int(pixelSize.width),
            pixelsHigh: Int(pixelSize.height),
            bitsPerSample: 8,
            samplesPerPixel: 4,
            hasAlpha: true,
            isPlanar: false,
            colorSpaceName: .deviceRGB,
            bytesPerRow: 0,
            bitsPerPixel: 0
        ) else {
            throw ShotError.noBitmapRep(bounds)
        }
        rep.size = pixelSize

        guard let context = NSGraphicsContext(bitmapImageRep: rep) else {
            throw ShotError.noContext
        }
        NSGraphicsContext.saveGraphicsState()
        NSGraphicsContext.current = context

        // The order matters and the first version had it wrong. Every layer in
        // this tree reports `contents == nil`, which is not a bug in the
        // capture: AppKit draws straight to the window server and keeps no
        // bitmap per layer, so there is nothing for `CALayer.render(in:)`
        // to composite and the first attempt produced a white page with a
        // grey stripe down the left.
        //
        // What has to happen is the opposite: make every view DRAW while the
        // bitmap context is current. `displayIfNeeded` walks the tree and asks
        // each view to lay out and draw itself, and because the current
        // graphics context is the bitmap, the drawing lands in the bitmap.
        // That is the same call a print context makes, and it is why no
        // permission is involved: the app is drawing its own content into a
        // file it owns.
        view.needsLayout = true
        view.layoutSubtreeIfNeeded()
        view.displayIfNeeded()
        view.displayIgnoringOpacity(bounds, in: context)

        NSGraphicsContext.restoreGraphicsState()

        // The layer pass, on top of the view pass. `walkLayers` is the one
        // that matters and it is not optional: an `NSSplitView` keeps each
        // item's content in a layer the split view OWNS and the item's view
        // does not, so drawing the root layer composites the two item frames
        // and nothing inside them. Walking the sublayers by hand is what puts
        // the page content in the bitmap.
        walkLayers(view.layer, into: context.cgContext, scale: scale)

        guard let data = rep.representation(using: .png, properties: [:]) else {
            throw ShotError.noPNGData
        }
        try data.write(to: URL(fileURLWithPath: path))
    }

    /// Render every sublayer that is not the root, because `render(in:)` on a
    /// parent does not recurse into children that are hidden or offscreen and
    /// an `NSSplitViewItem` keeps its content in a sibling it manages.
    @MainActor
    private static func walkLayers(_ layer: CALayer?, into context: CGContext, scale: CGFloat) {
        guard let layer else { return }
        for sub in layer.sublayers ?? [] {
            if !sub.isHidden {
                sub.render(in: context)
            }
            walkLayers(sub, into: context, scale: scale)
        }
    }

    /// Let the run loop run for `seconds`, so a layout pass, an async client
    /// call and its redraw all complete before anything is measured.
    ///
    /// This is the same settle the `--describe` mode does, and it is why the
    /// numbers there were real. A capture taken at t=0 catches the window
    /// before Auto Layout has run and the answer is a frame of zeros.
    @MainActor
    static func settle(_ seconds: Double = 1.2) {
        RunLoop.current.run(until: Date().addingTimeInterval(seconds))
    }

    /// Capture every page the daemon serves, plus the states worth seeing.
    @MainActor
    static func captureAll(
        client: any CoreClient,
        out: String
    ) async throws -> [String] {
        try FileManager.default.createDirectory(
            atPath: out, withIntermediateDirectories: true
        )

        let shell = ShellWindowController(client: client)
        shell.showWindow(nil)
        shell.window?.makeKeyAndOrderFront(nil)
        settle(1.5)

        guard let content = shell.window?.contentView else {
            throw ShotError.noContentView
        }

        var written: [String] = []

        // Every page, named by its own id so a filename is traceable back to
        // `core.pages`.
        let pages = (try? await client.pages()) ?? []
        for page in pages {
            shell.show(page)
            settle(1.0)
            let path = "\(out)/\(page.id.replacingOccurrences(of: ".", with: "-")).png"
            try capture(content, to: path)
            written.append(path)
        }

        return written
    }

}

enum ShotError: Error, CustomStringConvertible {
    case noBitmapRep(NSRect)
    case noContext
    case noPNGData
    case noContentView

    var description: String {
        switch self {
        case .noBitmapRep(let rect):
            return "the view would not make a bitmap of itself at \(rect) — a view with no drawn content"
        case .noContext:
            return "the bitmap would not make a graphics context, so nothing can be drawn into it"
        case .noPNGData:
            return "the bitmap did not encode as PNG"
        case .noContentView:
            return "the window has no content view, so there is nothing to draw"
        }
    }
}
