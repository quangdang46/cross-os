// UI source bridge tests — bead cross-os-72g.
//
// Pass criteria mapping:
//  1. Every page source reaches the frontend typed → TestPageSourcesServed
//     and TestServiceExposesFrozenSources.
//  2. A failed source call is logged AND returns a typed error, never a
//     zero value that reads as "successfully empty" → TestSourceFailuresLogged.
//  3. A null collection from the daemon is an empty list, not a null the UI
//     must defend against, and emptiness stays distinguishable from failure
//     → TestNullSourceBecomesEmptyList.
//  4. Writes fail closed: an unknown app/rule/zone id changes nothing
//     → TestOverrideAndZoneWritesFailClosed.
//  5. The Go Service and the generated TypeScript bindings declare the same
//     methods, and the generated models carry the same keys the daemon serves
//     → TestBindingsShimParity, TestGeneratedModelsMatchWireTags,
//     TestAppRowMatchesTheDaemonsRow.
package shell

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"crossos/core/pkg/ctx"
)

// populatedCore is a Core serving one honest row per new source — enough to
// catch a mis-mapped field on every one of the ten payloads.
func populatedCore() *stubCore {
	return &stubCore{
		matrix: []MatrixRow{{
			RuleID: "windows-keyboard.ctrl-c-copy", Plugin: "win-kb", Action: "copy",
			Keys: "Ctrl+C", Contexts: []string{"any", "finder"}, Enabled: true,
		}},
		overrides: []OverrideRow{{App: "Finder", RuleID: "mac-finder.copy", Action: "copy", Keys: "Cmd+C", Enabled: false}},
		zones:     []ZoneRow{{ID: "left", Name: "Left half", X: 0, Y: 0, W: 0.5, H: 1}},
		commands:  []CommandRow{{ID: "win-kb.reopen", Title: "Reopen window", Plugin: "win-kb"}},
		schemas: []SchemaRow{{
			Plugin: "win-wm", Title: "Window Manager",
			Schema: map[string]any{"type": "object", "widget": "checkbox"},
		}},
		audit:     []AuditRow{{Resource: "login-item", ID: "com.crossos.helper", Owner: "crossos", CreatedAt: "2026-09-01T10:00:00Z"}},
		trial:     TrialState{Plugin: "win-wm", State: "trial", RemainingMS: 12000, TimeoutMS: 30000},
		readiness: []ReadinessRow{{ID: "keyboard", Label: "Keyboard", Ready: true}, {ID: "windows", Label: "Window management", Ready: false, Detail: "grant Accessibility"}},
	}
}

func TestPageSourcesServed(t *testing.T) {
	app := NewApp(populatedCore())

	rows, err := app.GetMatrix()
	if err != nil {
		t.Fatalf("GetMatrix: %v", err)
	}
	want := MatrixRow{RuleID: "windows-keyboard.ctrl-c-copy", Plugin: "win-kb", Action: "copy", Keys: "Ctrl+C", Contexts: []string{"any", "finder"}, Enabled: true}
	if len(rows) != 1 || !reflect.DeepEqual(rows[0], want) {
		t.Fatalf("GetMatrix=%+v, want one row %+v", rows, want)
	}

	overs, err := app.GetOverrides()
	if err != nil {
		t.Fatalf("GetOverrides: %v", err)
	}
	if len(overs) != 1 || overs[0].App != "Finder" || overs[0].Enabled {
		t.Fatalf("GetOverrides=%+v, want one disabled Finder override", overs)
	}

	zones, err := app.GetZones()
	if err != nil {
		t.Fatalf("GetZones: %v", err)
	}
	if len(zones) != 1 || zones[0].ID != "left" || zones[0].W != 0.5 || zones[0].H != 1 {
		t.Fatalf("GetZones=%+v, want the left half", zones)
	}

	cmds, err := app.Commands()
	if err != nil {
		t.Fatalf("Commands: %v", err)
	}
	if len(cmds) != 1 || cmds[0].ID != "win-kb.reopen" || cmds[0].Plugin != "win-kb" {
		t.Fatalf("Commands=%+v, want win-kb.reopen", cmds)
	}

	schemas, err := app.PluginSchemas()
	if err != nil {
		t.Fatalf("PluginSchemas: %v", err)
	}
	// The schema must arrive as a decoded object: a re-parsed JSON string is
	// the exact failure App.tsx:41 documents for pages.
	if len(schemas) != 1 || schemas[0].Schema["widget"] != "checkbox" {
		t.Fatalf("PluginSchemas=%+v, want a decoded object schema", schemas)
	}

	audit, err := app.OwnershipAudit()
	if err != nil {
		t.Fatalf("OwnershipAudit: %v", err)
	}
	if len(audit) != 1 || audit[0].ID != "com.crossos.helper" || audit[0].Owner != "crossos" {
		t.Fatalf("OwnershipAudit=%+v, want the helper login item", audit)
	}

	st, err := app.TrialState()
	if err != nil {
		t.Fatalf("TrialState: %v", err)
	}
	if st != (TrialState{Plugin: "win-wm", State: "trial", RemainingMS: 12000, TimeoutMS: 30000}) {
		t.Fatalf("TrialState=%+v, want the in-flight 12s of 30s", st)
	}
	if len(app.UILogs()) != 0 {
		t.Fatalf("a healthy source read must stay out of the error log: %v", app.UILogs())
	}

	ready, err := app.Readiness()
	if err != nil {
		t.Fatalf("Readiness: %v", err)
	}
	if len(ready) != 2 || !ready[0].Ready || ready[1].Ready || ready[1].Detail == "" {
		t.Fatalf("Readiness=%+v, want keyboard ready + windows blocked with a hint", ready)
	}
}

