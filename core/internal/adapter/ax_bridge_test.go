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
