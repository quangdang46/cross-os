import AppKit
import CrossOSCore

/// A control that is a card: a heading, a body, and a place to put an error.
///
/// Most of the 28 kinds are this shape — a label, a note, and rows that arrive
/// from the daemon. The React shell had a `ControlFrame` component doing the
/// same thing in 263 lines with a `common.tsx` shared by every renderer; this
/// is the same abstraction with the shared part in the base class and the
/// shared strings in one place.
///
/// **The error is a row, not an alert.** `MatrixControl.tsx:15` says why: "A
/// disabled rule is shown as disabled, never as an error. Disabling a rule is
/// what the control is for; dressing it in the same red as a failed write would
/// teach users to ignore the row that actually matters." An error that shares
/// its visual weight with a state is an error people learn to skip — and the
/// row that actually matters is the one that would be skipped.
@MainActor
class CardControl: NSStackView {
    private let heading = NSTextField(labelWithString: "")
    private let subtitle = NSTextField(labelWithString: "")
    private let bodyStack = NSStackView()
    private var card: Card?
    private var cardColumn: NSStackView?

    init(control: Control, context: ControlContext) {
        super.init(frame: .zero)
        orientation = .vertical
        alignment = .leading
        distribution = .fill
        translatesAutoresizingMaskIntoConstraints = false

        // Humanised, for the same reason a section cap is: the daemon sends
        // `label: "matrix"` and a heading reading "matrix" is a schema value
        // leaking into the interface.
        heading.stringValue = Humanize.phrase(control.label.isEmpty ? control.id : control.label)
        // A section CAP, not a card title.
        //
        // It was `Typeface.cardTitle` in `Palette.primaryInk` — 15pt semibold
        // black — because it used to be a heading INSIDE the box, where a
        // heading that big was right. It now sits ABOVE the group, labelling
        // it, and at that weight and colour it is the loudest thing on the
        // page: on `core.about` "License" and "Credits" read as two more
        // headings beside "About", and the page lost the hierarchy that says
        // which one is the page and which two are groups.
        //
        // `sectionCap` is what System Settings uses — small, semibold, in the
        // secondary ink. The page title stays the loud thing on the page.
        heading.font = Typeface.sectionCap
        heading.textColor = Palette.secondaryInk
        heading.alignment = .left

        if control.note.isEmpty {
            subtitle.isHidden = true
        } else {
            subtitle.stringValue = control.note
            // Caption size, because this text now sits BELOW the group where
            // a footnote belongs. It was body size when it was a title INSIDE
            // the box, which made sense there; at body size under the group it
            // is the same size as the page's own description and the page
            // loses its hierarchy — on `core.activity` the footnote "Newest
            // last. Clear empties the list…" read as a second description
            // rather than as a note on the group above it.
            subtitle.font = Typeface.caption
            subtitle.textColor = Palette.secondaryInk
            subtitle.alignment = .left
            subtitle.lineBreakMode = .byWordWrapping
            // `labelWithString:` sets `usesSingleLineMode`, and with it set a
            // label ignores `byWordWrapping` and truncates instead.
            subtitle.usesSingleLineMode = false
            subtitle.maximumNumberOfLines = 0
            subtitle.setContentCompressionResistancePriority(.defaultLow, for: .horizontal)
        }

        bodyStack.orientation = .vertical
        bodyStack.alignment = .leading
        bodyStack.distribution = .fill
        bodyStack.spacing = Gap.row
        bodyStack.translatesAutoresizingMaskIntoConstraints = false
        bodyStack.setContentHuggingPriority(.required, for: .vertical)

        let card = Card()
        // The heading and the note are ABOVE and BELOW the group, not inside
        // it — see `SectionView`. A bold title inside a bordered box is a web
        // card; a small cap above the box and a caption below it is a macOS
        // section, and it is the single change that stopped every page reading
        // as a stack of panels.
        let column = NSStackView(views: [heading, card, subtitle])
        column.orientation = .vertical
        column.alignment = .leading
        column.distribution = .fill
        column.spacing = Gap.close
        column.translatesAutoresizingMaskIntoConstraints = false
        card.setContent(bodyStack)
        self.card = card
        self.cardColumn = column

        // The cap sits closer to the group it labels than to the group above
        // it, and the note is set apart from both — a note belongs to the group
        // above it and to nothing else.
        column.setCustomSpacing(Gap.close, after: heading)
        column.setCustomSpacing(Gap.close, after: card)
        heading.widthAnchor.constraint(equalTo: column.widthAnchor).isActive = true
        card.widthAnchor.constraint(equalTo: column.widthAnchor).isActive = true
        subtitle.widthAnchor.constraint(equalTo: column.widthAnchor).isActive = true

        // ARRANGED, and that word is the whole fix.
        //
        // `addSubview` on a view that is not a stack adds a subview: it is
        // positioned and it draws, and nothing measures it. The card was
        // correctly 0pt tall on every page that used this class — a table, a
        // checkbox, anything — and its content was laid out below that frame,
        // outside the clip view, so the page rendered with its title, its
        // description and a hole where the control goes. The audit could not
        // see it because the card's frame was 0 and its content was below it.
        addArrangedSubview(column)
        // Fill the pane. A settings card is the width of the pane it sits in.
        column.widthAnchor.constraint(equalTo: widthAnchor).isActive = true
    }

