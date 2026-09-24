// Profiles page: what the spec calls the product and the architecture calls a
// plugin set (spec §2 — "Plugin is implementation. Profile is product.").
//
// A profile bundles the capabilities someone actually wants (Windows-style
// keys, snap zones, Explorer actions) so turning them on is one choice
// instead of a row of toggles, and undoing it is one choice back. The
// per-capability rollup rides on each card, so the page answers whether the
// click landed without the person having to go and look.
//
// Individual switches are NOT duplicated here: they stay on the pages that
// own them (Keyboard, Windows, Explorer), the same boundary the Shortcuts
// page keeps for matrix editing.
package shell

import (
	"crossos/core/pkg/pluginapi"
)

// ProfilesPage returns the core.profiles settings-page contribution.
func ProfilesPage() pluginapi.UIContribution {
	return contrib("core.profiles", "Profiles", nav{group: "home", order: 10}, map[string]any{
		"type":        "page",
		"description": "Start from a setup rather than a list of switches. A profile turns on everything one kind of computer wants.",
		"controls": []any{
			map[string]any{"kind": "profileList", "id": "profiles", "source": "core:profiles", "applyAction": "core:profileApply", "note": "Applying a profile enables its capabilities in one step and leaves the rest alone."},
			map[string]any{"kind": "note", "id": "profileNote", "text": "A profile is a bundle, not a plugin. Individual switches stay on Keyboard, Windows and Explorer."},
		},
	}, []string{"profile.apply"}, "true")
}
