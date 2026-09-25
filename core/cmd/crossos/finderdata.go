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
//
// The menu toggle PERSISTS, unlike the plugin maps this file's neighbour seeds
// into: core.setMenuItemEnabled is bound, the Service method exists, and the
// control re-renders from the reply, so a write that stayed in memory answered
// success and then lost the answer to the next restart. It writes through to
// the settings document and loadMenuOff seeds the running map back from it.

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
// toggle, and whether this host can run the item at all — with the daemon's own
// words for the "no".
//
// The item is embedded rather than copied field by field, so a field added to
// findermenu.Item is a field the page shows with no second struct to update.
// contexts is never null: an item with no context is one nothing can offer.
//
// Reason is the half a bare supported=false cannot carry. A greyed row with no
// sentence is the failure a person cannot see from the keyboard they are still
// holding, and "disabled" does not say WHICH of the two very different refusals
// applies. It is empty exactly when Supported is true, so the page never has to
// decide whether a reason belongs on a row that works.
type menuRow struct {
	findermenu.Item
	Supported bool   `json:"supported"`
	Reason    string `json:"reason,omitempty"`
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

// menuSupported answers whether this capability actually runs here. The two
// ways it does not, and the words for each, are menuUnavailableReason's — this
// is a predicate over it rather than a second copy of its tests, because
// supported and the reason the page prints beside it are one fact about one
// host, and two functions answering that fact separately is how a row ends up
// greyed with no sentence, or live with one.
func menuSupported(capability string) bool {
	return menuUnavailableReason(capability) == ""
}

// menuUnavailableReason is the daemon's own sentence for why this host cannot
// run a capability, or "" when it can — the single place the verdict and its
// wording are decided together.
//
// The two refusals are kept apart because they are different facts with
// different remedies, and a person shown one of them is being told a truth
// they can act on:
//
//   - The registry has no row for the capability. copyRelativePath and
//     duplicateWithName name it, and both have working handlers over IPC
//     (findermenu.go's handleFinderCopyRelativePath and
//     handleFinderDuplicateWithName), but intent.DefaultRegistry is the
//     dispatch table a keyboard rule and a menu click both go through, and it
//     does not carry them yet. So the sentence says the verb is not wired into
//     the dispatcher — NOT that the operation is missing, which would send
//     someone looking for a feature to build that is already written.
//   - The adapter is present and refuses this platform. That is a permanent
//     property of the host, so the sentence names the host rather than
//     apologising.
func menuUnavailableReason(capability string) string {
	if _, ok := intent.DefaultRegistry().Get(capability); !ok {
		return fmt.Sprintf(
			"%s is not in the daemon's intent registry yet, so nothing dispatches it. "+
				"The Finder menu verb is written and answers over IPC; adding the capability "+
				"to the registry is what would turn this row on, on every host at once.",
			capability)
	}
	if !hostAdapterNames[capability] {
		return "" // no platform seam: a plain os call
	}
	if finderHost == "darwin" {
		return ""
	}
	return fmt.Sprintf(
		"%s opens a host application or talks to the window server, and that adapter "+
			"runs only on macOS. This daemon is running on %s.",
		capability, finderHost)
}

// menuRows renders the menu in the table's order with the user's toggles
// applied. The slice is always the same length: a disabled item is a row the
// user can turn back on, not a row that vanished.
//
// The support verdict and its sentence are asked of one function, so a row can
// never be greyed without words or carry words on a row that works.
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
		reason := menuUnavailableReason(item.Capability)
		out = append(out, menuRow{
			Item:      item,
			Supported: reason == "",
			Reason:    reason,
		})
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
//
// The verdict is written THROUGH to the settings document before the running
// map moves, and the two orders are not interchangeable. This verb is bound,
// the Service method exists, and the control re-renders from the reply — so a
// write that only touched memory answered "done" and then lost the answer to
// the next restart, which is the shape of a promise the product does not keep.
// Writing the store first also means a failed write leaves memory and the
// document agreeing with each other, instead of leaving a row the daemon is
// serving a state it could not record.
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
	if err := c.set.SetMenuItemDisabled(p.ID, !p.Enabled); err != nil {
		return nil, &ipc.RPCError{Code: ipc.ErrInternal, Message: err.Error()}
	}
	c.mu.Lock()
	c.menuOff[p.ID] = !p.Enabled
	c.mu.Unlock()
	return c.menuRows(), nil
}

// loadMenuOff seeds the running per-item toggle map from the settings document,
// so a restart comes up with the verdicts the person left rather than
// re-enabling every row. loadPluginEnablement is the same shape for plugins and
// is called beside it on start.
//
// The default is the one the map already had: an id the document does not name
// is ON, because findermenu.Menu is what the product declares and a person
// narrows it. That direction matters for an added row — a menu item shipped
// after the document was written is in the menu until somebody switches it off,
// rather than arriving invisible because the document predates it.
func (c *Core) loadMenuOff() {
	off := make(map[string]bool)
	for _, id := range c.set.MenuOff() {
		off[id] = true
	}
	c.mu.Lock()
	c.menuOff = off
	c.mu.Unlock()
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
