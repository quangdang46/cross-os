// Settings store tests: toggles, shortcut validation, persistence.
package settings

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"crossos/core/pkg/winlayout"
)

func TestRuleToggle(t *testing.T) {
	s, err := New("")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	known := func(id string) bool { return id == "windows-keyboard.ctrl-c-copy" }
	if !s.IsRuleEnabled("windows-keyboard.ctrl-c-copy") {
		t.Fatal("rules start enabled")
	}
	if err := s.SetRuleEnabled("windows-keyboard.ctrl-c-copy", false, known); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if s.IsRuleEnabled("windows-keyboard.ctrl-c-copy") {
		t.Fatal("disabled rule must not fire")
	}
	if err := s.SetRuleEnabled("windows-keyboard.ctrl-c-copy", true, known); err != nil {
		t.Fatalf("re-enable: %v", err)
	}
	if err := s.SetRuleEnabled("nope", false, known); err == nil {
		t.Fatal("unknown rule must fail closed")
	}
}

func TestSetShortcutsValidated(t *testing.T) {
	s, err := New("")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	good := winlayout.DefaultShortcuts()
	if err := s.SetShortcuts(good); err != nil {
		t.Fatalf("valid table: %v", err)
	}
	dup := append(append([]winlayout.Shortcut(nil), good...), good[0])
	if err := s.SetShortcuts(dup); err == nil {
		t.Fatal("duplicate chords must be rejected")
	}
	if got := len(s.Shortcuts()); got != len(good) {
		t.Fatalf("rejected edit must leave running set untouched: %d vs %d", got, len(good))
	}
}

func TestPersistRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	s, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	known := func(string) bool { return true }
	if err := s.SetRuleEnabled("some.rule", false, known); err != nil {
		t.Fatalf("disable: %v", err)
	}
	s2, err := New(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if s2.IsRuleEnabled("some.rule") {
		t.Fatal("disabled rule must survive reload (persisted)")
	}
}

// TestClearedListsStillPersist: emptying a list must survive the Config
// Manager's schema check. A nil slice marshals to null, which the "array"
// schema rejects — the write would abort, leaving the previous document on
// disk while the running set had already been cleared.
func TestClearedListsStillPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	s, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	known := func(string) bool { return true }
	if err := s.SetOverride("com.apple.Finder", "some.rule", false, known); err != nil {
		t.Fatalf("override: %v", err)
	}
	if err := s.SetZones([]winlayout.Zone{{ID: "a", Name: "A", W: 10, H: 10}}); err != nil {
		t.Fatalf("zones: %v", err)
	}
	if err := s.SetOverride("com.apple.Finder", "some.rule", true, known); err != nil {
		t.Fatalf("the first override write must persist: %v", err)
	}
	if err := s.SetZones([]winlayout.Zone{}); err != nil {
		t.Fatalf("clearing zones: %v", err)
	}
	// The shortcut table is covered here only as far as this change reaches:
	// clearing it must not fail the schema check, and the cleared table must
	// come back cleared — the Windows editor reports "Saved 0 shortcuts.", so
	// a store that restored the seed rows on the next start would tell the
	// user their clear was undone.
	if err := s.SetShortcuts([]winlayout.Shortcut{}); err != nil {
		t.Fatalf("clearing shortcuts: %v", err)
	}
	if got := s.Zones(); got == nil || len(got) != 0 {
		t.Fatalf("Zones()=%v, want an empty non-nil list (null is not a value the editor can read)", got)
	}
	if got := s.Overrides(); len(got) != 1 || !got[0].Enabled {
		t.Fatalf("Overrides()=%v, want the flipped verdict", got)
	}
	s2, err := New(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if got := s2.Zones(); len(got) != 0 {
		t.Fatalf("zones after reload=%v, want none", got)
	}
	if got := s2.Overrides(); len(got) != 1 || !got[0].Enabled {
		t.Fatalf("overrides after reload=%v", got)
	}
	if got := s2.Shortcuts(); len(got) != 0 {
		t.Fatalf("shortcuts after reload=%v, want the cleared table, not the seed rows", got)
	}
}

// TestShortcutsRehydrate: the seed table is the fallback, not the answer. A
// user who edited the Windows page and closed the app must find their table,
// and a document written before the editor existed — no "shortcuts" key at
// all — must still come up on the seed rows rather than on an empty one.
func TestShortcutsRehydrate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	s, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	edited := winlayout.DefaultShortcuts()[:1]
	if err := s.SetShortcuts(edited); err != nil {
		t.Fatalf("SetShortcuts: %v", err)
	}
	s2, err := New(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if got := s2.Shortcuts(); len(got) != 1 || got[0].Key != edited[0].Key {
		t.Fatalf("shortcuts after reload=%v, want the one row that was saved", got)
	}

	// A hand-edited file naming an action that resolves to nothing is dropped
	// back to the seed table, the same way a bad zone list is.
	raw := []byte(`{"shortcuts":[{"action":"nonsense","modifiers":["Ctrl"],"key":"Q"}]}`)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	s3, err := New(path)
	if err != nil {
		t.Fatalf("reopen after a hand edit: %v", err)
	}
	if got, want := len(s3.Shortcuts()), len(winlayout.DefaultShortcuts()); got != want {
		t.Fatalf("shortcuts after an invalid list=%d, want the %d seed rows", got, want)
	}
}

