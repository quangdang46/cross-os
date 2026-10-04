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
        // `.leading`, NOT `.width`.
        //
        // `.width` fixed the height and broke the width: a row became as wide
        // as its CONTENT (341pt, the longest line) and the stack then
        // distributed the rows across the pane, so every card sat at x=493 in
        // an 850pt pane — the right half of the screen.
        //
        // The height is fixed BELOW instead, per row, by an explicit width
        // constraint plus vertical hugging. That keeps `.leading`'s meaning
        // ("align to the leading edge, take the height of the first child")
        // and puts the fill where it belongs: on the row.
        //
        // A `.leading`-aligned vertical stack gives each row the height of its
        // FIRST child. Every row here is a control whose first child is
        // another `.leading` stack, and that runs down to a label that reports
        // the height of nothing — so every row measured 0pt, the page's own
        // stack was 176pt of title and description, and the cards were laid
        // out at y=-162: below their rows, outside the clip view, and
        // therefore neither on screen nor in a render of the page.
        //
        // `.width` makes the stack distribute children along the cross axis
        // and take its own height from the SUM of theirs, which is the thing
        // a vertical stack is for. Five fixes went into the card before this
        // one, and the card was innocent every time.
        stack.alignment = .leading
        stack.spacing = Gap.group
        stack.translatesAutoresizingMaskIntoConstraints = false
        stack.edgeInsets = NSEdgeInsets(top: Gap.plane, left: Gap.plane,
                                        bottom: Gap.plane, right: Gap.plane)
        stack.setContentHuggingPriority(.required, for: .vertical)

        // The stack HUGS its content vertically, and that is what stops the
        // page's slack from being handed to a card.
        //
        // The stack is held to at least the clip view's height so a short page
        // sits at the top, and every extra point of height goes to the one
        // arranged subview willing to absorb it. Measured with the default
        // priorities: the first card came out 810x400 — 400pt of white with
        // four lines of text in it — and the title, which is at the top of the
        // stack, was pushed off the top of the window entirely.
        //
        // Hugging at `.required` says "a card is as tall as what is in it", so
        // the slack has nowhere to go and collects at the bottom of the page
        // instead, which is where empty space belongs.
        stack.addArrangedSubview(titleField)
        stack.addArrangedSubview(descriptionField)
        stack.setCustomSpacing(Gap.plane, after: descriptionField)

        // The document view is `TopDownDocument`, not the stack. See
        // `TopDownDocument` for why a non-flipped stack hangs from the bottom
        // of the window, and why raising the stack's height instead made the
        // first card 500pt of empty white and pushed the title off the top.
        let document = TopDownDocument()
        document.translatesAutoresizingMaskIntoConstraints = false
        scroll.documentView = document
        // The scroller floats OVER the page and is absent when the page fits.
        //
        // `scrollerStyle` is AppKit's name for it — not `overlayScrollers`,
        // and not `NSScroller.style`; the compiler rejects both of those on
        // this SDK, and the header is the authority:
        // `NSScrollView.h:72` declares `@property NSScrollerStyle
        // scrollerStyle`.
        //
        // What it buys, measured before it: no 17pt gutter reserved on a page
        // whose content fits, and no track drawn down the full height of the
        // pane. `core.home` has 326pt of page in a 720pt window and was
        // carrying a grey stripe the whole way down beside it.
        scroll.hasVerticalScroller = true
        scroll.autohidesScrollers = true
        scroll.scrollerStyle = .overlay
        scroll.drawsBackground = false
        scroll.translatesAutoresizingMaskIntoConstraints = false

        container.addSubview(scroll)
        NSLayoutConstraint.activate([
            scroll.leadingAnchor.constraint(equalTo: container.leadingAnchor),
            scroll.trailingAnchor.constraint(equalTo: container.trailingAnchor),
            scroll.topAnchor.constraint(equalTo: container.topAnchor),
            scroll.bottomAnchor.constraint(equalTo: container.bottomAnchor),
            // A document view needs a width constraint or it collapses to its
            // content's intrinsic width, which is the narrowest line in it
            // rather than the width of the pane.
            //
            // Activated HERE and not at the point of writing, because a
            // constraint needs a common ancestor the moment it activates and
            // the document does not join the view hierarchy until the line
            // above assigns `documentView`. Setting it earlier throws.
            document.widthAnchor.constraint(equalTo: scroll.widthAnchor),
        ])
        document.hold(stack, width: document.widthAnchor, minimumHeight: scroll.heightAnchor)

        // The page's content is held to a READABLE measure and centred, which
        // is the most visible single difference between a macOS settings pane
        // and a web page in a window.
        //
        // System Settings does not run its groups to the window edge. At
        // 1100x720 with a 250pt sidebar the content pane is 850pt, and a group
        // stretched across all of it puts 810pt of unbroken text in front of
        // the reader: the eye locates the start of a line by its left edge,
        // and at that width there is nothing to locate it by.
        //
        // `centerX` rather than `leading`, because System Settings centres the
        // measure in the pane — which stays balanced when the sidebar is
        // dragged, and a settings pane is exactly the window whose sidebar a
        // person resizes.
        //
        // The measure is `defaultHigh` and the pane's width `required`: on a
        // narrow pane the content must still fill it rather than overflow, and
        // on a wide one the measure wins.
        let measure = stack.widthAnchor.constraint(equalToConstant: Measure.contentMaxWidth)
        measure.priority = .defaultHigh
        NSLayoutConstraint.activate([
            measure,
            stack.centerXAnchor.constraint(equalTo: document.centerXAnchor),
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
        // Everything that is not one of the three chrome views goes, and the
        // three are named rather than counted.
        //
        // A COUNT was wrong here and wrong in the way that costs an afternoon:
        // `pageId` is the third chrome view and it is added at the END of this
        // method, not in `loadView`, so "keep the first three" kept the
        // PREVIOUS page's first control and removed the page id instead. Two
        // pages rendered into one, and Safety showed Home's wizard above its
        // own three buttons. Naming the chrome is a claim about a thing; a
        // count is a claim about an arrangement, and the arrangement changed
        // when the page id moved.
        for view in stack.arrangedSubviews
        where view !== titleField && view !== descriptionField && view !== pageId {
            stack.removeArrangedSubview(view)
            view.removeFromSuperview()
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
                // Fill the pane, and hug the content vertically. Two
                // constraints, one axis each: `.leading` alignment already
                // keeps the row on the left, and the width is what fills.
                //
                // Hugging is REQUIRED here, and not `.defaultLow`. Low was
                // the fix for a row whose first child is another
                // leading-aligned stack reporting the height of nothing, and
                // it kept working long after the thing it was working around
                // stopped being true: with the page now held to the clip
                // view's height, a low-hugging row is the row that absorbs
                // every extra point. Measured, the first card was 810x400 —
                // 400pt of white around four lines of text.
                view.translatesAutoresizingMaskIntoConstraints = false
                view.widthAnchor.constraint(
                    equalTo: stack.widthAnchor,
                    constant: -(Gap.plane * 2)
                ).isActive = true
                view.setContentHuggingPriority(.required, for: .vertical)
            }
        }

        stack.addArrangedSubview(pageId)
    }
}

