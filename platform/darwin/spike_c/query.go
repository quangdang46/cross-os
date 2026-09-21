// Query result types + field-by-field validation (bead criterion 1).
//
// The live darwin seam (query_darwin.go) fills these from NSWorkspace (bundle
// ID + PID) + AXUIElement (title, role, frame) + CGWindowList (window ID);
// the Windows seam (query_windows.go) from GetForegroundWindow/GetWindowText.
// Validation is shared and pure so Tier-1 tests assert the contract exactly.
package spikec

import "fmt"

// Frame is a window rectangle in screen coordinates.
type Frame struct {
	X, Y, W, H float64
}

// AppInfo identifies the focused application.
type AppInfo struct {
	// BundleID on macOS (e.g. com.apple.Finder), executable name on Windows.
	BundleID string
	PID      int
}

// WindowInfo describes the focused window.
type WindowInfo struct {
	Title string
	Role  string // AX role, e.g. AXWindow
	ID    uint32 // CGWindowID / HWND (truncated)
	Frame Frame
}

// Focused describes the focused app + window snapshot. This is the shape the
// FastContext cache (bead cross-os-jiz) will serve from — the spike proves
// the query; jiz proves the cache around it.
type Focused struct {
	App    AppInfo
	Window WindowInfo
}

// Validate asserts every field the bead requires, returning one error per
// missing/invalid field so failures name the gap (never a bare "invalid").
func (f Focused) Validate() []error {
	var errs []error
	if f.App.BundleID == "" {
		errs = append(errs, fmt.Errorf("app.bundleID: empty"))
	}
	if f.App.PID <= 0 {
		errs = append(errs, fmt.Errorf("app.pid: got %d, want >0", f.App.PID))
	}
	if f.Window.Title == "" {
		errs = append(errs, fmt.Errorf("window.title: empty"))
	}
	if f.Window.Role == "" {
		errs = append(errs, fmt.Errorf("window.role: empty"))
	}
	if f.Window.ID == 0 {
		errs = append(errs, fmt.Errorf("window.id: zero"))
	}
	if f.Window.Frame.W <= 0 || f.Window.Frame.H <= 0 {
		errs = append(errs, fmt.Errorf("window.frame: non-positive size %+v", f.Window.Frame))
	}
	return errs
}

// Valid reports whether the snapshot passes validation.
func (f Focused) Valid() bool { return len(f.Validate()) == 0 }
