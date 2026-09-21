# Phase 6 Future Backlog (tracked, not implemented)

Bead: cross-os-qhp.3. Plan: §10 Phase 6, §13b D1, §6.1, §2, §3.4/§3.5b.

Rule: no implementation until Phase 5 ships. Each item below names its
locked decision/section and its acceptance gate — a future bead copies the
gate verbatim as pass criteria.

## 1. Virtual HID driver (production remap path)

- Decision D1 (§13b): CGEventTap + WH_KEYBOARD_LL for MVP;
  DriverKit (macOS) / filter driver (Windows) post-MVP. §6.1 names the
  driver as the production option.
- Gate: suppress + replace + recovery proven at driver level (same bar as
  Spikes A/B) before any keyboard-engine work depends on it.

## 2. Linux support (X11 + Wayland adapters)

- Third Adapter behind the same Platform Capability API (§4 pattern) — no
  Core contract changes expected, unscoped until Phase 6.
- Gate: window/input capabilities proven per compositor (X11 + Wayland
  separately — different interception mechanisms, no shared spike).

## 3. AI intent inference (ambiguous input)

- Sits atop the Intent Resolver (§3.4). Hard constraint: ambiguous-input
  handling must NEVER weaken the deterministic fast-path budget (<1ms,
  §2) or deterministic conflict resolution (§3.5b). Inference runs
  slow-path/advisory only; the fast matcher stays rule-compiled.
- Gate: latency budget + determinism proofs required in the proposal
  before any code.

(Gate signal: the Phase 5 distribution bead closing — until then these docs are read-only.)