// The Wave 3 stubs below complete stubCore's Core surface (bead
// w3-shell-bridge). The struct itself lives in shell_test.go, which owns the
// ten sources' fields, so these carry no rows of their own: the zero value
// returns a nil list, which is exactly what the null-source rules below need,
// and the daemon's own spelling for every Wave 3 row is pinned over the IPC
// stub in ipc_client_test.go instead — where it is JSON text rather than a
// struct agreeing with itself.
func (s *stubCore) Profiles() ([]ProfileRow, error) {
	if s.failSources != nil {
		return nil, s.failSources
	}
	return nil, nil
}

// ApplyProfile refuses an id no profile declares, the way core.profileApply
// does: the store's active-profile field is free text, so the catalog of what
// may be applied is the daemon's alone. A stub that accepted one would let the
// delegation test pass against a bridge that reports "applied" for a bundle
// that does not exist.
func (s *stubCore) ApplyProfile(profileID string) (map[string]any, error) {
	if s.failSources != nil {
		return nil, s.failSources
	}
	if profileID != "windows-11" {
		return nil, errors.New("shell: unknown profile " + profileID)
	}
	return map[string]any{"profile": profileID, "rules": 21, "plugins": 2}, nil
}

func (s *stubCore) Traces() ([]TraceRow, error) {
	if s.failSources != nil {
		return nil, s.failSources
	}
	return nil, nil
}

func (s *stubCore) PluginMeta() ([]PluginMetaRow, error) {
	if s.failSources != nil {
		return nil, s.failSources
	}
	return nil, nil
}

func (s *stubCore) Apps() ([]AppRow, error) {
	if s.failSources != nil {
		return nil, s.failSources
	}
	return nil, nil
}

func (s *stubCore) UserRules() ([]UserRuleRow, error) {
	if s.failSources != nil {
		return nil, s.failSources
	}
	return nil, nil
}

// SetUserRule refuses a rule with no key or no capability, which is the store's
// own first gate: a rule that cannot be compiled has no id to derive.
func (s *stubCore) SetUserRule(rule UserRuleRow) (string, error) {
	if s.failSources != nil {
		return "", s.failSources
	}
	if rule.Key == "" || rule.Capability == "" {
		return "", errors.New("shell: a rule needs a key and a capability")
	}
	if rule.ID == "" {
		return "user.ctrl+c@native", nil // derived server-side
	}
	return rule.ID, nil
}

// DeleteUserRule refuses an empty id rather than reading it as "delete the one
// unnamed rule": config.deleteUserRule answers a missing id with bad-params, and
// a stub that guessed would test the opposite contract.
func (s *stubCore) DeleteUserRule(id string) ([]UserRuleRow, error) {
	if s.failSources != nil {
		return nil, s.failSources
	}
	if id == "" {
		return nil, errors.New("shell: no rule id to delete")
	}
	return nil, nil
}

// The switcher stubs carry the daemon's own gates for the same reason the Wave
// 3 ones do: a stub that answered "yes" to everything would let a bridge that
// drops the wait budget or focuses a nameless window pass. They also carry no
// rows, so the null-source rules below reach them through the zero value — and
// the daemon's spelling is pinned over the IPC stub instead, where the payload
// is JSON text rather than a struct agreeing with itself.
func (s *stubCore) Windows() ([]WindowRow, error) {
	if s.failSources != nil {
		return nil, s.failSources
	}
	return nil, nil
}

// SwitcherWait answers the way a spent budget does: a real object that says
// nothing happened. A stub that errored would make an expired poll look like a
// broken daemon.
func (s *stubCore) SwitcherWait(timeoutMs int) (SwitcherTrigger, error) {
	if s.failSources != nil {
		return SwitcherTrigger{}, s.failSources
	}
	return SwitcherTrigger{Triggered: false}, nil
}

// SwitcherFocus refuses an empty id the way core.switcherFocus refuses a
// missing window_id with bad-params.
func (s *stubCore) SwitcherFocus(windowID string) error {
	if s.failSources != nil {
		return s.failSources
	}
	if windowID == "" {
		return errors.New("shell: no window id to focus")
	}
	return nil
}

// The onboarding stubs carry no rows of their own, the same bargain the wave-3
// stubs above make: the zero value is a fresh, unfinished wizard, which is what
// the null and failure rules below need to reach them. The wizard's own round
// trip — fresh daemon, then the write, then the re-read — is driven for real in
// core/cmd/crossos/pagedata_test.go, where the store and the derivation both
// exist, and the daemon's spelling is pinned against pagedata.go below rather
// than against a struct agreeing with itself.
func (s *stubCore) OnboardingState() (OnboardingRow, error) {
	if s.failSources != nil {
		return OnboardingRow{}, s.failSources
	}
	return OnboardingRow{
		CurrentStep: "welcome",
		Steps:       []OnboardingStep{},
		Readiness:   nil,
	}, nil
}

