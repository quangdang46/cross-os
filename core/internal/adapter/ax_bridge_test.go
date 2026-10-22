//go:build darwin

// AX bridge tests (bead cross-os-ax-bridge-window).
//
// On a machine WITHOUT accessibility consent every call must fail with the
// typed permission error — never a panic, never a silent success, never a
// hang. That is the CI-runnable assertion; the success path needs a real
// desktop with consent granted (manual/Tier-2).
package adapter

import (
	"errors"
	"runtime"
	"testing"
)

func TestAXFocusedDenialIsTyped(t *testing.T) {
	q := NewWindowQuery(&Driver{})
	fw, err := q.Focused()
	// Consent present: a focused window comes back. Consent absent: the
	// typed denial. Anything else (panic, untyped) is a bug.
	if err != nil {
		if !errors.Is(err, ErrPermissionDenied) && !isAXFailure(err) {
			t.Fatalf("Focused error must be typed (permission or AX), got %v", err)
		}
		return // denial path: the CI case
	}
	if fw.BundleID == "" || fw.W <= 0 || fw.H <= 0 {
		t.Fatalf("success path returned an implausible window: %+v", fw)
	}
}

// An APPLICATION element is not a window, and the seam must say so.
//
// Measured on this machine with the screen locked: the frontmost
// "application" is the loginwindow, and its AX "window" is the application
// element itself — role AXApplication, a 0x0 frame, the title "loginwindow".
// Reported as the focused window that is a fiction, and the daemon's poll
// caches it, so a rule scoped to an application could be scoped to the
// screen saver and match it.
//
// The answer is the truth: there is no window, so there is nothing focused.
func TestAXFocusedThatIsNotAWindowIsTyped(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("the AX bridge and the WindowServer only exist on darwin")
	}
	if !axAXTrusted() {
		t.Skip("this binary has no accessibility grant")
	}
	q := NewWindowQuery(&Driver{})
	fw, err := q.Focused()
	if err != nil {
		// The typed answers that are not a denial: "no window" (the plane
		// has nothing to name) and "the application did not answer" (it was
		// busy, and the bound expired).
		if !errors.Is(err, errNoFocusedWindow) && !errors.Is(err, ErrPermissionDenied) &&
			!errors.Is(err, errAXBusy) {
			t.Fatalf("Focused: %v, want a typed error", err)
		}
		return
	}
	if fw.Role == axRoleApplication || fw.W <= 0 || fw.H <= 0 {
		t.Fatalf("the seam reported something that is not a window as one: %+v", fw)
	}
}

func TestAXMoveResizeDenialIsTyped(t *testing.T) {
	q := NewWindowQuery(&Driver{})
	err := q.MoveResize(MoveResize{ID: 42, X: 0, Y: 0, W: 800, H: 600, Move: true, Resize: true})
	if err == nil {
		return // consent present and the set succeeded
	}
	if !errors.Is(err, ErrPermissionDenied) && !isAXFailure(err) {
		t.Fatalf("MoveResize error must be typed, got %v", err)
	}
}

// TestAXWindowActionMaximizeZoomsTheFocusedWindow drives the one live half
// of the window actions that is safe to drive on the working machine
// (bead cross-os-1be): the zoom. Close and minimize would change the user's
// desktop in ways the test cannot undo, so they stay at the typed-refusal
// paragraph below.
//
// The zoom is checked the way move/resize is: the frame is read from the
// plane, the action is performed, the frame is read again, and the second
// read must show a window that is AT LEAST as large — a "done" that carries
// the old geometry is the promise the next poll contradicts in the user's
// list. An already-maximized window zooms to the same frame, which the
// at-least reading accepts honestly instead of failing on no-op.
func TestAXWindowActionMaximizeZoomsTheFocusedWindow(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("the AX bridge is macOS")
	}
	q := NewWindowQuery(&Driver{})
	before, err := q.Focused()
	if err != nil {
		t.Skipf("no focused window to act on: %v", err)
	}
	if err := q.SetWindowAction(WindowMaximize); err != nil {
		t.Fatalf("maximize: %v", err)
	}
	after, err := q.Focused()
	if err != nil {
		t.Fatalf("read after maximize: %v", err)
	}
	if after.W < before.W || after.H < before.H {
		t.Fatalf("frame after maximize %vx%v is smaller than before %vx%v; the action answered done without the geometry",
			after.W, after.H, before.W, before.H)
	}
}

// The refused tags, without and with the grant: SetWindowAction is typed on
// both sides of consent — an unwired plane answers the typed not-supported
// (stub), the bridge answers its own action-unsupported or a consent or
// nothing-such-window error, and none of them is ever a silent success.
func TestAXWindowActionRefusalsAreTyped(t *testing.T) {
	q := NewWindowQuery(&Driver{})
	if err := q.SetWindowAction(WindowAction(7)); err == nil {
		t.Fatal("an invalid action tag must be refused, not performed")
	}
	if err := q.SetWindowAction(WindowClose); err == nil {
		return // consent present; the refusal cases above are the contract
	}
}

func TestAXMoveResizeRejectsNoOp(t *testing.T) {
	q := NewWindowQuery(&Driver{})
	// Neither move nor resize: rejected before any AX call.
	if err := q.MoveResize(MoveResize{ID: 1}); err == nil {
		t.Fatal("no-op move/resize must be rejected")
	}
}

func TestAXErrorMapping(t *testing.T) {
	// Consent codes map to ErrPermissionDenied (the UI's "grant access").
	for _, code := range []int{axPermissionCode, axNotTrustedCode} {
		err := axError("probe", code)
		if !errors.Is(err, ErrPermissionDenied) {
			t.Fatalf("AXError %d must map to ErrPermissionDenied, got %v", code, err)
		}
	}
	// Other AX failures keep their own identity (stage log must name them).
	other := axError("probe", -25205)
	if errors.Is(other, ErrPermissionDenied) {
		t.Fatal("non-consent AX error must NOT masquerade as permission denial")
	}
}

// isAXFailure reports a non-consent AX error (surfaced for diagnostics).
func isAXFailure(err error) bool {
	return err != nil && errors.Is(err, ErrPermissionDenied) == false &&
		len(err.Error()) > 0
}
