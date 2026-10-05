import AppKit
import CrossOSCore

// A design audit: the properties a pixel would tell you, measured.
//
// **`--shots` needs a display. This does not.** The renderer draws the view
// tree into a bitmap, and a process with no display has nothing to draw
// into — which is the same reason `screencapture` is refused here, and it is
// not a workaround for a missing permission, it is a different fact. So the
// things a screenshot is good for are checked arithmetically instead, and
// the four below are the ones that decide whether a settings window reads as
// native or as a web form:
//
// 1. **Contrast.** WCAG 1.4.11 asks 3:1 of a control's own boundary and
//    1.4.3 asks 4.5:1 of its text. The React shell shipped a **1.00:1** — a
//    white border on a white card, found only by reading resolved colours out
//    of a real WKWebView. It is a number here, and the number is wrong the
//    moment somebody's accent colour is not dark enough.
// 2. **Rhythm.** The React shell had a spacing scale and 93 literals off it.
//    Whether a page's gaps come from a scale is visible and is checkable.
// 3. **Fit.** A label that overflows, a row that overlaps its neighbour, a
//    hit target under 24x24 — each is a frame question, and the frames are
//    real after a layout pass.
// 4. **Duplication.** One accent, a small set of radii, one card shape. The
//    antislop rules are mostly "not too many of one thing", and "too many" is
//    countable.
//
// What this is NOT: a substitute for looking. It cannot tell you the page
// feels right, and a window can pass every check here and still be ugly. It
// tells you the six ways a settings pane is usually wrong, and those are the
// six this codebase has actually shipped.

enum Audit {
    struct Finding {
        var severity: Severity
        var page: String
        var what: String
        var detail: String
    }

    enum Severity: Int, Comparable, CustomStringConvertible {
        case note = 0
        /// A rule, broken. The thing is wrong and a person can see it.
        case fail = 1
        /// A rule, broken in a way nobody sees until it matters.
        case blind = 2

        static func < (a: Severity, b: Severity) -> Bool { a.rawValue < b.rawValue }

        var description: String {
            switch self {
            case .note: return "note"
            case .fail: return "FAIL"
            case .blind: return "BLIND"
            }
        }
    }

    // MARK: - 1. Contrast

    /// The WCAG relative-luminance contrast between two resolved colours.
    ///
    /// This is the formula the React shell could not run without a webview, and
    /// running it here is the whole reason `--line-strong` stopped being
    /// `ButtonBorder` — a value WebKit resolves to white, which on a white
    /// card is 1.00:1 where 1.4.11 asks 3:1.
    static func contrast(_ a: NSColor, _ b: NSColor) -> Double? {
        func luminance(_ color: NSColor) -> Double? {
            guard let rgb = color.usingColorSpace(.sRGB) else { return nil }
            func channel(_ value: CGFloat) -> Double {
                let v = Double(value)
                return v <= 0.04045 ? v / 12.92 : pow((v + 0.055) / 1.055, 2.4)
            }
            return 0.2126 * channel(rgb.redComponent)
                 + 0.7152 * channel(rgb.greenComponent)
                 + 0.0722 * channel(rgb.blueComponent)
        }
        guard let la = luminance(a), let lb = luminance(b) else { return nil }
        return (max(la, lb) + 0.05) / (min(la, lb) + 0.05)
    }

    /// Every ink on every surface the design system declares, measured.
    ///
    /// Not a sample: the whole matrix. A palette is a set of pairs, and a
    /// failure in one pair is a failure in the palette whether or not anybody
    /// scrolls to it.
    /// Contrast, measured in BOTH appearances.
    ///
    /// Only the light appearance used to be measured, and that is not a
    /// neutral default — every colour in this app is a DYNAMIC `NSColor`, so
    /// the dark palette is not a variant of one that has been seen. It is a
    /// palette nobody looked at, on an app whose entire reason for using
    /// system colours is to follow the user's Appearance setting.
    ///
    /// It is worth doing, because the two appearances do not agree. Measured
    /// here, in both:
    ///
    ///     light: primary/secondary/tertiary on accent   5.23:1  pass
    ///     dark:  primary/secondary/tertiary on accent   4.02:1  FAIL (AA wants 4.5:1)
    ///
    /// The sidebar's selected row is drawn by AppKit over the accent fill with
    /// the label left at full strength, so that label is the text this is
    /// about, and in dark appearance it is below AA on the control a person
    /// uses most.
    static func contrastMatrix() -> [Finding] {
        contrastMatrix(inAppearance: nil)
    }

