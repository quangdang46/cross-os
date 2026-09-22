// Permission prompts + enable flow backend (bead cross-os-nir.4).
//
// Plan §8.1–8.4 + §1 non-tech-first: NOTHING is enabled until the user
// explicitly clicks Enable per plugin; window management needs
// Accessibility (AX) consent (§8.3). This file owns the backend half of
// that flow: per-plugin enable state (explicit only, never auto), TRIAL
// attach (safety.Trial semantics, Core TRIALTimeout constant — never a
// local literal), consent-step tracking (Welcome → Enable → Open System
// Settings → Verify ready), and readiness verification the Activity page
// renders. The ActivityPage/OnboardingFlow schemas declare the UI; this
// file decides.
//
// No duplicate TRIAL UI: confirm/rollback surface lives on the ymh.3
// Safety page; here we only link (trialLink) and enforce (Confirm).
package shell

import (
	"fmt"
	"sync"
	"time"

	"crossos/core/pkg/safety"
)

// ConsentStep is one step of the enable flow.
type ConsentStep int

const (
	StepWelcome ConsentStep = iota
	StepEnable
	StepSystemSettings
	StepVerify
)

// EnableState tracks one plugin's explicit-enable journey. Enabled flips
// ONLY via SetEnabled(confirmed=true) — there is no auto path, matching
// safety.Trial.Confirm semantics.
type EnableState struct {
	PluginID  string
	Enabled   bool
	Step      ConsentStep
	Trial     *safety.Trial
	Confirmed bool
}

// Consent tracks enable flow + AX consent readiness across plugins.
type Consent struct {
	mu      sync.Mutex
	plugins map[string]*EnableState
	axReady bool // accessibility consent verified (adapter probe)
	now     func() time.Time
}

// NewConsent returns an empty tracker (test seam: now injectable).
func NewConsent(now func() time.Time) *Consent {
	if now == nil {
		now = time.Now
	}
	return &Consent{plugins: map[string]*EnableState{}, now: now}
}

// RequestEnable starts the flow for one plugin: DISABLED → TRIAL. Calling
// twice is idempotent (returns the in-flight trial, never a second one).
func (c *Consent) RequestEnable(pluginID string) (*EnableState, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if st, ok := c.plugins[pluginID]; ok && st.Trial != nil {
		return st, nil
	}
	tr, err := safety.BeginTrial(pluginID, c.now)
	if err != nil {
		return nil, err
	}
	st := &EnableState{PluginID: pluginID, Step: StepEnable, Trial: tr}
	c.plugins[pluginID] = st
	return st, nil
}

// ConfirmEnable completes TRIAL → ENABLED. confirmed=false or
// healthy=false fails closed (mirrors safety.Trial.Confirm — no auto path).
func (c *Consent) ConfirmEnable(pluginID string, confirmed, healthy bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	st, ok := c.plugins[pluginID]
	if !ok || st.Trial == nil {
		return fmt.Errorf("shell: no trial for plugin %q (request enable first)", pluginID)
	}
	if err := st.Trial.Confirm(confirmed, healthy); err != nil {
		return err
	}
	st.Enabled = true
	st.Confirmed = confirmed
	st.Step = StepVerify
	return nil
}

// AbortEnable rolls TRIAL → DISABLED (user cancel, timeout, kill-switch).
func (c *Consent) AbortEnable(pluginID, reason string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	st, ok := c.plugins[pluginID]
	if !ok || st.Trial == nil {
		return fmt.Errorf("shell: no trial for plugin %q", pluginID)
	}
	if err := st.Trial.Abort(reason); err != nil {
		return err
	}
	st.Enabled = false
	return nil
}

// AdvanceStep moves the consent flow forward (Welcome → Enable → System
// Settings → Verify). Steps only advance — the flow never skips Verify.
func (c *Consent) AdvanceStep(pluginID string, to ConsentStep) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	st, ok := c.plugins[pluginID]
	if !ok {
		return fmt.Errorf("shell: no flow for plugin %q", pluginID)
	}
	if to < st.Step {
		return fmt.Errorf("shell: consent steps never go backward (%d → %d)", st.Step, to)
	}
	st.Step = to
	return nil
}

// SetAXReady records verified Accessibility consent (adapter probe result).
func (c *Consent) SetAXReady(ready bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.axReady = ready
}

// Readiness reports per-area readiness for the checklist
// (source core:readiness: keyboard, windows, finder).
func (c *Consent) Readiness() map[string]bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := map[string]bool{"keyboard": true, "windows": c.axReady, "finder": true}
	return out
}

// IsEnabled reports explicit-enable state (never auto-true).
func (c *Consent) IsEnabled(pluginID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	st, ok := c.plugins[pluginID]
	return ok && st.Enabled
}
