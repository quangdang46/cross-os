// Package winlayout ports Nudge's window-geometry primitives to Go.
//
// Bead: cross-os-nir.1. Plan: §9.10 nudge row, §9.11, §6.2, §3.12.
//
// Provenance (§9.11 ADAPT, direct-reuse YES): mikusnuz/nudge, MIT,
// commit 57d1e6bcfd8acfe489ea84fb35564f49efcd3fef (2026-08-28).
// Source files: Nudge/Core/SnapZone.swift, Nudge/Core/SnapAction.swift,
// Nudge/Core/WindowManager.swift (frame-history pattern), Nudge/Helpers/
// DisplayHelper.swift (multi-monitor pattern). Per-file attribution in
// third_party/nudge/ (LICENSE + NOTICE + ATTRIBUTION.md).
//
// What was taken: geometry math (snap zones from visibleFrame, floor()
// half/third splits), the 19-action vocabulary, frame-history ring
// (capacity 128). What stayed behind: menubar UI, overlay, hotkeys,
// prefs, analytics, AX calls (darwin adapter bead owns those — this
// package is pure geometry, platform-agnostic, fully testable on any OS).
//
// The Adapter (platform/darwin, future bead) applies these frames via
// AXUIElementSetAttributeValue(kAXPosition/kAXSizeAttribute) — SIP-clean
// AX only, never scripting-addition/private-API paths.
package winlayout