    /// Contrast measured inside a specific appearance.
    ///
    /// The appearance is applied around the measurement rather than to the
    /// whole audit, so the per-page geometry that follows is still taken in
    /// whatever appearance the app is actually running in.
    static func contrastMatrix(inAppearance appearance: NSAppearance.Name?) -> [Finding] {
        guard let appearance else { return contrastInCurrentAppearance() }
        var findings: [Finding] = []
        NSAppearance(named: appearance)?.performAsCurrentDrawingAppearance {
            findings = contrastInCurrentAppearance()
        }
        return findings
    }

    private static func contrastInCurrentAppearance() -> [Finding] {
        // Ink, and the SURFACES IT ACTUALLY SITS ON.
        //
        // The greys go everywhere. The status inks go on the card and the
        // window and nowhere else, and saying so matters: the full cross
        // product reported `okInk on accent is 2.08:1`, which is true and is
        // a pair the app never draws. An audit that reports twenty
        // combinations of which three exist is noise, and noise is how people
        // learn to skip the line that matters.
        let everywhere: [(String, NSColor)] = [
            ("primary", Palette.primaryInk),
            ("secondary", Palette.secondaryInk),
            ("tertiary", Palette.tertiaryInk),
            ("onAccent", Palette.onAccent),
        ]
        let statusOnly: [(String, NSColor)] = [
            // **The status inks, which is what this list was missing.**
            //
            // It checked four inks and none was a colour the app paints TEXT
            // in. `danger`, `ok` and `warn` are what the "Ready" and "Not
            // ready" chips use, and measured against the card they were
            // 2.22:1, 2.31:1 and 3.57:1 — the first two failing WCAG AA.
            //
            // A contrast audit that only covers the greys cannot find the
            // colours that are actually unreadable, because unreadable
            // colour is almost never grey.
            ("okInk", Palette.okInk),
            ("warnInk", Palette.warnInk),
            ("dangerInk", Palette.dangerInk),
        ]
        let inks = everywhere + statusOnly
        let surfaces: [(String, NSColor)] = [
            ("window", Palette.windowBackground),
            ("sidebar", Palette.sidebarBackground),
            ("card", Palette.cardBackground),
            ("sunken", Palette.cardBackground), // a well sits on a card
            ("accent", Palette.accent),
        ]

        var findings: [Finding] = []
        // A status ink is skipped on a surface it never sits on, rather than
        // reported: see the comment where they are declared.
        func paints(_ inkName: String, on surfaceName: String) -> Bool {
            // `onAccent` is the ink of text ON the accent and nowhere else, so
            // pairing it with the card or the window asks what colour the sky
            // is over green. In the dark appearance it reported four failures
            // at 1.26:1 for combinations the app never draws.
            if inkName == "onAccent" { return surfaceName == "accent" }
            let isStatus = inkName.hasSuffix("Ink")
            guard isStatus else { return true }
            return surfaceName == "card" || surfaceName == "window"
        }

        for (inkName, ink) in inks {
            for (surfaceName, surface) in surfaces where paints(inkName, on: surfaceName) {
                guard let ratio = contrast(ink, surface) else {
                    findings.append(Finding(
                        severity: .blind, page: "contrast",
                        what: "\(inkName) on \(surfaceName) could not be measured",
                        detail: "the colour did not resolve to sRGB, so its ratio is unknown "
                              + "rather than good — a colour that cannot be measured is not a colour "
                              + "that can be relied on"
                    ))
                    continue
                }
                // 4.5:1 is 1.4.3 for text. 3:1 is 1.4.11 for a boundary, and
                // an ink used on a surface is text in every case here.
                let floor = 4.5
                if ratio < floor {
                    findings.append(Finding(
                        severity: .fail, page: "contrast",
                        what: String(format: "%@ on %@ is %.2f:1", inkName, surfaceName, ratio),
                        detail: String(
                            format: "WCAG 1.4.3 asks 4.5:1 for text and this is %.2f:1. "
                                  + "The React shell shipped a 1.00:1 here — a white border on a "
                                  + "white card — and found it only by reading resolved colours out "
                                  + "of a real WKWebView. This number is computed, so it cannot be "
                                  + "wrong by being unnoticed.",
                            ratio
                        )
                    ))
                } else if ratio < floor + 1 {
                    // Passing but close, which is the state a later change
                    // breaks. Worth saying while there is nothing to fix.
                    findings.append(Finding(
                        severity: .note, page: "contrast",
                        what: String(format: "%@ on %@ is %.2f:1 — passing, barely", inkName, surfaceName, ratio),
                        detail: "clears 4.5:1, and clears it by less than one. A palette edit that "
                              + "darkens the surface by one step puts this under the floor."
                    ))
                }
            }
        }
        return findings
    }