// CompleteOnboarding accepts the write, because a stub that refused one would
// make the failure rules below assert a rejection no daemon performs.
func (s *stubCore) CompleteOnboarding() error {
	if s.failSources != nil {
		return s.failSources
	}
	return nil
}

// SetObserve records the flag, because a stub that dropped it could not tell
// a read-back from a write. The stub holds the state rather than echoing the
// call back, so a test that reads ObserveState is reading something the write
// actually changed — the property the real recorder has and an optimistic
// echo would not.
func (s *stubCore) SetObserve(on bool) error {
	if s.failSources != nil {
		return s.failSources
	}
	s.observing = on
	return nil
}

// ObserveState reports what SetObserve last recorded, or the zero row when it
// never was called. It is a read of stored state for the same reason.
func (s *stubCore) ObserveState() (ObserveStateRow, error) {
	if s.failSources != nil {
		return ObserveStateRow{}, s.failSources
	}
	return ObserveStateRow{Observe: s.observing, Mode: "metadata-only"}, nil
}

// OpenSystemSettings accepts the call, for the same reason: a stub that refused
// it would make a test assert a rejection the daemon only performs off darwin,
// which is the daemon's own rule and not the shell's to invent.
func (s *stubCore) OpenSystemSettings() error {
	if s.failSources != nil {
		return s.failSources
	}
	return nil
}

// sourceReader names one bridge call so the null and failure rules below can
// be asserted once for every source instead of per method.
type sourceReader struct {
	name string
	read func(*App) error
}

// sourceReaders are the list-shaped sources — the ten the pages already ask for
// plus the Wave 3 set, so every rule below covers the new rows for free.
func sourceReaders() []sourceReader {
	return []sourceReader{
		{"GetMatrix", func(a *App) error { _, err := a.GetMatrix(); return err }},
		{"GetOverrides", func(a *App) error { _, err := a.GetOverrides(); return err }},
		{"GetZones", func(a *App) error { _, err := a.GetZones(); return err }},
		{"Commands", func(a *App) error { _, err := a.Commands(); return err }},
		{"PluginSchemas", func(a *App) error { _, err := a.PluginSchemas(); return err }},
		{"OwnershipAudit", func(a *App) error { _, err := a.OwnershipAudit(); return err }},
		{"Readiness", func(a *App) error { _, err := a.Readiness(); return err }},
		{"Profiles", func(a *App) error { _, err := a.Profiles(); return err }},
		{"Traces", func(a *App) error { _, err := a.Traces(); return err }},
		{"PluginMeta", func(a *App) error { _, err := a.PluginMeta(); return err }},
		{"Apps", func(a *App) error { _, err := a.Apps(); return err }},
		{"UserRules", func(a *App) error { _, err := a.UserRules(); return err }},
		{"DeleteUserRule", func(a *App) error { _, err := a.DeleteUserRule("user.ctrl+c@terminal"); return err }},
		{"Windows", func(a *App) error { _, err := a.Windows(); return err }},
	}
}

func TestNullSourceBecomesEmptyList(t *testing.T) {
	// A live daemon that answers null breaks the "[] never null" rule. The
	// page still gets a list it can render; the violation is reported so the
	// symptom is traceable instead of looking like "nothing configured".
	for _, c := range sourceReaders() {
		app := NewApp(&stubCore{}) // every list nil
		if err := c.read(app); err != nil {
			t.Fatalf("%s on a null result: want an empty list, got %v", c.name, err)
		}
		logs := app.UILogs()
		if len(logs) != 1 || !strings.Contains(logs[0], c.name) || !strings.Contains(logs[0], "null") {
			t.Fatalf("%s: null result must leave one UI-log note naming it, got %v", c.name, logs)
		}
	}
}

func TestSourceFailuresLogged(t *testing.T) {
	boom := errors.New("ipc: daemon unreachable")
	calls := append(sourceReaders(),
		sourceReader{"SetOverride", func(a *App) error { _, err := a.SetOverride("Finder", "mac-finder.copy", true); return err }},
		sourceReader{"SetZones", func(a *App) error { _, err := a.SetZones([]ZoneRow{{ID: "left", W: 1, H: 1}}); return err }},
		sourceReader{"TrialState", func(a *App) error { _, err := a.TrialState(); return err }},
		sourceReader{"ApplyProfile", func(a *App) error { _, err := a.ApplyProfile("windows-11"); return err }},
		sourceReader{"SetUserRule", func(a *App) error {
			_, err := a.SetUserRule(UserRuleRow{Key: "C", Capability: "clipboard.copy"})
			return err
		}},
		sourceReader{"SwitcherWait", func(a *App) error { _, err := a.SwitcherWait(2000); return err }},
		sourceReader{"SwitcherFocus", func(a *App) error { return a.SwitcherFocus("412") }},
	)
	for _, c := range calls {
		app := NewApp(&stubCore{failSources: boom})
		if err := c.read(app); !errors.Is(err, boom) {
			t.Fatalf("%s: want the daemon error surfaced, got %v", c.name, err)
		}
		logs := app.UILogs()
		if len(logs) != 1 || !strings.Contains(logs[0], c.name) {
			t.Fatalf("%s: failure must reach the UI log exactly once, got %v", c.name, logs)
		}
	}
}

