# Attribution — Amethyst window-status surface port (bead w6-frontend-setup-renderers)

Source repository: ianyh/Amethyst
Source commit: 6508ee2cacf8f9b357e1a3249a8f28ba5c94db2a
Source file: .amethyst.sample.yml
Original license: MIT
Original copyright: Copyright (c) 2015 Ian Ynda-Hummel
CrossOS destination: app/frontend/src/controls/HomeSummaryControl.tsx (the landing card: one row per fact about the machine, each row carrying the state AND a sentence about what the state means)
Modification: structural port, no code copied. What transfers is the RULE the sample settings file already keeps: every line is a fact with its own explanation, so a reader never has to know the key to know what the value does (window-margin-size at :255-258, and the per-application list beside the global facts at :270-274). The card's facts are the daemon's — running, interception, the active profile, the extension count and the readiness rollup — and each is rendered with the sentence that makes it mean something. Amethyst's own settings (layouts, modifiers, focus rules) are not ported: §9.11 keeps Amethyst as concepts/behavior/UX only.
Reason for modification: a YAML file a person edits becomes a read-only card the daemon fills; nothing about window management, tiling or the modifier model is reused, because CrossOS's window capabilities are the daemon's own (§ winlayout).
CrossOS license: MIT

No Amethyst source is included; the LICENSE is recorded here for the status-surface
reference only.
