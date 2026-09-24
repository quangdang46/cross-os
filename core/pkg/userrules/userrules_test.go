// User-rule table tests (bead w2-userrules).
//
// The contract under test is narrow and load-bearing: a rule maps 1:1 onto
// event.CompiledRule, its rank is derived from the dimensions the user picked
// rather than stored, a rule that does not validate persists nothing at all,
// and two overlapping rules resolve to one winner through the same
// rule.Resolve the router uses. Everything else here is fixture.

package userrules

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"crossos/core/pkg/event"
	"crossos/core/pkg/intent"
	"crossos/core/pkg/pluginapi"
	"crossos/core/pkg/rule"
	"crossos/core/pkg/winlayout"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := New("")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

// copyRule is the "Ctrl+C in ordinary apps" row the spec's example gives:
// IF App = Finder AND Ctrl+C THEN Copy. Emit true because the row names a
// capability to dispatch, which is what makes the router Replace rather than
// swallow the key.
func copyRule() Rule {
	return Rule{
		Key:        "C",
		Modifiers:  []string{"Ctrl"},
		AppModes:   []string{"native"},
		Capability: "clipboard.copy",
		Emit:       true,
	}
}

func TestCreateUpdateDelete(t *testing.T) {
	s := newStore(t)
	got, err := s.Create(copyRule())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got.ID != "user.ctrl+c@native" {
		t.Fatalf("id = %q, want the derived user.ctrl+c@native", got.ID)
	}
	// The stored row is normalized: lists sorted, empties empty rather than
	// nil, so the document reads the same however the rule was picked.
	if got.AppIDs == nil || got.Modifiers == nil {
		t.Fatalf("normalized row still has nil lists: %+v", got)
	}
	if rules := s.Rules(); len(rules) != 1 || rules[0].ID != got.ID {
		t.Fatalf("Rules() = %+v, want the one created rule", rules)
	}

	// The row reaches the router 1:1: the picked chord became a keycode and
	// a mask, the mode list became event.AppModes, and the capability became
	// a keyboard-source intent.
	table := s.Table()
	if len(table) != 1 {
		t.Fatalf("Table() = %d rules, want 1", len(table))
	}
	cr := table[0]
	if cr.KeyCode != 0x43 || cr.Modifiers != 0x01 {
		t.Fatalf("chord = %#x/%#x, want 0x43/0x01", cr.KeyCode, cr.Modifiers)
	}
	if len(cr.AppModes) != 1 || cr.AppModes[0] != event.AppModeNative {
		t.Fatalf("AppModes = %v, want [native]", cr.AppModes)
	}
	if cr.Intent.ID != "clipboard.copy" || cr.Intent.Source != intent.SourceKeyboard {
		t.Fatalf("intent = %+v, want a keyboard-source clipboard.copy", cr.Intent)
	}
	if cr.PluginID != PluginID || !cr.Emit {
		t.Fatalf("plugin/emit = %q/%v, want %q/true", cr.PluginID, cr.Emit, PluginID)
	}

	// Update rewrites the rule under its own ID. Re-picking the chord
	// re-derives the ID, so the row comes back renamed.
	edited := copyRule()
	edited.Capability = "clipboard.copyPath"
	updated, err := s.Update(got.ID, edited)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.ID != got.ID {
		t.Fatalf("update changed the id to %q, want %q", updated.ID, got.ID)
	}
	if rules := s.Rules(); len(rules) != 1 || rules[0].Capability != "clipboard.copyPath" {
		t.Fatalf("Rules() = %+v, want the edited rule", rules)
	}
	// The old capability is gone, and so is the grant it needed.
	if table := s.Table(); table[0].Intent.ID != "clipboard.copyPath" {
		t.Fatalf("compiled intent = %q, want clipboard.copyPath", table[0].Intent.ID)
	}

	if err := s.Delete(got.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if rules := s.Rules(); len(rules) != 0 {
		t.Fatalf("Rules() = %+v, want empty after delete", rules)
	}
	// A second delete names a row that is gone: an error, not a silent
	// success the editor would report for a row it can no longer see.
	if err := s.Delete(got.ID); err == nil {
		t.Fatal("deleting an absent rule must fail")
	}
	// So does an update.
	if _, err := s.Update(got.ID, copyRule()); err == nil {
		t.Fatal("updating an absent rule must fail")
	}
}

func TestPersistsAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "user-rules.json")
	s, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	first, err := s.Create(copyRule())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	half := copyRule()
	half.Key = "V"
	half.AppModes = nil
	if _, err := s.Create(half); err != nil {
		t.Fatalf("Create second: %v", err)
	}

	s2, err := New(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	got := s2.Rules()
	if len(got) != 2 {
		t.Fatalf("reopened with %d rules, want 2 (%+v)", len(got), got)
	}
	// Sorted by ID, and the edit survived whole: chord, mode list and
	// capability together, not half of one row and half of another.
	if got[0].ID != "user.ctrl+c@native" || got[1].ID != "user.ctrl+v@any" {
		t.Fatalf("reopened ids = %q, %q; want ctrl+c then ctrl+v", got[0].ID, got[1].ID)
	}
	if first.ID != got[0].ID || got[0].Capability != "clipboard.copy" {
		t.Fatalf("reopened row = %+v, want the created rule intact", got[0])
	}
	if n := len(s2.Table()); n != 2 {
		t.Fatalf("reopened Table() = %d rules, want 2", n)
	}
}

