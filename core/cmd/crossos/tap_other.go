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

// postChordLive has nothing to post through off macOS: no HID source to
// stand in for the machine's own keyboard. The refusal is the same typed
// offer as the tap's — ship a no-op and clipboard.copy would lie by
// succeeding on every platform and doing the work on none.
var postChordLive = func(_ uint16) error { return errTapUnsupported }
