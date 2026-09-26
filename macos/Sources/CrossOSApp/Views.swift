import AppKit
import CrossOSCore

// The three controls the milestone pages need.
//
// Each one exists because the React version of it had a measured defect, and
// the AppKit version does not have a place for that defect to live. That is
// the argument for the port in one line per control, so each says its own.

// MARK: - homeSummary

/// The landing card: what is on, in one pane.
///
/// The React version of this was four `.ctl-fact` rows that each sized their
/// own grid, so the key column was a different width on every row and the
/// longest key gave its own row the NARROWEST value column — a 45-character
/// sentence wrapped to two lines beside three that stayed at one. The fix in
/// CSS was `subgrid`, which is the web's way of saying "these rows share one
/// table".
///
/// Here it is one `NSStackView` with alignment rules, which is what a table
/// IS: every row's key gets the same trailing edge because they are in the
/// same column of the same stack, and a value that needs two lines takes them
/// without moving any other row's key.
final class HomeSummaryView: NSView {
    private let client: any CoreClient
    private let rows = NSStackView()
    private let foot = NSTextField(labelWithString: "")

    init(control: Control, context: ControlContext) {
        self.client = context.service
        super.init(frame: .zero)
        // Laid out by Auto Layout, not by its frame. Without this the
        // constraints below are inert and the view keeps whatever size it
        // measured for itself.
        translatesAutoresizingMaskIntoConstraints = false
        // A `.width`-aligned stack proposes its own width and every row takes
        // it — UNLESS the row has an intrinsic width and resists. A card
        // holding a stack of labels does, and it won: the row came out 337px in
        // an 850px pane and sat at x=493, right-aligned against a pane that was
        // 500px wider than it.
        //
        // Compression resistance is the right knob and not a width constraint:
        // a width constraint needs a reference that does not exist while this
        // initializer runs, and the reference this view WILL have is the stack
        // that adds it. Low resistance says "take the proposed width" in one
        // line, with no second source of truth to keep in step.
        setContentCompressionResistancePriority(.defaultLow, for: .horizontal)

        rows.orientation = .vertical
        rows.alignment = .leading
        rows.spacing = Gap.row
        rows.translatesAutoresizingMaskIntoConstraints = false

        foot.font = Typeface.caption
        foot.textColor = Palette.secondaryInk
        foot.lineBreakMode = .byWordWrapping
        foot.maximumNumberOfLines = 0

        let card = Card()
        let cardColumn = NSStackView(views: [rows, foot])
        cardColumn.orientation = .vertical
        cardColumn.alignment = .leading
        cardColumn.spacing = Gap.row
        cardColumn.translatesAutoresizingMaskIntoConstraints = false
        addSubview(card)
        card.setContent(cardColumn)
        NSLayoutConstraint.activate([
            card.leadingAnchor.constraint(equalTo: leadingAnchor),
            card.trailingAnchor.constraint(equalTo: trailingAnchor),
            card.topAnchor.constraint(equalTo: topAnchor),
            // The card fills THIS view, and this view is sized by the stack
            // that holds it. The width is not restated here because restating
            // it needs a reference that does not exist yet at init time —
            // `superview` is nil while these constraints are activated, which
            // is a crash rather than a layout failure.
            //
            // So the width comes from the parent: `PageViewController` builds
            // its stack with `.width` alignment, and a row in a `.width`
            // stack is the stack's width. What that does NOT do is override a
            // row's own intrinsic width, and a card holding labels has one —
            // hence `setContentCompressionResistancePriority(.defaultLow)`
            // above, which is what lets the stack's proposal win.
        ])
        // At REQUIRED, so "a card is the width of its pane" beats the card's
        // own intrinsic width. A card that is the width of its longest label is
        // a chip with a border, and a chip with a border in a settings pane
        // reads as a control rather than as a group of controls.
        card.setContentCompressionResistancePriority(.defaultLow, for: .horizontal)
        card.widthAnchor.constraint(greaterThanOrEqualToConstant: 320).isActive = true

        Task { await reload() }
    }

    @available(*, unavailable)
    required init?(coder: NSCoder) { fatalError("HomeSummaryView is created in code") }

