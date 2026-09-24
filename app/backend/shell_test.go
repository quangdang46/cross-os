// Shell tests — bead cross-os-80g.
//
// Pass criteria mapping:
//  1. Core-registered Dashboard with zero hardcoded pages → TestCorePagesViaRegistry.
//  2. Plugin-removal acceptance → TestPluginRemoval (shell runs on Core-only
//     registries; unknown plugin IDs are a typed error, never a crash).
//  3. Bridge reaches Core (+ failures in UI logs) → TestBridge + TestBridgeFailuresLogged.
package shell

import (
	"errors"
	"testing"

	"crossos/core/pkg/pluginapi"
)

// coreRegistry builds a Registry with the MVP Core pages (§7.2) registered
// the same way plugins register theirs.
func coreRegistry() *pluginapi.Registry {
	r := pluginapi.NewRegistry(nil)
	for _, u := range []pluginapi.UIContribution{
		{ID: "core.dashboard", Location: pluginapi.UILocationSettingsPage, Title: "Dashboard"},
		{ID: "core.keyboard", Location: pluginapi.UILocationSettingsPage, Title: "Keyboard"},
		{ID: "core.activity", Location: pluginapi.UILocationSettingsPage, Title: "Activity"},
		{ID: "core.safety", Location: pluginapi.UILocationSettingsPage, Title: "Safety"},
		{ID: "core.settings", Location: pluginapi.UILocationSettingsPage, Title: "Settings"},
	} {
		if err := r.RegisterUI(u); err != nil {
			panic(err)
		}
	}
	return r
}

func TestCorePagesViaRegistry(t *testing.T) {
	h := NewHost()
	if err := h.Register(coreRegistry()); err != nil {
		t.Fatalf("Register core: %v", err)
	}
	pages := h.Pages()
	if len(pages) != 5 {
		t.Fatalf("pages=%d, want 5 Core pages via Registry (zero hardcoded)", len(pages))
	}
	want := map[string]bool{"core.dashboard": true, "core.keyboard": true, "core.activity": true, "core.safety": true, "core.settings": true}
	for _, p := range pages {
		if !want[p.ID] {
			t.Fatalf("unexpected page %q — shell must not hardcode pages", p.ID)
		}
	}
	// Duplicate registration is a conflict, never a silent overwrite.
	dup := pluginapi.NewRegistry(nil)
	if err := dup.RegisterUI(pluginapi.UIContribution{ID: "core.dashboard", Location: pluginapi.UILocationSettingsPage, Title: "Clash"}); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := h.Register(dup); err == nil {
		t.Fatal("duplicate page ID: want conflict error, got nil")
	}
}

func TestPluginRemoval(t *testing.T) {
	// Acceptance: delete plugin source → shell runs on Core-only registry.
	h := NewHost()
	if err := h.Register(coreRegistry()); err != nil {
		t.Fatalf("core-only register: %v", err)
	}
	if len(h.Pages()) == 0 {
		t.Fatal("core-only shell must still serve Core pages")
	}
	app := NewApp(&stubCore{plugins: []PluginState{{ID: "core", Enabled: true, Healthy: "healthy"}}})
	if err := app.TogglePlugin("deleted-plugin", true); err == nil {
		t.Fatal("toggle on removed plugin: want typed error, got nil")
	}
	if len(app.UILogs()) == 0 {
		t.Fatal("unknown-plugin toggle must land in UI logs, not vanish")
	}
}

type stubCore struct {
	running   bool
	safeMode  bool
	killed    bool
	intercept bool
	tapErr    string
	plugins   []PluginState
	logs      []string
	failSet   error
	// Page sources (bead cross-os-72g). The zero value leaves every list
	// nil, which is what the null-decode test needs; failSources makes every
	// source fail at once, which is what the logging test needs.
	matrix      []MatrixRow
	overrides   []OverrideRow
	zones       []ZoneRow
	commands    []CommandRow
	schemas     []SchemaRow
	audit       []AuditRow
	trial       TrialState
	readiness   []ReadinessRow
	failSources error
}