// TestOverrideReader: the read side of a per-app verdict. ok=false is what
// tells the rule engine to fall back to the global toggle, and it must be
// false for an app nobody has expressed an opinion about — including the empty
// app id, which no rule can be scoped to.
func TestOverrideReader(t *testing.T) {
	s, err := New("")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	known := func(string) bool { return true }
	if _, ok := s.Override("com.apple.Finder", "some.rule"); ok {
		t.Fatal("an app with no stored verdict must report ok=false")
	}
	if _, ok := s.Override("", "some.rule"); ok {
		t.Fatal("the empty app id can never be scoped")
	}
	if err := s.SetOverride("com.apple.Finder", "some.rule", false, known); err != nil {
		t.Fatalf("SetOverride: %v", err)
	}
	if on, ok := s.Override("com.apple.Finder", "some.rule"); !ok || on {
		t.Fatalf("Override=%v,%v, want false,true for the stored off verdict", on, ok)
	}
	if _, ok := s.Override("com.apple.Safari", "some.rule"); ok {
		t.Fatal("a verdict stored for one app must not answer for another")
	}
	// Both verdicts are kept: an explicit "this app still wants it" has to
	// outlive a global switch-off, or the per-app intent silently follows the
	// global switch.
	if err := s.SetOverride("com.apple.Finder", "some.rule", true, known); err != nil {
		t.Fatalf("SetOverride on: %v", err)
	}
	if on, ok := s.Override("com.apple.Finder", "some.rule"); !ok || !on {
		t.Fatalf("Override=%v,%v, want true,true after the flip", on, ok)
	}
}

// TestOverrideRejectsUnknownRule: an override naming a rule that does not
// exist is refused, so a typo cannot leave a stored row nothing reads.
func TestOverrideRejectsUnknownRule(t *testing.T) {
	s, err := New("")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	known := func(id string) bool { return id == "real.rule" }
	if err := s.SetOverride("com.apple.Finder", "nope", false, known); err == nil {
		t.Fatal("unknown rule must fail closed")
	}
	if got := s.Overrides(); len(got) != 0 {
		t.Fatalf("a rejected override was stored: %+v", got)
	}
	if err := s.SetOverride("com.apple.Finder", "real.rule", false, known); err != nil {
		t.Fatalf("known rule: %v", err)
	}
	// Sorted by app then rule ID, because the editor polls this list.
	s2, _ := New("")
	if err := s2.SetOverride("com.microsoft.VSCode", "z.rule", true, func(string) bool { return true }); err != nil {
		t.Fatalf("z: %v", err)
	}
	if err := s2.SetOverride("com.apple.Finder", "b.rule", true, func(string) bool { return true }); err != nil {
		t.Fatalf("b: %v", err)
	}
	got := s2.Overrides()
	if len(got) != 2 || got[0].App != "com.apple.Finder" || got[1].App != "com.microsoft.VSCode" {
		t.Fatalf("Overrides()=%+v, want sorted by app", got)
	}
}

// TestSetZonesValidated: one bad row rejects the whole edit, and the running
// list is untouched.
func TestSetZonesValidated(t *testing.T) {
	s, err := New("")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	good := []winlayout.Zone{{ID: "a", Name: "A", W: 10, H: 10}}
	if err := s.SetZones(good); err != nil {
		t.Fatalf("valid: %v", err)
	}
	for _, bad := range [][]winlayout.Zone{
		{{ID: "a", Name: "A", W: 10, H: 10}, {ID: "a", Name: "B", W: 10, H: 10}},
		{{ID: "", Name: "B", W: 10, H: 10}},
		{{ID: "b", Name: "", W: 10, H: 10}},
		{{ID: "b", Name: "B", W: 0, H: 10}},
		{{ID: "b", Name: "B", X: math.NaN(), W: 10, H: 10}},
	} {
		if err := s.SetZones(bad); err == nil {
			t.Errorf("zone list %+v must be rejected", bad)
		}
		if got := s.Zones(); len(got) != 1 || got[0].ID != "a" {
			t.Fatalf("a rejected edit changed the running set: %+v", got)
		}
	}
}
