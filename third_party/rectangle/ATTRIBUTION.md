# Attribution — rectangle settings-surface port (bead cross-os-ui-port-m37)

Source repository: rxhanson/Rectangle
Source commit: 12a9bc79f99abeb86297da3d7436b4489f920fa2
Source file: Rectangle/Base.lproj/Main.storyboard:2685-3457 (the settings surface: one fixed form of grouped rows, with the version label and the check-for-updates row at its HEAD, and one outlet per setting) and Rectangle/PrefsWindow/SettingsViewController.swift:9-39 (those outlets — all 27 @IBOutlet declarations the class makes, which run to :39 and not :32), :57-61 (the per-setting action), :1177 and :1230 (the two lines that set the version string and the update title)
Original license: MIT
Original copyright: Copyright (c) 2019-2026 Ryan Hanson; based on the Spectacle
app, Copyright (c) 2017 Eric Czarny (the second line is the reference's own
LICENSE, at the pinned commit, and MIT requires it travel with any copy)

CrossOS destination: `app/frontend/src/App.tsx`, `app/frontend/public/style.css`

Modification: **structural port, no code copied.** The reference is AppKit and
the destination is React, so what transfers is the shape of the settings
surface, not its implementation:

- one fixed form of grouped rows, each row a control with its label beside it
  (`Main.storyboard:2693` — the content is a vertical, leading-aligned
  `NSStackView`, `spacing="10"`; `:2696` and `:2751` are two of its seven
  horizontal rows (`:2696`, `:2751`, `:2790`, `:2831`, `:2848`, `:2942`,
  `:3262`; `:2831` is `hidden="YES"`). The rows are not uniform: `:2696` pairs a
  check button with the version text field, while `:2751` pairs the
  check-for-updates checkbox with the Check for Updates… push button and no text
  field)
- the version and the update affordance at the HEAD of the form, not a closing
  About block (`Main.storyboard:2696` is the first arranged row and carries the
  version text field; `:2751` carries check-for-updates-automatically beside
  the Check for Updates… button. `SettingsViewController.swift:1177` writes the
  version string and `:1230` the button title)
- one control per setting, toggled in place
  (`SettingsViewController.swift:57-61` — the whole action: read the control,
  write Defaults. The checkbox itself is the outlet at `:9`)

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

Source repository: rxhanson/Rectangle
Source commit: 12a9bc79f99abeb86297da3d7436b4489f920fa2
Source file: Rectangle/PrefsWindow/ShortcutRecordingObserver.swift:6-64 (the class: `recordingViews` is a `Set<ObjectIdentifier>` at :10, and `recordingChanged` at :50-62 returns early unless `wasRecording != isRecordingAnyView`) and Rectangle/ShortcutManager.swift:289-309 (`shortcutRecordingChanged` — `unbindShortcuts()` at :295, `bindShortcuts()` at :300)
Original license: MIT
Original copyright: Copyright (c) 2019-2026 Ryan Hanson; based on the Spectacle
app, Copyright (c) 2017 Eric Czarny (the second line is the reference's own
LICENSE, at the pinned commit, and MIT requires it travel with any copy)
CrossOS destination: app/frontend/src/lib/recording.ts and app/frontend/src/controls/ChordRecorder.tsx
Modification: structural port, no code copied. The rule transfers whole, and it is the one thing Rectangle's recorder does that CrossOS's did not. When the FIRST recorder starts, Rectangle unbinds its own shortcuts; when the LAST one stops it rebinds them. Without that, pressing Ctrl+Shift+C to RECORD that chord also RUNS whatever Ctrl+Shift+C is bound to — the recorder fires the thing it is capturing. CrossOS had the same defect for the same reason: ChordRecorder calls preventDefault, which stops the webview acting on the key, and cannot reach the daemon, which taps the keyboard below it. So recording now stands the machine down in the daemon's own vocabulary — observe mode, which stages each action onto the trace instead of performing it. Two further shapes transfer. ShortcutRecordingObserver exists to make the suspension GLOBAL and IDEMPOTENT: it tracks a SET of recording views and only acts on wasRecording != isRecordingAnyView, so a second recorder opening does not re-suspend and the first one closing does not lift it — a ref count, and ours is module state for the same reason, two recorders on two pages having to share it. And the suspension belongs to the MACHINE rather than the view, which is why the state sits in lib/ and not in controls/ (index.tsx: a control is a function of what the daemon said and never of module-level state). One guard is ours alone: recording never switches observe mode OFF for a person who turned it on themselves, because CrossOS can read the state before touching it.
Reason for modification: language and medium port — a Cocoa key-value observation over MASShortcutView and a notification become a ref count and two bound calls; the reference cannot read whether the person already suspended, because it owns the bindings. §9.11 records rectangle as BEHAVIOR-only; the behaviour is the suspension, and the AppKit mechanism is not ported.
CrossOS license: MIT

Source repository: rxhanson/Rectangle
Source commit: 12a9bc79f99abeb86297da3d7436b4489f920fa2
Source file: Rectangle/PrefsWindow/SnapAreaViewController.swift:16-23 (the eight named landscape direction rows) and :158-160 (the PORTRAIT set appears only when a portrait display is attached)
Original license: MIT
Original copyright: Copyright (c) 2019-2026 Ryan Hanson; based on the Spectacle
app, Copyright (c) 2017 Eric Czarny (the second line is the reference's own
LICENSE, at the pinned commit, and MIT requires it travel with any copy)
CrossOS destination: app/frontend/src/controls/ZoneEditorControl.tsx (the PLACES table, placeFor, and the per-row place picker)
Modification: structural port, no code copied. What transfers is that a snap area is chosen by NAMING A PLACE — topLeft, top, topRight, left, right, bottomLeft, bottom, bottomRight — rather than by supplying coordinates. CrossOS's editor asked for four floats per zone and started blank, so a person wanting a left half had to work out what 0.5 meant before they could begin. The numbers STAY on the row: this is an addition to the editor, not a replacement of it, and a rectangle that is none of the eight is a legitimate thing to want, so a row that matches no place reports "An exact rectangle" rather than being rounded to the nearest one. Choosing a place fills the geometry and fills an id and name only when they are still blank — naming a zone is the person's, and a picker that renamed their zone every time they resized it would be a control fighting them. The reference's portrait set is not ported: Rectangle shows a second column of the same eight when a portrait display is attached, and CrossOS serves no display geometry at all, so a single set is the honest shape rather than an empty one.
Reason for modification: language and medium port — eight `NSPopUpButton` outlets
(`:16-23`; the storyboard elements are `<popUpButton>`, not buttons) become one
select per row. §9.11 records rectangle as BEHAVIOR-only; the behaviour is that the vocabulary is places, not coordinates.
CrossOS license: MIT
