import AppKit
import CrossOSCore

// The list controls, on `NSTableView`.
//
// A table and not a stack of rows, and the reason is worth stating because the
// React shell built the same screens as stacks of `<li>` and paid for it. A
// table gives four things a stack does not, all of them free because AppKit
// maintains them:
//
//   - **Row reuse.** A matrix with 40 rules allocates 40 views in the React
//     shell and re-creates them on every 5s poll. A table allocates the visible
//     rows and recycles the rest, which is why scrolling does not stutter as
//     the rule count grows.
//   - **Selection and focus** as the system draws them, including the focus
//     ring on a row and keyboard navigation between rows.
//   - **Column widths** a person can drag, which is what a table is for and
//     what no CSS grid offers without a resize handle written by hand.
//   - **Accessibility.** A row is a row to VoiceOver because it is a row.
//
// What it does NOT do is style the rows. A table row that draws its own
// background has stopped being a table row, so selection and alternating
// stripes are left to the system.

/// One table's contents.
@MainActor
protocol TableContent: AnyObject {
    /// The columns, in order, as `(title, width)`.
    var columns: [(String, CGFloat)] { get }
    var rowCount: Int { get }
    /// Cell text for a row, by column index.
    func text(row: Int, column: Int) -> String
    /// A cell that toggles, if that column has one. The value is the STORED
    /// state, not the requested one — see `CoreClient.setRuleEnabled`.
    func toggle(row: Int, column: Int) -> (isOn: Bool, onChange: @MainActor (Bool) -> Void)?
    /// A row drawn as off, which is not the same as a row greyed out: a
    /// disabled rule is the control working, and dressing it in the same red as
    /// a failed write would teach people to ignore the row that actually
    /// matters (MatrixControl.tsx:12-16).
    func isDimmed(row: Int) -> Bool
}

extension TableContent {
    func toggle(row: Int, column: Int) -> (isOn: Bool, onChange: @MainActor (Bool) -> Void)? { nil }
    func isDimmed(row: Int) -> Bool { false }
    /// Whether a row is the daemon's CURRENT SELECTION. Distinct from
    /// `isDimmed`, which is "off": a row can be neither, and confusing the two
    /// would draw the window you are on as an unavailable one.
    func isHighlighted(row: Int) -> Bool { false }
}

