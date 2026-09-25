// Home page: the landing card (spec §1 — after the wizard, the first thing a
// person sees is how CrossOS is doing, not a settings form).
//
// It assembles what the daemon already serves and nothing else: the status
// payload the shell keeps fresh, the readiness rollup, and the active
// profile. A landing card with a source of its own would be the first place
// the window could show two different answers to the same question — the
// card saying the tap is off while the masthead above it says running.
//
// Order 0 is shared with nothing else on purpose. core.onboarding also sits
// at 0 and carries firstRun, and the Host's tiebreak is what decides between
// them: the wizard leads until it is done, then Home does. See Host.sort.
package shell

import (
	"crossos/core/pkg/pluginapi"
)

// HomePage returns the core.home settings-page contribution.
func HomePage() pluginapi.UIContribution {
	return contrib("core.home", "Home", nav{group: "home", symbol: "⌂"}, map[string]any{
		"type":        "page",
		"description": "What CrossOS is doing right now — what is on, what is ready, and which profile is active.",
		"controls": []any{
			map[string]any{"kind": "statusCard", "id": "status", "profile": "core:profiles", "onboarding": "core:onboardingState", "note": "Running, interception, and the active profile in one card."},
			map[string]any{"kind": "checklist", "id": "readiness", "source": "core:readiness", "items": []string{"keyboard", "windows", "finder"}, "note": "Each line says what to do when it is not ready."},
		},
	}, nil, "true")
}
