//go:build darwin

// Window enumeration and focus on macOS (bead ws-1).
//
// The list is built from the physical plane. CGWindowList answers existence,
// geometry, ordered-in and the owning pid for every window in one batched
// call, and that call cannot be delayed by any application — which is the
// whole reason enumeration is not an AX sweep. An AX attribute is a call INTO
// the owning application: it costs that application as much as it is busy,
// and nothing outside can bound it. An app that stops answering takes an
// AX-driven list down with it; a WindowServer walk is still answering.
//
// The semantic plane — which window inside a process is the key one, what its
// real title is — is read lazily, and only where the physical plane has no
// answer. That is one batched kAXWindows read for the whole app, not one read
// per window, and the elements it returns are cached so a second call is free.
// The budget is measured rather than asserted: axWindowsReadsForPID is what
// the test checks the "minimized costs no extra read" property against.
//
// A window's minimized state is decoded from the batched row it already
// carries (see minimizedFrom), never from a kAXMinimized read into the owning
// app. That single substitution is what keeps enumeration non-blocking: it was
// the one per-window attribute read left in the design, and it was the one
// that stalled on a wedged app.
package adapter

import (
	"sort"
)

// darwinLister is the macOS enumeration + focus seam.
type darwinLister struct{ driver *Driver }

// NewWindowLister returns the enumeration + focus seam for this platform.
func NewWindowLister(d *Driver) WindowLister { return &darwinLister{driver: d} }

// ListWindows returns one row per on-screen normal-layer window that has a
// title and an owning bundle id.
//
// A missing accessibility grant is ErrPermissionDenied and costs one boolean:
// the grant is checked before any call into another application, so a machine
// without it returns a typed error immediately rather than a list of zero-value
// rows, and never blocks on a call that will not be answered.
func (q *darwinLister) ListWindows() ([]WindowRow, error) {
	if !axAXTrusted() {
		return nil, q.driver.ReportError("query", ErrPermissionDenied)
	}
	rows, err := axListWindows()
	if err != nil {
		return nil, q.driver.ReportError("query", err)
	}
	for i := range rows {
		rows[i].Minimized = minimizedFrom(rows[i].OnScreen, rows[i].W, rows[i].H)
	}
	if err := q.fillTitles(rows); err != nil {
		return nil, q.driver.ReportError("query", err)
	}

	out := rows[:0]
	for _, r := range rows {
		// No bundle id: a process the workspace does not name is a process no
		// rule can be scoped to, the same call ListApps makes. No title: a row
		// with a placeholder would sit in the user's list looking exactly like
		// a real one, and there is no way to tell them apart from there. No
		// area: the window is a helper surface, and there is nothing for the
		// user to aim at.
		if r.BundleID == "" || r.Title == "" || isSurface(r.OnScreen, r.W, r.H) {
			continue
		}
		// The list arrives front to back, so the row's position HERE is its
		// MRU rank. It is recorded before the sort below, because the sort
		// is a stability measure and a stable order is not a focus order.
		r.MRU = len(out)
		out = append(out, r)
	}
	// The window switcher polls this. A list that reshuffles between polls
	// reads as the switcher losing track of the windows it just showed, so the
	// order is fixed: bundle id, then window id.
	//
	// **The focus order travels beside the list, not inside it.** The
	// switcher reads its current window as the FIRST row — "the current
	// window is the front tile whenever the list has one", and the default
	// pick steps over it to reach the window you were on before. A list
	// sorted by bundle id has no front window in it, so the tile the user
	// was on was drawn in the middle of the list and the pick stepped over
	// the wrong row. The MRU rank above is what carries that, and the
	// switcher orders by it.
	sort.Slice(out, func(i, j int) bool {
		if out[i].BundleID != out[j].BundleID {
			return out[i].BundleID < out[j].BundleID
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// fillTitles names the windows the physical plane left unnamed, spending at
// most one batched read per application. Screen Recording already answers
// kCGWindowName on a machine that granted it, in which case there is nothing
// left to ask and this loop never runs.
//
// **One application's failure is that application's failure.** A single app
// whose Accessibility server answers badly used to abort the whole list, and
// the Switcher page showed "Could not list windows." with an empty list under
// it — every other app's windows thrown away because one of them misbehaved.
// A window list that is missing ONE app's windows is a far better answer
// than a window list that is empty, so a failure here is recorded and the
// walk continues.
//
// The error is still returned when NO application could be named, because
// that is a fact about the machine and not about one app.
func (q *darwinLister) fillTitles(rows []WindowRow) error {
	byApp := make(map[int][]uint32)
	frames := make(map[int][][4]float64)
	var order []int
	for _, r := range rows {
		if r.Title != "" {
			continue
		}
		if _, seen := byApp[r.PID]; !seen {
			order = append(order, r.PID)
		}
		byApp[r.PID] = append(byApp[r.PID], r.ID)
		frames[r.PID] = append(frames[r.PID], [4]float64{r.X, r.Y, r.W, r.H})
	}
	// Sorted so the order applications are read in is itself fixed; the count
	// is per application either way, but an unstable order would make a
	// failure here unreproducible.
	sort.Ints(order)
	var firstErr error
	named := 0
	for _, pid := range order {
		titles, err := axAppTitles(pid, byApp[pid], frames[pid])
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		named++
		for i := range rows {
			if rows[i].PID == pid {
				if t, ok := titles[rows[i].ID]; ok {
					rows[i].Title = t
				}
			}
		}
	}
	if firstErr != nil && named == 0 {
		return firstErr
	}
	return nil
}

// Focus brings the window named by id to the front.
//
// A window that is already at the front is answered from the physical plane
// alone and returns nil without touching the application that owns it, so the
// common case — a rule that re-raises what the user is already looking at —
// costs no AX call and cannot stall. Which of an application's several windows
// holds the front is an order fact the physical plane does not carry, so the
// cheap path declines whenever there is more than one and defers to AX.
func (q *darwinLister) Focus(id uint32) error {
	if id == 0 {
		return q.driver.ReportError("focus", errZeroWindow)
	}
	if !axAXTrusted() {
		return q.driver.ReportError("focus", ErrPermissionDenied)
	}
	if axFrontWindowID() == id {
		return nil
	}
	rows, err := axListWindows()
	if err != nil {
		return q.driver.ReportError("focus", err)
	}
	for _, r := range rows {
		if r.ID == id {
			if r.BundleID == "" {
				// Attributable to no application: there is nothing to bring
				// forward, and acting on the pid alone would raise a process
				// the caller could not have named.
				return q.driver.ReportError("focus", errWindowGone)
			}
			return q.driver.ReportError("focus",
				axFocusWindow(r.PID, [4]float64{r.X, r.Y, r.W, r.H}))
		}
	}
	return q.driver.ReportError("focus", errWindowGone)
}

// axPhysicalWindows is the batched walk on its own, without the grant gate
// ListWindows puts in front of it. It exists so the physical plane can be
// checked against the live WindowServer on a machine that has not granted
// accessibility: the walk is the part that must work everywhere, and gating
// its only test behind a consent prompt would leave it unproven on exactly the
// machines where a regression would be hardest to notice.
func axPhysicalWindows() ([]WindowRow, error) { return axListWindows() }
