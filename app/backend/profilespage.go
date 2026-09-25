// Profiles page: what the spec calls the product and the architecture calls a
// plugin set (spec §2 — "Plugin is implementation. Profile is product.").
//
// A profile bundles the capabilities someone actually wants (Windows-style
// keys, snap zones, Explorer actions) so turning them on is one choice
// instead of a row of toggles, and undoing it is one choice back. The
// per-capability rollup rides on each card, so the page answers whether the
// click landed without the person having to go and look.
//
// THE BOUNDARY, stated once here and cited by the other two places that used
// to state it wrongly (the note this page RENDERS at the bottom of ProfilesPage
// below, and ProfileListControl.tsx's header comment):
//
//	Per-capability switches DO exist here, and they are the profile's own.
//
// A bundle is a set of capabilities, so "apply the whole thing" and "apply two
// of the five" are the same gesture pointed at different sizes of the same
// question. The switches narrow what the one Apply does; they are not a second
// copy of the Keyboard, Windows and Explorer pages, and the page does not
// render an editor for a rule, a zone, or a chord. Those pages still own the
// individual verdict for a single shortcut — the boundary this page keeps is
// "a profile is chosen whole or in capabilities, and a single shortcut is
// edited on the page that owns it", not "there are no switches on this page".
//
// The three statements that contradicted this were the boundary comment above
// ("Individual switches are NOT duplicated here"), the rendered note, and the
// control's header. All three are changed in this change; leaving the rendered
// note would have been the worst of them, because the user would have read a
// sentence denying the switches they were looking at.
package shell

import (
	"crossos/core/pkg/pluginapi"
)

// ProfilesPage returns the core.profiles settings-page contribution.
func ProfilesPage() pluginapi.UIContribution {
	return contrib("core.profiles", "Profiles", nav{group: "home", order: 10, symbol: "⌂"}, map[string]any{
		"type":        "page",
		"description": "Start from a setup rather than a list of switches. A profile turns on everything one kind of computer wants.",
		"controls": []any{
			map[string]any{
				"kind": "profileList", "id": "profiles", "source": "core:profiles",
				"applyAction": "core:profileApply", "revertAction": "core:profileDeactivate",
				"note": "Applying a profile enables its capabilities in one step and leaves the rest alone. Open a card to switch individual capabilities off, to see what applying will change, and to put it back.",
			},
			map[string]any{
				"kind": "note", "id": "profileNote",
				// The rendered boundary. It says what the switches DO, because
				// the old sentence said what they were not, and the switches
				// were on the page in front of the reader. What the profile
				// pages still do not do is edit a single shortcut: that is
				// the Keyboard, Windows and Explorer pages' own boundary, and
				// saying so is useful rather than a refusal.
				"text": "A profile is a bundle, not a plugin. Its capabilities have switches here; a single shortcut is still edited on the Keyboard, Windows and Explorer pages.",
			},
		},
	}, []string{"profile.apply", "profile.deactivate"}, "true")
}
