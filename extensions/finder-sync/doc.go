// Spike D — Finder Sync platform extension harness (bead cross-os-3c6).
//
// Plan: COMPREHENSIVE_PLAN.md Phase 0 Spike D, §6.3, §3.7 Permission Manager,
// §9.11 newfile COPY/ADAPT source.
// Reference: tmp/research/newfile/Extension/FinderSync.swift — FIFinderSync
// subclass: directoryURLs rooted at "/", menu(for:) builds NSMenu from a
// tag-indexed snapshot (representedObject can't cross the XPC bridge),
// actions resolve target dir → create → reveal; daemon contact is OUTSIDE
// the appex (adapted here: the extension is a minimal IPC client).
//
// Security model (§3.7): the extension sends REQUESTS only, never executable
// code. Daemon-down: hide/fail gracefully, Finder never blocks.
//
// Layout mirrors spikes A/B/C: pure menu + IPC-client + daemon-down logic in
// testable Go; the Swift appex shape is documented in appex.swift.txt and
// wired by bead cross-os-vbl.1. Tier-1 unit tests always run; Tier-2 (real
// Finder menu + stub daemon) is gated for an interactive desktop.
package findersync
