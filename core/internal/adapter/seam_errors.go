package adapter

import "errors"

// errTapNotWired marks the native bridge not yet linked (spikes proved the
// shape; the C-ABI/DLL wiring lands behind this error, never as silent pass).
var errTapNotWired = errors.New("adapter: native bridge not yet wired (spike-proven shape, link pending)")

// errZeroWindow rejects move/resize against a zero window id pre-bridge.
var errZeroWindow = errors.New("adapter: zero window id")
