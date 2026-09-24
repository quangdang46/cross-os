//go:build darwin

// Darwin tap seam: install/remove go through the live C-ABI bridge
// (TapInstall/TapUninstall in tap_cgo.go over platform/darwin/adapter).
// NULL from the bridge (no TCC input-monitoring consent) is the typed
// denial — never nil success — so the gap stays explicit at runtime.
//
// The macOS tap-entry test seam lives here rather than in driver.go because
// it is the one part of the driver that cannot compile off darwin: it reaches
// setDecideExport/crossosGoDecide (tap_cgo.go) and ToWinKeycode
// (keycode_darwin.go). Keeping it beside the C callback it drives costs macOS
// nothing — the darwin file set is unchanged — and gives every other GOOS a
// compiling package (cross-os-j7p). These are exported, not _test.go: the
// daemon package in cmd/crossos calls them, and an in-package test file is
// invisible across a package boundary.
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

// Focused reads the frontmost app + its focused window over the real AX
// bridge. A consent denial maps to ErrPermissionDenied so Core's cache
// serves stale/empty instead of hanging; other AX errors keep their own
// identity in the stage log.
func (q *darwinQuery) Focused() (FocusedWindow, error) {
	fw, err := axFocusedWindow()
	if err != nil {
		return FocusedWindow{}, q.driver.ReportError("query", err)
	}
	return fw, nil
}

// MoveResize moves/resizes the focused window. A request with no move and
// no resize is rejected up front (nothing to do). The AX path resolves the
// frontmost app's focused window itself, so ID is used for reporting only.
func (q *darwinQuery) MoveResize(m MoveResize) error {
	if !m.Move && !m.Resize {
		return q.driver.ReportError("move", errZeroWindow)
	}
	if err := axMoveResize(m); err != nil {
		return q.driver.ReportError("move", err)
	}
	return nil
}

type darwinProbe struct{ driver *Driver }

func NewUIPIProbe(d *Driver) UIPIProbe { return &darwinProbe{driver: d} }

func (p *darwinProbe) Probe() UIPIStatus { return UIPIStatus{Applicable: false} }

// BindDecideForTest installs the decide entry the C tap callback uses, and
// DecideForTest runs one event through it. Exists so the daemon package can
// assert the END-TO-END suppress-or-pass property against the real
// dispatcher, not a mock (bead cross-os-vx9).
func BindDecideForTest(d *Driver) { setDecideExport(d) }

// DecideForTest runs one event through the bound decide entry and returns
// the tap verdict (1 = suppress, 0 = pass through). It reconstructs the
// CGEvent flag the real callback would see, so a chord actually MATCHES its
// rule — otherwise "pass" would be trivially true and the test would prove
// nothing (the failure mode this helper exists to prevent).
func DecideForTest(ev event.Event, _ event.FastContext) int32 {
	var flags uint64
	if ev.Modifiers&(1<<0) != 0 { // ModCtrl
		flags |= cgFlagCtrl
	}
	if ev.Modifiers&(1<<1) != 0 { // ModShift
		flags |= cgFlagShift
	}
	if ev.Modifiers&(1<<2) != 0 { // ModAlt
		flags |= cgFlagAlt
	}
	if ev.Modifiers&(1<<3) != 0 { // ModMeta
		flags |= cgFlagMeta
	}
	keyDown := int32(0)
	if ev.Type == event.EventKeyDown {
		keyDown = 1
	}
	return crossosGoDecide(uint16(ev.KeyCode), flags, keyDown, nil)
}

// MustWinKeycodeForTest resolves a macOS keycode for tests that build events
// directly, failing the test when it is unmapped.
func MustWinKeycodeForTest(t interface{ Fatalf(string, ...any) }, mac uint16) uint16 {
	win, ok := ToWinKeycode(mac)
	if !ok {
		t.Fatalf("macOS keycode %#x has no Windows VK mapping", mac)
	}
	return win
}

// DecideForMacTest runs one chord through the bound tap entry using macOS
// keycodes, as the real C callback would. mods are internal Mod* bits.
func DecideForMacTest(macKey uint16, mods uint32, _ event.FastContext) int32 {
	var flags uint64
	if mods&(1<<0) != 0 {
		flags |= cgFlagCtrl
	}
	if mods&(1<<1) != 0 {
		flags |= cgFlagShift
	}
	if mods&(1<<2) != 0 {
		flags |= cgFlagAlt
	}
	if mods&(1<<3) != 0 {
		flags |= cgFlagMeta
	}
	return crossosGoDecide(macKey, flags, 1, nil)
}
