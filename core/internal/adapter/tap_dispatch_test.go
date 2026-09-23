package adapter

import (
	"errors"
	"testing"

	"crossos/core/pkg/event"
	"crossos/core/pkg/intent"
	"crossos/core/pkg/pluginapi"
)

// The tap must never swallow a key whose action did not run (bead
// cross-os-vx9). A key that does nothing is worse than one that reaches
// the app unchanged.

func replaceOutcome() event.Outcome {
	return event.Outcome{
		Decision: pluginapi.DecisionReplace,
		Request: &intent.Request{
			PluginID:   "test",
			Intent:     intent.Intent{ID: "window.move", Version: 1, Source: intent.SourceKeyboard},
			Capability: intent.CapabilityDescriptor{ID: "window.move", Version: "1"},
		},
	}
}

func TestTapSuppressesOnlyAfterSuccessfulDispatch(t *testing.T) {
	var ran bool
	d := &Driver{
		Decide:   func(event.Event, event.FastContext) event.Outcome { return replaceOutcome() },
		Dispatch: func(intent.Request) error { ran = true; return nil },
	}
	setDecideExport(d)
	defer setDecideExport(nil)

	if got := crossosGoDecide(0x7B, 0, 1, nil); got != 1 {
		t.Fatalf("action dispatched: verdict=%d, want 1 (suppress)", got)
	}
	if !ran {
		t.Fatal("dispatcher never ran")
	}
}

func TestTapPassesKeyThroughWhenDispatchFails(t *testing.T) {
	d := &Driver{
		Decide:   func(event.Event, event.FastContext) event.Outcome { return replaceOutcome() },
		Dispatch: func(intent.Request) error { return errors.New("permission denied") },
	}
	setDecideExport(d)
	defer setDecideExport(nil)

	if got := crossosGoDecide(0x7B, 0, 1, nil); got != 0 {
		t.Fatalf("dispatch failed: verdict=%d, want 0 (pass through — a dead key is worse)", got)
	}
}

func TestTapPassesThroughWithoutDispatcher(t *testing.T) {
	d := &Driver{Decide: func(event.Event, event.FastContext) event.Outcome { return replaceOutcome() }}
	setDecideExport(d)
	defer setDecideExport(nil)

	if got := crossosGoDecide(0x7B, 0, 1, nil); got != 0 {
		t.Fatalf("no dispatcher wired: verdict=%d, want 0", got)
	}
}

func TestTapConsumeWithoutRequestStillSuppresses(t *testing.T) {
	// A declared absorb (Emit=false) has no action to run; swallowing it IS
	// the rule's behavior, not a failure.
	d := &Driver{
		Decide: func(event.Event, event.FastContext) event.Outcome {
			return event.Outcome{Decision: pluginapi.DecisionConsume}
		},
		Dispatch: func(intent.Request) error { return errors.New("must not be called") },
	}
	setDecideExport(d)
	defer setDecideExport(nil)

	if got := crossosGoDecide(0x7B, 0, 1, nil); got != 1 {
		t.Fatalf("consume with no request: verdict=%d, want 1", got)
	}
}

// The timeout-disable path must actually recover, and must give up loudly
// rather than poking a dead tap forever (audit blocker: the tap used to die
// silently, leaving remapping broken with no signal).
func TestTimeoutDisableRecoversThenEscalates(t *testing.T) {
	// No live tap installed, so a scheduled re-enable finds a nil handle and
	// no-ops — the accounting is what this pins.
	for i := 0; i < MaxReenables; i++ {
		crossosGoTapDisabled()
	}
	if TapUnhealthy() {
		t.Fatalf("tap unhealthy after %d disables, want healthy below the bound", MaxReenables)
	}
	disables, _ := TapRecoveryStats()
	if disables != MaxReenables {
		t.Fatalf("disables=%d, want %d", disables, MaxReenables)
	}
	// Past the bound it escalates and stops scheduling.
	crossosGoTapDisabled()
	if !TapUnhealthy() {
		t.Fatal("sustained timeout-disables must escalate, never spin forever")
	}
	// A fresh install resets the state.
	resetTapRecoveryForTest()
	if TapUnhealthy() {
		t.Fatal("a fresh install must start from a clean recovery state")
	}
}
