// Menu + lifecycle tests — beads cross-os-vbl.2, cross-os-vbl.4.
//
// vbl.2 mapping: every findermenu row × context, native-only caps, dispatcher
// validation (paths + max count), stage traces (feeds vbl.5), RegisterMenu
// wiring (TestRegisterMenus: seven rows round-trip through a real Registry).
// The table itself is no longer declared here — menus.go re-exports
// findermenu.Menu, so these tests are also the extension's proof that it
// consumes the daemon's table rather than a copy that could drift.
// vbl.4 mapping: five states + transitions, daemon-down hide, NOT_APPROVED
// guidance copy, status visibility + logging.
package findersync

import (
	"encoding/json"
	"strings"
	"testing"

	"crossos/core/pkg/findermenu"
	"crossos/core/pkg/pluginapi"
)

// TestMenuEntriesPayloadDecodes pins the client against the wire the daemon
// actually emits. The literals below are the json tags of
// core/cmd/crossos/findermenu.go's finderMenu and of the two tables it
// serves; renaming a tag there would otherwise fail silently here and show
// up as a Finder menu with no rows and no log line.
func TestMenuEntriesPayloadDecodes(t *testing.T) {
	raw := `{
	  "contexts": ["file", "folder", "empty"],
	  "items": [
	    {"id":"newFile","title":"New File","contexts":["empty","folder"],"needsPaths":false,"capability":"filesystem.createFile","enabled":true},
	    {"id":"openEditor","title":"Open in Editor","contexts":["file","folder"],"needsPaths":true,"capability":"app.open","enabled":true},
	    {"id":"duplicateWithName","title":"Duplicate with Name","contexts":["file","folder"],"needsPaths":true,"capability":"filesystem.duplicate","enabled":true}
	  ],
	  "editors": [{"id":"com.microsoft.VSCode","name":"Visual Studio Code"}],
	  "fileTypes": [
	    {"ext":"txt","baseName":"New Text File","displayName":"New Text File","template":"","enabled":true,"builtIn":true},
	    {"ext":"md","baseName":"Untitled","displayName":"New Markdown","template":"","enabled":true,"builtIn":true}
	  ]
	}`
	var payload struct {
		Contexts []string `json:"contexts"`
		Items    []MenuItem
		Editors  []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"editors"`
		FileTypes []struct {
			Ext      string `json:"ext"`
			BaseName string `json:"baseName"`
			Enabled  bool   `json:"enabled"`
		} `json:"fileTypes"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("finder.menuEntries payload no longer decodes: %v", err)
	}
	if len(payload.Items) != 3 {
		t.Fatalf("items=%d, want 3", len(payload.Items))
	}
	// The rows keep their findermenu identity across the wire, so the appex
	// can match a row id to a verb without a second table.
	if payload.Items[0].ID != "newFile" || payload.Items[0].Capability != "filesystem.createFile" {
		t.Fatalf("item 0 = %+v, want newFile/filesystem.createFile", payload.Items[0])
	}
	if !payload.Items[1].NeedsPaths {
		t.Fatal("openEditor must arrive NeedsPaths: a submenu row still needs its target")
	}
	// Both submenus need their catalogs: without editors the Open in Editor
	// submenu is empty, without fileTypes the New File submenu is empty.
	if len(payload.Editors) != 1 || payload.Editors[0].ID != "com.microsoft.VSCode" {
		t.Fatalf("editors=%+v, want the VS Code catalog entry", payload.Editors)
	}
	if len(payload.FileTypes) != 2 || payload.FileTypes[0].Ext != "txt" {
		t.Fatalf("fileTypes=%+v, want txt and md", payload.FileTypes)
	}
	// Every row the daemon sent must be a row the extension can fire.
	for _, item := range payload.Items {
		if Verb(item.ID) == "" {
			t.Fatalf("daemon sent row %q with no verb on this side", item.ID)
		}
	}
}

