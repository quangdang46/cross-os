import AppKit
import CrossOSCore

// The window.
//
// An `NSWindow` with an `NSSplitViewController`: a source list on the left, a
// page on the right. That is the shape macOS has used since System Preferences
// and it is what `NSSplitViewController` exists to draw — a resizable divider,
// a sidebar that can collapse, the title-bar inset that makes the split view
// read as chrome rather than as two views.
//
// Nothing here is custom chrome. The title bar is a title bar, the divider is
// a divider, and the sidebar is a source list. That is the point of the port:
// the previous shell drew a window that looked native and paid four bugs for
// it.

// MARK: - The window

/// The app's main window: a nav rail and a page.
public final class ShellWindowController: NSWindowController {
    private let client: any CoreClient
    private let split: NSSplitViewController
    private let sidebar: SidebarViewController
    private var refreshTimer: Timer?

    /// The app's window size.
    ///
    /// **This is the window's MINIMUM, not its size.** It was a fixed size —
    /// 1100x720 because `app/main.go:85` declared it and every screenshot in
    /// `docs/baseline/` was taken at it — and that fixed rectangle is the
    /// single biggest reason these panes looked wrong: content filled about a
    /// third of it and the rest was empty pane under every page. Fifteen pages
    /// of a few boxes each, floating in the bottom-left of a 720pt window.
    ///
    /// Rectangle sizes its window to its content and keeps the size per tab:
    ///
    ///     let fitting = vc.view.fittingSize          // SettingsWindowController.swift:145
    ///     let target = window.frameRect(forContentRect: ...)  // :167
    ///     window.setFrame(frame, display: true, animate: animated)
    ///
    /// and a macOS settings window is sized to what is in it. The titlebar
    /// adds on top of that, which is what `frameRect(forContentRect:)` is for.
    public static let contentSize = NSSize(width: 1100, height: 720)

    /// The window height floor. Below this the sidebar's fifteen rows stop
    /// fitting and scrolling a settings window to reach its last item is a
    /// worse answer than a little empty space.
    public static let minimumHeight: CGFloat = 480

    public init(client: any CoreClient) {
        self.client = client

        let window = NSWindow(
            contentRect: NSRect(origin: .zero, size: ShellWindowController.contentSize),
            styleMask: [.titled, .closable, .miniaturizable, .resizable, .fullSizeContentView],
            backing: .buffered,
            defer: false
        )
        window.title = "CrossOS"
        window.titlebarAppearsTransparent = false
        window.minSize = NSSize(width: 880, height: ShellWindowController.minimumHeight)
        // The split view IS the content view, so the title bar sits above it
        // rather than a toolbar floating in it. `fullSizeContentView` plus a
        // hidden title is what makes a sidebar reach the top of the window the
        // way System Settings' does.
        window.titleVisibility = .visible

        self.split = NSSplitViewController()
        self.sidebar = SidebarViewController(client: client)

        super.init(window: window)

        split.splitView.dividerStyle = .thin
        // The sidebar's width limits live on the SPLIT VIEW ITEM, not the
        // split view — `NSSplitView` has no thickness properties of its own,
        // and setting them there is a compile error rather than a silent
        // no-op, which is the friendlier of the two ways this could go.
        //
        // The rail's colour is on the sidebar's own view, not here: the split
        // view is transparent, and only the rail is a step behind the content
        // plane. The content side keeps the window's own background, which is
        // what makes the rail read as behind rather than beside.

        let page = PageViewController(client: client)
        sidebar.onSelect = { [weak self] page in
            self?.show(page)
        }

        let sidebarItem = NSSplitViewItem(sidebarWithViewController: sidebar)
        sidebarItem.minimumThickness = 180
        sidebarItem.maximumThickness = 260
        sidebarItem.canCollapse = true
        sidebarItem.holdingPriority = .defaultLow

        let pageItem = NSSplitViewItem(viewController: page)
        pageItem.minimumThickness = 560

        split.addSplitViewItem(sidebarItem)
        split.addSplitViewItem(pageItem)

        window.contentViewController = split
        window.setContentSize(ShellWindowController.contentSize)
        window.center()
    }