    // MARK: - 2. Rhythm

    /// Every gap and inset the app declares, checked against the scale.
    ///
    /// The React shell had a `--sp-*` scale and 93 literals off it, and the
    /// offenders were things like `10px` thirteen times and `6px` ten — a
    /// rhythm nobody had written down, which is why the two half-steps ended
    /// up named `--sp-4h` and `--sp-5h` rather than as the accident they were.
    ///
    /// The check is a set, not a formula: the scale here is four numbers and
    /// every value this app uses has to be one of them. A fifth value is a
    /// fifth decision nobody made.
    static func rhythmAudit() -> [Finding] {
        let scale: Set<CGFloat> = [0, 2, 4, 6, 8, 10, 12, 16, 20, 24, 32, 40, 56]
        let declared: [(String, CGFloat)] = [
            ("Gap.none", Gap.none),
            ("Gap.tight", Gap.tight),
            ("Gap.close", Gap.close),
            ("Gap.row", Gap.row),
            ("Gap.group", Gap.group),
            ("Gap.plane", Gap.plane),
        ]
        var findings: [Finding] = []
        for (name, value) in declared where !scale.contains(value) {
            findings.append(Finding(
                severity: .fail, page: "rhythm",
                what: "\(name) is \(value) and is not on the scale",
                detail: "the scale is \(scale.sorted().map(String.init).joined(separator: "/")) "
                      + "points. A value off it is a second spacing system, and two systems is how "
                      + "a form stops having a rhythm."
            ))
        }
        // Two DIFFERENT values less than 2pt apart read as one step with a
        // mistake in it. Two NAMES for the same value are not that — they are
        // one gap with two words for it, and `Gap.group` and `Gap.plane` are
        // both 20 on purpose.
        let distinct = Array(Set(declared.map(\.1))).sorted()
        for (a, b) in zip(distinct, distinct.dropFirst()) where b - a < 2 {
            findings.append(Finding(
                severity: .note, page: "rhythm",
                what: String(format: "%g and %g are adjacent on the scale", a, b),
                detail: "two steps less than 2pt apart read as one step with a mistake in it, and a "
                      + "person cannot tell which of the two a given gap is."
            ))
        }
        return findings
    }

    // MARK: - 3. Fit

