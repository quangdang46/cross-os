// Package observe productizes keyboard-trace record + replay + observe mode
// for MVP 0.
//
// Bead: cross-os-z3v. Plan: COMPREHENSIVE_PLAN.md §3.11, Phase 1 recorder.
//
// Division of labor with package record (ae7):
//
//	record = the mechanism (modes, redaction, storage, tier replay, dry-run
//	marking). This package = the MVP 0 product loop on top: fixtures drawn
//	from the s4i matrix, mismatch reproduction (reported vs replayed), and
//	the observe-mode gate (Would-execute log + explicit user approval before
//	any execution).
//
// Nothing here touches live input or executes capabilities. Replay is
// deterministic re-description of stored traces; observe mode logs intent
// without dispatch. Execution gating lives with the dispatcher (z3v owns
// the approval record, not the enforcement).
package observe