// TestRejectsUnknownAppMode: a mode the taxonomy does not have is refused
// with its name and the list that would have worked. It is refused HERE
// rather than stored, because a stored mode compiles to a matcher that never
// matches — a key the user believes they remapped and have not.
func TestRejectsUnknownAppMode(t *testing.T) {
	s := newStore(t)
	bad := copyRule()
	bad.AppModes = []string{"nautilus"}
	_, err := s.Create(bad)
	if err == nil {
		t.Fatal("unknown app mode must be rejected")
	}
	for _, want := range []string{"nautilus", "terminal", "native"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not name %q", err, want)
		}
	}
	if rules := s.Rules(); len(rules) != 0 {
		t.Fatalf("a rejected rule was stored: %+v", rules)
	}
}

// TestRejectsUnknownCapability: the registry's own reason reaches the editor
// unchanged, next to the rule that named it.
func TestRejectsUnknownCapability(t *testing.T) {
	s := newStore(t)
	bad := copyRule()
	bad.Capability = "window.explode"
	_, err := s.Create(bad)
	if err == nil {
		t.Fatal("unknown capability must be rejected")
	}
	for _, want := range []string{"window.explode", "unknown capability", "user.ctrl+c@native"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not say %q", err, want)
		}
	}
	// A banned parallel name is refused the same way — it reads like a
	// capability but is not one.
	banned := copyRule()
	banned.Capability = "window.manage"
	if _, err := s.Create(banned); err == nil {
		t.Fatal("a banned parallel capability name must be rejected")
	}
	// Parameters are validated against the capability's own input schema:
	// a required key missing, an undeclared key added, and arguments to a
	// capability that takes none at all. That last one ValidateAgainst does
	// not catch — it returns before it reads Parameters for a schema-less
	// capability — so the table has to, or the file ends up holding
	// parameters to an action that accepts none.
	noZone := copyRule()
	noZone.Capability = "window.move"
	noZone.Parameters = json.RawMessage(`{"side":"left"}`)
	noParams := copyRule()
	noParams.Parameters = json.RawMessage(`{"zone":"leftHalf"}`)
	undeclared := copyRule()
	undeclared.Parameters = json.RawMessage(`{"side":"left"}`)
	badZone := copyRule()
	badZone.Capability = "window.move"
	badZone.Parameters = json.RawMessage(`{"zone":"middleish"}`)
	for _, r := range []Rule{noZone, noParams, undeclared, badZone} {
		if _, err := s.Create(r); err == nil {
			t.Errorf("rule %+v must be rejected", r)
		}
	}
	if rules := s.Rules(); len(rules) != 0 {
		t.Fatalf("a rejected rule was stored: %+v", rules)
	}
	// A zone the window vocabulary owns IS accepted, so the check above is
	// not simply refusing every window.move.
	good := copyRule()
	good.Capability = "window.move"
	good.Parameters = json.RawMessage(`{"zone":"leftHalf"}`)
	if _, err := s.Create(good); err != nil {
		t.Fatalf("a valid window.move was rejected: %v", err)
	}
}

