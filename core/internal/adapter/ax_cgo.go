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

// axAXTrusted reports the accessibility grant as one boolean, read before any
// call into another app.
func axAXTrusted() bool {
	return C.cxax_ax_trusted() != 0
}

// axListWindows is the batched physical walk. A minimized window is a row, not
// a gap: the full window set is read, not only the on-screen one.
func axListWindows() ([]WindowRow, error) {
	n := int(C.cxax_list_windows(nil, 0))
	if n == 0 {
		return []WindowRow{}, nil
	}
	buf := make([]C.cxax_wrow, n)
	// A window set that shrank between the count and the fill costs the rows
	// past what the fill reported, never a row past the buffer: the count is
	// the larger of the two and a zeroed row carries no window id.
	if got := int(C.cxax_list_windows(&buf[0], C.int(n))); got < n {
		n = got
	}
	out := make([]WindowRow, 0, n)
	for i := 0; i < n; i++ {
		r := &buf[i]
		if r.pid == 0 && r.window_id == 0 {
			continue
		}
		out = append(out, WindowRow{
			ID:        uint32(r.window_id),
			PID:       int(r.pid),
			BundleID:  C.GoString(&r.bundle[0]),
			Title:     C.GoString(&r.title[0]),
			X:         float64(r.x),
			Y:         float64(r.y),
			W:         float64(r.w),
			H:         float64(r.h),
			OnScreen:  r.on_screen != 0,
			Minimized: r.minimized != 0,
		})
	}
	return out, nil
}

// axAppTitles resolves titles for the given window ids of one process. This is
// the semantic plane acquired lazily: the bridge spends at most one batched
// kAXWindows read on the app and answers from the elements that read returned.
func axAppTitles(pid int, ids []uint32) (map[uint32]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	wids := make([]C.uint, len(ids))
	for i, id := range ids {
		wids[i] = C.uint(id)
	}
	const titleCap = 512
	buf := make([]C.char, len(ids)*titleCap)
	var code C.int
	C.cxax_app_titles(C.int(pid), &wids[0], &buf[0], C.int(titleCap), C.int(len(ids)), &code)
	if code != 0 {
		return nil, axError("app titles", int(code))
	}
	out := make(map[uint32]string, len(ids))
	for i, id := range ids {
		if t := C.GoString(&buf[i*titleCap]); t != "" {
			out[id] = t
		}
	}
	return out, nil
}

// axFocusWindow raises the window and brings its application forward.
func axFocusWindow(id uint32, pid int) error {
	var code C.int
	if C.cxax_focus_window(C.uint(id), C.int(pid), &code) == 0 {
		return axError("focus", int(code))
	}
	return nil
}

// axFrontWindowID is the window the frontmost application is showing, or 0
// when that is not a fact the physical plane can answer.
func axFrontWindowID() uint32 {
	return uint32(C.cxax_front_wid())
}

// axWindowsReadsTotal, axWindowsReadsForPID and axWindowsReadsReset expose the
// bridge's batched-acquisition count, so the window seam is held to one read
// per app rather than claiming it. The reset is what lets a test measure one
// call rather than the process's history.
func axWindowsReadsTotal() int { return int(C.cxax_windows_reads_total()) }

func axWindowsReadsForPID(pid int) int { return int(C.cxax_windows_reads_for_pid(C.int(pid))) }

func axWindowsReadsReset() { C.cxax_windows_reads_reset() }
