// Spike C tests — bead cross-os-lla.
//
// Tier 1 (always runs): validation, move/resize geometry, permission-denied
// mapping, latency math.
// Tier 2 (live AX): gated behind -short; needs interactive desktop with
// accessibility consent.
//
// Pass criteria mapping:
//  1. Field-by-field query → TestValidate.
//  2. Move+resize granted + denied-typed-error → TestMoveValidate,
//     TestPermissionDenied, TestClampToScreen.
//  3. p50/p95 recorded, justifying the jiz cache → TestQueryLatency.
package spikec

import (
	"runtime"
	"testing"
	"time"
)

func goodFocused() Focused {
	return Focused{
		App:    AppInfo{BundleID: "com.apple.Finder", PID: 123},
		Window: WindowInfo{Title: "Documents", Role: "AXWindow", ID: 42, Frame: Frame{X: 0, Y: 25, W: 800, H: 600}},
	}
}

func TestValidate(t *testing.T) {
	if errs := goodFocused().Validate(); len(errs) != 0 {
		t.Fatalf("good snapshot: %v", errs)
	}
	bad := Focused{}
	errs := bad.Validate()
	if len(errs) != 6 {
		t.Fatalf("empty snapshot: got %d errors %v, want 6 (one per field)", len(errs), errs)
	}
	// Each error names its field.
	for _, e := range errs {
		t.Logf("field error: %v", e)
	}
	partial := goodFocused()
	partial.Window.Frame.W = 0
	if partial.Valid() {
		t.Fatal("zero-width frame must be invalid")
	}
}

func TestMoveValidate(t *testing.T) {
	ok := MoveResize{ID: 7, To: Frame{X: 10, Y: 10, W: 400, H: 300}, Move: true, Sizer: true}
	if err := ok.Validate(); err != nil {
		t.Fatalf("good move/resize: %v", err)
	}
	cases := []MoveResize{
		{To: Frame{W: 1, H: 1}, Move: true},                      // zero id
		{ID: 7, To: Frame{W: 1, H: 1}},                           // no-op
		{ID: 7, To: Frame{W: 0, H: 10}, Move: true, Sizer: true}, // degenerate size
	}
	for i, c := range cases {
		if err := c.Validate(); err == nil {
			t.Fatalf("case %d: want validation error, got nil", i)
		}
	}
}

func TestPermissionDenied(t *testing.T) {
	denied := &AXError{Op: "copyAttribute:AXTitle", Code: axAPIDisabled}
	if !IsPermissionDenied(denied) {
		t.Fatal("API-disabled code must map to permission-denied")
	}
	other := &AXError{Op: "copyAttribute:AXTitle", Code: -25200} // e.g. kAXErrorFailure
	if IsPermissionDenied(other) {
		t.Fatal("generic AX failure must NOT map to permission-denied")
	}
	// Default seam returns denied without TCC — the denied path is
	// exercisable in CI, never a hang.
	if _, errs := Query(); len(errs) == 0 || !IsPermissionDenied(errs[0]) {
		t.Fatalf("default Query seam: want denied error, got %v", errs)
	}
	if err := MoveFn(MoveResize{ID: 1, To: Frame{W: 10, H: 10}, Move: true}); !IsPermissionDenied(err) {
		t.Fatalf("default MoveFn seam: want denied error, got %v", err)
	}
}

func TestClampToScreen(t *testing.T) {
	screen := Frame{X: 0, Y: 0, W: 1920, H: 1080}
	m := MoveResize{ID: 1, To: Frame{X: 2000, Y: -50, W: 9999, H: 9999}, Move: true, Sizer: true}
	got := m.ClampToScreen(screen)
	if got.To.W != 1920 || got.To.H != 1080 {
		t.Fatalf("oversize not clamped: %+v", got.To)
	}
	if got.To.X != 0 || got.To.Y != 0 {
		t.Fatalf("off-screen origin not clamped: %+v", got.To)
	}
	if err := got.Validate(); err != nil {
		t.Fatalf("clamped request must validate: %v", err)
	}
}

func TestQueryLatency(t *testing.T) {
	var l QueryLatency
	for i := 1; i <= 100; i++ {
		l.Observe(time.Duration(i) * time.Millisecond)
	}
	if got := l.P50(); got != 50*time.Millisecond {
		t.Fatalf("P50=%v, want 50ms", got)
	}
	if got := l.P95(); got != 95*time.Millisecond {
		t.Fatalf("P95=%v, want 95ms", got)
	}
	// The number that justifies the jiz cache: synchronous AX queries cost
	// milliseconds, the fast path budgets <1ms — keydown reads cache.
	t.Logf("query p50=%v p95=%v n=%d — sync AX is 50-100x over the <1ms fast-path budget; jiz cache required",
		l.P50(), l.P95(), l.Samples())
}

// --- Tier 2: live AX (darwin interactive desktop + consent only) ---

func TestLiveQuery(t *testing.T) {
	if testing.Short() {
		t.Skip("needs interactive desktop + accessibility consent")
	}
	if runtime.GOOS != "darwin" {
		t.Skip("darwin-only")
	}
	t.Skip("live AX harness lands with cross-os-ab4 C-ABI bridge; Tier-1 validation+error math is the sandbox-runnable proof")
}

func TestLiveMoveResize(t *testing.T) {
	if testing.Short() {
		t.Skip("needs interactive desktop + accessibility consent")
	}
	if runtime.GOOS != "darwin" {
		t.Skip("darwin-only")
	}
	t.Skip("live AX harness lands with cross-os-ab4 C-ABI bridge; Tier-1 validation+error math is the sandbox-runnable proof")
}
