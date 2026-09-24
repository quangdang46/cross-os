// Settings store tests: toggles, shortcut validation, persistence, the
// first-run state, and the all-or-nothing batch apply.
package settings

import (
	"bytes"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"crossos/core/pkg/winlayout"
)

func boolPtr(b bool) *bool    { return &b }
func strPtr(s string) *string { return &s }

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

// TestSchemaDeclaresFirstRunKeys: the Config Manager rejects an undeclared
// key, and a rejected user layer is dropped WHOLE. So a key the store writes
// but the schema does not declare is not a cosmetic gap — it is the user's
// onboarding flag, plugin verdicts and profile gone on the next launch, with
// no error raised anywhere.
func TestSchemaDeclaresFirstRunKeys(t *testing.T) {
	schema := ShortcutSchema()
	for key, want := range map[string]string{
		"onboardingComplete": "bool",
		"enabledPlugins":     "array",
		"activeProfile":      "string",
	} {
		field, ok := schema[key]
		if !ok {
			t.Errorf("ShortcutSchema does not declare %q", key)
			continue
		}
		if field.Type != want {
			t.Errorf("%s: type %q, want %q", key, field.Type, want)
		}
	}
}

// TestFullRoundTrip: every persisted value survives a restart. The first-run
// fields are in here for the same reason panicStop is — a flag that reads back
// as its zero value after a restart is a user who finished the wizard and then
// meets it again on the next launch.
func TestFullRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	s, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	known := func(string) bool { return true }
	shortcuts := winlayout.DefaultShortcuts()[:2]
	zones := []winlayout.Zone{{ID: "tl", Name: "Top left", W: 0.5, H: 0.5}}
	for _, step := range []struct {
		what string
		do   func() error
	}{
		{"rule toggle", func() error { return s.SetRuleEnabled("some.rule", false, known) }},
		{"override", func() error { return s.SetOverride("com.apple.Finder", "some.rule", true, known) }},
		{"shortcuts", func() error { return s.SetShortcuts(shortcuts) }},
		{"zones", func() error { return s.SetZones(zones) }},
		{"panic stop", func() error { return s.SetPanicStopped(true) }},
		{"onboarding", func() error { return s.SetOnboardingComplete(true) }},
		{"plugin on", func() error { return s.SetPluginEnabled("windows-keyboard", true, known) }},
		{"plugin off", func() error { return s.SetPluginEnabled("windows-window", false, known) }},
		{"profile", func() error { return s.SetActiveProfile("windows11") }},
	} {
		if err := step.do(); err != nil {
			t.Fatalf("%s: %v", step.what, err)
		}
	}

	s2, err := New(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if s2.IsRuleEnabled("some.rule") {
		t.Error("a disabled rule came back enabled")
	}
	if on, ok := s2.Override("com.apple.Finder", "some.rule"); !ok || !on {
		t.Errorf("override after reload=(%v,%v), want (true,true)", on, ok)
	}
	if got := s2.Shortcuts(); !reflect.DeepEqual(got, shortcuts) {
		t.Errorf("shortcuts after reload=%+v, want %+v", got, shortcuts)
	}
	if got := s2.Zones(); !reflect.DeepEqual(got, zones) {
		t.Errorf("zones after reload=%+v, want %+v", got, zones)
	}
	if !s2.PanicStopped() {
		t.Error("PANIC STOP did not survive the restart")
	}
	if !s2.OnboardingComplete() {
		t.Error("a finished onboarding came back unfinished")
	}
	if got, want := s2.PluginsEnabled(), []string{"windows-keyboard"}; !reflect.DeepEqual(got, want) {
		t.Errorf("PluginsEnabled()=%v, want %v", got, want)
	}
	if got := s2.ActiveProfile(); got != "windows11" {
		t.Errorf("ActiveProfile()=%q, want windows11", got)
	}
}

// TestApplyBatchAppliesWholeProfile: one plan, one write, all of it landing —
// the "Windows 11 Experience" card the spec sells as a single click. A nil
// field is "leave alone", never "clear": the second plan here touches plugins
// only and must not disturb the rules, the profile or the onboarding flag.
func TestApplyBatchAppliesWholeProfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	s, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	cat := Catalog{
		Rules:   func(id string) bool { return id == "windows-keyboard.ctrl-c-copy" },
		Plugins: func(id string) bool { return id == "windows-keyboard" || id == "windows-window" },
	}
	if err := s.ApplyBatch(Plan{
		Rules: []Toggle{{ID: "windows-keyboard.ctrl-c-copy", Enabled: true}},
		Plugins: []Toggle{
			{ID: "windows-keyboard", Enabled: true},
			{ID: "windows-window", Enabled: true},
		},
		OnboardingComplete: boolPtr(true),
		ActiveProfile:      strPtr("windows11"),
	}, cat); err != nil {
		t.Fatalf("profile plan: %v", err)
	}
	if !s.OnboardingComplete() || s.ActiveProfile() != "windows11" {
		t.Fatalf("onboarding=%v profile=%q, want true,windows11", s.OnboardingComplete(), s.ActiveProfile())
	}
	if err := s.ApplyBatch(Plan{
		Plugins: []Toggle{{ID: "windows-window", Enabled: false}},
	}, cat); err != nil {
		t.Fatalf("plugins-only plan: %v", err)
	}
	if got, want := s.PluginsEnabled(), []string{"windows-keyboard"}; !reflect.DeepEqual(got, want) {
		t.Errorf("PluginsEnabled()=%v, want %v — a plan may not clear fields it does not name", got, want)
	}
	if got := s.ActiveProfile(); got != "windows11" {
		t.Errorf("ActiveProfile()=%q, want the profile the second plan never named", got)
	}
	if !s.OnboardingComplete() {
		t.Error("the second plan cleared a field it does not name")
	}
	if s2, err := New(path); err != nil {
		t.Fatalf("reopen: %v", err)
	} else if got, want := s2.PluginsEnabled(), []string{"windows-keyboard"}; !reflect.DeepEqual(got, want) {
		t.Errorf("PluginsEnabled() after reload=%v, want %v", got, want)
	} else if !s2.OnboardingComplete() || s2.ActiveProfile() != "windows11" {
		t.Errorf("after reload: onboarding=%v profile=%q, want true,windows11", s2.OnboardingComplete(), s2.ActiveProfile())
	}
}

