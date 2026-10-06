import AppKit
import CrossOSCore

// The controls that are projections of daemon data.
//
// Every one of these was a React component whose `useState` count was about one
// per hundred lines, which is the measurement behind the claim that they are
// projections and not stateful components: the state was view-local (which row
// is selected, whether a write is in flight) and the data was the daemon's. In
// AppKit there is no per-render cycle, so there is nothing to be "local" about
// — a row is drawn from a struct the daemon sent, and a control that changes
// asks the daemon and redraws from the answer.

// MARK: - matrix

/// The behaviour matrix: one row per rule, with a toggle.
@MainActor
final class MatrixView: CardControl {
    override init(control: Control, context: ControlContext) {
        super.init(control: control, context: context)
        Task { await load(context: context) }
    }

    private func load(context: ControlContext) async {
        guard let loaded = try? await context.service.matrix() else {
            showError(
                "Could not read the behaviour matrix.",
                didNotAnswer("config.getMatrix"),
                // The retry re-fires the same fetch. The closure escapes, so
                // `self` must be explicit — and it is also correct, because
                // a dead view's re-fire is a no-op spinner on a view that is
                // already gone, not a crash.
                onRetry: { [weak self] in Task { await self?.load(context: context) } }
            )
            return
        }
        replaceBody(with: MatrixTable(rows: loaded, service: context.service, note: context.note))
    }
}

@MainActor
private final class MatrixTable: NSStackView, TableContent {
    let columns: [(String, CGFloat)] = [("Chord", 150), ("Action", 180), ("Where", 140), ("On", 44)]
    private let entries: [MatrixRow]
    private let service: any CoreClient
    private let note: @Sendable (String) -> Void
    /// The STORED state, per rule, after a write. Not optimistic: the row shows
    /// what the daemon stored, so a refused edit is visible as a refused edit
    /// rather than a toggle that sprang back with no explanation.
    private var stored: [String: Bool] = [:]

    init(rows: [MatrixRow], service: any CoreClient, note: @escaping @Sendable (String) -> Void) {
        self.entries = rows
        self.service = service
        self.note = note
        super.init(frame: .zero)
        orientation = .vertical
        alignment = .width
        distribution = .fill
        translatesAutoresizingMaskIntoConstraints = false
        addArrangedSubview(TableView(content: self))
    }

    @available(*, unavailable)
    required init?(coder: NSCoder) { fatalError("MatrixTable is created in code") }

    var rowCount: Int { entries.count }

    func text(row: Int, column: Int) -> String {
        let row = entries[row]
        switch column {
        case 0: return Chord.format(row.keys)
        case 1: return Humanize.action(row.action)
        case 2: return describeContexts(row.contexts)
        default: return ""
        }
    }

    /// What a rule applies to, in a form a reader can finish.
    ///
    /// The full list is a wall of bundle ids: measured on `core.matrix`,
    /// "Native, Com.microsoft.vscode, Com.todesktop.230313mzl4w4u" — and the
    /// column truncated it at its edge, mid-word, to "Native, Com.micros…".
    /// A cut-off identifier is worse than a count: the reader cannot tell
    /// what was dropped or whether their app is in it.
    ///
    /// So the first two are named and the rest are counted. "Native +2 more"
    /// is a complete statement; "Native, Com.micros…" is not.
    private func describeContexts(_ contexts: [String]?) -> String {
        guard let contexts, !contexts.isEmpty else { return "Anywhere" }
        if contexts.count == 1 { return Humanize.phrase(contexts[0]) }
        // A bundle id is not a name. `Com.microsoft.vscode` is the OTHER side's
        // vocabulary, and printing it in a column 140pt wide truncates at the
        // column edge mid-word — measured: "Native, Com.micros…", which tells
        // the reader nothing about whether their app is in the list.
        //
        // So a list is a COUNT, and the count is the statement. One context is
        // named because "Anywhere +1" is a worse answer than the name itself.
        let named = contexts.map(Humanize.phrase)
        let first = shortIdentifier(named[0])
        return "\(first) +\(contexts.count - 1) more"
    }

