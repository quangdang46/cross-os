package winlayout

import "math"

// Rect is a screen/window frame in points. Origin at visibleFrame.Min
// (Nudge convention: zones computed relative to screen.visibleFrame, so
// menu bar + Dock are already excluded by the caller-supplied Visible).
type Rect struct {
	X, Y, W, H float64
}

// Screen is one display: full frame + visible frame (excl. menu/Dock).
type Screen struct {
	Frame   Rect
	Visible Rect
	Index   int
}

// Action is the 19-action vocabulary (Nudge SnapAction, minus UI).
// center/restore/nextDisplay/previousDisplay carry no geometry here:
// center needs window size (adapter-side), restore reads history,
// display moves need the screen list (MoveToDisplay).
type Action int

const (
	LeftHalf Action = iota
	RightHalf
	TopHalf
	BottomHalf
	TopLeft
	TopRight
	BottomLeft
	BottomRight
	LeftThird
	CenterThird
	RightThird
	LeftTwoThirds
	CenterTwoThirds
	RightTwoThirds
	Maximize
	Center
	Restore
	NextDisplay
	PreviousDisplay
)

// Frame returns the target frame for action on screen's visible area.
// Mirrors Nudge SnapZone.frame (floor half/third splits, remainder kept on
// the right/bottom so halves tile without gaps). Nil = no geometry
// (center/restore/display moves resolve adapter-side).
func Frame(action Action, visible Rect) *Rect {
	f := visible
	floor := math.Floor
	switch action {
	case LeftHalf:
		return &Rect{f.X, f.Y, floor(f.W / 2), f.H}
	case RightHalf:
		hw := floor(f.W / 2)
		return &Rect{f.X + hw, f.Y, f.W - hw, f.H}
	case TopHalf:
		hh := floor(f.H / 2)
		return &Rect{f.X, f.Y + hh, f.W, f.H - hh}
	case BottomHalf:
		return &Rect{f.X, f.Y, f.W, floor(f.H / 2)}
	case TopLeft:
		hw, hh := floor(f.W/2), floor(f.H/2)
		return &Rect{f.X, f.Y + hh, hw, f.H - hh}
	case TopRight:
		hw, hh := floor(f.W/2), floor(f.H/2)
		return &Rect{f.X + hw, f.Y + hh, f.W - hw, f.H - hh}
	case BottomLeft:
		return &Rect{f.X, f.Y, floor(f.W / 2), floor(f.H / 2)}
	case BottomRight:
		hw := floor(f.W / 2)
		return &Rect{f.X + hw, f.Y, f.W - hw, floor(f.H / 2)}
	case LeftThird:
		return &Rect{f.X, f.Y, floor(f.W / 3), f.H}
	case CenterThird:
		tw := floor(f.W / 3)
		return &Rect{f.X + tw, f.Y, tw, f.H}
	case RightThird:
		tw := floor(f.W / 3)
		return &Rect{f.X + tw*2, f.Y, f.W - tw*2, f.H}
	case LeftTwoThirds:
		tw := floor(f.W / 3)
		return &Rect{f.X, f.Y, tw * 2, f.H}
	case CenterTwoThirds:
		sw := floor(f.W / 6)
		return &Rect{f.X + sw, f.Y, f.W - sw*2, f.H}
	case RightTwoThirds:
		tw := floor(f.W / 3)
		return &Rect{f.X + tw, f.Y, f.W - tw, f.H}
	case Maximize:
		r := f
		return &r
	}
	return nil
}

// History is the frame-history ring (Nudge WindowFrameHistory, capacity
// 128): remembers pre-snap frames per window for restore. Single-owner
// (adapter-confined to one thread/goroutine) — no mutex, like Registry
// build-once. A shared history would need locking. (review: cross-os-c0)
type History struct {
	cap    int
	frames map[string]Rect
	order  []string
}

// NewHistory returns a History with the given capacity (min 1).
func NewHistory(capacity int) *History {
	if capacity < 1 {
		capacity = 1
	}
	return &History{cap: capacity, frames: map[string]Rect{}}
}

// Push remembers win's frame, evicting oldest past capacity. Re-push of a
// known window refreshes its FRAME but keeps its original recency slot
// (first-snap-wins eviction, not LRU — restore semantics want the oldest
// pre-snap frame, not the most recently touched). (review: cross-os-c0)
func (h *History) Push(win string, r Rect) {
	if _, ok := h.frames[win]; !ok {
		h.order = append(h.order, win)
	}
	h.frames[win] = r
	for len(h.order) > h.cap {
		delete(h.frames, h.order[0])
		h.order = h.order[1:]
	}
}

// Pop returns and forgets win's remembered frame.
func (h *History) Pop(win string) (Rect, bool) {
	r, ok := h.frames[win]
	if !ok {
		return Rect{}, false
	}
	delete(h.frames, win)
	for i, id := range h.order {
		if id == win {
			h.order = append(h.order[:i], h.order[i+1:]...)
			break
		}
	}
	return r, true
}

// Len returns the remembered window count.
func (h *History) Len() int { return len(h.frames) }

// MoveToDisplay returns the screen index after stepping count displays
// from cur (wrapping). The adapter re-applies the window's relative frame
// on the new screen; this function owns only the index arithmetic.
func MoveToDisplay(screens []Screen, cur, count int) int {
	if len(screens) == 0 {
		return cur
	}
	n := len(screens)
	next := (cur + count) % n
	if next < 0 {
		next += n
	}
	return next
}
