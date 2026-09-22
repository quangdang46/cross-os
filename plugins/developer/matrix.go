package developer

import (
	"encoding/json"

	"crossos/core/pkg/event"
	"crossos/core/pkg/intent"
	"crossos/core/pkg/rule"
)

// Virtual keys + modifiers (same convention as windows-keyboard: separate Go
// module, literals kept in sync by test, not import).
const (
	vkReturn = 0x0D
	vkC      = 0x43
	vkP      = 0x50
	vkF8     = 0x77

	modCtrl  = 1 << 0
	modShift = 1 << 1
)

const pluginID = "developer"

// SupportedTerminals is the CONFIG-DRIVEN terminal list (§6.4): Ghostty,
// WezTerm, Warp + system Terminal. Adding one is config, not code — the
// rules below reference capabilities, never terminal bundle IDs.
var SupportedTerminals = []string{"Ghostty", "WezTerm", "Warp", "Terminal"}

// SupportedEditors is the CONFIG-DRIVEN editor list: VSCode, Cursor.
// PreferredEditor selects between them (config key, default VSCode).
var SupportedEditors = []string{"VSCode", "Cursor"}

// PreferredEditor is the configured default (config key
// developer.preferredEditor; tests override the value, never the list).
var PreferredEditor = "VSCode"

// IDEAppIDs scopes IDE-specific rules so they never collide with the
// windows-keyboard global matrix (§6.1 AppMode routing + §3.5b).
var IDEAppIDs = []string{
	"com.microsoft.VSCode", "com.todesktop.230313mzl4w4u92", // VSCode, Cursor
	"com.apple.Terminal", "com.mitchellh.ghostty", "com.github.wez.wezterm",
}

// FinderAppIDs scopes Finder-context rules.
var FinderAppIDs = []string{"com.apple.Finder"}

// NeedsShellApproval reports whether capability id requires Level B explicit
// user approve (shell/process execution — never default).
func NeedsShellApproval(capability string) bool {
	return capability == "shell.execute"
}

func mkIntent(id, params string) intent.Intent {
	var raw json.RawMessage
	if params != "" {
		raw = json.RawMessage(params)
	}
	return intent.Intent{ID: id, Version: 1, Source: intent.SourceKeyboard, Parameters: raw}
}

// Matrix is the intent-first rule set (§6.4):
//
//	Ctrl+Shift+Enter → terminal.openAt (Finder + IDE scoped, app-specific).
//	Ctrl+Shift+C    → clipboard.copyPath.
//	Ctrl+Shift+P    → app.open (preferred editor via params).
//
// F8 is DELIBERATELY unclaimed (next-error passthrough — pinned by test).
func Matrix() []event.CompiledRule {
	return []event.CompiledRule{
		{
			KeyCode: vkReturn, Modifiers: modCtrl | modShift,
			AppModes: []event.AppMode{event.AppModeNative}, AppIDs: append(append([]string{}, FinderAppIDs...), IDEAppIDs...),
			RuleID: pluginID + ".ctrl-shift-enter-terminal", PluginID: pluginID,
			Priority: rule.PriorityApp, Specificity: 2, Scope: rule.ScopeApp,
			Intent: mkIntent("terminal.openAt", `{"path":"{finderDir}"}`), Emit: true,
		},
		{
			KeyCode: vkC, Modifiers: modCtrl | modShift,
			AppModes: []event.AppMode{event.AppModeNative}, AppIDs: append(append([]string{}, FinderAppIDs...), IDEAppIDs...),
			RuleID: pluginID + ".ctrl-shift-c-copypath", PluginID: pluginID,
			Priority: rule.PriorityApp, Specificity: 2, Scope: rule.ScopeApp,
			Intent: mkIntent("clipboard.copyPath", ""), Emit: true,
		},
		{
			KeyCode: vkP, Modifiers: modCtrl | modShift,
			AppModes: []event.AppMode{event.AppModeNative}, AppIDs: append(append([]string{}, FinderAppIDs...), IDEAppIDs...),
			RuleID: pluginID + ".ctrl-shift-p-editor", PluginID: pluginID,
			Priority: rule.PriorityApp, Specificity: 2, Scope: rule.ScopeApp,
			Intent: mkIntent("app.open", `{"target":"{preferred}"}`), Emit: true,
		},
	}
}

// Grants is the least-privilege set, matching Manifest().Permissions.
func Grants() []intent.Permission {
	return []intent.Permission{intent.PermAccessControl, intent.PermFilesystem, intent.PermShellExecution}
}

// FinderItems declares the three Finder context rows (§6.4) as capability
// rows (title → capability + params shape). Displayed through the vbl.2
// menu machinery; Level B rows carry the gate flag.
type FinderItem struct {
	Title        string
	Capability   string
	Params       map[string]string
	NeedsApprove bool // Level B gate
}

// FinderItems lists Open in Terminal / VS Code / Cursor.
func FinderItems() []FinderItem {
	return []FinderItem{
		{Title: "Open in Terminal", Capability: "terminal.openAt", Params: map[string]string{"terminal": "{default}"}},
		{Title: "Open in VS Code", Capability: "app.open", Params: map[string]string{"editor": "VSCode"}},
		{Title: "Open in Cursor", Capability: "app.open", Params: map[string]string{"editor": "Cursor"}},
	}
}
