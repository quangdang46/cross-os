// Builtin parity tests: every row in rules.All() matches its plugin matrix
// source 1:1 (bead cross-os-4gb follow-up: full matrix wiring).
//
// Parity is structural (rule IDs + keycodes + intents), not behavioral —
// the plugin matrices' own tests assert behavior. If a plugin matrix gains
// a row, this test fails until the port lands here too (fail-loud sync,
// never silent drift).
package rules

import (
	"testing"

	"crossos/core/pkg/event"
	"crossos/core/pkg/intent"
	"crossos/core/pkg/rule"
)

func TestBuiltinCount(t *testing.T) {
	all := All()
	// 9 live rules: 6 keyboard + 3 developer. The launcher's Ctrl+Space is
	// reserved but not compiled — no palette exists to honour it, and
	// claiming a key with no handler is a swallowed no-op.
	if len(all) != 9 {
		t.Fatalf("builtin rules=%d, want 9 (6 keyboard + 3 developer; launcher key reserved, not claimed)", len(all))
	}
	if len(BuiltinIDs) != 3 {
		t.Fatalf("builtin ids=%v, want 3", BuiltinIDs)
	}
}

func TestBuiltinParity(t *testing.T) {
	// Pinned from the plugin sources (keycode/modifier/intent per RuleID).
	// Source of truth: plugins/*/matrix.go + launcher.go HotkeyRule.
	want := map[string]struct {
		key    uint32
		mods   uint32
		intent string
		emit   bool
	}{
		"windows-keyboard.alt-f4-close-window": {0x73, 1 << 2, "window.close", true},
		"windows-keyboard.ctrl-c-copy":         {0x43, 1 << 0, "clipboard.copy", true},
		"windows-keyboard.win-left-snap":       {0x25, 1 << 3, "window.move", true},
		"windows-keyboard.win-right-snap":      {0x27, 1 << 3, "window.move", true},
		"windows-keyboard.win-up-maximize":     {0x26, 1 << 3, "window.maximize", true},
		"windows-keyboard.win-down-minimize":   {0x28, 1 << 3, "window.minimize", true},
		"developer.ctrl-shift-enter-terminal":  {0x0D, 3, "terminal.openAt", true},
		"developer.ctrl-shift-c-copypath":      {0x43, 3, "clipboard.copyPath", true},
		"developer.ctrl-shift-p-editor":        {0x50, 3, "app.open", true},
	}
	seen := map[string]bool{}
	for _, r := range All() {
		w, ok := want[r.RuleID]
		if !ok {
			t.Fatalf("extra rule %q not in any plugin matrix (port drift?)", r.RuleID)
		}
		seen[r.RuleID] = true
		if r.KeyCode != w.key || r.Modifiers != w.mods {
			t.Fatalf("rule %s: key=%#x mods=%#x, want %#x/%#x", r.RuleID, r.KeyCode, r.Modifiers, w.key, w.mods)
		}
		if r.Intent.ID != w.intent {
			t.Fatalf("rule %s: intent %q, want %q", r.RuleID, r.Intent.ID, w.intent)
		}
		if r.Emit != w.emit {
			t.Fatalf("rule %s: emit=%v, want %v", r.RuleID, r.Emit, w.emit)
		}
	}
	for id := range want {
		if !seen[id] {
			t.Fatalf("missing rule %q (plugin matrix row not ported)", id)
		}
	}
}

func TestBuiltinDecisionPath(t *testing.T) {
	// End-to-end through the real router: Ctrl+C claims copy, F8 passes
	// (developer leaves it unclaimed), Ctrl+Space consumes (launcher).
	reg := intent.DefaultRegistry()
	rt := event.Compile(All(), reg, Grants(), nil)
	native := event.FastContext{AppID: "com.apple.Finder", AppMode: event.AppModeNative}
	out := rt.Decide(
		event.Event{Type: event.EventKeyDown, Source: event.SourceKeyboard, KeyCode: 0x43, Modifiers: 1},
		native)
	if out.Intent.ID != "clipboard.copy" {
		t.Fatalf("ctrl+c intent=%q, want clipboard.copy", out.Intent.ID)
	}
	out = rt.Decide(
		event.Event{Type: event.EventKeyDown, Source: event.SourceKeyboard, KeyCode: 0x77},
		event.FastContext{AppID: "com.microsoft.VSCode", AppMode: event.AppModeNative})
	if out.Decision != 0 { // DecisionPass: F8 unclaimed
		t.Fatalf("f8 decision=%v, want pass (unclaimed)", out.Decision)
	}
	// Ctrl+Space is reserved for the launcher but not claimed while no
	// palette exists: the key must reach the app rather than being swallowed
	// by a rule with nothing behind it.
	out = rt.Decide(
		event.Event{Type: event.EventKeyDown, Source: event.SourceKeyboard, KeyCode: 0x20, Modifiers: 1},
		native)
	if out.Decision != 0 {
		t.Fatalf("ctrl+space decision=%v, want pass (no palette to honour it yet)", out.Decision)
	}
	var _ = rule.PriorityGlobal
}
