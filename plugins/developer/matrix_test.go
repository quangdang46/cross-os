package developer

import (
	"testing"
	"time"

	"crossos/core/pkg/event"
	"crossos/core/pkg/intent"
	"crossos/core/pkg/plugin"
	"crossos/core/pkg/pluginapi"
	"crossos/core/pkg/rule"
)

func testRouter(extra ...event.CompiledRule) *event.Router {
	reg := intent.DefaultRegistry()
	grants := map[string][]intent.Permission{pluginID: Grants()}
	return event.Compile(append(Matrix(), extra...), reg, grants, nil)
}

// TestFiveActions: the three claimed chords resolve; F8 passes through.
func TestFiveActions(t *testing.T) {
	rt := testRouter()
	cases := []struct {
		name   string
		key    uint32
		mods   uint32
		app    string
		intent string
		dec    pluginapi.Decision
	}{
		{"terminal", vkReturn, modCtrl | modShift, "com.apple.Finder", "terminal.openAt", pluginapi.DecisionReplace},
		{"copypath", vkC, modCtrl | modShift, "com.apple.Finder", "clipboard.copyPath", pluginapi.DecisionReplace},
		{"editor", vkP, modCtrl | modShift, "com.microsoft.VSCode", "app.open", pluginapi.DecisionReplace},
		{"f8-passthrough", vkF8, 0, "com.microsoft.VSCode", "", pluginapi.DecisionPass},
	}
	for _, c := range cases {
		out := rt.Decide(
			event.Event{Type: event.EventKeyDown, Source: event.SourceKeyboard, KeyCode: c.key, Modifiers: c.mods},
			event.FastContext{AppID: c.app, AppMode: event.AppModeNative})
		if out.Decision != c.dec {
			t.Fatalf("%s: decision %v, want %v", c.name, out.Decision, c.dec)
		}
		if c.intent != "" && out.Intent.ID != c.intent {
			t.Fatalf("%s: intent %q, want %q", c.name, out.Intent.ID, c.intent)
		}
	}
}

// TestShellGate: shell-bearing actions need Level B approve; the matrix
// itself only requests native capabilities. Enforcement is structural but
// lives one layer down from NeedsShellApproval (review: cross-os-ed):
// terminal.openAt itself carries PermShellExecution in the capability
// registry, and Router.Decide's Authorize step fails closed on a missing
// grant — NeedsShellApproval documents the boundary, it isn't the gate.
func TestShellGate(t *testing.T) {
	if !NeedsShellApproval("shell.execute") {
		t.Fatal("shell.execute must need Level B approve")
	}
	for _, cap := range []string{"terminal.openAt", "clipboard.copyPath", "app.open"} {
		if NeedsShellApproval(cap) {
			t.Fatalf("native capability %s must NOT need Level B", cap)
		}
	}
	// Matrix rules target native caps only — no rule dispatches shell.execute.
	reg := intent.DefaultRegistry()
	for _, r := range Matrix() {
		if _, ok := reg.Get(r.Intent.ID); !ok {
			t.Fatalf("rule %s targets unknown capability %q", r.RuleID, r.Intent.ID)
		}
		if r.Intent.ID == "shell.execute" {
			t.Fatalf("rule %s dispatches shell.execute as default (Level B only)", r.RuleID)
		}
	}
}