    /// The last meaningful segment of a reverse-DNS identifier.
    ///
    /// `Com.microsoft.vscode` -> `vscode`, which is short enough to read and
    /// specific enough to recognise. Dropping the prefix loses no information
    /// a reader can act on: nobody reads `com.todesktop.230313mzl4w4u` to learn
    /// which app it is.
    private func shortIdentifier(_ value: String) -> String {
        guard value.contains(".") else { return value }
        let tail = value.split(separator: ".").last.map(String.init) ?? value
        return tail.count > 18 ? String(tail.prefix(17)) + "…" : tail
    }

    func toggle(row: Int, column: Int) -> (isOn: Bool, onChange: @MainActor (Bool) -> Void)? {
        guard column == 3 else { return nil }
        let entry = entries[row]
        let current = stored[entry.ruleID] ?? entry.enabled
        let chord = Chord.format(entry.keys)
        return (current, { next in
            Task { await self.write(rule: entry, keys: chord, next: next) }
        })
    }

    private func write(rule: MatrixRow, keys: String, next: Bool) async {
        do {
            // The stored state, not a success flag. Reading it as "did it
            // stick?" printed "The daemon kept Ctrl+C off." the moment somebody
            // turned a rule OFF — on the one control whose whole job is making
            // the machine stop remapping that chord
            // (MatrixControl.tsx:34-41).
            let storedState = try await service.setRuleEnabled(ruleID: rule.ruleID, enabled: next)
            stored[rule.ruleID] = storedState
            note("\(keys.isEmpty ? rule.ruleID : keys) is now \(storedState ? "on" : "off").")
        } catch {
            note("Could not change \(keys): \(error)")
        }
    }
}

// MARK: - overrides

/// The per-app overrides: what replaces a matrix row for one app.
@MainActor
final class OverridesView: CardControl {
    override init(control: Control, context: ControlContext) {
        super.init(control: control, context: context)
        Task { await load(context: context) }
    }

    private func load(context: ControlContext) async {
        guard let loaded = try? await context.service.overrides() else {
            showError(
                "Could not read the overrides.",
                didNotAnswer("config.getOverrides"),
                onRetry: { [weak self] in Task { await self?.load(context: context) } }
            )
            return
        }
        guard !loaded.isEmpty else {
            replaceBody(with: EmptyStateView(
                headline: "No overrides.",
                detail: "An override replaces one matrix row for one app. None are in place."
            ))
            return
        }
        replaceBody(with: OverridesTable(rows: loaded))
    }
}

@MainActor
private final class OverridesTable: NSStackView, TableContent {
    let columns: [(String, CGFloat)] = [("App", 180), ("Action", 200), ("Chord", 140), ("On", 44)]
    private let overrides: [OverrideRow]

    init(rows: [OverrideRow]) {
        self.overrides = rows
        super.init(frame: .zero)
        orientation = .vertical
        alignment = .width
        distribution = .fill
        translatesAutoresizingMaskIntoConstraints = false
        orientation = .vertical
        alignment = .width
        distribution = .fill
        translatesAutoresizingMaskIntoConstraints = false
        let table = TableView(content: self)
        // ARRANGED, not `addSubview` with edge constraints. Pinning a view to
        // all four of a stack's edges fights the width binding `TableView`
        // makes on `viewDidMoveToSuperview`: measured on `core.commands`,
        // `TableView 0x208` — zero wide — with the columns drawn at 489pt
        // anyway and the alternating stripes and a vertical seam drawn past
        // the group's rounded edge. See `TableView.viewDidMoveToSuperview`.
        addArrangedSubview(table)
        setContentHuggingPriority(.required, for: .vertical)
    }

    @available(*, unavailable)
    required init?(coder: NSCoder) { fatalError("OverridesTable is created in code") }

    var rowCount: Int { overrides.count }

    func text(row: Int, column: Int) -> String {
        let row = overrides[row]
        switch column {
        case 0: return row.app
        case 1: return Humanize.phrase(row.action)
        case 2: return Chord.format(row.keys)
        default: return ""
        }
    }