    @available(*, unavailable)
    required init?(coder: NSCoder) { fatalError("CardControl is created in code") }

    /// Put a view in the card, replacing whatever was there.
    ///
    /// **The view is added BEFORE the width constraint is activated**, and
    /// that order is not cosmetic. A constraint needs a common ancestor at
    /// the moment it activates, and the first version built the constraint
    /// then called `addArrangedSubview` — which threw
    /// `NSGenericException: ... they have no common ancestor` and took the
    /// process with it.
    ///
    /// It crashed on `core.profiles` and only there, because that is the one
    /// page whose load calls `showError`, and the error path is the one that
    /// replaces a body when the stack is already built. Every other page loads
    /// a body into an empty stack and got away with it.
    func replaceBody(with view: NSView) {
        bodyStack.arrangedSubviews.forEach {
            bodyStack.removeArrangedSubview($0)
            $0.removeFromSuperview()
        }
        view.translatesAutoresizingMaskIntoConstraints = false
        bodyStack.addArrangedSubview(view)
        // Now they share an ancestor, and the constraint holds the body to
        // the card's width — which is what makes a card fill its pane rather
        // than size itself to its longest label.
        bodyStack.arrangedSubviews.last?.widthAnchor.constraint(
            equalTo: bodyStack.widthAnchor
        ).isActive = true
    }

    /// Replace the body with a few lines of text.
    ///
    /// Four controls are exactly this — a heading and two or three sentences —
    /// and writing a stack view for each of them is four copies of the same
    /// eight lines. `typeface` is a parameter because the point of the four is
    /// that they are NOT the same size: a licence is a caption and a title is
    /// a card title, and a helper that forced one on all of them would be
    /// flattening the difference the four exist to express.
    func replaceBody(with text: [(String, NSFont)]) {
        let stack = NSStackView()
        stack.orientation = .vertical
        stack.alignment = .leading
        stack.spacing = Gap.tight
        for (line, font) in text {
            let field = NSTextField(labelWithString: line)
            field.font = font
            field.textColor = font == Typeface.caption ? Palette.secondaryInk : Palette.primaryInk
            field.lineBreakMode = .byWordWrapping
            field.maximumNumberOfLines = 0
            stack.addArrangedSubview(field)
        }
        replaceBody(with: stack)
    }

    /// Show a failure, in the card, in words that say what to do.
    ///
    /// The detail names the RPC because a person reading "could not load" with
    /// no method in it has nothing to search for, and the person who can fix
    /// it — whoever runs the daemon — needs the method name, not a mood.
    func showError(_ headline: String, _ detail: String) {
        replaceBody(with: ErrorView(headline: headline, detail: detail))
    }
}

