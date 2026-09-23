//go:build darwin

// CGO surface for the macOS AX bridge (bead cross-os-ax-bridge-window).
//
// Focused() and MoveResize() now reach the real Accessibility API, so the
// window.* capabilities actually execute instead of returning a blanket
// denial. Failure stays typed: kAXErrorAPIDisabled / kAXErrorNotTrusted /
// kAXErrorCannotComplete map to ErrPermissionDenied (the UI guides the user
// to System Settings), any other AX error returns as a distinct error so
// the stage log names the real cause. No key suppression lives here — the
// tap's Dispatch seam owns that (bead cross-os-vx9).
package adapter

/*
#cgo CFLAGS: -x objective-c -I${SRCDIR}/../../../platform/darwin/adapter
#cgo LDFLAGS: -framework AppKit -framework ApplicationServices
#include <stdint.h>
#include "ax_bridge.c"
*/
import "C"

import (
	"fmt"
)

// axPermissionCode is kAXErrorAPIDisabled (accessibility not enabled for
// this process) — the consent case the UI surfaces as "grant access".
const axPermissionCode = -25211

// axNotTrustedCode is kAXErrorNotTrusted.
const axNotTrustedCode = -25243

// axError maps an AXError to the adapter's typed errors.
func axError(op string, code int) error {
	if code == axPermissionCode || code == axNotTrustedCode {
		return fmt.Errorf("%w: %s (AXError %d)", ErrPermissionDenied, op, code)
	}
	return fmt.Errorf("adapter: ax %s failed (AXError %d)", op, code)
}

// axFocusedWindow reads the frontmost app + its focused window.
func axFocusedWindow() (FocusedWindow, error) {
	var w C.cxax_window
	if C.cxax_focused(&w) == 0 {
		return FocusedWindow{}, axError("focused", int(w.err))
	}
	return FocusedWindow{
		BundleID: C.GoString(&w.bundle[0]),
		PID:      int(w.pid),
		Title:    C.GoString(&w.title[0]),
		Role:     C.GoString(&w.role[0]),
		ID:       uint32(w.window_id),
		X:        float64(w.x),
		Y:        float64(w.y),
		W:        float64(w.w),
		H:        float64(w.h),
	}, nil
}

// axMoveResize moves/resizes the frontmost app's focused window.
func axMoveResize(m MoveResize) error {
	var code C.int
	ok := C.cxax_move_resize(
		C.double(m.X), C.double(m.Y), C.double(m.W), C.double(m.H),
		C.int(boolToIntC(m.Move)), C.int(boolToIntC(m.Resize)), &code)
	if ok == 0 {
		return axError("move/resize", int(code))
	}
	return nil
}

func boolToIntC(b bool) int {
	if b {
		return 1
	}
	return 0
}