    @available(*, unavailable)
    required init?(coder: NSCoder) { fatalError("ShellWindowController is created in code") }

    /// The content pane's view controller, so a caller that needs to measure
    /// ONE pane — the audit, which is about a page's controls and not about
    /// the window's whole geometry — can reach it without walking the split
    /// view and guessing which half it landed in.
    public var pageController: NSViewController? {
        split.splitViewItems.last?.viewController
    }

    public func show(_ page: Page) {
        (split.splitViewItems.last?.viewController as? PageViewController)?.show(page)
        window?.title = page.title
        fitWindowToContent()
    }

    /// Size the window to the page's content, within a floor and a ceiling.
    ///
    /// This is the change that stops every page from being a few boxes in the
    /// bottom-left of a 720pt window. A macOS settings window is sized to what
    /// is in it — Rectangle does exactly this and keeps a saved size per tab
    /// (`SettingsWindowController.swift:141-171`):
    ///
    ///     let fitting = vc.view.fittingSize
    ///     let target = window.frameRect(forContentRect: NSRect(origin: .zero, size: contentSize))
    ///     window.setFrame(frame, display: true, animate: animated)
    ///
    /// **The floor is the point.** A page can legitimately be short — `core.about`
    /// is a licence and a version — and a window shrunk to a licence is a
    /// window the reader cannot see the rest of the app in. So the window
    /// shrinks to the content down to `minimumHeight` and no further, which
    /// means a short page has a little empty space BELOW it and a tall page
    /// has none at all. The empty space is at the bottom, where the eye does
    /// not read for content, instead of on three sides of a small island.
    ///
    /// Not animated: the window resizes on every nav click and an animation
    /// there makes the whole frame move under the pointer.
    private func fitWindowToContent() {
        guard let window, let page = pageController?.view else { return }
        // The page's fitting size is asked AFTER layout, or it is the previous
        // page's number: this runs on a nav click, and the new page's controls
        // have not loaded yet.
        let fitting = page.fittingSize
        guard fitting.width > 0, fitting.height > 0 else { return }
        let content = NSSize(
            width: ShellWindowController.contentSize.width,
            height: max(ShellWindowController.minimumHeight, fitting.height + Gap.plane)
        )
        let target = window.frameRect(forContentRect: NSRect(origin: .zero, size: content))
        var frame = window.frame
        // Keep the window's own position and only change its size — a settings
        // window that jumps up and down the screen as the reader navigates is
        // worse than the void it was fixing.
        frame.size = target.size
        window.setFrame(frame, display: true)
    }

    /// The 5s refresh, inherited from the React shell rather than invented.
    ///
    /// `App.tsx:385-389` polls every 5s and threads the result through a token
    /// that controls re-read on; there is no push anywhere in the protocol.
    /// The rule `useResource.ts:5-9` states — a control loads on mount and when
    /// this changes, and at no other time — ports with it.
    /// Stop the refresh. Called on terminate, and the counterpart to
    /// `startPolling` — a timer that outlives its window keeps a daemon
    /// connection open for an app that is on its way out.
    public func stopPolling() {
        refreshTimer?.invalidate()
        refreshTimer = nil
    }

    public func startPolling() {
        refreshTimer?.invalidate()
        refreshTimer = Timer.scheduledTimer(withTimeInterval: 5, repeats: true) { [weak self] _ in
            Task { @MainActor in
                await self?.sidebar.reload()
            }
        }
    }
}

// MARK: - The sidebar

/// The nav rail: a source list of pages, grouped by their declared group.
public final class SidebarViewController: NSViewController {
    private let client: any CoreClient
    private let outline = NSOutlineView()
    private let scroll = NSScrollView()
    private var pages: [Page] = []
    private var groups: [String] = []

    /// Called with the page a person picked. Set by the window.
    var onSelect: ((Page) -> Void)?

    /// A group of pages, in the shape `NSOutlineView` wants: an item that has
    /// children rather than a leaf.
    private struct Group {
        let name: String
        var pages: [Page]
    }

    public init(client: any CoreClient) {
        self.client = client
        super.init(nibName: nil, bundle: nil)
    }

