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

    /// The app's window size. `app/main.go:85` is where the Wails window
    /// declared 1100x720, and it is the size every screenshot in
    /// docs/baseline/ was taken at — the port is judged against those, so the
    /// window does not get to be a different shape than what it replaces.
    public static let contentSize = NSSize(width: 1100, height: 720)

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
        window.minSize = NSSize(width: 880, height: 560)
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

    public func show(_ page: Page) {
        (split.splitViewItems.last?.viewController as? PageViewController)?.show(page)
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
        container.wantsLayer = true

        outline.dataSource = self
        outline.delegate = self
        outline.headerView = nil
        outline.rowSizeStyle = .default
        outline.style = .sourceList
        // A source list draws its own selection and its own focus. Both of
        // those are the reason to use it rather than a table with a custom
        // appearance: the focus ring and the selected-row treatment are the
        // system's, so they are correct in every appearance and at every
        // accessibility setting.
        outline.usesAlternatingRowBackgroundColors = false
        outline.floatsGroupRows = false
        outline.indentationPerLevel = 12
        outline.rowHeight = 26

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
    public func outlineView(_ outlineView: NSOutlineView, numberOfChildrenOfItem item: Any?) -> Int {
        guard item == nil else { return 0 }
        return groups.count
    }

    public func outlineView(_ outlineView: NSOutlineView, child index: Int, ofItem item: Any?) -> Any {
        groups[index]
    }

    public func outlineView(_ outlineView: NSOutlineView, isItemExpandable item: Any) -> Bool {
        false
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