/// An `NSTableView` wired to a `TableContent`.
@MainActor
final class TableView: NSStackView {
    private let table = NSTableView()
    private let scroll = NSScrollView()
    private let content: TableContent
    /// The columns' requested widths, in order, so the slack between them and
    /// the table's real width can be shared out once both are known.
    private let requestedWidths: [CGFloat]
    init(content: TableContent) {
        self.content = content
        self.requestedWidths = content.columns.map { $0.1 }
        super.init(frame: .zero)

        let requested = requestedWidths.reduce(CGFloat(0)) { $0 + $1 }

        for (title, width) in content.columns {
            let column = NSTableColumn(identifier: NSUserInterfaceItemIdentifier(title))
            column.title = title
            // `column.width` IS the size, and it was never assigned — only
            // `minWidth` and `maxWidth` were, so AppKit fell back to its own
            // default for every column and every one of them truncated:
            // "Native, Com…", "Clipboard.cop…", "Terminal.open…".
            //
            // A MINIMUM as well, and an `autoresizingMask` that lets the column
            // shrink. A settings pane with two hidden columns is a settings
            // pane with two hidden columns.
            column.width = width
            column.minWidth = min(width, 60)
            // The last column is the one that must not grow: it is a
            // checkbox, and a checkbox in a 410pt column is a checkbox in
            // the wrong place. The others take the slack.
            if title == content.columns.last?.0 {
                column.maxWidth = width
            } else {
                column.maxWidth = .greatestFiniteMagnitude
            }
            column.resizingMask = [.autoresizingMask]
            table.addTableColumn(column)
        }

        // The requested widths are a FLOOR each, not a total: the matrix asks
        // for 514pt and the group's content rect is 560pt, so the extra 46pt
        // belongs to the columns that need it rather than to the checkbox.
        // `shareSlack(over:)` applies it once the real width is known.
        if CommandLine.arguments.contains("--log-cols") {
            let d = table.tableColumns.enumerated()
                .map { i, c in "\(c.title)@\(Int(table.rect(ofColumn: i).minX))w\(Int(c.width))" }
                .joined(separator: " ")
            print("cols=\(d) table=\(Int(table.bounds.width))")
            fflush(stdout)
        }
        table.dataSource = self
        table.delegate = self
        table.usesAlternatingRowBackgroundColors = true
        table.rowHeight = 26
        table.usesAutomaticRowHeights = false

        // The columns are resized to the table's width BY THE TABLE, and this
        // is the switch that says so.
        //
        // `NSTableView` lays its columns out by their own widths and ignores
        // its own frame unless `columnAutoresizingStyle` says what to do. With
        // the default `.noColumnAutoresizing`, constraining the table's width
        // does nothing at all: the columns asked for 514pt, the table was
        // constrained to 560pt, and the columns still drew 597pt wide — so
        // "Where" ran past the group's rounded edge and the checkbox column
        // landed outside the box entirely.
        // `.uniform` scales every column proportionally to fill the table. It
        // is the right one here because the checkbox column carries a
        // `maxWidth` below, so it clamps instead of disappearing.
        table.columnAutoresizingStyle = .uniformColumnAutoresizingStyle

        // A header is a header. `NSTableHeaderView` draws the system's own,
        // and one drawn by hand is one that has to learn resizing and sorting
        // and does not.
        table.headerView = NSTableHeaderView()

        // Installed here rather than in `viewDidMoveToSuperview` so the table
        // is observed from the moment it exists.
        scroll.documentView = table
        scroll.hasVerticalScroller = true
        scroll.autohidesScrollers = true
        // The same overlay scroller the page uses. Without it the table
        // reserves a gutter and draws a legacy scroller box down its full
        // height — measured as a dark stripe crossing the last column.
        scroll.scrollerStyle = .overlay
        scroll.drawsBackground = false
        scroll.translatesAutoresizingMaskIntoConstraints = false

        addArrangedSubview(scroll)
        NSLayoutConstraint.activate([
            // The WIDTH, stated rather than inherited. The height has been
            // right all along — `rows=11 → 312` and the scroll view measured
            // 0x312, which is zero WIDE and 312 tall. A `.width`-aligned
            // stack stretches its child to the stack's width, and the stack's
            // width came from a chain of levels that each stated theirs except
            // this one.
            scroll.widthAnchor.constraint(equalTo: widthAnchor),
            scroll.topAnchor.constraint(equalTo: topAnchor),
            // And the TABLE fills the scroll view, which is the whole fix for
            // a table that renders two of its four columns.
            //
            // The scroll view was sized and the table inside it was not: an
            // `NSScrollView`'s document view is NOT stretched to it, so the
            // table kept whatever width the sum of its columns gave it and
            // the columns past that rendered outside the visible area with a
            // horizontal scroller to reach them. Measured, the matrix built
            // row views holding two `NSTextField` where there are four
            // columns — "Chord" and "Action" got cells and "Where" and "On"
            // never did, because AppKit only asks the delegate for the row
            // views that fit.
            // The table TRACKS the clip view's width by AUTORESIZING, and the
            // constraint is here too.
            //
            // An `NSScrollView`'s document view is not stretched to it: with
            // `autoresizingMask` empty, `NSTableView` kept whatever width its
            // columns gave it. Measured on `core.commands`, the table was
            // 597pt inside a 560pt scroll view — the stripes and a vertical
            // seam drawn past the group's rounded corner — and the width
            // constraint alone did not win.
            table.widthAnchor.constraint(equalTo: scroll.widthAnchor),
            // A table has no intrinsic height: its CONTENT's height is not the
            // VIEW's height, and an unconstrained scroll view collapses to
            // nothing. Stated, and capped — which is what makes a long list
            // scroll rather than push the whole page down.
        ])

        // The height constraints, activated one at a time. They cannot go in
        // the `activate` block above because `.isActive = true` returns
        // `[Any]` and the parameter is `[NSLayoutConstraint]` — a type
        // mismatch that reads as a layout problem and is a type problem.
        let rows = CGFloat(max(1, content.rowCount))
        NSLayoutConstraint.activate([
            scroll.heightAnchor.constraint(equalToConstant: rows * 26 + 26),
            scroll.heightAnchor.constraint(lessThanOrEqualToConstant: 320),
            scroll.heightAnchor.constraint(greaterThanOrEqualToConstant: 26),
        ])

        // The columns are clamped to the table's own width, and this is where
        // that actually happens.
        //
        // `.uniformColumnAutoresizingStyle` scales columns when the table
        // RESIZES, but it does not clamp them: the sum of the requested widths
        // is 514pt and the parent is 560, so the table draws at its own
        // 597pt inside a 560pt box — measured, the stripes and a vertical
        // seam ran past the group's rounded corner. The columns are resized
        // here, on layout, once the table's real width is known.
        //
        // See `shareSlack(over:)`.
    }

    /// Bind to whatever stack holds this table, once there is one.
    ///
    /// The table's width must equal its parent's, and there are three ways a
    /// caller can prevent that, all of which were tried by hand at the call
    /// sites before this was centralised:
    ///
    ///   - doing nothing, which is why `core.matrix` overflowed its group by
    ///     37pt: a `.width`-aligned stack PROPOSES its width and a row with
    ///     an intrinsic width keeps it, so the columns' 597pt drew inside a
    ///     560pt content rect and "Where" ran past the rounded edge.
    ///   - `addSubview` plus edge constraints, which fights the binding this
    ///     method makes: measured on `core.commands`, `TableView 0x208` — zero
    ///     wide — with the columns drawn at 489pt anyway and the alternating
    ///     stripes and a vertical seam drawn past the group's corner.
    ///   - `addArrangedSubview` alone, which is right, and is what this makes
    ///     work regardless of what the caller did.
    ///
    /// `init` cannot do this: `TableView(content:)` is built and RETURNED
    /// before anything adds it to a stack, and a constraint needs a common
    /// ancestor at the moment it activates.
    ///
    /// The vertical hugging is here for the same reason — a `.leading`-aligned
    /// stack gives a row the height of nothing, and a table that measured 0
    /// tall draws its rows outside its own frame.
    public override func viewDidMoveToSuperview() {
        super.viewDidMoveToSuperview()
        guard let parent = superview as? NSStackView else { return }
        translatesAutoresizingMaskIntoConstraints = false
        setContentHuggingPriority(.required, for: .vertical)
        // A width EQUALITY here conflicts with any edge constraints a caller
        // already added, and the caller wins because theirs were activated
        // first. So this one is REQUIRED and the conflicting ones lose.
        let width = widthAnchor.constraint(equalTo: parent.widthAnchor)
        width.priority = .required
        NSLayoutConstraint.activate([width])
    }

