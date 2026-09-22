// Page contribution tests — beads ymh.3, ymh.4, nir.7, nir.4, 10s,
// nir.5, jpr.2, jpr.6.
//
// Every page must: register via Registry (no hardcoded shell list), carry a
// namespaced ID, and honor its bead's gates (no marketplace UI, no TRIAL
// literal, no matrix duplication, MVP widget tier only).
package shell

import (
	"strings"
	"testing"

	"crossos/core/pkg/pluginapi"
)

func registerAll(t *testing.T, h *Host) {
	t.Helper()
	r := pluginapi.NewRegistry(nil)
	for _, p := range CorePages() {
		if err := r.RegisterUI(p); err != nil {
			t.Fatalf("RegisterUI %s: %v", p.ID, err)
		}
	}
	if err := h.Register(r); err != nil {
		t.Fatalf("Host.Register: %v", err)
	}
}

func pageByID(t *testing.T, h *Host, id string) Page {
	t.Helper()
	for _, p := range h.Pages() {
		if p.ID == id {
			return p
		}
	}
	t.Fatalf("page %s not discovered", id)
	return Page{}
}

func TestAllPagesDiscovered(t *testing.T) {
	h := NewHost()
	registerAll(t, h)
	want := []string{"core.safety", "core.about", "core.plugins", "core.activity", "core.keyboard", "core.windows", "core.schemaHelp", "core.shortcuts", "core.onboarding", "core.finder"}
	if len(h.Pages()) != len(want) {
		t.Fatalf("pages=%d, want %d", len(h.Pages()), len(want))
	}
	for _, id := range want {
		pageByID(t, h, id)
	}
}

func TestSafetyPageContract(t *testing.T) {
	h := NewHost()
	registerAll(t, h)
	p := pageByID(t, h, "core.safety")
	joined := strings.Join(p.Actions, ",")
	for _, a := range []string{"safety.panicStop", "safety.reset", "safety.confirmTrial", "safety.rollbackTrial", "safety.rollback"} {
		if !strings.Contains(joined, a) {
			t.Fatalf("safety actions %s missing %s", joined, a)
		}
	}
}

func TestNoTrialLiteral(t *testing.T) {
	// The TRIAL countdown must reference the Core constant (safety.TRIALTimeout),
	// never a hardcoded 30s literal in app/. Asserted on schema text: no
	// '"timeoutSec":30' / '"30s"' style literal smuggled into any page.
	for _, p := range CorePages() {
		s := string(p.Schema)
		if strings.Contains(s, "30s") || strings.Contains(s, "30 sec") || strings.Contains(s, `"timeout":30`) {
			t.Fatalf("page %s embeds a TRIAL literal: %s", p.ID, s)
		}
	}
}

func TestPluginsPageNoMarketplace(t *testing.T) {
	for _, p := range CorePages() {
		if p.ID != "core.plugins" {
			continue
		}
		s := strings.ToLower(string(p.Schema))
		// Gate on marketplace-enabling surfaces (widget kinds / remote
		// sources), not substrings: the "noMarketplace":true marker itself
		// contains the word "marketplace".
		for _, banned := range []string{`"kind":"marketplace"`, `"kind":"remotebrowse"`, `"kind":"gitregistry"`, "remote registry", "browse remote plugins"} {
			if strings.Contains(s, banned) {
				t.Fatalf("plugins page contains marketplace surface %q (hard gate)", banned)
			}
		}
		if !strings.Contains(s, "nomarketplace") {
			t.Fatal("plugins page must carry the noMarketplace marker")
		}
		return
	}
	t.Fatal("core.plugins page missing")
}

func TestShortcutsLinksDontDuplicate(t *testing.T) {
	// jpr.6: per-rule content editing stays in owning pages — the Shortcuts
	// page links (editLinks: owner), never embeds a second matrix.
	for _, p := range CorePages() {
		if p.ID != "core.shortcuts" {
			continue
		}
		s := string(p.Schema)
		if !strings.Contains(s, "editLinks") {
			t.Fatal("shortcuts page must link to owning pages, not duplicate editing")
		}
		if strings.Contains(s, `"kind":"matrix"`) {
			t.Fatal("shortcuts page must not embed the matrix (owned by 10s)")
		}
		return
	}
	t.Fatal("core.shortcuts page missing")
}

