// AX errors + move/resize geometry (bead criterion 2).
//
// Rectangle reference (AXExtension.swift): AX reads return nil on error —
// callers must treat "no value" as a typed failure, never spin waiting.
// Permission-DENIED (accessibility consent not granted) is the most common
// one and must return fast with a typed error so the jiz cache can serve
// stale-or-empty instead of hanging the fast path.
package spikec

import (
	"errors"
	"fmt"
)

// AXError is a typed Accessibility-API failure.
type AXError struct {
	Op   string // e.g. "copyAttribute:AXTitle", "setAttribute:AXPosition"
	Code int    // AXError code (e.g. kAXErrorAPIDisabled = -25211)
}

func (e *AXError) Error() string {
	return fmt.Sprintf("ax %s: code %d", e.Op, e.Code)
}

// kAXErrorAPIDisabled: accessibility consent not granted (or API disabled).
const axAPIDisabled = -25211

// IsPermissionDenied reports whether err is the accessibility-consent denial
// the jiz cache design must handle (serve stale/empty, never block).
func IsPermissionDenied(err error) bool {
	var ax *AXError
	return errors.As(err, &ax) && ax.Code == axAPIDisabled
}

// MoveResize is a validated move/resize request. Validation is pure so the
// permission-granted path (assert new frame) and the denied path (typed
// error, no hang) are both Tier-1 testable; the live AX set calls sit behind
// the darwin seam and return *AXError on failure.
type MoveResize struct {
	ID    uint32 // target window
	To    Frame  // desired frame
	Move  bool   // set kAXPositionAttribute
	Sizer bool   // set kAXSizeAttribute
}

// Validate rejects degenerate requests before any AX call: zero target,
// non-positive size, or a no-op (neither move nor resize set).
func (m MoveResize) Validate() error {
	if m.ID == 0 {
		return fmt.Errorf("move/resize: zero window id")
	}
	if !m.Move && !m.Sizer {
		return fmt.Errorf("move/resize: neither move nor resize requested")
	}
	if m.Sizer && (m.To.W <= 0 || m.To.H <= 0) {
		return fmt.Errorf("move/resize: non-positive size %+v", m.To)
	}
	return nil
}

// ClampToScreen pins the desired frame inside the screen frame, mirroring the
// nudge geometry port (cross-os-nir.1): windows never end up fully off-screen
// from a programmatic move.
func (m MoveResize) ClampToScreen(screen Frame) MoveResize {
	if m.To.W > screen.W {
		m.To.W = screen.W
	}
	if m.To.H > screen.H {
		m.To.H = screen.H
	}
	if m.To.X < screen.X {
		m.To.X = screen.X
	}
	if m.To.Y < screen.Y {
		m.To.Y = screen.Y
	}
	if m.To.X+m.To.W > screen.X+screen.W {
		m.To.X = screen.X + screen.W - m.To.W
	}
	if m.To.Y+m.To.H > screen.Y+screen.H {
		m.To.Y = screen.Y + screen.H - m.To.H
	}
	return m
}
