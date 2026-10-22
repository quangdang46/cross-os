//go:build !windows && !darwin

// Portable stub: no native tap/hook on this platform. Every seam returns a
// typed "not supported" error (never nil success, never silent) so tests and
// Core's init path observe the gap explicitly. darwin/windows files override
// per-platform.
package adapter

import (
	"crossos/core/pkg/event"
)

type stubTap struct{ driver *Driver }

func NewKeyboardTap(d *Driver) KeyboardTap { return &stubTap{driver: d} }

func (s *stubTap) Install() error { return s.driver.ReportError("tap", errUnsupported) }

// Uninstall is clean-nil even with nothing installed (matches
// darwin/windows): uninstalling a non-installed tap is a no-op, not an error.
func (s *stubTap) Uninstall() error { return nil }
func (s *stubTap) DecideOne(ev event.Event, ctx event.FastContext) KeyAction {
	return s.driver.DecideOne(ev, ctx)
}

type stubQuery struct{ driver *Driver }

func NewWindowQuery(d *Driver) WindowQuery { return &stubQuery{driver: d} }

func (s *stubQuery) Focused() (FocusedWindow, error) {
	return FocusedWindow{}, s.driver.ReportError("query", errUnsupported)
}
func (s *stubQuery) MoveResize(m MoveResize) error {
	return s.driver.ReportError("move", errUnsupported)
}
func (s *stubQuery) SetWindowAction(a WindowAction) error {
	return s.driver.ReportError("window action", errUnsupported)
}

// The rot guard for every GOOS whose build compiles this file: it resolves
// only while stubQuery still carries all three WindowQuery methods with
// the signatures the interface gives them, so a method dropped or renamed
// here breaks the linux cross-GOOS build under this var instead of
// surfacing as a nil-satisfaction somewhere a test would have to find.
var _ = func(s *stubQuery, a WindowAction) error { return s.SetWindowAction(a) }

type stubProbe struct{ driver *Driver }

func NewUIPIProbe(d *Driver) UIPIProbe { return &stubProbe{driver: d} }

func (s *stubProbe) Probe() UIPIStatus { return UIPIStatus{Applicable: false} }
