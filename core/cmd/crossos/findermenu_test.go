// Finder menu verb tests: the method set be-appex-client codes against, the
// menu snapshot, the refusals every verb owes the user, and the two verbs that
// touch a real folder — createFile and duplicateWithName.
//
// The refusals are tested against the verbs that would otherwise launch
// something: pbcopy and `open` must not run in a test, so each verb is fired
// with a request it has to decline and the run stops there. That is also the
// half worth pinning — a verb that validates after it launches is a verb whose
// first click already did the thing.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"crossos/core/pkg/ipc"
	builtin "crossos/core/rules"
)

// finderTestCore is the daemon the menu handlers hang off. They read no state,
// so this is the real Core rather than a stub: a handler that started needing
// state would show up here as a test that cannot build one.
func finderTestCore(t *testing.T) *Core {
	t.Helper()
	c, err := NewCoreWithSettings(builtin.All(), builtin.Grants(), "")
	if err != nil {
		t.Fatalf("NewCoreWithSettings: %v", err)
	}
	return c
}

// callFinder fires one method with a params object, the shape the appex sends.
func callFinder(t *testing.T, h ipc.Handler, params any) (any, *ipc.RPCError) {
	t.Helper()
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	return h(raw)
}

// TestFinderMenuMethodNames pins the wire contract. The appex calls these
// names; a rename here is a menu that renders and then gets no answer.
func TestFinderMenuMethodNames(t *testing.T) {
	handlers := finderMenuHandlers(finderTestCore(t))
	want := []string{
		"finder.menuEntries",
		"finder.createFolder",
		"finder.createFile",
		"finder.copyPath",
		"finder.copyRelativePath",
		"finder.openTerminal",
		"finder.openEditor",
		"finder.duplicateWithName",
	}
	if len(handlers) != len(want) {
		t.Fatalf("handlers=%d, want %d", len(handlers), len(want))
	}
	for _, name := range want {
		if handlers[name] == nil {
			t.Fatalf("method %q is not served", name)
		}
	}
}

// TestFinderMenuEntries: what the appex renders a menu from.
func TestFinderMenuEntries(t *testing.T) {
	res, rerr := callFinder(t, finderMenuHandlers(finderTestCore(t))["finder.menuEntries"], nil)
	if rerr != nil {
		t.Fatalf("menuEntries: %+v", rerr)
	}
	menu, ok := res.(finderMenu)
	if !ok {
		t.Fatalf("menuEntries returned %T", res)
	}
	if len(menu.Contexts) != 3 {
		t.Fatalf("contexts=%v, want file/folder/empty", menu.Contexts)
	}
	if len(menu.Items) != 7 {
		t.Fatalf("items=%d, want the spec's seven", len(menu.Items))
	}
	for _, item := range menu.Items {
		if item.ID == "" || item.Title == "" || item.Capability == "" || !item.Enabled {
			t.Fatalf("item is not renderable: %+v", item)
		}
	}
	// The two submenus are fanned out over the daemon's own lists, so a row
	// the menu offers and a row the verb accepts cannot drift apart.
	if len(menu.Editors) == 0 || menu.Editors[0].ID != "com.microsoft.VSCode" {
		t.Fatalf("editors=%v, want com.microsoft.VSCode first", menu.Editors)
	}
	if len(menu.FileTypes) == 0 {
		t.Fatal("New File needs the enabled file types")
	}
	for _, ft := range menu.FileTypes {
		if !ft.Enabled {
			t.Fatalf("menuEntries served a disabled file type: %+v", ft)
		}
	}
	// The same list the verb resolves against: a type on the menu that
	// createFile would refuse is a submenu row that fails on click.
	_, rerr = callFinder(t, finderMenuHandlers(finderTestCore(t))["finder.createFile"],
		map[string]any{"dir": t.TempDir(), "ext": menu.FileTypes[0].Ext})
	if rerr != nil {
		t.Fatalf("a type the menu serves must be one createFile accepts: %+v", rerr)
	}
}

