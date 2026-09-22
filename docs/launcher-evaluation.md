# Launcher evaluation: lgrammel/app-launcher (bead cross-os-jpr.4)

Source: `tmp/research/app-launcher` (commit `41f663e`), MIT
(Copyright (c) 2026 Lars Grammel). STUDY mode per §9.10 row.

## Hotkey approach

Resilient dual path (`Sources/AppLauncher/GlobalHotKeyRegistrar.swift`):
Carbon `RegisterEventHotKey` as the always-on fallback + a CGEventTap-based
`AccessibilityGlobalHotKeyRegistrar` preferred path, with 0.25s duplicate
dedup. Verdict: CrossOS does NOT need either — the Core tap/hook already
owns the input pipeline (spikes A/B). The launcher hotkey is one standard
Rule (`HotkeyRule`, Ctrl+Space → CONSUME) with conflict resolution, never a
second registration. Adopting the base's registrar would double-register
every keystroke path.

## Discovery approach

`AppScanner.swift`: FileManager enumeration of `/Applications`,
`/System/Applications`, `~/Applications` (skips hidden + package
descendants) merged with running regular apps, deduped by bundle ID,
sorted case-insensitively. Clean, ~40 lines of logic. Verdict: re-express
the SHAPE (name + bundle ID + path; scan + merge + dedupe) against the
`app.launch` capability params. Nothing OS-specific beyond what the
Adapter already does.

## License + transitive deps

MIT, no conflicting transitive deps for the logic evaluated (SwiftUI app
chrome is not reused). A fork would still pass the §9.11 gate — but fork
loses on engineering grounds below, so no ATTRIBUTION entry is needed
(no code copied).

## Decision: REIMPLEMENT (no fork)

1. Hotkey: reimplement as a standard rule (conflict-visible, no side channel).
2. Discovery: reimplement shape against `app.launch` (name/bundleID/path).
3. Chrome: CrossOS renders the palette declaratively (§3.6c); the base's
   SwiftUI panel, calculator, settings shortcuts stay behind.
