import AppKit

// The design system, and what it is made of.
//
// There is no stylesheet here, and that is the whole difference from the
// React shell. Every value below is a NATIVE one: a semantic colour resolved
// by AppKit, a control AppKit draws, a metric macOS chose. The React shell had
// 2,646 lines of CSS whose job was to make a webview impersonate this, and it
// spent a session losing four classes of bug to the effort — a step label that
// broke one character per line, a 1.00:1 control border, a 220px gutter full
// of void, a tile drawn outside its own grid.
//
// Those are not bugs that are cheaper to fix here. They are bugs that have no
// representation: `NSTextField` coerces, `NSSwitch` draws its own knob with
// its own shadow, `NSTableView` computes row height. There is no
// `overflow-wrap: anywhere` to get wrong because there is no text layout to do
// it.
//
// So the rule for this file is short: **reach for the system first, and write
// a number only where the system has no answer.** Every constant here has a
// comment saying which of the two it is.

// MARK: - Colour

/// A semantic colour, resolved by AppKit.
///
/// Not a hex value. macOS light and dark are different palettes rather than
/// one palette inverted, and the React shell learned that the hard way: it
/// bridged `--line-strong` to `ButtonBorder`, WebKit resolves that to
/// `rgb(255,255,255)`, and every control had a white border on a white card at
/// 1.00:1 where WCAG 1.4.11 asks 3:1. A dynamic colour is the fix and it is
/// also the whole fix — AppKit picks the right value for the current
/// appearance, and there is no bridge to get wrong.
public enum Palette {
    /// The window's own background.
    public static var windowBackground: NSColor { .windowBackgroundColor }

    /// A card or a group sitting on that background.
    public static var cardBackground: NSColor { .controlBackgroundColor }

    /// The nav rail — a step behind the content plane rather than beside it.
    ///
    /// The WINDOW background, not `underPageBackgroundColor`.
    ///
    /// `underPageBackgroundColor` is what an old-style sidebar is painted with
    /// and it is heavy: measured on this host it resolves to `(150, 150, 150)`
    /// in the light appearance, against a `(255, 255, 255)` content pane. That
    /// is a 40% grey slab down 45% of the window, and it is the single ugliest
    /// thing in the app — a colour so far from the content plane that the rail
    /// stops being "behind" and becomes a second panel arguing with the first.
    ///
    /// System Settings since Ventura does NOT paint its rail that colour. It
    /// uses the window background and separates the two panes with the split
    /// divider, so the rail reads as part of the same surface at a glance and
    /// separates only where you look. That is one colour instead of two, it is
    /// what the system draws, and it follows the user's appearance in both
    /// directions for free.
    public static var sidebarBackground: NSColor { .windowBackgroundColor }

    /// The rail's selected row, as the system actually draws it.
    ///
    /// **Not the accent.** AppKit's `.regular` selection highlight tints the
    /// row with the user's accent at a wash, which on this host resolves to a
    /// neutral grey — measured on the dark render as `(70, 70, 70)` with a
    /// white label, 9.44:1.
    ///
    /// It is a token because the audit needs the surface the app really puts
    /// behind text, and using `controlAccentColor` for that measured a surface
    /// nothing draws: `Palette.accent` appeared nowhere in the app outside the
    /// audit's own list.
    public static var selectionSurface: NSColor {
        NSColor.selectedContentBackgroundColor.blended(withFraction: 0.65,
                                                       of: .windowBackgroundColor) ?? .windowBackgroundColor
    }

    /// A row at rest, before hover. The hover colour is `alternatingContentBackgroundColors`
    ///'s neighbour, not a hand-mixed translucent black: the system knows what
    /// a hover looks like in both appearances and in every accent.
    public static var rowHover: NSColor { .alternatingContentBackgroundColors.first ?? .controlAccentColor }

    /// Primary ink. `labelColor`, not black — black is wrong in dark mode and
    /// the React shell's own baseline recorded WebKit resolving its declared
    /// `#1d1d1f` to absolute black anyway.
    public static var primaryInk: NSColor { .labelColor }

    /// A description, a hint, a value that is not the thing being read.
    public static var secondaryInk: NSColor { .secondaryLabelColor }

    /// A timestamp, a cap, a disabled label. The weakest ink macOS has, and it
    /// is the one WCAG 1.4.11's 3:1 applies to.
    public static var tertiaryInk: NSColor { .tertiaryLabelColor }

    /// A hairline between rows — decorative, so the weakest of the three line
    /// tiers and the one that is allowed to be faint.
    public static var hairline: NSColor { .separatorColor }

