package event

import (
	"encoding/json"
	"testing"

	"crossos/core/pkg/intent"
	"crossos/core/pkg/pluginapi"
	"crossos/core/pkg/rule"
)

// testRouter builds a router over the canonical registry with a small rule
// set: global copy (Emit), terminal interrupt (Emit), and a sink rule that
// absorbs without dispatch (Emit=false → Consume).
func testRouter(killed func() bool) *Router {
	reg := intent.DefaultRegistry()
	mk := func(id string, params string) intent.Intent {
		return intent.Intent{ID: id, Version: 1, Source: intent.SourceKeyboard,
			Parameters: json.RawMessage(params)}
	}
	rules := []CompiledRule{
		{
			KeyCode: 0x43, Modifiers: 0x02, // Ctrl+C
			RuleID: "global-copy", PluginID: "windows-keyboard",
			Priority: rule.PriorityGlobal, Specificity: 0, Scope: rule.ScopeGlobal,
			Intent: mk("clipboard.copyPath", ``), Emit: true,
		},
		{
			KeyCode: 0x43, Modifiers: 0x02,
			AppModes: []AppMode{AppModeTerminal},
			RuleID:   "terminal-interrupt", PluginID: "terminal-ux",
			Priority: rule.PriorityTerminal, Specificity: 3, Scope: rule.ScopeApp,
			Intent: mk("terminal.openAt", `{"path":"/tmp"}`), Emit: true,
		},
		{
			KeyCode: 0x1B, Modifiers: 0x00, // Esc → absorb
			RuleID: "esc-sink", PluginID: "windows-keyboard",
			Priority: rule.PriorityGlobal, Specificity: 0, Scope: rule.ScopeGlobal,
			Intent: mk("window.close", ``), Emit: false,
		},
	}
	grants := map[string][]intent.Permission{
		"windows-keyboard": {intent.PermInputIntercept, intent.PermAccessControl},
		"terminal-ux":      {intent.PermInputIntercept, intent.PermShellExecution},
	}
	return Compile(rules, reg, grants, killed)
}

func TestPassWhenNoRuleMatches(t *testing.T) {
	rt := testRouter(nil)
	out := rt.Decide(Event{Type: EventKeyDown, Source: SourceKeyboard,
		KeyCode: 0x41, Modifiers: 0x02}, FastContext{AppMode: AppModeNative})
	if out.Decision != pluginapi.DecisionPass {
		t.Fatalf("want Pass, got %v", out.Decision)
	}
}

func TestReplaceDispatchesIntent(t *testing.T) {
	rt := testRouter(nil)
	out := rt.Decide(Event{Type: EventKeyDown, Source: SourceKeyboard,
		KeyCode: 0x43, Modifiers: 0x02}, FastContext{AppMode: AppModeNative})
	if out.Decision != pluginapi.DecisionReplace {
		t.Fatalf("want Replace, got %v", out.Decision)
	}
	if out.Request == nil || out.Request.Capability.ID != "clipboard.copyPath" {
		t.Fatalf("bad request binding: %+v", out.Request)
	}
}

func TestTerminalInterruptNotCopy(t *testing.T) {
	// Same physical Ctrl+C in a terminal resolves to the terminal intent —
	// the intent-first matrix, not raw shortcut translation.
	rt := testRouter(nil)
	out := rt.Decide(Event{Type: EventKeyDown, Source: SourceKeyboard,
		KeyCode: 0x43, Modifiers: 0x02},
		FastContext{AppMode: AppModeTerminal, AppID: "ghostty"})
	if out.Decision != pluginapi.DecisionReplace {
		t.Fatalf("want Replace, got %v", out.Decision)
	}
	if out.Intent.ID != "terminal.openAt" {
		t.Fatalf("want terminal.openAt, got %q", out.Intent.ID)
	}
}

func TestConsumeOnEmitFalse(t *testing.T) {
	rt := testRouter(nil)
	out := rt.Decide(Event{Type: EventKeyDown, Source: SourceKeyboard,
		KeyCode: 0x1B}, FastContext{AppMode: AppModeNative})
	if out.Decision != pluginapi.DecisionConsume {
		t.Fatalf("want Consume, got %v", out.Decision)
	}
	if out.Request != nil {
		t.Fatal("consume must not bind a request")
	}
}