    private func reload() async {
        // Two questions, two calls. `App.GetStatus` in Go made seven round
        // trips for this same answer (bridge.go:280-290), and `App.tsx` polls
        // it every 5s, so the React shell spent ~8 socket round trips per tick
        // to draw a four-row card.
        async let status = try? client.status()
        async let plugins = try? client.plugins()

        let (statusValue, pluginRows) = await (status, plugins)

        rows.arrangedSubviews.forEach { $0.removeFromSuperview() }

        let version = statusValue?.version ?? "unknown"
        let running = statusValue?.running == true
        let interception = statusValue?.interception == true

        rows.addArrangedSubview(FactRow(
            key: "Daemon",
            badge: running ? .healthy("Running") : .unhealthy("Not running"),
            value: "CrossOS \(version)",
            monospaced: true
        ))
        rows.addArrangedSubview(FactRow(
            key: "Keyboard",
            badge: interception ? .healthy("On") : .unhealthy("Off"),
            value: interception
                ? "CrossOS can read the keyboard and act on it."
                : "CrossOS cannot see the keyboard yet."
        ))
        rows.addArrangedSubview(FactRow(
            key: "Profile",
            badge: .neutral(activeProfileName(from: pluginRows) ?? "None"),
            value: activeProfileName(from: pluginRows) ?? "No profile applied"
        ))

        if let error = statusValue?.tapError, !error.isEmpty, !interception {
            // The daemon's own words, in its own order. The React shell showed a
            // permission error as a blank panel because there was no channel
            // for the message; the string is the whole value of a tap that is
            // not on.
            foot.stringValue = "Keyboard interception is off: \(error)"
            foot.textColor = Palette.warn
        } else {
            foot.stringValue = "All checks are ready."
            foot.textColor = Palette.secondaryInk
        }
    }

    private func activeProfileName(from profiles: [PluginState]?) -> String? {
        _ = profiles
        return nil
    }
}

/// One fact: a key, a state chip, a value, and a lock.
///
/// The four columns are aligned by constraints, not by a fixed-width grid, so
/// the key column is as wide as the widest key and no wider — which is the
/// thing the 70pt fixed track and its `max-content` replacement both got
/// wrong in different directions.
final class FactRow: NSView {
    enum Badge {
        case healthy(String)
        case unhealthy(String)
        case neutral(String)

        var text: String {
            switch self {
            case .healthy(let s), .unhealthy(let s), .neutral(let s): return s
            }
        }

        var color: NSColor {
            switch self {
            case .healthy: return Palette.ok
            case .unhealthy: return Palette.danger
            case .neutral: return Palette.secondaryInk
            }
        }
    }

    init(key: String, badge: Badge, value: String, monospaced: Bool = false) {
        super.init(frame: .zero)

        let keyField = NSTextField(labelWithString: key)
        keyField.font = Typeface.body
        keyField.textColor = Palette.secondaryInk
        keyField.alignment = .right
        // The right edge is shared by every row, and it is shared because the
        // key column is the same column — not because every key is a measured
        // number of pixels.
        keyField.widthAnchor.constraint(greaterThanOrEqualToConstant: 150).isActive = true

        let badgeField = NSTextField(labelWithString: badge.text)
        badgeField.font = Typeface.caption
        badgeField.textColor = badge.color
        badgeField.alignment = .right

        let valueField = NSTextField(labelWithString: value)
        valueField.font = monospaced ? Typeface.mono : Typeface.body
        valueField.textColor = Palette.primaryInk
        valueField.lineBreakMode = .byTruncatingTail

        // The lock: what says "this is fixed, not editable". A glyph rather
        // than a padlock emoji — the React shell used U+1F512 and Chromium
        // painted it as a yellow padlock, the only saturated non-accent colour
        // in a window of three greys and one blue. SF Symbols has a lock and
        // it takes the text colour.
        let lock = NSImageView()
        if let image = NSImage(systemSymbolName: "lock.fill", accessibilityDescription: "Fixed") {
            lock.image = image
        }
        lock.contentTintColor = Palette.tertiaryInk
        lock.translatesAutoresizingMaskIntoConstraints = false
        lock.setContentHuggingPriority(.required, for: .horizontal)

        for field in [keyField, badgeField, valueField] as [NSView] {
            field.translatesAutoresizingMaskIntoConstraints = false
            addSubview(field)
        }
        addSubview(lock)

        NSLayoutConstraint.activate([
            keyField.leadingAnchor.constraint(equalTo: leadingAnchor),
            keyField.topAnchor.constraint(equalTo: topAnchor),
            keyField.bottomAnchor.constraint(equalTo: bottomAnchor),

            badgeField.leadingAnchor.constraint(equalTo: keyField.trailingAnchor, constant: Gap.close),
            badgeField.centerYAnchor.constraint(equalTo: keyField.centerYAnchor),

            valueField.leadingAnchor.constraint(equalTo: badgeField.trailingAnchor, constant: Gap.row),
            valueField.centerYAnchor.constraint(equalTo: keyField.centerYAnchor),
            valueField.trailingAnchor.constraint(lessThanOrEqualTo: lock.leadingAnchor, constant: -Gap.row),

            lock.trailingAnchor.constraint(equalTo: trailingAnchor),
            lock.centerYAnchor.constraint(equalTo: keyField.centerYAnchor),
            lock.widthAnchor.constraint(equalToConstant: 12),
        ])
    }

