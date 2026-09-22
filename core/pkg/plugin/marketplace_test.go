// Marketplace tests — bead cross-os-jpr.1.
//
// Pass criteria mapping:
//  1. Git registry browse/install/enable/disable/uninstall →
//     TestInstallFlow (end-to-end: install → confirm → healthy →
//     disable → uninstall); update + uninstall-guards →
//     TestUpdateNeedsApproval (unapproved update fails, approved
//     re-enters TRIAL, enabled pack uninstall fails closed).
//     (review: cross-os-ed — the old header named a phantom
//     TestUpdateUninstall; coverage lives in these two real tests.)
//  2. Health states tracked + shown → TestHealthStates.
//  3. New installs enter TRIAL (Core constant) + confirm/rollback →
//     TestTrialConfirmRollback (+ TestInstallNeedsApproval for the
//     approval gate).
//  4. apiVersion gate rejects with clear UX → TestApiVersionGate.
//  5. Updates need approval → TestUpdateNeedsApproval.
package plugin

import (
	"strings"
	"testing"
	"time"

	"crossos/core/pkg/pluginapi"
)

func testManifest(id, minCore string) pluginapi.Manifest {
	return pluginapi.Manifest{
		ID: id, Name: id, Version: "1.0.0", Entry: "builtin", Type: pluginapi.PluginBuiltin,
		APIVersion: pluginapi.APIVersion{ManifestVersion: "1", PluginAPIVersion: "1", CapabilityAPIVersion: "1", MinCoreVersion: minCore},
	}
}

var approve = Approval{Granted: true, By: "test-user"}
var deny = Approval{Granted: false}

func TestInstallFlow(t *testing.T) {
	r := NewRegistry("test")
	now := time.Now()
	p, err := r.InstallManifest(testManifest("pack-a", "0.1.0"), "https://git.example/a", "v1", "0.2.0", time.Minute, approve, now)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if p.Health != PackTrial || p.State != pluginapi.LifecycleTrial {
		t.Fatalf("new install health=%s state=%s, want TRIAL/trial", p.Health, p.State)
	}
	// Browse: registry lists installed.
	if len(r.Packs) != 1 {
		t.Fatal("registry must list the installed pack")
	}
	// Enable/disable through the table.
	if err := p.ConfirmTrial(now.Add(30 * time.Second)); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if p.Health != PackEnabled {
		t.Fatalf("health=%s after confirm, want ENABLED", p.Health)
	}
	p.MarkHealthy()
	if p.Health != PackHealthy {
		t.Fatalf("health=%s after probe, want HEALTHY", p.Health)
	}
	if err := p.SetEnabled(false); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if err := r.Uninstall("pack-a", approve); err != nil {
		t.Fatalf("uninstall disabled: %v", err)
	}
	if len(r.Packs) != 0 {
		t.Fatal("registry must drop uninstalled pack")
	}
}

func TestInstallNeedsApproval(t *testing.T) {
	r := NewRegistry("test")
	if _, err := r.InstallManifest(testManifest("x", "0.1.0"), "u", "v1", "0.2.0", time.Minute, deny, time.Now()); err == nil {
		t.Fatal("unapproved install must fail closed")
	}
}

func TestTrialConfirmRollback(t *testing.T) {
	r := NewRegistry("test")
	now := time.Now()
	p, err := r.InstallManifest(testManifest("t", "0.1.0"), "u", "v1", "0.2.0", time.Minute, approve, now)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	// TrialUntil honors the passed timeout (callers pass safety.TRIALTimeout;
	// tests pass a short value — never a literal in product code).
	if p.TrialUntil.Sub(now) != time.Minute {
		t.Fatalf("TrialUntil=%v, want passed timeout", p.TrialUntil.Sub(now))
	}
	// Late confirm → auto-rollback + clear error.
	if err := p.ConfirmTrial(now.Add(2 * time.Minute)); err == nil {
		t.Fatal("late confirm must fail with auto-rollback")
	}
	if p.Health != PackDisabled {
		t.Fatalf("health=%s after lapse, want DISABLED", p.Health)
	}
	// Explicit rollback path.
	p2, _ := r.InstallManifest(testManifest("t2", "0.1.0"), "u", "v1", "0.2.0", time.Minute, approve, now)
	if err := p2.RollbackTrial(); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	// Health line names the Safety surface while in trial.
	p3, _ := r.InstallManifest(testManifest("t3", "0.1.0"), "u", "v1", "0.2.0", time.Minute, approve, now)
	if line := p3.HealthLine(now); !strings.Contains(line, "Safety page") {
		t.Fatalf("health line %q must point at the Safety surface", line)
	}
}

func TestApiVersionGate(t *testing.T) {
	r := NewRegistry("test")
	// Pack needs newer core → rejected with clear UX.
	_, err := r.InstallManifest(testManifest("new", "9.9.9"), "u", "v1", "0.2.0", time.Minute, approve, time.Now())
	if err == nil || !strings.Contains(err.Error(), "incompatible") {
		t.Fatalf("want incompatible-pack error, got %v", err)
	}
}

func TestUpdateNeedsApproval(t *testing.T) {
	r := NewRegistry("test")
	now := time.Now()
	p, err := r.InstallManifest(testManifest("u", "0.1.0"), "repo", "v1", "0.2.0", time.Minute, approve, now)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if err := p.ConfirmTrial(now); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if err := p.Update("v2", testManifest("u", "0.1.0"), "0.2.0", time.Minute, deny, now); err == nil {
		t.Fatal("unapproved update must fail (no silent auto-update)")
	}
	if err := p.Update("v2", testManifest("u", "0.1.0"), "0.2.0", time.Minute, approve, now); err != nil {
		t.Fatalf("approved update: %v", err)
	}
	if p.Ref != "v2" || p.Health != PackTrial {
		t.Fatalf("after update ref=%s health=%s, want v2/TRIAL", p.Ref, p.Health)
	}
	// Enabled pack must disable first — uninstall while enabled fails.
	p2, _ := r.InstallManifest(testManifest("u2", "0.1.0"), "repo", "v1", "0.2.0", time.Minute, approve, now)
	_ = p2.ConfirmTrial(now)
	if err := r.Uninstall("u2", approve); err == nil {
		t.Fatal("uninstall of enabled pack must fail (disable first)")
	}
}

func TestHealthStates(t *testing.T) {
	// All four states reachable: TRIAL (install), ENABLED (confirm),
	// HEALTHY (probe), DISABLED (disable/rollback).
	r := NewRegistry("test")
	now := time.Now()
	p, err := r.InstallManifest(testManifest("h", "0.1.0"), "u", "v1", "0.2.0", time.Minute, approve, now)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if p.Health != PackTrial {
		t.Fatalf("health=%s, want TRIAL after install", p.Health)
	}
	if err := p.ConfirmTrial(now); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if p.Health != PackEnabled {
		t.Fatalf("health=%s, want ENABLED after confirm", p.Health)
	}
	p.MarkHealthy()
	if p.Health != PackHealthy {
		t.Fatalf("health=%s, want HEALTHY after probe", p.Health)
	}
	if err := p.SetEnabled(false); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if p.Health != PackDisabled && p.State != pluginapi.LifecycleDisabled {
		t.Fatalf("health=%s state=%s, want DISABLED/disabled", p.Health, p.State)
	}
	// MarkHealthy on a non-enabled pack is a no-op (never fake HEALTHY).
	p.MarkHealthy()
	if p.Health == PackHealthy {
		t.Fatal("disabled pack must not become HEALTHY on probe")
	}
}
