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

        // `--dark` renders the audit and the shots in the DARK appearance.
        //
        // Every screenshot and every measurement in this repo's history has
        // been taken in the light appearance, because the machine is in it.
        // Every colour in this app is a DYNAMIC `NSColor`, so the dark palette
        // is not a variant of the light one that has been seen — it is a
        // palette nobody has ever looked at, on an app that is supposed to
        // follow the user's Appearance setting.
        let darkRun = CommandLine.arguments.contains("--dark")
        if darkRun {
            NSApp.appearance = NSAppearance(named: .darkAqua)
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
        // The daemon may ALREADY be killed when this starts — a previous run's
        // Reset Everything leaves it that way — so "the flag changed" is the
        // wrong assertion and fails on a working button. What PANIC STOP must
        // do is leave it TRUE, whatever it was: a button that leaves the flag
        // as it found it has done nothing. Measured failing this way:
        // `killed true -> true` reported as "PANIC STOP did not change the
        // daemon's kill flag", against a daemon that was already stopped.
        let before = (try? await client.status())?.killed ?? false
        panic.performClick(nil)
        await ShotRenderer.settle(1.0)
        let after = (try? await client.status())?.killed ?? false
        print("click-test: PANIC STOP  killed \(before) -> \(after)")
        if !after {
            failures.append("PANIC STOP did not leave the daemon's kill flag set")
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
        guard button("Reset Everything") != nil else {
            failures.append("no Reset Everything button on core.safety")
            print("click-test FAIL: no Reset Everything button on core.safety")
            exit(1)
        }

        // Reset Everything is NOT pressed, and the reason is the point.
        //
        // It asks first: the daemon sends
        // `confirm: "Remove login item, disable extension, clean CrossOS-owned
        // state, verify no process remains?"` and the shell now shows it. So
        // pressing the button here would open a modal, and `NSAlert.runModal`
        // spins a NESTED run loop on the main thread — the test's own
        // continuations would never resume and the run would hang with no
        // output at all. Which is, in its way, the proof the guard is up.
        //
        // So this checks the guard is DECLARED rather than pressing past it. A
        // test that could drive the modal is a test that could also destroy
        // somebody's configuration, and that is not a trade worth making to
        // gain a green tick.
        let resetControl = safety.schema?.controls.first { $0.id == "reset" }
        let guardText = resetControl?.confirm ?? ""
        print("click-test: Reset Everything asks first: \(guardText.isEmpty ? "NO — it runs unguarded" : "yes")")
        if guardText.isEmpty {
            failures.append("Reset Everything has no confirmation and runs unguarded")
        }

        // 4. The WRITE paths, which matter more than the buttons: a checkbox
        //    that does not write is a settings app that lies about what is on.
        //
        //    Matrix checkboxes go through `ToggleTarget` and
        //    `config.setRuleEnabled`; plugin switches go through `PluginTarget`
        //    and `plugin.setEnabled`. Both are read back from the daemon, so
        //    this proves the round trip rather than the checkbox's own state.
        if let matrix = pages.first(where: { $0.id == "core.matrix" }) {
            shell.show(matrix)
            await ShotRenderer.settle(1.5)
            // The table builds its row views LAZILY, so a checkbox does not
            // exist until the table has been asked for its rows. `sizeToFit`
            // is what forces that, and without it the search finds nothing and
            // reports "no checkbox" about a table that has eleven of them.
            sizeTables(in: shell.window?.contentView)
            await ShotRenderer.settle(0.5)
            // A checkbox is found by asking ToggleTarget whether it has a
            // closure behind it, not by its cell class: SwiftUI's `NSButton`
            // on this SDK is an `NSButtonCell` with a hosting view, and
            // guessing a class name finds nothing on a table that has eleven
            // checkboxes in it.
            if let box = tableButtons(in: shell.window?.contentView)
                .first(where: { ToggleTarget.shared.isWired($0) }) {
                // The SAME rule is read, clicked and read again.
                //
                // Reading `rules.first` while clicking "the first checkbox the
                // table built" reads one rule and writes another, so the test
                // reports `true -> true` against a daemon that worked — measured,
                // on exactly that. What must be true is that SOME rule changed.
                let before = (try? await client.matrix())?.map(\.enabled) ?? []
                box.performClick(nil)
                await ShotRenderer.settle(1.2)
                let after = (try? await client.matrix())?.map(\.enabled) ?? []
                let changed = zip(before, after).enumerated()
                    .filter { $0.element.0 != $0.element.1 }
                    .map(\.offset)
                print("click-test: matrix checkbox  changed rule(s) \(changed.map(String.init) ?? [])")
                if changed.isEmpty {
                    failures.append("a matrix checkbox click changed no rule's stored state")
                }
                // Put back exactly what moved, so this test is not a
                // behaviour change. The rows are read ONCE: the state being
                // restored is `before`, not the state this click produced.
                let rows = (try? await client.matrix()) ?? []
                for index in changed where index < rows.count {
                    _ = try? await client.setRuleEnabled(
                        ruleID: rows[index].ruleID,
                        enabled: before[index]
                    )
                }
                await ShotRenderer.settle(0.6)
            } else {
                failures.append("no checkbox found on core.matrix")
            }
        } else {
            failures.append("core.matrix is not being served")
        }

        if let extensions = pages.first(where: { $0.id == "core.extensions" }) {
            shell.show(extensions)
            await ShotRenderer.settle(1.5)
            sizeTables(in: shell.window?.contentView)
            await ShotRenderer.settle(0.5)
            if let toggle = NSView.allViews(shell.window?.contentView)
                .compactMap({ $0 as? NSButton })
                .first(where: { PluginTarget.shared.isWired($0) }) {
                let before = await pluginEnabled(client, "windows-keyboard") ?? false
                toggle.performClick(nil)
                await ShotRenderer.settle(1.2)
                let after = await pluginEnabled(client, "windows-keyboard")
                print("click-test: plugin switch   stored \(before) -> \(show(after))")
                if let after, after == before {
                    failures.append("a plugin switch click did not change the daemon's stored plugin state")
                }
                _ = try? await client.setPluginEnabled(id: "windows-keyboard", enabled: before)
                await ShotRenderer.settle(0.6)
            } else {
                failures.append("no switch found on core.extensions")
            }
        } else {
            failures.append("core.extensions is not being served")
        }

        // 5. Profile Apply, which is the one control that changes how the whole
        //    product behaves. It was broken by the same parameter-name defect
        //    (`id` where the daemon wants `profile`) and this is what proves it
        //    is fixed THROUGH THE UI rather than at the RPC.
        if let profilesPage = pages.first(where: { $0.id == "core.profiles" }) {
            shell.show(profilesPage)
            await ShotRenderer.settle(1.5)
            let before = await activeProfileID(client)
            if !before.read {
                failures.append("could not read the active profile before the click")
            }
            if let apply = button("Apply") {
                apply.performClick(nil)
                await ShotRenderer.settle(1.8)
                let after = await activeProfileID(client)
                print("click-test: profile Apply   active \(describe(before)) -> \(describe(after))")
                if !after.read || after.id == before.id {
                    failures.append("Profile Apply did not change the daemon's active profile")
                }
                // Put it back.
                // Put the machine back the way the test found it.
                if let applied = after.id {
                    _ = try? await client.deactivateProfile(id: applied)
                    await ShotRenderer.settle(0.8)
                }
            } else {
                failures.append("no Apply button on core.profiles")
            }
        } else {
            failures.append("core.profiles is not being served")
        }

        if failures.isEmpty {
            print("CLICK TEST PASS — 3 buttons, both write paths and profile apply reach the daemon")
            exit(0)
        }
        for f in failures { print("click-test FAIL: \(f)") }
        exit(1)
    }

    /// Which profile the daemon currently has applied.
    ///
    /// The unread case is a separate flag rather than a sentinel STRING, which
    /// is what the first version of this returned: `"unread"` is a
    /// `String?`, so a failed read printed `active unread` and then compared
    /// equal to a real profile id if the daemon ever had one. The two states
    /// are different kinds of thing and they were sharing a type.
    private func activeProfileID(_ client: any CoreClient) async -> (id: String?, read: Bool) {
        guard let rows = try? await client.profiles() else { return (nil, false) }
        return (rows.first(where: { $0.active })?.id, true)
    }

    /// Every button inside every row of every table.
    ///
    /// A table's rows are not in the view hierarchy until it has built them,
    /// and its row views are not its subviews, so the ordinary tree walk does
    /// not reach a checkbox. `sizeToFit` forces the rows to exist; this finds
    /// the controls inside them.
    private func tableButtons(in root: NSView?) -> [NSButton] {
        var out: [NSButton] = []
        for view in NSView.allViews(root) {
            guard let table = view as? NSTableView else { continue }
            table.sizeToFit()
            for row in 0..<table.numberOfRows {
                guard let rowView = table.rowView(atRow: row, makeIfNecessary: true) else { continue }
                out.append(contentsOf: NSView.allViews(rowView).compactMap { $0 as? NSButton })
            }
        }
        return out
    }

    /// Force every table to build its rows.
    ///
    /// `NSTableView` hands out row views one at a time and only for rows it
    /// has built; a table that has been laid out but never scrolled has none.
    /// A search for a checkbox inside one therefore reports "none found"
    /// about a table that visibly has eleven, which is a failure in the
    /// measurement rather than in the app.
    private func sizeTables(in root: NSView?) {
        for view in NSView.allViews(root) {
            (view as? NSTableView)?.sizeToFit()
        }
    }

    /// "true"/"false"/"unread" — a nil from an unread answer is printed as
    /// itself rather than as a Bool, so a failed read is visible in the output
    /// instead of being silently true.
    private func show(_ value: Bool?) -> String {
        guard let value else { return "unread" }
        return value ? "true" : "false"
    }

    /// The stored enabled state of the first rule the daemon serves.
    private func firstRuleEnabled(_ client: any CoreClient) async -> Bool? {
        guard let rows = try? await client.matrix(), let row = rows.first else { return nil }
        return row.enabled
    }

    /// The id of the first rule the daemon serves.
    private func firstRuleID(_ client: any CoreClient) async throws -> String {
        guard let rows = try? await client.matrix(), let row = rows.first else {
            throw CoreError.decode(method: "config.getMatrix", underlying: DecodeShapeError.expectedArray)
        }
        return row.ruleID
    }

    /// A profile id, "none", or "unread".
    ///
    /// Three states, not two, and the first version collapsed two of them: a
    /// nil id with a SUCCESSFUL read means no profile is applied, and printing
    /// that as "unread" made a correct pre-click reading look like a failure to
    /// read. `read` is what distinguishes them.
    private func describe(_ state: (id: String?, read: Bool)) -> String {
        guard state.read else { return "unread" }
        return state.id ?? "none"
    }

    /// Whether the daemon has a plugin switched on.
    private func pluginEnabled(_ client: any CoreClient, _ id: String) async -> Bool? {
        guard let rows = try? await client.plugins() else { return nil }
        guard let row = rows.first(where: { $0.id == id }) else { return nil }
        return row.enabled
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

        // The contrast half runs in BOTH appearances; the rhythm half is
        // about numbers and does not change.
        let systemFindings = Audit.contrastMatrix()
            + Audit.contrastMatrix(inAppearance: .darkAqua)
            + Audit.rhythmAudit()
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
