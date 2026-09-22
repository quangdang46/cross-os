// Finder page tests — bead cross-os-vbl.6 (page half).
//
// The FinderPage contribution is covered by the shared gates
// (TestAllPagesDiscovered count, TestNoTrialLiteral, TestSchemaRendererTier,
// TestPageControlKinds): this file pins the vbl.6-specific contracts —
// pack actions surface, Level B badging, boundary (no plugin-lifecycle
// duplication), Config-Manager-validated edits.
package shell

import (
	"strings"
	"testing"
)

func TestFinderPageContract(t *testing.T) {
	f := FinderPage()
	if f.ID != "core.finder" {
		t.Fatalf("id=%s, want core.finder", f.ID)
	}
	s := string(f.Schema)
	// Pack actions surface with row actions (enable/disable/reorder +
	// local install/remove — Git registry is Phase 4, not here).
	for _, want := range []string{"packList", "pack.enableAction", "pack.disableAction", "pack.reorderAction", "pack.installLocal", "pack.remove"} {
		if !strings.Contains(s, want) {
			t.Fatalf("finder schema missing %q", want)
		}
	}
	// Action settings: placement/variants/timeout editable via Config
	// Manager; targets/utis read-only.
	for _, want := range []string{"placement", "variants", "timeoutSeconds", "readOnly", "targets", "utis"} {
		if !strings.Contains(s, want) {
			t.Fatalf("finder schema missing settings field %q", want)
		}
	}
	// Level B badging + native capability names.
	if !strings.Contains(s, "gateBadge") {
		t.Fatal("finder schema missing Level B gateBadge")
	}
	// Boundary: no plugin-lifecycle duplication (nir.7 owns it).
	for _, banned := range []string{"plugin.installDisk", "plugin.update", "plugin.uninstall"} {
		if strings.Contains(s, banned) {
			t.Fatalf("finder schema duplicates plugin lifecycle %q (nir.7 owns it)", banned)
		}
	}
}
