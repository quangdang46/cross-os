// Package approute implements config-driven AppMode routing and device
// filtering.
//
// Bead: cross-os-jd9. Plan: COMPREHENSIVE_PLAN.md §6.1, §3.3.
//
// The Phase-0 ctx.SeedClassifier hardcodes a small built-in table. This
// package is the jd9 seam it plugs into: operator-owned bundle/exe lists
// for terminal/remote/vm/excluded categories plus per-device exclusions,
// changing behavior with config edits and zero code changes.
//
// Semantics consumed by the s4i shortcut matrix:
//
//	terminal → INTERRUPT/PASTE per-intent (native meaning kept);
//	remote/vm → full passthrough (remapping inside guests double-translates);
//	excluded → full passthrough (user opt-out).
//
// Routing decisions surface in Recorder traces as the AppMode cause.
//
// Operator note: config cannot demote a seed-terminal to plain native
// except via Excluded — behaviorally equivalent (both passthrough),
// though the mode label differs in traces. (review: cross-os-c0)
package approute
