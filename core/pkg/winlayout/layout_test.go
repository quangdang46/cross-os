package winlayout

import "testing"

// 1920x1080 visible area (menu bar + Dock already excluded by caller).
func vis() Rect { return Rect{0, 0, 1920, 1080} }

func TestHalvesTileWithoutGaps(t *testing.T) {
	v := vis()
	l, r := Frame(LeftHalf, v), Frame(RightHalf, v)
	if l.W+r.W != v.W || l.X != 0 || r.X != l.W {
		t.Fatalf("halves don't tile: %+v %+v", l, r)
	}
	// Nudge convention: floor left, remainder right.
	if l.W != 960 || r.W != 960 {
		t.Fatalf("half widths: %v %v", l.W, r.W)
	}
}

func TestThirdsCover(t *testing.T) {
	v := vis()
	a, b, c := Frame(LeftThird, v), Frame(CenterThird, v), Frame(RightThird, v)
	if a.W+b.W+c.W != v.W {
		t.Fatalf("thirds don't cover: %v %v %v", a.W, b.W, c.W)
	}
	if a.X != 0 || b.X != a.W || c.X != a.W+b.W {
		t.Fatalf("thirds misaligned: %+v %+v %+v", a, b, c)
	}
}

func TestQuartersAnchor(t *testing.T) {
	v := vis()
	tl := Frame(TopLeft, v)
	// Y-orientation (review: cross-os-c0): Y=0 is BOTTOM (Nudge/Cocoa
	// convention) — TopLeft sits at Y=540, not 0. Assert it so nobody
	// "fixes" the orientation.
	if tl.X != 0 || tl.Y != 540 || tl.W != 960 || tl.H != 540 {
		t.Fatalf("top-left wrong: %+v", tl)
	}
	br := Frame(BottomRight, v)
	if br.X+br.W != v.W || br.Y != 0 {
		t.Fatalf("bottom-right wrong: %+v", br)
	}
}

func TestMaximizeIsVisible(t *testing.T) {
	v := vis()
	m := Frame(Maximize, v)
	if *m != v {
		t.Fatalf("maximize != visible: %+v", m)
	}
}

func TestNoGeometryActionsNil(t *testing.T) {
	v := vis()
	for _, a := range []Action{Center, Restore, NextDisplay, PreviousDisplay} {
		if Frame(a, v) != nil {
			t.Fatalf("action %d should have no geometry", a)
		}
	}
}

func TestHistoryEvictsOldest(t *testing.T) {
	h := NewHistory(2)
	h.Push("a", Rect{0, 0, 1, 1})
	h.Push("b", Rect{0, 0, 2, 2})
	h.Push("c", Rect{0, 0, 3, 3}) // evicts a
	if h.Len() != 2 {
		t.Fatalf("len: got %d", h.Len())
	}
	if _, ok := h.Pop("a"); ok {
		t.Fatal("evicted frame survived")
	}
	r, ok := h.Pop("c")
	if !ok || r.W != 3 {
		t.Fatalf("pop wrong: %+v %v", r, ok)
	}
}

func TestHistoryCapacity128(t *testing.T) {
	// Pins the §6.2 requirement: production history holds 128.
	h := NewHistory(128)
	for i := 0; i < 200; i++ {
		h.Push(string(rune('a'+i%26))+string(rune('0'+i/26)), Rect{W: float64(i)})
	}
	if h.Len() != 128 {
		t.Fatalf("len: got %d, want 128", h.Len())
	}
}

func TestMoveToDisplayWraps(t *testing.T) {
	ss := []Screen{{Index: 0}, {Index: 1}, {Index: 2}}
	if got := MoveToDisplay(ss, 2, 1); got != 0 {
		t.Fatalf("wrap: got %d", got)
	}
	if got := MoveToDisplay(ss, 0, -1); got != 2 {
		t.Fatalf("backward wrap: got %d", got)
	}
	if got := MoveToDisplay(ss, 1, 0); got != 1 {
		t.Fatalf("stay: got %d", got)
	}
	if got := MoveToDisplay(nil, 0, 1); got != 0 {
		t.Fatalf("empty: got %d", got)
	}
}
