// Package rules is the builtin rule table: the three builtin plugins'
// intent-first matrices, compiled INTO core (bead cross-os-4gb follow-up:
// full matrix wiring).
//
// Module-boundary reason: plugins/* are separate Go modules that import
// core — core cannot import them back (import cycle). The rule rows here
// are DATA (keycode/modifier/mode/intent rows), ported 1:1 from the plugin
// matrices, pinned by TestBuiltinParity below. Behavior changes land in the
// plugin matrix first, then port here with the parity test updated — never
// the reverse.
//
// Sources:
//   - windows-keyboard Matrix() (plugins/windows-keyboard/matrix.go): 6 rules.
//   - developer Matrix() (plugins/developer/matrix.go): 3 rules.
//   - launcher HotkeyRule() (plugins/launcher/launcher.go): 1 rule (macOS
//     keycode; the Windows variant is a jpr.7 follow-up in the plugin).
package rules

import (
	"encoding/json"

	"crossos/core/pkg/event"
	"crossos/core/pkg/intent"
	"crossos/core/pkg/rule"
)

// Virtual keys + modifiers mirror the plugin matrices (separate modules keep
// literals in sync by test, not import — same convention as the plugins).
const (
	vkC      = 0x43
	vkP      = 0x50
	vkLeft   = 0x25
	vkUp     = 0x26
	vkRight  = 0x27
	vkDown   = 0x28
	vkF4     = 0x73
	vkSpace  = 0x20 // Windows VK_SPACE; the macOS kVK_Space (0x31) is translated at the tap boundary (cross-os-uok)
	vkReturn = 0x0D
	vkF8     = 0x77

	modCtrl  = 1 << 0
	modShift = 1 << 1
	modAlt   = 1 << 2
	modMeta  = 1 << 3 // Win key
)

func mkIntent(id, params string) intent.Intent {
	var raw json.RawMessage
	if params != "" {
		raw = json.RawMessage(params)
	}
	return intent.Intent{ID: id, Version: 1, Source: intent.SourceKeyboard, Parameters: raw}
}

func mkRule(key, mods uint32, modes []event.AppMode, appIDs []string, ruleID, pluginID string, prio, spec int, scope rule.Scope, in intent.Intent, emit bool) event.CompiledRule {
	return event.CompiledRule{
		KeyCode: key, Modifiers: mods,
		AppModes: modes, AppIDs: appIDs,
		RuleID: ruleID, PluginID: pluginID,
		Priority: prio, Specificity: spec, Scope: scope,
		Intent: in, Emit: emit,
	}
}

var nativeOnly = []event.AppMode{event.AppModeNative}
var nativeTerminal = []event.AppMode{event.AppModeNative, event.AppModeTerminal}

// windows-keyboard Matrix() port (6 rules): Alt+F4 close, Ctrl+C copy,
// Win+arrows snap/maximize/minimize. Terminal excluded from copy-only so
// physical Ctrl+C interrupts (SIGINT).
func keyboardRules() []event.CompiledRule {
	return []event.CompiledRule{
		mkRule(vkF4, modAlt, nativeTerminal, nil,
			"windows-keyboard.alt-f4-close-window", "windows-keyboard",
			rule.PriorityGlobal, 1, rule.ScopeGlobal,
			mkIntent("window.close", ""), true),
		mkRule(vkC, modCtrl, nativeOnly, nil,
			"windows-keyboard.ctrl-c-copy", "windows-keyboard",
			rule.PriorityApp, 1, rule.ScopeApp,
			mkIntent("clipboard.copy", ""), true),
		mkRule(vkLeft, modMeta, nativeTerminal, nil,
			"windows-keyboard.win-left-snap", "windows-keyboard",
			rule.PriorityGlobal, 1, rule.ScopeGlobal,
			mkIntent("window.move", `{"zone":"left-half"}`), true),
		mkRule(vkRight, modMeta, nativeTerminal, nil,
			"windows-keyboard.win-right-snap", "windows-keyboard",
			rule.PriorityGlobal, 1, rule.ScopeGlobal,
			mkIntent("window.move", `{"zone":"right-half"}`), true),
		mkRule(vkUp, modMeta, nativeTerminal, nil,
			"windows-keyboard.win-up-maximize", "windows-keyboard",
			rule.PriorityGlobal, 1, rule.ScopeGlobal,
			mkIntent("window.maximize", ""), true),
		mkRule(vkDown, modMeta, nativeTerminal, nil,
			"windows-keyboard.win-down-minimize", "windows-keyboard",
			rule.PriorityGlobal, 1, rule.ScopeGlobal,
			mkIntent("window.minimize", ""), true),
	}
}

// IDE + Finder app scopes for the developer matrix (ported literals).
var developerAppIDs = []string{
	"com.microsoft.VSCode", "com.todesktop.230313mzl4w4u92",
	"com.apple.Terminal", "com.mitchellh.ghostty", "com.github.wez.wezterm",
	"com.apple.Finder",
}

// developer Matrix() port (3 rules): terminal-open, copy-path, editor-open.
// F8 deliberately unclaimed (passthrough, pinned by plugin test).
func developerRules() []event.CompiledRule {
	return []event.CompiledRule{
		mkRule(vkReturn, modCtrl|modShift, nativeOnly, developerAppIDs,
			"developer.ctrl-shift-enter-terminal", "developer",
			rule.PriorityApp, 2, rule.ScopeApp,
			mkIntent("terminal.openAt", `{"path":"{finderDir}"}`), true),
		mkRule(vkC, modCtrl|modShift, nativeOnly, developerAppIDs,
			"developer.ctrl-shift-c-copypath", "developer",
			rule.PriorityApp, 2, rule.ScopeApp,
			mkIntent("clipboard.copyPath", ""), true),
		mkRule(vkP, modCtrl|modShift, nativeOnly, developerAppIDs,
			"developer.ctrl-shift-p-editor", "developer",
			rule.PriorityApp, 2, rule.ScopeApp,
			mkIntent("app.open", `{"target":"{preferred}"}`), true),
	}
}

// launcher HotkeyRule() port (1 rule): Ctrl+Space CONSUME.
func launcherRules() []event.CompiledRule {
	return []event.CompiledRule{
		mkRule(vkSpace, modCtrl, nativeOnly, nil,
			"launcher.ctrl-space-launcher", "launcher",
			rule.PriorityGlobal, 1, rule.ScopeGlobal,
			intent.Intent{ID: "launcher.open", Version: 1, Source: intent.SourceKeyboard}, false),
	}
}

// BuiltinIDs lists the builtin plugin IDs in registration order (matches
// the daemon's registerBuiltin order and the shell's plugin list).
var BuiltinIDs = []string{"windows-keyboard", "developer", "launcher"}

// All returns every builtin rule (6 + 3 + 1 = 10).
func All() []event.CompiledRule {
	out := keyboardRules()
	out = append(out, developerRules()...)
	out = append(out, launcherRules()...)
	return out
}

// Grants returns the least-privilege permissions per builtin plugin,
// mirroring each plugin's Grants() (router Authorize is fail-closed).
func Grants() map[string][]intent.Permission {
	return map[string][]intent.Permission{
		"windows-keyboard": {intent.PermInputIntercept, intent.PermAccessControl},
		"developer":        {intent.PermAccessControl, intent.PermFilesystem, intent.PermShellExecution},
		"launcher":         {intent.PermInputIntercept, intent.PermAccessControl},
	}
}