// TestRejectionPersistsNothing: a rejected edit leaves the file on disk byte
// for byte as it was. The editor is told the write failed, and a file that
// had already changed anyway would put the table the user sees and the table
// the router compiles on different rules.
func TestRejectionPersistsNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "user-rules.json")
	s, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := s.Create(copyRule()); err != nil {
		t.Fatalf("Create: %v", err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	for _, bad := range []Rule{
		func() Rule { r := copyRule(); r.Key = "Kkk"; return r }(),
		func() Rule { r := copyRule(); r.Modifiers = []string{"Hyper"}; return r }(),
		func() Rule { r := copyRule(); r.AppModes = []string{"nautilus"}; return r }(),
		func() Rule { r := copyRule(); r.Capability = "window.explode"; return r }(),
	} {
		if _, err := s.Create(bad); err == nil {
			t.Fatalf("rule %+v must be rejected", bad)
		}
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("a rejected edit changed the file:\nbefore %s\nafter  %s", before, after)
	}
	if rules := s.Rules(); len(rules) != 1 {
		t.Fatalf("running table = %+v, want the one good rule", rules)
	}
	// A rejected update leaves the row it would have replaced alone.
	bad := copyRule()
	bad.Capability = "window.explode"
	if _, err := s.Update(rules0ID(t, s), bad); err == nil {
		t.Fatal("a rejected update must fail")
	}
	if r := s.Rules()[0]; r.Capability != "clipboard.copy" {
		t.Fatalf("rejected update changed the row: %+v", r)
	}
}

func rules0ID(t *testing.T, s *Store) string {
	t.Helper()
	rules := s.Rules()
	if len(rules) == 0 {
		t.Fatal("no rules to address")
	}
	return rules[0].ID
}

// TestDuplicateRuleRejected: the ID is derived, so building the same rule
// twice is a duplicate RuleID — caught by rule.ValidateCandidates over the
// whole table, which is the check that keeps two claims of one identity from
// both reaching the router.
func TestDuplicateRuleRejected(t *testing.T) {
	s := newStore(t)
	if _, err := s.Create(copyRule()); err != nil {
		t.Fatalf("Create: %v", err)
	}
	_, err := s.Create(copyRule())
	if err == nil {
		t.Fatal("the same rule twice must be rejected")
	}
	if !strings.Contains(err.Error(), "duplicate RuleID") {
		t.Fatalf("error %q does not report the duplicate", err)
	}
	if rules := s.Rules(); len(rules) != 1 {
		t.Fatalf("Rules() = %+v, want the one rule", rules)
	}
	// The pick order does not make a different rule: the lists are sorted
	// before the ID is derived, so the same five apps in another order are
	// the same claim.
	shuffled := copyRule()
	shuffled.Modifiers = []string{"Ctrl"}
	shuffled.AppModes = []string{"native"}
	if _, err := s.Create(shuffled); err == nil {
		t.Fatal("a re-ordered copy of the same rule must be rejected")
	}
}

// TestDerivesRankFromPickedDimensions: priority, specificity and scope are a
// function of the picks. Nothing in the stored row can move them, which is
// why they are not stored — the table below is the whole contract.
func TestDerivesRankFromPickedDimensions(t *testing.T) {
	for _, tc := range []struct {
		name string
		rule Rule
		want event.CompiledRule
	}{
		{
			name: "no narrowing is global",
			rule: Rule{Key: "C", Capability: "clipboard.copy"},
			want: event.CompiledRule{Priority: rule.PriorityGlobal, Specificity: 1, Scope: rule.ScopeGlobal},
		},
		{
			name: "an app mode is app-scoped",
			rule: Rule{Key: "C", AppModes: []string{"native"}, Capability: "clipboard.copy"},
			want: event.CompiledRule{Priority: rule.PriorityApp, Specificity: 2, Scope: rule.ScopeApp},
		},
		{
			name: "terminals take the 100 tier",
			rule: Rule{Key: "C", AppModes: []string{"terminal"}, Capability: "clipboard.copy"},
			want: event.CompiledRule{Priority: rule.PriorityTerminal, Specificity: 2, Scope: rule.ScopeApp},
		},
		{
			name: "native and terminal together is not a terminal rule",
			rule: Rule{Key: "F4", Modifiers: []string{"Alt"}, AppModes: []string{"terminal", "native"}, Capability: "window.close"},
			want: event.CompiledRule{Priority: rule.PriorityApp, Specificity: 2, Scope: rule.ScopeApp},
		},
		{
			name: "a named app is app-scoped",
			rule: Rule{Key: "C", AppIDs: []string{"com.apple.Finder"}, Capability: "clipboard.copy"},
			want: event.CompiledRule{Priority: rule.PriorityApp, Specificity: 2, Scope: rule.ScopeApp},
		},
		{
			name: "app and mode together are two levels deeper",
			rule: Rule{Key: "C", AppModes: []string{"native"}, AppIDs: []string{"com.apple.Finder"}, Capability: "clipboard.copy"},
			want: event.CompiledRule{Priority: rule.PriorityApp, Specificity: 3, Scope: rule.ScopeApp},
		},
		{
			name: "a device is the narrowest scope the table can claim",
			rule: Rule{Key: "C", DeviceID: "kbd-1", Capability: "clipboard.copy"},
			want: event.CompiledRule{Priority: rule.PriorityApp, Specificity: 2, Scope: rule.ScopeDevice},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := tc.rule
			r.ID = DeriveID(r)
			cr, err := r.Compile(intent.DefaultRegistry())
			if err != nil {
				t.Fatalf("Compile: %v", err)
			}
			if cr.Priority != tc.want.Priority || cr.Specificity != tc.want.Specificity || cr.Scope != tc.want.Scope {
				t.Fatalf("derived priority/specificity/scope = %d/%d/%s, want %d/%d/%s",
					cr.Priority, cr.Specificity, ScopeName(cr.Scope),
					tc.want.Priority, tc.want.Specificity, ScopeName(tc.want.Scope))
			}
			// And the derivation is stable: same picks, same rank, every
			// time — there is no stored copy to drift.
			again, err := r.Compile(intent.DefaultRegistry())
			if err != nil {
				t.Fatalf("Compile again: %v", err)
			}
			if !reflect.DeepEqual(again, cr) {
				t.Fatalf("derivation is not deterministic: %+v then %+v", cr, again)
			}
		})
	}
}