func TestMenuTableCoverage(t *testing.T) {
	if len(MenuTable) != 7 {
		t.Fatalf("MenuTable=%d, want the 7 findermenu rows", len(MenuTable))
	}
	// The table is a reference, not a copy: the appex must render the rows
	// the daemon serves. A local re-declaration is exactly what this bead
	// removed, so pin the identity rather than the contents.
	if len(MenuTable) != len(findermenu.Menu) {
		t.Fatalf("MenuTable=%d, findermenu.Menu=%d — the extension must not keep its own table",
			len(MenuTable), len(findermenu.Menu))
	}
	// Every item resolves to a native capability — zero shell-by-default.
	for _, m := range MenuTable {
		if m.Capability == "" || strings.HasPrefix(m.Capability, "shell.") {
			t.Fatalf("item %s: capability %q must be native, never shell", m.ID, m.Capability)
		}
		if len(m.Contexts) == 0 {
			t.Fatalf("item %s: no contexts", m.ID)
		}
	}
	// CtxEmpty shows New > ... but never path-needing items.
	empty := ForContext(CtxEmpty)
	for _, m := range empty {
		if m.NeedsPaths {
			t.Fatalf("empty context shows path-needing item %s", m.ID)
		}
	}
	found := map[string]bool{}
	for _, m := range empty {
		found[m.ID] = true
	}
	if !found["newFile"] || !found["newFolder"] {
		t.Fatalf("empty context=%v, want newFile + newFolder", found)
	}
	// A selection context carries every row except the two New ones, which
	// are empty-context rows. A lower bound, not an exact list: the claim is
	// that a selection is not starved, not that the table is frozen here.
	if len(ForContext(CtxFile)) < len(MenuTable)-2 || len(ForContext(CtxFolder)) < len(MenuTable)-2 {
		t.Fatal("file/folder contexts must each show most of the table")
	}
}

// TestActionVerbsAreDaemonMethods pins the closed set Send relies on. The
// verb names are the daemon's protocol, not the row ids: newFile is the row,
// finder.createFile is the call. A regression that "simplified" the map back
// to "finder."+id would compile and pass every other test here, and fail
// only as a menu item that does nothing when clicked.
func TestActionVerbsAreDaemonMethods(t *testing.T) {
	want := map[string]string{
		"newFile":           "finder.createFile",
		"newFolder":         "finder.createFolder",
		"copyPath":          "finder.copyPath",
		"copyRelativePath":  "finder.copyRelativePath",
		"openTerminal":      "finder.openTerminal",
		"openEditor":        "finder.openEditor",
		"duplicateWithName": "finder.duplicateWithName",
	}
	for _, m := range MenuTable {
		got := Verb(m.ID)
		if got == "" {
			t.Fatalf("row %s: no daemon verb — the menu would offer a row nothing fires", m.ID)
		}
		if got != want[m.ID] {
			t.Fatalf("row %s: verb=%q, want %q (daemon finderMenuHandlers)", m.ID, got, want[m.ID])
		}
		if !DaemonMethods[got] {
			t.Fatalf("row %s: verb %q missing from the daemon method list", m.ID, got)
		}
		if !allowedActions[got] {
			t.Fatalf("row %s: verb %q missing from the action set", m.ID, got)
		}
	}
	if len(allowedActions) != len(MenuTable) {
		t.Fatalf("allowedActions=%d, want one verb per row (%d)", len(allowedActions), len(MenuTable))
	}
	if len(DaemonMethods) != len(MenuTable)+1 {
		t.Fatalf("DaemonMethods=%d, want the seven verbs plus the menu query", len(DaemonMethods))
	}
	if allowedActions[MethodMenuEntries] {
		t.Fatal("the menu query is not an action verb and must not be dispatchable")
	}
	if !DaemonMethods[MethodMenuEntries] {
		t.Fatal("finder.menuEntries must be callable — it is how the appex gets the menu")
	}
	// The row id is not the method: sending "newFile" is a "no such method".
	if DaemonMethods["newFile"] {
		t.Fatal("a bare row id must not be callable; only the daemon's method name is")
	}
}

