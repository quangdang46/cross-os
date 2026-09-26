import Foundation

// The daemon's own types, in Swift.
//
// Every shape here is transcribed from the Go that already serves it —
// `app/backend/uisources.go`, `app/backend/bridge.go`, and `statusPayload` at
// `app/backend/ipc_client.go:127-134`. The JSON keys are the contract and are
// copied exactly; where the Go carries a comment explaining a decision, that
// comment came too, because the decision is the part that is easy to undo by
// accident.
//
// Go's `omitempty` has no Swift equivalent and does not need one: a field that
// the daemon omits decodes to `nil` here, and a field it sends as `""` decodes
// to `""`. Those are different states and Swift can tell them apart.

// MARK: - Status

/// One `core.status` response.
///
/// The Go client calls `core.status` **six times** to build the six accessors
/// above it — `IsRunning`, `InSafeMode`, `IsKilled`, `Interception`,
/// `TapError`, `Version` — each one dialing a fresh socket and decoding this
/// same object from scratch (ipc_client.go:145-199). With `App.tsx` polling
/// every 5s that is ~7 round trips per tick, 8 with `plugin.list`.
///
/// `CoreClient.status` calls `core.status` once and hands back this struct. The
/// footgun is named in its own doc comment so it does not get reintroduced by a
/// refactor that sees six accessors and reaches for six calls.
public struct DaemonStatus: Codable, Sendable, Equatable {
    public var running: Bool
    public var safeMode: Bool
    public var killed: Bool
    public var interception: Bool
    public var tapError: String
    public var version: String

    enum CodingKeys: String, CodingKey {
        case running
        case safeMode = "safe_mode"
        case killed
        case interception
        case tapError = "tap_error"
        case version
    }

    public init(
        running: Bool = false,
        safeMode: Bool = false,
        killed: Bool = false,
        interception: Bool = false,
        tapError: String = "",
        version: String = ""
    ) {
        self.running = running
        self.safeMode = safeMode
        self.killed = killed
        self.interception = interception
        self.tapError = tapError
        self.version = version
    }
}

/// One plugin's enabled state, from `plugin.list`. A separate call from
/// `core.status` in the Go client, and still one here — the daemon's two
/// answers are genuinely two questions.
public struct PluginState: Codable, Sendable, Equatable {
    public var id: String
    public var enabled: Bool

    public init(id: String, enabled: Bool) {
        self.id = id
        self.enabled = enabled
    }
}

// MARK: - Rows
//
// The comment above CommandRow in the Go is the rule the whole registry
// depends on, so it is the comment above the group: a plugin shipping
// something new must show up with no shell change. §3.6c's acceptance test is
// literally "delete a plugin from the source tree entirely → the shell UI
// still runs".

/// One command-palette entry (`core.commands`). Commands arrive from plugins,
/// so a new command must show up without a shell change — this row is data,
/// not a code path.
public struct CommandRow: Codable, Sendable, Equatable {
    public var id: String
    public var title: String
    public var plugin: String
}

/// One onboarding checklist item (`core.readiness`). `detail` carries the
/// "grant Accessibility" style hint, so a not-ready row can tell the user what
/// to do instead of only that it is red.
public struct ReadinessRow: Codable, Sendable, Equatable {
    public var id: String
    public var label: String
    public var ready: Bool
    public var detail: String
}

/// One window in the switcher (`core.windows`).
///
/// `index`, `selected` and `skippable` are CrossOS's own — the daemon owns the
/// MRU, so the shell draws the position it is handed instead of sorting a
/// second order. **Do not re-sort.** That sentence is the kind that gets
/// "tidied up" into a bug, so it is repeated here.
public struct WindowRow: Codable, Sendable, Equatable {
    public var windowID: String
    public var appID: String
    public var title: String
    public var index: Int
    public var selected: Bool
    public var skippable: Bool

    enum CodingKeys: String, CodingKey {
        case windowID = "window_id"
        case appID = "app_id"
        case title
        case index
        case selected
        case skippable
    }

    public init(
        windowID: String,
        appID: String,
        title: String,
        index: Int,
        selected: Bool,
        skippable: Bool
    ) {
        self.windowID = windowID
        self.appID = appID
        self.title = title
        self.index = index
        self.selected = selected
        self.skippable = skippable
    }
}

/// One thing the switcher did that the shell has not seen yet
/// (`core.switcherWait`).
///
/// `triggered == false` is the daemon's "your budget ran out", which is the
/// answer a poll expects, **not an incident**. A client that treats it as an
/// error will show a broken switcher every time nothing happens, which is most
/// of the time.
///
/// `action` is the switcher's own vocabulary — summon, commit — not a rule id
/// or an intent id: the shell draws a switcher, it does not read the decision
/// path.
public struct SwitcherTrigger: Codable, Sendable, Equatable {
    public var triggered: Bool
    public var action: String?
    public var windowID: String?

    enum CodingKeys: String, CodingKey {
        case triggered
        case action
        case windowID = "window_id"
    }

    public init(triggered: Bool, action: String? = nil, windowID: String? = nil) {
        self.triggered = triggered
        self.action = action
        self.windowID = windowID
    }
}

/// The trial in flight (`safety.trialState`).
///
/// Both durations are milliseconds: the countdown is driven by a timer, not a
/// re-render, and a wall-clock string would force the page to parse it.
/// `state == "none"` with an empty `plugin` is the daemon's honest "no trial in
/// flight" — a value, not a failure, and it must not render as a broken
/// countdown.
public struct TrialState: Codable, Sendable, Equatable {
    public var plugin: String
    public var state: String
    public var remainingMS: Int
    public var timeoutMS: Int

    enum CodingKeys: String, CodingKey {
        case plugin
        case state
        case remainingMS = "remaining_ms"
        case timeoutMS = "timeout_ms"
    }

    public init(plugin: String, state: String, remainingMS: Int, timeoutMS: Int) {
        self.plugin = plugin
        self.state = state
        self.remainingMS = remainingMS
        self.timeoutMS = timeoutMS
    }

    /// The daemon's "no trial in flight", so the question has one answer in the
    /// codebase instead of a string comparison in every view.
    public var isIdle: Bool { state == "none" }
}

/// One thing CrossOS created on this machine (`safety.ownershipAudit`) — the
/// Safety page's "What CrossOS created" list, and the input to Reset
/// Everything's ownership scoping (§8.2).
public struct AuditRow: Codable, Sendable, Equatable {
    public var resource: String
    public var id: String
    public var owner: String
    public var createdAt: String

    enum CodingKeys: String, CodingKey {
        case resource
        case id
        case owner
        case createdAt = "created_at"
    }

    public init(resource: String, id: String, owner: String, createdAt: String) {
        self.resource = resource
        self.id = id
        self.owner = owner
        self.createdAt = createdAt
    }
}
