// Package developer is the CrossOS builtin Developer UX plugin —
// §6.4 actions as intent-first rules + capability requests.
//
// Bead: cross-os-jpr.3. Plan: COMPREHENSIVE_PLAN.md §6.4, §3.6 Level B,
// §6.1 AppMode routing, §3.5b conflicts.
//
// Five §6.4 actions:
//
//	Ctrl+Shift+Enter → terminal.openAt (open terminal in current dir).
//	Ctrl+Shift+C    → clipboard.copyPath (copy full file path).
//	Ctrl+Shift+P    → app.open (preferred editor: VSCode or Cursor).
//	F8              → passthrough verified (NO rule claims F8 — the test pins
//	                  that absence, so IDE next-error keeps working).
//	Finder items    → "Open in Terminal / VS Code / Cursor" via the vbl.2
//	                  menu table shapes (declared here as capability rows).
//
// Shell/process execution runs through Level B gating (PermShellExecution +
// explicit user approve) — never a default native capability. Supported
// terminals/IDEs are a CONFIG-DRIVEN list (SupportedTerminals /
// SupportedEditors): adding one is config, not code. Per-app scoping keeps
// IDE shortcuts from colliding with windows-keyboard global remaps — the
// conflict test pins clean resolution (most-specific wins).
package developer
