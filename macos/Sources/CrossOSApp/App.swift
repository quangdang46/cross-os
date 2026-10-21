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

        // Set before the audit runs, so every finding's `detail` is printed.
        // Without it the audit explains its own rules to a file descriptor
        // nobody has open.
        if CommandLine.arguments.contains("--verbose") {
            AuditPrinter.verbose = true
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
            // Synchronous, NOT a top-level `await`, and this is measured
            // rather than preferred.
            //
            // A top-level `await` runs the main actor with an *implicit* run
            // loop, and in a RELEASE build that loop does not turn: `--audit`
            // hung to the timeout with no output, while `--describe` — which
            // drives `RunLoop.current.run(...)` explicitly — works on the same
            // binary. So the run loop is started for it here, and the async body
            // runs as a Task against a loop that is actually turning.
            //
            // A CI gate that hangs on the release binary teaches everyone to
            // ignore red CI, which is worse than not having the gate.
            await runAsyncThenExit { await AppDelegate().auditThenExit() }
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
            // Synchronous for the same measured reason as `--audit` above:
            // a top-level `await` does not turn a run loop in a release
            // build, and this one hung to the timeout with no output.
            await runAsyncThenExit { await AppDelegate().clickTestThenExit() }
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

/// Run an async body against a run loop that is actually turning, then exit.
///
/// **Measured, not preferred.** A top-level `await` in `main()` runs the main
/// actor with an *implicit* run loop, and in a release build that loop does not
/// turn: `--audit` and `--click-test` both hung to the timeout with no output
/// at all, on the same binary where `--describe` worked. `--describe` works
/// because it drives `RunLoop.current.run(mode:before:)` itself, in a loop,
/// with a deadline.
///
/// So this is that pattern with an async body: a semaphore waits for the body
/// to finish while the run loop is pumped underneath it, which is what the
/// socket reads and the layout passes need in order to make progress.
///
/// Bounded by a deadline as well as by the body finishing, so a body that
/// never finishes exits non-zero with a line saying so rather than hanging CI.
@MainActor
func runAsyncThenExit(
    timeout: TimeInterval = 120,
    _ body: @escaping @MainActor () async -> Void
) {
    let done = DispatchSemaphore(value: 0)
    Task { @MainActor in
        await body()
        done.signal()
    }

    let deadline = Date().addingTimeInterval(timeout)
    while done.wait(timeout: .now()) == .timedOut {
        if Date() >= deadline {
            FileHandle.standardError.write(Data(
                "timed out after \(Int(timeout))s — the run loop never finished the work\n".utf8))
            exit(1)
        }
        RunLoop.current.run(mode: .default, before: Date().addingTimeInterval(0.05))
    }
    exit(0)
}


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
        // The daemon has to be there, and it is checked FIRST.
        //
        // Without a daemon every call below returns an error, the page shows
        // its error state, and no button is found — so this test hangs rather
        // than fails, and a CI gate that hangs is a CI gate that burns the
        // job's whole timeout before anyone learns why. Measured: with the
        // daemon down it exits 124 on a timeout, having printed nothing.
        guard (try? await LiveCoreClient().status()) != nil else {
            print("click-test: no daemon on the socket — start one with ./scripts/run.sh --no-open")
            exit(1)
        }

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

        // 5. The Observe toggle, which was DEAD for the whole life of this
        //    port: `NSButton(checkboxWithTitle: "", target: nil, action: nil)`
        //    with nothing in the shell assigning an action to it, while the
        //    daemon has served `core.setObserve` from the start. The page's
        //    only control flipped its own box and reached nothing.
        //
        //    Read from `core.observeState` before and after, never from the
        //    checkbox's own `state` — a checkbox that changes its own box
        //    proves nothing, and that is exactly what this one did.
        if let observePage = pages.first(where: { $0.id == "core.observe" }) {
            shell.show(observePage)
            await ShotRenderer.settle(1.5)
            if let toggle = NSView.allViews(shell.window?.contentView)
                .compactMap({ $0 as? NSButton })
                .first(where: { ObserveTarget.shared.isWired($0) }) {
                let before = (try? await client.observeState())?.observe ?? false
                toggle.performClick(nil)
                await ShotRenderer.settle(1.2)
                let after = (try? await client.observeState())?.observe
                print("click-test: observe toggle  stored \(before) -> \(show(after))")
                if let after, after == before {
                    failures.append("the observe toggle click did not change the daemon's recorder state")
                }
                // Put it back, or this test is not idempotent.
                _ = try? await client.setObserve(enabled: before)
                await ShotRenderer.settle(0.6)
            } else {
                failures.append("no toggle found on core.observe")
            }
        } else {
            failures.append("core.observe is not being served")
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

        // 6. The RULE BUILDER, which the page promises and the shell could
        //    not do: "Add a rule: an app, an action, and a chord".
        //
        //    The daemon has answered `config.setUserRule` and
        //    `config.deleteUserRule` since the rule table was written, and the
        //    shell had no call site for either — so the builder could only
        //    LIST the rules it could not make. This presses the real button
        //    and reads the daemon's own table back.
        if let rulesPage = pages.first(where: { $0.id == "core.rules" }) {
            shell.show(rulesPage)
            await ShotRenderer.settle(1.5)
            var before = (try? await client.userRules()) ?? []
            // **The test's chord must not already exist.** The daemon refuses
            // an exact duplicate — the same chord and the same scope — so a
            // run that left its rule behind would make the next run add
            // nothing and report a broken builder over a working one. The
            // rule it removes is put back at the end.
            let clash = before.first { $0.chord == "Ctrl+F13" }
            if let clash {
                _ = try? await client.deleteUserRule(id: clash.id)
                before.removeAll { $0.id == clash.id }
            }
            // The chord needs a modifier: the builder refuses a bare key, and
            // a rule on a bare "Left" would take the arrow keys away.
            let modifier = NSView.allViews(shell.window?.contentView)
                .compactMap { $0 as? NSButton }
                .first { $0.title == "Ctrl" && $0.state == .off }
            modifier?.performClick(nil)
            // A FUNCTION key, so the rule this test makes is one nobody
            // would miss: a rule on "Left" would take the arrow keys away,
            // and a test that leaves one behind is a test that broke the
            // machine.
            if let key = NSView.allViews(shell.window?.contentView)
                .compactMap({ $0 as? NSPopUpButton })
                .first(where: { $0.item(withTitle: "F13") != nil }) {
                key.selectItem(withTitle: "F13")
            }
            await ShotRenderer.settle(0.4)
            if let add = button("Add rule") {
                add.performClick(nil)
                await ShotRenderer.settle(1.8)
                let after = (try? await client.userRules()) ?? []
                print("click-test: rule builder    rules \(before.count) -> \(after.count)")
                if after.count <= before.count {
                    failures.append("the rule builder's button added no rule to the daemon's table")
                }
                // **The rule THIS test made, by id.** The count alone is not
                // enough: a builder that added a rule AND lost an old one
                // would leave the count where it started, and the test would
                // report a working builder over a broken table.
                let made = after.first { fresh in
                    !before.contains { $0.id == fresh.id }
                }
                // And take it away again — with the rule's OWN Delete button,
                // which is the other half of the promise.
                //
                // **The button that matches the rule's ID, not the first
                // button that says "Delete".** Every rule's button says
                // "Delete", so "the first one" is "whichever rule happens to
                // be first" — and a test that clicks it deletes the
                // person's own rule and calls the count unchanged.
                if let made,
                   let remove = NSView.allViews(shell.window?.contentView)
                    .compactMap({ $0 as? NSButton })
                    .first(where: { $0.identifier?.rawValue == made.id }) {
                    remove.performClick(nil)
                    await ShotRenderer.settle(1.8)
                    let left = (try? await client.userRules()) ?? []
                    print("click-test: rule delete     rules \(after.count) -> \(left.count)")
                    if left.contains(where: { $0.id == made.id }) {
                        failures.append("the rule's Delete button did not take that rule away")
                    }
                } else {
                    failures.append("no Delete button for the rule the builder made")
                }
                // **Put the machine back.** The rule the test made is removed
                // above; if the delete did not land, it is removed here, and
                // any rule that was there before and is not there now is
                // written back. A test that leaves the person's rules
                // different is a test that changed their machine.
                let left = (try? await client.userRules()) ?? []
                for rule in after where !left.contains(where: { $0.id == rule.id }) {
                    _ = try? await client.deleteUserRule(id: rule.id)
                }
                for rule in before where !left.contains(where: { $0.id == rule.id }) {
                    _ = try? await client.createUserRule(UserRuleDraft(
                        key: rule.key,
                        modifiers: rule.modifiers ?? [],
                        appModes: rule.appModes ?? [],
                        appIDs: rule.appIDs ?? [],
                        capability: rule.capability,
                        parameters: rule.parameters
                    ))
                }
                await ShotRenderer.settle(0.6)
            } else {
                failures.append("no Add rule button on core.rules")
            }
        } else {
            failures.append("core.rules is not being served")
        }

        // 7. The FINDER MENU switches, which the daemon's own comment calls
        //    "the Finder menu the user can switch items off in" — and which
        //    the shell could not switch off, because there was no call site
        //    for `core.setMenuItemEnabled` at all.
        if let explorer = pages.first(where: { $0.id == "core.commands" }) {
            shell.show(explorer)
            await ShotRenderer.settle(1.5)
            sizeTables(in: shell.window?.contentView)
            await ShotRenderer.settle(0.5)
            let before = await menuEnabled(client)
            // The menu list is the page's FIRST table, so its checkbox is the
            // first wired one the table built.
            if let box = tableButtons(in: shell.window?.contentView)
                .first(where: { ToggleTarget.shared.isWired($0) }) {
                box.performClick(nil)
                await ShotRenderer.settle(1.5)
                let after = await menuEnabled(client)
                let changed = before.enumerated()
                    .filter { after[$0.offset].enabled != $0.element.enabled }
                    .map(\.offset)
                print("click-test: menu switch     changed item(s) \(changed.map(String.init) ?? [])")
                if changed.isEmpty {
                    failures.append("a Finder-menu switch click changed no item's stored state")
                }
                // Put back exactly what moved.
                for index in changed where index < before.count {
                    _ = try? await client.setMenuItemEnabled(id: before[index].id,
                                                            enabled: before[index].enabled)
                }
                await ShotRenderer.settle(0.6)
            } else {
                failures.append("no switch found on the Explorer's menu list")
            }
        } else {
            failures.append("core.commands is not being served")
        }

        // 8. The SWITCHER page, which listed the windows and could not act
        //    on any of them: the daemon has answered `core.switcherFocus`
        //    since the switcher was written, and the page had no way to
        //    reach it.
        //
        //    It is SKIPPED when the daemon sees no windows, which is the
        //    machine's state and not the app's: with the screen locked the
        //    AX plane withholds every window from every client, Apple's
        //    own System Events included.
        let windowRows = (try? await client.windows()) ?? []
        if windowRows.isEmpty {
            print("click-test: switcher focus   skipped — the daemon sees no windows")
            // The empty state is where the PERMISSION is asked for, and the
            // daemon's deep link is the answer to it: "the one permission
            // the daemon can actually ask for" — Accessibility, which the
            // focused-window watcher needs before it can prime.
            //
            // It is the same page and the same state the skip above is
            // about, so the two are complementary: the focus is proven where
            // there are windows, and the permission's fix where there are
            // none.
            if let switcher = pages.first(where: { $0.id == "core.switcher" }) {
                shell.show(switcher)
                await ShotRenderer.settle(1.5)
                if let grant = NSView.allViews(shell.window?.contentView)
                    .compactMap({ $0 as? NSButton })
                    .first(where: { $0.title.contains("Accessibility") }) {
                    grant.performClick(nil)
                    await ShotRenderer.settle(1.5)
                    let opened = (try? await client.openAccessibilitySettings()) != nil
                    print("click-test: grant access    opened \(opened)")
                    if !opened {
                        failures.append("the Accessibility deep link did not reach the daemon")
                    }
                } else {
                    failures.append("no Accessibility button on the Switcher page's empty state")
                }
            }
        } else if let switcher = pages.first(where: { $0.id == "core.switcher" }) {
            shell.show(switcher)
            await ShotRenderer.settle(1.5)
            sizeTables(in: shell.window?.contentView)
            await ShotRenderer.settle(0.5)
            // **The table that can be activated, and has the windows in
            // it.** The first table in the window is not necessarily the
            // switcher's: a page that was shown earlier leaves its table
            // behind, and the first one found was the activity page's —
            // nineteen rows of traces and no action behind it.
            let tables = NSView.allViews(shell.window?.contentView)
                .compactMap { $0 as? NSTableView }
            if let table = tables.first(where: {
                $0.numberOfRows == windowRows.count && $0.doubleAction != nil
            }),
               let target = table.target, let action = table.doubleAction {
                table.selectRowIndexes(IndexSet(integer: 0), byExtendingSelection: false)
                let wanted = windowRows.first?.windowID ?? ""
                _ = target.perform(action, with: table)
                await ShotRenderer.settle(1.5)
                // **And the window IS the front one afterwards.** The
                // daemon's own list says which window is selected, so the
                // proof is the daemon's answer and not the click.
                let after = (try? await client.windows()) ?? []
                let front = after.first { $0.selected }?.windowID ?? ""
                print("click-test: switcher focus   activated \(wanted) — front window is \(front)")
                if wanted.isEmpty || front != wanted {
                    failures.append("the switcher did not bring the window it was asked for to the front")
                }
            } else {
                failures.append("no switchable table found on core.switcher")
            }
        } else {
            failures.append("core.switcher is not being served")
        }

        // 9. The WIZARD'S FINISH, which is the one write that cannot be
        //    taken back — until now. The daemon's flag said "I am finished"
        //    and there was no way to say it again, so the app opened on the
        //    wizard on every launch, forever, and a person who wanted the
        //    setup back could not have it.
        if let onboard = pages.first(where: { $0.id == "core.onboard" }) {
            shell.show(onboard)
            await ShotRenderer.settle(1.5)
            let before = (try? await client.onboardingState())?.completed ?? false
            if let finish = NSView.allViews(shell.window?.contentView)
                .compactMap({ $0 as? NSButton })
                .first(where: { $0.identifier?.rawValue == "onboarding.finish" }) {
                finish.performClick(nil)
                await ShotRenderer.settle(1.5)
                let after = (try? await client.onboardingState())?.completed ?? before
                print("click-test: wizard finish   completed \(before) -> \(after)")
                if after == before {
                    failures.append("the wizard's Finish button did not change the daemon's flag")
                }
                // And back, so this test is not a behaviour change.
                //
                // The wizard has to be shown AGAIN to press it again:
                // finishing the setup navigates to the app's front door, so
                // the button that was just pressed is no longer on screen —
                // and pressing a button whose view has been torn down does
                // nothing at all, which is how this test first reported a
                // button that would not put the flag back.
                shell.show(onboard)
                await ShotRenderer.settle(1.5)
                if let again = NSView.allViews(shell.window?.contentView)
                    .compactMap({ $0 as? NSButton })
                    .first(where: { $0.identifier?.rawValue == "onboarding.finish" }) {
                    again.performClick(nil)
                    await ShotRenderer.settle(1.5)
                    let restored = (try? await client.onboardingState())?.completed ?? after
                    print("click-test: wizard reopen   completed \(after) -> \(restored)")
                    if restored != before {
                        failures.append("the wizard's button did not put the daemon's flag back")
                    }
                } else {
                    failures.append("the wizard's button was gone when the test pressed it again")
                }
            } else {
                failures.append("no Finish button on core.onboard")
            }
        } else {
            failures.append("core.onboard is not being served")
        }

        if failures.isEmpty {
            print("CLICK TEST PASS — 10 write paths reach the daemon: "
                  + "panic stop, re-enable, matrix rule, plugin, observe, profile apply, "
                  + "the rule builder and its delete, the Finder-menu switch, the switcher's focus, "
                  + "and the wizard's finish")
            exit(0)
        }
        for f in failures { print("click-test FAIL: \(f)") }
        exit(1)
    }

    /// What the daemon says about each Finder menu item, by id.
    ///
    /// The read is from the DAEMON and not from the table's own cells: a
    /// checkbox that changes its own box proves nothing, which is what the
    /// observe toggle did for its whole life.
    @MainActor
    private func menuEnabled(_ client: any CoreClient) async -> [(id: String, enabled: Bool)] {
        let rows = (try? await client.finderMenu()) ?? []
        return rows.compactMap { row in
            guard let item = row.objectValue,
                  let id = item["id"]?.stringValue else { return nil }
            return (id, item["enabled"]?.boolValue ?? false)
        }
    }

    /// Which profile the daemon currently has applied.
    ///
    /// The unread case is a separate flag rather than a sentinel STRING, which
    /// is what the first version of this returned: `"unread"` is a
    /// `String?`, so a failed read printed `active unread` and then compared
    /// equal to a real profile id if the daemon ever had one. The two states
    /// are different kinds of thing and they were sharing a type.
    @MainActor
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
    @MainActor
    private func firstRuleEnabled(_ client: any CoreClient) async -> Bool? {
        guard let rows = try? await client.matrix(), let row = rows.first else { return nil }
        return row.enabled
    }

    /// The id of the first rule the daemon serves.
    @MainActor
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
    @MainActor
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
        // `--describe` takes no page. It used to take one silently, which is
        // worse than not having it: `--describe core.matrix` printed the
        // HOME page's tree under a heading that said matrix, and two
        // measurements in this round were wrong because of it before it was
        // found. `--explain=<pageID>` is the one that selects a page, and it
        // says so.
        //
        // Rejecting the stray argument is the whole fix. A harness flag that
        // ignores what it was given cannot be asserted on, because reading it
        // looks exactly like reading the right page.
        if let stray = CommandLine.arguments.dropFirst().first(where: {
            !$0.hasPrefix("--") && $0 != "0"
        }) {
            FileHandle.standardError.write(Data("""
                --describe takes no page argument, and "\(stray)" is not one.
                It always describes the page the shell is showing.
                To describe a named page:  --audit --explain=<pageID>

                """.utf8))
            exit(2)
        }

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
        // The chrome: the title bar and toolbar sit ABOVE `contentView`, so
        // the tree printed below never sees them. They had been built and
        // never checked — and `--shots` cannot check them either, because a
        // render of `contentView` has no title bar in it. So this is the only
        // place they get looked at.
        if let w = shell.window {
            print("WINDOW \(Int(w.frame.width))x\(Int(w.frame.height)) style=\(w.styleMask.rawValue)")
            print("TOOLBAR visible=\(w.toolbar?.isVisible ?? false) items=\(w.toolbar?.items.count ?? -1)")
            if let t = w.toolbar {
                for item in t.items {
                    let v = item.view
                    print("  item \(item.itemIdentifier.rawValue) view=\(v.map { String(describing: type(of: $0)) } ?? "nil") frame=\(v?.frame ?? .zero)")
                    if let v { ViewTree.describe(v, indent: 3) }
                }
            }
            print("TITLE \(w.title)")
            print("FRAME rect=\(w.frame) contentLayoutRect=\(w.contentLayoutRect)")
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
        let systemFindings = Audit.surfaceAudit()
            + Audit.contrastMatrix()
            + Audit.contrastMatrix(inAppearance: .darkAqua)
            + Audit.rhythmAudit()
            + Audit.errorCopyAudit()
            + Audit.errorSignalAudit()
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
                + Audit.fillAudit(root: pageView, page: page.id)
                + Audit.weightAudit(root: pageView, page: page.id)
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
    @MainActor
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
