# Attribution — VS Code keybindings-editor port (bead cross-os-1uu)

**No VS Code source is vendored.** Nothing in this directory is a copy, a
transliteration, or a partial extract of microsoft/vscode, and no file anywhere
in CrossOS is derived from its text. This entry exists to record the reference
and to be the evidence that a structural port was made rather than a copy.

Source repository: microsoft/vscode
Source commit: 2ec783d855253a817b5787fb48bc6c3d8d31c0c5
Source file: src/vs/workbench/contrib/preferences/browser/keybindingsEditor.ts:329-333 (showSimilarKeybindings builds `"` + the chord's aria label + `"` and hands it to the search widget, so the editor's search becomes the quoted chord) and :354-355 (the two lines that wire the define-keybinding widget: onDidChange prints how many existing bindings hold the chord, and onShowExistingKeybidings sets that same search widget to the quoted chord — which is how the count leads somewhere instead of being a dead end). Line 353 constructs the widget; it is not one of the two behaviours. Both behaviours are implemented in src/vs/workbench/contrib/preferences/browser/keybindingWidgets.ts: :227-237 (printExisting writes "{0} existing commands have this keybinding" and fires onShowExistingKeybindings from the text's onclick at :236) and :257-259 (onKeybinding fires onDidChange on every key resolution, which is what makes the count track the chord as it is typed rather than after it is saved).
Original license: MIT
Original copyright: Copyright (c) 2015 - present Microsoft Corporation (LICENSE.txt at the pinned commit; the file is LICENSE.txt upstream, not LICENSE)
CrossOS destination: app/frontend/src/controls/ChordRecorder.tsx (the claim count under the captured chord, and the Show them button) and app/frontend/src/controls/KeymapEditorControl.tsx (Show them sets this control's own search field)
Modification: structural port, no code copied. Two things transfer. The first is that a recorder that presses keys rather than accepting typed text should PRINT ON ITSELF how many existing bindings claim the chord the moment it completes, before anything is saved — VS Code's widget does exactly that on every change, and it is the difference between learning a chord is taken while the keys are still under your fingers and learning it from a list on another part of the page afterwards. The second is that the count must be able to LEAD somewhere: the reference's widget can set the editor's own search to the quoted chord, so "this is taken" and "here is where" are one step apart. CrossOS now does the same by setting its search field, which is why that prop exists at all.

The number is never worked out in the shell. It is the daemon's own Conflicts(),
compiled from the rules that are still enabled by a real router, because a
count done by comparing chord strings in the browser would be a second and
stale opinion about which rules still fire. And a count that could not be read
prints nothing at all rather than printing zero: "no rule claims that" over a
daemon that would not answer is the same class of lie as the health chip that
stood on the Extensions list until this audit removed it.

Reason for modification: language and medium port — a monaco-editor workbench
widget with a model, a search widget and a context key becomes a declarative
control over the ServiceApi. §9.11's direct-reuse table has no row for vscode,
and neither does the §9.10 file-level matrix, so the decision recorded here is
this block's own: the keyboard-editor SURFACE is consulted and no code is taken.
That is NOT the grade the plan gives Karabiner-Elements, whose directory does
sit beside this one — the plan's own reference table (COMPREHENSIVE_PLAN.md:1115)
grades pqrs-org/Karabiner-Elements 🟢 study/adapt, a stronger grade than the
behaviour-only reading recorded above, and it records that repo's licence as
Unlicense. The two are therefore not comparable, and this block should not be
read as inheriting a grade it does not have.
CrossOS license: MIT
