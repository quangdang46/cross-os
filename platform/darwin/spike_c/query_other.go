//go:build !darwin

// Non-darwin stub: Tier-2 live AX tests are darwin-only; Windows UIA lives in
// platform/windows (bead cross-os-lla covers both OSes, UIA side deferred to
// the windows seam under cross-os-ab4).
//
// NOTE: the live QueryFn/MoveFn/Query seams live in query_darwin.go, which
// carries the union tag `//go:build darwin || !darwin` so Tier-1 tests
// (validation, error mapping) run on every platform. The bodies are
// consent-denied defaults — safe anywhere, since real AX calls land behind
// the ab4 C-ABI bridge. Filenames are swapped relative to content history;
// tags are authoritative, names cosmetic.
package spikec

// QueryFn fills a Focused snapshot or returns a typed error (default:
// consent-denied, exercisable without TCC).
var QueryFn = func() (Focused, error) {
	return Focused{}, &AXError{Op: "query:focused", Code: axAPIDisabled}
}

// MoveFn applies a validated MoveResize or returns a typed error.
var MoveFn = func(m MoveResize) error {
	if err := m.Validate(); err != nil {
		return err
	}
	return &AXError{Op: "setAttribute:AXPosition", Code: axAPIDisabled}
}

// Query is the spike entry point: run the seam and validate field-by-field.
func Query() (Focused, []error) {
	f, err := QueryFn()
	if err != nil {
		return f, []error{err}
	}
	return f, f.Validate()
}
