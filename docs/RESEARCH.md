# CrossOS — OSS Research Matrix

Purpose: repo archaeology. Each repo should answer 5 questions
(input/event model? context detection? config schema? plugin/extensibility?
which parts are reusable under its LICENSE?), then map Our Feature → reuse source.

## Reuse rules

- 🟢 **COPY/ADAPT** — permissive license (MIT/Unlicense…); check LICENSE + dependency tree per file before copying.
- 🟡 **ARCHITECTURE** — study architecture/behavior only, do NOT lift code (GPL/copyleft or complex native code).
- 🟡 **BEHAVIOR/UX** — take the shortcut matrix, UX, settings model.

## A. Keyboard / Input — P0

| Repo | OS | License | Takeaways | Type |
|---|---|---|---|---|
| pqrs-org/Karabiner-Elements | macOS | Unlicense | low-level remap, virtual HID, device identification | 🟢 study/adapt |
| jtroo/kanata | Win/macOS/Linux | GPL-3.0 | layers, tap-hold, macros, config engine | 🟡 architecture |
| houmain/keymapper | Win/macOS/Linux | GPL-3.0 | context-aware + per-app mapping (closest to core idea) | 🟡 architecture |
| Fuzzy-and-Fluffy/windows-keyboard-for-mac | macOS | check repo | Windows muscle-memory rules + behavior matrix | 🟡 behavior |
| venkatarangan/karabiner-mac-to-windows | macOS | check repo | deep Windows mapping, Finder behavior | 🟡 behavior |
| joeytroy/karabiner-windows | macOS | check repo | Win11 shortcuts + Ghostty/VSCode/VM exclusions | 🟡 behavior |
| rux616/karabiner-windows-mode | macOS | check repo | Windows/Linux rules + dev exclusions | 🟡 behavior |
| Fuzzy-and-Fluffy/windsify-free | macOS | check repo | native-app keyboard layer (no Karabiner) | 🟡 architecture |
| shootdaj/ke-custom | macOS | check repo | physical modifier remap (reference only, not recommended) | 🟡 behavior |
| raxigan/pcfy-my-mac | macOS | MIT | overall base: Karabiner + AltTab + Rectangle + setup CLI | 🟢 adapt |
| alper-han/CrossMacro | Win/macOS/Linux | GPL-3.0 | Profiles → Triggers → Workflow → Actions; shared GUI+CLI engine | 🟡 architecture |
| hammerspoon/hammerspoon | macOS | check repo | module architecture, scripting API, event/window/hotkey model | 🟡 architecture |

## B. Window management / switching

| Repo | License | Takeaways | Type |
|---|---|---|---|
| lwouis/alt-tab-macos | GPL-3.0 | hook → enumerate windows → filter → MRU → switcher UI → focus | 🟡 architecture |
| rxhanson/rectangle | check repo | Accessibility API, frame calc, shortcuts, snap zones | 🟡 architecture |
| mikusnuz/nudge | MIT | 19 window actions, customizable shortcuts, drag-to-snap, multi-monitor — read BEFORE Rectangle | 🟢 adapt |
| asmvik/yabai | MIT | windows/spaces/displays internals (heavyweight, study) | 🟢 study |
| ianyh/Amethyst | MIT | window state, layouts, keyboard commands, config, accessibility | 🟢 study |
| saforem2/chunkwm | check repo | plugin architecture splitting WM into separate modules | 🟡 architecture |
| LGUG2Z/komorebi | custom | engine → CLI → JSON schema → TCP socket → external clients (Core↔Plugin model) | 🟡 architecture |

## C. Finder / Context menu — P0