func TestNoAppQuitCapability(t *testing.T) {
	// Alt+F4 → window.close (close window), never an app-quit capability.
	// Asserts registry absence only — Alt+F4 routing is covered by the
	// keyboard-plugin bead, not here. (review: cross-os-c0)
	rt := testRouter(nil)
	for _, id := range rt.registry.IDs() {
		if id == "app.quit" {
			t.Fatal("registry must not contain app.quit")
		}
	}
}

func TestKillSwitchPassesThrough(t *testing.T) {
	// 2ha binding: kill = interception disabled = OS handles keys normally
	// = PASS. Consuming would brick the keyboard (swallow all, dispatch
	// nothing). Passthrough IS the safe state. (review correction: cross-os-c0)
	rt := testRouter(func() bool { return true })
	out := rt.Decide(Event{Type: EventKeyDown, Source: SourceKeyboard,
		KeyCode: 0x43, Modifiers: 0x02}, FastContext{AppMode: AppModeNative})
	if out.Decision != pluginapi.DecisionPass {
		t.Fatalf("want Pass under kill, got %v", out.Decision)
	}
	if out.Request != nil {
		t.Fatal("kill switch must not dispatch")
	}
}

func TestSyntheticPassesThrough(t *testing.T) {
	// Self-loop guard: CrossOS-injected events never re-enter matching,
	// even if a rule would claim the keycode.
	rt := testRouter(nil)
	out := rt.Decide(Event{Type: EventKeyDown, Source: SourceSynthetic,
		KeyCode: 0x43, Modifiers: 0x02}, FastContext{AppMode: AppModeNative})
	if out.Decision != pluginapi.DecisionPass {
		t.Fatalf("want Pass for synthetic, got %v", out.Decision)
	}
}

func TestAuthorizeFailClosed(t *testing.T) {
	// 9tb binding: winner without a grant consumes instead of replacing.
	rt := testRouter(nil)
	rt.grants["windows-keyboard"] = []intent.Permission{intent.PermFilesystem}
	out := rt.Decide(Event{Type: EventKeyDown, Source: SourceKeyboard,
		KeyCode: 0x43, Modifiers: 0x02}, FastContext{AppMode: AppModeNative})
	if out.Decision != pluginapi.DecisionConsume {
		t.Fatalf("want fail-closed Consume, got %v", out.Decision)
	}
}

func TestStageTraceComplete(t *testing.T) {
	// ae7 binding: every outcome carries event→context→rule→intent→action→result.
	rt := testRouter(nil)
	out := rt.Decide(Event{Type: EventKeyDown, Source: SourceKeyboard,
		KeyCode: 0x43, Modifiers: 0x02}, FastContext{AppMode: AppModeNative})
	stages := map[string]bool{}
	for _, s := range out.Traces {
		stages[s.Stage] = true
	}
	for _, want := range []string{"event", "context", "rule", "intent", "action", "result"} {
		if !stages[want] {
			t.Fatalf("trace missing stage %q: %+v", want, out.Traces)
		}
	}
	if out.WinnerRule == "" {
		t.Fatal("winner not recorded")
	}
}

type recordObs struct{ got []Outcome }

func (o *recordObs) OnOutcome(out Outcome) { o.got = append(o.got, out) }
func (o *recordObs) Name() string          { return "record" }

func TestBusPublishDrain(t *testing.T) {
	b := NewBus(2)
	o := &recordObs{}
	b.Subscribe(o)
	rt := testRouter(nil)
	out := rt.Decide(Event{Type: EventKeyDown, Source: SourceKeyboard,
		KeyCode: 0x43, Modifiers: 0x02}, FastContext{})
	b.Publish(out)
	if b.Pending() != 1 {
		t.Fatalf("want 1 pending, got %d", b.Pending())
	}
	b.Drain()
	if len(o.got) != 1 || o.got[0].Decision != pluginapi.DecisionReplace {
		t.Fatalf("drain failed: %+v", o.got)
	}
}

func TestBusDropCounted(t *testing.T) {
	// "No-drop" = accounting, not delivery: overflow counts, never blocks.
	b := NewBus(1)
	o := &recordObs{}
	b.Subscribe(o)
	rt := testRouter(nil)
	out := rt.Decide(Event{Type: EventKeyDown, Source: SourceKeyboard,
		KeyCode: 0x43, Modifiers: 0x02}, FastContext{})
	b.Publish(out)
	b.Publish(out) // overflow
	if b.Dropped != 1 {
		t.Fatalf("want 1 counted drop, got %d", b.Dropped)
	}
	b.Drain()
	if len(o.got) != 1 {
		t.Fatalf("want 1 delivered, got %d", len(o.got))
	}
}
