// Finder context menus: §6.3 menu table as declarative menus whose actions
// execute as NATIVE capabilities (bead cross-os-vbl.2).
//
// Plan: COMPREHENSIVE_PLAN.md §6.3 (12 menu items × file/folder/empty
// contexts), §3.6 RegisterMenu (normative path), §3.12 registry,
// §9.10 files row (BEHAVIOR only — ideas, not files; MPL file-level
// copyleft on their side, so nothing copied).
//
// Authority: menus DECLARE; the ActionDispatcher validates requests
// against local config (path checks, max path count, §6.3 Security).
// Shell/process actions are opt-in Level B only, never the default.
// Every execution appends Recorder-bound stage traces (feeds vbl.5).
// TODO(vbl.3): wire MenuTable through pluginapi.RegisterMenu when the
// pack-loader lands (packs supply these menus; Dispatch standalone is the
// interim normative path, not the finished wiring).
package findersync

import (
	"fmt"
	"path/filepath"
	"strings"
)

// SelectionCtx is the Finder selection context a menu item appears in.
type SelectionCtx int

const (
	// CtxFile: files selected (copy/cut/paste/rename/info/trash/compress...).
	CtxFile SelectionCtx = iota
	// CtxFolder: folders selected.
	CtxFolder
	// CtxEmpty: background / empty-space right-click (New > ...).
	CtxEmpty
)

// MaxPathCount caps multi-selection dispatch (§6.3 Security).
const MaxPathCount = 100

// MenuItem is one §6.3 row: declarative, native capability only.
type MenuItem struct {
	ID         string // stable id, e.g. "newText"
	Title      string // e.g. "New > Text Document"
	Capability string // native capability, e.g. filesystem.createFile
	Contexts   []SelectionCtx
	// NeedsPaths: item requires ≥1 selected path (hidden on CtxEmpty).
	NeedsPaths bool
}

// MenuTable is the §6.3 context-menu table (12 items).
var MenuTable = []MenuItem{
	{ID: "newText", Title: "New > Text Document", Capability: "filesystem.createFile", Contexts: []SelectionCtx{CtxEmpty, CtxFolder}},
	{ID: "newFolder", Title: "New > Folder", Capability: "filesystem.createFolder", Contexts: []SelectionCtx{CtxEmpty, CtxFolder}},
	{ID: "copyPath", Title: "Copy as Path", Capability: "clipboard.copyPath", Contexts: []SelectionCtx{CtxFile, CtxFolder}, NeedsPaths: true},
	{ID: "openTerminal", Title: "Open in Terminal", Capability: "terminal.openAt", Contexts: []SelectionCtx{CtxFile, CtxFolder, CtxEmpty}},
	{ID: "openEditor", Title: "Open in Editor", Capability: "app.open", Contexts: []SelectionCtx{CtxFile, CtxFolder}},
	{ID: "cut", Title: "Cut", Capability: "clipboard.copy", Contexts: []SelectionCtx{CtxFile, CtxFolder}, NeedsPaths: true},
	{ID: "copy", Title: "Copy", Capability: "clipboard.copy", Contexts: []SelectionCtx{CtxFile, CtxFolder}, NeedsPaths: true},
	{ID: "paste", Title: "Paste", Capability: "clipboard.write", Contexts: []SelectionCtx{CtxFile, CtxFolder, CtxEmpty}},
	{ID: "rename", Title: "Rename (F2)", Capability: "app.open", Contexts: []SelectionCtx{CtxFile, CtxFolder}, NeedsPaths: true},
	{ID: "getInfo", Title: "Get Info", Capability: "app.open", Contexts: []SelectionCtx{CtxFile, CtxFolder}, NeedsPaths: true},
	{ID: "trash", Title: "Move to Trash", Capability: "file.moveToTrash", Contexts: []SelectionCtx{CtxFile, CtxFolder}, NeedsPaths: true},
	{ID: "compress", Title: "Compress", Capability: "filesystem.createFile", Contexts: []SelectionCtx{CtxFile, CtxFolder}, NeedsPaths: true},
}

// ForContext returns the items visible in ctx (empty-selection items never
// require paths; path items hide on CtxEmpty).
func ForContext(ctx SelectionCtx) []MenuItem {
	var out []MenuItem
	for _, m := range MenuTable {
		for _, c := range m.Contexts {
			if c == ctx {
				out = append(out, m)
				break
			}
		}
	}
	return out
}

// Dispatch validates a menu execution (§6.3 Security) and returns the
// capability invocation + trace stages. Validation FIRST (paths, count,
// context fit); the trace records event → action → result for vbl.5.
// Shell-bearing actions are rejected here (Level B gate) — native only.
func Dispatch(itemID string, ctx SelectionCtx, paths []string) (capability string, params map[string]string, stages []string, err error) {
	stages = append(stages, "event: menu="+itemID)
	var item *MenuItem
	for i := range MenuTable {
		if MenuTable[i].ID == itemID {
			item = &MenuTable[i]
		}
	}
	if item == nil {
		return "", nil, stages, fmt.Errorf("menu %q: unknown item", itemID)
	}
	allowed := false
	for _, c := range item.Contexts {
		if c == ctx {
			allowed = true
		}
	}
	if !allowed {
		return "", nil, stages, fmt.Errorf("menu %q: not available in this context", itemID)
	}
	if len(paths) > MaxPathCount {
		return "", nil, stages, fmt.Errorf("menu %q: %d paths exceed max %d", itemID, len(paths), MaxPathCount)
	}
	if item.NeedsPaths && len(paths) == 0 {
		return "", nil, stages, fmt.Errorf("menu %q: needs a selection", itemID)
	}
	for _, p := range paths {
		// Traversal check on the RAW path first: Clean("/tmp/../etc") ==
		// "/etc" (no ".." left), so cleaning before checking would let
		// escapes through. Reject ".." segments pre-clean, then clean.
		if strings.Contains(p, "..") {
			return "", nil, stages, fmt.Errorf("menu %q: rejected path %q", itemID, p)
		}
		clean := filepath.Clean(p)
		if clean == "." || clean == "/" || strings.Contains(clean, "..") {
			return "", nil, stages, fmt.Errorf("menu %q: rejected path %q", itemID, p)
		}
		if !filepath.IsAbs(clean) {
			return "", nil, stages, fmt.Errorf("menu %q: relative path %q rejected", itemID, p)
		}
	}
	stages = append(stages, "action: "+item.Capability+" paths="+fmt.Sprint(len(paths)))
	params = map[string]string{"paths": strings.Join(paths, "\n")}
	stages = append(stages, "result: dispatched (adapter executes)")
	return item.Capability, params, stages, nil
}
