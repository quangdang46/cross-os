// Explorer page data: the Finder context menu and the file-type catalog
// (bead be-finderdata, spec §7.2).
//
// Two sources and three verbs, and nothing else about the page:
//   - core:finderMenu          the seven menu items, one row per toggle
//   - core:fileTypes           the catalog the New > submenu offers
//   - core.setMenuItemEnabled  {id, enabled} → the whole menu table
//   - core.setFileType         one row in, validated, the whole catalog out
//   - core.reorderFileTypes    an explicit id list → the whole catalog out
//
// The rows are core/pkg/findermenu's, not a second copy: that table is what
// the appex renders (finder.menuEntries serves the same values), so a page
// that listed its own seven would be a list that could disagree with the menu
// the user actually right-clicks. The catalog is core/pkg/filetype's, read
// and written through the settings store — the daemon owns the list because it
// is the thing that persists, and the extension is a client of it.
//
// Two rules carry over from pagedata.go, because this page is one more of the
// same kind: a list leaves in a fixed order (the shell polls it), and a write
// validates the whole payload before it replaces anything (a half-applied
// catalog is a menu offering a file the daemon will refuse to create).

package main

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strings"

	"crossos/core/pkg/filetype"
	"crossos/core/pkg/findermenu"
	"crossos/core/pkg/intent"
	"crossos/core/pkg/ipc"
)

// --- core:finderMenu ---

// menuRow is the wire row for core:finderMenu: the shared item, the user's
// toggle, and whether this host can run the item at all.
//
// The item is embedded rather than copied field by field, so a field added to
// findermenu.Item is a field the page shows with no second struct to update.
// contexts is never null: an item with no context is one nothing can offer.
type menuRow struct {
	findermenu.Item
	Supported bool `json:"supported"`
}

// hostAdapterNames are the capabilities whose adapter opens a process or talks
// to the window server, and so darwinOnly guards. Everything else the menu
// reaches is a plain os call that runs wherever the daemon does. darwinOnly
// itself only knows the running OS, so the table names which capabilities it
// is standing in for.
var hostAdapterNames = map[string]bool{
	"clipboard.copyPath":         true,
	"clipboard.copyRelativePath": true,
	"terminal.openAt":            true,
	"app.open":                   true,
}

// menuSupported answers whether this capability actually runs here. Two ways
// it does not, and the page has to be able to say which:
//
//   - The registry does not carry it. copyRelativePath and duplicateWithName
//     name the rows the intent registry has not gained yet (findermenu's own
//     header says so), so no adapter would be reached from a keyboard rule or
//     a menu click. Those rows are unsupported on every host, and marking them
//     otherwise would be a row that looks live and fails on click.
//   - The adapter exists but refuses this platform. The gate is finderHost,
//     which is runtime.GOOS in production — the same value darwinOnly reads,
//     and the same one the keyboard path refuses on — so a row says "disabled"
//     exactly when the key it is bound to would pass straight through. It is
//     read through the variable rather than through darwinOnly so a test can
//     ask what a Windows user sees without a Windows build.
//
// The Windows row is the point of both: a capability that quietly does nothing
// is the one failure a user cannot see from the keyboard they are still holding.
func menuSupported(capability string) bool {
	if _, ok := intent.DefaultRegistry().Get(capability); !ok {
		return false
	}
	if !hostAdapterNames[capability] {
		return true // no platform seam: a plain os call
	}
	return finderHost == "darwin"
}

// menuRows renders the menu in the table's order with the user's toggles
// applied. The slice is always the same length: a disabled item is a row the
// user can turn back on, not a row that vanished.
func (c *Core) menuRows() []menuRow {
	c.mu.Lock()
	off := make(map[string]bool, len(c.menuOff))
	for id, disabled := range c.menuOff {
		off[id] = disabled
	}
	c.mu.Unlock()

	out := make([]menuRow, 0, len(findermenu.Menu))
	for _, item := range findermenu.Menu {
		if off[item.ID] {
			item.Enabled = false
		}
		out = append(out, menuRow{Item: item, Supported: menuSupported(item.Capability)})
	}
	return out
}

// handleFinderMenu serves core:finderMenu: the seven §7.2 items, each with the
// verb it runs, where it appears, whether the user has it on, and whether this
// host can run it.
//
// The whole table, always. An empty menu on a host with no Finder would be the
// wrong answer: these are declared capabilities, and supported=false is how the
// page says so without a row that looks live and is not.
func (c *Core) handleFinderMenu(_ json.RawMessage) (any, *ipc.RPCError) {
	return c.menuRows(), nil
}

// handleSetMenuItemEnabled serves core.setMenuItemEnabled: {id, enabled} → the
// whole menu table.
//
// An unknown id is a typed error, never a toggle that silently did nothing: the
// page re-renders from the reply, so a silent no-op would leave the switch
// showing a state the user did not choose. A known id is stored whatever
// supported says — supported is the host's answer to "would this run", and
// enabled is the user's answer to "do I want it"; the page disables the
// control on an unsupported host, and the daemon does not overrule a person
// who set the flag anyway.
func (c *Core) handleSetMenuItemEnabled(raw json.RawMessage) (any, *ipc.RPCError) {
	var p struct {
		ID      string `json:"id"`
		Enabled bool   `json:"enabled"`
	}
	if err := json.Unmarshal(raw, &p); err != nil || strings.TrimSpace(p.ID) == "" {
		return nil, &ipc.RPCError{Code: ipc.ErrBadParams, Message: "need {id, enabled}"}
	}
	if _, ok := findermenu.Lookup(p.ID); !ok {
		return nil, &ipc.RPCError{Code: ipc.ErrInvalid,
			Message: fmt.Sprintf("no menu item %q", p.ID)}
	}
	c.mu.Lock()
	c.menuOff[p.ID] = !p.Enabled
	c.mu.Unlock()
	return c.menuRows(), nil
}