    /// The accent. The system's, so it follows the user's Appearance choice,
    /// which is the whole point: a settings app with its own blue is a settings
    /// app that disagrees with the rest of the machine.
    public static var accent: NSColor { .controlAccentColor }

    /// Text drawn ON the accent. `controlAccentColor` is not guaranteed to be
    /// dark enough for white text — a yellow accent would fail — so this asks
    /// the system for a colour that meets the contrast rather than assuming.
    ///
    /// The comparison is against the *resolved* accent, because that is the
    /// colour the text will actually sit on, and `controlAccentColor` follows
    /// the user's choice: a person who picked a yellow accent would otherwise
    /// get white text on yellow at 1.6:1.
    public static var onAccent: NSColor {
        let accent = NSColor.controlAccentColor.usingColorSpace(.sRGB) ?? .controlAccentColor
        return contrastRatio(.white, accent) >= 4.5 ? .white : .black
    }

    /// The WCAG relative-luminance contrast between two resolved colours.
    ///
    /// This is here rather than imported because it is the one measurement the
    /// React shell could not do without a webview: `--line-strong` had been
    /// bridged to `ButtonBorder`, WebKit resolved that to `rgb(255,255,255)`,
    /// and every control had a white border on a white card at 1.00:1 where
    /// 1.4.11 asks 3:1. It was found by reading resolved values out of a real
    /// WKWebView (docs/baseline/contrast.txt), which is a whole harness for one
    /// formula.
    static func contrastRatio(_ a: NSColor, _ b: NSColor) -> Double {
        func luminance(_ color: NSColor) -> Double {
            guard let rgb = color.usingColorSpace(.sRGB) else { return 0 }
            func channel(_ value: CGFloat) -> Double {
                let v = Double(value)
                return v <= 0.03928 ? v / 12.92 : pow((v + 0.055) / 1.055, 2.4)
            }
            return 0.2126 * channel(rgb.redComponent)
                 + 0.7152 * channel(rgb.greenComponent)
                 + 0.0722 * channel(rgb.blueComponent)
        }
        let la = luminance(a), lb = luminance(b)
        return (max(la, lb) + 0.05) / (min(la, lb) + 0.05)
    }

    /// A destructive action's ink. The system's red, not a hex, for the same
    /// reason as the accent: it follows the appearance.
    public static var danger: NSColor { .systemRed }

    /// A status colour meant to be READ, not to fill something.
    ///
    /// **The system status colours fail WCAG AA as text on a light surface.**
    /// Measured against `controlBackgroundColor`:
    ///
    ///     systemGreen    2.22:1   FAIL      (AA wants 4.5:1)
    ///     systemOrange   2.31:1   FAIL
    ///     systemRed      3.57:1   large text only
    ///
    /// macOS uses those three for FILLS, icons and badges, where the rule is
    /// non-text contrast at 3:1 and they pass it. This app used them for
    /// CAPTION-SIZED TEXT — the "Ready" and "Not ready" chips — and 2.22:1 is a
    /// reading nobody should have to do.
    ///
    /// The colour is darkened toward black in the light appearance and
    /// lightened toward white in the dark one, so it still follows the user's
    /// appearance instead of being another value this file owns.
    static func readable(_ colour: NSColor) -> NSColor {
        // `NSAppearance.currentDrawing()`, NOT `NSApp.effectiveAppearance`.
        //
        // The audit measures the same palette in both appearances by wrapping
        // each pass in `performAsCurrentDrawingAppearance`, and that changes
        // what is DRAWING — not what the app is configured as. Reading
        // `NSApp.effectiveAppearance` here produced a dark-appropriated ink
        // that was then measured against a light surface, and the audit
        // reported `okInk on card is 2.12:1` for a colour that measures 8.35:1
        // in the appearance it was built for.
        //
        // Caught by the audit, which is the first time here that a check written
        // to catch a defect caught one in the thing that fixed a defect.
        let drawing = NSAppearance.currentDrawing() ?? NSApp.effectiveAppearance
        let dark = drawing.bestMatch(from: [NSAppearance.Name.aqua, .darkAqua]) == .darkAqua
        return colour.usingColorSpace(.sRGB).map { rgb in
            let mix: CGFloat = dark ? 0.30 : 0.55
            let to: CGFloat = dark ? 1.0 : 0.0
            return NSColor(
                srgbRed: rgb.redComponent * (1 - mix) + to * mix,
                green: rgb.greenComponent * (1 - mix) + to * mix,
                blue: rgb.blueComponent * (1 - mix) + to * mix,
                alpha: rgb.alphaComponent
            )
        } ?? colour
    }

