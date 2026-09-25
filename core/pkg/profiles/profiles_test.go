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

	"crossos/core/pkg/intent"
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

// --- the two unavailable rows' reasons ------------------------------------
//
// A Reason is the one string on a settings page that a user cannot check and
// we cannot regenerate, so it rots silently: the table it described gained an
// entry, the row kept denying it, and the page told someone their Alt+Tab did
// not exist. Both rows below had gone stale that way. These tests are the
// anti-staleness half — they read the tables the Reason talks about, so the
// next registry or rule addition breaks THIS build rather than shipping a
// second lie.

// registryIDs is the set of capability ids intent.DefaultRegistry() serves.
func registryIDs() map[string]bool {
	out := map[string]bool{}
	for _, id := range intent.DefaultRegistry().IDs() {
		out[id] = true
	}
	return out
}

// TestUnavailableReasonDeniesNothingTheRegistryHas is the check the stale
// strings failed: a Reason that NAMES a registered capability must not be
// denying it.
//
// Normalisation is the part that makes this a real test rather than a
// tautology. The old window.alt-tab string spelled the capability
// "window-switcher" — a hyphen where the registry has a dot — so a literal
// lookup would never have matched and the string would have shipped forever.
// Folding hyphens to dots before comparing is what makes the next one fail.
func TestUnavailableReasonDeniesNothingTheRegistryHas(t *testing.T) {
	known := registryIDs()
	// A denial here is a claim that a capability DOES NOT EXIST. The list is
	// existence-only on purpose, and the distinction is not pedantry: a reason
	// is allowed to say "file.moveToTrash is the one verb that fires, and this
	// bundle does not claim it", which is TRUE, names a registered capability,
	// and would be flagged by any blunter list. What it may never say is that
	// a registered capability is absent. "no " and "exists yet" are the two
	// phrasings the stale strings actually used, and both are here.
	denials := []string{"no ", "exists yet", "without ", "missing", "absent", "never registered", "does not exist", "not implemented"}
	for _, p := range All() {
		for _, c := range p.Capabilities {
			if c.Available || c.Reason == "" {
				continue
			}
			for id := range known {
				for _, spelling := range []string{id, strings.ReplaceAll(id, ".", "-")} {
					at := strings.Index(c.Reason, spelling)
					if at < 0 {
						continue
					}
					// The clause is the SENTENCE the mention sits in, not a
					// character window: a reason routinely denies a chord in
					// one sentence and names the intent it would need in the
					// next, and a ±90-character window makes those two
					// sentences accuse each other.
					lowered := strings.ToLower(sentenceAround(c.Reason, at, len(spelling)))
					for _, d := range denials {
						if strings.Contains(lowered, d) {
							t.Errorf("capability %q reason denies %q, which IS in the intent registry:\n  %s",
								c.ID, spelling, c.Reason)
							break
						}
					}
				}
			}
		}
	}
}

// TestUnavailableReasonNamesOnlyRegisteredCapabilities is the other half: the
// Reason may not invent a capability either. An id the registry does not serve
// is a name from someone's memory, and the next reader has no way to tell it
// apart from one that resolved — which is exactly how the two stale strings
// came to be trusted.
func TestUnavailableReasonNamesOnlyRegisteredCapabilities(t *testing.T) {
	known := registryIDs()
	for _, p := range All() {
		for _, c := range p.Capabilities {
			if c.Available || c.Reason == "" {
				continue
			}
			// Every dotted token the Reason mentions must resolve — as a
			// registered capability, as a builtin rule id, or as the name of
			// the thing the Reason says is MISSING. That third case is
			// legitimate and necessary: a reason that blames an absent
			// capability has to be able to name it, and the finder row's
			// does. It is exempt only when a denial word introduces it in
			// the same sentence, so "no filesystem.rename is registered"
			// passes and a bare invented name does not.
			for _, tok := range dottedTokens(c.Reason) {
				if known[tok] || ruleIDs()[tok] {
					continue
				}
				if namesTheMissingThing(c.Reason, tok) {
					continue
				}
				t.Errorf("capability %q reason names %q, which is neither a registered "+
					"capability, a rule id, nor a capability the reason denies:\n  %s",
					c.ID, tok, c.Reason)
			}
		}
	}
}

