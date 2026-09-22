// Default window shortcuts: §6.2 table as data (bead cross-os-nir.5).
//
// The §6.2 plan table (Ctrl+Win+arrows et al.) is the CrossOS-native
// default set. Nudge's own defaults (Ctrl+Opt+arrows, from its README
// Default Keyboard Shortcuts table) are the UX cross-check per the §9.10
// rectangle/nudge rows — BEHAVIOR reference only, nothing copied: the
// table below re-expresses the §6.2 plan rows as Go data, not Nudge's
// bindings. Users edit these through the Windows page (persisted via
// Config Manager with schema validation); edits resolve through the
// standard Rule → Intent → window.* path, so hotkey conflicts use §3.5b
// resolution like any other rule — no privileged path.
package winlayout

import "fmt"

// Shortcut is one editable default: physical chord → window Action.
type Shortcut struct {
	Action Action
	// Modifiers as keyboard.Mod* bit names (Ctrl, Win/Meta, Shift), kept as
	// strings so the table stays readable without importing keyboard from
	// this pure-geometry package. The adapter/editor maps names → bits.
	Modifiers []string
	Key       string // "Left", "Right", "Up", "Down", "Home", "End", "PageUp", "PageDown"
}

// DefaultShortcuts is the §6.2 shortcut table as data (10 rows).
func DefaultShortcuts() []Shortcut {
	ctrlWin := []string{"Ctrl", "Win"}
	ctrlWinShift := []string{"Ctrl", "Win", "Shift"}
	return []Shortcut{
		{LeftHalf, ctrlWin, "Left"},
		{RightHalf, ctrlWin, "Right"},
		{Maximize, ctrlWin, "Up"},
		{Minimize, ctrlWin, "Down"},
		{LeftThird, ctrlWinShift, "Left"},
		{RightTwoThirds, ctrlWinShift, "Right"},
		{Center, ctrlWin, "Home"},
		{Fullscreen, ctrlWin, "End"},
		{NextDisplay, ctrlWin, "PageUp"},
		{PreviousDisplay, ctrlWin, "PageDown"},
	}
}

// Validate checks a user-edited shortcut set: every action resolves to a
// known window.* capability, no duplicate chords (duplicates are §3.5b
// conflicts — surfaced, never silently kept).
func ValidateShortcuts(set []Shortcut) error {
	seen := map[string]string{}
	for _, s := range set {
		cap, _ := CapabilityFor(s.Action)
		if cap != "window.move" && cap != "window.minimize" && cap != "window.maximize" {
			return fmt.Errorf("winlayout: action %s resolves to %q (not a window capability)", ActionName(s.Action), cap)
		}
		chord := fmt.Sprintf("%v+%s", s.Modifiers, s.Key)
		if prev, dup := seen[chord]; dup {
			return fmt.Errorf("winlayout: duplicate chord %s (%s vs %s — resolve via §3.5b)", chord, prev, ActionName(s.Action))
		}
		seen[chord] = ActionName(s.Action)
	}
	return nil
}