// TestOverlappingRulesResolveToOneWinner: the spec's Context Rule Builder
// examples — Ctrl+C in Finder, and Ctrl+C everywhere — plus a third claim
// scoped to one keyboard. In Finder, on that keyboard, all three genuinely
// match one keystroke, so rule.Resolve has a real choice: the deepest claim
// wins and the other two come back ranked. This is the router's own resolver,
// not a re-implementation of it.
func TestOverlappingRulesResolveToOneWinner(t *testing.T) {
	s := newStore(t)
	global := Rule{
		Key: "C", Modifiers: []string{"Ctrl"},
		Capability: "clipboard.write", Parameters: json.RawMessage(`{"text":""}`), Emit: true,
	}
	inFinder := Rule{
		Key: "C", Modifiers: []string{"Ctrl"},
		AppIDs:     []string{"com.apple.Finder"},
		Capability: "clipboard.copy", Emit: true,
	}
	onOneKeyboard := Rule{
		Key: "C", Modifiers: []string{"Ctrl"},
		AppIDs:     []string{"com.apple.Finder"},
		DeviceID:   "kbd-1",
		Capability: "clipboard.copyPath", Emit: true,
	}
	for _, r := range []Rule{global, inFinder, onOneKeyboard} {
		if _, err := s.Create(r); err != nil {
			t.Fatalf("Create %s: %v", DeriveID(r), err)
		}
	}

	// Resolve ranks by specificity, then scope, then priority (rule.go:63-75),
	// and every one of those is derived from the picks — so no number in the
	// stored row could have changed this answer.
	res := rule.Resolve(s.Candidates())
	if res.RuleID != DeriveID(onOneKeyboard) {
		t.Fatalf("winner = %q, want the device-scoped rule %q", res.RuleID, DeriveID(onOneKeyboard))
	}
	if res.Decision != pluginapi.DecisionReplace {
		t.Fatalf("decision = %v, want replace", res.Decision)
	}
	if res.Intent.ID != "clipboard.copyPath" {
		t.Fatalf("winner intent = %q, want clipboard.copyPath", res.Intent.ID)
	}
	// The losers come back ranked — the app claim above the global one — and
	// the winner is not among them.
	if len(res.Losers) != 2 {
		t.Fatalf("losers = %d, want the other two", len(res.Losers))
	}
	if res.Losers[0].RuleID != DeriveID(inFinder) || res.Losers[1].RuleID != DeriveID(global) {
		t.Fatalf("losers out of rank: %q then %q, want the app rule then the global one",
			res.Losers[0].RuleID, res.Losers[1].RuleID)
	}
	for _, l := range res.Losers {
		if l.RuleID == res.RuleID {
			t.Fatal("the winner is in its own loser list")
		}
	}
	// A second resolve of the same table gives the same answer: the tie-break
	// is the rule ID, not the order the rows were written in.
	if again := rule.Resolve(s.Candidates()); again.RuleID != res.RuleID {
		t.Fatalf("second resolve = %q, want the stable %q", again.RuleID, res.RuleID)
	}

	// The spec's other example, Ctrl+C in a terminal, is a claim that cannot
	// overlap either of the others — a terminal is not Finder. Resolved on its
	// own it wins trivially, and its 100 tier is asserted from the candidate
	// directly: rule.Resolution reports the winner, not the numbers it beat.
	terminal := Rule{Key: "C", Modifiers: []string{"Ctrl"}, AppModes: []string{"terminal"}, Capability: "clipboard.copy", Emit: true}
	terminalCand := Candidate(mustCompile(t, terminal))
	if terminalCand.Priority != rule.PriorityTerminal {
		t.Fatalf("terminal priority = %d, want %d", terminalCand.Priority, rule.PriorityTerminal)
	}
	if byMode := rule.Resolve([]rule.Candidate{terminalCand}); byMode.RuleID != DeriveID(terminal) {
		t.Fatalf("terminal winner = %q, want %q", byMode.RuleID, DeriveID(terminal))
	}
}