| Repo | License | Takeaways | Type |
|---|---|---|---|
| mariusgm/newfile | MIT | SwiftUI host + Finder Sync Extension + custom types/templates/ordering/submenu — clone first | 🟢 adapt |
| Hibrielle/menumate | MIT | Actions + editable scripts + enable/disable/reorder + file-type scope + Extension Packs (Git repos) — plugin-ecosystem blueprint | 🟢 adapt heavily |
| wflixu/RClick | check repo | Finder Sync comparison: New File, Copy Path, Open With, Delete, Hide/Unhide, AirDrop | 🟢 study |
| rowild/MoreMenu | check repo | minimal New Text/Markdown + open file after creation (MVP test) | 🟢 study |
| sherman-yang/mac-finder-menu | check repo | FINDINGS.md: top-level menu → Finder Sync Extension is the official route; Services/Quick Actions can't be promoted | 🟡 behavior |
| funny-dog/FinderRight | check repo | full dev workflow: New File + Copy Path + Terminal/Editor + Cut/Paste + Compress | 🟡 behavior |
| InfinityBowman/FinderTools | check repo | minimal Ghostty/Code/New Text File — easy Finder Sync read | 🟢 study |

## D. Launcher / hotkey / automation

| Repo | License | Takeaways | Type |
|---|---|---|---|
| zjy4fun/HotkeyLauncher | check repo | global hotkey, JSON import/export, SwiftUI/AppKit/Carbon | 🟡 architecture |
| conversun/tinycast-cn | check repo | global + per-app hotkey, launcher, file search, clipboard | 🟡 architecture |
| lgrammel/app-launcher | check repo | forkable base: global hotkey, app discovery, search, system actions, tests | 🟢 study |

## E. macOS UI / device customization (future plugins)

| Repo | License | Takeaways | Type |
|---|---|---|---|
| FelixKratz/SketchyBar | GPL-3.0 | core → components → plugins/scripts → events; composable desktop | 🟡 architecture |
| linearmouse/linearmouse | check repo | mouse/trackpad modules + tests (extend to Keyboard+Mouse+Trackpad) | 🟡 architecture |
| MonitorControl/MonitorControl | check repo | hardware capability, custom shortcuts, OSD, helper process → Display Controls plugin | 🟡 architecture |

## F. Windows side (user-expectation UX + plugin model)

| Repo | License | Takeaways | Type |
|---|---|---|---|
| ramensoftware/windhawk | GPL-3.0 | 🔥 ARCHITECTURE REFERENCE: Core → Mod → Settings → Marketplace → Install/Enable/Disable/Update | 🟡 architecture |
| ramensoftware/windows-11-taskbar-styling-guide (+ windhawk-mods, Explorer Styler, Start Menu Styler) | check repo | a plugin can customize a single capability without touching core | 🟡 behavior |
| files-community/files | MIT+MPL | Windows Explorer UX expectations (don't copy the UI) | 🟡 behavior |

## P0 — clone & read first (10 repos)

1. houmain/keymapper 2. jtroo/kanata 3. Fuzzy-and-Fluffy/windows-keyboard-for-mac
4. mariusgm/newfile 5. Hibrielle/menumate 6. mikusnuz/nudge 7. lwouis/alt-tab-macos
8. alper-han/CrossMacro 9. ramensoftware/windhawk 10. LGUG2Z/komorebi
\+ pqrs-org/Karabiner-Elements, raxigan/pcfy-my-mac, asmvik/yabai

## Our Feature → source mapping

- Keyboard Engine → Keymapper / Kanata (arch) + windows-keyboard-for-mac (behavior)
- macOS HID → Karabiner-Elements (study)
- Window → Nudge / Rectangle / yabai
- AltTab → AltTab (arch)
- Finder → NewFile / MenuMate (adapt)
- Automation → CrossMacro / Hammerspoon (arch)
- Plugin Marketplace → Windhawk / MenuMate / komorebi (arch)

## See also

For the full implementation plan — architecture, module designs, MVP breakdown,
safety layer, phased timeline, testing, and build/distribution — see
[COMPREHENSIVE_PLAN.md](COMPREHENSIVE_PLAN.md).