func TestFailedWriteReturnsNoOptimisticRow(t *testing.T) {
	// A denied write must not hand the page a row that looks applied.
	boom := errors.New("ipc: daemon unreachable")
	app := NewApp(&stubCore{failSources: boom})
	row, err := app.SetOverride("Finder", "mac-finder.copy", true)
	if !errors.Is(err, boom) {
		t.Fatalf("SetOverride error=%v, want %v", err, boom)
	}
	if row != (OverrideRow{}) {
		t.Fatalf("SetOverride returned %+v on failure — the page would render an edit that never happened", row)
	}
	if n, err := app.SetZones([]ZoneRow{{ID: "left", W: 1, H: 1}}); err == nil || n != 0 {
		t.Fatalf("SetZones=%d,%v, want 0 and the daemon error", n, err)
	}
	// The same rule for the two Wave 3 writes: a refused rule edit hands back no
	// id for the editor to keep, and a refused delete hands back no table for it
	// to render as if the rule were gone.
	if id, err := app.SetUserRule(UserRuleRow{Key: "C", Modifiers: []string{"Ctrl"}, Capability: "clipboard.copy"}); err == nil || id != "" {
		t.Fatalf("SetUserRule=%q,%v, want no id and the daemon error", id, err)
	}
	if rows, err := app.DeleteUserRule("user.win+left@com.apple.Finder"); err == nil || rows != nil {
		t.Fatalf("DeleteUserRule=%v,%v, want no rows and the daemon error", rows, err)
	}
}

func TestOverrideAndZoneWritesFailClosed(t *testing.T) {
	c := populatedCore()
	app := NewApp(c)
	before := append([]OverrideRow(nil), c.overrides...)
	zones := append([]ZoneRow(nil), c.zones...)

	// Three distinct denials, none of which may reach the stored set: a rule
	// that exists nowhere, a write with no app, and a known rule that has no
	// override for this app.
	denied := []struct {
		what   string
		app    string
		ruleID string
	}{
		{"unknown rule", "Finder", "nope"},
		{"no app", "", "windows-keyboard.ctrl-c-copy"},
		{"known rule with no override for this app", "Ghost", "windows-keyboard.ctrl-c-copy"},
	}
	for _, d := range denied {
		if _, err := app.SetOverride(d.app, d.ruleID, true); err == nil {
			t.Fatalf("%s must fail closed", d.what)
		}
	}
	for i := range c.overrides {
		if c.overrides[i] != before[i] {
			t.Fatalf("a rejected override edit touched the stored set: %+v", c.overrides)
		}
	}
	if len(app.UILogs()) != len(denied) {
		t.Fatalf("all %d denials must reach the UI log, got %v", len(denied), app.UILogs())
	}

	// The one accepted write echoes the daemon's stored row, not the request.
	row, err := app.SetOverride("Finder", "mac-finder.copy", true)
	if err != nil {
		t.Fatalf("SetOverride: %v", err)
	}
	if !row.Enabled || row.App != "Finder" {
		t.Fatalf("SetOverride echo=%+v, want the stored Finder override enabled", row)
	}

	// Zone edits: an area-less rectangle is rejected, the set is unchanged.
	if _, err := app.SetZones([]ZoneRow{{ID: "left", Name: "Left", W: 0, H: 1}}); err == nil {
		t.Fatal("zero-width zone must fail closed")
	}
	for i := range c.zones {
		if c.zones[i] != zones[i] {
			t.Fatalf("a rejected zone edit touched the stored set: %+v", c.zones)
		}
	}
	// An empty set is a destructive edit, and the daemon accepts one:
	// winlayout.ValidateZones returns nil for a zero-length slice and
	// config.setZones is full-replace. So this asserts the CLEAR lands (the
	// contract settings_test.go:92 and pagedata_test.go pin), and the guard
	// against the page firing it by accident belongs in the editor, where Save
	// stays disabled until there is a draft.
	if n, err := app.SetZones(nil); err != nil || n != 0 {
		t.Fatalf("empty zone set=%d,%v, want the clear to be accepted", n, err)
	}
	if len(c.zones) != 0 {
		t.Fatalf("stored zones=%v after clearing, want none", c.zones)
	}
	ok := []ZoneRow{{ID: "left", Name: "Left half", X: 0, Y: 0, W: 0.5, H: 1}, {ID: "top", Name: "Top", X: 0, Y: 0, W: 1, H: 0.25}}
	if n, err := app.SetZones(ok); err != nil || n != 2 {
		t.Fatalf("SetZones=%d,%v, want 2", n, err)
	}
	if rows, err := app.GetZones(); err != nil || len(rows) != 2 {
		t.Fatalf("GetZones after write=%v,%v, want 2", rows, err)
	}
}

