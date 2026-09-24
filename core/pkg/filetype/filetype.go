// Package filetype is the daemon's file-type catalog: what "New >" can
// create, and the rules that turn a preset into a filename.
//
// COPY/ADAPT source (§9.10 newfile row): tmp/research/newfile
// (commit b3f665ab839dfe95e6925735ca2a8209d3100fb8):
//
//	Shared/SeedPresets.swift + Shared/FileTypeEntry.swift → FileType + Seeds
//
// The catalog lives here rather than in the Finder extension: the extension is
// a minimal IPC client that cannot own a list the daemon has to persist, and
// two FileType structs in two modules is how the daemon and the menu start
// disagreeing about what "New >" offers. The eight presets are ported verbatim
// (Swift's isBuiltIn becomes BuiltIn); UUID/Identifiable/SwiftUI identity is
// dropped because the daemon owns identity — the extension is named by
// (ext, baseName), not by an id it would have to mint.
//
// §9.11 merge gate: the attribution block for this port is in
// third_party/newfile/ATTRIBUTION.md, and it must name this file as the
// destination once the move lands.
package filetype

import (
	"fmt"
	"strings"
)

// FileType is one creatable file type (adapted from FileTypeEntry). The json
// tags are the settings-document names, so the persisted row and the wire row
// are one shape and neither can drift from the other.
type FileType struct {
	Ext         string `json:"ext"`
	BaseName    string `json:"baseName"`
	DisplayName string `json:"displayName"`
	Template    string `json:"template"`
	Enabled     bool   `json:"enabled"`
	BuiltIn     bool   `json:"builtIn"`
}

// MenuTitle is the Finder menu label: explicit displayName, else the derived
// "New .<ext>" label (ports FileTypeEntry.menuTitle/derivedDisplayName).
func (f FileType) MenuTitle() string {
	if t := strings.TrimSpace(f.DisplayName); t != "" {
		return t
	}
	return "New ." + strings.TrimSpace(f.Ext)
}

// Seeds returns the eight built-in presets (adapted from
// SeedPresets.builtIns). A fresh slice per call: a caller that edits the
// catalog it was handed must not be editing the table every later reader sees,
// and the store hands this list straight to a page.
func Seeds() []FileType {
	return []FileType{
		{Ext: "txt", BaseName: "New Text File", DisplayName: "New Text File", Enabled: true, BuiltIn: true},
		{Ext: "md", BaseName: "Untitled", DisplayName: "New Markdown", BuiltIn: true},
		{Ext: "env", BaseName: "", DisplayName: "New .env", BuiltIn: true},
		{Ext: "json", BaseName: "data", DisplayName: "New JSON", BuiltIn: true},
		{Ext: "yml", BaseName: "config", DisplayName: "New YAML", BuiltIn: true},
		{Ext: "sh", BaseName: "script", DisplayName: "New Shell Script", BuiltIn: true},
		{Ext: "gitignore", BaseName: "", DisplayName: "New .gitignore", BuiltIn: true},
		{Ext: "html", BaseName: "index", DisplayName: "New HTML", BuiltIn: true},
	}
}

// Validate rejects a catalog no menu could act on. The extension carries the
// whole row: MenuTitle derives a label from it and the filename rules glue it
// onto a base name, so a blank ext yields "foo." and a dotted one yields
// "foo..txt". A separator is refused harder than it looks — the base name is
// glued to a dot, but path.Join still cleans the result, so ext="/../../x"
// walks the created file out of the directory it was to land in.
//
// Everything else is the user's to decide, including a repeated extension:
// "data.json" and "report.json" are two presets, not a duplicate.
func Validate(set []FileType) error {
	for _, f := range set {
		ext := strings.TrimSpace(f.Ext)
		switch {
		case ext == "":
			return fmt.Errorf("filetype: %q has no extension", f.MenuTitle())
		case strings.HasPrefix(ext, "."):
			return fmt.Errorf("filetype: extension %q is dotted; the filename rules add the dot", f.Ext)
		case strings.ContainsAny(ext, `/\`):
			return fmt.Errorf("filetype: extension %q contains a path separator", f.Ext)
		}
	}
	return nil
}
