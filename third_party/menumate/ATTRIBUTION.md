# Attribution — menumate concept port (bead cross-os-vbl.3)

Source repository: Hibrielle/menumate
Source commit: 017d6dae1e8569b9d92513036503b013dc528283
Source file: Core/Sources/MenuMateCore/PackManifest.swift
Original license: MIT
Original copyright: Copyright (c) 2026 Hibrielle
CrossOS destination: core/pkg/plugin/pack.go (ParsePackManifest + Validate)
Modification: decode-then-validate + unknown-keys-ignored + defaults (pack icon shippingbox, placement topLevel, isEnabled true) ported to Go; schema shapes (§5.3/5.4) adapted; script executor NOT copied
Reason for modification: language port; CrossOS executes native capabilities, script execution is Level B gated
CrossOS license: MIT

Source repository: Hibrielle/menumate
Source commit: 017d6dae1e8569b9d92513036503b013dc528283
Source file: Core/Sources/MenuMateCore/RuleMatcher.swift
Original license: MIT
Original copyright: Copyright (c) 2026 Hibrielle
CrossOS destination: core/pkg/plugin/pack.go (Match + VisibleActions, MatchCtx/MatchItem/MatchResult)
Modification: targets/UTI/count filters + typed result (never bare bool) ported; UTType resolution stays platform-side (MatchItem.UTI is a string)
Reason for modification: language port; filesystem metadata resolution is the caller's job
CrossOS license: MIT

Source repository: Hibrielle/menumate
Source commit: 017d6dae1e8569b9d92513036503b013dc528283
Source file: Core/Sources/MenuMateCore/PackInspector.swift
Original license: MIT
Original copyright: Copyright (c) 2026 Hibrielle
CrossOS destination: core/pkg/plugin/pack.go (InspectPack)
Modification: undeclared-file surfacing ported (.git + metadata skipped, hidden files NOT skipped, sorted); symlink-escape defense adapted as pre-clean ".." + Rel check in ValidatePackPath
Reason for modification: language port (filepath.WalkDir instead of FileManager enumerator)
CrossOS license: MIT

Source repository: Hibrielle/menumate
Source commit: 017d6dae1e8569b9d92513036503b013dc528283
Source file: Core/Sources/MenuMateCore/ConfigStore.swift
Original license: MIT
Original copyright: Copyright (c) 2026 Hibrielle
CrossOS destination: core/pkg/plugin/pack.go (parse layering: ParsePackManifest decodes, Validate checks semantics)
Modification: concept only (mtime cache + atomic save stay in Config Manager §3.8, not duplicated here)
Reason for modification: avoid a second config authority; pack loader parses, Config Manager owns persistence
CrossOS license: MIT

Source repository: Hibrielle/menumate
Source commit: 017d6dae1e8569b9d92513036503b013dc528283
Source file: App/UI/MenuHubScreen.swift (:63-68 the fixed-width sidebar beside a detail pane; :94 the sidebar column; :692-710 SectionCap, a 9.5pt semibold tracked label over each section)
Original license: MIT
Original copyright: Copyright (c) 2026 Hibrielle
CrossOS destination: app/frontend/src/App.tsx (navGroups + the nav rail markup), app/frontend/public/style.css (the .sections / .nav-group / .nav-group-title rules)
Modification: structural port, no code copied — a left rail of grouped page buttons with a section cap per group; the reference is SwiftUI on macOS and the destination is React in a webview, so the layout and the label treatment transfer and the drawing does not. The group names, the group order and the pages under each group are the daemon's (Host.Group / Host.Order), not the reference's menu sections.
Reason for modification: CrossOS navigates discovered UIContributions rather than an extension list, and its rail is rendered from a webview; the reference's own sections, search field and filter segmented control are not part of the settings nav.
CrossOS license: MIT

