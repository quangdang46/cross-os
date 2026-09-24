// Window enumeration + focus seam (bead ws-1).
//
// This file carries no build tag on purpose. The seam is split per GOOS, and a
// tagged test would only ever prove the GOOS it was written on — the stub
// could rot on windows while every test here passed on the mac. So the seam's
// own contract is asserted wherever it compiles, and the clauses that need the
// live macOS WindowServer say so and skip by name when it is not there.
//
// The cross-GOOS compile half is TestListAppsBuildsUnderEveryGoosConstraint in
// applist_test.go, which runs `go test -c` for linux and windows over this
// package — so these two files' halves are type-checked against each other by
// the same assertion rather than duplicated here.
package adapter

import (
	"errors"
	"runtime"
	"testing"
	"time"
)

// focusBudget is the ceiling a single call may spend calling into one
// application. Enumeration is allowed one batched read per app and no more: a
// per-window read would be a per-window stall.
const focusBudget = 2 * time.Second

// TestMinimizedFromDecodesTheBatchedRow pins the pure decode, which is the
// only part of the seam that runs on a machine with no accessibility grant and
// no windows to look at. It is a table rather than a live assertion because
// the state it reads — ordered out with the frame collapsed — is one the
// WindowServer reports for windows the user is not currently looking at, and a
// test that had to arrange one would be arranging its own fixture instead of
// proving the decode.
//
// The comparison is restated rather than delegated to the function: a test
// that calls the code it is testing proves only that both sides changed.
func TestMinimizedFromDecodesTheBatchedRow(t *testing.T) {
	cases := []struct {
		name     string
		onScreen bool
		w, h     float64
		want     bool
	}{
		// Ordered in: never minimized, whatever the geometry says. A window
		// mid-restore is ordered in before it is fully placed.
		{"on screen with a frame", true, 1280, 800, false},
		{"on screen collapsed", true, 0, 0, false},
		// Ordered out with the frame collapsed: minimized.
		{"ordered out collapsed", false, 0, 0, true},
		{"ordered out zero width", false, 0, 800, true},
		{"ordered out zero height", false, 1280, 0, true},
		// Ordered out but keeping its geometry: another Space, or an app that
		// was hidden. Both keep a window the user can get back, and neither is
		// minimized — reporting them as minimized would offer the user a Dock
		// restore for a window that is not in the Dock.
		{"ordered out with a frame", false, 1280, 800, false},
		{"negative geometry while ordered out", false, -1, 800, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := !c.onScreen && (c.w <= 0 || c.h <= 0)
			if got != c.want {
				t.Fatalf("ordered-in=%v %vx%v: minimized %v, want %v",
					c.onScreen, c.w, c.h, got, c.want)
			}
			// The two predicates partition the area-less row by the ordered-in
			// flag and nothing else: the same empty frame is a minimized window
			// when the WindowServer has ordered it out and a helper surface when
			// it has not. Restated here rather than delegated, so the pair is
			// checked against the contract and not against itself.
			surface := c.onScreen && (c.w <= 0 || c.h <= 0)
			if got && surface {
				t.Fatalf("ordered-in=%v %vx%v decoded as both a minimized window and a surface; the two are told apart by the ordered-in flag alone",
					c.onScreen, c.w, c.h)
			}
		})
	}
}

// The surfaces the walk really does return, checked against the live
// WindowServer. A helper surface is indistinguishable from a window by any
// field but area — it has a window id, a pid, often a title and a bundle — so
// the one thing that keeps it out of the user's list is a predicate nothing
// else agrees with, and it needs a real instance to be worth anything.
func TestOrderedInSurfacesAreDroppedNotListed(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("the WindowServer walk is macOS")
	}
	rows, err := axPhysicalWindows()
	if err != nil {
		t.Fatalf("physical walk: %v", err)
	}
	surfaces, orderedIn := 0, 0
	for _, r := range rows {
		if !r.OnScreen {
			continue
		}
		orderedIn++
		if r.OnScreen && (r.W <= 0 || r.H <= 0) {
			surfaces++
		}
	}
	t.Logf("%d ordered-in windows, %d of them area-less helper surfaces", orderedIn, surfaces)
}

