// UI source payloads: the typed rows every settings page asks the daemon for
// (bead cross-os-72g).
//
// These structs are declared ONCE PER MODULE on purpose. The daemon and the
// shell are separate Go modules, so nothing but this comment and
// uisources_test.go keeps the two declarations honest — the wire contract IS
// "both sides spell the same json tags". Two conventions are frozen here and
// must not drift:
//   - Go FIELD NAMES become the TypeScript models (Wails v3 generates them
//     from the fields), so a rename here silently renames a frontend type.
//   - The json tags are the snake_case the daemon actually serves, so a tag
//     change is a wire break in the other direction.
//
// Every field below is what a page renders. Nothing here interprets or
// defaults a value: a page that asks for the matrix and gets [] must be able
// to say "no rules configured", and a page that gets an error must be able to
// say "the daemon is unreachable". Collapsing those two is how a settings
// window becomes a blank page (bridge.go: failures are logged AND returned).
//
// The rows are in waves — the ten sources the ten settings pages already ask
// for, then the profile cards, traces, plugin meta, the app list, the
// person-authored rules, and the switcher's tiles — and the conventions above
// hold for all of them. One row (AppRow) carries no tags at all, because the
// type it mirrors on the daemon side does not; that is the wire, and it is
// pinned by a test rather than by this comment.
package shell

// MatrixRow is one behavior-matrix rule (config.getMatrix). Contexts are the
// app contexts the rule fires in; the Keyboard page matrix toggles Enabled
// through config.setRuleEnabled, never by editing this row.
type MatrixRow struct {
	RuleID   string   `json:"rule_id"`
	Plugin   string   `json:"plugin"`
	Action   string   `json:"action"`
	Keys     string   `json:"keys"`
	Contexts []string `json:"contexts"`
	Enabled  bool     `json:"enabled"`
}

// OverrideRow is one per-app rule override (config.getOverrides). It shadows a
// matrix rule inside a single app, which is why it repeats the action/keys it
// currently resolves to instead of pointing at a rule.
type OverrideRow struct {
	App     string `json:"app"`
	RuleID  string `json:"rule_id"`
	Action  string `json:"action"`
	Keys    string `json:"keys"`
	Enabled bool   `json:"enabled"`
}