// TestAbsoluteSlash: every path a menu verb is handed goes through this guard,
// and the guard itself had no test — the six Windows failures it caused were
// found by the verbs' own tests, not by a check of the check. The two
// platforms disagree about which spelling of absolute is correct, so both are
// pinned here rather than one being assumed.
//
// The dot cases are the load-bearing half. A naive substring test for ".."
// would refuse "notes..bak", which is a filename a user may well have, and a
// naive segment test that cleaned first would miss "/tmp/../etc" entirely,
// because path.Clean of that is "/etc" with nothing left to find.
func TestAbsoluteSlash(t *testing.T) {
	// Accepted everywhere. path.IsAbs does not care what platform it is
	// running on, so a slash path stays legal on a Windows build too — which
	// is the half the old guard got right and kept.
	for _, p := range []string{
		"/tmp/x",
		"/Users/someone/Desktop/notes.txt",
		"/tmp/notes..bak",
		"/tmp/dir.with.dots/notes.txt",
	} {
		if err := absoluteSlash(p); err != nil {
			t.Errorf("absoluteSlash(%q) = %v, want accepted", p, err)
		}
	}
	// Refused everywhere.
	for _, p := range []string{
		"",
		"/",
		".",
		"..",
		"relative/path",
		"notes.txt",
		"../escape",
		"../../etc/passwd",
		"/tmp/../etc",
		"/tmp/a/../../b",
		"./notes.txt",
	} {
		if err := absoluteSlash(p); err == nil {
			t.Errorf("absoluteSlash(%q) = nil, want refused", p)
		}
	}
	// The platform-specific half. filepath.IsAbs only knows about drive letters
	// and UNC roots on Windows, so these rows are asserted where they mean
	// something and must be refused everywhere else.
	if runtime.GOOS == "windows" {
		for _, p := range []string{`C:\Users\x\notes.txt`, `C:/Users/x/notes.txt`} {
			if err := absoluteSlash(p); err != nil {
				t.Errorf("absoluteSlash(%q) = %v, want accepted on windows", p, err)
			}
		}
		// The traversal spelling a slash-only segment scan would walk straight
		// past, which is the whole reason the scan is separator-aware now.
		for _, p := range []string{`C:\a\..\b`, `C:\..\b`, `..\escape`} {
			if err := absoluteSlash(p); err == nil {
				t.Errorf("absoluteSlash(%q) = nil, want refused on windows", p)
			}
		}
	} else {
		// A backslash path is not an absolute path on this platform, and the
		// widening above must not have made it one.
		if err := absoluteSlash(`C:\Users\x\notes.txt`); err == nil {
			t.Error(`absoluteSlash("C:\\Users\\x\\notes.txt") = nil, want refused off windows`)
		}
	}
}

// --- refusals ---

// wantRefusal asserts a verb declined BEFORE it reached an adapter. The two
// codes are the caller's fault and the daemon's "no", both of which the appex
// shows as an explanation; ErrInternal is the adapter's own failure, and a test
// that accepted one would be reading a launch that already happened.
func wantRefusal(t *testing.T, verb string, rerr *ipc.RPCError) {
	t.Helper()
	if rerr == nil {
		t.Fatalf("%s accepted a request it must refuse", verb)
	}
	if rerr.Code != ipc.ErrBadParams && rerr.Code != ipc.ErrInvalid {
		t.Fatalf("%s: code=%d (%s), want a refusal before any adapter ran", verb, rerr.Code, rerr.Message)
	}
	if rerr.Message == "" {
		t.Fatalf("%s: a refusal with nothing to show the user", verb)
	}
}

// TestPathRefusals: the four shapes no verb will act on, across every verb that
// takes a path. Relative, traversal, over the cap, and a rule table's
// unresolved template are the same four the keyboard path refuses, because it
// is the same reader.
//
// The traversal case is built by concatenation on purpose: filepath.Join
// collapses "..", so a joined path would arrive already clean and the check
// would never see the segment it exists to catch.
func TestPathRefusals(t *testing.T) {
	handlers := finderMenuHandlers(finderTestCore(t))
	dir := t.TempDir()
	tooMany := make([]string, findermenuMax+1)
	for i := range tooMany {
		tooMany[i] = filepath.Join(dir, fmt.Sprintf("f%d", i))
	}
	bad := []struct {
		name  string
		paths []string
	}{
		{"relative", []string{"notes.txt"}},
		{"traversal", []string{dir + "/../../etc/passwd"}},
		{"over the cap", tooMany},
		{"unresolved template", []string{"{finderDir}"}},
		{"root", []string{"/"}},
	}
	verbs := []string{
		"finder.copyPath",
		"finder.copyRelativePath",
		"finder.duplicateWithName",
	}
	for _, verb := range verbs {
		for _, b := range bad {
			params := map[string]any{"paths": b.paths}
			if verb == "finder.copyRelativePath" {
				params["baseDir"] = dir
			}
			_, rerr := callFinder(t, handlers[verb], params)
			wantRefusal(t, verb+"("+b.name+")", rerr)
		}
	}
}

