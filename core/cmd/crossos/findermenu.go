// Finder menu verbs over IPC (bead be-finderverbs).
//
// The appex builds its menu from finder.menuEntries and fires one of seven
// verbs. Every verb is validation plus a call into an adapter main.go already
// runs for the keyboard rules: the menu is a second door onto the same
// operations, not a second implementation of them. A createFile of our own
// would be a createFile whose O_EXCL guarantee holds on one path and not the
// other, which is the kind of difference nobody finds until it eats a file.
//
// The refusals are the other half. A menu verb is fired by a click on a
// selection, so every path in it is checked before an adapter sees it:
// relative, "..", over the cap, or a rule table's unresolved "{finderDir}" is
// refused with a typed error the appex can show, never acted on.
//
// finderMenuHandlers is deliberately not in methods() yet. Publishing the eight
// methods is two lines in the map be-finderdata owns; until they are there
// these handlers are reachable from a test and from nowhere else, which is the
// dead-code case the note on methods() warns about.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"crossos/core/pkg/filetype"
	"crossos/core/pkg/findermenu"
	"crossos/core/pkg/ipc"
)

// finderMenuHandlers is the appex-facing method set: the menu snapshot and the
// seven verbs it can fire. The names are the contract be-appex-client codes
// against, so they are spelled out here rather than derived from the table's
// ids — the table is a menu, this is a protocol.
func finderMenuHandlers(c *Core) map[string]ipc.Handler {
	return map[string]ipc.Handler{
		"finder.menuEntries":       c.handleFinderMenuEntries,
		"finder.createFolder":      c.handleFinderCreateFolder,
		"finder.createFile":        c.handleFinderCreateFile,
		"finder.copyPath":          c.handleFinderCopyPath,
		"finder.copyRelativePath":  c.handleFinderCopyRelativePath,
		"finder.openTerminal":      c.handleFinderOpenTerminal,
		"finder.openEditor":        c.handleFinderOpenEditor,
		"finder.duplicateWithName": c.handleFinderDuplicateWithName,
	}
}

// finderMenu is what finder.menuEntries returns. The three lists are what a
// menu needs to be built and nothing more: the contexts to slice by, the rows
// to render, the editors and file types the two submenus fan out over.
type finderMenu struct {
	Contexts  []findermenu.Context `json:"contexts"`
	Items     []findermenu.Item    `json:"items"`
	Editors   []findermenu.Editor  `json:"editors"`
	FileTypes []filetype.FileType  `json:"fileTypes"`
}

// handleFinderMenuEntries serves finder.menuEntries.
//
// The file types are the enabled rows of the daemon's catalog, so a settings
// page that turns a type off removes it from the menu without either side
// keeping its own list. A fresh catalog enables txt alone (newfile parity, and
// filetype's own seed test pins it), so until a store is wired behind it the
// New File submenu carries one row — that is the current catalog, not a menu
// that failed to load.
func (c *Core) handleFinderMenuEntries(_ json.RawMessage) (any, *ipc.RPCError) {
	items := make([]findermenu.Item, 0, len(findermenu.Menu))
	items = append(items, findermenu.Menu...)
	fileTypes := make([]filetype.FileType, 0, len(filetype.Seeds()))
	for _, ft := range filetype.Seeds() {
		if ft.Enabled {
			fileTypes = append(fileTypes, ft)
		}
	}
	return finderMenu{
		Contexts:  findermenu.Contexts,
		Items:     items,
		Editors:   findermenu.Editors,
		FileTypes: fileTypes,
	}, nil
}

// handleFinderCreateFolder serves finder.createFolder: {dir}.
func (c *Core) handleFinderCreateFolder(raw json.RawMessage) (any, *ipc.RPCError) {
	dir, rerr := finderLocation(raw, "dir")
	if rerr != nil {
		return nil, rerr
	}
	if err := createFolder(mustParams(map[string]any{"path": dir})); err != nil {
		return nil, &ipc.RPCError{Code: ipc.ErrInternal, Message: err.Error()}
	}
	return map[string]any{"path": dir}, nil
}

