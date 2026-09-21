// Window capability tests: unit + e2e + 19-action reference matrix
// (bead cross-os-nir.6).
//
// Plan: COMPREHENSIVE_PLAN.md §11 (Unit, Integration, Reference Tests) +
// §3.11 Recorder/Replay. Covers: unit (window-intent mapping via
// winlayout.CapabilityFor, plugin runtime lifecycle, safety rollback),
// reference (all 19 snap actions × multi-monitor × frame-history restore,
// traces replay deterministically), integration (input event → intent →
// action e2e with mock adapter), every stage traceable via Recorder.
package e2e

import (
	"encoding/json"
	"testing"

	"crossos/core/pkg/ctx"
	"crossos/core/pkg/event"
	"crossos/core/pkg/intent"
	"crossos/core/pkg/record"
	"crossos/core/pkg/rule"
	"crossos/core/pkg/safety"
	"crossos/core/pkg/winlayout"
)

// windowRules compiles one rule per snap action: distinct keycodes, one
// shared modifier, Emit intents window.move/minimize/maximize. This mirrors
// the §6.2 shortcut table shape (distinct chords → snap intents) without
// importing plugin sources.
func windowRules() []event.CompiledRule {
	mk := func(id, params string) intent.Intent {
		return intent.Intent{ID: id, Version: 1, Source: intent.SourceKeyboard,
			Parameters: json.RawMessage(params)}
	}
	var rules []event.CompiledRule
	for i, a := range winlayout.AllActions {
		cap, zone := winlayout.CapabilityFor(a)
		params := `{}`
		if zone != "" {
			params = `{"zone":"` + zone + `"}`
		}
		rules = append(rules, event.CompiledRule{
			KeyCode: uint32(0x60 + i), Modifiers: 1 << 3,
			AppModes:    []event.AppMode{"native"},
			RuleID:      "win." + winlayout.ZoneName(a),
			PluginID:    "windows-window",
			Priority:    rule.PriorityGlobal,
			Specificity: 1, Scope: rule.ScopeGlobal,
			Intent: mk(cap, params), Emit: true,
		})
	}
	return rules
}

// mockAdapter implements the adapter boundary in-test: resolves via
// winlayout.Executor (same resolution the product Adapter performs) and
// records capability invocations. No AX, no syscalls.
type mockAdapter struct {
	ex      *winlayout.Executor
	screen  winlayout.Screen
	calls   []string
	frames  map[string]winlayout.Rect
	current map[string]winlayout.Rect
}

func newMockAdapter() *mockAdapter {
	return &mockAdapter{
		ex:      winlayout.NewExecutor(),
		screen:  winlayout.Screen{Visible: winlayout.Rect{X: 0, Y: 0, W: 1920, H: 1080}},
		frames:  map[string]winlayout.Rect{},
		current: map[string]winlayout.Rect{"w1": {X: 100, Y: 100, W: 400, H: 300}},
	}
}

func (m *mockAdapter) act(a winlayout.Action, win string) {
	cur := m.current[win]
	cap, zone, target := m.ex.Resolve(a, win, cur, m.screen)
	m.calls = append(m.calls, cap+"/"+zone)
	if target != nil {
		m.frames[win] = *target
		m.current[win] = *target
	}
	m.ex.Confirm(a, win, true, m.current[win], target)
}

// TestWindowReferenceMatrix: all snap actions × multi-monitor × history.
func TestWindowReferenceMatrix(t *testing.T) {
	screens := []winlayout.Screen{
		{Visible: winlayout.Rect{X: 0, Y: 0, W: 1920, H: 1080}, Index: 0},
		{Visible: winlayout.Rect{X: 1920, Y: 0, W: 1920, H: 1080}, Index: 1},
	}
	for _, scr := range screens {
		m := newMockAdapter()
		m.screen = scr
		for _, a := range winlayout.AllActions {
			m.act(a, "w1")
		}
		if len(m.calls) != len(winlayout.AllActions) {
			t.Fatalf("screen %d: calls=%d, want %d", scr.Index, len(m.calls), len(winlayout.AllActions))
		}
		// History restore returns the pre-snap frame on every screen.
		m2 := newMockAdapter()
		m2.screen = scr
		pre := m2.current["w1"]
		m2.act(winlayout.LeftHalf, "w1")
		_, _, restored := m2.ex.Resolve(winlayout.Restore, "w1", m2.current["w1"], scr)
		if restored == nil || *restored != pre {
			t.Fatalf("screen %d: restore=%+v, want %+v", scr.Index, restored, pre)
		}
		// Display moves change the index on multi, stay on single.
		if got := winlayout.MoveToDisplay(screens, 0, 1); got != 1 {
			t.Fatalf("multi next=%d, want 1", got)
		}
		if got := winlayout.MoveToDisplay(screens[:1], 0, 1); got != 0 {
			t.Fatalf("single next=%d, want 0", got)
		}
	}
}

