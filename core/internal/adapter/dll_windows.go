//go:build windows

// DLL loader seam for crossos-keyboard-win.dll (thin native helper: hook +
// Raw Input observe + SendInput ONLY; all decision logic stays in Go per
// §12). Loaded at runtime via LoadLibrary so a missing DLL is a typed error
// at Install time, never a link-time surprise. Until the CMake-built DLL
// ships, the loader reports errTapNotWired (same never-silent-succes rule
// as the tap seams).
//
// DLL surface (frozen — CMake builds exactly this, nothing more):
//
//	CrossOS_HookInstall() -> hook handle (WH_KEYBOARD_LL, thin callback
//	  forwards to Go via the C-ABI decide entry; returns non-zero = suppress)
//	CrossOS_HookUninstall(handle)
//	CrossOS_SendInput(vk, flags) -> accepted count (UIPI-blocked injections
//	  report 0 + last-error; caller owns passthrough+notice fallback)
//	CrossOS_UIPIStatus() -> elevation/reachability bits
//	CrossOS_DeviceId() -> Raw Input device identity (observe-only path)
package adapter

import "errors"

// errDLLMissing marks the runtime-load failure mode distinctly from the
// not-yet-linked seam: same handling (typed error to caller + stage log),
// clearer cause for the operator.
var errDLLMissing = errors.New("adapter: crossos-keyboard-win.dll not found (build via platform/windows CMake)")

// dllHandle is the loaded DLL; nil until loadDLL succeeds.
var dllHandle uintptr

// loadDLL is the LoadLibrary seam. Currently unlinked (CMake step pending),
// so it reports the missing-DLL error through the driver's dual-surface
// logging — the exact shape the real loader keeps.
func loadDLL(d *Driver) error {
	return d.ReportError("dll", errDLLMissing)
}
