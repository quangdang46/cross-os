// Filename generation + file-type presets for the Finder Sync Platform
// Extension (bead cross-os-vbl.1).
//
// COPY/ADAPT source (§9.10 newfile row): tmp/research/newfile
// (commit b3f665ab839dfe95e6925735ca2a8209d3100fb8):
//
//	Shared/FilenameGenerator.swift → UniqueName below
//	Shared/SeedPresets.swift + Shared/FileTypeEntry.swift → FileType + Seeds
//
// Adaptations: Go port (no Foundation); collision probe injected for
// testability (same seam as the Swift fileExists closure); menuTitle
// fallback (derived "New .<ext>" label when displayName is blank) ported.
// The extension resolves target dir → unique name → create via the
// filesystem.createFile capability; host-app UI (SwiftUI prefs, template
// editor) is NOT copied — settings are declarative (§3.6c).
//
// Merge gate (§9.11): attribution entries in third_party/newfile/.
package findersync

import (
	"fmt"
	"path/filepath"
	"strings"
)

// FileType is one creatable file type (adapted from FileTypeEntry).
type FileType struct {
	Ext         string
	BaseName    string
	DisplayName string
	Template    string
	Enabled     bool
	BuiltIn     bool
}

// MenuTitle is the Finder menu label: explicit displayName, else the derived
// "New .<ext>" label (ports FileTypeEntry.menuTitle/derivedDisplayName).
func (f FileType) MenuTitle() string {
	if t := strings.TrimSpace(f.DisplayName); t != "" {
		return t
	}
	return "New ." + strings.TrimSpace(f.Ext)
}

// Seeds are the built-in presets (adapted from SeedPresets.builtIns).
var Seeds = []FileType{
	{Ext: "txt", BaseName: "New Text File", DisplayName: "New Text File", Enabled: true, BuiltIn: true},
	{Ext: "md", BaseName: "Untitled", DisplayName: "New Markdown", BuiltIn: true},
	{Ext: "env", BaseName: "", DisplayName: "New .env", BuiltIn: true},
	{Ext: "json", BaseName: "data", DisplayName: "New JSON", BuiltIn: true},
	{Ext: "yml", BaseName: "config", DisplayName: "New YAML", BuiltIn: true},
	{Ext: "sh", BaseName: "script", DisplayName: "New Shell Script", BuiltIn: true},
	{Ext: "gitignore", BaseName: "", DisplayName: "New .gitignore", BuiltIn: true},
	{Ext: "html", BaseName: "index", DisplayName: "New HTML", BuiltIn: true},
}

// UniqueName returns a non-colliding filename inside dir for baseName+ext.
// Ports FilenameGenerator.uniqueFileURL: strips one redundant ".<ext>"
// suffix (users type "data.json" into the base field), empty base → dotfile,
// then "<base>.<ext>", "<base> 2.<ext>", … Exists is injected (same seam as
// the Swift fileExists closure); filepath.Join replaces URL appending.
func UniqueName(dir, baseName, ext string, exists func(string) bool) string {
	base := baseName
	suffix := "." + ext
	if len(base) >= len(suffix) && strings.EqualFold(base[len(base)-len(suffix):], suffix) {
		base = base[:len(base)-len(suffix)]
	}
	candidate := func(i int) string {
		if base == "" {
			if i == 1 {
				return filepath.Join(dir, "."+ext)
			}
			return filepath.Join(dir, fmt.Sprintf(".%s %d", ext, i))
		}
		if i == 1 {
			return filepath.Join(dir, base+"."+ext)
		}
		return filepath.Join(dir, fmt.Sprintf("%s %d.%s", base, i, ext))
	}
	for i := 1; ; i++ {
		if c := candidate(i); !exists(c) {
			return c
		}
	}
}
