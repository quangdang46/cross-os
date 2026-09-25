// Onboarding flow tests — bead cross-os-qhp.2 (in-app half).
//
// Lives in a _test file so `go test` actually runs it (a Test func in a
// non-test file compiles but never executes — review catch: cross-os-8d).
package shell

import (
	"strings"
	"testing"

	"crossos/core/pkg/pluginapi"
)

func TestOnboardingFlow(t *testing.T) {
	f := OnboardingFlow()
	if f.ID != "core.onboarding" {
		t.Fatalf("id=%s, want core.onboarding", f.ID)
	}
	s := string(f.Schema)
	for _, want := range []string{"Welcome", "Enable per plugin", "Open System Settings", "Verify ready", "core.about", "core.safety", "docs/install.md"} {
		if !strings.Contains(s, want) {
			t.Fatalf("onboarding schema missing %q", want)
		}
	}
	// No parallel Enable path: exactly one wizard control.
	if n := strings.Count(s, "wizard"); n != 1 {
		t.Fatalf("wizard count=%d, want 1 (reuse nir.4, no parallel path)", n)
	}
	// Non-tech copy: no jargon in user-facing strings.
	for _, banned := range []string{"CGEventTap", "AXUIElement", "CGEvent", "WH_KEYBOARD", "syscall"} {
		if strings.Contains(s, banned) {
			t.Fatalf("onboarding leaks jargon %q", banned)
		}
	}
	// Registers through the same Registry path (discovery, no hardcode).
	r := pluginapi.NewRegistry(nil)
	if err := r.RegisterUI(f); err != nil {
		t.Fatalf("RegisterUI: %v", err)
	}
	h := NewHost()
	if err := h.Register(r); err != nil {
		t.Fatalf("Host.Register: %v", err)
	}
	found := false
	for _, p := range h.Pages() {
		if p.ID == "core.onboarding" {
			found = true
		}
	}
	if !found {
		t.Fatal("onboarding page not discovered via Registry")
	}
}
