// Page contribution tests — beads ymh.3, ymh.4, nir.7, nir.4, 10s,
// nir.5, jpr.2, jpr.6.
//
// Every page must: register via Registry (no hardcoded shell list), carry a
// namespaced ID, and honor its bead's gates (no marketplace UI, no TRIAL
// literal, no matrix duplication, MVP widget tier only).
package shell

import (
	"encoding/json"
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
	// The served order, frozen. This is the product's navigation: Home and
	// Profiles, the shortcut surfaces (Keyboard, Windows, Switcher,
	// Shortcuts, Explorer), the two activity surfaces, then the advanced
	// group. It is written out rather than derived so a page that quietly
	// changes its Order — or a group that starts sorting somewhere
	// surprising — fails here instead of shipping a new nav.
	//
	// Onboarding is not in this list. On a fresh profile it LEADS (see
	// TestFirstRunLandingGate), which is the one thing that makes an
	// otherwise-ordered list wrong to freeze on its own.
	want := []struct {
		id    string
		group string
		order int
	}{
		{"core.home", "home", 0},
		{"core.profiles", "home", 10},
		{"core.keyboard", "shortcuts", 20},
		{"core.windows", "shortcuts", 30},
		{"core.switcher", "shortcuts", 35},
		{"core.shortcuts", "shortcuts", 40},
		{"core.finder", "shortcuts", 50},
		{"core.activity", "activity", 60},
		{"core.observe", "activity", 70},
		{"core.extensions", "advanced", 80},
		{"core.schemaHelp", "advanced", 90},
		{"core.safety", "advanced", 100},
		{"core.about", "advanced", 110},
	}
	pages := h.Pages()
	if len(pages) != len(want)+1 {
		t.Fatalf("pages=%d, want %d (+ the first-run page)", len(pages), len(want)+1)
	}
	for _, w := range want {
		p := pageByID(t, h, w.id)
		if p.Group != w.group || p.Order != w.order {
			t.Fatalf("page %s is group %q order %d, want group %q order %d", w.id, p.Group, p.Order, w.group, w.order)
		}
	}
	// The group+order fields are what the nav sorts on, so they have to
	// agree with the served position — a page that is third in the list but
	// declares order 90 would render correctly today and reorder itself the
	// first time a second registry registered.
	for i, w := range want {
		if pages[i+1].ID != w.id {
			t.Fatalf("served[%d]=%s, want %s (the frozen nav order)", i+1, pages[i+1].ID, w.id)
		}
	}
}

// TestFirstRunLandingGate pins the one ordering rule that depends on state:
// a fresh profile opens the wizard, and a profile that has finished
// onboarding does not. Both halves are asserted because the failure mode is
// asymmetric — a wizard that never leads strands someone at Home with no
// setup, and a wizard that leads forever is a Welcome page in the way.
func TestFirstRunLandingGate(t *testing.T) {
	h := NewHost()
	registerAll(t, h)

	first := h.Pages()[0]
	if first.ID != "core.onboarding" {
		t.Fatalf("fresh profile opens %s, want core.onboarding", first.ID)
	}
	if !first.FirstRun {
		t.Fatal("core.onboarding must be discovered as the first-run page")
	}

	h.OnboardingComplete(true)
	after := h.Pages()[0]
	if after.ID == "core.onboarding" {
		t.Fatal("the first-run page must stop leading once onboarding is done")
	}
	if after.ID != "core.home" {
		t.Fatalf("after onboarding the nav opens %s, want core.home", after.ID)
	}
	// The flag stops deciding, but the page is still there — a person who
	// wants to re-run the checklist has to be able to find it.
	if pageByID(t, h, "core.onboarding").ID != "core.onboarding" {
		t.Fatal("onboarding page must survive onboarding")
	}
}

// TestFirstRunAndVisibilityHaveReaders pins the two fields the Host used to
// carry without reading. A contribution that declares firstRun and a
// visibility condition must arrive on the Page, or the nav has nothing to
// gate the landing page or the controls on.
func TestFirstRunAndVisibilityHaveReaders(t *testing.T) {
	h := NewHost()
	registerAll(t, h)
	for _, p := range h.Pages() {
		if p.Visibility == "" {
			t.Fatalf("page %s: empty Visibility — the contribution's condition never reached the shell", p.ID)
		}
		if p.ID == "core.onboarding" && !p.FirstRun {
			t.Fatal("core.onboarding: firstRun marker in the schema was not read")
		}
		if p.ID != "core.onboarding" && p.FirstRun {
			t.Fatalf("page %s claims firstRun; only the wizard may", p.ID)
		}
	}
}

