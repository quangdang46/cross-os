# Attribution — rectangle settings-surface port (bead cross-os-ui-port-m37)

Source repository: ramonwessels/rectangle
Source commit: 12a9bc79f99abeb86297da3d7436b4489f920fa2
Original license: MIT
Original copyright: Copyright (c) 2019-2026 Ryan Hanson

CrossOS destination: `app/frontend/src/App.tsx`, `app/frontend/public/style.css`

Modification: **structural port, no code copied.** The reference is AppKit and
the destination is React, so what transfers is the shape of the settings
surface, not its implementation:

- one scrolling form of grouped rows, each row a label beside its control
  (`SettingsViewController.swift:307-311` — vertical, leading-aligned stack,
  uniform row spacing; the port keeps a single `--row-gap: 5px` scale)
- a closing About block carrying the version plus the newest activity line
  (`SettingsViewController.swift:10,15` — version label and
  check-for-updates row)
- one control per setting, toggled in place (`SettingsViewController.swift:60`)

What is CrossOS's own: the rows are not hardcoded. The Go Host discovers the
pages and their controls (§3.6c) and the renderer draws whatever the Service
serves, so a new settings page is a Go change and never a UI change. The
spacing scale, system-font stack and colour tokens are CrossOS's, chosen to
read as a native macOS pane inside a webview.

No rectangle source is included; the LICENSE is recorded here for the
structural reference only.
CrossOS license: MIT
