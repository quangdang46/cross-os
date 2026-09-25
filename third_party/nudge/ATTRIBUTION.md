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

Source repository: mikusnuz/nudge
Source commit: 57d1e6bcfd8acfe489ea84fb35564f49efcd3fef
Source file: Nudge/UI/PreferencesWindow.swift:53-66 (a permission row whose glyph and whose explanatory text are a function of the state, not one picture for every state)
Original license: MIT
Original copyright: Copyright (c) Nudge contributors
CrossOS destination: app/frontend/src/controls/ChecklistControl.tsx
Modification: structural port, no code copied. The rule that transfers is that a check which has NOT been performed is not a failed check. The reference gives a permission row a glyph and a sentence that both change with the state, because a row drawn the same whether the answer is yes, no or not-yet tells a person to act on an answer nobody gave. CrossOS's readiness list drew "Not checked" for three genuinely different things — the daemon answered and left this id out, the read is still in flight, and the read FAILED — and on the first-run page, which is where a person decides whether they are finished, that told them to wait for an answer that was not coming.
Reason for modification: language and medium port — SwiftUI row states become three separately-worded branches in a React renderer. The reference's poll-until-granted loop (AccessibilityHelper.swift:31-39) is NOT ported: it belongs to a process that can watch the permission itself, and the daemon here derives readiness per read, so the honest port is to say which of the three things happened rather than to fake a poll.
CrossOS license: MIT