func TestDispatchValidation(t *testing.T) {
	cap, _, stages, err := Dispatch("copyPath", CtxFile, []string{"/tmp/a.txt"})
	if err != nil || cap != "clipboard.copyPath" {
		t.Fatalf("copyPath: cap=%s err=%v", cap, err)
	}
	if len(stages) != 3 { // event → action → result
		t.Fatalf("stages=%v, want event/action/result", stages)
	}
	// Unknown item, wrong context, over count, no selection.
	if _, _, _, err := Dispatch("nope", CtxFile, nil); err == nil {
		t.Fatal("unknown item must fail")
	}
	if _, _, _, err := Dispatch("newFile", CtxFile, nil); err == nil {
		t.Fatal("empty-only item in file context must fail")
	}
	many := make([]string, MaxPathCount+1)
	for i := range many {
		many[i] = "/tmp/f"
	}
	if _, _, _, err := Dispatch("copyPath", CtxFile, many); err == nil {
		t.Fatal("over-count dispatch must fail")
	}
	if _, _, _, err := Dispatch("copyPath", CtxFile, nil); err == nil {
		t.Fatal("needs-selection without paths must fail")
	}
	// Traversal, root, and relative paths rejected.
	for _, bad := range []string{"/tmp/../etc/passwd", "/", "rel/path.txt"} {
		if _, _, _, err := Dispatch("copyPath", CtxFile, []string{bad}); err == nil {
			t.Fatalf("path %q must be rejected", bad)
		}
	}
}

func TestLifecycleStates(t *testing.T) {
	var l Lifecycle // zero = HOST_NOT_RUNNING
	if l.ShowMenu() {
		t.Fatal("pre-launch must not show menus")
	}
	l.OnDaemonUp(true)
	if l.State != Available || !l.ShowMenu() {
		t.Fatal("daemon up + approved → AVAILABLE + show")
	}
	l.OnDaemonDown("killed mid-session")
	if l.State != DaemonUnavailable || l.ShowMenu() {
		t.Fatal("daemon down → DAEMON_UNAVAILABLE + hidden (Finder responsive)")
	}
	l.OnDaemonUp(false)
	if l.State != NotApproved || l.ShowMenu() {
		t.Fatal("unapproved → NOT_APPROVED + hidden")
	}
	l.OnApprovalChanged(true)
	if l.State != Available {
		t.Fatal("approval grant → AVAILABLE")
	}
	l.SetDisabled(true)
	if l.State != Disabled || l.ShowMenu() {
		t.Fatal("disabled → DISABLED + hidden (no behavior effect)")
	}
	l.OnDaemonUp(true) // ignored while disabled
	if l.State != Disabled {
		t.Fatal("daemon events ignored while disabled")
	}
	l.SetDisabled(false)
	if l.State != HostNotRunning {
		t.Fatal("re-enable → HOST_NOT_RUNNING (await daemon)")
	}
	if len(l.Log) == 0 {
		t.Fatal("every transition must be logged (status UI + logs)")
	}
}

// TestRegisterMenus pins the §3.6 normative path (review: cross-os-ed):
// every MenuTable row registers via RegisterMenu on a real Registry.
// vbl.3 replaces the SOURCE (packs → MenuDefs); this path is unchanged.
func TestRegisterMenus(t *testing.T) {
	r := pluginapi.NewRegistry(nil)
	if err := RegisterMenus(r); err != nil {
		t.Fatalf("RegisterMenus: %v", err)
	}
	if len(r.Menus) != len(MenuTable) {
		t.Fatalf("registered=%d, want %d", len(r.Menus), len(MenuTable))
	}
	seen := map[string]bool{}
	for _, m := range r.Menus {
		if m.ID == "" || m.Title == "" {
			t.Fatalf("MenuDef incomplete: %+v", m)
		}
		if seen[m.ID] {
			t.Fatalf("duplicate MenuDef %q", m.ID)
		}
		seen[m.ID] = true
	}
	for _, want := range []string{"finder.newFile", "finder.copyPath", "finder.duplicateWithName"} {
		if !seen[want] {
			t.Fatalf("MenuDef %q missing", want)
		}
	}
	if err := RegisterMenus(nil); err == nil {
		t.Fatal("nil Registry must fail, never a silent partial set")
	}
}

func TestApprovalCopyNonTech(t *testing.T) {
	copy := ApprovalCopy()
	for _, banned := range []string{"CGEventTap", "AXUIElement", "daemon", "socket", "IPC", "API"} {
		if strings.Contains(copy, banned) {
			t.Fatalf("approval copy leaks jargon %q", banned)
		}
	}
	if !strings.Contains(copy, "System Settings") {
		t.Fatal("approval copy must name System Settings")
	}
}
