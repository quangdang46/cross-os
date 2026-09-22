// Pack marketplace + install/update lifecycle with health states
// (bead cross-os-jpr.1).
//
// Plan: COMPREHENSIVE_PLAN.md §10 Phase 4 (local → git → signed registry →
// marketplace), §7.2 Plugins page ("NO marketplace browser until Phase 4" —
// this task IS that gate opening), §8 safety, §3.6 manifest apiVersion,
// §14 Q4 (Git-based auto-update with user approval). Builds on the Level B
// runtime (nir.3) and pack loading (vbl.3).
//
// Model: a Registry is a named set of git-hosted packs (repo URL + ref).
// Install/update NEVER run silently: every mutation needs an explicit
// Approval token from the user (§8.4). New installs enter TRIAL (Core
// constant safety.TRIALTimeout, never a literal) with confirm/rollback on
// the ymh.3 Safety surface. Health states DISABLED/TRIAL/HEALTHY/ENABLED
// (§10 Phase 4) ride the pluginapi lifecycle table. apiVersion gating
// (APIVersion.CompatibleWith) rejects incompatible packs with clear UX.
package plugin

import (
	"fmt"
	"strings"
	"time"

	"crossos/core/pkg/pluginapi"
)

// PackHealth is the §10 Phase 4 health vocabulary. It maps onto the
// pluginapi lifecycle table (Disabled→Trial→Enabled + Paused), with HEALTHY
// as the steady-state display for an enabled pack whose last probe passed.
type PackHealth string

const (
	PackDisabled PackHealth = "DISABLED"
	PackTrial    PackHealth = "TRIAL"
	PackHealthy  PackHealth = "HEALTHY"
	PackEnabled  PackHealth = "ENABLED"
)

// InstalledPack is one pack under lifecycle management.
type InstalledPack struct {
	ID         string
	Repo       string // git repo URL
	Ref        string // pinned ref (tag/commit)
	Manifest   PackManifest
	Health     PackHealth
	State      pluginapi.LifecycleState
	Installed  time.Time
	TrialUntil time.Time // set on install; confirm before it lapses
	CoreVer    string    // running core version (apiVersion gate)
}

// Approval is the explicit user-approve token (§8.4). Mutations without a
// granted approval fail closed — no silent updates, ever.
type Approval struct {
	Granted bool
	By      string // user id / "local-operator"
}

// Registry is a named git-pack source.
type Registry struct {
	Name  string
	Packs map[string]*InstalledPack // id → installed
}

// NewRegistry returns an empty registry.
func NewRegistry(name string) *Registry {
	return &Registry{Name: name, Packs: map[string]*InstalledPack{}}
}

// Install adds an UNVERSIONED pack from a parsed PackManifest. PackManifest
// (vbl.3) carries no apiVersion/minCore field, so this path CANNOT version-
// gate — local/dev only. Production MUST use InstallManifest (fail-closed
// apiVersion gate). Unversioned installs are marked by recording CoreVer as
// "unversioned" in the pack, surfaced in HealthLine for audit.
//
// Fail-closed (review: cross-os-ed): localDev MUST be true or Install
// refuses — a doc comment alone cannot stop a future caller sleepwalking
// into the permissive path. There are zero production callers today; the
// first production caller uses InstallManifest (and passes
// safety.TRIALTimeout — see the TODO there).
func (r *Registry) Install(id, repo, ref string, m PackManifest, trialTimeout time.Duration, localDev bool, ap Approval, now time.Time) (*InstalledPack, error) {
	if !localDev {
		return nil, fmt.Errorf("marketplace: Install is local/dev-only (PackManifest carries no apiVersion); production uses InstallManifest")
	}
	if !ap.Granted {
		return nil, fmt.Errorf("marketplace: install %q needs user approval (no silent installs)", id)
	}
	if _, dup := r.Packs[id]; dup {
		return nil, fmt.Errorf("marketplace: pack %q already installed", id)
	}
	if errs := m.Validate(); len(errs) != 0 {
		return nil, fmt.Errorf("marketplace: manifest invalid: %v", errs[0])
	}
	p := &InstalledPack{ID: id, Repo: repo, Ref: ref, Manifest: m, CoreVer: "unversioned"}
	p.State = pluginapi.LifecycleDisabled
	if err := p.advance(pluginapi.LifecycleTrial); err != nil {
		return nil, err
	}
	p.Health = PackTrial
	p.Installed = now
	p.TrialUntil = now.Add(trialTimeout)
	r.Packs[id] = p
	return p, nil
}

// InstallManifest is the full install path with a real pluginapi.Manifest
// (apiVersion gate enforced for real). Preferred over Install when the
// caller has the manifest (always, in production).
// TODO(jpr.1): the first production caller MUST pass safety.TRIALTimeout
// (not a literal) — no callers exist yet, so nothing wires it today.
func (r *Registry) InstallManifest(pm pluginapi.Manifest, repo, ref, coreVer string, trialTimeout time.Duration, ap Approval, now time.Time) (*InstalledPack, error) {
	if !ap.Granted {
		return nil, fmt.Errorf("marketplace: install %q needs user approval (no silent installs)", pm.ID)
	}
	if _, dup := r.Packs[pm.ID]; dup {
		return nil, fmt.Errorf("marketplace: pack %q already installed", pm.ID)
	}
	if err := pm.APIVersion.CompatibleWith(coreVer); err != nil {
		return nil, fmt.Errorf("marketplace: pack %q incompatible: %w (clear UX: update pack or core)", pm.ID, err)
	}
	p := &InstalledPack{ID: pm.ID, Repo: repo, Ref: ref, CoreVer: coreVer}
	p.State = pluginapi.LifecycleDisabled
	if err := p.advance(pluginapi.LifecycleTrial); err != nil {
		return nil, err
	}
	p.Health = PackTrial
	p.Installed = now
	p.TrialUntil = now.Add(trialTimeout)
	r.Packs[pm.ID] = p
	return p, nil
}

