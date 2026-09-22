// Shortcut default tests — bead cross-os-nir.5.
//
// Pass criteria mapping:
//  1. §6.2 table as data (10 rows, all resolve via window.*) → TestDefaults.
//  2. User edits validate + persist shape (schema-shaped check) → TestValidate.
//  3. Duplicate chords surface as §3.5b conflicts, never silent → TestDuplicate.
//  4. Nudge defaults are BEHAVIOR reference only (no Nudge binding copied) →
//     TestNotNudgeBindings (our Ctrl+Win chords differ from Nudge Ctrl+Opt).
package winlayout

import "testing"

func TestDefaults(t *testing.T) {
	got := DefaultShortcuts()
	if len(got) != 10 {
		t.Fatalf("defaults=%d, want 10 §6.2 rows", len(got))
	}
	for _, s := range got {
		cap, _ := CapabilityFor(s.Action)
		if cap != "window.move" && cap != "window.minimize" && cap != "window.maximize" {
			t.Fatalf("action %s resolves outside window.*: %q", ActionName(s.Action), cap)
		}
	}
	if err := ValidateShortcuts(got); err != nil {
		t.Fatalf("defaults fail own validation: %v", err)
	}
}

func TestValidate(t *testing.T) {
	edited := DefaultShortcuts()
	edited[0].Key = "F9" // user remap: still valid (known action, unique chord)
	if err := ValidateShortcuts(edited); err != nil {
		t.Fatalf("remap rejected: %v", err)
	}
}

func TestDuplicate(t *testing.T) {
	dup := DefaultShortcuts()
	dup[1] = dup[0] // same chord twice
	if err := ValidateShortcuts(dup); err == nil {
		t.Fatal("duplicate chord silently kept (want §3.5b conflict surface)")
	}
}

func TestNotNudgeBindings(t *testing.T) {
	// Nudge defaults use Ctrl+Opt (⌃⌥); ours use Ctrl+Win per §6.2.
	// If these ever match, someone copied Nudge's table instead of §6.2.
	for _, s := range DefaultShortcuts() {
		for _, m := range s.Modifiers {
			if m == "Opt" || m == "Option" {
				t.Fatalf("Nudge binding leaked into defaults: %+v", s)
			}
		}
	}
}
