// Portable driver seam: the Driver below is pure Go (decision +
// recovery math, no syscalls) and intentionally compiles everywhere so
// Tier-1 tests run on Windows/Linux CI AND on darwin (the spike's actual
// target — review: cross-os-ed caught that the old //go:build !darwin tag
// left Driver undefined on darwin). The live CGEventTap loop is
// darwin-only and Tier-2 gated; CGO is NOT wired in this spike (runtime TCC
// consent unavailable in sandbox/CI — bead cross-os-ab4 wires the C-ABI
// bridge). NOTE: tap_darwin.go holds the darwin-tagged live-loop doc only
// (no Go symbols); this file holds the portable driver.
package spikea

import "time"

// TapEvents enumerates the callback inputs the driver handles.
type TapEvents int

const (
	// TapKey is a normal key event delivered to the callback.
	TapKey TapEvents = iota
	// TapDisabledByTimeout is kCGEventTapDisabledByTimeout — the system
	// disabled the tap because the callback exceeded its time budget.
	TapDisabledByTimeout
)

// Driver is the testable live-tap driver: pure decision + recovery state,
// with reenable as the seam where the real CGEventTapEnable call goes.
type Driver struct {
	Replace  bool // suppress+replace (F9) vs suppress-only
	Recovery Recovery
	Latency  Latency
	// Reenable is called after backoff on each timeout-disable (nil = no-op,
	// used by tests that only assert the decision/recovery math).
	Reenable func()
	// Reenabled counts successful re-enables (stage log).
	Reenabled int
	// HealthFired latches when sustained disable escalates.
	HealthFired bool
}

// Handle processes one tap input at now. For TapKey it returns the decision
// (the C callback maps Pass→return event, Suppress→NULL,
// SuppressReplace→NULL + async CGEventPost of F9). For TapDisabledByTimeout
// it runs the recovery loop: re-enable within the measured bound, or escalate
// to the health signal when the window is exhausted.
func (d *Driver) Handle(ev TapEvents, keyCode uint16, ctrl, keyDown bool, now time.Time) Decision {
	if ev == TapDisabledByTimeout {
		backoff := d.Recovery.NoteDisable(now)
		if d.Recovery.Escalated {
			d.HealthFired = true
			return DecisionPass // tap dead: pass everything, surface health
		}
		if d.Reenable != nil {
			time.AfterFunc(backoff, d.Reenable)
		}
		d.Reenabled++
		return DecisionPass
	}
	start := time.Now()
	dec := ClassifyCtrlC(keyCode, ctrl, keyDown, d.Replace)
	d.Latency.Observe(time.Since(start))
	return dec
}
