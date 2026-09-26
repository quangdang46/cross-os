import AppKit
import CrossOSCore

// The last of the kinds, and the ones with real logic in them.
//
// The React audit found about one `useState` per hundred lines in every
// renderer, and the exceptions are here: the trial's one-second tick, the zone
// editor's draft, the shortcut recorder's capture. Those are the parts that
// were never projections, and they are the parts the port has to think about
// rather than transliterate.

// MARK: - trial

/// The trial countdown and its three buttons.
///
/// The **one-second tick is a clock, not a poll.** It redraws a duration the
/// daemon already handed over; the trial STATE is re-read only when the
/// refresh token changes, which is what keeps a control from talking to the
/// daemon on a timer of its own (TrialControl.tsx:19-21).
///
/// The **Confirm flag is worth reading twice.** `confirmTrial` takes `healthy`,
/// which is a claim about the daemon's own state and is read from
/// `status.plugins`. When the shell cannot see the plugin it passes false — a
/// health flag the frontend invented is exactly the auto-approval the gate
/// exists to prevent, and the daemon will then say why it refused
/// (TrialControl.tsx:9-15).
@MainActor
final class TrialView: CardControl {
    private var state = TrialState(plugin: "", state: "none", remainingMS: 0, timeoutMS: 0)
    private var countdown: NSTextField?
    private var tick: Timer?
    private let buttons = NSStackView()
    private var plugin: PluginMetaRow?

    override init(control: Control, context: ControlContext) {
        super.init(control: control, context: context)
        Task { await load(context: context) }
    }

    private func load(context: ControlContext) async {
        guard let loaded = try? await context.service.trialState() else {
            showError("Could not read the trial.", "The daemon did not answer safety.trialState.")
            return
        }
        // The plugin's own facts, so the row can name what is being trialled
        // rather than showing an id with nothing behind it.
        plugin = try? await context.service.pluginMeta()
            .first { $0.id == loaded.plugin }
        state = loaded
        draw(context: context)
    }

    private func draw(context: ControlContext) {
        buttons.arrangedSubviews.forEach {
            buttons.removeArrangedSubview($0)
            $0.removeFromSuperview()
        }

        if state.isIdle {
            replaceBody(with: EmptyStateView(
                headline: "No trial in flight.",
                detail: "A trial is a time-boxed run of one plugin. Start one to try it without a full install."
            ))
            addButton("Start a trial", in: buttons) { [weak self] in
                Task { await self?.begin(context: context) }
            }
            replaceBody(with: boxed(buttons))
            return
        }

        // Nothing expires a trial on the daemon side yet: `safety.trialState`
        // is a pure read, and only confirm and rollback remove an entry
        // (TrialControl.tsx:25-29). So an elapsed trial keeps reporting its
        // state with nothing left, and the countdown says what is actually
        // true — the window is closed and the way out is rollback — rather
        // than counting past zero and looking broken.
        let expired = state.remainingMS <= 0

        let timer = NSTextField(labelWithString: Self.clock(state.remainingMS))
        timer.font = Typeface.mono
        timer.textColor = expired ? Palette.danger : Palette.primaryInk
        countdown = timer

        let name = NSTextField(labelWithString: plugin?.name ?? state.plugin)
        name.font = Typeface.bodyStrong
        name.textColor = Palette.primaryInk

        let detail = NSTextField(labelWithString: expired
            ? "This trial has ended. Keeping it does not change anything — roll it back to return to the state before it."
            : "Roll back at any time to return to exactly the state before this trial started.")
        detail.font = Typeface.caption
        detail.textColor = Palette.secondaryInk
        detail.lineBreakMode = .byWordWrapping
        detail.maximumNumberOfLines = 0

        let column = NSStackView(views: [name, timer, detail, buttons])
        column.orientation = .vertical
        column.alignment = .leading
        column.spacing = Gap.tight

        if !expired {
            addButton("Keep it", in: buttons) { [weak self] in
                Task { await self?.confirm(context: context) }
            }
        }
        addButton("Roll back", in: buttons) { [weak self] in
            Task { await self?.rollback(context: context) }
        }

        replaceBody(with: column)
        startTicking()
    }

    private func boxed(_ view: NSView) -> NSView {
        let column = NSStackView(views: [view])
        column.orientation = .vertical
        column.alignment = .leading
        column.spacing = Gap.tight
        return column
    }

    /// A button whose action is fire-and-forget.
    ///
    /// Not `async`: `NSButton.action` is a selector, so the call site wraps its
    /// own `Task`, and a helper that took an async closure would be a closure
    /// nobody can call from a selector anyway.
    private func addButton(_ title: String, in stack: NSStackView, action: @escaping () -> Void) {
        let button = NSButton(title: title, target: ClosureTarget.shared, action: nil)
        button.bezelStyle = .rounded
        let proxy = ClosureTarget.shared.register(button, action: action)
        button.target = proxy
        button.action = #selector(ClosureTarget.fire(_:))
        stack.addArrangedSubview(button)
    }