// mustCompile is the fixture path for a rule the table does not hold: same
// Compile the store runs, so the test is asserting on the shape the router
// would receive.
func mustCompile(t *testing.T, r Rule) event.CompiledRule {
	t.Helper()
	r.ID = DeriveID(r)
	cr, err := r.Compile(intent.DefaultRegistry())
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	return cr
}

// TestGrantsFollowStoredCapabilities: the table authorizes as the union of
// what its capabilities need, so deleting a rule stops asking for the access
// it needed rather than leaving a standing grant behind.
func TestGrantsFollowStoredCapabilities(t *testing.T) {
	s := newStore(t)
	if len(s.Grants()[PluginID]) != 0 {
		t.Fatal("an empty table grants nothing")
	}
	if _, err := s.Create(copyRule()); err != nil {
		t.Fatalf("Create: %v", err)
	}
	granted := s.Grants()[PluginID]
	if len(granted) != 1 || granted[0] != intent.PermAccessControl {
		t.Fatalf("grants = %v, want [accessibility.control]", granted)
	}
	if _, err := s.Create(Rule{Key: "P", Modifiers: []string{"Ctrl", "Shift"}, Capability: "terminal.openAt",
		Parameters: json.RawMessage(`{"path":"/tmp"}`), Emit: true}); err != nil {
		t.Fatalf("Create second: %v", err)
	}
	if len(s.Grants()[PluginID]) != 2 {
		t.Fatalf("grants = %v, want two permissions", s.Grants()[PluginID])
	}
	if err := s.Delete(DeriveID(copyRule())); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if got := s.Grants()[PluginID]; len(got) != 1 || got[0] != intent.PermShellExecution {
		t.Fatalf("grants after delete = %v, want only shell.execute", got)
	}
}

