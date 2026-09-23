//go:build !darwin

// Non-macOS tap defaults (bead cross-os-2io): no live keyboard tap here, so
// the daemon reports interception off with a typed reason instead of silently
// pretending. Windows interception is the crossos-keyboard-win.dll seam
// (bead cross-os-ab4); this keeps the daemon compiling and honest elsewhere.
package main

import (
	"errors"

	"crossos/core/internal/adapter"
)

var errTapUnsupported = errors.New("keyboard interception is not implemented on this platform yet")

var tapStartLive = func(d *adapter.Driver) error { return errTapUnsupported }

var tapStopLive = func(d *adapter.Driver) error { return nil }

var tapLiveLive = func() bool { return false }

// unhealthyTap is a no-op where there is no live tap to disable.
func unhealthyTap() bool { return false }

// droppedDispatches is always zero where there is no live tap.
func droppedDispatches() int64 { return 0 }
