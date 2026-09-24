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