// TestPickedActionResolvesThroughWinlayout: the window-action picker stores a
// capability and the parameters winlayout says that action means. The editor
// composes neither.
func TestPickedActionResolvesThroughWinlayout(t *testing.T) {
	cap, params, err := ActionCapability(winlayout.ActionName(winlayout.LeftHalf))
	if err != nil {
		t.Fatalf("ActionCapability: %v", err)
	}
	if cap != "window.move" || string(params) != `{"zone":"leftHalf"}` {
		t.Fatalf("Left Half = %q/%s, want window.move/leftHalf", cap, params)
	}
	if _, _, err := ActionCapability("Sideways"); err == nil {
		t.Fatal("an unknown action must fail closed")
	}

	// Every row the picker serves resolves back to itself: Actions is
	// winlayout.AllActions, and the round-trip is what keeps the list and the
	// compiler reading one table.
	rows := Actions()
	if len(rows) != len(winlayout.AllActions) {
		t.Fatalf("Actions() = %d rows, want the %d winlayout actions", len(rows), len(winlayout.AllActions))
	}
	for _, row := range rows {
		cap, params, err := ActionCapability(row.Value)
		if err != nil {
			t.Fatalf("%s does not round-trip: %v", row.Value, err)
		}
		if cap != row.Capability || string(params) != string(row.Parameters) {
			t.Fatalf("%s round-trips to %q/%s, want %q/%s",
				row.Value, cap, params, row.Capability, row.Parameters)
		}
		// And a rule naming the row is one the table accepts.
		r := Rule{Key: "Left", Modifiers: []string{"Ctrl", "Win"}, Capability: cap, Parameters: params, Emit: true}
		r.ID = DeriveID(r)
		if _, err := r.Compile(intent.DefaultRegistry()); err != nil {
			t.Fatalf("a rule naming %s does not compile: %v", row.Value, err)
		}
	}
}

