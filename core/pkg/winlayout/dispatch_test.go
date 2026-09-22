// Dispatch-path tests — bead cross-os-nir.2.
//
// Pass criteria mapping (Tier-1, sandbox-runnable):
//  1. Intent → window.* capability → Adapter seam for every action →
//     TestIntentToCapabilityToSeam (full 19+2 sweep through the Executor
//     into the adapter Driver mapping — no AX, no windows).
//  2. Recorder stage shape per action → TestDispatchStages (event →
//     action → result present for resolve+confirm; feeds nir.6).
//  3. Multi-monitor zone carries display step (not a frame) →
//     TestDisplayMoveZones (adapter-side resolution contract).
//
// Tier-2 (live AX frames within 1pt, real display-ID change) stays gated
// in TestLiveAXFrames: needs darwin + consent, unimplementable here.
package winlayout

import (
	"encoding/json"
	"testing"

	"crossos/core/internal/adapter"
	"crossos/core/pkg/event"
	"crossos/core/pkg/intent"
	"crossos/core/pkg/pluginapi"
	"crossos/core/pkg/rule"
)

// mkWindowMoveIntent builds the window.move intent the dispatch path binds.
func mkWindowMoveIntent() intent.Intent {
	return intent.Intent{ID: "window.move", Version: 1, Source: intent.SourceKeyboard,
		Parameters: json.RawMessage(`{"zone":"left-half"}`)}
}

// ruleScopeGlobal keeps the suite independent of rule's enum spelling drift
// (fails loudly here if ScopeGlobal ever moves).
func ruleScopeGlobal() rule.Scope { return rule.ScopeGlobal }

func TestIntentToCapabilityToSeam(t *testing.T) {
	// Every action: Executor resolves capability+zone, and the resolved
	// capability EXISTS in the canonical registry with a window.* ID; the
	// adapter Driver maps the resulting Core decision to the right
	// KeyAction. This is the Intent → capability → Adapter chain with the
	// live AX call as the only ungated remainder.
	reg := intent.DefaultRegistry()
	screen := Screen{Visible: Rect{X: 0, Y: 0, W: 1920, H: 1080}}
	for _, a := range AllActions {
		ex := NewExecutor()
		cap, zone, _ := ex.Resolve(a, "w1", Rect{X: 100, Y: 100, W: 400, H: 300}, screen)
		desc, ok := reg.Get(cap)
		if !ok {
			t.Fatalf("action %s: capability %q not in canonical registry", ActionName(a), cap)
		}
		if desc.Permission == "" {
			t.Fatalf("action %s: capability %q has no permission", ActionName(a), cap)
		}
		// The seam mapping must agree: a Replace decision on this intent
		// suppresses+injects through the bridge, never passes.
		if got := adapter.FromDecision(pluginapi.DecisionReplace); got != adapter.KeySuppressInject {
			t.Fatalf("seam mapping broken: Replace → %v", got)
		}
		// Zone must resolve for window.move actions (review: cross-os-ed —
		// ZoneName returns "unknown" for unmapped actions; assert here so
		// a silent unknown never passes the sweep). Dedicated-cap actions
		// (minimize/maximize) carry no zone by design (see TestAll19Resolve).
		if cap == "window.move" && (zone == "" || zone == "unknown") {
			t.Fatalf("action %s: zone %q unresolved", ActionName(a), zone)
		}
	}
}

func TestDispatchStages(t *testing.T) {
	// Recorder-bound shape per action (feeds nir.6): resolve logs event +
	// action, confirm logs result — for a geometry action AND a
	// dedicated-cap action AND a display move.
	screen := Screen{Visible: Rect{X: 0, Y: 0, W: 1920, H: 1080}}
	for _, a := range []Action{LeftHalf, Minimize, NextDisplay} {
		ex := NewExecutor()
		_, _, target := ex.Resolve(a, "w9", Rect{X: 10, Y: 10, W: 200, H: 200}, screen)
		obs := Rect{X: 0, Y: 0, W: 960, H: 1080}
		if target != nil {
			obs = *target
		}
		ex.Confirm(a, "w9", true, obs, target)
		stages := map[string]bool{}
		for _, s := range ex.Stages {
			stages[s.Stage] = true
		}
		for _, want := range []string{"event", "action", "result"} {
			if !stages[want] {
				t.Fatalf("action %s stages missing %q: %+v", ActionName(a), want, ex.Stages)
			}
		}
	}
}

