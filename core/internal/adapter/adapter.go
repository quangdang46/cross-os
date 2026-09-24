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
	"errors"

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

// WindowRow is one window as the physical plane describes it. Every field
// comes from a single batched WindowServer answer, so listing a screenful of
// them costs the same as listing one and a busy application cannot delay the
// answer. OnScreen and Minimized are separate because they answer different
// questions: an ordered-out window may be minimized, off-Space, or closing.
type WindowRow struct {
	// ID is the CGWindowID. It identifies a window to the WindowServer and
	// nothing else — there is no route from it to an AX element, so the
	// bridge treats it as a name, never a handle.
	ID  uint32
	PID int
	// BundleID is the owning application's. Empty for a process with no
	// bundle, which is a process no rule can be scoped to.
	BundleID string
	// Title is never fabricated. A window the system will not name is not a
	// row, because a row with a placeholder title is indistinguishable from
	// a real one in the list the user is choosing from.
	Title string
	// X, Y, W, H are screen points.
	X, Y, W, H float64
	OnScreen   bool
	Minimized  bool
}

// minimizedFrom decodes one batched row into a minimized flag. The ordered-in
// flag alone cannot say it: a window on another Space is ordered out too, and
// the two are told apart by geometry — minimize collapses the window's frame,
// an off-Space window keeps it. Both inputs are fields of the same batched
// answer the row already carries, which is the whole point: the alternative is
// a kAXMinimized read, a call into the owning application that stalls for as
// long as that application is busy and cannot be bounded from outside.
func minimizedFrom(onScreen bool, w, h float64) bool {
	return !onScreen && (w <= 0 || h <= 0)
}

// isSurface reports a window that is ordered in and occupies no area. The
// WindowServer marks these ordered in — an authentication helper, a
// notification anchor — and they carry a real window id, a real pid and
// often a real title, so nothing about them is obviously wrong. What makes
// them not windows is that there is no area to point at, aim a move at, or
// raise, and a row the user cannot aim at is the fabricated row this list
// exists to avoid.
//
// A minimized window has the same empty geometry and is deliberately not one
// of these: it is ordered OUT, and the list keeps it so a rule has something
// to name. The ordered-in flag is the whole difference, which is why both
// predicates read it.
func isSurface(onScreen bool, w, h float64) bool {
	return onScreen && (w <= 0 || h <= 0)
}

// errWindowGone reports a window id the physical plane no longer lists. It is
// not errZeroWindow: "no such window" and "no id was given" send the caller to
// different places, and the stage log is how the difference gets noticed.
var errWindowGone = errors.New("adapter: window id is not on this machine")

// WindowQuery is the context seam (spike C product path): focused query +
// move/resize. Permission-DENIED returns ErrPermissionDenied (typed, fast —
// never a hang); callers serve stale/empty cache on it.
type WindowQuery interface {
	Focused() (FocusedWindow, error)
	MoveResize(m MoveResize) error
}

// WindowLister is the enumeration + focus seam. It is a separate interface
// from WindowQuery because the three WindowQuery implementations live in
// files split by GOOS, and a method added to that interface has to be added
// to all three by hand — a seam whose implementations can only be edited from
// three places is a seam that rots in the one the author is not looking at.
// One type per GOOS, each in the file that already owns that platform, is the
// same split the package uses everywhere else.
type WindowLister interface {
	// ListWindows returns one row per on-screen normal-layer window with a
	// non-empty title. It never returns a partially-filled row: a window that
	// cannot be named or attributed is absent rather than a zero value.
	ListWindows() ([]WindowRow, error)
	// Focus brings the window named by id to the front. Focusing the window
	// already at the front is nil and touches no other application.
	Focus(id uint32) error
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
