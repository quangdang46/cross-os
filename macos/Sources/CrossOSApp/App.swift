import AppKit
import CrossOSCore

// CrossOS.app — the entry point.
//
// `@main` on a struct rather than a `main.swift` top-level, so the delegate
// type is named and the app has a symbol rather than a pile of statements in a
// file called main.

@main
struct CrossOSApp {
    @MainActor
    static func main() async {
        let app = NSApplication.shared

        // `--describe` prints the view tree and exits. It exists because this
        // app has to be checkable without a screenshot: a window existing is
        // not evidence that it drew anything, and the previous shell's defects
        // were all invisible in a passing test suite and visible only by
        // looking. Printing the tree is looking, in a form CI can read.
        if CommandLine.arguments.contains("--probe-layers") {
            app.setActivationPolicy(.accessory)
            let delegate = AppDelegate()
            app.delegate = delegate
            app.finishLaunching()
            delegate.probeLayers()
            return
        }

        if CommandLine.arguments.contains("--audit") {
            // Top-level, NOT inside a Task. Swift's top-level `await` runs
            // the main actor with an implicit run loop, and that is the thing
            // a window and a socket read both need. An audit launched as
            // `Task { @MainActor in }` inside `app.run()` deadlocked on the
            // first page with no output at all: the task held the main actor
            // while the run loop that would have drained it had not started.
            //
            // That is worth writing down rather than fixing silently, because
            // the deadlock looks exactly like a slow build and the fix —
            // moving it — looks like giving up on the measurement.
            await AppDelegate().auditThenExit()
            return
        }

        if CommandLine.arguments.contains("--shots") {
            app.setActivationPolicy(.accessory)
            let delegate = AppDelegate()
            app.delegate = delegate
            app.finishLaunching()
            // The run loop is started rather than a bare `Task`, because
            // `main` returning tears the process down and an unstructured
            // task dies with it — which is why the first run exited 0 having
            // written nothing. `app.run()` plus a `finishLaunching` that
            // already happened is the arrangement that keeps it alive long
            // enough to draw, and the task calls `exit` when it is done.
            delegate.shotsThenExit()
            app.run()
            return
        }

        if CommandLine.arguments.contains("--geometry") {
            probeGeometry()
            exit(0)
        }

        if CommandLine.arguments.contains("--describe") {
            app.setActivationPolicy(.accessory)
            let delegate = AppDelegate()
            app.delegate = delegate
            app.finishLaunching()
            delegate.describeThenExit()
            return
        }

        let delegate = AppDelegate()
        app.delegate = delegate
        // `.regular` and `.accessory` are the two answers: a settings window
        // with a Dock tile and no menu-bar item, or a menu-bar app with no
        // window at all. CrossOS has a window people open, and the daemon is
        // the thing that runs whether or not it does, so it is regular.
        app.setActivationPolicy(.regular)
        app.run()
    }
}

@MainActor
final class AppDelegate: NSObject, NSApplicationDelegate {
    private var shell: ShellWindowController?
    private let client = LiveCoreClient()

    func applicationDidFinishLaunching(_ notification: Notification) {
        let shell = ShellWindowController(client: client)
        self.shell = shell
        shell.showWindow(nil)
        shell.startPolling()
        NSApp.activate(ignoringOtherApps: true)
    }

    /// Walk the real view tree and print it, then exit.
    ///
    /// After a real layout pass, so Auto Layout has resolved — a tree printed
    /// before the first layout pass shows frames of zero and proves nothing.
    func describeThenExit() {
        // The delegate's own `applicationDidFinishLaunching` has not run — the
        // run loop never started, which is the point of this mode — so `shell`
        // is nil. Build it here rather than starting the run loop and racing
        // it: a describe mode that sometimes has a window and sometimes does
        // not is a describe mode whose output cannot be asserted on.
        let shell = shell ?? ShellWindowController(client: client)
        self.shell = shell
        shell.showWindow(nil)
        shell.window?.makeKeyAndOrderFront(nil)

        let deadline = Date().addingTimeInterval(2.5)
        while Date() < deadline {
            RunLoop.current.run(mode: .default, before: Date().addingTimeInterval(0.05))
        }
        if let content = shell.window?.contentView {
            print("TREE")
            ViewTree.describe(content, indent: 0)
            if CommandLine.arguments.contains("--explain-width") {
                print("\nWIDTH")
                if let card = content.firstDescendant(ofType: NSBox.self) {
                    ViewTree.explainWidth(of: card, in: content)
                }
            }
        }
        exit(0)
    }

