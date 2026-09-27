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
        register(.menuList) { control, context in MenuListView(control: control, context: context) }
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
        // Three retired spellings, each mapping to the renderer that replaced
        // it. `traceList` is the one that was wrong: it pointed at `.note`, so
        // the Activity page's timeline rendered as a sentence. The daemon
        // still sends `traceList` (core/pkg/pluginapi/pages.go, `activityPage`)
        // while `pipelineTrace` is what the registry knows, and the two are the
        // same control under two names.
        alias("enableFlow", to: .wizard)
        alias("statusCard", to: .homeSummary)
        alias("traceList", to: .pipelineTrace)
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
    case menuList
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
final class UnsupportedView: NSStackView {
    init(kind: String, id: String) {
        super.init(frame: .zero)
        orientation = .vertical
        alignment = .width
        distribution = .fill
        translatesAutoresizingMaskIntoConstraints = false

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
/// **An `NSView` with a layer, not an `NSBox`.** An NSBox inserts a
/// material-capable host view as its own subview, and content added
/// afterwards sits behind that host. On screen the host is a transparent
/// material so nothing is lost; in a bitmap and in a PDF it is an
/// `NSVisualEffectView` that composites against the window server, has
/// nothing to composite against, and draws opaque. Every page rendered
/// with its title, its description and two holes exactly where the cards
/// were, and the two fixes before this one — `boxType = .custom`, then
/// bringing the content to the front — did not touch it, because the host
/// view is inserted by the BOX and not by the box type.
///
/// A plain view with a corner radius draws a fill and a stroke and nothing
/// else, which is what a card is: a settings pane's card is flat
/// `controlBackgroundColor` on a hairline, not glass. Vibrancy on a card is
/// also the "glass everywhere" tell the antislop rules name — a surface that
/// pretends to be see-through so that elevation reads as depth when there is
/// no depth behind it.
class Card: NSStackView {
    init(title: String? = nil) {
        super.init(frame: .zero)
        orientation = .vertical
        alignment = .width
        distribution = .fill
        translatesAutoresizingMaskIntoConstraints = false
        translatesAutoresizingMaskIntoConstraints = false
        orientation = .vertical
        // `.leading`, NOT `.width`.
        //
        // `.width` was supposed to make the card take the height of its rows
        // and it does — but it also leaves each child at the child's own
        // width and places it on the cross axis, so a 301pt stack of labels
        // sat at x=489 inside an 810pt card. Every line in every card was
        // right-aligned in a card that fills, which is a card that is
        // technically laid out and completely unreadable.
        //
        // `.leading` with a `.fill` distribution stretches the child to the
        // card's width and keeps its rows at the left, which is what a
        // settings card is.
        alignment = .leading
        distribution = .fill
        // The padding is ON the card rather than around it, and that is the
        // fix as much as the padding is: an inner stack with `edgeInsets`
        // measures its insets above its own origin, which put the content at
        // y=-162 in a card whose frame said 0.
        edgeInsets = NSEdgeInsets(
            top: Gap.row, left: Gap.group,
            bottom: Gap.row, right: Gap.group
        )
    }

    /// The fill and the hairline, drawn in `draw(_:)` rather than set on the
    /// layer.
    ///
    /// A layer's `backgroundColor` is a property the compositor reads, and a
    /// PDF context is not a compositor: `dataWithPDF(inside:)` asks each view
    /// to DRAW, and a view whose only fill lives on its layer has nothing to
    /// draw. Every page came out with its title, its description and two holes
    /// exactly where the cards were, for two reasons stacked — the NSBox host
    /// view on top, and then the layer fill underneath it.
    ///
    /// `draw(_:)` is the path a print job takes and the path a PDF takes, so
    /// this is the one that renders in both.
    public override func draw(_ dirtyRect: NSRect) {
        let path = NSBezierPath(roundedRect: bounds.insetBy(dx: 0.5, dy: 0.5),
                                xRadius: Radius.card, yRadius: Radius.card)
        Palette.cardBackground.setFill()
        path.fill()
        Palette.hairline.setStroke()
        path.lineWidth = 1
        path.stroke()
    }

    @available(*, unavailable)
    required init?(coder: NSCoder) { fatalError("Card is created in code") }

    /// Put a view inside, padded, and make the card as tall as it is.
    /// The card IS the stack.
    ///
    /// The first arrangement wrapped the content in a second
    /// `NSStackView` with `edgeInsets`, pinned it to the card on four edges,
    /// and left the card itself to work out its own height. It never could:
    /// a `.leading`-aligned vertical stack takes the height of its first
    /// child, that child is another stack that does the same, and the chain
    /// bottoms out at a leaf that reports the height of nothing. The card
    /// measured 0, `edgeInsets` added 40pt ABOVE its own origin, and the
    /// content was laid out at y=-162 — below the card, below the control
    /// that holds it, outside the clip view.
    ///
    /// So every page rendered with its title, its description and a hole
    /// exactly where the cards go, and the audit could not see it: the
    /// card's frame WAS 162pt, and its content was outside the frame. A
    /// measurement of a frame says nothing about what is inside it.
    ///
    /// `Card` is a stack now, and the padding is on it rather than around it.
    /// One coordinate space, one alignment, and the height is the sum of the
    /// rows.
    func setContent(_ view: NSView) {
        // `view` is already a vertical stack in most cases; adopting it means
        // the card's height IS the content's height, with no intermediate
        // view to mis-measure.
        //
        // The child is aligned LEADING, and that is the other half of the
        // card's own `.width`: the card's alignment is what makes the card
        // take the height of its rows, and the child's is what puts those
        // rows at the LEFT of a card that now fills 810pt. With `.width`
        // down here they stretched to the card's width and the text sat at
        // the trailing edge, which is a card that fills and is unreadable.
        if let stack = view as? NSStackView {
            // LEADING, and that is the other half of the card's own alignment:
            // the card's is what makes the card take the height of its rows,
            // and the child's is what puts those rows at the LEFT of a card
            // that fills the pane. With `.width` down here they stretched to
            // the card's width and the text sat at the trailing edge, which is
            // a card that fills and is unreadable.
            stack.alignment = .leading
        }
        view.translatesAutoresizingMaskIntoConstraints = false
        addArrangedSubview(view)

        // The content FILLS the card, and a priority is not a statement about
        // width — it says only how hard this view resists being squeezed, and
        // a view with no width to fill is measured at its intrinsic one no
        // matter how willing it is.
        //
        // Measured, the card was 810pt and its stack 387pt: the matrix's table
        // never saw the card's width, so the table kept the width of the
        // columns it could fit and the two columns past that rendered outside
        // the visible area with a horizontal scroller to reach them. A
        // settings pane with two hidden columns is a settings pane with two
        // hidden columns.
        NSLayoutConstraint.activate([
            view.widthAnchor.constraint(equalTo: widthAnchor),
            view.leadingAnchor.constraint(equalTo: leadingAnchor),
            view.trailingAnchor.constraint(equalTo: trailingAnchor),
        ])

        setHuggingPriority(.defaultLow, for: .vertical)
    }
}

/// A row: a label on the left, a value on the right, one shared edge.
///
/// Not a two-column grid. The React shell measured every row at
/// `cols=220px 460px` regardless of its label, which is a column it invented
/// rather than one the content needs, and the cost was ~170px of void beside a
/// 50px label. Here the value is pinned to the row's trailing edge with a
/// constraint and the label takes the rest, so a short label and a long one
/// both land on the same line and neither reserves space for the other.
/// One row of a settings card: a label, a sentence about it, and a control on
/// the right.
///
/// **The shape every reference agrees on.** Linear, Devin, Langdock, Fabric,
/// Mistral, Plain: the label is set at the left with its explanation directly
/// beneath in a lighter weight, the control sits alone on the shared right
/// edge, and a hairline separates one row from the next. The row is 44pt
/// because the label and its sentence need it — a single line is a web row, and
/// a row whose description is where the sentence is has no room for one.
///
/// A stack, because a plain `NSView` holding a pinned column measures the
/// column's absence rather than its content — which is how six other views in
/// this app rendered as nothing.
class RowView: NSStackView {
    let label = NSTextField(labelWithString: "")
    let detail = NSTextField(labelWithString: "")

    /// The control on the right. A row with no control is a heading, and a
    /// heading is not a row.
    var trailing: NSView?

    init(label text: String, detail subtitle: String = "", control: NSView? = nil) {
        super.init(frame: .zero)
        orientation = .vertical
        alignment = .width
        distribution = .fill
        translatesAutoresizingMaskIntoConstraints = false
        orientation = .horizontal
        alignment = .firstBaseline
        distribution = .fill
        translatesAutoresizingMaskIntoConstraints = false
        self.trailing = control

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

        // The label and its sentence on the LEFT, the control on the right, and
        // a gap between them that the row fills. That is the whole row: a
        // settings row is two texts and a control, and the two texts are a
        // column because a sentence describes the word above it.
        let texts = NSStackView(views: subtitle.isEmpty ? [label] : [label, detail])
        texts.orientation = .vertical
        texts.alignment = .leading
        texts.spacing = 1
        texts.translatesAutoresizingMaskIntoConstraints = false
        texts.setContentHuggingPriority(.defaultLow, for: .horizontal)
        texts.widthAnchor.constraint(greaterThanOrEqualToConstant: 200).isActive = true
        addArrangedSubview(texts)
        if let control {
            control.setContentHuggingPriority(.required, for: .horizontal)
            addArrangedSubview(control)
        }

    }

    @available(*, unavailable)
    required init?(coder: NSCoder) { fatalError("RowView is created in code") }
}

extension NSView {
    /// The first `Card` at or below this view, breadth-first. Used by
    /// `--audit` to print the constraint chain on a card, which is how the
    /// "renders as a hole" defect was traced: the card was 162pt tall and
    /// sitting at y=-162, below a row whose own height was zero.
    func firstCard() -> Card? {
        var queue: [NSView] = [self]
        while let view = queue.first {
            queue.removeFirst()
            if let card = view as? Card { return card }
            queue.append(contentsOf: view.subviews)
        }
        return nil
    }
}

// MARK: - menuList

/// The Finder menu: what a file can be done with.
///
/// A `TableContent` and a table like the others, because it IS a list and the
/// list is the thing. It was the one kind on the Explorer's page that the
/// registry did not know, and the shell said so in words rather than leaving a
/// hole: "finder — unsupported control: menuList". Naming the gap is right;
/// leaving it there once the daemon serves the data is not.
final class MenuListView: NSStackView, TableContent {
    let columns: [(String, CGFloat)] = [("Item", 260), ("Source", 180)]
    /// Filled by the async load below, so it cannot be a `let`: a control
    /// whose data arrives from the daemon has to be able to change once, and
    /// this is the one place it does.
    private var entries: [JSONValue] = []
    private let service: any CoreClient
    private let note: @Sendable (String) -> Void

    init(control: Control, context: ControlContext) {
        self.service = context.service
        self.note = context.note
        super.init(frame: .zero)
        orientation = .vertical
        alignment = .width
        distribution = .fill
        translatesAutoresizingMaskIntoConstraints = false

        let card = Card()
        let column = NSStackView()
        column.orientation = .vertical
        column.alignment = .leading
        column.spacing = Gap.row
        column.translatesAutoresizingMaskIntoConstraints = false
        addSubview(column)

        // Filled by the load below; the card is added either way so a failure
        // reads as a sentence in a card rather than as a page that lost its
        // shape.
        Task { await load(into: column, context: context) }
    }

    @available(*, unavailable)
    required init?(coder: NSCoder) { fatalError("MenuListView is created in code") }

    var rowCount: Int { entries.count }

    func text(row: Int, column: Int) -> String {
        let item = entries[row].objectValue ?? [:]
        switch column {
        case 0: return item["title"]?.stringValue ?? item["label"]?.stringValue ?? "(unnamed)"
        default: return item["source"]?.stringValue ?? item["bundle"]?.stringValue ?? ""
        }
    }

    private func load(into column: NSStackView, context: ControlContext) async {
        do {
            let items = try await context.service.finderMenu()
            entries = items
            let table = TableView(content: self)
            table.translatesAutoresizingMaskIntoConstraints = false
            column.addArrangedSubview(table)
            NSLayoutConstraint.activate([
                table.leadingAnchor.constraint(equalTo: column.leadingAnchor),
                table.trailingAnchor.constraint(equalTo: column.trailingAnchor),
                table.widthAnchor.constraint(equalTo: column.widthAnchor),
            ])
        } catch {
            column.addArrangedSubview(ErrorView(
                headline: "Could not read the Finder menu.",
                detail: "\(error)"
            ))
        }
        await MainActor.run { self.needsLayout = true }
    }
}