// TestPerAppScoping: developer rules (Specificity 2, app-scoped) beat the
// windows-keyboard-style global matrix (Specificity 1) on overlap, and the
// conflict log shows clean resolution with winner + losers.
func TestPerAppScoping(t *testing.T) {
	// Simulated windows-keyboard global claim on the same chord.
	global := event.CompiledRule{
		KeyCode: vkC, Modifiers: modCtrl | modShift,
		AppModes: []event.AppMode{event.AppModeNative},
		RuleID:   "windows-keyboard.ctrl-shift-c", PluginID: "windows-keyboard",
		Priority:    rule.PriorityGlobal,
		Specificity: 1, Scope: rule.ScopeGlobal,
		Intent: mkIntent("clipboard.copy", ""), Emit: true,
	}
	rt := testRouter(global)
	out := rt.Decide(
		event.Event{Type: event.EventKeyDown, Source: event.SourceKeyboard, KeyCode: vkC, Modifiers: modCtrl | modShift},
		event.FastContext{AppID: "com.apple.Finder", AppMode: event.AppModeNative})
	if out.WinnerRule != pluginID+".ctrl-shift-c-copypath" {
		t.Fatalf("winner=%q, want developer app-scoped rule (specificity 2 beats 1)", out.WinnerRule)
	}
	if len(out.Losers) == 0 {
		t.Fatal("conflict log must show losers (clean resolution)")
	}
	// Outside the scoped apps, the global claim still wins (no overreach).
	out2 := rt.Decide(
		event.Event{Type: event.EventKeyDown, Source: event.SourceKeyboard, KeyCode: vkC, Modifiers: modCtrl | modShift},
		event.FastContext{AppID: "com.other.app", AppMode: event.AppModeNative})
	if out2.WinnerRule != "windows-keyboard.ctrl-shift-c" {
		t.Fatalf("unscoped app winner=%q, want global claim", out2.WinnerRule)
	}
}

// TestConfigDrivenLists: terminals/editors are config (add = list append).
func TestConfigDrivenLists(t *testing.T) {
	// defer-restore (review nit: cross-os-ed) so a mid-test Fatal can't
	// leave the package globals polluted for the rest of the suite.
	origTerm, origEdit := SupportedTerminals, SupportedEditors
	defer func() { SupportedTerminals, SupportedEditors = origTerm, origEdit }()
	before := len(SupportedTerminals) + len(SupportedEditors)
	SupportedTerminals = append(append([]string{}, SupportedTerminals...), "TestTerm")
	SupportedEditors = append(append([]string{}, SupportedEditors...), "TestEdit")
	if len(SupportedTerminals)+len(SupportedEditors) != before+2 {
		t.Fatal("lists must be appendable config (no code change to add one)")
	}
	if PreferredEditor == "" {
		t.Fatal("preferred editor must have a default")
	}
}

// TestFinderItems: three context rows, capability shapes, no shell default.
func TestFinderItems(t *testing.T) {
	items := FinderItems()
	if len(items) != 3 {
		t.Fatalf("finder items=%d, want 3 (Terminal/VS Code/Cursor)", len(items))
	}
	for _, it := range items {
		if it.Title == "" || it.Capability == "" {
			t.Fatalf("incomplete row %+v", it)
		}
	}
}

// TestInstallsThroughLifecycle proves "shipped as installable pack through
// the Phase 4 lifecycle" (review: cross-os-ed — TestGrantsMatchManifest only
// proved grants⊆permissions, not installability): the manifest actually
// installs via the jpr.1 marketplace path (apiVersion-gated, TRIAL entry).
func TestInstallsThroughLifecycle(t *testing.T) {
	reg := plugin.NewRegistry("test")
	approve := plugin.Approval{Granted: true, By: "test-user"}
	p, err := reg.InstallManifest(Manifest(), "local", "v1", "0.2.0", time.Minute, approve, time.Now())
	if err != nil {
		t.Fatalf("InstallManifest: %v", err)
	}
	if p.Health != plugin.PackTrial || p.State != pluginapi.LifecycleTrial {
		t.Fatalf("health=%s state=%s, want TRIAL/trial", p.Health, p.State)
	}
}

// TestGrantsMatchManifest mirrors the windows-keyboard invariant.
func TestGrantsMatchManifest(t *testing.T) {
	m := Manifest()
	grantSet := map[string]bool{}
	for _, g := range Grants() {
		grantSet[string(g)] = true
	}
	for _, p := range m.Permissions {
		if !grantSet[p] {
			t.Fatalf("manifest declares %q but Grants() omits it", p)
		}
	}
}
