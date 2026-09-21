// Package shell is the UI shell scaffold (bead cross-os-80g).
//
// Plan: COMPREHENSIVE_PLAN.md §7 UI Layer + §3.6c UI Contribution API.
// Wails decision: docs/wails-spike.md — v3 beta (multi-window + tray-attached
// windows are structural CrossOS needs), revisit at scaffold: if v3 platform
// APIs are still unstable, start on v2 and port at GA. This scaffold keeps
// the shell thin enough to port: Core logic (contribution host, bridge)
// lives here in pure Go with NO Wails import; the Wails app wiring
// (v2/v3-specific lifecycle, tray, windows) lands in wailsapp.go behind a
// build tag once the toolchain is chosen at scaffold time.
//
// Contribution rule (§7.2): the shell has NO hardcoded page list. Core pages
// (Dashboard/Keyboard/Activity/Safety/Settings) register as UIContributions
// through the SAME Registry plugins use; the host renders by discovery.
// Acceptance test: deleting a plugin from source leaves Core building and
// the shell running (TestNoHardcodedPages + TestPluginRemoval).
package shell