    /// What the real view tree says about fit: overflow, overlap, truncation.
    ///
    /// Measured after a layout pass, so the frames are the ones that ship.
    static func fitAudit(root: NSView, page: String) -> [Finding] {
        var findings: [Finding] = []
        let leaves = collect(root)
        let size = root.bounds.size

        let scrolled = markScrolled(leaves)
        for view in leaves {
            let frame = view.frame

            // Outside the pane. A view at a negative origin is a view whose
            // content is scrolled or clipped, and one at the far edge is a
            // view wider than its container — both invisible and both wrong.
            if (frame.minX < -1 || frame.minY < -1
                || frame.maxX > size.width + 1 || frame.maxY > size.height + 1),
               !isAppKitInternal(view) {
                // A card is allowed to hang below a scrolling page: that is
                // what a scroll view is for. Only flag what is not inside one.
                guard !scrolled.contains(ObjectIdentifier(view)) else { continue }
                findings.append(Finding(
                    severity: .fail, page: page,
                    what: "\(type(of: view)) sits outside the pane at \(describe(frame))",
                    detail: "the pane is \(Int(size.width))x\(Int(size.height)) and this view's frame "
                          + "escapes it. A view outside its container is either clipped or overlapping "
                          + "something, and both look like a bug whether or not they are."
                ))
            }

            // A zero-height view with content is the failure this whole port
            // was for: the wizard was 883px because a button was measured at
            // zero, and a card that renders 1px tall is present, correctly
            // configured, and invisible.
            //
            // **Only a view that is a CONTAINER** — a box, a stack, a scroll
            // view, a card. A leaf with no height is a label with no text, or
            // a private AppKit view like `NSHardPocketView` that this file
            // has never heard of, and flagging those is the same cry-wolf the
            // hit-target check had: 180 findings on one page, every one of
            // them a view that was never going to be drawn and is not the
            // problem the rule was written for.
            // A container with no height is only a DEFECT when its children
            // have none either.
            //
            // The first version flagged the parent and was wrong every time:
            // `WizardView 810x0` with a `Card 810x162` inside it is a view
            // whose stack aligns leading, so the row takes its content's
            // height and the ROW box stays at zero while the card draws
            // normally. The rule was reporting the wrapper for a child that
            // is fine, and it did so on every control on every page — 180
            // findings, none of them the thing the rule was written for.
            //
            // The fix is to ask the descendants: a container is collapsed
            // when NOTHING inside it has a height either.
            if frame.height < 2, isContainer(view),
               !hasTallDescendant(view), !isAppKitInternal(view),
               !isEmptyContainer(view), !isOnlyFrameworkFurniture(view) {
                let descendants = collect(view).filter { node in
                    node !== view && node.frame.height > 2
                }.count
                findings.append(Finding(
                    severity: .fail, page: page,
                    what: "\(type(of: view)) (wrapping \(view.subviews.map { String(describing: type(of: $0)) }.joined(separator: ", "))) is \(frame.height)pt tall, \(descendants) tall descendant(s)",
                    detail: "it has content and no height. This is what a collapsed `NSBox` looks like, "
                          + "and it is the shape the wizard bug took: a control measured at zero, "
                          + "invisible, with no diagnostic."
                ))
            }

            // A hit target under the floor.
            if isInteractive(view), !isIndicatorInRow(view) {
                let size = frame.size
                if size.width < 20 || size.height < 20 {
                    findings.append(Finding(
                        severity: .fail, page: page,
                        what: "\(type(of: view)) is \(Int(size.width))x\(Int(size.height)) — under 20pt",
                        detail: "a control smaller than 20pt in either axis is hard to hit and harder "
                              + "to see the focus ring on. The macOS minimum is not a suggestion here: "
                              + "this is a window people click in a hurry."
                    ))
                }
            }
        }

        // Overlap, by walking the tree ONCE and comparing only the
        // interactive views against each other.
        //
        // The first version did this inside the per-leaf loop, which meant
        // `convert(_:to:)` — a coordinate conversion that walks the view
        // hierarchy — ran for every leaf against every one of its subviews.
        // On a page with a few hundred views that is tens of thousands of
        // hierarchy walks, and the audit took minutes instead of seconds. A
        // measurement nobody waits for is a measurement nobody runs, and a
        // check nobody runs is not a check.
        let interactive = leaves.filter(isInteractive)
        let root0 = root
        for (i, a) in interactive.enumerated() {
            for b in interactive[(i + 1)...] where a !== b {
                guard a.superview != nil, b.superview != nil else { continue }
                // Both frames converted to the WINDOW, which is the one
                // coordinate space every view agrees on. Comparing a.frame to
                // b.frame is wrong whenever they have different parents, and
                // comparing them in one of the two parents is wrong whenever
                // the third sits above both — which is the shape every
                // control here is, because a row is a card inside a stack
                // inside a scroll view.
                let root = root0
                let fa = a.convert(a.bounds, to: root)
                let fb = b.convert(b.bounds, to: root)
                let overlap = fa.intersection(fb)
                guard overlap.width > 2, overlap.height > 2 else { continue }
                // A control inside a cell is inside the cell, and that is
                // containment rather than a fight over the same pixels. The
                // real rule is about two controls that are SIBLINGS of the
                // same row, so one being inside the other rules it out.
                guard !isAncestor(a, of: b), !isAncestor(b, of: a) else { continue }
                    let commonParent = a.superview === b.superview
                findings.append(Finding(
                    severity: .fail, page: page,
                    what: "\(type(of: a)) and \(type(of: b)) overlap by \(Int(overlap.width))x\(Int(overlap.height)) [window A \(Audit.describe(fa)) | window B \(Audit.describe(fb))]",
                    detail: "two controls occupying the same space means the top one gets the clicks "
                          + "and the bottom one is a picture of a control."
                ))
            }
        }

        return findings
    }

    // MARK: - 4. Duplication

