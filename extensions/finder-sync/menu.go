// Menu model — the tag-indexed snapshot pattern adapted from newfile.
//
// newfile's FinderSync keeps menuEntrySnapshot indexed by NSMenuItem.tag
// because representedObject can't carry a Swift struct across the FinderSync
// XPC bridge; the action handler looks the entry up by tag. Same constraint
// applies to the CrossOS appex, so the menu model is a snapshot: build once
// per menu-open, resolve by index on action.
package findersync

import (
	"fmt"

	"crossos/core/pkg/findermenu"
)

// MenuEntry is one row in the Finder context/toolbar menu.
type MenuEntry struct {
	// Title is the rendered label (e.g. "New Markdown").
	Title string
	// Action is the JSON-RPC method sent to the daemon on click — one of the
	// closed set in DaemonMethods, never code, never a path to execute
	// (§3.7: requests only).
	Action string
	// Ext is the file extension the daemon creates (e.g. "md").
	Ext string
}

// allowedActions is the closed set of action verbs the extension may send:
// the seven menu rows under the names the daemon actually serves them. It is
// derived rather than hand-listed, because a hand-kept second list is how an
// appex ends up offering a verb the daemon dropped — and that failure looks
// like a menu item that does nothing on click, with nothing in any log naming
// the row that drifted. The menu query is NOT an action, so
// MethodMenuEntries is absent here and present only in DaemonMethods.
var allowedActions = func() map[string]bool {
	out := make(map[string]bool, len(findermenu.Menu))
	for _, item := range findermenu.Menu {
		if verb := Verb(item.ID); verb != "" {
			out[verb] = true
		}
	}
	return out
}()

// Menu is an immutable snapshot built at menu-open time.
type Menu struct {
	entries []MenuEntry
}

// BuildMenu validates entries and freezes the snapshot. Empty input yields
// the single "empty row" (mirrors newfile's appendEmptyRow: guide the user to
// enable a type instead of showing a dead empty menu).
func BuildMenu(entries []MenuEntry) (Menu, error) {
	if len(entries) == 0 {
		return Menu{entries: []MenuEntry{{Title: "Enable a file type in CrossOS…", Action: MethodMenuEntries}}}, nil
	}
	for i, e := range entries {
		if e.Title == "" {
			return Menu{}, fmt.Errorf("menu entry %d: empty title", i)
		}
		if !allowedActions[e.Action] {
			return Menu{}, fmt.Errorf("menu entry %d (%q): action %q not in closed verb set", i, e.Title, e.Action)
		}
	}
	cp := append([]MenuEntry(nil), entries...)
	return Menu{entries: cp}, nil
}

// Len reports the snapshot size.
func (m Menu) Len() int { return len(m.entries) }

// Lookup resolves a tag (menu item index) to its entry, mirroring
// createFromMenuItem's tag → snapshot lookup. Out-of-range tags are a typed
// error (newfile beeps; we return the error so the caller — and stage logs —
// name it instead of acting on a wrong entry).
func (m Menu) Lookup(tag int) (MenuEntry, error) {
	if tag < 0 || tag >= len(m.entries) {
		return MenuEntry{}, fmt.Errorf("menu tag %d out of range (snapshot=%d)", tag, len(m.entries))
	}
	return m.entries[tag], nil
}

// Entries returns a copy of the snapshot (for IPC send / tests).
func (m Menu) Entries() []MenuEntry { return append([]MenuEntry(nil), m.entries...) }
