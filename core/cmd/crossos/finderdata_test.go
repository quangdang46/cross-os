// Explorer page data tests (bead be-finderdata).
//
// Three cases per handler, because the page has three ways to be wrong: it
// serves nothing (the shaped hole this repo keeps shipping), it serves rows
// the daemon cannot act on, or it accepts a write that is half a write. The
// last test is the gate: a page source with no method behind it is the failure
// every direct-call test here would pass straight through.

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"crossos/core/pkg/filetype"
	"crossos/core/pkg/findermenu"
	"crossos/core/pkg/ipc"
)

// --- core:finderMenu ---

// TestFinderMenuServesTheSevenItems: a fresh daemon still serves the whole
// table. The empty case is the one that matters here — a menu with nothing in
// it is a menu the user cannot switch anything on, and it is what a source with
// no handler renders today.
func TestFinderMenuServesTheSevenItems(t *testing.T) {
	c := testCore(t)
	rows := mustMenuRows(t, c, "core.finderMenu", nil)

	if len(rows) != len(findermenu.Menu) {
		t.Fatalf("menu rows=%d, want the %d items findermenu declares", len(rows), len(findermenu.Menu))
	}
	for i, row := range rows {
		want := findermenu.Menu[i]
		if row.ID != want.ID {
			t.Fatalf("row %d is %q, want %q — the order is what Finder renders", i, row.ID, want.ID)
		}
		if row.Title == "" || row.Capability == "" {
			t.Errorf("row %s is incomplete: %+v", row.ID, row)
		}
		if len(row.Contexts) == 0 {
			t.Errorf("row %s has no contexts: an item nothing can offer", row.ID)
		}
		// The full case: every row is on until a person turns it off.
		if !row.Enabled {
			t.Errorf("row %s ships disabled; the daemon declares what the product offers", row.ID)
		}
	}
}

// TestMenuSupportTracksTheHost: supported is the host's answer, and the two
// ways it says no are different facts. A capability the registry never carried
// is unsupported everywhere; one the adapter guards is unsupported off darwin.
func TestMenuSupportTracksTheHost(t *testing.T) {
	restore := finderHost
	t.Cleanup(func() { finderHost = restore })

	for _, host := range []string{"darwin", "windows"} {
		finderHost = host
		for _, item := range findermenu.Menu {
			got := menuSupported(item.Capability)
			if item.Capability == "clipboard.copyRelativePath" || item.Capability == "filesystem.duplicate" {
				if got {
					t.Errorf("%s: %s names a capability the registry does not carry, so it cannot be supported on %s",
						host, item.Capability, host)
				}
				continue
			}
			want := host == "darwin" || !hostAdapterNames[item.Capability]
			if got != want {
				t.Errorf("%s: supported(%s)=%v, want %v", host, item.Capability, got, want)
			}
		}
	}
}

// TestSetMenuItemEnabledReturnsTheWholeTable: one id in, the table out, with
// that row the only one that moved. The page re-renders from the reply, so a
// partial answer is a page showing a state the user did not choose.
func TestSetMenuItemEnabledReturnsTheWholeTable(t *testing.T) {
	c := testCore(t)
	before := mustMenuRows(t, c, "core.finderMenu", nil)

	rows := mustMenuRows(t, c, "core.setMenuItemEnabled",
		json.RawMessage(`{"id":"openTerminal","enabled":false}`))
	if len(rows) != len(before) {
		t.Fatalf("the reply has %d rows for a %d-row table", len(rows), len(before))
	}
	for i, row := range rows {
		want := before[i].Enabled
		if row.ID == "openTerminal" {
			want = false
		}
		if row.Enabled != want {
			t.Errorf("row %s enabled=%v, want %v — one toggle moved %d rows", row.ID, row.Enabled, want, len(rows))
		}
	}
	// The toggle survives the reply: the next read is the same table.
	again := mustMenuRows(t, c, "core.finderMenu", nil)
	if !reflect.DeepEqual(again, rows) {
		t.Errorf("core.finderMenu disagrees with the write that just answered:\n%+v\n%+v", again, rows)
	}
	// And back on.
	rows = mustMenuRows(t, c, "core.setMenuItemEnabled",
		json.RawMessage(`{"id":"openTerminal","enabled":true}`))
	for _, row := range rows {
		if row.ID == "openTerminal" && !row.Enabled {
			t.Fatal("openTerminal did not come back on")
		}
	}
}