func TestSchemaRendererTier(t *testing.T) {
	// jpr.2: MVP tier widgets only — no custom-view.
	for _, p := range CorePages() {
		if strings.Contains(string(p.Schema), "custom-view") {
			t.Fatalf("page %s smuggles custom-view (Phase 4+ gated)", p.ID)
		}
	}
}

// TestPageControlKinds pins each page's key control kinds so a refactor
// cannot silently drop a bead contract (review: cross-os-8d).
func TestPageControlKinds(t *testing.T) {
	schemas := map[string]string{}
	for _, p := range CorePages() {
		schemas[p.ID] = string(p.Schema)
	}
	// nir.4 Activity: enableFlow with the 4 steps + traceList with trialLink.
	if s := schemas["core.activity"]; !strings.Contains(s, `"kind":"enableFlow"`) || !strings.Contains(s, "trialLink") {
		t.Fatalf("activity missing enableFlow/trialLink: %s", s)
	}
	// 10s Keyboard: matrix immediate + overrides.
	if s := schemas["core.keyboard"]; !strings.Contains(s, `"kind":"matrix"`) || !strings.Contains(s, `"immediate":true`) || !strings.Contains(s, `"kind":"overrides"`) {
		t.Fatalf("keyboard missing immediate matrix/overrides: %s", s)
	}
	// nir.5 Windows: editable shortcutList + zoneEditor.
	if s := schemas["core.windows"]; !strings.Contains(s, `"kind":"shortcutList"`) || !strings.Contains(s, `"editable":true`) || !strings.Contains(s, `"kind":"zoneEditor"`) {
		t.Fatalf("windows missing editable shortcutList/zoneEditor: %s", s)
	}
	// jpr.2 schema help: MVP 4-widget renderer + acceptance note.
	if s := schemas["core.schemaHelp"]; !strings.Contains(s, "checkbox") || !strings.Contains(s, "acceptance") {
		t.Fatalf("schemaHelp missing renderer tier/acceptance: %s", s)
	}
	// ymh.4 About: version + license + credits sources.
	if s := schemas["core.about"]; !strings.Contains(s, `"kind":"version"`) || !strings.Contains(s, `"kind":"license"`) || !strings.Contains(s, `"kind":"credits"`) {
		t.Fatalf("about missing version/license/credits: %s", s)
	}
	// nir.7 Plugins: pluginList with health + install-from-disk.
	if s := schemas["core.plugins"]; !strings.Contains(s, `"kind":"pluginList"`) || !strings.Contains(s, `"kind":"button"`) {
		t.Fatalf("plugins missing pluginList/install button: %s", s)
	}
	// jpr.6 Shortcuts: palette + shortcutList.
	if s := schemas["core.shortcuts"]; !strings.Contains(s, `"kind":"palette"`) || !strings.Contains(s, `"kind":"shortcutList"`) {
		t.Fatalf("shortcuts missing palette/shortcutList: %s", s)
	}
	// qhp.2 Onboarding: single enableFlow + readiness checklist, linked to
	// About (credits) and Safety (trial), docs-linked.
	if s := schemas["core.onboarding"]; !strings.Contains(s, `"kind":"enableFlow"`) || !strings.Contains(s, `"kind":"checklist"`) || !strings.Contains(s, "core.about") || !strings.Contains(s, "core.safety") {
		t.Fatalf("onboarding missing enableFlow/checklist/about/safety links: %s", s)
	}
	// vbl.6 Finder: packList + actionSettings + gateBadge, boundary kept
	// (no plugin-lifecycle duplication — nir.7 owns it).
	if s := schemas["core.finder"]; !strings.Contains(s, `"kind":"packList"`) || !strings.Contains(s, `"kind":"actionSettings"`) || !strings.Contains(s, `"kind":"gateBadge"`) {
		t.Fatalf("finder missing packList/actionSettings/gateBadge: %s", s)
	}
	if s := schemas["core.finder"]; strings.Contains(s, "plugin.installDisk") {
		t.Fatalf("finder duplicates plugin lifecycle: %s", s)
	}
}
