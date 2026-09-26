// The switcher overlay, on its own, against the fixture.
//
// src/switcher.html is the app's SECOND document — a window of its own, not a
// route inside the settings shell — and nothing in this repo had ever rendered
// it. The unit tests drive SwitcherPanelControl directly, so they cover the
// panel but not the document around it: the mount, the overlay's own chrome,
// and whether the two windows' stylesheets agree.
//
// There is no createRoot here on purpose. src/switcher.tsx renders itself at
// module scope — that is how the real document works — so importing it is the
// whole job. Calling createRoot as well mounts the overlay twice and React says
// so, and the two mounts race for the same container.
//
// The `?summon=1` seam lives in service-stub, which loads before this module
// and therefore before the overlay's first poll. See the comment there.

import { machine } from '../src/test/fixtures'

// Three windows, one selected, one skippable — the same machine the shell's
// Switcher page shows, so the two documents can be compared side by side. The
// panel reads this when it renders, which is after the trigger, so setting it
// here rather than before an import is not a race.
machine.windows = [
  { window_id: '412', app_id: 'com.apple.finder', title: 'Downloads', index: 0, selected: false, skippable: false },
  { window_id: '881', app_id: 'com.apple.Terminal', title: 'crossos — zsh', index: 1, selected: true, skippable: false },
  { window_id: '207', app_id: 'com.apple.Safari', title: '', index: 2, selected: false, skippable: true },
]

await import('../src/switcher')
