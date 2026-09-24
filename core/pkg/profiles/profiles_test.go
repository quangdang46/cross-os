// Bundle integrity tests: every id the Windows 11 Experience advertises must
// resolve in the builtin tables it names, and every capability it does NOT
// deliver must say why.
//
// The whole point of the profile layer is that a renamed rule or a dropped
// plugin can never leave the bundle pointing at nothing: the page renders
// this data generically, so a dangling id is a row the user can click with
// nothing behind it — the fabricated control pagedata.go:11 refuses to
// serve. These tests are the fail-loud half of that contract: the data is
// declared once, and a rename in core/rules or core/pkg/winlayout breaks
// THIS build rather than shipping a broken bundle.
package profiles

import (
	"strings"
	"testing"

	"crossos/core/pkg/winlayout"
	builtin "crossos/core/rules"
)

// ruleIDs is the set of behavior-matrix rule IDs rules.All() compiles.
func ruleIDs() map[string]bool {
	out := map[string]bool{}
	for _, r := range builtin.All() {
		out[r.RuleID] = true
	}
	return out
}

// zoneIDs is the set of window-action zone names winlayout resolves, keyed by
// both the parameter spelling (ZoneName: "leftHalf") and the display name
// (ActionName: "Left Half") so a capability may reference either.
func zoneIDs() map[string]bool {
	out := map[string]bool{}
	for _, a := range winlayout.AllActions {
		out[winlayout.ZoneName(a)] = true
		out[winlayout.ActionName(a)] = true
	}
	return out
}

func builtinPlugins() map[string]bool {
	out := map[string]bool{}
	for _, id := range builtin.BuiltinIDs {
		out[id] = true
	}
	return out
}

// TestBundleIDsResolve is the renamed-rule guard: every plugin, rule, and
// zone id a capability names must exist in the corresponding builtin table.
// A capability with an empty id, an unknown plugin, or a rule/zone id that no
// longer compiles fails here.
func TestBundleIDsResolve(t *testing.T) {
	plugins := builtinPlugins()
	rules := ruleIDs()
	zones := zoneIDs()
	for _, p := range All() {
		if p.ID == "" || p.Label == "" {
			t.Fatalf("profile %q: id and label are required", p.ID)
		}
		for _, c := range p.Capabilities {
			if c.ID == "" || c.Label == "" {
				t.Fatalf("profile %q: capability id and label are required", p.ID)
			}
			if !c.Available {
				// An unavailable capability names no plugin and no rules by
				// construction; it must still carry the reason.
				if c.Reason == "" {
					t.Fatalf("profile %q capability %q: unavailable with no reason", p.ID, c.ID)
				}
				if c.Plugin != "" || len(c.RuleIDs) != 0 {
					t.Fatalf("profile %q capability %q: unavailable but names plugin=%q rules=%v (must be bare + reasoned)",
						p.ID, c.ID, c.Plugin, c.RuleIDs)
				}
				continue
			}
			// An available capability is a row the user can act on, so it must
			// resolve to a real plugin and at least one real rule — never a
			// passing checkmark over nothing.
			if !plugins[c.Plugin] {
				t.Fatalf("profile %q capability %q: plugin %q not in rules.BuiltinIDs", p.ID, c.ID, c.Plugin)
			}
			if len(c.RuleIDs) == 0 {
				t.Fatalf("profile %q capability %q: available with no rule ids", p.ID, c.ID)
			}
			for _, id := range c.RuleIDs {
				if !rules[id] && !zones[id] {
					t.Fatalf("profile %q capability %q: rule id %q resolves in neither rules.All() nor winlayout.AllActions",
						p.ID, c.ID, id)
				}
			}
		}
	}
}