    /// The antislop rules, counted.
    ///
    /// "Not too many of one thing" is countable, and a palette with five hues
    /// or a radius scale with seven values is countable. This is the one
    /// section where the checklist itself is the specification.
    static func duplicationAudit(root: NSView) -> [Finding] {
        var findings: [Finding] = []
        let leaves = collect(root)

        // Radii in use. The rule is a small set, applied deliberately.
        var radii = Set<String>()
        for view in leaves {
            if let box = view as? NSBox { radii.insert("\(Int(box.cornerRadius))") }
            if let field = view as? NSTextField, field.isBezeled { radii.insert("field") }
        }
        if radii.count > 4 {
            findings.append(Finding(
                severity: .fail, page: "duplication",
                what: "\(radii.count) distinct corner radii in one page: \(radii.sorted().joined(separator: ", "))",
                detail: "radius becomes decoration instead of a hierarchy tool once there are more "
                      + "than a few. Three is the design system's whole scale."
            ))
        }

        // How many times one control type appears. Forty switches is a table,
        // and a table should say so.
        var counts: [String: Int] = [:]
        for view in leaves {
            counts[String(describing: type(of: view)), default: 0] += 1
        }
        let noisiest = counts.max { $0.value < $1.value }
        if let noisiest, noisiest.value > 25 {
            findings.append(Finding(
                severity: .note, page: "duplication",
                what: "\(noisiest.value) \(noisiest.key) views on one page",
                detail: "not a failure on its own — a 40-row matrix is a 40-row matrix. It is worth "
                      + "saying because it is where a page stops being a settings pane and becomes a "
                      + "table, and the difference is a scroll view."
            ))
        }
        return findings
    }

    // MARK: - Walking

    static func collect(_ view: NSView) -> [NSView] {
        var out: [NSView] = [view]
        for sub in view.subviews { out.append(contentsOf: collect(sub)) }
        return out
    }

    /// A view with something in it — a string, a subview, a filled layer.
    static func hasVisibleContent(_ view: NSView) -> Bool {
        if let field = view as? NSTextField, !field.stringValue.isEmpty { return true }
        if let button = view as? NSControl, button.isEnabled, !(button is NSBox) { return true }
        if let box = view as? NSBox, box.fillColor != nil { return true }
        if let layer = view.layer, layer.backgroundColor != nil { return true }
        if !view.subviews.isEmpty { return true }
        return false
    }

    /// Whether a view is a control a person clicks.
    ///
    /// **Not `NSTextField`, and the first version had it in the list.** A
    /// label is 16pt tall because a label is 16pt tall — that is the type
    /// size, not a hit target — and counting every text field as a control
    /// produced 180 findings on the first page, all of them "NSTextField is
    /// 539x16 — under 20pt", which is a measurement of a sentence. A gate
    /// that cries wolf on every label is a gate nobody reads, and the real
    /// findings underneath it would be lost.
    ///
    /// What counts is a view that takes a click: a button, a switch, a table,
    /// a search field. Those have a minimum because a finger has to find them.
    /// A control that is an INDICATOR inside a clickable row rather than the
    /// target itself.
    ///
    /// A checkbox draws at 16pt whatever the font, and it is not a hit-target
    /// problem: in a macOS settings row the ROW is what you click and the box
    /// is what you look at. `NSSwitch` is 31x19 for the same reason — it is
    /// sized to be seen, not to be aimed at.
    ///
    /// So a checkbox inside a row is exempt, and the exemption is narrow on
    /// purpose: an unlabelled checkbox alone on a page is still a 16pt target,
    /// and that one is still a finding.
    static func isIndicatorInRow(_ view: NSView) -> Bool {
        guard view is NSButton, let parent = view.superview else { return false }
        // A row, not a bare stack: the row is what carries the label and the
        // click, and a checkbox whose parent is a `RowView` is that row's
        // indicator rather than its only control.
        var node: NSView? = parent
        while let current = node {
            if current is RowView { return true }
            node = current.superview
        }
        return false
    }

    static func isInteractive(_ view: NSView) -> Bool {
        if view is NSButton || view is NSSwitch || view is NSSearchField { return true }
        if view is NSTableView || view is NSOutlineView { return true }
        // A text field that is EDITABLE is a control; one that is a label is
        // not, and `isEditable` is the difference.
        if let field = view as? NSTextField, field.isEditable { return true }
        return false
    }

