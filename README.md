# CrossOS — Cross-Operating-System UX Compatibility Layer

> **Goal:** move from Windows to macOS without relearning how to use your computer.
> Keep the OS underneath untouched; add a muscle-memory layer so familiar
> actions keep working the same way. Architecture: **pluggable plugins**,
> non-technical users first, developers second.

```
             macOS / Windows
                   │
           ┌───────▼────────┐
           │  CrossOS Core  │  Input → Context → Intent → Rule → Action
           │ + Safety Layer │  Snapshot / Ownership / Rollback / Safe Mode
           └───────┬────────┘
                   │ Plugin API
        ┌──────────┼──────────┐
        ▼          ▼          ▼
   Windows UX  Developer UX  Custom UX
```

## MVP (3 plugins)

1. **Windows Keyboard** — Ctrl+C/V/X/A/Z/F/S, F2 rename, Alt+F4, Alt+Tab, Home/End,
   Ctrl+Arrow, Win+Shift+S… (terminal/IDE-safe, context-aware)
2. **Windows Window Management** — Win+←/→/↑/↓, Win+Shift+←/→, Win+D/M
3. **Windows Explorer UX** — Right-click → New File, Copy Path, Open Terminal/Editor,
   Cut/Paste, F2

## Final stack

| Layer | Choice |
|---|---|
| Core | Go (event bus, context/intent engine, plugin runtime, permissions, config, IPC) |
| Desktop shell | Wails (Go + native WebView, no bundled Chromium) |
| UI | React + TypeScript (+ Tailwind/shadcn) |
| macOS adapter | Swift / ObjC at the boundary only (CGEvent, AX, FIFinderSync) |
| Windows adapter | Minimal Win32 / C/C++ (Raw Input, SendInput) |
| Plugin | manifest.json + rules + controlled actions (no native DLL/SO in MVP) |
| Config | JSON/YAML |
| License | MIT |

**The Finder Sync Extension is native Swift**, kept out of the Go core
(`extensions/finder-sync/`).

```
crossos/
├── core/            # pure Go: event, context, intent, plugin, permission, config, runtime
├── platform/
│   ├── darwin/      # keyboard, accessibility, windows, finder
│   └── windows/     # keyboard, windows, shell
├── plugins/         # windows-ux, finder, window-management, developer…
├── app/             # Wails backend + frontend (React/TS)
├── extensions/
│   └── finder-sync/ # Swift Finder Sync Extension
├── COMPREHENSIVE_PLAN.md  # architecture (source of truth)
├── docs/            # research matrix
└── tmp/research/    # read-only clones of reference OSS repos (gitignored, not committed)
```

## Run it

```bash
./scripts/run.sh
```

That builds the daemon, starts it if one is not already serving, and opens
the shell. It is safe to re-run: a second daemon never steals the socket from
a running one. `--no-open` runs headless, `--rebuild-frontend` forces the
frontend rebuild.

**First run will say keyboard remapping is OFF.** That is macOS input
monitoring, not a bug:

> System Settings → Privacy & Security → Input Monitoring → add CrossOS →
> enable → restart CrossOS

Everything else — the window, settings pages, plugin trials, config — talks to
the live daemon immediately.

## Safety principles (P0, on par with the input engine)

- Intercept events only → disabling removes all effects; never patch Finder,
  system binaries, or the kernel.
- Every integration has a lifecycle + ownership tracking + rollback;
  only clean up what CrossOS created.
- Emergency kill switch + Safe Mode (enable for 30s → confirm/rollback).
- A **Reset Everything** button: disable hooks → stop daemon → disable
  plugins/extension → remove login item → verify no CrossOS process remains.

## Research & Planning

- [COMPREHENSIVE_PLAN.md](COMPREHENSIVE_PLAN.md) — the single source of truth:
  detailed architecture, module-by-module design, plugin spec, research/licensing
  matrix (§9, ~37 repos), MVP breakdown (Windows Keyboard, Window Management,
  Explorer UX), safety layer, phased implementation, testing, and
  build/distribution.

Reference repos are shallow-cloned into `tmp/research/` for code archaeology
(gitignored). Read these 10 P0 repos first: Keymapper, Kanata,
windows-keyboard-for-mac, NewFile, MenuMate, Nudge, AltTab, CrossMacro,
Windhawk, komorebi.

⚠️ **Licensing:** do not copy GPL code (Keymapper, Kanata, CrossMacro, AltTab,
Windhawk…) into the MIT core — study architecture/behavior only. Adapt code
directly only from permissively licensed repos (MIT/Unlicense) after checking
each file's LICENSE + dependency tree.
