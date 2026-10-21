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
            showError(
                "Could not read the zones.",
                "\(error)",
                onRetry: { [weak self] in Task { await self?.load(context: context) } }
            )
            return
        }
        // An empty editor is a state, and the audit is right to call a stack
        // with nothing in it 0pt tall — the two facts are the same fact. What
        // was missing is the SAYING of it: a pane with no rows is a question
        // the person has to answer from the rest of the page, and the answer
        // is not on the page.
        //
        // The zone NAMES are the daemon's, but their ids and names are
        // editable (ZoneEditorControl.tsx:13-18): `winlayout.DefaultZones` is
        // empty by design — the daemon holds no display geometry outside the
        // adapter — so without an Add control this editor could only ever
        // show "No snap zones are configured" and a person who wanted a snap
        // rectangle would have nowhere to put one.
        if draft.isEmpty {
            replaceBody(with: EmptyStateView(
                headline: "No snap zones.",
                detail: "A zone is a rectangle a window snaps to. None are defined, and the daemon "
                      + "ships none because it holds no display geometry of its own — the ones you "
                      + "add are stored here."
            ))
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
                },
                onDelete: { [weak self] in
                    guard let self, index < draft.count else { return }
                    draft.remove(at: index)
                    redraw(context: context)
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
        // `redrawFields` has no context, so it cannot call `redraw`. It is
        // only reached right after a save re-fetched the daemon state — the
        // button hidden keeps the signature honest rather than wiring a
        // delete that points at the wrong index.
        for zone in draft {
            fields.addArrangedSubview(ZoneRowView(zone: zone, onChange: { _ in }, onDelete: nil))
        }
    }
}

