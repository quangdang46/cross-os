// Windows 11 preset tests — task w1-win11-preset.
//
// Pass criteria mapping:
//  1. The spec chord set is data, verbatim (10 rows) → TestWindows11ChordSet.
//  2. Every action round-trips through ActionByName/ActionName, and a window
//     row's stored Capability agrees with the action it names →
//     TestWindows11ActionsRoundTrip.
//  3. Every dispatchable row is registry-bound — a preset never invents a
//     capability ID the canonical registry cannot honour →
//     TestWindows11CapabilitiesRegistryBound.
//  4. Disjoint from DefaultShortcuts: no shared chord, and the two window
//     actions both tables use sit on DIFFERENT chords (the muscle memory is
//     the point) → TestWindows11DisjointFromDefaults.
//  5. Rows own their modifier slice — editing one row cannot alias a
//     neighbour's → TestWindows11ModifierSlicesIndependent.
package winlayout

import (
	"fmt"
	"testing"

	"crossos/core/pkg/intent"
)

// chordOf renders a chord in ValidateShortcuts' own identity format, so the
// disjointness check compares chords the way the validator would.
func chordOf(mods []string, key string) string {
	return fmt.Sprintf("%v+%s", mods, key)
}

// TestWindows11ChordSet pins the spec table row for row. A preset is the
// product's promise — pick the Windows 11 profile and these chords just work —
// so a dropped chord or a renamed label is a regression, not a refactor.
func TestWindows11ChordSet(t *testing.T) {
	want := []ProfileShortcut{
		{[]string{"Ctrl"}, "C", "Copy", "clipboard.copy"},
		{[]string{"Ctrl"}, "V", "Paste", ""},
		{[]string{"Ctrl"}, "X", "Cut", ""},
		{[]string{"Ctrl"}, "A", "Select All", ""},
		{[]string{}, "F2", "Rename", ""},
		{[]string{"Win"}, "E", "Open Finder", "app.open"},
		{[]string{"Win"}, "Left", "Left Half", "window.move"},
		{[]string{"Win"}, "Right", "Right Half", "window.move"},
		{[]string{"Alt"}, "Tab", "App Switcher", ""},
		{[]string{"Shift"}, "Delete", "Move to Trash", "file.moveToTrash"},
	}
	got := Windows11Shortcuts()
	if len(got) != len(want) {
		t.Fatalf("preset rows=%d, want %d spec chords", len(got), len(want))
	}
	for i, w := range want {
		g := got[i]
		if chordOf(g.Modifiers, g.Key) != chordOf(w.Modifiers, w.Key) {
			t.Errorf("row %d chord=%s, want %s", i, chordOf(g.Modifiers, g.Key), chordOf(w.Modifiers, w.Key))
		}
		if g.Action != w.Action || g.Capability != w.Capability {
			t.Errorf("row %d %s: action=%q cap=%q, want %q/%q",
				i, chordOf(g.Modifiers, g.Key), g.Action, g.Capability, w.Action, w.Capability)
		}
	}
}

// TestWindows11ActionsRoundTrip proves the name ↔ action contract both ways.
// A window row's label resolves to its Action and back to the identical
// label, and the capability the row stores is the one that action resolves
// to — the row cannot advertise window.move under a label the config path
// (which speaks names) would not find. A non-window label must FAIL CLOSED:
// ActionByName("Paste") resolving to some neighbouring window action is how a
// clipboard chord would silently start snapping windows.
func TestWindows11ActionsRoundTrip(t *testing.T) {
	for _, row := range Windows11Shortcuts() {
		chord := chordOf(row.Modifiers, row.Key)
		a, ok := ActionByName(row.Action)
		if !ok {
			if row.Capability == "window.move" ||
				row.Capability == "window.minimize" ||
				row.Capability == "window.maximize" {
				t.Fatalf("%s dispatches %q but its label %q is not a window action",
					chord, row.Capability, row.Action)
			}
			continue
		}
		if back := ActionName(a); back != row.Action {
			t.Fatalf("%s: ActionByName(%q) → %q, want the same label back", chord, row.Action, back)
		}
		cap, zone := CapabilityFor(a)
		if cap != row.Capability {
			t.Fatalf("%s: action %q resolves to %q, row stores %q", chord, row.Action, cap, row.Capability)
		}
		if zone == "" || zone == "unknown" {
			t.Fatalf("%s: action %q leaves its zone unresolved (%q)", chord, row.Action, zone)
		}
	}
}

