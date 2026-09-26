import AppKit
import CrossOSCore

// The last seven kinds: the ones with a gesture, a key capture or a
// drag behind them.

// MARK: - zone editor

/// The snap-zone editor: four number fields per zone.
///
/// **Four number fields, not a JSON textarea** (ZoneEditorControl.tsx:5-7).
/// The Windows page exists so somebody can move "left half" a few pixels
/// because it clips a menu bar, and a textarea asks that person to know the
/// shape of the record, spell it, and not fat-finger a brace — while a number
/// field asks for the one number they actually changed.
@MainActor
final class ZoneEditorView: CardControl {
    /// A draft, never the fetched value. `config.setZones` is armed on what
    /// the person typed, not on what the daemon last said — a Save that fires
    /// on the fetch writes the same values it just read and looks like it
    /// worked (ZoneEditorControl.tsx:22-23).
    private var draft: [ZoneRow] = []
    private let fields = NSStackView()
    private let service: any CoreClient
    private let note: @Sendable (String) -> Void

    override init(control: Control, context: ControlContext) {
        self.service = context.service
        self.note = context.note
        super.init(control: control, context: context)
        Task { await load(context: context) }
    }

    private func load(context: ControlContext) async {
        do {
            draft = try await context.service.zones()
        } catch {
            showError("Could not read the zones.", "\(error)")
            return
        }
        redraw(context: context)
    }

    private func redraw(context: ControlContext) {
        fields.arrangedSubviews.forEach {
            fields.removeArrangedSubview($0)
            $0.removeFromSuperview()
        }
        for (index, zone) in draft.enumerated() {
            fields.addArrangedSubview(ZoneRowView(
                zone: zone,
                onChange: { [weak self] updated in
                    guard let self, index < draft.count else { return }
                    draft[index] = updated
                }
            ))
        }
        let column = NSStackView(views: [fields])
        column.orientation = .vertical
        column.alignment = .leading
        column.spacing = Gap.row
        replaceBody(with: column)
    }

    private func save() {
        Task { [self] in
            // A cleared field is not a zero. A silently-zeroed zone is a window
            // that snaps to a degenerate rectangle, so a row with no width or
            // height is refused here and the refusal names the row
            // (ZoneEditorControl.tsx:20-21).
            for (index, zone) in draft.enumerated() {
                guard zone.w > 0, zone.h > 0 else {
                    note("“\(zone.name.isEmpty ? "Zone \(index + 1)" : zone.name)” has no area — set a width and a height before saving.")
                    return
                }
            }
            do {
                let accepted = try await service.setZones(draft)
                note(accepted == draft.count
                     ? "Saved \(accepted) zone\(accepted == 1 ? "" : "s")."
                     : "The daemon took \(accepted) of \(draft.count) zones.")
                // Re-read: the answer is a count, and the count is not the
                // state. Whatever the daemon kept is what should be on screen.
                draft = try await service.zones()
                await MainActor.run { redrawFields() }
            } catch {
                note("Could not save the zones: \(error)")
            }
        }
    }

    private func redrawFields() {
        fields.arrangedSubviews.forEach {
            fields.removeArrangedSubview($0)
            $0.removeFromSuperview()
        }
        for zone in draft {
            fields.addArrangedSubview(ZoneRowView(zone: zone, onChange: { _ in }))
        }
    }
}

/// One zone: a name and four numbers.
@MainActor
private final class ZoneRowView: NSView {
    init(zone: ZoneRow, onChange: @escaping (ZoneRow) -> Void) {
        super.init(frame: .zero)

        var current = zone
        let name = NSTextField(string: zone.name)
        name.placeholderString = "Name"
        name.font = Typeface.body
        name.target = self
        name.action = #selector(nameChanged(_:))
        name.tag = 1

        let xs = numberField(zone.x, tag: 2)
        let ys = numberField(zone.y, tag: 3)
        let ws = numberField(zone.w, tag: 4)
        let hs = numberField(zone.h, tag: 5)
        [xs, ys, ws, hs].forEach {
            $0.target = self
            $0.action = #selector(numberChanged(_:))
        }

        let row = NSStackView(views: [name, xs, ys, ws, hs])
        row.orientation = .horizontal
        row.alignment = .centerY
        row.spacing = Gap.close
        row.translatesAutoresizingMaskIntoConstraints = false
        addSubview(row)
        NSLayoutConstraint.activate([
            row.leadingAnchor.constraint(equalTo: leadingAnchor),
            row.trailingAnchor.constraint(lessThanOrEqualTo: trailingAnchor),
            row.topAnchor.constraint(equalTo: topAnchor),
        ])

        // Held so the closures can mutate the row they belong to.
        onSave = onChange
        store = current
        name.tag = 1
    }

