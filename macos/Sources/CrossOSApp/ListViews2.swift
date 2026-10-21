import AppKit
import CrossOSCore

// The rest of the kinds.
//
// Same shape as ListViews.swift: a card, a heading, a body the daemon fills.
// What differs is the amount of local state, and that is the measurement the
// React audit produced — every renderer ran about one `useState` per hundred
// lines, so the data was always the daemon's and only the interaction was
// local. Here "local" means a closure, which is what the AppKit equivalent of
// `useState` is when the state is one boolean or one string.

// MARK: - profiles

/// Apply a whole configuration at once.
@MainActor
final class ProfileListView: CardControl {
    override init(control: Control, context: ControlContext) {
        super.init(control: control, context: context)
        Task { await load(context: context) }
    }

    private func load(context: ControlContext) async {
        guard let profiles = try? await context.service.profiles() else {
            showError(
                "Could not read the profiles.",
                didNotAnswer("core.profiles"),
                onRetry: { [weak self] in Task { await self?.load(context: context) } }
            )
            return
        }
        guard !profiles.isEmpty else {
            replaceBody(with: EmptyStateView(
                headline: "No profiles.",
                detail: "A profile is rules, shortcuts and extensions together. None are defined yet."
            ))
            return
        }
        let stack = NSStackView()
        stack.orientation = .vertical
        stack.alignment = .leading
        stack.spacing = Gap.row
        for profile in profiles {
            stack.addArrangedSubview(ProfileCard(
                profile: profile,
                service: context.service,
                note: context.note
            ))
        }
        replaceBody(with: stack)
    }
}

