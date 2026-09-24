// Settings store tests: toggles, shortcut validation, persistence, the
// first-run state, and the all-or-nothing batch apply.
package settings

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"crossos/core/pkg/filetype"
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
		"fileTypes":          "array",
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
	catalog := []filetype.FileType{
		{Ext: "rs", BaseName: "main", DisplayName: "New Rust", Enabled: true, BuiltIn: false},
		{Ext: "env", BaseName: "", DisplayName: "New .env", Enabled: true, BuiltIn: true},
	}
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
		{"file types", func() error { return s.SetFileTypes(catalog) }},
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
	if got := s2.FileTypes(); !reflect.DeepEqual(got, catalog) {
		t.Errorf("FileTypes() after reload=%+v, want %+v", got, catalog)
	}
}

// TestFileTypesSeedsWhenNeverWritten: the Finder menu renders from this list,
// so a store with nothing in it would hand "New >" an empty menu on a fresh
// install. Two shapes of "never written" have to answer with the seeds: no
// config file at all, and a config file that exists but predates the catalog
// (the first unrelated write puts one on disk without the key).
func TestFileTypesSeedsWhenNeverWritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	s, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got := s.FileTypes()
	if got == nil {
		t.Fatal("a store with no catalog must serve the seeds, not nil")
	}
	if want := filetype.Seeds(); !reflect.DeepEqual(got, want) {
		t.Fatalf("FileTypes()=%+v, want the %d seeds", got, len(want))
	}
	if err := s.SetPanicStopped(true); err != nil {
		t.Fatalf("unrelated write: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, present := doc["fileTypes"]; !present {
		t.Fatalf("the write path dropped the catalog key: %s", raw)
	}
	s2, err := New(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if got := s2.FileTypes(); !reflect.DeepEqual(got, filetype.Seeds()) {
		t.Fatalf("seeds after a reload that carried no catalog=%+v", got)
	}

	// A hand-edited catalog that no longer validates is dropped back to the
	// seeds, the same deal a bad zone list gets: a menu row the daemon
	// refuses to create is worse than the default row.
	if err := os.WriteFile(path, []byte(`{"fileTypes":[{"ext":"","displayName":"New Nothing"}]}`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	s3, err := New(path)
	if err != nil {
		t.Fatalf("reopen after a hand edit: %v", err)
	}
	if got := s3.FileTypes(); !reflect.DeepEqual(got, filetype.Seeds()) {
		t.Fatalf("an invalid catalog was served: %+v", got)
	}
}

// TestFileTypesClearedStaysCleared: clearing the catalog is a user decision
// ("I don't want New > to offer these"), so it has to come back cleared. A
// store that restored the seeds on the next start would undo it silently —
// and null would fail the schema check outright, leaving the previous
// document on disk while the running set had already moved.
func TestFileTypesClearedStaysCleared(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	s, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.SetFileTypes(nil); err != nil {
		t.Fatalf("clearing: %v", err)
	}
	if got := s.FileTypes(); got == nil || len(got) != 0 {
		t.Fatalf("FileTypes()=%v, want an empty non-nil list", got)
	}
	s2, err := New(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if got := s2.FileTypes(); len(got) != 0 {
		t.Fatalf("catalog after reload=%+v, want it still cleared", got)
	}
	// And the order the user listed is the order the menu renders: the index
	// is a row someone can see, so it must not be sorted away.
	listed := []filetype.FileType{
		{Ext: "yml", DisplayName: "New YAML"},
		{Ext: "env", DisplayName: "New .env"},
		{Ext: "md", DisplayName: "New Markdown"},
	}
	if err := s2.SetFileTypes(listed); err != nil {
		t.Fatalf("relisting: %v", err)
	}
	s3, err := New(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if got := s3.FileTypes(); !reflect.DeepEqual(got, listed) {
		t.Fatalf("catalog after reload=%+v, want the rows in the order they were saved %+v", got, listed)
	}
}

// TestFileTypesRejectsUnusableRow: one row the daemon cannot create refuses
// the whole edit, leaving the running catalog and the file alone. A partial
// write would put a row in the menu that the create path turns away.
func TestFileTypesRejectsUnusableRow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	s, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	before := s.FileTypes()
	for _, bad := range [][]filetype.FileType{
		{{Ext: "rs", DisplayName: "New Rust"}, {Ext: "  ", DisplayName: "New Nothing"}},
		{{Ext: ".env"}},
		{{Ext: "rs", BaseName: "main"}, {Ext: "/../../etc/passwd", BaseName: "x"}},
	} {
		if err := s.SetFileTypes(bad); err == nil {
			t.Errorf("catalog %+v must be rejected", bad)
		}
		if got := s.FileTypes(); !reflect.DeepEqual(got, before) {
			t.Fatalf("a rejected edit changed the running catalog: %+v", got)
		}
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("a rejected catalog created a config file")
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

// TestFileTypesReadDuringWrite: a reader on the FileTypes() path must never
// catch a write in flight. Two whole catalogs exist — the seeds and the
// replacement — and every observation has to be one of them, never a mix and
// never nothing. The file read is the same check from the other side: a
// half-written document is one a fresh daemon cannot parse, and it would take
// the user's whole settings layer down with it.
func TestFileTypesReadDuringWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	s, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	seeds := filetype.Seeds()
	custom := []filetype.FileType{
		{Ext: "rs", BaseName: "main", DisplayName: "New Rust", Enabled: true},
		{Ext: "env", BaseName: "", DisplayName: "New .env", Enabled: true},
	}
	whole := func(got []filetype.FileType) bool {
		return reflect.DeepEqual(got, seeds) || reflect.DeepEqual(got, custom)
	}
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			if err := s.SetFileTypes(custom); err != nil {
				t.Errorf("write custom: %v", err)
				return
			}
			if err := s.SetFileTypes(seeds); err != nil {
				t.Errorf("write seeds: %v", err)
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			if got := s.FileTypes(); !whole(got) {
				t.Errorf("FileTypes() read a partial catalog: %+v", got)
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			raw, err := os.ReadFile(path)
			if err != nil {
				// Nothing has been renamed into place yet; the write
				// above has not run. Not a torn read.
				continue
			}
			var doc document
			if err := json.Unmarshal(raw, &doc); err != nil {
				t.Errorf("the file was read half-written: %v (%s)", err, raw)
				return
			}
			if doc.FileTypes == nil {
				t.Errorf("the document carried no catalog: %s", raw)
				return
			}
			if !whole(doc.FileTypes) {
				t.Errorf("the document carried a partial catalog: %+v", doc.FileTypes)
				return
			}
		}
	}()
	wg.Wait()
	// A store reopened from the file agrees with the last writer's catalog.
	s2, err := New(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if got := s2.FileTypes(); !whole(got) {
		t.Fatalf("catalog after reload=%+v, want one of the two whole lists", got)
	}
}

// TestConcurrentWritersDoNotLoseEachOther: a writer that returns nil is in the
// file. The write path used to snapshot the state, drop the lock, and only
// then rename — so two writers could snapshot the same before-state and the
// second rename would silently drop the first one's change, leaving the
// document and the running set disagreeing about what the user set.
func TestConcurrentWritersDoNotLoseEachOther(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	s, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	known := func(string) bool { return true }
	const writers = 8
	var wg sync.WaitGroup
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for round := 0; round < 8; round++ {
				rule := fmt.Sprintf("rule.%d", i)
				if err := s.SetRuleEnabled(rule, false, known); err != nil {
					t.Errorf("disable %s: %v", rule, err)
					return
				}
				if err := s.SetOverride("com.apple.Finder", rule, true, known); err != nil {
					t.Errorf("override %s: %v", rule, err)
					return
				}
			}
		}(i)
	}
	wg.Wait()
	s2, err := New(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if got := len(s2.Overrides()); got != writers {
		t.Errorf("overrides after reload=%d, want %d — a write was lost", got, writers)
	}
	for i := 0; i < writers; i++ {
		rule := fmt.Sprintf("rule.%d", i)
		if s2.IsRuleEnabled(rule) {
			t.Errorf("%s came back enabled: a write was lost", rule)
		}
	}
}
