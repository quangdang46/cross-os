// UI pages: Core + plugin pages as UIContribution registrations
// (beads cross-os-ymh.3, cross-os-ymh.4, cross-os-nir.7, cross-os-nir.4,
// cross-os-10s, cross-os-nir.5, cross-os-jpr.2, cross-os-jpr.6).
//
// Plan: COMPREHENSIVE_PLAN.md §7.2 (every page) + §3.6c (contribution
// discovery, never hardcoded page lists). Every page here is a constructor
// returning pluginapi.UIContribution values registered through the SAME
// Registry path plugins use — the Host discovers them, the shell renders
// from schema. No page reaches the shell any other way.
//
// NAV ORDER. The sidebar follows the product's information architecture —
// Home, Profiles, Keyboard, Windows, Explorer, Activity, Advanced — so each
// page declares the group it renders under and an explicit Order. Order is
// global rather than per-group because the Host sorts one flat list and the
// group is nav furniture, not a sort key; two pages that share a group take
// consecutive Orders.
//
// The first-run page is the one Order that deliberately ties: it shares 0
// with Home, and the firstRun flag breaks the tie until onboarding is done.
// See Host.sort for why the flag stops counting.
//
// Cross-bead contracts honored:
//   - Trial/countdown durations reference safety.TRIALTimeout (the Core
//     constant from bead cross-os-2ha), never a local 30s literal — grep
//     "30 * time.Second" must find nothing new in app/.
//   - nir.4 links to the ymh.3 Safety surface for confirm/rollback (no
//     duplicate TRIAL UI); nir.7 links per-plugin config to owning pages
//     (10s matrix, jpr.2 schema forms); jpr.6 links to owning pages and
//     never duplicates the matrix.
//   - nir.7 carries no marketplace/network UI (hard gate, Phase 4).
//   - jpr.2 renders the §3.6c MVP declarative tier only
//     (checkbox/select/slider/button); custom-view is rejected upstream.
package shell

import (
	"encoding/json"

	"crossos/core/pkg/pluginapi"
)

// nav is where a page sits in the sidebar: the group it renders under and
// its position in the served order.
type nav struct {
	group string
	order int
}

// contrib is a small builder for settings-page contributions.
func contrib(id, title string, at nav, schema map[string]any, actions []string, visibility string) pluginapi.UIContribution {
	raw, _ := json.Marshal(schema)
	return pluginapi.UIContribution{
		ID:         id,
		Location:   pluginapi.UILocationSettingsPage,
		Title:      title,
		Group:      at.group,
		Order:      at.order,
		Schema:     raw,
		Visibility: visibility,
		Actions:    actions,
	}
}

// SafetyPage (ymh.3, §7.2 MVP 0): kill switch, Reset Everything, Safe Mode
// TRIAL countdown + confirm, ownership audit. Wired to 2ha Core semantics;
// actions flow through the Capability API like any caller.
func SafetyPage() pluginapi.UIContribution {
	return contrib("core.safety", "Safety", nav{group: "advanced", order: 100}, map[string]any{
		"type":             "page",
		"description":      "Stop everything instantly, reset CrossOS state, review what CrossOS changed.",
		"trialTimeoutNote": "Countdown uses the Core TRIAL timeout (safety.TRIALTimeout); the page never hardcodes it.",
		"controls": []any{
			map[string]any{"kind": "button", "id": "panicStop", "label": "PANIC STOP — disable interception + plugin actions", "action": "safety.panicStop", "note": "Login item stays. Reversible via Re-enable."},
			map[string]any{"kind": "button", "id": "resume", "label": "Re-enable interception", "action": "safety.resume", "note": "Clears a latched PANIC STOP and installs the tap again."},
			map[string]any{"kind": "button", "id": "reset", "label": "Reset Everything…", "action": "safety.reset", "confirm": "Remove login item, disable extension, clean CrossOS-owned state, verify no process remains?"},
			map[string]any{"kind": "trial", "id": "trialCountdown", "label": "New integration trial", "source": "core:trialCountdown", "actions": []string{"safety.confirmTrial", "safety.rollbackTrial"}},
			map[string]any{"kind": "auditList", "id": "ownership", "label": "What CrossOS created", "source": "core:ownershipAudit", "rowAction": "safety.rollback"},
		},
	}, []string{"safety.panicStop", "safety.resume", "safety.reset", "safety.confirmTrial", "safety.rollbackTrial", "safety.rollback"}, "true")
}

// AboutPage (ymh.4, §7.2 MVP 0): version, MIT license, repository credits
// generated from third_party ATTRIBUTION.md entries (never hand-maintained).
func AboutPage() pluginapi.UIContribution {
	return contrib("core.about", "About", nav{group: "advanced", order: 110}, map[string]any{
		"type":        "page",
		"description": "CrossOS version, license, and the repositories it builds on.",
		"controls": []any{
			map[string]any{"kind": "version", "id": "version", "source": "core:version"},
			map[string]any{"kind": "license", "id": "license", "source": "core:licenseMIT"},
			map[string]any{"kind": "credits", "id": "credits", "source": "core:attributionEntries", "note": "Generated from third_party/*/ATTRIBUTION.md"},
		},
	}, nil, "true")
}

