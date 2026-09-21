//go:build windows

// Windows hook seam: install/remove go through crossos-keyboard-win.dll
// (WH_KEYBOARD_LL + SendInput + UIPI status query) loaded at runtime.
// Until the DLL loader lands (dll_windows.go), install returns a typed
// not-implemented error — never nil success.
package adapter

import (
	"crossos/core/pkg/event"
)

type windowsHook struct {
	driver    *Driver
	installed bool
}

func NewKeyboardTap(d *Driver) KeyboardTap { return &windowsHook{driver: d} }

func (h *windowsHook) Install() error {
	return h.driver.ReportError("hook", errTapNotWired)
}

func (h *windowsHook) Uninstall() error {
	h.installed = false
	return nil
}

func (h *windowsHook) DecideOne(ev event.Event, ctx event.FastContext) KeyAction {
	return h.driver.DecideOne(ev, ctx)
}

type windowsQuery struct{ driver *Driver }

func NewWindowQuery(d *Driver) WindowQuery { return &windowsQuery{driver: d} }

func (q *windowsQuery) Focused() (FocusedWindow, error) {
	return FocusedWindow{}, q.driver.ReportError("query", errTapNotWired)
}

func (q *windowsQuery) MoveResize(m MoveResize) error {
	if m.ID == 0 {
		return q.driver.ReportError("move", errZeroWindow)
	}
	return q.driver.ReportError("move", errTapNotWired)
}

type windowsProbe struct{ driver *Driver }

func NewUIPIProbe(d *Driver) UIPIProbe { return &windowsProbe{driver: d} }

// Probe reports the UIPI shape with the normative fallback: passthrough +
// user notice, never silent drop (spike B contract carried into the seam).
func (p *windowsProbe) Probe() UIPIStatus {
	return UIPIStatus{Applicable: true, Fallback: "passthrough+notice"}
}
