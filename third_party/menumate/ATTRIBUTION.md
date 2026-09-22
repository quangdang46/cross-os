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
