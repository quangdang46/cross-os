// Package profiles is the profile layer: a named bundle of capabilities the
// user turns on as one product choice ("Windows 11 Experience"), rather than
// a list of plugin IDs. Plugins are the implementation, the profile is the
// product — the spec's §2 complaint, answered as data.
//
// A profile is DATA so a settings page renders it generically: the page
// reads a Profile and draws a row per capability, the same way pagedata.go
// reads the behavior matrix instead of hand-listing shortcuts. No page ID and
// no capability list lives here.
//
// Reference discipline, the reason this file reads the builtin tables rather
// than naming things twice:
//   - A plugin ID is builtin.BuiltinIDs[windowsKeyboard], not a typed
//     "windows-keyboard" — the same registration-order list the daemon seeds
//     from (main.go registerBuiltin). A typed string would be a second source
//     of truth for one ID.
//   - A window-action rule ID is winlayout's own zone name, and the layout
//     capability walks winlayout.AllActions wholesale, so the advertised
//     vocabulary cannot drift from the 21 actions that resolve.
//   - A behavior-matrix rule ID IS spelled out, because it is the one thing a
//     rename should break: TestBundleIDsResolve resolves each against
//     rules.All(), so renaming windows-keyboard.ctrl-c-copy fails this build
//     rather than leaving a bundle row pointing at nothing.
//
// Honesty: a capability CrossOS cannot deliver is declared unavailable with
// the reason, never as a passing check. pagedata.go:11 — "a fabricated row is
// worse than an honest gap" — is why Alt+Tab is a gap here. The reason is NOT
// that the switcher is missing, which is the story this file used to tell and
// the intent registry contradicts: window.switcher is registered and two rules
// drive it. The gap is that this bundle does not claim those rules, which the
// two row reasons below now say, and the two table tests now hold them to.
package profiles

import (
	"crossos/core/pkg/winlayout"
	builtin "crossos/core/rules"
)

// windowsKeyboard is builtin.BuiltinIDs' position of the windows-keyboard
// plugin. Referenced by position so the bundle cannot name a plugin the
// daemon does not register; the rule IDs a capability carries name the same
// plugin, and the resolution test below is what pins the two together.
const windowsKeyboard = 0

// windowsKeyboardPlugin is the windows-keyboard plugin ID, read from the
// registration-order table rather than typed — one deref so the data and the
// tests name the plugin the same way.
func windowsKeyboardPlugin() string { return builtin.BuiltinIDs[windowsKeyboard] }

// Capability is one line in a profile's bundle: a user-facing behavior
// (Label) mapped to the Plugin that provides it and the RuleIDs that
// implement it. A capability is available — a resolvable plugin and at least
// one resolvable rule — or explicitly unavailable, in which case Plugin and
// RuleIDs are empty and Reason says why.
type Capability struct {
	// ID is the stable capability identifier (the row key).
	ID string `json:"id"`
	// Label is the display name.
	Label string `json:"label"`
	// Plugin is the builtin plugin ID from rules.BuiltinIDs, or "" on an
	// unavailable capability.
	Plugin string `json:"plugin"`
	// RuleIDs are the behaviors behind the capability: a behavior-matrix
	// rule ID (rules.All()) or a window-action zone name
	// (winlayout.AllActions). Empty only on an unavailable capability.
	RuleIDs []string `json:"rule_ids"`
	// Available reports whether the profile delivers the capability today.
	Available bool `json:"available"`
	// Reason is why an unavailable capability is not here yet; empty when
	// the capability is available.
	Reason string `json:"reason,omitempty"`
}

// Profile is a curated, named bundle of capabilities the user picks as a
// single product choice. The builtin list starts with the Windows 11
// Experience the spec leads with.
type Profile struct {
	ID           string       `json:"id"`
	Label        string       `json:"label"`
	Description  string       `json:"description"`
	Capabilities []Capability `json:"capabilities"`
}

// zones renders window Action IDs by asking winlayout for each zone name.
// The names come from winlayout (which owns the vocabulary) rather than being
// typed here, so a renamed zone reaches the bundle without an edit and the
// resolution test is what holds each name to a real action.
func zones(actions ...winlayout.Action) []string {
	out := make([]string, 0, len(actions))
	for _, a := range actions {
		out = append(out, winlayout.ZoneName(a))
	}
	return out
}

