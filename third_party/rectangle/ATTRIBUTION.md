# Attribution — rectangle settings-surface port (bead cross-os-ui-port-m37)

Source repository: ramonwessels/rectangle
Source commit: 12a9bc79f99abeb86297da3d7436b4489f920fa2
Source file: Rectangle/PrefsWindow/SettingsViewController.swift:10,15,60,307-311 (the settings surface: one scrolling form of grouped rows, a closing About block carrying the version, and one control per setting)
Original license: MIT
Original copyright: Copyright (c) 2019-2026 Ryan Hanson

CrossOS destination: `app/frontend/src/App.tsx`, `app/frontend/public/style.css`

Modification: **structural port, no code copied.** The reference is AppKit and
the destination is React, so what transfers is the shape of the settings
surface, not its implementation:

- one scrolling form of grouped rows, each row a label beside its control
  (`SettingsViewController.swift:307-311` — vertical, leading-aligned stack,
  uniform row spacing; the port keeps a single `--row-gap: 5px` scale)
- a closing About block carrying the version plus the newest activity line
  (`SettingsViewController.swift:10,15` — version label and
  check-for-updates row)
- one control per setting, toggled in place (`SettingsViewController.swift:60`)

What is CrossOS's own: the rows are not hardcoded. The Go Host discovers the
pages and their controls (§3.6c) and the renderer draws whatever the Service
serves, so a new settings page is a Go change and never a UI change. The
spacing scale, system-font stack and colour tokens are CrossOS's, chosen to
read as a native macOS pane inside a webview.

No rectangle source is included; the LICENSE is recorded here for the
structural reference only.
Reason for modification: AppKit on macOS becomes React over a declared schema in
a webview. What transfers is the SHAPE of a settings surface — grouped rows,
a label beside each control, a version and an update line in one place — and
what does not is AppKit's itself: its Auto Layout, its NSView hierarchy, its
run loop. CrossOS's rows are not hardcoded at all, which is the one place this
port departs from the reference's structure: the Go Host discovers the pages
and their controls (§3.6c), so a new settings page is a Go change and never a
UI change. §9.11 records rectangle as BEHAVIOR-only — the surface informed the
layout, no code was taken from it.
CrossOS license: MIT

Source repository: ramonwessels/rectangle
Source commit: 12a9bc79f99abeb86297da3d7436b4489f920fa2
Source file: Rectangle/PrefsWindow/SnapAreaViewController.swift
Original license: MIT
Original copyright: Copyright (c) 2019-2026 Ryan Hanson
CrossOS destination: app/frontend/src/controls/ProfileListControl.tsx (the per-capability rollup: one row per named area, each with its own state, none collapsed into the one beside it)
Modification: structural port, no code copied. What transfers is the ZONE LIST's rule: every named area is a row of its own carrying its own state and its own label — one selector per area (:16-24), a label beside its own popup so the area is named in words rather than by its key (:75-86), and each area listed under its own displayName rather than summarised (:269, :279). CrossOS's rows are a profile's capabilities, counted and named by the daemon; the snap geometry, the Defaults keys and the window-action dispatch are not ported.
Reason for modification: AppKit zone controls become rows in a webview card. §9.11 records rectangle as BEHAVIOR-only precedent — zone geometry and defaults inform UX, no code — and this is that: the LIST rule, not the snapping.
CrossOS license: MIT

