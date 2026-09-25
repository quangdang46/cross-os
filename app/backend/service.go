// Service adapter: exposes the shell backend to the Wails v3 binding
// generator as one service (bead cross-os-4gb).
//
// The backend App + Host stay Wails-free (pure Go, no Wails import); this
// struct is the ONLY Wails-facing seam — method names here become the
// TypeScript API under frontend/bindings. Page discovery stays Host-owned:
// Pages() serves whatever the Host discovered, never a hardcoded list.
package shell

import "sync"

// Service is the Wails-bound service: bridge calls + page discovery.
type Service struct {
	app  *App
	host *Host
	// seedMu guards the one-shot read of the daemon's first-run flag, and
	// seeded records that the read SUCCEEDED. A sync.Once would be the wrong
	// shape here: it burns on the first ATTEMPT, and a returning user's
	// daemon is not down at every call — it is down, or not yet answering, at
	// the one call that happens to land during a slow start. That would
	// strand them on the welcome wizard for the life of the process, which is
	// the exact symptom this exists to remove. On an error the memo stays
	// unset and the next Pages() retries.
	seedMu sync.Mutex
	seeded bool
}

// NewService binds the bridge App and the discovery Host.
func NewService(app *App, host *Host) *Service { return &Service{app: app, host: host} }

// Pages serves the Host-discovered pages (§7.2: rendered by discovery).
func (s *Service) Pages() []Page {
	s.seedFirstRun()
	return s.host.Pages()
}

// seedFirstRun reads the daemon's own first-run flag into the Host once, and
// re-sorts the nav from it.
//
// Both halves of the gate used to be open. The write never reached the Host,
// and the READ never came from the daemon at all: Host.onboarded was a
// process-local bool that NewHost always built false, while the daemon
// persisted the same fact and served it. So a user who finished the wizard
// was sent back to it on every launch, no matter how many times they clicked
// finish. One call from the daemon into the Host closes the read side.
//
// It fails CLOSED, and the direction is the whole decision. Until a read has
// succeeded, onboarded stays false and the first-run page keeps leading. A
// returning user whose daemon is down would otherwise land on Home and find
// every control on it refusing, which reads as a broken app; a first-run user
// with an unreachable daemon lands in the wizard, which is told what is
// wrong. The unreachable case is also almost exactly the fresh-profile case,
// which is the one the previous behaviour already handled by accident.
func (s *Service) seedFirstRun() {
	s.seedMu.Lock()
	defer s.seedMu.Unlock()
	if s.seeded {
		return
	}
	state, err := s.app.OnboardingState()
	if err != nil {
		return // fail closed, memo NOT consumed, the next call tries again
	}
	s.host.OnboardingComplete(state.Completed)
	s.seeded = true
}

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

// OnboardingState serves the first-run wizard: the done flag, the step the user
// is on, and the verdicts, all as the daemon derives them.
func (s *Service) OnboardingState() (OnboardingRow, error) {
	return s.app.OnboardingState()
}

// CompleteOnboarding dismisses the first-run wizard for good.
//
// The daemon write and the Host write are two different stores: the first
// survives a restart, the second is what the nav reads right now. Only the
// daemon's half was wired, so finishing the wizard changed nothing until the
// app was relaunched — and the relaunch then started a fresh Host that had
// never been told. Telling the Host here is what makes the landing page move
// in the session the user is actually in.
//
// The Host is told only after the daemon accepted the write. On a refusal the
// flow is not finished, and a nav that stopped leading would be a second
// lie on top of the first.
func (s *Service) CompleteOnboarding() error {
	if err := s.app.CompleteOnboarding(); err != nil {
		return err
	}
	s.host.OnboardingComplete(true)
	return nil
}

// The Wave 3 source bindings (bead w3-shell-bridge): the profile cards, the
// decision trace, the plugin manifest facts, the app list the rule builder
// picks from, and the person-authored rule table.

// OpenSystemSettings opens the System Settings pane the Accessibility grant
// is made in (permissions.openSettings).
//
// This is the first-run flow's one door onto another application. The wizard
// can already derive whether the permission is granted — the daemon re-derives
// that on every read — but the grant itself is made by hand, by a person, in a
// window this app does not own. Without this method the step that names the
// permission had no way to take the user there, and the step's button refused
// in words on every build.
//
// It opens a window and nothing else. The step's verdict stays the daemon's,
// so a successful open is never reported as a granted permission — that would
// be the shell inventing an answer the daemon deliberately did not give.
func (s *Service) OpenSystemSettings() error { return s.app.OpenSystemSettings() }

// SetObserve turns the recorder's dry-run on or off (core.setObserve).
//
// Observe mode is what lets a person read back what their rules do without
// disturbing the desktop: each decision that WOULD have fired is staged onto
// the trace instead. Turning it on is a change to what the machine does with a
// keypress, so the write is explicit and the position is read back rather than
// assumed.
func (s *Service) SetObserve(on bool) error { return s.app.SetObserve(on) }

// ObserveState reports the recorder's own position (core.observeState): the
// dry-run flag read from the recorder, and the privacy mode it is keeping.
//
// A product that records every keystroke decision owes the person reading it
// two things at once — where it stands, and what it is keeping. Both come from
// the daemon here, and neither is optimistically reported: a toggle that could
// only be written would label itself from the click that produced it.
func (s *Service) ObserveState() (ObserveStateRow, error) { return s.app.ObserveState() }

// Conflicts serves the chords two rules both claim (core.conflicts).
//
// A rule editor that cannot see its own collisions is a person saving a rule
// that quietly does nothing. The winner and the losers here are the decision
// path's own verdict, so "turn this one off" is advice about the rule that
// actually loses, not a guess from comparing two strings.
func (s *Service) Conflicts() ([]ConflictRow, error) { return s.app.Conflicts() }

// Profiles serves the profile cards.
func (s *Service) Profiles() ([]ProfileRow, error) { return s.app.Profiles() }

// ApplyProfile activates one profile's capabilities in one plan.
func (s *Service) ApplyProfile(profileID string) (map[string]any, error) {
	return s.app.ApplyProfile(profileID)
}

// Traces serves the recorded decisions as rows.
func (s *Service) Traces() ([]TraceRow, error) { return s.app.Traces() }

// TracesClear empties the recorder and serves the list as it stands.
func (s *Service) TracesClear() ([]TraceRow, error) { return s.app.TracesClear() }

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

// FinderMenu serves the Explorer's menu table.
func (s *Service) FinderMenu() ([]map[string]any, error) { return s.app.FinderMenu() }

// SetMenuItemEnabled toggles one Explorer menu row.
func (s *Service) SetMenuItemEnabled(id string, enabled bool) ([]map[string]any, error) {
	return s.app.SetMenuItemEnabled(id, enabled)
}
