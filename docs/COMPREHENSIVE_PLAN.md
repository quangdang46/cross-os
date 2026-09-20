# CrossOS — Comprehensive Architecture & Implementation Plan

> **Goal:** Move from Windows to macOS (and beyond) without relearning how to use your computer. Create a muscle-memory layer on top of the real OS.
>
> **Architecture:** Pluggable plugins + native adapters. Non-technical users first, developers second.
>
> **Tech Stack:**
> - Core: Go (event bus, context/intent engine, plugin runtime, permissions, config, IPC)
> - Desktop shell: Wails (Go + native WebView, no bundled Chromium)
> - UI: React + TypeScript (+ Tailwind/shadcn)
> - macOS adapter: Swift/ObjC at the boundary only (CGEvent, AX, FIFinderSync)
> - Windows adapter: Minimal Win32/C++ (Raw Input, SendInput)
> - Config: JSON/YAML
> - License: MIT

---

## 1. Vision & Principles

### Vision
CrossOS is a **desktop UX compatibility layer** — not a VM, not a shell replacement, not a theme engine. It intercepts input events, resolves the runtime context (what app is active, what's selected, where the cursor is), translates user intent into the OS-native action, and dispatches it through a plugin system. The underlying OS stays 100% native; only muscle-memory shortcuts and UI affordances are remapped.

### Design Principles
| Principle | What it means |
|---|---|
| Intercept, don't patch | Only remap/intercept input events. Never patch Finder, system binaries, or the kernel. Disabling = zero effect. |
| Ownership | Every integration records ownership metadata. Cleanup removes only what CrossOS created. |
| Reversibility | All state changes are reversible. Every action has a rollback path. |
| Non-tech first | Default config "just works" for typical Windows migrants. No setup required for MVP. |
| Dev second | Developers can write plugins, extend rules, and customize behavior. |
| Kill switch | Emergency deactivation disables hooks → stops daemon → disables plugins → removes login item → verifies no process remains. |
| Safe mode | New integrations enabled for 30s; user must confirm or auto-rollback. |

---

## 2. Architecture Overview

```
┌─────────────────────────────────────────────────────────┐
│                      Input Layer                          │
│  (Global Hotkey / Keyboard / Mouse Events)              │
├─────────────────────────────────────────────────────────┤
│                  CrossOS Core (Go)                        │
│                                                         │
│  ┌─────────────┐   ┌──────────────┐   ┌──────────────┐   │
│  │  Event Bus   │──▶│ Context      │──▶│ Intent       │   │
│  │  (channel)   │   │ Resolver     │   │ Resolver     │   │
│  └─────────────┘   └──────────────┘   └──────────────┘   │
│                         │                  │             │
│                         └──────┬───────────┘             │
│                                ▼                         │
│                     ┌─────────────────┐                  │
│                     │  Rule Engine    │                  │
│                     │  (matchers)     │                  │
│                     └─────────────────┘                  │
│                           │                              │
│                           ▼                              │
│                    ┌──────────────┐                     │
│                    │ Plugin       │                     │
│                    │ Runtime      │                     │
│                    └──────────────┘                     │
│                           │                              │
│                    ┌──────┴──────┐                       │
│                    │ Permission  │                       │
│                    │ Manager     │                       │
│                    └─────────────┘                       │
├─────────────────────────────────────────────────────────┤
│                    Adapter Layer                         │
│  macOS: Swift/ObjC (CGEvent, AX, FIFinderSync)           │
│  Windows: C++ (Raw Input, SendInput)                      │
├─────────────────────────────────────────────────────────┤
│                    Plugin Layer                          │
│  - Built-in plugins (Go)                                 │
│  - Script plugins (shell/python/etc)                     │
│  - Extension packs (git repo + manifest)                 │
└─────────────────────────────────────────────────────────┘
```

### Data Flow (per event)

1. **Input Layer** captures raw event (keyboard, mouse, hotkey).
2. **Event Bus** publishes it as a typed `Event` (KeyPress, Hotkey, MouseClick, etc.).
3. **Context Resolver** enriches with runtime context:
   - Active application (bundle ID / executable name)
   - Frontmost window title
   - Active window class
   - Focused element (AX-focused UIElement)
   - Cursor position / screen
   - Selected items (for Finder/file ops)
   - Device identity (for keyboard remapping)
4. **Intent Resolver** matches event+context against rules → produces an `Intent` (e.g., `Intent{Action: "copy", Modifiers: "cmd"}`).
5. **Rule Engine** evaluates intent through plugin rules, considering enable/disable filters, application exclusions, device filters.
6. **Plugin Runtime** loads the matching plugin, passes the intent.
7. **Permission Manager** checks if the action is allowed for this plugin/context.
8. **Adapter Layer** executes the action on the native OS.

### Event-Driven Plugin API (inspired by Pi / windhawk / komorebi)

The plugin model is event-driven and capability-based. Plugins declare what they can do and subscribe to what they want to see. This mirrors the Pi extension API model described in the ChatGPT conversation:

- **Plugin manifest** (`plugin.json`): declares hooks subscribed, capabilities registered, permissions required, config schema.
- **Hooks**: `onIntent`, `onContext`, `onEvent`, `onLifecycle`.
- **Capabilities**: plugins register capability handlers (e.g., `"keyboard.intercept"`, `"window.move"`, `"finder.contextMenu"`).
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

The EventBus is a publish-subscribe channel. Plugins can subscribe to events by type. The event bus is the "single source of truth" — all input flows through it.

### 3.3 Context Resolver

**Source:** keymapper `FocusedWindow` classes, menumate `RuleMatcher`, nudge AX patterns.

```go
type Context struct {
    Event       Event
    Application ApplicationInfo
    Window      WindowInfo
    Cursor      CursorInfo
    Selection   SelectionInfo
    Device      DeviceInfo
    Session     SessionInfo   // login state, power state
}

type ApplicationInfo struct {
    BundleID      string   // macOS: com.apple.finder, com.microsoft.teams
    Executable    string   // process name
    PID           int
    DisplayName   string
    IsRemote      bool     // VM/RDP/Remote session
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
type Intent struct {
    Action   string            // "copy", "cut", "paste", "rename", "closeWindow", "quitApp", "snapWindow", "newFile", "openTerminal", etc.
    Modifiers map[string]bool   // which logical modifiers are held
    Source   string            // "keyboard", "hotkey", "contextMenu"
    Parameters map[string]any   // action-specific params
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

### 3.6 Plugin Runtime

**Source:** windhawk's Mod system, komorebi's external client model, menumate's extension pack system.

```go
type Plugin interface {
    ID() string
    Name() string
    Version() string
    Init(config json.RawMessage) error
    OnIntent(intent *Intent) error
    OnEvent(event *Event) error
    OnLifecycle(state LifecycleState) error
    Capabilities() []Capability
    Manifest() PluginManifest
}

type Capability struct {
    Name        string   // e.g. "keyboard.intercept", "window.manage", "finder.contextMenu"
    Description string
}

type PluginManifest struct {
    ID            string            `json:"id"`
    Name          string            `json:"name"`
    Version       string            `json:"version"`
    Entry         string            `json:"entry"`          // path to executable or Go plugin
    Type          PluginType        `json:"type"`           // "builtin", "script", "extension"
    Permissions   []Permission      `json:"permissions"`
    Capabilities  []Capability      `json:"capabilities"`
    ConfigSchema  json.RawMessage   `json:"config_schema"`
    Hooks         []Hook            `json:"hooks"`
    Safety        PluginSafety      `json:"safety"`
}

type PluginType string
const (
    PluginBuiltin  PluginType = "builtin"    // compiled into core
    PluginScript   PluginType = "script"     // shell/python/etc, runs as subprocess
    PluginCompiled PluginType = "compiled"   // Go plugins (.so) loaded at runtime
    PluginExtPack  PluginType = "extpack"    // git repo + manifest, script-based
)

type Hook string
const (
    HookOnIntent     Hook = "onIntent"
    HookOnEvent      Hook = "onEvent"
    HookOnContext    Hook = "onContext"
    HookOnLifecycle  Hook = "onLifecycle"
)
```

Plugin loading strategy:
1. **Builtin plugins** are compiled into the core binary. No loading risk.
2. **Script plugins** run as child processes, communicate via JSON over stdin/stdout or a Unix socket.
3. **Compiled plugins** are Go `.so` files loaded via `plugin.Open()` (Linux/macOS only).
4. **Extension packs** are Git repos with a `manifest.json` (modeled after MenuMate's PackManifest), installed into the plugins directory.

### 3.7 Permission Manager

**Source:** menumate's `ActionDispatcher` security model, the concept of "secure by default."

```go
type Permission string

const (
    PermInputIntercept   Permission = "input.intercept"    // capture keyboard/mouse
    PermAccessibility    Permission = "accessibility"     // control other apps
    PermFilesystem       Permission = "filesystem"        // read/write files
    PermNetwork         Permission = "network"            // network access
    PermShellExecution  Permission = "shell.execute"     // run shell commands
    PermFinderModify     Permission = "finder.modify"     // modify Finder state
)
```

Permission flow:
1. At install time, the plugin manifest declares required permissions.
2. Core prompts the user to approve (non-tech first: auto-approve safe defaults, prompt for sensitive ones).
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
)
```

Safety mechanisms:
- **Safe Mode**: When a new plugin/integration is enabled, Core enters safe mode for 30 seconds. If the user doesn't confirm, it auto-rolls back.
- **Kill Switch**: Global emergency deactivation.
- **Reset Everything**: Disables all hooks, stops daemon, disables all plugins, removes login item, verifies no process remains.
- **Snapshot/Restore**: Before enabling any OS integration, Core snapshots the current state. On rollback, it restores.
- **Ownership Tracking**: Every system modification (login item, config file, etc.) records: creator plugin ID, timestamp, rollback procedure.

---

## 4. Platform Adapters

### 4.1 macOS Adapter (Swift/ObjC)

**Source:** newfile's FIFinderSync, menumate's FinderExtension, nudge's AX/Carbon patterns, Karabiner-Elements' CGEvent/DriverKit.

The macOS adapter is a Swift framework (`platform/darwin/`) that exposes C-compatible functions called via CGO from Go core. It provides:

#### Keyboard Interception
- **CGEvent tap** (`CGEventTapCreate`) for low-level keyboard/mouse event capture.
- Uses `kCGHIDEventTap` for hardware event interception.
- Inspired by Karabiner-Elements' approach but at the application level (no kernel driver needed for MVP).

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
- IPC with Core daemon via `DistributedNotificationCenter` (as in newfile and menumate) or Unix socket.

#### App Categories
- Bundle ID → category mapping (terminal, browser, remote, system).
- Hardcoded terminal bundles: Terminal, iTerm2, WezTerm, Warp, Alacritty, Kitty, Hyper, Ghostty.
- Hardcoded browser bundles: Safari, Brave, Chrome, Edge, Opera, Firefox.
- Hardcoded remote bundles: Microsoft RDC, Parallels, VMware Fusion, VirtualBox.

### 4.2 Windows Adapter (C++)

**Source:** keymapper's `server/windows/`, windhawk's mod injection model.

Provides equivalent functionality on Windows:
- **Raw Input** (`/win/0x100` + `RegisterRawInputDevices`) for keyboard/mouse capture.
- **SendInput** for synthetic input generation.
- **GetForegroundWindow** + **GetWindowText** for active window/app detection.
- **AccessibleObject** / UI Automation for window manipulation.
- Shell context menu via `IContextMenu` or `IShellExtInit`.

---

## 5. Plugin System

### 5.1 Plugin Manifest Format

```json
{
  "schemaVersion": 2,
  "id": "windows-keyboard",
  "name": "Windows Keyboard",
  "description": "Windows muscle-memory keyboard shortcuts on macOS",
  "version": "1.0.0",
  "author": "CrossOS",
  "entry": "builtin",
  "type": "builtin",
  "permissions": ["input.intercept", "filesystem"],
  "capabilities": [
    {"name": "keyboard.intercept", "description": "Intercept and remap keyboard events"}
  ],
  "hooks": ["onIntent", "onEvent"],
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

| Plugin ID | Type | Capabilities | MVP Phase |
|---|---|---|---|
| `windows-keyboard` | builtin | keyboard.intercept | Phase 1 |
| `windows-window` | builtin | window.manage | Phase 1 |
| `finder-ux` | extpack | finder.contextMenu | Phase 1 |
| `developer` | script | shell.execute, terminal | Phase 2 |
| `launcher` | script | keyboard.intercept, app.launch | Phase 2 |

### 5.3 Extension Pack Format (modeled after MenuMate PackManifest)

Extension packs are Git repos installed into `plugins/<pack-id>/`. They contain:
```
pack/
├── manifest.json       # PackManifest (schemaVersion, name, author, description, icon, actions[])
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
  "schemaVersion": 2,
  "name": "Windows Explorer UX",
  "author": "CrossOS",
  "description": "Right-click context menu items matching Windows Explorer",
  "icon": "shippingbox",
  "actions": [
    {
      "id": "new-text-file",
      "title": "New > Text Document",
      "icon": "doc.text",
      "script": "actions/new-file.zsh",
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
      "script": "actions/copy-path.zsh",
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
| `script` | string | yes | Path relative to pack root. Must be within pack dir. |
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

**Behavior matrix** (from windows-keyboard-for-mac + Karabiner profile JSON):

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
| Ctrl+W | Ctrl+W (close tab) | Ctrl+W |
| Ctrl+T | Cmd+T | Ctrl+T |
| Ctrl+Tab | Ctrl+Tab (or Cmd+Option+Left) | Ctrl+Tab |
| Ctrl+Shift+Tab | Ctrl+Shift+Tab | Ctrl+Shift+Tab |
| F2 | Enter (rename in Finder) | Enter |
| F5 | Refresh (scroll lock) | Fn+Cmd+R or F5 |
| Alt+F4 | Cmd+Q (quit app) | Cmd+Q |
| Alt+F4 (on VM) | Alt+F4 (pass through) | Alt+F4 |
| Alt+Tab | Cmd+Tab | Cmd+Tab |
| Win+D | F11 (fullscreen) or Mission Control | F11 |
| Win+L | Ctrl+Cmd+Q (lock screen) | Ctrl+Cmd+Q |
| Win+E | Cmd+Space (Spotlight) or Finder | Cmd+Space |
| Win+Shift+S | Shift+Cmd+5 (screenshot) | Shift+Cmd+5 |
| Win+Arrow Left | Cmd+Ctrl+F2 (snap left) → use Moom/Yabai | Cmd+Ctrl+F2 |
| Win+Arrow Right | Cmd+Ctrl+F2 (snap right) | Cmd+Ctrl+F2 |
| Win+Arrow Up | Zoom/maximize | — |
| Win+Arrow Down | Minimize | — |
| Ctrl+Win+F | Cmd+Ctrl+F (fullscreen) | Cmd+Ctrl+F |
| Win+Shift+Left/Right | Alt+Cmd+←/→ (move space) | — |
| Home/End | Cmd+Home/End (scroll to top/bottom) | — |
| Ctrl+Home/End | Cmd+Up/Down | — |
| NumLock | Clear | — |
| Scroll Lock | F18 (or Shift+F18) | — |

**Terminal apps** (pass-through Ctrl): Terminal, iTerm2, WezTerm, Warp, Alacritty, Kitty, Hyper, Ghostty, VSCode, Chrome Dev Tools.
**Remote apps** (pass-through everything): Microsoft RDC, Parallels, VMware Fusion, VirtualBox, JumpDesktop, Citrix.

**Implementation**:
- Uses Karabiner-Elements' virtual HID approach (pqrs/Karabiner-DriverKit-VirtualHIDDevice) OR application-level CGEvent tap.
- For MVP: CGEvent tap at application level (no driver needed). For production: virtual HID driver for system-wide remapping.
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

### 6.3 Plugin 3: Windows Explorer UX (Extension Pack, Swift FIFinderSync)

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
- Actions executed as shell scripts with env injection (like MenuMate's `ShellRunner`).
- Security: ActionDispatcher validates requests against local config, checks paths, max path count.
- IPC: DistributedNotificationCenter or Unix socket to Core for execution.

### 6.4 Plugin 4: Developer UX (Script Plugin, Phase 2)

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

### 7.2 Key UI Pages

| Page | Purpose |
|---|---|
| **Dashboard** | Status of all plugins, enable/disable toggle, safe mode indicator, kill switch button. |
| **Keyboard** | View/edit the Windows keyboard behavior matrix. App-specific overrides. |
| **Windows** | Window management shortcuts, snap zone editor. |
| **Finder** | Context menu items, extension packs, action settings. |
| **Plugins** | Installed plugins, marketplace browser, install/enable/disable/update. |
| **Shortcuts** | Global keyboard shortcut manager. |
| **Activity** | Real-time event log, intent resolution trace, plugin actions. |
| **Safety** | Kill switch, Reset Everything, Safe Mode settings, ownership audit. |
| **About** | Version, license, attribution, repository credits. |

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
3. **Emergency kill switch** → one click: disable hooks → stop daemon → disable plugins/extension → remove login item → verify.
4. **Safe Mode** → enable for 30s → confirm/rollback.
5. **Reset Everything** → restore snapshot → all integrations reversed → verify no process remains.

### 8.2 Snapshot & Restore

Before installing/enabling any OS integration:
```go
type SystemSnapshot struct {
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
| Keyboard interception | Accessibility (AX) + Accessibility API | Run as admin / Raw Input |
| Window management | Accessibility (AX) | Run as admin |
| Finder context menu | Finder Sync Extension (user approves in System Settings) | Context menu handler (registry) |
| Clipboard | Accessibility or Accessibility API | — |
| App switching | Accessibility (AX) | — |

### 8.4 Non-Tech Safety Defaults

- Default config enables all 3 MVP plugins with sensible Windows defaults.
- Safe Mode is ON by default for new installations.
- Kill switch is prominent and always accessible.
- "Reset Everything" is prominent and always accessible.
- No integration is enabled until the user explicitly clicks "Enable."

---

## 9. Licensing Matrix

From the research (see `docs/RESEARCH.md` for full matrix):

| Repo | License | CrossOS Action | Used For |
|---|---|---|---|
| **menumate** | MIT | 🟢 COPY/ADAPT | Plugin system, manifest format, IPC, security model |
| **newfile** | MIT | 🟢 COPY/ADAPT | FIFinderSync extension, file creation |
| **nudge** | MIT | 🟢 COPY/ADAPT | Window snap actions, frame calc, hotkeys |
| **windows-keyboard-for-mac** | MIT | 🟡 BEHAVIOR/UX | Shortcut matrix, app-specific rules |
| **pcfy-my-mac** | MIT | 🟢 ADAPT | Overall setup orchestration (reference) |
| **Karabiner-Elements** | Unlicense | 🟡 ARCHITECTURE | Virtual HID, event model (learn only) |
| **keymapper** | GPL-3.0 | 🟡 ARCHITECTURE | Context detection patterns (learn only) |
| **kanata** | GPL-3.0 | 🟡 ARCHITECTURE | Layer/tap-hold config engine (learn only) |
| **windhawk** | GPL-3.0 | 🟡 ARCHITECTURE | Plugin marketplace model (learn only) |
| **komorebi** | custom | 🟡 ARCHITECTURE | JSON schema + TCP client model (learn only) |
| **alt-tab-macos** | GPL-3.0 | 🟡 ARCHITECTURE | Window enumeration (learn only) |
| **hammerspoon** | MIT | 🟡 ARCHITECTURE | Module API model (learn only) |
| **yabai** | MIT | 🟢 STUDY | AX window internals (study only) |
| **Amethyst** | MIT | 🟢 STUDY | Layout management (study only) |
| **Rectangle** | MIT | 🟡 BEHAVIOR | Snap zones UX (behavior only) |
| **SketchyBar** | MIT | 🟡 ARCHITECTURE | Plugin/composable bar model (learn only) |

**Strict rule**: No GPL/copyleft code is copied into the MIT-licensed CrossOS core. GPL repos are studied for architecture only. Only MIT/Unlicense repos contribute code (with per-file LICENSE + dependency tree checks).

---

## 10. Implementation Phases

### Phase 0: Foundation (2-3 weeks)
- [ ] Create Go core project structure (`core/` + `pkg/`)
- [ ] Event bus implementation
- [ ] Config manager with schema validation
- [ ] IPC server (JSON-RPC over Unix socket)
- [ ] Core daemon lifecycle (init, run, stop, safe mode)
- [ ] Plugin runtime scaffold (Plugin interface, PluginLoader)
- [ ] macOS adapter skeleton (Swift Framework, CGEvent tap, AX bridge)
- [ ] Windows adapter skeleton (C++ project, Raw Input, SendInput)
- [ ] Basic UI shell (Wails + React + shadcn)
- [ ] Safety layer (kill switch, reset everything, snapshot)

### Phase 1: MVP Plugins (4-5 weeks)
- [ ] **Plugin 1: Windows Keyboard**
  - CGEvent tap on macOS, Raw Input on Windows
  - Behavior matrix (Ctrl→Cmd, etc.) with app filter support
  - Terminal/browser/remote app exclusion lists
  - Device filtering
  - Config UI page
- [ ] **Plugin 2: Windows Window Management**
  - AXUIElement window manipulation
  - 19 SnapAction frames (copy from Nudge)
  - Frame history for restore
  - Hotkey registration (Carbon EventHotKey)
  - Configurable shortcuts
  - Config UI page
- [ ] **Plugin 3: Windows Explorer UX**
  - FIFinderSync extension in `extensions/finder-sync/`
  - Context menu items (New > Text Document, Copy as Path, Open Terminal, Cut/Copy/Paste, Rename, Delete, Compress)
  - Extension pack loading (manifest.json parser)
  - Script execution with env injection (model after MenuMate ShellRunner)
  - Security (path validation, undeclared file inspection)
  - Config UI page

### Phase 2: Polish & Safety (2 weeks)
- [ ] Safe Mode with 30s confirm/rollback
- [ ] Ownership tracking for all integrations
- [ ] Snapshot/restore for system state
- [ ] Emergency kill switch (prominent, always accessible)
- [ ] Reset Everything button
- [ ] Activity log / event trace
- [ ] Permission prompts (non-tech friendly)
- [ ] App category auto-detection
- [ ] Performance: event processing < 1ms (target)

### Phase 3: Plugin Ecosystem (3-4 weeks)
- [ ] Script plugin support (shell, python, lua)
- [ ] Extension pack marketplace (Git-based)
- [ ] Plugin install/enable/disable/update lifecycle
- [ ] Plugin config schema UI (auto-generated forms)
- [ ] **Plugin 4: Developer UX**
  - Terminal-in-directory
  - Open in editor
  - IDE shortcuts
- [ ] **Plugin 5: Launcher** (global hotkey launcher)
- [ ] Plugin compatibility layer (version checking)

### Phase 4: Distribution & Polish (2 weeks)
- [ ] macOS app bundling (signed .app, notarized)
- [ ] Windows MSI installer (signed)
- [ ] Auto-update mechanism
- [ ] Documentation site
- [ ] CI/CD pipeline (GitHub Actions)
- [ ] User onboarding flow

### Phase 5: Future (backlog)
- Virtual HID driver integration (Kernel DriverKit) for full system-wide remapping
- Cross-Platform: Linux support (X11 + Wayland)
- AI assistance (intent inference from ambiguous input)

---

## 11. Testing Strategy

### Unit Tests (Go)
- Event bus: publish/subscribe, ordering, backpressure
- Context resolver: app categorization, window parsing
- Intent resolver: rule matching, behavior matrix correctness
- Rule engine: filter evaluation (app, UTI, device, selection count)
- Plugin runtime: load/unload, hook dispatch, capability registration
- Safety: snapshot/restore, rollback, safe mode timer
- Config: schema validation, merge priority

### Integration Tests
- End-to-end: input event → intent → action (mock adapter)
- Plugin lifecycle: install, enable, disable, update, uninstall
- Safety: kill switch interrupts all processing
- Config: overrides propagate correctly

### Platform-Specific Tests
- macOS: FIFinderSync context menu visibility, AX window manipulation
- Windows: Raw Input capture, SendInput output

### Reference Tests
- Keyboard matrix: 32 shortcuts × 3 contexts (normal/terminal/remote) = 96 test cases
- Window actions: 19 snap actions × multi-monitor × history = 380+ test cases
- Finder actions: 11 menu items × file/folder/empty selection = 22+ test cases

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
│   ├── darwin/               # Swift Framework (CGEvent, AX, FIFinderSync)
│   │   └── extensions/finder-sync/   # Swift FIFinderSync Extension
│   └── windows/              # C++ static library (Raw Input, SendInput)
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
├── docs/
│   ├── RESEARCH.md           # research matrix
│   └── COMPREHENSIVE_PLAN.md # this file
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

# Build Windows adapter (C++)
cd platform/windows && cmake . && make

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
| Go | 1.22+ | BSD | Core engine |
| Wails | v2.x | MIT | Desktop shell |
| React | 18+ | MIT | UI framework |
| Tailwind CSS | 3+ | MIT | Styling |
| shadcn/ui | latest | MIT | Component library |
| Swift | 5.9+ | Apache 2.0 | macOS adapter |
| Xcode | 15+ | - | Finder extension |

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

## 14. Open Questions / Risks

| # | Question | Options | Recommendation |
|---|---|---|---|
| 1 | How to intercept keyboard input? | App-level CGEvent tap vs kernel DriverKit | Start with CGEvent tap for MVP; upgrade to DriverKit for production |
| 2 | How to communicate with Finder extension? | DistributedNotificationCenter vs Unix socket | Unix socket (more secure, better error handling) |
| 3 | How to handle multi-user / multiple macOS instances? | Session-aware context | Design context resolver to include session info |
| 4 | How to handle updates to extension packs? | Git pull + reload vs bundled version | Git-based auto-update with user approval |
| 5 | How to prevent plugin conflicts? | Priority + capability resolution | Priority system + intent deduplication |
| 6 | Apple Silicon vs Intel differences? | Architecture-specific builds | Build for arm64 + amd64 |
| 7 | Notarization requirements for Finder extension? | Required for distribution | Plan for Apple Developer account, hardened runtime |
