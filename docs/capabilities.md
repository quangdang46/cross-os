# Capabilities registry (overview)

User-facing overview. Authors: the normative table is the source registry
(`core/pkg/intent`), surfaced in-app on the About page — this file never
duplicates capability IDs.

- **Windows**: move, resize, minimize, maximize, close, read focused info
- **Clipboard**: copy, copy path, read, write
- **Files**: create file/folder, move to trash, read
- **Apps & terminal**: launch, open, open terminal here
- **Finder**: context-menu items (macOS)
- **Input**: observe, intercept (explicit approval only)

Every capability needs a permission you approve per plugin. UI-declared
actions run through the same checks as everything else — UI gets no
special access.