    func isDimmed(row: Int) -> Bool { !overrides[row].enabled }
}

// MARK: - conflicts

/// Which rule wins each contested chord.
@MainActor
final class ConflictResolverView: CardControl {
    override init(control: Control, context: ControlContext) {
        super.init(control: control, context: context)
        Task { await load(context: context) }
    }

    private func load(context: ControlContext) async {
        guard let loaded = try? await context.service.conflicts() else {
            showError(
                "Could not read the conflicts.",
                didNotAnswer("core.conflicts"),
                onRetry: { [weak self] in Task { await self?.load(context: context) } }
            )
            return
        }
        guard !loaded.isEmpty else {
            replaceBody(with: EmptyStateView(
                headline: "No conflicts.",
                detail: "Every chord is claimed once. A conflict is a chord two rules both want."
            ))
            return
        }
        let stack = NSStackView()
        stack.orientation = .vertical
        stack.alignment = .leading
        stack.spacing = Gap.row
        for row in loaded {
            stack.addArrangedSubview(ConflictCard(conflict: row))
        }
        replaceBody(with: stack)
    }
}

/// One conflict: the chord, who wins, and who loses.
///
/// The loser is shown as a loser, not hidden. A person whose rule silently
/// stopped firing needs to see WHICH one lost and to whom; a list that only
/// showed the winner would be a list that answered the wrong question.
@MainActor
private final class ConflictCard: NSStackView {
    init(conflict: ConflictRow) {
        super.init(frame: .zero)

        let chord = NSTextField(labelWithString: Chord.format(conflict.keys))
        chord.font = Typeface.mono
        chord.textColor = Palette.primaryInk

        let winner = NSTextField(labelWithString: "→ \(Humanize.phrase(conflict.winner))")
        winner.font = Typeface.body
        winner.textColor = Palette.okInk

        let header = NSStackView(views: [chord, winner])
        header.orientation = .horizontal
        header.alignment = .firstBaseline
        header.spacing = Gap.row

        let column = NSStackView(views: [header])
        column.orientation = .vertical
        column.alignment = .leading
        column.spacing = Gap.tight

        for loser in conflict.losers ?? [] {
            let field = NSTextField(labelWithString: "· also claimed by \(Humanize.phrase(loser)) — not applied")
            field.font = Typeface.caption
            field.textColor = Palette.tertiaryInk
            column.addArrangedSubview(field)
        }

        column.translatesAutoresizingMaskIntoConstraints = false
        addSubview(column)
        NSLayoutConstraint.activate([
            column.leadingAnchor.constraint(equalTo: leadingAnchor),
            column.trailingAnchor.constraint(lessThanOrEqualTo: trailingAnchor),
            column.topAnchor.constraint(equalTo: topAnchor),
            // The BOTTOM pin, and its absence is why three plugin rows
            // overlapped by 14pt each in the first audit run.
            //
            // A view pinned leading/trailing/top and nothing else has no
            // height: Auto Layout is free to give it zero and let the
            // content overflow, and the content does — the switch on row two
            // lands on top of the switch on row one. The audit named it as
            // "NSSwitch and NSSwitch overlap by 54x14 [window A x=306 y=-11 |
            // window B x=306 y=-21]", and the ten points between them were
            // the ten points each row claimed for itself and none of them
            // gave back.
            column.bottomAnchor.constraint(lessThanOrEqualTo: bottomAnchor),
        ])
    }

    @available(*, unavailable)
    required init?(coder: NSCoder) { fatalError("ConflictCard is created in code") }
}

// MARK: - observe

/// The Observe page's one toggle, and what the recorder is actually keeping.
@MainActor
final class ObserveToggleView: CardControl {
    override init(control: Control, context: ControlContext) {
        super.init(control: control, context: context)
        Task { await load(context: context) }
    }

    private func load(context: ControlContext) async {
        guard let state = try? await context.service.observeState() else {
            showError(
                "Could not read the recorder.",
                didNotAnswer("core.observeState"),
                onRetry: { [weak self] in Task { await self?.load(context: context) } }
            )
            return
        }
        replaceBody(with: ObserveBody(
            state: state,
            service: context.service,
            note: context.note
        ))
    }
}

