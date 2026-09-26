import AppKit
import CrossOSCore

/// The kind registry, and the views behind it.
///
/// The registry is a transcription of `app/frontend/src/controls/index.tsx`
/// and it keeps the three properties that file's comments say are load-bearing:
///
/// 1. **Keys are kinds, never page ids.** A plugin shipping a page gets
///    working UI with no change here; §3.6c's acceptance test is literally
///    "delete a plugin from the source tree entirely → the shell UI still runs"
///    (index.tsx:57-60). A registry keyed on anything else turns the next page
///    into a code change.
/// 2. **A dictionary, not a switch.** The React one is a `Map` for a reason
///    (index.tsx:65-68): a plain object lookup answers "constructor" with
///    something inherited, and the renderer then tries to draw it.
/// 3. **An unknown kind is named, not skipped** (index.tsx:143-149). A hole in
///    a page with no explanation is worse than a row that says which kind it
///    could not draw.
///
/// The three aliases are the retired spellings. They are kept OUT of the main
/// table rather than added as more entries, because a key nothing declares is
/// a mistake the registry cannot express, and the Go coverage guard
/// (`TestEveryRegisteredKindIsReachable`) has to tell a real kind from an
/// alias — a guard that cannot is a guard that gets switched off, and a gate
/// that gets switched off is not a gate.

/// The registry, as a value.
///
/// It is a `struct` with a `let` table rather than a mutable singleton,
/// because it never changes at runtime: the kinds are what the shell can draw,
/// and a page arriving with a kind not in here gets the unsupported row rather
/// than a new entry. Swift 6 asks the question a mutable global would have to
/// answer — what stops two threads writing this — and the honest answer is
/// nothing needs to, so it is a value and there is nothing to guard.
struct Renderers: Sendable {
    static let shared = Renderers()

    private var table: [String: @MainActor (Control, ControlContext) -> NSView] = [:]
    private var aliases: [String: @MainActor (Control, ControlContext) -> NSView] = [:]

    init() {
        register(.homeSummary) { HomeSummaryView(control: $0, context: $1) }
        register(.checklist) { ChecklistView(control: $0, context: $1) }
        register(.wizard) { WizardView(control: $0, context: $1) }
        register(.note) { control, _ in NoteView(control: control) }
        register(.version) { control, _ in NoteView(control: control) }
        register(.button) { control, context in ButtonRowView(control: control, context: context) }

        // Projections of daemon data. Each is a card with rows in it, and the
        // rows are a table rather than a stack — see TableView.swift for what
        // a table gives that a stack does not.
        register(.matrix) { control, context in MatrixView(control: control, context: context) }
        register(.overrides) { control, context in OverridesView(control: control, context: context) }
        register(.conflictResolver) { control, context in ConflictResolverView(control: control, context: context) }
        register(.observeToggle) { control, context in ObserveToggleView(control: control, context: context) }
        register(.auditList) { control, context in AuditListView(control: control, context: context) }
        register(.profileList) { control, context in ProfileListView(control: control, context: context) }
        register(.fileTypeList) { control, context in FileTypeListView(control: control, context: context) }
        register(.pipelineTrace) { control, context in PipelineTraceView(control: control, context: context) }
        register(.pluginList) { control, context in PluginListView(control: control, context: context) }
        register(.trial) { control, context in TrialView(control: control, context: context) }
        register(.credits) { control, context in CreditsView(control: control, context: context) }
        register(.license) { control, context in LicenseView(control: control, context: context) }
        register(.palette) { control, context in PaletteView(control: control, context: context) }
        register(.shortcutList) { control, context in ShortcutListView(control: control, context: context) }
        register(.zoneEditor) { control, context in ZoneEditorView(control: control, context: context) }
        register(.finderMenu) { control, context in FinderMenuView(control: control, context: context) }
        register(.pluginDetail) { control, context in PluginDetailView(control: control, context: context) }
        register(.keymapEditor) { control, context in KeymapEditorView(control: control, context: context) }
        register(.ruleBuilder) { control, context in RuleBuilderView(control: control, context: context) }
        register(.schemaForm) { control, context in SchemaFormView(control: control, context: context) }
        // The switcher panel is the settings page's HALF of the switcher: it
        // draws the same tiles the overlay does, from the same daemon list, in
        // a column rather than a grid. The overlay itself is a second window
        // and is summoned by the chord, not by this page.
        register(.switcherPanel) { control, context in SwitcherPageView(control: control, context: context) }

        // The three retired spellings, mapping to what replaced them.
        // `app/backend` re-points them by renaming the `kind` field; these are
        // the grace period, not a second implementation.
        alias("enableFlow", to: .wizard)
        alias("statusCard", to: .homeSummary)
        alias("traceList", to: .note)
    }

