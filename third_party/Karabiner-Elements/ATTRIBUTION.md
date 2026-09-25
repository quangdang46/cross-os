# Attribution — Karabiner-Elements editor-flow port (beads w6-frontend-setup-renderers, w7-frontend-remap)

Source repository: pqrs-org/Karabiner-Elements
Source commit: c7197aaf27345c11a0d1e4bf9ddad0c9020ae387
Source file: src/apps/SettingsWindow/src/View/SimpleModificationsView.swift
Original license: Unlicense (public domain dedication)
Original copyright: No copyright reserved — the authors dedicated the work to the public domain
CrossOS destination: app/frontend/src/controls/PipelineTraceControl.tsx (one row per pipeline stage, each carrying its own value, in recorded order), app/frontend/src/controls/PluginDetailControl.tsx (a fixed-width list of what is installed beside the detail of the one being read)
Modification: structural port, no code copied. What transfers is the LAYOUT — the row that holds one mapping end, an arrow, and the other end (SimpleModificationsView.swift:47-72), and the fixed-width selector column beside a detail pane (:9-21). The mapping itself is CrossOS's own: a decision trace's stages (event → context → rule → intent → action → result) are the daemon's recorder vocabulary (core/pkg/record/record.go), served as fields on a row rather than as a JSON document the shell parses.
Reason for modification: SwiftUI on macOS becomes React over a declared schema in a webview; the reference's device selector, its search field and its rule JSON editor are not part of a settings window that renders daemon rows, and §9.11 keeps Karabiner as concepts/behavior/UX only.
CrossOS license: MIT