// TestApplyBatchLeavesStoreUntouchedOnBadRule: the batch exists so a profile
// is one gesture, and one gesture has to be all-or-nothing. A plan whose last
// element names a rule that does not exist must leave both the running set and
// the file exactly as they were — a rewritten file is how the two start
// disagreeing about what the user set.
func TestApplyBatchLeavesStoreUntouchedOnBadRule(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	s, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	cat := Catalog{
		Rules:   func(id string) bool { return id == "windows-keyboard.ctrl-c-copy" },
		Plugins: func(string) bool { return true },
	}
	if err := s.ApplyBatch(Plan{
		Plugins:       []Toggle{{ID: "windows-keyboard", Enabled: true}},
		ActiveProfile: strPtr("windows11"),
	}, cat); err != nil {
		t.Fatalf("first plan: %v", err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	err = s.ApplyBatch(Plan{
		Rules: []Toggle{
			{ID: "windows-keyboard.ctrl-c-copy", Enabled: false},
			{ID: "windows-keyboard.typo", Enabled: false},
		},
		OnboardingComplete: boolPtr(true),
		ActiveProfile:      strPtr("macos-native"),
	}, cat)
	if err == nil {
		t.Fatal("a plan naming an unknown rule must be refused")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("a refused plan rewrote the document:\nbefore %s\nafter  %s", before, after)
	}
	if !s.IsRuleEnabled("windows-keyboard.ctrl-c-copy") {
		t.Error("the valid element of a refused plan was applied anyway")
	}
	if s.OnboardingComplete() {
		t.Error("a refused plan latched the onboarding flag")
	}
	if got := s.ActiveProfile(); got != "windows11" {
		t.Errorf("ActiveProfile()=%q, want the profile the refused plan never got to change", got)
	}
	if got, want := s.PluginsEnabled(), []string{"windows-keyboard"}; !reflect.DeepEqual(got, want) {
		t.Errorf("PluginsEnabled()=%v, want %v", got, want)
	}
}

// TestApplyBatchFailsClosedWithoutCatalog: a nil predicate is a missing
// answer, not permission. If the daemon has not published its rule table or
// its plugin registry, the batch refuses rather than persisting ids nothing
// will ever read.
func TestApplyBatchFailsClosedWithoutCatalog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	s, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	rules := func(string) bool { return true }
	if err := s.ApplyBatch(Plan{Plugins: []Toggle{{ID: "any", Enabled: true}}}, Catalog{Rules: rules}); err == nil {
		t.Error("a plugin id with no plugin catalog must be refused")
	}
	if err := s.ApplyBatch(Plan{Rules: []Toggle{{ID: "any", Enabled: false}}}, Catalog{}); err == nil {
		t.Error("a rule id with no rule catalog must be refused")
	}
	if err := s.ApplyBatch(Plan{ActiveProfile: strPtr("  ")}, Catalog{}); err == nil {
		t.Error("an empty profile id must be refused")
	}
	if got := s.PluginsEnabled(); len(got) != 0 {
		t.Errorf("a refused plan stored %v", got)
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("a refused plan created a config file")
	}
}

// TestFirstRunSettersFailClosed: the single-value writes keep the same
// contract the batch has. A typo'd plugin ID must not persist a row no page
// can act on, and a plugin nobody has touched must not appear enabled.
func TestFirstRunSettersFailClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	s, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	known := func(id string) bool { return id == "windows-keyboard" }
	if err := s.SetPluginEnabled("nope", true, known); err == nil {
		t.Fatal("an unknown plugin must fail closed")
	}
	if got := s.PluginsEnabled(); len(got) != 0 {
		t.Fatalf("a rejected plugin write was stored: %v", got)
	}
	if err := s.SetPluginEnabled("windows-keyboard", true, known); err != nil {
		t.Fatalf("known plugin: %v", err)
	}
	if err := s.SetPluginEnabled("windows-keyboard", false, known); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if got := s.PluginsEnabled(); len(got) != 0 {
		t.Errorf("PluginsEnabled()=%v, want none after the disable", got)
	}
	if err := s.SetActiveProfile("  "); err == nil {
		t.Error("an empty profile id must be refused")
	}
	if got := s.ActiveProfile(); got != "" {
		t.Errorf("ActiveProfile()=%q, want the refused write not to have landed", got)
	}
}

// TestPluginsEnabledSorted: the pages poll this list, and Go map order would
// reshuffle the rows between refreshes — which reads to the user as a list
// that flickers for no reason.
func TestPluginsEnabledSorted(t *testing.T) {
	s, err := New("")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	known := func(string) bool { return true }
	for _, id := range []string{"windows-window", "alt-tab", "windows-keyboard"} {
		if err := s.SetPluginEnabled(id, true, known); err != nil {
			t.Fatalf("enable %s: %v", id, err)
		}
	}
	want := []string{"alt-tab", "windows-keyboard", "windows-window"}
	if got := s.PluginsEnabled(); !reflect.DeepEqual(got, want) {
		t.Errorf("PluginsEnabled()=%v, want %v", got, want)
	}
}