// TestAltTabRowIsTrueOfTheTables holds the window.alt-tab reason to the facts
// it asserts, and it is the test that tells the next person what to do.
//
// The reason says: the switcher ships, the capability is registered, two rules
// drive it, and THIS BUNDLE CLAIMS NEITHER. Every clause is checked against the
// tables, so when somebody adds those two RuleIDs to a capability this test
// fails and says the row should now be Available — which is the one edit that
// turns the row on, and the one the stale string used to imply had already
// happened in reverse.
func TestAltTabRowIsTrueOfTheTables(t *testing.T) {
	const capID = "window.alt-tab"
	cap := capabilityByID(t, capID)
	if cap.Available {
		t.Fatalf("capability %q is Available; if the bundle now claims the switcher rules, "+
			"this test should assert the positive case instead", capID)
	}
	// Clause one: the capability is registered. The old reason denied it.
	if _, ok := intent.DefaultRegistry().Get("window.switcher"); !ok {
		t.Fatalf("window.switcher is not in the intent registry, so the %q reason needs rewriting", capID)
	}
	// Clause two: two rules drive it, both under the windows-keyboard plugin.
	var switcherRules []string
	for _, r := range builtin.All() {
		if r.Intent.ID == "window.switcher" {
			switcherRules = append(switcherRules, r.RuleID)
		}
	}
	if len(switcherRules) == 0 {
		t.Fatalf("no builtin rule emits window.switcher, so the %q reason needs rewriting", capID)
	}
	for _, id := range switcherRules {
		if !strings.Contains(cap.Reason, id) {
			t.Errorf("capability %q reason does not name the rule %q that drives it:\n  %s",
				capID, id, cap.Reason)
		}
		// Clause three: the bundle claims neither. This is the assertion the
		// whole row turns on.
		if bundleClaimsRule(t, id) {
			t.Errorf("capability %q claims rule %q but is still unavailable — the row should "+
				"be Available now that the bundle carries it", capID, id)
		}
	}
}

// TestFinderExplorerRowIsTrueOfTheTables holds the finder.explorer reason to
// the tables, the way the Alt+Tab one is. Its old string was wrong in one
// clause: F2 and Win+E ARE declared chords in winlayout.Windows11Shortcuts(),
// so "no rule or shortcut behind them" denied a row that exists. What is absent
// is the rule that FIRES them, and the reason says exactly that.
func TestFinderExplorerRowIsTrueOfTheTables(t *testing.T) {
	const capID = "finder.explorer"
	cap := capabilityByID(t, capID)
	if cap.Available {
		t.Fatalf("capability %q is Available; if the bundle now claims the Explorer verbs, "+
			"this test should assert the positive case instead", capID)
	}
	preset := winlayout.Windows11Shortcuts()
	f2, winE := presetRow(preset, "F2"), presetRow(preset, "E")
	// Clause one: both chords ARE declared. The old reason denied this.
	if f2 == nil {
		t.Fatalf("F2 is not a Windows11Shortcuts row, so the %q reason needs rewriting", capID)
	}
	if winE == nil {
		t.Fatalf("Win+E is not a Windows11Shortcuts row, so the %q reason needs rewriting", capID)
	}
	// Clause two: F2's intent is empty, and the registry has no
	// filesystem.rename that could fill it. Both halves, because a reason that
	// blames an empty intent which a later registry entry fills is stale again.
	if f2.Capability != "" {
		t.Errorf("F2's preset row now carries intent %q; the %q reason must say what "+
			"that intent still lacks", f2.Capability, capID)
	}
	if _, ok := intent.DefaultRegistry().Get("filesystem.rename"); ok {
		t.Errorf("filesystem.rename is now registered, so F2 rename is one rule away and "+
			"the %q reason is stale:\n  %s", capID, cap.Reason)
	}
	// Clause three: Win+E's intent is app.open, and only the developer plugin
	// fires app.open.
	if winE.Capability != "app.open" {
		t.Errorf("Win+E's preset row carries intent %q, not app.open; the %q reason "+
			"names app.open and must be rewritten", winE.Capability, capID)
	}
	for _, r := range builtin.All() {
		if r.Intent.ID != "app.open" {
			continue
		}
		if r.KeyCode == winEKey {
			t.Errorf("rule %q now fires the Win+E chord, so the %q reason is stale:\n  %s",
				r.RuleID, capID, cap.Reason)
		}
	}
	// Clause four: NO rule fires F2 either. Checked by keycode because the
	// reason claims the absence of a chord, not of an intent.
	for _, r := range builtin.All() {
		if r.KeyCode == f2Key {
			t.Errorf("rule %q now fires the F2 key, so the %q reason is stale:\n  %s",
				r.RuleID, capID, cap.Reason)
		}
	}
	// Clause five: file.moveToTrash really is registered and really is bound to
	// Shift+Delete, so the sentence that calls it the one Explorer verb that
	// fires is checkable rather than remembered.
	if _, ok := intent.DefaultRegistry().Get("file.moveToTrash"); !ok {
		t.Errorf("file.moveToTrash is not registered, so the %q reason overstates it:\n  %s",
			capID, cap.Reason)
	}
	trash := presetRow(preset, "Delete")
	if trash == nil || trash.Capability != "file.moveToTrash" {
		t.Errorf("Shift+Delete is not bound to file.moveToTrash, so the %q reason "+
			"names a binding that moved:\n  %s", capID, cap.Reason)
	}
	if bundleClaimsRule(t, "windows-keyboard.shift-delete-move-trash") {
		t.Logf("note: the bundle now claims a trash rule; re-read the %q reason", capID)
	}
}

