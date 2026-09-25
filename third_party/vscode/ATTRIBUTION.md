# Attribution — VS Code keybindings-editor port (bead cross-os-1uu)

**No VS Code source is vendored.** Nothing in this directory is a copy, a
transliteration, or a partial extract of microsoft/vscode, and no file anywhere
in CrossOS is derived from its text. This entry exists to record the reference
and to be the evidence that a structural port was made rather than a copy.

Source repository: microsoft/vscode
Source commit: 2ec783d855253a817b5787fb48bc6c3d8d31c0c5
Source file: src/vs/workbench/contrib/preferences/browser/keybindingsEditor.ts:329-333 (showSimilarKeybindings sets the search to the quoted chord) and :353-354 (the define-keybinding widget prints how many existing bindings hold the chord as it is typed, and can pop the list out)
Original license: MIT
Original copyright: Copyright (c) Microsoft Corporation.
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
control over the ServiceApi. §9.11 has no row for vscode; the decision recorded
here is that its keyboard-editor SURFACE is consulted and no code is taken,
which is the same grade the table gives to Karabiner-Elements, whose directory
sits beside this one.
CrossOS license: MIT
