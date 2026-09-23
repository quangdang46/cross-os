//go:build darwin

// Per-platform tap defaults (bead cross-os-2io). macOS owns the real
// CGEventTap; the daemon seam in main.go (tapStartFn/tapStopFn/
// tapLiveStatus) binds to these so the daemon code stays platform-free.
package main

import (
	"crossos/core/internal/adapter"
)

var tapStartLive = func(d *adapter.Driver) error { return adapter.TapStart(d) }

var tapStopLive = func(d *adapter.Driver) error { return adapter.TapStop(d) }

var tapLiveLive = func() bool { return adapter.TapLive() }

// unhealthyTap reports whether the live tap escalated after repeated
// timeout-disables (audit blocker: a silently dead tap used to leave
// remapping broken with no signal).
func unhealthyTap() bool { return adapter.TapUnhealthy() }
