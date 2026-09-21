// Package safety locks the CrossOS safety contract.
//
// Bead: cross-os-2ha. Plan: COMPREHENSIVE_PLAN.md §3.10, §8, §3.7.
//
// Boundary (normative): CrossOS rolls back CrossOS-owned state only — never
// arbitrary OS state (Accessibility grants, Input Monitoring consent,
// foreign login items, CGEvent taps owned by other processes). The
// IntegrationState records what CrossOS created; Rollback removes/restores
// exactly those resources.
//
// TRIALTimeout is defined ONCE here. UI beads (ymh.3, nir.4, jpr.1) and any
// future caller reference this constant — never a second hardcoded value.
//
// Deliberate-shape notes (review: cross-os-c0):
//   - Trial.Abort is callable from any non-disabled state (including ENABLED):
//     the kill-switch path reuses it for enabled trials. The table governs
//     legality; the doc comment on Abort states the crash/timeout/kill-switch
//     callers.
//   - PanicStop stamps time.Now directly (constructor, not a transition); the
//     injected clock discipline applies to Trial only.
//   - Trial is single-owner at the loader (no mutex), like Registry
//     build-once. A shared/concurrent trial manager would need locking.
//   - SafetyEvent.Action is free-form strings in v1; promote to constants
//     when the recorder bead filters on them.
package safety