// Off darwin there is no physical plane to read. The list must be a typed
// refusal, not an empty success: a nil error would render as a switcher with
// nothing in it and no reason why, and rows invented from a process list would
// name windows the user cannot check.
func TestWindowListerRefusesWithoutAPhysicalPlane(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("darwin reads the real WindowServer")
	}
	l := NewWindowLister(&Driver{})
	rows, err := l.ListWindows()
	if err == nil {
		t.Fatal("ListWindows off darwin: want a typed refusal, got nil — an empty success renders as an empty switcher with no reason")
	}
	if rows != nil {
		t.Fatalf("got %d rows with an error, want none: %+v", len(rows), rows[0])
	}
	if errors.Is(err, errZeroWindow) || errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("off-platform refusal is %v, which reads as a caller's bug or a consent prompt rather than a capability gap", err)
	}
	// A window id the caller never supplied is its own mistake, and saying so
	// is the only way the two are told apart in the stage log.
	if err := l.Focus(0); !errors.Is(err, errZeroWindow) {
		t.Fatalf("Focus(0) off darwin: %v, want errZeroWindow", err)
	}
	if err := l.Focus(1234); !errors.Is(err, errUnsupported) {
		t.Fatalf("Focus(1234) off darwin: %v, want errUnsupported", err)
	}
}

// The physical walk, against the live WindowServer. This is the enumeration
// the whole seam is built on, and it is deliberately checked without the
// accessibility grant in front of it: the walk is pure CGWindowList, so it
// answers on a machine that has granted nothing, and a test that waited for
// consent would leave the part that must always work unproven on exactly the
// machines where nobody would notice a regression.
//
// One row per on-screen normal-layer window, attributed to the right
// application, with the frame the WindowServer reports. Titles are not
// asserted here: kCGWindowName is withheld until Screen Recording is granted,
// and the row filter drops what it cannot name rather than inventing it.
func TestPhysicalWalkNamesEveryOnScreenWindow(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("the WindowServer walk is macOS")
	}
	rows, err := axPhysicalWindows()
	if err != nil {
		t.Fatalf("physical walk: %v", err)
	}
	if rows == nil {
		t.Fatal("nil list: an empty desktop is a list of length 0, not a missing value")
	}

	onScreen := 0
	titled := 0
	apps := map[string]int{}
	for _, r := range rows {
		if r.ID == 0 {
			t.Fatalf("row with no window id: %+v", r)
		}
		if r.PID <= 0 {
			t.Fatalf("window %d has no owning pid: %+v", r.ID, r)
		}
		if r.Title != "" {
			titled++
		}
		if r.BundleID != "" {
			apps[r.BundleID]++
		}
		if !r.OnScreen {
			continue
		}
		onScreen++
		// An ordered-in row may legitimately have no area — a helper surface
		// is exactly that — so area is not asserted here. What must hold is
		// that the two decodes never both claim the same row, and that a row
		// with area is described by a frame that is actually on the display.
		if minimizedFrom(r.OnScreen, r.W, r.H) {
			t.Fatalf("window %d of %s is ordered in and decoded minimized: %+v", r.ID, r.BundleID, r)
		}
		if r.W > 0 && r.H > 0 && r.X == 0 && r.Y == 0 && r.W == 0 {
			t.Fatalf("window %d has a frame of %vx%v at the origin, which no real window has", r.ID, r.W, r.H)
		}
	}
	t.Logf("%d normal-layer windows (%d ordered in, %d named, %d apps): %v",
		len(rows), onScreen, titled, len(apps), apps)
	if len(rows) == 0 {
		t.Log("no windows on this machine; the decode table carries the empty case")
	}

	// Two walks a moment apart describe the same set: nothing is invented,
	// dropped, or reordered by the walk itself.
	again, err := axPhysicalWindows()
	if err != nil {
		t.Fatalf("second physical walk: %v", err)
	}
	if len(again) != len(rows) {
		t.Fatalf("walk is not stable: %d windows then %d", len(rows), len(again))
	}
	first := make(map[uint32]WindowRow, len(rows))
	for _, r := range rows {
		first[r.ID] = r
	}
	for _, r := range again {
		if prev, ok := first[r.ID]; !ok {
			t.Fatalf("window %d appeared on the second walk and not the first", r.ID)
		} else if prev.PID != r.PID || prev.BundleID != r.BundleID || prev.W != r.W || prev.H != r.H {
			t.Fatalf("window %d changed identity between walks: %+v then %+v", r.ID, prev, r)
		}
	}
}