// The two keycodes the Finder row's reason talks about, as the platform spells
// them. Declared here rather than borrowed from the rules package because those
// constants are deliberately unexported: a test that reached for them would be
// asserting against the same literal it is trying to check.
const (
	f2Key   = 0x71 // F2
	winEKey = 0x45 // E, under the Win modifier
)

// builtinPrefix is the plugin-id namespace the builtin rules live in. A dotted
// token carrying it is a rule id from a rule table, not a capability name.
const builtinPrefix = "windows-keyboard."

// namesTheMissingThing reports whether a dotted token in a Reason is the name
// of the capability the Reason says is absent, rather than an invention.
//
// The test is the denial prefix: a sentence that says the id is unregistered,
// missing, absent, or never registered is naming what is MISSING, which is the
// only honest way for a reason to mention an id the registry does not serve.
// A sentence that merely contains the name is not enough, because a reason
// could equally be inventing one.
func namesTheMissingThing(reason, tok string) bool {
	// Existence-only, for the same reason the main check's list is: naming the
	// absent thing is the point, and a denial word that merely says the BUNDLE
	// does not claim it says nothing about whether the capability exists.
	denials := []string{"no ", "without ", "missing", "absent", "never registered", "does not exist", "not implemented"}
	at := strings.Index(reason, tok)
	if at < 0 {
		return false
	}
	clause := strings.ToLower(sentenceAround(reason, at, len(tok)))
	for _, d := range denials {
		if strings.Contains(clause, d) {
			return true
		}
	}
	return false
}

// sentenceAround returns the sentence containing the mention at the character
// offset at, which is length bytes long.
//
// The length is load-bearing and the first version of this helper was wrong
// because of it: every capability id in the registry contains a dot
// ("file.moveToTrash"), so a forward scan for the sentence terminator that
// started AT the mention stopped on the id's own dot and returned a clause
// ending mid-word. The old window.alt-tab string slipped past the check for
// exactly that reason. The forward scan therefore starts after the mention, and
// the backward scan may not: a sentence that ends in a dot before the mention
// is a sentence that ended.
func sentenceAround(s string, at, length int) string {
	start := 0
	for i := at; i > 0; i-- {
		if isSentenceEnd(s[i-1]) {
			start = i
			break
		}
	}
	end := len(s)
	for i := at + length; i < len(s); i++ {
		if isSentenceEnd(s[i]) {
			end = i
			break
		}
	}
	return s[start:end]
}

// isSentenceEnd reports whether a byte ends a clause. A colon and a semicolon
// count because a reason uses them to introduce a list of what is missing, and
// a newline because a wrapped reason is still two claims.
func isSentenceEnd(b byte) bool {
	return b == '.' || b == ':' || b == ';' || b == '\n'
}

// capabilityByID resolves one capability of a named profile, failing the test
// if the bundle does not declare it.
func capabilityByID(t *testing.T, id string) Capability {
	t.Helper()
	for _, p := range All() {
		for _, c := range p.Capabilities {
			if c.ID == id {
				return c
			}
		}
	}
	t.Fatalf("no profile declares capability %q", id)
	return Capability{}
}

// bundleClaimsRule reports whether ANY capability in the bundle names a rule id.
func bundleClaimsRule(t *testing.T, ruleID string) bool {
	t.Helper()
	for _, p := range All() {
		for _, c := range p.Capabilities {
			for _, id := range c.RuleIDs {
				if id == ruleID {
					return true
				}
			}
		}
	}
	return false
}

// presetRow finds one Windows11Shortcuts row by its key, ignoring modifiers —
// the reason talks about the chord, and "Shift+Delete" and "Delete" are the
// same row.
func presetRow(rows []winlayout.ProfileShortcut, key string) *winlayout.ProfileShortcut {
	for i := range rows {
		if rows[i].Key == key {
			return &rows[i]
		}
	}
	return nil
}

// dottedTokens pulls the dotted identifiers out of a Reason, so each can be
// resolved against the tables rather than trusted.
func dottedTokens(s string) []string {
	var out []string
	cur := ""
	flush := func() {
		// Trimmed first and checked for an INTERNAL dot after: a token
		// picked up from "the developer plugin. file.moveToTrash" ends with
		// the sentence's full stop, and "plugin." is not a name anybody
		// registered.
		tok := strings.Trim(cur, ".-_")
		if strings.Contains(tok, ".") {
			out = append(out, tok)
		}
		cur = ""
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-', r == '_':
			cur += string(r)
		default:
			flush()
		}
	}
	flush()
	return out
}