func (p *InstalledPack) advance(to pluginapi.LifecycleState) error {
	if err := pluginapi.Transition(p.State, to); err != nil {
		return err
	}
	p.State = to
	return nil
}

// ConfirmTrial moves TRIAL → ENABLED (user confirmed on the Safety surface);
// rollback moves TRIAL → DISABLED. Late confirm (past TrialUntil) fails —
// the pack already auto-rolled-back.
func (p *InstalledPack) ConfirmTrial(now time.Time) error {
	if p.State != pluginapi.LifecycleTrial {
		return fmt.Errorf("marketplace: pack %q not in trial", p.ID)
	}
	if now.After(p.TrialUntil) {
		_ = p.advance(pluginapi.LifecycleDisabled)
		p.Health = PackDisabled
		return fmt.Errorf("marketplace: trial lapsed — auto-rolled-back, re-install to retry")
	}
	if err := p.advance(pluginapi.LifecycleEnabled); err != nil {
		return err
	}
	p.Health = PackEnabled
	return nil
}

// RollbackTrial moves TRIAL → DISABLED explicitly.
func (p *InstalledPack) RollbackTrial() error {
	if err := p.advance(pluginapi.LifecycleDisabled); err != nil {
		return err
	}
	p.Health = PackDisabled
	return nil
}

// MarkHealthy records a passing probe on an enabled pack (HEALTHY display).
func (p *InstalledPack) MarkHealthy() {
	if p.State == pluginapi.LifecycleEnabled {
		p.Health = PackHealthy
	}
}

// Update re-pins the ref. Requires approval (no silent auto-update, §14 Q4)
// and re-runs the apiVersion gate; the pack re-enters TRIAL.
func (p *InstalledPack) Update(newRef string, pm pluginapi.Manifest, coreVer string, trialTimeout time.Duration, ap Approval, now time.Time) error {
	if !ap.Granted {
		return fmt.Errorf("marketplace: update %q needs user approval (no silent updates)", p.ID)
	}
	if err := pm.APIVersion.CompatibleWith(coreVer); err != nil {
		return fmt.Errorf("marketplace: update %q incompatible: %w", p.ID, err)
	}
	p.Ref = newRef
	// Re-enter trial through the table: current → DISABLED → TRIAL.
	// Every hop uses advance (never a direct assignment) so the
	// normative table validates each move.
	if p.State != pluginapi.LifecycleDisabled {
		if err := p.advance(pluginapi.LifecycleDisabled); err != nil {
			return err
		}
	}
	if err := p.advance(pluginapi.LifecycleTrial); err != nil {
		return err
	}
	p.Health = PackTrial
	p.TrialUntil = now.Add(trialTimeout)
	return nil
}

// SetEnabled toggles ENABLED ↔ DISABLED through the table (explicit user
// action; the Plugins page calls this).
func (p *InstalledPack) SetEnabled(enabled bool) error {
	if enabled {
		if p.State == pluginapi.LifecycleEnabled {
			return nil
		}
		if err := p.advance(pluginapi.LifecycleEnabled); err != nil {
			return err
		}
		p.Health = PackEnabled
		return nil
	}
	if err := p.advance(pluginapi.LifecycleDisabled); err != nil {
		return err
	}
	p.Health = PackDisabled
	return nil
}

// Uninstall removes the pack (approval required). Only DISABLED packs
// uninstall — enabled/trial packs must be disabled first (explicit, never
// surprising).
func (r *Registry) Uninstall(id string, ap Approval) error {
	if !ap.Granted {
		return fmt.Errorf("marketplace: uninstall %q needs user approval", id)
	}
	p, ok := r.Packs[id]
	if !ok {
		return fmt.Errorf("marketplace: pack %q not installed", id)
	}
	if p.State != pluginapi.LifecycleDisabled {
		return fmt.Errorf("marketplace: disable pack %q before uninstall", id)
	}
	delete(r.Packs, id)
	return nil
}

// HealthLine renders one Plugins-UI row (id, health, state, trial note).
// Unversioned (local/dev) installs are marked so audit sees the gap.
func (p *InstalledPack) HealthLine(now time.Time) string {
	trial := ""
	if p.State == pluginapi.LifecycleTrial {
		left := p.TrialUntil.Sub(now).Round(time.Second)
		if left < 0 {
			left = 0
		}
		trial = " (trial " + left.String() + " left — confirm on Safety page)"
	}
	row := strings.Join([]string{string(p.Health), string(p.State), p.Ref}, " ") + trial
	if p.CoreVer == "unversioned" {
		row += " [unversioned local install]"
	}
	return row
}