/// A failure that says what failed and what to do about it.
///
/// The rule is that "No data" is a failure of the state and not a description
/// of the situation. A person reading it should learn the cause and the one
/// action that changes it.
final class ErrorView: NSStackView {
    init(headline: String, detail: String) {
        super.init(frame: .zero)
        orientation = .vertical
        alignment = .width
        distribution = .fill
        translatesAutoresizingMaskIntoConstraints = false

        let headlineField = NSTextField(labelWithString: headline)
        headlineField.font = Typeface.bodyStrong
        headlineField.textColor = Palette.danger
        headlineField.lineBreakMode = .byWordWrapping
        headlineField.maximumNumberOfLines = 0

        let detailField = NSTextField(labelWithString: Explain.daemon(detail))
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

    @available(*, unavailable)
    required init?(coder: NSCoder) { fatalError("ErrorView is created in code") }
}

// MARK: - Formatting

/// A chord, as macOS writes it.
///
/// The daemon stores the key with a leading modifier marker and no separator —
/// `^c`, `⌥⇧k` — and the React shell had a `formatChord` that turned it into
/// `⌃C` with a real superscript-capable glyph. Here it is `NSString` and the
/// system's own key symbols, so the chord is the same mark macOS draws on its
/// own menus rather than one this file maintains.
enum Chord {
    static func format(_ keys: String) -> String {
        guard !keys.isEmpty else { return "" }
        var out = ""
        var rest = Substring(keys)

        // Leading modifiers, in the order macOS shows them.
        let prefixOrder: [(Character, String)] = [
            ("^", "\u{2303}"),   // control
            ("\u{2325}", "\u{2325}"), // the right-hand variant, as sent
            ("~", "\u{2318}"),   // command
            ("$", "\u{2325}"),   // the other spelling of command
            ("!", "\u{21A7}"),   // option
            ("+", "\u{21E7}"),   // shift
        ]
        var matched = false
        for (marker, symbol) in prefixOrder where rest.first == marker {
            if symbol != rest.first!.description { out += symbol }
            matched = true
            rest = rest.dropFirst()
            if out.isEmpty { out = String(rest.prefix(1)).uppercased() }
        }
        if !matched { return keys }
        if out.isEmpty { out = String(rest.prefix(1)).uppercased() }
        return out
    }
}

/// A machine word, as a person reads it.
///
/// The daemon sends `toggleWindow`, `moveToSpace` and `kill`; a settings pane
/// says "Toggle window". The React shell had a `humanize` doing this with
/// string splitting. Same here, with a table for the words splitting gets wrong
/// — "AppMode" and "MRU" do not split on a case boundary.
enum Humanize {
    private static let phrases: [String: String] = [
        "toggleWindow": "Toggle window",
        "moveToSpace": "Move to space",
        "kill": "Quit the app",
        "launch": "Open the app",
        "switchSpace": "Switch space",
        "focus": "Bring to front",
        "fullscreen": "Enter full screen",
        "minimize": "Minimize",
        "maximize": "Zoom",
        "close": "Close the window",
        // The daemon's own action vocabulary, verbatim — these keys are
        // lowerCamel and were measured from `config.getMatrix`, not guessed:
        // `app.open`, `clipboard.copyPath`, `terminal.openAt`, `window.close`.
        // An earlier table spelled them `App.open` and matched nothing, which
        // is why the matrix kept printing "app.open" on the one page whose job
        // is saying what each action does.
        //
        // `app.open` and `terminal.openAt` are two DIFFERENT actions — one
        // opens whatever app is in front, the other opens a terminal at the
        // cursor — so they cannot share an entry.
        "app.open": "Open the front app",
        "app.close": "Close the app",
        "app.quit": "Quit the app",
        "app.focus": "Bring the app forward",
        "terminal.openAt": "Open Terminal here",
        "clipboard.copy": "Copy to clipboard",
        "clipboard.copyPath": "Copy the path",
        "clipboard.paste": "Paste from clipboard",
        "window.close": "Close the window",
        "window.minimize": "Minimize the window",
        "window.zoom": "Zoom the window",
        "space.switch": "Switch space",
        "finder.reveal": "Reveal in Finder",
        "finder.rename": "Rename in Finder",
    ]

