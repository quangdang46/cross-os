// Spike A tests — bead cross-os-vjz.
//
// Tier 1 (always runs): pure decision, recovery-loop, and latency math — no
// macOS input needed.
// Tier 2 (live tap): gated behind -short skip; needs an interactive desktop
// with input-monitoring consent. Run `go test` (not -short) on real hardware.
//
// Pass criteria mapping:
//  1. Ctrl+C suppressed → TestClassifySuppress + Tier-2 TestLiveSuppress.
//  2. Replacement emitted → TestClassifyReplace + Tier-2 TestLiveReplace.
//  3. Re-enable after timeout-disable → TestRecoveryLoop (+ Tier-2 TestLiveReenable).
//  4. Callback p95 recorded → TestLatencyP95 (numbers, not adjectives).
//  5. Recovery is a loop with bounded escalate → TestRecoveryEscalates.
package spikea

import (
	"runtime"
	"testing"
	"time"
)

func TestClassifySuppress(t *testing.T) {
	if got := ClassifyCtrlC(KeyC, true, true, false); got != DecisionSuppress {
		t.Fatalf("Ctrl+C keydown suppress-only: got %v, want suppress", got)
	}
	if got := ClassifyCtrlC(KeyC, true, false, false); got != DecisionPass {
		t.Fatalf("Ctrl+C key-up: got %v, want pass", got)
	}
	if got := ClassifyCtrlC(KeyC, false, true, false); got != DecisionPass {
		t.Fatalf("bare C: got %v, want pass", got)
	}
	if got := ClassifyCtrlC(0x00, true, true, false); got != DecisionPass { // Ctrl+A keycode differs
		t.Fatalf("Ctrl+other: got %v, want pass", got)
	}
}

func TestClassifyReplace(t *testing.T) {
	if got := ClassifyCtrlC(KeyC, true, true, true); got != DecisionSuppressReplace {
		t.Fatalf("Ctrl+C keydown replace mode: got %v, want suppress+replace", got)
	}
	if got := ClassifyCtrlC(KeyC, true, false, true); got != DecisionPass {
		t.Fatalf("Ctrl+C key-up replace mode: got %v, want pass", got)
	}
}

func TestRecoveryLoop(t *testing.T) {
	var d Driver
	now := time.Now()
	// Up to MaxReenables disables: each re-enables with backoff.
	for i := 0; i < MaxReenables; i++ {
		if got := d.Handle(TapDisabledByTimeout, 0, false, false, now); got != DecisionPass {
			t.Fatalf("disable %d: want pass-through while recovering", i+1)
		}
		now = now.Add(time.Second)
	}
	if d.Reenabled != MaxReenables {
		t.Fatalf("reenabled=%d, want %d", d.Reenabled, MaxReenables)
	}
	if d.HealthFired {
		t.Fatal("health must not fire within the bound")
	}
}

func TestRecoveryEscalates(t *testing.T) {
	var d Driver
	now := time.Now()
	for i := 0; i <= MaxReenables; i++ {
		d.Handle(TapDisabledByTimeout, 0, false, false, now)
		now = now.Add(time.Second)
	}
	if !d.HealthFired {
		t.Fatal("sustained disable must escalate to health signal, never silent death")
	}
	if !d.Recovery.Escalated {
		t.Fatal("recovery must latch escalated")
	}
	// Old disables age out of the window: a disable long ago leaves the
	// current window empty and must not latch escalated.
	d2 := Driver{}
	old := time.Now().Add(-2 * RecoveryWindow)
	d2.Handle(TapDisabledByTimeout, 0, false, false, old)
	if got := d2.Recovery.DisableCount(time.Now()); got != 0 {
		t.Fatalf("window count=%d, want 0 (old disables age out)", got)
	}
	if got := d2.Recovery.DisableCount(old.Add(time.Second)); got != 1 {
		t.Fatalf("window count at old+1s=%d, want 1", got)
	}
	if d2.Recovery.Escalated {
		t.Fatal("single stale disable must not escalate")
	}
}

func TestLatencyP95(t *testing.T) {
	var l Latency
	if got := l.P95(); got != 0 {
		t.Fatalf("empty P95=%v, want 0", got)
	}
	// 100 samples: 1µs..100µs → p95 = 95µs by nearest-rank.
	for i := 1; i <= 100; i++ {
		l.Observe(time.Duration(i) * time.Microsecond)
	}
	if got := l.P95(); got != 95*time.Microsecond {
		t.Fatalf("P95=%v, want 95µs", got)
	}
	if got := l.Samples(); got != 100 {
		t.Fatalf("samples=%d, want 100", got)
	}
	// Feasibility verdict as a number: record the measured driver overhead.
	var d Driver
	for i := 0; i < 100; i++ {
		d.Handle(TapKey, KeyC, true, true, time.Now())
	}
	t.Logf("driver classify p50=%v p95=%v n=%d (target callback p95 ~100µs; live-tap number from Tier 2)",
		d.Latency.P50(), d.Latency.P95(), d.Latency.Samples())
}

// --- Tier 2: live tap (darwin interactive desktop only) ---

func TestLiveSuppress(t *testing.T) {
	if testing.Short() {
		t.Skip("needs interactive desktop + input-monitoring consent")
	}
	if runtime.GOOS != "darwin" {
		t.Skip("darwin-only")
	}
	t.Skip("live tap harness lands with cross-os-ab4 C-ABI bridge; Tier-1 decision+recovery math is the sandbox-runnable proof")
}

func TestLiveReplace(t *testing.T) {
	if testing.Short() {
		t.Skip("needs interactive desktop + input-monitoring consent")
	}
	if runtime.GOOS != "darwin" {
		t.Skip("darwin-only")
	}
	t.Skip("live tap harness lands with cross-os-ab4 C-ABI bridge; Tier-1 decision+recovery math is the sandbox-runnable proof")
}

func TestLiveReenable(t *testing.T) {
	if testing.Short() {
		t.Skip("needs interactive desktop + input-monitoring consent")
	}
	if runtime.GOOS != "darwin" {
		t.Skip("darwin-only")
	}
	t.Skip("live tap harness lands with cross-os-ab4 C-ABI bridge; Tier-1 decision+recovery math is the sandbox-runnable proof")
}
