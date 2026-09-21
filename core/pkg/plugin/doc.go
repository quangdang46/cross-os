// Package plugin implements the Level B executable-plugin runtime.
//
// Bead: cross-os-nir.3. Plan: COMPREHENSIVE_PLAN.md §3.6, §2 Data Flow,
// §7.2, §10 Phase 2.
//
// Level B plugins run as child processes from
// ~/.crossos/plugins/<id>/{manifest.json, plugin}, speaking the SAME
// JSON-RPC 2.0 protocol as package ipc (framing reused, not a second
// protocol — the 090 contract). Go `.so` via plugin.Open() is REJECTED;
// out-of-process is the only sanctioned native-code extension mechanism.
//
// Fast-path invariant: external plugins NEVER run in the native callback.
// Their rules are compiled into the cached matcher ahead of time (see
// CompiledRules); a 100ms plugin hang must not break typing. The
// no-sync-execution probe pins this: the decision path never blocks on
// plugin IPC.
//
// Crash isolation: a plugin crash never takes down Core — the supervisor
// restarts or disables per policy. Health states (DISABLED/TRIAL/HEALTHY/
// ENABLED) feed the Plugins UI and Phase 4 lifecycle. Only DISABLED and
// HEALTHY are assigned in MVP 1; TRIAL/ENABLED are vocabulary placeholders
// Phase 4 acts on. (review: cross-os-c0)
package plugin