Source repository: ramonwessels/rectangle
Source commit: 12a9bc79f99abeb86297da3d7436b4489f920fa2
Source file: Rectangle/PrefsWindow/SnapAreaViewController.swift:75-86 (a label in words beside its own control, so a field is never identified by its key alone)
Original license: MIT
Original copyright: Copyright (c) 2019-2026 Ryan Hanson
CrossOS destination: app/frontend/src/controls/ActionSettingsControl.tsx (one row per setting, the label beside the value, with the page's own field name kept reachable in the title), app/frontend/src/controls/GateBadgeControl.tsx (the gate stated in words beside the extension it applies to, never as a colour alone)
Modification: structural port, no code copied. What transfers is the RULE: a setting is named in words next to its value rather than being identified by its key, and a state that matters is said in words rather than only tinted. The field names are the page's own — a page declares its editable and read-only fields in its control schema and this control draws exactly those, so a new field needs no shell change.
Reason for modification: the reference is AppKit on macOS with settings CrossOS does not have (window margins, gaps); CrossOS draws the vocabulary the page declared and reports that the daemon serves no value for it, because no pack manifest is loaded at runtime.
CrossOS license: MIT

Source repository: ramonwessels/rectangle
Source commit: 12a9bc79f99abeb86297da3d7436b4489f920fa2
Source file: Rectangle/PrefsWindow/ShortcutRecordingObserver.swift and Rectangle/ShortcutManager.swift:288-308
Original license: MIT
Original copyright: Copyright (c) 2019-2026 Ryan Hanson
CrossOS destination: app/frontend/src/lib/recording.ts and app/frontend/src/controls/ChordRecorder.tsx
Modification: structural port, no code copied. The rule transfers whole, and it is the one thing Rectangle's recorder does that CrossOS's did not. When the FIRST recorder starts, Rectangle unbinds its own shortcuts; when the LAST one stops it rebinds them. Without that, pressing Ctrl+Shift+C to RECORD that chord also RUNS whatever Ctrl+Shift+C is bound to — the recorder fires the thing it is capturing. CrossOS had the same defect for the same reason: ChordRecorder calls preventDefault, which stops the webview acting on the key, and cannot reach the daemon, which taps the keyboard below it. So recording now stands the machine down in the daemon's own vocabulary — observe mode, which stages each action onto the trace instead of performing it. Two further shapes transfer. ShortcutRecordingObserver exists to make the suspension GLOBAL and IDEMPOTENT: it tracks a SET of recording views and only acts on wasRecording != isRecordingAnyView, so a second recorder opening does not re-suspend and the first one closing does not lift it — a ref count, and ours is module state for the same reason, two recorders on two pages having to share it. And the suspension belongs to the MACHINE rather than the view, which is why the state sits in lib/ and not in controls/ (index.tsx: a control is a function of what the daemon said and never of module-level state). One guard is ours alone: recording never switches observe mode OFF for a person who turned it on themselves, because CrossOS can read the state before touching it.
Reason for modification: language and medium port — a Cocoa key-value observation over MASShortcutView and a notification become a ref count and two bound calls; the reference cannot read whether the person already suspended, because it owns the bindings. §9.11 records rectangle as BEHAVIOR-only; the behaviour is the suspension, and the AppKit mechanism is not ported.
CrossOS license: MIT

Source repository: ramonwessels/rectangle
Source commit: 12a9bc79f99abeb86297da3d7436b4489f920fa2
Source file: Rectangle/PrefsWindow/SnapAreaViewController.swift:18-25 (eight named direction rows) and :158-160 (those rows appear only when a display exists)
Original license: MIT
Original copyright: Copyright (c) 2019-2026 Ryan Hanson
CrossOS destination: app/frontend/src/controls/ZoneEditorControl.tsx (the PLACES table, placeFor, and the per-row place picker)
Modification: structural port, no code copied. What transfers is that a snap area is chosen by NAMING A PLACE — topLeft, top, topRight, left, right, bottomLeft, bottom, bottomRight — rather than by supplying coordinates. CrossOS's editor asked for four floats per zone and started blank, so a person wanting a left half had to work out what 0.5 meant before they could begin. The numbers STAY on the row: this is an addition to the editor, not a replacement of it, and a rectangle that is none of the eight is a legitimate thing to want, so a row that matches no place reports "An exact rectangle" rather than being rounded to the nearest one. Choosing a place fills the geometry and fills an id and name only when they are still blank — naming a zone is the person's, and a picker that renamed their zone every time they resized it would be a control fighting them. The reference's portrait set is not ported: Rectangle shows a second column of the same eight when a portrait display is attached, and CrossOS serves no display geometry at all, so a single set is the honest shape rather than an empty one.
Reason for modification: language and medium port — eight NSButtons each bound to a popup become one select per row. §9.11 records rectangle as BEHAVIOR-only; the behaviour is that the vocabulary is places, not coordinates.
CrossOS license: MIT
