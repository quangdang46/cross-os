package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The plist template is the single source; the installer only substitutes
// paths. A duplicated template in Go would drift from it, which is the
// failure this bead removes.
func TestRenderAutostartPlistSubstitutesPaths(t *testing.T) {
	out := renderAutostartPlist("/opt/crossos/crossos", "/Users/me/Library/Application Support/CrossOS")
	if strings.Contains(out, "__CROSSOS_BIN__") || strings.Contains(out, "__CROSSOS_STATE__") {
		t.Fatalf("placeholders survived:\n%s", out)
	}
	if !strings.Contains(out, "<string>/opt/crossos/crossos</string>") {
		t.Fatal("binary path not substituted")
	}
	if !strings.Contains(out, "dev.crossos.daemon") {
		t.Fatal("label missing")
	}
	// Rendering is deterministic: the same inputs give the same file, so a
	// reinstall produces no spurious diff.
	if renderAutostartPlist("/a", "/b") != renderAutostartPlist("/a", "/b") {
		t.Fatal("render is not deterministic")
	}
}

// The embedded template must be a plist Apple will load.
func TestEmbeddedPlistIsValid(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "agent.plist")
	if err := os.WriteFile(p, []byte(renderAutostartPlist("/bin/true", dir)), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := execPlistLint(p)
	if err != nil {
		t.Fatalf("plutil rejected the rendered agent: %v", err)
	}
	if !strings.Contains(out, "OK") {
		t.Fatalf("plutil output: %s", out)
	}
}

func execPlistLint(path string) (string, error) {
	out, err := exec.Command("plutil", "-lint", path).CombinedOutput()
	return string(out), err
}