    /// Measure the design properties a screenshot would show, and print them.
    ///
    /// This is what replaces looking. The renderer needs a display and this
    /// machine has none, but the four things a screenshot is good for —
    /// contrast, rhythm, fit and duplication — are all arithmetic, and the
    /// arithmetic is exact where the eye is approximate.
    ///
    /// It is not a substitute. A window can pass everything here and still
    /// feel wrong, and nothing here can tell you the page is beautiful. It
    /// tells you the six ways a settings pane is usually broken, and this
    /// codebase has shipped all six.
    @MainActor
    func auditThenExit() async {
        let shell = ShellWindowController(client: client)
        self.shell = shell
        shell.showWindow(nil)
        shell.window?.makeKeyAndOrderFront(nil)
        await ShotRenderer.settle(1.2)

        // The design system's own findings, before any page. These are
        // properties of the PALETTE and the SCALE rather than of a page, so
        // they are measured once and printed under their own heading — they
        // are not a per-page finding and printing them as one would be a
        // number repeated fifteen times.
        let systemFindings = Audit.contrastMatrix() + Audit.rhythmAudit()
        if let pageView = shell.pageController?.view {
            print("SHOT CONSTRAINTS")
            for v in [pageView, pageView.firstCard(), pageView.firstCard()?.superview].compactMap({ $0 }) {
                print("  \(type(of: v)) frame=\(v.frame) huggingV=\(v.contentHuggingPriority(for: .vertical))")
                for c in v.constraints {
                    let a = (c.firstItem as? NSView).map { "\(type(of: $0))" } ?? "nil"
                    let b = (c.secondItem as? NSView).map { "\(type(of: $0))" } ?? "nil"
                    print("    \(c.relation.rawValue) \(a).\(c.firstAttribute.rawValue) -> \(b).\(c.secondAttribute.rawValue) p=\(c.priority.rawValue) active=\(c.isActive)")
                }
            }
        }

        var findings = systemFindings
        print("design system")
        AuditPrinter.emitPage("design system", findings: systemFindings)
        let pages = (try? await client.pages()) ?? []

        for page in pages {
            shell.show(page)
            await ShotRenderer.settle(0.9)
            // The CONTENT PANE, not the window. `contentView` is the whole
            // window including the sidebar, and the sidebar has an outline
            // view at x=0..255 — so auditing the window compared every
            // page's switch at x=244 against the nav rail's own scroll
            // view, which cannot happen to a person. The split view's second
            // item is the pane a page's controls actually live in.
            guard let pageView = shell.pageController?.view else { continue }
            let found = Audit.fitAudit(root: pageView, page: page.id)
                + Audit.duplicationAudit(root: pageView)
            findings += found
            // Per page, immediately. A hang on page fifteen must not cost
            // the report on pages one to fourteen.
            AuditPrinter.emitPage(page.id, findings: found)
        }

        print("")
        AuditPrinter.emitSummary(findings, pages: pages.count)
        exit(findings.contains { $0.severity >= .fail } ? 1 : 0)
    }

    /// Print the layer tree of the window, to find out where the pixels are.
    func probeLayers() {
        let shell = ShellWindowController(client: client)
        self.shell = shell
        shell.showWindow(nil)
        shell.window?.makeKeyAndOrderFront(nil)
        // Synchronous on purpose: this mode is not in a Task, so nothing is
        // waiting on an async call and spinning the run loop is safe. The
        // audit's settle is async for the opposite reason — it IS in a Task
        // and spinning there deadlocks.
        RunLoop.current.run(until: Date().addingTimeInterval(1.2))
        if let content = shell.window?.contentView {
            printLayerTree(content, depth: 0)
        }
        exit(0)
    }

