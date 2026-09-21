//go:build darwin

// Live AX query + move/resize seam (darwin-only).
//
// Shape (Rectangle AXExtension.swift / yabai window_manager.c):
//
//	app: NSWorkspace.shared.frontmostApplication (bundleIdentifier, processIdentifier)
//	window: AXUIElement focused window of the app PID
//	  (kAXFocusedWindowAttribute) → title (kAXTitleAttribute),
//	  role (kAXRoleAttribute), frame (kAXPositionAttribute + kAXSizeAttribute)
//	id: CGWindowList entry matching the PID + bounds (window number)
//	move/resize: AXUIElementSetAttributeValue(kAXPositionAttribute /
//	  kAXSizeAttribute) with AXValue CGPoint/CGSize; failures return *AXError.
//
// CGO is NOT wired in this spike (same reason as spike A: runtime TCC
// consent unavailable in sandbox/CI). The QueryFn/MoveFn seams are where the
// ab4 C-ABI bridge plugs in; tests exercise validation + error mapping.
package spikec

// QueryFn fills a Focused snapshot or returns a typed error. The product
// implementation calls NSWorkspace + AXUIElement + CGWindowList; the spike
// default returns the consent-denied error so the denied path is exercisable
// without TCC.
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
