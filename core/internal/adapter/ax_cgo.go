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

// **The C file above is #included, not compiled as a cgo source, and Go's build
// cache does not track it.** The cache key for a cgo package is the content of
// the .go files and the compiler flags; a .c file reached through an #include
// is invisible to it. So editing platform/darwin/adapter/ax_bridge.c and
// rebuilding produces the SAME binary — the change compiles nowhere and the
// daemon keeps the old bridge.
//
// Measured: the SIGBUS fix in cxax_app_titles was edited, rebuilt, and
// tested against a binary that did not contain it, which is why the crash
// reproduced with a byte-identical address three times over.
//
// The build stamps the C sources' content into the compiler flags, so a
// change to any of them changes the action id and the bridge is recompiled.
// See scripts/build-core.sh, which every build goes through.

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

// axAppTitles resolves titles for the given windows of one process. This is
// the semantic plane acquired lazily: the bridge spends at most one batched
// kAXWindows read on the app and answers from the elements that read returned.
//
// `bounds` is each window's frame — x, y, w, h — in the same order as `ids`.
// The frame is how an Accessibility element is matched to the physical row it
// belongs to: the AX plane reports a window's frame in the same coordinates
// CGWindowList reports its bounds, and measured on this machine the two are
// the same four numbers to the point. The private API that used to do this
// matching faulted and took the daemon down with it; see the bridge.
func axAppTitles(pid int, ids []uint32, bounds [][4]float64) (map[uint32]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	wids := make([]C.uint, len(ids))
	frames := make([]C.double, len(ids)*4)
	for i, id := range ids {
		wids[i] = C.uint(id)
		frames[i*4+0] = C.double(bounds[i][0])
		frames[i*4+1] = C.double(bounds[i][1])
		frames[i*4+2] = C.double(bounds[i][2])
		frames[i*4+3] = C.double(bounds[i][3])
	}
	const titleCap = 512
	buf := make([]C.char, len(ids)*titleCap)
	var code C.int
	C.cxax_app_titles(C.int(pid), &wids[0], &frames[0], &buf[0], C.int(titleCap),
		C.int(len(ids)), &code)
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

// axFocusWindow raises the window with the given frame and brings its
// application forward. The frame is the window's identity here, for the
// reason it is in `axAppTitles`.
func axFocusWindow(pid int, frame [4]float64) error {
	var code C.int
	// The RETURN VALUE says what happened; `code` carries the AX error when
	// something failed. Reading `code` for the outcome would report a
	// failure on every success, because a successful raise sets it to zero.
	//
	// The three answers are kept apart because the caller has to act on
	// them differently: 1 is "the window is focused", 0 is "the raise
	// failed, and here is the AX error", and 2 is "the raise worked and the
	// platform did not hand the activation over" — a refusal, which is not
	// the daemon's to fix and is not a failure of the raise.
	raised := C.cxax_focus_window(C.int(pid),
		C.double(frame[0]), C.double(frame[1]), C.double(frame[2]), C.double(frame[3]),
		&code)
	switch raised {
	case 1:
		return nil
	case 2:
		return ErrFocusRefused
	default:
		return axError("focus", int(code))
	}
}

// axFrontWindowID is the window the frontmost application is showing, or 0
// when that is not a fact the physical plane can answer.
func axFrontWindowID() uint32 {
	return uint32(C.cxax_front_wid())
}

// axOnScreenWindowIDs is the WindowServer's own front-to-back order of the
// on-screen normal-layer windows.
//
// It is read on its own, separately from the walk that builds the rows, so
// the order the rows claim can be CHECKED against the order the platform
// reports. A list and the witness to its order that come from the same
// walk cannot disagree, and a walk that is not in front-to-back order would
// never be caught.
func axOnScreenWindowIDs() []uint32 {
	n := int(C.cxax_onscreen_window_ids(nil, 0))
	if n == 0 {
		return nil
	}
	buf := make([]C.uint, n)
	if got := int(C.cxax_onscreen_window_ids(&buf[0], C.int(n))); got < n {
		n = got
	}
	out := make([]uint32, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, uint32(buf[i]))
	}
	return out
}

// axWindowsReadsTotal, axWindowsReadsForPID and axWindowsReadsReset expose the
// bridge's batched-acquisition count, so the window seam is held to one read
// per app rather than claiming it. The reset is what lets a test measure one
// call rather than the process's history.
func axWindowsReadsTotal() int { return int(C.cxax_windows_reads_total()) }

func axWindowsReadsForPID(pid int) int { return int(C.cxax_windows_reads_for_pid(C.int(pid))) }

func axWindowsReadsReset() { C.cxax_windows_reads_reset() }

// axWindowsUnanswered counts the applications whose last Accessibility read
// did not answer, and whose windows are owed another read.
func axWindowsUnanswered() int { return int(C.cxax_windows_unanswered()) }
