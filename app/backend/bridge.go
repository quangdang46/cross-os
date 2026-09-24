// Wails bridge (§7.3): Go functions callable from JS.
//
// The bridge is a thin facade over Core: GetStatus reads daemon state,
// TogglePlugin flips enablement, ResetEverything plans the safety reset,
// GetEventLogs serves the activity trace. Bridge failures are returned as
// typed errors AND recorded in the UI log sink — never silent blank pages
// (bead criterion: bridge/IPC failures surfaced in UI logs).
package shell

import (
	"fmt"
	"sync"
)

// PluginState is one row of the Dashboard page.
type PluginState struct {
	ID      string
	Enabled bool
	Healthy string // "healthy" | "degraded" | "disabled"
}

// Status is the Dashboard payload.
type Status struct {
	Running  bool
	SafeMode bool
	Killed   bool
	Plugins  []PluginState
	// Interception reports whether the live keyboard tap is installed. The
	// daemon owns the truth; the shell only displays it.
	Interception bool
	// TapError is the install failure (usually missing input-monitoring
	// consent), shown so the user knows which permission to grant.
	TapError string
	// Version is the running daemon's version, so the About block cannot
	// drift from a hardcoded literal.
	Version string
}

// UILog is the shell's UI-visible log sink: bridge/IPC failures land here
// so the frontend renders them instead of a blank page.
type UILog struct {
	mu    sync.Mutex
	lines []string
}

// Append records one UI-visible line.
func (l *UILog) Append(line string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, line)
}

// Lines returns a copy of the log.
func (l *UILog) Lines() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.lines...)
}

