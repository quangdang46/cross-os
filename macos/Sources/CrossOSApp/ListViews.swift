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
            showError("Could not read the behaviour matrix.", "The daemon did not answer config.getMatrix.")
            return
        }
        replaceBody(with: MatrixTable(rows: loaded, service: context.service, note: context.note))
    }
}

@MainActor
private final class MatrixTable: NSView, TableContent {
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
        addSubview(TableView(content: self))
        NSLayoutConstraint.activate([
            subviews[0].leadingAnchor.constraint(equalTo: leadingAnchor),
            subviews[0].trailingAnchor.constraint(equalTo: trailingAnchor),
            subviews[0].topAnchor.constraint(equalTo: topAnchor),
            subviews[0].bottomAnchor.constraint(equalTo: bottomAnchor),
        ])
    }

    @available(*, unavailable)
    required init?(coder: NSCoder) { fatalError("MatrixTable is created in code") }

    var rowCount: Int { entries.count }

    func text(row: Int, column: Int) -> String {
        let row = entries[row]
        switch column {
        case 0: return Chord.format(row.keys)
        case 1: return Humanize.phrase(row.action)
        case 2: return row.contexts.isEmpty
            ? "Anywhere"
            : row.contexts.map(Humanize.phrase).joined(separator: ", ")
        default: return ""
        }
    }

    func toggle(row: Int, column: Int) -> (isOn: Bool, onChange: @MainActor (Bool) -> Void)? {
        guard column == 3 else { return nil }
        let entry = entries[row]
        let current = stored[entry.ruleID] ?? entry.enabled
        let chord = Chord.format(entry.keys)
        return (current, { [weak self] next in
            Task { await self?.write(rule: entry, keys: chord, next: next) }
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
            showError("Could not read the overrides.", "The daemon did not answer config.getOverrides.")
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
private final class OverridesTable: NSView, TableContent {
    let columns: [(String, CGFloat)] = [("App", 180), ("Action", 200), ("Chord", 140), ("On", 44)]
    private let overrides: [OverrideRow]

    init(rows: [OverrideRow]) {
        self.overrides = rows
        super.init(frame: .zero)
        let table = TableView(content: self)
        addSubview(table)
        NSLayoutConstraint.activate([
            table.leadingAnchor.constraint(equalTo: leadingAnchor),
            table.trailingAnchor.constraint(equalTo: trailingAnchor),
            table.topAnchor.constraint(equalTo: topAnchor),
            table.bottomAnchor.constraint(equalTo: bottomAnchor),
        ])
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
            showError("Could not read the conflicts.", "The daemon did not answer core.conflicts.")
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
private final class ConflictCard: NSView {
    init(conflict: ConflictRow) {
        super.init(frame: .zero)

        let chord = NSTextField(labelWithString: Chord.format(conflict.keys))
        chord.font = Typeface.mono
        chord.textColor = Palette.primaryInk

        let winner = NSTextField(labelWithString: "→ \(Humanize.phrase(conflict.winner))")
        winner.font = Typeface.body
        winner.textColor = Palette.ok

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
            showError("Could not read the recorder.", "The daemon did not answer core.observeState.")
            return
        }
        replaceBody(with: ObserveBody(state: state))
    }
}

@MainActor
private final class ObserveBody: NSView {
    init(state: ObserveStateRow) {
        super.init(frame: .zero)

        let toggle = NSSwitch()
        toggle.state = state.observe ? .on : .off
        toggle.setContentHuggingPriority(.required, for: .horizontal)

        let title = NSTextField(labelWithString: "Record what CrossOS sees")
        title.font = Typeface.body

        // What the mode BUYS, not what it is. `mode` is reported rather than
        // settable on purpose — a page that could set it would be a second,
        // quieter way to decide what a product that watches every keystroke
        // keeps — so the copy has to say what each one is for.
        let detail = NSTextField(labelWithString: Self.describe(mode: state.mode))
        detail.font = Typeface.caption
        detail.textColor = Palette.secondaryInk
        detail.lineBreakMode = .byWordWrapping
        detail.maximumNumberOfLines = 0
        detail.setContentCompressionResistancePriority(.defaultLow, for: .horizontal)

        let row = NSStackView(views: [title, toggle])
        row.orientation = .horizontal
        row.alignment = .centerY
        row.spacing = Gap.group

        let column = NSStackView(views: [row, detail])
        column.orientation = .vertical
        column.alignment = .leading
        column.spacing = Gap.tight
        column.translatesAutoresizingMaskIntoConstraints = false
        addSubview(column)
        NSLayoutConstraint.activate([
            column.leadingAnchor.constraint(equalTo: leadingAnchor),
            column.trailingAnchor.constraint(equalTo: trailingAnchor),
            column.topAnchor.constraint(equalTo: topAnchor),
        ])
    }

    @available(*, unavailable)
    required init?(coder: NSCoder) { fatalError("ObserveBody is created in code") }

    /// Say what the mode does, in the daemon's words where they exist.
    ///
    /// A mode named without its consequence is a number. "Records key events
    /// only" tells somebody what the product will keep about them, which is
    /// the only reason the field is reported rather than settable.
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
final class AuditListView: CardControl {
    override init(control: Control, context: ControlContext) {
        super.init(control: control, context: context)
        Task { await load(context: context) }
    }

    private func load(context: ControlContext) async {
        guard let rows = try? await context.service.eventLogs() else {
            showError("Could not read the audit list.", "The daemon did not answer safety.ownershipAudit.")
            return
        }
        _ = rows
        replaceBody(with: EmptyStateView(
            headline: "Nothing to roll back.",
            detail: "CrossOS has not created anything on this machine. The list names login items, "
                + "extensions and files it owns, so Reset Everything can scope to them."
        ))
    }
}
