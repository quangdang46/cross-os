//go:build darwin

// Darwin tap seam: install/remove go through the live C-ABI bridge
// (TapInstall/TapUninstall in tap_cgo.go over platform/darwin/adapter).
// NULL from the bridge (no TCC input-monitoring consent) is the typed
// denial — never nil success — so the gap stays explicit at runtime.
package adapter

import (
	"crossos/core/pkg/event"
)

type darwinTap struct {
	driver    *Driver
	installed bool
}

func NewKeyboardTap(d *Driver) KeyboardTap { return &darwinTap{driver: d} }

func (t *darwinTap) Install() error {
	if err := TapInstall(t.driver); err != nil {
		return err // dual-surfaced inside TapInstall (caller + stage log)
	}
	t.installed = true
	return nil
}

func (t *darwinTap) Uninstall() error {
	t.installed = false
	return TapUninstall(t.driver)
}

func (t *darwinTap) DecideOne(ev event.Event, ctx event.FastContext) KeyAction {
	return t.driver.DecideOne(ev, ctx)
}

type darwinQuery struct{ driver *Driver }

func NewWindowQuery(d *Driver) WindowQuery { return &darwinQuery{driver: d} }

// Focused maps the consent-denied C-ABI result to ErrPermissionDenied so
// Core's cache serves stale/empty instead of hanging.
func (q *darwinQuery) Focused() (FocusedWindow, error) {
	return FocusedWindow{}, q.driver.ReportError("query", ErrPermissionDenied)
}

func (q *darwinQuery) MoveResize(m MoveResize) error {
	if m.ID == 0 {
		return q.driver.ReportError("move", errZeroWindow)
	}
	return q.driver.ReportError("move", ErrPermissionDenied)
}

type darwinProbe struct{ driver *Driver }

func NewUIPIProbe(d *Driver) UIPIProbe { return &darwinProbe{driver: d} }

func (p *darwinProbe) Probe() UIPIStatus { return UIPIStatus{Applicable: false} }