/// A scroll view's document view that starts at the TOP of its bounds.
///
/// `isFlipped` is get-only on `NSView`, so a stack cannot be told to flip, and
/// a non-flipped `NSStackView` lays its arranged subviews out from the BOTTOM.
/// The page therefore hung from the bottom of the window with a band of
/// nothing above the title — measured at 278pt on `core.home`.
///
/// This is the smallest thing that answers it: one view, flipped, that holds
/// the stack pinned to its top edge. The stack is unchanged and still a stack,
/// which is what the page's width and height arithmetic already relies on.
final class TopDownDocument: NSView {
    override var isFlipped: Bool { true }

    /// `init(frame:)` is restated rather than inherited, because marking
    /// `init(coder:)` unavailable stops the compiler synthesising it.
    override init(frame frameRect: NSRect) {
        super.init(frame: frameRect)
    }

    /// Hold `content` against the top edge and the full width, and be at
    /// least as tall as the clip view.
    ///
    /// **This view's height is the clip view's; the content's height is its
    /// own.** Getting that backwards is what made every page either blank or
    /// ballooned, and both were measured:
    ///
    ///   - The document left unconstrained measured 850x0 while the stack
    ///     inside it measured 442pt, and a 0-height view clips its own
    ///     content: every render came out blank white.
    ///   - Forcing the CONTENT to the clip view's height instead hands every
    ///     spare point to the first row willing to grow, and the first card
    ///     came out 810x400 — 400pt of white around four lines of text.
    ///
    /// So the document fills the pane (which is what puts a short page at the
    /// top, because this view is flipped) and the content hugs itself (which
    /// is what leaves the empty space BELOW the page instead of inside it).
    func hold(
        _ content: NSView,
        width: NSLayoutDimension,
        minimumHeight: NSLayoutDimension
    ) {
        content.translatesAutoresizingMaskIntoConstraints = false
        addSubview(content)
        NSLayoutConstraint.activate([
            content.topAnchor.constraint(equalTo: topAnchor),
            content.leadingAnchor.constraint(equalTo: leadingAnchor),
            content.widthAnchor.constraint(equalTo: width),
            heightAnchor.constraint(greaterThanOrEqualTo: minimumHeight),
        ])
    }

    @available(*, unavailable)
    required init?(coder: NSCoder) { fatalError("TopDownDocument is created in code") }
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
final class EmptyStateView: NSStackView {
    init(headline: String, detail: String) {
        super.init(frame: .zero)
        orientation = .vertical
        alignment = .width
        distribution = .fill
        translatesAutoresizingMaskIntoConstraints = false

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
            // The BOTTOM pin, and its absence is why three plugin rows
            // overlapped by 14pt each in the first audit run.
            //
            // A view pinned leading/trailing/top and nothing else has no
            // height: Auto Layout is free to give it zero and let the
            // content overflow, and the content does — the switch on row two
            // lands on top of the switch on row one. The audit named it as
            // "NSSwitch and NSSwitch overlap by 54x14 [window A x=306 y=-11 |
            // window B x=306 y=-21]", and the ten points between them were
            // the ten points each row claimed for itself and none of them
            // gave back.
            column.bottomAnchor.constraint(lessThanOrEqualTo: bottomAnchor),
        ])
    }

    override var isFlipped: Bool { true }

    @available(*, unavailable)
    required init?(coder: NSCoder) { fatalError("EmptyStateView is created in code") }
}