// ExtensionsPage (nir.7, §7.2 MVP 1+ local only): the user-facing name for
// what the architecture still calls a plugin. "Extensions" is what the person
// installed; "Plugin" stays the technical term underneath — the source
// (core:plugins), the capability ids (plugin.*) and the Registry type are all
// unchanged, so renaming the surface costs the wire nothing.
//
// NO marketplace/network UI (hard gate — marketplace is cross-os-jpr.1).
//
// The row actions are the ones the shell can actually run. plugin.update and
// plugin.uninstall are declared by no daemon method (the plugin table is
// plugin.list + plugin.setEnabled) and were read by no renderer, so they are
// gone rather than left on the schema as a promise. installDisk keeps its
// button and its fail-closed command for the same reason it keeps its place.
func ExtensionsPage() pluginapi.UIContribution {
	return contrib("core.extensions", "Extensions", nav{group: "advanced", order: 80}, map[string]any{
		"type":          "page",
		"description":   "Extensions installed on this machine. New installs enter trial; confirm on the Safety page.",
		"noMarketplace": true,
		"controls": []any{
			map[string]any{"kind": "pluginList", "id": "plugins", "source": "core:plugins", "rowHealth": true, "rowActions": []string{"plugin.enable", "plugin.disable"}},
			map[string]any{"kind": "button", "id": "installDisk", "label": "Install from disk…", "action": "plugin.installDisk"},
			map[string]any{"kind": "note", "id": "trialNote", "text": "New installs enter trial (Core TRIAL timeout). Confirm or roll back on the Safety page."},
			map[string]any{"kind": "note", "id": "configNote", "text": "Per-extension settings live on owning pages: keyboard matrix, config schema forms."},
		},
	}, []string{"plugin.enable", "plugin.disable", "plugin.installDisk"}, "true")
}

// ActivityPage (nir.4, §7.2): permission-prompt flow (Welcome → Enable →
// Open System Settings → Verify) + real-time intent trace. Confirm/rollback
// links to the ymh.3 Safety surface; this page owns no TRIAL UI.
func ActivityPage() pluginapi.UIContribution {
	return contrib("core.activity", "Activity", nav{group: "activity", order: 60}, map[string]any{
		"type":        "page",
		"description": "What CrossOS did and why — every shortcut, rule, intent, and action.",
		"controls": []any{
			map[string]any{"kind": "enableFlow", "id": "enable", "label": "Enable CrossOS", "steps": []string{"Welcome", "Enable per plugin", "Open System Settings", "Verify ready"}, "note": "Nothing is enabled until you explicitly enable it."},
			map[string]any{"kind": "traceList", "id": "trace", "source": "core:eventLogs", "format": "Physical → Context → Rule → Intent → Action → result", "trialLink": "core.safety"},
		},
	}, []string{"plugin.enable", "permissions.openSettings", "permissions.verify"}, "true")
}

// ObservePage (spec §6): the event inspector, kept off Activity on purpose.
// The spec treats the conflict surface (§5) and the observe surface (§6) as
// two different questions — "which rule won" versus "what is the keyboard
// doing right now" — and folding observe into Activity grew a page that
// answered neither. This one is the second question alone.
//
// Observe is a dry run: the recorder writes a "would-execute" stage instead
// of acting, which is what makes it safe to leave on while someone is
// learning what their shortcuts do.
func ObservePage() pluginapi.UIContribution {
	return contrib("core.observe", "Observe", nav{group: "activity", order: 70}, map[string]any{
		"type":        "page",
		"description": "Watch what CrossOS sees. With Observe on, actions are shown instead of performed.",
		"controls": []any{
			// Not a `button`: this one's label is a fact about the recorder, so
			// it reads where observe mode stands and offers the other position
			// (cross-os-pbd). A button captioned "Turn Observe on" that is
			// already on tells the reader something false about their machine.
			map[string]any{"kind": "observeToggle", "id": "observeToggle", "label": "Observe", "note": "Dry run — CrossOS reports what it would do and changes nothing."},
			// The copy below used to call this "The live feed". It is not one:
			// there is no push, and this is the shell's five-second poll of a
			// 200-row tail (traceRowLimit, pagedata.go:1105; the interval is
			// App.tsx:273). A page that names itself live and is polled teaches
			// the reader to trust a freshness it does not have — on the one
			// surface whose whole job is telling them what their rules did.
			map[string]any{"kind": "traceList", "id": "events", "source": "core:traces", "format": "Key → App → Rule → Intent → Action", "note": "The last 200 decisions, newest first, re-read every few seconds — polled, not pushed. A decision that falls off the end is gone."},
		},
	}, []string{"core.setObserve"}, "true")
}

