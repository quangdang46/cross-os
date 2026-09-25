# Attribution — Windhawk panel reference (bead cross-os-1uu)

**No Windhawk source is vendored.** Nothing in this directory is a copy, a
transliteration, or a partial extract of ramen-/windhawk (microsoft/windhawk),
and no file anywhere in CrossOS is derived from its GPL-3.0 text. This entry
records a LEARN-ONLY decision and is the evidence that a structural port was
made rather than a copy.

Source repository: microsoft/windhawk
Source commit: 61d99ed8e182e1af1b60109612b6763ad1b4b74e
Source file: src/windhawk-frontend/apps/windhawk-frontend/src/app/panel/shared/ModCard.tsx:317-360 (the card's title container, its ModMetadataLine with singleLine, and the description's explicit no-description fallback) and :322-327 (a card rendered without the selection prop is laid out exactly as it is with none of it)
Original license: GPL-3.0
Original copyright: Copyright (c) Windhawk contributors
CrossOS destination: app/frontend/src/controls/PluginListControl.tsx (metaLine and the metadata row under each extension)
Modification: structural port, no code copied. Two things transfer, and the second is the one this repository needed. First, the facts about an item go on ONE line under its name (ModMetadataLine singleLine) rather than being scattered. Second — and this is the load-bearing half — that line has an EXPLICIT empty state: ModCard falls back to an italic "no description" rather than leaving the slot blank (:357). CrossOS's daemon genuinely has nothing to say for most of these rows today, because the plugin-meta source loads no manifest, so name and version come back empty. A row that quietly carried no facts read as a row with none to carry, which is the same class of thing as the health chip that stood on every row until this audit removed it: a blank slot is read as a fact. It now says, in the daemon's own words, that no manifest was loaded — or, when the daemon gave a reason, quotes that reason rather than prettifying it.

A third shape is recorded and deliberately NOT ported: the card's selection
checkbox, which the reference adds to a card and which leaves a card rendered
without it laid out identically (:322-327). CrossOS's list has no multi-select
and the row's toggle already writes through TogglePlugin, so there is nothing
to select. The ribbon, the pinned-to-bottom action row, and the repository
rating tooltip belong to a marketplace that CrossOS does not have — §3.6c and
pages.go:101 both refuse it until Phase 4 — and inventing a version of them
would be building an affordance for a surface that does not exist.

Reason for modification: medium and language port — an antd + styled-components
React app becomes a declarative control over ServiceApi. §9.11 grades Windhawk
architecture-only and this is that: the card's anatomy, not its code. It is
also the substitute the audit settled on for the Extensions Manager, because
the goal named Raycast, which is closed source and unreadable.
CrossOS license: MIT