// Core is the minimal Core surface the bridge needs. Core's concrete daemon
// implements this; tests stub it. Kept narrow so the shell stays portable
// across the v2/v3 Wails decision.
type Core interface {
	IsRunning() bool
	InSafeMode() bool
	IsKilled() bool
	Plugins() []PluginState
	SetEnabled(id string, enabled bool) error
	Reset() []string // returns the reset plan steps for audit
	EventLogs() []string
	// CheckForUpdate reports whether manifest.Version is newer than the
	// running daemon (core.checkForUpdate). No network from the shell —
	// the daemon fetches and compares.
	CheckForUpdate(manifestVersion, platform, url, sha256 string) (bool, string, error)
	// ApplyUpdate downloads + checksum-verifies + approval-gated installs
	// (core.applyUpdate). Approved=false fails closed (§8.4).
	ApplyUpdate(manifestVersion, platform, url, sha256 string, approved bool, approvedBy string) (string, error)
	// PanicStop executes the kill switch (safety.panicStop): stops
	// interception + plugin actions, keeps the login item. Result names
	// what stopped — never a silent kill.
	PanicStop() (map[string]any, error)
	// Resume clears a latched PANIC STOP and re-installs the tap.
	Resume() (map[string]any, error)
	// Interception reports whether the live keyboard tap is installed.
	Interception() bool
	// TapError is the last tap install failure, or "" when healthy.
	TapError() string
	// Version is the running daemon's version.
	Version() string
	// BeginTrial starts DISABLED → TRIAL for one plugin (safety.beginTrial).
	// Idempotent: repeat calls return the in-flight trial.
	BeginTrial(pluginID string) (string, error)
	// ConfirmTrial completes TRIAL → ENABLED (safety.confirmTrial).
	// Confirmed=false or healthy=false fails closed — no auto path.
	ConfirmTrial(pluginID string, confirmed, healthy bool) (string, error)
	// RollbackTrial moves TRIAL → DISABLED (safety.rollbackTrial).
	RollbackTrial(pluginID, reason string) (string, error)
	// SetRuleEnabled toggles one matrix row by RuleID
	// (config.setRuleEnabled). Unknown RuleIDs fail closed.
	SetRuleEnabled(ruleID string, enabled bool) (bool, error)
	// Shortcuts returns the current shortcut table (config.getShortcuts).
	Shortcuts() ([]map[string]any, error)
	// SetShortcuts replaces the shortcut table after validation
	// (config.setShortcuts): [{action, modifiers, key}].
	SetShortcuts(shortcuts []map[string]any) (int, error)
	// Matrix returns the behavior-matrix rules (config.getMatrix). The
	// Keyboard page renders whatever arrives; the shell never derives a row.
	Matrix() ([]MatrixRow, error)
	// Overrides returns the per-app rule overrides (config.getOverrides).
	Overrides() ([]OverrideRow, error)
	// SetOverride toggles one per-app override (config.setOverride) and
	// returns the daemon's echo of the stored row. Unknown app/rule ids are
	// rejected by the daemon, which is the only validator: a second rule book
	// in the shell would drift and would report "saved" for a denied edit.
	SetOverride(app, ruleID string, enabled bool) (OverrideRow, error)
	// Zones returns the snap-zone rectangles (config.getZones).
	Zones() ([]ZoneRow, error)
	// SetZones replaces the snap-zone set (config.setZones) and returns how
	// many the daemon accepted. A rejected rectangle changes nothing.
	SetZones(zones []ZoneRow) (int, error)
	// Commands returns the command-palette entries (core.commands) so a
	// plugin's commands appear with zero shell change.
	Commands() ([]CommandRow, error)
	// PluginSchemas returns each plugin's config_schema
	// (core.pluginSchemas) for the declarative form renderer.
	PluginSchemas() ([]SchemaRow, error)
	// OwnershipAudit lists what CrossOS created on this machine
	// (safety.ownershipAudit) — the Safety page ownership list and the input
	// to Reset Everything's ownership scoping.
	OwnershipAudit() ([]AuditRow, error)
	// TrialState reports the trial in flight (safety.trialState). State
	// "none" is an answer (no trial), not a failure.
	TrialState() (TrialState, error)
	// Readiness returns the per-area readiness rows (core.readiness) the
	// onboarding checklist renders.
	Readiness() ([]ReadinessRow, error)
	// Profiles returns the profile cards (core.profiles): each bundle with the
	// live rollup of its capabilities, so a card can say whether a click landed.
	Profiles() ([]ProfileRow, error)
	// ApplyProfile activates one profile's available capabilities as ONE
	// settings plan (core.profileApply). The daemon applies it all-or-nothing
	// and answers the counts; the shell never applies it rule by rule, because
	// a profile that saved half its edits is the outcome the store's atomicity
	// exists to prevent.
	ApplyProfile(profileID string) (map[string]any, error)
	// Traces returns the recorded decisions (core.traces) as rows, oldest
	// first and truncated to the tail the daemon keeps.
	Traces() ([]TraceRow, error)
	// PluginMeta returns each registered plugin's manifest facts
	// (core.pluginMeta) — permissions, load state, and the reason a fact is
	// missing rather than a prettified guess.
	PluginMeta() ([]PluginMetaRow, error)
	// Apps returns the installed applications a rule may be scoped to
	// (core.apps, the page-data spelling of the "core:apps" control source
	// core.profiles and core.pluginMeta already live in). Empty is the answer
	// on a platform with no cheap enumeration — the adapter returns a list it
	// is sure of — and it is not a failure.
	Apps() ([]AppRow, error)
	// UserRules returns the person-authored rules (config.getUserRules) with
	// the derived fields the editor displays beside them.
	UserRules() ([]UserRuleRow, error)
	// SetUserRule creates or updates one rule (config.setUserRule) and returns
	// the id the daemon derived. An id the table does not have is rejected
	// there, so a stale editor cannot get a duplicate row instead of an answer.
	SetUserRule(rule UserRuleRow) (string, error)
	// DeleteUserRule removes one rule (config.deleteUserRule) and returns the
	// table as it now stands, so the editor updates from one response.
	DeleteUserRule(id string) ([]UserRuleRow, error)
}

// App is the Wails-bound service (§7.3 shape: GetStatus, TogglePlugin,
// ResetEverything, GetEventLogs).
type App struct {
	core Core
	log  *UILog
}

// NewApp binds the bridge to Core.
func NewApp(c Core) *App { return &App{core: c, log: &UILog{}} }

// UILogs exposes the UI log sink to the frontend.
func (a *App) UILogs() []string { return a.log.Lines() }

// GetStatus serves the Dashboard page.
func (a *App) GetStatus() *Status {
	return &Status{
		Running:      a.core.IsRunning(),
		SafeMode:     a.core.InSafeMode(),
		Killed:       a.core.IsKilled(),
		Plugins:      a.core.Plugins(),
		Interception: a.core.Interception(),
		TapError:     a.core.TapError(),
		Version:      a.core.Version(),
	}
}

