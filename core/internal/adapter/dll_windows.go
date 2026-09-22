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

import (
	"errors"
	"syscall"
)

// errDLLMissing marks the runtime-load failure mode distinctly from the
// not-yet-linked seam: same handling (typed error to caller + stage log),
// clearer cause for the operator.
var errDLLMissing = errors.New("adapter: crossos-keyboard-win.dll not found (build via platform/windows CMake)")

// dllName is the thin helper built by platform/windows (CMakeLists.txt +
// crossos-keyboard-win.c, MSVC-verified in bead cross-os-qhp.8). Searched
// on the standard DLL path (app dir first); signing the DLL stays
// cert-gated in qhp.6 — loading it does not need a cert.
const dllName = "crossos-keyboard-win.dll"

// dllHandle is the loaded DLL; nil until loadDLL succeeds.
var dllHandle syscall.Handle

// dllLoad is the LoadLibrary seam, injected for tests (default:
// syscall.LoadDLL, which resolves on the standard DLL search path).
var dllLoad = syscall.LoadDLL

// loadDLL loads the thin helper at runtime. A missing DLL stays a typed
// error through the driver's dual-surface logging (caller + stage log) —
// never a silent success, never a link-time surprise (bead cross-os-qhp.8).
func loadDLL(d *Driver) error {
	if dllHandle != 0 {
		return nil
	}
	h, err := dllLoad(dllName)
	if err != nil {
		return d.ReportError("dll", errDLLMissing)
	}
	dllHandle = syscall.Handle(h.Handle)
	return nil
}
