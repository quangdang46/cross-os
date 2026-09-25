// Onboarding flow: welcome → per-extension Enable → System Settings
// guidance → readiness verification (bead cross-os-qhp.2, in-app half).
//
// Plan §10 Phase 5 + §1 non-tech-first. Reuses the nir.4 permission flow
// shapes (enableFlow steps) — no parallel Enable path. Links to the ymh.4
// About page for credits (no second copy). Trial confirm/rollback lives on
// the ymh.3 Safety surface. Copy: no jargon (no CGEventTap/AXUIElement/LL).
//
// Order 0 is shared with core.home and the schema's firstRun flag is the
// tiebreak, which is what puts this page first on a fresh profile and stops
// putting it first the moment onboarding is done. The tie is what forces the
// group: a page that leads one ordering and trails the next cannot be
// contiguity-safe in a group of its own, and the wizard is the head of the
// Home group either way — it is where someone lands, before and after.
package shell

import (
	"crossos/core/pkg/pluginapi"
)

// OnboardingFlow is the in-app first-run contribution: a settings-page that
// hosts the 4-step flow and the readiness checklist.
func OnboardingFlow() pluginapi.UIContribution {
	return contrib("core.onboarding", "Welcome", nav{group: "home", symbol: "⌂"}, map[string]any{
		"type":        "page",
		"description": "Get CrossOS working in four steps.",
		"firstRun":    true,
		"controls": []any{
			map[string]any{"kind": "enableFlow", "id": "onboard", "label": "Welcome to CrossOS", "steps": []string{"Welcome", "Enable per plugin", "Open System Settings", "Verify ready"}, "aboutLink": "core.about", "trialLink": "core.safety"},
			map[string]any{"kind": "checklist", "id": "readiness", "source": "core:readiness", "items": []string{"keyboard", "windows", "finder"}, "docsLink": "docs/install.md"},
		},
	}, []string{"plugin.enable", "permissions.openSettings", "permissions.verify"}, "firstRun")
}
