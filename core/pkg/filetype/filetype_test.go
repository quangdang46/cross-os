// Catalog tests: the newfile menu label fallback, seed parity, and the rows
// the store must refuse before they reach a page.
package filetype

import (
	"encoding/json"
	"reflect"
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
	seeds := Seeds()
	if len(seeds) != 8 {
		t.Fatalf("seeds=%d, want 8 built-ins (SeedPresets parity)", len(seeds))
	}
	enabled := 0
	for _, s := range seeds {
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
	if err := Validate(seeds); err != nil {
		t.Fatalf("the seed catalog must satisfy its own validator: %v", err)
	}
	// The eight rows are the presets, in order: a seed that silently changed
	// ext or display name would move a menu item a user has already used.
	want := []FileType{
		{Ext: "txt", BaseName: "New Text File", DisplayName: "New Text File", Enabled: true, BuiltIn: true},
		{Ext: "md", BaseName: "Untitled", DisplayName: "New Markdown", BuiltIn: true},
		{Ext: "env", DisplayName: "New .env", BuiltIn: true},
		{Ext: "json", BaseName: "data", DisplayName: "New JSON", BuiltIn: true},
		{Ext: "yml", BaseName: "config", DisplayName: "New YAML", BuiltIn: true},
		{Ext: "sh", BaseName: "script", DisplayName: "New Shell Script", BuiltIn: true},
		{Ext: "gitignore", DisplayName: "New .gitignore", BuiltIn: true},
		{Ext: "html", BaseName: "index", DisplayName: "New HTML", BuiltIn: true},
	}
	if !reflect.DeepEqual(seeds, want) {
		t.Fatalf("Seeds()=%+v\nwant %+v", seeds, want)
	}
}

// TestSeedsAreCopies: the seed table is handed to the store, and the store
// hands it to a page. An edit that reached the table itself would rewrite the
// catalog every later reader of a fresh store starts from.
func TestSeedsAreCopies(t *testing.T) {
	first := Seeds()
	first[0].Ext = "hijacked"
	first[0].Enabled = false
	if second := Seeds(); second[0].Ext != "txt" || !second[0].Enabled {
		t.Fatalf("Seeds() leaked the caller's edit: %+v", second[0])
	}
}

func TestValidate(t *testing.T) {
	good := []FileType{
		{Ext: "txt", BaseName: "New Text File"},
		{Ext: "env", BaseName: ""},
		{Ext: "json", BaseName: "data"},
		// Same extension, different base name: two presets, not a duplicate.
		{Ext: "json", BaseName: "report"},
	}
	if err := Validate(good); err != nil {
		t.Fatalf("a usable catalog must be accepted: %v", err)
	}
	for _, bad := range []struct {
		name string
		row  FileType
	}{
		{"blank ext", FileType{Ext: "", DisplayName: "New Nothing"}},
		{"whitespace ext", FileType{Ext: "   "}},
		{"dotted ext", FileType{Ext: ".txt"}},
		// path.Join cleans the joined result, so a separator walks the created
		// file out of the directory the menu named.
		{"traversal ext", FileType{Ext: "/../../etc/passwd", BaseName: "x"}},
		{"backslash ext", FileType{Ext: `txt\robs`, BaseName: "x"}},
	} {
		if err := Validate([]FileType{bad.row}); err == nil {
			t.Errorf("%s must be rejected: %+v", bad.name, bad.row)
		}
	}
}

// TestJSONRoundTrip: the catalog is persisted into the settings document, so
// every field has to come back — a dropped baseName would come home as a
// dotfile and a dropped enabled flag would switch a preset off for good.
func TestJSONRoundTrip(t *testing.T) {
	row := FileType{Ext: "sh", BaseName: "script", DisplayName: "New Shell Script", Template: "#!/bin/sh\n", Enabled: true, BuiltIn: true}
	raw, err := json.Marshal(row)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back FileType
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !reflect.DeepEqual(back, row) {
		t.Fatalf("round trip: got %+v, want %+v", back, row)
	}
	if got, want := string(raw), `{"ext":"sh","baseName":"script","displayName":"New Shell Script","template":"#!/bin/sh\n","enabled":true,"builtIn":true}`; got != want {
		t.Fatalf("persisted row=%s, want %s", got, want)
	}
}