// TogglePlugin enables/disables one plugin. Unknown IDs and Core errors are
// returned AND logged (never a silent no-op toggle).
func (a *App) TogglePlugin(id string, enabled bool) error {
	known := false
	for _, p := range a.core.Plugins() {
		if p.ID == id {
			known = true
		}
	}
	if !known {
		err := fmt.Errorf("shell: unknown plugin %q", id)
		a.log.Append("TogglePlugin: " + err.Error())
		return err
	}
	if err := a.core.SetEnabled(id, enabled); err != nil {
		a.log.Append("TogglePlugin " + id + ": " + err.Error())
		return err
	}
	return nil
}

// ResetEverything returns the audited reset plan (§8.2 ownership scope).
func (a *App) ResetEverything() []string { return a.core.Reset() }

// GetEventLogs serves the Activity page; a backend failure surfaces as a
// one-line UI log entry instead of a blank page.
func (a *App) GetEventLogs() []string {
	logs := a.core.EventLogs()
	if logs == nil {
		a.log.Append("GetEventLogs: backend returned nil, showing empty")
		return []string{}
	}
	return logs
}

// CheckForUpdate serves the update badge (Dashboard/Settings): whether the
// manifest version is newer. Failures land in the UI log, never silent.
func (a *App) CheckForUpdate(manifestVersion, platform, url, sha256 string) (bool, string, error) {
	avail, ver, err := a.core.CheckForUpdate(manifestVersion, platform, url, sha256)
	if err != nil {
		a.log.Append("CheckForUpdate: " + err.Error())
	}
	return avail, ver, err
}

// ApplyUpdate applies a manifest update with explicit user approval
// (§8.4: unapproved installs fail closed in the daemon, surfaced here AND
// in the UI log — never a silent no-op).
func (a *App) ApplyUpdate(manifestVersion, platform, url, sha256 string, approved bool, approvedBy string) (string, error) {
	installed, err := a.core.ApplyUpdate(manifestVersion, platform, url, sha256, approved, approvedBy)
	if err != nil {
		a.log.Append("ApplyUpdate " + manifestVersion + ": " + err.Error())
		return "", err
	}
	return installed, nil
}

// Resume clears a latched PANIC STOP (the Safety page re-enable control).
func (a *App) Resume() (map[string]any, error) {
	res, err := a.core.Resume()
	if err != nil {
		a.log.Append("Resume: " + err.Error())
		return nil, err
	}
	return res, nil
}

// PanicStop executes the kill switch (Safety page button): stops CrossOS
// from affecting the computer instantly. Failures land in the UI log.
func (a *App) PanicStop() (map[string]any, error) {
	res, err := a.core.PanicStop()
	if err != nil {
		a.log.Append("PanicStop: " + err.Error())
		return nil, err
	}
	return res, nil
}

// BeginTrial starts the enable trial for one plugin (Safety page countdown).
func (a *App) BeginTrial(pluginID string) (string, error) {
	state, err := a.core.BeginTrial(pluginID)
	if err != nil {
		a.log.Append("BeginTrial " + pluginID + ": " + err.Error())
		return "", err
	}
	return state, nil
}

// ConfirmTrial completes the trial (Safety page Confirm button). Denials
// fail closed in the daemon and surface here AND in the UI log.
func (a *App) ConfirmTrial(pluginID string, confirmed, healthy bool) (string, error) {
	state, err := a.core.ConfirmTrial(pluginID, confirmed, healthy)
	if err != nil {
		a.log.Append("ConfirmTrial " + pluginID + ": " + err.Error())
		return "", err
	}
	return state, nil
}

// RollbackTrial aborts the trial (Safety page rollback / timeout path).
func (a *App) RollbackTrial(pluginID, reason string) (string, error) {
	state, err := a.core.RollbackTrial(pluginID, reason)
	if err != nil {
		a.log.Append("RollbackTrial " + pluginID + ": " + err.Error())
		return "", err
	}
	return state, nil
}

