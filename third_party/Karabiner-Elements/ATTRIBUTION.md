# Attribution — Karabiner-Elements editor-flow port (bead w6-frontend-setup-renderers)

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
