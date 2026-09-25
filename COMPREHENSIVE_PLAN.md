# CrossOS — Comprehensive Architecture & Implementation Plan

> **Goal:** Move from Windows to macOS (and beyond) without relearning how to use your computer. Create a muscle-memory layer on top of the real OS.
>
> **Architecture:** Pluggable plugins + native adapters. Non-technical users first, developers second.
>
> **Tech Stack:**
> - Core: Go (event pipeline, context/intent engine, plugin runtime, permissions, config, IPC)
> - Desktop shell: Wails (Go + native WebView, no bundled Chromium)
> - UI: React + TypeScript (+ Tailwind/shadcn)
> - macOS adapter: Swift/ObjC at the boundary only (CGEvent, AX, FIFinderSync)
> - Windows adapter: Minimal Win32/C++ (WH_KEYBOARD_LL intercept + Raw Input observe + SendInput; UIPI applies to elevated targets)
> - Config: JSON/YAML
> - License: MIT
>
> **Toolchain policy:** always use the current stable release at scaffold time — do not pin
> to the versions below unless a verified incompatibility exists. **Wails: v3 beta decided
> by spike 2026-09-21 (see docs/wails-spike.md)** — multi-window + tray-attached windows are
> structural CrossOS needs (single-window v2 forces workarounds); revisit at scaffold: if v3
> platform APIs are still unstable, start shell on v2 and port at GA.

---

## 1. Vision & Principles