/// Where a click on the Observe toggle goes.
///
/// The shape is `PluginTarget`'s, and it differs in the one place that
/// matters: the note reports what the daemon STORED, not what was asked for.
/// `setRuleEnabled` documents why — a control labelled from the click is
/// labelled from the belief, and the recorder's own docs say the same thing
/// about `core.observeState`: "a toggle that cannot say which position it is
/// in can only ever be labelled from the click that produced it, which is the
/// belief, not the state."
@MainActor
final class ObserveTarget: NSObject {
    static let shared = ObserveTarget()

    private struct Binding {
        let next: Bool
        let service: any CoreClient
        let note: @Sendable (String) -> Void
    }

    private var bindings: [ObjectIdentifier: Binding] = [:]

    /// Whether this button has a write behind it.
    ///
    /// The click test asks this rather than looking for a cell class, because
    /// SwiftUI's `NSButton` on this SDK is an `NSButtonCell` behind a hosting
    /// view and a class check finds nothing.
    func isWired(_ toggle: NSButton) -> Bool {
        bindings[ObjectIdentifier(toggle)] != nil
    }

    func register(
        _ toggle: NSButton,
        next: Bool,
        service: any CoreClient,
        note: @escaping @Sendable (String) -> Void
    ) -> ObserveTarget {
        bindings[ObjectIdentifier(toggle)] = Binding(next: next, service: service, note: note)
        return self
    }

    @objc func fire(_ sender: NSButton) {
        guard let binding = bindings[ObjectIdentifier(sender)] else { return }
        Task {
            do {
                let stored = try await binding.service.setObserve(enabled: binding.next)
                sender.state = stored ? .on : .off
                binding.note(stored
                    ? "The recorder is now writing a dry-run trace for everything it sees."
                    : "The recorder is writing nothing.")
            } catch {
                // Put the box back. It is the one control on a page about what
                // the product keeps, and leaving it showing a position the
                // daemon refused is the worst thing this view can do.
                sender.state = binding.next ? .off : .on
                binding.note("Could not change the recorder: \(error)")
            }
        }
    }
}

@MainActor
/// The Observe page's one toggle, and what the recorder is keeping.
///
/// A `RowView` and not a bespoke view: the label and its sentence on the left,
/// the switch on the right, and the row is a stack so it measures itself from
/// the two of them. The mode sentence is what makes this page safe to read —
/// a toggle that says "records key events, nothing else" tells a person what
/// the product will keep about them, and a bare switch labelled "Record what
/// CrossOS sees" does not.
private final class ObserveBody: NSStackView {
    /// `service` and `note` because the toggle has to be able to write.
    ///
    /// It could not. `NSButton(checkboxWithTitle: "", target: nil, action: nil)`,
    /// and nothing anywhere in the shell assigns an action to it —
    /// `grep '.action ='` finds FinalViews, TableView, Views, ListViews2 and
    /// RemainingViews, and not this file. So the page's only control flipped
    /// its own box and reached nothing: `core.setObserve` has been served by
    /// the daemon all along, and the shell had no client call for it.
    init(state: ObserveStateRow,
         service: any CoreClient,
         note: @escaping @Sendable (String) -> Void) {
        super.init(frame: .zero)
        orientation = .vertical
        alignment = .width
        distribution = .fill
        translatesAutoresizingMaskIntoConstraints = false

        // A CHECKBOX and not an NSSwitch, and the reason is not that a
        // checkbox looks better.
        //
        // `NSSwitch` draws through a path a PDF context cannot reach: its
        // `cell.draw(withFrame:in:)` returns nil and the bitmap comes out
        // empty, which is why every rendered page was missing its switches
        // while their labels were there. `NSButton(checkboxWithTitle:)` draws
        // through its cell and renders — measured, not argued.
        //
        // So this is a checkbox on two counts. It renders, and it is what a
        // macOS settings row uses for a boolean that is not "on by default
        // and quiet about it" — System Settings draws `NSSwitch` in the
        // narrower panes and a checkbox where the row has a label and a
        // sentence, which is exactly this row.
        let toggle = NSButton(checkboxWithTitle: "", target: nil, action: nil)
        toggle.state = state.observe ? .on : .off
        toggle.setContentHuggingPriority(.defaultHigh, for: .horizontal)
        // The row was NOT the target. `RowView` has no `mouseDown` and there
        // is no gesture recognizer anywhere in the shell, so the checkbox was
        // the only clickable thing and it did nothing when clicked. Both
        // halves of that are fixed: the button's frame now meets
        // `Measure.hitTarget`, and `RowView.mouseDown` forwards a click on the
        // label to this button so the whole row works.
        let proxy = ObserveTarget.shared.register(toggle, next: !state.observe, service: service, note: note)
        toggle.target = proxy
        toggle.action = #selector(ObserveTarget.fire(_:))

        let row = RowView(
            label: "Record what CrossOS sees",
            detail: Self.describe(mode: state.mode),
            control: toggle
        )
        addArrangedSubview(row)
    }