// SetRuleEnabled toggles one matrix row (Keyboard page matrix control).
// Denials fail closed here AND in the UI log.
func (a *App) SetRuleEnabled(ruleID string, enabled bool) (bool, error) {
	ok, err := a.core.SetRuleEnabled(ruleID, enabled)
	if err != nil {
		a.log.Append("SetRuleEnabled " + ruleID + ": " + err.Error())
		return false, err
	}
	return ok, nil
}

// Shortcuts serves the Windows page editor table.
func (a *App) Shortcuts() ([]map[string]any, error) {
	rows, err := a.core.Shortcuts()
	if err != nil {
		a.log.Append("Shortcuts: " + err.Error())
		return nil, err
	}
	return rows, nil
}

// SetShortcuts replaces the shortcut table (Windows page zone/shortcut
// editor). Validation failures fail closed here AND in the UI log.
func (a *App) SetShortcuts(shortcuts []map[string]any) (int, error) {
	n, err := a.core.SetShortcuts(shortcuts)
	if err != nil {
		a.log.Append("SetShortcuts: " + err.Error())
		return 0, err
	}
	return n, nil
}

// sourceList normalizes a Core slice before it crosses the Wails boundary.
// The wire contract says collections are [], never null; a nil here means the
// daemon broke that promise or the transport died mid-decode. The row is
// still rendered (an empty list, never a null the frontend must defend
// against) and the anomaly is noted in the UI log, so "nothing configured"
// never masquerades as a real answer.
func sourceList[T any](a *App, method string, rows []T) []T {
	if rows == nil {
		a.log.Append(method + ": backend returned null, rendering empty list")
		return []T{}
	}
	return rows
}

// GetMatrix serves the Keyboard page behavior matrix. A failed call is logged
// AND returned: a page that got [] because nothing is configured must not
// look identical to a page whose daemon died.
func (a *App) GetMatrix() ([]MatrixRow, error) {
	rows, err := a.core.Matrix()
	if err != nil {
		a.log.Append("GetMatrix: " + err.Error())
		return nil, err
	}
	return sourceList(a, "GetMatrix", rows), nil
}

// GetOverrides serves the Keyboard page app-override table.
func (a *App) GetOverrides() ([]OverrideRow, error) {
	rows, err := a.core.Overrides()
	if err != nil {
		a.log.Append("GetOverrides: " + err.Error())
		return nil, err
	}
	return sourceList(a, "GetOverrides", rows), nil
}

// SetOverride toggles one app override. The returned row is the daemon's echo
// so the page renders what was stored, not what it optimistically sent; a
// rejected edit returns the zero row and the error, leaving the already-loaded
// table untouched.
func (a *App) SetOverride(app, ruleID string, enabled bool) (OverrideRow, error) {
	row, err := a.core.SetOverride(app, ruleID, enabled)
	if err != nil {
		a.log.Append("SetOverride " + app + "/" + ruleID + ": " + err.Error())
		return OverrideRow{}, err
	}
	return row, nil
}

// GetZones serves the Windows page snap-zone editor.
func (a *App) GetZones() ([]ZoneRow, error) {
	rows, err := a.core.Zones()
	if err != nil {
		a.log.Append("GetZones: " + err.Error())
		return nil, err
	}
	return sourceList(a, "GetZones", rows), nil
}

// SetZones replaces the snap-zone set and returns the accepted count. An
// invalid rectangle (unknown zone id, negative extent) fails closed in the
// daemon: the error is returned AND logged, and no zone is applied.
func (a *App) SetZones(zones []ZoneRow) (int, error) {
	n, err := a.core.SetZones(zones)
	if err != nil {
		a.log.Append("SetZones: " + err.Error())
		return 0, err
	}
	return n, nil
}

// Commands serves the command palette (plugin commands arrive as data).
func (a *App) Commands() ([]CommandRow, error) {
	rows, err := a.core.Commands()
	if err != nil {
		a.log.Append("Commands: " + err.Error())
		return nil, err
	}
	return sourceList(a, "Commands", rows), nil
}

// PluginSchemas serves the declarative plugin-settings forms.
func (a *App) PluginSchemas() ([]SchemaRow, error) {
	rows, err := a.core.PluginSchemas()
	if err != nil {
		a.log.Append("PluginSchemas: " + err.Error())
		return nil, err
	}
	return sourceList(a, "PluginSchemas", rows), nil
}

