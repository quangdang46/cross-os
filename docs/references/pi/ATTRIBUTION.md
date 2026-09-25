# Attribution — pi resource-registry port (bead cross-os-1uu)

**No pi source is vendored.** Nothing in this directory is a copy, a
transliteration, or a partial extract of earendil-works/pi, and no file anywhere
in CrossOS is derived from its text. This entry exists to record a LEARN-ONLY
decision and to be the evidence that a structural port was made rather than a
copy. The project is MIT, so nothing here is here for licence reasons; it is
here because §9.11 grades pi NO and the merge gate asks every reused file to
name its source.

Source repository: earendil-works/pi
Source commit: 890f920884f6d21fc7617d236ef9e1cc5d7a0ef8
Source file: packages/coding-agent/src/modes/interactive/components/config-selector.ts:57-64 (ResourceGroup: key, label, scope, origin, source, subgroups) and :417-423 (a group the current scope cannot write is dimmed rather than drawn as if it could)
Original license: MIT
Original copyright: Copyright (c) 2025 Mario Zechner
CrossOS destination: core/cmd/crossos/main.go (handlePluginList), app/backend/bridge.go (PluginState), app/frontend/src/controls/PluginListControl.tsx (the origin grouping)
Modification: structural port, no code copied. Two things transfer. The first is that a registry row is grouped by WHERE it came from — pi's ResourceGroup carries an origin of "package" or "top-level" and a scope, and a person reading a list mixing installed things with built-in ones cannot tell them apart from an id, when the two behave differently. CrossOS's Extensions list is now grouped the same way, and the group carries the sentence a person acts on: these are compiled from the rule table, so there is no process to install, remove, or crash. The second is the rule behind that sentence. pi dims a group the current scope cannot change rather than drawing it as though it could, and the same reasoning pointed at a chip that had been on every row here: the daemon sent the literal string "healthy" for every id while no supervisor ran and no plugin was a child process, so the chip asserted something nobody had measured, in the same word the trial gate uses to mean "I checked". It has been removed from the list rather than restated. The daemon now also sends the third of PluginState's own documented words — "disabled" — for a plugin that is switched off, which it never did, so a row used to carry a healthy chip beside the word Disabled and contradict itself.
Reason for modification: language and medium port — a TypeScript TUI over ResolvedPaths becomes a declarative control over the daemon's own plugin list. The scope half of pi's grouping is not ported: CrossOS has one scope, so a group that cannot be written from here does not yet exist. §9.11 grades pi NO and this is that: the structure, not the source.
CrossOS license: MIT
