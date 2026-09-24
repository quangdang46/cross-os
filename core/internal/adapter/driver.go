// Driver wires the adapter seams to Core's decision path.
//
// The driver owns no policy: DecideOne fills Core's Event/FastContext (the
// native callback fills them from tap/hook data), calls Router.Decide, maps
// the canonical Decision via FromDecision. Bridge errors (install/remove,
// query, move/resize, probe) are returned AND appended to the caller's
// stage-log sink — never silent.
//
// Everything here is GOOS-portable on purpose: the Windows hook, the
// non-darwin stub and daemon code all drive Driver. The macOS tap-entry
// test seam is NOT portable — it calls setDecideExport/crossosGoDecide
// (tap_cgo.go) and ToWinKeycode (keycode_darwin.go), all darwin-constrained —
// so it lives in tap_darwin.go beside the callback it exercises. Leaving it
// here gave the whole core module a darwin-only build failure on every other
// GOOS, which meant no compiler at all on a non-mac dev box (cross-os-j7p).
package adapter

import (
	"errors"

	"crossos/core/pkg/event"
	"crossos/core/pkg/intent"
)

// ErrPermissionDenied is the typed accessibility-consent denial (spike C
// product path): callers serve stale/empty cache, never block or retry-loop.
var ErrPermissionDenied = errors.New("adapter: accessibility permission denied")

// ErrNotInstalled reports calls against a non-installed tap/hook.
var ErrNotInstalled = errors.New("adapter: tap/hook not installed")

// StageLogger receives bridge error lines for Core's per-stage logs.
type StageLogger interface {
	Log(stage, msg string)
}

// RouterDecide is the Core decision function the driver calls. It matches
// event.Router.Decide's shape without importing resolver wiring — Core
// passes rt.Decide bound.
type RouterDecide func(ev event.Event, ctx event.FastContext) event.Outcome

// Driver is the product tap driver: thin glue between the native callback
// and Core's Router.
type Driver struct {
	Decide RouterDecide
	Log    StageLogger
	// Dispatch executes an authorized capability (bead cross-os-vx9). The tap
	// calls it between Decide and suppress: a key is swallowed ONLY when its
	// action actually ran. A nil Dispatch means nothing can execute, so the
	// tap passes keys through rather than eating them.
	Dispatch func(intent.Request) error
	// Context supplies the focused app for the decision (bead cross-os-heu).
	// It MUST return a cached snapshot: the tap callback is on the hot path
	// and an AX query per keystroke would blow the <1ms budget. Nil falls
	// back to AppModeNative with an empty AppID, which is honest but makes
	// every app-scoped rule unreachable.
	Context func() event.FastContext
}

// DecideOne implements KeyboardTap.DecideOne: Core decides, the bridge maps.
func (d *Driver) DecideOne(ev event.Event, ctx event.FastContext) KeyAction {
	if d.Decide == nil {
		return KeyPass // no router bound (tests, pre-init): inert pass-through
	}
	out := d.Decide(ev, ctx)
	return FromDecision(out.Decision)
}

// ReportError surfaces a bridge error to the caller AND the stage log.
// Convenience so every seam follows the same never-silent shape. A nil
// Driver is tolerated: seams can be built before a logger exists, and a
// missing logger must not turn a typed denial into a panic.
func (d *Driver) ReportError(stage string, err error) error {
	if err != nil && d != nil && d.Log != nil {
		d.Log.Log(stage, err.Error())
	}
	return err
}
