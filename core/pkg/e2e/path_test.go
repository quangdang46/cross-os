package e2e

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"crossos/core/pkg/approute"
	"crossos/core/pkg/ctx"
	"crossos/core/pkg/event"
	"crossos/core/pkg/intent"
	"crossos/core/pkg/keyboard"
	"crossos/core/pkg/pluginapi"
	"crossos/core/pkg/record"
	"crossos/core/pkg/rule"
)

// harness wires the full decision path: keyboard state → ctx cache →
// approute classification → event router → recorder. One place, so every
// e2e case exercises the real composition, not a mock of it.
type harness struct {
	state  *keyboard.State
	cache  *ctx.Cache
	router *event.Router
	rec    *record.Recorder
	route  *approute.Router
}

func newHarness() *harness {
	reg := intent.DefaultRegistry()
	mk := func(id, params string) intent.Intent {
		return intent.Intent{ID: id, Version: 1, Source: intent.SourceKeyboard,
			Parameters: json.RawMessage(params)}
	}
	// Same shape as the s4i matrix (core must not import plugins).
	rules := []event.CompiledRule{
		{KeyCode: 0x73, Modifiers: 1 << 2, AppModes: []event.AppMode{"native", "terminal"},
			RuleID: "kb.alt-f4", PluginID: "windows-keyboard",
			Priority: rule.PriorityGlobal, Specificity: 1, Scope: rule.ScopeGlobal,
			Intent: mk("window.close", ""), Emit: true},
		{KeyCode: 0x43, Modifiers: 1 << 0, AppModes: []event.AppMode{"native"},
			RuleID: "kb.ctrl-c", PluginID: "windows-keyboard",
			Priority: rule.PriorityApp, Specificity: 1, Scope: rule.ScopeApp,
			Intent: mk("clipboard.copy", ""), Emit: true},
		{KeyCode: 0x1B, Modifiers: 0,
			RuleID: "kb.esc-sink", PluginID: "windows-keyboard",
			Priority: rule.PriorityGlobal, Specificity: 0, Scope: rule.ScopeGlobal,
			Intent: mk("window.close", ""), Emit: false},
	}
	grants := map[string][]intent.Permission{
		"windows-keyboard": {intent.PermInputIntercept, intent.PermAccessControl},
	}
	h := &harness{
		state: &keyboard.State{},
		cache: ctx.NewCache(nil),
		rec:   record.NewRecorder(),
		route: approute.New(approute.Lists{}),
	}
	h.router = event.Compile(rules, reg, grants, nil)
	return h
}

// press runs one key event through the FULL path: state update → (skip on
// passthrough) → cache read + modifier overlay → approute mode → decide →
// record. Returns the outcome.
//
// Window/device flow (review fix: cross-os-c0): the harness populates the
// cache's window + device identity (UpdateWindow/UpdateDevice) and threads
// ALL FIVE ToEventFields through to the router — scope-window rules,
// device-filtered rules, and window-class matching are covered e2e, not
// silently dropped at the boundary.
func (h *harness) press(t *testing.T, ke keyboard.KeyEvent, bundleID, exe string) event.Outcome {
	t.Helper()
	return h.pressFull(t, ke, bundleID, exe, "w1", "AXWindow", "kbd-1")
}

func (h *harness) pressFull(t *testing.T, ke keyboard.KeyEvent, bundleID, exe, windowID, winClass, deviceID string) event.Outcome {
	t.Helper()
	if pt := h.state.Update(ke); pt {
		// Synthetic/composition passthrough: still record the pass. These
		// harness-synthesized outcomes are trace-incomplete BY CONSTRUCTION
		// (no router ran — event/context/result only); recorder-downstream
		// consumers must expect short traces here. (review: cross-os-c0)
		out := event.Outcome{
			Event:    event.Event{Type: event.EventKeyDown, Source: event.SourceKeyboard, KeyCode: ke.KeyCode, Modifiers: ke.Modifiers},
			Decision: pluginapi.DecisionPass,
			Traces: []event.StageLog{
				{Stage: "event", Detail: "synthetic/composition passthrough"},
				{Stage: "context", Detail: "skipped (no router)"},
				{Stage: "result", Detail: "pass (harness-synthesized)"},
			},
		}
		h.rec.OnOutcome(out)
		return out
	}
	mode, _, _ := h.route.Route(bundleID, exe)
	h.cache.UpdateApp(ctx.ApplicationInfo{BundleID: bundleID, Executable: exe, AppMode: mode})
	h.cache.UpdateWindow(ctx.WindowInfo{WindowID: windowID, Role: winClass})
	h.cache.UpdateDevice(deviceID)
	fc := h.cache.Read()
	mods, _, _, _, _, _ := h.state.Words()
	fc = fc.WithModifiers(mods)
	appID, appMode, winID, wc, dev := fc.ToEventFields()
	out := h.router.Decide(
		event.Event{Type: event.EventKeyDown, Source: event.SourceKeyboard,
			KeyCode: ke.KeyCode, Modifiers: fc.Modifiers, DeviceID: dev},
		event.FastContext{AppID: appID, AppMode: event.AppMode(appMode),
			WindowID: winID, WinClass: wc, DeviceID: dev})
	h.rec.OnOutcome(out)
	return out
}

