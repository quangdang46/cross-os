// Pack tests — bead cross-os-vbl.3.
//
// Pass criteria mapping:
//  1. Manifest + action + rule matching → TestPackParseValidate, TestMatch.
//  2. Path validation, traversal rejected → TestValidatePackPath.
//  3. Undeclared-file inspection → TestInspectPack (incl. malformed-pack
//     fixture: traversal + undeclared files rejected — feeds vbl.5).
//  4. Shell gate → TestShellGate.
package plugin

import (
	"os"
	"path/filepath"
	"testing"
)

const goodManifest = `{
  "schemaVersion": 2,
  "name": "Windows Explorer UX",
  "author": "CrossOS",
  "icon": "shippingbox",
  "futureField": "ignored",
  "actions": [
    {"id": "new-text-file", "title": "New > Text Document",
     "action": {"type": "native", "capability": "filesystem.createFile", "parameters": {"extension": "txt"}},
     "targets": "container", "placement": "submenu"},
    {"id": "copy-path", "title": "Copy as Path",
     "action": {"type": "native", "capability": "clipboard.copyPath"},
     "targets": "any", "placement": "topLevel"},
    {"id": "term-here", "title": "Open in Terminal",
     "action": {"type": "process", "plugin": "dev", "operation": "openTerminal"},
     "targets": ["files", "folders"]}
  ]
}`

func TestPackParseValidate(t *testing.T) {
	m, err := ParsePackManifest([]byte(goodManifest))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if m.Icon != "shippingbox" || m.Name != "Windows Explorer UX" {
		t.Fatalf("defaults/fields: %+v", m)
	}
	if len(m.Actions) != 3 {
		t.Fatalf("actions=%d, want 3", len(m.Actions))
	}
	if m.Actions[0].Placement != "submenu" || m.Actions[1].Placement != "topLevel" {
		t.Fatal("placement defaults wrong")
	}
	if errs := m.Validate(); len(errs) != 0 {
		t.Fatalf("validate good: %v", errs)
	}
	// Unknown keys ignored (forward compat).
	if _, ok := m.Raw["futureField"]; !ok {
		t.Fatal("Raw must retain unknown keys")
	}
	// Bad manifests: version, empty name, dup id, script type, bad placement.
	bad := []string{
		`{"schemaVersion": 99, "name": "x", "actions": [{"id": "a", "title": "t", "action": {"type": "native", "capability": "c"}, "targets": "any"}]}`,
		`{"schemaVersion": 2, "name": "", "actions": [{"id": "a", "title": "t", "action": {"type": "native", "capability": "c"}, "targets": "any"}]}`,
		`{"schemaVersion": 2, "name": "x", "actions": []}`,
		`{"schemaVersion": 2, "name": "x", "actions": [{"id": "a", "title": "t", "action": {"type": "script", "capability": "c"}, "targets": "any"}]}`,
		`{"schemaVersion": 2, "name": "x", "actions": [{"id": "a", "title": "t", "action": {"type": "native", "capability": "c"}, "targets": "any", "placement": "drawer"}]}`,
		`{"schemaVersion": 2, "name": "x", "actions": [{"id": "a", "title": "t", "action": {"type": "native", "capability": "c"}, "targets": "any"}, {"id": "a", "title": "u", "action": {"type": "native", "capability": "c"}, "targets": "any"}]}`,
	}
	for i, b := range bad {
		m, err := ParsePackManifest([]byte(b))
		if err != nil {
			continue // parse-level rejection also fine
		}
		if errs := m.Validate(); len(errs) == 0 {
			t.Fatalf("bad manifest %d accepted", i)
		}
	}
}

