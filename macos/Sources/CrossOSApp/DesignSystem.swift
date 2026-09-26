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
    public static var sidebarBackground: NSColor { .underPageBackgroundColor }

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
    /// Between two groups of rows.
    public static let group: CGFloat = 18
    /// Between the rail and the content plane.
    public static let plane: CGFloat = 20
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
    public static let control: CGFloat = 5
    /// A card or a group. The one generous value, on the one shape that earns
    /// it.
    public static let card: CGFloat = 8
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

    /// A card's title.
    public static var cardTitle: NSFont { .systemFont(ofSize: NSFont.systemFontSize + 2, weight: .semibold) }

    /// The page's title. `.largeTitle` is the system VARIANT, so it tracks the
    /// user's setting rather than being a number this file owns.
    public static var pageTitle: NSFont { .systemFont(ofSize: 22, weight: .semibold) }

    /// A description, a hint, a detail under a label.
    public static var caption: NSFont { .systemFont(ofSize: NSFont.smallSystemFontSize) }

    /// A cap above a group, and the one place uppercase is right: a section
    /// label is a label for a group, not a sentence, and the system has a
    /// face for it.
    public static var sectionCap: NSFont {
        .systemFont(ofSize: NSFont.smallSystemFontSize, weight: .semibold)
    }

    /// Machine text — a version, a daemon's error string, a log line.
    public static var mono: NSFont { .monospacedSystemFont(ofSize: NSFont.smallSystemFontSize, weight: .regular) }
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
