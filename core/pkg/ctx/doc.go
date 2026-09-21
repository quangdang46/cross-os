// Package ctx implements the CrossOS context resolver split.
//
// Bead: cross-os-jiz. Plan: COMPREHENSIVE_PLAN.md §3.3, Data Flow steps 2/6.
//
// Fast (hot path): cache-resident fields only — app, window, device. The
// keydown path READS the cache; it never queries AX/UIA. Cache updates
// arrive on app/window/device change events, off the hot path.
//
// Lazy (slow path): selection, cursor, focused element — resolved async and
// ONLY for rules declaring requires on them. Everything else skips the
// lookup entirely.
//
// Full Context (slow path / debugging): event + fast + optional lazy +
// application + window + session. Never constructed on the callback path.
//
// AppMode seed list (Phase 0): a small built-in table classifies well-known
// terminal/remote/VM executables. Config-driven bundle/exe lists are jd9
// scope, not this bead — the Classifier interface is the seam jd9 plugs
// into.
package ctx
