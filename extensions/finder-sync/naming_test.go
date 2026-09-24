// Naming tests — bead cross-os-vbl.1 (newfile Shared helpers port). The
// file-type catalog tests moved with the catalog to crossos/core/pkg/filetype.
package findersync

import (
	"testing"
)

// TestUniqueNameStripsRedundantSuffix: the base field is a file name, and
// people type the whole name into it. "foo.txt" + txt is "foo.txt", not
// "foo.txt.txt" — the double extension is the one that files the bug.
func TestUniqueNameStripsRedundantSuffix(t *testing.T) {
	never := func(string) bool { return false }
	if got := UniqueName("/d", "foo.txt", "txt", never); got != "/d/foo.txt" {
		t.Fatalf("redundant suffix: got %q, want /d/foo.txt", got)
	}
	// Case-insensitively, because the filesystem the names land on is
	// case-insensitive too: "foo.TXT" is still the name the user typed.
	if got := UniqueName("/d", "foo.TXT", "txt", never); got != "/d/foo.txt" {
		t.Fatalf("redundant suffix, other case: got %q, want /d/foo.txt", got)
	}
	// A different extension is not redundant: "foo.md" + md stays "foo.md",
	// and "data.json" + json is likewise the name, not data.json.json.
	if got := UniqueName("/d", "data.json", "json", never); got != "/d/data.json" {
		t.Fatalf("matching suffix: got %q, want /d/data.json", got)
	}
	if got := UniqueName("/d", "Untitled", "md", never); got != "/d/Untitled.md" {
		t.Fatalf("plain base: got %q", got)
	}
}

// TestUniqueNameDotfileForm: a blank base name is a dotfile, not a file called
// ".env" would be wrong — it is the whole name, extension included
// (FileTypeRow.swift:43-51, the base field's own hint).
func TestUniqueNameDotfileForm(t *testing.T) {
	never := func(string) bool { return false }
	if got := UniqueName("/d", "", "env", never); got != "/d/.env" {
		t.Fatalf("dotfile: got %q, want /d/.env", got)
	}
	if got := UniqueName("/d", "", "gitignore", never); got != "/d/.gitignore" {
		t.Fatalf("multi-part dotfile: got %q, want /d/.gitignore", got)
	}
}

// TestUniqueNameIncrementsOnCollision: the number is appended inside the
// extension, so the created file keeps working tools' idea of its type, and
// the dotfile form counts the same way.
func TestUniqueNameIncrementsOnCollision(t *testing.T) {
	taken := boolMap{"/d/Untitled.md": true, "/d/Untitled 2.md": true}
	if got := UniqueName("/d", "Untitled", "md", taken.Contains); got != "/d/Untitled 3.md" {
		t.Fatalf("collision: got %q, want /d/Untitled 3.md", got)
	}
	dotTaken := boolMap{"/d/.env": true}
	if got := UniqueName("/d", "", "env", dotTaken.Contains); got != "/d/.env 2" {
		t.Fatalf("dotfile collision: got %q, want /d/.env 2", got)
	}
	// A gap in the taken list is not a licence to reuse a number: the first
	// free slot wins, and " 3" is free here.
	gappy := boolMap{"/d/data.json": true, "/d/data 3.json": true}
	if got := UniqueName("/d", "data", "json", gappy.Contains); got != "/d/data 2.json" {
		t.Fatalf("gap: got %q, want /d/data 2.json", got)
	}
}

type boolMap map[string]bool

func (m boolMap) Contains(p string) bool { return m[p] }