    /// A clock, and only a clock. It reads a duration the daemon already handed
    /// over and redraws it; it never asks the daemon anything, which is what
    /// keeps this a `Timer` and not a second poll.
    ///
    /// It is invalidated when the countdown reaches zero and it holds its
    /// closure weakly, so a control that goes away stops being ticked without a
    /// `deinit` — which matters because `deinit` runs off the main actor and
    /// a `Timer` is not Sendable, so invalidating it from there is the one thing
    /// this class cannot do.
    private func startTicking() {
        tick?.invalidate()
        guard state.remainingMS > 0 else { return }
        tick = Timer.scheduledTimer(withTimeInterval: 1, repeats: true) { [weak self] _ in
            MainActor.assumeIsolated {
                guard let self else { return }
                self.state.remainingMS = max(0, self.state.remainingMS - 1000)
                self.countdown?.stringValue = Self.clock(self.state.remainingMS)
                if self.state.remainingMS == 0 {
                    self.tick?.invalidate()
                    self.tick = nil
                }
            }
        }
    }

    /// `m:ss`, from milliseconds. A wall-clock string would force the page to
    /// parse it back into a number every second; this is the number the daemon
    /// sent with two digits in front of it.
    static func clock(_ ms: Int) -> String {
        let total = max(0, ms) / 1000
        let minutes = total / 60
        let seconds = total % 60
        return minutes > 0
            ? String(format: "%d:%02d", minutes, seconds)
            : "\(seconds)s"
    }

    private func begin(context: ControlContext) async {
        let available = try? await context.service.pluginMeta()
        guard let target = plugin ?? available?.first else {
            context.note("No plugin is loaded, so there is nothing to trial.")
            return
        }
        do {
            _ = try await context.service.beginTrial(plugin: target.id)
            await load(context: context)
        } catch {
            context.note("Could not start a trial: \(error)")
        }
    }

    private func confirm(context: ControlContext) async {
        // `healthy` is a claim about the DAEMON's state, read from
        // `status.plugins`. A health flag this shell invented is exactly the
        // auto-approval the gate exists to prevent, so when the plugin is not
        // visible the flag is false and the daemon says why it refused.
        let visible = (try? await context.service.plugins())?
            .contains { $0.id == state.plugin && $0.enabled } ?? false
        do {
            _ = try await context.service.confirmTrial(plugin: state.plugin, healthy: visible)
            context.note("Trial kept.")
            await load(context: context)
        } catch {
            context.note("The daemon refused to keep this trial: \(error)")
        }
    }

    private func rollback(context: ControlContext) async {
        do {
            _ = try await context.service.rollbackTrial(plugin: state.plugin)
            context.note("Rolled back. Everything is as it was before the trial.")
            await load(context: context)
        } catch {
            context.note("Could not roll back: \(error)")
        }
    }
}

@MainActor
final class ClosureTarget: NSObject {
    static let shared = ClosureTarget()
    private var actions: [ObjectIdentifier: () -> Void] = [:]

    @discardableResult
    func register(_ button: NSButton, action: @escaping () -> Void) -> ClosureTarget {
        actions[ObjectIdentifier(button)] = action
        return self
    }

    @objc func fire(_ sender: NSButton) {
        actions[ObjectIdentifier(sender)]?()
    }
}

// MARK: - credits and licence

/// Who made this.
@MainActor
final class CreditsView: CardControl {
    override init(control: Control, context: ControlContext) {
        super.init(control: control, context: context)
        replaceBody(with: [
            ("CrossOS", Typeface.cardTitle),
            ("A window manager for macOS and Windows.", Typeface.body),
            ("Licensed MIT. See the licence for what that means in practice.", Typeface.caption),
        ])
    }
}

/// What licence this is under.
@MainActor
final class LicenseView: CardControl {
    override init(control: Control, context: ControlContext) {
        super.init(control: control, context: context)
        replaceBody(with: [
            ("MIT", Typeface.bodyStrong),
            ("Use it, change it, sell it. The one thing asked is that the notice goes with it.", Typeface.caption),
        ])
    }
}

// MARK: - palette

/// The command palette: everything CrossOS can be asked to do.
@MainActor
final class PaletteView: CardControl {
    override init(control: Control, context: ControlContext) {
        super.init(control: control, context: context)
        Task { await load(context: context) }
    }