// MatrixPage (10s, §7.2 Keyboard MVP 0): behavior-matrix content editing +
// app overrides, owned by the s4i rules. Per-plugin config_schema forms are
// jpr.2 scope; the global Shortcuts registry is jpr.6 scope (read-mostly).
func MatrixPage() pluginapi.UIContribution {
	return contrib("core.keyboard", "Keyboard", nav{group: "shortcuts", order: 20}, map[string]any{
		"type":        "page",
		"description": "Which shortcuts do what, per app. Edits take effect immediately.",
		"controls": []any{
			map[string]any{"kind": "matrix", "id": "matrix", "source": "core:behaviorMatrix", "immediate": true, "note": "Disabling a shortcut takes effect without restart."},
			map[string]any{"kind": "overrides", "id": "overrides", "source": "core:appOverrides", "note": "App-specific overrides."},
		},
	}, []string{"config.writeMatrix", "config.writeOverride"}, "true")
}

// WindowsPage (nir.5, §7.2 MVP 1): §6.2 shortcut defaults (editable,
// persisted via Config Manager) + snap zone editor. Rectangle is
// BEHAVIOR-only precedent: zone geometry + defaults inform UX, no code.
func WindowsPage() pluginapi.UIContribution {
	return contrib("core.windows", "Windows", nav{group: "shortcuts", order: 30}, map[string]any{
		"type":        "page",
		"description": "Window shortcuts and snap zones. Conflicts resolve like any rule — winner + losers shown.",
		"controls": []any{
			map[string]any{"kind": "shortcutList", "id": "shortcuts", "source": "core:windowShortcuts", "editable": true, "conflicts": "core:conflicts"},
			map[string]any{"kind": "zoneEditor", "id": "zones", "source": "core:snapZones", "note": "Zone edits take effect through the capability path."},
		},
	}, []string{"config.writeShortcuts", "config.writeZones"}, "true")
}

// SchemaFormHelp (jpr.2): the JSON-schema → declarative-form renderer
// contract. The renderer supports the §3.6c MVP tier
// (checkbox/select/slider/button); schemas arrive via manifest
// config_schema, writes validate + propagate per §3.8 (never direct file
// writes from UI), actions permission-checked like any caller.
func SchemaFormHelp() pluginapi.UIContribution {
	return contrib("core.schemaHelp", "Plugin Settings", nav{group: "advanced", order: 90}, map[string]any{
		"type":        "page",
		"description": "Settings declared by extensions render here automatically.",
		"renderer":    map[string]any{"tier": "mvp", "widgets": []string{"checkbox", "select", "slider", "button"}, "acceptance": "New extension with config_schema shows working UI with zero Core UI changes."},
		"controls": []any{
			map[string]any{"kind": "schemaForm", "id": "pluginSettings", "source": "core:pluginSchemas", "note": "Each extension's config_schema renders here with MVP widgets (checkbox/select/slider/button)."},
		},
	}, nil, "true")
}

// CommandsPage (jpr.6): command-palette host + Shortcuts page (global list,
// per-shortcut enable/disable, conflict display with winner + reason).
// Per-rule content editing stays in owning pages (10s matrix) — linked,
// never duplicated.
func CommandsPage() pluginapi.UIContribution {
	return contrib("core.shortcuts", "Shortcuts", nav{group: "shortcuts", order: 40}, map[string]any{
		"type":        "page",
		"description": "Every shortcut in one place. Edit content on owning pages.",
		"controls": []any{
			map[string]any{"kind": "palette", "id": "palette", "source": "core:commands", "note": "Extension commands appear with zero Core UI changes."},
			map[string]any{"kind": "shortcutList", "id": "all", "source": "core:allShortcuts", "conflicts": "core:conflicts", "editLinks": "owner"},
		},
	}, []string{"command.execute", "shortcut.setEnabled"}, "true")
}

// CorePages returns every Core page contribution in nav order, including the
// first-run onboarding page (qhp.2) and the Explorer extpack page (vbl.6):
// both register through the same Registry path, so the shared gates (trial
// literal, custom-view, control kinds) cover them.
//
// The returned slice is the authored order for readability; the Host re-sorts
// on Group/Order/firstRun, and a test pins the served order so the two can
// never drift apart silently.
func CorePages() []pluginapi.UIContribution {
	return []pluginapi.UIContribution{
		HomePage(), ProfilesPage(),
		MatrixPage(), WindowsPage(), SwitcherPage(), CommandsPage(),
		FinderPage(),
		ActivityPage(), ObservePage(),
		ExtensionsPage(), SchemaFormHelp(), SafetyPage(), AboutPage(),
		OnboardingFlow(),
	}
}
