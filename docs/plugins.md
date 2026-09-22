# Plugins & capabilities

CrossOS features come as **plugins**: small packages that declare what
they do. The app's pages (Dashboard, Keyboard, Safety…) are plugins too —
everything appears through the same discovery, nothing is hardcoded.

- **Permissions**: each plugin lists what it needs. You approve each one.
- **Settings**: plugins describe their settings; the app draws the form
  automatically (checkboxes, lists, sliders, buttons).
- **Commands**: plugins can add command-palette entries. They run with the
  same permissions as everything else.
- **Capabilities** (for authors): the named operations Core exposes
  (`window.move`, `filesystem.createFile`, `clipboard.copy`…). Plugins
  request; Core decides; the system adapter executes. Full table: see the
  in-app About page and the source registry (`core/pkg/intent`).

Repository credits: see the in-app About page (generated from
`third_party/*/ATTRIBUTION.md`, never hand-maintained).
