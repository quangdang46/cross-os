// Package pluginapi locks the CrossOS Plugin API v1 contract.
//
// Bead: cross-os-4lm. Plan: COMPREHENSIVE_PLAN.md §3.6, §3.6c, §3.10, §3.12.
//
// Principle: plugins REGISTER behavior; Core owns the pipeline. A plugin never
// sits in the event path — no per-event callbacks on the fast path. OnEvent
// and friends are async slow-path observers, never decision hooks.
//
// Go `.so` via plugin.Open() is explicitly REJECTED (OS/version/symbol
// fragility, undeployable cross-platform). MVP 0 ships builtin + declarative
// only; Level B executable plugins arrive in MVP 1.
//
// Deliberate-shape notes (review: cross-os-c0):
//   - Registry is a concrete struct, not an interface: dependents
//     (cross-os-z9v EventRouter, cross-os-80g UI host) couple to one
//     normative implementation for MVP. An interface comes only if a second
//     implementation is ever needed.
//   - Permissions in Manifest are []string, not []Permission: the Permission
//     type lives with the permission-manager contract; the mapping
//     string↔Permission is owned there, not here.
//   - Namespacing ("<pluginID>.<name>") is conventional in MVP, enforced by
//     the loader bead (manifest ID is only known at load): Registry rejects
//     core collisions, the loader must reject unnamespaced plugin IDs.
//     TODO(loader): validate the "<pluginID>." prefix on Register*.
//   - Registration failures share OwnershipError; a finer RegistrationError
//     taxonomy comes with the loader (it owns user-facing diagnostics).
package pluginapi
