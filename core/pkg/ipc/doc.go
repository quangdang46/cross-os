// Package ipc implements the CrossOS JSON-RPC 2.0 IPC server.
//
// Bead: cross-os-090. Plan: COMPREHENSIVE_PLAN.md §3.9.
//
// Transport: Unix socket (macOS/Linux), named pipe (Windows). Framing:
// newline-delimited JSON-RPC 2.0 (one object per line). Methods: UI ↔ Core
// status/control, script plugins ↔ Core, Finder Sync extension ↔ Core — one
// protocol for all three, locked here before the shell scaffold and Level B
// runtime (which must not invent divergent ad-hoc protocols).
//
// Security: the socket/pipe admits the owning user only (Unix file mode
// 0600 on the socket dir; Windows named-pipe ACL restricted to the user).
// Another local user cannot inject commands.
package ipc
