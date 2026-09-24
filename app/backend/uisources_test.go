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
//     → TestBindingsShimParity, TestGeneratedModelsMatchWireTags.
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
	"strings"
	"testing"
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

// sourceReader names one bridge call so the null and failure rules below can
// be asserted once for every source instead of per method.
type sourceReader struct {
	name string
	read func(*App) error
}

// sourceReaders are the list-shaped sources.
func sourceReaders() []sourceReader {
	return []sourceReader{
		{"GetMatrix", func(a *App) error { _, err := a.GetMatrix(); return err }},
		{"GetOverrides", func(a *App) error { _, err := a.GetOverrides(); return err }},
		{"GetZones", func(a *App) error { _, err := a.GetZones(); return err }},
		{"Commands", func(a *App) error { _, err := a.Commands(); return err }},
		{"PluginSchemas", func(a *App) error { _, err := a.PluginSchemas(); return err }},
		{"OwnershipAudit", func(a *App) error { _, err := a.OwnershipAudit(); return err }},
		{"Readiness", func(a *App) error { _, err := a.Readiness(); return err }},
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
func TestServiceExposesFrozenSources(t *testing.T) {
	svc := NewService(NewApp(populatedCore()), NewHost())
	frozen := []string{
		"GetMatrix", "GetOverrides", "SetOverride", "GetZones", "SetZones",
		"Commands", "PluginSchemas", "OwnershipAudit", "TrialState", "Readiness",
	}
	declared := map[string]bool{}
	for _, m := range serviceMethodNames(t) {
		declared[m] = true
	}
	for _, name := range frozen {
		if !declared[name] {
			t.Errorf("Service is missing the frozen bound method %s", name)
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
	if _, err := svc.SetOverride("Finder", "nope", true); err == nil {
		t.Fatal("Service.SetOverride must surface the daemon rejection")
	}
	if len(svc.UILogs()) == 0 {
		t.Fatal("Service.SetOverride denial must reach the UI log")
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
// paired with the shape the shell's frontend reads it as.
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
	{"Status", Status{}},
	{"PluginState", PluginState{}},
	{"Page", Page{}},
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
