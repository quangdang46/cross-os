// Spike C — focused app/window query + move/resize primitives (bead cross-os-lla).
//
// Plan: COMPREHENSIVE_PLAN.md Phase 0 Spike C, §3.3 Context Resolver, §4.1/§9.11.
// References:
//   - tmp/research/rectangle/Rectangle/Utilities/AXExtension.swift — AX get/set
//     via AXUIElementCopyAttributeValue / AXUIElementSetAttributeValue with
//     AXValue CGPoint/CGSize wrapping; nil-on-error (never hang).
//   - tmp/research/yabai/src/window_manager.c — CGWindowList enumeration +
//     AX move/resize via kAXPositionAttribute/kAXSizeAttribute.
//   - tmp/research/nudge (see bead cross-os-nir.1): frame calc + snap zones.
//
// Layout mirrors spikes A/B: pure query-result types, validation, geometry,
// and latency math in testable Go; OS plumbing behind build tags; Tier-1 unit
// tests always run, Tier-2 live AX/UIA tests gated for a real desktop with
// accessibility consent.
package spikec