// TestServiceExposesFrozenSources pins the Wails-bound names. The frontend
// calls these by string, so a rename is a runtime TypeError in the window —
// and nothing in app/backend would otherwise notice.
//
// The list is EXHAUSTIVE in both directions. A name added here without a
// binding is a page calling a method that was never generated, and a binding
// added to service.go without a name here is a method the frontend cannot
// discover — the Wails generator publishes every exported method, so the second
// failure is silent until someone writes the call. Listing every method is what
// makes the next addition a deliberate act: the generator cannot widen the
// frontend API without this test naming what it widened.
func TestServiceExposesFrozenSources(t *testing.T) {
	svc := NewService(NewApp(populatedCore()), NewHost())
	frozen := []string{
		// Page discovery, dashboard and the update/safety controls.
		"Pages", "GetStatus", "TogglePlugin", "ResetEverything", "GetEventLogs",
		"UILogs", "CheckForUpdate", "ApplyUpdate", "PanicStop", "Resume",
		"BeginTrial", "ConfirmTrial", "RollbackTrial", "SetRuleEnabled",
		"Shortcuts", "SetShortcuts",
		// The Explorer's menu table and its per-row toggle.
		"FinderMenu", "SetMenuItemEnabled",
		// The ten page sources.
		"GetMatrix", "GetOverrides", "SetOverride", "GetZones", "SetZones",
		"Commands", "PluginSchemas", "OwnershipAudit", "TrialState", "Readiness",
		// Wave 3: profile cards, decision trace, plugin meta, the app list and
		// the person-authored rule table.
		"Profiles", "ApplyProfile", "Traces", "PluginMeta", "Apps",
		"UserRules", "SetUserRule", "DeleteUserRule",
		// The first-run wizard: its derived state, the one write that
		// latches it, and the one door onto another application — the pane
		// where the Accessibility grant is actually made.
		"OnboardingState", "CompleteOnboarding", "OpenSystemSettings",
		// Observe mode: the write that turns it on, and the read that says
		// where it actually stands. Both, because a toggle that can only be
		// written labels itself from the click instead of the state.
		"SetObserve", "ObserveState",
		// The switcher: its tiles, the long poll that opens it, and the focus a
		// click performs.
		"Windows", "SwitcherWait", "SwitcherFocus",
	}
	declared := map[string]bool{}
	for _, m := range serviceMethodNames(t) {
		declared[m] = true
	}
	listed := map[string]bool{}
	for _, name := range frozen {
		listed[name] = true
		if !declared[name] {
			t.Errorf("Service is missing the frozen bound method %s", name)
		}
	}
	for _, m := range serviceMethodNames(t) {
		if !listed[m] {
			t.Errorf("Service.%s is bound but absent from the frozen list — add it to "+
				"TestServiceExposesFrozenSources or the frontend has no way to call it", m)
		}
	}
	// And the same names must actually delegate, not exist as stubs.
	if rows, err := svc.GetMatrix(); err != nil || len(rows) != 1 {
		t.Fatalf("Service.GetMatrix=%v,%v, want the one served row", rows, err)
	}
	if st, err := svc.TrialState(); err != nil || st.State != "trial" {
		t.Fatalf("Service.TrialState=%+v,%v, want the in-flight trial", st, err)
	}
	if ready, err := svc.Readiness(); err != nil || len(ready) != 2 {
		t.Fatalf("Service.Readiness=%v,%v, want 2 rows", ready, err)
	}
	// A Wave 3 name delegates the same way: a real error out of Core, logged.
	if _, err := svc.ApplyProfile("ghost-profile"); err == nil {
		t.Fatal("Service.ApplyProfile must surface the daemon rejection")
	}
	if _, err := svc.SetOverride("Finder", "nope", true); err == nil {
		t.Fatal("Service.SetOverride must surface the daemon rejection")
	}
	if logs := svc.UILogs(); len(logs) != 2 {
		t.Fatalf("both denials must reach the UI log, got %v", logs)
	}
	// The switcher bindings delegate too, and the wait's "nothing happened" is
	// an answer that travels back rather than a failure. Windows reads the
	// stub's nil list, which is the null case the list rule above covers, so
	// this is the last assertion and the UI log is not counted again.
	if rows, err := svc.Windows(); err != nil || len(rows) != 0 {
		t.Fatalf("Service.Windows=%v,%v, want the empty list the daemon owes", rows, err)
	}
	if trig, err := svc.SwitcherWait(2000); err != nil || trig.Triggered {
		t.Fatalf("Service.SwitcherWait=%+v,%v, want an untriggered answer", trig, err)
	}
	if err := svc.SwitcherFocus("412"); err != nil {
		t.Fatalf("Service.SwitcherFocus: %v", err)
	}
}

// serviceMethodNames returns the exported methods declared on Service in
// service.go, read with go/ast so the list comes from the source the Wails
// generator reads — not from a hand-kept copy that could rot.
func serviceMethodNames(t *testing.T) []string {
	t.Helper()
	// go test runs the binary with the package directory as its working
	// directory, so service.go is the file next to this test.
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "service.go", nil, 0)
	if err != nil {
		t.Fatalf("parse service.go: %v", err)
	}
	var out []string
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || len(fn.Recv.List) == 0 || !fn.Name.IsExported() {
			continue
		}
		if receiverTypeName(fn.Recv.List[0].Type) == "Service" {
			out = append(out, fn.Name.Name)
		}
	}
	sort.Strings(out)
	return out
}

func receiverTypeName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.StarExpr:
		return receiverTypeName(e.X)
	case *ast.Ident:
		return e.Name
	case *ast.IndexExpr: // generic receiver, unused today
		return receiverTypeName(e.X)
	}
	return ""
}

