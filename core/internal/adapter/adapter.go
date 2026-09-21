// Adapter interface — the stable seam Core programs against.
//
// Every method maps 1:1 to a C-ABI function implemented in platform/.
// Implementations MUST be thin (translate + call + map errors); any policy
// (which key suppresses, which window moves) is decided in Core BEFORE the
// call. Bridge errors are returned to the Go caller AND recorded in stage
// logs by the caller — never silent (bead criterion: bridge errors surfaced,
// never swallowed).
package adapter

import (
	"crossos/core/pkg/event"
	"crossos/core/pkg/pluginapi"
)

// KeyAction is what the native keyboard tap/hook does with one event,
// mirroring pluginapi.Decision but scoped to the adapter boundary so the
// bridge does not import resolver internals.
type KeyAction int

const (
	// KeyPass returns the original event untouched.
	KeyPass KeyAction = iota
	// KeySuppress swallows the original, no replacement.
	KeySuppress
	// KeySuppressInject swallows the original; Core dispatches the resolved
	// Intent separately (injection only when the capability itself needs it).
	KeySuppressInject
)

// FromDecision maps Core's canonical decision to the adapter action. One
// mapping, one place — no per-platform reinterpretation.
func FromDecision(d pluginapi.Decision) KeyAction {
	switch d {
	case pluginapi.DecisionConsume:
		return KeySuppress
	case pluginapi.DecisionReplace:
		return KeySuppressInject
	default:
		return KeyPass
	}
}

// FocusedWindow is the adapter's query result: app + window identity +
// frame, mirroring spike C's Focused shape without importing it (spike
// packages are harnesses, not dependencies).
type FocusedWindow struct {
	BundleID   string
	PID        int
	Title      string
	Role       string
	ID         uint32
	X, Y, W, H float64
}

// MoveResize requests an AX/UIA move/resize. Validated by Core first.
type MoveResize struct {
	ID           uint32
	X, Y, W, H   float64
	Move, Resize bool
}

// KeyboardTap is the intercept seam: install/remove/decide. The callback
// classifies synchronously via Core's Decide (fast path) and returns the
// KeyAction; injection is dispatched off-callback (spike A/B reentrancy rule).
type KeyboardTap interface {
	Install() error
	Uninstall() error
	// DecideOne is the testable core: normalize → Decide → map. The live
	// callback calls this and returns immediately.
	DecideOne(ev event.Event, ctx event.FastContext) KeyAction
}

// WindowQuery is the context seam (spike C product path): focused query +
// move/resize. Permission-DENIED returns ErrPermissionDenied (typed, fast —
// never a hang); callers serve stale/empty cache on it.
type WindowQuery interface {
	Focused() (FocusedWindow, error)
	MoveResize(m MoveResize) error
}

// UIPIStatus reports Windows injection reachability (spike B product path):
// whether SendInput from this IL reaches the foreground target, plus the
// normative fallback. Non-Windows implementations return NotApplicable.
type UIPIStatus struct {
	// Applicable is false off Windows.
	Applicable bool
	// Elevated reports this process's integrity level.
	Elevated bool
	// Reachable reports the last probe's SendInput acceptance.
	Reachable bool
	// Fallback is the normative degraded behavior (never silent drop).
	Fallback string
}

// UIPIProbe is the Windows UIPI seam (spike B probeUIPI product path).
type UIPIProbe interface {
	Probe() UIPIStatus
}