func TestTerminalInterruptE2E(t *testing.T) {
	// Ctrl+C in ghostty: state tracks chord, approute says terminal, no
	// rule claims terminal Ctrl+C → Pass (native INTERRUPT).
	h := newHarness()
	h.press(t, keyboard.KeyEvent{KeyCode: 0x11, Down: true}, "com.mitchellh.ghostty", "ghostty")
	out := h.press(t, keyboard.KeyEvent{KeyCode: 0x43, Down: true, Modifiers: keyboard.ModCtrl},
		"com.mitchellh.ghostty", "ghostty")
	if out.Decision != pluginapi.DecisionPass {
		t.Fatalf("want Pass (interrupt), got %v trace=%+v", out.Decision, out.Traces)
	}
}

func TestCloseWindowNotQuitE2E(t *testing.T) {
	// Alt+F4 in Finder: Alt tracked, rule matches, window.close dispatched.
	h := newHarness()
	h.press(t, keyboard.KeyEvent{KeyCode: 0x12, Down: true}, "com.apple.finder", "Finder")
	out := h.press(t, keyboard.KeyEvent{KeyCode: 0x73, Down: true, Modifiers: keyboard.ModAlt},
		"com.apple.finder", "Finder")
	if out.Decision != pluginapi.DecisionReplace {
		t.Fatalf("want Replace, got %v", out.Decision)
	}
	if out.Request == nil || out.Request.Capability.ID != "window.close" {
		t.Fatalf("want window.close, got %+v", out.Request)
	}
}

func TestConsumePathE2E(t *testing.T) {
	// Esc hits the Emit=false sink → Consume, no dispatch.
	h := newHarness()
	out := h.press(t, keyboard.KeyEvent{KeyCode: 0x1B, Down: true},
		"com.apple.finder", "Finder")
	if out.Decision != pluginapi.DecisionConsume {
		t.Fatalf("want Consume, got %v", out.Decision)
	}
}

func TestEveryTraceHasAllStages(t *testing.T) {
	// Bead criterion: every ROUTED e2e trace contains event→result stage
	// logs. The harness overlays the LIVE mask from keyboard state, so the
	// chord needs its modifier HELD first (raw KeyEvent.Modifiers is
	// ignored by design — the real callback path works the same way).
	h := newHarness()
	h.press(t, keyboard.KeyEvent{KeyCode: 0x11, Down: true},
		"com.apple.finder", "Finder")
	h.press(t, keyboard.KeyEvent{KeyCode: 0x43, Down: true},
		"com.apple.finder", "Finder")
	h.press(t, keyboard.KeyEvent{KeyCode: 0x1B, Down: true},
		"com.apple.finder", "Finder")
	trs := h.rec.Traces()
	if len(trs) != 3 {
		t.Fatalf("want 3 traces (modifier + chord + esc), got %d", len(trs))
	}
	// Trace 1 (Replace chord) carries all six stages. Trace 2 (Consume
	// sink, Emit=false) correctly OMITS "action" — nothing was dispatched
	// or authorized; the router logs intent + consumed-result instead.
	// Trace 0 is the modifier-only press (no rule matches bare Ctrl):
	// event/context/result short trace.
	full := map[string]bool{}
	for _, s := range trs[1].Stages {
		full[s.Stage] = true
	}
	for _, want := range []string{"event", "context", "rule", "intent", "action", "result"} {
		if !full[want] {
			t.Fatalf("replace trace missing stage %q: %+v", want, trs[1].Stages)
		}
	}
	consume := map[string]bool{}
	for _, s := range trs[2].Stages {
		consume[s.Stage] = true
	}
	for _, want := range []string{"event", "context", "rule", "intent", "result"} {
		if !consume[want] {
			t.Fatalf("consume trace missing stage %q: %+v", want, trs[2].Stages)
		}
	}
	if consume["action"] {
		t.Fatalf("consume trace must not log action (nothing dispatched): %+v", trs[2].Stages)
	}
	if len(trs[0].Stages) != 3 {
		t.Fatalf("modifier-only trace should be short (event/context/result), got %+v", trs[0].Stages)
	}
}

