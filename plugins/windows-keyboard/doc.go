// Package windowskeyboard is the CrossOS builtin Windows Keyboard plugin —
// the whole of MVP 0.
//
// Bead: cross-os-s4i. Plan: COMPREHENSIVE_PLAN.md §6.1, Phase 1 MVP 0,
// §9.11 (behavior data rows).
//
// Intent-first matrix (~10 shortcuts, first slice of §6.1): the physical
// Windows shortcut resolves to an Intent, never a raw key-to-key mapping —
// Alt+F4 → CLOSE_WINDOW (never a hardcoded Cmd+Q), Ctrl+C in Terminal →
// INTERRUPT (never blanket COPY). The Adapter resolves the native action
// per app/platform from the Intent.
//
// Behavior data provenance (§9.11 DATA ONLY): the shortcut table below is
// re-expressed CrossOS Rules → Intents, sourced as data from
// windows-keyboard-for-mac (MIT, docs/shortcut-matrix.md) and
// karabiner-mac-to-windows / karabiner-windows-mode (MIT/Unlicense). No
// Karabiner config, JSON profile, or source file is copied or executed —
// only the physical-shortcut→behavior mapping is read and manually
// re-encoded as Go rule literals.
package windowskeyboard
