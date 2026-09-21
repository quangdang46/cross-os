# Windows Shell Context Menu Backlog (tracked, not implemented)

Bead: cross-os-qhp.4. Plan: §4.2, §6.3, §8, §10 Phase 3 scope note.

Rule: no implementation until Phase 5 ships. Phase 3 scopes Finder UX
only — this note holds the Windows surface so it is not silently dropped.

## Design (for the future implementation bead)

- Host: IContextMenu / IShellExtInit handler = the Windows equivalent of
  the Finder Sync extension (§4.2).
- Parity: every finder.menu item (§6.3 table) gets a Windows host executing
  the SAME native capabilities (filesystem.createFile, clipboard.copyPath,
  …) — one capability set, two hosts.
- Constraints (same as Finder, §8): minimal IPC client; daemon-down
  graceful fallback (hide, never block Explorer); requests validated
  against local config (§3.7); registry-based install with ownership
  metadata + rollback (§8).
- Gate: menu → Unix-socket/named-pipe → daemon-down-hide proven (same bar
  as Spike D) before productization.

(Gate signal: the Phase 5 distribution bead closing — until then this doc is read-only.)
