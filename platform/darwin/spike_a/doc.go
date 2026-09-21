// Spike A — macOS CGEventTap intercept harness (bead cross-os-vjz).
//
// Plan: COMPREHENSIVE_PLAN.md Phase 0 Spike A, Data Flow fast path steps 1-4.
// Reference: tmp/research/Karabiner-Elements
//
//	src/share/monitor/event_tap_monitor.hpp — tap at kCGHIDEventTap with
//	head-insert, timeout-disable recovery loop (re-enable, bounded, escalate
//	to health signal on sustained disable, never silent death).
//
// Layout mirrors spike B (platform/windows/spike_b): pure decision + recovery
// logic in testable Go, OS plumbing behind build tags, Tier-1 unit tests
// always run, Tier-2 live-tap tests gated for a real interactive desktop.
package spikea
