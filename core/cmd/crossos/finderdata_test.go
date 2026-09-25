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
	"crossos/core/pkg/intent"
	"crossos/core/pkg/ipc"
	builtin "crossos/core/rules"
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

// TestSetFileTypeRefusesAChangedBaseName: the identity refusal, pinned with the
// two facts that make it a contract rather than a coincidence — a changed base
// name is refused, and the catalog is left exactly as it was, INCLUDING the
// other fields carried in the same payload.
//
// This is the shape the shell sends when a person edits a default filename:
// every field but the base name is the stored row's, so it is the payload most
// likely to slip through by accident if the lookup ever widens to match on ext
// alone. And it is the load-bearing half of the pair — the extension has no
// field to edit in the UI at all, the base name had one, and the shell that
// shipped it reported a save that no referent existed for. The control now
// refuses in words, and that refusal is only honest while this holds.
func TestSetFileTypeRefusesAChangedBaseName(t *testing.T) {
	c := testCore(t)
	before := mustFileTypeRows(t, c, "core.fileTypes", nil)

	// json's stored base name is "data" (filetype.Seeds). Everything else here
	// is that row as the daemon holds it, so the base name is the only change.
	const renamed = `{"ext":"json","baseName":"report","displayName":"Renamed JSON",` +
		`"template":"{}","enabled":false,"builtIn":true}`
	res, rerr := c.handleSetFileType(json.RawMessage(renamed))
	if rerr == nil {
		t.Fatalf("a changed base name was accepted and answered %v; the row moved instead of being refused", res)
	}
	if rerr.Code != ipc.ErrInvalid {
		t.Errorf("code=%d, want %d (%s)", rerr.Code, ipc.ErrInvalid, rerr.Message)
	}
	if !strings.Contains(rerr.Message, "no file type") {
		t.Errorf("message=%q, want the refusal that names the row it could not find", rerr.Message)
	}
	// The whole catalog, not just the row: the other fields in this payload —
	// a new label, a new template, a new on/off state — must not have landed
	// either. A write refused on identity that still applied half of itself is a
	// worse outcome than one that did not land at all.
	if got := mustFileTypeRows(t, c, "core.fileTypes", nil); !reflect.DeepEqual(got, before) {
		t.Errorf("a refused edit changed the catalog\n got: %+v\nwant: %+v", got, before)
	}

	// The control case, and the reason this is pinned as IDENTITY rather than as
	// a general refusal: the same payload with the stored base name is accepted.
	// Without this line, a handler that refused every json row would pass.
	const accepted = `{"ext":"json","baseName":"data","displayName":"Renamed JSON",` +
		`"template":"{}","enabled":false,"builtIn":true}`
	if _, rerr := c.handleSetFileType(json.RawMessage(accepted)); rerr != nil {
		t.Fatalf("the same edit with the stored base name was refused (%s); the test above is not about identity", rerr.Message)
	}
	got := mustFileTypeRows(t, c, "core.fileTypes", nil)
	if len(got) != len(before) {
		t.Fatalf("the accepted edit has %d rows, want %d — nothing was added", len(got), len(before))
	}
	var js *fileTypeRow
	for i := range got {
		if got[i].Ext == "json" {
			js = &got[i]
		}
	}
	if js == nil {
		t.Fatal("the json row is missing after the accepted edit")
	}
	if js.DisplayName != "Renamed JSON" || js.Template != "{}" || js.Enabled {
		t.Errorf("json row=%+v, want the accepted edit applied to the STORED row", js.FileType)
	}
	if fileTypeID(js.FileType) != "data.json" {
		t.Errorf("json id is %q, want data.json — the accepted edit kept the stored identity",
			fileTypeID(js.FileType))
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

// --- the three things this page got wrong ---

// TestMenuToggleSurvivesARestart: the toggle is the one write on this page
// whose promise is a lie unless it reaches the disk. The verb is bound, the
// Service method exists, and the control re-renders from the reply the daemon
// hands back — so a write that only moved the running map answered "done" and
// then lost the answer to the next launch, with nothing said. The person who
// switched a row off is the only one who can see it come back.
//
// Both directions are checked, because a document that can record an "off" and
// not an "on" is its own kind of lie: SetMenuItemDisabled DELETES the id when
// the row comes back on, precisely so a menu item shipped after the document
// was written arrives on rather than being pinned off by a stale entry.
func TestMenuToggleSurvivesARestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	c, err := NewCoreWithSettings(builtin.All(), builtin.Grants(), path)
	if err != nil {
		t.Fatalf("run 1: %v", err)
	}
	c.loadMenuOff()
	// A fresh daemon ships every row on: the table is what the product
	// declares and a person narrows it, so this is the state a restart is
	// expected to come back to when nothing has been touched.
	if !menuRowByID(t, c, "openTerminal").Enabled {
		t.Fatal("openTerminal ships off; a fresh daemon serves the whole table")
	}

	off := mustMenuRows(t, c, "core.setMenuItemEnabled",
		json.RawMessage(`{"id":"openTerminal","enabled":false}`))
	if menuRowByIDIn(off, "openTerminal").Enabled {
		t.Fatal("the write's own reply says the row is still on")
	}

	restarted, err := NewCoreWithSettings(builtin.All(), builtin.Grants(), path)
	if err != nil {
		t.Fatalf("run 2: %v", err)
	}
	restarted.loadMenuOff()
	after := mustMenuRows(t, restarted, "core.finderMenu", nil)
	if menuRowByIDIn(after, "openTerminal").Enabled {
		t.Error("a row the user switched off came back on after a restart — " +
			"the write never reached the document")
	}
	// And the rows nobody touched are still on: a seed that turned the whole
	// map off would make this test pass for the wrong reason.
	for _, row := range after {
		if row.ID != "openTerminal" && !row.Enabled {
			t.Errorf("row %s came back off; only the toggled row should persist", row.ID)
		}
	}

	// Back on, and off it stays across the next restart too.
	mustMenuRows(t, restarted, "core.setMenuItemEnabled",
		json.RawMessage(`{"id":"openTerminal","enabled":true}`))
	again, err := NewCoreWithSettings(builtin.All(), builtin.Grants(), path)
	if err != nil {
		t.Fatalf("run 3: %v", err)
	}
	again.loadMenuOff()
	if !menuRowByIDIn(mustMenuRows(t, again, "core.finderMenu", nil), "openTerminal").Enabled {
		t.Error("the row did not come back on; a document that can record an " +
			"off but not an on pins the menu to the day the key landed")
	}
}

// TestTheTwoReadersAgreeOnOneCatalog: core.fileTypes is what the Explorer page
// shows and writes; finder.menuEntries is what the appex actually renders the
// New submenu from. They are two readers of ONE catalog, and when they were two
// readers of two lists the page could switch a type on and the right-click menu
// would not carry it — the submenu row and the setting, disagreeing, which is
// the single bug FinderRight's own header says the shared catalog exists to
// prevent (MenuFeature.swift:3-6).
//
// Driven through the real writes rather than by poking the store, because the
// reader is only half the claim: a store that held the edit and a menu that
// ignored it would pass any test that only set the store up.
func TestTheTwoReadersAgreeOnOneCatalog(t *testing.T) {
	c := testCore(t)
	page := mustFileTypeRows(t, c, "core.fileTypes", nil)
	if len(page) < 2 {
		t.Fatalf("the catalog has %d rows; this needs two to tell disable from rename", len(page))
	}

	// Take the FIRST type off and RENAME the second, so the two changes are
	// checked separately: a reader that ignored `enabled` and one that ignored
	// `displayName` are different bugs with the same symptom.
	first := page[0]
	second := page[1]
	raw, err := json.Marshal(map[string]any{
		"ext": first.Ext, "baseName": first.BaseName,
		"displayName": first.DisplayName, "template": first.Template,
		"enabled": false, "builtIn": first.BuiltIn,
	})
	if err != nil {
		t.Fatalf("marshal the disabled row: %v", err)
	}
	if got := mustFileTypeRows(t, c, "core.setFileType", raw); len(got) != len(page) {
		t.Fatalf("the write's reply has %d rows for a %d-row catalog", len(got), len(page))
	}
	renamed := "Renamed by the Explorer page"
	raw, err = json.Marshal(map[string]any{
		"ext": second.Ext, "baseName": second.BaseName,
		"displayName": renamed, "template": second.Template,
		"enabled": second.Enabled, "builtIn": second.BuiltIn,
	})
	if err != nil {
		t.Fatalf("marshal the renamed row: %v", err)
	}
	if got := mustFileTypeRows(t, c, "core.setFileType", raw); len(got) != len(page) {
		t.Fatalf("the rename's reply has %d rows for a %d-row catalog", len(got), len(page))
	}

	menu := mustFinderMenuEntries(t, c)
	// Every row the page is still offering is offered in the menu, in the
	// page's order — the submenu IS the catalog, filtered to what is on.
	var want []filetype.FileType
	for _, row := range mustFileTypeRows(t, c, "core.fileTypes", nil) {
		if row.Enabled {
			want = append(want, row.FileType)
		}
	}
	if len(menu.FileTypes) != len(want) {
		t.Fatalf("the menu offers %d types, the page lists %d enabled: %v vs %v",
			len(menu.FileTypes), len(want), menu.FileTypes, want)
	}
	for i := range want {
		if menu.FileTypes[i] != want[i] {
			t.Fatalf("row %d: menu has %+v, page has %+v — the two readers disagree", i, menu.FileTypes[i], want[i])
		}
	}
	// Named directly, so the failure says which of the two edits did not land.
	for _, ft := range menu.FileTypes {
		if ft.Ext == first.Ext && ft.BaseName == first.BaseName {
			t.Errorf("a type the Explorer page switched off is still in the New submenu: %+v", ft)
		}
		if ft.Ext == second.Ext && ft.DisplayName != renamed {
			t.Errorf("the menu still shows the old label for .%s: %q, want %q",
				second.Ext, ft.DisplayName, renamed)
		}
	}
	// A menu row is a promise the click keeps: every type it offers must be
	// one finder.createFile accepts, or the submenu carries a row that fails.
	for _, ft := range menu.FileTypes {
		_, rerr := callFinder(t, finderMenuHandlers(c)["finder.createFile"],
			map[string]any{"dir": t.TempDir(), "ext": ft.Ext})
		if rerr != nil {
			t.Errorf("the menu offers .%s but createFile refuses it: %+v", ft.Ext, rerr)
		}
	}
}

// TestEveryUnsupportedRowSaysWhy: supported is a verdict and it is not a
// sentence. Two of the seven rows are off on EVERY host — clipboard.
// copyRelativePath and filesystem.duplicate are named by findermenu's table but
// intent.DefaultRegistry does not carry them, so nothing dispatches them even
// though both Finder verbs have working handlers over IPC. A row that is merely
// greyed tells a person nothing they can act on, and the failure it hides is
// the one a user cannot see from the keyboard they are still holding.
//
// So: every unsupported row names its own capability, and the two refusals stay
// DISTINGUISHABLE. "Not in the registry" means one canonical-registry change
// would light the row up everywhere; "the adapter refuses this platform" means
// nothing anyone can do on that host. Collapsing them into one grey box loses
// exactly the distinction that tells a person whether to file anything at all.
func TestEveryUnsupportedRowSaysWhy(t *testing.T) {
	restore := finderHost
	t.Cleanup(func() { finderHost = restore })

	for _, host := range []string{"darwin", "windows"} {
		finderHost = host
		rows := mustMenuRows(t, testCore(t), "core.finderMenu", nil)

		for _, row := range rows {
			_, inRegistry := intent.DefaultRegistry().Get(row.Capability)
			switch {
			case row.Supported && row.Reason != "":
				t.Errorf("%s: row %s is supported but carries a reason: %q", host, row.ID, row.Reason)
			case !row.Supported && strings.TrimSpace(row.Reason) == "":
				t.Errorf("%s: row %s (%s) is unsupported with no words attached — "+
					"a greyed row a person cannot act on", host, row.ID, row.Capability)
			case row.Reason != "" && !strings.Contains(row.Reason, row.Capability):
				t.Errorf("%s: row %s names the wrong capability: %q", host, row.ID, row.Reason)
			}
			if inRegistry {
				continue
			}
			// The registry gap is the one both hosts must agree on, and it
			// must be the one reason given: a Windows build must not blame its
			// platform for a capability the dispatcher never carried.
			if !strings.Contains(row.Reason, "registry") {
				t.Errorf("%s: row %s is not in the registry, so that is what the "+
					"reason has to say; it says %q", host, row.ID, row.Reason)
			}
		}
	}

	// The two rows the bead names, on the host a user is actually on.
	rows := mustMenuRows(t, testCore(t), "core.finderMenu", nil)
	for _, id := range []string{"copyRelativePath", "duplicateWithName"} {
		row := menuRowByIDIn(rows, id)
		if row.Supported {
			t.Errorf("row %s reports supported, but %s is not in the intent registry",
				id, row.Capability)
		}
	}
	// The platform refusal, which is a different sentence about a different
	// cause — and which a Windows user must be able to tell from the registry
	// gap without running anything.
	finderHost = "windows"
	row := menuRowByIDIn(mustMenuRows(t, testCore(t), "core.finderMenu", nil), "openTerminal")
	if row.Supported {
		t.Fatal("openTerminal reports supported on windows")
	}
	if strings.Contains(row.Reason, "registry") {
		t.Errorf("openTerminal is a live capability the platform refuses, so the "+
			"registry is not why: %q", row.Reason)
	}
	if !strings.Contains(row.Reason, "windows") {
		t.Errorf("the platform refusal does not name the host: %q", row.Reason)
	}
	// Copy Path is in the same hostAdapterNames bucket, so it moves together.
	if menuRowByIDIn(mustMenuRows(t, testCore(t), "core.finderMenu", nil), "copyPath").Supported {
		t.Error("copyPath is guarded by the same darwinOnly seam and must not be supported")
	}
	// New File is a plain os call, so it survives on every host.
	for _, id := range []string{"newFile", "newFolder"} {
		if !menuRowByIDIn(mustMenuRows(t, testCore(t), "core.finderMenu", nil), id).Supported {
			t.Errorf("row %s is a plain os call and must be supported off darwin too", id)
		}
	}
}

// menuRowByIDIn finds one row of a served table, and fails the test rather than
// returning a zero value: a lookup that misses silently is a zero row, whose
// Enabled=false would read as "the row is off" in a test about persistence.
func menuRowByIDIn(rows []menuRow, id string) menuRow {
	for _, row := range rows {
		if row.ID == id {
			return row
		}
	}
	panic("no menu row " + id)
}

// menuRowByID is the same lookup against a fresh read of the running daemon.
func menuRowByID(t *testing.T, c *Core, id string) menuRow {
	t.Helper()
	return menuRowByIDIn(mustMenuRows(t, c, "core.finderMenu", nil), id)
}

// mustFinderMenuEntries reads the menu the appex renders. It is not in
// methods() — it is the appex-facing handler set (finderMenuHandlers) — so
// going through c.methods() here would test a method nobody registers.
func mustFinderMenuEntries(t *testing.T, c *Core) finderMenu {
	t.Helper()
	res, rerr := callFinder(t, finderMenuHandlers(c)["finder.menuEntries"], nil)
	if rerr != nil {
		t.Fatalf("finder.menuEntries: %d %s", rerr.Code, rerr.Message)
	}
	menu, ok := res.(finderMenu)
	if !ok {
		t.Fatalf("finder.menuEntries returned %T", res)
	}
	return menu
}
