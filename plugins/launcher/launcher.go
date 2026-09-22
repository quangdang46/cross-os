package launcher

import (
	"encoding/json"
	"sort"
	"strings"

	"crossos/core/pkg/event"
	"crossos/core/pkg/intent"
	"crossos/core/pkg/rule"
)

// Hotkey: Ctrl+Space (distinct from Spotlight/Alfred defaults; user-editable
// via the Shortcuts page — this rule is the default registration, not a
// side-channel hook).
//
// Keycode note: 0x31 is macOS kVK_Space (Windows VK_SPACE is 0x20).
// TODO(jpr.4-followup): Windows keycode mapping (GOOS-branch or per-platform
// rule) when the launcher ships on Windows — currently macOS-first, and a
// hardcoded 0x31 on a Windows build would listen on the wrong key silently.
const (
	vkSpace  = 0x31
	modCtrl  = 1 << 0
	pluginID = "launcher"
)

// App is one discoverable application (re-expressed discovery shape: name +
// bundle ID + path — the FileManager-enumeration + running-apps merge from
// the STUDY base, without its SwiftUI chrome).
type App struct {
	Name     string
	BundleID string
	Path     string
}

// Rank scores query against app (exact > prefix > substring > fuzzy Bluff):
// 4/3/2/1, 0 = no match. Matches name + bundle ID (case-insensitive).
func Rank(query string, a App) int {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return 0
	}
	name := strings.ToLower(a.Name)
	id := strings.ToLower(a.BundleID)
	if q == name || q == id {
		return 4
	}
	if strings.HasPrefix(name, q) || strings.HasPrefix(id, q) {
		return 3
	}
	if strings.Contains(name, q) || strings.Contains(id, q) {
		return 2
	}
	if fuzzy(q, name) || fuzzy(q, id) {
		return 1
	}
	return 0
}

// fuzzy is subsequence matching (q runes appear in order in s).
// Rune-wise on both sides (byte indexing breaks on multi-byte names like
// "Café" — review: cross-os-8d).
func fuzzy(q, s string) bool {
	qr, sr := []rune(q), []rune(s)
	j := 0
	for _, c := range qr {
		found := false
		for ; j < len(sr); j++ {
			if sr[j] == c {
				found = true
				j++
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// Search ranks apps by query, highest first (stable: ties keep input order).
func Search(query string, apps []App) []App {
	type scored struct {
		app   App
		score int
		idx   int
	}
	var ss []scored
	for i, a := range apps {
		if s := Rank(query, a); s > 0 {
			ss = append(ss, scored{a, s, i})
		}
	}
	sort.SliceStable(ss, func(i, j int) bool {
		if ss[i].score != ss[j].score {
			return ss[i].score > ss[j].score
		}
		return ss[i].idx < ss[j].idx
	})
	out := make([]App, len(ss))
	for i, s := range ss {
		out[i] = s.app
	}
	return out
}

// LaunchIntent builds the app.launch request for app (Adapter executes).
func LaunchIntent(a App) intent.Intent {
	raw, _ := json.Marshal(map[string]string{"app": a.BundleID})
	return intent.Intent{ID: "app.launch", Version: 1, Source: intent.SourceKeyboard, Parameters: raw}
}

// HotkeyRule is the global-hotkey registration as a standard rule: Ctrl+Space
// → launcher.open intent → capability path with conflict resolution (§3.5b).
// The intent resolves to a shell-owned palette action (no capability needed
// to OPEN the palette; app.launch fires on selection).
func HotkeyRule() event.CompiledRule {
	return event.CompiledRule{
		KeyCode: vkSpace, Modifiers: modCtrl,
		AppModes:    []event.AppMode{event.AppModeNative},
		RuleID:      pluginID + ".ctrl-space-launcher",
		PluginID:    pluginID,
		Priority:    rule.PriorityGlobal,
		Specificity: 1, Scope: rule.ScopeGlobal,
		Intent: intent.Intent{ID: "launcher.open", Version: 1, Source: intent.SourceKeyboard},
		Emit:   false, // CONSUME: palette opens, nothing dispatched yet
	}
}

// Grants is the least-privilege set, matching Manifest().Permissions.
func Grants() []intent.Permission {
	return []intent.Permission{intent.PermInputIntercept, intent.PermAccessControl}
}