    @available(*, unavailable)
    required init?(coder: NSCoder) { fatalError("SidebarViewController is created in code") }

    public override func loadView() {
        let container = NSView()
        // The rail PAINTS ITS BACKGROUND.
        //
        // `wantsLayer = true` on its own makes the view layer-backed and gives
        // it a transparent one — and a comment in `ShellWindowController`
        // claimed "the rail's colour is on the sidebar's own view", which was
        // never true: nothing ever set it. The rail rendered as clear glass
        // over the window's own background, so it was the same colour as the
        // content pane beside it and the two read as one pane with a list in
        // it.
        //
        // `Palette.sidebarBackground` is `underPageBackgroundColor`, which is
        // the system's own step behind the content plane and follows the
        // user's appearance — so this is a macOS rail in light and dark
        // without a colour in this file.
        //
        // It is DRAWN rather than set on the layer, for the reason `Card` draws
        // its own fill: `dataWithPDF(inside:)` asks a view to draw, and a view
        // whose only fill lives on its layer has nothing to draw — which is
        // why the rail was transparent in every render as well as on screen.
        container.wantsLayer = false

        let backdrop = FilledView(color: Palette.sidebarBackground)
        backdrop.translatesAutoresizingMaskIntoConstraints = false
        container.addSubview(backdrop, positioned: .below, relativeTo: nil)
        NSLayoutConstraint.activate([
            backdrop.leadingAnchor.constraint(equalTo: container.leadingAnchor),
            backdrop.trailingAnchor.constraint(equalTo: container.trailingAnchor),
            backdrop.topAnchor.constraint(equalTo: container.topAnchor),
            backdrop.bottomAnchor.constraint(equalTo: container.bottomAnchor),
        ])
        outline.dataSource = self

        outline.delegate = self
        outline.headerView = nil
        outline.rowSizeStyle = .default
        // `NSTableViewStyleSourceList`, and not the `.sourceList` shorthand —
        // the shorthand has been deprecated since macOS 12 and what it selects
        // is the OLD source list: a solid accent bar down the full height of
        // the selected row. It is the 2010 System Preferences look, and on a
        // modern window it reads as a highlight rather than as "you are
        // here".
        //
        // What System Settings draws is a TINT behind the row plus a small
        // accent bar, which is `.regular` with the accent brought down to a
        // wash. `NSTableView.SelectionHighlightStyle.sourceList` is the
        // deprecated spelling; `.regular` is the modern one and AppKit tints
        // it with the user's accent automatically, in both appearances.
        outline.style = .sourceList
        outline.selectionHighlightStyle = .regular
        // `.regular` and not the deprecated `.sourceList` highlight: the
        // deprecated one paints a SOLID accent bar the height of the row, which
        // is the 2010 System Preferences look. `.regular` tints the row with
        // the user's accent at a wash and leaves the text at full strength,
        // which is what System Settings draws and what a 26pt sidebar row can
        // carry without the highlight swallowing the label.
        outline.usesAlternatingRowBackgroundColors = false
        outline.floatsGroupRows = false
        outline.indentationPerLevel = 12
        outline.rowHeight = 30

        let symbolColumn = NSTableColumn(identifier: NSUserInterfaceItemIdentifier("symbol"))
        symbolColumn.width = 20
        symbolColumn.minWidth = 20
        symbolColumn.maxWidth = 20
        outline.addTableColumn(symbolColumn)

        let titleColumn = NSTableColumn(identifier: NSUserInterfaceItemIdentifier("title"))
        titleColumn.width = 200
        outline.addTableColumn(titleColumn)
        outline.outlineTableColumn = titleColumn

        scroll.documentView = outline
        scroll.hasVerticalScroller = true
        scroll.autohidesScrollers = true
        scroll.drawsBackground = false
        scroll.translatesAutoresizingMaskIntoConstraints = false

        container.addSubview(scroll)
        NSLayoutConstraint.activate([
            scroll.leadingAnchor.constraint(equalTo: container.leadingAnchor),
            scroll.trailingAnchor.constraint(equalTo: container.trailingAnchor),
            scroll.topAnchor.constraint(equalTo: container.topAnchor),
            scroll.bottomAnchor.constraint(equalTo: container.bottomAnchor),
        ])

        view = container
    }