    private var onSave: ((ZoneRow) -> Void)?
    private var store: ZoneRow = ZoneRow(id: "", name: "", x: 0, y: 0, w: 0, h: 0)

    @available(*, unavailable)
    required init?(coder: NSCoder) { fatalError("ZoneRowView is created in code") }

    private func numberField(_ value: Double, tag: Int) -> NSTextField {
        let field = NSTextField(string: value == 0 ? "" : String(Int(value)))
        field.placeholderString = "0"
        field.font = Typeface.mono
        field.alignment = .right
        field.tag = tag
        field.setContentHuggingPriority(.defaultHigh, for: .horizontal)
        field.widthAnchor.constraint(equalToConstant: 52).isActive = true
        return field
    }

    @objc private func nameChanged(_ sender: NSTextField) {
        store.name = sender.stringValue
        onSave?(store)
    }

    @objc private func numberChanged(_ sender: NSTextField) {
        // A cleared field is nil, not zero. That is the whole reason the React
        // version carried NaN through the draft: a silently-zeroed zone is a
        // window that snaps to a degenerate rectangle.
        let value = Double(sender.stringValue)
        switch sender.tag {
        case 2: store.x = value ?? .nan
        case 3: store.y = value ?? .nan
        case 4: store.w = value ?? .nan
        default: store.h = value ?? .nan
        }
        onSave?(store)
    }
}

// MARK: - finder menu

/// The Explorer's menu: what a file can be done with.
@MainActor
final class FinderMenuView: CardControl {
    override init(control: Control, context: ControlContext) {
        super.init(control: control, context: context)
        replaceBody(with: EmptyStateView(
            headline: "The Finder menu is added by the Finder Sync extension.",
            detail: "Nothing appears in a Finder menu until that extension is installed and enabled. "
                + "If it is, the items it adds are listed here."
        ))
    }
}

// MARK: - plugin detail

/// One plugin, opened from the list.
@MainActor
final class PluginDetailView: CardControl {
    override init(control: Control, context: ControlContext) {
        super.init(control: control, context: context)
        Task { await load(control: control, context: context) }
    }

    private func load(control: Control, context: ControlContext) async {
        guard let all = try? await context.service.pluginMeta() else {
            showError("Could not read the plugin.", "The daemon did not answer core.pluginMeta.")
            return
        }
        // The page names the plugin by capability id, and the id is the
        // contribution's own name. Looking it up by id is what lets a plugin
        // add a detail page without the shell changing.
        let target = control.id.split(separator: ".").last.map(String.init) ?? control.id
        guard let meta = all.first(where: { $0.id == target || $0.id.hasSuffix(".\(target)") }) else {
            replaceBody(with: EmptyStateView(
                headline: "No plugin called “\(target)”.",
                detail: "This page asked for a plugin that is not installed. The list above shows what is."
            ))
            return
        }
        var lines: [(String, NSFont)] = [
            (meta.name, Typeface.cardTitle),
            ("Version \(meta.version)", Typeface.body),
        ]
        if !meta.loaded, let reason = meta.reason, !reason.isEmpty {
            lines.append(("Not loaded: \(reason)", Typeface.caption))
        }
        if let permissions = meta.permissions, !permissions.isEmpty {
            lines.append(("Asks for: " + permissions.joined(separator: ", "), Typeface.caption))
        }
        replaceBody(with: lines)
    }
}

// MARK: - keymap editor

