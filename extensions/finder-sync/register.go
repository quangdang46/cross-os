// RegisterMenu wiring: static MenuTable → pluginapi Registry (bead
// cross-os-vbl.2, §3.6 normative path).
//
// The static MenuTable is the source of truth today; vbl.3 (pack loader)
// replaces the SOURCE (packs → MenuDefs) without changing this path.
// This file must stay free of pack-loader imports: the day packs supply
// menus, the caller swaps MenuDefsForTable(MenuTable) for the pack-derived
// rows and this registration call is unchanged.
//
// (review: cross-os-ed — RegisterMenu had zero call sites; the criterion
// "Menus registered via RegisterMenu" now closes standalone.)
package findersync

import (
	"fmt"

	"crossos/core/pkg/pluginapi"
)

// MenuDefsForTable converts MenuTable rows to pluginapi.MenuDef values.
// One MenuDef per row; the row ID is namespaced so pack-supplied rows
// (vbl.3) cannot collide with the static table.
func MenuDefsForTable(table []MenuItem) []pluginapi.MenuDef {
	defs := make([]pluginapi.MenuDef, 0, len(table))
	for _, m := range table {
		defs = append(defs, pluginapi.MenuDef{
			ID:    "finder." + m.ID,
			Title: m.Title,
		})
	}
	return defs
}

// RegisterMenus registers every MenuTable row on r via RegisterMenu.
// It fails on the first registration error (never a partial silent set);
// duplicate registration is the Registry's typed error, surfaced here.
func RegisterMenus(r *pluginapi.Registry) error {
	if r == nil {
		return fmt.Errorf("findersync: nil Registry")
	}
	for _, d := range MenuDefsForTable(MenuTable) {
		if err := r.RegisterMenu(d); err != nil {
			return fmt.Errorf("findersync: RegisterMenu %q: %w", d.ID, err)
		}
	}
	return nil
}
