import AppKit
import CrossOSCore

// A page: its title, its description, and whatever its controls draw.
//
// The layout is a vertical stack, and every control is an `NSView` the
// renderer returned. There is no grid with a label column, because there is no
// label column: the daemon's schema decides what a page contains, and the
// React shell's fixed `minmax(0, 220px) minmax(0, 1fr)` was a column it had
// invented and every row then had to fit into — a 50px label like "version"
// left ~170px of void beside it, and a page could hold a 220px-label row
// beside a 1fr-label row so the label edge jumped between two rows a reader
// was meant to see as one list.
//
// A vertical stack has no such edge to jump, and Auto Layout sizes each row to
// the row.

public final class PageViewController: NSViewController {
    private let client: any CoreClient
    private let scroll = NSScrollView()
    private let stack = NSStackView()
    private let titleField = NSTextField(labelWithString: "")
    private let descriptionField = NSTextField(labelWithString: "")
    private let pageId = NSTextField(labelWithString: "")
    private var currentPage: Page?

    public init(client: any CoreClient) {
        self.client = client
        super.init(nibName: nil, bundle: nil)
    }

    @available(*, unavailable)
    required init?(coder: NSCoder) { fatalError("PageViewController is created in code") }

    public override func loadView() {
        let container = NSView()

        // The page title. A card, not a full-bleed banner: this is a settings
        // window, and the first screen a person meets should look like the rest
        // of them.
        titleField.font = Typeface.pageTitle
        titleField.textColor = Palette.primaryInk

        descriptionField.font = Typeface.body
        descriptionField.textColor = Palette.secondaryInk
        descriptionField.maximumNumberOfLines = 2

        // The page's own id, small and quiet at the foot. It is there because
        // a person reporting a problem says "the About page is wrong" and the
        // id is what makes that reproducible — and because a plugin page whose
        // id is visible is a page whose provenance is not a mystery.
        pageId.font = Typeface.mono
        pageId.textColor = Palette.tertiaryInk

        stack.orientation = .vertical
        // `.width`, not `.leading`. A `.leading` stack gives each row its
        // INTRINSIC width, so a card came out 337px wide in an 850px pane with
        // 500px of empty pane beside it — the same mistake the React shell
        // made in the other direction, where a fixed `minmax(0, 220px)` label
        // gutter left ~170px of void between a label and its value.
        //
        // `.width` makes every row the width of the stack, which is what a
        // settings pane's rows are. What aligns INSIDE a row is its own
        // business: the card's content stack is `.leading`, so a short label
        // sits at the card's left edge and does not pretend to fill it.
        // NOT `.width`, and that is a measured fact rather than a preference.
        //
        // `NSStackView.alignment` is an `NSLayoutConstraint.Attribute` on
        // macOS, and `.width` is a real case of it (rawValue 7) — but
        // assigning it does nothing: the stack falls back to `.leading`, and
        // every row then takes its own intrinsic width. Measured on this SDK:
        //
        //     let a: NSLayoutConstraint.Attribute = .width   // rawValue 7
        //     stack.alignment = a                           // reads back 0
        //                                                          ^ .leading
        //
        // There is no compiler diagnostic and no visual symptom other than a
        // card that is 337px wide in an 850px pane and sits right-aligned at
        // x=493. Six fixes went in before this was measured; every one looked
        // right and changed nothing. See `ViewTree.explainWidth`, which is
        // the thing that found it.
        //
        // So the width is a CONSTRAINT on the stack's children, and the
        // alignment stays leading, which is the right alignment for a
        // vertical list of short rows.
        stack.alignment = .leading
        stack.spacing = Gap.group
        stack.translatesAutoresizingMaskIntoConstraints = false
        stack.edgeInsets = NSEdgeInsets(top: Gap.plane, left: Gap.plane,
                                        bottom: Gap.plane, right: Gap.plane)
        stack.addArrangedSubview(titleField)
        stack.addArrangedSubview(descriptionField)
        stack.setCustomSpacing(Gap.plane, after: descriptionField)


        scroll.documentView = stack
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
            // A stack inside a scroll view needs a width constraint or it
            // collapses to its intrinsic width, which is the narrowest line in
            // it rather than the width of the pane.
            //
            // Activated HERE and not at the point of writing, because a
            // constraint needs a common ancestor the moment it activates and
            // the stack does not join the view hierarchy until the line above
            // assigns `documentView`. Setting it earlier throws.
            stack.widthAnchor.constraint(equalTo: scroll.widthAnchor),
        ])

        view = container
    }

    public func show(_ page: Page) {
        currentPage = page
        titleField.stringValue = page.title
        descriptionField.stringValue = page.schema?.description ?? ""
        descriptionField.isHidden = (page.schema?.description ?? "").isEmpty
        pageId.stringValue = page.id

        rebuild(for: page)
    }

    /// Draw the page's controls.
    ///
    /// Everything after the controls is cleared and the id is appended, so a
    /// page that is shown twice does not stack two copies of itself. That was
    /// a real class of bug in the React shell — `WizardControl` and the
    /// behaviour matrix both re-created their rows on a refresh token bump and
    /// neither tore the old ones down.
    private func rebuild(for page: Page) {
        while stack.arrangedSubviews.count > 3 {
            let last = stack.arrangedSubviews.last
            stack.removeArrangedSubview(last!)
            last?.removeFromSuperview()
        }

        let context = ControlContext(
            service: client,
            status: nil,
            logs: [],
            refreshToken: 0,
            pageId: page.id
        )

        let controls = page.schema?.controls ?? []
        if controls.isEmpty {
            stack.addArrangedSubview(
                EmptyStateView(
                    headline: "This page has nothing to show yet.",
                    detail: "\(page.id) declared no controls. That is the daemon's answer, not a failure to draw one."
                )
            )
        } else {
            for control in controls {
                let view = Renderers.shared.makeView(for: control, context: context)
                stack.addArrangedSubview(view)
                // Every row is the width of the pane less the stack's padding,
                // stated rather than negotiated: `NSStackView.alignment`
                // cannot express it on macOS (see the note on `alignment`
                // above), and a row that sizes to its own content is a card
                // the width of its longest label.
                //
                // The autoresizing flag goes first. A constraint on a view
                // that still has a frame-based layout is inert, and the row
                // keeps whatever width it measured for itself — which is how
                // the card stayed 337px through six fixes that each looked
                // right and changed nothing.
                view.translatesAutoresizingMaskIntoConstraints = false
                view.widthAnchor.constraint(
                    equalTo: stack.widthAnchor,
                    constant: -(Gap.plane * 2)
                ).isActive = true
            }
        }

        stack.addArrangedSubview(pageId)
    }
}

// MARK: - Empty and error states

/// A state that says what is wrong and what to do about it.
///
/// The rule this follows is that "No data" is a failure of the state, not a
/// description of the situation: a person reading it should learn the cause
/// and the one action that changes it. `page.id` is in the detail because a
/// page that declared nothing is a daemon-side fact somebody can act on, and
/// naming it is the difference between a report they can file and one they
/// cannot.
final class EmptyStateView: NSView {
    init(headline: String, detail: String) {
        super.init(frame: .zero)

        let headlineField = NSTextField(labelWithString: headline)
        headlineField.font = Typeface.bodyStrong
        headlineField.textColor = Palette.primaryInk
        headlineField.lineBreakMode = .byWordWrapping
        headlineField.maximumNumberOfLines = 0

        let detailField = NSTextField(labelWithString: detail)
        detailField.font = Typeface.caption
        detailField.textColor = Palette.secondaryInk
        detailField.lineBreakMode = .byWordWrapping
        detailField.maximumNumberOfLines = 0

        let column = NSStackView(views: [headlineField, detailField])
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

    override var isFlipped: Bool { true }

    @available(*, unavailable)
    required init?(coder: NSCoder) { fatalError("EmptyStateView is created in code") }
}
