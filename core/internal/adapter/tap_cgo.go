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
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"crossos/core/pkg/event"
	"crossos/core/pkg/pluginapi"
)

// errTapDenied marks TCC input-monitoring denial (bridge returned NULL).
var errTapDenied = errors.New("adapter: tap refused (input-monitoring consent missing?)")

// CGEvent modifier flag bits → internal keyboard.Mod* bits. Mapping ALL of
// them matters: the Windows-muscle-memory rules are Win+Arrow (ModMeta) and
// Ctrl+Shift combos, so a tap that only understood Control would silently
// never fire them (bead cross-os-vx9 — found by the negative control in
// TestRealDispatchFailurePassesKeyThrough).
const (
	cgFlagShift = 1 << 0
	cgFlagCtrl  = 1 << 18
	cgFlagAlt   = 1 << 19
	cgFlagMeta  = 1 << 20
)

// modsFromFlags translates CGEventFlags into the internal modifier mask the
// router matches against (keyboard.ModCtrl/Shift/Alt/Meta).
func modsFromFlags(flags uint64) uint32 {
	var m uint32
	if flags&cgFlagCtrl != 0 {
		m |= 1 << 0 // ModCtrl
	}
	if flags&cgFlagShift != 0 {
		m |= 1 << 1 // ModShift
	}
	if flags&cgFlagAlt != 0 {
		m |= 1 << 2 // ModAlt (Option)
	}
	if flags&cgFlagMeta != 0 {
		m |= 1 << 3 // ModMeta (Command = the Windows key)
	}
	return m
}

// decideExport is the Go decide entry the C callback calls per key event.
// The C callback runs on the tap's run-loop thread while Install/Stop touch
// it from other goroutines, so every access is under decideExportMu.
var (
	decideExport   *Driver
	decideExportMu sync.RWMutex
)

func setDecideExport(d *Driver) {
	decideExportMu.Lock()
	decideExport = d
	decideExportMu.Unlock()
}

func getDecideExport() *Driver {
	decideExportMu.RLock()
	defer decideExportMu.RUnlock()
	return decideExport
}

//export crossosGoDecide
func crossosGoDecide(keycode uint16, flags uint64, keyDown int32, ctx unsafe.Pointer) int32 {
	d := getDecideExport()
	if d == nil || d.Decide == nil {
		return 0 // inert pass-through pre-init
	}
	typ := event.EventKeyUp
	if keyDown != 0 {
		typ = event.EventKeyDown
	}
	mods := modsFromFlags(flags)
	// CGEvent reports macOS virtual keycodes; the rule table is authored in
	// Windows VK. Translate here, at the only boundary that sees a mac keycode
	// (bead cross-os-uok). The core.keyEvent diagnostic IPC does NOT go
	// through this path — it already speaks the internal convention, so its
	// meaning is unchanged. Unmapped codes pass through and match nothing.
	win, _ := ToWinKeycode(keycode)
	// Focused-app context comes from a CACHED snapshot owned by the caller
	// (bead cross-os-heu) — never an AX query on this hot path.
	fastCtx := event.FastContext{AppMode: event.AppModeNative}
	if d.Context != nil {
		fastCtx = d.Context()
	}
	out := d.Decide(event.Event{
		Type:      typ,
		Source:    event.SourceKeyboard,
		KeyCode:   uint32(win),
		Modifiers: mods,
	}, fastCtx)
	if out.Decision == pluginapi.DecisionPass {
		return 0
	}
	// Decide said "handle this key". Before swallowing it, make sure the
	// action actually HAPPENED — suppressing a key whose action failed
	// leaves the user with a key that does nothing, which is worse than
	// passing it through (bead cross-os-vx9).
	if out.Request != nil {
		if d.Dispatch == nil {
			// Nothing can execute: never eat the key.
			return 0
		}
		if err := d.Dispatch(*out.Request); err != nil {
			// Action failed (permission denied, no adapter, ...): pass the
			// original through so the OS still sees it, and log the reason.
			if d.Log != nil {
				d.Log.Log("action", "dispatch failed, passing key through: "+err.Error())
			}
			return 0
		}
		return 1 // action done — suppress the original
	}
	// CONSUME with no request is a declared absorb: swallow with no action
	// to run. That is the rule's stated behavior, not a failure.
	return 1
}

// TapInstall creates the live tap (NULL → typed denial, dual-surfaced).
// Prefer TapStart, which also owns the run loop; this raw form is what the
// seam tests and non-loop callers use.
func TapInstall(d *Driver) error {
	setDecideExport(d) // BEFORE create: the source can fire the moment it is enabled
	tap := C.cxtap_create(nil, nil)
	if tap == nil {
		setDecideExport(nil)
		return d.ReportError("tap", errTapDenied)
	}
	tapMu.Lock()
	tapHandle = tap
	tapMu.Unlock()
	return nil
}