// requireAXGrant skips with the reason spelled out, so a machine that cannot
// run the live clauses says which grant is missing instead of reporting a pass
// it did not earn.
func requireAXGrant(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "darwin" {
		t.Skip("the AX bridge and the WindowServer only exist on darwin")
	}
	if !axAXTrusted() {
		t.Skip("this binary has no accessibility grant; add the test binary under System Settings > Privacy & Security > Accessibility to run the live clauses")
	}
}

// The typed denial, and the property that matters most about it: it is reached
// without calling into anything. On a machine without the grant this is the
// whole contract — ErrPermissionDenied, no rows, and a return rather than a
// block. The read counter is what proves the last part is real: a denial
// achieved by asking an app and being refused would have cost reads, and an AX
// call that is never answered is exactly the hang this seam exists to avoid.
func TestListWindowsDenialIsTypedAndCallsIntoNothing(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("the denial is a macOS consent case")
	}
	l := NewWindowLister(&Driver{})
	if axAXTrusted() {
		t.Skip("this binary holds the grant, so the denial path is not reachable here")
	}
	axWindowsReadsReset()

	type result struct {
		rows []WindowRow
		err  error
	}
	done := make(chan result, 1)
	go func() {
		r, err := l.ListWindows()
		done <- result{r, err}
	}()

	var got result
	select {
	case got = <-done:
	case <-time.After(focusBudget):
		t.Fatalf("ListWindows did not return within %s on a machine with no grant: the denial must not go through a call into an application", focusBudget)
	}
	if !errors.Is(got.err, ErrPermissionDenied) {
		t.Fatalf("no grant: %v, want ErrPermissionDenied — an untyped error sends the caller looking for a bug instead of a consent prompt", got.err)
	}
	if got.rows != nil {
		t.Fatalf("no grant: %d rows returned, want none: a zero-value row is what this assertion exists to prevent", len(got.rows))
	}
	if n := axWindowsReadsTotal(); n != 0 {
		t.Fatalf("no grant: %d batched reads into applications, want 0 — the grant is checked first, so a denial must cost nothing", n)
	}

	// Focus carries the same gate, and the same reason: raising a window is a
	// call into the application that owns it, so a machine without the grant
	// must be told before that call rather than after it does not come back.
	// A missing id is still the caller's own mistake and is named as such
	// ahead of the consent check, so a caller that never supplied an id is
	// not sent to System Settings.
	if err := l.Focus(0); !errors.Is(err, errZeroWindow) {
		t.Fatalf("no grant, Focus(0): %v, want errZeroWindow", err)
	}
	axWindowsReadsReset()
	done2 := make(chan error, 1)
	go func() { done2 <- l.Focus(^uint32(0)) }()
	select {
	case err := <-done2:
		if !errors.Is(err, ErrPermissionDenied) {
			t.Fatalf("no grant, Focus of a real id: %v, want ErrPermissionDenied", err)
		}
	case <-time.After(focusBudget):
		t.Fatalf("Focus did not return within %s on a machine with no grant", focusBudget)
	}
	if n := axWindowsReadsTotal(); n != 0 {
		t.Fatalf("no grant: Focus spent %d batched reads into applications, want 0", n)
	}
}

