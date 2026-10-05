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
final class HomeSummaryView: NSStackView {
    private let client: any CoreClient
    private let rows = NSStackView()
    /// The section: cap, group, footnote. The footnote is where the daemon's
    /// tap warning goes — below the group, not as a fourth row inside it.
    private var section: SectionView?

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

        // The rows are a GROUP with a cap above it, not a titled card. See
        // `SectionView` for why the cap is outside the box: inside, every
        // control renders as its own titled card and the page reads as a
        // stack of web panels rather than as one settings window.
        // The rows stack holds the FactRows directly; the group's insets pad
        // the group as a whole. No spacing between rows: the group draws a
        // hairline between them, which is the macOS arrangement. A gap AND a
        // hairline would put two separators between every pair of rows.
        rows.orientation = .vertical
        rows.alignment = .leading
        rows.distribution = .fill
        rows.spacing = 0
        rows.translatesAutoresizingMaskIntoConstraints = false
        rows.setContentHuggingPriority(.required, for: .vertical)

        let section = SectionView(title: "Status")
        self.section = section
        addArrangedSubview(section)
        let column = NSStackView(views: [rows])
        column.alignment = .leading
        column.distribution = .fill
        section.group.setContent(column)
        // `rows` is the group's content rect, and this is what says so. A
        // `.leading`-aligned stack gives each row the row's own width, so
        // without this the three locks sat at three different x positions in
        // three rows that read as one column.
        rows.widthAnchor.constraint(equalTo: column.widthAnchor).isActive = true
        NSLayoutConstraint.activate([
            section.leadingAnchor.constraint(equalTo: leadingAnchor),
            section.trailingAnchor.constraint(equalTo: trailingAnchor),
            section.topAnchor.constraint(equalTo: topAnchor),
        ])

        // Hug the group VERTICALLY, which is the width constraint's other
        // half. Without it this view measures zero, the group is laid out
        // below its own control — outside the clip view — and the page
        // renders with a hole where the rows go.
        setContentHuggingPriority(.required, for: .vertical)
        // Low compression resistance so "the group is the width of its pane"
        // beats the group's own intrinsic width.
        section.group.setContentCompressionResistancePriority(.defaultLow, for: .horizontal)

