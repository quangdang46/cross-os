// Package config implements the CrossOS config manager.
//
// Bead: cross-os-ymh.1. Plan: COMPREHENSIVE_PLAN.md §3.8.
//
// Four levels, lowest to highest precedence:
//  1. System defaults (built-in JSON)
//  2. User config (config.json, overrides defaults)
//  3. Plugin config (plugins/<id>/config.json)
//  4. Session overrides (runtime temporary changes)
//
// Schema validation runs at startup (ValidateAtStartup): a plugin's config
// must satisfy its manifest config_schema, and the merged result must
// satisfy the core schema. Invalid config fails closed — the daemon refuses
// to start with a schema error, never with half-merged guesses.
package config
