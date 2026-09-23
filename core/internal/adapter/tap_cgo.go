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
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"crossos/core/pkg/event"
	"crossos/core/pkg/intent"
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
		// Hand the action to the worker and return NOW. The capability
		// implementations do synchronous AX work, and this callback has a
		// sub-millisecond budget: running them inline is what makes macOS
		// post kCGEventTapDisabledByTimeout and switch the tap off.
		enqueueDispatch(d, *out.Request)
		return 1 // the action is committed to the worker; suppress the original
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
	// Destroy under the lock: every other user of the handle (the recovery
	// goroutine, TapReenable) holds it across its C call, so holding it here
	// is what makes the free safe.
	tapMu.Lock()
	tap := tapHandle
	tapHandle = nil
	if tap != nil {
		C.cxtap_destroy(tap)
	}
	tapMu.Unlock()
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
		if err != nil {
			// Release the lifecycle slots: a failed install (TCC denial) must
			// not make every later start report "tap already running" while
			// nothing is running. The goroutine has already returned, so the
			// channels are safe to clear.
			tapLoopMu.Lock()
			tapLoopStop, tapLoopDone = nil, nil
			tapLoopMu.Unlock()
		}
		return err
	case <-time.After(installTimeout):
		// (6) Do not report failure while the install may still succeed in
		// the background — saying "failed" and then having a live tap come
		// up is the worst of both. Report it as still starting; the caller
		// checks TapLive for the truth.
		return errors.New("adapter: tap install still in progress (no response yet)")
	}
}

// installTimeout bounds how long TapStart waits for the install reply. It is
// generous because exceeding it is not a failure, just "still starting".
const installTimeout = 5 * time.Second

// stopTimeout bounds TapStop's wait for the run loop to exit. A wedged
// callback must not hang shutdown: a daemon that cannot exit leaves a live
// tap behind a socket nobody is serving, which is worse than a slow stop.
const stopTimeout = 3 * time.Second

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
	select {
	case <-done:
	case <-time.After(stopTimeout):
		// The run loop did not wind down in time. Tear the tap down anyway:
		// an un-destroyed tap outlives the process, and the socket is
		// about to close with nothing serving it.
		_ = TapUninstall(d)
		return fmt.Errorf("adapter: tap run loop did not stop within %s", stopTimeout)
	}
	return TapUninstall(d)
}

// TapLive reports whether a tap is currently installed and running — the
// daemon serves this over IPC so the UI can show interception on/off.
func TapLive() bool { return tapLive.Load() }