// OwnershipAudit serves the Safety page list of what CrossOS created.
func (a *App) OwnershipAudit() ([]AuditRow, error) {
	rows, err := a.core.OwnershipAudit()
	if err != nil {
		a.log.Append("OwnershipAudit: " + err.Error())
		return nil, err
	}
	return sourceList(a, "OwnershipAudit", rows), nil
}

// TrialState serves the Safety page countdown. "none" travels as a value with
// no log line — a real "no trial in flight" is not an incident. A null result
// is: the daemon owes an object here, and the page is told so.
func (a *App) TrialState() (TrialState, error) {
	st, err := a.core.TrialState()
	if err != nil {
		a.log.Append("TrialState: " + err.Error())
		return TrialState{}, err
	}
	return st, nil
}

// Readiness serves the onboarding checklist (keyboard / windows / finder).
func (a *App) Readiness() ([]ReadinessRow, error) {
	rows, err := a.core.Readiness()
	if err != nil {
		a.log.Append("Readiness: " + err.Error())
		return nil, err
	}
	return sourceList(a, "Readiness", rows), nil
}

// Profiles serves the profile cards. An unavailable capability keeps its
// reason: a profile the spec promises and CrossOS does not deliver is shown as
// a gap, so the row arrives whole and the card decides.
func (a *App) Profiles() ([]ProfileRow, error) {
	rows, err := a.core.Profiles()
	if err != nil {
		a.log.Append("Profiles: " + err.Error())
		return nil, err
	}
	return sourceList(a, "Profiles", rows), nil
}

// ApplyProfile turns a whole profile on. A denial returns the daemon's error
// and no counts: a card that rendered "0 rules, 0 plugins" from a rejected
// call would read as a profile that applied and did nothing.
func (a *App) ApplyProfile(profileID string) (map[string]any, error) {
	res, err := a.core.ApplyProfile(profileID)
	if err != nil {
		a.log.Append("ApplyProfile " + profileID + ": " + err.Error())
		return nil, err
	}
	return res, nil
}

// Traces serves the decision trace as rows.
func (a *App) Traces() ([]TraceRow, error) {
	rows, err := a.core.Traces()
	if err != nil {
		a.log.Append("Traces: " + err.Error())
		return nil, err
	}
	return sourceList(a, "Traces", rows), nil
}

// PluginMeta serves the plugin manifest facts the Plugins page shows.
func (a *App) PluginMeta() ([]PluginMetaRow, error) {
	rows, err := a.core.PluginMeta()
	if err != nil {
		a.log.Append("PluginMeta: " + err.Error())
		return nil, err
	}
	return sourceList(a, "PluginMeta", rows), nil
}

// Apps serves the installed-application list behind the rule builder's app
// picker. A machine with no cheap enumeration answers [], which is a value the
// picker renders as "type the bundle id" — never an error page.
func (a *App) Apps() ([]AppRow, error) {
	rows, err := a.core.Apps()
	if err != nil {
		a.log.Append("Apps: " + err.Error())
		return nil, err
	}
	return sourceList(a, "Apps", rows), nil
}

// UserRules serves the person-authored rule table.
func (a *App) UserRules() ([]UserRuleRow, error) {
	rows, err := a.core.UserRules()
	if err != nil {
		a.log.Append("UserRules: " + err.Error())
		return nil, err
	}
	return sourceList(a, "UserRules", rows), nil
}

// SetUserRule creates or updates one rule and returns the id the daemon
// derived from the dimensions. The id is the only part of the reply the editor
// could not have computed — an edit that re-picks the chord renames the rule —
// and the next delete has to name the row that exists.
func (a *App) SetUserRule(rule UserRuleRow) (string, error) {
	id, err := a.core.SetUserRule(rule)
	if err != nil {
		a.log.Append("SetUserRule: " + err.Error())
		return "", err
	}
	return id, nil
}

// DeleteUserRule removes one rule and answers with the table as it now stands.
// The same rows a fresh read would give, so the editor updates from one
// response instead of re-reading to find out whether its write landed.
func (a *App) DeleteUserRule(id string) ([]UserRuleRow, error) {
	rows, err := a.core.DeleteUserRule(id)
	if err != nil {
		a.log.Append("DeleteUserRule " + id + ": " + err.Error())
		return nil, err
	}
	return sourceList(a, "DeleteUserRule", rows), nil
}