// TestWindows11CapabilitiesRegistryBound pins every dispatchable row to the
// canonical registry, the same gate dispatch_test.go applies to window
// actions. A preset row the registry cannot honour is a chord that silently
// does nothing on the user's machine.
func TestWindows11CapabilitiesRegistryBound(t *testing.T) {
	reg := intent.DefaultRegistry()
	dispatchable := 0
	for _, row := range Windows11Shortcuts() {
		if row.Capability == "" {
			continue // OS-native, or no capability exists yet (row says so)
		}
		desc, ok := reg.Get(row.Capability)
		if !ok {
			t.Fatalf("%s: capability %q not in canonical registry", chordOf(row.Modifiers, row.Key), row.Capability)
		}
		if desc.Permission == "" {
			t.Fatalf("%s: capability %q has no permission", chordOf(row.Modifiers, row.Key), row.Capability)
		}
		dispatchable++
	}
	if dispatchable == 0 {
		t.Fatal("preset dispatches nothing — every row would be a no-op chord")
	}
}

// TestWindows11DisjointFromDefaults is the merge guard. The two tables must
// claim no chord in common; the window actions they SHARE (Left Half / Right
// Half) are the same action on two different physical chords, which is the
// preset's whole reason to exist — Windows muscle memory (Win+Left) instead
// of the CrossOS-native default (Ctrl+Win+Left). Two rows inside the preset
// claiming one chord would be a §3.5b conflict, surfaced here instead of at
// dispatch time.
func TestWindows11DisjointFromDefaults(t *testing.T) {
	preset := Windows11Shortcuts()
	defaults := DefaultShortcuts()

	defaultChords := make(map[string]Action, len(defaults))
	for _, s := range defaults {
		defaultChords[chordOf(s.Modifiers, s.Key)] = s.Action
	}
	seen := make(map[string]string, len(preset))
	for _, row := range preset {
		chord := chordOf(row.Modifiers, row.Key)
		if first, dup := seen[chord]; dup {
			t.Fatalf("preset claims %s twice: %q then %q", chord, first, row.Action)
		}
		seen[chord] = row.Action
		if s, clash := defaultChords[chord]; clash {
			t.Fatalf("preset chord %s already belongs to DefaultShortcuts (%s)", chord, ActionName(s))
		}
	}
	// The shared actions are rebound, not dropped: the preset's whole point is
	// that Left Half answers to Win+Left instead of Ctrl+Win+Left.
	for _, action := range []Action{LeftHalf, RightHalf} {
		label, presetChord, defaultChord := ActionName(action), "", ""
		for chord, l := range seen {
			if l == label {
				presetChord = chord
			}
		}
		for chord, s := range defaultChords {
			if s == action {
				defaultChord = chord
			}
		}
		if presetChord == "" || defaultChord == "" {
			t.Fatalf("%s bound at preset=%q default=%q, want it bound in both tables",
				label, presetChord, defaultChord)
		}
		if presetChord == defaultChord {
			t.Fatalf("%s: preset and DefaultShortcuts both claim %s", label, presetChord)
		}
	}
}

// TestWindows11ModifierSlicesIndependent guards the aliasing footgun a shared
// modifier literal would leave in the table: an editor that rewrites one row's
// modifiers must not rewrite every other row that used the same slice. The
// deep copy of Modifiers is the point — copying a row copies the slice header,
// not the array behind it.
func TestWindows11ModifierSlicesIndependent(t *testing.T) {
	rows := Windows11Shortcuts()
	before := make([][]string, len(rows))
	for i, row := range rows {
		before[i] = append([]string(nil), row.Modifiers...)
	}
	rows[0].Modifiers[0] = "Alt"
	for i := 1; i < len(rows); i++ {
		if fmt.Sprint(rows[i].Modifiers) != fmt.Sprint(before[i]) {
			t.Fatalf("row %d picked up row 0's edit: %v → %v", i, before[i], rows[i].Modifiers)
		}
	}
}
