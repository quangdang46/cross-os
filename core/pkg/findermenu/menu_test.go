// Menu table tests: the seven spec items, the contexts each one appears in,
// the empty-context rule, the editor catalog, and the filename rule.
//
// The table is what the appex renders, so a row that moves or loses a context
// is a menu item a user has already used that is now somewhere else.
package findermenu

import (
	"reflect"
	"testing"
)

// TestMenuIsTheSpecSeven pins the ids and their order. The order is the
// submenu order the spec shows (ChatGPT-CROSSOS-20260924-2106:1007-1013).
func TestMenuIsTheSpecSeven(t *testing.T) {
	want := []string{
		"newFile", "newFolder", "copyPath", "copyRelativePath",
		"openTerminal", "openEditor", "duplicateWithName",
	}
	got := make([]string, 0, len(Menu))
	for _, item := range Menu {
		got = append(got, item.ID)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("menu ids=%v\nwant %v", got, want)
	}
	// Every row names a title and a native capability, and ships enabled: an
	// item that arrives disabled is a menu the user cannot act on, and the
	// flag is the appex's to flip, not the table's to carry off.
	for _, item := range Menu {
		if item.Title == "" || item.Capability == "" {
			t.Fatalf("item %s is incomplete: %+v", item.ID, item)
		}
		if !item.Enabled {
			t.Fatalf("item %s must default on", item.ID)
		}
		if len(item.Contexts) == 0 {
			t.Fatalf("item %s has no contexts", item.ID)
		}
	}
}

// TestContextsPerItem pins which contexts each verb appears in, across all
// three. A verb that drifts into the wrong context is a row that is there when
// it cannot work, or missing where it is the point.
func TestContextsPerItem(t *testing.T) {
	want := map[string][]Context{
		"newFile":           {ContextEmpty, ContextFolder},
		"newFolder":         {ContextEmpty, ContextFolder},
		"copyPath":          {ContextFile, ContextFolder},
		"copyRelativePath":  {ContextFile, ContextFolder},
		"openTerminal":      {ContextFile, ContextFolder, ContextEmpty},
		"openEditor":        {ContextFile, ContextFolder},
		"duplicateWithName": {ContextFile, ContextFolder},
	}
	for id, contexts := range want {
		item, ok := Lookup(id)
		if !ok {
			t.Fatalf("item %s is missing from the table", id)
		}
		if !reflect.DeepEqual(item.Contexts, contexts) {
			t.Fatalf("item %s contexts=%v, want %v", id, item.Contexts, contexts)
		}
	}
	// The three contexts, read back the way the appex reads them: every
	// context is reachable and no item is listed twice in one of them.
	for _, ctx := range Contexts {
		seen := map[string]bool{}
		for _, item := range ForContext(ctx) {
			if seen[item.ID] {
				t.Fatalf("item %s listed twice in context %s", item.ID, ctx)
			}
			seen[item.ID] = true
		}
	}
	if len(ForContext(ContextFile)) == 0 || len(ForContext(ContextFolder)) == 0 {
		t.Fatal("file and folder contexts must both offer the selection verbs")
	}
}

// TestEmptyContextHidesPathItems is the same rule
// extensions/finder-sync/menus_lifecycle_test.go:24 asserts for the extension's
// table: right-clicking empty space has no selection, so nothing that needs one
// can be offered there.
func TestEmptyContextHidesPathItems(t *testing.T) {
	empty := ForContext(ContextEmpty)
	if len(empty) == 0 {
		t.Fatal("the empty context must still offer New >")
	}
	for _, item := range empty {
		if item.NeedsPaths {
			t.Fatalf("empty context shows path-needing item %s", item.ID)
		}
	}
	// New File and New Folder are the two the empty context is for.
	found := map[string]bool{}
	for _, item := range empty {
		found[item.ID] = true
	}
	if !found["newFile"] || !found["newFolder"] {
		t.Fatalf("empty context=%v, want newFile + newFolder", found)
	}
}

// TestLookupMiss: an id the table does not have is a miss, not a default row.
func TestLookupMiss(t *testing.T) {
	if _, ok := Lookup("nope"); ok {
		t.Fatal("unknown id must not resolve")
	}
}

func TestEditorCatalog(t *testing.T) {
	if len(Editors) == 0 {
		t.Fatal("the Open in Editor submenu needs a catalog")
	}
	// The catalog is the submenu: its order is what the appex renders, and
	// com.microsoft.VSCode is the one the spec's menu names.
	first := Editors[0]
	if first.ID != "com.microsoft.VSCode" {
		t.Fatalf("first editor=%s, want com.microsoft.VSCode first", first.ID)
	}
	seen := map[string]bool{}
	for i, e := range Editors {
		if e.ID == "" || e.Name == "" {
			t.Fatalf("editor %d is incomplete: %+v", i, e)
		}
		if seen[e.ID] {
			t.Fatalf("duplicate editor %s", e.ID)
		}
		seen[e.ID] = true
	}
	// Both ends read this list: the submenu offers it and openEditor resolves
	// against it, so every id on the menu must resolve.
	for _, e := range Editors {
		got, ok := EditorByID(e.ID)
		if !ok || got != e {
			t.Fatalf("EditorByID(%q)=%+v ok=%v, want the catalog row", e.ID, got, ok)
		}
	}
	if _, ok := EditorByID("com.example.NoSuchEditor"); ok {
		t.Fatal("an id outside the catalog must not resolve")
	}
}

func TestUniqueName(t *testing.T) {
	never := func(string) bool { return false }
	taken := map[string]bool{"/d/Untitled.md": true, "/d/Untitled 2.md": true}
	exists := func(p string) bool { return taken[p] }

	if got := UniqueName("/d", "Untitled", "md", never); got != "/d/Untitled.md" {
		t.Fatalf("free name: %q", got)
	}
	// A base the user typed with the extension already on it.
	if got := UniqueName("/d", "data.json", "json", never); got != "/d/data.json" {
		t.Fatalf("redundant suffix: %q", got)
	}
	// An empty base is a dotfile, the newfile rule.
	if got := UniqueName("/d", "", "env", never); got != "/d/.env" {
		t.Fatalf("dotfile: %q", got)
	}
	// Collisions walk the increment.
	if got := UniqueName("/d", "Untitled", "md", exists); got != "/d/Untitled 3.md" {
		t.Fatalf("increment: %q", got)
	}
	if got := UniqueName("/d", "", "env", func(p string) bool { return p == "/d/.env" }); got != "/d/.env 2" {
		t.Fatalf("dotfile increment: %q", got)
	}
	// A folder has no extension, so the candidate carries no trailing dot.
	if got := UniqueName("/d", "Projects", "", never); got != "/d/Projects" {
		t.Fatalf("folder: %q", got)
	}
	folderTaken := map[string]bool{"/d/Projects": true}
	if got := UniqueName("/d", "Projects", "", func(p string) bool { return folderTaken[p] }); got != "/d/Projects 2" {
		t.Fatalf("folder increment: %q", got)
	}
}
