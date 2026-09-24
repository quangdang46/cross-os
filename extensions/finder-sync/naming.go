// Filename generation for the Finder Sync Platform Extension
// (bead cross-os-vbl.1).
//
// COPY/ADAPT source (§9.10 newfile row): tmp/research/newfile
// (commit b3f665ab839dfe95e6925735ca2a8209d3100fb8):
//
//	Shared/FilenameGenerator.swift → UniqueName below
//
// Adaptations: Go port (no Foundation); collision probe injected for
// testability (same seam as the Swift fileExists closure). The extension
// resolves target dir → unique name → create via the
// filesystem.createFile capability; host-app UI (SwiftUI prefs, template
// editor) is NOT copied — settings are declarative (§3.6c).
//
// The file-type presets that once lived here moved to crossos/core/pkg/filetype
// (same newfile commit, Shared/SeedPresets.swift + Shared/FileTypeEntry.swift).
// The daemon owns the catalog it persists; the extension is a client of it, and
// one struct in the core is the only way the menu and the store can be made to
// describe the same rows.
//
// Merge gate (§9.11): attribution entries in third_party/newfile/.
package findersync

import (
	"fmt"
	"path"
	"strings"
)

// UniqueName returns a non-colliding filename inside dir for baseName+ext.
// Ports FilenameGenerator.uniqueFileURL: strips one redundant ".<ext>"
// suffix (users type "data.json" into the base field), empty base → dotfile,
// then "<base>.<ext>", "<base> 2.<ext>", … Exists is injected (same seam as
// the Swift fileExists closure).
//
// path (slash) semantics, NOT filepath: Finder paths are slash-separated on
// macOS, and the port must behave identically on every build OS. Using
// filepath.Join here miscompiles on Windows: "/d" parses as a volume-less
// rooted path and Join yields "\\d\\Untitled.md". path.Join is correct on
// all platforms for this slash-domain function.
func UniqueName(dir, baseName, ext string, exists func(string) bool) string {
	base := baseName
	suffix := "." + ext
	if len(base) >= len(suffix) && strings.EqualFold(base[len(base)-len(suffix):], suffix) {
		base = base[:len(base)-len(suffix)]
	}
	candidate := func(i int) string {
		if base == "" {
			if i == 1 {
				return path.Join(dir, "."+ext)
			}
			return path.Join(dir, fmt.Sprintf(".%s %d", ext, i))
		}
		if i == 1 {
			return path.Join(dir, base+"."+ext)
		}
		return path.Join(dir, fmt.Sprintf("%s %d.%s", base, i, ext))
	}
	for i := 1; ; i++ {
		if c := candidate(i); !exists(c) {
			return c
		}
	}
}
