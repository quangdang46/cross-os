// Explorer page: per-pack action list with enable/disable/reorder + action
// settings (bead cross-os-vbl.6, §7.2 MVP 2).
//
// "Explorer" is the user-facing name the product uses; "Finder" stays the
// technical term underneath — the page id, the core:finderPacks source and
// the pack.* capabilities are unchanged, so the rename is title-only. It
// sits in the shortcuts group because a Finder menu item is a shortcut the
// user reaches through a context menu, not a setting in its own right.
//
// Boundary: this page manages Finder extpacks (manifest actions); generic
// extension install/enable lifecycle is nir.7/jpr.1, NOT duplicated here.
// Edits write through Config Manager validation (§3.8); UTI/targets
// filters shown read-only; shell/process actions badge their Level B
// gating state; native actions show the capability name.
package shell

import (
	"crossos/core/pkg/pluginapi"
)

// FinderPage returns the core.finder settings-page contribution, titled
// "Explorer" — the constructor keeps its name because the page is still a
// Finder surface underneath.
func FinderPage() pluginapi.UIContribution {
	return contrib("core.finder", "Explorer", nav{group: "shortcuts", order: 50}, map[string]any{
		"type":        "page",
		"description": "Finder menu items from extension packs. Disable an action and the menu updates without restart.",
		"boundary":    "Finder extpacks only — generic extension lifecycle lives on the Extensions page.",
		"controls": []any{
			map[string]any{"kind": "packList", "id": "packs", "source": "core:finderPacks", "rowActions": []string{"pack.enableAction", "pack.disableAction", "pack.reorderAction", "pack.installLocal", "pack.remove"}},
			map[string]any{"kind": "actionSettings", "id": "actionSettings", "source": "core:packAction", "fields": []string{"placement", "variants", "timeoutSeconds"}, "readOnly": []string{"targets", "utis"}, "note": "Edits validate via Config Manager (§3.8)."},
			map[string]any{"kind": "gateBadge", "id": "levelB", "source": "core:packActionGate", "note": "Shell actions show Level B gating; native actions show the capability name."},
		},
	}, []string{"pack.enableAction", "pack.disableAction", "pack.reorderAction", "pack.installLocal", "pack.remove"}, "true")
}