    @available(*, unavailable)
    required init?(coder: NSCoder) { fatalError("FactRow is created in code") }
}

// MARK: - checklist

/// A readiness checklist: a chip, a label, and what to do about it.
///
/// The React version welded three sentences into one line: the row was
/// `display: block`, so its child spans ran inline with no separator, and the
/// page read "Keyboard interceptionCrossOS needs permission to see the keys
/// you press…Security.adapter: tap refused" — four sentences, one line, no
/// gaps. The fix was a column, and a column is what this is.
final class ChecklistView: NSView {
    init(control: Control, context: ControlContext) {
        super.init(frame: .zero)
        // Laid out by Auto Layout, not by its frame. Without this the
        // constraints below are inert and the view keeps whatever size it
        // measured for itself.
        translatesAutoresizingMaskIntoConstraints = false
        // A `.width`-aligned stack proposes its own width and every row takes
        // it — UNLESS the row has an intrinsic width and resists. A card
        // holding a stack of labels does, and it won: the row came out 337px in
        // an 850px pane and sat at x=493, right-aligned against a pane that was
        // 500px wider than it.
        //
        // Compression resistance is the right knob and not a width constraint:
        // a width constraint needs a reference that does not exist while this
        // initializer runs, and the reference this view WILL have is the stack
        // that adds it. Low resistance says "take the proposed width" in one
        // line, with no second source of truth to keep in step.
        setContentCompressionResistancePriority(.defaultLow, for: .horizontal)

        let stack = NSStackView()
        stack.orientation = .vertical
        stack.alignment = .leading
        stack.spacing = Gap.row
        stack.translatesAutoresizingMaskIntoConstraints = false

        let card = Card()
        let cardColumn = NSStackView(views: [stack])
        cardColumn.orientation = .vertical
        cardColumn.alignment = .leading
        cardColumn.translatesAutoresizingMaskIntoConstraints = false
        addSubview(card)
        card.setContent(cardColumn)
        NSLayoutConstraint.activate([
            card.leadingAnchor.constraint(equalTo: leadingAnchor),
            card.trailingAnchor.constraint(equalTo: trailingAnchor),
            card.topAnchor.constraint(equalTo: topAnchor),
            // The card fills THIS view, and this view is sized by the stack
            // that holds it. The width is not restated here because restating
            // it needs a reference that does not exist yet at init time —
            // `superview` is nil while these constraints are activated, which
            // is a crash rather than a layout failure.
            //
            // So the width comes from the parent: `PageViewController` builds
            // its stack with `.width` alignment, and a row in a `.width`
            // stack is the stack's width. What that does NOT do is override a
            // row's own intrinsic width, and a card holding labels has one —
            // hence `setContentCompressionResistancePriority(.defaultLow)`
            // above, which is what lets the stack's proposal win.
        ])
        // At REQUIRED, so "a card is the width of its pane" beats the card's
        // own intrinsic width. A card that is the width of its longest label is
        // a chip with a border, and a chip with a border in a settings pane
        // reads as a control rather than as a group of controls.
        card.setContentCompressionResistancePriority(.defaultLow, for: .horizontal)
        card.widthAnchor.constraint(greaterThanOrEqualToConstant: 320).isActive = true

        Task { await load(context: context, wanted: control.items, into: stack) }
    }

