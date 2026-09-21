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
		Running:  a.core.IsRunning(),
		SafeMode: a.core.InSafeMode(),
		Killed:   a.core.IsKilled(),
		Plugins:  a.core.Plugins(),
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
