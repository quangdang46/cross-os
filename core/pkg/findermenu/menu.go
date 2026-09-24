// Package findermenu is the Explorer's right-click menu: the seven verbs, the
// three selection contexts they appear in, the editor catalog behind Open in
// Editor, and the filename rule that keeps a new name off an existing one.
//
// The item/context table is ported from extensions/finder-sync/menus.go:56-94 —
// the same §6.3 rows the extension builds its FIFinderSync menu from. The
// extension keeps its own copy because crossos/finder_sync depends on
// crossos/core and that dependency does not run the other way, so the daemon
// cannot reach across to it. The seven ids are the spec's Explorer menu
// (ChatGPT-CROSSOS-20260924-2106:1007-1013); copyRelativePath and
// duplicateWithName are new, and openTerminal is the one row that changed shape
// — it now names a directory rather than a selection.
package findermenu

import (
	"fmt"
	"path"
	"strings"
)

// Context is the Finder selection context an item appears in. Strings on the
// wire because the appex switches on the name; an int would make the appex
// carry this file's declaration order in its own source.
type Context string

const (
	ContextFile   Context = "file"
	ContextFolder Context = "folder"
	ContextEmpty  Context = "empty"
)

// Contexts is every context a menu can be built in, in the order the appex
// asks for them.
var Contexts = []Context{ContextFile, ContextFolder, ContextEmpty}

// MaxPathCount caps a multi-selection verb, ported from
// extensions/finder-sync/menus.go. The cap is the same number the keyboard
// rules dispatch under, so a selection the keyboard refuses is one the menu
// refuses.
const MaxPathCount = 100

// Item is one menu row: declarative, and native capability only. Nothing here
// carries a shell string, so a menu can be rendered but not aimed.
type Item struct {
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	Contexts   []Context `json:"contexts"`
	NeedsPaths bool      `json:"needsPaths"`
	Capability string    `json:"capability"`
	Enabled    bool      `json:"enabled"`
}

// Menu is the seven-item table, in the order the appex shows it.
//
// copyRelativePath and duplicateWithName name capabilities the intent registry
// does not carry: core/pkg/intent/registry_v1.go is not this file's to edit,
// and until the rows land there neither verb can be reached by a keyboard rule.
// The names are the ones those rows will take, so a rule that starts working
// later does not also need the menu renamed.
var Menu = []Item{
	{ID: "newFile", Title: "New File", Contexts: []Context{ContextEmpty, ContextFolder}, Capability: "filesystem.createFile", Enabled: true},
	{ID: "newFolder", Title: "New Folder", Contexts: []Context{ContextEmpty, ContextFolder}, Capability: "filesystem.createFolder", Enabled: true},
	{ID: "copyPath", Title: "Copy Path", Contexts: []Context{ContextFile, ContextFolder}, NeedsPaths: true, Capability: "clipboard.copyPath", Enabled: true},
	{ID: "copyRelativePath", Title: "Copy Relative Path", Contexts: []Context{ContextFile, ContextFolder}, NeedsPaths: true, Capability: "clipboard.copyRelativePath", Enabled: true},
	{ID: "openTerminal", Title: "Open in Terminal", Contexts: []Context{ContextFile, ContextFolder, ContextEmpty}, Capability: "terminal.openAt", Enabled: true},
	{ID: "openEditor", Title: "Open in Editor", Contexts: []Context{ContextFile, ContextFolder}, NeedsPaths: true, Capability: "app.open", Enabled: true},
	{ID: "duplicateWithName", Title: "Duplicate with Name", Contexts: []Context{ContextFile, ContextFolder}, NeedsPaths: true, Capability: "filesystem.duplicate", Enabled: true},
}

// ForContext returns the items visible in ctx, a fresh slice: the caller
// renders it and the table is shared by every request.
func ForContext(ctx Context) []Item {
	var out []Item
	for _, item := range Menu {
		for _, c := range item.Contexts {
			if c == ctx {
				out = append(out, item)
				break
			}
		}
	}
	return out
}

// Lookup finds one item by its menu id.
func Lookup(id string) (Item, bool) {
	for _, item := range Menu {
		if item.ID == id {
			return item, true
		}
	}
	return Item{}, false
}

// Editor is one entry in the Open in Editor submenu. ID is the bundle id,
// which is also what `open -b` takes, so the catalog and the launch cannot
// name an editor two different ways.
type Editor struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Editors is the Open in Editor catalog, and its order IS the submenu order.
//
// One ordered list, read by the appex to build the submenu and by openEditor to
// resolve an id — a submenu that offers an editor the daemon would refuse is a
// menu item that fails on click, so the two ends cannot be separate lists.
//
// The entries are CrossOS's own, not a port of FinderRight's
// (tmp/research/FinderRight/…/EditorCatalog.swift:16-31, MIT): what transfers
// from that file is the single-source rule above, not its Swift.
var Editors = []Editor{
	{ID: "com.microsoft.VSCode", Name: "Visual Studio Code"},
	{ID: "com.todesktop.230313mzl4w4u92", Name: "Cursor"},
	{ID: "dev.zed.Zed", Name: "Zed"},
	{ID: "com.apple.dt.Xcode", Name: "Xcode"},
	{ID: "com.sublimetext.4", Name: "Sublime Text"},
	{ID: "com.panic.Nova", Name: "Nova"},
}

// EditorByID resolves a catalog entry. A miss is a refusal, not a fallback to
// some other editor: silently opening a file in a program the user did not
// pick is not a smaller failure than saying the id is unknown.
func EditorByID(id string) (Editor, bool) {
	for _, e := range Editors {
		if e.ID == id {
			return e, true
		}
	}
	return Editor{}, false
}

// UniqueName returns a non-colliding name inside dir for baseName+ext. Ports
// FilenameGenerator.uniqueFileURL (tmp/research/newfile, commit b3f665a) as
// attributed in third_party/newfile/ATTRIBUTION.md:1-7 — strip one redundant
// ".<ext>" suffix, empty base becomes a dotfile, then "<base>.<ext>",
// "<base> 2.<ext>", … The collision probe is injected, the same seam as the
// Swift fileExists closure.
//
// ext "" is the folder case, which the newfile helper never had: a folder has
// no extension, so the candidate is the bare base and the increment is " 2"
// with no trailing dot. The file path is the port unchanged.
func UniqueName(dir, baseName, ext string, exists func(string) bool) string {
	base := baseName
	suffix := "." + ext
	if len(base) >= len(suffix) && strings.EqualFold(base[len(base)-len(suffix):], suffix) {
		base = base[:len(base)-len(suffix)]
	}
	candidate := func(i int) string {
		switch {
		case base == "":
			if i == 1 {
				return path.Join(dir, "."+ext)
			}
			return path.Join(dir, fmt.Sprintf(".%s %d", ext, i))
		case ext == "":
			if i == 1 {
				return path.Join(dir, base)
			}
			return path.Join(dir, fmt.Sprintf("%s %d", base, i))
		case i == 1:
			return path.Join(dir, base+"."+ext)
		default:
			return path.Join(dir, fmt.Sprintf("%s %d.%s", base, i, ext))
		}
	}
	for i := 1; ; i++ {
		if c := candidate(i); !exists(c) {
			return c
		}
	}
}