    /// Whether the view is inside a scroll view, computed ONCE per leaf by
    /// the caller rather than by walking the superview chain each time.
    ///
    /// The first version called this from the escape check for every leaf,
    /// which walks to the root — on a page with a few hundred views that is
    /// tens of thousands of superview hops, and the audit took minutes
    /// instead of seconds. A measurement you wait minutes for is one nobody
    /// runs, and a check nobody runs is not a check.
    /// Whether `outer` is somewhere above `inner` in the view tree.
    static func isAncestor(_ outer: NSView, of inner: NSView) -> Bool {
        var node = inner.superview
        while let current = node {
            if current === outer { return true }
            node = current.superview
        }
        return false
    }

    /// Whether anything below this view has a real height.
    static func hasTallDescendant(_ view: NSView) -> Bool {
        for sub in view.subviews where sub.frame.height >= 2 {
            return true
        }
        for sub in view.subviews where hasTallDescendant(sub) {
            return true
        }
        return false
    }

    /// Whether a view is AppKit's own rather than this app's.
    ///
    /// The names starting with `_` and AppKit's private classes
    /// (`NSHardPocketView` is a scroller's own pocket) belong to the framework
    /// and are not this file's to judge. Flagging them was 60 of the 90
    /// findings the first run produced, every one of them "NSHardPocketView is
    /// 0.0pt tall" on a view that has never had a height and never will —
    /// it is a marker, not a container.
    ///
    /// A check that spends two thirds of its output on the framework's
    /// internals is not auditing the app. This boundary is what the check is
    /// for.
    static func isAppKitInternal(_ view: NSView) -> Bool {
        let name = String(describing: type(of: view))
        if name.hasPrefix("_") { return true }
        // AppKit's private classes, none of which this file has a contract
        // with: `NSHardPocketView` is a scroller's pocket, and
        // `AdditionalDimmingView` is the scrim a scroll view draws while
        // rubber-banding. Both are 0pt by design and neither is ever a
        // control a person clicks.
        return name.hasSuffix("PocketView") || name.hasSuffix("DimmingView")
    }

    /// Whether a view is an empty container — nothing inside it, so a height
    /// of zero is the correct answer rather than a collapse.
    ///
    /// The first audit run reported "NSStackView (wrapping NSStackView) is
    /// 0.0pt tall" three times on the Windows page, and the truth is that
    /// those inner stacks hold nothing: `ZoneEditorView` adds its rows in an
    /// async continuation, and a stack that has not been filled has no height
    /// and is not wrong. Flagging it would be flagging the gap between two
    /// frames rather than a defect.
    static func isEmptyContainer(_ view: NSView) -> Bool {
        view.subviews.isEmpty
    }

    /// Whether everything inside a view belongs to the framework.
    ///
    /// The flagged view is the app's own `NSView` wrapping a scroll view's
    /// scrim, so `isAppKitInternal(view)` is false — the wrapper is ours — and
    /// the check has to look at what it WRAPS. A container holding nothing
    /// but framework furniture has no content of its own to collapse.
    static func isOnlyFrameworkFurniture(_ view: NSView) -> Bool {
        !view.subviews.isEmpty && view.subviews.allSatisfy(isAppKitInternal)
    }

    /// A view whose own size decides its content's size.
    ///
    /// A zero height is only a DEFECT on a container: a card that measures
    /// 1px is present, correctly configured and invisible, which is the
    /// shape the wizard bug took. A leaf with no height is a label with no
    /// text or a private AppKit view this file has never heard of, and
    /// counting those is the same cry-wolf the hit-target check had — 180
    /// findings on one page, none of them the problem the rule was written for.
    static func isContainer(_ view: NSView) -> Bool {
        if view is NSBox || view is NSStackView || view is NSScrollView
            || view is NSClipView || view is NSSplitView {
            return true
        }
        return !view.subviews.isEmpty
    }

    static func markScrolled(_ leaves: [NSView]) -> Set<ObjectIdentifier> {
        var scrolled = Set<ObjectIdentifier>()
        for view in leaves {
            var node = view.superview
            while let current = node {
                if current is NSScrollView {
                    scrolled.insert(ObjectIdentifier(view))
                    break
                }
                node = current.superview
            }
        }
        return scrolled
    }

    static func describe(_ frame: NSRect) -> String {
        String(
            format: "x=%d y=%d w=%d h=%d",
            Int(frame.minX), Int(frame.minY), Int(frame.width), Int(frame.height)
        )
    }
}
