// Menu model — the tag-indexed snapshot pattern adapted from newfile.
//
// newfile's FinderSync keeps menuEntrySnapshot indexed by NSMenuItem.tag
// because representedObject can't carry a Swift struct across the FinderSync
// XPC bridge; the action handler looks the entry up by tag. Same constraint
// applies to the CrossOS appex, so the menu model is a snapshot: build once
// per menu-open, resolve by index on action.
package findersync

import "fmt"

// MenuEntry is one row in the Finder context/toolbar menu.
type MenuEntry struct {
	// Title is the rendered label (e.g. "New Markdown").
	Title string
	// Action is the request name sent to the daemon (e.g. "createFile").
	// It is a fixed verb from a closed set — never code, never a path to
	// execute (§3.7: requests only).
	Action string
	// Ext is the file extension the daemon creates (e.g. "md").
	Ext string
}

// allowedActions is the closed verb set the extension may request.
var allowedActions = map[string]bool{
	"createFile": true,
	"openPrefs":  true,
}

// Menu is an immutable snapshot built at menu-open time.
type Menu struct {
	entries []MenuEntry
}

// BuildMenu validates entries and freezes the snapshot. Empty input yields
// the single "empty row" (mirrors newfile's appendEmptyRow: guide the user to
// enable a type instead of showing a dead empty menu).
func BuildMenu(entries []MenuEntry) (Menu, error) {
	if len(entries) == 0 {
		return Menu{entries: []MenuEntry{{Title: "Enable a file type in CrossOS…", Action: "openPrefs"}}}, nil
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