func TestMatch(t *testing.T) {
	m, _ := ParsePackManifest([]byte(goodManifest))
	fileCtx := MatchCtx{Items: []MatchItem{{Path: "/tmp/a.txt", UTI: "public.plain-text"}}}
	folderCtx := MatchCtx{Items: []MatchItem{{Path: "/tmp/d", IsDir: true, UTI: "public.folder"}}}
	containerCtx := MatchCtx{Container: "/tmp/d"}
	emptyCtx := MatchCtx{}
	byID := map[string]PackAction{}
	for _, a := range m.Actions {
		byID[a.ID] = a
	}
	// container-only action: container matches, files don't.
	if got := Match(byID["new-text-file"], containerCtx, false, 0, 0); got != MatchOK {
		t.Fatalf("new-text container: %s", got)
	}
	if got := Match(byID["new-text-file"], fileCtx, false, 0, 0); got != MatchTarget {
		t.Fatalf("new-text on file: %s, want targetMismatch", got)
	}
	// any-target native: files + folders match; empty fails.
	if got := Match(byID["copy-path"], fileCtx, false, 0, 0); got != MatchOK {
		t.Fatalf("copy-path file: %s", got)
	}
	if got := Match(byID["copy-path"], emptyCtx, false, 0, 0); got != MatchEmpty {
		t.Fatalf("copy-path empty: %s", got)
	}
	// process action gated without Level B.
	if got := Match(byID["term-here"], folderCtx, false, 0, 0); got != MatchShellGated {
		t.Fatalf("term-here no-LevelB: %s, want shellGated", got)
	}
	if got := Match(byID["term-here"], folderCtx, true, 0, 0); got != MatchOK {
		t.Fatalf("term-here LevelB: %s", got)
	}
	// Count bounds + UTI filter.
	if got := Match(byID["copy-path"], fileCtx, false, 2, 0); got != MatchCountMin {
		t.Fatalf("min count: %s", got)
	}
	utiAction := byID["copy-path"]
	utiAction.UTIs = []string{"public.folder"}
	if got := Match(utiAction, fileCtx, false, 0, 0); got != MatchNoUTI {
		t.Fatalf("UTI mismatch: %s", got)
	}
	// VisibleActions: container ctx shows only container action (native).
	vis := VisibleActions(m, containerCtx, false)
	if len(vis) != 1 || vis[0].ID != "new-text-file" {
		t.Fatalf("visible container=%v, want [new-text-file]", vis)
	}
}

func TestValidatePackPath(t *testing.T) {
	dir := t.TempDir()
	if err := ValidatePackPath(dir, "actions/copy.zsh"); err != nil {
		t.Fatalf("in-pack relative: %v", err)
	}
	for _, bad := range []string{"../escape.sh", "/x/../../etc", "a/../../b", "/"} {
		if err := ValidatePackPath(dir, bad); err == nil {
			t.Fatalf("traversal %q accepted", bad)
		}
	}
	if err := ValidatePackPath(dir, "/tmp/abs.txt"); err != nil {
		t.Fatalf("absolute dispatch path: %v", err)
	}
}

func TestShellGate(t *testing.T) {
	m, _ := ParsePackManifest([]byte(goodManifest))
	byID := map[string]PackAction{}
	for _, a := range m.Actions {
		byID[a.ID] = a
	}
	// Native packs load by default (no gate) across contexts.
	nativeCtxs := map[string]MatchCtx{
		"new-text-file": {Container: "/d"},
		"copy-path":     {Items: []MatchItem{{Path: "/tmp/a.txt"}}},
	}
	for id, ctx := range nativeCtxs {
		if got := Match(byID[id], ctx, false, 0, 0); got != MatchOK {
			t.Fatalf("native %s gated without Level B: %s", id, got)
		}
	}
	// Process actions need Level B; disabled actions report disabled.
	if got := Match(byID["term-here"], MatchCtx{Items: []MatchItem{{Path: "/tmp/d", IsDir: true}}}, false, 0, 0); got != MatchShellGated {
		t.Fatalf("process without Level B: %s, want shellGated", got)
	}
	off := false
	disabled := byID["copy-path"]
	disabled.Enabled = &off
	if got := Match(disabled, MatchCtx{Items: []MatchItem{{Path: "/tmp/a.txt"}}}, false, 0, 0); got != MatchDisabled {
		t.Fatalf("disabled action: %s, want disabled", got)
	}
}

func TestInspectPack(t *testing.T) {
	// Malformed-pack fixture: declared manifest + README (metadata) +
	// undeclared helper + hidden script + binary-ish extra.
	dir := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("manifest.json", goodManifest)
	write("README.md", "docs")
	write("actions/copy.zsh", "echo hi")
	write("actions/.evil.sh", "pwn")
	write("bin/helper", "\x00\x01binary")
	write(".git/objects/x", "git-internal")
	declared := map[string]bool{"actions/copy.zsh": true}
	got, err := InspectPack(dir, declared)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	want := map[string]bool{"actions/.evil.sh": true, "bin/helper": true}
	if len(got) != len(want) {
		t.Fatalf("undeclared=%v, want %v", got, want)
	}
	for _, f := range got {
		if !want[f] {
			t.Fatalf("unexpected flag %q", f)
		}
	}
}
