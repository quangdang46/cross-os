// Finder page tests — the Explorer page (bead cross-os-vbl.6, page half).
//
// The nav freeze, the discovered-page count and the shared schema gates live
// in pages_test.go. This file pins what is specific to this page: the two
// sources the daemon serves, the writes each control may make, and the
// placeholder controls that are gone.
package shell

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

// finderControl is the part of a control the page contract is written in:
// what it draws, where its rows come from, and what a row or the whole list
// can write.
type finderControl struct {
	Kind       string   `json:"kind"`
	ID         string   `json:"id"`
	Source     string   `json:"source"`
	RowActions []string `json:"rowActions"`
	Actions    []string `json:"actions"`
}

type finderSchema struct {
	Description string          `json:"description"`
	Boundary    string          `json:"boundary"`
	Controls    []finderControl `json:"controls"`
}

func decodeFinder(t *testing.T) finderSchema {
	t.Helper()
	var body finderSchema
	if err := json.Unmarshal(FinderPage().Schema, &body); err != nil {
		t.Fatalf("finder schema not JSON: %v", err)
	}
	return body
}

// TestFinderPageContract pins the two lists the Explorer page is — the Finder
// right-click menu and the file types it offers — each on a source the daemon
// serves and each carrying the writes that source accepts. A control the
// daemon cannot answer is the failure this catches: it renders empty, which
// reads as "nothing here yet" rather than as a missing method, and an action
// id spelled differently from the daemon's is a permission token that matches
// nothing.
func TestFinderPageContract(t *testing.T) {
	body := decodeFinder(t)
	if FinderPage().ID != "core.finder" {
		t.Fatalf("id=%s, want core.finder", FinderPage().ID)
	}
	if len(body.Controls) != 2 {
		t.Fatalf("finder declares %d controls, want exactly 2: %+v", len(body.Controls), body.Controls)
	}
	want := []finderControl{
		{Kind: "menuList", ID: "menu", Source: "core:finderMenu", RowActions: []string{"core.setMenuItemEnabled"}},
		{Kind: "fileTypeList", ID: "fileTypes", Source: "core:fileTypes", RowActions: []string{"core.setFileType"}, Actions: []string{"core.reorderFileTypes"}},
	}
	for i, w := range want {
		got := body.Controls[i]
		if got.Kind != w.Kind || got.ID != w.ID || got.Source != w.Source {
			t.Fatalf("control %d is %+v, want kind %q id %q source %q", i, got, w.Kind, w.ID, w.Source)
		}
		if !slices.Equal(got.RowActions, w.RowActions) {
			t.Fatalf("%s row actions are %v, want %v", got.Kind, got.RowActions, w.RowActions)
		}
		if !slices.Equal(got.Actions, w.Actions) {
			t.Fatalf("%s actions are %v, want %v", got.Kind, got.Actions, w.Actions)
		}
	}
	// The contribution's action list is every action its controls declare, so a
	// control that gains a write without the contribution hearing about it
	// fails here rather than at the daemon.
	var declared []string
	for _, c := range body.Controls {
		declared = append(declared, c.RowActions...)
		declared = append(declared, c.Actions...)
	}
	if got := FinderPage().Actions; !slices.Equal(got, declared) {
		t.Fatalf("contribution actions are %v, want the controls' %v", got, declared)
	}
}

// TestFinderPageDropsPlaceholders pins the controls that were promises: the
// extpack list, the per-action settings form and the Level B badge described
// a Finder editor this build does not draw, and the five pack.* ids named
// writes no daemon method served. Left in place they would keep an id alive
// in the schema after its only reason to be there was gone — and the
// registry entry that once gave it a refusal would be the only thing still
// mentioning it.
func TestFinderPageDropsPlaceholders(t *testing.T) {
	s := string(FinderPage().Schema)
	for _, gone := range []string{
		"packList", "actionSettings", "gateBadge", "core:finderPacks", "core:packAction", "core:packActionGate",
		"pack.enableAction", "pack.disableAction", "pack.reorderAction", "pack.installLocal", "pack.remove",
	} {
		if strings.Contains(s, gone) {
			t.Fatalf("finder schema still declares %q", gone)
		}
	}
	for _, gone := range FinderPage().Actions {
		if strings.HasPrefix(gone, "pack.") {
			t.Fatalf("finder contribution still declares %q", gone)
		}
	}
}

// TestFinderPageDescribesItself pins the two halves of a description a person
// can act on: what the screen is, in words rather than ids, and the boundary
// that keeps this from becoming a second Extensions page.
func TestFinderPageDescribesItself(t *testing.T) {
	body := decodeFinder(t)
	d := strings.ToLower(body.Description)
	for _, want := range []string{"right-click menu", "file types", "finder"} {
		if !strings.Contains(d, want) {
			t.Fatalf("description %q does not say %q", body.Description, want)
		}
	}
	if !strings.Contains(strings.ToLower(body.Boundary), "extensions page") {
		t.Fatalf("boundary %q no longer points extension lifecycle at the Extensions page", body.Boundary)
	}
	// No plugin-lifecycle duplication (nir.7 owns it).
	for _, banned := range []string{"plugin.installDisk", "plugin.update", "plugin.uninstall"} {
		if strings.Contains(string(FinderPage().Schema), banned) {
			t.Fatalf("finder schema duplicates plugin lifecycle %q (nir.7 owns it)", banned)
		}
	}
}