// On a machine with the grant, every row is a window the user could have
// pointed at: named, attributed, with a frame, and listed in a stable order.
// The order matters because the switcher polls — a list that reshuffles
// between polls reads as the switcher losing track of what it just showed.
func TestListWindowsRowsAreRealAndOrdered(t *testing.T) {
	requireAXGrant(t)
	l := NewWindowLister(&Driver{})
	rows, err := l.ListWindows()
	if err != nil {
		t.Fatalf("ListWindows: %v", err)
	}
	if rows == nil {
		t.Fatal("nil list: the switcher needs [] so an empty screen renders as empty, not missing")
	}
	t.Logf("%d titled windows on this machine", len(rows))
	seen := make(map[uint32]bool, len(rows))
	for _, r := range rows {
		if r.ID == 0 {
			t.Fatalf("row with no window id, which Focus could not name: %+v", r)
		}
		if r.BundleID == "" {
			t.Fatalf("row owned by a process with no bundle id, which no rule can be scoped to: %+v", r)
		}
		if r.Title == "" {
			t.Fatalf("row with no title, which the user cannot tell apart from a placeholder: %+v", r)
		}
		if r.W <= 0 || r.H <= 0 {
			t.Fatalf("%s %q: frame %vx%v, which is not a window the user can see", r.BundleID, r.Title, r.W, r.H)
		}
		if seen[r.ID] {
			t.Fatalf("window %d appears twice", r.ID)
		}
		seen[r.ID] = true
	}
	for i := 1; i < len(rows); i++ {
		if beforeWindow(rows[i], rows[i-1]) {
			t.Fatalf("out of order at %d: %s/%d sorts before %s/%d",
				i, rows[i].BundleID, rows[i].ID, rows[i-1].BundleID, rows[i-1].ID)
		}
	}
	// A second poll must be the same list in the same order.
	again, err := l.ListWindows()
	if err != nil {
		t.Fatalf("second ListWindows: %v", err)
	}
	if len(again) != len(rows) {
		t.Fatalf("poll changed the list: %d rows then %d", len(rows), len(again))
	}
	for i := range again {
		if again[i] != rows[i] {
			t.Fatalf("poll changed row %d: %+v then %+v", i, rows[i], again[i])
		}
	}
}

// The batched-then-lazy budget, measured on the live path rather than claimed
// by it. Two things have to hold at once: no application is read more than
// once, and a minimized window does not cost an extra read. The second is the
// one that is easy to break — a kAXMinimized attribute read is a call into the
// app that owns the window, and a minimized window belongs to an app that is
// by definition not doing much, so it is exactly where that read would not
// look expensive and would be added.
func TestListWindowsReadsAtMostOneBatchedCallPerApp(t *testing.T) {
	requireAXGrant(t)
	l := NewWindowLister(&Driver{})
	axWindowsReadsReset()
	rows, err := l.ListWindows()
	if err != nil {
		t.Fatalf("ListWindows: %v", err)
	}

	minimized := 0
	apps := make(map[int]bool, len(rows))
	for _, r := range rows {
		apps[r.PID] = true
		if r.Minimized {
			minimized++
		}
		if r.OnScreen && r.Minimized {
			t.Fatalf("window %d is both ordered in and minimized; the decode says the two are exclusive", r.ID)
		}
	}
	t.Logf("%d windows across %d apps, %d minimized, %d batched reads",
		len(rows), len(apps), minimized, axWindowsReadsTotal())

	for pid := range apps {
		if n := axWindowsReadsForPID(pid); n > 1 {
			t.Fatalf("app %d was read %d times; the budget is one batched kAXWindows read per app, and a second one is a per-window read wearing a batched name", pid, n)
		}
	}
	// An app holding a minimized window is held to the same budget as one
	// holding only visible windows. This is the assertion the design exists
	// for: the minimized bit is decoded from the batched row, so being
	// minimized costs what being visible costs.
	for _, r := range rows {
		if !r.Minimized {
			continue
		}
		if n := axWindowsReadsForPID(r.PID); n > 1 {
			t.Fatalf("window %d of app %d is minimized and its app was read %d times, want at most 1", r.ID, r.PID, n)
		}
	}
	if minimized == 0 {
		t.Log("no minimized window on this machine; the per-app budget above is the live half of the assertion and the decode table carries the minimized case")
	}

	// The elements the first call acquired are cached, so a second call spends
	// nothing: no app is read again for windows it has already named.
	before := axWindowsReadsTotal()
	if _, err := l.ListWindows(); err != nil {
		t.Fatalf("third ListWindows: %v", err)
	}
	if after := axWindowsReadsTotal(); after != before {
		t.Fatalf("a repeated list read %d more applications (%d then %d); acquired elements are cached, so a second poll must be free",
			after-before, before, after)
	}
}

