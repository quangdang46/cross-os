import Foundation

// The page schema.
//
// This is the whole UI contract, and it lives in Go — not in Swift, not in
// this package. A page is a `map[string]any` marshalled to JSON
// (app/backend/pages.go:60-72, `contrib`), handed to the shell as
// `Page.Schema`, and rendered by dispatching on each control's `kind`.
//
// That shape is deliberate and load-bearing. `app/backend/pages.go:30-32` says
// it: custom views are rejected upstream, and the declarative tier is the only
// thing a plugin may contribute. So a plugin CANNOT hand the shell a Swift
// struct or a JSX tree. It hands over data, and the shell draws it — which is
// why "a plugin shipping a page gets working UI with no shell change" is a
// property of the architecture rather than a promise (app/frontend/src/
// controls/index.tsx:57-60).
//
// The consequence for this port is the reason it is cheap: there is no
// per-page Swift code to write. A page is a list of controls, a control is a
// `kind` plus data, and the registry maps kinds to views. Twenty-eight kinds
// and three aliases cover every page the daemon can currently produce.

/// One settings page, as `app/backend/host.go:18-41` defines it.
public struct Page: Sendable, Equatable {
    /// `"<pluginID>.<contribID>"` — the contribution's own name, never
    /// something the shell invented. A page id is namespaced and dotted, and
    /// `rendererKinds()` in the React registry asserts that no registered KIND
    /// is dotted, which is how a registry that started naming screens instead
    /// of kinds would be caught.
    public let id: String
    public let title: String

    /// The nav section, from the contribution.
    public let group: String

    /// The mark the shell draws beside the section title, from the
    /// contribution. Empty means the contribution chose no mark.
    ///
    /// It lives in the contribution rather than in a table on the shell side
    /// because the section's identity is the daemon's to declare: a shell that
    /// hardcoded which word got which glyph would be a second place a section
    /// could be renamed without the picture following it
    /// (app/backend/pages.go:45-56). That is the argument for a SF Symbol
    /// lookup here, not an emoji map.
    public let symbol: String

    /// Position in the nav.
    public let order: Int

    /// Marks the page a fresh profile lands on. The nav needs this before it
    /// can choose an opening page, so the daemon reads it here rather than
    /// leaving the shell to guess from the id (host.go:29-30).
    public let firstRun: Bool

    public let visibility: String
    public let actions: [String]?

    /// The declarative body. Decoded by `Page.decodeSchema` rather than by
    /// `Codable`, because the shape is open — a control's fields depend on its
    /// `kind`, and there are 28 kinds.
    public let schema: PageSchema?
}

/// The decoded body of a page: a `type`, a `description`, and the controls.
///
/// `controls` is deliberately `[Control]` where `Control` is a `kind` plus an
/// untyped payload, NOT a 28-case enum with 28 typed payloads. The reason is in
/// the Go: pages are built with `map[string]any` and a control carries only
/// what its renderer reads. Typing all 28 payloads here would be 28 types to
/// keep in step with Go builders that are themselves `map[string]any` — a
/// translation bug waiting to happen, in exchange for checking nothing the
/// registry does not already check.
public struct PageSchema: Sendable, Equatable {
    public var type: String?
    public var description: String?
    public var controls: [Control]

    /// Fields that are not on a page's body but appear on a control — trial
    /// timeout notes, and so on. They exist, they are served, and nothing
    /// renders them yet; a schema is allowed to carry more than the shell
    /// currently draws, because the plugin that shipped it had no way to know.
    public var extra: [String: JSONValue]
}

/// One control on a page: a `kind` and whatever that kind reads.
public struct Control: Sendable, Equatable {
    public let kind: String
    public let id: String
    public let payload: [String: JSONValue]

    public init(kind: String, id: String, payload: [String: JSONValue] = [:]) {
        self.kind = kind
        self.id = id
        self.payload = payload
    }

    // The accessors the two milestone pages need. Every one of them returns a
    // default rather than throwing, because a page missing a field must draw
    // something — a page that fails to decode is a blank window with no clue
    // which side is at fault, which is the failure `UnsupportedControl` exists
    // to avoid (index.tsx:143-149).

    public var label: String { payload["label"]?.stringValue ?? "" }
    public var note: String { payload["note"]?.stringValue ?? "" }
    public var action: String { payload["action"]?.stringValue ?? "" }
    public var source: String { payload["source"]?.stringValue ?? "" }

    /// The rows a list-ish control names explicitly. `items` on the Home
    /// checklist is `["keyboard", "windows", "finder"]` — a subset of what
    /// `core.readiness` returns, in the order the page wants.
    public var items: [String] {
        (payload["items"]?.arrayValue ?? []).compactMap(\.stringValue)
    }

    /// A wizard's step names, in order.
    public var steps: [String] {
        (payload["steps"]?.arrayValue ?? []).compactMap(\.stringValue)
    }

    /// Other pages this one links to, by page id — `aboutLink`, `trialLink`
    /// on the wizard (app/backend/onboarding.go:29).
    public func link(_ key: String) -> String? {
        payload[key]?.stringValue
    }
}

// MARK: - Decoding

extension Page {
    /// Decode a page list from the daemon.
    ///
    /// The `id`/`title`/`group` keys are Go's exported field names — the
    /// binding generator emits them verbatim because it reflects over the
    /// struct, so PascalCase reaches the wire and this is not a typo to fix.
    /// The generated TypeScript carried exactly this hazard once, which is why
    /// `app/frontend/src/lib/service.ts:12-17` exists as a comment about it.
    public static func decodeList(_ value: JSONValue) throws -> [Page] {
        guard let rows = value.arrayValue else {
            throw CoreError.decode(method: "pages", underlying: DecodeShapeError.expectedArray)
        }
        return try rows.map { row in
            guard let object = row.objectValue else {
                throw CoreError.decode(method: "pages", underlying: DecodeShapeError.expectedObject)
            }
            return try decodePage(object)
        }
    }

    private static func decodePage(_ object: [String: JSONValue]) throws -> Page {
        func string(_ key: String) -> String { object[key]?.stringValue ?? "" }
        func int(_ key: String) -> Int { object[key]?.intValue ?? 0 }
        func bool(_ key: String) -> Bool { object[key]?.boolValue ?? false }

        let schema = object["Schema"].flatMap { $0.objectValue }.map(decodeSchema)

        return Page(
            id: string("ID"),
            title: string("Title"),
            group: string("Group"),
            symbol: string("Symbol"),
            order: int("Order"),
            firstRun: bool("FirstRun"),
            visibility: string("Visibility"),
            actions: (object["Actions"]?.arrayValue ?? []).compactMap(\.stringValue),
            schema: schema
        )
    }

    private static func decodeSchema(_ object: [String: JSONValue]) -> PageSchema {
        let known: Set<String> = ["type", "description", "controls"]
        return PageSchema(
            type: object["type"]?.stringValue,
            description: object["description"]?.stringValue,
            controls: (object["controls"]?.arrayValue ?? []).compactMap { row in
                guard let fields = row.objectValue,
                      let kind = fields["kind"]?.stringValue else { return nil }
                return Control(
                    kind: kind,
                    id: fields["id"]?.stringValue ?? "",
                    payload: fields
                )
            },
            extra: object.filter { !known.contains($0.key) }
        )
    }
}

public enum DecodeShapeError: Error, CustomStringConvertible {
    case expectedArray
    case expectedObject

    public var description: String {
        switch self {
        case .expectedArray: return "expected a JSON array"
        case .expectedObject: return "expected a JSON object"
        }
    }
}