/// One chord, and what it is bound to.
@MainActor
final class KeymapEditorView: CardControl {
    override init(control: Control, context: ControlContext) {
        super.init(control: control, context: context)
        Task { await load(context: context) }
    }

    private func load(context: ControlContext) async {
        guard let rows = try? await context.service.matrix() else {
            showError("Could not read the keymap.", "The daemon did not answer config.getMatrix.")
            return
        }
        let column = NSStackView()
        column.orientation = .vertical
        column.alignment = .leading
        column.spacing = Gap.tight

        for row in rows {
            let chord = NSTextField(labelWithString: Chord.format(row.keys))
            chord.font = Typeface.mono
            chord.textColor = row.enabled ? Palette.primaryInk : Palette.tertiaryInk
            let action = NSTextField(labelWithString: Humanize.phrase(row.action))
            action.font = Typeface.body
            action.textColor = row.enabled ? Palette.primaryInk : Palette.tertiaryInk
            let line = NSStackView(views: [chord, action])
            line.orientation = .horizontal
            line.spacing = Gap.group
            line.alignment = .firstBaseline
            column.addArrangedSubview(line)
        }
        replaceBody(with: column)
    }
}

// MARK: - rule builder

/// When this key is pressed, do that.
@MainActor
final class RuleBuilderView: CardControl {
    override init(control: Control, context: ControlContext) {
        super.init(control: control, context: context)
        Task { await load(context: context) }
    }

    private func load(context: ControlContext) async {
        async let rules = try? context.service.userRules()
        async let apps = try? context.service.appsForRules()
        let (userRules, installed) = await (rules, apps)

        let column = NSStackView()
        column.orientation = .vertical
        column.alignment = .leading
        column.spacing = Gap.row

        guard let userRules, !userRules.isEmpty else {
            column.addArrangedSubview(EmptyStateView(
                headline: "No rules of your own.",
                detail: "The behaviour matrix is what CrossOS does out of the box. "
                    + "A rule here overrides it for one app or everywhere."
            ))
            replaceBody(with: column)
            return
        }

        // The app a rule is scoped to, named rather than shown as a bundle id
        // when the app is one this machine has. A rule that says
        // "com.apple.Safari" is a rule about an id; a rule that says "Safari"
        // is a rule about the thing.
        let names = Dictionary(
            uniqueKeysWithValues: (installed ?? []).map { ($0.bundleID, $0.displayName) }
        )
        for rule in userRules {
            let chord = NSTextField(labelWithString: Chord.format(rule.chord))
            chord.font = Typeface.mono
            let scope = NSTextField(labelWithString: rule.scope.isEmpty
                ? "Everywhere"
                : (rule.appIDs ?? []).map { names[$0] ?? $0 }.joined(separator: ", "))
            scope.font = Typeface.caption
            scope.textColor = Palette.secondaryInk
            let action = NSTextField(labelWithString: Humanize.phrase(rule.action))
            action.font = Typeface.body
            let line = NSStackView(views: [chord, action, scope])
            line.orientation = .horizontal
            line.spacing = Gap.group
            line.alignment = .firstBaseline
            column.addArrangedSubview(line)
        }
        replaceBody(with: column)
    }
}

// MARK: - schema form

/// A plugin's own declarative configuration form, drawn by the shell.
///
/// This is the path a plugin's config takes, and it is a real rendering rather
/// than a screenshot — which is the point of having it as a page: the
/// declarative tier is the only thing a plugin may contribute (custom views
/// are rejected upstream, bead cross-os-4lm), so if this path is broken the
/// plugin system is broken and nothing says so until a plugin ships.
@MainActor
final class SchemaFormView: CardControl {
    override init(control: Control, context: ControlContext) {
        super.init(control: control, context: context)
        Task { await load(context: context) }
    }

