// Onboarding flow: welcome → per-plugin Enable → System Settings
// guidance → readiness verification (bead cross-os-qhp.2, in-app half).
//
// Plan §10 Phase 5 + §1 non-tech-first. Reuses the nir.4 permission flow
// shapes (enableFlow steps) — no parallel Enable path. Links to the ymh.4
// About page for credits (no second copy). Trial confirm/rollback lives on
// the ymh.3 Safety surface. Copy: no jargon (no CGEventTap/AXUIElement/LL).
package shell

import (
	"crossos/core/pkg/pluginapi"
)

// OnboardingFlow is the in-app first-run contribution: a settings-page that
// hosts the 4-step flow and the readiness checklist.
func OnboardingFlow() pluginapi.UIContribution {
	return contrib("core.onboarding", "Welcome", map[string]any{
		"type":        "page",
		"description": "Get CrossOS working in four steps.",
		"firstRun":    true,
		"controls": []any{
			map[string]any{"kind": "enableFlow", "id": "onboard", "label": "Welcome to CrossOS", "steps": []string{"Welcome", "Enable per plugin", "Open System Settings", "Verify ready"}, "aboutLink": "core.about", "trialLink": "core.safety"},
			map[string]any{"kind": "checklist", "id": "readiness", "source": "core:readiness", "items": []string{"keyboard", "windows", "finder"}, "docsLink": "docs/install.md"},
		},
	}, []string{"plugin.enable", "permissions.openSettings", "permissions.verify"}, "firstRun")
}
