import AppKit
import CrossOSCore

// The switcher: a second window, on its own, drawn by AppKit.
//
// This is the app's other document. The settings window is a column of rows
// and every problem in it is a layout problem. This one is a borderless
// fullscreen panel that has to become key without a title bar, show a grid of
// tiles, and go away when the chord is released — and none of those four
// things is expressible in a settings window, which is why the React shell
// carried this as a second HTML document and why that document had never
// been rendered by anything in the repo before the port.
//
// The panel is an `NSPanel` with `.nonactivatingPanel` and
// `.borderless`, ordered with `orderFrontRegardless` so it appears without
// the switcher having to activate first. That is AppKit's own mechanism for
// a window that appears on a chord; the research's one caveat about it is
// that HIG's three-state taxonomy (key/main/inactive) does not describe what
// this window is, and AppKit's real key/main/inactive mechanics are what to
// design against.

@MainActor
public final class SwitcherOverlayController: NSObject, NSWindowDelegate {
    private let client: any CoreClient
    private let panel: NSPanel
    private let grid = SwitcherGridView()
    private var poll: Task<Void, Never>?

    /// The generation counter that stands in for the React shell's
    /// `useResource` one. The long poll is NOT cancelled when the overlay
    /// closes — the orphaned RPC runs to the daemon's own answer, which is the
    /// behaviour `switcher.tsx:152-155` had, and cancelling it would change
    /// the daemon's load rather than the shell's. What the counter does is
    /// DISCARD the answer from a run that has been torn down, which is the
    /// thing that would otherwise paint a panel that is already gone.
    private var generation = 0

    public init(client: any CoreClient) {
        self.client = client

        let panel = NSPanel(
            contentRect: NSRect(x: 0, y: 0, width: 720, height: 400),
            styleMask: [.borderless, .nonactivatingPanel],
            backing: .buffered,
            defer: false
        )
        panel.isFloatingPanel = true
        panel.level = .floating
        panel.hidesOnDeactivate = false
        panel.isReleasedWhenClosed = false
        // The panel is transparent and draws its own rounded card, which is
        // what a window-over-everything switcher looks like. The material is
        // the system's, so it is correct in both appearances — a
        // hand-mixed grey behind it would be a card that only matches in one.
        panel.backgroundColor = .clear
        panel.isOpaque = false
        panel.hasShadow = true
        panel.contentView = grid
        self.panel = panel

        super.init()
        // After super.init: assigning a delegate is touching `self`, and
        // Swift will not let an initialiser do that before the superclass
        // is initialised.
        panel.delegate = self
        // After super.init, because it reads  and Swift will not let
        // an initialiser touch self before the superclass is initialised.
        centre()
    }

    /// On the screen the person is looking at, which is the one under the
    /// pointer when there are several. Centring on `NSScreen.main` puts a
    /// switcher for a window on a second display in the wrong place.
    private func centre() {
        guard let screen = NSScreen.main else { return }
        let size = panel.frame.size
        let visible = screen.visibleFrame
        panel.setFrameOrigin(NSPoint(
            x: visible.midX - size.width / 2,
            y: visible.midY - size.height / 2
        ))
    }

    /// Show the panel and start waiting for a trigger.
    ///
    /// `SwitcherWait(timeoutMs:)` may block for the daemon's own 35s cap
    /// whatever budget it is passed, so this is a detached task and not
    /// anything on the main actor's behalf — the window stays responsive
    /// while the call is outstanding.
    public func summon() {
        panel.orderFrontRegardless()
        generation += 1
        let mine = generation

        poll?.cancel()
        poll = Task { [weak self] in
            while !Task.isCancelled {
                // The generation is read into a local before the comparison,
                // because `await` cannot appear to the right of an operator.
                // It is read TWICE — once for the loop condition and once
                // after the RPC — and that is the whole mechanism: an answer
                // from a run that has been torn down is discarded rather than
                // painted onto a panel that is already gone.
                let live = await self?.currentGeneration
                guard let self, mine == live else { return }
                do {
                    let trigger = try await self.client.switcherWait(timeoutMS: 0)
                    let afterRPC = await self.currentGeneration
                    guard mine == afterRPC else { return }
                    if trigger.triggered {
                        await self.refreshWindows()
                    }
                    // `triggered == false` is the daemon's "budget expired" —
                    // the answer a poll expects. It is NOT an error, and a
                    // client that treats it as one shows a broken switcher
                    // every time nothing happens, which is most of the time.
                } catch {
                    let stillLive = await self.currentGeneration
                    guard mine == stillLive else { return }
                    self.grid.showError("The daemon did not answer.", "\(error)")
                    return
                }
            }
        }
    }