    /// Humanise a daemon action verb.
    ///
    /// A thin name for `phrase`, used at the call sites that are reading an
    /// ACTION rather than a word. It says at the point of use that this string
    /// is the daemon's vocabulary and not a UI label — the reason it exists
    /// rather than a call to `phrase` at each site.
    ///
    /// Measured against `config.getMatrix`, the actions carry no argument:
    /// `app.open`, `clipboard.copyPath`, `terminal.openAt`, `window.close`. An
    /// earlier version of this split on a space to keep an argument it was
    /// written for and the daemon does not send.
    static func action(_ raw: String) -> String { phrase(raw) }

    static func phrase(_ word: String) -> String {
        if let known = phrases[word] { return known }
        guard !word.isEmpty else { return word }
        // camelCase to words, then capitalise the first. A word that is one
        // letter is left alone: "x" is an axis name here, not a word.
        //
        // The split must not ADD a space where the daemon already wrote one.
        // The matrix sends "Left Half" and "Open Window Switcher" in that Title
        // Case, and splitting on the capital produced "Left  half" and "Open
        // window  switcher" — a doubled gap on every sentence with no table
        // entry, which is most of the Action column.
        var out = ""
        for (index, character) in word.enumerated() {
            if character.isUppercase, index > 0, out.last != " " { out += " " }
            out += String(character).lowercased()
        }
        guard let first = out.first else { return out }
        return String(first).uppercased() + out.dropFirst()
    }
}

/// Turn the daemon's error text into something a reader can act on.
///
/// **A raw RPC error is developer output on a user-facing screen.** Measured on
/// `core.switcher`, the failure detail read:
///
///     core.windows: rpc code -32603: adapter: accessibility permission denied
///
/// Nobody reading a settings window knows what `-32603` is, and nobody can
/// act on `core.windows`. What they can act on is the permission, and where
/// to grant it — which is the whole point of saying it.
///
/// So the known failures are translated, the ACTION comes first, and the
/// original string is kept underneath it in the secondary ink for whoever is
/// going to file the bug. Nothing is swallowed: an unrecognised error passes
/// through unchanged, because inventing a friendly message for an error nobody
/// has described is how a real problem gets hidden behind a plausible one.
enum Explain {
    static func daemon(_ raw: String) -> String {
        let lower = raw.lowercased()

        if lower.contains("accessibility permission denied") {
            return """
            CrossOS needs Accessibility permission to see your windows.
            System Settings → Privacy & Security → Accessibility → add \
            CrossOS, then turn it on and reopen this window.

            Reported by the daemon as: \(raw)
            """
        }
        if lower.contains("input-monitoring") || lower.contains("input monitoring") {
            return """
            CrossOS needs Input Monitoring permission to read the keyboard.
            System Settings → Privacy & Security → Input Monitoring → enable \
            CrossOS, then reopen this window.

            Reported by the daemon as: \(raw)
            """
        }
        if lower.contains("no such method") {
            return """
            This build of CrossOS asks for something the daemon does not have.
            That is a bug in the app, not something to fix on this machine.

            Reported by the daemon as: \(raw)
            """
        }
        if lower.contains("socket") || lower.contains("connection refused") {
            return """
            The daemon is not answering. Start it with ./scripts/run.sh \
            --no-open, then reopen this window.

            Reported by the daemon as: \(raw)
            """
        }
        // Nothing matched, so nothing is claimed. A friendly message invented
        // for an error nobody has described is worse than the error.
        return raw
    }
}