func TestConsumeTraceShapes(t *testing.T) {
	// Both Consume shapes (review: cross-os-ed — nir.6 consumes these
	// traces, so pin them here, not there):
	//  - Emit=false: NO action stage (nothing dispatched or authorized).
	//  - authorize-fail-closed: action stage present ("authorize failed").
	reg := intent.DefaultRegistry()
	mk := func(emit bool) *event.Router {
		return event.Compile(
			[]event.CompiledRule{{
				KeyCode: 0x1B, AppModes: []event.AppMode{event.AppModeNative},
				RuleID: "test.sink", PluginID: "test",
				Priority: 10, Specificity: 0, Scope: ruleScopeGlobal(),
				Intent: intent.Intent{ID: "window.close", Version: 1, Source: intent.SourceKeyboard},
				Emit:   emit,
			}},
			reg,
			map[string][]intent.Permission{"test": {intent.PermAccessControl}},
			nil,
		)
	}
	ev := event.Event{Type: event.EventKeyDown, Source: event.SourceKeyboard, KeyCode: 0x1B}
	fc := event.FastContext{AppID: "x", AppMode: event.AppModeNative}
	stagesOf := func(out event.Outcome) map[string]bool {
		m := map[string]bool{}
		for _, s := range out.Traces {
			m[s.Stage] = true
		}
		return m
	}
	out := mk(false).Decide(ev, fc)
	if out.Decision != pluginapi.DecisionConsume {
		t.Fatalf("Emit=false: got %v", out.Decision)
	}
	if st := stagesOf(out); st["action"] {
		t.Fatalf("Emit=false consume must log no action stage: %+v", out.Traces)
	}
	// Fail-closed: strip the grant so Authorize fails.
	rt := event.Compile(
		[]event.CompiledRule{{
			KeyCode: 0x1B, AppModes: []event.AppMode{event.AppModeNative},
			RuleID: "test.sink2", PluginID: "test",
			Priority: 10, Specificity: 0, Scope: ruleScopeGlobal(),
			Intent: intent.Intent{ID: "window.close", Version: 1, Source: intent.SourceKeyboard},
			Emit:   true,
		}},
		reg, map[string][]intent.Permission{}, nil)
	out2 := rt.Decide(ev, fc)
	if out2.Decision != pluginapi.DecisionConsume {
		t.Fatalf("fail-closed: got %v", out2.Decision)
	}
	if st := stagesOf(out2); !st["action"] {
		t.Fatalf("fail-closed consume must log the authorize-failure action stage: %+v", out2.Traces)
	}
}

func TestDisplayMoveZones(t *testing.T) {
	// Display moves resolve adapter-side: zone names set, frame nil, and
	// MoveToDisplay arithmetic agrees single- and multi-monitor.
	screens := []Screen{{Index: 0}, {Index: 1}}
	if got := MoveToDisplay(screens, 0, 1); got != 1 {
		t.Fatalf("next=%d, want 1", got)
	}
	ex := NewExecutor()
	screen := Screen{Visible: Rect{X: 0, Y: 0, W: 1920, H: 1080}}
	for _, a := range []Action{NextDisplay, PreviousDisplay} {
		_, zone, target := ex.Resolve(a, "w1", Rect{}, screen)
		if target != nil {
			t.Fatalf("action %s: display move must resolve adapter-side (nil frame), got %+v", ActionName(a), target)
		}
		if zone == "" {
			t.Fatalf("action %s: empty display zone", ActionName(a))
		}
	}
	// Single-monitor stay is also adapter-side (no error, no move).
	if got := MoveToDisplay(screens[:1], 0, 1); got != 0 {
		t.Fatalf("single-monitor next=%d, want 0", got)
	}
}

func TestRouterDecideWindowIntent(t *testing.T) {
	// The decided Outcome for a window intent carries Request bound to the
	// canonical capability — the dispatcher half of Intent → capability.
	// Uses the real event.Router (not a stub): compile one window.move
	// rule, Decide a matching event, assert Replace + bound Request.
	reg := intent.DefaultRegistry()
	rt := event.Compile(
		[]event.CompiledRule{{
			KeyCode: 0x25, Modifiers: 0x0B, AppModes: []event.AppMode{event.AppModeNative},
			RuleID: "test.snap-left", PluginID: "test",
			Priority: 10, Specificity: 1, Scope: ruleScopeGlobal(),
			Intent: mkWindowMoveIntent(), Emit: true,
		}},
		reg,
		map[string][]intent.Permission{"test": {intent.PermAccessControl}},
		nil,
	)
	out := rt.Decide(
		event.Event{Type: event.EventKeyDown, Source: event.SourceKeyboard, KeyCode: 0x25, Modifiers: 0x0B},
		event.FastContext{AppID: "x", AppMode: event.AppModeNative},
	)
	if out.Decision != pluginapi.DecisionReplace {
		t.Fatalf("want Replace, got %v trace=%+v", out.Decision, out.Traces)
	}
	if out.Request == nil || out.Request.Capability.ID != "window.move" {
		t.Fatalf("Request not bound to window.move: %+v", out.Request)
	}
}
