// Package daemon implements the CrossOS core daemon lifecycle.
//
// Bead: cross-os-ymh.2. Plan: COMPREHENSIVE_PLAN.md §3.1, §3.10.
//
// Mechanism only: init/run/stop/safe-mode entry/exit as a state machine
// over pluginapi.LifecycleState. TRIAL/rollback SEMANTICS (confirm,
// expiry, ownership) live in package safety — the daemon calls into it,
// never reimplements it.
//
// Transition ownership (deliberate, review cross-os-c0): plugin-visible
// moves go through pluginapi.Transition (single chokepoint). Two
// daemon-internal steps bypass it — Resume's enabled→running and Stop's
// enabled→stopped — because the plugin table has no such edges (they are
// daemon liveness steps, not plugin enable moves). Both are recorded with
// full From/At like table moves. Do not "fix" either side without the
// other: adding the edges to the table would legalize them for plugins,
// which must never self-transition those paths.
package daemon
