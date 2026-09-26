# Attribution — Nudge geometry port (bead cross-os-nir.1)

Source repository: mikusnuz/nudge
Source commit: 57d1e6bcfd8acfe489ea84fb35564f49efcd3fef
Source file: Nudge/Core/SnapZone.swift:4-54 (frame(for:on:) — the 15 geometry cases; :49-52 are the four actions that return nil)
Original license: MIT
Original copyright: Copyright (c) 2026 mikusnuz
CrossOS destination: core/pkg/winlayout/layout.go (Frame)
Modification: rewritten in Go; floor half/third splits preserved; remainder-on-right preserved
Reason for modification: platform-agnostic geometry core (no Cocoa); AX application is the darwin adapter's job
CrossOS license: MIT

Source repository: mikusnuz/nudge
Source commit: 57d1e6bcfd8acfe489ea84fb35564f49efcd3fef
Source file: Nudge/Core/SnapAction.swift:5-10 (the 19 cases); the dropped UI metadata is displayName at :12-34 and defaultHotkey at :36-60
Original license: MIT
Original copyright: Copyright (c) 2026 mikusnuz
CrossOS destination: core/pkg/winlayout/layout.go (Action vocabulary)
Modification: 19-action vocabulary ported; displayName/hotkey UI metadata dropped
Reason for modification: hotkeys/prefs are plugin scope, not geometry scope
CrossOS license: MIT

Source repository: mikusnuz/nudge
Source commit: 57d1e6bcfd8acfe489ea84fb35564f49efcd3fef
Source file: Nudge/Core/WindowManager.swift:8-36 (WindowFrameHistory — frames dict + insertionOrder array, evicting from the front past capacity), and :41 where it is constructed at capacity 128
Original license: MIT
Original copyright: Copyright (c) 2026 mikusnuz
CrossOS destination: core/pkg/winlayout/layout.go (History)
Modification: ring rewritten in Go (map + order slice, capacity 128 preserved)
Reason for modification: language port; AX move/resize calls NOT ported (darwin adapter bead owns those)
CrossOS license: MIT

Source repository: mikusnuz/nudge
Source commit: 57d1e6bcfd8acfe489ea84fb35564f49efcd3fef
Source file: Nudge/Helpers/DisplayHelper.swift:96-115 (bestScreenIndex), :46-53 (sortedScreens), :56-73 (nextScreen/previousScreen, whose doc comments at :55 and :65 both read "No wrap")
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

**Why Nudge/UI/PreferencesWindow.swift has no block here.** A block was
removed from this file on 2026-09-26 that cited `PreferencesWindow.swift:53-66`
as "a permission row whose glyph and whose explanatory text are a function of
the state". The range is real; the claim is not, and it failed in both halves
when the source was opened at the pinned commit.

The glyph is not there. `PreferencesWindow.swift` is 171 lines and contains no
glyph construct of any kind — no `Image(systemName:)`, no `NSImage`, no
checkmark or cross character. What varies with state inside :53-66 is an
`NSButton` **checkbox** (:56, `state = isRequested ? .on : .off`) and its
**toolTip** (:57-59 requires-approval message, :64 the pre-macOS-13 message).
The sentence half is true; the glyph half is a checkbox cited as a glyph.

The rule belongs to a different repository, and the destination says so itself.
`app/frontend/src/controls/ChecklistControl.tsx:35-39` names its own reference:
menumate's `App/UI/OnboardingView.swift:210-213`, where `granted` is a `Bool?`
and the icon is drawn only when it is non-nil. Opened at that commit, :210-213
is `if let granted {` / `Image(systemName: granted ? "checkmark.circle.fill" :
"circle.dashed")` / `.foregroundStyle(granted ? green : label3)` — a real glyph
whose image name and colour are both functions of the state, and which is
absent entirely when the state is unknown. That is the behaviour the removed
block claimed for Nudge, and it is in menumate.

Nothing was taken from this file, so no block is owed for it. Every string
literal in `PreferencesWindow.swift` was diffed against
`ChecklistControl.tsx` — 15 of 15 miss; the "no code copied" half was true and
does not rescue a claim whose cited behaviour is not in the range. It is also
outside what this port is allowed to take: §9.10's nudge row says "take ONLY
geometry+AX primitives. Hotkey/prefs/analytics stay behind", §9.11's list of
copyable files does not name it, and `core/pkg/winlayout/doc.go:13-15` records
the same exclusion. That block also carried `Original copyright: Copyright (c)
Nudge contributors`; the LICENSE at the pinned commit says `Copyright (c) 2026
mikusnuz`, the line the four blocks above already carry.