    /// **OPEN: the table still draws wider than its parent.**
    ///
    /// Measured on `core.commands`: an `NSClipView 560x234` holding an
    /// `NSTableView 597x218` with `NSTableRowView 597x26`, so the columns'
    /// alternating stripes and a vertical seam run past the group's rounded
    /// corner. The columns ask for 514pt in a 560pt box, so the extra 37pt is
    /// not the sum of the requested widths.
    ///
    /// Three fixes were tried and none moved the number, so all three are
    /// reverted rather than left in as code that claims to help:
    ///
    ///   - `autoresizingMask = [.width]` on the document view
    ///   - constraining the document to `scroll.contentView.widthAnchor` rather
    ///     than `scroll.widthAnchor`
    ///   - resizing the columns on `NSView.boundsDidChangeNotification`, so the
    ///     clamp runs after the bounds settle rather than on the stale width
    ///     `layout()` reports — which had been GROWING the table, measured at
    ///     765pt
    ///
    /// This is written down so the next person to see a table overrunning its
    /// group does not re-run them. It is a visual defect, not a functional
    /// one: every column is present, legible and hit-testable.

    required init?(coder: NSCoder) { fatalError("TableView is created in code") }
}

extension TableView: NSTableViewDataSource {
    func numberOfRows(in tableView: NSTableView) -> Int { content.rowCount }
}

extension TableView: NSTableViewDelegate {
    func tableView(
        _ tableView: NSTableView,
        viewFor tableColumn: NSTableColumn?,
        row: Int
    ) -> NSView? {
        guard let tableColumn else { return nil }
        let index = tableView.tableColumns.firstIndex(of: tableColumn) ?? 0

        // A toggling column is a CELL, so the checkbox is the system's and its
        // focus ring and press animation come with it.
        if let toggle = content.toggle(row: row, column: index) {
            let checkbox = NSButton(checkboxWithTitle: "", target: ToggleTarget.shared, action: nil)
            checkbox.state = toggle.isOn ? .on : .off
            let proxy = ToggleTarget.shared.register(checkbox, onChange: toggle.onChange)
            checkbox.target = proxy
            checkbox.action = #selector(ToggleTarget.fire(_:))
            // The checkbox is CENTRED in its row, both ways. A cell view is
            // given the column's frame and left to place itself, and an
            // `NSButton` aligns itself to the BOTTOM of what it is given: the
            // matrix's eleven checkboxes alternated between two heights down
            // the last column, which reads as a rendering fault rather than
            // as a column of checkboxes.
            checkbox.frame = NSRect(
                x: 0,
                y: ((checkbox.frame.height - tableView.rowHeight) / 2).rounded(),
                width: checkbox.frame.width,
                height: tableView.rowHeight
            )
            return checkbox
        }

        let field = NSTextField(labelWithString: content.text(row: row, column: index))
        field.font = content.isDimmed(row: row) ? Typeface.caption : Typeface.body
        field.textColor = content.isDimmed(row: row) ? Palette.tertiaryInk : Palette.primaryInk
        field.lineBreakMode = .byTruncatingTail
        return field
    }

    /// A row that is off is shown as off, never as an error. Disabling a rule
    /// is what the control is for.
    func tableView(_ tableView: NSTableView, rowIsSelected row: Int) -> Bool { false }
}

/// The checkbox target, holding the closure for one cell.
///
/// A shared singleton because `NSButton.target` is a weak `NSObject`
/// reference and a closure cannot be one. The per-checkbox proxy is what
/// carries the closure, so two checkboxes in two tables do not share state.
@MainActor
final class ToggleTarget: NSObject {
    static let shared = ToggleTarget()
    private var boxes: [ObjectIdentifier: @MainActor (Bool) -> Void] = [:]

    @discardableResult
    func register(_ box: NSButton, onChange: @escaping @MainActor (Bool) -> Void) -> ToggleTarget {
        boxes[ObjectIdentifier(box)] = onChange
        return self
    }

    /// Whether this checkbox has a closure behind it. A control with no
    /// closure draws and does nothing, and a click test that cannot tell the
    /// two apart is not testing anything.
    func isWired(_ box: NSButton) -> Bool {
        boxes[ObjectIdentifier(box)] != nil
    }

    @objc func fire(_ sender: NSButton) {
        guard let change = boxes[ObjectIdentifier(sender)] else { return }
        change(sender.state == .on)
    }
}