// windows11 is the profile layer's one builtin: the Windows 11 Experience. Its
// two declared gaps (Alt+Tab, File Explorer extras) are capabilities the spec
// promises and this bundle does not yet carry — carried with their reasons
// rather than ticked, so a future release fills them by making the capability
// available, not by the UI re-drawing a checkmark.
//
// "Does not yet carry" is the load-bearing phrase, and it is deliberately not
// "does not yet exist": the rules behind both rows already exist. The gaps are
// in what this bundle CLAIMS, which is a smaller and more tractable hole than a
// missing capability, and it is why both row reasons are written from the rule
// and capability tables rather than from what was true when they were first
// typed.
var windows11 = Profile{
	ID:          "windows-11-experience",
	Label:       "Windows 11 Experience",
	Description: "Windows keyboard muscle memory on macOS: Windows shortcuts, Win+Arrow snapping, and the full window-action layout vocabulary. Alt+Tab and File Explorer extras are declared but not delivered yet.",
	Capabilities: []Capability{
		{
			ID:     "keyboard.shortcuts",
			Label:  "Windows Keyboard Shortcuts",
			Plugin: windowsKeyboardPlugin(),
			RuleIDs: []string{
				"windows-keyboard.alt-f4-close-window",
				"windows-keyboard.ctrl-c-copy",
			},
			Available: true,
		},
		{
			ID:     "window.snap",
			Label:  "Win+Arrow Snap",
			Plugin: windowsKeyboardPlugin(),
			RuleIDs: []string{
				"windows-keyboard.win-left-snap",
				"windows-keyboard.win-right-snap",
				"windows-keyboard.win-up-maximize",
				"windows-keyboard.win-down-minimize",
			},
			Available: true,
		},
		{
			ID:     "window.layouts",
			Label:  "Window Layouts",
			Plugin: windowsKeyboardPlugin(),
			// The whole §6.2 action vocabulary, read off AllActions: the
			// layout picker the Windows page edits cannot advertise fewer
			// actions than winlayout resolves.
			RuleIDs:   zones(winlayout.AllActions...),
			Available: true,
		},
		{
			ID:        "window.alt-tab",
			Label:     "Alt+Tab Window Switcher",
			Available: false,
			// The one string that was a falsehood and is now a fact. It used
			// to read "no window-switcher capability, plugin, or rule
			// exists yet", and all three of those DO exist: window.switcher
			// is in the intent registry (registry_v1.go:75), and
			// windows-keyboard.alt-tab-summon-switcher and
			// -commit-switcher are two rules behind it (rules.go:152,157)
			// with the switcher shipped in cmd/crossos/switcher.go. The
			// rules.go comment says so outright — "these rules are the
			// capability that row said it did not have yet."
			//
			// So the row stays unavailable, for the one reason that is
			// true: this bundle claims neither rule, so applying it does
			// not turn Alt+Tab on, and profiles.go:93-96 says a passing
			// row must not advertise a control that does nothing. The
			// missing thing is the CLAIM, not the capability. Adding the
			// two RuleIDs here is what flips this row, and
			// TestAltTabRowIsTrueOfTheTables is what says so.
			Reason: "the switcher ships — window.switcher is a registered capability and " +
				"windows-keyboard.alt-tab-summon-switcher and " +
				"windows-keyboard.alt-tab-commit-switcher are the two rules driving it — " +
				"but this bundle claims neither, so applying the profile leaves Alt+Tab off",
		},
		{
			ID:        "finder.explorer",
			Label:     "File Explorer Shortcuts",
			Available: false,
			// Also re-derived from the tables rather than from memory, and
			// it fixes the same kind of error one clause over. The old
			// string said F2 rename and Win+E "have no rule or shortcut
			// behind them", and the second half was false: BOTH are rows
			// in winlayout.Windows11Shortcuts(). What is missing is the
			// rule that FIRES them, and for F2 the intent those two rows
			// would carry.
			//
			// F2's preset row has an empty intent and the registry has no
			// filesystem.rename to fill it with; Win+E's preset row is
			// bound to app.open, and the only rule emitting app.open is
			// the developer plugin's Ctrl+Shift+P. file.moveToTrash is
			// real, registered and dispatched, and it is bound to
			// Shift+Delete — but it is the one Explorer verb this bundle
			// does not claim, which is the only part of the old sentence
			// that was right. TestFinderExplorerRowIsTrueOfTheTables
			// pins every clause.
			Reason: "F2 rename and Win+E are declared chords that no rule fires. " +
				"F2's intent is empty, because no filesystem.rename capability is " +
				"registered to fill it. Win+E's app.open intent is fired only by the " +
				"developer plugin's own chord. file.moveToTrash is the one Explorer " +
				"verb that does fire, on Shift+Delete, and this bundle does not claim it",
		},
	},
}

// All returns the builtin profiles (one today: Windows 11 Experience). A
// second profile arrives as data beside this one, not as a new code path, and
// the copy keeps a caller's edit off the declaration other pages read.
func All() []Profile {
	return []Profile{windows11}
}
