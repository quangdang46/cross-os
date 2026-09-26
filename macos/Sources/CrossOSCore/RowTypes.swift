import Foundation

// The row types the pages draw, transcribed from the Go that serves them.
//
// Sources: `core/cmd/crossos/pagedata.go` for the ones the daemon builds, and
// `app/frontend/bindings/.../models.ts` for the shapes the generated bindings
// recorded. Where the Go carries a comment explaining WHY a field is shaped
// that way, it came across — those comments are the decision, and a field with
// no stated reason is one that gets refactored into a bug.
//
// The keys are `snake_case` here, not PascalCase as they are on `Page`. That
// split is not an accident: `Page` is served by a struct the binding generator
// reflected over, so it kept Go's exported field names, while these rows are
// built by hand with explicit `json:` tags, and those tags are the wire. The
// generated TypeScript carries both spellings, which is exactly the hazard
// `app/frontend/src/lib/service.ts:12-17` documents.

// MARK: - matrix

/// One behaviour-matrix row (`config.getMatrix`).
///
/// `contexts` is never null: a rule with no app-mode restriction renders an
/// empty list, which is the honest reading of "anywhere" rather than a missing
/// value (pagedata.go:47-49). Swift's `[String]` matches that — an absent array
/// and an empty one are different, and the daemon only ever sends the empty
/// one.
public struct MatrixRow: Codable, Sendable, Equatable {
    public var ruleID: String
    public var plugin: String
    public var action: String
    public var keys: String
    public var contexts: [String]
    public var enabled: Bool

    enum CodingKeys: String, CodingKey {
        case ruleID = "rule_id"
        case plugin
        case action
        case keys
        case contexts
        case enabled
    }

    public init(
        ruleID: String, plugin: String, action: String,
        keys: String, contexts: [String], enabled: Bool
    ) {
        self.ruleID = ruleID
        self.plugin = plugin
        self.action = action
        self.keys = keys
        self.contexts = contexts
        self.enabled = enabled
    }
}

/// One override (`config.getOverrides`) — what replaces a matrix row for one
/// app.
public struct OverrideRow: Codable, Sendable, Equatable {
    public var app: String
    public var ruleID: String
    public var action: String
    public var keys: String
    public var enabled: Bool

    enum CodingKeys: String, CodingKey {
        case app
        case ruleID = "rule_id"
        case action
        case keys
        case enabled
    }
}

// MARK: - conflicts

/// One conflict (`core.conflicts`).
///
/// `losers` and `rules` are nullable because a conflict with nothing to show
/// beside it is a state the daemon does not have — a conflict IS a winner and
/// at least one loser — so nil here means the field is absent, not empty.
public struct ConflictRow: Codable, Sendable, Equatable {
    public var keys: String
    public var winner: String
    public var losers: [String]?
    public var rules: [ConflictClaim]?
}

public struct ConflictClaim: Codable, Sendable, Equatable {
    public var ruleID: String
    public var plugin: String
    public var action: String
    public var keys: String
    public var enabled: Bool

    enum CodingKeys: String, CodingKey {
        case ruleID = "rule_id"
        case plugin
        case action
        case keys
        case enabled
    }
}

// MARK: - observe

/// What the recorder is ACTUALLY doing, read back from it
/// (`core.observeState`).
///
/// It is a read of the recorder and not an echo of the last write: a toggle
/// that can only be written reports the click it just received, which is the
/// belief rather than the state.
///
/// `mode` is the privacy level the recorder is keeping, and it is REPORTED
/// here rather than settable. A page that could set it would be a second,
/// quieter way to decide what a product that watches every keystroke keeps —
/// so the copy has to name what each mode buys, and the daemon keeps the
/// policy.
public struct ObserveStateRow: Codable, Sendable, Equatable {
    public var observe: Bool
    public var mode: String

    public init(observe: Bool, mode: String) {
        self.observe = observe
        self.mode = mode
    }
}

// MARK: - apps and profiles

/// One installed app (`core.apps`).
///
/// The keys are PascalCase, like `Page`, because this row is served by a
/// struct the binding generator reflected over. See the note at the top of this
/// file.
public struct AppRow: Codable, Sendable, Equatable {
    public var bundleID: String
    public var executable: String
    public var pid: Int
    public var displayName: String
    public var appMode: String
    public var category: String

    enum CodingKeys: String, CodingKey {
        case bundleID = "BundleID"
        case executable = "Executable"
        case pid = "PID"
        case displayName = "DisplayName"
        case appMode = "AppMode"
        case category = "Category"
    }
}

/// One profile (`core.profiles`).
public struct ProfileRow: Codable, Sendable, Equatable {
    public var id: String
    public var label: String
    public var description: String
    public var active: Bool
    public var capabilities: [ProfileCapabilityRow]?
}

/// One thing a profile can turn on.
///
/// `willEnable` and `alreadyOn` are counted ONCE per capability, not once per
/// item inside it — pagedata.go:935-943 says so, and a count doubled by nesting
/// is a count that disagrees with itself on two pages at once.
public struct ProfileCapabilityRow: Codable, Sendable, Equatable {
    public var id: String
    public var label: String
    public var description: String
    public var willEnable: Int
    public var alreadyOn: Int

    enum CodingKeys: String, CodingKey {
        case id
        case label
        case description
        case willEnable = "will_enable"
        case alreadyOn = "already_on"
    }
}