    /// A destructive action's INK. Readable at caption size, unlike `danger`.
    public static var dangerInk: NSColor { readable(.systemRed) }

    /// A healthy state's INK. Readable at caption size, unlike `ok`.
    public static var okInk: NSColor { readable(.systemGreen) }

    /// A warning's INK. Readable at caption size, unlike `warn`.
    public static var warnInk: NSColor { readable(.systemOrange) }

    /// A healthy state.
    public static var ok: NSColor { .systemGreen }

    /// A state that wants attention but is not an error.
    public static var warn: NSColor { .systemOrange }
}

// MARK: - Metrics

/// A gap, in points.
///
/// The React shell had a scale — 0/2/4/6/8/10/12/16/20/24/32/40/56 — and a
/// 15-test contract keeping to it, and that contract is what caught the
/// off-scale literals. The scale itself is worth keeping; what is gone is the
/// need to declare one, because AppKit's controls carry their own internal
/// metrics and the only spacing this app owns is BETWEEN controls.
public enum Gap {
    /// Nothing.
    public static let none: CGFloat = 0
    /// Between a label and its own detail line.
    public static let tight: CGFloat = 2
    /// Between related controls — a chip and the text it qualifies.
    public static let close: CGFloat = 6
    /// The default row gap, and the one a control's own padding adds to.
    public static let row: CGFloat = 10
    /// Between two groups of rows, and between the rail and the content
    /// plane.
    ///
    /// 20, and it was 18 — the audit's rhythm check found it, and the story
    /// is worth keeping: 18 was the only value in this file that was not on
    /// the scale, and it got there the way off-scale values always do, by
    /// being written once and then looked at enough times to feel right. The
    /// scale is 0/2/4/6/8/10/12/16/20/24/32/40/56; 18 is a half-step nobody
    /// named, which is the same failure the React shell had ninety-three
    /// times and caught with a fifteen-test contract.
    ///
    /// `group` and `plane` were separate names for one value once `group`
    /// moved to 20, and two names for one gap is two decisions pretending to
    /// be one. They are the same gap.
    public static let group: CGFloat = 20
    public static let plane: CGFloat = 20
}

// MARK: - Measure

/// The sizes a System Settings pane is built from.
///
/// This exists because "looks like macOS" was not something the design could
/// get right by accident. The three values here are the ones that separate a
/// macOS settings pane from a web page that happens to run in a window:
/// content does not run the full width of the window, a group is a filled
/// rounded rectangle with a 10pt radius rather than a bordered card with an
/// 8pt one, and the rows inside a group are separated by inset hairlines
/// rather than by each row carrying its own border.
public enum Measure {
    /// The widest a page's content is allowed to be.
    ///
    /// System Settings does not stretch its groups to the window edge; it
    /// holds them to a readable measure and leaves the rest of the pane empty.
    /// Without this a row of text runs the full 810pt of the pane, which is
    /// the single clearest tell of a web layout in a native window — the eye
    /// has no way to find the start of the next line.
    public static let contentMaxWidth: CGFloat = 640

    /// A group's own padding, and the inset its row separators start at.
    ///
    /// The separator is inset to the text's left edge rather than drawn from
    /// the group's edge, which is what makes the rows read as one list instead
    /// of as stacked boxes.
    public static let groupPadding: CGFloat = 14

    /// The height of a row in a group. System Settings' rows are 32pt with
    /// room for a second line; a settings pane with 18pt rows reads as a list
    /// of chips.
    public static let rowHeight: CGFloat = 32
}

/// A corner radius, in points.
///
/// Three values, and that is the whole scale. The React shell had seven radii
/// and a `--r-md` that meant three different things in three rules, which is
/// how a card ends up at 8px in one place and 10px in another and nobody can
/// say which is right.
public enum Radius {
    /// A control — a field, a button. Small, because a control with a large
    /// radius reads as a chip, not as something you put a value into.

    /// A group.
    ///
    /// **10, not 8.** System Settings' inset groups are 10pt; 8 is the web
    /// card radius, read off a CSS `border-radius` and carried through the
    /// port. The number is the most visible single thing that made these
    /// panes look like a browser, because the eye reads the shape before it
    /// reads anything on it.
    public static let card: CGFloat = 10
    /// A switch track and a chip. Full, because a capsule is what those two
    /// are and a rounded rectangle is neither.
    public static let capsule: CGFloat = 999
}

// MARK: - Type