Source repository: Hibrielle/menumate
Source commit: 017d6dae1e8569b9d92513036503b013dc528283
Source file: App/UI/MenuHubScreen.swift (:181 the SectionCap, and :183-188 the ForEach under it in which every entry is handed to actionRow, so each entry gets its own row rather than being summarised into the one above it; :246-247 the same ForEach->actionRow shape for a parent's sub-items, but nested under that parent's own header row at :231-240 rather than under a cap)
Original license: MIT
Original copyright: Copyright (c) 2026 Hibrielle
CrossOS destination: app/frontend/src/controls/PluginListControl.tsx (one row per installed extension, each carrying its own state and its own reason beside it: the group label standing over each list at :212-213, the row per plugin at :215-256, the Enabled/Disabled state beside the box at :237, and the daemon's own reason quoted into the row at :238 by metaLine at :67-74)
Modification: structural port, no code copied. What transfers is the LAYOUT: a list under a cap in which every entry is drawn as its own row, because the question a reader is answering is which extension contributes what — collapsing the entries into one summary answers it for none of them. The rows here are the daemon's extension registry (core:pluginMeta) and each carries the daemon's own reason for its load state.
Reason for modification: the reference is a SwiftUI app computing live menu state from an extension manager; this is React over a declared schema reading a registry, and no pack manifest is loaded at runtime so there are no contributed actions to list yet.
CrossOS license: MIT

Source repository: Hibrielle/menumate
Source commit: 017d6dae1e8569b9d92513036503b013dc528283
Source file: App/UI/PacksScreen.swift (:255-368 struct PackRow — :258 the `let expanded: Bool` parameter and :278 `if expanded { expandedBody }`, so the card ships collapsed and expansion is the parent's state; :282-318 the header is one Button whose whole surface toggles (.contentShape(Rectangle()) at :315, .buttonStyle(.plain) at :317); :309-311 the trailing "n of m" enabledCount/totalCount Text; :320-367 expandedBody, inset under the header text by .padding(.leading, 46) at :359, holding one row per member each with its own MMSwitch bound to a per-row callback at :327-330)
Original license: MIT
Original copyright: Copyright (c) 2026 Hibrielle
CrossOS destination: app/frontend/src/controls/ProfileListControl.tsx (the expand-in-place profile card, the trailing selected-of-total rollup, the per-capability switch list inside the expanded card, and the reviewed-all gate that arms the Apply)
Modification: structural port, no code copied. What transfers is the INTERACTION MODEL and the state machine: a card that is collapsed until asked, whose expansion is owned above it rather than hidden inside the row; a header that carries an "n of m" count so a reader can see how much of the thing is on without opening it; and one switch per member living inside the expanded body rather than on a separate settings page. The reference is SwiftUI on macOS and the destination is React over a declared schema, so the drawing, the icons, the AppIcon and the MMSwitch component do not transfer and none were copied. CrossOS's own vocabulary travels in its place: the members are the profile layer's capabilities (core/pkg/profiles) and the counts come from the daemon's plan arithmetic. Of those counts, the per-capability INPUTS (will_enable, will_enable_plugin, enabled, total) are served by the daemon and the rollups are summed in the control — both the header's "n of m" and the preview sentence — because the selection is a client-side filter over the bundle and the daemon cannot narrow its own answer to a subset it was not told about until the apply. The script executor is NOT copied (§9.11 PARTIAL: menumate is concepts-and-schema-shapes only).
Reason for modification: the reference lists installed third-party extension packs with their contributed menu actions, each of which the user can enable independently and which the import path treats as a bulk write. CrossOS lists its own profile bundles — data declared in-repo, not installed code — and the settings nav has no per-pack settings page to put a duplicate of each member's editor on, so the members' individual verdicts stay on the pages that own them (Keyboard, Windows, Explorer) while the profile's own capabilities get switches here. The boundary is stated once in app/backend/profilespage.go and cited by this destination, because the previous version of this control stated the opposite.
CrossOS license: MIT

Source repository: Hibrielle/menumate
Source commit: 017d6dae1e8569b9d92513036503b013dc528283
Source file: App/UI/PackImportSheet.swift (:230-277 private func reviewList — :236-245 a row shows a filled check only once its id is in the reviewer's `viewed` set and an empty circle until then; :260-263 the `.onTapGesture { selectedActionID = a.id; viewed.insert(a.id) }` that selects a row is the same tap that inserts it into `viewed`, so looking at a row IS the act of acknowledging it; :271-273 the `viewedProgress` "viewed n of m" Text, its font and its colour, which sits under the list and says plainly how much is left)
Original license: MIT
Original copyright: Copyright (c) 2026 Hibrielle
CrossOS destination: app/frontend/src/controls/ProfileListControl.tsx (the reviewGate helper and its rendered sentence, the per-capability switch that doubles as the review, and the "Reviewed n of m" line beside the Apply)
Modification: structural port, no code copied. What transfers is the GATE and its state machine, which is the idea rather than the code: a bulk write that will change several things at once should not be one click away from a list nobody looked at, a row is marked by being looked at rather than by a separate confirmation, and the progress is stated as words under the list so a blocked button is never blocked silently. The reference's own safety target is a script import, and a script is executable code arriving from elsewhere; CrossOS's is a bundle of rules the daemon already has, which is why the gate here is a few lines of state and the reference's is a review screen with a detail pane. The reviewDetail pane, the matchSummary badge, the clone step and the script viewer are NOT ported. The gate is over every available capability rather than over the selection, which the reference's list is by construction — a review set that shrank as rows were deselected would be circular, and that was this control's first implementation. ONE BEHAVIOUR IS A CROSSOS ADDITION and is not a port: the control clears the review set when a card COLLAPSES, where the reference keeps `viewed` for the whole session and assigns a fresh set only on entering the review sheet (:434, a different screen rather than a collapse). The destination is a card on a settings page that can be folded away with its selection still applied in the daemon, so an armed Apply over rows the reader can no longer see is a review of something other than what will be written; the reference has no equivalent hazard because its list cannot be hidden without leaving the sheet. The other defensible rule — that having seen a row is a session-long fact — is the one the reference chose.
Reason for modification: the reference's gate protects against running code the user has not read. The CrossOS equivalent risk is a bulk settings write the user has not seen, so the same viewed-set state machine gates the Apply; the difference in stakes is why no detail pane came with it, and why a disabled Apply always carries its sentence on the page rather than only in a title attribute. The two failure modes the port inherits from the reference are kept deliberately — a stuck gate that cannot be cleared, and a row that never marks — because the reference resolves both by making the review gesture and the mark the same click.
CrossOS license: MIT