### Vision
CrossOS is a **desktop UX compatibility layer** — not a VM, not a shell replacement, not a theme engine. It intercepts input events, resolves the runtime context (what app is active, what's selected, where the cursor is), translates user intent into the OS-native action, and dispatches it through a plugin system. **CrossOS does not replace or patch the OS. It uses supported OS APIs and reversible integrations to add a compatibility layer** — only muscle-memory shortcuts and UI affordances are remapped; the OS underneath stays native.


### Glossary (normative vocabulary)

| Term | Meaning | Example |
|---|---|---|
| **Plugin** | User-behavior logic: declares rules + requests intents. Never touches the OS directly. | Windows UX, Developer UX, Vim UX |
| **Adapter** | OS implementation: executes a granted capability via native APIs. | macOS Keyboard Adapter, Windows Keyboard Adapter |
| **Extension** | Native OS integration component installed into the OS. | Finder Sync Extension |
| **Capability** | A named privileged operation the Core exposes. | `filesystem.createFile`, `window.close` |
| **Intent** | A platform-independent user goal. | `COPY`, `INTERRUPT`, `CLOSE_WINDOW` |
| **Action** | A concrete capability invocation with parameters, resolved from an intent. | `window.close{windowID}` |

> **Core authority rule: Plugin can request. Core decides. Adapter executes.** Plugins never
> perform OS operations directly — every request passes permission, context, conflict, and
> safety checks in Core before an Adapter executes it.

### Design Principles
| Principle | What it means |
|---|---|
| Intercept, don't patch | Only remap/intercept input events. Never patch Finder, system binaries, or the kernel. Disabled = CrossOS stops affecting input/behavior (OS permission entries in System Settings may remain until the user removes them). |
| Ownership | Every integration records ownership metadata. Cleanup removes only what CrossOS created. |
| Reversibility | All state changes are reversible. Every action has a rollback path. |
| Non-tech first | No technical configuration required. CrossOS guides the user through required OS permissions (Welcome → Enable → Open System Settings → Verify ready). |
| Dev second | Developers can write plugins, extend rules, and customize behavior. |
| Kill switch (PANIC STOP) | One click: disable event interception → disable plugin actions → flush/close. Does NOT remove login item, does NOT uninstall. |
| Safe mode | New integrations enabled for 30s; user must confirm or auto-rollback. |

---

## 2. Architecture Overview

```
                         CrossOS
                            │
              ┌─────────────▼─────────────┐
              │          Core             │
              │                           │
              │ Event Normalizer          │
              │ Context Cache             │
              │ Intent Resolver           │
              │ Rule Engine               │
              │ Action Dispatcher         │
              │ Plugin Manager            │
              │ Permission Manager        │
              │ Safety / Lifecycle        │
              │ Recorder (observe/replay) │
              └─────────────┬─────────────┘
                            │
                     Plugin API / IPC
                     (JSON-RPC over stdio / Unix socket)
                            │
           ┌────────────────┼────────────────┐
           ▼                ▼                ▼
      Windows UX       Developer UX      Finder UX
       Plugin             Plugin           Plugin
           │                │                │
           └────────────────┼────────────────┘
                            │
                  Platform Capability API
                            │
             ┌──────────────┴──────────────┐
             ▼                             ▼
        macOS Adapter                 Windows Adapter
             │                             │
      CGEvent / AX / Finder          WH_KEYBOARD_LL / Raw Input / Win32
```

> Interface note: the MVP exposes the event pipeline behind an `EventBus`-named
> interface, but the implementation is a **deterministic synchronous EventRouter**
> (`input → normalize → context → rule → action`). Full pub/sub is deferred until
> the plugin ecosystem genuinely needs broadcast events (avoids goroutine/channel
> ordering, backpressure, and subscription-lifecycle complexity before there are users).

### Data Flow (per event)

> Latency budget: **intercept decision < 1ms** = CGEvent/hook callback < ~100µs +
> fast rule match < ~500µs. Recorder, UI logs, IPC, and context enrichment are on the
> slow path and NEVER in the budget.

**Fast path (fully synchronous, INSIDE the native callback — decision is made and
returned before the callback exits; nothing about the current event is queued or
deferred to a worker first):**
1. **Native callback** (CGEventTap / WH_KEYBOARD_LL) normalizes minimal data and
   updates atomic keyboard state (in-place, lock-free).
2. **FastContext** supplies the decision inputs from cache (updated on app/window
   change, NOT queried synchronously per keydown): AppID (bundle ID / executable +
   AppMode: native | terminal | remote | vm | excluded), WindowID + class, DeviceID
   (for keyboard remapping), modifier state.
3. **Fast matcher**: Keyboard State + FastContext → Rule lookup (Core-compiled
   matcher, filter → rank by specificity → break ties by priority) → exactly one
   `Decision`: `PASS / CONSUME / REPLACE` (see §3.6 for the canonical enum — this is
   the ONLY decision vocabulary used anywhere in this document). Declarative/builtin
   rules only; external (Level B) plugins are compiled into this cached matcher
   ahead of time and never run synchronously on this path (a 100ms plugin hang would
   break typing).
4. **Callback returns the decision immediately**: `PASS` → return the original event
   untouched; `CONSUME` → return null/block, no replacement; `REPLACE` → return
   null/block AND dispatch the resolved Intent to the native action (§3.6b) —
   synthesizing input (SendInput/CGEvent) only when the target action itself
   requires keyboard/mouse injection (e.g. remapping to a different physical key),
   never as a default way to invoke a capability. A slow callback risks the tap
   being disabled by timeout (`kCGEventTapDisabledByTimeout`) on macOS or the hook
   being removed on Windows — this is why steps 1-4 must stay allocation-light and
   branch on cached state only.
5. **Only after the callback has returned**, the (already-decided) event + decision
   is enqueued as a bounded async record for the slow path. The slow path never
   feeds back into step 3-4 for that same event.

**Slow path (async worker, runs after the fast-path decision already shipped):**
Recorder, context enrichment (LazyContext: selection, cursor, focused element, UTI),
UI activity log, plugin IPC, analytics/debug. Nothing here can retroactively change
a decision already returned to the OS.
6. **Rule Engine + Intent Resolver** (this is the same resolver whose compiled output
   the fast matcher consults in step 3; described here for the general
   non-keyboard-critical-path case, e.g. window/menu/finder actions) match normalized
   event + context against rules → produce a platform-independent `Intent` (never a
   raw shortcut translation): `Physical Shortcut → Logical Shortcut → Intent → Native
   Action`. Example: `Ctrl+C` in Finder → `COPY_SELECTION` → Finder native copy; same
   `Ctrl+C` in Terminal → `INTERRUPT` → SIGINT. `Alt+F4` → `CLOSE_WINDOW` (close
   current window — NOT `Cmd+Q`, which quits the app), resolved per-platform by the
   Adapter.
7. **Conflict resolution** picks the winner when several plugins claim one event
   (most-specific match wins; see §3.5b): Priority × Scope × Specificity, verdict
   `CONSUME / PASS / REPLACE` (same enum as step 3 — kept identical on purpose).
8. **Permission Manager** checks the request against granted least-privilege
   permissions.
9. **Action Dispatcher → Platform Capability API → Adapter** executes the native
   action. The Recorder taps every stage (event → context → rule → intent → action →
   result) for observe mode and replay.

### Event-Driven Plugin API (inspired by Pi / windhawk / komorebi)

The plugin model is event-driven and capability-based. Plugins declare what they can do and subscribe to what they want to see. This mirrors the Pi extension API model described in the ChatGPT conversation:

- **Plugin manifest** (`plugin.json`): declares hooks subscribed, capabilities registered, permissions required, config schema.
- **Registration** (not per-event hooks): `RegisterRule`, `RegisterIntent`, `RegisterCapability`, `RegisterContextProvider`, `RegisterMenu`. `OnEvent` exists only as an async slow-path observer — never a fast-path decision hook.
- **Capabilities**: plugins register capability handlers using ONLY names from the canonical registry (§3.12), e.g. `"input.intercept"`, `"window.move"`, `"finder.menu"`. Naming convention (locked): **permission** = consent granted by the user (e.g. `input.intercept` — Core checks); **capability** = operation Core provides (e.g. `input.intercept`, `window.close` — Core executes); **intent** = user goal (e.g. `COPY`, `CLOSE_WINDOW` — resolver produces). Do not invent parallel names — `keyboard.intercept`, `window.manage`, `finder.contextMenu` are NOT valid names anywhere in this document; use the canonical `input.intercept`, `window.*`, `finder.menu` instead.
- **Intent**: plugins emit intents that the intent resolver routes to capability handlers.
- **IPC**: plugins communicate with Core via a JSON-over-stdin or Unix socket protocol. Script plugins run as child processes.

---

## 3. Core Engine (Go)

### 3.1 Module Structure

```
core/
├── go.mod
├── cmd/crossos/              # daemon entrypoint
│   └── main.go
├── pkg/
│   ├── event/               # Event type definitions, EventBus
│   ├── context/             # ContextResolver
│   ├── intent/              # IntentResolver, Intent types
│   ├── rule/                # RuleEngine, matchers
│   ├── plugin/              # PluginRuntime, PluginLoader
│   ├── pluginapi/           # Plugin API (protobuf or JSON schema)
│   ├── permission/          # PermissionManager
│   ├── config/              # ConfigManager
│   ├── ipc/                 # IPC server (Unix socket / named pipe)
│   ├── lifecycle/           # Plugin lifecycle, safety
│   ├── safety/              # Kill switch, safe mode, reset
│   └── util/                # shared helpers
├── internal/
│   └── adapter/             # CGO bridge to platform adapters
└── proto/                   # protobuf definitions (if used)
```

### 3.2 Event Bus

**Source:** inspired by keymapper's `Stage` class and hammerspoon's event model.

```go
// Event is the normalized input event.
type Event struct {
    Type      EventType      // KeyPress, KeyRelease, Hotkey, MouseClick, MouseMove, Scroll, AppSwitch, etc.
    Timestamp time.Time
    Source    EventSource    // device identity, input method
    Payload   json.RawMessage // type-specific data
}

type EventType string
const (
    EventKeyPress    EventType = "keypress"
    EventKeyRelease  EventType = "keyrelease"
    EventHotkey      EventType = "hotkey"
    EventMouseClick  EventType = "mouseclick"
    EventMouseMove   EventType = "mousemove"
    EventScroll      EventType = "scroll"
    EventAppSwitch   EventType = "appswitch"
    EventWindowFocus EventType = "windowfocus"
)

type EventSource struct {
    DeviceID    string
    DeviceType  string // "keyboard", "mouse", "trackpad", "remote"
    VendorID    uint16
    ProductID   uint16
    IsBuiltIn   bool
}
```

The `EventBus` interface is the single entry point for the **observation path**
(async, best-effort — recorder, activity log, plugin observers), but the
**decision path** is a deterministic synchronous EventRouter
(`input → normalize → context → rule → action`), fully resolved inside the native
callback before it returns (§Data Flow). `EventBus` is never on the decision path —
plugins do NOT subscribe to hot-path events; they register rules/capabilities (§3.6).
Generic pub/sub is deferred until broadcast is genuinely needed.

### 3.3 Context Resolver

**Source:** keymapper `FocusedWindow` classes, menumate `RuleMatcher`, nudge AX patterns.

// FastContext rides the hot path (all fields cache-resident). LazyContext is resolved
// async on the slow path, only for rules that declare `requires:` on it.
```go
type FastContext struct {
    AppID      string       // bundle ID / executable
    AppMode    AppMode      // native | terminal | remote | vm | excluded
    WindowID   string
    DeviceID   string
    Modifiers  ModifierState
}

type LazyContext struct {
    Selection      SelectionInfo // Finder selection, UTIs
    Cursor         CursorInfo
    FocusedElement string        // AX path
}

type Context struct {            // full context (slow path / debugging)
    Event       Event
    Fast        FastContext
    Lazy        *LazyContext     // nil unless a matching rule requires it
    Application ApplicationInfo
    Window      WindowInfo
    Session     SessionInfo      // login state, power state
}

type AppMode string
const (
    AppModeNative   AppMode = "native"
    AppModeTerminal AppMode = "terminal" // Ctrl+C = INTERRUPT, Ctrl+V = PASTE, etc.
    AppModeRemote   AppMode = "remote"   // RDP/Citrix: CrossOS disabled
    AppModeVM       AppMode = "vm"       // Parallels/VMware/VirtualBox: CrossOS disabled
    AppModeExcluded AppMode = "excluded" // user-excluded apps
)

type ApplicationInfo struct {
    BundleID      string   // macOS: com.apple.finder, com.microsoft.teams
    Executable    string   // process name
    PID           int
    DisplayName   string
    AppMode       AppMode    // replaces coarse IsRemote bool + excludedApps[] allowlist
    Category      AppCategory // "terminal", "browser", "remote", "system", "user"
}

type WindowInfo struct {
    Title         string
    Role          string   // AXRole description
    Frame         CGRect   // screen coordinates
    ScreenIndex   int      // multi-monitor
    IsFullScreen  bool
    WindowID      string   // platform-specific ID
}

type CursorInfo struct {
    Position      CGPoint
    Screen        int
    InElement     string   // AX path under cursor
}

type SelectionInfo struct {
    Items         []URL    // selected files in Finder or focused selection
    Container     string   // parent directory
    UTITypes      []string
}
```

Context Resolver is pluggable per-platform:
- macOS: Swift adapter queries `NSWorkspace`, `AXUIElement`, `CGWindowList`.
- Windows: C++ adapter queries `GetForegroundWindow`, `GetWindowText`, `AccessibleObject`.

### 3.4 Intent Resolver

**Source:** keymapper `Stage` output matching, windows-keyboard-for-mac behavior matrix.

```go
// Intent is a versioned structured type — never a loose Action string + any-map.
// Parameters are validated against the target capability's input schema.
type Intent struct {
    ID         string          // e.g. "window.snap", "clipboard.copy" (canonical registry, §3.12)
    Version    int             // intent schema version
    Source     Source          // "keyboard", "hotkey", "contextMenu"
    Parameters json.RawMessage // e.g. {"zone":"left-half"} — schema-validated
}
```

The Intent Resolver is a rule-based matcher. It takes Event + Context and produces an Intent. Rules are derived from the behavior matrix of each plugin.

### 3.5 Rule Engine

**Source:** menumate `RuleMatcher`, keymapper `Stage` context matching.

The Rule Engine evaluates whether an intent should be dispatched based on:
- Application filters (bundle IDs, categories, device types)
- Device filters (physical keyboard identity)
- Context filters (window role, cursor position, selection type)
- Plugin state (enabled/disabled)
- Safety mode (30-second grace period)

Rules are expressed as JSON/YAML and stored in the plugin's config.

### 3.5b Conflict Resolution (P0)

When several plugins claim one event (e.g. Windows UX maps `Ctrl+C → COPY`, Vim UX maps
`Ctrl+C` to something else, Terminal plugin maps it to `SIGINT`):

```go
type RuleMatch struct {
    RuleID      string
    PluginID    string
    Priority    int        // baseline: terminal/VM = 100, app-specific = 80, global = 10
    Specificity int        // computed: app+window+device+requires match depth
    Scope       Scope
    Intent      Intent
}

type Resolution struct {
    RuleID   string
    PluginID string
    Intent   Intent
    Decision Decision
}
```

Resolution is a Core function over candidate rules: filter (enabled, AppMode, requires
satisfiable) → rank by specificity → break ties by priority → exactly ONE winner →
one Intent. Plugins never REPLACE another plugin's resolved intent after the fact;
a "replace" is just the winner's own Intent substituting the native action.
Core logs winner + losers per event (feeds the Recorder).

### 3.6 Plugin Runtime

**Source:** windhawk's Mod system, komorebi's external client model, menumate's extension pack system.

```go
// Plugin API v1 — plugins REGISTER behavior; Core owns the pipeline.
// A plugin never sits in the event path: no per-event callbacks on the fast path.
// OnEvent is an async observer hook (slow path) — NOT a decision hook.
type Plugin interface {
    Manifest() Manifest
    Register(registry *Registry) error
}

type Registry interface {
    RegisterRule(rule Rule) error             // physical → intent, with requires:/scope:/priority
    RegisterIntent(intent IntentDef) error
    RegisterCapability(cap Capability) error  // plugin-owned capability only — see ownership rule below
    RegisterContextProvider(p CtxProvider) error
    RegisterMenu(menu MenuDef) error          // Finder/context menus (native actions only)
    RegisterUI(contrib UIContribution) error  // UI extension surface — see §3.6c
}

// Decision is produced by CORE's conflict resolver, not returned by plugins.
// This is the ONE canonical decision enum for the whole document — the fast
// path (§ Data Flow, step 3) and the slow-path conflict resolver (step 7)
// both produce and consume exactly this type. Do not reintroduce a separate
// PASS/SUPPRESS/REPLACE vocabulary anywhere else.
type Decision int
const (
    DecisionPass    Decision = iota // no rule matched — pass through
    DecisionConsume                  // winner handled — suppress original
    DecisionReplace                  // winner's Intent substitutes the native action
// Normative: DecisionReplace = suppress the original physical event and execute
// the resolved Intent. It does NOT imply synthetic keyboard injection. Intent →
// Capability uses CGEvent/SendInput ONLY when the capability itself is keyboard
// injection (remap to a different physical key); e.g. Ctrl+C → COPY →
// clipboard.copy goes straight to the native capability, never replays Cmd+C
// through the OS. CrossOS copies UX, not implementation.
)

// Capability ownership (P0 — locked before Phase 0 native spikes):
//   - "core"   — capabilities in the canonical registry (§3.12: window.close,
//                filesystem.write, clipboard.write, ...). Implemented by Core/Adapter
//                only. RegisterCapability MUST reject any manifest that declares an
//                Owner == "core" ID already present in the canonical registry —
//                plugins can never shadow or override a core capability.
//   - "plugin" — a capability namespaced under the plugin's own ID
//                (e.g. "myplugin.doSomething"), implemented by that plugin's own
//                process/handler. Plugins may freely register these.
// A plugin that needs a core capability calls it through the Capability API
// (request), it never re-declares/owns it.
type CapabilityOwner string
const (
    CapabilityOwnerCore   CapabilityOwner = "core"
    CapabilityOwnerPlugin CapabilityOwner = "plugin"
)

type Capability struct {
    Name        string          // canonical registry name, e.g. "window.close" (§3.12),
                                 // or "<pluginID>.<name>" for plugin-owned capabilities
    Owner       CapabilityOwner // "core" IDs are reserved; Register fails if a plugin
                                 // tries to claim one
    Description string
}

type PluginManifest struct {
    ID            string            `json:"id"`
    Name          string            `json:"name"`
    Version       string            `json:"version"`
    Entry         string            `json:"entry"`          // path to executable or Go plugin
    Type          PluginType        `json:"type"`           // "builtin", "declarative", "executable", "extpack"
    Permissions   []Permission      `json:"permissions"`
    Capabilities  []Capability      `json:"capabilities"`
    ConfigSchema  json.RawMessage   `json:"config_schema"`
    Hooks         []Hook            `json:"hooks"`
    Safety        PluginSafety      `json:"safety"`
}

// Plugin levels. Go `.so` via `plugin.Open()` is explicitly REJECTED
// (OS/version/symbol fragility, undeployable cross-platform).
type PluginType string
const (
    PluginBuiltin PluginType = "builtin"     // compiled into core (MVP latency path)
    PluginDecl    PluginType = "declarative" // Level A: manifest/rules/actions JSON only, no arbitrary code (default)
    PluginExec    PluginType = "executable"  // Level B: out-of-process binary, JSON-RPC over stdio/Unix socket
    PluginExtPack PluginType = "extpack"     // git repo + manifest (declarative; shell only via Level B gate)
)

// Layout: ~/.crossos/plugins/<id>/{manifest.json, plugin}
// Core spawns the executable; a plugin crash never takes down Core (restart/disable).
// Level B is MVP 1 — MVP 0 ships builtin + declarative only.

// Hook names. Registry methods in §3.6 are the normative way to register behavior;
// hooks below exist for the manifest schema but carry the following semantics — READ:
//  - `onIntent` / `onEvent` / `onContext` are ASYNC OBSERVERS on the slow path ONLY.
//    They never run on the keyboard critical path and never return a Decision.
//    (This supersedes any earlier wording that implied per-event fast-path callbacks.)
type Hook string
const (
    HookOnIntent     Hook = "onIntent"     // async observer of resolved intents — NOT a decision hook
    HookOnEvent      Hook = "onEvent"      // async observer, slow path only
    HookOnContext    Hook = "onContext"
    HookOnLifecycle  Hook = "onLifecycle"
)
```

Plugin loading strategy:
1. **Builtin plugins** are compiled into the core binary. No loading risk. (MVP 0)
2. **Declarative plugins** (Level A) are pure `manifest.json` + `rules.json` + `actions.json` —
   no arbitrary code; Core interprets them through the Capability API. (MVP 0)
3. **Executable plugins** (Level B) run as child processes, JSON-RPC over stdin/stdout or
   Unix socket. Plugin crash → Core survives → restart/disable. (MVP 1)
4. **Extension packs** are Git repos with a `manifest.json` (modeled after MenuMate's
   PackManifest). Shell scripts inside a pack are Level B capabilities (`shell.execute`),
   never the default action mechanism — default Finder actions go through native capabilities
   (`filesystem.createFile`, `clipboard.copyPath`, `app.open`, `terminal.openAt`, `file.moveToTrash`).

### 3.12 Capability Registry (canonical, P0)

Every capability is a versioned contract:

```go
type CapabilityDescriptor struct {
    ID          string       // e.g. "window.close"
    Version     string
    Description string
    Permission  Permission    // CrossOS permission required
    InputSchema Schema
    OutputSchema Schema
    Platforms   []Platform    // macOS / Windows
    SideEffects []SideEffect
    Reversible  bool
}
```

Canonical registry (v1):

```
input.observe, input.intercept
window.read, window.move, window.close, window.minimize, window.maximize
clipboard.read, clipboard.write, clipboard.copy, clipboard.copyPath
filesystem.read, filesystem.write, filesystem.createFile, filesystem.createFolder, file.moveToTrash
app.launch, app.open, terminal.openAt
finder.menu
```

Example — `window.close`: permission `accessibility.control`; platforms macOS+Windows;
side effect closes focused window; reversible `false`.
Manifest, Plugin API, and Capability API versions are independent
(`manifestVersion` / `pluginApiVersion` / `capabilityApiVersion` + `minCoreVersion`).

### 3.7 Permission Manager

**Source:** menumate's `ActionDispatcher` security model, the concept of "secure by default."

```go
type Permission string

const (
    PermInputMonitor     Permission = "input.monitor"       // observe input events (macOS: Input Monitoring consent)
    PermInputIntercept   Permission = "input.intercept"    // suppress/replace input (macOS: Input Monitoring consent; Win: WH_KEYBOARD_LL)
    PermAccessControl    Permission = "accessibility.control" // control other apps via AX/UIA (macOS: Accessibility; Win: UIA)
    // NOTE: this is the ONLY "control other apps" permission. A prior draft also had a
    // separate `PermAccessibility = "accessibility"` — removed as a duplicate of this one;
    // do not reintroduce a second permission for the same OS consent.
    PermFilesystem       Permission = "filesystem"        // read/write files
    PermNetwork         Permission = "network"            // network access
    PermShellExecution  Permission = "shell.execute"     // run shell commands
    PermFinderModify     Permission = "finder.modify"     // modify Finder state
)
```

Permission flow:
1. At install time, the plugin manifest declares required permissions (least privilege — a keyboard plugin declaring `filesystem` is rejected as over-scoped; Windows Keyboard → `["input.intercept"]`, Window → `["input.intercept", "accessibility.control"]`, Finder → `["finder.menu", "filesystem.write"]`, Developer → `["shell.execute", "filesystem.read"]`).
2. Core prompts once in non-tech language and the user MUST approve — NO auto-approve, not even for "safe" defaults (`input.intercept`, `accessibility`, `filesystem` are all sensitive). Example: "Windows Keyboard wants: read keyboard events, modify keyboard behavior — [Allow]".
3. At runtime, the PermissionManager checks if an action is allowed for the current plugin + context.
4. Every action is logged with: plugin ID, timestamp, context, action, result, success/failure.

Security model inspired by MenuMate:
- **Extension requests are untrusted**: the Finder extension sends a request, but the Core daemon validates it against local config. The extension never sends executable code.
- **Path validation**: script paths must be relative and within the plugin's directory.
- **File inspection**: undeclared files in extension packs are highlighted for review (MenuMate's `PackInspector`).

### 3.8 Config Manager

**Source:** menumate `ConfigStore`, komorebi `schema.json`.

Config is stored as JSON in `~/.crossos/config.json` (or `~/Library/Application Support/CrossOS/config.json` on macOS). Schema validation is done at startup.

Config levels (in priority order):
1. System defaults (built-in JSON)
2. User config (`config.json`, can override defaults)
3. Plugin config (`plugins/<id>/config.json`)
4. Session overrides (runtime temporary changes)

### 3.9 IPC Server

Core exposes an IPC server for:
- UI (Wails) ↔ Core communication (Unix socket on macOS/Linux, named pipe on Windows).
- Script/compiled plugins ↔ Core communication (same protocol).

Protocol: JSON-RPC 2.0 over bidirectional transport.

### 3.10 Lifecycle & Safety

**Source:** menumate's "Safe Mode," CrossOS's "Reset Everything."

```go
type LifecycleState string
const (
    LifecycleInitializing LifecycleState = "initializing"
    LifecycleRunning      LifecycleState = "running"
    LifecycleSafeMode     LifecycleState = "safemode"      // 30s grace
    LifecyclePaused       LifecycleState = "paused"
    LifecycleStopped      LifecycleState = "stopped"
    // Plugin enable flow (transactional, ownership-scoped, NOT an OS snapshot):
    // DISABLED → TRIAL → (user confirms + healthy) → ENABLED.
    // crash / timeout / kill-switch during TRIAL → DISABLED.
    LifecycleDisabled     LifecycleState = "disabled"
    LifecycleTrial        LifecycleState = "trial"
    LifecycleEnabled      LifecycleState = "enabled"
)
```

Safety mechanisms:
- **Safe Mode**: When a new plugin/integration is enabled, Core enters safe mode for 30 seconds. If the user doesn't confirm, it auto-rolls back.
- **Kill Switch**: Global emergency deactivation.
- **Reset Everything**: Disables all hooks, stops daemon, disables all plugins, removes login item, verifies no process remains.
- **CrossOS Integration State (NOT a system snapshot)**: before enabling an integration, Core records only resources CrossOS itself owns (its login item, its config, its plugin state). Rollback removes/restores those. CrossOS cannot and does not snapshot/restore arbitrary macOS state (Accessibility grants, Input Monitoring, CGEvent taps, foreign Login Items). **Principle: CrossOS can rollback CrossOS state, not arbitrary OS state.**
- **Ownership Tracking**: Every system modification (login item, config file, etc.) records: creator plugin ID, timestamp, rollback procedure.

---

### 3.11 Event Recorder / Replay + Observe Mode (P0)

Debugging `Ctrl+C doesn't work in Ghostty` by hand does not scale. Core taps every stage:

```
Real Input ─┬──→ Runtime pipeline
            └──→ Recorder (event → context → rule → intent → action → result)
```

- **Record**: each key event stores the full trace, e.g.
  `KeyDown Ctrl → KeyDown C → Context: Ghostty → Rule: windows-keyboard.copy → Intent: COPY → Action: macOS.copy → success`.
- **Replay**: a recorded trace re-runs through Core deterministically without live input —
  the primary tool for AI/code-agent debugging.
- **Observe (dry-run) mode**: before CrossOS modifies anything, the UI logs
  `Physical → Context → Matched rule → Intent → Would-execute` WITHOUT executing.
  User enables execution only after the trace looks right. Essential for testing new plugins.
- **Privacy model (normative):** modes `OFF / METADATA_ONLY (default) / DEBUG / FULL_TRACE`.
  METADATA_ONLY redacts typed text, clipboard content, and password fields. Replay replays
  semantic events (`KEY_DOWN Ctrl`, `KEY_DOWN C`) — never raw clipboard/secret contents.
- **Replay tiers:** (1) Raw — `Ctrl down / C down / C up / Ctrl up`; (2) Semantic — `COPY_SELECTION`;
  (3) Capability — `clipboard.copySelection`. Tier 2/3 are the primary debug loop
  (reproduce → fix → replay → verify).

## 4. Platform Adapters

### 4.1 macOS Adapter (Swift/ObjC)

**Source:** newfile's FIFinderSync, menumate's FinderExtension, nudge's AX/Carbon patterns, Karabiner-Elements' CGEvent/DriverKit.

The macOS adapter lives in `platform/darwin/`. Boundary is **Go → C ABI (cgo) → Objective-C++ → Swift/AppKit/CoreGraphics** (Swift is never called from Go directly). It provides:

#### Keyboard Interception
- **CGEvent tap** (`CGEventTapCreate`) for session/HID-level keyboard/mouse interception.
- Session/HID-level interception via `kCGHIDEventTap` (NOT "application-level" — it sits low in the event pipeline; permission scope differs by tap location). No kernel driver needed for MVP; DriverKit virtual-HID is post-MVP.

#### Context Detection
- `NSWorkspace` for active application (bundle ID, PID).
- `AXUIElement` for focused window, window list, screen frames.
- `CGWindowListCopyWindowInfo` for window enumeration with PIDs.
- `CGEventSource` for device identity (vendor/product ID).

#### Window Management
- `AXUIElementSetAttributeValue` with `kAXSizeAttribute`, `kAXPositionAttribute`.
- Uses Accessibility API (requires Accessibility permission).
- Inspired by Nudge's `WindowManager.swift`: frame history (capacity 128), snap zones from `NSScreen.visibleFrame`.

#### Finder Context Menu
- **FIFinderSync** extension (in `extensions/finder-sync/`).
- Provides context menu items in Finder via `menu(for:)` and `menuForContainerCopyItems`.
- Uses `FIFinderSyncController` for directory-scoped menus.
- IPC with Core daemon via **Unix socket** (normative). `DistributedNotificationCenter` is rejected for command transport (no delivery guarantees / error handling).
- The Finder Sync Extension is a **minimal IPC client only**: display menu → send action request → receive result. It MUST NOT become a mini CrossOS runtime — no rule evaluation, no script execution.
- **Lifecycle/fallback (normative):** daemon-not-running, restarting, stale/unavailable socket, permission mismatch, extension-launched-before-daemon, daemon-killed → the extension MUST fail gracefully: hide the CrossOS menu section, NEVER block Finder. IPC timeout ~100-300ms; no response → fallback/hide. Finder never waits for CrossOS.

#### App Categories
- Bundle ID → category mapping (terminal, browser, remote, system).
- Hardcoded terminal bundles: Terminal, iTerm2, WezTerm, Warp, Alacritty, Kitty, Hyper, Ghostty.
- Hardcoded browser bundles: Safari, Brave, Chrome, Edge, Opera, Firefox.
- Hardcoded remote bundles: Microsoft RDC, Parallels, VMware Fusion, VirtualBox.

### 4.2 Windows Adapter (C++)

**Source:** keymapper's `server/windows/`, windhawk's mod injection model.

Provides equivalent functionality on Windows:
- **WH_KEYBOARD_LL** (`SetWindowsHookEx`) for interception/suppression — callback returns non-zero to suppress. Raw Input (`RegisterRawInputDevices`, `WM_INPUT`) is observation/device identity ONLY, not a suppression mechanism.
- **SendInput** for synthetic input. Subject to UIPI: can only inject into apps at equal-or-lower integrity level (elevated targets require elevated CrossOS).
- **GetForegroundWindow** + **GetWindowText** for active window/app detection.
- **AccessibleObject** / UI Automation for window manipulation.
- Shell context menu via `IContextMenu` or `IShellExtInit`.

---

## 5. Plugin System

### 5.1 Plugin Manifest Format

```json
{
  "manifestVersion": 2,
  "pluginApiVersion": 1,
  "capabilityApiVersion": 1,
  "minCoreVersion": "0.1.0",
  "id": "windows-keyboard",
  "name": "Windows Keyboard",
  "description": "Windows muscle-memory keyboard shortcuts on macOS",
  "version": "1.0.0",
  "author": "CrossOS",
  "entry": "builtin",
  "type": "builtin",
  "permissions": ["input.intercept"],
  "capabilities": [
    {"name": "input.intercept", "owner": "core", "description": "Request keyboard interception behavior declared via rules"}
  ],
  "config_schema": {
    "type": "object",
    "properties": {
      "excludedApps": {"type": "array", "items": {"type": "string"}},
      "terminalBundles": {"type": "array", "items": {"type": "string"}},
      "browserBundles": {"type": "array", "items": {"type": "string"}}
    }
  },
  "safety": {
    "safeModeDurationSec": 30,
    "autoRollback": true
  }
}
```

### 5.2 Built-in Plugins Manifest

| Plugin ID | Type | Capabilities | Phase (§10) |
|---|---|---|---|
| `windows-keyboard` | builtin | input.intercept | Phase 1 (MVP 0) |
| `windows-window` | builtin | window.* (move/close/minimize/maximize) | Phase 2 (MVP 1) |
| `finder-ux` | extpack | finder.menu | Phase 3 (MVP 2) |
| `developer` | script | shell.execute, terminal.openAt | Phase 4 (Ecosystem) |
| `launcher` | script | input.intercept, app.launch | Phase 4 (Ecosystem) |

### 5.3 Extension Pack Format (modeled after MenuMate PackManifest)

Extension packs are Git repos installed into `plugins/<pack-id>/`. They contain:
```
pack/
├── manifest.json       # PackManifest (manifestVersion, name, author, description, icon, actions[])
├── actions/            # scripts (shell, python, etc.)
│   ├── copy-path.zsh
│   └── open-terminal.zsh
├── README.md
├── LICENSE
└── .git                # optional (for updates)
```

#### manifest.json (extension pack)

```json
{
  "manifestVersion": 2,
  "pluginApiVersion": 1,
  "capabilityApiVersion": 1,
  "name": "Windows Explorer UX",
  "author": "CrossOS",
  "description": "Right-click context menu items matching Windows Explorer",
  "icon": "shippingbox",
  "actions": [
    {
      "id": "new-text-file",
      "title": "New > Text Document",
      "icon": "doc.text",
      "action": {"type": "native", "capability": "filesystem.createFile", "parameters": {"extension": "txt"}},
      "targets": "files",
      "utis": ["public.folder"],
      "placement": "submenu",
      "variants": {"fixed": [".txt", ".md"]},
      "timeoutSeconds": 30,
      "localizedTitles": {"en": "New > Text Document", "vi": "Mới > Tài liệu văn bản"}
    },
    {
      "id": "copy-path",
      "title": "Copy as Path",
      "icon": "doc.on.doc",
      "action": {"type": "native", "capability": "clipboard.copyPath"},
      "targets": "any",
      "placement": "topLevel"
    }
  ]
}
```

### 5.4 Action Schema (modeled after MenuMate PackAction)

| Field | Type | Required | Description |
|---|---|---|---|
| `id` | string | yes | Unique action identifier |
| `title` | string | yes | Display name in context menu |
| `icon` | string | yes | SF Symbol name (macOS) / icon name (Windows) |
| `action` | object | yes | `{"type":"native","capability":"…","parameters":{…}}` (default) or `{"type":"process","plugin":"…","operation":"…"}` (Level B). `script` is NOT part of the schema. |
| `targets` | string\|string[] | yes | `"files"`, `"folders"`, `"any"`, `"container"`, `"selection"` |
| `utis` | string[] | no | UTI filters (macOS only) |
| `placement` | string | no | `"topLevel"` or `"submenu"` (default: `"topLevel"`) |
| `variants` | object | no | `{"fixed": ["png","jpeg","tiff"]}` or `{"confirmation": true}` |
| `timeoutSeconds` | int | no | Script timeout (default: 60) |
| `interface` | string | no | `"confirmation"`, `"none"` |
| `localizedTitles` | object | no | `{"en": "...", "vi": "..."}` |
| `isEnabled` | bool | no | Default true |

### 5.5 Rule Matching (modeled after MenuMate RuleMatcher)

Rules match actions against runtime context:
- **Targets filter**: files only, folders only, any, container (the folder itself), selection (any selected item).
- **UTI filter**: only show actions for specific file types.
- **Application filter**: show actions only in specific apps.
- **Selection count**: minimum/maximum number of selected items.

---

## 6. MVP Plugins

### 6.1 Plugin 1: Windows Keyboard (Builtin, Go + Swift adapter)

**Behavior matrix** (from windows-keyboard-for-mac + Karabiner profile JSON).
Framing is INTENT-first: the left column is the physical Windows shortcut, the middle is the
`Intent`, and the Adapter resolves the native action per app/platform. Raw mappings like
`Alt+F4 → Cmd+Q` are WRONG (`Cmd+Q` quits the app; intent `CLOSE_WINDOW` closes the window) —
the Adapter, not a static table, decides the native action. `Ctrl+W` similarly resolves per-app.

| Windows Shortcut | macOS Equivalent | Terminal/Browser Override |
|---|---|---|
| Ctrl+C | Cmd+C | Ctrl+C (pass through) |
| Ctrl+V | Cmd+V | Ctrl+V (pass through) |
| Ctrl+X | Cmd+X | Ctrl+X (pass through) |
| Ctrl+A | Cmd+A | Ctrl+A |
| Ctrl+Z | Cmd+Z | Ctrl+Z |
| Ctrl+Y | Cmd+Shift+Z (or Ctrl+Y) | Ctrl+Y |
| Ctrl+F | Cmd+F | Ctrl+F |
| Ctrl+S | Cmd+S | Ctrl+S |
| Ctrl+O | Cmd+O | Ctrl+O |
| Ctrl+N | Cmd+N | Ctrl+N |
| Ctrl+P | Cmd+P | Ctrl+P |
| Ctrl+W | `CLOSE_TAB` → Adapter resolves per-app native close-tab | Ctrl+W |
| Ctrl+T | Cmd+T | Ctrl+T |
| Ctrl+Tab | Ctrl+Tab (or Cmd+Option+Left) | Ctrl+Tab |
| Ctrl+Shift+Tab | Ctrl+Shift+Tab | Ctrl+Shift+Tab |
| F2 | Enter (rename in Finder) | Enter |
| F5 | Refresh (scroll lock) | Fn+Cmd+R or F5 |
| Alt+F4 | `CLOSE_WINDOW` → Adapter closes current window (NOT Cmd+Q quit) | pass through |
| Alt+F4 (on VM) | Alt+F4 (pass through) | Alt+F4 |
| Alt+Tab | Cmd+Tab | Cmd+Tab |
| Win+D | `SHOW_DESKTOP` intent → Adapter native show-desktop | — |
| Win+L | Ctrl+Cmd+Q (lock screen) | Ctrl+Cmd+Q |
| Win+E | `OPEN_FILE_MANAGER` intent → Adapter opens Finder/Explorer | — |
| Win+Shift+S | `SCREENSHOT_REGION` intent → Adapter native region capture | — |
| Win+Arrow Left | `SNAP_LEFT` intent → Adapter native snap | — |
| Win+Arrow Right | `SNAP_RIGHT` intent → Adapter native snap | — |
| Win+Arrow Up | Zoom/maximize | — |
| Win+Arrow Down | Minimize | — |
| Ctrl+Win+F | Cmd+Ctrl+F (fullscreen) | Cmd+Ctrl+F |
| Win+Shift+Left/Right | `MOVE_WINDOW_TO_DISPLAY` intent → Adapter moves window across displays | — |
| Home/End | Cmd+Home/End (scroll to top/bottom) | — |
| Ctrl+Home/End | Cmd+Up/Down | — |
| NumLock | Clear | — |
| Scroll Lock | F18 (or Shift+F18) | — |

**AppMode routing (config-driven bundle lists, not hardcoded logic):** Terminal apps (Terminal, iTerm2, WezTerm, Warp, Alacritty, Kitty, Hyper, Ghostty, VSCode) → `AppModeTerminal` (Ctrl+C = INTERRUPT, Ctrl+V = PASTE per-intent, not blanket passthrough). Remote/VM apps (RDC, Parallels, VMware, VirtualBox, JumpDesktop, Citrix) → `AppModeRemote`/`AppModeVM` (CrossOS disabled for that window).

**Implementation**:
- MVP: session/HID-level CGEventTap (no driver). Production option: DriverKit virtual-HID for full system-wide remapping.
- Uses device filters (like keymapper's `Stage`) to exclude specific keyboards.
- Uses app filters (like windows-keyboard-for-mac's `frontmost_application_if`) to apply different rules per app.

### 6.2 Plugin 2: Windows Window Management (Builtin, Go + Swift adapter)

**Actions** (from Nudge's `SnapAction` enum — 19 actions):

```
Maximize, Minimize, Fullscreen
Left Half, Right Half, Top Half, Bottom Half
Top Left, Top Right, Bottom Left, Bottom Right
Left Third, Center Third, Right Third
Left Two Thirds, Center Two Thirds, Right Two Thirds
Center, Restore
Next Display, Previous Display
```

**Shortcuts** (from Nudge):
- `Ctrl+Win+←` → Left Half
- `Ctrl+Win+→` → Right Half
- `Ctrl+Win+↑` → Maximize
- `Ctrl+Win+↓` → Minimize
- `Ctrl+Win+Shift+←` → Left Third
- `Ctrl+Win+Shift+→` → Right Two Thirds
- `Ctrl+Win+Home` → Center
- `Ctrl+Win+End` → Fullscreen
- `Ctrl+Win+PageUp` → Next Display
- `Ctrl+Win+PageDown` → Previous Display

**Implementation**:
- Uses Accessibility API (AXUIElement) — like Nudge's `WindowManager.swift`.
- Frame calculation from `NSScreen.visibleFrame` — like Nudge's `SnapZone.swift`.
- Frame history (capacity 128) for restore.
- Multi-monitor support.

### 6.3 Plugin 3: Windows Explorer UX (Plugin; uses the Finder Sync Platform Extension)

**Context menu items** (from FinderRight + newfile + menumate):

| Windows Explorer | CrossOS Menu Item | Icon |
|---|---|---|
| Right-click → New > Text Document | New > Text Document | 📄 |
| Right-click → New > Folder | New > Folder | 📁 |
| Right-click → Copy as Path | Copy as Path | 📄 |
| Right-click → Open in Terminal | Open in Terminal | ⌨️ |
| Right-click → Open in Editor | Open in Editor | ✏️ |
| Right-click → Cut | Cut | ✂️ |
| Right-click → Copy | Copy | 📄 |
| Right-click → Paste | Paste | 📋 |
| Right-click → Rename | Rename (F2) | ✏️ |
| Right-click → Properties | Get Info | ℹ️ |
| Right-click → Delete | Move to Trash | 🗑️ |
| Right-click → Send to → Compressed | Compress | 📦 |

**Implementation**:
- FIFinderSync extension in `extensions/finder-sync/` (Swift).
- Context menu built from extension pack's `manifest.json` actions.
- Actions execute as NATIVE capabilities via the Platform Capability API. Shell/process actions are opt-in Level B executable plugins only (never the default).
- Security: ActionDispatcher validates requests against local config, checks paths, max path count.
- IPC: Unix socket to Core for execution (normative — see §4.1).

### 6.4 Plugin 4: Developer UX (Script Plugin, Phase 4)

**Actions**:
- `Ctrl+Shift+Enter` → Open terminal in current directory (Finder/Codespaces/IDE).
- `Ctrl+Shift+C` → Copy full file path.
- `Ctrl+Shift+P` → Open in preferred editor (VSCode, Cursor, etc.).
- `F8` in IDE → Next error (already standard, but ensure Windows IDE shortcuts work).
- Context menu: "Open in Terminal", "Open in VS Code", "Open in Cursor".

**Implementation**:
- Script plugin running shell scripts.
- Integrates with Finder extension, IDE extensions, terminal apps.

---

## 7. UI Layer (React + TypeScript + Wails)

### 7.1 Structure

```
app/
├── wails/*              # Wails Go bindings (bridges UI ↔ Core)
├── frontend/
│   ├── src/
│   │   ├── components/   # React components (shadcn/ui)
│   │   ├── pages/        # Settings, Plugins, Hotkeys, Activity, Logs
│   │   ├── hooks/        # Wails runtime hooks
│   │   └── lib/          # utilities
│   ├── package.json
│   └── tsconfig.json
└── dist/                 # built UI assets (served by Wails)
```

### 3.6c UI Contribution API (P0 architecture principle — contribution-driven, not hardcoded)

> **CrossOS UI must be contribution-driven and extensible. Core must not hardcode
> plugin-specific UI, pages, menus, settings, or navigation. Plugins contribute UI
> through a stable UI Extension API.**
> Acceptance test: deleting a plugin from the source tree entirely → Core still
> builds and the shell UI still runs; installing a plugin → its UI appears through
> the registry with zero changes to Core UI code.

Plugin = custom logic + behavior + capabilities + configuration + UI, hosted by the
CrossOS runtime. Core provides host + primitives + lifecycle + permissions; the
plugin decides what UI it contributes. Plugin UI never touches the DOM/window/native
UI directly — it declares schema/state/actions; the shell renderer executes them.

Extension points (MVP implements the declarative tier only; Custom View is Phase 4+):

```
UI Extension Points
├── settings-page      (MVP: declarative form schema — checkbox/select/slider/button)
├── settings-section   (MVP: inject a section into an existing Core page)
├── sidebar-item       (MVP: navigation entry → plugin settings page)
├── context-menu-item  (MVP 2: Finder/right-click items)
├── command            (MVP 1+: command-palette entries)
├── status-indicator   (MVP 1+: tray/sidebar health dot)
└── custom-view        (Phase 4+: sandboxed plugin UI — NOT in MVP)
```

```go
// UIContribution — the ONLY way plugin UI reaches the shell. Declared at
// Register() time alongside rules/capabilities; rendered by the shell from
// schema, never by plugin code.
type UILocation string
const (
    UILocationSettingsPage     UILocation = "settings-page"
    UILocationSettingsSection  UILocation = "settings-section"
    UILocationSidebar          UILocation = "sidebar-item"
    UILocationContextMenu      UILocation = "context-menu-item"
    UILocationCommandPalette   UILocation = "command"
    UILocationStatus           UILocation = "status-indicator"
    UILocationCustomView       UILocation = "custom-view" // Phase 4+ only; MVP Registry rejects it
)

type UIContribution struct {
    ID         string        // "<pluginID>.<contribID>", namespaced — no collisions
    Location   UILocation
    Title      string        // sidebar label / page title / menu label
    Schema     Schema        // declarative form: checkbox/select/slider/button/actions.
                             // MVP renders ONLY from this schema.
    Visibility Condition     // e.g. "plugin.enabled && platform == macOS"
    Actions    []ActionRef   // capabilities the UI may invoke (permission-checked like any caller)
}

// Rules:
// - Every UIContribution.ID must be namespaced under its plugin ID; the shell
//   renders contributions by discovery, never by plugin-name switch/case.
// - UI-declared actions execute through the same Capability API + Permission
//   Manager as everything else — UI gets no privileged Core access.
// - Core pages (Dashboard/Keyboard/Activity/Safety/Settings) are themselves
//   contributions registered by Core at startup through the SAME Registry —
//   the shell has no hardcoded page list beyond the contribution host itself.
// - Custom-view runtime (Phase 4+, NOT MVP) must be fully specified before any
//   implementation: plugin UI lifecycle, UI ↔ plugin state sync, UI invoking
//   plugin-owned capabilities, UI → plugin events, plugin → UI state push,
//   component API, sandbox boundary, API versioning (UI API v1/v2...), resource
//   limits. The declarative tier stays independent of this. Lesson from `pi`:
//   the UI extension API is a versioned public contract — never expose the
//   internal React component tree to plugins.
```

### 7.2 Key UI Pages (Core-registered contributions — the shell discovers these via
§3.6c, it does not hardcode a page list; plugin settings pages appear the same way)

| Page | Purpose | MVP |
|---|---|---|
| **Dashboard** | Status of all plugins, enable/disable toggle, safe mode indicator, kill switch button. | MVP 0 |
| **Keyboard** | View/edit the Windows keyboard behavior matrix. App-specific overrides. | MVP 0 |
| **Activity** | Real-time event log, intent resolution trace, plugin actions. | MVP 0 |
| **Safety** | Kill switch, Reset, Safe Mode settings, ownership audit. | MVP 0 |
| **Settings** | Permissions, autostart, general config. | MVP 0 |
| **Windows** | Window management shortcuts, snap zone editor. | MVP 1 |
| **Finder** | Context menu items, extension packs, action settings. | MVP 2 |
| **Plugins** | Installed plugins, install/enable/disable/update. | MVP 1+ (local only; NO marketplace browser until Phase 4) |
| **Shortcuts** | Global keyboard shortcut manager. | later |
| **About** | Version, license, attribution, repository credits. | MVP 0 |

### 7.3 Wails Bridge

Wails provides a Go function callable from JS:
```go
// In wails/app.go
func (a *App) GetStatus() *Status { ... }
func (a *App) TogglePlugin(id string) error { ... }
func (a *App) ResetEverything() error { ... }
func (a *App) GetEventLogs() []string { ... }
```

---

## 8. Safety & Reversibility

### 8.1 Safety Principles (P0)

1. **Intercept only** → disabling removes all effects; never patch Finder, system binaries, or the kernel.
2. **Ownership tracking** → every integration records ownership metadata; only clean up what CrossOS created.
3. **Emergency kill switch (PANIC STOP)** → one click: disable interception + plugin actions + flush/close. Login item stays.
4. **Safe Mode** → enable for 30s → confirm/rollback.
5. **Reset / Uninstall** → remove login item, disable extension, clean CrossOS-owned state, verify no process remains.

### 8.2 CrossOS Integration State & Rollback

CrossOS owns rollback of CrossOS state only — never arbitrary OS state.
Before installing/enabling any OS integration:
```go
// Renamed from IntegrationSnapshot: this is not a system snapshot and cannot
// restore arbitrary OS state — it only tracks the CrossOS-owned integrations
// listed below, for rollback of those specific resources.
type IntegrationState struct {
    Timestamp    time.Time
    Integrations []IntegrationRecord
}

type IntegrationRecord struct {
    PluginID     string
    Type         string   // "loginitem", "accessibility", "cgeventtap", "findersync", "config"
    Identifier   string   // the system resource ID
    StateBefore  string   // the state before CrossOS modified it
    RollbackFunc string   // how to reverse (e.g., "remove_login_item")
}
```

### 8.3 Permission Requirements

| Feature | macOS Permission | Windows Permission |
|---|---|---|
| Keyboard observation | Input Monitoring consent | Raw Input (no admin) |
| Keyboard interception | Input Monitoring consent | WH_KEYBOARD_LL (no admin; driver = future) |
| Synthetic input | — | SendInput (UIPI: equal-or-lower IL only) |
| Window management | Accessibility (AX) | Run as admin |
| Finder context menu | Finder Sync Extension (user approves in System Settings) | Context menu handler (registry) |
| Clipboard | Accessibility or Accessibility API | — |
| App switching | Accessibility (AX) | — |

### 8.4 Non-Tech Safety Defaults

- Default config ships Windows defaults but NOTHING is enabled until the user explicitly
  clicks "Enable" per plugin (explicit approve — no auto-approve).
- Safe Mode (30s TRIAL) is ON by default for new integrations.
- Kill switch is prominent and always accessible.
- "Reset Everything" is prominent and always accessible.
- No integration is enabled until the user explicitly clicks "Enable."

---

## 9. Licensing Matrix (single source of truth — merged from the former `docs/RESEARCH.md`, now deleted)

Purpose: repo archaeology. Each repo answers 5 questions (input/event model?
context detection? config schema? plugin/extensibility? which parts are reusable
under its LICENSE?), then maps Our Feature → reuse source.

Reuse rules:
- 🟢 **COPY/ADAPT** — permissive license (MIT/Unlicense…); check LICENSE + dependency tree per file before copying.
- 🟡 **ARCHITECTURE** — study architecture/behavior only, do NOT lift code (GPL/copyleft or complex native code).
- 🟡 **BEHAVIOR/UX** — take the shortcut matrix, UX, settings model.

### 9.1 Keyboard / Input — P0

| Repo | OS | License | Takeaways | Type |
|---|---|---|---|---|
| pqrs-org/Karabiner-Elements | macOS | Unlicense | low-level remap, virtual HID, device identification | 🟢 study/adapt |
| jtroo/kanata | Win/macOS/Linux | GPL-3.0 | layers, tap-hold, macros, config engine | 🟡 architecture |
| houmain/keymapper | Win/macOS/Linux | GPL-3.0 | context-aware + per-app mapping (closest to core idea) | 🟡 architecture |
| Fuzzy-and-Fluffy/windows-keyboard-for-mac | macOS | MIT (LICENSE verified in clone) | Windows muscle-memory rules + behavior matrix | 🟡 behavior |
| venkatarangan/karabiner-mac-to-windows | macOS | MIT (LICENSE verified in clone) | deep Windows mapping, Finder behavior | 🟡 behavior |
| joeytroy/karabiner-windows | macOS | NO LICENSE — all rights reserved | Win11 shortcuts + Ghostty/VSCode/VM exclusions — behavior reference ONLY, no code copy | 🟡 behavior |
| rux616/karabiner-windows-mode | macOS | Unlicense/public domain (LICENSE verified) | Windows/Linux rules + dev exclusions | 🟡 behavior |
| Fuzzy-and-Fluffy/windsify-free | macOS | GPL (LICENSE verified in clone) | native-app keyboard layer (no Karabiner) — architecture only, NO copy | 🟡 architecture |
| shootdaj/ke-custom | macOS | NO LICENSE (personal-use config) | physical modifier remap — behavior reference ONLY, no code copy | 🟡 behavior |
| raxigan/pcfy-my-mac | macOS | MIT | overall base: Karabiner + AltTab + Rectangle + setup CLI | 🟢 adapt |
| alper-han/CrossMacro | Win/macOS/Linux | GPL-3.0 | Profiles → Triggers → Workflow → Actions; shared GUI+CLI engine | 🟡 architecture |
| hammerspoon/hammerspoon | macOS | MIT (LICENSE verified in clone) | module architecture, scripting API, event/window/hotkey model | 🟡 architecture |

### 9.2 Window management / switching

| Repo | License | Takeaways | Type |
|---|---|---|---|
| lwouis/alt-tab-macos | GPL-3.0 | hook → enumerate windows → filter → MRU → switcher UI → focus | 🟡 architecture |
| ramonwessels/rectangle | MIT (LICENSE verified in clone) | Accessibility API, frame calc, shortcuts, snap zones | 🟡 architecture |
| mikusnuz/nudge | MIT | 19 window actions, customizable shortcuts, drag-to-snap, multi-monitor — read BEFORE Rectangle | 🟢 adapt |
| asmvik/yabai | MIT | windows/spaces/displays internals (heavyweight, study) | 🟢 study |
| ianyh/Amethyst | MIT | window state, layouts, keyboard commands, config, accessibility | 🟢 study |
| saforem2/chunkwm | MIT (LICENSE verified in clone) | plugin architecture splitting WM into separate modules | 🟡 architecture |
| LGUG2Z/komorebi | custom | engine → CLI → JSON schema → TCP socket → external clients (Core↔Plugin model) | 🟡 architecture |

### 9.3 Finder / Context menu — P0

| Repo | License | Takeaways | Type |
|---|---|---|---|
| mariusgm/newfile | MIT | SwiftUI host + Finder Sync Extension + custom types/templates/ordering/submenu — clone first | 🟢 adapt |
| Hibrielle/menumate | MIT | Actions + editable scripts + enable/disable/reorder + file-type scope + Extension Packs (Git repos) — plugin-ecosystem blueprint | 🟢 adapt heavily |
| wflixu/RClick | GPL-3.0 (LICENSE verified in clone — DOWNGRADED from study) | Finder Sync comparison: New File, Copy Path, Open With, Delete, Hide/Unhide, AirDrop — architecture/behavior only, NO copy | 🟡 architecture |
| rowild/MoreMenu | NO LICENSE — all rights reserved | minimal New Text/Markdown + open file after creation — behavior reference ONLY, no code copy | 🟡 behavior |
| sherman-yang/mac-finder-menu | MIT (LICENSE verified in clone) | FINDINGS.md: top-level menu → Finder Sync Extension is the official route; Services/Quick Actions can't be promoted | 🟡 behavior |
| funny-dog/FinderRight | MIT (LICENSE verified in clone) | full dev workflow: New File + Copy Path + Terminal/Editor + Cut/Paste + Compress | 🟡 behavior |
| InfinityBowman/FinderTools | NO LICENSE — all rights reserved | minimal Ghostty/Code/New Text File — behavior reference ONLY, no code copy | 🟡 behavior |

### 9.4 Launcher / hotkey / automation

| Repo | License | Takeaways | Type |
|---|---|---|---|
| zjy4fun/HotkeyLauncher | NO LICENSE — all rights reserved | global hotkey, JSON import/export, SwiftUI/AppKit/Carbon — behavior reference ONLY, no code copy | 🟡 architecture |
| conversun/tinycast-cn | AGPL-3.0 (LICENSE verified in clone) | global + per-app hotkey, launcher, file search, clipboard — architecture only, NO copy | 🟡 architecture |
| lgrammel/app-launcher | MIT (LICENSE verified in clone) | forkable base: global hotkey, app discovery, search, system actions, tests | 🟢 study |

### 9.5 macOS UI / device customization (future plugins)

| Repo | License | Takeaways | Type |
|---|---|---|---|
| FelixKratz/SketchyBar | GPL-3.0 | core → components → plugins/scripts → events; composable desktop | 🟡 architecture |
| linearmouse/linearmouse | MIT (LICENSE verified in clone) | mouse/trackpad modules + tests (extend to Keyboard+Mouse+Trackpad) | 🟡 architecture |
| MonitorControl/MonitorControl | MIT (License.txt verified in clone) | hardware capability, custom shortcuts, OSD, helper process → Display Controls plugin | 🟡 architecture |

### 9.6 Windows side (user-expectation UX + plugin model)

| Repo | License | Takeaways | Type |
|---|---|---|---|
| ramensoftware/windhawk | GPL-3.0 | 🔥 ARCHITECTURE REFERENCE: Core → Mod → Settings → Marketplace → Install/Enable/Disable/Update | 🟡 architecture |
| ramensoftware/windows-11-taskbar-styling-guide (+ windhawk-mods, Explorer Styler, Start Menu Styler) | NO LICENSE — all rights reserved | a plugin can customize a single capability without touching core — behavior reference ONLY, no code copy | 🟡 behavior |
| files-community/files | MIT+MPL | Windows Explorer UX expectations (don't copy the UI) | 🟡 behavior |

### 9.7 Plugin/UI contribution model (declarative UI + commands + scriptability)

| Repo | License | Takeaways | Type |
|---|---|---|---|
| earendil-works/pi | MIT (LICENSE verified in clone) | extension loader/discovery, versioned extension+UI API, commands/tools/events, plugin-contributed widgets/views — the philosophy reference for §3.6c UI contributions. API is a versioned public contract, never the internal component tree | 🟡 architecture |
| hluk/CopyQ | GPL-3.0 | user-defined custom commands + global shortcuts + context menus + scripting API (commands/settings/network/filesystem) without touching core — closest model for CrossOS Plugin → register command/shortcut/menu/settings/UI | 🟡 architecture |

### 9.8 Our Feature → source mapping

- Keyboard Engine → Keymapper / Kanata (arch) + windows-keyboard-for-mac (behavior)
- macOS HID → Karabiner-Elements (study)
- Window → Nudge / Rectangle / yabai
- AltTab → AltTab (arch)
- Finder → NewFile / MenuMate (adapt)
- Automation → CrossMacro / Hammerspoon (arch)
- Plugin Marketplace → Windhawk / MenuMate / komorebi (arch)
- Plugin UI contributions → pi (philosophy + versioned UI API) + CopyQ (commands/shortcuts/menus/scripting without touching core)

### 9.9 P0 — clone & read first (10 repos)

1. houmain/keymapper 2. jtroo/kanata 3. Fuzzy-and-Fluffy/windows-keyboard-for-mac
4. mariusgm/newfile 5. Hibrielle/menumate 6. mikusnuz/nudge 7. lwouis/alt-tab-macos
8. alper-han/CrossMacro 9. ramensoftware/windhawk 10. LGUG2Z/komorebi
\+ pqrs-org/Karabiner-Elements, raxigan/pcfy-my-mac, asmvik/yabai

**Verification rule**: every row with an unverified license must be resolved before COPY — each reused repo records `Repo / License / License verified at / Dependencies / Direct reuse? / Attribution required?`. MIT means direct reuse possible + retain copyright + inspect dependencies. **Strict rule**: No GPL/copyleft code is copied into the MIT-licensed CrossOS core. GPL repos are studied for architecture only. Only MIT/Unlicense repos contribute code (with per-file LICENSE + dependency tree checks).

### 9.10 Implementation Reuse Matrix (file-level — the anti-Frankenstein contract)

§9.1–9.7 say WHAT each repo teaches. This table says EXACTLY what may be taken,
from which files, through which CrossOS contract, into which destination. A
developer implements ONLY the rows below — no wholesale repo copies, no
"read 20 repos and guess". Every COPY/ADAPT row requires, before code lands:

```
repo URL + pinned commit/tag + LICENSE text + dependency licenses +
exact source files + copyright headers retained + modification log +
ATTRIBUTION entry + target CrossOS contract + divergence reason
```

Per-file attribution lives in `third_party/<repo>/` (LICENSE + NOTICE +
ATTRIBUTION.md), or per-file header where the project convention requires it.

### 9.11 Direct-reuse flag, attribution format, merge gate (locked)

**Direct code reuse** (disambiguates COPY vs ADAPT — "ADAPT" alone must never be
read as "copy the file then edit"):

| Repo | Direct code reuse? | Meaning |
|---|---|---|
| newfile | YES | FIFinderSync subclass + filename/filetype helpers may be copied, then adapted to CrossOS IPC/contracts |
| nudge | YES | geometry/AX helpers (`WindowManager`, `SnapZone`, `SnapAction`, `AccessibilityHelper`, `DisplayHelper`) may be copied, then stripped of app UI |
| menumate | PARTIAL | manifest/rule/config/IPC *concepts + schema shapes* may be ported; script executor is NOT copied (CrossOS executes native capabilities; shell is Level B) |
| windows-keyboard-for-mac, karabiner-mac-to-windows, karabiner-windows-mode | DATA ONLY | behavior matrices re-expressed as CrossOS Rules→Intents; no source files copied |
| pcfy-my-mac | SCRIPTS ONLY | installer sequencing reference; nothing ships in the Core binary |
| Everything 🟡 (yabai, Amethyst, rectangle, app-launcher, hammerspoon, linearmouse, MonitorControl, files, pi, chunkwm) | NO | concepts/behavior/UX only — reimplement through CrossOS contracts |

**Mandatory `ATTRIBUTION.md` entry format** (one block per copied/adapted file):

```
Source repository:
Source commit:
Source file:
Original license:
Original copyright:
CrossOS destination:
Modification:
Reason for modification:
CrossOS license:
```

Example:

```
Source repository: mikusnuz/nudge
Source commit: <SHA>
Source file: Nudge/Core/SnapZone.swift
Original license: MIT
Original copyright: Copyright (c) 2026 mikusnuz
CrossOS destination: platform/darwin/window/
Modification: rewritten to expose Window Capability API
Reason for modification: remove Nudge-specific UI/preferences/state
CrossOS license: MIT
```

**Merge gate** (enforced in review/CI for any PR containing reused OSS code):

```
PR contains reused OSS code
        ↓
source repo + pinned commit recorded? ──NO──→ reject
        ↓ YES
license verified (not GPL/copyleft/no-license)? ──NO──→ reject
        ↓ YES
transitive dependency licenses checked? ──NO──→ reject
        ↓ YES
copyright/NOTICE preserved + ATTRIBUTION entry added? ──NO──→ reject
        ↓ YES
merge
```

CrossOS is MIT: this gate is load-bearing, not advisory. GPL/copyleft and
no-license sources can never pass it — they stay architecture/behavior-only.

| Repo (license) | Feature | Exact source files / modules | Primitive to extract | Mode | CrossOS destination | CrossOS contract | Divergence / rewrite reason |
|---|---|---|---|---|---|---|---|
| mikusnuz/nudge (MIT) | Window snapping | `Nudge/Core/WindowManager.swift`, `Nudge/Core/SnapZone.swift`, `Nudge/Core/SnapAction.swift`, `Nudge/Helpers/AccessibilityHelper.swift`, `Nudge/Helpers/DisplayHelper.swift` | frame calc, snap zones, frame history, AX move/resize calls, multi-monitor logic | 🟢 ADAPT | `platform/darwin/` (AX calls) + window capability handlers | `window.*` capabilities (§3.12) via Adapter | Nudge is a full app (menubar UI, overlay, analytics); take ONLY geometry+AX primitives. Hotkey/prefs/analytics stay behind. |
| mariusgm/newfile (MIT) | Finder Sync + file creation | `Extension/FinderSync.swift`, `Shared/FilenameGenerator.swift`, `Shared/FileTypeEntry.swift`, `Shared/SettingsStore.swift`, `Shared/SeedPresets.swift` | FIFinderSync subclass pattern, menu building, filename generation, file-type presets | 🟢 COPY/ADAPT | `extensions/finder-sync/` + `platform/darwin/` | `finder.menu`, `filesystem.createFile`, `filesystem.createFolder` | Host app UI (SwiftUI prefs, template editor) is NOT copied — CrossOS settings UI is declarative (§3.6c). |
| Hibrielle/menumate (MIT) | Plugin/manifest/IPC/security | `Core/Sources/MenuMateCore/PackManifest.swift`, `Core/Sources/MenuMateCore/RuleMatcher.swift`, `Core/Sources/MenuMateCore/ConfigStore.swift`, `Core/Sources/MenuMateCore/PackInspector.swift`, `Core/Sources/MenuMateCore/IPC.swift`, `Core/Sources/MenuMateCore/ShellRunner.swift`, `Core/Sources/MenuMateCore/ActionInterface.swift`, `FinderExtension/FinderSync.swift` | manifest schema, rule-matching semantics, config store, pack inspection, IPC framing | 🟢 ADAPT | `core/` (registry, matcher, config) | Plugin Registry (§3.6), Rule Engine, Permission Manager | MenuMate executes user scripts as actions; CrossOS executes native capabilities by default — script execution is Level B gated. Port concepts, not the executor verbatim. |
| Fuzzy-and-Fluffy/windows-keyboard-for-mac (MIT) | Shortcut behavior matrix | Karabiner complex-modification JSONs + behavior docs in repo | per-app rule matrix (which Ctrl+key maps to what, terminal/VM/browser exclusions) | 🟡 BEHAVIOR | `plugins/windows-keyboard/` rules | `Rule` + `Intent` (§3.5) | Behavior data only — re-expressed as CrossOS Rules→Intents, never executed as Karabiner config. |
| venkatarangan/karabiner-mac-to-windows (MIT) | Deep Windows mapping | Karabiner JSON rules | Finder behavior, extended key coverage | 🟡 BEHAVIOR | `plugins/windows-keyboard/` rules | `Rule` + `Intent` | Same as above — data in, CrossOS contracts out. |
| rux616/karabiner-windows-mode (Unlicense) | Windows/Linux rules | Karabiner JSON rules | dev-tool exclusions, mode toggle semantics | 🟡 BEHAVIOR | `plugins/windows-keyboard/` rules | `Rule` + `Intent` | Same as above. |
| raxigan/pcfy-my-mac (MIT) | Setup orchestration | setup scripts/CLI | install-order, app provisioning sequence | 🟢 ADAPT (scripts only) | `scripts/` | build/install tooling, NOT runtime | Reference for installer UX only — nothing ships in the Core binary. |
| asmvik/yabai (MIT) | AX/Space internals | window/space/display query code | AX attribute names, space/display enumeration semantics | 🟡 ARCHITECTURE | `platform/darwin/` | `window.read` capability internals | yabai leans on scripting-addition/SIP-disabled paths — CrossOS uses ONLY SIP-clean AX APIs. Study, don't port. |
| ianyh/Amethyst (MIT) | Layout management | layout algorithms | tiling layout math, window-state model | 🟡 ARCHITECTURE | future layout capability | `window.*` (Phase 2+) | Layout engine is a future plugin, not MVP — concepts only. |
| ramonwessels/rectangle (MIT) | Snap zones UX | snap-zone definitions, shortcut defaults | zone geometry, default shortcut set | 🟡 BEHAVIOR | `plugins/windows-window/` | `window.*` capabilities | Nudge is the code source; Rectangle is the UX cross-check. |
| lgrammel/app-launcher (MIT) | Launcher base | global-hotkey + app-discovery code | hotkey registration, app enumeration | 🟢 STUDY | Phase 2 launcher plugin | `app.launch` | Forkable base — evaluate at Phase 2, not MVP. |
| hammerspoon/hammerspoon (MIT) | Module API model | Lua module/event/window/hotkey APIs | extension-point taxonomy (what surfaces a automation host exposes) | 🟡 ARCHITECTURE | `core/` plugin API design | Plugin Registry (§3.6) | API taxonomy reference — CrossOS API is Go/JSON-RPC, not Lua. |
| linearmouse/linearmouse (MIT) | Device modules + tests | per-device module structure | module-per-device pattern, test layout | 🟡 ARCHITECTURE | future device plugins | Capability Registry pattern | Pattern reference for Keyboard+Mouse+Trackpad split. |
| MonitorControl/MonitorControl (MIT) | Display controls | helper-process + OSD + shortcut code | privileged-helper pattern, OSD pattern | 🟡 ARCHITECTURE | future display plugin | `shell.execute` / helper model | Pattern only — display DDC is out of MVP scope. |
| files-community/files (MIT+MPL) | Explorer UX expectations | UX flows (navigation, context actions) | user-expectation model for file UX | 🟡 BEHAVIOR | `plugins/finder-ux/` design | `finder.menu`, `filesystem.*` | UX expectations only — never copy UI. Note MPL file-level copyleft on their side; we take ideas, not files. |
| earendil-works/pi (MIT) | Extension + versioned UI API philosophy | extension loader, contribution points, UI API versioning | contribution-driven UI, API-as-public-contract discipline | 🟡 ARCHITECTURE | §3.6c design | `RegisterUI` / `UIContribution` | Philosophy + contract discipline — no code port. |
| saforem2/chunkwm (MIT) | WM plugin split | plugin-module split | process/module boundary pattern | 🟡 ARCHITECTURE | `core/` module boundaries | Plugin Runtime | Historical reference (chunkwm → yabai lineage). |

Architecture-only rows (NEVER copied — GPL/copyleft or closed): jtroo/kanata
(LGPL-3.0 — input config engine semantics), houmain/keymapper (GPL-3.0 —
context detection), lwouis/alt-tab-macos (GPL-3.0 — window enumeration flow),
alper-han/CrossMacro (GPL-3.0 — trigger→workflow model), ramensoftware/windhawk
(GPL-3.0 — marketplace model), FelixKratz/SketchyBar (GPL-3.0 — composable bar),
hluk/CopyQ (GPL-3.0 — commands/scripting model), conversun/tinycast-cn
(AGPL-3.0), Fuzzy-and-Fluffy/windsify-free (GPL), wflixu/RClick (GPL-3.0),
LGUG2Z/komorebi (custom license — JSON schema + TCP client model).

No-license rows (behavior reference ONLY — no code leaves these repos):
joeytroy/karabiner-windows, shootdaj/ke-custom, rowild/MoreMenu,
InfinityBowman/FinderTools, zjy4fun/HotkeyLauncher,
ramensoftware/windows-11-taskbar-styling-guide.

---

## 10. Implementation Phases

Roadmap numbering (locked — MVP 0/1/2 are phases, not floating milestones):

```
Phase 0 — Foundation (contracts + spikes)
Phase 1 — MVP 0: Keyboard
Phase 2 — MVP 1: Window
Phase 3 — MVP 2: Finder
Phase 4 — Ecosystem (local → git → signed registry → marketplace)
Phase 5 — Distribution & Polish
Phase 6 — Future (backlog)
```

### Phase 0: Foundation — lock the 4 contracts first (2-3 weeks)
- [ ] Wails v2-stable vs v3-beta spike (1–2 days, NO scaffolding before it) — decide on CrossOS needs: tray, settings, multi-window, native menu, lifecycle, autostart, signing, macOS/Windows packaging, Go service integration. Never pick v3 just for novelty.
- [ ] Native spikes BEFORE Go Core (each must pass or architecture is reworked):
  - [ ] Spike A (macOS CGEventTap): intercept Ctrl+C → suppress → emit replacement → recover after timeout-disable
  - [ ] Spike B (Windows WH_KEYBOARD_LL): suppress Ctrl+C → SendInput replacement → elevated vs non-elevated targets (UIPI)
  - [ ] Spike C (AX/UIA): get focused app/window → move/resize window
  - [ ] Spike D (Finder Sync): menu → Unix socket → daemon-down graceful fallback (hide, never block)
- [ ] Create Go core project structure (`core/` + `pkg/`)
- [ ] **P0 contracts (must be reviewed before any plugin code):**
  - [ ] Plugin API v1 (manifest + lifecycle + `Decision`: PASS/CONSUME/REPLACE) + apiVersion
  - [ ] Intent model + Platform Capability API (Intent → Action → Capability → Adapter)
  - [ ] Conflict resolution (Priority × Scope × Specificity; most-specific match wins)
  - [ ] Safety model (ownership rollback, TRIAL trial-state, explicit-approve permissions)
- [ ] Deterministic EventRouter behind `EventBus` interface (NOT generic pub/sub yet)
- [ ] Keyboard state machine (keydown/keyup/modifier/repeat/dead-key/IME/synthetic-device)
- [ ] Context cache (updated on app/window/selection change; keydown path reads cache, <1ms)
- [ ] Recorder hook points (tap every stage: event → context → rule → intent → action → result)
- [ ] Config manager with schema validation
- [ ] IPC server (JSON-RPC over Unix socket)
- [ ] Core daemon lifecycle (init, run, stop, safe mode)
- [ ] macOS adapter skeleton (C-ABI bridge → ObjC++/Swift: session/HID CGEventTap, AX bridge; build/linking/signing locked here)
- [ ] Basic UI shell (Wails + React + shadcn)
- [ ] Safety layer (kill switch, reset everything, ownership rollback)
- [ ] Observe/dry-run mode (log `Physical → Context → Rule → Intent → Would-execute` without executing)

### Phase 1 — MVP 0: Core + Keyboard only (3-4 weeks)
- [ ] **Plugin: Windows Keyboard** (builtin + declarative rules)
  - Session/HID CGEventTap on macOS (DriverKit deferred to post-MVP)
  - Intent-first matrix (~10 shortcuts first): Ctrl+C/V/X/A, F2, Alt+F4 (`CLOSE_WINDOW`),
    Alt+Tab, Win+Left/Right — validated in Finder, text editor, browser, Terminal
  - Terminal/Remote/VM exclusions (Terminal, iTerm2, WezTerm, Warp, Alacritty, Kitty, Hyper,
    Ghostty, VSCode; RDC, Parallels, VMware, VirtualBox)
  - Device filtering, app-category detection (config-driven, not deep-hardcoded)
  - Event Recorder + Replay for keyboard traces
  - Config UI page

### Phase 2 — MVP 1: + Window management (3 weeks)
- [ ] **Plugin: Windows Window Management** (builtin)
  - AXUIElement manipulation, snap frames (from Nudge), frame history, multi-monitor
  - Level B executable-plugin runtime (out-of-process JSON-RPC, crash isolation, health states)
- [ ] Permission prompts (non-tech friendly, explicit approve), activity log

### Phase 3 — MVP 2: + Finder UX (3 weeks)
- [ ] **Plugin: Finder UX** (declarative rules + menus; Finder Sync Platform Extension = minimal IPC client).
  Note: "Extension Pack" = package/distribution format only — never the name for the native Finder Sync Platform Extension.
  - Context menu via native capabilities (`filesystem.createFile`, `clipboard.copyPath`,
    `app.open`, `terminal.openAt`, `file.moveToTrash`); shell is opt-in Level B only
  - Extension-pack loading (manifest parser), path validation, undeclared-file inspection

### Phase 4: Plugin Ecosystem — staged (local → git → signed registry → marketplace)
- [ ] Extension pack marketplace (Git-based)
- [ ] Plugin install/enable/disable/update lifecycle + health states (DISABLED/TRIAL/HEALTHY/ENABLED)
- [ ] Plugin config schema UI (auto-generated forms)
- [ ] **Plugin 4: Developer UX**
  - Terminal-in-directory
  - Open in editor
  - IDE shortcuts
- [ ] **Plugin 5: Launcher** (global hotkey launcher)
- [ ] Plugin compatibility layer (version checking)

### Phase 5: Distribution & Polish (2 weeks)
- [ ] macOS app bundling (signed .app, notarized)
- [ ] Windows MSI installer (signed)
- [ ] Auto-update mechanism
- [ ] Documentation site
- [ ] CI/CD pipeline (GitHub Actions)
- [ ] User onboarding flow

### Phase 6: Future (backlog)
- Virtual HID driver integration (Kernel DriverKit) for full system-wide remapping
- Cross-Platform: Linux support (X11 + Wayland)
- AI assistance (intent inference from ambiguous input)

---

## 11. Testing Strategy

### Unit Tests (Go)
- EventRouter (behind EventBus interface): deterministic ordering on the **decision
  path** (synchronous, bounded, inside the native callback — §Data Flow steps 1-4).
  `EventBus` itself names the **observation path only** (async, best-effort: recorder,
  activity log, plugin observers) — it is NOT a queue the decision path sits behind,
  and "no-drop" applies to the observation path's own accounting (e.g. detecting when
  it had to drop a record under backpressure), never to whether a keyboard event
  reaches the fast matcher.
- Context resolver: app categorization, window parsing
- Intent resolver: rule matching, behavior matrix correctness
- Rule engine: filter evaluation (app, UTI, device, selection count)
- Plugin runtime: load/unload, hook dispatch, capability registration
- Safety: ownership rollback, TRIAL trial-state, safe mode timer
- Config: schema validation, merge priority

### Integration Tests
- End-to-end: input event → intent → action (mock adapter)
- Plugin lifecycle: install, enable, disable, update, uninstall
- Safety: kill switch interrupts all processing
- Config: overrides propagate correctly

### Platform-Specific Tests
- macOS: FIFinderSync context menu visibility, AX window manipulation
- Windows: WH_KEYBOARD_LL suppress/replace, SendInput output (incl. UIPI elevated-target case)

### Reference Tests (behavioral matrix — Shortcut × Application × Input state × OS state)
- Keyboard: shortcuts × {Finder, Browser, VSCode, Terminal, Remote Desktop} ×
  {keydown, keyup, repeat, modifier-held, IME} — recorded traces replayed deterministically
- Window: snap actions × multi-monitor × frame history
- Finder: menu items × file/folder/empty selection
- Recorder/Replay: every stage logged
  (`KeyDown Ctrl → KeyDown C → Context: Ghostty → Rule: windows-keyboard.copy → Intent: COPY → Action: macOS.copy → result`)
  and replayable without live input

---

## 12. Build & Distribution

### Project Structure
```
crossos/
├── core/
│   ├── go.mod
│   ├── cmd/crossos/
│   ├── pkg/
│   └── internal/adapter/     # CGO bridge to platform adapters
├── platform/
│   ├── darwin/               # ObjC++/Swift via C ABI (CGEventTap, AX, FIFinderSync)
│   │   └── extensions/finder-sync/   # Swift FIFinderSync Extension
│   └── windows/              # small native helper only (hook + raw input + sendinput; logic stays in Go)
├── plugins/                  # plugin manifests + scripts
│   ├── windows-keyboard/
│   │   └── plugin.json
│   ├── windows-window/
│   │   └── plugin.json
│   ├── finder-ux/            # extension pack
│   │   ├── manifest.json
│   │   └── actions/
│   └── developer/
│       └── plugin.json
├── app/                      # Wails app (Go + React frontend)
│   ├── wails/
│   ├── frontend/
│   │   ├── src/
│   │   └── package.json
│   └── build/
├── extensions/
│   └── finder-sync/          # Swift Finder Sync Extension (Xcode project)
├── docs/                       # implementation docs (research matrix lives in §9 of this file)
├── COMPREHENSIVE_PLAN.md     # this file (source of truth, root)
├── .github/
│   ├── workflows/
│   │   ├── build.yml
│   │   ├── test.yml
│   │   └── release.yml
├── LICENSE
├── README.md
└── tmp/research/             # gitignored reference clones
```

### Build Commands
```bash
# Build core (Go)
cd core && go build ./...

# Build macOS adapter (Swift)
cd platform/darwin && swift build

# Build Windows native helper → artifact: crossos-keyboard-win.dll
# (hook + raw input + sendinput ONLY; all decision logic stays in Go, reached
# via C ABI from internal/adapter). CMake builds just this thin DLL, nothing more.
cd platform/windows && cmake . && make   # outputs crossos-keyboard-win.dll

# Build UI (React)
cd app/frontend && npm run build

# Build Wails app
cd app && wails build

# Build Finder extension (requires Xcode)
cd extensions/finder-sync && xcodebuild

# Build everything
./scripts/build.sh
```

### CI/CD
- GitHub Actions
- Runs on Push + PR
- Steps: lint, test, build (macOS + Windows), artifact upload
- Release: signed artifacts, auto-update manifest

### Dependencies

| Dependency | Version | License | Purpose |
|---|---|---|---|
| Go | current stable | BSD | Core engine |
| Wails | v2 stable default; v3 only if spike justifies | MIT | Desktop shell |
| React | current stable | MIT | UI framework |
| Tailwind CSS | current stable | MIT | Styling |
| shadcn/ui | latest | MIT | Component library |
| Swift | current Xcode toolchain | Apache 2.0 | macOS adapter |
| Xcode | current stable | - | Finder extension |

---

## 13. Repository Credits

All reference repos cloned into `tmp/research/` for code archaeology:

**Keyboard/Input:** Karabiner-Elements, kanata, keymapper, windows-keyboard-for-mac, karabiner-mac-to-windows, karabiner-windows, karabiner-windows-mode, windsify-free, ke-custom, pcfy-my-mac, CrossMacro, hammerspoon

**Window Management:** alt-tab-macos, rectangle, nudge, yabai, Amethyst, chunkwm, komorebi

**Finder/Context Menu:** newfile, menumate, RClick, MoreMenu, mac-finder-menu, FinderRight, FinderTools

**Launcher/Hotkey:** HotkeyLauncher, tinycast-cn, app-launcher

**Device/UI:** SketchyBar, linearmouse, MonitorControl

**Windows Side:** windhawk, windows-11-taskbar-styling-guide, files

---

## 13b. Architecture Decisions (locked — moved out of Open Questions)

| # | Decision | Resolution |
|---|---|---|
| D1 | Keyboard interception mechanism | Session/HID CGEventTap (macOS) + WH_KEYBOARD_LL (Windows) for MVP; DriverKit/driver post-MVP. Proved by Spikes A+B before Core. |
| D2 | Finder extension transport | Unix socket (normative). DistributedNotificationCenter rejected for command transport. |
| D3 | Plugin conflict resolution | Priority × Scope × Specificity, most-specific match wins; verdict enum CONSUME / PASS / REPLACE (§3.5b). |

## 14. Open Questions / Risks (genuinely open items ONLY — resolved items live in §13b)

| # | Question | Options | Recommendation |
|---|---|---|---|
| 3 | How to handle multi-user / multiple macOS instances? | Session-aware context | Design context resolver to include session info |
| 4 | How to handle updates to extension packs? | Git pull + reload vs bundled version | Git-based auto-update with user approval |
| 6 | Apple Silicon vs Intel differences? | Architecture-specific builds | Build for arm64 + amd64 |
| 7 | Notarization requirements for Finder extension? | Required for distribution | Plan for Apple Developer account, hardened runtime |
