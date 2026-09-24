// Explorer page: the Finder right-click menu and the file types it offers
// (bead cross-os-vbl.6, §7.2 MVP 2; spec §8).
//
// "Explorer" is the user-facing name the product uses; "Finder" stays the
// technical term underneath — the page id stays core.finder and the sources
// keep the core:finder* names, so the rename is title-only. It sits in the
// shortcuts group because a Finder menu item is a shortcut the user reaches
// through a context menu, not a setting in its own right.
//
// §8 asks for a library here rather than a blank "New File" form: the person
// enables a capability that is already written. So both controls list rows
// the daemon owns, and the writes are the three the finder data source
// serves — a row's enabled flag, a catalog row, and the catalog's order. An
// action id is the permission token the daemon checks, so the spellings here
// are the daemon's own (core.setMenuItemEnabled, core.setFileType,
// core.reorderFileTypes) and a page may not invent a fourth it cannot run.
//
// Boundary: this page shows the menu CrossOS puts in Finder; generic
// extension install/enable lifecycle is nir.7/jpr.1, NOT duplicated here.
// Edits write through Config Manager validation (§3.8).
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
		"description": "The right-click menu in Finder, and the file types it offers. Enable an item and Finder updates without a restart.",
		"boundary":    "The Finder menu only — generic extension lifecycle lives on the Extensions page.",
		"controls": []any{
			map[string]any{"kind": "menuList", "id": "menu", "source": "core:finderMenu", "rowActions": []string{"core.setMenuItemEnabled"}, "note": "One row per menu item, ready to enable."},
			map[string]any{"kind": "fileTypeList", "id": "fileTypes", "source": "core:fileTypes", "rowActions": []string{"core.setFileType"}, "actions": []string{"core.reorderFileTypes"}, "note": "The types Finder's New menu offers — create one without writing a file first."},
		},
	}, []string{"core.setMenuItemEnabled", "core.setFileType", "core.reorderFileTypes"}, "true")
}
