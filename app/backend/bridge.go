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
