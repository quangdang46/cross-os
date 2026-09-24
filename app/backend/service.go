// Service adapter: exposes the shell backend to the Wails v3 binding
// generator as one service (bead cross-os-4gb).
//
// The backend App + Host stay Wails-free (pure Go, no Wails import); this
// struct is the ONLY Wails-facing seam — method names here become the
// TypeScript API under frontend/bindings. Page discovery stays Host-owned:
// Pages() serves whatever the Host discovered, never a hardcoded list.
package shell

// Service is the Wails-bound service: bridge calls + page discovery.
type Service struct {
	app  *App
	host *Host
}

// NewService binds the bridge App and the discovery Host.
func NewService(app *App, host *Host) *Service { return &Service{app: app, host: host} }

// Pages serves the Host-discovered pages (§7.2: rendered by discovery).
func (s *Service) Pages() []Page { return s.host.Pages() }

// GetStatus serves the Dashboard page.
func (s *Service) GetStatus() *Status { return s.app.GetStatus() }

// TogglePlugin enables/disables one plugin (returned AND UI-logged).
func (s *Service) TogglePlugin(id string, enabled bool) error {
	return s.app.TogglePlugin(id, enabled)
}

// ResetEverything returns the audited reset plan (§8.2 ownership scope).
func (s *Service) ResetEverything() []string { return s.app.ResetEverything() }

// GetEventLogs serves the Activity page trace.
func (s *Service) GetEventLogs() []string { return s.app.GetEventLogs() }

// UILogs exposes the UI-visible log sink (bridge/IPC failures land here).
func (s *Service) UILogs() []string { return s.app.UILogs() }

// CheckForUpdate serves the update badge: whether manifest.Version is newer
// than the running daemon (no shell-side network — daemon compares).
func (s *Service) CheckForUpdate(manifestVersion, platform, url, sha256 string) (bool, string, error) {
	return s.app.CheckForUpdate(manifestVersion, platform, url, sha256)
}

// ApplyUpdate applies a manifest update with explicit user approval (§8.4).
func (s *Service) ApplyUpdate(manifestVersion, platform, url, sha256 string, approved bool, approvedBy string) (string, error) {
	return s.app.ApplyUpdate(manifestVersion, platform, url, sha256, approved, approvedBy)
}

// PanicStop executes the kill switch (Safety page PANIC STOP button).
func (s *Service) PanicStop() (map[string]any, error) {
	return s.app.PanicStop()
}

// Resume clears a latched PANIC STOP (Safety page re-enable).
func (s *Service) Resume() (map[string]any, error) { return s.app.Resume() }

// BeginTrial starts the enable trial (Safety page countdown).
func (s *Service) BeginTrial(pluginID string) (string, error) {
	return s.app.BeginTrial(pluginID)
}

// ConfirmTrial completes the trial (Safety page Confirm button).
func (s *Service) ConfirmTrial(pluginID string, confirmed, healthy bool) (string, error) {
	return s.app.ConfirmTrial(pluginID, confirmed, healthy)
}

// RollbackTrial aborts the trial (Safety page rollback).
func (s *Service) RollbackTrial(pluginID, reason string) (string, error) {
	return s.app.RollbackTrial(pluginID, reason)
}

// SetRuleEnabled toggles one matrix row (Keyboard page matrix control).
func (s *Service) SetRuleEnabled(ruleID string, enabled bool) (bool, error) {
	return s.app.SetRuleEnabled(ruleID, enabled)
}

// Shortcuts serves the Windows page editor table.
func (s *Service) Shortcuts() ([]map[string]any, error) {
	return s.app.Shortcuts()
}

// SetShortcuts replaces the shortcut table (Windows page editor).
func (s *Service) SetShortcuts(shortcuts []map[string]any) (int, error) {
	return s.app.SetShortcuts(shortcuts)
}

// The ten source bindings below are the Wails-bound surface the pages read
// (bead cross-os-72g). Their names ARE the frontend API — App.tsx calls them
// and nothing else, so they change only with the shim, never per page. The
// Wave 3 bindings after them follow the same rule for the same reason: a page
// that needs a new source calls one of these names rather than growing a
// private path to the daemon.