// TestOrderBeatsAlphabet pins the sort the alphabetical one used to hide:
// core.about sorted first because "about" < "safety", which is not a
// navigation anybody chose. Group/Order is the declared answer, and it has to
// win over the id.
func TestOrderBeatsAlphabet(t *testing.T) {
	h := NewHost()
	registerAll(t, h)
	h.OnboardingComplete(true)
	first := h.Pages()[0]
	if first.ID == "core.about" {
		t.Fatal("the nav is back to alphabetical order — core.about cannot lead")
	}
	if first.Order > 10 {
		t.Fatalf("nav leads with order %d (%s), want the lowest declared order", first.Order, first.ID)
	}
}

// TestNavGroupsAreContiguous pins that pages sharing a group are served
// together. A nav that renders one group heading per page is a page per
// group, which is the thing the Group field exists to prevent.
func TestNavGroupsAreContiguous(t *testing.T) {
	h := NewHost()
	registerAll(t, h)
	h.OnboardingComplete(true)
	seen := map[string]bool{}
	previous := ""
	for _, p := range h.Pages() {
		if p.Group == "" {
			t.Fatalf("page %s has no group", p.ID)
		}
		if p.Group != previous {
			if seen[p.Group] {
				t.Fatalf("group %q appears in two runs; pages sharing a group must be served together", p.Group)
			}
			seen[p.Group] = true
			previous = p.Group
		}
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

func TestExtensionsPageNoMarketplace(t *testing.T) {
	for _, p := range CorePages() {
		if p.ID != "core.extensions" {
			continue
		}
		s := strings.ToLower(string(p.Schema))
		// Gate on marketplace-enabling surfaces (widget kinds / remote
		// sources), not substrings: the "noMarketplace":true marker itself
		// contains the word "marketplace".
		for _, banned := range []string{`"kind":"marketplace"`, `"kind":"remotebrowse"`, `"kind":"gitregistry"`, "remote registry", "browse remote plugins"} {
			if strings.Contains(s, banned) {
				t.Fatalf("extensions page contains marketplace surface %q (hard gate)", banned)
			}
		}
		if !strings.Contains(s, "nomarketplace") {
			t.Fatal("extensions page must carry the noMarketplace marker")
		}
		return
	}
	t.Fatal("core.extensions page missing")
}

// TestExtensionsNaming pins the split the product asked for: "Extensions" is
// what the person reads, "Plugin" stays the technical term underneath. Both
// halves are asserted because the failure is quiet — a page called
// "Extensions" whose body still says "Plugins" is a half-rename nobody
// notices until a support question arrives.
func TestExtensionsNaming(t *testing.T) {
	p := ExtensionsPage()
	if p.Title != "Extensions" {
		t.Fatalf("title=%q, want Extensions", p.Title)
	}
	if p.ID != "core.extensions" {
		t.Fatalf("id=%q, want core.extensions", p.ID)
	}
	s := string(p.Schema)
	// The wire keeps the technical vocabulary: the source, the capability
	// ids and the note all still say plugin.
	for _, want := range []string{"core:plugins", "plugin.enable", "plugin.disable", "plugin.installDisk"} {
		if !strings.Contains(s, want) {
			t.Fatalf("extensions schema lost the technical term %q", want)
		}
	}
	// The user-facing copy must not call an extension a plugin.
	var body struct {
		Description string `json:"description"`
		Controls    []struct {
			Kind string `json:"kind"`
			Text string `json:"text"`
		} `json:"controls"`
	}
	if err := json.Unmarshal(p.Schema, &body); err != nil {
		t.Fatalf("schema not JSON: %v", err)
	}
	if strings.Contains(strings.ToLower(body.Description), "plugin") {
		t.Fatalf("user-facing description still says plugin: %q", body.Description)
	}
	for _, c := range body.Controls {
		if strings.Contains(strings.ToLower(c.Text), "plugin") {
			t.Fatalf("user-facing note still says plugin: %q", c.Text)
		}
	}
}

// TestExtensionsRowActionsAreLive pins the dead rowActions removal. A row
// action the daemon declares no method for, and no renderer reads, is a
// promise the page cannot keep; plugin.update and plugin.uninstall were both,
// and leaving them on the schema is how they stay that way.
func TestExtensionsRowActionsAreLive(t *testing.T) {
	s := string(ExtensionsPage().Schema)
	for _, dead := range []string{"plugin.update", "plugin.uninstall"} {
		if strings.Contains(s, dead) {
			t.Fatalf("extensions schema still declares dead row action %q", dead)
		}
	}
	for _, live := range []string{"plugin.enable", "plugin.disable"} {
		if !strings.Contains(s, live) {
			t.Fatalf("extensions schema dropped live row action %q", live)
		}
	}
}

// TestFinderTitledExplorer pins the rename. The page id and the Finder
// vocabulary underneath stay — only what the person reads changes.
func TestFinderTitledExplorer(t *testing.T) {
	f := FinderPage()
	if f.Title != "Explorer" {
		t.Fatalf("title=%q, want Explorer", f.Title)
	}
	if f.ID != "core.finder" {
		t.Fatalf("id=%q, want core.finder (the technical name does not change)", f.ID)
	}
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
	// nir.7 Extensions: pluginList with health + install-from-disk.
	if s := schemas["core.extensions"]; !strings.Contains(s, `"kind":"pluginList"`) || !strings.Contains(s, `"kind":"button"`) {
		t.Fatalf("extensions missing pluginList/install button: %s", s)
	}
	// Home: the landing card over sources the daemon already serves, plus the
	// readiness checklist. No source of its own — a landing card that fetched
	// something separate could disagree with the masthead above it.
	if s := schemas["core.home"]; !strings.Contains(s, `"kind":"statusCard"`) || !strings.Contains(s, "core:profiles") || !strings.Contains(s, `"kind":"checklist"`) {
		t.Fatalf("home missing statusCard/profiles/checklist: %s", s)
	}
	// Profiles: cards that apply in one step, and a note that keeps the
	// per-capability switches on their owning pages.
	if s := schemas["core.profiles"]; !strings.Contains(s, `"kind":"profileList"`) || !strings.Contains(s, "core:profileApply") {
		t.Fatalf("profiles missing profileList/apply: %s", s)
	}
	// Observe: the event inspector on its own page, with the dry-run toggle
	// and the live feed.
	if s := schemas["core.observe"]; !strings.Contains(s, "core.setObserve") || !strings.Contains(s, `"kind":"traceList"`) {
		t.Fatalf("observe missing setObserve/traceList: %s", s)
	}
	// Observe is a page of its own, not a second control on Activity: §5
	// (conflicts) and §6 (observe) are separate questions.
	if s := schemas["core.activity"]; strings.Contains(s, "core.setObserve") {
		t.Fatalf("activity grows the observe surface; it has its own page: %s", s)
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
	// vbl.6 Finder: the two lists the daemon serves — the right-click menu and
	// the file types it offers — boundary kept (no plugin-lifecycle
	// duplication, nir.7 owns it).
	if s := schemas["core.finder"]; !strings.Contains(s, `"kind":"menuList"`) || !strings.Contains(s, "core:finderMenu") || !strings.Contains(s, `"kind":"fileTypeList"`) || !strings.Contains(s, "core:fileTypes") {
		t.Fatalf("finder missing menuList/fileTypeList over the served sources: %s", s)
	}
	if s := schemas["core.finder"]; strings.Contains(s, "plugin.installDisk") {
		t.Fatalf("finder duplicates plugin lifecycle: %s", s)
	}
}

func TestPagesCarrySchema(t *testing.T) {
	// The frontend renders page bodies from Schema controls — Pages()
	// without Schema would leave every page a title-only shell.
	h := NewHost()
	r := pluginapi.NewRegistry(nil)
	for _, p := range CorePages() {
		if err := r.RegisterUI(p); err != nil {
			t.Fatalf("RegisterUI %s: %v", p.ID, err)
		}
	}
	if err := h.Register(r); err != nil {
		t.Fatalf("Host.Register: %v", err)
	}
	for _, p := range h.Pages() {
		if len(p.Schema) == 0 {
			t.Fatalf("page %s: empty Schema (frontend would render title-only)", p.ID)
		}
		var body struct {
			Controls []struct {
				Kind string `json:"kind"`
				ID   string `json:"id"`
			} `json:"controls"`
			Type string `json:"type"`
		}
		if err := json.Unmarshal(p.Schema, &body); err != nil {
			t.Fatalf("page %s: schema not JSON: %v", p.ID, err)
		}
		if body.Type != "page" {
			t.Fatalf("page %s: type=%q, want page", p.ID, body.Type)
		}
		if len(body.Controls) == 0 {
			t.Fatalf("page %s: no controls (title-only shell)", p.ID)
		}
		for _, c := range body.Controls {
			if c.Kind == "" || c.ID == "" {
				t.Fatalf("page %s: control %+v missing kind/id", p.ID, c)
			}
		}
	}
}

// switcherControl is the part of a control the page contract is written in:
// what it draws, where its rows come from, and what a row or the whole list
// can write. Same shape as the Explorer page's, for the same reason — the
// schema is authored as a Go map literal, so these tests read the JSON the
// Host serves rather than the struct that built it.
type switcherControl struct {
	Kind      string   `json:"kind"`
	ID        string   `json:"id"`
	Label     string   `json:"label"`
	Source    string   `json:"source"`
	RowAction string   `json:"rowAction"`
	Actions   []string `json:"actions"`
}

func decodeSwitcher(t *testing.T) []switcherControl {
	t.Helper()
	var body struct {
		Controls []switcherControl `json:"controls"`
	}
	if err := json.Unmarshal(SwitcherPage().Schema, &body); err != nil {
		t.Fatalf("switcher schema not JSON: %v", err)
	}
	return body.Controls
}

// TestSwitcherPageContract pins the three things that decide whether the page
// shows anything: the kind ws-6 renders, the source the daemon serves, and the
// two action ids it declares. Each is checked against the method table rather
// than against a wish — a source no handler serves renders an empty page that
// reads as "no windows open" rather than as a missing method, and an action id
// spelled differently from the daemon's is a permission token that matches
// nothing.
func TestSwitcherPageContract(t *testing.T) {
	p := SwitcherPage()
	if p.ID != "core.switcher" || p.Title != "Switcher" {
		t.Fatalf("id=%q title=%q, want core.switcher / Switcher", p.ID, p.Title)
	}
	controls := decodeSwitcher(t)
	if len(controls) != 1 {
		t.Fatalf("switcher declares %d controls, want exactly 1: %+v", len(controls), controls)
	}
	c := controls[0]
	if c.Kind != "switcherPanel" {
		t.Fatalf("kind=%q, want switcherPanel (the kind ws-6 draws)", c.Kind)
	}
	if c.ID != "windows" {
		t.Fatalf("id=%q, want windows", c.ID)
	}
	// The label is in words, not the source id: the layout rule this page
	// follows is that a named thing is never identified by its key alone.
	if c.Label == "" || c.Label == c.Source {
		t.Fatalf("label=%q must name the control in words, not repeat %q", c.Label, c.Source)
	}
	if c.Source != "core:windows" {
		t.Fatalf("source=%q, want core:windows (ws-3's core.windows)", c.Source)
	}
	if c.RowAction != "core.switcherFocus" {
		t.Fatalf("rowAction=%q, want core.switcherFocus", c.RowAction)
	}
	if len(c.Actions) != 1 || c.Actions[0] != "core.switcherWait" {
		t.Fatalf("actions=%v, want [core.switcherWait]", c.Actions)
	}
	// The contribution's action list is every action its controls declare, so a
	// control that gains a write without the contribution hearing about it
	// fails here rather than at the daemon.
	if got, want := strings.Join(p.Actions, ","), "core.switcherFocus,core.switcherWait"; got != want {
		t.Fatalf("contribution actions=%q, want %q", got, want)
	}
}

// TestSwitcherSourcesAreServed pins the source→method pairing the page depends
// on. The daemon is a separate Go module, so core cannot import these pages
// and app cannot reach the daemon's method table; what this holds is that the
// page's own source and action ids are the spellings the bridge below is bound
// under. A bridge renamed without the page following renames the permission
// token and leaves the panel reading a source no method serves.
func TestSwitcherSourcesAreServed(t *testing.T) {
	app := NewApp(populatedCore())
	svc := NewService(app, NewHost())
	// core:windows is served by the bound Windows call. The stub answers a nil
	// list, so this asserts the two properties the page depends on: the source
	// is reachable at all, and what arrives is a list the panel can render. The
	// UI-log note the null leaves is the bridge's own rule (sourceList), and a
	// machine with no window server is exactly the case where the page must
	// still draw — an unreachable source is a different failure.
	rows, err := svc.Windows()
	if err != nil {
		t.Fatalf("the switcher page's source is not served: %v", err)
	}
	if rows == nil {
		t.Fatal("core:windows reached the page as null, not as an empty list")
	}
	// The wait is a source too, and the one call on this page that is allowed
	// to answer "nothing happened": a spent budget is a poll resuming, not a
	// daemon fault. A trigger is what says the switcher was summoned.
	trig, err := svc.SwitcherWait(0)
	if err != nil {
		t.Fatalf("core.switcherWait is not served: %v", err)
	}
	if trig.Triggered {
		t.Fatalf("an idle switcher answered triggered=true: %+v", trig)
	}
	// The row action fails closed on a nameless window, the way the daemon
	// refuses a missing window_id — a click that focused nothing must not leave
	// the page believing it did.
	if err := svc.SwitcherFocus(""); err == nil {
		t.Fatal("core.switcherFocus must refuse an empty window id")
	}
	if len(svc.UILogs()) == 0 {
		t.Fatal("the refused focus must reach the UI log, not vanish")
	}
}