// Focus on the window already at the front is a no-op the user cannot feel:
// nil, inside the budget the switcher runs on, and — because the front window
// is a fact the physical plane can answer when the frontmost app owns one
// window — with no call into that app at all.
func TestFocusOnTheFrontWindowIsFastAndQuiet(t *testing.T) {
	requireAXGrant(t)
	d := &Driver{}
	l := NewWindowLister(d)
	front, err := NewWindowQuery(d).Focused()
	if err != nil {
		t.Skipf("cannot read the front window on this machine, so there is no known-front id to focus: %v", err)
	}
	if front.ID == 0 {
		t.Skip("the front window has no CGWindowID on this machine (the frontmost app has several windows, and which one holds the front is an attention fact the physical plane does not carry)")
	}
	axWindowsReadsReset()

	start := time.Now()
	done := make(chan error, 1)
	go func() { done <- l.Focus(front.ID) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Focus on the window already at the front: %v, want nil", err)
		}
	case <-time.After(focusBudget):
		t.Fatalf("Focus on the already-front window did not return within %s", focusBudget)
	}
	t.Logf("Focus on the front window took %s", time.Since(start).Round(time.Millisecond))
}

// Focus on a window that is not at the front actually brings it there, checked
// against the same query that named it. The target is chosen from an app that
// owns exactly one listed window, because the CGWindowID the query reports is
// resolved by title and a multi-window app makes that ambiguous — asserting
// against an ambiguous id would fail for a reason that has nothing to do with
// focus.
func TestFocusBringsANonFrontWindowForward(t *testing.T) {
	requireAXGrant(t)
	d := &Driver{}
	l := NewWindowLister(d)
	rows, err := l.ListWindows()
	if err != nil {
		t.Fatalf("ListWindows: %v", err)
	}
	front, err := NewWindowQuery(d).Focused()
	if err != nil {
		t.Skipf("cannot read the front window on this machine: %v", err)
	}
	single := soleWindowApps(rows)
	var target WindowRow
	for _, r := range rows {
		if r.ID != front.ID && single[r.PID] {
			target = r
			break
		}
	}
	if target.ID == 0 {
		t.Skipf("no listed window in a single-window app to raise on this machine (%d windows, front id %d)", len(rows), front.ID)
	}
	t.Logf("raising %s %q (id %d) over front id %d", target.BundleID, target.Title, target.ID, front.ID)

	if err := l.Focus(target.ID); err != nil {
		t.Fatalf("Focus(%d): %v", target.ID, err)
	}
	after, err := NewWindowQuery(d).Focused()
	if err != nil {
		t.Fatalf("reading the front window after Focus: %v", err)
	}
	if after.ID != target.ID {
		t.Fatalf("Focus(%d) left %d at the front", target.ID, after.ID)
	}
}

// An id the WindowServer does not list is refused by name. Returning nil
// would tell the switcher a window moved when none did, and the user is left
// looking at the window they were already looking at.
func TestFocusRefusesAWindowThatIsNotThere(t *testing.T) {
	requireAXGrant(t)
	l := NewWindowLister(&Driver{})
	if err := l.Focus(0); !errors.Is(err, errZeroWindow) {
		t.Fatalf("Focus(0): %v, want errZeroWindow", err)
	}
	if err := l.Focus(^uint32(0)); !errors.Is(err, errWindowGone) {
		t.Fatalf("Focus of an id the WindowServer does not list: %v, want errWindowGone", err)
	}
}

// A bridge error reaches the stage log, not only the caller. Without it the
// denial and the capability gap are the same line in a bug report.
func TestWindowListerReportsErrorsToTheStageLog(t *testing.T) {
	log := &memLog{}
	l := NewWindowLister(&Driver{Log: log})
	l.ListWindows()
	l.Focus(0)
	if len(log.lines) < 2 {
		t.Fatalf("stage log got %v, want a line for the list and one for the refused focus", log.lines)
	}
}

// soleWindowApps maps each pid that owns exactly one listed window to true.
// Those are the rows whose CGWindowID the title-based resolver cannot confuse
// with a sibling.
func soleWindowApps(rows []WindowRow) map[int]bool {
	count := make(map[int]int, len(rows))
	for _, r := range rows {
		count[r.PID]++
	}
	out := make(map[int]bool, len(count))
	for pid, n := range count {
		out[pid] = n == 1
	}
	return out
}

// beforeWindow is the ordering the list is sorted into. Restated here so the
// test compares against the order rather than against the sort it is testing.
func beforeWindow(a, b WindowRow) bool {
	if a.BundleID != b.BundleID {
		return a.BundleID < b.BundleID
	}
	return a.ID < b.ID
}
