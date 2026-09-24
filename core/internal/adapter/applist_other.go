//go:build !darwin

// Installed-app enumeration off macOS (bead w1-app-enumeration).
//
// ListApps has no cheap equivalent here, and that is a capability gap rather
// than a bug to route around. The Windows and Linux counterparts to
// LaunchServices' application database are a registry hive and a .desktop
// database: each needs its own reader, its own parse, and its own answer to
// what a display name is. Approximating one — scanning $PATH, listing
// whatever process happens to be running, or reading what the shell's own
// history suggests — produces rows the user cannot check, and a rule scoped
// to an invented bundle id never fires while looking like it does.
//
// So the list is empty and no error comes back. The picker still works: its
// app field is a typed bundle id, so the rule builder accepts
// "com.googlecode.iterm2" on a machine with nothing behind the list. An empty
// source and a broken one look alike to the caller, which is the honest
// shape here — this function cannot fail, it can only have nothing to report.
package adapter

import "crossos/core/pkg/ctx"

// ListApps returns an empty list. The classifier is accepted and ignored:
// with no rows there is nothing to classify, and taking the argument keeps
// the signature identical to the darwin file so the caller compiles the same
// way on both. See the file comment for why the picker still functions.
func ListApps(_ ctx.Classifier) ([]ctx.ApplicationInfo, error) {
	return []ctx.ApplicationInfo{}, nil
}