@MainActor
private final class ProfileCard: NSStackView {
    init(profile: ProfileRow, service: any CoreClient, note: @escaping @Sendable (String) -> Void) {
        super.init(frame: .zero)
        orientation = .vertical
        alignment = .width
        distribution = .fill
        translatesAutoresizingMaskIntoConstraints = false

        let title = NSTextField(labelWithString: profile.label)
        title.font = Typeface.bodyStrong
        title.textColor = Palette.primaryInk

        let detail = NSTextField(labelWithString: profile.description)
        detail.font = Typeface.caption
        detail.textColor = Palette.secondaryInk
        detail.lineBreakMode = .byWordWrapping
        detail.maximumNumberOfLines = 0

        let header = NSStackView(views: [title])
        header.orientation = .horizontal
        header.alignment = .centerY
        header.spacing = Gap.row

        // The counts, and what they mean. A profile that will enable three
        // things and of those one is already on is a different promise from
        // "applies 3 changes", and a card that says only the second is a card
        // that over-promises.
        //
        // EVERY capability is named, and one that changes nothing is left out.
        //
        // It labelled only the first: measured on `core.profiles`, a profile
        // with four capabilities rendered as
        //
        //     Windows Keyboard Shortcuts: 0 on, 0 already · 0 on, 0 already
        //     · 0 on, 0 already · 0 on, 0 already
        //
        // — one name and three unlabelled counts, so a reader cannot tell
        // which is which and the line reads as the same thing repeated four
        // times. And a capability that will enable nothing and already has
        // nothing on is not a fact about this profile; it is noise between the
        // ones that are.
        let relevant = (profile.capabilities ?? []).filter {
            $0.willEnable > 0 || $0.alreadyOn > 0
        }
        let summary = relevant
            .map { "\($0.label): \($0.willEnable) on, \($0.alreadyOn) already" }
            .joined(separator: " · ")

        let body = NSStackView(views: [detail])
        body.orientation = .vertical
        body.alignment = .leading
        body.spacing = Gap.tight
        if !summary.isEmpty {
            let counts = NSTextField(labelWithString: summary)
            counts.font = Typeface.caption
            counts.textColor = Palette.tertiaryInk
            counts.lineBreakMode = .byWordWrapping
            counts.maximumNumberOfLines = 0
            body.addArrangedSubview(counts)
        }

        let column = NSStackView(views: [header, body])
        column.orientation = .vertical
        column.alignment = .leading
        column.spacing = Gap.tight
        column.translatesAutoresizingMaskIntoConstraints = false

        if profile.active {
            let applied = NSTextField(labelWithString: "Active")
            applied.font = Typeface.caption
            applied.textColor = Palette.okInk
            header.addArrangedSubview(applied)
        } else {
            // A button only where there is a button's worth of point. The
            // active profile has nothing to apply, and a greyed button on the
            // row somebody is looking at says "broken" rather than "already
            // there".
            let apply = NSButton(title: "Apply", target: ApplyTarget.shared, action: nil)
            apply.bezelStyle = .push
            // ⌘↩, which is what a macOS default button is on. Measured
            // before this: every button in the app reported `key=NONE`, so
            // applying a profile — the whole point of the page — could not be
            // done from the keyboard.
            apply.keyEquivalent = "\r"
            apply.keyEquivalentModifierMask = [.command]
            apply.target = ApplyTarget.shared
            let proxy = ApplyTarget.shared.register(apply, profile: profile, service: service, note: note)
            apply.target = proxy
            apply.action = #selector(ApplyTarget.fire(_:))
            header.addArrangedSubview(apply)
        }

        addSubview(column)
        NSLayoutConstraint.activate([
            column.leadingAnchor.constraint(equalTo: leadingAnchor),
            column.trailingAnchor.constraint(equalTo: trailingAnchor),
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
    required init?(coder: NSCoder) { fatalError("ProfileCard is created in code") }
}

@MainActor
final class ApplyTarget: NSObject {
    static let shared = ApplyTarget()
    private var bindings: [ObjectIdentifier: (ProfileRow, any CoreClient, @Sendable (String) -> Void)] = [:]

    @discardableResult
    func register(
        _ button: NSButton,
        profile: ProfileRow,
        service: any CoreClient,
        note: @escaping @Sendable (String) -> Void
    ) -> ApplyTarget {
        bindings[ObjectIdentifier(button)] = (profile, service, note)
        return self
    }

    @objc func fire(_ sender: NSButton) {
        guard let (profile, service, note) = bindings[ObjectIdentifier(sender)] else { return }
        Task {
            do {
                let answer = try await service.applyProfile(id: profile.id)
                note(answer.objectValue?["message"]?.stringValue ?? "Applied \(profile.label).")
            } catch {
                note("Could not apply \(profile.label): \(error)")
            }
        }
    }
}

// MARK: - file types

/// The Explorer's file-type catalog: what the New menu offers.
@MainActor
final class FileTypeListView: CardControl {
    override init(control: Control, context: ControlContext) {
        super.init(control: control, context: context)
        Task { await load(context: context) }
    }

    private func load(context: ControlContext) async {
        guard let rows = try? await context.service.fileTypes() else {
            showError(
                "Could not read the file types.",
                didNotAnswer("core.fileTypes"),
                onRetry: { [weak self] in Task { await self?.load(context: context) } }
            )
            return
        }
        replaceBody(with: FileTypeTable(rows: rows, service: context.service, note: context.note))
    }
}

@MainActor
private final class FileTypeTable: NSStackView, TableContent {
    let columns: [(String, CGFloat)] = [
        ("Name", 180), ("Extension", 110), ("Menu title", 180), ("On", 44)
    ]
    /// The rows, in the order the daemon holds them. Not a `let`: a drag
    /// replaces the list with the order the daemon answered.
    private var entries: [FileTypeRow]
    private let service: any CoreClient
    private let note: @Sendable (String) -> Void
    private var stored: [String: Bool] = [:]
    /// The table this content draws, so a write can make it redraw the row.
    private var table: TableView?

    init(rows: [FileTypeRow], service: any CoreClient, note: @escaping @Sendable (String) -> Void) {
        self.entries = rows
        self.service = service
        self.note = note
        super.init(frame: .zero)
        orientation = .vertical
        alignment = .width
        distribution = .fill
        translatesAutoresizingMaskIntoConstraints = false
        let table = TableView(content: self)
        self.table = table
        // ARRANGED, and no edge constraints. Pinning a view to all four of a
        // stack's edges conflicts with the width binding `TableView` makes on
        // `viewDidMoveToSuperview`, and the two constraints fight to a draw:
        // measured on `core.commands`, `TableView 0x208` — zero wide — inside
        // a group that rendered its columns at 489pt anyway and drew its
        // alternating stripes and a vertical seam straight past the group's
        // rounded edge.
        //
        // This is the same conflict as `Card.setContent` pinning content to
        // the card's own edges and cancelling its insets, and the same
        // conflict as `FactRow`'s `addSubview` measuring nothing. Arranged
        // and hugging is what the layout wants.
        addArrangedSubview(table)
        setContentHuggingPriority(.required, for: .vertical)
    }

    @available(*, unavailable)
    required init?(coder: NSCoder) { fatalError("FileTypeTable is created in code") }

    var rowCount: Int { entries.count }

    /// The identity is `(ext, baseName)`, not the display name.
    ///
    /// A display name is what the person reads and it is not unique — two
    /// presets can both be called "Markdown". Sending the name back as an id
    /// would toggle whichever one the daemon found first, which is a bug that
    /// looks like the toggle working.
    private func identity(_ row: FileTypeRow) -> String { "\(row.ext)|\(row.baseName)" }

    func text(row: Int, column: Int) -> String {
        let entry = entries[row]
        switch column {
        case 0: return entry.displayName
        case 1: return entry.ext
        case 2: return entry.menuTitle
        default: return ""
        }
    }

    func toggle(row: Int, column: Int) -> (isOn: Bool, onChange: @MainActor (Bool) -> Void)? {
        guard column == 3 else { return nil }
        let entry = entries[row]
        let key = identity(entry)
        let current = stored[key] ?? entry.enabled
        return (current, { [weak self] next in
            Task { await self?.write(entry: entry, next: next) }
        })
    }

    func isDimmed(row: Int) -> Bool { !entries[row].enabled }

    /// **The catalog can be dragged into a new order.**
    ///
    /// The order is the order Finder's New menu offers, and the daemon has
    /// answered `core.reorderFileTypes` since the catalog was written — the
    /// write takes the WHOLE list, in the new order, and refuses a list that
    /// does not name every row exactly once, so a drag that lost a row
    /// cannot half-apply.
    var canReorder: Bool { true }

    func reorder(row: Int, to destination: Int) -> (@MainActor () async -> Void)? {
        guard row >= 0, row < entries.count, destination >= 0, destination < entries.count else { return nil }
        var order = entries
        let moved = order.remove(at: row)
        order.insert(moved, at: destination)
        return { [weak self] in
            guard let self else { return }
            do {
                let rows = try await service.reorderFileTypes(order)
                entries = rows
                stored = [:]
                table?.reload()
                note("The New menu now offers \(rows.count) file types in the new order.")
            } catch {
                note("Could not reorder the file types: \(error)")
            }
        }
    }



    private func write(entry: FileTypeRow, next: Bool) async {
        do {
            // The answer is the WHOLE catalog, not the row and not a count.
            // A partial answer would leave the table showing a row the daemon
            // has already replaced, and the next poll would have no way to tell
            // which of the two is true.
            let updated = try await service.setFileType(entry: entry, enabled: next)
            stored[identity(entry)] = updated.first { identity($0) == identity(entry) }?.enabled ?? next
            // The row answers to what was STORED.
            table?.reload()
            note("\(entry.displayName) is now \(next ? "offered" : "hidden").")
        } catch {
            table?.reload()
            note("Could not change \(entry.displayName): \(error)")
        }
    }
}

// MARK: - traces

/// What CrossOS saw and did this session.
@MainActor
final class PipelineTraceView: CardControl {
    override init(control: Control, context: ControlContext) {
        super.init(control: control, context: context)
        Task { await load(context: context) }
    }

    private func load(context: ControlContext) async {
        do {
            let rows = try await context.service.traces()
            guard !rows.isEmpty else {
                replaceBody(with: EmptyStateView(
                    headline: "Nothing recorded yet.",
                    detail: "Traces appear once CrossOS has seen a key and decided what to do with it."
                ))
                return
            }
            replaceBody(with: TraceTable(rows: rows, service: context.service, note: context.note))
        } catch {
            showError(
                "Could not read the timeline.",
                didNotAnswer("core.traces"),
                onRetry: { [weak self] in Task { await self?.load(context: context) } }
            )
        }
    }
}

@MainActor
private final class TraceTable: NSStackView, TableContent {
    let columns: [(String, CGFloat)] = [
        ("When", 150), ("Chord", 120), ("Decision", 200), ("App", 160)
    ]
    private let entries: [TraceRow]
    private let service: any CoreClient
    private let note: @Sendable (String) -> Void

    init(rows: [TraceRow], service: any CoreClient, note: @escaping @Sendable (String) -> Void) {
        self.entries = rows
        self.service = service
        self.note = note
        super.init(frame: .zero)
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
    required init?(coder: NSCoder) { fatalError("TraceTable is created in code") }

    var rowCount: Int { entries.count }

    func text(row: Int, column: Int) -> String {
        let entry = entries[row]
        switch column {
        case 0: return entry.at
        case 1: return Chord.format(entry.event.keys)
        case 2: return "\(entry.winner) → \(Humanize.phrase(entry.action))"
        default: return entry.context.appID
        }
    }

    func isDimmed(row: Int) -> Bool {
        // A decision with losers is a decision that was contested. Showing it
        // dimmer is the whole reason the losers are in the row at all — a
        // contested key and an uncontested one look the same otherwise, and
        // "why did MY rule not fire" is the question this page exists for.
        !(entries[row].losers ?? []).isEmpty
    }
}

// MARK: - plugins

/// What CrossOS can do, and what you have switched on.
@MainActor
final class PluginListView: CardControl {
    /// The one reason every plugin reports, when they all report the same one.
    ///
    /// `core.pluginMeta` gives all three builtin plugins the same
    /// `reason` and `loaded: false` — they are compiled from the rule table
    /// and have no manifest. The row knows this is SHARED because it can
    /// compare against this, and stays quiet; the list states it once.
    static var sharedLoadReason: String = ""

    override init(control: Control, context: ControlContext) {
        super.init(control: control, context: context)
        Task { await load(context: context) }
    }

    private func load(context: ControlContext) async {
        async let meta = try? context.service.pluginMeta()
        async let state = try? context.service.plugins()
        let (metaRows, pluginStates) = await (meta, state)

        guard let metaRows, !metaRows.isEmpty else {
            replaceBody(with: EmptyStateView(
                headline: "No plugins loaded.",
                detail: "Plugins are how CrossOS does things beyond remapping keys. None are installed."
            ))
            return
        }
        let enabled = Dictionary(
            uniqueKeysWithValues: (pluginStates ?? []).map { ($0.id, $0.enabled) }
        )
        let stack = NSStackView()
        stack.orientation = .vertical
        stack.alignment = .leading
        stack.distribution = .fill
        stack.spacing = Gap.row

        // A reason SHARED by every plugin is one sentence for the group, not
        // one per row. Three identical orange lines read as three problems;
        // measured on `core.extensions`, that is what it rendered.
        let reasons = metaRows.compactMap { row -> String? in
            guard !row.loaded, let reason = row.reason, !reason.isEmpty else { return nil }
            return reason
        }
        let shared = (reasons.count == metaRows.count && !reasons.isEmpty)
            ? (reasons.first ?? "") : ""
        PluginListView.sharedLoadReason = shared

        if !shared.isEmpty {
            let field = NSTextField(labelWithString: shared)
            field.font = Typeface.caption
            field.textColor = Palette.tertiaryInk
            field.lineBreakMode = .byWordWrapping
            field.usesSingleLineMode = false
            field.maximumNumberOfLines = 0
            // It takes the width of the group and WRAPS in it. Without it the
            // sentence keeps its own width — measured at 397pt in a 600pt card
            // — and is placed at the trailing edge of the space it did not
            // take, so the reason starts 200pt into the card and runs off
            // its right edge.
            field.setContentCompressionResistancePriority(.defaultLow, for: .horizontal)
            stack.addArrangedSubview(field)
            field.widthAnchor.constraint(equalTo: stack.widthAnchor).isActive = true
            stack.setCustomSpacing(Gap.row, after: field)
        }

        for meta in metaRows {
            let row = PluginRow(
                meta: meta,
                isOn: enabled[meta.id] ?? false,
                service: context.service,
                note: context.note
            )
            stack.addArrangedSubview(row)
            // **The row is the width of the group.**
            //
            // A `.leading`-aligned stack gives each row the width of its own
            // content, and a row's content is the plugin's name — so each
            // plugin's checkbox lands where that NAME ends. Measured on
            // `core.extensions`: three checkboxes at x=1008, x=1112 and
            // x=1008, in three rows that are supposed to be three rows of one
            // list.
            //
            // The card's own alignment is `.leading` — that is the card's
            // rule, and the comment on `Card.setContent` says why: with
            // `.width` down there the rows stretched to the card's width and
            // the text sat at the trailing edge, which is a card that fills
            // and is unreadable. So the width is stated here, per row, and
            // the alignment stays the card's.
            row.widthAnchor.constraint(equalTo: stack.widthAnchor).isActive = true
        }
        replaceBody(with: stack)
    }
}

@MainActor
private final class PluginRow: NSStackView {
    init(meta: PluginMetaRow, isOn: Bool, service: any CoreClient, note: @escaping @Sendable (String) -> Void) {
        super.init(frame: .zero)
        orientation = .vertical
        alignment = .width
        distribution = .fill
        translatesAutoresizingMaskIntoConstraints = false
        // **The row takes the width it is offered**, so the checkbox in it
        // lands on the TRAILING edge of the card — and the three of them
        // land on the same edge, in a column.
        //
        // Without it the row keeps the width of its own content and is
        // placed at the trailing edge of the space it did not take:
        // measured on `core.extensions`, three rows of 237pt, 288pt and
        // 237pt at x=383, x=332 and x=383 in a 600pt card.
        setContentCompressionResistancePriority(.defaultLow, for: .horizontal)

        // A checkbox, for the reason in ObserveBody: `NSSwitch` does not draw
        // into a PDF context and a checkbox does, and a settings row with a
        // label and a sentence wants a checkbox anyway.
        let toggle = NSButton(checkboxWithTitle: "", target: nil, action: nil)
        toggle.state = isOn ? .on : .off
        toggle.target = PluginTarget.shared
        let proxy = PluginTarget.shared.register(toggle, id: meta.id, next: !isOn, service: service, note: note)
        toggle.target = proxy
        toggle.action = #selector(PluginTarget.fire(_:))

        // A plugin always has a NAME on this row.
        //
        // `core.pluginMeta` sends `"name": ""` for all three builtin plugins —
        // they are compiled from the rule table and have no manifest to name
        // them — so the Extensions page rendered three rows with a checkbox, a
        // reason, and no label at all: three identical orange sentences a
        // reader cannot tell apart and cannot act on.
        //
        // The id is what the daemon does have, and it is humanised rather than
        // printed raw: "windows-keyboard" is the schema, "Windows keyboard" is
        // the plugin.
        let displayName = meta.name.isEmpty
            ? Humanize.phrase(meta.id.replacingOccurrences(of: "-", with: " "))
            : meta.name

        // A `RowView` and not a hand-built header: the title on the left with
        // low hugging so the control wins the right edge, which is what every
        // other row in the app does. Two ways of building one row is how the
        // checkbox ended up outside every exemption the audit knows about.
        let header = RowView(label: displayName, control: toggle)

        let column = NSStackView(views: [header])
        column.orientation = .vertical
        column.alignment = .leading
        column.spacing = Gap.tight

        // A plugin that did not load says WHY — but only when the reason is its
        // OWN.
        //
        // All three builtin plugins report the same one: "no manifest is
        // loaded — the builtin plugins are compiled from the rule table".
        // Rendered per row, that is the same orange sentence three times on the
        // page, which reads as three problems and is one. The shared case is
        // stated once under the group by `PluginListView`, and the row says
        // nothing about it.
        //
        // The reason stays in the row when it is specific to that plugin —
        // one plugin failing to load for its own reason is that plugin's news.
        let shared = PluginListView.sharedLoadReason
        let reasonIsShared = !meta.loaded && meta.reason == shared && !shared.isEmpty

        if !meta.loaded, let reason = meta.reason, !reason.isEmpty, !reasonIsShared {
            let field = NSTextField(labelWithString: reason)
            field.font = Typeface.caption
            field.textColor = Palette.warnInk
            field.lineBreakMode = .byWordWrapping
            field.usesSingleLineMode = false
            field.maximumNumberOfLines = 0
            column.addArrangedSubview(field)
        } else if let permissions = meta.permissions, !permissions.isEmpty {
            let field = NSTextField(labelWithString: "Asks for: " + permissions.joined(separator: ", "))
            field.font = Typeface.caption
            field.textColor = Palette.tertiaryInk
            field.lineBreakMode = .byWordWrapping
            field.maximumNumberOfLines = 0
            column.addArrangedSubview(field)
        }

        column.translatesAutoresizingMaskIntoConstraints = false
        // **The header is the width of the row**, so the checkbox in it goes
        // to the trailing edge of the row rather than to the end of the
        // plugin's name. The row is pinned to the group's width by
        // `PluginListView`, and this is the other half of that: the row
        // fills, and what is IN the row fills the row.
        header.widthAnchor.constraint(equalTo: column.widthAnchor).isActive = true
        for case let field as NSTextField in column.arrangedSubviews {
            field.widthAnchor.constraint(equalTo: column.widthAnchor).isActive = true
        }
        addSubview(column)
        NSLayoutConstraint.activate([
            column.leadingAnchor.constraint(equalTo: leadingAnchor),
            column.trailingAnchor.constraint(equalTo: trailingAnchor),
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
    required init?(coder: NSCoder) { fatalError("PluginRow is created in code") }
}

@MainActor
final class PluginTarget: NSObject {
    static let shared = PluginTarget()
    private struct Binding {
        let id: String
        let next: Bool
        let service: any CoreClient
        let note: @Sendable (String) -> Void
    }
    private var bindings: [ObjectIdentifier: Binding] = [:]

    @discardableResult
    func register(
        _ toggle: NSButton,
        id: String,
        next: Bool,
        service: any CoreClient,
        note: @escaping @Sendable (String) -> Void
    ) -> PluginTarget {
        bindings[ObjectIdentifier(toggle)] = Binding(id: id, next: next, service: service, note: note)
        return self
    }

    /// Whether this switch has a binding behind it.
    func isWired(_ toggle: NSButton) -> Bool {
        bindings[ObjectIdentifier(toggle)] != nil
    }

    @objc func fire(_ sender: NSSwitch) {
        guard let binding = bindings[ObjectIdentifier(sender)] else { return }
        Task {
            do {
                try await binding.service.setPluginEnabled(id: binding.id, enabled: binding.next)
                binding.note("\(binding.id) is now \(binding.next ? "on" : "off").")
            } catch {
                binding.note("Could not change \(binding.id): \(error)")
            }
        }
    }
}
