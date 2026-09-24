//go:build darwin

// darwin-only: the dispatch path under test runs through crossosGoDecide and
// setDecideExport, the real CGEvent tap callback. The portable half of this
// property is covered by adapter_test.go (TestNoPolicyInBridge) and by
// pkg/winlayout's dispatch sweep (cross-os-j7p).
package adapter

import (
	"errors"
	"testing"
	"time"

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

// The callback must never block on the action: AX work takes tens of ms and
// the tap budget is sub-millisecond, so dispatch runs on a worker.
func TestTapReturnsWithoutWaitingForDispatch(t *testing.T) {
	release := make(chan struct{})
	entered := make(chan struct{}, 1)
	d := &Driver{
		Decide: func(event.Event, event.FastContext) event.Outcome { return replaceOutcome() },
		Dispatch: func(intent.Request) error {
			entered <- struct{}{} // signal we are inside, then block
			<-release
			return nil
		},
	}
	setDecideExport(d)
	defer setDecideExport(nil)

	done := make(chan int32, 1)
	go func() { done <- crossosGoDecide(0x08, cgFlagCtrl, 1, nil) }()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("dispatch never started")
	}
	// The verdict must arrive even though the dispatcher is still blocked.
	select {
	case got := <-done:
		if got != 1 {
			t.Fatalf("verdict=%d, want 1", got)
		}
	case <-time.After(time.Second):
		close(release)
		t.Fatal("callback blocked on dispatch — this is the timeout-disable cause")
	}
	close(release)
}

func TestTapPassesThroughWithoutDispatcher(t *testing.T) {
	d := &Driver{Decide: func(event.Event, event.FastContext) event.Outcome { return replaceOutcome() }}
	setDecideExport(d)
	defer setDecideExport(nil)

	if got := crossosGoDecide(0x08, cgFlagCtrl, 1, nil); got != 0 {
		t.Fatalf("no dispatcher wired: verdict=%d, want 0", got)
	}
}

func TestTapConsumeWithoutRequestStillSuppresses(t *testing.T) {
	// A declared absorb (Emit=false) has no action to run; swallowing it IS
	// the rule's stated behavior, not a failure.
	d := &Driver{
		Decide: func(event.Event, event.FastContext) event.Outcome {
			return event.Outcome{Decision: pluginapi.DecisionConsume}
		},
		Dispatch: func(intent.Request) error { return errors.New("must not be called") },
	}
	setDecideExport(d)
	defer setDecideExport(nil)

	if got := crossosGoDecide(0x08, cgFlagCtrl, 1, nil); got != 1 {
		t.Fatalf("consume with no request: verdict=%d, want 1", got)
	}
}