// TestVocabulariesCoverWhatTheyClaim: the key list is a name→code map and
// the modifier list a name→bit map, and both are total in both directions —
// the compiler reads them and the picker shows them, so a value in one
// direction with no match in the other is a key the user can pick and the
// rule cannot fire.
func TestVocabulariesCoverWhatTheyClaim(t *testing.T) {
	keys := Keys()
	if len(keys) < 60 {
		t.Fatalf("Keys() = %d rows, want the full VK set", len(keys))
	}
	seen := map[string]bool{}
	for _, k := range keys {
		if k.Value == "" || seen[k.Value] {
			t.Fatalf("key %q is empty or listed twice", k.Value)
		}
		seen[k.Value] = true
		code, err := KeyCode(k.Value)
		if err != nil || code != k.Code {
			t.Fatalf("KeyCode(%q) = %#x/%v, want %#x", k.Value, code, err, k.Code)
		}
	}
	if _, err := KeyCode("Kkk"); err == nil {
		t.Fatal("an unknown key must fail closed")
	}
	// "C" has to be 0x43: the builtin Ctrl+C rule is authored in that code,
	// and a key list that disagreed with it would make a user-built rule and
	// a builtin rule the same chord in name only.
	if code, _ := KeyCode("C"); code != 0x43 {
		t.Fatalf("C = %#x, want 0x43", code)
	}

	if len(Modifiers()) != 4 {
		t.Fatalf("Modifiers() = %d, want the four bits keyboard.State keeps", len(Modifiers()))
	}
	mask, err := ModifierMask([]string{"Ctrl", "Win"})
	if err != nil {
		t.Fatalf("ModifierMask: %v", err)
	}
	if mask != 0x01|0x08 {
		t.Fatalf("Ctrl+Win = %#x, want %#x", mask, 0x01|0x08)
	}
	if _, err := ModifierMask([]string{"Hyper"}); err == nil {
		t.Fatal("an unknown modifier must fail closed")
	}

	// Every capability the picker offers is one the registry has, and every
	// capability the registry has is offered — sorted, because the picker is
	// polled.
	caps := Capabilities()
	ids := intent.DefaultRegistry().IDs()
	if len(caps) != len(ids) {
		t.Fatalf("Capabilities() = %d, want the %d registry IDs", len(caps), len(ids))
	}
	for i, c := range caps {
		if i > 0 && caps[i-1].Value >= c.Value {
			t.Fatalf("capabilities are not sorted: %q then %q", caps[i-1].Value, c.Value)
		}
		if err := intent.ValidateCapabilityID(intent.DefaultRegistry(), c.Value); err != nil {
			t.Fatalf("picker offers %q, which the registry rejects: %v", c.Value, err)
		}
	}
}

// TestAppModesAreTheOnesTheMatcherKnows: the vocabulary is read from ctx and
// the matcher matches on event.AppMode. They are two spellings of one
// taxonomy, and a mode that exists in one and not the other is a row the
// builder offers and the router can never satisfy.
func TestAppModesAreTheOnesTheMatcherKnows(t *testing.T) {
	matcher := map[string]bool{
		string(event.AppModeNative):   true,
		string(event.AppModeTerminal): true,
		string(event.AppModeRemote):   true,
		string(event.AppModeVM):       true,
		string(event.AppModeExcluded): true,
	}
	modes := AppModes()
	if len(modes) != len(matcher) {
		t.Fatalf("AppModes() = %d rows, want the %d the matcher knows", len(modes), len(matcher))
	}
	for _, o := range modes {
		if !matcher[o.Value] {
			t.Errorf("app mode %q is in the vocabulary but not in the matcher taxonomy", o.Value)
		}
	}
	if len(AppCategories()) == 0 {
		t.Fatal("AppCategories() is empty; the builder has no groups to narrow the app list with")
	}
}

// TestRehydrateDropsUnusableRows: a document written by an older release (or
// by hand) carries a row naming a capability the registry no longer has. The
// rest of the table still comes up — dropping the whole file would take every
// other rule with it.
func TestRehydrateDropsUnusableRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "user-rules.json")
	good := copyRule()
	good.ID = DeriveID(good)
	bad := copyRule()
	bad.ID = "user.stale"
	bad.Capability = "window.explode"
	raw, err := json.Marshal(document{Rules: []Rule{good, bad}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	s, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if rules := s.Rules(); len(rules) != 1 || rules[0].ID != good.ID {
		t.Fatalf("Rules() = %+v, want the one rule that still compiles", rules)
	}
	// A file that is not a table at all is not a start-up failure either.
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	s2, err := New(path)
	if err != nil {
		t.Fatalf("New on a junk file: %v", err)
	}
	if len(s2.Rules()) != 0 {
		t.Fatalf("Rules() = %+v, want empty", s2.Rules())
	}
}
