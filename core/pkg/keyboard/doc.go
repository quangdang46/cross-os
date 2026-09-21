// Package keyboard implements the atomic lock-free keyboard state machine.
//
// Bead: cross-os-wge. Plan: COMPREHENSIVE_PLAN.md Data Flow steps 1-3,
// Phase 0 keyboard-state-machine item, §3.2 EventSource.
//
// The native callback (Spike A CGEventTap / Spike B WH_KEYBOARD_LL) updates
// this state in place, then the fast matcher decides on State + FastContext.
// Stale or racy modifier state means wrong decisions on every chorded
// shortcut — so updates are lock-free (atomic bitmask + atomics) and
// allocation-free on the callback path.
//
// Synthetic self-ignore: the adapter stamps injectTag (dwExtraInfo magic,
// reserved in Spike B) on every CrossOS-injected event and translates it to
// SourceSynthetic upstream. The native callback filters on the tag; the
// router guards on the Source. IsSynthetic(tag) is the shared tag predicate
// defined ONCE here so adapter and callback agree on what "synthetic" means;
// the router's Source check is the second layer, not the same call.
// (review correction: cross-os-c0)
package keyboard