// TestSetMenuItemEnabledRefuses: an id the table does not have is an error, not
// a toggle that silently did nothing — and it leaves the table as it was.
func TestSetMenuItemEnabledRefuses(t *testing.T) {
	c := testCore(t)
	want := mustMenuRows(t, c, "core.finderMenu", nil)

	for _, tc := range []struct {
		name string
		raw  string
		code int
	}{
		{"unknown id", `{"id":"renameEverything","enabled":false}`, ipc.ErrInvalid},
		{"no id", `{"enabled":false}`, ipc.ErrBadParams},
		{"blank id", `{"id":"  ","enabled":false}`, ipc.ErrBadParams},
		{"not json", `{`, ipc.ErrBadParams},
	} {
		_, rerr := c.handleSetMenuItemEnabled(json.RawMessage(tc.raw))
		if rerr == nil {
			t.Errorf("%s: a refused toggle must be an error, not a silent no-op", tc.name)
			continue
		}
		if rerr.Code != tc.code {
			t.Errorf("%s: code=%d, want %d (%s)", tc.name, rerr.Code, tc.code, rerr.Message)
		}
	}
	if got := mustMenuRows(t, c, "core.finderMenu", nil); !reflect.DeepEqual(got, want) {
		t.Error("a refused toggle changed the table")
	}
}

// --- core:fileTypes ---

// TestFileTypesServeTheCatalogOnAFreshDaemon: the empty store is the eight
// seeds, not []. A New menu with nothing in it is a menu the daemon has not
// built yet, and the page would say so with a row that looks like data.
func TestFileTypesServeTheCatalogOnAFreshDaemon(t *testing.T) {
	c := testCore(t)
	rows := mustFileTypeRows(t, c, "core.fileTypes", nil)

	if len(rows) != len(filetype.Seeds()) {
		t.Fatalf("file types=%d, want the %d seeds a fresh store carries", len(rows), len(filetype.Seeds()))
	}
	for i, row := range rows {
		seed := filetype.Seeds()[i]
		if row.Ext != seed.Ext || row.BaseName != seed.BaseName {
			t.Errorf("row %d is %+v, want the seed %+v — the order is the order the menu renders", i, row.FileType, seed)
		}
		if !row.BuiltIn {
			t.Errorf("row %s lost builtIn; the page cannot tell a preset from a user's own", row.Ext)
		}
		if row.MenuTitle == "" {
			t.Errorf("row %s has no menu title", row.Ext)
		}
		if row.MenuTitle != row.FileType.MenuTitle() {
			t.Errorf("row %s menuTitle=%q, want the derived %q", row.Ext, row.MenuTitle, row.FileType.MenuTitle())
		}
	}
}

// TestSetFileTypeReturnsTheWholeCatalog: one row in, validated, the catalog
// back. The reply is what the page re-renders from, so a single row would
// leave the rest of the list to the shell's guess.
func TestSetFileTypeReturnsTheWholeCatalog(t *testing.T) {
	c := testCore(t)
	rows := mustFileTypeRows(t, c, "core.setFileType",
		json.RawMessage(`{"ext":"md","baseName":"Untitled","displayName":"New Markdown File","template":"# ","enabled":true}`))

	if len(rows) != len(filetype.Seeds()) {
		t.Fatalf("the reply has %d rows for an %d-row catalog", len(rows), len(filetype.Seeds()))
	}
	var md *fileTypeRow
	for i := range rows {
		if rows[i].Ext == "md" {
			md = &rows[i]
		}
	}
	if md == nil {
		t.Fatal("the edited row is missing from the reply")
	}
	if md.DisplayName != "New Markdown File" || md.Template != "# " || !md.Enabled {
		t.Errorf("md row=%+v, want the edit applied", md.FileType)
	}
	// builtIn is the stored flag, not the caller's: a page that could clear
	// it would make a preset look like something the user typed.
	if !md.BuiltIn {
		t.Error("the edit cleared builtIn")
	}
	// The store kept it, so the next read agrees.
	again := mustFileTypeRows(t, c, "core.fileTypes", nil)
	if !reflect.DeepEqual(again, rows) {
		t.Error("core.fileTypes disagrees with the write that just answered")
	}
}

