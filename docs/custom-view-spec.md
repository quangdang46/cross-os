# Custom-View Runtime Contract Spec (v1, locked)

Bead: cross-os-jpr.5. Plan: §3.6c (Phase 4+ gate), §9.10 `pi` row.

Status: SPEC ONLY — no implementation in this task. The MVP Registry
rejects `UILocationCustomView` (§3.6c, enforced in `pluginapi.RegisterUI`
with a test). Implementation (if approved) is a separate future bead; this
document is its normative input.

Non-goals (explicit):
- No exposure of the internal React component tree to plugins, ever.
- The declarative tier (settings-page/section/sidebar/command/status) is
  unaffected — custom-view is additive, never a replacement.
- No DOM/window/native UI access for plugin code, ever (§3.6c rule).

## 1. Plugin UI lifecycle

States: `declared → mounted → visible → hidden → unmounted → removed`.
Transitions owned by the shell (mount on navigation, unmount on plugin
disable/uninstall); the plugin is NOTIFIED via lifecycle events (§4), it
never drives mounting itself. Unmount always succeeds (no veto); plugin
state persists per §2.

## 2. UI ↔ plugin state sync

Single source of truth: the plugin's state object, declared as JSON Schema
at RegisterUI time (new `StateSchema` field on UIContribution — additive,
optional in v1). The shell holds a last-known-good copy; sync is
pull-on-render + push-on-change (§5). Schema validation on every sync —
invalid state fails closed (view shows stale-good + error badge, never
unvalidated render).

## 3. UI invoking plugin-owned capabilities

Views invoke ONLY capabilities through the Capability API with the
Permission Manager in the path — identical to every other caller (no
privileged UI channel). The view declares invocable actions in
`UIContribution.Actions`; undeclared invocations are rejected at dispatch.
Core-owned capabilities are requestable (never re-declarable) per the
ownership rule.

## 4. UI → plugin events

Fixed event vocabulary v1: `onMount`, `onUnmount`, `onAction(id, args)`,
`onStateRequest`. No arbitrary event names in v1 (extension point for v2).
Events are async, best-effort, never blocking render. Malformed events are
dropped + logged, never propagated.

## 5. Plugin → UI state push

Push channel: plugin → Core (validated against StateSchema) → shell
re-render. Backpressure: latest-wins with coalescing (a slow view never
accumulates a queue of stale states); depth-1 buffer, drops counted for
diagnostics (same accounting posture as the event Bus: no-drop means
counted, not delivered).

## 6. Component API v1

Closed component set (no custom HTML/JS): `text`, `button`, `checkbox`,
`select`, `slider`, `progress`, `list`, `section`, `tabs`. Each component
has a fixed prop schema versioned with the API (see §8). New components
arrive only via API minor versions, never ad hoc. Props are data
(strings/numbers/bools/action refs) — no callbacks, no functions, no
`eval`-shaped fields.

## 7. Sandbox boundary

Enforcement layers (all three, not one):
1. **Protocol**: views are JSON schema + state + actions. There is no code
   channel — nothing to sandbox at the code level because no plugin code
   reaches the renderer.
2. **Renderer**: the shell renders from schema with a fixed component map.
   Unknown component types fail closed (error badge, not passthrough).
3. **Transport**: if a future out-of-process view host arrives, it speaks
   the same validated schema over IPC (the 090 framing); validation happens
   on receipt, before render.
What the sandbox is NOT: it is not a JS/DOM sandbox (there is no JS), not
an iframe allowlist, not CSP — those concepts do not apply to a
schema-rendered view.

## 8. API versioning (UI API v1/v2…)

- `uiApiVersion` declared per contribution (new field, additive, defaults
  to `"1"`).
- Minor additions (new components, new optional props): v1 renderers
  ignore unknown fields (forward-compatible render).
- Major removals/changes: new major version; shell supports N and N-1
  concurrently; contributions pin their major.
- Lesson from `pi` (§9.10): the UI API is a versioned PUBLIC contract.
  Internal React tree changes never constitute an API break AND never leak
  as API surface.

## 9. Resource limits

Per-view budgets (v1 defaults, tunable per contribution):
- state object ≤ 64 KiB (validated on sync; oversize → error badge);
- push rate ≤ 10/s (coalesced; excess counted, not queued);
- component tree depth ≤ 8, nodes ≤ 200 (rejected at declaration);
- render time budget logged (slow views flagged in diagnostics, never
  blocking input — views never run on the keyboard path).

## 10. Acceptance for the future implementation bead

- All eight §3.6c bullets + limits above are testable assertions, not prose.
- `custom-view` Registry rejection test stays green until the
  implementation bead flips it (with versioning + sandbox tests).
- Declarative-tier tests unaffected (additive proof).

## 11. Open items owned by the implementation bead (review: cross-os-c0)

- (a) Error-badge lifecycle: who clears the badge (next valid sync?
  user dismiss?), does it block interaction, per-view or global scope?
- (b) `onAction(id, args)` validation: args validate against the action's
  capability InputSchema (consistent with every other channel), or §4
  states why not — one line either way.
- (c) N/N-1 eviction: when N-2 drops, pinned contributions refuse-to-render
  with badge vs auto-migrate — state the rule.
