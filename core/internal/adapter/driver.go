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

// BindDecideForTest installs the decide entry the C tap callback uses, and
// DecideForTest runs one event through it. Exists so the daemon package can
// assert the END-TO-END suppress-or-pass property against the real
// dispatcher, not a mock (bead cross-os-vx9).
func BindDecideForTest(d *Driver) { setDecideExport(d) }

// DecideForTest runs one event through the bound decide entry and returns
// the tap verdict (1 = suppress, 0 = pass through). It reconstructs the
// CGEvent flag the real callback would see, so a chord actually MATCHES its
// rule — otherwise "pass" would be trivially true and the test would prove
// nothing (the failure mode this helper exists to prevent).
func DecideForTest(ev event.Event, _ event.FastContext) int32 {
	var flags uint64
	if ev.Modifiers&(1<<0) != 0 { // ModCtrl
		flags |= cgFlagCtrl
	}
	if ev.Modifiers&(1<<1) != 0 { // ModShift
		flags |= cgFlagShift
	}
	if ev.Modifiers&(1<<2) != 0 { // ModAlt
		flags |= cgFlagAlt
	}
	if ev.Modifiers&(1<<3) != 0 { // ModMeta
		flags |= cgFlagMeta
	}
	keyDown := int32(0)
	if ev.Type == event.EventKeyDown {
		keyDown = 1
	}
	return crossosGoDecide(uint16(ev.KeyCode), flags, keyDown, nil)
}