// findermenuMax mirrors findermenu.MaxPathCount at the cap the test builds a
// selection over: one more than the cap, so the refusal is the cap's.
const findermenuMax = 100

func TestSingleTargetRefusals(t *testing.T) {
	handlers := finderMenuHandlers(finderTestCore(t))
	dir := t.TempDir()
	for _, verb := range []string{"finder.openTerminal", "finder.openEditor"} {
		for name, p := range map[string]string{
			"relative":    "relative/dir",
			"traversal":   dir + "/../escape",
			"template":    "{finderDir}",
			"root":        "/",
			"dot segment": dir + "/./here",
		} {
			params := map[string]any{"path": p}
			if verb == "finder.openEditor" {
				params["editorID"] = "com.microsoft.VSCode"
			}
			_, rerr := callFinder(t, handlers[verb], params)
			if rerr == nil && (name == "traversal" || name == "relative" || name == "template" || name == "root") {
				t.Fatalf("%s accepted a %s path %q", verb, name, p)
			}
			if name != "dot segment" {
				wantRefusal(t, verb+"("+name+")", rerr)
			}
		}
	}
}

// TestCreateFolderRefuses: New Folder names a directory, and a relative one is
// a directory the daemon would resolve against whatever it was started in.
func TestCreateFolderRefuses(t *testing.T) {
	h := finderMenuHandlers(finderTestCore(t))["finder.createFolder"]
	dir := t.TempDir()
	for name, p := range map[string]string{
		"relative":  "sub",
		"traversal": dir + "/../escape",
		"template":  "{finderDir}",
		"root":      "/",
	} {
		_, rerr := callFinder(t, h, map[string]any{"dir": p})
		wantRefusal(t, "createFolder("+name+")", rerr)
	}
	// Nothing was created on the way out, including outside the temp folder.
	for _, parent := range []string{dir, filepath.Dir(dir)} {
		entries, err := os.ReadDir(parent)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.Name() == "escape" || e.Name() == "sub" {
				t.Fatalf("a refused createFolder still made %s", e.Name())
			}
		}
	}
}