// TestBindingsShimParity keeps the Go service and the generated TypeScript
// bindings in agreement. `wails3 generate bindings ./...` writes JavaScript
// (service.js), not declarations, so this reads what the generator actually
// produces: reading a hand-written .d.ts here meant the test skipped on every
// machine that had ever generated, and the Go↔TypeScript contract was
// permanently unverified.
func TestBindingsShimParity(t *testing.T) {
	goMethods := serviceMethodNames(t)
	if len(goMethods) == 0 {
		t.Fatal("no exported Service methods parsed out of service.go")
	}
	generated := filepath.Join("..", "frontend", "bindings", "crossos", "app", "backend", "service.js")
	raw, err := os.ReadFile(generated)
	if err != nil {
		t.Skipf("bindings %s are not generated yet (%v) — run `wails3 generate bindings ./...` in app/ and re-run; until then the Go↔TypeScript method contract is unverified", generated, err)
	}
	declared := generatedServiceMethods(t, string(raw))
	for _, m := range goMethods {
		if !declared[m] {
			t.Errorf("Service.%s is exported in Go but missing from %s", m, generated)
		}
	}
	for m := range declared {
		if !containsString(goMethods, m) {
			t.Errorf("%s declares Service.%s, which service.go does not export", generated, m)
		}
	}
}

// generatedServiceMethods extracts the bound method names the generator wrote
// into service.js. It fails loudly on an empty match: a scan that found nothing
// would let the parity test pass for exactly the wrong reason — which is the
// state it was in when it read a .d.ts the generator had already deleted.
func generatedServiceMethods(t *testing.T, src string) map[string]bool {
	t.Helper()
	exportRe := regexp.MustCompile(`(?m)^export function ([A-Za-z_$][\w$]*)\s*\(`)
	out := map[string]bool{}
	for _, m := range exportRe.FindAllStringSubmatch(src, -1) {
		out[m[1]] = true
	}
	if len(out) == 0 {
		t.Fatalf("no `export function Name(` declarations in the generated service:\n%s", excerpt(src))
	}
	return out
}

