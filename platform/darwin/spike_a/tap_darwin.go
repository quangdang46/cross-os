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

// Driver, TapEvents, and Handle live in tap_other.go (portable pure-Go
// driver, compiles everywhere for Tier-1 tests). This file reserves the
// darwin live-loop seam: the C-ABI bridge (bead cross-os-ab4) plugs the
// real CGEventTapCreate/Enable/Post calls here. No Go symbols needed yet —
// the presence of this file documents where the live loop lands.
