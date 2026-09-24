//go:build !darwin

// Window enumeration and focus off macOS (bead ws-1).
//
// The methods live here, on both non-darwin seam types, because the split is
// the one the rest of the package already uses: a build-tagged file per GOOS,
// and a signature that changes on one side breaking the other at compile time
// rather than at run time. Neither platform has an equivalent of the macOS
// physical plane to read, and inventing one would produce rows the user cannot
// check — an off-Space window, a window on a screen this process cannot see,
// and a window that no longer exists all look identical from a process list,
// and an id minted from that list would focus the wrong window.
//
// So the list is empty and the answer is a typed refusal, which is a shape the
// caller already handles: it serves stale or empty cache on a permission
// denial, and a capability gap reported as one is a gap the shell can name.
package adapter

// otherLister is the enumeration + focus seam for every platform without the
// macOS physical plane.
type otherLister struct{ driver *Driver }

// NewWindowLister returns the enumeration + focus seam for this platform.
func NewWindowLister(d *Driver) WindowLister { return &otherLister{driver: d} }

// ListWindows returns no rows and a typed refusal. Never a fabricated row and
// never a nil success, which would render as a switcher with nothing in it and
// no reason why.
func (l *otherLister) ListWindows() ([]WindowRow, error) {
	return nil, l.driver.ReportError("query", errUnsupported)
}

// Focus refuses too, and refuses a missing id differently: "no id was given" is
// a caller's bug and "this platform cannot" is a capability gap, and the stage
// log is the only place that difference is visible.
func (l *otherLister) Focus(id uint32) error {
	if id == 0 {
		return l.driver.ReportError("focus", errZeroWindow)
	}
	return l.driver.ReportError("focus", errUnsupported)
}

// The batched-acquisition budget has no counter off darwin, because there is no
// AX bridge to count. Zero is the honest reading: no application was read.
// These exist so windowlist_test.go can stay untagged and assert the live
// property on the platform that has a bridge, without failing to compile on the
// platforms that do not.
func axAXTrusted() bool                { return false }
func axWindowsReadsTotal() int         { return 0 }
func axWindowsReadsForPID(pid int) int { return 0 }
func axWindowsReadsReset()             {}

// axPhysicalWindows has no counterpart off darwin: there is no WindowServer
// walk to expose, and the shim answers with the capability gap rather than an
// empty list so a caller that reaches for it cannot mistake silence for a
// machine with no windows.
func axPhysicalWindows() ([]WindowRow, error) { return nil, errUnsupported }