    private func printLayerTree(_ view: NSView, depth: Int) {
        let pad = String(repeating: "  ", count: depth)
        let f = view.frame
        let layerInfo = view.layer.map { l in
            "subs=\(l.sublayers?.count ?? 0) hidden=\(l.isHidden) needsDisplay=\(l.needsDisplay)"
                + " contents=\(l.contents == nil ? "nil" : "set")"
        } ?? "layer=nil"
        print("\(pad)\(type(of: view)) \(Int(f.width))x\(Int(f.height))@\(Int(f.minX)),\(Int(f.minY)) \(layerInfo)")
        for sub in view.subviews {
            printLayerTree(sub, depth: depth + 1)
        }
    }

    /// Render every page to a PNG and exit.
    ///
    /// The app's own pixels, drawn by the app. See `ShotRenderer` for why
    /// this exists instead of `screencapture` — and for what it does not
    /// include, which is the window chrome.
    func shotsThenExit() {
        let out = CommandLine.arguments
            .first { $0.hasPrefix("--out=") }
            .map { String($0.dropFirst("--out=".count)) } ?? "/tmp/crossos-shots-swift"

        // The window is AppKit, so every touch of it is on the main actor,
        // and this method IS on the main actor — so the work happens in a
        // Task and the run loop above is what keeps that Task alive.
        Task { @MainActor in
            let shell = ShellWindowController(client: client)
            self.shell = shell
            shell.showWindow(nil)
            shell.window?.makeKeyAndOrderFront(nil)
            await ShotRenderer.settle(1.5)

            do {
                let written = try await ShotRenderer.captureAll(client: client, out: out)
                for path in written { print(path) }
                print("\(written.count) shots in \(out)")
            } catch {
                print("shots failed: \(error)")
            }
            exit(0)
        }
    }

    /// A window that closes is a window that can come back. Without this the
    /// window is gone for the session and the person has to relaunch to get
    /// it — which is a broken promise from a Dock icon.
    func applicationShouldHandleReopen(_ sender: NSApplication, hasVisibleWindows flag: Bool) -> Bool {
        shell?.showWindow(nil)
        return true
    }

    func applicationWillTerminate(_ notification: Notification) {
        shell?.stopPolling()
    }
}

// A geometry probe, for the same reason `--describe` exists: the switcher's
// grid is arithmetic over widths, and arithmetic is worth checking without a
// window in front of it. The expected values live in the test suite, which is
// the place a number that somebody will later argue about belongs.
extension CrossOSApp {
    static func probeGeometry() {
        let cases: [(String, [CGFloat], CGFloat, CGFloat, CGFloat)] = [
            ("3x300 @720", [300, 300, 300], 100, 8, 720),
            ("3x300 @640", [300, 300, 300], 100, 8, 640),
            ("300+300+84 @720", [300, 300, 84], 100, 8, 720),
            ("900 @720", [900], 100, 8, 720),
            ("3x210 pad0 @640", [210, 210, 210], 100, 0, 640),
            ("3x210 pad8 @640", [210, 210, 210], 100, 8, 640),
            ("3x215 pad0 @640", [215, 215, 215], 100, 0, 640),
            ("empty", [], 100, 8, 720),
            ("6x119.5 @720", [119.5, 119.5, 119.5, 119.5, 119.5, 119.5], 100, 8, 720),
        ]
        for (name, widths, height, padding, maxWidth) in cases {
            let g = tileGrid(.init(
                widths: widths, tileHeight: height,
                padding: padding, widthMax: maxWidth
            ))
            print("\(name): rows=\(g.rows.count) maxX=\(g.maxX) maxY=\(g.maxY)")
        }
    }
}
