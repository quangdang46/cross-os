// Windows 11 chord preset: the product spec's "Zero-learning Windows UX"
// table as data (ChatGPT-CROSSOS-20260924-2106.md; task w1-win11-preset) —
// the preset a user picks at onboarding instead of toggling a plugin.
//
// NOT DefaultShortcuts, and the two must not be re-merged. DefaultShortcuts is
// the window-SNAP table and its action set is exactly window.move /
// window.minimize / window.maximize: ValidateShortcuts rejects every row
// outside that set, so Copy or Move to Trash has nowhere to sit in it (and
// Settings.SetShortcuts would refuse the whole list). The Windows 11 preset is
// a second table over a WIDER action vocabulary — clipboard.*, app.*, file.*,
// plus the chords CrossOS does not dispatch at all. One chord vocabulary, two
// tables; choosing a profile is what moves a chord between them.
//
// Data only, like DefaultShortcuts/DefaultZones: no dispatch, no rule
// compilation, no AX. plugins/windows-keyboard stays the runtime path — a
// preset row is what the user selects, not what runs.
package winlayout

// ProfileShortcut is one row of a chord-profile preset: a physical chord plus
// the action the profile binds to it. Modifiers/Key carry the same vocabulary
// as Shortcut (keyboard.Mod* bit names + key name) so one chord formatter
// serves both tables. Action is the label the user reads, and on a window row
// it is that window Action's display name — so ActionByName resolves it back
// to the Action. Capability is the §3.12 capability the chord dispatches;
// empty when the OS handles the chord itself (Ctrl+V/X/A: the Ctrl→Cmd
// translation macOS already does inside a focused control) or when no
// capability exists yet (F2 rename, Alt+Tab switching — the same gap
// plugins/windows-keyboard records in its coverage note).
type ProfileShortcut struct {
	Modifiers  []string
	Key        string
	Action     string
	Capability string
}

// Windows11Shortcuts returns the preset's chord set as data: the ten chords
// of the spec table (Ctrl+C/V/X, Ctrl+A, F2, Win+E, Win+Left/Right, Alt+Tab,
// Shift+Delete). Win+Up/Down and Alt+F4 are matrix rows, not preset rows —
// the preset ships the table the spec lists and no more. Each row owns its
// modifier slice: a shared literal would alias one row's edit into every
// other row.
func Windows11Shortcuts() []ProfileShortcut {
	return []ProfileShortcut{
		{[]string{"Ctrl"}, "C", "Copy", "clipboard.copy"},
		{[]string{"Ctrl"}, "V", "Paste", ""},      // OS translates the modifier
		{[]string{"Ctrl"}, "X", "Cut", ""},        // OS translates the modifier
		{[]string{"Ctrl"}, "A", "Select All", ""}, // OS translates the modifier
		{[]string{}, "F2", "Rename", ""},          // no filesystem.rename capability yet
		{[]string{"Win"}, "E", "Open Finder", "app.open"},
		{[]string{"Win"}, "Left", "Left Half", "window.move"},
		{[]string{"Win"}, "Right", "Right Half", "window.move"},
		{[]string{"Alt"}, "Tab", "App Switcher", ""}, // no app-switching capability yet
		{[]string{"Shift"}, "Delete", "Move to Trash", "file.moveToTrash"},
	}
}