// ZoneRow is one snap-zone rectangle in screen coordinates (config.getZones).
// Geometry is stored as floats because a monitor's usable area rarely divides
// evenly; the Windows page editor rounds for display, not here.
type ZoneRow struct {
	ID   string  `json:"id"`
	Name string  `json:"name"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	W    float64 `json:"w"`
	H    float64 `json:"h"`
}

// CommandRow is one command-palette entry (core.commands). Commands arrive
// from plugins, so a new command must show up without a shell change — this
// row is data, not a code path (§3.6c discovery, same rule as pages).
type CommandRow struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Plugin string `json:"plugin"`
}

// SchemaRow is one plugin's declarative config_schema (core.pluginSchemas),
// served as an object so the frontend renders it directly rather than
// re-parsing a JSON string (the same trap App.tsx:41 documents for pages).
type SchemaRow struct {
	Plugin string         `json:"plugin"`
	Title  string         `json:"title"`
	Schema map[string]any `json:"schema"`
}

// AuditRow is one thing CrossOS created on this machine
// (safety.ownershipAudit) — the Safety page "What CrossOS created" list, and
// the input to Reset Everything's ownership scoping (§8.2).
type AuditRow struct {
	Resource  string `json:"resource"`
	ID        string `json:"id"`
	Owner     string `json:"owner"`
	CreatedAt string `json:"created_at"`
}

// TrialState is the trial in flight (safety.trialState). Both durations are
// milliseconds: the countdown is driven by a timer, not a re-render, and a
// wall-clock string would force the page to parse it. State "none" with an
// empty Plugin is the daemon's honest "no trial in flight" — it is a value,
// not a failure, and must not render as a broken countdown.
type TrialState struct {
	Plugin      string `json:"plugin"`
	State       string `json:"state"`
	RemainingMS int64  `json:"remaining_ms"`
	TimeoutMS   int64  `json:"timeout_ms"`
}

// ReadinessRow is one onboarding checklist item (core.readiness). Detail
// carries the "grant Accessibility" style hint, so a not-ready row can tell
// the user what to do instead of only that it is red.
type ReadinessRow struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Ready  bool   `json:"ready"`
	Detail string `json:"detail"`
}

// ProfileRow is one profile card (core.profiles) — the Windows-experience
// bundle and whatever profiles ship beside it. The bundle is the profile's own
// data; what the card adds is the rollup, so one answer settles whether the
// click landed instead of the page joining a second source to find out.
type ProfileRow struct {
	ID           string                 `json:"id"`
	Label        string                 `json:"label"`
	Description  string                 `json:"description"`
	Active       bool                   `json:"active"`
	Capabilities []ProfileCapabilityRow `json:"capabilities"`
}

// ProfileCapabilityRow is one declared capability plus its live verdict.
//
// RuleIDs keeps the declared list, which mixes two vocabularies on purpose: a
// behavior-matrix rule id (something the user can toggle) and a window-action
// zone name (something winlayout resolves). Enabled/Total count only the
// former, because "1 of 21 shortcuts on" for a capability that ships no rules
// reads as a bug.
type ProfileCapabilityRow struct {
	ID        string   `json:"id"`
	Label     string   `json:"label"`
	Plugin    string   `json:"plugin"`
	Available bool     `json:"available"`
	Reason    string   `json:"reason,omitempty"`
	RuleIDs   []string `json:"rule_ids"`
	Enabled   int      `json:"enabled"`
	Total     int      `json:"total"`
	Live      bool     `json:"live"`
}

// TraceRow is one recorded decision as columns (core.traces). core.eventLogs
// flattens these same traces into "winner action params=…" strings; the stages,
// the physical event, the focused app and the rules that lost are all still in
// the recorded trace, so a page asking for them gets fields instead of a
// sentence to parse.
type TraceRow struct {
	At       string       `json:"at"`
	Decision string       `json:"decision"`
	Event    TraceEvent   `json:"event"`
	Context  TraceContext `json:"context"`
	Winner   string       `json:"winner"`
	Losers   []string     `json:"losers"`
	Intent   string       `json:"intent"`
	Action   string       `json:"action"`
	Stages   []StageRow   `json:"stages"`
	Params   string       `json:"params"`
}

// TraceEvent is the physical input, the chord rendered the way the matrix
// renders a rule's binding so a recorded key and the rule claiming it read
// alike on one screen. KeyCode is the Windows virtual-key code the recorder
// stores, kept as a number beside the rendered name so a key the table does
// not name is still a value the user can report.
type TraceEvent struct {
	Keys    string `json:"keys"`
	Source  string `json:"source"`
	Device  string `json:"device,omitempty"`
	KeyCode uint32 `json:"key_code"`
}

// TraceContext is the cached decision context the router logged — never an
// accessibility query, which is the contract that keeps the trace path off the
// keyboard.
type TraceContext struct {
	AppID    string `json:"app_id"`
	AppMode  string `json:"app_mode"`
	WindowID string `json:"window_id,omitempty"`
	WinClass string `json:"win_class,omitempty"`
}

// StageRow is one pipeline step the router logged, in the order it ran.
type StageRow struct {
	Stage  string `json:"stage"`
	Detail string `json:"detail"`
}

// PluginMetaRow is one registered plugin's manifest facts (core.pluginMeta).
// Name and Version are empty today — the builtin plugins are compiled straight
// from the rule table and no manifest is loaded — so every row carries the
// Reason saying so rather than a display name the daemon prettified from an
// id. Permissions are not a guess: they are the grants the router authorizes
// that plugin's rules against.
type PluginMetaRow struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Permissions []string `json:"permissions"`
	Loaded      bool     `json:"loaded"`
	Reason      string   `json:"reason,omitempty"`
}

// AppRow is one application a rule may be scoped to (core.apps) — the row
// behind the rule builder's "IF App = X" picker.
//
// It carries NO json tags, and that is the wire: the daemon's enumeration is
// adapter.ListApps, which returns []ctx.ApplicationInfo — the app-identity
// record its own context resolver reads, so the picker's row and the matcher
// cannot drift into two shapes for one application — and that struct declares
// no tags, so encoding/json spells its keys as the Go field names. The
// PascalCase here is therefore not a style slip; it is the shape of the wire.
// TestAppRowMatchesTheDaemonsRow holds this list to ctx.ApplicationInfo's own
// fields and kinds, so a field added to the daemon's row fails here instead of
// arriving as a column the shell has nowhere to put.
type AppRow struct {
	BundleID    string
	Executable  string
	PID         int
	DisplayName string
	AppMode     string
	Category    string
}

// UserRuleRow is one person-authored rule (config.getUserRules) as the rule
// editor renders it.
//
// The first block is the stored rule's own dimensions, under the same keys the
// write path takes them — an edit sends a row back with an id and the editor
// never composes a payload. Chord, Action, Priority, Specificity and Scope are
// added for display and are DERIVED: the daemon computes them from the
// dimensions, so sending one back would be a number the user can change without
// changing the rule. Parameters is an object because the capability's input
// schema is one — the same reason SchemaRow serves a decoded schema rather than
// a JSON string for the page to re-parse.
type UserRuleRow struct {
	ID          string         `json:"id"`
	Key         string         `json:"key"`
	Modifiers   []string       `json:"modifiers"`
	AppModes    []string       `json:"app_modes"`
	AppIDs      []string       `json:"app_ids"`
	DeviceID    string         `json:"device_id"`
	Capability  string         `json:"capability"`
	Parameters  map[string]any `json:"parameters,omitempty"`
	Emit        bool           `json:"emit"`
	Chord       string         `json:"chord"`
	Action      string         `json:"action"`
	Priority    int            `json:"priority"`
	Specificity int            `json:"specificity"`
	Scope       string         `json:"scope"`
}

// WindowRow is one tile of the window switcher (core.windows), a copy of the
// daemon's switcherRow — that row is the wire.
//
// The field checklist a switcher row is measured against is the alt-tab
// reference's TrackedWindowState (pid, title, bounds, minimized, fullscreen,
// is-main), cited as a checklist only: this struct mirrors what the daemon
// sends, and a field it does not send is a column the shell has nowhere to put.
// Index, Selected and Skippable are CrossOS's own — the daemon owns the MRU, so
// the shell draws the position it is handed instead of sorting a second order.
type WindowRow struct {
	WindowID  string `json:"window_id"`
	AppID     string `json:"app_id"`
	Title     string `json:"title"`
	Index     int    `json:"index"`
	Selected  bool   `json:"selected"`
	Skippable bool   `json:"skippable"`
}

// SwitcherTrigger is one thing the switcher did that the shell has not seen
// yet (core.switcherWait). Action is the switcher's own vocabulary — summon,
// commit — not a rule id or an intent id: the shell draws a switcher, it does
// not read the decision path. Triggered false is the daemon's "your budget ran
// out", which is the answer a poll expects, not an incident.
type SwitcherTrigger struct {
	Triggered bool   `json:"triggered"`
	Action    string `json:"action,omitempty"`
	WindowID  string `json:"window_id,omitempty"`
}
