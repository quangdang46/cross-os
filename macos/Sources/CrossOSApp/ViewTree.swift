import AppKit

/// The view tree, printed.
///
/// The reason this exists at all: the React shell's four worst defects were
/// invisible to its test suite and visible only by looking at it. jsdom does
/// not load a stylesheet, so a 220px gutter full of void, a step label broken
/// one character per line, a white control border on a white card at 1.00:1,
/// and a switcher tile drawn below its own grid were all green. They were found
/// by building a harness that measured the real engine — `preview/` and
/// `webkit-probe.swift` — and both of those are for a webview that is being
/// deleted.
///
/// AppKit is not that engine being deleted; it is the engine that ships. So
/// the harness goes away with the thing it existed to compensate for, and what
/// replaces it is a print of the tree, because a window existing is not
/// evidence that it drew anything and a passing test is not evidence either.
///
/// **What this is not.** It is not a snapshot test and it is not a pixel
/// comparison. It reports frames and text, so it catches "this row is 883px
/// tall in a 720px window" and "this label wrapped to four lines" — the
/// measurements that were the actual defects. It cannot catch "this grey is
/// wrong", because that is a judgement about a colour and a judgement belongs
/// to a person looking at the screen.

enum ViewTree {
    static func describe(_ view: NSView, indent: Int) {
        let pad = String(repeating: "  ", count: indent)
        let frame = view.frame
        let size = "\(round(frame.width))x\(round(frame.height))"
        let position = "(\(round(frame.minX)),\(round(frame.minY)))"

        // A text field prints its string, because that is what a person reads
        // and a frame alone does not say whether the text is there.
        if let field = view as? NSTextField {
            let text = field.stringValue
            let lines = max(1, field.maximumNumberOfLines == 0 ? 1 : field.maximumNumberOfLines)
            let shown = text.isEmpty ? "(empty)" : "\"\(text.prefix(60))\""
            print("\(pad)\(type(of: view)) \(size) \(position) \(shown) lines=\(lines)")
        } else {
            print("\(pad)\(type(of: view)) \(size) \(position)")
        }

        for subview in view.subviews {
            describe(subview, indent: indent + 1)
        }
    }

    private static func round(_ value: CGFloat) -> Int {
        Int(value.rounded())
    }
}

extension ViewTree {
    /// Print the constraints on one view and on everything above it, with
    /// priorities.
    ///
    /// The width problem this was written for cost six wrong fixes in a row,
    /// each of which looked right and changed nothing, because the symptom —
    /// "the card is 337px in an 850px pane" — does not say WHICH of the
    /// constraints in play is winning. The measurement is: find the view, walk
    /// up, print what each level is asking for. It turns "the width is wrong"
    /// into "the stack proposes 850 at priority 750, the row resists at 750,
    /// and the tie goes to the row".
    static func explainWidth(of target: NSView, in root: NSView) {
        var chain: [NSView] = []
        var node: NSView? = target
        while let current = node {
            chain.append(current)
            node = current.superview
        }
        for (level, view) in chain.enumerated().reversed() {
            print("  \(level == 0 ? "target" : "ancestor \(level)")  \(type(of: view)) frame=\(view.frame)")
            if let stack = view as? NSStackView {
                print("      alignment=\(stack.alignment) distribution=\(stack.distribution)")
            }
            for constraint in view.constraints {
                let first = (constraint.firstItem as? NSView).map { String(describing: type(of: $0)) } ?? "nil"
                let second = (constraint.secondItem as? NSView).map { String(describing: type(of: $0)) } ?? "nil"
                print("      \(constraint.relation.rawValue) priority=\(constraint.priority.rawValue) "
                    + "\(first).\(constraint.firstAttribute.rawValue) "
                    + "\(second).\(constraint.secondAttribute.rawValue)")
            }
        }
    }
}

extension NSView {
    /// The first descendant of a type, breadth-first. Used by `--explain-width`
    /// to find the card without the caller knowing the tree.
    func firstDescendant<T: NSView>(ofType type: T.Type) -> T? {
        var queue: [NSView] = subviews
        while let view = queue.first {
            queue.removeFirst()
            if let match = view as? T { return match }
            queue.append(contentsOf: view.subviews)
        }
        return nil
    }
}
