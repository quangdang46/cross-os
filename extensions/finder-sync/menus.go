// Finder context menus: the seven verbs the daemon owns, and the validation
// + trace this package adds before a click leaves the appex (bead
// cross-os-vbl.2).
//
// The table itself is NOT declared here any more. It lives in
// crossos/core/pkg/findermenu, which the daemon serves from
// finder.menuEntries, so the menu Finder renders and the menu the daemon
// executes cannot be two lists that drift. What stays here is the part the
// extension alone owns: rejecting a bad path, and recording the stage trace.
//
// findermenu's own header argues the extension must keep a private copy
// because the dependency does not run the other way. That is backwards for
// this edge: crossos/finder_sync already requires crossos/core, and core
// requires nothing from the extension, so importing the table the daemon owns
// is the one direction that works. The copy is what let the appex and the
// daemon disagree about which rows exist.
//
// Authority: menus DECLARE; the ActionDispatcher validates requests
// against local config (path checks, max path count, §6.3 Security).
// Every execution appends Recorder-bound stage traces (feeds vbl.5).
package findersync

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"crossos/core/pkg/findermenu"
)

// SelectionCtx is a findermenu context. Aliased rather than re-declared: an
// int enum declared on both sides carried each package's own ordering, and
// the appex then had to guess which ordering the wire strings came from. The
// core type is string-valued, so the appex sends the same names it declares.
type SelectionCtx = findermenu.Context

const (
	// CtxFile: files selected (copy/duplicate/open…).
	CtxFile = findermenu.ContextFile
	// CtxFolder: folders selected.
	CtxFolder = findermenu.ContextFolder
	// CtxEmpty: background / empty-space right-click (New > ...).
	CtxEmpty = findermenu.ContextEmpty
)

// MaxPathCount caps a multi-selection dispatch (§6.3 Security). The number
// belongs to the daemon's table because a selection the keyboard rules
// refuse must be one the menu refuses too.
const MaxPathCount = findermenu.MaxPathCount

// MenuItem is one declarative row: native capability only.
type MenuItem = findermenu.Item

// MenuTable is the seven rows the daemon owns and serves. It is a reference,
// not a copy — the daemon serialises these exact values.
var MenuTable = findermenu.Menu

// MethodMenuEntries is the menu query the appex sends at menu-open.
const MethodMenuEntries = "finder.menuEntries"

// daemonVerbs maps each row id to the method that fires it.
//
// This is a table, not "finder." + id, and the two differ for both New rows:
// the row is newFile, the call is finder.createFile. The daemon spells its
// method names out in finderMenuHandlers for the same reason — the table is
// a menu, the method names are a protocol, and a row id is neither. Deriving
// the name yields methods the daemon answers "no such method", which surfaces
// as a menu item that does nothing on click.
var daemonVerbs = map[string]string{
	"newFile":           "finder.createFile",
	"newFolder":         "finder.createFolder",
	"copyPath":          "finder.copyPath",
	"copyRelativePath":  "finder.copyRelativePath",
	"openTerminal":      "finder.openTerminal",
	"openEditor":        "finder.openEditor",
	"duplicateWithName": "finder.duplicateWithName",
}

// Verb is the daemon method a row's action is sent as, or "" for a row the
// daemon has no verb for. An empty Verb is a menu row with nothing behind it,
// so TestActionVerbsAreDaemonMethods fails rather than letting it ship.
func Verb(id string) string { return daemonVerbs[id] }

// DaemonMethods is every method the appex may call — the menu query plus one
// action verb per row. ipc.Send validates against this rather than a private
// verb list, so the closed set the extension refuses to invent IS the daemon's
// own method list: a method the daemon drops becomes uncallable here without
// anyone editing a second list, and a name the daemon never had is refused
// before the socket is dialled.
var DaemonMethods = func() map[string]bool {
	out := make(map[string]bool, len(daemonVerbs)+1)
	out[MethodMenuEntries] = true
	for _, verb := range daemonVerbs {
		out[verb] = true
	}
	return out
}()

// ForContext returns the items visible in ctx. A fresh slice — the caller
// renders it and the table is shared by every request.
func ForContext(ctx SelectionCtx) []MenuItem { return findermenu.ForContext(ctx) }

// Dispatch validates a menu execution (§6.3 Security) and returns the
// capability invocation + trace stages. Validation FIRST (paths, count,
// context fit); the trace records event → action → result for vbl.5.
// Native-only gate: the rows carry capability IDs only (no shell strings
// exist at dispatch time), so shell-by-default is impossible by
// construction; TestMenuTableCoverage pins the table side. Dispatch's job
// is validation + tracing, not gate-keeping a parameter it never takes.
// (review: cross-os-ed — the old comment overclaimed the gate lives here.)
func Dispatch(itemID string, ctx SelectionCtx, paths []string) (capability string, params map[string]string, stages []string, err error) {
	stages = append(stages, "event: menu="+itemID)
	item, ok := findermenu.Lookup(itemID)
	if !ok {
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
		//
		// Slash-domain paths (Finder paths are slash-separated on macOS):
		// use path (not filepath) so validation behaves identically on
		// every build OS. filepath.IsAbs("/tmp/x") is FALSE on Windows
		// ("/tmp" parses as volume-less rooted), which wrongly rejects
		// every absolute Finder path when tests build on Windows.
		if strings.Contains(p, "..") {
			return "", nil, stages, fmt.Errorf("menu %q: rejected path %q", itemID, p)
		}
		clean := path.Clean(p)
		if clean == "." || clean == "/" || strings.Contains(clean, "..") {
			return "", nil, stages, fmt.Errorf("menu %q: rejected path %q", itemID, p)
		}
		if !path.IsAbs(clean) {
			return "", nil, stages, fmt.Errorf("menu %q: relative path %q rejected", itemID, p)
		}
		// Belt-and-suspenders: filepath is still consulted so OS-native
		// separators in an already-validated slash path cannot smuggle an
		// escape on the executing platform.
		if filepath.Separator != '/' && strings.ContainsRune(p, filepath.Separator) {
			return "", nil, stages, fmt.Errorf("menu %q: native separator in %q rejected", itemID, p)
		}
	}
	stages = append(stages, "action: "+item.Capability+" paths="+fmt.Sprint(len(paths)))
	params = map[string]string{"paths": strings.Join(paths, "\n")}
	stages = append(stages, "result: dispatched (adapter executes)")
	return item.Capability, params, stages, nil
}
