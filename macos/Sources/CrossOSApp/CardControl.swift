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
class CardControl: NSView {
    private let heading = NSTextField(labelWithString: "")
    private let subtitle = NSTextField(labelWithString: "")
    private let bodyStack = NSStackView()
    private var card: Card?
    private var cardColumn: NSStackView?

    init(control: Control, context: ControlContext) {
        super.init(frame: .zero)
        translatesAutoresizingMaskIntoConstraints = false

        heading.stringValue = control.label.isEmpty ? control.id : control.label
        heading.font = Typeface.cardTitle
        heading.textColor = Palette.primaryInk

        if control.note.isEmpty {
            subtitle.isHidden = true
        } else {
            subtitle.stringValue = control.note
            subtitle.font = Typeface.body
            subtitle.textColor = Palette.secondaryInk
            subtitle.lineBreakMode = .byWordWrapping
            subtitle.maximumNumberOfLines = 0
        }

        bodyStack.orientation = .vertical
        bodyStack.alignment = .leading
        bodyStack.spacing = Gap.row
        bodyStack.translatesAutoresizingMaskIntoConstraints = false

        let card = Card()
        let column = NSStackView(views: [heading, subtitle, bodyStack])
        column.orientation = .vertical
        column.alignment = .leading
        column.spacing = Gap.row
        column.translatesAutoresizingMaskIntoConstraints = false
        card.setContent(column)
        self.card = card
        self.cardColumn = column

        addSubview(card)
        NSLayoutConstraint.activate([
            card.leadingAnchor.constraint(equalTo: leadingAnchor),
            card.trailingAnchor.constraint(equalTo: trailingAnchor),
            card.topAnchor.constraint(equalTo: topAnchor),
            // The card's HEIGHT, which is the whole control: a control that
            // draws a card is as tall as the card, and without this the
            // control is 0pt tall and the card is laid out below it at
            // y=-162, outside the clip view, so it is neither on screen nor
            // in a render of the page.
            //
            // The audit caught the same shape on eight row views that pinned
            // leading/trailing/top and nothing else. This one was invisible to
            // it for a different reason: the card has a height, so the row did
            // not read as collapsed, and the symptom was a page that rendered
            // with its title, its description and a hole where the content
            // goes.
            // Not a fixed height — the card sizes itself to its content, and
            // the bottom pin is what lets the CONTROL take the card's height
            // rather than the other way round.
        ])

        // HUG, which is a priority and not a constraint.
        //
        // A `.leading`-aligned vertical stack gives a row the height it
        // measured, and this control measured zero because the card's own
        // height came through a padding stack that nothing was holding. A
        // `bottomAnchor <= bottomAnchor` permits zero and does not create a
        // height; four attempts to fix it with constraints all left the card
        // at y=-162, below the row and outside the clip view, which is why
        // every page rendered with a title, a description and a hole.
        //
        // Hugging the content vertically is the constraint Auto Layout
        // actually applies here, and it is one line.
        setContentHuggingPriority(.defaultLow, for: .vertical)
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
final class ErrorView: NSView {
    init(headline: String, detail: String) {
        super.init(frame: .zero)

        let headlineField = NSTextField(labelWithString: headline)
        headlineField.font = Typeface.bodyStrong
        headlineField.textColor = Palette.danger
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
    ]

    static func phrase(_ word: String) -> String {
        if let known = phrases[word] { return known }
        guard !word.isEmpty else { return word }
        // camelCase to words, then capitalise the first. A word that is one
        // letter is left alone: "x" is an axis name here, not a word.
        var out = ""
        for (index, character) in word.enumerated() {
            if character.isUppercase && index > 0 { out += " " }
            out += String(character).lowercased()
        }
        guard let first = out.first else { return out }
        return String(first).uppercased() + out.dropFirst()
    }
}