// TestSetFileTypeRefuses: the identity is the stored row's, so a caller cannot
// mint a preset or retarget one, and a bad payload leaves the catalog whole.
func TestSetFileTypeRefuses(t *testing.T) {
	c := testCore(t)
	want := mustFileTypeRows(t, c, "core.fileTypes", nil)

	for _, tc := range []struct {
		name string
		raw  string
		code int
	}{
		{"unknown extension", `{"ext":"exe","baseName":"x","displayName":"New EXE"}`, ipc.ErrInvalid},
		{"known extension, wrong base name", `{"ext":"json","baseName":"report","displayName":"New Report"}`, ipc.ErrInvalid},
		{"no extension", `{"displayName":"New Nothing"}`, ipc.ErrBadParams},
		{"blank extension", `{"ext":"   ","displayName":"New Nothing"}`, ipc.ErrBadParams},
		{"not json", `{`, ipc.ErrBadParams},
		{"template is not a string", `{"ext":"md","baseName":"Untitled","template":7}`, ipc.ErrBadParams},
	} {
		_, rerr := c.handleSetFileType(json.RawMessage(tc.raw))
		if rerr == nil {
			t.Errorf("%s: must be refused", tc.name)
			continue
		}
		if rerr.Code != tc.code {
			t.Errorf("%s: code=%d, want %d (%s)", tc.name, rerr.Code, tc.code, rerr.Message)
		}
	}
	if got := mustFileTypeRows(t, c, "core.fileTypes", nil); !reflect.DeepEqual(got, want) {
		t.Error("a refused edit changed the catalog")
	}
}

// TestReorderFileTypes: an explicit list, so the whole order moves at once.
// The identity list is the no-op and the reversed list is the full case; the
// partial, unknown and duplicated lists are the three ways a reorder would
// otherwise compose itself out of a drag the daemon could not see the end of.
func TestReorderFileTypes(t *testing.T) {
	c := testCore(t)
	start := mustFileTypeRows(t, c, "core.fileTypes", nil)
	ids := make([]string, 0, len(start))
	for _, row := range start {
		ids = append(ids, fileTypeID(row.FileType))
	}

	reversed := make([]string, len(ids))
	for i, id := range ids {
		reversed[len(ids)-1-i] = id
	}
	rows := mustFileTypeRows(t, c, "core.reorderFileTypes", encodeIDs(t, reversed))
	got := make([]string, 0, len(rows))
	for _, row := range rows {
		got = append(got, fileTypeID(row.FileType))
	}
	if !reflect.DeepEqual(got, reversed) {
		t.Fatalf("reordered to %v, want %v", got, reversed)
	}
	// The store kept the order, so the next read is not re-sorted.
	again := mustFileTypeRows(t, c, "core.fileTypes", nil)
	gotAgain := make([]string, 0, len(again))
	for _, row := range again {
		gotAgain = append(gotAgain, fileTypeID(row.FileType))
	}
	if !reflect.DeepEqual(gotAgain, reversed) {
		t.Fatalf("the order did not survive the write: %v", gotAgain)
	}

	// The identity list is a legal no-op, not an error.
	if _, rerr := c.handleReorderFileTypes(encodeIDs(t, reversed)); rerr != nil {
		t.Errorf("re-applying the same order must be a no-op, got %s", rerr.Message)
	}

	// Everything below must leave the order exactly where it was.
	duplicated := append(append([]string{}, reversed...), reversed[0])
	for _, tc := range []struct {
		name string
		raw  json.RawMessage
		code int
	}{
		{"partial list", encodeIDs(t, reversed[:len(reversed)-1]), ipc.ErrInvalid},
		{"one id too many", encodeIDs(t, append(append([]string{}, reversed...), "nope.txt")), ipc.ErrInvalid},
		{"unknown id", encodeIDs(t, append(append([]string{}, reversed[:len(reversed)-1]...), "nope.txt")), ipc.ErrInvalid},
		{"the same id twice", encodeIDs(t, duplicated), ipc.ErrInvalid},
		{"not json", json.RawMessage(`{`), ipc.ErrBadParams},
	} {
		_, rerr := c.handleReorderFileTypes(tc.raw)
		if rerr == nil {
			t.Errorf("%s: must be refused", tc.name)
			continue
		}
		if rerr.Code != tc.code {
			t.Errorf("%s: code=%d, want %d (%s)", tc.name, rerr.Code, tc.code, rerr.Message)
		}
		after := mustFileTypeRows(t, c, "core.fileTypes", nil)
		now := make([]string, 0, len(after))
		for _, row := range after {
			now = append(now, fileTypeID(row.FileType))
		}
		if !reflect.DeepEqual(now, reversed) {
			t.Fatalf("%s moved the order to %v, want it left at %v", tc.name, now, reversed)
		}
	}
}

