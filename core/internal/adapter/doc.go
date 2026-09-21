// Package adapter is the CGO bridge from Core to platform adapters
// (bead cross-os-ab4).
//
// Plan: COMPREHENSIVE_PLAN.md §4 + Phase 0 adapter skeleton item. Boundary:
// Go → C ABI (cgo) → Objective-C++ → Swift/AppKit/CoreGraphics on macOS
// (Swift never called from Go directly); Go → crossos-keyboard-win.dll on
// Windows (WH_KEYBOARD_LL + SendInput helper + UIPI status query).
//
// Authority rule: adapters EXECUTE, Core DECIDES. No business logic lives
// here — this package translates Core's Decision + capability invocations
// into stable C-ABI calls whose implementations evolve behind them in
// platform/darwin and platform/windows.
//
// Build/linking/signing notes (bead criterion 2, dev builds):
//   - macOS: the C sources in platform/darwin/adapter compile to a static
//     archive linked via #cgo CFLAGS/LDFLAGS below; the appex + tap require
//     input-monitoring + accessibility consent at RUNTIME (TCC) — no build
//     flag substitutes. Unsigned dev builds run with the consent prompts;
//     distribution signing is bead cross-os-qhp.1.
//   - Windows: crossos-keyboard-win.dll is loaded at runtime via LoadLibrary
//     (see dll_windows.go); the spike B harness proved the hook shape, the
//     DLL is the product seam for it.
//   - No Swift-direct-from-Go: cgo speaks C only; Swift symbols stay behind
//     the ObjC++ C-ABI surface (TapInstall/TapUninstall/AXQuery/...).
package adapter