    @available(*, unavailable)
    required init?(coder: NSCoder) { fatalError("ObserveBody is created in code") }

    /// Say what the mode DOES, in the daemon's words where they exist.
    ///
    /// A mode named without its consequence is a number. "Records key events,
    /// nothing else" tells somebody what the product will keep about them,
    /// which is the only reason the field is reported rather than settable —
    /// a page that could set it would be a second, quieter way to decide what
    /// a product that watches every keystroke keeps.
    static func describe(mode: String) -> String {
        switch mode {
        case "keys": return "Records which keys were pressed, and nothing else."
        case "all": return "Records key events, window titles and app switches."
        default: return "Records nothing."
        }
    }
}

// MARK: - audit

/// What CrossOS created on this machine.
@MainActor
/// What CrossOS created on this machine — the Safety page's rollback list.
///
/// This was a stub that called `core.eventLogs` and then said "nothing to
/// roll back", with the error text naming a method it had not called. It
/// reported honestly ("the daemon did not answer safety.ownershipAudit") and
/// wrongly at the same time, which is the worst of both: the sentence was
/// true and the answer was not.
final class AuditListView: CardControl {
    override init(control: Control, context: ControlContext) {
        super.init(control: control, context: context)
        Task { await load(context: context) }
    }

    private func load(context: ControlContext) async {
        do {
            let rows = try await context.service.ownershipAudit()
            guard !rows.isEmpty else {
                replaceBody(with: EmptyStateView(
                    headline: "Nothing to roll back.",
                    detail: "CrossOS has created nothing on this machine. The list names the login "
                          + "item, the extension and the config file it owns, so Reset Everything "
                          + "can scope to them and leave the rest alone."
                ))
                return
            }
            let column = NSStackView()
            column.orientation = .vertical
            column.alignment = .width
            column.spacing = Gap.tight
            column.translatesAutoresizingMaskIntoConstraints = false
            for row in rows {
                let line = NSTextField(labelWithString: "\(row.resource) — \(row.id)")
                line.font = Typeface.body
                line.textColor = Palette.primaryInk
                line.lineBreakMode = .byTruncatingMiddle
                line.maximumNumberOfLines = 1
                column.addArrangedSubview(line)

                // The owner and the moment, in the second tone. "What is this
                // and is it mine" is the question the list exists to answer,
                // and an id alone answers half of it.
                let detail = NSTextField(
                    labelWithString: "\(row.owner) · \(row.createdAt)"
                )
                detail.font = Typeface.caption
                detail.textColor = Palette.secondaryInk
                detail.lineBreakMode = .byTruncatingMiddle
                detail.maximumNumberOfLines = 1
                column.addArrangedSubview(detail)
            }
            replaceBody(with: column)
        } catch {
            showError(
                "Could not read the audit list.",
                "The daemon did not answer safety.ownershipAudit. Is it running? (./scripts/run.sh) \(error)",
                onRetry: { [weak self] in Task { await self?.load(context: context) } }
            )
        }
    }
}