// handleFinderCreateFile serves finder.createFile: {dir, ext, baseName,
// template}.
//
// The name is chosen here, not by the appex, because UniqueName has to probe
// the folder: two clicks in the same second must not both answer
// "New Text File.txt". The extension must be a row in the catalog, because the
// catalog is what the submenu offered and a verb that accepts any string would
// create a file the menu never described.
func (c *Core) handleFinderCreateFile(raw json.RawMessage) (any, *ipc.RPCError) {
	dir, rerr := finderLocation(raw, "dir")
	if rerr != nil {
		return nil, rerr
	}
	params, err := dispatchParams(raw)
	if err != nil {
		return nil, &ipc.RPCError{Code: ipc.ErrBadParams, Message: err.Error()}
	}
	ext, rerr := finderFileType(params)
	if rerr != nil {
		return nil, rerr
	}
	baseName := ext.BaseName
	if raw, ok := params["baseName"]; ok {
		if err := json.Unmarshal(raw, &baseName); err != nil {
			return nil, &ipc.RPCError{Code: ipc.ErrBadParams, Message: `core: "baseName" must be a string`}
		}
		if unresolvedTemplate(baseName) {
			return nil, &ipc.RPCError{Code: ipc.ErrInvalid, Message: fmt.Sprintf("core: %q is the rule table's unresolved template, not a name", baseName)}
		}
		// The base name is glued onto a dot and joined into dir, so a
		// separator in it is a way out of dir — and path.Join would clean the
		// result into a folder the menu never named.
		if strings.ContainsAny(baseName, `/\`) || baseName == ".." {
			return nil, &ipc.RPCError{Code: ipc.ErrInvalid, Message: fmt.Sprintf("core: %q is not a name, it is a path", baseName)}
		}
	}
	body := ext.Template
	if raw, ok := params["template"]; ok {
		if err := json.Unmarshal(raw, &body); err != nil {
			return nil, &ipc.RPCError{Code: ipc.ErrBadParams, Message: `core: "template" must be a string`}
		}
		// Same reasoning as pathParam: a body the rule table left open would
		// be written into the file literally, and the user would find
		// "{fileName}" in a file they never typed it into.
		if unresolvedTemplate(body) {
			return nil, &ipc.RPCError{Code: ipc.ErrInvalid, Message: fmt.Sprintf("core: template %s is the rule table's unresolved template", body)}
		}
	}
	name := findermenu.UniqueName(dir, baseName, ext.Ext, pathTaken)
	if err := createFile(mustParams(map[string]any{"path": name, "template": body})); err != nil {
		return nil, &ipc.RPCError{Code: ipc.ErrInternal, Message: err.Error()}
	}
	return map[string]any{"path": name}, nil
}

// handleFinderCopyPath serves finder.copyPath: {paths}.
func (c *Core) handleFinderCopyPath(raw json.RawMessage) (any, *ipc.RPCError) {
	paths, rerr := finderSelection(raw)
	if rerr != nil {
		return nil, rerr
	}
	if err := copyPaths(raw); err != nil {
		return nil, &ipc.RPCError{Code: ipc.ErrInternal, Message: err.Error()}
	}
	return map[string]any{"paths": paths}, nil
}

// handleFinderCopyRelativePath serves finder.copyRelativePath:
// {paths, baseDir}.
//
// The payload is not a path list, so it cannot be copyPaths' as it stands —
// but the strings it does carry are ordinary non-blank text, which is all
// pathList requires, so the relativised lines go back through the same
// adapter, the same platform gate and the same error wrapping rather than
// reaching for pbcopy a second time.
func (c *Core) handleFinderCopyRelativePath(raw json.RawMessage) (any, *ipc.RPCError) {
	paths, rerr := finderSelection(raw)
	if rerr != nil {
		return nil, rerr
	}
	baseDir, rerr := finderLocation(raw, "baseDir")
	if rerr != nil {
		return nil, rerr
	}
	rel := make([]string, 0, len(paths))
	for _, p := range paths {
		rel = append(rel, relativeSlash(baseDir, p))
	}
	if err := copyPaths(mustParams(map[string]any{"paths": rel})); err != nil {
		return nil, &ipc.RPCError{Code: ipc.ErrInternal, Message: err.Error()}
	}
	return map[string]any{"paths": rel}, nil
}

// handleFinderOpenTerminal serves finder.openTerminal: {path}.
func (c *Core) handleFinderOpenTerminal(raw json.RawMessage) (any, *ipc.RPCError) {
	dir, rerr := finderLocation(raw, "path")
	if rerr != nil {
		return nil, rerr
	}
	if err := openTerminalAt(mustParams(map[string]any{"path": dir})); err != nil {
		return nil, &ipc.RPCError{Code: ipc.ErrInternal, Message: err.Error()}
	}
	return map[string]any{"path": dir}, nil
}

// handleFinderOpenEditor serves finder.openEditor: {path, editorID}.
//
// The id is checked against the catalog the submenu was built from rather than
// handed to `open -b`: an id the menu never offered is either a stale appex or
// a request to aim a launch somewhere the user was never shown.
func (c *Core) handleFinderOpenEditor(raw json.RawMessage) (any, *ipc.RPCError) {
	target, rerr := finderLocation(raw, "path")
	if rerr != nil {
		return nil, rerr
	}
	params, err := dispatchParams(raw)
	if err != nil {
		return nil, &ipc.RPCError{Code: ipc.ErrBadParams, Message: err.Error()}
	}
	id, err := stringParam(params, "editorID")
	if err != nil {
		return nil, &ipc.RPCError{Code: ipc.ErrBadParams, Message: err.Error()}
	}
	if _, ok := findermenu.EditorByID(id); !ok {
		return nil, &ipc.RPCError{Code: ipc.ErrInvalid, Message: fmt.Sprintf("core: no editor %q in the catalog", id)}
	}
	if err := darwinOnly("app.open"); err != nil {
		return nil, &ipc.RPCError{Code: ipc.ErrInvalid, Message: err.Error()}
	}
	if err := launch("open", "-b", id, target); err != nil {
		return nil, &ipc.RPCError{Code: ipc.ErrInternal, Message: err.Error()}
	}
	return map[string]any{"path": target, "editorID": id}, nil
}

// handleFinderDuplicateWithName serves finder.duplicateWithName: {paths}.
func (c *Core) handleFinderDuplicateWithName(raw json.RawMessage) (any, *ipc.RPCError) {
	paths, rerr := finderSelection(raw)
	if rerr != nil {
		return nil, rerr
	}
	out := make([]string, 0, len(paths))
	for _, src := range paths {
		dst, err := duplicateBeside(src)
		if err != nil {
			return nil, &ipc.RPCError{Code: ipc.ErrInternal, Message: err.Error()}
		}
		out = append(out, dst)
	}
	return map[string]any{"paths": out}, nil
}

// duplicateBeside copies src to the first free "name 2" beside it and leaves
// the original exactly where it was. A duplicate that moved the original would
// be the one mistake this verb cannot make: the user asked for a second thing.
//
// The copy is staged under a name the menu never shows and renamed into place,
// so an interrupted copy leaves a file the user can delete rather than a
// truncated "report 2.txt" that looks finished.
func duplicateBeside(src string) (string, error) {
	info, err := os.Lstat(src)
	if err != nil {
		return "", fmt.Errorf("core: duplicate %s: %w", src, err)
	}
	name := info.Name()
	ext := strings.TrimPrefix(filepath.Ext(name), ".")
	base := strings.TrimSuffix(name, filepath.Ext(name))
	dst := findermenu.UniqueName(filepath.Dir(src), base, ext, pathTaken)
	// The staging path is cleared before the copy as well as after it: the
	// copy creates its files O_EXCL, so a leftover from a run that was
	// killed mid-copy would otherwise wedge this verb for good.
	staging := dst + ".crossos-partial"
	if err := os.RemoveAll(staging); err != nil {
		return "", fmt.Errorf("core: duplicate %s: %w", src, err)
	}
	defer os.RemoveAll(staging)
	if err := copyTree(src, staging); err != nil {
		return "", err
	}
	// The name is re-probed after the copy. A big folder takes long enough to
	// copy that the user can create the name we picked meanwhile, and
	// os.Rename replaces what is there rather than failing — so taking the
	// next free name costs one probe, and overwriting something the user
	// made while the copy ran costs their file.
	for pathTaken(dst) {
		dst = findermenu.UniqueName(filepath.Dir(src), base, ext, pathTaken)
	}
	if err := os.Rename(staging, dst); err != nil {
		return "", fmt.Errorf("core: duplicate %s: %w", src, err)
	}
	return dst, nil
}

// copyTree copies src to dst: folders with their contents, regular files with
// their mode, symlinks as symlinks. A socket, fifo or device is refused rather
// than approximated — a duplicate that quietly dropped part of a tree is worse
// than one that names the file it could not copy.
func copyTree(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return fmt.Errorf("core: duplicate %s: %w", src, err)
	}
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		target, err := os.Readlink(src)
		if err != nil {
			return fmt.Errorf("core: duplicate %s: %w", src, err)
		}
		if err := os.Symlink(target, dst); err != nil {
			return fmt.Errorf("core: duplicate %s: %w", src, err)
		}
		return nil
	case info.IsDir():
		if err := os.MkdirAll(dst, info.Mode().Perm()); err != nil {
			return fmt.Errorf("core: duplicate %s: %w", src, err)
		}
		entries, err := os.ReadDir(src)
		if err != nil {
			return fmt.Errorf("core: duplicate %s: %w", src, err)
		}
		for _, entry := range entries {
			if err := copyTree(filepath.Join(src, entry.Name()), filepath.Join(dst, entry.Name())); err != nil {
				return err
			}
		}
		return nil
	case info.Mode().IsRegular():
		return copyFile(src, dst, info.Mode().Perm())
	default:
		return fmt.Errorf("core: duplicate %s: %s is neither a file nor a folder", src, info.Name())
	}
}

// copyFile copies one regular file's bytes and mode.
func copyFile(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("core: duplicate %s: %w", src, err)
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return fmt.Errorf("core: duplicate %s: %w", src, err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return fmt.Errorf("core: duplicate %s: %w", src, err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("core: duplicate %s: %w", src, err)
	}
	return nil
}

// pathTaken is the collision probe UniqueName asks: Lstat, not Stat, so a
// broken symlink still counts as a name something is using.
func pathTaken(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

// --- request validation ---

// finderSelection reads the {paths:[…]} a verb was fired with and refuses the
// shapes a menu must never act on. The reader is pathList, the same one the
// capability adapters use, so a request is not refused for spelling a field a
// sibling verb accepted.
//
// The code carries which refusal it is: a missing or malformed parameter is
// the caller's bug (ErrBadParams), while a relative path, a "..", or too many
// paths is a well-formed request the daemon declines (ErrInvalid) and the appex
// shows as an explanation rather than a crash.
func finderSelection(raw json.RawMessage) ([]string, *ipc.RPCError) {
	paths, err := pathList(raw)
	if err != nil {
		return nil, &ipc.RPCError{Code: ipc.ErrBadParams, Message: err.Error()}
	}
	if len(paths) > findermenu.MaxPathCount {
		return nil, &ipc.RPCError{
			Code:    ipc.ErrInvalid,
			Message: fmt.Sprintf("core: %d paths exceed max %d", len(paths), findermenu.MaxPathCount),
		}
	}
	for _, p := range paths {
		if err := absoluteSlash(p); err != nil {
			return nil, &ipc.RPCError{Code: ipc.ErrInvalid, Message: err.Error()}
		}
	}
	return paths, nil
}

// finderLocation reads the one path a single-target verb names ("path",
// "dir", "baseDir").
func finderLocation(raw json.RawMessage, name string) (string, *ipc.RPCError) {
	params, err := dispatchParams(raw)
	if err != nil {
		return "", &ipc.RPCError{Code: ipc.ErrBadParams, Message: err.Error()}
	}
	// pathParam, so a value the rule table left open is refused here for the
	// same reason it is refused on the keyboard path.
	value, err := pathParam(params, name)
	if err != nil {
		return "", &ipc.RPCError{Code: ipc.ErrBadParams, Message: err.Error()}
	}
	if err := absoluteSlash(value); err != nil {
		return "", &ipc.RPCError{Code: ipc.ErrInvalid, Message: err.Error()}
	}
	return value, nil
}

// absoluteSlash refuses a path a Finder menu must never act on: a relative one,
// the root, or one that walks out with a ".." segment.
//
// The traversal check runs on the RAW path, segment by segment. Cleaning first
// would defeat it — path.Clean("/tmp/../etc") is "/etc", with nothing left to
// find — and a substring test would refuse "notes..bak", a filename a user may
// well have. Splitting on os.IsPathSeparator rather than on "/" alone is what
// keeps that check true for a backslash path: on macOS it splits exactly where
// it always did, and on Windows it also catches the "..\" spelling.
//
// Both absolute-path conventions are accepted, and the reason is that neither
// test is right on both platforms. filepath.IsAbs alone is false for "/tmp/x"
// on Windows, so using it alone would refuse every path the daemon is given
// when the tests build there — which is what the old comment was guarding
// against. path.IsAbs alone is false for "C:\...", which is the other half of
// the same problem. So the check is "absolute under either" rather than a coin
// toss between two functions that are each correct at home.
//
// The product domain is unchanged: on macOS neither test accepts a backslash
// path, and Finder hands out slash paths. A "C:\..." string is only ever
// accepted on a Windows build, and only ever arrives here from a test's
// TempDir.
func absoluteSlash(p string) error {
	if p == "" || p == "/" {
		return fmt.Errorf("core: %q is not a location the menu can act on", p)
	}
	for _, segment := range strings.FieldsFunc(p, func(r rune) bool { return os.IsPathSeparator(uint8(r)) }) {
		if segment == ".." {
			return fmt.Errorf("core: %q leaves the folder it is relative to", p)
		}
	}
	clean := path.Clean(p)
	if clean == "." || clean == "/" || (!path.IsAbs(clean) && !filepath.IsAbs(p)) {
		return fmt.Errorf("core: %q is relative, and the menu only acts on absolute paths", p)
	}
	return nil
}

// finderFileType resolves the ext a createFile was asked for against the
// catalog. An ext the submenu never offered is refused rather than created:
// the file would land with a name the user did not pick and cannot connect to
// the menu entry that made it.
func finderFileType(params map[string]json.RawMessage) (filetype.FileType, *ipc.RPCError) {
	ext, err := stringParam(params, "ext")
	if err != nil {
		return filetype.FileType{}, &ipc.RPCError{Code: ipc.ErrBadParams, Message: err.Error()}
	}
	for _, ft := range filetype.Seeds() {
		if ft.Ext == ext {
			if !ft.Enabled {
				return filetype.FileType{}, &ipc.RPCError{
					Code:    ipc.ErrInvalid,
					Message: fmt.Sprintf("core: %q is switched off in the file-type catalog", ext),
				}
			}
			return ft, nil
		}
	}
	return filetype.FileType{}, &ipc.RPCError{
		Code:    ipc.ErrInvalid,
		Message: fmt.Sprintf("core: %q is not a file type in the catalog", ext),
	}
}

// relativeSlash expresses target relative to base, in the slash domain both
// arrive in. A path outside base comes back with the ".." steps a paste into a
// terminal at base needs — that is what "relative" means here, and refusing
// the sibling case would refuse the reason anyone uses this verb.
func relativeSlash(base, target string) string {
	baseParts := slashParts(base)
	targetParts := slashParts(target)
	shared := 0
	for shared < len(baseParts) && shared < len(targetParts) && baseParts[shared] == targetParts[shared] {
		shared++
	}
	parts := make([]string, 0, len(baseParts)-shared+len(targetParts)-shared)
	for range baseParts[shared:] {
		parts = append(parts, "..")
	}
	parts = append(parts, targetParts[shared:]...)
	if len(parts) == 0 {
		return "."
	}
	return strings.Join(parts, "/")
}

// slashParts splits a slash path into its segments. The root is zero segments,
// not one empty one — otherwise "/" came back as a trailing empty name and a
// path relative to "/" grew a phantom step.
func slashParts(p string) []string {
	trimmed := strings.Trim(p, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}

// mustParams renders a map as the raw parameter object the capability adapters
// take. The inputs are Go values this file just built — a path, a template
// string — so a failure here is a bug in the caller, not a request to refuse.
func mustParams(params map[string]any) json.RawMessage {
	raw, err := json.Marshal(params)
	if err != nil {
		panic("core: built menu parameters must marshal: " + err.Error())
	}
	return raw
}
