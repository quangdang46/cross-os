// Package e2e owns decision-path semantics + the logging harness.
//
// Bead: cross-os-v27. Plan: COMPREHENSIVE_PLAN.md Data Flow latency budget,
// §3.5b, §3.11, §6.1 matrix.
//
// Scope lock (bead criterion): the full 10-shortcut product matrix is s4i
// scope; window reference matrices are nir.6 scope. THIS package owns path
// semantics (PASS/CONSUME/REPLACE end to end, incl. Terminal-INTERRUPT and
// CLOSE_WINDOW-not-quit) + the logging harness every e2e trace flows
// through — nothing else.
//
// What it wires: keyboard.State → ctx.Cache → approute.Router → event.Router
// → record, asserting Decision + Intent per case with full stage logs, plus
// the sub-1ms decision budget as a recorded benchmark.
//
// Matrix-copy note (review: cross-os-c0): harness rules hand-duplicate the
// s4i Matrix() AND observe S4IFixtures (core must not import plugins) —
// three synced copies of the same matrix. Whoever edits one must update the
// other two; a shared core testmatrix helper (single source consumed by all
// three) is the consolidation follow-up, filed as tech debt, not done here.
package e2e