func (s *stubCore) IsRunning() bool    { return s.running }
func (s *stubCore) InSafeMode() bool   { return s.safeMode }
func (s *stubCore) IsKilled() bool     { return s.killed }
func (s *stubCore) Interception() bool { return s.intercept }
func (s *stubCore) Resume() (map[string]any, error) {
	return map[string]any{"resumed": true, "interception": s.intercept}, nil
}
func (s *stubCore) TapError() string       { return s.tapErr }
func (s *stubCore) Version() string        { return "v0.1.0" }
func (s *stubCore) Plugins() []PluginState { return s.plugins }
func (s *stubCore) EventLogs() []string    { return s.logs }
func (s *stubCore) Reset() []string {
	return []string{"remove login item", "disable extension", "clean owned state"}
}
func (s *stubCore) SetEnabled(id string, e bool) error {
	if s.failSet != nil {
		return s.failSet
	}
	for i := range s.plugins {
		if s.plugins[i].ID == id {
			s.plugins[i].Enabled = e
		}
	}
	return nil
}
func (s *stubCore) CheckForUpdate(mv, plat, url, sum string) (bool, string, error) {
	return mv == "v9.9.9", mv, nil
}
func (s *stubCore) ApplyUpdate(mv, plat, url, sum string, approved bool, by string) (string, error) {
	if !approved {
		return "", errors.New("shell: update needs approval")
	}
	return mv, nil
}
func (s *stubCore) PanicStop() (map[string]any, error) {
	return map[string]any{"interceptionDisabled": true, "loginItemKept": true}, nil
}
func (s *stubCore) BeginTrial(pluginID string) (string, error) { return "trial", nil }
func (s *stubCore) ConfirmTrial(pluginID string, confirmed, healthy bool) (string, error) {
	if !confirmed || !healthy {
		return "", errors.New("shell: trial not confirmed-healthy")
	}
	return "enabled", nil
}
func (s *stubCore) RollbackTrial(pluginID, reason string) (string, error) { return "disabled", nil }
func (s *stubCore) SetRuleEnabled(ruleID string, enabled bool) (bool, error) {
	if ruleID == "nope" {
		return false, errors.New("shell: unknown rule")
	}
	return enabled, nil
}
func (s *stubCore) Shortcuts() ([]map[string]any, error) {
	return []map[string]any{{"action": "Left Half", "modifiers": []string{"Ctrl", "Win"}, "key": "Left"}}, nil
}
func (s *stubCore) SetShortcuts(shortcuts []map[string]any) (int, error) {
	return len(shortcuts), nil
}

// The source stubs below stand in for the DAEMON's side of the two config
// writers, so they carry the validation the bridge deliberately does not
// duplicate: an unknown app/rule/zone id is rejected here and the bridge only
// has to surface that rejection. Tests that want a clean source leave the
// table nil (empty) instead of pre-filling it.
//
// An EMPTY list is deliberately NOT rejected here. winlayout.ValidateZones and
// ValidateShortcuts both return nil for a zero-length slice, and
// config.setZones/setShortcuts are full-replace — so the daemon accepts
// {"zones":[]} and {"shortcuts":[]} and clears the running set. A stub that
// refused it would verify "writes fail closed" against a rule production does
// not have, and the two test files would then pin opposite contracts for the
// same call. The UI's guard against firing that write by accident lives in the
// editors (Save is disabled until there is a draft), not here.

func (s *stubCore) Matrix() ([]MatrixRow, error) {
	if s.failSources != nil {
		return nil, s.failSources
	}
	return s.matrix, nil
}

func (s *stubCore) Overrides() ([]OverrideRow, error) {
	if s.failSources != nil {
		return nil, s.failSources
	}
	return s.overrides, nil
}

func (s *stubCore) SetOverride(app, ruleID string, enabled bool) (OverrideRow, error) {
	if s.failSources != nil {
		return OverrideRow{}, s.failSources
	}
	if app == "" {
		return OverrideRow{}, errors.New("shell: no app for override")
	}
	// A rule is real if the matrix declares it or an override already does.
	known := false
	for _, m := range s.matrix {
		if m.RuleID == ruleID {
			known = true
		}
	}
	for _, o := range s.overrides {
		if o.RuleID == ruleID {
			known = true
		}
	}
	if !known {
		return OverrideRow{}, errors.New("shell: unknown rule " + ruleID)
	}
	for i := range s.overrides {
		if s.overrides[i].App == app && s.overrides[i].RuleID == ruleID {
			s.overrides[i].Enabled = enabled
			return s.overrides[i], nil
		}
	}
	return OverrideRow{}, errors.New("shell: no override for " + app + "/" + ruleID)
}

func (s *stubCore) Zones() ([]ZoneRow, error) {
	if s.failSources != nil {
		return nil, s.failSources
	}
	return s.zones, nil
}

func (s *stubCore) SetZones(zones []ZoneRow) (int, error) {
	if s.failSources != nil {
		return 0, s.failSources
	}
	for _, z := range zones {
		if z.ID == "" {
			return 0, errors.New("shell: zone without id")
		}
		if z.W <= 0 || z.H <= 0 {
			return 0, errors.New("shell: zone " + z.ID + " has no area")
		}
	}
	s.zones = append([]ZoneRow(nil), zones...)
	return len(s.zones), nil
}

func (s *stubCore) Commands() ([]CommandRow, error) {
	if s.failSources != nil {
		return nil, s.failSources
	}
	return s.commands, nil
}

func (s *stubCore) PluginSchemas() ([]SchemaRow, error) {
	if s.failSources != nil {
		return nil, s.failSources
	}
	return s.schemas, nil
}

func (s *stubCore) OwnershipAudit() ([]AuditRow, error) {
	if s.failSources != nil {
		return nil, s.failSources
	}
	return s.audit, nil
}