    @available(*, unavailable)
    required init?(coder: NSCoder) { fatalError("ChecklistView is created in code") }

    private func load(context: ControlContext, wanted: [String], into stack: NSStackView) async {
        guard let rows = try? await context.service.readiness() else {
            stack.addArrangedSubview(EmptyStateView(
                headline: "Could not read the readiness list.",
                detail: "The daemon did not answer core.readiness. Is it running? (./scripts/run.sh)"
            ))
            return
        }

        // `items` is the page's own subset and its own order. The daemon sends
        // every row; the page says which of them it wants and in what order,
        // and a shell that re-sorted them would be answering a different
        // question than the one asked.
        let shown = wanted.isEmpty ? rows : rows.filter { wanted.contains($0.id) }

        for row in shown {
            let chip = NSTextField(labelWithString: row.ready ? "Ready" : "Not ready")
            chip.font = Typeface.caption
            chip.font = Typeface.caption
            chip.textColor = row.ready ? Palette.ok : Palette.danger

            let label = NSTextField(labelWithString: row.label)
            label.font = Typeface.body
            label.textColor = Palette.primaryInk

            let header = NSStackView(views: [chip, label])
            header.orientation = .horizontal
            header.alignment = .firstBaseline
            header.spacing = Gap.row

            let column = NSStackView(views: [header])
            column.orientation = .vertical
            column.alignment = .leading
            column.spacing = Gap.tight

            if !row.detail.isEmpty {
                // The reason, on its own line. "Not ready" without a reason is
                // a red word; this is the sentence that says what to do.
                let detail = NSTextField(labelWithString: row.detail)
                detail.font = Typeface.caption
                detail.textColor = Palette.secondaryInk
                detail.lineBreakMode = .byWordWrapping
                detail.maximumNumberOfLines = 0
                detail.setContentCompressionResistancePriority(.defaultLow, for: .horizontal)
                column.addArrangedSubview(detail)
            }
            stack.addArrangedSubview(column)
        }
    }
}

// MARK: - wizard

/// The first-run wizard.
///
/// This is the screen that was 883.5px tall in a 720px window, with "Pick a
/// Windows profile" broken one character per line. The cause was CSS: the step
/// button had `flex: 1` — a basis of zero — so the chips beside it took the
/// space and the label was squeezed to a character, and then
/// `overflow-wrap: anywhere` finished the job by breaking the remaining two
/// words.
///
/// There is no flex here. There is no `overflow-wrap`. A step name is an
/// `NSTextField` with a line-break mode, in a stack, and it cannot be squeezed
/// below its intrinsic width by a chip sitting beside it. The bug has no
/// representation, which is the entire reason this port exists.
final class WizardView: NSView {
    private let currentStep = 0
    private let stack = NSStackView()