Source repository: pqrs-org/Karabiner-Elements
Source commit: c7197aaf27345c11a0d1e4bf9ddad0c9020ae387
Source file: src/apps/SettingsWindow/src/View/ProfileEditView.swift
Original license: Unlicense (public domain dedication)
Original copyright: No copyright reserved — the authors dedicated the work to the public domain
CrossOS destination: app/frontend/src/controls/PluginDetailControl.tsx (the detail side of that split: the selected item's own fields, one label beside its value per row)
Modification: structural port, no code copied — the reference's editable name field becomes a read-only manifest row, and its Save/Cancel pair is not ported: an extension's manifest is the daemon's (core:pluginMeta) and the shell has no write path to it. The rule that transfers is the one the reference's detail pane already keeps: a field it does not recognise is still shown, not dropped.
Reason for modification: language and medium port; the reference edits a profile, CrossOS reports one.
CrossOS license: MIT

No Karabiner-Elements source is included; the LICENSE is recorded here for the
editor-flow reference only. The project is public domain, which passes the
§9.11 gate — the gate rejects GPL, copyleft and unlicensed sources, and a
dedication to the public domain is none of those.

Source repository: pqrs-org/Karabiner-Elements
Source commit: c7197aaf27345c11a0d1e4bf9ddad0c9020ae387
Source file: src/apps/SettingsWindow/src/View/ComplexModificationsView.swift (filterKeyword, normalizedFilterKeyword, matchesFilter(_:) matching rule.searchText.localizedCaseInsensitiveContains)
Original license: Unlicense (public domain dedication)
Original copyright: No copyright reserved — the authors dedicated the work to the public domain
CrossOS destination: app/frontend/src/controls/KeymapEditorControl.tsx (the search field that narrows the served rule list)
Modification: structural port, no code copied. What transfers is the RULE the reference already keeps: a long list of rules is narrowed by one keyword field above it, the keyword is trimmed before it is compared, and it is matched against a per-rule search TEXT rather than against an id — so what a person can see in a row is what the filter matches. The list here is the daemon's registry (config.getMatrix), not the reference's own profile file.
Reason for modification: the reference is SwiftUI on macOS and this is React over a declared schema; its rules live in a document the shell has no path to, and §9.11 keeps Karabiner as concepts/behavior/UX only.
CrossOS license: MIT

Source repository: pqrs-org/Karabiner-Elements
Source commit: c7197aaf27345c11a0d1e4bf9ddad0c9020ae387
Source file: src/apps/SettingsWindow/src/View/SearchField.swift (a text Binding wrapped around an NSSearchField, with the change debounced through a Coordinator)
Original license: Unlicense (public domain dedication)
Original copyright: No copyright reserved — the authors dedicated the work to the public domain
CrossOS destination: app/frontend/src/controls/KeymapEditorControl.tsx (the same one-field filter, matching on every keystroke rather than on a debounce)
Modification: structural port, no code copied. What transfers is the SHAPE — one text field bound to the list it filters — and the decision NOT to port the debounce, which exists because the reference's filter runs over a profile held in a file on disk while this one runs over rows the daemon already answered.
Reason for modification: medium port; the shell filters a served list in memory and has nothing to debounce against.
CrossOS license: MIT

Source repository: pqrs-org/Karabiner-Elements
Source commit: c7197aaf27345c11a0d1e4bf9ddad0c9020ae387
Source file: src/apps/SettingsWindow/src/View/SimpleModificationsView.swift:47-72 (one row carrying one end of a remapping and the other, each end independently legible)
Original license: Unlicense (public domain dedication)
Original copyright: No copyright reserved — the authors dedicated the work to the public domain
CrossOS destination: app/frontend/src/controls/RuleBuilderControl.tsx (the rule sentence, where each dimension of the rule is a pick carrying its own value rather than typed text, and the assembled sentence is shown back as the spec writes it)
Modification: structural port, no code copied. What transfers is the ROW: two ends of a rule, each with its own control, and neither end a text field the person has to spell correctly. CrossOS's two ends are the app scope and the action, and the key between them is captured by pressing it.
Reason for modification: language and medium port; the reference edits Karabiner's own remapping document, CrossOS writes a rule the rules engine compiles through the capability API (config.setUserRule).
CrossOS license: MIT

Source repository: pqrs-org/Karabiner-Elements
Source commit: c7197aaf27345c11a0d1e4bf9ddad0c9020ae387
Source file: src/apps/EventViewer/src/View/CaptureInputEventsView.swift:15-32 and src/apps/EventViewer/src/View/CaptureActiveLabel.swift:16-49
Original license: Unlicense (public domain dedication)
Original copyright: No copyright reserved — the authors dedicated the work to the public domain
CrossOS destination: app/frontend/src/controls/ObserveToggleControl.tsx and the .ctl-live / .ctl-live-dot / .ctl-stop rules in app/frontend/public/style.css
Modification: structural port, no code copied. Three things transfer. (1) The running state is a role:.destructive control carrying stop.fill, not the idle button re-rendered: stopping a recorder is not the mirror of starting one, and a button that reads "start" beside a machine that is already recording is the failure this control exists to prevent. (2) The sign of a live recorder sits BESIDE the control that ends it, as a filled circle whose opacity breathes on a 2s cycle dimming to 0.35, updated at 1/30s and touching only the opacity so the label never moves with it. (3) A third state Karabiner treats as first-class: waiting, drawn still and dimmed, because the daemon is up and asked to capture but the device it needs is not accessible yet. Two deviations, both forced by the medium and both commented at the point they happen — SwiftUI's accessibilityReduceMotion becomes the reduced-motion block already in style.css, and SF Symbols become text glyphs.
Reason for modification: the reference captures raw input events in a standalone macOS app; CrossOS reads a recorder the daemon already runs and answers three questions from it (where observe mode stands, what a decision was, and what is being kept). The reference has no privacy mode to disclose, so the line naming what the daemon records is not a port of anything — it is the part a product that records every keystroke decision owes the person reading it. The waiting state also needs a retry affordance the reference does not, because the reference holds the capture state locally and this one does not.
CrossOS license: MIT