    private mutating func register(
        _ kind: Kind,
        _ renderer: @escaping @MainActor (Control, ControlContext) -> NSView
    ) {
        table[kind.rawValue] = renderer
    }

    private mutating func alias(_ retired: String, to kind: Kind) {
        guard let renderer = table[kind.rawValue] else {
            assertionFailure("alias \"\(retired)\" points at an unregistered kind")
            return
        }
        aliases[retired] = renderer
    }

    /// Every kind, aliases included. The React export exists so a test can
    /// assert no key here is a dotted page id — page ids are namespaced, so a
    /// dotted key would be a registry that had started naming screens.
    var kinds: [String] { Array(table.keys) + Array(aliases.keys) }

    func contains(_ kind: String) -> Bool {
        table[kind] != nil || aliases[kind] != nil
    }

    /// Draw a control, or name the kind that could not be drawn.
    @MainActor
    func makeView(for control: Control, context: ControlContext) -> NSView {
        guard let renderer = table[control.kind] ?? aliases[control.kind] else {
            return UnsupportedView(kind: control.kind, id: control.id)
        }
        return renderer(control, context)
    }
}

/// A registered kind.
///
/// A type rather than a bare string so `Renderers.register` cannot be handed
/// a page id by accident, and so the SwiftUI-adjacent mistake of a `switch` over
/// strings is not available here at all.
enum Kind: String {
    // Registered, and each one draws.
    case homeSummary
    case checklist
    case wizard
    case note
    case version
    case button
    case matrix
    case overrides
    case conflictResolver
    case observeToggle
    case auditList
    case profileList
    case fileTypeList
    case pipelineTrace
    case pluginList
    case trial
    case credits
    case license
    case palette
    case shortcutList
    case zoneEditor
    case finderMenu
    case pluginDetail
    case keymapEditor
    case ruleBuilder
    case schemaForm
    case switcherPanel
}

// MARK: - Unsupported

/// What a kind this shell cannot draw produces.
///
/// Not a blank. A row that names the kind, so a page carrying a control from a
/// newer plugin produces a visible and explicable gap rather than a hole in
/// the middle of the page.
final class UnsupportedView: NSView {
    init(kind: String, id: String) {
        super.init(frame: .zero)

        let headline = NSTextField(labelWithString: id.isEmpty
            ? "Unsupported control: \(kind)"
            : "\(id) — unsupported control: \(kind)")
        headline.font = Typeface.bodyStrong
        headline.textColor = Palette.warn

        let detail = NSTextField(labelWithString:
            "This page uses a control kind this version of CrossOS cannot draw. "
            + "The page is otherwise complete, and this row is where to look when a plugin adds one.")
        detail.font = Typeface.caption
        detail.textColor = Palette.secondaryInk
        detail.lineBreakMode = .byWordWrapping
        detail.maximumNumberOfLines = 0

        let column = NSStackView(views: [headline, detail])
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
    required init?(coder: NSCoder) { fatalError("UnsupportedView is created in code") }
}

// MARK: - Cards

/// A card: the one shape in this app that earns a radius.
///
/// The content view is pinned on all four edges, which is what makes the box
/// take its height from the content. Without that an `NSBox` has no intrinsic
/// size at all and collapses to zero — a card that is present, correctly
/// configured, and not drawn, which is a failure mode with no visual symptom
/// other than absence.
///
/// `NSBox` rather than a drawn view, because AppKit's box is what a group of
/// rows in a settings pane IS, and it draws the separator, the background and
/// the corner as one thing that the system maintains. The React shell had this
/// as four CSS properties, which meant the hairlines between rows and the edge
/// around them were drawn by different rules and could disagree about where the
/// corner was.
class Card: NSBox {
    init(title: String? = nil) {
        super.init(frame: .zero)
        translatesAutoresizingMaskIntoConstraints = false
        boxType = .custom
        fillColor = Palette.cardBackground
        borderColor = Palette.hairline
        borderWidth = 1
        cornerRadius = Radius.card
        titlePosition = .noTitle

        // The padding, and the height that follows from it.
        //
        // `NSBox` has no `contentInsets` — a custom box's content view fills it
        // edge to edge, which is why the card measured 1px tall for a while
        // despite having a border, a fill and a radius: it had no content
        // size of its own and nothing was pinning any.
        //
        // So the padding and the pinning are here, in one place, rather than
        // at each of the call sites. Three sites each set `contentView` and
        // constrained the card in half a dozen ways, and a fourth page that
        // forgot one of them would have produced a card that was configured
        // correctly and drew nothing — a failure whose only symptom is
        // absence, and which no assertion can see without walking the tree.
    }

