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
