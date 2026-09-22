// Menu + lifecycle tests — beads cross-os-vbl.2, cross-os-vbl.4.
//
// vbl.2 mapping: all §6.3 items × contexts, native-only caps, dispatcher
// validation (paths + max count), stage traces (feeds vbl.5), RegisterMenu
// wiring (TestRegisterMenus: 12 rows round-trip through a real Registry).
// vbl.4 mapping: five states + transitions, daemon-down hide, NOT_APPROVED
// guidance copy, status visibility + logging.
package findersync

import (
	"strings"
	"testing"

	"crossos/core/pkg/pluginapi"
)

func TestMenuTableCoverage(t *testing.T) {
	if len(MenuTable) != 12 {
		t.Fatalf("MenuTable=%d, want 12 §6.3 items", len(MenuTable))
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
	if !found["newText"] || !found["newFolder"] {
		t.Fatalf("empty context=%v, want newText + newFolder", found)
	}
	if len(ForContext(CtxFile)) < 8 || len(ForContext(CtxFolder)) < 8 {
		t.Fatal("file/folder contexts must each show most of the table")
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
	if _, _, _, err := Dispatch("newText", CtxFile, nil); err == nil {
		t.Fatal("empty-only item in file context must fail")
	}
	many := make([]string, MaxPathCount+1)
	for i := range many {
		many[i] = "/tmp/f"
	}
	if _, _, _, err := Dispatch("copy", CtxFile, many); err == nil {
		t.Fatal("over-count dispatch must fail")
	}
	if _, _, _, err := Dispatch("copy", CtxFile, nil); err == nil {
		t.Fatal("needs-selection without paths must fail")
	}
	// Traversal, root, and relative paths rejected.
	for _, bad := range []string{"/tmp/../etc/passwd", "/", "rel/path.txt"} {
		if _, _, _, err := Dispatch("trash", CtxFile, []string{bad}); err == nil {
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
// all 12 MenuTable rows register via RegisterMenu on a real Registry.
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
	for _, want := range []string{"finder.newText", "finder.trash", "finder.compress"} {
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