    public override func viewDidAppear() {
        super.viewDidAppear()
        Task { await reload() }
    }

    /// Re-read the page list.
    ///
    /// A failure here empties the rail, which is the wrong response: a daemon
    /// that is not running is not a product with no pages, and a window that
    /// says so is more useful than a window that looks broken. The error goes
    /// in the log sink the page draws.
    public func reload() async {
        do {
            let served = try await client.pages()
            pages = served
            groups = []
            for page in served where !groups.contains(page.group) {
                groups.append(page.group)
            }
            outline.reloadData()
            // Every group is EXPANDED, because a settings window's rail is a
            // list of pages and a collapsed group is a heading over nothing.
            //
            // `isItemExpandable` returning true is what makes a group ELIGIBLE
            // to expand; it does not expand it. A freshly reloaded outline
            // collapses every group, which is why the rail still showed four
            // headers and no pages after the datasource was fixed — and why
            // the first selection landed on whatever row the collapsed tree
            // happened to number that way.
            // Iterated by GROUP, not by row index. Expanding a group ADDS
            // rows, so a loop over `numberOfRows` captured before the loop
            // misses every group after the first: measured, the rail showed
            // "HOME" with its three pages and then three empty headings —
            // SHORTCUTS, ACTIVITY and ADVANCED with nothing under them.
            for group in groups {
                outline.expandItem(group, expandChildren: true)
            }
            if outline.selectedRow < 0, !served.isEmpty {
                // A fresh profile lands on the page that declares firstRun —
                // the wizard — and the daemon decides which, not the id
                // (`app/backend/host.go:29-30`).
                let landing = served.firstIndex(where: \.firstRun) ?? 0
                selectRow(for: served[landing])
            }
        } catch {
            pages = []
            groups = []
            outline.reloadData()
        }
    }

    private var flatRows: [Row] {
        groups.flatMap { group in
            [.group(name: group)] + pages.filter { $0.group == group }.map { .page($0) }
        }
    }

    private enum Row {
        case group(name: String)
        case page(Page)
    }

    private func selectRow(for page: Page) {
        let rows = flatRows
        guard let index = rows.firstIndex(where: { row in
            if case .page(let candidate) = row { return candidate.id == page.id }
            return false
        }) else { return }
        outline.selectRowIndexes(IndexSet(integer: index), byExtendingSelection: false)
        outline.scrollRowToVisible(index)
        onSelect?(page)
    }
}

extension SidebarViewController: NSOutlineViewDataSource {
    /// A group has the pages in it; a page has nothing under it.
    ///
    /// Both halves were wrong, and together they meant **the sidebar rendered
    /// four group headers and no page rows at all** — the nav rail had nothing
    /// to click, so the app could not be navigated. Measured on the live tree:
    /// `NSOutlineView 260x670` holding four `NSTableRowView`, every one of them
    /// a `NSTextField` reading "HOME" / "SHORTCUTS" / "ACTIVITY" / "ADVANCED",
    /// and the selection parked on the ADVANCED header because
    /// `NSTableRowSidebarSelectionView` was on a group.
    ///
    ///   - `numberOfChildrenOfItem` returned 0 for anything that was not the
    ///     root, so a group claimed to have no children.
    ///   - `isItemExpandable` returned `false` unconditionally, so the outline
    ///     never asked for them even when they existed.
    ///
    /// Every layout defect fixed before this was measured on the content pane
    /// alone — the shot harness rendered `pageController.view` and nothing
    /// else, so the half of the app that frames every page was never rendered
    /// until `--shots --window`.
    public func outlineView(_ outlineView: NSOutlineView, numberOfChildrenOfItem item: Any?) -> Int {
        if let group = item as? String {
            return pages.filter { $0.group == group }.count
        }
        return groups.count
    }

