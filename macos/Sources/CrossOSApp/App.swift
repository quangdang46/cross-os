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

        // `--help` prints and exits.
        //
        // There was no handler for it, so it fell through every branch and
        // launched the real app: `./CrossOS --help` opened a window, connected
        // to the daemon and sat there until it was killed — and looked exactly
        // like a hang. It is the first thing anybody types.
        if CommandLine.arguments.contains("-h") || CommandLine.arguments.contains("--help") {
            print(usage)
            exit(0)
        }
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

        // `--click-test` presses real buttons and watches the daemon.
        //
        // It exists because "the buttons are wired" is not the same claim as
        // "a button does something", and this session established the first
        // and not the second: every button in the app used to print "is not
        // wired yet" and none of them had ever been pressed.
        //
        // It needs no Accessibility permission and no screen recording.
        // `NSButton.performClick(nil)` fires the control's target/action
        // exactly as a pointer click does — same selector, same sender — so
        // this exercises the AppKit half of the path rather than standing in
        // for it. What it does NOT cover is the pixel: whether the button is
        // visible and hittable where it is drawn. That needs a human or a
        // screenshot, and it is the smaller half.
        if CommandLine.arguments.contains("--click-test") {
            await AppDelegate().clickTestThenExit()
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

    /// What the app is, and every flag it answers to.
    ///
    /// Written out rather than derived, because a `--help` that lists the
    /// flags a developer remembers is a `--help` that is wrong the moment a
    /// flag is added, and this file's flags are how the UI is verified.
    static let usage = """
    CrossOS — a cross-OS UX compatibility layer for macOS.

    With no flags, opens the settings window against the running daemon.
    Start the daemon first with ./scripts/run.sh --no-open.

    Verification flags (they render or measure and then exit):

      --describe           print the live view tree and exit
      --explain=<pageID>   with --audit, print one page's tree
      --audit              measure every page's design properties
      --shots              render every page to PNG
      --shots --window     …of the whole window, sidebar included
      --out=<dir>          with --shots, where to write (default /tmp/crossos-shots-swift)
      --geometry           print the switcher's grid arithmetic and exit
      --probe-layers       print the window's layer tree and exit
      --log-cols           with --shots, print each table column's geometry

    """
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

    /// Press real buttons and check the daemon's state actually changed.
    ///
    /// The Safety page's controls are the app's most consequential, and until
    /// now none had ever been pressed: the dispatch table was empty, so every
    /// button printed "not wired yet" and nothing else. A table that names its
    /// capabilities proves the mapping exists; this proves the mapping REACHES
    /// the daemon.
    ///
    /// It round-trips on purpose — PANIC STOP and then Re-enable — so the check
    /// leaves the machine as it found it. A verification that leaves panic
    /// stop latched is not a safe thing to run from a script.
    ///
    /// No Accessibility permission and no screen recording: `performClick(_:)`
    /// fires the control's target/action exactly as a pointer click does. What
    /// this does NOT prove is that the button is visible and hittable where it
    /// is drawn — that is the half only a screenshot or a human can answer.
    @MainActor
    func clickTestThenExit() async {
        let client = LiveCoreClient()
        let shell = ShellWindowController(client: client)
        self.shell = shell
        shell.showWindow(nil)
        shell.window?.makeKeyAndOrderFront(nil)
        await ShotRenderer.settle(1.5)

        guard let pages = try? await client.pages() else {
            print("click-test: could not read core.pages")
            exit(1)
        }
        guard let safety = pages.first(where: { $0.id == "core.safety" }) else {
            print("click-test: core.safety is not being served")
            exit(1)
        }
        shell.show(safety)
        await ShotRenderer.settle(1.2)

        var failures: [String] = []

        func button(_ label: String) -> NSButton? {
            NSView.allViews(shell.window?.contentView)
                .compactMap { $0 as? NSButton }
                .first { $0.title.contains(label) }
        }

        // 1. PANIC STOP must reach the daemon: `killed` is the daemon's own
        //    kill flag, not a UI echo of the click.
        guard let panic = button("PANIC STOP") else {
            print("click-test: no PANIC STOP button on core.safety")
            exit(1)
        }
        let before = (try? await client.status())?.killed ?? false
        panic.performClick(nil)
        await ShotRenderer.settle(1.0)
        let after = (try? await client.status())?.killed ?? false
        print("click-test: PANIC STOP  killed \(before) -> \(after)")
        if before || !after {
            failures.append("PANIC STOP did not change the daemon's kill flag")
        }

        // 2. Re-enable must put it back.
        guard let resume = button("Re-enable") else {
            print("click-test: no Re-enable button on core.safety")
            exit(1)
        }
        resume.performClick(nil)
        await ShotRenderer.settle(1.0)
        let restored = (try? await client.status())?.killed ?? true
        print("click-test: Re-enable  killed -> \(restored)")
        if restored {
            failures.append("Re-enable did not clear the daemon's kill flag")
        }

        // 3. Reset Everything is the destructive one, so it runs LAST — after
        //    the round trip that leaves the machine as it was found.
        //
        //    It is not skipped for being destructive; it is last because a
        //    verification script must not leave panic stop latched, and
        //    resetting afterwards would put the machine into the very state
        //    the first two steps were checking.
        guard let reset = button("Reset Everything") else {
            failures.append("no Reset Everything button on core.safety")
            print("click-test FAIL: no Reset Everything button on core.safety")
            exit(1)
        }
        reset.performClick(nil)
        await ShotRenderer.settle(1.5)
        // Reset stops the tap, so `killed` must now be true. That is the
        // daemon's own flag, not a UI echo of the click.
        let afterReset = (try? await client.status())?.killed ?? false
        print("click-test: Reset Everything pressed, daemon killed -> \(afterReset)")
        if !afterReset {
            failures.append("Reset Everything did not stop the daemon's tap")
        }

        if failures.isEmpty {
            print("CLICK TEST PASS — all 3 buttons reach the daemon")
            exit(0)
        }
        for f in failures { print("click-test FAIL: \(f)") }
        exit(1)
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
        // `--explain <pageID>` instead of a full render, so a page that comes
        // out blank can be asked what it actually built.
        let explainTarget: String? = {
            guard let flag = CommandLine.arguments.first(where: { $0.hasPrefix("--explain=") })
            else { return nil }
            return String(flag.dropFirst("--explain=".count))
        }()

        let systemFindings = Audit.contrastMatrix() + Audit.rhythmAudit()
        var findings = systemFindings
        print("design system")
        AuditPrinter.emitPage("design system", findings: systemFindings)
        let pages = (try? await client.pages()) ?? []

        for page in pages {
            shell.show(page)
            // `--explain <pageID>` prints that page's live view tree. Three
            // defects in this port had the same shape — a container that
            // measured zero and laid its content out below its own frame —
            // and the tree is how each was told apart from the next.
            // Settle BEFORE anything is measured or printed. A page's
            // controls arrive from the daemon asynchronously, so a tree read
            // before the load lands shows every control at 0x0 — which reads
            // as a layout bug and is not one.
            await ShotRenderer.settle(0.9)
            if let only = explainTarget {
                if page.id == only {
                    print("TREE \(page.id)")
                    ViewTree.describe(shell.pageController?.view ?? NSView(), indent: 0)
                }
                continue
            }
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
                let written = try await ShotRenderer.captureAll(client: client, out: out, wholeWindow: CommandLine.arguments.contains("--window"))
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
