# Wails v2 vs v3 Spike (bead cross-os-4c6, 1–2 day timebox)

Date: 2026-09-21. Method: docs review (v3.wails.io migration guide, API
reference, beta announcement 2026-08-02, release tracker) — no scaffolding
committed, per the bead's explicit constraint.

## Findings per CrossOS need

| Need | v2 stable | v3 beta (desktop API stable) |
|---|---|---|
| Tray icon + menu | Built-in, single-window-coupled | First-class `SystemTray` object, attach-window, offsets, debounce |
| Settings window | Workaround (second app / hidden hacks) | `app.Window.New()` anytime — main + settings + tools as normal code |
| Multi-window | Single window per app (structural limit) | Core feature: dynamic create/destroy, per-window lifecycle/events |
| Native menu | Global runtime functions, implicit target | Methods on app/window/menu objects; autocomplete-friendly |
| Daemon lifecycle | `wails.Run()` monolith (setup+window+loop fused) | Explicit phases: app create → window create → run; testable separately |
| Autostart/login item | Platform code by hand | Same (no built-in either version) — no differentiator |
| Signing + packaging | Opaque managed build | Visible Taskfile-based build; icon/manifest as inspectable tool commands |
| Go service integration | Context-bound bindings via reflection | Services: standalone structs, DI, static-analysis bindings (comments + param names kept) |

## Status (2026-09)

- v3 is BETA: desktop API stable, teams in production with it, but "test
  thoroughly before deploying". v2 remains the current stable release and
  gets critical fixes. Latest v3 prerelease in tracker: beta.9.
- v3 is a rewrite: migration is a real port (lifecycle, services, direct
  APIs, regenerated bindings), not a version bump.

## Decision: v3 beta — with a v2 fallback constraint

CrossOS needs multi-window structurally (Dashboard + Settings + Activity +
per-plugin pages from §3.6c contributions) and tray-attached windows
(tray popup pattern). On v2 these are workarounds against a single-window
structural limit; on v3 they are normal code. The service model (standalone
structs + asset bundling) also matches the plugin-contribution direction
better than context-bound bindings.

Constraints of this decision:
- Revisit at scaffold time: if v3 is still beta with unstable platform APIs
  at scaffold, start the shell on v2 and port when v3 GAs (the migration
  guide is the supported path; keep the shell thin enough to port).
- Never depend on v3 experimental mobile support or unstable advanced
  window options (marked as such in the API reference).
- Toolchain policy stands: use the current stable release at scaffold time
  unless this decision's multi-window reasoning still holds — it will, the
  structural argument doesn't expire.

## Sources

- https://v3.wails.io/migration/v2-to-v3/ (2026-09-17)
- https://v3.wails.io/blog/wails-v3-beta/ (2026-08-02)
- https://v3.wails.io/features/windows/multiple/
- https://v3.wails.io/features/menus/systray/
- https://v3.wails.io/reference/overview/ (stability labels)
- https://github.com/wailsapp/wails/issues/5844 (release tracker)