    init(control: Control, context: ControlContext) {
        super.init(frame: .zero)
        // Laid out by Auto Layout, not by its frame. Without this the
        // constraints below are inert and the view keeps whatever size it
        // measured for itself.
        translatesAutoresizingMaskIntoConstraints = false
        // A `.width`-aligned stack proposes its own width and every row takes
        // it — UNLESS the row has an intrinsic width and resists. A card
        // holding a stack of labels does, and it won: the row came out 337px in
        // an 850px pane and sat at x=493, right-aligned against a pane that was
        // 500px wider than it.
        //
        // Compression resistance is the right knob and not a width constraint:
        // a width constraint needs a reference that does not exist while this
        // initializer runs, and the reference this view WILL have is the stack
        // that adds it. Low resistance says "take the proposed width" in one
        // line, with no second source of truth to keep in step.
        setContentCompressionResistancePriority(.defaultLow, for: .horizontal)

        stack.orientation = .vertical
        stack.alignment = .leading
        stack.spacing = Gap.row
        stack.translatesAutoresizingMaskIntoConstraints = false

        let card = Card()
        let cardColumn = NSStackView(views: [stack])
        cardColumn.orientation = .vertical
        cardColumn.alignment = .leading
        cardColumn.spacing = Gap.row
        cardColumn.translatesAutoresizingMaskIntoConstraints = false
        addSubview(card)
        card.setContent(cardColumn)
        NSLayoutConstraint.activate([
            card.leadingAnchor.constraint(equalTo: leadingAnchor),
            card.trailingAnchor.constraint(equalTo: trailingAnchor),
            card.topAnchor.constraint(equalTo: topAnchor),
            // The card fills THIS view, and this view is sized by the stack
            // that holds it. The width is not restated here because restating
            // it needs a reference that does not exist yet at init time —
            // `superview` is nil while these constraints are activated, which
            // is a crash rather than a layout failure.
            //
            // So the width comes from the parent: `PageViewController` builds
            // its stack with `.width` alignment, and a row in a `.width`
            // stack is the stack's width. What that does NOT do is override a
            // row's own intrinsic width, and a card holding labels has one —
            // hence `setContentCompressionResistancePriority(.defaultLow)`
            // above, which is what lets the stack's proposal win.
        ])
        // At REQUIRED, so "a card is the width of its pane" beats the card's
        // own intrinsic width. A card that is the width of its longest label is
        // a chip with a border, and a chip with a border in a settings pane
        // reads as a control rather than as a group of controls.
        card.setContentCompressionResistancePriority(.defaultLow, for: .horizontal)
        card.widthAnchor.constraint(greaterThanOrEqualToConstant: 320).isActive = true

        build(control: control, context: context)
        Task { await loadVerdicts(context: context) }
    }

    @available(*, unavailable)
    required init?(coder: NSCoder) { fatalError("WizardView is created in code") }

    private func build(control: Control, context: ControlContext) {
        let steps = control.steps

        for (index, step) in steps.enumerated() {
            let number = NSTextField(labelWithString: "\(index + 1)")
            number.font = Typeface.caption
            number.textColor = Palette.tertiaryInk
            number.alignment = .center
            number.translatesAutoresizingMaskIntoConstraints = false

            // The step name. `.byTruncatingTail` rather than a break: a step
            // is a phrase, and a phrase that does not fit is shortened at its
            // end rather than re-flowed into a column. The React shell's
            // break-at-every-character was a consequence of `anywhere`
            // shrinking the box's min-content, not of wrapping being wrong.
            let title = NSTextField(labelWithString: step)
            title.font = index == currentStep ? Typeface.bodyStrong : Typeface.body
            title.textColor = index == currentStep ? Palette.primaryInk : Palette.secondaryInk
            title.lineBreakMode = .byTruncatingTail
            title.setContentCompressionResistancePriority(.defaultHigh, for: .horizontal)
            title.setContentHuggingPriority(.defaultLow, for: .horizontal)

            let header = NSStackView(views: [number, title])
            header.orientation = .horizontal
            header.alignment = .firstBaseline
            header.spacing = Gap.row

            // A placeholder for the daemon's verdict, filled in by
            // `loadVerdicts`. It is a view rather than a string because the
            // verdict arrives after the steps are drawn, and an
            // `NSTextField` whose string is set twice is a control that
            // reflows the stack twice.
            let verdict = NSTextField(labelWithString: "")
            verdict.font = Typeface.caption
            verdict.textColor = Palette.secondaryInk
            verdict.tag = index
            verdict.setContentCompressionResistancePriority(.defaultLow, for: .horizontal)

            let column = NSStackView(views: [header, verdict])
            column.orientation = .vertical
            column.alignment = .leading
            column.spacing = Gap.tight
            stack.addArrangedSubview(column)
        }
    }