/// The font roles, in the one place that names them.
///
/// `NSFont.systemFont` is the system face, so light/dark, dynamic type and the
/// user's font size all work without a token for any of them. The React shell
/// declared a 13px body and a 15px card title and a 20px page title and had to
/// be told about the user's text size; here the roles pick up whatever the
/// system has.
public enum Typeface {
    /// A row's label, a value. Regular.
    public static var body: NSFont { .systemFont(ofSize: NSFont.systemFontSize) }

    /// A row's label when it is the thing being acted on — a changed setting,
    /// a dangerous one. The system has a weight for this; it is not bold-by-hand.
    public static var bodyStrong: NSFont { .systemFont(ofSize: NSFont.systemFontSize, weight: .semibold) }

    /// A card's title. **Regular**, which is what System Settings uses for a
    /// group label.
    ///
    /// This was semibold, and with `pageTitle` also semibold the pane had two
    /// semibold weights competing in one hierarchy — a 26pt page title and a
    /// 15pt card title — so the eye had to read both before it could rank
    /// them. Regular leaves the page title as the ONLY bold-ish thing on
    /// screen, which is what makes it read as the page's name rather than as
    /// the largest of several headings. macOS's group labels are regular too;
    /// a group reads as a group because of its position and the space above
    /// it, not because its label is heavier than the rows under it.
    public static var cardTitle: NSFont { .systemFont(ofSize: NSFont.systemFontSize + 2) }

    /// The page's title.
    ///
    /// 26, not 22. System Settings' content title is the system's LARGE title
    /// — `.preferredFont(forTextStyle: .largeTitle)` — which is 26pt here, and
    /// a page title has to out-weigh a section cap by enough that the eye
    /// knows which is which before it reads either. At 22 against an 11pt cap
    /// the ratio was there but the title still read as a heading beside two
    /// headings rather than as the page's name.
    ///
    /// `.preferredFont(forTextStyle:)` is the VARIANT, so it tracks the user's
    /// text size rather than being a number this file owns.
    ///
    /// **Semibold, not bold**, and that is the weight System Settings uses for
    /// a content title. `convert(_:toHaveTrait: .boldFontMask)` was here, and
    /// bold at 26pt is the heaviest text the app draws — heavier than any
    /// control, heavier than any value, heavier than the card title it is
    /// meant to out-weigh. A page title should dominate by size and position,
    /// which it already does; making it the boldest thing on screen as well
    /// turns the top of the pane into a shout, and a shout at the top of a
    /// window is what a web form looks like.
    public static var pageTitle: NSFont {
        let large = NSFont.preferredFont(forTextStyle: .largeTitle)
        return NSFont.systemFont(ofSize: large.pointSize, weight: .semibold)
    }

    /// A description, a hint, a detail under a label.
    public static var caption: NSFont { .systemFont(ofSize: NSFont.smallSystemFontSize) }

    /// A cap above a group, and the one place uppercase is right: a section
    /// label is a label for a group, not a sentence, and the system has a
    /// face for it.
    ///
    /// **Regular weight**, where it was semibold. Together with `pageTitle`
    /// also semibold, the pane had two bold things in it — and the audit's new
    /// weight check put it plainly: every one of the fifteen pages drew two
    /// bold weights, the 26pt title and this 11pt cap, at a ratio of more than
    /// two to one. Two bold things means neither is the thing the eye lands on.
    ///
    /// What the cap should be doing is quietly: it labels a group the size and
    /// the space above it already identify, which is the same reason the cards
    /// lost their borders. macOS's group labels are regular 13pt, and the
    /// uppercase is a legible way to make a regular label read as a label
    /// without making it louder than the page.
    public static var sectionCap: NSFont {
        .systemFont(ofSize: NSFont.smallSystemFontSize)
    }

    /// Machine text — a version, a daemon's error string, a log line.
    public static var mono: NSFont { .monospacedSystemFont(ofSize: NSFont.smallSystemFontSize, weight: .regular) }
}

// MARK: - Section

/// One section of a settings page: a cap, a group, and a footnote.
///
/// **This is the shape System Settings draws, and getting it is most of what
/// separates a macOS pane from a web page.** The three parts are:
///
///   1. a CAP above the group — a small semibold label for the group, not a
///      title inside it. A heading inside the box makes the box a card with a
///      title, which is a web pattern; a heading above it makes it a labelled
///      group, which is a macOS one.
///   2. the GROUP — the filled rounded rectangle holding the rows.
///   3. a FOOTNOTE below it, in the secondary ink at caption size, explaining
///      the group rather than heading it.
///
/// Everything inside the group is a ROW, and rows are separated by hairlines
/// rather than each carrying its own border.
///
/// Putting the cap and the footnote outside the group is the change that
/// matters. Both were inside the box, so every control rendered as its own
/// titled card, and a page of them read as a stack of web panels rather than
/// as one settings window.
@MainActor
final class SectionView: NSStackView {
    /// The cap above the group.
    let cap = NSTextField(labelWithString: "")
    /// The footnote under it.
    let footnote = NSTextField(labelWithString: "")
    /// The group itself.
    let group = Card()
    /// The stack inside the group, where rows go.
    let body = NSStackView()

