package windowskeyboard

import (
	"testing"

	"crossos/core/pkg/event"
	"crossos/core/pkg/intent"
	"crossos/core/pkg/pluginapi"
)

func testRouter() *event.Router {
	reg := intent.DefaultRegistry()
	grants := map[string][]intent.Permission{pluginID: Grants()}
	return event.Compile(Matrix(), reg, grants, nil)
}

// expectedRow is one per-app row of the intent-first matrix (bead
// criterion: "10-shortcut matrix asserted as a per-app expected-Intent
// table").
type expectedRow struct {
	app      string
	appMode  event.AppMode
	keyCode  uint32
	mods     uint32
	decision pluginapi.Decision
	intentID string // "" when decision is Pass (no intent resolved)
}

func TestPerAppExpectedIntentMatrix(t *testing.T) {
	rt := testRouter()
	rows := []expectedRow{
		// Finder: Alt+F4 closes window, Ctrl+C copies.
		{"Finder", event.AppModeNative, vkF4, modAlt, pluginapi.DecisionReplace, "window.close"},
		{"Finder", event.AppModeNative, vkC, modCtrl, pluginapi.DecisionReplace, "clipboard.copy"},
		// Text editor (native mode): same matrix.
		{"Editor", event.AppModeNative, vkF4, modAlt, pluginapi.DecisionReplace, "window.close"},
		{"Editor", event.AppModeNative, vkC, modCtrl, pluginapi.DecisionReplace, "clipboard.copy"},
		// Browser (native mode): same matrix.
		{"Browser", event.AppModeNative, vkF4, modAlt, pluginapi.DecisionReplace, "window.close"},
		{"Browser", event.AppModeNative, vkC, modCtrl, pluginapi.DecisionReplace, "clipboard.copy"},
		// Terminal: Alt+F4 still closes the window; Ctrl+C is EXCLUDED so
		// it passes through as native INTERRUPT (bead's hard criterion).
		{"Terminal", event.AppModeTerminal, vkF4, modAlt, pluginapi.DecisionReplace, "window.close"},
		{"Terminal", event.AppModeTerminal, vkC, modCtrl, pluginapi.DecisionPass, ""},
		// Remote/VM: full passthrough, nothing matches.
		{"RemoteDesktop", event.AppModeRemote, vkF4, modAlt, pluginapi.DecisionPass, ""},
		{"VMware", event.AppModeVM, vkC, modCtrl, pluginapi.DecisionPass, ""},
	}
	for _, r := range rows {
		out := rt.Decide(
			event.Event{Type: event.EventKeyDown, Source: event.SourceKeyboard, KeyCode: r.keyCode, Modifiers: r.mods},
			event.FastContext{AppID: r.app, AppMode: r.appMode})
		if out.Decision != r.decision {
			t.Fatalf("%s key=%#x: decision got %v, want %v (trace=%+v)",
				r.app, r.keyCode, out.Decision, r.decision, out.Traces)
		}
		if r.intentID != "" && out.Intent.ID != r.intentID {
			t.Fatalf("%s key=%#x: intent got %q, want %q", r.app, r.keyCode, out.Intent.ID, r.intentID)
		}
	}
}

func TestAltF4ClosesNotQuits(t *testing.T) {
	// Bead criterion: Alt+F4 closes window without quitting app. Assert the
	// dispatched capability is window.close, which has no quit semantics —
	// window.close never terminates the process (registry-level guarantee).
	rt := testRouter()
	out := rt.Decide(
		event.Event{Type: event.EventKeyDown, Source: event.SourceKeyboard, KeyCode: vkF4, Modifiers: modAlt},
		event.FastContext{AppID: "Finder", AppMode: event.AppModeNative})
	if out.Request == nil || out.Request.Capability.ID != "window.close" {
		t.Fatalf("want window.close dispatch, got %+v", out.Request)
	}
	// Registry has no quit capability at all (pinned regression, mirrors
	// the router's own TestNoAppQuitCapability).
	reg := intent.DefaultRegistry()
	for _, id := range reg.IDs() {
		if id == "app.quit" {
			t.Fatal("registry must not contain app.quit")
		}
	}
}

func TestTerminalCtrlCInterrupts(t *testing.T) {
	// Bead criterion: Ctrl+C in Terminal interrupts. No CrossOS rule claims
	// it (mechanism, not special-case): Decision Pass means the original
	// physical Ctrl+C reaches the terminal unmodified — native INTERRUPT.
	rt := testRouter()
	out := rt.Decide(
		event.Event{Type: event.EventKeyDown, Source: event.SourceKeyboard, KeyCode: vkC, Modifiers: modCtrl},
		event.FastContext{AppID: "Terminal", AppMode: event.AppModeTerminal})
	if out.Decision != pluginapi.DecisionPass {
		t.Fatalf("want Pass (native interrupt), got %v", out.Decision)
	}
}

func TestNoKarabinerConfigExecuted(t *testing.T) {
	// Bead criterion: rules are CrossOS Rules→Intents, no Karabiner config
	// executed. Structural proof: every rule's Intent.ID is a canonical
	// registry capability, never a raw macOS key combo string.
	reg := intent.DefaultRegistry()
	for _, r := range Matrix() {
		if _, ok := reg.Get(r.Intent.ID); !ok {
			t.Fatalf("rule %s targets non-canonical intent %q (looks like a raw remap, not a CrossOS Intent)",
				r.RuleID, r.Intent.ID)
		}
	}
}

func TestNoSyntheticInjectionDiscipline(t *testing.T) {
	// §3.6 DecisionReplace discipline: COPY/clipboard intents go straight
	// to the native capability, never replayed keystrokes. Every matrix
	// rule's target capability has NO "injects-input" side effect —
	// dispatch is direct capability invocation, not CGEvent/SendInput replay.
	reg := intent.DefaultRegistry()
	for _, r := range Matrix() {
		desc, ok := reg.Get(r.Intent.ID)
		if !ok {
			t.Fatalf("rule %s: unknown capability %q", r.RuleID, r.Intent.ID)
		}
		for _, se := range desc.SideEffects {
			if se == intent.SideEffectInjectsInput {
				t.Fatalf("rule %s targets %q which injects input — matrix rules must dispatch native capabilities, never replay keystrokes",
					r.RuleID, r.Intent.ID)
			}
		}
	}
}

func TestFullStageLogging(t *testing.T) {
	// Bead criterion: unit + e2e with full stage logging (event → result,
	// winner + losers); mismatches replayable via Recorder.
	rt := testRouter()
	out := rt.Decide(
		event.Event{Type: event.EventKeyDown, Source: event.SourceKeyboard, KeyCode: vkF4, Modifiers: modAlt},
		event.FastContext{AppID: "Finder", AppMode: event.AppModeNative})
	stages := map[string]bool{}
	for _, s := range out.Traces {
		stages[s.Stage] = true
	}
	for _, want := range []string{"event", "context", "rule", "intent", "action", "result"} {
		if !stages[want] {
			t.Fatalf("missing stage %q: %+v", want, out.Traces)
		}
	}
	if out.WinnerRule == "" {
		t.Fatal("winner not recorded")
	}
}

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