// TapUninstall destroys the live tap (nil-safe, idempotent). It does NOT
// stop the run loop — TapStop owns that; Uninstall is the raw teardown both
// TapStop and the tests use.
func TapUninstall(d *Driver) error {
	tapMu.Lock()
	tap := tapHandle
	tapHandle = nil
	tapMu.Unlock()
	if tap != nil {
		C.cxtap_destroy(tap)
	}
	decideExportMu.Lock()
	decideExport = nil
	decideExportMu.Unlock()
	return nil
}

// --- run-loop ownership (bead cross-os-2io) ---
//
// CGEventTap delivery requires a live CFRunLoop: cxtap_create attaches the
// Mach-port source to CFRunLoopGetCurrent(). Installing from a goroutine
// that then blocks elsewhere (the daemon's signal wait) would never fire a
// callback, so the tap gets its OWN locked OS thread that owns its run loop
// for the process lifetime. TapStart/TapStop own that thread.

var (
	tapLoopStop chan struct{} // closed to stop the run loop
	tapLoopDone chan struct{} // closed when the run loop goroutine exits
	tapLive     atomic.Bool   // true between a successful install and stop
	tapLoopMu   sync.Mutex    // guards tapLoopStop/tapLoopDone (TapStart/TapStop)
	tapMu       sync.Mutex    // guards tapHandle across goroutines
	tapStopOnce sync.Mutex    // serializes TapStop against a concurrent TapStart
)

// TapStart installs the tap on a dedicated locked OS thread and runs its
// CFRunLoop there. It blocks only long enough to learn whether the install
// succeeded (TCC consent) — the run loop itself keeps running in the
// background and feeds crossosGoDecide.
//
// Denial is a typed error, never a panic: the caller (daemon) keeps serving
// IPC and reports interception=off.
func TapStart(d *Driver) error {
	tapStopOnce.Lock()
	defer tapStopOnce.Unlock()
	tapLoopMu.Lock()
	if tapLoopStop != nil {
		tapLoopMu.Unlock()
		return errors.New("adapter: tap already running")
	}
	install := make(chan error, 1)
	stop := make(chan struct{})
	done := make(chan struct{})
	tapLoopStop, tapLoopDone = stop, done
	tapLoopMu.Unlock()

	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(done)

		// Bind the decide entry BEFORE the source exists: cxtap_create
		// enables the tap, so a callback can fire the moment it returns.
		setDecideExport(d)
		tap := C.cxtap_create(nil, nil)
		if tap == nil {
			setDecideExport(nil)
			install <- d.ReportError("tap", errTapDenied)
			return
		}
		tapMu.Lock()
		tapHandle = tap
		tapMu.Unlock()
		install <- nil
		tapLive.Store(true)

		// Stop arrives on another goroutine; wake the run loop through the
		// stored loop ref (CFRunLoopGetCurrent would be the wrong loop
		// off-thread). The local `tap` avoids racing the global handle.
		go func() {
			<-stop
			C.cxtap_stop(tap)
		}()

		C.cxtap_run(tap) // blocks until cxtap_stop
		tapLive.Store(false)
	}()

	select {
	case err := <-install:
		return err
	case <-time.After(2 * time.Second):
		return errors.New("adapter: tap install timed out")
	}
}

// TapStop stops the run loop and destroys the tap. Safe to call when no
// tap is running (idempotent no-op) so shutdown paths need no guard.
func TapStop(d *Driver) error {
	tapStopOnce.Lock()
	defer tapStopOnce.Unlock()
	tapLoopMu.Lock()
	stop, done := tapLoopStop, tapLoopDone
	tapLoopStop, tapLoopDone = nil, nil
	tapLoopMu.Unlock()
	if stop == nil {
		return nil
	}
	close(stop)
	<-done
	return TapUninstall(d)
}

// TapLive reports whether a tap is currently installed and running — the
// daemon serves this over IPC so the UI can show interception on/off.
func TapLive() bool { return tapLive.Load() }

// TapReenable re-enables after kCGEventTapDisabledByTimeout (Go Recovery
// loop owns the backoff + escalation — spike A recovery.go).
func TapReenable() {
	tapMu.Lock()
	tap := tapHandle
	tapMu.Unlock()
	if tap != nil {
		C.cxtap_enable(tap)
	}
}

// TapPostF9 posts the synthetic F9 replacement (async handoff only —
// never inside the callback; spike A/B reentrancy rule).
func TapPostF9() {
	C.cxpost_f9()
}

// tapHandle is the installed tap record, or NULL. Guarded by tapMu: it is
// written on the tap thread at install and read by TapStop/TapReenable on
// other goroutines.
var tapHandle *C.cxtap_t