func TestDecisionLatencyBudget(t *testing.T) {
	// Bead criterion: intercept decision under 1ms. Recorded benchmark:
	// full Decide (match + resolve + authorize + trace) timed per call.
	h := newHarness()
	ev := event.Event{Type: event.EventKeyDown, Source: event.SourceKeyboard, KeyCode: 0x43, Modifiers: keyboard.ModCtrl}
	fc := event.FastContext{AppID: "Finder", AppMode: event.AppModeNative}
	const n = 2000
	start := time.Now()
	for i := 0; i < n; i++ {
		_ = h.router.Decide(ev, fc)
	}
	per := time.Since(start) / n
	t.Logf("per-decision: %v (budget 1ms)", per)
	if per >= time.Millisecond {
		t.Fatalf("decision too slow: %v >= 1ms", per)
	}
}

func TestRemotePassthroughE2E(t *testing.T) {
	// Ctrl+C in RDP client: approute says remote → no rule claims → Pass.
	h := newHarness()
	out := h.press(t, keyboard.KeyEvent{KeyCode: 0x43, Down: true, Modifiers: keyboard.ModCtrl},
		"", "mstsc.exe")
	if out.Decision != pluginapi.DecisionPass {
		t.Fatalf("want Pass (remote passthrough), got %v", out.Decision)
	}
}

func TestPressReleaseCycleClearsState(t *testing.T) {
	// Keyup coverage (review: cross-os-c0): full Ctrl-down → C-down →
	// C-up → Ctrl-up cycle routes the chord mid-cycle AND leaves state
	// empty — a stuck-modifier regression (keyup failing to clear) fails
	// here instead of hiding.
	h := newHarness()
	h.press(t, keyboard.KeyEvent{KeyCode: 0x11, Down: true},
		"com.apple.finder", "Finder")
	out := h.press(t, keyboard.KeyEvent{KeyCode: 0x43, Down: true},
		"com.apple.finder", "Finder")
	if out.Decision != pluginapi.DecisionReplace {
		t.Fatalf("mid-cycle chord: got %v", out.Decision)
	}
	h.press(t, keyboard.KeyEvent{KeyCode: 0x43, Down: false},
		"com.apple.finder", "Finder")
	h.press(t, keyboard.KeyEvent{KeyCode: 0x11, Down: false},
		"com.apple.finder", "Finder")
	mods, w0, w1, w2, w3, ex := h.state.Words()
	if mods != 0 || w0 != 0 || w1 != 0 || w2 != 0 || w3 != 0 || ex != 0 {
		t.Fatalf("state not empty after release cycle: mods=%#x words=%#x,%#x,%#x,%#x ex=%#x",
			mods, w0, w1, w2, w3, ex)
	}
	// Post-release bare C is NOT a chord (Ctrl gone) → Pass.
	out = h.press(t, keyboard.KeyEvent{KeyCode: 0x43, Down: true},
		"com.apple.finder", "Finder")
	if out.Decision != pluginapi.DecisionPass {
		t.Fatalf("post-release bare C: got %v, want Pass", out.Decision)
	}
}

func TestWindowDeviceFieldsFlowE2E(t *testing.T) {
	// Boundary proof (review: cross-os-c0): window + device identity set
	// via pressFull reaches the decided outcome's Context. A regression
	// dropping fields at ToEventFields fails here.
	h := newHarness()
	out := h.pressFull(t, keyboard.KeyEvent{KeyCode: 0x43, Down: true, Modifiers: keyboard.ModCtrl},
		"com.apple.finder", "Finder", "w-42", "AXDialog", "kbd-9")
	if out.Context.WindowID != "w-42" || out.Context.WinClass != "AXDialog" ||
		out.Context.DeviceID != "kbd-9" {
		t.Fatalf("context fields dropped at boundary: %+v", out.Context)
	}
}

func TestRecorderSeesDecisions(t *testing.T) {
	// The recorder downstream of e2e holds one trace per press with the
	// decision + winner recorded. The harness overlays the LIVE modifier
	// mask from keyboard state (Words), not the KeyEvent.Modifiers field —
	// the raw 0x43-down event carries mods=0 until Ctrl is held, exactly
	// like the real callback path.
	h := newHarness()
	h.press(t, keyboard.KeyEvent{KeyCode: 0x11, Down: true},
		"com.apple.finder", "Finder")
	h.press(t, keyboard.KeyEvent{KeyCode: 0x43, Down: true},
		"com.apple.finder", "Finder")
	trs := h.rec.Traces()
	if len(trs) != 2 {
		t.Fatalf("want 2 traces (modifier press + chord), got %d", len(trs))
	}
	last := trs[1]
	if last.Decision != pluginapi.DecisionReplace || last.Winner != "kb.ctrl-c" {
		t.Fatalf("bad trace: %+v", last)
	}
	if strings.Contains(last.IntentID, "copyPath") {
		t.Fatal("stale copyPath mapping leaked into e2e")
	}
}
