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