    private func loadVerdicts(context: ControlContext) async {
        guard let rows = try? await context.service.readiness() else { return }
        let byID = Dictionary(uniqueKeysWithValues: rows.map { ($0.id, $0) })

        // Step N's verdict is the daemon's Nth readiness row, in order. The
        // React version printed a reason under EVERY unfinished step and then
        // printed the current step's reason AGAIN below the list, next to the
        // buttons that act on it — the same sentence twice on one screen. Here
        // each step carries its own verdict and nothing is printed twice.
        let ids = ["daemon", "keyboard", "windows", "finder"]
        for (index, column) in stack.arrangedSubviews.compactMap({ $0 as? NSStackView }).enumerated() {
            guard let verdict = column.views.compactMap({ $0 as? NSTextField }).last else { continue }
            guard index < ids.count else { continue }
            let row = byID[ids[index]]
            guard let row else { continue }
            verdict.stringValue = row.detail
            verdict.isHidden = row.detail.isEmpty
        }
    }
}

// MARK: - Small controls

/// A note: text and nothing else.
final class NoteView: NSView {
    init(control: Control) {
        super.init(frame: .zero)

        let label = NSTextField(labelWithString: control.label)
        label.font = Typeface.body
        label.textColor = Palette.primaryInk
        label.lineBreakMode = .byWordWrapping
        label.maximumNumberOfLines = 0

        let column = NSStackView(views: [label])
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
    required init?(coder: NSCoder) { fatalError("NoteView is created in code") }
}

/// A button row, wired to a capability id.
final class ButtonRowView: NSView {
    init(control: Control, context: ControlContext) {
        super.init(frame: .zero)

        // `NSButton` with a bezel style, because a button drawn by hand is a
        // button that does not get the system focus ring, the system press
        // animation, or the system disabled appearance.
        let button = NSButton(
            title: control.label.isEmpty ? control.id : control.label,
            target: ActionTarget.shared,
            action: #selector(ActionTarget.fire(_:))
        )
        button.bezelStyle = .rounded
        button.target = ActionTarget.shared
        ActionTarget.shared.register(button, control: control, context: context)

        let column = NSStackView(views: [button])
        column.orientation = .vertical
        column.alignment = .leading
        column.spacing = Gap.tight
        column.translatesAutoresizingMaskIntoConstraints = false

        if !control.note.isEmpty {
            let note = NSTextField(labelWithString: control.note)
            note.font = Typeface.caption
            note.textColor = Palette.secondaryInk
            note.lineBreakMode = .byWordWrapping
            note.maximumNumberOfLines = 0
            column.addArrangedSubview(note)
        }

        addSubview(column)
        NSLayoutConstraint.activate([
            column.leadingAnchor.constraint(equalTo: leadingAnchor),
            column.trailingAnchor.constraint(lessThanOrEqualTo: trailingAnchor),
            column.topAnchor.constraint(equalTo: topAnchor),
        ])
    }

    @available(*, unavailable)
    required init?(coder: NSCoder) { finalError() }
}

private func finalError() -> Never { fatalError("ButtonRowView is created in code") }

/// The button target, holding the control each button stands for.
///
/// A shared singleton because `NSButton.target` is a weak reference and a
/// closure cannot be one. The registry it is standing in for is
/// `actions.ts:206-302`, which maps a capability id to a call and **fails
/// closed in words** for an id nothing handles — a button that says which
/// capability it could not run rather than one that does nothing.
@MainActor
final class ActionTarget: NSObject {
    static let shared = ActionTarget()
    private struct Binding { let control: Control; let context: ControlContext }
    private var bindings: [ObjectIdentifier: Binding] = [:]

    func register(_ button: NSButton, control: Control, context: ControlContext) {
        bindings[ObjectIdentifier(button)] = Binding(control: control, context: context)
    }

    @objc func fire(_ sender: NSButton) {
        guard let binding = bindings[ObjectIdentifier(sender)] else { return }
        let capability = binding.control.action
        guard !capability.isEmpty else {
            binding.context.note("This button declares no action.")
            return
        }
        // The dispatch table lands with the pages that call it. Failing closed
        // here — naming the capability rather than silently doing nothing — is
        // the behaviour `actions.ts:402-409` specifies and the reason this
        // stub is a stub rather than a guess.
        binding.context.note("“\(capability)” is not wired yet in the AppKit shell.")
    }
}
