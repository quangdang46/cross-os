# Attribution — FinderRight single-catalog and editor-catalog reference

**No FinderRight source is vendored.** Nothing in this directory is a copy, a
transliteration, or a partial extract of funny-dog/FinderRight, and no file in
CrossOS is derived from its text. This entry records a behaviour reference and
is the evidence that a structural port was made rather than a copy.

Source repository: funny-dog/FinderRight
Source commit: af8d5f0fe2479e3d1b0afc7d01829feb0e022910
Source file: FinderRightKit/Sources/FinderRightKit/MenuFeature.swift:3-6 and FinderRightKit/Sources/FinderRightKit/EditorCatalog.swift:16-19
Original license: MIT
Original copyright: Copyright (c) FinderRight contributors
CrossOS destination: core/cmd/crossos/findermenu.go (handleFinderMenuEntries and finderFileType)
Modification: structural port, no code copied. What transfers is the rule the reference states about itself in Chinese and then keeps: MenuFeature.swift:3-6 calls a menu item 全应用的「单一事实来源」— the single source of truth for the whole app — because "两端共用同一份清单与同一套 id，避免「设置里的开关与右键菜单对不上」": the main app's settings screen and the FinderSync extension share ONE list and ONE set of ids, precisely so the toggle in Settings cannot disagree with the menu in the right-click. CrossOS had exactly that disagreement. handleFinderMenuEntries read filetype.Seeds(), so the appex's New submenu carried whatever the shipped catalog enabled — one row — no matter what a person switched on in the Explorer page. It now reads the same c.set.FileTypes() the page writes. finderFileType is changed in the same commit because it resolved createFile's extension from the same seeds: fixing only the reader would put a row in the submenu that the verb it fires then refuses, which is the submenu entry that fails on click.
Reason for modification: medium and language port — a Swift struct with localized name keys becomes a Go handler reading one store. §9.10 records FinderRight as behaviour-only (🟡) and this is that: the single-catalog rule, not the code. EditorCatalog.swift:16-19 contributes the ORDER rule — the order of the list is the order the submenu shows — but NOT its installed-filter, which the extension applies at display time with NSWorkspace; CrossOS serves no way to enumerate installed editors, so the honest port keeps the ordering and leaves the filter as a stated gap rather than inventing a stand-in.
CrossOS license: MIT