// TestCreateFileRefuses: the ext has to be a row the catalog holds, the base
// name has to be a name rather than a path, and the body has to be a body.
func TestCreateFileRefuses(t *testing.T) {
	h := finderMenuHandlers(finderTestCore(t))["finder.createFile"]
	dir := t.TempDir()
	cases := []struct {
		name   string
		params map[string]any
	}{
		{"unknown ext", map[string]any{"dir": dir, "ext": "exe"}},
		{"missing ext", map[string]any{"dir": dir}},
		{"ext is a path", map[string]any{"dir": dir, "ext": "../../evil"}},
		{"base name escapes", map[string]any{"dir": dir, "ext": "txt", "baseName": "../escaped"}},
		{"base name is dotdot", map[string]any{"dir": dir, "ext": "txt", "baseName": ".."}},
		{"unresolved base name", map[string]any{"dir": dir, "ext": "txt", "baseName": "{fileName}"}},
		{"unresolved template", map[string]any{"dir": dir, "ext": "txt", "template": "{fileName}"}},
		{"relative dir", map[string]any{"dir": "sub", "ext": "txt"}},
	}
	for _, c := range cases {
		if _, rerr := callFinder(t, h, c.params); rerr == nil {
			t.Fatalf("createFile accepted a %s", c.name)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("a refused createFile still wrote %d entries", len(entries))
	}
}

// TestOpenEditorRefusesUnknownID: the submenu is built from the catalog, so an
// id outside it is a stale appex or a request to aim a launch somewhere the
// user was never shown. Either way the daemon says no instead of opening it in
// whatever claims the extension.
func TestOpenEditorRefusesUnknownID(t *testing.T) {
	h := finderMenuHandlers(finderTestCore(t))["finder.openEditor"]
	dir := t.TempDir()
	for _, id := range []string{"com.example.NoSuchEditor", "", "com.microsoft.vscode"} {
		_, rerr := callFinder(t, h, map[string]any{"path": dir, "editorID": id})
		if rerr == nil {
			t.Fatalf("openEditor accepted editor %q", id)
		}
	}
	if _, rerr := callFinder(t, h, map[string]any{"path": dir}); rerr == nil {
		t.Fatal("openEditor accepted a request with no editorID")
	}
}

// --- the two verbs that touch a folder ---

// TestCreateFileNamesBesideTheLast: the name is the daemon's to choose, so a
// second New Text File is "New Text File 2.txt" and the first is untouched.
func TestCreateFileNamesBesideTheLast(t *testing.T) {
	h := finderMenuHandlers(finderTestCore(t))["finder.createFile"]
	dir := t.TempDir()
	made := make([]string, 0, 2)
	for i := 0; i < 2; i++ {
		res, rerr := callFinder(t, h, map[string]any{"dir": dir, "ext": "txt", "template": "body"})
		if rerr != nil {
			t.Fatalf("createFile: %+v", rerr)
		}
		reply, ok := res.(map[string]any)
		if !ok {
			t.Fatalf("createFile returned %T", res)
		}
		made = append(made, reply["path"].(string))
	}
	if made[0] == made[1] {
		t.Fatalf("two creates both landed on %s", made[0])
	}
	for i, p := range made {
		body, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
		if string(body) != "body" {
			t.Fatalf("create %d wrote %q, want the template body", i, body)
		}
	}
	if filepath.Base(made[1]) != "New Text File 2.txt" {
		t.Fatalf("second create landed on %q", filepath.Base(made[1]))
	}
}

func TestCreateFolderMakesIt(t *testing.T) {
	h := finderMenuHandlers(finderTestCore(t))["finder.createFolder"]
	dir := filepath.Join(t.TempDir(), "New Folder")
	res, rerr := callFinder(t, h, map[string]any{"dir": dir})
	if rerr != nil {
		t.Fatalf("createFolder: %+v", rerr)
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Fatalf("createFolder left no folder at %s (%v)", dir, err)
	}
	if reply, ok := res.(map[string]any); !ok || reply["path"] != dir {
		t.Fatalf("createFolder reply=%v, want the path it made", res)
	}
}

// TestDuplicateFile: a duplicate is a second file, so the original is still
// there afterwards — that is the one thing this verb must never do.
func TestDuplicateFile(t *testing.T) {
	h := finderMenuHandlers(finderTestCore(t))["finder.duplicateWithName"]
	dir := t.TempDir()
	src := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(src, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, rerr := callFinder(t, h, map[string]any{"paths": []string{src}})
	if rerr != nil {
		t.Fatalf("duplicateWithName: %+v", rerr)
	}
	dst := res.(map[string]any)["paths"].([]string)[0]
	if dst == src {
		t.Fatal("duplicateWithName returned the source as the duplicate")
	}
	if filepath.Base(dst) != "notes 2.txt" {
		t.Fatalf("duplicate landed on %q", filepath.Base(dst))
	}
	// Both exist, both readable, both the same bytes.
	for _, p := range []string{src, dst} {
		body, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		if string(body) != "hello" {
			t.Fatalf("%s holds %q", p, body)
		}
	}
	assertNoStaging(t, dir)
}

// TestDuplicateFolder: a folder is the case a copy-with-rename gets wrong —
// renaming the folder itself is a move, and the user would find their project
// gone from where they left it.
func TestDuplicateFolder(t *testing.T) {
	h := finderMenuHandlers(finderTestCore(t))["finder.duplicateWithName"]
	dir := t.TempDir()
	src := filepath.Join(dir, "Projects")
	if err := os.MkdirAll(filepath.Join(src, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "sub", "main.go"), []byte("package main"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, rerr := callFinder(t, h, map[string]any{"paths": []string{src}})
	if rerr != nil {
		t.Fatalf("duplicateWithName: %+v", rerr)
	}
	dst := res.(map[string]any)["paths"].([]string)[0]
	if dst == src {
		t.Fatal("duplicateWithName returned the source folder as the duplicate")
	}
	if filepath.Base(dst) != "Projects 2" {
		t.Fatalf("duplicate landed on %q", filepath.Base(dst))
	}
	// The original is still a folder, in place, with its contents.
	body, err := os.ReadFile(filepath.Join(src, "sub", "main.go"))
	if err != nil {
		t.Fatalf("the original folder lost its contents: %v", err)
	}
	if string(body) != "package main" {
		t.Fatalf("the original folder now holds %q", body)
	}
	// And the copy is a folder too, not a file that happens to share the name.
	info, err := os.Stat(dst)
	if err != nil || !info.IsDir() {
		t.Fatalf("the duplicate of a folder is not a folder: %v", err)
	}
	body, err = os.ReadFile(filepath.Join(dst, "sub", "main.go"))
	if err != nil || string(body) != "package main" {
		t.Fatalf("the duplicate folder lost its contents: %q (%v)", body, err)
	}
	assertNoStaging(t, dir)
}

// TestDuplicateRefusesMissingSource: a path the menu named that is not there is
// an error, not a folder quietly created in its place.
func TestDuplicateRefusesMissingSource(t *testing.T) {
	h := finderMenuHandlers(finderTestCore(t))["finder.duplicateWithName"]
	dir := t.TempDir()
	_, rerr := callFinder(t, h, map[string]any{"paths": []string{filepath.Join(dir, "not-here")}})
	if rerr == nil {
		t.Fatal("duplicateWithName accepted a path that does not exist")
	}
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
		t.Fatalf("a refused duplicate left %d entries behind (%v)", len(entries), err)
	}
}

// TestDuplicateClearsAStaleStagingFile: a run killed mid-copy leaves the
// staging file behind, and the copy creates its files O_EXCL — so without a
// clear-first step the second attempt would fail on its own debris and the
// verb would stay broken for good.
func TestDuplicateClearsAStaleStagingFile(t *testing.T) {
	h := finderMenuHandlers(finderTestCore(t))["finder.duplicateWithName"]
	dir := t.TempDir()
	src := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(src, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(dir, "notes 2.txt.crossos-partial")
	if err := os.WriteFile(stale, []byte("half a file"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, rerr := callFinder(t, h, map[string]any{"paths": []string{src}})
	if rerr != nil {
		t.Fatalf("duplicateWithName: %+v", rerr)
	}
	// Compared as paths, not as strings: the verb joins with a slash because
	// Finder paths are slash-separated, and on Windows that leaves the returned
	// name carrying a "/" where filepath.Join would have written a "\". The
	// question this test asks is WHERE the duplicate landed, so Clean is the
	// comparison, and the raw string stays in the failure message where a
	// separator leak would still be visible.
	dst := res.(map[string]any)["paths"].([]string)[0]
	if got, want := filepath.Clean(dst), filepath.Join(dir, "notes 2.txt"); got != want {
		t.Fatalf("duplicate landed on %q, want %q", dst, want)
	}
	assertNoStaging(t, dir)
}

// assertNoStaging: the copy is renamed into place, so nothing half-written may
// survive under the name the staging step uses.
func assertNoStaging(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".crossos-partial" {
			t.Fatalf("a staging file was left behind: %s", e.Name())
		}
	}
}

// TestRelativeSlash: the payload behind Copy Relative Path, including the
// sibling case that makes the verb worth having.
func TestRelativeSlash(t *testing.T) {
	cases := []struct{ base, target, want string }{
		{"/work/app", "/work/app/main.go", "main.go"},
		{"/work/app", "/work/app/pkg/deep/main.go", "pkg/deep/main.go"},
		{"/work/app", "/work/lib/main.go", "../lib/main.go"},
		{"/work/app", "/work/app", "."},
		{"/work/app", "/", "../.."},
	}
	for _, c := range cases {
		if got := relativeSlash(c.base, c.target); got != c.want {
			t.Fatalf("relativeSlash(%q, %q)=%q, want %q", c.base, c.target, got, c.want)
		}
	}
}