func (s *stubCore) TrialState() (TrialState, error) {
	if s.failSources != nil {
		return TrialState{}, s.failSources
	}
	return s.trial, nil
}

func (s *stubCore) Readiness() ([]ReadinessRow, error) {
	if s.failSources != nil {
		return nil, s.failSources
	}
	return s.readiness, nil
}

func TestBridge(t *testing.T) {
	c := &stubCore{running: true, plugins: []PluginState{{ID: "win-kb", Enabled: true, Healthy: "healthy"}}, logs: []string{"e1"}}
	app := NewApp(c)
	st := app.GetStatus()
	if !st.Running || len(st.Plugins) != 1 {
		t.Fatalf("GetStatus=%+v, want running + 1 plugin", st)
	}
	if err := app.TogglePlugin("win-kb", false); err != nil {
		t.Fatalf("TogglePlugin: %v", err)
	}
	if c.plugins[0].Enabled {
		t.Fatal("toggle did not reach Core")
	}
	if got := app.GetEventLogs(); len(got) != 1 {
		t.Fatalf("GetEventLogs=%v, want 1 line", got)
	}
	if got := app.ResetEverything(); len(got) != 3 {
		t.Fatalf("ResetEverything=%v, want 3 audited steps", got)
	}
}

func TestBridgeFailuresLogged(t *testing.T) {
	c := &stubCore{plugins: []PluginState{{ID: "win-kb", Enabled: true}}, failSet: errors.New("ipc: daemon unreachable")}
	app := NewApp(c)
	if err := app.TogglePlugin("win-kb", false); err == nil {
		t.Fatal("want bridge error, got nil")
	}
	if len(app.UILogs()) == 0 {
		t.Fatal("bridge failure must surface in UI logs (no silent blank page)")
	}
	nilLogs := NewApp(&stubCore{})
	if got := nilLogs.GetEventLogs(); got == nil || len(got) != 0 {
		t.Fatalf("nil backend logs must degrade to empty slice, got %v", got)
	}
	if len(nilLogs.UILogs()) == 0 {
		t.Fatal("nil-logs degradation must be noted in UI logs")
	}
}

func TestPanicStopSurfaced(t *testing.T) {
	app := NewApp(&stubCore{})
	res, err := app.PanicStop()
	if err != nil {
		t.Fatalf("PanicStop: %v", err)
	}
	if res["interceptionDisabled"] != true || res["loginItemKept"] != true {
		t.Fatalf("PanicStop=%v, want interception stopped + login kept", res)
	}
}

func TestTrialBridgeSurfaced(t *testing.T) {
	app := NewApp(&stubCore{})
	if st, err := app.BeginTrial("plug"); err != nil || st != "trial" {
		t.Fatalf("BeginTrial=%q,%v, want trial", st, err)
	}
	if st, err := app.ConfirmTrial("plug", true, true); err != nil || st != "enabled" {
		t.Fatalf("ConfirmTrial=%q,%v, want enabled", st, err)
	}
	if _, err := app.ConfirmTrial("plug", false, true); err == nil {
		t.Fatal("unconfirmed trial must fail closed")
	}
	if len(app.UILogs()) == 0 {
		t.Fatal("denied confirm must land in UI logs")
	}
	if st, err := app.RollbackTrial("plug", "cancel"); err != nil || st != "disabled" {
		t.Fatalf("RollbackTrial=%q,%v, want disabled", st, err)
	}
}

func TestConfigBridgeSurfaced(t *testing.T) {
	app := NewApp(&stubCore{})
	if ok, err := app.SetRuleEnabled("windows-keyboard.ctrl-c-copy", false); err != nil || ok != false {
		t.Fatalf("SetRuleEnabled=%v,%v, want false,nil", ok, err)
	}
	if _, err := app.SetRuleEnabled("nope", false); err == nil {
		t.Fatal("unknown rule must fail closed")
	}
	if len(app.UILogs()) == 0 {
		t.Fatal("denied toggle must land in UI logs")
	}
	rows, err := app.Shortcuts()
	if err != nil || len(rows) != 1 || rows[0]["action"] != "Left Half" {
		t.Fatalf("Shortcuts=%v,%v", rows, err)
	}
	if n, err := app.SetShortcuts(rows); err != nil || n != 1 {
		t.Fatalf("SetShortcuts=%d,%v, want 1", n, err)
	}
	// Clearing the table is a legal write: winlayout.ValidateShortcuts returns
	// nil for an empty slice and config.setShortcuts is full-replace, so the
	// daemon clears the running set. Asserting the stub REFUSED it would pin a
	// rule production does not have.
	if n, err := app.SetShortcuts(nil); err != nil || n != 0 {
		t.Fatalf("SetShortcuts(nil)=%d,%v, want 0,nil (the daemon accepts a clear)", n, err)
	}
}
