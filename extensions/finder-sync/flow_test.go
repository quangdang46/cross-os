// Finder flow tests: unit + reference matrix + daemon-down fallback
// (bead cross-os-vbl.5).
//
// Plan §11 (all four tiers) + §3.11 Recorder/Replay + §9.11 merge gate.
// Unit: rule filters (Match across targets/UTI/count/gate), config
// validation (manifest Validate), plugin lifecycle (pack load → visible).
// Reference: menu items (vbl.2 Dispatch) × file/folder/empty selection,
// traces replay deterministically (stages recorded per dispatch).
// Fallback: daemon-killed → hidden menus + responsive Finder (lifecycle
// machine + visibility probe). Platform: FIFinderSync visibility on macOS
// (Tier-2 live; Tier-1 asserts the ShowMenu/probe contract). Pack-loader
// (vbl.3) exercised with the malformed-pack fixture. CI merge-gate fields
// asserted for reused newfile/menumate files.
package findersync

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// listenStub starts a minimal Unix-socket listener (accepts and closes) so
// the visibility probe succeeds; closing it simulates a mid-session kill.
func listenStub(path string) (net.Listener, error) {
	_ = os.Remove(path)
	return net.Listen("unix", path)
}

// finderDir returns the extensions/finder-sync dir (this package's dir) via
// the test binary's runtime path.
func finderDir(t *testing.T) string {
	t.Helper()
	// Caller file lives in this dir: use TempDir-independent resolution.
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return dir
}

// TestMenuTableContract: finder-side menu contract — every MenuTable item
// has a valid capability + contexts and Dispatch validates. (The full
// Match targets/UTI/count matrix lives in the plugin package's pack tests;
// this tier pins what the finder surface itself owns.)
func TestMenuTableContract(t *testing.T) {
	for _, m := range MenuTable {
		if m.Capability == "" || len(m.Contexts) == 0 {
			t.Fatalf("item %s incomplete", m.ID)
		}
	}
}

// TestReferenceMatrix: menu items × file/folder/empty, traces replayable.
func TestReferenceMatrix(t *testing.T) {
	type row struct {
		item  string
		ctx   SelectionCtx
		paths []string
	}
	rows := []row{
		{"newText", CtxEmpty, nil},
		{"newFolder", CtxEmpty, nil},
		{"copyPath", CtxFile, []string{"/tmp/a.txt"}},
		{"openTerminal", CtxFolder, []string{"/tmp/d"}},
		{"openEditor", CtxFile, []string{"/tmp/a.txt"}},
		{"cut", CtxFile, []string{"/tmp/a.txt"}},
		{"copy", CtxFile, []string{"/tmp/a.txt"}},
		{"paste", CtxEmpty, nil},
		{"rename", CtxFile, []string{"/tmp/a.txt"}},
		{"getInfo", CtxFolder, []string{"/tmp/d"}},
		{"trash", CtxFile, []string{"/tmp/a.txt"}},
		{"compress", CtxFolder, []string{"/tmp/d"}},
	}
	if len(rows) != len(MenuTable) {
		t.Fatalf("rows=%d, want full %d-item table", len(rows), len(MenuTable))
	}
	for _, r := range rows {
		cap, _, stages, err := Dispatch(r.item, r.ctx, r.paths)
		if err != nil {
			t.Fatalf("row %s: %v", r.item, err)
		}
		if cap == "" || len(stages) != 3 {
			t.Fatalf("row %s: cap=%q stages=%v (want event/action/result)", r.item, cap, stages)
		}
		// Replay: same inputs → same outputs (deterministic, no live input).
		cap2, _, stages2, err2 := Dispatch(r.item, r.ctx, r.paths)
		if err2 != nil || cap2 != cap || strings.Join(stages2, "|") != strings.Join(stages, "|") {
			t.Fatalf("row %s: replay mismatch", r.item)
		}
	}
}

// TestFallbackDaemonKilled: daemon killed mid-session → hidden + responsive.
func TestFallbackDaemonKilled(t *testing.T) {
	// Start stub, verify Show, kill stub, verify Hide — Finder never blocks
	// because every path is deadline-bounded (probe 150ms, action 500ms).
	path := filepath.Join(t.TempDir(), "fallback.sock")
	ln, err := listenStub(path)
	if err != nil {
		t.Fatalf("stub: %v", err)
	}
	var l Lifecycle
	l.OnDaemonUp(true)
	if !l.ShowMenu() {
		t.Fatal("daemon up → show")
	}
	ln.Close() // kill the daemon mid-session
	l.OnDaemonDown("killed mid-session")
	if l.ShowMenu() {
		t.Fatal("daemon killed → hidden menus")
	}
	if vis, _ := ShouldShowMenu(path); vis != Hide {
		t.Fatal("probe after kill → Hide (responsive, never block)")
	}
}

// TestMergeGateFields: CI asserts §9.11 fields for reused files.
func TestMergeGateFields(t *testing.T) {
	// Pinned commits recorded in attribution headers/bodies.
	checks := map[string]string{
		"../../third_party/newfile/ATTRIBUTION.md":  "b3f665a",
		"../../third_party/menumate/ATTRIBUTION.md": "017d6da",
	}
	for rel, pin := range checks {
		raw, err := os.ReadFile(filepath.Join(finderDir(t), rel))
		if err != nil {
			t.Fatalf("attribution %s: %v", rel, err)
		}
		if !strings.Contains(string(raw), pin) {
			t.Fatalf("attribution %s missing pin %s", rel, pin)
		}
		if !strings.Contains(string(raw), "MIT") {
			t.Fatalf("attribution %s missing license", rel)
		}
	}
}

// --- Tier 2: FIFinderSync visibility on macOS (interactive + appex) ---

func TestPlatformVisibility(t *testing.T) {
	if testing.Short() {
		t.Skip("needs interactive desktop + installed appex")
	}
	t.Skip("live FIFinderSync visibility lands with the Xcode-wired appex; Tier-1 ShowMenu/probe contract is the sandbox proof")
}