        Task { await reload() }
    }

    /// Put a row in the column, at the column's full width.
    ///
    /// The width is STATED rather than negotiated. A `.leading`-aligned stack
    /// gives each row the row's own intrinsic width, and a `.width`-aligned
    /// one proposes its width without enforcing it — measured, rows came out
    /// 314pt and 451pt, centred at x=456 in a 770pt content rect, so the lock
    /// floated mid-card instead of on the card's trailing edge.
    private func addRow(_ row: FactRow) {
        rows.addArrangedSubview(row)
        row.widthAnchor.constraint(equalTo: rows.widthAnchor).isActive = true
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
        let profile = activeProfileName(from: pluginRows)

        addRow(FactRow(
            key: "Daemon",
            badge: running ? .healthy("Running") : .unhealthy("Not running"),
            value: "CrossOS \(version)",
            monospaced: true
        ))
        addRow(FactRow(
            key: "Keyboard",
            badge: interception ? .healthy("On") : .unhealthy("Off"),
            value: interception
                ? "CrossOS can read the keyboard and act on it."
                : "CrossOS cannot see the keyboard yet."
        ))
        addRow(FactRow(
            key: "Profile",
            badge: .neutral(profile ?? "None"),
            value: profile ?? "No profile applied"
        ))

        // The daemon's own words, in its own order. The React shell showed a
        // permission error as a blank panel because there was no channel for
        // the message; the string is the whole value of a tap that is not on.
        //
        // "All checks are ready" is shown ONLY when there is something to say
        // — a green footnote under a group of three green facts is noise, and
        // it is noise on the most-read page in the app.
        // The daemon's own words, in its own order. The React shell showed a
        // permission error as a blank panel because there was no channel for
        // the message; the string is the whole value of a tap that is not on.
        //
        // It goes in the FOOTNOTE, below the group. As a fourth row inside the
        // box it read as a fourth fact, which is not what it is — it explains
        // the "Off" above it.
        //
        // And it is shown ONLY when there is something to say: a green
        // "All checks are ready" under a group of three green facts is noise,
        // on the most-read page in the app.
        //
        // And it is shown ONLY when the sentence is not ALREADY on the page.
        // Measured on `core.home`: "interception stopped by PANIC STOP
        // (re-enable from the Safety page)" appeared TWICE — once as this
        // footnote and again as the detail line under "Keyboard interception"
        // in the Readiness group. Two copies of one sentence on one screen
        // reads as two problems and is one.
        //
        // The Readiness rows own the sentences they are about. This footnote is
        // for a tap error that no readiness row carries.
        let namedInReadiness = (try? await client.readiness())?
            .contains { $0.detail == statusValue?.tapError } ?? false
        if let error = statusValue?.tapError, !error.isEmpty, !interception, !namedInReadiness {
            section?.footnote.stringValue = "Keyboard interception is off: \(error)"
            // SECONDARY ink, not `warn`.
            //
            // Orange in the middle of the page was the loudest thing on it —
            // louder than the page title — for a sentence that repeats what the
            // Readiness group below already says in two red rows. The urgency
            // belongs where the reader is looking for it, and a footnote is
            // where the reader is NOT looking.
            //
            // The colour is not lost: the Readiness group's rows carry "Not
            // ready" in red, so the page still says what is wrong without the
            // footnote competing with the title for the first glance.
            section?.footnote.textColor = Palette.secondaryInk
            section?.footnote.isHidden = false
        } else {
            section?.footnote.isHidden = true
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
final class FactRow: NSStackView {
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

        // The row fills whatever width it is given, and says so.
        //
        // A row in a `.width`-aligned stack is meant to take the stack's
        // width, but this one kept its 314pt intrinsic width and was centred
        // in the 770pt content rect at x=456 — so the lock floated in the
        // middle of the card instead of on its trailing edge. Low compression
        // resistance is what "take the width you are offered" means to Auto
        // Layout, and the stack's alignment is what offers it.
        setContentCompressionResistancePriority(.defaultLow, for: .horizontal)

        let keyField = NSTextField(labelWithString: key)
        keyField.font = Typeface.body
        keyField.textColor = Palette.secondaryInk
        // LEFT aligned, not right. The key column is a fixed width so the
        // values line up, and right-aligning inside it pushed "Profile" to
        // x=192 while "Daemon" sat at x=126 — three keys, three left edges,
        // in a column whose entire purpose is one shared left edge.
        keyField.alignment = .left
        // A FIXED key column, not `greaterThanOrEqualToConstant`. A minimum
        // lets each row's key claim whatever width its own badge and value
        // need, so the values started at three different x positions across
        // the three rows above.
        keyField.widthAnchor.constraint(equalToConstant: 110).isActive = true

        let badgeField = NSTextField(labelWithString: badge.text)
        badgeField.font = Typeface.caption
        badgeField.textColor = badge.color
        badgeField.alignment = .left
        // The badge is a COLUMN too, for the same reason: "Running" beside
        // "Off" beside "None" put the values at three more x positions.
        badgeField.widthAnchor.constraint(equalToConstant: 70).isActive = true

        let valueField = NSTextField(labelWithString: value)
        valueField.font = monospaced ? Typeface.mono : Typeface.body
        valueField.textColor = Palette.primaryInk
        valueField.lineBreakMode = .byTruncatingTail

        // The lock, at the row's trailing edge.
        //
        // A glyph rather than a padlock emoji — the React shell used U+1F512
        // and Chromium painted it as a yellow padlock, the only saturated
        // non-accent colour in a window of three greys and one blue. SF
        // Symbols has a lock and it takes the text colour.
        let lock = NSImageView()
        if let image = NSImage(systemSymbolName: "lock.fill", accessibilityDescription: "Fixed") {
            lock.image = image
        }
        lock.contentTintColor = Palette.tertiaryInk
        lock.translatesAutoresizingMaskIntoConstraints = false
        lock.setContentHuggingPriority(.required, for: .horizontal)
        // Both axes: without a height the image view asked for 0 and the row
        // gave it 0, so nothing drew.
        lock.setContentHuggingPriority(.required, for: .vertical)
        lock.setContentCompressionResistancePriority(.required, for: .horizontal)

        for field in [keyField, badgeField, valueField] as [NSView] {
            field.translatesAutoresizingMaskIntoConstraints = false
            addSubview(field)
        }
        addSubview(lock)

        NSLayoutConstraint.activate([
            // The row's HEIGHT is the tallest thing in it. `centerY` on a row
            // with no height of its own is `centerY` on nothing: the row
            // measured 314x0, every field landed at y=-7 (half of zero,
            // negated) and all of it fell outside the row's own frame.
            //
            // 32pt, not the 18pt this declared before. A settings row is a
            // target, and three 18pt rows separated by hairlines read as a
            // list of chips rather than as the contents of one group.
            // `greaterThanOrEqual` so a value that wraps grows the row rather
            // than being clipped by it.
            heightAnchor.constraint(greaterThanOrEqualToConstant: Measure.rowHeight),

            keyField.leadingAnchor.constraint(equalTo: leadingAnchor),
            keyField.centerYAnchor.constraint(equalTo: centerYAnchor),
            keyField.widthAnchor.constraint(equalToConstant: 110),

            badgeField.leadingAnchor.constraint(equalTo: keyField.trailingAnchor, constant: Gap.close),
            badgeField.centerYAnchor.constraint(equalTo: centerYAnchor),
            badgeField.widthAnchor.constraint(equalToConstant: 70),

            // The value takes the rest of the row, so the lock sits at the
            // trailing edge rather than floating wherever the value ended.
            // Before this the value was `lessThanOrEqualTo` the lock with
            // nothing pulling it there, so the lock drifted left into the
            // middle of the card — measured at x=426 in a 438pt row, hanging
            // in the whitespace between "CrossOS v0.1.0" and the card edge.
            valueField.leadingAnchor.constraint(equalTo: badgeField.trailingAnchor, constant: Gap.row),
            valueField.centerYAnchor.constraint(equalTo: centerYAnchor),
            valueField.trailingAnchor.constraint(equalTo: lock.leadingAnchor, constant: -Gap.row),

            lock.trailingAnchor.constraint(equalTo: trailingAnchor),
            lock.centerYAnchor.constraint(equalTo: centerYAnchor),
            lock.widthAnchor.constraint(equalToConstant: 12),
            lock.heightAnchor.constraint(equalToConstant: 14),
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
final class ChecklistView: NSStackView {
    init(control: Control, context: ControlContext) {
        super.init(frame: .zero)
        // A stack in name only is a stack that measures nothing: without an
        // orientation and a `.width` alignment this view is a vertical column
        // that takes each row's height from its first child, and the whole
        // chain runs down to a label that reports the height of nothing.
        orientation = .vertical
        alignment = .width
        distribution = .fill
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
        // `.leading`, so each row sits at the LEFT of a group that fills the
        // page. `.width` would stretch each row and distribute them, which is
        // what put every line at the trailing edge: a group that fills and is
        // unreadable.
        stack.alignment = .leading
        stack.distribution = .fill
        // No spacing: the group draws a hairline between its rows, which is
        // what macOS does. A gap AND a hairline is two separators.
        stack.spacing = 0
        stack.translatesAutoresizingMaskIntoConstraints = false

        // A cap above the group and a footnote below it, per `SectionView`.
        let section = SectionView(title: "Readiness", note: control.note)
        addArrangedSubview(section)
        let column = NSStackView(views: [stack])
        column.alignment = .leading
        column.distribution = .fill
        section.group.setContent(column)
        stack.widthAnchor.constraint(equalTo: column.widthAnchor).isActive = true
        NSLayoutConstraint.activate([
            section.leadingAnchor.constraint(equalTo: leadingAnchor),
            section.trailingAnchor.constraint(equalTo: trailingAnchor),
            section.topAnchor.constraint(equalTo: topAnchor),
        ])

        // Hug the group VERTICALLY. A `.leading`-aligned vertical stack gives
        // a row the height it measured, and this view measured zero: nothing
        // tied its height to the group it holds. The group was then laid out
        // below its own control — outside the clip view — so the page
        // rendered with a hole exactly where the rows go.
        setContentHuggingPriority(.required, for: .vertical)
        // Low compression resistance so "the group is the width of its pane"
        // beats the group's own intrinsic width.
        section.group.setContentCompressionResistancePriority(.defaultLow, for: .horizontal)

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
            chip.textColor = row.ready ? Palette.okInk : Palette.dangerInk

            let label = NSTextField(labelWithString: row.label)
            label.font = Typeface.body
            label.textColor = Palette.primaryInk

            // The chip is a fixed-width column so every label in the group
            // starts at the same x. Without it "Ready" and "Not ready" put
            // their labels at two different positions in one list.
            let header = NSStackView(views: [chip, label])
            header.orientation = .horizontal
            header.alignment = .firstBaseline
            header.spacing = Gap.row
            chip.widthAnchor.constraint(equalToConstant: 70).isActive = true

            let column = NSStackView(views: [header])
            column.orientation = .vertical
            column.alignment = .leading
            column.spacing = Gap.tight
            column.translatesAutoresizingMaskIntoConstraints = false

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

            // Rows breathe. `stack.spacing = 0` because the group draws the
            // hairline between rows, but a row still needs its own padding or
            // the text sits ON the separator above it.
            //
            // The wrapper's constraints reference `padded` itself rather than
            // the enclosing stack: a constraint needs a common ancestor at the
            // moment it activates, and `padded` has no superview until
            // `addArrangedSubview` — so pinning to a stack the row is not yet
            // in throws `NSGenericException: ... no common ancestor`.
            let padded = NSView()
            padded.translatesAutoresizingMaskIntoConstraints = false
            padded.addSubview(column)
            stack.addArrangedSubview(padded)
            NSLayoutConstraint.activate([
                column.topAnchor.constraint(equalTo: padded.topAnchor, constant: Gap.close),
                column.bottomAnchor.constraint(equalTo: padded.bottomAnchor, constant: -Gap.close),
                column.leadingAnchor.constraint(equalTo: padded.leadingAnchor),
                column.trailingAnchor.constraint(equalTo: padded.trailingAnchor),
                padded.widthAnchor.constraint(equalTo: stack.widthAnchor),
            ])
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
final class WizardView: NSStackView {
    private let currentStep = 0
    private let stack = NSStackView()
    init(control: Control, context: ControlContext) {
        super.init(frame: .zero)
        // A stack in name only is a stack that measures nothing: without an
        // orientation and a `.width` alignment this view is a vertical column
        // that takes each row's height from its first child, and the whole
        // chain runs down to a label that reports the height of nothing.
        orientation = .vertical
        alignment = .width
        distribution = .fill
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
        // `.leading` so the rows sit at the LEFT of a card that fills 810pt.
        // `.width` would stretch each row and distribute them, which is what
        // put every line at the trailing edge of the card: a card that fills
        // and is unreadable.
        stack.alignment = .leading
        stack.distribution = .fill
        stack.translatesAutoresizingMaskIntoConstraints = false

        // No wrapper between the view and the card. The wrapper was a
        // `.leading`-aligned vertical stack holding another one, and
        // `.leading` takes the height of the FIRST child — so the chain ran
        // down to a leaf that reports the height of nothing, the card
        // measured zero, and the content was laid out at y=-162, below its
        // own view and outside the clip view. Every page rendered with a
        // title, a description and a hole exactly where the cards go.
        //
        // `card` is a `.width`-aligned stack: it takes its height from the
        // sum of its rows, which is what a vertical stack is for.
        let card = Card()
        addArrangedSubview(card)
        card.setContent(stack)
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

        // Hug the card VERTICALLY, which is the width constraint's other half
        // and was missing.
        //
        // A `.leading`-aligned vertical stack gives a row the height it
        // measured, and this view measured zero: nothing tied its height to
        // the card it holds. The card was then laid out below its own control
        // — outside the clip view, below the page's own origin — so it was
        // neither on screen nor in a render of the page. Every page came out
        // with its title, its description and a hole exactly where the cards
        // go, and the audit could not see it because the card DOES have a
        // height; the row it hangs in does not.
        setContentHuggingPriority(.defaultLow, for: .vertical)
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

    /// The link buttons' target needs the page it navigates to, and that
    /// lives on the context the view was built with. A closure rather than the
    /// whole context, so this type holds no service it would never call.
    private var navigate: (String) -> Void = { _ in }

    private func build(control: Control, context: ControlContext) {
        navigate = { id in context.navigate(id) }
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

            // An EMPTY text field is not a neutral placeholder: an
            // `NSTextField` with no string still has an intrinsic width of a
            // few points, so before the verdict lands this was a 4pt box that
            // reserved a row's worth of height and drew nothing. Hidden from
            // the start and revealed when there is a reason to read it, which
            // is also what `loadVerdicts` expects to find.
            verdict.isHidden = true

            let column = NSStackView(views: [header, verdict])
            column.orientation = .vertical
            column.alignment = .leading
            column.spacing = Gap.tight
            stack.addArrangedSubview(column)
        }

        // The links go LAST, after the steps. They were built first and the
        // wizard read as two buttons floating above four numbered steps, with
        // no way to tell they were the end of the list rather than the
        // beginning of it.
        buildLinks(on: stack, from: control)
    }

    /// The pages the wizard says it links to, as buttons.
    ///
    /// **The daemon has declared these since the page was written and the shell
    /// has never read them.** `core.onboard` sends `aboutLink: core.about` and
    /// `trialLink: core.safety`; `Control.link(_:)` has existed the whole time
    /// to read them and nothing called it. So the welcome wizard — the first
    /// thing anybody sees — told a new user to enable things and gave them no
    /// way to reach the page that explains them or the page where the
    /// dangerous button lives.
    ///
    /// Same class as `core.finder`, which promised a list and showed a
    /// sentence: something is declared and nothing draws it.
    private func buildLinks(on column: NSStackView, from control: Control) {
        let links: [(String, String, String)] = [
            ("trialLink", "Open Safety", "Where panic stop and Reset Everything live."),
            ("aboutLink", "About CrossOS", "What it does, and what it asks for."),
        ]
        var made: [NSButton] = []
        for (key, title, help) in links {
            guard let pageID = control.link(key), !pageID.isEmpty else { continue }
            let button = NSButton(title: title, target: self, action: #selector(followLink(_:)))
            button.bezelStyle = .rounded
            button.tag = 0
            button.setAccessibilityLabel("\(title) — \(help)")
            button.identifier = NSUserInterfaceItemIdentifier(pageID)
            column.addArrangedSubview(button)
            made.append(button)
        }
        // The first link is separated from the step above it, the two from
        // each other by the stack's own spacing.
        if made.count == column.arrangedSubviews.count - 1, let lastStep = column.arrangedSubviews.dropLast(made.count).last {
            column.setCustomSpacing(Gap.group, after: lastStep)
        }
        guard made.count > 1 else { return }
        for button in made {
            button.widthAnchor.constraint(equalTo: made[0].widthAnchor).isActive = true
        }
    }

    @objc private func followLink(_ sender: NSButton) {
        guard let id = sender.identifier?.rawValue else { return }
        navigate(id)
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
final class NoteView: NSStackView {
    init(control: Control) {
        super.init(frame: .zero)
        // A stack, so it takes its height from the column it holds. As an
        // NSView it measured 16pt and the text inside it measured 4pt, which
        // reads as an empty label and is a collapsed one.
        orientation = .vertical
        alignment = .leading
        distribution = .fill
        translatesAutoresizingMaskIntoConstraints = false

        let label = NSTextField(labelWithString: control.label)
        // Caption size in the secondary ink, not body size in primary ink.
        //
        // A note is prose ABOUT a page, and it was being drawn at the same
        // size and weight as the page title and its description — so on
        // `core.finder` the page read "Finder" / "What the Finder extension
        // adds." / "The Finder Sync extension is what draws CrossOS's entries
        // in Finder." as three headings of nearly equal weight, the last of
        // them at the left margin with nothing around it.
        //
        // Secondary ink at caption size is what macOS uses for exactly this:
        // text that elaborates rather than announces.
        label.font = Typeface.caption
        label.textColor = Palette.secondaryInk
        label.alignment = .left
        label.lineBreakMode = .byWordWrapping
        label.usesSingleLineMode = false
        label.maximumNumberOfLines = 0
        label.setContentCompressionResistancePriority(.defaultLow, for: .horizontal)

        addArrangedSubview(label)
        setContentHuggingPriority(.required, for: .vertical)
    }

    @available(*, unavailable)
    required init?(coder: NSCoder) { fatalError("NoteView is created in code") }
}

/// A button row, wired to a capability id.
final class ButtonRowView: NSStackView {
    init(control: Control, context: ControlContext) {
        super.init(frame: .zero)
        orientation = .vertical
        alignment = .leading
        distribution = .fill
        translatesAutoresizingMaskIntoConstraints = false

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
        // Hug horizontally so the button is its own width rather than the
        // row's. Measured without it: a button in a row with no note came out
        // 520pt — the full pane — with its label centred in it, which is a
        // banner rather than a button.
        //
        // Compression resistance is `.defaultLow`, NOT `.required`. A
        // `.rounded` button resists being squeezed, and required resistance
        // outranks the width cap below: measured, the cap was active and the
        // button was still 339pt, having starved the sentence beside it to
        // 245pt and clipped its second line. A button that shrinks and
        // truncates its label is better than one that starves its own
        // description.
        //
        // The height is the system's, taken rather than declared: a `.rounded`
        // bezel on a 14pt label measures 24pt, and pinning it to a number here
        // is a number this file has to keep correct across macOS releases.
        button.setContentHuggingPriority(.required, for: .horizontal)
        button.setContentCompressionResistancePriority(.defaultLow, for: .horizontal)
        ActionTarget.shared.register(button, control: control, context: context)

        // The row is the button and the sentence beside it, side by side, and
        // it is CONSTRAINED rather than stacked.
        //
        // A horizontal `NSStackView` with a `.fill` distribution stretches its
        // only subview to the row's width: measured, the Reset button — the
        // one row with no note — came out 520pt, the full pane, with its
        // label centred in it, which is a banner rather than a button. A stack
        // also cannot say "the button is its own width and the note takes what
        // is left of the row".
        //
        // So the two are pinned directly: the button at the leading edge at
        // its own width, the note filling the rest on the button's first
        // baseline. Three of these share one leading edge and one trailing
        // edge, which is what makes them read as rows rather than as three
        // loose widgets.
        addSubview(button)
        NSLayoutConstraint.activate([
            button.leadingAnchor.constraint(equalTo: leadingAnchor),
            button.centerYAnchor.constraint(equalTo: centerYAnchor),
        ])

        // The button's label is `control.label`, and one of them is a sentence
        // ("PANIC STOP — disable interception + plugin actions"). Hugged at its
        // full width that button took 339pt of a 600pt row and left the note
        // 245pt — enough to wrap and not enough to show the result, so the
        // second line was clipped ("Reversible via Re-…").
        //
        // A button on macOS truncates; the sentence beside it does not. The cap
        // is what makes that the outcome instead of the reverse.
        button.widthAnchor.constraint(lessThanOrEqualToConstant: 260).isActive = true

        if !control.note.isEmpty {
            let note = NSTextField(labelWithString: control.note)
            note.font = Typeface.body
            note.textColor = Palette.secondaryInk
            note.alignment = .left
            note.lineBreakMode = .byWordWrapping
            // `labelWithString:` sets `usesSingleLineMode`, and with it set a
            // label IGNORES `byWordWrapping` and truncates at the frame's edge
            // instead. That is why this sentence rendered as "Login item stays.
            // Reversible via Re-…" at 245pt wide, with the rest of the second
            // line clipped away rather than wrapped onto it.
            note.usesSingleLineMode = false
            note.maximumNumberOfLines = 0
            note.translatesAutoresizingMaskIntoConstraints = false
            // The note FILLS the gap between the button and the row's end.
            //
            // Leading and trailing alone do not say how WIDE it is, so a label
            // keeps its intrinsic width and Auto Layout puts it as far from the
            // button as it can — measured on `core.safety`, "Login item stays.
            // Reversible via Re-enable" starting at x=1170 against a button
            // ending at x=815, running off the right edge of the pane.
            note.setContentHuggingPriority(.defaultLow, for: .horizontal)
            note.setContentCompressionResistancePriority(.defaultLow, for: .horizontal)
            addSubview(note)
            NSLayoutConstraint.activate([
                note.leadingAnchor.constraint(equalTo: button.trailingAnchor, constant: Gap.group),
                note.trailingAnchor.constraint(lessThanOrEqualTo: trailingAnchor),
                note.firstBaselineAnchor.constraint(equalTo: button.firstBaselineAnchor),
                note.topAnchor.constraint(greaterThanOrEqualTo: topAnchor),
                note.bottomAnchor.constraint(lessThanOrEqualTo: bottomAnchor),
            ])
        }

        // Hug vertically, with a floor at the standard row height so a short
        // button is still a comfortable target.
        //
        // Hugging alone is not enough and the height floor is not either: the
        // children are pinned by `centerY` and `firstBaseline`, neither of
        // which says how tall the row is, so a two-line note measured 32pt
        // inside a 24pt row and ran from y=-12 to y=20 — half of it ABOVE the
        // row's own origin. The note's own top and bottom are what close that.
        setContentHuggingPriority(.required, for: .vertical)
        heightAnchor.constraint(greaterThanOrEqualToConstant: Measure.rowHeight).isActive = true
    }

    @available(*, unavailable)
    required init?(coder: NSCoder) { fatalError("ButtonRowView is created in code") }
}

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

    /// The dispatch table, and it is a TABLE rather than a guess.
    ///
    /// It used to be empty, so every button in the app printed "“X” is not
    /// wired yet in the AppKit shell." — which is honest, and also means the
    /// Safety page's three buttons, the app's most consequential controls,
    /// did nothing at all. A stub that reports itself is not a feature.
    ///
    /// The three capabilities the daemon serves are `safety.panicStop`,
    /// `safety.resume` and `safety.reset`. The first two are wired here and
    /// were verified against a live daemon:
    ///
    ///     safety.panicStop → {"buffersFlushed":true,"interceptionDisabled":true,…}
    ///     safety.resume   → {"interception":false,"resumed":true}
    ///
    /// **`safety.reset` was absent for a long time and is now real.** The page
    /// declared a Reset Everything button with `"action": "safety.reset"` and
    /// the daemon answered `no such method: safety.reset` — the app's most
    /// consequential control was a button that did nothing. The daemon now
    /// implements it, and it reports each step rather than a bare success.
    @objc func fire(_ sender: NSButton) {
        guard let binding = bindings[ObjectIdentifier(sender)] else { return }
        let capability = binding.control.action
        guard !capability.isEmpty else {
            binding.context.note("This button declares no action.")
            return
        }

        // A destructive control asks FIRST.
        //
        // `core.safety`'s Reset Everything sends
        // `confirm: "Remove login item, disable extension, clean CrossOS-owned
        // state, verify no process remains?"` and the shell never read it — so
        // the button that removes the login item, disables the extension and
        // deletes CrossOS's own state was ONE CLICK, in an app whose first
        // page is a Safety page. That is the one defect in this whole history
        // that can cost somebody their setup.
        //
        // The question is the DAEMON's sentence, not one written here: the
        // daemon knows what the step removes, and a paraphrase is one more
        // place for the two to disagree about it.
        if let confirm = binding.control.confirm, !confirm.isEmpty {
            askThenRun(confirm, capability: capability, binding: binding)
            return
        }

        dispatch(capability, binding)
    }

    /// Run a capability. Split out of `fire(_:)` so the confirmation path and
    /// the direct path cannot drift into being two dispatch tables.
    private func dispatch(_ capability: String, _ binding: Binding) {
        switch capability {
        case "safety.panicStop":
            run(binding, failure: "Panic stop failed.") {
                try await binding.context.service.panicStop()
            }
        case "safety.resume":
            run(binding, failure: "Could not re-enable interception.") {
                try await binding.context.service.resume()
            }
        case "safety.reset":
            // Reset Everything reports rather than announces: the note is the
            // daemon's own step list, because a destructive control that says
            // "done" without saying what it did is the failure this whole
            // button exists to prevent.
            Task { @MainActor in
                do {
                    let report = try await binding.context.service.reset()
                    binding.context.note(report.summary)
                } catch {
                    binding.context.note("Reset Everything failed: \(error)")
                }
            }
        default:
            // Failing closed, naming the capability — the behaviour
            // `actions.ts:402-409` specifies.
            binding.context.note("“\(capability)” is not wired yet in the AppKit shell.")
        }
    }

    /// Put the question in front of the person, and run the capability on yes.
    ///
    /// An `NSAlert` with the daemon's own wording, a Cancel that is the
    /// default, and the destructive action named plainly rather than as
    /// "OK". A sheet would suit a document better than a settings window, so
    /// the alert is used rather than a bespoke overlay drawn from scratch.
    private func askThenRun(
        _ confirm: String,
        capability: String,
        binding: Binding
    ) {
        let alert = NSAlert()
        alert.messageText = Humanize.phrase(capability)
        alert.informativeText = confirm
        alert.addButton(withTitle: capability == "safety.reset" ? "Reset Everything" : "Run")
        alert.addButton(withTitle: "Cancel")
        // Cancel is the default: a destructive action must be pressed twice on
        // purpose, never taken by a stray Return.
        alert.buttons.last?.keyEquivalent = "\u{1b}"
        guard alert.runModal() == .alertFirstButtonReturn else {
            binding.context.note("\(Humanize.phrase(capability)) was not run.")
            return
        }
        dispatch(capability, binding)
    }

    /// Run a capability and say plainly what happened.
    ///
    /// The note is the ONLY thing a person sees after a destructive button is
    /// pressed, so it names the outcome rather than the attempt: "Panic stop is
    /// on" is a claim about the machine, and "Panic stop ran" is not. This is
    /// the stored-state rule `MatrixControl.tsx:34-41` states for the toggles
    /// and it is the same rule for a button.
    private func run(
        _ binding: Binding,
        failure: String,
        _ body: @escaping @MainActor () async throws -> JSONValue
    ) {
        Task { @MainActor in
            do {
                _ = try await body()
                binding.context.note(success(for: binding.control))
            } catch {
                binding.context.note("\(failure) \(error)")
            }
        }
    }

    /// What the machine now is, in words.
    private func success(for control: Control) -> String {
        switch control.action {
        case "safety.panicStop":
            return "Panic stop is on: interception, plugin actions and the login item are off."
        case "safety.resume":
            return "Re-enable requested. Check the Home page for the current state."
        default:
            return "\(control.label) ran."
        }
    }
}
