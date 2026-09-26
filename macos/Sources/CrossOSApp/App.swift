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
    static func main() {
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

    /// Print the layer tree of the window, to find out where the pixels are.
    func probeLayers() {
        let shell = ShellWindowController(client: client)
        self.shell = shell
        shell.showWindow(nil)
        shell.window?.makeKeyAndOrderFront(nil)
        ShotRenderer.settle(1.2)
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
            ShotRenderer.settle(1.5)

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