    init(title: String, note: String = "") {
        super.init(frame: .zero)
        orientation = .vertical
        alignment = .leading
        distribution = .fill
        translatesAutoresizingMaskIntoConstraints = false

        // The cap is HUMANISED, not printed raw. The daemon sends
        // `label: "panel"` and `label: "rules"`, and a section cap reading
        // "panel" is a schema value leaking into the interface — System
        // Settings' caps read "Siri & Spotlight", never "siriAndSpotlight".
        //
        // Uppercase is NOT the answer: macOS section caps are sentence case,
        // and a page of them set in all-caps reads as shouting and fights the
        // page title it sits under.
        cap.stringValue = Humanize.phrase(title)
        cap.font = Typeface.sectionCap
        cap.textColor = Palette.secondaryInk
        cap.isHidden = title.isEmpty

        body.orientation = .vertical
        body.alignment = .leading
        body.distribution = .fill
        body.spacing = 0
        body.translatesAutoresizingMaskIntoConstraints = false
        body.setContentHuggingPriority(.required, for: .vertical)

        footnote.stringValue = note
        footnote.font = Typeface.caption
        footnote.textColor = Palette.secondaryInk
        footnote.lineBreakMode = .byWordWrapping
        footnote.maximumNumberOfLines = 0
        footnote.setContentCompressionResistancePriority(.defaultLow, for: .horizontal)
        footnote.isHidden = note.isEmpty

        group.setContent(body)

        // Cap, group, footnote — and the SPACING is the design. The cap sits
        // closer to the group it labels than to the group above it, and the
        // footnote is set apart from both, because a footnote belongs to the
        // group above it and to nothing else.
        spacing = Gap.close

        // Arrange FIRST, space SECOND.
        //
        // `setCustomSpacing(_:after:)` throws
        // `NSInternalInconsistencyException` — "View is not (and has to be) in
        // stack view" — for a view that is not yet an arranged subview, and
        // there is no diagnostic saying which view it meant. The calls are
        // therefore after every `addArrangedSubview`, which is also the order
        // that reads correctly: the arrangement establishes what exists, and
        // the spacing says how far apart they are.
        if !title.isEmpty { addArrangedSubview(cap) }
        addArrangedSubview(group)
        // The footnote is ALWAYS arranged, and hidden when empty. Arranging it
        // only when there is text in it means a caller that fills it later —
        // the home page's tap warning arrives after the first paint — has no
        // view to put the text in.
        addArrangedSubview(footnote)

        // The footnote is set apart from the group: 2pt reads as attached to
        // it, and 10pt reads as belonging to the NEXT section.
        if !title.isEmpty { setCustomSpacing(Gap.close, after: cap) }
        setCustomSpacing(Gap.close, after: group)

        // The group is the width of this section. STATED, not negotiated: a
        // `.leading`-aligned stack gives each child the child's own width and
        // low compression resistance alone does not make it take the width it
        // is offered — measured, the two home groups came out 491pt and 341pt
        // inside 600pt sections, so two groups on one page ended two different
        // widths and the page read as two unrelated panels.
        NSLayoutConstraint.activate([
            group.widthAnchor.constraint(equalTo: widthAnchor),
        ])
    }

    @available(*, unavailable)
    required init?(coder: NSCoder) { fatalError("SectionView is created in code") }
}

// MARK: - Controls

/// A switch, drawn by the system.
///
/// `NSSwitch`, not a drawn one. The hand-rolled version that was here first
/// existed because `NSSwitch` on macOS has no off-state that matches what a
/// settings pane wants — and the fix it reached for, drawing the capsule and
/// the knob by hand, is exactly the work this port exists to stop doing. It
/// also drew its own focus ring, which meant answering a question that does
/// not need answering: macOS does not Tab to controls unless the person has
/// enabled system-wide keyboard navigation, so a bespoke ring is a bespoke
/// ring for a case where the system already draws one.
///
/// The system control is smaller than System Settings' switch. That is the
/// cost, and it is the right trade: a control the system maintains across
/// macOS releases, accessibility settings and future appearances beats a
/// closer pixel match this file has to keep correct forever.
public typealias Toggle = NSSwitch
