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

        // Caption size, and it wraps.
        //
        // The description was body size — the same size as the text inside
        // every group on the page — so on `core.commands` a sentence about the
        // Finder menu was set larger than the Finder menu. It is a subtitle,
        // not content, and it should read as one.
        descriptionField.font = Typeface.caption
        descriptionField.textColor = Palette.secondaryInk
        descriptionField.alignment = .left
        descriptionField.lineBreakMode = .byWordWrapping
        // `labelWithString:` sets `usesSingleLineMode`, and with it set a label
        // ignores `byWordWrapping` and truncates instead.
        descriptionField.usesSingleLineMode = false
        descriptionField.maximumNumberOfLines = 0

        // The page's own id, at the foot, and **only when it is asked for.**
        //
        // The reasoning for it existing at all still holds: a person reporting
        // a problem says "the About page is wrong" and the id is what makes
        // that reproducible. But `core.home` at the bottom of every page in
        // monospace is a developer string on every screen a user ever sees,
        // and it is the single thing that makes the app read as a diagnostic
        // tool rather than as a Mac app.
        //
        // `--show-ids` is how someone reports a bug without it costing every
        // other person the noise.
        pageId.font = Typeface.mono
        pageId.textColor = Palette.tertiaryInk
        pageId.isHidden = !CommandLine.arguments.contains("--show-ids")

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
            // LEFT, not centred.
            //
            // Centring the measure was right while every group drew a box —
            // the box had to go somewhere in the middle. With the boxes gone it
            // is simply a column floating in the middle of the pane, and its
            // rows start a third of the way in with the whole left margin
            // empty. System Settings puts its content against the left edge of
            // the pane and lets the right side breathe; the width is a limit
            // on how long a line gets, not a column to centre.
            stack.leadingAnchor.constraint(equalTo: document.leadingAnchor),
            stack.trailingAnchor.constraint(
                lessThanOrEqualTo: document.trailingAnchor,
                constant: -(Gap.plane * 2)
            ),
        ])

        view = container
    }

    /// How tall the page's CONTENT is, ignoring the pane it is shown in.
    ///
    /// The window is sized to this, and it cannot use the page view's
    /// `fittingSize` because the document is deliberately held to AT LEAST the
    /// clip view's height — so the view's fitting height is the pane's height,
    /// and a window sized to it never shrinks. The stack is the thing whose
    /// height the document pads out, so the stack is what is measured.
    var contentFittingHeight: CGFloat {
        let fitting = stack.fittingSize
        return fitting.height > 0 ? fitting.height : view.fittingSize.height
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
    /// Navigate to another page the daemon named, by id.
    ///
    /// An id the daemon sends that no page answers is reported in the page's
    /// own note rather than silently ignored: a link that goes nowhere should
    /// say so, because the alternative is a button that looks like it worked.
    func follow(pageID: String) async {
        guard let pages = try? await client.pages() else { return }
        guard let target = pages.first(where: { $0.id == pageID }) else {
            onFollowPage?(nil)
            return
        }
        onFollowPage?(target)
    }

    /// Set by the window, which is what owns the rail and the page swap.
    /// `nil` means the daemon named a page that does not exist, and the
    /// window says so rather than leaving a button that appears to work.
    var onFollowPage: ((Page?) -> Void)?

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
            pageId: page.id,
            // A control that names another page can now send the reader
            // there. The daemon has been declaring cross-page links the whole
            // time — `core.onboard` sends `aboutLink` and `trialLink` — and
            // nothing could act on one.
            navigate: { [weak self] id in
                guard let self else { return }
                Task { @MainActor in
                    await self.follow(pageID: id)
                }
            }
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
            // The description and the first section are 40pt apart, and that
            // is the same gap two sections get.
            stack.setCustomSpacing(Gap.group + Gap.group, after: descriptionField)
            for (index, control) in controls.enumerated() {
                let view = Renderers.shared.makeView(for: control, context: context)
                stack.addArrangedSubview(view)
                // SECTIONS are further apart than things INSIDE one.
                //
                // The stack's spacing is a single number for everything, which
                // is why a page of five sections reads as one list of five
                // equal-weight things. macOS gives a group more air above it
                // than the rows inside it have, and the eye groups by that gap
                // rather than by a border — which is also why removing the
                // borders was not enough on its own.
                //
                // **Two BUTTONS in a row are ACTIONS, not sections.** The
                // Safety page's PANIC STOP, Re-enable and Reset Everything
                // are one group of three actions, and 40pt between them read
                // as three unrelated items: the gap between the actions was
                // LARGER than the gap between the sections around them, so
                // the page had no hierarchy at all — the rhythm check said
                // 2.32x, under its 2.5x floor, because the space between the
                // groups was smaller than the space inside one.
                //
                // The actions now sit 20pt apart — still twice the 2pt
                // between a button and the sentence that explains it — and
                // the sections around them keep their 40pt.
                if index < controls.count - 1 {
                    let next = controls[index + 1]
                    let bothButtons = control.kind == "button" && next.kind == "button"
                    // `after: view` — the view just added — and NOT an index
                    // into `arrangedSubviews`. The stack holds the title, the
                    // description and a hidden page-id label before the
                    // controls, so an index into the array is off by however
                    // many of those there are, and the spacing landed on the
                    // wrong view: the sections stayed 40pt apart while the
                    // code said 56.
                    stack.setCustomSpacing(
                        bothButtons ? Gap.group : Gap.group + Gap.group,
                        after: view
                    )
                }
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
                // A section cap that says what the title already said is
                // dropped here, where the title is known. Measured on
                // `core.profiles` and `core.observe`: the page said
                // "Profiles" and the section under it said "Profiles" again.
                if let card = view as? CardControl, card.repeatsTitle {
                    card.dropCap()
                }
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
    /// An optional ACTION, drawn under the sentence.
    ///
    /// An empty state that says what to do and offers no way to do it sends
    /// the reader hunting: the Switcher page's state says "or Accessibility
    /// access is not granted", and the daemon has a deep link that fixes
    /// exactly that — the one permission it can ask for. A sentence with a
    /// button under it is the difference between a dead end and a step.
    private let action: NSButton?

    init(headline: String, detail: String, action: NSButton? = nil) {
        self.action = action
        super.init(frame: .zero)
        orientation = .vertical
        // `.leading`, NOT `.width`. A `.width`-aligned stack distributes its
        // child on the cross axis, so an empty state came out CENTRED inside
        // its group — measured at x=148 in a 600pt row, which reads as a
        // deliberate centring rather than as a state that has nothing to say.
        // An empty state in a settings group belongs at the group's leading
        // edge, with the rest of the row empty to its right.
        alignment = .leading
        distribution = .fill
        translatesAutoresizingMaskIntoConstraints = false

        // An icon above the headline, so an empty state reads as a state
        // rather than as a sentence. Without it the only signal is text at
        // caption size — easy to miss in a page of controls. The icon is
        // tertiary ink: it is decoration, not a warning, and dressing it in
        // danger or warn would make every empty state look like a problem.
        let icon = NSImageView()
        if let image = NSImage(systemSymbolName: "tray", accessibilityDescription: "Empty") {
            icon.image = image
        }
        icon.contentTintColor = Palette.tertiaryInk
        icon.translatesAutoresizingMaskIntoConstraints = false
        // **The icon is GIVEN A SIZE.**
        //
        // An `NSImageView` with an SF Symbol and no frame measures the
        // symbol's own natural size, and "tray" is a shallow trapezoid: it
        // came out 30x20 in the render, a smudge the width of a thumbnail
        // that reads as a broken image rather than as an illustration. A
        // state wants to be seen at a glance, and 20pt of pale grey is not
        // seen at a glance.
        icon.widthAnchor.constraint(equalToConstant: 28).isActive = true
        icon.heightAnchor.constraint(equalToConstant: 28).isActive = true
        // The image SCALES to the frame. Without this the frame is 28pt and
        // the symbol is drawn at its own size inside it — "tray" is a shallow
        // trapezoid 13pt wide, and it came out as a smudge the width of a
        // thumbnail. An SF Symbol is a vector, so scaling it up is crisp.
        icon.imageScaling = .scaleProportionallyUpOrDown
        icon.setContentHuggingPriority(.required, for: .horizontal)
        icon.setContentHuggingPriority(.required, for: .vertical)
        icon.setContentCompressionResistancePriority(.required, for: .horizontal)

        let headlineField = NSTextField(labelWithString: headline)
        headlineField.font = Typeface.bodyStrong
        headlineField.textColor = Palette.primaryInk
        headlineField.alignment = .left
        headlineField.lineBreakMode = .byWordWrapping
        headlineField.maximumNumberOfLines = 0

        let detailField = NSTextField(labelWithString: detail)
        detailField.font = Typeface.caption
        detailField.textColor = Palette.secondaryInk
        detailField.alignment = .left
        detailField.lineBreakMode = .byWordWrapping
        detailField.maximumNumberOfLines = 0
        detailField.setContentCompressionResistancePriority(.defaultLow, for: .horizontal)

        // **`.gravityAreas`, `.center`**, which is AppKit's own way to say
        // "put this in the middle of whatever height I get".
        //
        // At the fitting height the area is the whole stack and nothing moves,
        // so a state sitting under a card title still sits directly under it.
        // Given extra height the text lands in the middle, which is where
        // macOS puts an empty state that has the space to itself — an empty
        // Mail list, an empty folder, an empty search.
        //
        // Doing it here rather than at each call site matters because empty
        // states are built three different ways — `replaceBody`, a bare
        // `addArrangedSubview`, and a column literal — and the two that do not
        // go through `replaceBody` were the two that stayed at the top.
        //
        // **Two things that look like they should work and do not**, both
        // measured rather than assumed: low-hugging spacer views do nothing,
        // because a plain `NSView` has no intrinsic content size for a
        // hugging priority to resist — both spacers stayed at 0pt inside a
        // 132pt state and the text stayed at its top edge.
        distribution = .gravityAreas
        addView(icon, in: .center)
        addView(headlineField, in: .center)
        addView(detailField, in: .center)
        // The action goes LAST, in the same area, so it sits under the
        // sentence rather than in a third of the state of its own.
        if let action {
            action.translatesAutoresizingMaskIntoConstraints = false
            addView(action, in: .center)
            setCustomSpacing(Gap.row, after: detailField)
        }
        // **The icon sits CLOSE to the sentence it introduces.**
        //
        // It was `Gap.group` (20pt), and an icon 20pt above its own headline
        // reads as a section heading rather than as the mark on a label: the
        // rhythm check measured the gap between the icon and the words under
        // it at 27pt — as wide as the space between two SECTIONS on the
        // same page, which is how a page with a hierarchy loses it. The
        // other gaps inside the state are 11pt, and the icon's is now one
        // of them.
        setCustomSpacing(Gap.row, after: icon)
        // **THE STATE SITS AT THE GROUP'S LEADING EDGE.**
        //
        // `.gravityAreas` splits the stack into thirds — leading, centre,
        // trailing — and a view added to the CENTRE area lands in the middle
        // third of the stack, horizontally as well as vertically. Measured on
        // `core.safety`: "No trial in flight." at x=206 in a 600pt card,
        // and the icon a further 13pt right of that, at x=597 on
        // `core.activity` where the state happened to be narrow.
        //
        // So the same empty state sat at the leading edge on one page and a
        // third of the way in on another, and which one it was depended on
        // how wide its container happened to be. The constraints say what the
        // layout was always supposed to say: the icon, the headline and the
        // sentence all start where the group starts. The VERTICAL centring
        // the gravity areas were for is untouched — that is the cross axis,
        // and these constraints are on the main one.
        NSLayoutConstraint.activate([
            icon.leadingAnchor.constraint(equalTo: leadingAnchor),
            headlineField.leadingAnchor.constraint(equalTo: leadingAnchor),
            detailField.leadingAnchor.constraint(equalTo: leadingAnchor),
        ])
        if let action {
            action.leadingAnchor.constraint(equalTo: leadingAnchor).isActive = true
        }
    }

    override var isFlipped: Bool { true }

    @available(*, unavailable)
    required init?(coder: NSCoder) { fatalError("EmptyStateView is created in code") }
}
