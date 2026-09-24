# Attribution — newfile FinderSync + Shared helpers port (bead cross-os-vbl.1)

Source repository: mariusgm/newfile
Source commit: b3f665ab839dfe95e6925735ca2a8209d3100fb8
Source file: Extension/FinderSync.swift
Original license: MIT
Original copyright: Copyright (c) 2026 Wheel Up Labs
CrossOS destination: extensions/finder-sync/menu.go + ipc.go + visibility.go (menu snapshot, Unix-socket IPC client, daemon-down hide)
Modification: FIFinderSync subclass pattern adapted — menu building + tag-indexed snapshot ported; DistributedNotificationCenter + host-app launch replaced by Unix-socket IPC client (D2 transport decision); Swift/Cocoa rewritten in Go
Reason for modification: CrossOS extension is a minimal IPC client (requests only, never code); host-app UI stays out (declarative §3.6c instead)
CrossOS license: MIT

Source repository: mariusgm/newfile
Source commit: b3f665ab839dfe95e6925735ca2a8209d3100fb8
Source file: Shared/FilenameGenerator.swift
Original license: MIT
Original copyright: Copyright (c) 2026 Wheel Up Labs
CrossOS destination: extensions/finder-sync/naming.go (UniqueName)
Modification: rewritten in Go; fileExists closure seam preserved as exists func; redundant-suffix strip + dotfile + increment rules preserved
Reason for modification: language port; URL appending replaced by filepath.Join
CrossOS license: MIT

Source repository: mariusgm/newfile
Source commit: b3f665ab839dfe95e6925735ca2a8209d3100fb8
Source file: Shared/FileTypeEntry.swift + Shared/SeedPresets.swift
Original license: MIT
Original copyright: Copyright (c) 2026 Wheel Up Labs
CrossOS destination: extensions/finder-sync/naming.go (FileType + Seeds)
Modification: menuTitle/derivedDisplayName fallback ported (MenuTitle); 8 built-in presets ported verbatim (fields renamed to Go initialisms)
Reason for modification: language port; UUID/Identifiable/SwiftUI concerns dropped (daemon owns identity)
CrossOS license: MIT

Source repository: mariusgm/newfile
Source commit: b3f665ab839dfe95e6925735ca2a8209d3100fb8
Source file: Shared/SeedPresets.swift + Shared/FileTypeEntry.swift
Original license: MIT
Original copyright: Copyright (c) 2026 Wheel Up Labs
CrossOS destination: app/frontend/src/controls/ProfileListControl.tsx (a profile card is a DECLARED ENTRY with a display name of its own, not a list of switches)
Modification: structural port, no code copied — the library shape is what transfers: a list of entries the daemon declares, each with its own display name and its own state (SeedPresets.swift:4-14, FileTypeEntry.swift:3-12), so the thing a person picks is a thing they can read. CrossOS's entries are profiles and their capabilities, served by the daemon (core:profiles); the eight built-in file-type presets, the filename rules and the FinderSync menu are not ported.
Reason for modification: language and medium port (a SwiftUI settings list over a fixed preset table becomes declared cards rendered from a schema in a webview); §9.11 records newfile as DIRECT REUSE for the FinderSync helpers, and this is a second, independent use of its LIBRARY shape on the frontend.
CrossOS license: MIT

Source repository: mariusgm/newfile
Source commit: b3f665ab839dfe95e6925735ca2a8209d3100fb8
Source file: App/PreferencesView.swift + App/FileTypeRow.swift + App/TemplateEditorSheet.swift
Original license: MIT
Original copyright: Copyright (c) 2026 Wheel Up Labs
CrossOS destination: app/frontend/src/controls/FileTypeListControl.tsx
Modification: structural port, no code copied — the ROW SHAPE is what transfers, and it is the fourth independent use of this repository's library shape on the frontend (after ProfileListControl and the Explorer menu list). What ports: one row per file type, in the order the New menu renders, each row carrying its own toggle, extension, menu label and default filename as separate fields rather than one summary (PreferencesView.swift:75-102, FileTypeRow.swift:19-57); a "Custom Types" divider above the first row the daemon did not write; column captions in the row's field order (:124-140); a built-in that can be turned off and renamed but never deleted (FileTypeRow.swift:96-109). Three RULES port as behaviour, because each is the reason a person does not lose work: a row whose extension is invalid stays on screen, disabled, with the reason beside it, and is never dropped out from under the person editing it (PreferencesView.swift:32-34, FileTypeRow.swift:82-84); the template editor HINTS and does not gate, so an unparseable .json body is reported and still saved (TemplateEditorSheet.swift:26-41, :50-63); and a reorder writes the COMPLETE new order rather than a move, because the daemon refuses a partial list and rightly so. validateExtension's own limits (non-empty, at most 16 characters, a-z 0-9 . _ -) are what decide what "invalid" means here. CrossOS's rows, identities, validation and persistence are the daemon's (core/pkg/filetype); the reference's eight presets, its SwiftUI drag-to-reorder DropDelegate and its UserDefaults store are not ported.
Reason for modification: language and medium port — a SwiftUI settings sheet over a local store becomes a React renderer over a daemon-served schema in a webview (§3.6c). The reference's drag handle becomes a keyboard-operable move control, because a list reorderable only by mouse is not reorderable at all for a keyboard user, and the daemon's reorder verb takes an explicit id list rather than a drop. §9.11 records newfile as DIRECT REUSE for the FinderSync helpers; this entry is a frontend use of its library SHAPE, adding no copied code and no new license obligation.
CrossOS license: MIT