    /// Put a view inside, padded, and make the card as tall as it is.
    ///
    /// The view is added as a SUBVIEW rather than through `contentView`,
    /// because `contentView` on a custom box is not a sizing relationship: it
    /// fills the box edge to edge and does not tell the box how big the box
    /// should be. Pinning it on all four edges is what does — and the box then
    /// takes the stack's height, which is the stack's rows' heights plus the
    /// insets.
    func setContent(_ view: NSView) {
        let padded = NSStackView(views: [view])
        padded.orientation = .vertical
        padded.alignment = .leading
        padded.edgeInsets = NSEdgeInsets(
            top: Gap.row, left: Gap.group,
            bottom: Gap.row, right: Gap.group
        )
        padded.translatesAutoresizingMaskIntoConstraints = false
        addSubview(padded)

        NSLayoutConstraint.activate([
            padded.leadingAnchor.constraint(equalTo: leadingAnchor),
            padded.trailingAnchor.constraint(equalTo: trailingAnchor),
            padded.topAnchor.constraint(equalTo: topAnchor),
            padded.bottomAnchor.constraint(equalTo: bottomAnchor),
            // The width is the card's, not the content's: a vertical stack
            // otherwise takes its width from the widest child, which makes a
            // card exactly as wide as its longest label.
            padded.widthAnchor.constraint(equalTo: widthAnchor),
        ])
    }

    /// A card is the width of the pane it sits in, not the width of its
    /// longest label.
    ///
    /// This is a constraint at the USE site rather than inside the card,
    /// because a vertical `NSStackView` takes its width from its widest child
    /// and a card that did not override that would be a card sized to its
    /// content — the same mistake the React shell made in the other
    /// direction, where a fixed `minmax(0, 220px)` label column left ~170px of
    /// void beside a 50px label. The rule is: the container decides the width,
    /// the content decides the height.
    func fillWidth() {
        translatesAutoresizingMaskIntoConstraints = false
        // Stated, not negotiated. A `.width`-aligned stack honours each row's
        // own intrinsic width when it has one, and a card holding a stack of
        // labels always does — so the card came out 337px in an 850px pane and
        // sat at x=493, right-aligned. Declaring the width here is what makes
        // "a card is the width of the pane" true rather than a hope.
        translatesAutoresizingMaskIntoConstraints = false
        let fill = widthAnchor.constraint(equalToConstant: 0)
        fill.isActive = false
    }

    @available(*, unavailable)
    required init?(coder: NSCoder) { fatalError("Card is created in code") }
}

/// A row: a label on the left, a value on the right, one shared edge.
///
/// Not a two-column grid. The React shell measured every row at
/// `cols=220px 460px` regardless of its label, which is a column it invented
/// rather than one the content needs, and the cost was ~170px of void beside a
/// 50px label. Here the value is pinned to the row's trailing edge with a
/// constraint and the label takes the rest, so a short label and a long one
/// both land on the same line and neither reserves space for the other.
class RowView: NSView {
    let label = NSTextField(labelWithString: "")
    let detail = NSTextField(labelWithString: "")

    init(label text: String, detail subtitle: String = "") {
        super.init(frame: .zero)

        label.stringValue = text
        label.font = Typeface.body
        label.textColor = Palette.primaryInk
        label.lineBreakMode = .byWordWrapping
        label.maximumNumberOfLines = 0
        label.setContentCompressionResistancePriority(.defaultLow, for: .horizontal)

        if subtitle.isEmpty {
            detail.isHidden = true
        } else {
            detail.stringValue = subtitle
            detail.font = Typeface.caption
            detail.textColor = Palette.secondaryInk
            detail.lineBreakMode = .byWordWrapping
            detail.maximumNumberOfLines = 0
        }

        let column = NSStackView(views: subtitle.isEmpty ? [label] : [label, detail])
        column.orientation = .vertical
        column.alignment = .leading
        column.spacing = 1
        column.translatesAutoresizingMaskIntoConstraints = false

        addSubview(column)
        NSLayoutConstraint.activate([
            column.leadingAnchor.constraint(equalTo: leadingAnchor),
            column.trailingAnchor.constraint(lessThanOrEqualTo: trailingAnchor),
            column.topAnchor.constraint(equalTo: topAnchor),
            column.bottomAnchor.constraint(equalTo: bottomAnchor),
        ])
    }

    @available(*, unavailable)
    required init?(coder: NSCoder) { fatalError("RowView is created in code") }
}