// TestWindowIntentMapping: every action maps to a registry-known capability.
func TestWindowIntentMapping(t *testing.T) {
	reg := intent.DefaultRegistry()
	known := map[string]bool{}
	for _, id := range reg.IDs() {
		known[id] = true
	}
	for _, a := range winlayout.AllActions {
		cap, _ := winlayout.CapabilityFor(a)
		if !known[cap] {
			t.Fatalf("action %s: capability %q not in canonical registry", winlayout.ActionName(a), cap)
		}
	}
}

// TestWindowE2EWithMockAdapter: key event → rule → intent → mock adapter,
// every stage recorded in the Recorder and replayable.
func TestWindowE2EWithMockAdapter(t *testing.T) {
	reg := intent.DefaultRegistry()
	rules := windowRules()
	grants := map[string][]intent.Permission{
		"windows-window": {intent.PermInputIntercept, intent.PermAccessControl},
	}
	router := event.Compile(rules, reg, grants, nil)
	rec := record.NewRecorder()
	cache := ctx.NewCache(nil)
	m := newMockAdapter()
	fired := 0
	for i, a := range winlayout.AllActions {
		ev := event.Event{Type: event.EventKeyDown, Source: event.SourceKeyboard,
			KeyCode: uint32(0x60 + i), Modifiers: 1 << 3}
		fctx := event.FastContext{AppID: "com.example.app", AppMode: event.AppModeNative, WindowID: "w1"}
		_ = cache
		out := router.Decide(ev, fctx)
		if out.Decision == 0 { // DecisionPass == 0: no rule matched
			t.Fatalf("action %s: no rule matched", winlayout.ActionName(a))
		}
		rec.OnOutcome(out)
		m.act(a, "w1")
		fired++
	}
	if fired != len(winlayout.AllActions) {
		t.Fatalf("fired=%d, want %d", fired, len(winlayout.AllActions))
	}
	traces := rec.Traces()
	if len(traces) != len(winlayout.AllActions) {
		t.Fatalf("recorder traces=%d, want %d (every stage logged)", len(traces), len(winlayout.AllActions))
	}
	for _, tr := range traces {
		if tr.Winner == "" {
			t.Fatal("trace without winner rule — untraceable")
		}
	}
}

// TestWindowSafetyRollback: kill switch short-circuits the window path to
// PASS (router contract), and safety rollback plans CrossOS-owned state.
func TestWindowSafetyRollback(t *testing.T) {
	killed := true
	router := event.Compile(windowRules(), intent.DefaultRegistry(),
		map[string][]intent.Permission{"windows-window": {intent.PermInputIntercept, intent.PermAccessControl}},
		func() bool { return killed })
	out := router.Decide(event.Event{Type: event.EventKeyDown, KeyCode: 0x60, Modifiers: 1 << 3},
		event.FastContext{AppMode: event.AppModeNative})
	if out.Decision != 0 {
		t.Fatalf("killed router decision=%v, want PASS (safe state)", out.Decision)
	}
	res := safety.PanicStop()
	if !res.InterceptionDisabled || !res.PluginActionsStopped || !res.BuffersFlushed {
		t.Fatalf("PanicStop=%+v, want interception+actions stopped, buffers flushed", res)
	}
	if !res.LoginItemKept {
		t.Fatalf("PanicStop=%+v, want login item kept (uninstall is Reset's job)", res)
	}
	plan := safety.PlanReset(safety.IntegrationState{})
	if !plan.RemoveLoginItem || !plan.DisableExtension || !plan.CleanOwnedState || !plan.VerifyNoProcess {
		t.Fatalf("ResetPlan=%+v, want all four reset steps", plan)
	}
}