// --- the gate ---

// pageSourceMethods is the source→method table: every `source` a Core page
// control declares, and the IPC method that serves it. The names are not the
// same strings — a page says core:behaviorMatrix and the daemon serves
// config.getMatrix — so the pairing is a fact somebody has to write down, and
// this is where.
var pageSourceMethods = map[string]string{
	"core:trialCountdown":  "safety.trialState",
	"core:ownershipAudit":  "safety.ownershipAudit",
	"core:version":         "core.status",
	"core:plugins":         "plugin.list",
	"core:eventLogs":       "core.eventLogs",
	"core:traces":          "core.traces",
	"core:behaviorMatrix":  "config.getMatrix",
	"core:appOverrides":    "config.getOverrides",
	"core:windowShortcuts": "config.getShortcuts",
	"core:allShortcuts":    "config.getShortcuts",
	"core:snapZones":       "config.getZones",
	"core:pluginSchemas":   "core.pluginSchemas",
	"core:commands":        "core.commands",
	"core:profiles":        "core.profiles",
	"core:readiness":       "core.readiness",
	"core:finderMenu":      "core.finderMenu",
	"core:fileTypes":       "core.fileTypes",
	"core:windows":         "core.windows",
}

// shellLocalSources are sources the shell resolves without the daemon: the
// control derives the value out of the source string itself, so there is
// nothing for a method to serve. Listed rather than left out so that "no
// method" is always a decision somebody can read, never an omission.
var shellLocalSources = map[string]string{
	"core:licenseMIT":         "LicenseControl reads the license off the end of the source name",
	"core:attributionEntries": "CreditsControl names third_party/ATTRIBUTION.md; it ships no values",
}

// pageSourcePattern finds the source a control declares. The backend authors
// pages as Go map literals, so the string is the contract; a page that stops
// spelling it out this way is a change this test cannot see, which is the one
// thing worth knowing about it.
var pageSourcePattern = regexp.MustCompile(`"source":\s*"([^"]+)"`)