// GetMatrix serves the Keyboard page behavior matrix.
func (s *Service) GetMatrix() ([]MatrixRow, error) {
	return s.app.GetMatrix()
}

// GetOverrides serves the Keyboard page app-override table.
func (s *Service) GetOverrides() ([]OverrideRow, error) {
	return s.app.GetOverrides()
}

// SetOverride toggles one app override (returns the daemon's stored row).
func (s *Service) SetOverride(app, ruleID string, enabled bool) (OverrideRow, error) {
	return s.app.SetOverride(app, ruleID, enabled)
}

// GetZones serves the Windows page snap-zone editor.
func (s *Service) GetZones() ([]ZoneRow, error) {
	return s.app.GetZones()
}

// SetZones replaces the snap-zone set (returns the accepted count).
func (s *Service) SetZones(zones []ZoneRow) (int, error) {
	return s.app.SetZones(zones)
}

// Commands serves the command palette.
func (s *Service) Commands() ([]CommandRow, error) {
	return s.app.Commands()
}

// PluginSchemas serves the declarative plugin-settings forms.
func (s *Service) PluginSchemas() ([]SchemaRow, error) {
	return s.app.PluginSchemas()
}

// OwnershipAudit serves the Safety page "What CrossOS created" list.
func (s *Service) OwnershipAudit() ([]AuditRow, error) {
	return s.app.OwnershipAudit()
}

// TrialState serves the Safety page countdown ("none" when idle).
func (s *Service) TrialState() (TrialState, error) {
	return s.app.TrialState()
}

// Readiness serves the onboarding readiness checklist.
func (s *Service) Readiness() ([]ReadinessRow, error) {
	return s.app.Readiness()
}

// The Wave 3 source bindings (bead w3-shell-bridge): the profile cards, the
// decision trace, the plugin manifest facts, the app list the rule builder
// picks from, and the person-authored rule table.

// Profiles serves the profile cards.
func (s *Service) Profiles() ([]ProfileRow, error) { return s.app.Profiles() }

// ApplyProfile activates one profile's capabilities in one plan.
func (s *Service) ApplyProfile(profileID string) (map[string]any, error) {
	return s.app.ApplyProfile(profileID)
}

// Traces serves the recorded decisions as rows.
func (s *Service) Traces() ([]TraceRow, error) { return s.app.Traces() }

// PluginMeta serves the plugin manifest facts.
func (s *Service) PluginMeta() ([]PluginMetaRow, error) { return s.app.PluginMeta() }

// Apps serves the installed applications a rule may be scoped to.
func (s *Service) Apps() ([]AppRow, error) { return s.app.Apps() }

// UserRules serves the person-authored rule table.
func (s *Service) UserRules() ([]UserRuleRow, error) { return s.app.UserRules() }

// SetUserRule creates or updates one rule (returns the derived id).
func (s *Service) SetUserRule(rule UserRuleRow) (string, error) {
	return s.app.SetUserRule(rule)
}

// DeleteUserRule removes one rule (returns the table as it now stands).
func (s *Service) DeleteUserRule(id string) ([]UserRuleRow, error) {
	return s.app.DeleteUserRule(id)
}

// The switcher's three bindings. The list and the wait are the sources the
// page reads; SwitcherFocus is the one write, and it is the same action the
// commit half of the chord performs.

// Windows serves the switcher's tiles in the daemon's order, with the
// highlight on the row the daemon picked.
func (s *Service) Windows() ([]WindowRow, error) { return s.app.Windows() }

// SwitcherWait is the bounded long poll; an expired budget answers
// triggered=false rather than an error.
func (s *Service) SwitcherWait(timeoutMs int) (SwitcherTrigger, error) {
	return s.app.SwitcherWait(timeoutMs)
}

// SwitcherFocus brings the tile the user pointed at to the front.
func (s *Service) SwitcherFocus(windowID string) error { return s.app.SwitcherFocus(windowID) }
