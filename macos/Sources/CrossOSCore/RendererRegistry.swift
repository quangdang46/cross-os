import Foundation

// The kind registry.
//
// This is a transcription of `app/frontend/src/controls/index.tsx:73-149` and
// it carries three properties that are load-bearing, not incidental. Each has
// a name here so a well-meaning edit cannot quietly drop it.

/// A view for one control.
///
/// In Swift this is a closure over the control, its context, and the view it
/// should build. AppKit's `NSView` is the product, so a renderer is a factory
/// rather than a class hierarchy — which is what makes an unknown kind a
/// one-line answer instead of a subclass declaration.
public typealias ControlRenderer = @Sendable (Control, ControlContext) -> DrawnControl

/// The result of drawing a control.
///
/// A protocol rather than `NSView` so the registry can be exercised without a
/// window, and so a renderer that has nothing to draw says so explicitly
/// instead of returning a zero-sized view. `nil` is the "nothing to show"
/// answer and is distinct from a control that drew something empty.
public protocol DrawnControl {}

public extension DrawnControl {
    var isEmpty: Bool { self is NothingDrawn }
}

/// A control that drew nothing. The default for a kind whose data is missing.
public struct NothingDrawn: DrawnControl {
    public let reason: String
    public init(reason: String = "") { self.reason = reason }
}

/// What a control can reach.
///
/// The mirror of `ControlContext` (index.tsx:17-31) and the reason a control
/// is a function of what the daemon said and never of module-level state. The
/// React comment at :13-15 says it: "Everything a control can reach — the
/// service, the status, the refresh cadence, the note sink — arrives in
/// ControlContext."
public struct ControlContext: Sendable {
    public let service: any CoreClient
    public let status: DaemonStatus?
    public let logs: [String]

    /// Bumped on every refresh; a control re-reads when it changes.
    ///
    /// This is a 5s poll, not a subscription. There is no push anywhere in the
    /// protocol — `grep EventsEmit` over `backend/` and `core/` finds nothing,
    /// and the daemon's only listener is the Unix socket
    /// (`core/cmd/crossos/main.go:1005`). The React shell polls every 5s
    /// (`App.tsx:385-389`) and threads the result through this value, and
    /// `useResource.ts:5-9` makes it a RULE: a control loads on mount and when
    /// this changes, and at no other time. Ports inherit the rule, not just the
    /// value.
    public let refreshToken: Int

    /// Show a transient note. The React `note` sink.
    public let note: @Sendable (String) -> Void

    /// Re-read now, out of cadence.
    public let refresh: @Sendable () -> Void

    /// The page this control sits on. A control that assembles other controls
    /// (`SchemaFormControl`, `PluginListControl`) needs to know which page it
    /// is drawing for.
    public let pageId: String

    public init(
        service: any CoreClient,
        status: DaemonStatus? = nil,
        logs: [String] = [],
        refreshToken: Int = 0,
        note: @escaping @Sendable (String) -> Void = { _ in },
        refresh: @escaping @Sendable () -> Void = {},
        pageId: String = ""
    ) {
        self.service = service
        self.status = status
        self.logs = logs
        self.refreshToken = refreshToken
        self.note = note
        self.refresh = refresh
        self.pageId = pageId
    }
}

// MARK: - The registry

/// The kinds, and what draws them.
///
/// **A dictionary, not a switch, and the same reason the React one is a Map.**
/// `index.tsx:65-68`: a plain object lookup would answer "constructor" or
/// "toString" with something inherited, and `renderControl` would then try to
/// draw it. A Swift `[String: ControlRenderer]` has the same hazard, so the
/// lookup goes through here and returns nil for a key that is not a kind.
public struct RendererRegistry: Sendable {
    private var renderers: [String: ControlRenderer]

    /// Kinds no page declares, kept apart from the map on purpose — see
    /// `aliases` below.
    private var aliases: [String: ControlRenderer]

    public init() {
        renderers = [:]
        aliases = [:]
    }

    public mutating func register(_ kind: String, _ renderer: @escaping ControlRenderer) {
        renderers[kind] = renderer
    }

    /// A retired spelling, mapping to the renderer that replaced it.
    ///
    /// These are kept OUT of `renderers` rather than added as more entries,
    /// and the React comment explains why (index.tsx:114-121): a key in the
    /// main map that no page declares is a mistake the registry cannot express,
    /// and the Go coverage guard (`TestEveryRegisteredKindIsReachable`) has to
    /// be able to tell a real kind from an alias. A guard that cannot is a
    /// guard that gets switched off, and a gate that gets switched off is not a
    /// gate.
    public mutating func alias(_ retired: String, to kind: String) {
        guard let renderer = renderers[kind] else {
            assertionFailure("alias \"\(retired)\" points at unregistered kind \"\(kind)\"")
            return
        }
        aliases[retired] = renderer
    }

    /// Every kind, aliases included, in served order. The React export exists
    /// so a test can assert no key here is a page id — page ids in that codebase
    /// are namespaced and dotted, so a dotted key is a registry that had started
    /// naming screens instead of kinds.
    public var kinds: [String] {
        Array(renderers.keys) + Array(aliases.keys)
    }

    public func contains(_ kind: String) -> Bool {
        renderers[kind] != nil || aliases[kind] != nil
    }

    /// Draw a control.
    ///
    /// An unknown kind is NAMED, not skipped. Rendering nothing leaves a hole
    /// in the page and no way to tell whether the page or the shell is at
    /// fault; naming it makes the gap self-describing (index.tsx:143-149). The
    /// same sentence is why the failure is a value and not a thrown error.
    public func render(_ control: Control, _ context: ControlContext) -> DrawnControl {
        guard let renderer = renderers[control.kind] ?? aliases[control.kind] else {
            return UnsupportedControlView(kind: control.kind, id: control.id)
        }
        return renderer(control, context)
    }
}

/// What an unknown kind draws.
///
/// Not a blank. A row that says which kind it could not draw, so a plugin
/// shipping a control the shell has not heard of produces a visible, explicable
/// gap rather than a hole in the page.
public struct UnsupportedControlView: DrawnControl {
    public let kind: String
    public let id: String

    public init(kind: String, id: String) {
        self.kind = kind
        self.id = id
    }

    public var message: String {
        id.isEmpty
            ? "This page uses a control this version of CrossOS cannot draw: \(kind)."
            : "\(id) — this page uses a control this version of CrossOS cannot draw: \(kind)."
    }
}