// --- core:fileTypes ---

// fileTypeRow is the wire row for core:fileTypes: the catalog's own row, plus
// the derived menu label. Embedded for the same reason menuRow is — a field
// added to the catalog is a field the menu shows, with no second struct to
// update and nothing to forget.
type fileTypeRow struct {
	filetype.FileType
	MenuTitle string `json:"menuTitle"`
}

// fileTypeRows renders the catalog in stored order. menuTitle is recomputed
// per read rather than stored: it is a function of displayName and ext, and a
// stored copy is a second thing to keep true.
func fileTypeRows(set []filetype.FileType) []fileTypeRow {
	out := make([]fileTypeRow, 0, len(set))
	for _, f := range set {
		out = append(out, fileTypeRow{FileType: f, MenuTitle: f.MenuTitle()})
	}
	return out
}

// handleFileTypes serves core:fileTypes: the presets the New > submenu offers.
//
// A store that has never been configured hands back the eight seeds rather
// than an empty list (settings.Store.FileTypes), because a New menu with
// nothing in it is a menu the daemon has not built yet. The order is the order
// the editor left, so a reorder survives a restart instead of being re-sorted
// out from under the user.
func (c *Core) handleFileTypes(_ json.RawMessage) (any, *ipc.RPCError) {
	return fileTypeRows(c.set.FileTypes()), nil
}

// fileTypeID is the handle a reorder names a row by: the filename the preset
// creates. The extension alone is not an identity — the catalog allows two
// presets to share one ("data.json" and "report.json" are two presets, not a
// duplicate) — and a blank base name is a dotfile, so the id carries its dot.
func fileTypeID(f filetype.FileType) string {
	if f.BaseName == "" {
		return "." + f.Ext
	}
	return f.BaseName + "." + f.Ext
}

// handleSetFileType serves core.setFileType: one catalog row in, the whole
// catalog out.
//
// The row's identity is (ext, baseName) and it is the stored row's, not the
// caller's: the payload is a read-modify-write of an existing preset, not a way
// to mint one. A caller that changed the extension would be creating a
// different preset, and this page is where a person enables a capability which
// is already written — a brand-new row is a capability nobody implemented, and
// the menu would offer it anyway. The editable fields (displayName, template,
// enabled) are the user's; builtIn stays the stored flag.
func (c *Core) handleSetFileType(raw json.RawMessage) (any, *ipc.RPCError) {
	var p filetype.FileType
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, &ipc.RPCError{Code: ipc.ErrBadParams, Message: "need one file-type row"}
	}
	if strings.TrimSpace(p.Ext) == "" {
		return nil, &ipc.RPCError{Code: ipc.ErrBadParams, Message: "the row names no extension"}
	}
	current := c.set.FileTypes()
	idx := -1
	for i, f := range current {
		if f.Ext == p.Ext && f.BaseName == p.BaseName {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil, &ipc.RPCError{Code: ipc.ErrInvalid,
			Message: fmt.Sprintf("no file type %q", fileTypeID(p))}
	}
	row := current[idx]
	row.DisplayName = p.DisplayName
	row.Template = p.Template
	row.Enabled = p.Enabled
	next := append([]filetype.FileType(nil), current...)
	next[idx] = row
	// SetFileTypes validates the whole catalog, so an unusable edit leaves the
	// running one exactly as it was — never half a menu replaced.
	if err := c.set.SetFileTypes(next); err != nil {
		return nil, &ipc.RPCError{Code: ipc.ErrInvalid, Message: err.Error()}
	}
	return fileTypeRows(next), nil
}

// handleReorderFileTypes serves core.reorderFileTypes: {ids:[…]} → the whole
// catalog out.
//
// The list is the complete new order, not a move. A partial list would be a
// reorder composed from a drag the daemon could not see the end of — the rows
// it did not mention would keep their old positions and the user would be
// looking at a list that is neither the one they dragged nor the one they
// started with. So the payload must be a permutation of exactly the ids the
// catalog holds: no id twice, none missing, none invented. A list that fails
// that is refused whole, and the running order does not move.
func (c *Core) handleReorderFileTypes(raw json.RawMessage) (any, *ipc.RPCError) {
	var p struct {
		IDs []string `json:"ids"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, &ipc.RPCError{Code: ipc.ErrBadParams, Message: "need {ids:[…]}"}
	}
	current := c.set.FileTypes()
	if len(p.IDs) != len(current) {
		return nil, &ipc.RPCError{Code: ipc.ErrInvalid,
			Message: fmt.Sprintf("reorder lists %d ids for %d file types; the list must name them all",
				len(p.IDs), len(current))}
	}
	byID := make(map[string]filetype.FileType, len(current))
	for _, f := range current {
		byID[fileTypeID(f)] = f
	}
	next := make([]filetype.FileType, 0, len(p.IDs))
	for _, id := range p.IDs {
		f, ok := byID[id]
		if !ok {
			return nil, &ipc.RPCError{Code: ipc.ErrInvalid,
				Message: fmt.Sprintf("no file type %q", id)}
		}
		// Consumed on the way through, so a list that names one id twice
		// misses the second time and is refused rather than silently
		// duplicating a row.
		delete(byID, id)
		next = append(next, f)
	}
	if err := c.set.SetFileTypes(next); err != nil {
		return nil, &ipc.RPCError{Code: ipc.ErrInvalid, Message: err.Error()}
	}
	return fileTypeRows(next), nil
}

// finderHost is the host the support flags are answered for. It is a variable
// so a test can ask the other question — what does a Windows user see — without
// a Windows build; production never assigns it.
var finderHost = runtime.GOOS
