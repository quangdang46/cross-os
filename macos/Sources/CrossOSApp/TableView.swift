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
final class TableView: NSView {
    private let table = NSTableView()
    private let scroll = NSScrollView()
    private let content: TableContent

    init(content: TableContent) {
        self.content = content
        super.init(frame: .zero)

        for (title, width) in content.columns {
            let column = NSTableColumn(identifier: NSUserInterfaceItemIdentifier(title))
            column.title = title
            column.width = width
            column.minWidth = 50
            table.addTableColumn(column)
        }
        table.dataSource = self
        table.delegate = self
        table.usesAlternatingRowBackgroundColors = true
        table.rowHeight = 26
        table.usesAutomaticRowHeights = false
        // A header is a header. `NSTableHeaderView` draws the system's own,
        // and one drawn by hand is one that has to learn resizing and sorting
        // and does not.
        table.headerView = NSTableHeaderView()

        scroll.documentView = table
        scroll.hasVerticalScroller = true
        scroll.autohidesScrollers = true
        scroll.drawsBackground = false
        scroll.translatesAutoresizingMaskIntoConstraints = false

        addSubview(scroll)
        NSLayoutConstraint.activate([
            scroll.leadingAnchor.constraint(equalTo: leadingAnchor),
            scroll.trailingAnchor.constraint(equalTo: trailingAnchor),
            scroll.topAnchor.constraint(equalTo: topAnchor),
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
    }

    @available(*, unavailable)
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

    @objc func fire(_ sender: NSButton) {
        guard let change = boxes[ObjectIdentifier(sender)] else { return }
        change(sender.state == .on)
    }
}
