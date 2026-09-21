package safety

import (
	"testing"
	"time"

	"crossos/core/pkg/pluginapi"
)

func testClock(start time.Time) (func() time.Time, *time.Time) {
	now := start
	return func() time.Time { return now }, &now
}

func TestTrialHappyPath(t *testing.T) {
	now, _ := testClock(time.Now())
	tr, err := BeginTrial("p1", now)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if tr.State != pluginapi.LifecycleTrial {
		t.Fatalf("state: got %q", tr.State)
	}
	if err := tr.Confirm(true, true); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if tr.State != pluginapi.LifecycleEnabled {
		t.Fatalf("state: got %q", tr.State)
	}
	// Every transition logged with plugin ID + timestamp.
	if len(tr.Log) != 2 {
		t.Fatalf("want 2 safety events, got %d", len(tr.Log))
	}
	for _, e := range tr.Log {
		if e.PluginID != "p1" || e.At.IsZero() {
			t.Fatalf("event missing plugin/timestamp: %+v", e)
		}
	}
}

func TestNoAutoApprove(t *testing.T) {
	now, _ := testClock(time.Now())
	tr, err := BeginTrial("p1", now)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	// No confirm → stays un-enabled.
	if tr.State == pluginapi.LifecycleEnabled {
		t.Fatal("enabled without confirm")
	}
	// Explicit refusal.
	if err := tr.Confirm(false, true); err == nil {
		t.Fatal("unconfirmed enable allowed")
	}
	if tr.State == pluginapi.LifecycleEnabled {
		t.Fatal("enabled after refused confirm")
	}
	// Unhealthy.
	if err := tr.Confirm(true, false); err == nil {
		t.Fatal("unhealthy enable allowed")
	}
}

func TestTrialTimeout(t *testing.T) {
	start := time.Now()
	nowFn, now := testClock(start)
	tr, err := BeginTrial("p1", nowFn)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	*now = start.Add(TRIALTimeout + time.Second)
	if !tr.Expired() {
		t.Fatal("should be expired past TRIALTimeout")
	}
	if err := tr.Confirm(true, true); err == nil {
		t.Fatal("confirm after timeout allowed")
	}
	if err := tr.Abort("timeout"); err != nil {
		t.Fatalf("abort: %v", err)
	}
	if tr.State != pluginapi.LifecycleDisabled {
		t.Fatalf("state: got %q", tr.State)
	}
}

func TestKillSwitchKeepsLoginItem(t *testing.T) {
	// PANIC STOP ≠ uninstall: login item stays, everything else stops.
	r := PanicStop()
	if !r.InterceptionDisabled || !r.PluginActionsStopped || !r.BuffersFlushed {
		t.Fatalf("kill switch incomplete: %+v", r)
	}
	if !r.LoginItemKept {
		t.Fatal("kill switch must keep the login item (Reset removes it)")
	}
}

func TestRollbackOrderAndBoundary(t *testing.T) {
	base := time.Now()
	s := IntegrationState{Timestamp: base, Integrations: []IntegrationRecord{
		{PluginID: "p", Type: IntegrationLoginItem, Identifier: "com.crossos",
			StateBefore: "absent", Rollback: "remove login item",
			CreatedAt: base.Add(time.Second)},
		{PluginID: "p", Type: IntegrationConfig, Identifier: "~/.crossos/config.json",
			StateBefore: "absent", Rollback: "delete file",
			CreatedAt: base.Add(2 * time.Second)},
	}}
	plan := s.RollbackPlan()
	if len(plan) != 2 || plan[0].Type != IntegrationConfig {
		t.Fatalf("rollback must run newest-first: %+v", plan)
	}
	// Boundary: only CrossOS-owned types exist — no OS-grant types.
	for _, rec := range plan {
		switch rec.Type {
		case IntegrationLoginItem, IntegrationAccessTap, IntegrationFinderSync,
			IntegrationAccessibility, IntegrationConfig:
		default:
			t.Fatalf("rollback covers non-owned type %q", rec.Type)
		}
	}
}

func TestResetPlan(t *testing.T) {
	s := IntegrationState{Integrations: []IntegrationRecord{
		{PluginID: "p", Type: IntegrationConfig, Identifier: "x",
			CreatedAt: time.Now()},
	}}
	p := PlanReset(s)
	if !p.RemoveLoginItem || !p.DisableExtension || !p.CleanOwnedState || !p.VerifyNoProcess {
		t.Fatalf("reset incomplete: %+v", p)
	}
	if len(p.OwnedRecords) != 1 {
		t.Fatalf("reset must carry owned records: %+v", p)
	}
}

func TestManifestScopeCheck(t *testing.T) {
	needs := map[string]string{
		"window.close": "accessibility.control",
		"window.move":  "accessibility.control",
	}
	// Exact scope → pass.
	if err := ManifestScopeCheck(
		[]string{"accessibility.control"}, needs); err != nil {
		t.Fatalf("exact scope rejected: %v", err)
	}
	// Over-scoped (keyboard plugin declaring filesystem) → reject.
	if err := ManifestScopeCheck(
		[]string{"accessibility.control", "filesystem"}, needs); err == nil {
		t.Fatal("over-scoped manifest allowed")
	}
	// Under-declared → reject.
	if err := ManifestScopeCheck(nil, needs); err == nil {
		t.Fatal("under-declared manifest allowed")
	}
}

func TestTrialTimeoutValue(t *testing.T) {
	// Pins §8.1: exactly 30s, defined once here.
	if TRIALTimeout != 30*time.Second {
		t.Fatalf("TRIALTimeout: got %v", TRIALTimeout)
	}
}
