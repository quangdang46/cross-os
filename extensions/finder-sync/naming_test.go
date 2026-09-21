// Naming tests — bead cross-os-vbl.1 (newfile Shared helpers port).
package findersync

import (
	"testing"
)

func TestMenuTitle(t *testing.T) {
	if got := (FileType{DisplayName: "New Markdown", Ext: "md"}).MenuTitle(); got != "New Markdown" {
		t.Fatalf("explicit label: got %q", got)
	}
	if got := (FileType{DisplayName: "  ", Ext: "md"}).MenuTitle(); got != "New .md" {
		t.Fatalf("blank label fallback: got %q, want New .md", got)
	}
}

func TestSeeds(t *testing.T) {
	if len(Seeds) != 8 {
		t.Fatalf("seeds=%d, want 8 built-ins (SeedPresets parity)", len(Seeds))
	}
	enabled := 0
	for _, s := range Seeds {
		if !s.BuiltIn {
			t.Fatalf("seed %+v must be built-in", s)
		}
		if s.Enabled {
			enabled++
		}
		if s.MenuTitle() == "" {
			t.Fatalf("seed %+v has no menu title", s)
		}
	}
	if enabled != 1 {
		t.Fatalf("enabled seeds=%d, want 1 (txt only, newfile parity)", enabled)
	}
}

func TestUniqueName(t *testing.T) {
	never := func(string) bool { return false }
	if got := UniqueName("/d", "Untitled", "md", never); got != "/d/Untitled.md" {
		t.Fatalf("fresh: got %q", got)
	}
	if got := UniqueName("/d", "", "env", never); got != "/d/.env" {
		t.Fatalf("dotfile: got %q", got)
	}
	// Redundant suffix stripped: "data.json" + json → data.json, not data.json.json.
	if got := UniqueName("/d", "data.json", "json", never); got != "/d/data.json" {
		t.Fatalf("suffix strip: got %q", got)
	}
	// Collisions increment: existing {Untitled.md, Untitled 2.md} → Untitled 3.md.
	taken := boolMap{"/d/Untitled.md": true, "/d/Untitled 2.md": true}
	if got := UniqueName("/d", "Untitled", "md", taken.Contains); got != "/d/Untitled 3.md" {
		t.Fatalf("collision: got %q", got)
	}
	// Dotfile collisions increment the same way.
	taken2 := boolMap{"/d/.env": true}
	if got := UniqueName("/d", "", "env", taken2.Contains); got != "/d/.env 2" {
		t.Fatalf("dotfile collision: got %q", got)
	}
}

type boolMap map[string]bool

func (m boolMap) Contains(p string) bool { return m[p] }
