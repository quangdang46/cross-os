# Attribution — Nudge geometry port (bead cross-os-nir.1)

Source repository: mikusnuz/nudge
Source commit: 57d1e6bcfd8acfe489ea84fb35564f49efcd3fef
Source file: Nudge/Core/SnapZone.swift
Original license: MIT
Original copyright: Copyright (c) 2026 mikusnuz
CrossOS destination: core/pkg/winlayout/layout.go (Frame)
Modification: rewritten in Go; floor half/third splits preserved; remainder-on-right/bottom preserved
Reason for modification: platform-agnostic geometry core (no Cocoa); AX application is the darwin adapter's job
CrossOS license: MIT

Source repository: mikusnuz/nudge
Source commit: 57d1e6bcfd8acfe489ea84fb35564f49efcd3fef
Source file: Nudge/Core/SnapAction.swift
Original license: MIT
Original copyright: Copyright (c) 2026 mikusnuz
CrossOS destination: core/pkg/winlayout/layout.go (Action vocabulary)
Modification: 19-action vocabulary ported; displayName/hotkey UI metadata dropped
Reason for modification: hotkeys/prefs are plugin scope, not geometry scope
CrossOS license: MIT

Source repository: mikusnuz/nudge
Source commit: 57d1e6bcfd8acfe489ea84fb35564f49efcd3fef
Source file: Nudge/Core/WindowManager.swift (frame-history pattern)
Original license: MIT
Original copyright: Copyright (c) 2026 mikusnuz
CrossOS destination: core/pkg/winlayout/layout.go (History)
Modification: ring rewritten in Go (map + order slice, capacity 128 preserved)
Reason for modification: language port; AX move/resize calls NOT ported (darwin adapter bead owns those)
CrossOS license: MIT

Source repository: mikusnuz/nudge
Source commit: 57d1e6bcfd8acfe489ea84fb35564f49efcd3fef
Source file: Nudge/Helpers/DisplayHelper.swift (multi-monitor pattern)
Original license: MIT
Original copyright: Copyright (c) 2026 mikusnuz
CrossOS destination: core/pkg/winlayout/layout.go (MoveToDisplay index arithmetic)
Modification: index arithmetic only; NSScreen enumeration NOT ported (darwin adapter bead owns it)
Reason for modification: platform-agnostic core stays testable on any OS
CrossOS license: MIT

**Why Nudge/Helpers/AccessibilityHelper.swift has no block here** (recorded
under cross-os-xxr, which asked for either a block or a reason). §9.11
enumerates five copyable files and this file is one of them, so a block is
owed if anything was taken from it. Nothing was: the AX move and resize calls
it holds are written directly against the accessibility API in C and Go —
platform/darwin/adapter/ax_bridge.c and platform/darwin/spike_c — not ported
from that Swift helper. The four blocks above are the geometry primitives,
which is what the NOTICE already said this repository informed.

This also settles a disagreement between two tables in the plan. §9.10 listed
six files for nudge and added DragSnapManager.swift; §9.11, which is the locked
gate, lists five. The five-file list is the one that binds, and §9.10 now
matches it.
