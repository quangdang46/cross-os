// Package record implements the CrossOS event recorder, replay, and
// observe (dry-run) mode.
//
// Bead: cross-os-ae7. Plan: COMPREHENSIVE_PLAN.md §3.11.
//
// The recorder taps every pipeline stage (event → context → rule → intent →
// action → result) as DATA. It consumes event.Outcome values published on
// the Bus AFTER the callback returns — it never runs on the decision path,
// so recording cannot slow a keystroke.
//
// Privacy model (normative): OFF / METADATA_ONLY (default) / DEBUG /
// FULL_TRACE. METADATA_ONLY redacts typed text, clipboard content, and
// password fields at record time — redaction is load-bearing, not cosmetic:
// replay replays semantic events (KEY_DOWN Ctrl), never raw secrets.
//
// Replay tiers: (1) Raw — key down/up sequences; (2) Semantic — intents;
// (3) Capability — capability invocations. Tiers 2/3 are the primary debug
// loop (reproduce → fix → replay → verify).
//
// Observe (dry-run) mode: the router resolves Physical → Context → Rule →
// Intent → Would-execute WITHOUT executing. Users validate new plugins
// before CrossOS modifies anything.
package record
