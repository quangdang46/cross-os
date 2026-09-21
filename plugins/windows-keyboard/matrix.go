package windowskeyboard

import (
	"encoding/json"

	"crossos/core/pkg/event"
	"crossos/core/pkg/intent"
	"crossos/core/pkg/rule"
)

// Virtual keys (Windows VK; matches core/pkg/keyboard's convention).
const (
	vkC     = 0x43
	vkLeft  = 0x25
	vkUp    = 0x26
	vkRight = 0x27
	vkDown  = 0x28
	vkF4    = 0x73
)

// Modifier bits. Mirrors core/pkg/keyboard.Mod* — kept as literals (not an
// import) because this is a separate Go module; if keyboard renumbers, both
// must change together. (review: cross-os-c0)
const (
	modCtrl = 1 << 0
	modAlt  = 1 << 2
	modMeta = 1 << 3 // Win key
)

// pluginID is this plugin's manifest ID, used to namespace rule ownership.
const pluginID = "windows-keyboard"

// zonesEnabled/native are the AppModes CrossOS acts in. Remote/VM/Excluded
// always fall through untouched (no rule claims them) — that is how the
// intent-first matrix delivers "remote/vm full passthrough" without special
// casing: simply never register a rule for those modes.
var activeModes = []event.AppMode{event.AppModeNative, event.AppModeTerminal}

// copyOnlyModes intentionally excludes Terminal: physical Ctrl+C in a
// terminal must interrupt (SIGINT), not copy — so no rule claims it there
// and the original keystroke passes through unchanged. This is the
// mechanism, not a special case: same router, same "no match → Pass" rule.
var copyOnlyModes = []event.AppMode{event.AppModeNative}

func mkIntent(id, params string) intent.Intent {
	var raw json.RawMessage
	if params != "" {
		raw = json.RawMessage(params)
	}
	return intent.Intent{ID: id, Version: 1, Source: intent.SourceKeyboard, Parameters: raw}
}

// Matrix is the intent-first shortcut matrix (§6.1 first slice, DATA ONLY
// re-expression per §9.11 — see doc.go provenance note).
//
// Coverage note (bead criterion: "remaining §6.1 rows tracked as follow-up
// scope inside this plugin, no new bead"): Ctrl+V/X/A, F2 (rename), and
// Alt+Tab (app switch) are deferred — they either have no CrossOS
// capability yet (rename, app switching) or are automatic Ctrl→Cmd
// modifier translation the OS handles without CrossOS mediation (paste/
// cut/select-all inside a focused control). Only rows requiring a CrossOS
// capability (window management, clipboard) are wired here.
func Matrix() []event.CompiledRule {
	return []event.CompiledRule{
		{
			// Alt+F4 → CLOSE_WINDOW. Never Cmd+Q: the Adapter closes the
			// focused window, it does not quit the app. Applies in
			// Terminal too (still a native window); VM/Remote pass
			// through untouched (row: "Alt+F4 on VM → pass through").
			KeyCode: vkF4, Modifiers: modAlt,
			AppModes: activeModes,
			RuleID:   pluginID + ".alt-f4-close-window", PluginID: pluginID,
			Priority: rule.PriorityGlobal, Specificity: 1, Scope: rule.ScopeGlobal,
			Intent: mkIntent("window.close", ""), Emit: true,
		},
		{
			// Ctrl+C → COPY (clipboard.copy = selection copy via the native
			// copy path — NOT clipboard.copyPath, which copies file paths.
			// Review correction: cross-os-c0). Native-mode only: Terminal
			// is deliberately excluded so physical Ctrl+C passes through
			// as INTERRUPT (bead criterion). Remote/VM never match.
			KeyCode: vkC, Modifiers: modCtrl,
			AppModes: copyOnlyModes,
			RuleID:   pluginID + ".ctrl-c-copy", PluginID: pluginID,
			Priority: rule.PriorityApp, Specificity: 1, Scope: rule.ScopeApp,
			Intent: mkIntent("clipboard.copy", ""), Emit: true,
		},
		{
			// Win+Left → SNAP left half.
			KeyCode: vkLeft, Modifiers: modMeta,
			AppModes: activeModes,
			RuleID:   pluginID + ".win-left-snap", PluginID: pluginID,
			Priority: rule.PriorityGlobal, Specificity: 1, Scope: rule.ScopeGlobal,
			Intent: mkIntent("window.move", `{"zone":"left-half"}`), Emit: true,
		},
		{
			// Win+Right → SNAP right half.
			KeyCode: vkRight, Modifiers: modMeta,
			AppModes: activeModes,
			RuleID:   pluginID + ".win-right-snap", PluginID: pluginID,
			Priority: rule.PriorityGlobal, Specificity: 1, Scope: rule.ScopeGlobal,
			Intent: mkIntent("window.move", `{"zone":"right-half"}`), Emit: true,
		},
		{
			// Win+Up → maximize.
			KeyCode: vkUp, Modifiers: modMeta,
			AppModes: activeModes,
			RuleID:   pluginID + ".win-up-maximize", PluginID: pluginID,
			Priority: rule.PriorityGlobal, Specificity: 1, Scope: rule.ScopeGlobal,
			Intent: mkIntent("window.maximize", ""), Emit: true,
		},
		{
			// Win+Down → minimize.
			KeyCode: vkDown, Modifiers: modMeta,
			AppModes: activeModes,
			RuleID:   pluginID + ".win-down-minimize", PluginID: pluginID,
			Priority: rule.PriorityGlobal, Specificity: 1, Scope: rule.ScopeGlobal,
			Intent: mkIntent("window.minimize", ""), Emit: true,
		},
	}
}

// Grants is the least-privilege permission set this plugin needs, matching
// Manifest().Permissions — the router's Authorize call is fail-closed
// against exactly this set.
func Grants() []intent.Permission {
	return []intent.Permission{intent.PermInputIntercept, intent.PermAccessControl}
}

// Priority note: all six rules share Specificity 1 with global-or-app
// Priority — fine single-plugin, but the first cross-plugin overlap (e.g.
// Vim UX vs this matrix on Ctrl+C) tie-breaks on RuleID alphabetically.
// Provisional until a second claimant exists. (review: cross-os-c0)