// TestWindowsKeyboardRulesOwnedByPlugin is the ownership guard: every
// behavior-matrix rule a windows-keyboard capability names must actually be
// shipped by the plugin the capability attributes it to. A rule that moved
// plugins would otherwise render as "this profile does it" while the plugin
// the user is enabling is not the one that carries the rule.
func TestWindowsKeyboardRulesOwnedByPlugin(t *testing.T) {
	owner := map[string]string{}
	for _, r := range builtin.All() {
		owner[r.RuleID] = r.PluginID
	}
	for _, p := range All() {
		for _, c := range p.Capabilities {
			if !c.Available {
				continue
			}
			for _, id := range c.RuleIDs {
				if got, isRule := owner[id]; isRule && got != c.Plugin {
					t.Fatalf("capability %q names rule %q owned by %q, but attributes it to %q",
						c.ID, id, got, c.Plugin)
				}
			}
		}
	}
}

// TestAltTabIsExplicitlyUnavailable pins the specific honesty requirement: the
// spec's Windows profile advertises Alt+Tab, and CrossOS has no window
// switcher behind it, so the bundle must carry it as an UNAVAILABLE capability
// with a reason — not as a passing row, and not silently omitted.
func TestAltTabIsExplicitlyUnavailable(t *testing.T) {
	var alt *Capability
	for i, c := range windows11.Capabilities {
		if c.ID == "window.alt-tab" {
			alt = &windows11.Capabilities[i]
			break
		}
	}
	if alt == nil {
		t.Fatal("windows 11 experience: Alt+Tab capability is missing (must be declared, even as unavailable)")
	}
	if alt.Available {
		t.Fatal("Alt+Tab marked available, but no window-switcher capability/plugin/rule exists — that would be a fabricated row")
	}
	if alt.Reason == "" {
		t.Fatal("Alt+Tab unavailable with no reason string")
	}
	// A reason naming the gap is the contract; one that merely says "not
	// implemented" without the mechanism is not enough to be actionable.
	if !strings.Contains(strings.ToLower(alt.Reason), "window") {
		t.Fatalf("Alt+Tab reason does not name the missing mechanism: %q", alt.Reason)
	}
}

// TestLayoutsCoverAllActions pins the window.layouts capability to the FULL
// winlayout vocabulary: the layout picker the Windows page edits cannot
// advertise fewer actions than winlayout resolves. The capability is declared
// with zones(winlayout.AllActions...), so this guards that the two have not
// drifted if the capability is ever hand-edited.
func TestLayoutsCoverAllActions(t *testing.T) {
	var got map[string]bool
	for _, c := range windows11.Capabilities {
		if c.ID == "window.layouts" {
			got = map[string]bool{}
			for _, id := range c.RuleIDs {
				got[id] = true
			}
		}
	}
	if got == nil {
		t.Fatal("window.layouts capability is missing")
	}
	if len(got) != len(winlayout.AllActions) {
		t.Fatalf("window.layouts has %d zone ids, want %d (all actions)", len(got), len(winlayout.AllActions))
	}
	for _, a := range winlayout.AllActions {
		if !got[winlayout.ZoneName(a)] {
			t.Fatalf("window.layouts missing action %q", winlayout.ZoneName(a))
		}
	}
}

// TestBuiltinWindows11Experience pins the single builtin profile's identity
// and the two keyboard capabilities the Windows 11 Experience is named for.
func TestBuiltinWindows11Experience(t *testing.T) {
	all := All()
	if len(all) != 1 {
		t.Fatalf("builtin profiles=%d, want 1 (Windows 11 Experience)", len(all))
	}
	if all[0].ID != "windows-11-experience" {
		t.Fatalf("builtin profile id=%q, want windows-11-experience", all[0].ID)
	}
	kb := windowsKeyboardPlugin()
	if !builtinPlugins()[kb] {
		t.Fatalf("windowsKeyboard index %d resolves to %q, not a builtin plugin", windowsKeyboard, kb)
	}
	// The two capabilities the product leads with: Windows shortcuts and
	// Win+Arrow snap, both delivered by the windows-keyboard plugin.
	for _, want := range []string{"keyboard.shortcuts", "window.snap"} {
		found := false
		for _, c := range windows11.Capabilities {
			if c.ID == want && c.Available && c.Plugin == kb {
				found = true
			}
		}
		if !found {
			t.Fatalf("Windows 11 Experience missing available capability %q backed by %q", want, kb)
		}
	}
}
