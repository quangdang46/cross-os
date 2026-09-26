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

// MARK: - zones

/// One zone (`config.getZones`) — a rectangle a window can be moved to.
///
/// The geometry is four numbers because the daemon sends four numbers, and a
/// `CGRect` here would be a conversion this repo owns on both sides of the
/// wire for no gain: the values arrive in the daemon's coordinate space and go
/// back in it.
public struct ZoneRow: Codable, Sendable, Equatable {
    public var id: String
    public var name: String
    public var x: Double
    public var y: Double
    public var w: Double
    public var h: Double

    enum CodingKeys: String, CodingKey {
        case id, name, x, y, w, h
    }

    public init(id: String, name: String, x: Double, y: Double, w: Double, h: Double) {
        self.id = id
        self.name = name
        self.x = x
        self.y = y
        self.w = w
        self.h = h
    }
}

// MARK: - user rules

/// One user rule (`config.getUserRules`).
///
/// `modifiers`, `app_modes` and `app_ids` are nullable because a rule that
/// applies to everything says nothing about apps, and an empty array would be
/// a claim that it applies to NO apps — which is a different rule. `chord` is
/// the rendered form of `key` plus `modifiers`, the same string the matrix
/// draws, so a recorded key and the rule claiming it read alike on one screen.
public struct UserRuleRow: Codable, Sendable, Equatable {
    public var id: String
    public var key: String
    public var modifiers: [String]?
    public var appModes: [String]?
    public var appIDs: [String]?
    public var deviceID: String
    public var capability: String
    public var parameters: [String: JSONValue]?
    public var emit: Bool
    public var chord: String
    public var action: String
    public var priority: Int
    public var specificity: Int
    public var scope: String

    enum CodingKeys: String, CodingKey {
        case id, key, modifiers
        case appModes = "app_modes"
        case appIDs = "app_ids"
        case deviceID = "device_id"
        case capability, parameters, emit, chord, action
        case priority, specificity, scope
    }
}

// MARK: - traces

/// One recorded decision, as columns (`core.traces`).
///
/// `core.eventLogs` flattens these same traces into `"winner action params=…"`
/// strings; the stages, the physical event, the focused app and the rules that
/// lost are all still in here, which is the whole reason the Activity page uses
/// this and not the log.
public struct TraceRow: Codable, Sendable, Equatable {
    public var at: String
    public var decision: String
    public var event: TraceEvent
    public var context: TraceContext
    public var winner: String
    public var losers: [String]?
    public var intent: String
    public var action: String

    enum CodingKeys: String, CodingKey {
        case at, decision, event, context, winner, losers, intent, action
    }
}

/// The physical input. `keyCode` is the Windows virtual-key code the recorder
/// sends; the chord is rendered the way the matrix renders a rule's binding.
public struct TraceEvent: Codable, Sendable, Equatable {
    public var keys: String
    public var source: String
    public var device: String?
    public var keyCode: Int

    enum CodingKeys: String, CodingKey {
        case keys, source, device
        case keyCode = "key_code"
    }
}

/// What had focus when the key arrived.
public struct TraceContext: Codable, Sendable, Equatable {
    public var appID: String
    public var appMode: String
    public var windowID: String?
    public var winClass: String?

    enum CodingKeys: String, CodingKey {
        case appID = "app_id"
        case appMode = "app_mode"
        case windowID = "window_id"
        case winClass = "win_class"
    }
}

// MARK: - file types and plugins

/// One row of the Explorer's file-type catalog (`core.fileTypes`).
///
/// Identity is `(ext, baseName)`, not the display name, and reorder needs a
/// COMPLETE permutation of ids — a partial list is a list the daemon cannot
/// apply and this repo would have to detect. `builtIn` is why a row cannot be
/// deleted rather than merely not restored.
public struct FileTypeRow: Codable, Sendable, Equatable {
    public var ext: String
    public var baseName: String
    public var displayName: String
    public var template: String
    public var enabled: Bool
    public var builtIn: Bool
    public var menuTitle: String

    enum CodingKeys: String, CodingKey {
        case ext
        case baseName = "baseName"
        case displayName = "displayName"
        case template, enabled
        case builtIn = "builtIn"
        case menuTitle = "menuTitle"
    }
}

/// One plugin's manifest facts (`core.pluginMeta`).
///
/// `reason` is present only when `loaded` is false, and it says why. A plugin
/// listed without a reason is a plugin with nothing for the person to act on,
/// so the two travel together.
public struct PluginMetaRow: Codable, Sendable, Equatable {
    public var id: String
    public var name: String
    public var version: String
    public var permissions: [String]?
    public var loaded: Bool
    public var reason: String?
}

/// One plugin's declarative config schema (`core.pluginSchemas`).
///
/// `schema` is an untyped object rather than a modelled form, and that is
/// deliberate: a schema-shape this client does not know must render
/// VISIBLY rather than silently as nothing. A plugin may only contribute the
/// declarative tier (custom views are rejected upstream, bead cross-os-4lm),
/// so this is the whole plugin-configuration path — and a path that fails
/// quietly is a plugin system that is broken with nothing saying so.
public struct SchemaRow: Codable, Sendable, Equatable {
    public var plugin: String
    public var title: String
    public var schema: [String: JSONValue]
}