    private var currentGeneration: Int { generation }

    /// Re-read the window list. The daemon owns the MRU, so the order of the
    /// answer IS the order to draw — re-sorting it would be a bug that looks
    /// like tidying, and `WindowRow`'s comment says so because it got said
    /// once already.
    private func refreshWindows() async {
        do {
            let windows = try await client.windows()
            await MainActor.run { self.grid.draw(windows: windows) }
        } catch {
            // The daemon's own message, in the panel. A blank switcher with no
            // reason is the failure the React shell had for a week: a
            // permission error is not a layout problem, and only the daemon
            // knows what it was.
            await MainActor.run { self.grid.showError("Could not list windows.", "\(error)") }
        }
    }

    public func dismiss() {
        generation += 1
        poll?.cancel()
        poll = nil
        panel.orderOut(nil)
    }

    public func windowWillClose(_ notification: Notification) {
        poll?.cancel()
        poll = nil
    }

    public func showForTesting(_ windows: [WindowRow]) {
        grid.draw(windows: windows)
    }
}

/// The tile grid.
///
/// The geometry is `SwitcherGeometry.tileGrid`, which is the arithmetic ported
/// back from the TypeScript that had ported it from Swift. The drawing is
/// `NSView` in a flipped view — the same approach `alt-tab-macos` takes, and
/// the reason that reference's specs are phrased in terms of "the far edge"
/// rather than "the right edge": the document is flipped, so y grows down and
/// the whole thing reads as a grid on a page rather than a stack of rectangles
/// measured from the bottom.
@MainActor
final class SwitcherGridView: NSView {
    private let stack = NSStackView()
    private var tileSize = TileSize.candidates[0]

    override var isFlipped: Bool { true }

    init() {
        super.init(frame: NSRect(x: 0, y: 0, width: 720, height: 400))
        wantsLayer = true

        let card = NSVisualEffectView()
        card.material = .hudWindow
        card.blendingMode = .behindWindow
        card.state = .active
        card.wantsLayer = true
        card.layer?.cornerRadius = 14
        card.translatesAutoresizingMaskIntoConstraints = false

        stack.orientation = .vertical
        stack.alignment = .leading
        stack.spacing = 2
        stack.translatesAutoresizingMaskIntoConstraints = false

        card.addSubview(stack)
        addSubview(card)
        NSLayoutConstraint.activate([
            card.centerXAnchor.constraint(equalTo: centerXAnchor),
            card.centerYAnchor.constraint(equalTo: centerYAnchor),
            card.leadingAnchor.constraint(greaterThanOrEqualTo: leadingAnchor, constant: 24),
            card.trailingAnchor.constraint(lessThanOrEqualTo: trailingAnchor, constant: -24),
            card.topAnchor.constraint(greaterThanOrEqualTo: topAnchor, constant: 24),
            card.bottomAnchor.constraint(lessThanOrEqualTo: bottomAnchor, constant: -24),
        ])
    }

    @available(*, unavailable)
    required init?(coder: NSCoder) { fatalError("SwitcherGridView is created in code") }

