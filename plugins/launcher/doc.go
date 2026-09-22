// Package launcher is the CrossOS builtin Launcher plugin — global-hotkey
// app discovery + launch via the app.launch capability.
//
// Bead: cross-os-jpr.4. Plan: COMPREHENSIVE_PLAN.md §10 Phase 4 Plugin 5,
// §9.10 app-launcher row, §9.11, §3.5b, §3.12.
//
// STUDY outcome (decision doc, fixed path: docs/launcher-evaluation.md):
// REIMPLEMENT through the Capability API — no fork. Reasons: (1) the base's
// hotkey path (Carbon RegisterEventHotKey + CGEventTap fallback) is exactly
// what CrossOS's own tap/hook already owns — a fork would double-register
// the pipeline; (2) its discovery (FileManager enumeration + running-apps
// merge) is ~30 lines to re-express against app.launch params; (3) fork
// drags SwiftUI app chrome CrossOS renders declaratively anyway. No code
// copied, so no §9.11 merge gate beyond this STUDY note.
//
// Design: one global-hotkey rule (Rule → Intent → capability, standard
// conflict path, never a side-channel hook) + pure discovery/ranking
// (exact/prefix/substring/fuzzy over name + bundle ID) + launch request
// building (app.launch with target). The Adapter performs the OS launch.
package launcher