    public func outlineView(_ outlineView: NSOutlineView, child index: Int, ofItem item: Any?) -> Any {
        // The children of a group are that group's PAGES, in the daemon's order
        // — the same order `flatRows` walks, which is what makes the row index
        // `selectRow(for:)` computes land on the page rather than beside it.
        if let group = item as? String {
            return pages.filter { $0.group == group }[index]
        }
        return groups[index]
    }

    /// A group expands; a page is a leaf.
    public func outlineView(_ outlineView: NSOutlineView, isItemExpandable item: Any) -> Bool {
        item is String
    }
}

extension SidebarViewController: NSOutlineViewDelegate {
    public func outlineView(_ outlineView: NSOutlineView, isGroupItem item: Any) -> Bool { true }

    public func outlineView(_ outlineView: NSOutlineView, shouldSelectItem item: Any) -> Bool {
        // A group header is a cap, not a destination. A settings window whose
        // "Advanced" row opens onto nothing is a broken promise, and this is
        // where that promise is made.
        if let name = item as? String { return name.isEmpty }
        return true
    }

    public func outlineView(_ outlineView: NSOutlineView, viewFor tableColumn: NSTableColumn?, item: Any) -> NSView? {
        if let group = item as? String {
            let cap = NSTextField(labelWithString: group.uppercased())
            cap.font = Typeface.sectionCap
            cap.textColor = Palette.secondaryInk
            return cap
        }
        guard let page = item as? Page else { return nil }
        return SidebarRowView(page)
    }

    public func outlineViewSelectionDidChange(_ notification: Notification) {
        let rows = flatRows
        guard outline.selectedRow >= 0, outline.selectedRow < rows.count else { return }
        if case .page(let page) = rows[outline.selectedRow] {
            onSelect?(page)
        }
    }
}

/// One nav row: a symbol and a title, both drawn by the system.
final class SidebarRowView: NSTableCellView {
    init(_ page: Page) {
        super.init(frame: .zero)

        let title = NSTextField(labelWithString: page.title)
        title.font = Typeface.body
        title.lineBreakMode = .byTruncatingTail
        title.translatesAutoresizingMaskIntoConstraints = false
        addSubview(title)

        // An SF Symbol, or nothing. The daemon sends a NAME and the system
        // draws it; a name the system does not know is nil, and a nil symbol
        // leaves the title alone rather than a gap. That is the whole contract
        // with `core.pages`.
        let imageView = NSImageView()
        if let image = NSImage(systemSymbolName: page.symbol, accessibilityDescription: page.title) {
            imageView.image = image
        }
        imageView.contentTintColor = Palette.secondaryInk
        imageView.translatesAutoresizingMaskIntoConstraints = false
        addSubview(imageView)

        NSLayoutConstraint.activate([
            imageView.leadingAnchor.constraint(equalTo: leadingAnchor, constant: 2),
            imageView.centerYAnchor.constraint(equalTo: centerYAnchor),
            imageView.widthAnchor.constraint(equalToConstant: 16),
            imageView.heightAnchor.constraint(equalToConstant: 16),

            title.leadingAnchor.constraint(equalTo: imageView.trailingAnchor, constant: 7),
            title.trailingAnchor.constraint(equalTo: trailingAnchor, constant: -6),
            title.centerYAnchor.constraint(equalTo: centerYAnchor),
        ])

        textField = title
    }

    @available(*, unavailable)
    required init?(coder: NSCoder) { fatalError("SidebarRowView is created in code") }
}


/// A view that paints one colour over its own bounds.
///
/// `NSView` has no `drawsBackground`, and `draw` is a method rather than a
/// settable closure — so a plain view cannot be given a fill without either a
/// layer or a subclass. The layer route is the one that made the rail
/// invisible: `dataWithPDF(inside:)` asks a view to DRAW, and a view whose
/// only fill lives on its layer has nothing to draw.
final class FilledView: NSView {
    private let color: NSColor

    init(color: NSColor) {
        self.color = color
        super.init(frame: .zero)
    }

    override var isFlipped: Bool { true }

    override func draw(_ dirtyRect: NSRect) {
        color.setFill()
        dirtyRect.fill()
    }

    @available(*, unavailable)
    required init?(coder: NSCoder) { fatalError("FilledView is created in code") }
}