    func draw(windows: [WindowRow]) {
        stack.arrangedSubviews.forEach {
            stack.removeArrangedSubview($0)
            $0.removeFromSuperview()
        }
        guard !windows.isEmpty else {
            stack.addArrangedSubview(label("No windows to switch between.", secondary: true))
            return
        }

        let widths = windows.map { _ in CGFloat(300) }
        let input = TileGridInput(
            widths: widths,
            tileHeight: tileSize.tileHeight,
            padding: SwitcherGeometry.tilePadding,
            widthMax: SwitcherGeometry.panelMaxWidth
        )
        let grid = tileGrid(input)
        let offsets = centeringOffsets(
            rowWidths: grid.rows.map { $0.map { widths[$0] } },
            padding: SwitcherGeometry.tilePadding,
            within: SwitcherGeometry.panelMaxWidth
        )

        for (rowIndex, row) in grid.rows.enumerated() where !row.isEmpty {
            let line = NSStackView()
            line.orientation = .horizontal
            line.alignment = .top
            line.spacing = SwitcherGeometry.tilePadding * 2
            // The row's own offset along the writing direction. It is the
            // PANEL's width and not the grid's maxX that this is measured
            // against, which is why the last row of a short list sits under
            // the middle of the panel rather than under the middle of the
            // tiles.
            line.edgeInsets = NSEdgeInsets(top: 0, left: offsets[rowIndex],
                                          bottom: 0, right: 0)
            for position in row {
                line.addArrangedSubview(tile(windows[position]))
            }
            stack.addArrangedSubview(line)
        }
    }

    private func tile(_ window: WindowRow) -> NSView {
        let box = NSBox()
        box.boxType = .custom
        box.fillColor = window.selected
            ? NSColor.controlAccentColor.withAlphaComponent(0.16)
            : NSColor.controlBackgroundColor.withAlphaComponent(0.7)
        box.borderColor = window.selected
            ? .controlAccentColor
            : .separatorColor
        box.borderWidth = window.selected ? 2 : 1
        box.cornerRadius = 10
        box.titlePosition = .noTitle

        // The window's title and the app it belongs to get the SAME treatment,
        // and it is a TRUNCATION rather than a break. The React shell had
        // `overflow-wrap: anywhere` on the name and ellipsis on the app line,
        // so the same string was drawn two ways a few pixels apart:
        // "com.apple.Safar" / "i" in one and "com.apple.find…" in the other.
        // `anywhere` also shrinks an element's min-content size, which is what
        // squeezed it in the first place.
        let name = NSTextField(labelWithString: window.title.isEmpty ? window.appID : window.title)
        name.font = .systemFont(ofSize: 12, weight: .medium)
        name.textColor = .labelColor
        name.lineBreakMode = .byTruncatingTail
        name.maximumNumberOfLines = 1

        let app = NSTextField(labelWithString: window.appID)
        app.font = .systemFont(ofSize: 11)
        app.textColor = .secondaryLabelColor
        app.lineBreakMode = .byTruncatingTail
        app.maximumNumberOfLines = 1

        let column = NSStackView(views: [name, app])
        column.orientation = .vertical
        column.alignment = .leading
        column.spacing = 1
        column.edgeInsets = NSEdgeInsets(
            top: SwitcherGeometry.tilePadding, left: SwitcherGeometry.tilePadding,
            bottom: SwitcherGeometry.tilePadding, right: SwitcherGeometry.tilePadding
        )
        column.translatesAutoresizingMaskIntoConstraints = false

        box.addSubview(column)
        NSLayoutConstraint.activate([
            column.leadingAnchor.constraint(equalTo: box.leadingAnchor),
            column.trailingAnchor.constraint(equalTo: box.trailingAnchor),
            column.topAnchor.constraint(equalTo: box.topAnchor),
            column.bottomAnchor.constraint(lessThanOrEqualTo: box.bottomAnchor),
            box.widthAnchor.constraint(equalToConstant: 300),
        ])
        return box
    }

    func showError(_ headline: String, _ detail: String) {
        stack.arrangedSubviews.forEach {
            stack.removeArrangedSubview($0)
            $0.removeFromSuperview()
        }
        let column = NSStackView(views: [label(headline), label(detail, secondary: true)])
        column.orientation = .vertical
        column.alignment = .leading
        column.spacing = 4
        stack.addArrangedSubview(column)
    }

    private func label(_ text: String, secondary: Bool = false) -> NSTextField {
        let field = NSTextField(labelWithString: text)
        field.font = secondary ? .systemFont(ofSize: 11) : .systemFont(ofSize: 13, weight: .medium)
        field.textColor = secondary ? .secondaryLabelColor : .labelColor
        field.lineBreakMode = .byWordWrapping
        field.maximumNumberOfLines = 0
        return field
    }
}
