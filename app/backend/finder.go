// Finder page: per-pack action list with enable/disable/reorder + action
// settings (bead cross-os-vbl.6, §7.2 MVP 2).
//
// Boundary: this page manages Finder extpacks (manifest actions); generic
// plugin install/enable lifecycle is nir.7/jpr.1, NOT duplicated here.
// Edits write through Config Manager validation (§3.8); UTI/targets
// filters shown read-only; shell/process actions badge their Level B
// gating state; native actions show the capability name.
package shell

import (
	"crossos/core/pkg/pluginapi"
)

// FinderPage returns the core.finder settings-page contribution.
func FinderPage() pluginapi.UIContribution {
	return contrib("core.finder", "Finder", map[string]any{
		"type":        "page",
		"description": "Finder menu items from extension packs. Disable an action and the menu updates without restart.",
		"boundary":    "Finder extpacks only — generic plugin lifecycle lives on the Plugins page.",
		"controls": []any{
			map[string]any{"kind": "packList", "id": "packs", "source": "core:finderPacks", "rowActions": []string{"pack.enableAction", "pack.disableAction", "pack.reorderAction", "pack.installLocal", "pack.remove"}},
			map[string]any{"kind": "actionSettings", "id": "actionSettings", "source": "core:packAction", "fields": []string{"placement", "variants", "timeoutSeconds"}, "readOnly": []string{"targets", "utis"}, "note": "Edits validate via Config Manager (§3.8)."},
			map[string]any{"kind": "gateBadge", "id": "levelB", "source": "core:packActionGate", "note": "Shell actions show Level B gating; native actions show the capability name."},
		},
	}, []string{"pack.enableAction", "pack.disableAction", "pack.reorderAction", "pack.installLocal", "pack.remove"}, "true")
}
