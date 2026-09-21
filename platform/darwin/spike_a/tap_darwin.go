//go:build darwin

// Live CGEventTap loop — darwin-only seam. The tap callback stays thin:
// classify synchronously, return the decision, re-enable on timeout-disable
// via the Recovery loop. Injection uses CGEventCreateKeyboardEvent + post
// (async handoff, never inside the callback — same reentrancy rule as
// spike B's goroutine handoff).
//
// CGO is intentionally NOT wired in this spike: compiling a tap requires
// TCC input-monitoring consent at RUNTIME, which no sandbox/CI has, so the
// live path is Tier-2 gated and this file only holds the documented shape +
// the pure-Go driver the tests exercise via the enableFn seam. Bead
// cross-os-ab4 wires the real C-ABI bridge.
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
