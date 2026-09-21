package daemon

import (
	"testing"

	"crossos/core/pkg/pluginapi"
)

func TestFullLifecycle(t *testing.T) {
	d := New()
	if d.State() != pluginapi.LifecycleInitializing {
		t.Fatalf("start: got %q", d.State())
	}
	if err := d.Run(); err != nil {
		t.Fatalf("run: %v", err)
	}
	if err := d.EnterSafeMode(); err != nil {
		t.Fatalf("safe: %v", err)
	}
	if err := d.ExitSafeMode(); err != nil {
		t.Fatalf("exit safe: %v", err)
	}
	if err := d.Pause(); err != nil {
		t.Fatalf("pause: %v", err)
	}
	if err := d.Resume(); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if err := d.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if d.State() != pluginapi.LifecycleStopped {
		t.Fatalf("end: got %q", d.State())
	}
	// 7 events: run/safe/exit-safe/pause/resume→enabled/resume→running/stop.
	evs := d.Events()
	if len(evs) != 7 {
		t.Fatalf("want 7 audited events, got %d", len(evs))
	}
	// Every event carries full From/At (review fix: cross-os-c0 — the
	// resume→running step once recorded a sourceless zero-timestamp event).
	for i, e := range evs {
		if e.At.IsZero() {
			t.Fatalf("event %d missing timestamp: %+v", i, e)
		}
	}
	// First event has empty From (genesis); all others chain.
	for i := 1; i < len(evs); i++ {
		if evs[i].From == "" {
			t.Fatalf("event %d missing From: %+v", i, evs[i])
		}
	}
}

func TestIllegalMoves(t *testing.T) {
	d := New()
	// initializing → running only; stop/pause/safe illegal from init... except
	// stopped IS legal per the table (init→stopped for fast shutdown).
	if err := d.Pause(); err == nil {
		t.Fatal("pause from initializing allowed")
	}
	if err := d.EnterSafeMode(); err == nil {
		t.Fatal("safemode from initializing allowed")
	}
	if err := d.Resume(); err == nil {
		t.Fatal("resume from initializing allowed")
	}
}

func TestStopFromSafeMode(t *testing.T) {
	d := New()
	if err := d.Run(); err != nil {
		t.Fatal(err)
	}
	if err := d.EnterSafeMode(); err != nil {
		t.Fatal(err)
	}
	if err := d.Stop(); err != nil {
		t.Fatalf("stop from safemode: %v", err)
	}
}