// TestEveryPageSourceHasAMethod: the shaped-hole gate. A page control that
// declares a source no method serves renders empty, and nothing else in this
// package fails for it — every handler test here calls a handler directly, so
// a source with no handler at all is invisible to all of them.
//
// The pages live in app/backend and core cannot import them (app depends on
// core), so the contract is checked the only way that dependency direction
// allows: read the page declarations as text and pair each source with the
// method that serves it. A source added to a page without an entry here, or an
// entry whose method methods() does not register, both fail.
func TestEveryPageSourceHasAMethod(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "app", "backend")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Skipf("app/backend is not readable from here (%v); core alone cannot check the page contract", err)
	}
	c := testCore(t)
	reg := c.methods()

	declared := map[string]bool{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		for _, m := range pageSourcePattern.FindAllStringSubmatch(string(body), -1) {
			declared[m[1]] = true
		}
	}
	if len(declared) == 0 {
		t.Fatal("no page source was found — the scan is broken, not the pages")
	}

	names := make([]string, 0, len(declared))
	for source := range declared {
		names = append(names, source)
	}
	sort.Strings(names)
	for _, source := range names {
		if reason, local := shellLocalSources[source]; local {
			if _, alsoDaemon := pageSourceMethods[source]; alsoDaemon {
				t.Errorf("%s is listed as both daemon-served and shell-local (%s)", source, reason)
			}
			continue
		}
		method, served := pageSourceMethods[source]
		if !served {
			t.Errorf("page declares source %q and no method serves it; the control renders empty", source)
			continue
		}
		if _, ok := reg[method]; !ok {
			t.Errorf("page source %q maps to %q, which methods() does not register", source, method)
		}
	}

	// The reverse: a mapping for a source no page declares is a table that has
	// drifted, and a later reader cannot tell which half is right.
	for source, method := range pageSourceMethods {
		if declared[source] {
			continue
		}
		t.Errorf("pageSourceMethods maps %q → %q but no page declares that source", source, method)
	}
}

// TestFinderMethodsAreReachableOverTheTable: registration is the whole of this
// bead for the shell — a handler wired to a name the method table does not
// carry is one no client can reach, and a direct call never sees that.
func TestFinderMethodsAreReachableOverTheTable(t *testing.T) {
	c := testCore(t)
	reg := c.methods()
	for _, name := range []string{
		"core.finderMenu", "core.fileTypes", "core.setFileType",
		"core.reorderFileTypes", "core.setMenuItemEnabled",
		// be-finderverbs' eight, merged into the same table.
		"finder.menuEntries", "finder.createFolder", "finder.createFile",
		"finder.copyPath", "finder.copyRelativePath", "finder.openTerminal",
		"finder.openEditor", "finder.duplicateWithName",
	} {
		if _, ok := reg[name]; !ok {
			t.Errorf("%q is not in the method table", name)
		}
	}
	if got := mustMenuRows(t, c, "core.finderMenu", nil); len(got) != len(findermenu.Menu) {
		t.Errorf("core.finderMenu over the table served %d rows", len(got))
	}
	if got := mustFileTypeRows(t, c, "core.fileTypes", nil); len(got) != len(filetype.Seeds()) {
		t.Errorf("core.fileTypes over the table served %d rows", len(got))
	}
}

// --- helpers ---

// finderCall dispatches through the method table the way a client does, so a test
// cannot pass on a handler the daemon never registered.
func finderCall(t *testing.T, c *Core, method string, params json.RawMessage) (any, *ipc.RPCError) {
	t.Helper()
	h, ok := c.methods()[method]
	if !ok {
		t.Fatalf("%s is not in the method table", method)
	}
	return h(params)
}

func mustMenuRows(t *testing.T, c *Core, method string, params json.RawMessage) []menuRow {
	t.Helper()
	res, rerr := finderCall(t, c, method, params)
	return decodeRows[menuRow](t, res, rerr)
}

func mustFileTypeRows(t *testing.T, c *Core, method string, params json.RawMessage) []fileTypeRow {
	t.Helper()
	res, rerr := finderCall(t, c, method, params)
	return decodeRows[fileTypeRow](t, res, rerr)
}

func encodeIDs(t *testing.T, ids []string) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"ids": ids})
	if err != nil {
		t.Fatalf("marshal ids: %v", err)
	}
	return raw
}