/// One zone: a name and four numbers.
@MainActor
private final class ZoneRowView: NSStackView {
    init(zone: ZoneRow, onChange: @escaping (ZoneRow) -> Void, onDelete: (() -> Void)? = nil) {
        super.init(frame: .zero)
        orientation = .vertical
        alignment = .width
        distribution = .fill
        translatesAutoresizingMaskIntoConstraints = false

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

        // A DELETE button, because a zone editor that can add and edit but
        // not remove is an editor with a one-way door. The React shell had
        // one (ZoneEditorControl.tsx:14-16) and the AppKit port dropped it.
        //
        // The button is wired to a closure so the caller decides what
        // "delete" means: remove the row from the draft, or something else.
        // The view does not know.
        let deleteButton = NSButton(title: "Delete", target: nil, action: nil)
        deleteButton.bezelStyle = .rounded
        deleteButton.controlSize = .small
        deleteButton.translatesAutoresizingMaskIntoConstraints = false
        if let onDelete {
            let proxy = DeleteTarget.shared.register(deleteButton, onDelete: onDelete)
            deleteButton.target = proxy
            deleteButton.action = #selector(DeleteTarget.fire(_:))
        } else {
            deleteButton.isHidden = true
        }

        let outer = NSStackView(views: [row, deleteButton])
        outer.orientation = .horizontal
        outer.alignment = .centerY
        outer.spacing = Gap.group
        outer.translatesAutoresizingMaskIntoConstraints = false

        // ARRANGED, not `addSubview`. A plain `addSubview` positions the view
        // but nothing measures it, and this view is a `.leading`-aligned
        // vertical stack whose height comes from its arranged subview's — so
        // with none, it measured 0pt and every field in it sat outside its own
        // frame. Measured on `core.windows`: `ZoneRowView 232x0` with a
        // `NSTextField 0x24` inside it, which the audit names as "under 20pt"
        // because the name field is a plain `NSTextField` and measured zero.
        //
        // The same defect as the card that measured 0, the row that measured
        // 0, and the plugin row's floating checkbox — and the one shape this
        // app keeps getting wrong.
        addArrangedSubview(outer)
        // Hug, so the row is as tall as the fields it holds.
        setContentHuggingPriority(.required, for: .vertical)

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

/// The delete button's target, holding the closure for one button.
///
/// A shared singleton because `NSButton.target` is a weak `NSObject`
/// reference and a closure cannot be one. The per-button proxy is what
/// carries the closure, so two delete buttons in two rows do not share state.
@MainActor
final class DeleteTarget: NSObject {
    static let shared = DeleteTarget()
    private var bindings: [ObjectIdentifier: () -> Void] = [:]

    @discardableResult
    func register(_ button: NSButton, onDelete: @escaping () -> Void) -> DeleteTarget {
        bindings[ObjectIdentifier(button)] = onDelete
        return self
    }

    @objc func fire(_ sender: NSButton) {
        bindings[ObjectIdentifier(sender)]?()
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
            showError(
                "Could not read the plugin.",
                didNotAnswer("core.pluginMeta"),
                onRetry: { [weak self] in Task { await self?.load(control: control, context: context) } }
            )
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
            showError(
                "Could not read the keymap.",
                didNotAnswer("config.getMatrix"),
                onRetry: { [weak self] in Task { await self?.load(context: context) } }
            )
            return
        }
        let column = NSStackView()
        column.orientation = .vertical
        column.alignment = .leading
        column.spacing = Gap.tight

        for row in rows {
            let chord = NSTextField(labelWithString: Chord.format(row.keys))
            // The SYSTEM face: `Chord.format` returns ⌃⌥⇧ and ← — Apple's
            // key symbols, drawn for the system face and falling back to
            // something that reads as typing in a monospace one.
            chord.font = Typeface.body
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
    /// Which load is the CURRENT one.
    ///
    /// **A load that has been overtaken does not draw.** The card loads when
    /// it appears, and it loads AGAIN after every write — and the two are
    /// concurrent, so the one that started first can finish last. Measured:
    /// the card was showing "No rules of your own yet" AFTER a rule had been
    /// added to the daemon, because the first load came back empty and
    /// replaced the body the second load had just filled. The daemon had the
    /// rule; the page said it did not.
    ///
    /// A counter is the whole fix: a load that is no longer the newest one
    /// has nothing to say.
    private var loadGeneration = 0

    override init(control: Control, context: ControlContext) {
        super.init(control: control, context: context)
        Task { await load(context: context) }
    }

    private func load(context: ControlContext) async {
        loadGeneration += 1
        let generation = loadGeneration
        async let rules = try? context.service.userRules()
        async let apps = try? context.service.appsForRules()
        async let words = try? context.service.userRuleVocabulary()
        let (userRules, installed, vocabulary) = await (rules, apps, words)
        // A newer load has the answer. This one does not.
        guard generation == loadGeneration else { return }

        let column = NSStackView()
        column.orientation = .vertical
        column.alignment = .leading
        column.spacing = Gap.row

        // **THE BUILDER COMES FIRST.**
        //
        // The page says "Add a rule: an app, an action, and a chord", and
        // until the shell called `config.setUserRule` there was nowhere to
        // add one: the card listed the rules and stopped. A builder that is
        // below the list it feeds is a builder nobody finds — the list is
        // usually empty, and an empty list with a builder under it reads as
        // "there is nothing here", not as "add one".
        if let vocabulary {
            column.addArrangedSubview(builder(vocabulary: vocabulary,
                                              apps: installed ?? [],
                                              context: context))
        }

        guard let userRules, !userRules.isEmpty else {
            column.addArrangedSubview(EmptyStateView(
                headline: "No rules of your own yet.",
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
            // The SYSTEM face: `Chord.format` returns ⌃⌥⇧ and ← — Apple's
            // key symbols, drawn for the system face and falling back to
            // something that reads as typing in a monospace one.
            chord.font = Typeface.body
            // The chord is a COLUMN, so every action starts at the same x.
            //
            // It took its own width, and the chords are not the same width:
            // measured on `core.shortcuts`, "Ctrl+Shift+Return", "Alt+Tab on
            // release" and "Win+Left" put their actions at three different x
            // positions down what reads as one list — a list whose second
            // column has no edge.
            // `.left` inside the field, because the field is wider than the
            // text: `labelWithString:` aligns naturally, which centres a short
            // chord in the 190pt column and undoes the column.
            chord.alignment = .left
            chord.widthAnchor.constraint(equalToConstant: 190).isActive = true
            chord.setContentHuggingPriority(.required, for: .horizontal)
            chord.setContentCompressionResistancePriority(.required, for: .horizontal)
            let scope = NSTextField(labelWithString: rule.scope.isEmpty
                ? "Everywhere"
                : (rule.appIDs ?? []).map { names[$0] ?? $0 }.joined(separator: ", "))
            scope.font = Typeface.caption
            scope.textColor = Palette.secondaryInk
            let action = NSTextField(labelWithString: Humanize.phrase(rule.action))
            action.font = Typeface.body
            action.alignment = .left

            // **A DELETE button on every rule.**
            //
            // A rule the user cannot take away is a rule the user has to
            // live with: the daemon's table grows and nothing in the app can
            // shrink it. The daemon has answered `config.deleteUserRule`
            // since the table was written; the shell had no call site.
            let delete = NSButton(title: "Delete", target: nil, action: nil)
            delete.bezelStyle = .rounded
            delete.controlSize = .small
            delete.translatesAutoresizingMaskIntoConstraints = false
            // **The button carries the rule's id, so a test can tell WHICH
            // rule's Delete it is looking at.**
            //
            // Every rule's button says "Delete", so "the first Delete button"
            // is "whichever rule happens to be first" — and a click test that
            // clicks it has deleted the person's own rule and called the
            // count unchanged. The identifier is AppKit's own handle for a
            // control, and it makes the pairing exact.
            delete.identifier = NSUserInterfaceItemIdentifier(rule.id)
            delete.toolTip = "Remove this rule."
            let ruleID = rule.id
            let proxy = DeleteTarget.shared.register(delete, onDelete: { [weak self] in
                Task { await self?.deleteRule(id: ruleID, context: context) }
            })
            delete.target = proxy
            delete.action = #selector(DeleteTarget.fire(_:))

            let line = NSStackView(views: [chord, action, scope, delete])
            line.orientation = .horizontal
            line.distribution = .fill
            line.spacing = Gap.group
            line.alignment = .firstBaseline
            line.translatesAutoresizingMaskIntoConstraints = false
            column.addArrangedSubview(line)
        }
        replaceBody(with: column)
    }

    /// The builder: an app, an action, and a chord.
    ///
    /// The three things the page's own note names, in that order. The pickers
    /// are the daemon's lists (`config.getUserRuleVocabulary`), so a new key
    /// or a new action is a change on the daemon's side and not a change in
    /// this file.
    private func builder(vocabulary: UserRuleVocabulary,
                         apps: [AppRow],
                         context: ControlContext) -> NSView {
        let card = Card()
        let column = NSStackView()
        column.orientation = .vertical
        column.alignment = .leading
        column.spacing = Gap.tight
        column.translatesAutoresizingMaskIntoConstraints = false

        let heading = NSTextField(labelWithString: "Add a rule")
        heading.font = Typeface.cardTitle
        column.addArrangedSubview(heading)

        // The app the rule is scoped to. "Everywhere" is the FIRST entry and
        // the default: a rule that applies everywhere is the common case, and
        // a picker whose first entry is an app is a picker that defaults to
        // the wrong thing.
        //
        // The bundle id travels in the item's `representedObject`, not in its
        // title: the title is what the person reads ("Safari") and the rule
        // needs what the daemon matches on ("com.apple.Safari").
        let appPicker = NSPopUpButton(frame: .zero, pullsDown: false)
        appPicker.menu?.addItem(withTitle: "Everywhere", action: nil, keyEquivalent: "")
        appPicker.menu?.addItem(withTitle: "Only in an app…", action: nil, keyEquivalent: "")
        appPicker.menu?.addItem(.separator())
        for app in apps {
            let item = appPicker.menu?.addItem(withTitle: app.displayName, action: nil, keyEquivalent: "")
            item?.representedObject = app.bundleID
        }
        appPicker.toolTip = "Which app the rule applies in."

        // The chord: the modifiers and the key. The modifiers are the
        // daemon's four, as checkboxes — a chord is a SET of modifiers, and
        // a popup can only hold one.
        let modifierButtons = vocabulary.modifiers.map { entry in
            let box = NSButton(checkboxWithTitle: entry.label, target: nil, action: nil)
            box.controlSize = .small
            box.translatesAutoresizingMaskIntoConstraints = false
            box.toolTip = entry.value
            // **The HIT TARGET, not just the glyph.**
            //
            // A checkbox reports an intrinsic height of 14pt at the small
            // control size, and 14pt is a target a person has to aim at. The
            // audit named it: "core.rules — NSButton is 38x14 — under 24pt",
            // four times over, once per modifier. The GLYPH stays what AppKit
            // draws; the FRAME goes to the floor — the same floor the table's
            // checkboxes and the plugin switches get.
            NSLayoutConstraint.activate([
                box.widthAnchor.constraint(greaterThanOrEqualToConstant: Measure.hitTarget),
                box.heightAnchor.constraint(greaterThanOrEqualToConstant: Measure.hitTarget),
            ])
            return (entry.value, box)
        }
        // The key's VALUE travels in the item, for the same reason: the
        // label says "Page Up" and the daemon matches on "PageUp".
        let keyPicker = NSPopUpButton(frame: .zero, pullsDown: false)
        for key in vocabulary.keys {
            let item = keyPicker.menu?.addItem(withTitle: key.label, action: nil, keyEquivalent: "")
            item?.representedObject = key.value
        }
        keyPicker.toolTip = "The key the rule listens for."

        // The action: what the rule DOES. The entry carries the capability
        // and the parameters it needs — a `window.move` rule says WHICH
        // zone — so the rule is built from the action rather than from a
        // capability name the person would have to know.
        let actionPicker = NSPopUpButton(frame: .zero, pullsDown: false)
        for action in vocabulary.actions {
            let item = actionPicker.menu?.addItem(withTitle: action.label, action: nil, keyEquivalent: "")
            var carried: [String: Any] = ["capability": action.capability ?? ""]
            if let parameters = action.parameters {
                var plain: [String: Any] = [:]
                for (name, value) in parameters {
                    if case .string(let text) = value { plain[name] = text }
                }
                carried["parameters"] = plain
            }
            item?.representedObject = carried
        }
        actionPicker.toolTip = "What the rule does."

        let add = NSButton(title: "Add rule", target: nil, action: nil)
        add.bezelStyle = .rounded
        add.keyEquivalent = "\r"
        add.keyEquivalentModifierMask = [.command]
        add.toolTip = "⌘↩ — save this rule."

        // The modifier boxes are a ROW of their own: a chord is a set of
        // modifiers, and a popup can hold only one.
        let modifierStrip = NSStackView(views: modifierButtons.map(\.1))
        modifierStrip.orientation = .horizontal
        modifierStrip.alignment = .firstBaseline
        modifierStrip.spacing = Gap.group
        modifierStrip.translatesAutoresizingMaskIntoConstraints = false

        let appRow = labelled("Applies to", appPicker)
        let modifierRow = labelled("When you press", modifierStrip)
        let keyRow = labelled("Key", keyPicker)
        // **The button is on the LAST ROW, not on a row of its own.**
        //
        // A form whose submit button sits below its last field puts the two
        // things a person looks at last, one under the other, at the bottom
        // of a card that is already taller than the window. Measured on
        // `core.rules`, the button was half off the bottom edge of the
        // picture — the one control the whole page exists for, cut in half.
        let actionRow = labelled("Do", actionPicker, add)

        let target = RuleBuilderTarget.shared
        let proxy = target.register(add, context: context, appPicker: appPicker,
                                    keyPicker: keyPicker, actionPicker: actionPicker,
                                    modifiers: modifierButtons,
                                    reload: { [weak self] in await self?.load(context: context) })
        add.target = proxy
        add.action = #selector(RuleBuilderTarget.fire(_:))

        for row in [appRow, modifierRow, keyRow, actionRow] {
            column.addArrangedSubview(row)
        }
        card.setContent(column)
        return card
    }

    /// A label and the control it names, side by side.
    ///
    /// The label is a fixed-width column so the controls line up: a form
    /// whose labels are ragged is a form nobody can scan.
    private func labelled(_ text: String, _ controls: NSView...) -> NSStackView {
        let label = NSTextField(labelWithString: text)
        label.font = Typeface.caption
        label.textColor = Palette.secondaryInk
        label.alignment = .right
        label.widthAnchor.constraint(equalToConstant: 110).isActive = true
        let row = NSStackView(views: [label] + controls)
        row.orientation = .horizontal
        row.alignment = .firstBaseline
        row.spacing = Gap.group
        row.translatesAutoresizingMaskIntoConstraints = false
        return row
    }

    /// Take a rule away, and say what happened.
    private func deleteRule(id: String, context: ControlContext) async {
        do {
            let rules = try await context.service.deleteUserRule(id: id)
            context.note("Removed \(rules.count) rule\(rules.count == 1 ? "" : "s").")
            await load(context: context)
        } catch {
            context.note("Could not remove that rule: \(error)")
        }
    }
}

/// The button behind "Add rule".
///
/// It reads the three pickers, builds the rule and hands it to the daemon.
/// The rule's ID is the daemon's to derive — it is derived from the chord and
/// the scope, so a rule's name always answers to what it matches, and the
/// shell cannot invent one.
@MainActor
final class RuleBuilderTarget: NSObject {
    static let shared = RuleBuilderTarget()
    private var bindings: [ObjectIdentifier: BuilderBinding] = [:]

    /// What the button needs to build a rule: the three pickers, the
    /// modifier boxes, and a way to redraw the list afterwards.
    struct BuilderBinding {
        let context: ControlContext
        let appPicker: NSPopUpButton
        let keyPicker: NSPopUpButton
        let actionPicker: NSPopUpButton
        let modifiers: [(String, NSButton)]
        let reload: @Sendable () async -> Void
    }

    @discardableResult
    func register(_ button: NSButton,
                  context: ControlContext,
                  appPicker: NSPopUpButton,
                  keyPicker: NSPopUpButton,
                  actionPicker: NSPopUpButton,
                  modifiers: [(String, NSButton)],
                  reload: @escaping @Sendable () async -> Void) -> RuleBuilderTarget {
        bindings[ObjectIdentifier(button)] = BuilderBinding(
            context: context,
            appPicker: appPicker,
            keyPicker: keyPicker,
            actionPicker: actionPicker,
            modifiers: modifiers,
            reload: reload
        )
        return self
    }

    @objc func fire(_ sender: NSButton) {
        guard let binding = bindings[ObjectIdentifier(sender)] else { return }
        Task { await save(binding) }
    }

    private func save(_ binding: BuilderBinding) async {
        let context = binding.context
        // The scope. "Everywhere" is index 0; "Only in an app…" is index 1
        // and asks the person to pick one below it, so the entries after the
        // separator are the apps themselves.
        let selectedApp = binding.appPicker.indexOfSelectedItem
        var appIDs: [String] = []
        if selectedApp >= 2 {
            appIDs = [binding.appPicker.item(at: selectedApp)?.representedObject as? String ?? ""]
                .filter { !$0.isEmpty }
        }
        if selectedApp == 1 {
            context.note("Pick an app below “Only in an app…”, or choose Everywhere.")
            return
        }

        let key = binding.keyPicker.selectedItem?.representedObject as? String ?? ""
        let chosen = binding.modifiers.filter { $0.1.state == .on }.map(\.0)
        let action = binding.actionPicker.selectedItem?.representedObject as? [String: Any]

        guard !key.isEmpty else {
            context.note("Pick a key for the rule.")
            return
        }
        // **A CHORD, not a key.**
        //
        // The page's own note says "an app, an action, and a chord", and a
        // chord is a key WITH modifiers. The daemon would accept a bare one —
        // `config.setUserRule` stored `user.left@any` when asked — and a rule
        // on a bare arrow key takes the arrow keys away from every app on the
        // machine. The picker's first key is "Left" and its first action is
        // "Maximize", so the one-click rule is the one that breaks the arrow
        // keys; the builder says so rather than storing it.
        guard !chosen.isEmpty else {
            context.note("Pick at least one modifier — a rule with none would take the key on its own.")
            return
        }
        guard let capability = action?["capability"] as? String, !capability.isEmpty else {
            context.note("Pick an action for the rule.")
            return
        }
        var parameters: [String: JSONValue] = [:]
        if let raw = action?["parameters"] as? [String: Any] {
            for (name, value) in raw {
                if let text = value as? String { parameters[name] = .string(text) }
            }
        }

        let draft = UserRuleDraft(key: key,
                                  modifiers: chosen,
                                  appModes: [],
                                  appIDs: appIDs,
                                  capability: capability,
                                  parameters: parameters.isEmpty ? nil : parameters)
        do {
            let id = try await context.service.createUserRule(draft)
            context.note("Added the rule \(id).")
            await binding.reload()
        } catch {
            context.note("Could not add that rule: \(error)")
        }
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
            showError(
                "Could not read the plugin schemas.",
                didNotAnswer("core.pluginSchemas"),
                onRetry: { [weak self] in Task { await self?.load(context: context) } }
            )
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
            showError(
                "Could not list windows.",
                "\(error)",
                onRetry: { [weak self] in Task { await self?.load(context: context) } }
            )
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
        // ARRANGED, not `addSubview` with edge constraints. Pinning a view to
        // This view is a plain `NSView`, not a stack, so the table is pinned
        // to all four edges — which is correct HERE and is what the other four
        // call sites got wrong: pinning to a STACK's edges fights the width
        // binding `TableView` makes, and `TableView` measured 0x208 with its
        // columns drawn past the group's rounded edge. See
        // `TableView.viewDidMoveToSuperview`.
        table.translatesAutoresizingMaskIntoConstraints = false
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