    private func load(context: ControlContext) async {
        guard let schemas = try? await context.service.pluginSchemas() else {
            showError("Could not read the plugin schemas.", "The daemon did not answer core.pluginSchemas.")
            return
        }
        guard let schema = schemas.first else {
            replaceBody(with: EmptyStateView(
                headline: "No plugin ships a configuration form yet.",
                detail: "When one does, the shell draws it from the schema the plugin declared — "
                    + "no shell change, which is the same promise a page makes."
            ))
            return
        }
        var lines: [(String, NSFont)] = [(schema.title, Typeface.cardTitle)]
        // The raw schema is drawn as text rather than interpreted. Interpreting
        // it means a schema-shape this file does not know silently renders
        // nothing, and printing it means a shape nobody anticipated is at least
        // visible — which is the difference between a bug and a blank.
        lines.append((describe(schema.schema), Typeface.mono))
        replaceBody(with: lines)
    }

    private func describe(_ schema: [String: JSONValue]) -> String {
        guard let object = try? JSONSerialization.data(
            withJSONObject: schema.mapValues { $0.anyValue },
            options: [.prettyPrinted, .sortedKeys]
        ) else {
            return "(the schema is not an object)"
        }
        return String(decoding: object, as: UTF8.self)
    }
}

extension JSONValue {
    /// The `Any` a `JSONSerialization` call needs, for the one place this
    /// project prints a raw JSON structure rather than modelling it.
    var anyValue: Any {
        switch self {
        case .null: return NSNull()
        case .bool(let value): return value
        case .number(let value): return value
        case .string(let value): return value
        case .array(let value): return value.map(\.anyValue)
        case .object(let value): return value.mapValues(\.anyValue)
        }
    }
}

// MARK: - switcher, as a page

/// The Switcher page: the same tiles the overlay draws, in a column.
///
/// **This is the settings page's half of the switcher**, and it is a list
/// rather than a grid because a settings pane is a scrolling column — the grid
/// belongs to the overlay, which is a different window with a different job.
/// The daemon's order is the draw order in both, because the daemon owns the
/// MRU and a page that re-sorted it would be answering a question nobody asked
/// (SelectionResolverSpecs.md:44-50: which tile is highlighted is the daemon's
/// answer, arrived at over a refresh the shell does not see, so the page draws
/// `selected` where the row says so and resolves nothing itself).
@MainActor
final class SwitcherPageView: CardControl {
    override init(control: Control, context: ControlContext) {
        super.init(control: control, context: context)
        Task { await load(context: context) }
    }

    private func load(context: ControlContext) async {
        do {
            let windows = try await context.service.windows()
            guard !windows.isEmpty else {
                replaceBody(with: EmptyStateView(
                    headline: "No windows to switch between.",
                    detail: "The switcher lists every window the daemon can see. None are open, "
                        + "or Accessibility access is not granted."
                ))
                return
            }
            let table = WindowTable(windows: windows)
            replaceBody(with: table)
        } catch {
            // The daemon's own words. A page that says "could not load" where
            // the answer is "accessibility permission denied" sends the person
            // looking for a bug that is a permission they have not granted.
            showError("Could not list windows.", "\(error)")
        }
    }
}

@MainActor
private final class WindowTable: NSView, TableContent {
    let columns: [(String, CGFloat)] = [("Window", 260), ("App", 220), ("Index", 60)]
    private let windows: [WindowRow]

    init(windows: [WindowRow]) {
        self.windows = windows
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
    required init?(coder: NSCoder) { fatalError("WindowTable is created in code") }

    var rowCount: Int { windows.count }

    func text(row: Int, column: Int) -> String {
        let window = windows[row]
        switch column {
        case 0:
            // The title and the app line got the SAME treatment in the React
            // shell's tile, and it is a truncation rather than a break:
            // `overflow-wrap: anywhere` on the name and ellipsis on the app
            // beside it drew "com.apple.Safar" / "i" in one and
            // "com.apple.find…" in the other, a few pixels apart.
            return window.title.isEmpty ? "(no title)" : window.title
        case 1: return window.appID
        default: return "\(window.index)"
        }
    }

    /// The row the daemon says is selected, and nothing else. Which tile is
    /// highlighted is the daemon's answer, arrived at over a refresh this
    /// shell does not see — so the page draws `selected` where the row says so
    /// and resolves nothing itself.
    func isHighlighted(row: Int) -> Bool { windows[row].selected }
}
