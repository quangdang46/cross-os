// Consent backend tests — bead cross-os-nir.4.
//
// Pass criteria mapping:
//  1. Explicit per-plugin Enable, nothing auto-enabled → TestNoAutoEnable +
//     TestConfirmRequiresBoth.
//  2. TRIAL via Core constant (no local 30s literal) → TestTrialExpiry (clock
//     advanced past safety.TRIALTimeout, not a local number).
//  3. Readiness verification (keyboard/windows/finder) → TestReadiness.
//  4. Flow order (no skips/backward) → TestStepOrder.
//  5. Abort path → TestAbortEnable.
package shell

import (
	"testing"
	"time"

	"crossos/core/pkg/safety"
)

func TestNoAutoEnable(t *testing.T) {
	c := NewConsent(nil)
	st, err := c.RequestEnable("win-wm")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if st.Enabled || c.IsEnabled("win-wm") {
		t.Fatal("request must not enable (explicit confirm only)")
	}
	// Idempotent re-request: same trial, no second trial.
	st2, err := c.RequestEnable("win-wm")
	if err != nil || st2 != st {
		t.Fatalf("re-request: %+v %v", st2, err)
	}
}

func TestConfirmRequiresBoth(t *testing.T) {
	c := NewConsent(nil)
	if _, err := c.RequestEnable("p"); err != nil {
		t.Fatal(err)
	}
	if err := c.ConfirmEnable("p", false, true); err == nil {
		t.Fatal("unconfirmed enable allowed")
	}
	if err := c.ConfirmEnable("p", true, false); err == nil {
		t.Fatal("unhealthy enable allowed")
	}
	if c.IsEnabled("p") {
		t.Fatal("enabled without confirm+healthy")
	}
	if err := c.ConfirmEnable("p", true, true); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if !c.IsEnabled("p") {
		t.Fatal("confirmed+healthy must enable")
	}
}

func TestTrialExpiry(t *testing.T) {
	// Core constant, not a local literal: advance past safety.TRIALTimeout.
	now := time.Now()
	cur := now
	c := NewConsent(func() time.Time { return cur })
	if _, err := c.RequestEnable("p"); err != nil {
		t.Fatal(err)
	}
	cur = now.Add(safety.TRIALTimeout + time.Second)
	if err := c.ConfirmEnable("p", true, true); err == nil {
		t.Fatal("confirm after TRIAL expiry allowed")
	}
	if c.IsEnabled("p") {
		t.Fatal("enabled after expiry")
	}
}

func TestAbortEnable(t *testing.T) {
	c := NewConsent(nil)
	if _, err := c.RequestEnable("p"); err != nil {
		t.Fatal(err)
	}
	if err := c.AbortEnable("p", "user cancel"); err != nil {
		t.Fatalf("abort: %v", err)
	}
	if c.IsEnabled("p") {
		t.Fatal("enabled after abort")
	}
	if err := c.AbortEnable("ghost", "x"); err == nil {
		t.Fatal("abort without trial allowed")
	}
}

func TestStepOrder(t *testing.T) {
	c := NewConsent(nil)
	if _, err := c.RequestEnable("p"); err != nil {
		t.Fatal(err)
	}
	if err := c.AdvanceStep("p", StepSystemSettings); err != nil {
		t.Fatalf("forward: %v", err)
	}
	if err := c.AdvanceStep("p", StepEnable); err == nil {
		t.Fatal("backward step allowed")
	}
	if err := c.AdvanceStep("p", StepVerify); err != nil {
		t.Fatalf("verify: %v", err)
	}
}

func TestReadiness(t *testing.T) {
	c := NewConsent(nil)
	r := c.Readiness()
	if !r["keyboard"] || !r["finder"] {
		t.Fatalf("keyboard/finder ready without consent: %v", r)
	}
	if r["windows"] {
		t.Fatalf("windows ready without AX consent: %v", r)
	}
	c.SetAXReady(true)
	if !c.Readiness()["windows"] {
		t.Fatal("windows not ready after AX consent")
	}
}