func containsString(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// wireModels is every Go struct the generator turns into a TypeScript model,
// paired with the shape the shell's frontend reads it as. The Wave 3 rows are
// here for the same reason the ten are: a model the generator did not emit, or
// emitted with a property the daemon does not send, is a row of `undefined` in
// the window.
var wireModels = []struct {
	name string
	row  any
}{
	{"MatrixRow", MatrixRow{}},
	{"OverrideRow", OverrideRow{}},
	{"ZoneRow", ZoneRow{}},
	{"CommandRow", CommandRow{}},
	{"SchemaRow", SchemaRow{}},
	{"AuditRow", AuditRow{}},
	{"TrialState", TrialState{}},
	{"ReadinessRow", ReadinessRow{}},
	{"OnboardingRow", OnboardingRow{}},
	{"OnboardingStep", OnboardingStep{}},
	{"ProfileRow", ProfileRow{}},
	{"ProfileCapabilityRow", ProfileCapabilityRow{}},
	{"TraceRow", TraceRow{}},
	{"TraceEvent", TraceEvent{}},
	{"TraceContext", TraceContext{}},
	{"StageRow", StageRow{}},
	{"PluginMetaRow", PluginMetaRow{}},
	{"AppRow", AppRow{}},
	{"UserRuleRow", UserRuleRow{}},
	{"WindowRow", WindowRow{}},
	{"SwitcherTrigger", SwitcherTrigger{}},
	{"Status", Status{}},
	{"PluginState", PluginState{}},
	{"Page", Page{}},
}

// TestAppRowMatchesTheDaemonsRow pins the one row with no json tags. The
// daemon serves its app list as ctx.ApplicationInfo, so the wire keys ARE that
// struct's Go field names — and the shell's AppRow is a hand copy of it. This
// is the test that keeps the copy honest: a field added to the daemon's row
// (or a json tag added there) fails here rather than decoding to a column the
// picker never fills.
func TestAppRowMatchesTheDaemonsRow(t *testing.T) {
	daemon := reflect.TypeOf(ctx.ApplicationInfo{})
	got := wireFieldNames(AppRow{})
	if len(got) != daemon.NumField() {
		t.Fatalf("AppRow has %d fields %v, ctx.ApplicationInfo has %d — the picker's "+
			"row and the daemon's row must be the same row", len(got), got, daemon.NumField())
	}
	for i := range got {
		if want := daemon.Field(i).Name; got[i] != want {
			t.Errorf("AppRow field %d is %q, ctx.ApplicationInfo field %d is %q — the daemon "+
				"sends the Go field name, so a picker column keyed on %q reads empty",
				i, got[i], i, want, want)
		}
	}
	// And the two must keep the same JSON kinds, not just the same names: a
	// field the shell declares as a string where the daemon sends a number
	// decodes to the zero value instead of failing. The comparison is on Kind
	// rather than Type because two of the daemon's fields are NAMED string
	// types (ctx.AppMode, ctx.AppCategory) and the shell declares the bare
	// kind, which is the same JSON string to a page and the same one in a
	// picker.
	for i := 0; i < daemon.NumField(); i++ {
		f := reflect.TypeOf(AppRow{}).Field(i)
		if want := daemon.Field(i).Type.Kind(); f.Type.Kind() != want {
			t.Errorf("AppRow.%s is a %s, ctx.ApplicationInfo.%s is a %s", f.Name, f.Type.Kind(), daemon.Field(i).Name, want)
		}
	}
}

// TestWindowRowMatchesTheDaemonsRow pins the switcher row the way the app row
// is pinned, against the struct the daemon really serves. core.windows answers
// with []switcherRow, and that type lives in package main — which this module
// cannot import — so the declaration is read out of the daemon's source with
// go/ast. Same guarantee, different door: a field renamed, added or dropped on
// the daemon side fails here rather than arriving as a column the switcher has
// nowhere to put.
func TestWindowRowMatchesTheDaemonsRow(t *testing.T) {
	path := filepath.Join("..", "..", "core", "cmd", "crossos", "switcher.go")
	daemon := daemonStructFields(t, path, "switcherRow")
	shell := reflect.TypeOf(WindowRow{})
	got := wireFieldNames(WindowRow{})
	if len(got) != len(daemon) {
		t.Fatalf("WindowRow has %d fields %v, the daemon's switcherRow has %d — the tile the shell "+
			"draws and the tile the daemon serves must be the same row", len(got), got, len(daemon))
	}
	for i := range daemon {
		if got[i] != daemon[i].tag {
			t.Errorf("WindowRow field %d marshals as %q, the daemon's switcherRow field %d sends %q — "+
				"the switcher would render an empty column", i, got[i], i, daemon[i].tag)
		}
		// The kinds too: a field the shell declares as a string where the daemon
		// sends a number decodes to the zero value instead of failing, which is
		// the same silent-empty column a wrong name produces.
		if kind := shell.Field(i).Type.Kind().String(); kind != daemon[i].kind {
			t.Errorf("WindowRow field %d is a %s, the daemon's switcherRow field %d is a %s", i, kind, i, daemon[i].kind)
		}
	}
}

// TestOnboardingRowsMatchTheDaemonsRows pins the first-run wizard's two rows
// against the structs the daemon really serves, the way the switcher row is
// pinned — same door, because both structs live in package main, which this
// module cannot import. onboardingRow is read out of pagedata.go because that
// is the file that declares it.
//
// The tags are the whole point: a wizard whose `current_step` decodes to an
// empty string renders every step as current, which is a page that cannot be
// dismissed and gives the user nothing to act on. The kinds are compared too,
// because a field the shell declares as a string where the daemon sends a
// number decodes to the zero value instead of failing.
func TestOnboardingRowsMatchTheDaemonsRows(t *testing.T) {
	path := filepath.Join("..", "..", "core", "cmd", "crossos", "pagedata.go")
	for _, tc := range []struct {
		name   string
		daemon string
		shell  any
	}{
		{"OnboardingRow", "onboardingRow", OnboardingRow{}},
		{"OnboardingStep", "onboardingStep", OnboardingStep{}},
	} {
		daemon := daemonStructFields(t, path, tc.daemon)
		typ := reflect.TypeOf(tc.shell)
		got := wireFieldNames(tc.shell)
		if len(got) != len(daemon) {
			t.Errorf("%s has %d fields %v, the daemon's %s has %d — the row the shell "+
				"renders and the row the daemon serves must be the same row",
				tc.name, len(got), got, tc.daemon, len(daemon))
			continue
		}
		for i := range daemon {
			if got[i] != daemon[i].tag {
				t.Errorf("%s field %d marshals as %q, the daemon's %s field %d sends %q — "+
					"the wizard would render an empty field", tc.name, i, got[i], tc.daemon, i, daemon[i].tag)
			}
			if kind := wireKindName(typ.Field(i).Type); kind != daemon[i].kind {
				t.Errorf("%s field %d is a %s, the daemon's %s field %d is a %s",
					tc.name, i, kind, tc.daemon, i, daemon[i].kind)
			}
		}
	}
}

// wireKindName spells a Go type the way daemonFieldKind spells the daemon's, so
// the two sides of the module boundary are compared on JSON shape: a slice on
// one side is a slice on the other, and a nested row on one side is a row on
// the other, whatever the two modules call the type.
func wireKindName(t reflect.Type) string {
	switch t.Kind() {
	case reflect.Slice:
		return "[]" + wireKindName(t.Elem())
	case reflect.Struct:
		return "row"
	}
	return t.Kind().String()
}

// daemonField is one field of a struct read out of a daemon source file: the
// tag is the key the daemon sends, the kind is the JSON type it arrives as.
type daemonField struct {
	tag  string
	kind string
}

// daemonStructFields reads one struct declaration out of a file this module
// cannot compile against. It fails loudly for a struct it cannot find, an
// untagged field or a type it cannot name: an empty result would let the pin
// above pass for exactly the wrong reason.
func daemonStructFields(t *testing.T, path, name string) []daemonField {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	for _, d := range f.Decls {
		gen, ok := d.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok || ts.Name.Name != name {
				continue
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				t.Fatalf("%s: %s is not a struct", path, name)
			}
			out := make([]daemonField, 0, len(st.Fields.List))
			for _, field := range st.Fields.List {
				if field.Tag == nil {
					t.Fatalf("%s: %s declares an untagged field, and the wire is the tag", path, name)
				}
				tag, err := strconv.Unquote(field.Tag.Value)
				if err != nil {
					t.Fatalf("%s: %s tag %s: %v", path, name, field.Tag.Value, err)
				}
				// The wire key is the name inside the json tag, so the tag key
				// itself is read here rather than assumed: renaming it is as
				// much a break as renaming the name.
				wire, ok := strings.CutPrefix(tag, "json:")
				if !ok {
					t.Fatalf("%s: %s tags a field %q, which this reader does not parse — extend it "+
						"before pinning the row", path, name, tag)
				}
				key, _, _ := strings.Cut(wire, ",")
				kind := daemonFieldKind(t, path, name, field.Type)
				for range field.Names {
					out = append(out, daemonField{tag: strings.Trim(key, `"`), kind: kind})
				}
			}
			return out
		}
	}
	t.Fatalf("%s declares no type %s", path, name)
	return nil
}

