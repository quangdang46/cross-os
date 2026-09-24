// Switcher page: the window switcher the user summons with a chord.
//
// The page is a live view of the switcher's own list, not a settings form: the
// daemon decides the order and the highlight, and the page draws the answer
// from one source rather than keeping a second ordering here. That is why the
// control is a panel over core:windows and not a list the shell can sort.
//
// The layout is one row per window, each named in words beside its own tile
// rather than by its id — the same rule the snap-area list follows (see
// third_party/rectangle/ATTRIBUTION.md, block 3); the row carries the window
// title the daemon sent, and a machine with no title falls back to the app so
// no row is identified by a key alone.
//
// The two actions are the daemon's own method names because an action id is
// the permission token it checks. core.switcherFocus is the row write (the
// same focus a release of the chord performs, so a click does not have to
// synthesize a keystroke) and core.switcherWait is the bounded poll that
// says the switcher was summoned. A wait that runs out of budget answers
// triggered=false rather than failing, so a page that treated it as an error
// would show a broken switcher on an idle machine.
package shell

import (
	"crossos/core/pkg/pluginapi"
)

// SwitcherPage returns the core.switcher settings-page contribution. It sits
// in the shortcuts group between the Windows page and the Shortcuts page: a
// window the switcher can raise is a shortcut the user reaches by key, not a
// setting in its own right — the same reason the Explorer page sits there.
func SwitcherPage() pluginapi.UIContribution {
	return contrib("core.switcher", "Switcher", nav{group: "shortcuts", order: 35}, map[string]any{
		"type":        "page",
		"description": "The windows the switcher can raise, in the order it will raise them. The highlighted row is the one the chord lands on.",
		"controls": []any{
			map[string]any{"kind": "switcherPanel", "id": "windows", "label": "Open windows", "source": "core:windows", "rowAction": "core.switcherFocus", "actions": []string{"core.switcherWait"}, "note": "One row per window, named by its title. Clicking a row raises it, which is what letting go of the chord does."},
		},
	}, []string{"core.switcherFocus", "core.switcherWait"}, "true")
}
