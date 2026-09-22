// Settings store tests: toggles, shortcut validation, persistence.
package settings

import (
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
