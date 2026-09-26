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
            showError("Could not read the profiles.", "The daemon did not answer core.profiles.")
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
private final class ProfileCard: NSView {
    init(profile: ProfileRow, service: any CoreClient, note: @escaping @Sendable (String) -> Void) {
        super.init(frame: .zero)

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
        var summary = ""
        for capability in profile.capabilities ?? [] {
            if summary.isEmpty { summary = "\(capability.label): " }
            else { summary += " · " }
            summary += "\(capability.willEnable) on, \(capability.alreadyOn) already"
        }

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
            applied.textColor = Palette.ok
            header.addArrangedSubview(applied)
        } else {
            // A button only where there is a button's worth of point. The
            // active profile has nothing to apply, and a greyed button on the
            // row somebody is looking at says "broken" rather than "already
            // there".
            let apply = NSButton(title: "Apply", target: ApplyTarget.shared, action: nil)
            apply.bezelStyle = .rounded
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
            showError("Could not read the file types.", "The daemon did not answer core.fileTypes.")
            return
        }
        replaceBody(with: FileTypeTable(rows: rows, service: context.service, note: context.note))
    }
}

@MainActor
private final class FileTypeTable: NSView, TableContent {
    let columns: [(String, CGFloat)] = [
        ("Name", 180), ("Extension", 110), ("Menu title", 180), ("On", 44)
    ]
    private let entries: [FileTypeRow]
    private let service: any CoreClient
    private let note: @Sendable (String) -> Void
    private var stored: [String: Bool] = [:]

    init(rows: [FileTypeRow], service: any CoreClient, note: @escaping @Sendable (String) -> Void) {
        self.entries = rows
        self.service = service
        self.note = note
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

    private func write(entry: FileTypeRow, next: Bool) async {
        do {
            // The answer is the WHOLE catalog, not the row and not a count.
            // A partial answer would leave the table showing a row the daemon
            // has already replaced, and the next poll would have no way to tell
            // which of the two is true.
            let updated = try await service.setFileType(entry: entry, enabled: next)
            stored[identity(entry)] = updated.first { identity($0) == identity(entry) }?.enabled ?? next
            note("\(entry.displayName) is now \(next ? "offered" : "hidden").")
        } catch {
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
            showError("Could not read the timeline.", "The daemon did not answer core.traces.")
        }
    }
}

@MainActor
private final class TraceTable: NSView, TableContent {
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
        stack.spacing = Gap.row
        for meta in metaRows {
            stack.addArrangedSubview(PluginRow(
                meta: meta,
                isOn: enabled[meta.id] ?? false,
                service: context.service,
                note: context.note
            ))
        }
        replaceBody(with: stack)
    }
}

@MainActor
private final class PluginRow: NSView {
    init(meta: PluginMetaRow, isOn: Bool, service: any CoreClient, note: @escaping @Sendable (String) -> Void) {
        super.init(frame: .zero)

        let toggle = NSSwitch()
        toggle.state = isOn ? .on : .off
        toggle.target = PluginTarget.shared
        let proxy = PluginTarget.shared.register(toggle, id: meta.id, next: !isOn, service: service, note: note)
        toggle.target = proxy
        toggle.action = #selector(PluginTarget.fire(_:))

        let title = NSTextField(labelWithString: meta.name)
        title.font = Typeface.body
        title.textColor = Palette.primaryInk

        let header = NSStackView(views: [title, toggle])
        header.orientation = .horizontal
        header.alignment = .centerY
        header.spacing = Gap.group
        header.setHuggingPriority(.defaultLow, for: .horizontal)

        let column = NSStackView(views: [header])
        column.orientation = .vertical
        column.alignment = .leading
        column.spacing = Gap.tight

        // A plugin that did not load says WHY, in words, in the row. A plugin
        // listed with a switch and no reason is a control that does nothing and
        // a person cannot tell whether it is broken or switched off.
        if !meta.loaded, let reason = meta.reason, !reason.isEmpty {
            let field = NSTextField(labelWithString: reason)
            field.font = Typeface.caption
            field.textColor = Palette.warn
            field.lineBreakMode = .byWordWrapping
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
        addSubview(column)
        NSLayoutConstraint.activate([
            column.leadingAnchor.constraint(equalTo: leadingAnchor),
            column.trailingAnchor.constraint(equalTo: trailingAnchor),
            column.topAnchor.constraint(equalTo: topAnchor),
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
        _ toggle: NSSwitch,
        id: String,
        next: Bool,
        service: any CoreClient,
        note: @escaping @Sendable (String) -> Void
    ) -> PluginTarget {
        bindings[ObjectIdentifier(toggle)] = Binding(id: id, next: next, service: service, note: note)
        return self
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
