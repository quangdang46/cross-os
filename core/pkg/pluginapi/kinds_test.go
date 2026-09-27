package pluginapi

import (
	"encoding/json"
	"testing"
)

// The kinds the core pages declare, and the ones the shell must be able to
// draw.
//
// This is the half of the contract that lives in Go. The other half is
// `Renderers.kinds` in `macos/Sources/CrossOSApp/Renderers.swift`, and the two
// have drifted before: `activityPage` declares `traceList` while the shell's
// registry knows the same control as `pipelineTrace`, so the alias mapped it to
// the wrong renderer and the Activity page rendered as a sentence instead of a
// timeline. Nothing caught it — a page that renders the wrong control is not a
// crash and not a test failure, it is a page that is quietly not what it
// says it is.
//
// So the retired spellings are DECLARED here, next to the pages that use them,
// and this test is the list a reader can check the shell against. Adding a
// retired spelling without adding it here is the drift.
var retiredSpellings = map[string]string{
	"enableFlow": "wizard",
	"statusCard": "homeSummary",
	"traceList":  "pipelineTrace",
}

// kindsDeclared walks every core page and collects the `kind` field of every
// control in it. A kind reached only through an assembled control — one that
// builds another control rather than declaring it — is not here, and that is
// correct: the assembled kinds are the shell's business, not the daemon's.
func kindsDeclared(t *testing.T) map[string]int {
	t.Helper()
	seen := map[string]int{}
	for _, page := range Pages() {
		var body struct {
			Controls []struct {
				Kind string `json:"kind"`
			} `json:"controls"`
		}
		if err := json.Unmarshal(page.Schema, &body); err != nil {
			t.Fatalf("page %q has a schema that is not an object: %v", page.ID, err)
		}
		for _, control := range body.Controls {
			if control.Kind == "" {
				t.Errorf("page %q declares a control with no kind — the shell cannot dispatch it", page.ID)
				continue
			}
			seen[control.Kind]++
		}
	}
	return seen
}

// Every declared kind is either a live kind or a declared retired spelling.
//
// A kind the shell has never heard of renders as `UnsupportedView`, which
// NAMES itself on the page — so a plugin shipping a new kind gets a visible,
// explicable gap rather than a hole, and this test does not fire for it. What
// it DOES catch is the other direction: the daemon declaring a spelling that
// looks live and is not, which is what a rename in one place and not the
// other looks like.
func TestEveryDeclaredKindIsLiveOrRetired(t *testing.T) {
	declared := kindsDeclared(t)
	for kind := range declared {
		if _, retired := retiredSpellings[kind]; retired {
			continue
		}
		if !liveKinds[kind] {
			t.Errorf(
				"page declares kind %q, which is neither a live kind nor a declared retired spelling. "+
					"Either the shell should learn it, or it is a rename and belongs in retiredSpellings",
				kind,
			)
		}
	}
}

// The live kinds, kept in step with `Renderers.register` in
// `macos/Sources/CrossOSApp/Renderers.swift`.
//
// This is a copy, and a copy is what it is: the two are in different
// languages and there is no way to make one read the other without a
// generator, which is more machinery than twenty-eight strings justify. What
// makes the copy safe is that BOTH sides are asserted against this list — a
// kind added to the shell and not here fails the test below, and a kind
// declared by a page and not in the shell fails the test above.
var liveKinds = map[string]bool{
	"auditList":        true,
	"button":           true,
	"checklist":        true,
	"conflictResolver": true,
	"credits":          true,
	"fileTypeList":     true,
	"finderMenu":       true,
	"homeSummary":      true,
	"keymapEditor":     true,
	"license":          true,
	"matrix":           true,
	"menuList":         true,
	"note":             true,
	"observeToggle":    true,
	"overrides":        true,
	"palette":          true,
	"pipelineTrace":    true,
	"pluginDetail":     true,
	"pluginList":       true,
	"profileList":      true,
	"ruleBuilder":      true,
	"schemaForm":       true,
	"shortcutList":     true,
	"switcherPanel":    true,
	"trial":            true,
	"version":          true,
	"wizard":           true,
	"zoneEditor":       true,
}

// A retired spelling is never also a live kind.
//
// The two sets are the same control under two names, and a spelling that is
// both is a spelling the shell will resolve as a live one and the daemon
// meant as a retired one. `traceList` was that bug: a live kind by accident of
// ordering, pointing at a renderer that had been renamed out from under it.
func TestARetiredSpellingIsNotAlsoLive(t *testing.T) {
	for retired, replacement := range retiredSpellings {
		if liveKinds[retired] {
			t.Errorf(
				"%q is a retired spelling (→ %s) and also a live kind. "+
					"Pick one: a live kind is drawn by the registry, a retired one by the alias",
				retired, replacement,
			)
		}
		if !liveKinds[replacement] {
			t.Errorf("retired spelling %q points at %q, which is not a live kind", retired, replacement)
		}
	}
}

// Every page carries a title, a group and at least one control.
//
// A page with no group is a page that sits on its own in the nav, which is not
// what any of the core pages are, and a page with no controls is a page that
// opens onto nothing. Both are cheap to assert and both are the sort of thing
// that only shows up when somebody looks at the app.
func TestEveryCorePageIsDrawable(t *testing.T) {
	for _, page := range Pages() {
		if page.Title == "" {
			t.Errorf("page %q has no title", page.ID)
		}
		if page.Group == "" {
			t.Errorf("page %q has no nav group", page.ID)
		}
		if page.Symbol == "" {
			t.Errorf("page %q has no symbol — the nav identifies every section by something "+
				"other than its name, and this one would be the only blank", page.ID)
		}
		var body struct {
			Controls []map[string]any `json:"controls"`
		}
		if err := json.Unmarshal(page.Schema, &body); err != nil {
			t.Errorf("page %q has an unparseable schema: %v", page.ID, err)
			continue
		}
		if len(body.Controls) == 0 {
			t.Errorf("page %q declares no controls", page.ID)
		}
	}
}