// TapReenable re-enables after kCGEventTapDisabledByTimeout (Go Recovery
// loop owns the backoff + escalation — spike A recovery.go).
func TapReenable() {
	tapMu.Lock()
	defer tapMu.Unlock() // held across the C call: destroy takes the same lock
	if tapHandle != nil {
		C.cxtap_enable(tapHandle)
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

// --- timeout-disable recovery (audit blocker) ---
//
// macOS disables an event tap whenever its callback overruns the system
// budget and posts kCGEventTapDisabledByTimeout. Before this the C callback
// returned the event and nothing re-enabled the tap: remapping stopped
// working with no error, no log line, and no recovery but a daemon restart.
//
// Recovery is a LOOP with a bound, not a one-shot: one re-enable treats a
// symptom, while repeated disables mean the callback is systematically too
// slow. Past the bound the tap is declared unhealthy and the health signal
// is surfaced over IPC rather than pretending remapping still works.

// Bounds mirror the spike A recovery harness (platform/darwin/spike_a).
const (
	RecoveryWindow  = 30 * time.Second
	MaxReenables    = 5
	ReenableBackoff = 100 * time.Millisecond
)

// tapHealthState guards the recovery counters: the C callback runs on the
// run-loop thread while re-enables and status reads happen elsewhere.
var tapHealthState struct {
	sync.Mutex
	disables  []time.Time
	escalated bool
	reenables int
}

// crossosGoTapDisabled is called by the C callback on
// kCGEventTapDisabledByTimeout. It schedules the re-enable on its own
// goroutine so the callback returns immediately — running recovery inline
// would blow the budget that caused the disable.
//
//export crossosGoTapDisabled
func crossosGoTapDisabled() {
	now := time.Now()
	tapHealthState.Lock()
	cutoff := now.Add(-RecoveryWindow)
	kept := tapHealthState.disables[:0]
	for _, t := range tapHealthState.disables {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	tapHealthState.disables = append(kept, now)
	escalated := len(tapHealthState.disables) > MaxReenables
	if escalated {
		tapHealthState.escalated = true
	}
	tapHealthState.Unlock()

	if escalated {
		// Repeated disables: the callback is too slow to run this way. Stop
		// poking a dead tap and let the health signal surface instead.
		return
	}
	go func() {
		time.Sleep(ReenableBackoff)
		tapMu.Lock()
		tap := tapHandle
		tapMu.Unlock()
		if tap == nil {
			return
		}
		C.cxtap_enable(tap)
		tapHealthState.Lock()
		tapHealthState.reenables++
		tapHealthState.Unlock()
	}()
}

// TapUnhealthy reports whether the tap gave up after repeated
// timeout-disables. The daemon serves this so the UI can say remapping
// degraded instead of silently doing nothing.
func TapUnhealthy() bool {
	tapHealthState.Lock()
	defer tapHealthState.Unlock()
	return tapHealthState.escalated
}

// TapRecoveryStats reports disables seen and re-enables performed.
func TapRecoveryStats() (disables, reenables int) {
	tapHealthState.Lock()
	defer tapHealthState.Unlock()
	return len(tapHealthState.disables), tapHealthState.reenables
}

// resetTapRecoveryForTest clears the recovery counters so a test starts from
// the same state a fresh install has.
func resetTapRecoveryForTest() {
	tapHealthState.Lock()
	tapHealthState.disables = nil
	tapHealthState.escalated = false
	tapHealthState.reenables = 0
	tapHealthState.Unlock()
}

// --- async dispatch ---
//
// The capability implementations do synchronous AX work (tens of ms per
// call). The tap callback has a sub-millisecond budget, so dispatch runs on
// a worker and the callback returns immediately.
//
// Trade this makes explicit: the callback can no longer wait for the action
// to SUCCEED before suppressing, because waiting is what blows the budget.
// Instead a failed dispatch is logged loudly and surfaced as a health
// signal, rather than silently swallowed. The alternative — eating the key
// only on success — is only achievable by blocking the tap, which is the
// self-inflicted timeout this design exists to avoid.

const dispatchQueue = 64

var dispatchJobs = make(chan dispatchJob, dispatchQueue)

type dispatchJob struct {
	driver *Driver
	req    intent.Request
}

// dispatchFailures counts actions that never ran, for the health signal.
var dispatchFailures atomic.Int64

// startDispatcher runs the single worker. Started lazily by enqueue.
var dispatcherOnce sync.Once

func enqueueDispatch(d *Driver, req intent.Request) {
	dispatcherOnce.Do(func() { go dispatchWorker() })
	select {
	case dispatchJobs <- dispatchJob{driver: d, req: req}:
	default:
		// Queue full means the actions are slower than the keys arriving.
		// Dropping keeps the callback fast; count it so it is not silent.
		dispatchFailures.Add(1)
	}
}

func dispatchWorker() {
	for job := range dispatchJobs {
		if err := job.driver.Dispatch(job.req); err != nil {
			// The key is already gone; count it so the daemon can say so.
			dispatchFailures.Add(1)
			if job.driver.Log != nil {
				job.driver.Log.Log("action", "dispatch failed after suppression: "+err.Error())
			}
		}
	}
}

// DispatchFailures reports actions that were dropped or failed to run.
func DispatchFailures() int64 { return dispatchFailures.Load() }