    private func load(context: ControlContext) async {
        guard let commands = try? await context.service.commands() else {
            showError("Could not read the commands.", "The daemon did not answer core.commands.")
            return
        }
        // A search field and a list, because the palette's whole reason is
        // "type the first three letters" — a table of forty rows with no filter
        // is a list somebody scrolls, which is the thing a palette replaces.
        guard !commands.isEmpty else {
            replaceBody(with: EmptyStateView(
                headline: "No commands.",
                detail: "Commands arrive from plugins, so a new one shows up here without a shell change."
            ))
            return
        }

        let search = NSSearchField()
        search.placeholderString = "Search commands"
        let table = CommandTable(entries: commands, filter: search)

        let column = NSStackView(views: [search, table])
        column.orientation = .vertical
        column.alignment = .leading
        column.spacing = Gap.row
        column.translatesAutoresizingMaskIntoConstraints = false
        replaceBody(with: column)
    }
}

@MainActor
private final class CommandTable: NSTableView, NSTableViewDataSource, NSTableViewDelegate {
    private let entries: [CommandRow]
    /// The rows currently drawn, which is the filtered subset. A separate
    /// array rather than filtering `entries` in place, because `entries` is
    /// what the daemon sent and a search that destroyed it would make the next
    /// keystroke search a shorter list.
    var visible: [CommandRow]?
    private var filterObserver: NSObjectProtocol?
    private let column = NSTableColumn(identifier: NSUserInterfaceItemIdentifier("command"))

    init(entries: [CommandRow], filter: NSSearchField) {
        self.entries = entries
        super.init(frame: .zero)
        column.title = "Command"
        column.width = 420
        addTableColumn(column)
        dataSource = self
        delegate = self
        usesAlternatingRowBackgroundColors = true
        rowHeight = 26
        headerView = NSTableHeaderView()
        // The filter is on `controlTextDidChange`, not a timer and not a
        // daemon call: the list is already here, and searching it is not
        // asking a question.
        // The observer is stored rather than removed in `deinit`: it holds a
        // weak reference, so a table that goes away stops updating itself, and
        // the only thing a `deinit` would buy is tidiness at the cost of a
        // Sendable capture that Swift 6 will not accept under a class it does
        // not own.
        filterObserver = NotificationCenter.default.addObserver(
            forName: NSControl.textDidChangeNotification,
            object: filter,
            queue: .main
        ) { [weak self] _ in
            MainActor.assumeIsolated {
                let needle = filter.stringValue.lowercased()
                self?.visible = self?.entries.filter {
                    needle.isEmpty || $0.title.lowercased().contains(needle)
                } ?? []
                self?.reloadData()
            }
        }
    }

    @available(*, unavailable)
    required init?(coder: NSCoder) { fatalError("CommandTable is created in code") }

    func numberOfRows(in tableView: NSTableView) -> Int { visible?.count ?? entries.count }

    func tableView(
        _ tableView: NSTableView,
        viewFor tableColumn: NSTableColumn?,
        row: Int
    ) -> NSView? {
        let list = visible ?? entries
        guard row < list.count else { return nil }
        let field = NSTextField(labelWithString: list[row].title)
        field.font = Typeface.body
        field.textColor = Palette.primaryInk
        field.lineBreakMode = .byTruncatingTail
        return field
    }
}

// MARK: - shortcut list

/// The shortcut table, with a recorder.
@MainActor
final class ShortcutListView: CardControl {
    override init(control: Control, context: ControlContext) {
        super.init(control: control, context: context)
        Task { await load(context: context) }
    }

    private func load(context: ControlContext) async {
        async let matrix = try? context.service.matrix()
        async let rows = try? context.service.shortcuts()
        let (matrixRows, shortcuts) = await (matrix, rows)

        // Two sources, and they answer different questions. The matrix is what
        // CrossOS CAN do; the shortcut table is what the person has actually
        // bound. Showing the matrix alone is a settings page that lists
        // defaults as though they were choices.
        guard matrixRows != nil || shortcuts != nil else {
            showError("Could not read the shortcuts.", "The daemon did not answer config.getShortcuts.")
            return
        }
        let stack = NSStackView()
        stack.orientation = .vertical
        stack.alignment = .leading
        stack.spacing = Gap.row

        for row in matrixRows ?? [] {
            let line = NSStackView(views: [
                NSTextField(labelWithString: Chord.format(row.keys)),
                NSTextField(labelWithString: Humanize.phrase(row.action)),
            ])
            line.orientation = .horizontal
            line.spacing = Gap.group
            line.alignment = .firstBaseline
            if !row.enabled {
                for field in line.views as? [NSTextField] ?? [] {
                    field.textColor = Palette.tertiaryInk
                }
            }
            stack.addArrangedSubview(line)
        }

        let count = NSTextField(labelWithString: shortcuts?.isEmpty == false
            ? "\((shortcuts?.count ?? 0)) of these are bound to a chord of your own."
            : "None are bound to a chord of your own — these are the defaults.")
        count.font = Typeface.caption
        count.textColor = Palette.secondaryInk
        count.lineBreakMode = .byWordWrapping
        count.maximumNumberOfLines = 0
        stack.addArrangedSubview(count)

        replaceBody(with: stack)
    }
}