// wireScalarNames are the type names a row field can carry that JSON encodes as
// a scalar. Any other ident names a nested row, which the two modules spell
// differently on purpose (onboardingStep here, OnboardingStep in the shell), so
// it compares as "row" and the json tags do the real work.
var wireScalarNames = map[string]bool{
	"string": true, "bool": true, "int": true, "int64": true,
	"uint32": true, "uint64": true, "float64": true, "any": true,
}

// daemonFieldKind names the JSON kind a daemon field's type marshals as. Only
// the bare type names and slices of them are known, and anything else stops the
// test rather than passing a comparison it did not make.
//
// A slice reports "[]" plus its element's kind, so the two modules can be
// compared on shape without the shell having to reuse the daemon's own type
// names.
func daemonFieldKind(t *testing.T, path, name string, expr ast.Expr) string {
	t.Helper()
	switch e := expr.(type) {
	case *ast.Ident:
		if wireScalarNames[e.Name] {
			return e.Name
		}
		return "row"
	case *ast.ArrayType:
		if _, ok := e.Len.(*ast.Ident); ok {
			// [N]T is a fixed-size array, which JSON marshals as an array but
			// which no row here declares; naming it would blur that.
			t.Fatalf("%s: %s declares a fixed-size array, which this reader does not "+
				"pin — extend it before pinning the row", path, name)
		}
		return "[]" + daemonFieldKind(t, path, name, e.Elt)
	}
	t.Fatalf("%s: %s declares a field of type %T, which this reader does not name — extend it "+
		"before pinning the row", path, name, expr)
	return ""
}

// TestGeneratedModelsMatchWireTags is the test that would have caught the
// PascalCase row types. Wails generates a model property from the Go json tag
// when the field has one and from the Go field name when it does not — the same
// rule encoding/json applies, because that is what actually marshals the
// result. Asserting it here means the generated models.js, the daemon's wire
// and the shell's decode target cannot disagree: a tag added or renamed on
// either side of the module boundary shows up as a test failure instead of as
// `undefined` in a settings row.
func TestGeneratedModelsMatchWireTags(t *testing.T) {
	generated := filepath.Join("..", "frontend", "bindings", "crossos", "app", "backend", "models.js")
	raw, err := os.ReadFile(generated)
	if err != nil {
		t.Skipf("bindings %s are not generated yet (%v) — run `wails3 generate bindings ./...` in app/ and re-run; until then the model field contract is unverified", generated, err)
	}
	src := string(raw)
	for _, model := range wireModels {
		want := wireFieldNames(model.row)
		if len(want) == 0 {
			t.Fatalf("%s has no exported fields to compare", model.name)
		}
		got := generatedModelFields(t, src, model.name)
		if len(got) != len(want) {
			t.Errorf("%s: generator emitted %d properties %v, the Go struct has %d %v",
				model.name, len(got), got, len(want), want)
			continue
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("%s property %d: generator emits %q, the Go struct marshals %q — the frontend would read undefined",
					model.name, i, got[i], want[i])
			}
		}
	}
}

// wireFieldNames is what encoding/json (and therefore the generator) calls each
// field: the tag name when a tag is present, the Go field name when it is not.
func wireFieldNames(row any) []string {
	v := reflect.Indirect(reflect.ValueOf(row))
	t := v.Type()
	out := make([]string, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.PkgPath != "" {
			continue
		}
		name := strings.Split(f.Tag.Get("json"), ",")[0]
		if name == "-" {
			continue
		}
		if name == "" {
			name = f.Name
		}
		out = append(out, name)
	}
	return out
}

// generatedModelFields reads one model's property names out of the generated
// JavaScript, in declaration order. It fails loudly rather than returning
// nothing: an empty match is indistinguishable from "the model is gone" and
// would let this test pass for the wrong reason.
func generatedModelFields(t *testing.T, src, class string) []string {
	t.Helper()
	start := strings.Index(src, "export class "+class+" {")
	if start < 0 {
		t.Fatalf("the generated models declare no `export class %s`", class)
	}
	rest := src[start:]
	if next := strings.Index(rest[1:], "\nexport class "); next >= 0 {
		rest = rest[:next+1]
	}
	fieldRe := regexp.MustCompile(`this\["([^"]+)"\]\s*=`)
	out := fieldRe.FindAllStringSubmatch(rest, -1)
	names := make([]string, 0, len(out))
	for _, m := range out {
		names = append(names, m[1])
	}
	if len(names) == 0 {
		t.Fatalf("`export class %s` declares no `this[\"field\"] =` properties", class)
	}
	return names
}

// excerpt keeps a failure message readable when it has to dump a bindings file.
func excerpt(src string) string {
	const limit = 1200
	if len(src) <= limit {
		return src
	}
	return src[:limit] + "\n… (truncated)"
}
