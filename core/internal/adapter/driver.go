// Driver wires the adapter seams to Core's decision path.
//
// The driver owns no policy: DecideOne fills Core's Event/FastContext (the
// native callback fills them from tap/hook data), calls Router.Decide, maps
// the canonical Decision via FromDecision. Bridge errors (install/remove,
// query, move/resize, probe) are returned AND appended to the caller's
// stage-log sink — never silent.
package adapter

import (
	"errors"

	"crossos/core/pkg/event"
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
// Convenience so every seam follows the same never-silent shape.
func (d *Driver) ReportError(stage string, err error) error {
	if err != nil && d.Log != nil {
		d.Log.Log(stage, err.Error())
	}
	return err
}
