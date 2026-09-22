//go:build darwin

// CGO surface for the macOS tap bridge (native tap bridge bead; plan §4).
//
// This file is the ONLY cgo in the adapter: it compiles
// platform/darwin/adapter/tap_bridge.c and exposes TapInstall/TapUninstall/
// TapReenable/TapPostF9 to the darwinTap seam. Swift stays behind the ObjC++
// C-ABI surface (no Swift-direct-from-Go — adapter authority rule).
//
// TCC reality: CGEventTapCreate returns NULL without input-monitoring
// consent at RUNTIME. NULL maps to the typed error below (dual-surfaced via
// ReportError) — never nil success, never a panic. Sandbox/CI without
// consent exercises the typed-error path; hardware with consent gets the
// live tap.
package adapter

/*
#cgo CFLAGS: -I${SRCDIR}/../../../platform/darwin/adapter
#cgo LDFLAGS: -framework CoreGraphics -framework Cocoa
#include <stdint.h>
#include <stdlib.h>

// Bridge implementation (tap_bridge.c compiled into this cgo unit).
#include "../../../platform/darwin/adapter/tap_bridge.c"
*/
import "C"

import (
	"errors"
	"unsafe"

	"crossos/core/pkg/event"
	"crossos/core/pkg/pluginapi"
)

// errTapDenied marks TCC input-monitoring denial (bridge returned NULL).
var errTapDenied = errors.New("adapter: tap refused (input-monitoring consent missing?)")

// kCGControlFlag is kCGEventFlagMaskControl (NX_CONTROLMASK = 1<<18).
const kCGControlFlag = uint64(1 << 18)

// decideExport is the Go decide entry the C callback calls per key event.
// Set at Install from the bound Driver; cleared at Uninstall. Single-tap
// ownership: Install/Uninstall never race the callback.
var decideExport *Driver

//export crossosGoDecide
func crossosGoDecide(keycode uint16, flags uint64, keyDown int32, ctx unsafe.Pointer) int32 {
	if decideExport == nil {
		return 0 // inert pass-through pre-init
	}
	typ := event.EventKeyUp
	mods := uint32(0)
	if keyDown != 0 {
		typ = event.EventKeyDown
		if flags&kCGControlFlag != 0 {
			mods = 1 // modCtrl bit (matches builtin rules port)
		}
	}
	out := decideExport.Decide(event.Event{
		Type:      typ,
		Source:    event.SourceKeyboard,
		KeyCode:   uint32(keycode),
		Modifiers: mods,
	}, event.FastContext{AppMode: event.AppModeNative})
	if out.Decision == pluginapi.DecisionPass {
		return 0
	}
	return 1 // CONSUME/REPLACE both suppress; REPLACE injects via TapPostF9
}

// TapInstall creates the live tap (NULL → typed denial, dual-surfaced).
func TapInstall(d *Driver) error {
	decideExport = d
	tap := C.cxtap_create(nil, nil)
	if tap == nil {
		decideExport = nil
		return d.ReportError("tap", errTapDenied)
	}
	tapHandle = tap
	return nil
}

// TapUninstall destroys the live tap (nil-safe, idempotent).
func TapUninstall(d *Driver) error {
	if tapHandle != nil {
		C.cxtap_destroy(tapHandle)
		tapHandle = nil
	}
	decideExport = nil
	return nil
}

// TapReenable re-enables after kCGEventTapDisabledByTimeout (Go Recovery
// loop owns the backoff + escalation — spike A recovery.go).
func TapReenable() {
	if tapHandle != nil {
		C.cxtap_enable(tapHandle)
	}
}

// TapPostF9 posts the synthetic F9 replacement (async handoff only —
// never inside the callback; spike A/B reentrancy rule).
func TapPostF9() {
	C.cxpost_f9()
}

// tapHandle is the installed tap record, or NULL.
var tapHandle *C.cxtap_t
