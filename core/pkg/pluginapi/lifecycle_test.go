package pluginapi

import "testing"

func TestEnableFlow(t *testing.T) {
	// DISABLED → TRIAL → ENABLED is the only enable path.
	if err := Transition(LifecycleDisabled, LifecycleTrial); err != nil {
		t.Fatalf("disabled→trial: %v", err)
	}
	if err := Transition(LifecycleTrial, LifecycleEnabled); err != nil {
		t.Fatalf("trial→enabled: %v", err)
	}
	// Crash/timeout/kill-switch during TRIAL → DISABLED.
	if err := Transition(LifecycleTrial, LifecycleDisabled); err != nil {
		t.Fatalf("trial→disabled: %v", err)
	}
}

func TestIllegalTransitions(t *testing.T) {
	for _, tc := range [][2]LifecycleState{
		{LifecycleDisabled, LifecycleEnabled}, // must pass through TRIAL
		{LifecycleEnabled, LifecycleTrial},    // no re-trial without disable
		{LifecycleTrial, LifecycleRunning},    // enable flow ≠ daemon flow
		{LifecycleRunning, LifecycleEnabled},  // daemon flow ≠ enable flow
		{LifecycleSafeMode, LifecycleEnabled},
		{"bogus", LifecycleEnabled},
	} {
		if err := Transition(tc[0], tc[1]); err == nil {
			t.Fatalf("expected rejection of %q → %q", tc[0], tc[1])
		}
	}
}

func TestDaemonFlow(t *testing.T) {
	for _, tc := range [][2]LifecycleState{
		{LifecycleInitializing, LifecycleRunning},
		{LifecycleRunning, LifecycleSafeMode},
		{LifecycleSafeMode, LifecycleRunning},
		{LifecycleRunning, LifecycleStopped},
		{LifecycleStopped, LifecycleInitializing},
	} {
		if err := Transition(tc[0], tc[1]); err != nil {
			t.Fatalf("legal %q → %q rejected: %v", tc[0], tc[1], err)
		}
	}
}

func TestCompatibleWith(t *testing.T) {
	ok := APIVersion{MinCoreVersion: "0.1.0"}
	if err := ok.CompatibleWith("0.1.0"); err != nil {
		t.Fatalf("equal versions: %v", err)
	}
	if err := ok.CompatibleWith("0.2.0"); err != nil {
		t.Fatalf("newer core: %v", err)
	}
	if err := ok.CompatibleWith("0.0.9"); err == nil {
		t.Fatal("expected rejection of older core")
	}
	empty := APIVersion{}
	if err := empty.CompatibleWith("1.0.0"); err == nil {
		t.Fatal("expected rejection of empty min_core_version")
	}
	bad := APIVersion{MinCoreVersion: "not-a-version"}
	if err := bad.CompatibleWith("1.0.0"); err == nil {
		t.Fatal("expected rejection of malformed min_core_version")
	}
}
