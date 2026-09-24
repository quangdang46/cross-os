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
