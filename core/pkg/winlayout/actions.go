// Window actions: 19 §6.2 snap actions via Intent → window.* capability →
// Adapter (bead cross-os-nir.2).
//
// Plan: COMPREHENSIVE_PLAN.md §6.2 (19 actions + shortcut table), §4.1 Window
// Management, §3.12 capabilities, §9.10 yabai row (SIP-clean constraint).
// Geometry: core/pkg/winlayout Frame/History/MoveToDisplay (Nudge port,
// bead cross-os-nir.1). AX move/resize primitives: spike C (cross-os-lla).
//
// Authority: this package RESOLVES actions to capability invocations
// (zone names + capability IDs). The Adapter EXECUTES via AX. No AX calls,
// no syscalls here — pure resolution + history + trace, Tier-1 testable.
// Live AX assertions (post-action frame within 1pt) are Tier-2 gated for a
// real desktop with accessibility consent.
//
// SIP-clean: only AX APIs (AXUIElementSetAttributeValue kAXPosition/
// kAXSizeAttribute, action kAXMinimizeAttribute etc.). yabai-style
// scripting-addition paths are explicitly rejected — nothing here needs
// SIP disabled, and nothing here can disable it.
package winlayout

import (
	"fmt"
	"math"
)

// CapabilityFor maps an Action to its window.* capability + zone parameter.
// Geometry actions (halves/quadrants/thirds/two-thirds/center/restore/
// fullscreen/display moves) resolve to window.move with a zone name; the
// Adapter computes the frame (center needs window size, restore reads
// history, display moves need the screen list). Minimize/Maximize use their
// dedicated capabilities.
func CapabilityFor(a Action) (capability, zone string) {
	switch a {
	case Minimize:
		return "window.minimize", ""
	case Maximize:
		return "window.maximize", ""
	default:
		return "window.move", ZoneName(a)
	}
}

// ZoneName is the window.move zone parameter for action a.
func ZoneName(a Action) string {
	switch a {
	case LeftHalf:
		return "leftHalf"
	case RightHalf:
		return "rightHalf"
	case TopHalf:
		return "topHalf"
	case BottomHalf:
		return "bottomHalf"
	case TopLeft:
		return "topLeft"
	case TopRight:
		return "topRight"
	case BottomLeft:
		return "bottomLeft"
	case BottomRight:
		return "bottomRight"
	case LeftThird:
		return "leftThird"
	case CenterThird:
		return "centerThird"
	case RightThird:
		return "rightThird"
	case LeftTwoThirds:
		return "leftTwoThirds"
	case CenterTwoThirds:
		return "centerTwoThirds"
	case RightTwoThirds:
		return "rightTwoThirds"
	case Maximize:
		return "maximize"
	case Center:
		return "center"
	case Restore:
		return "restore"
	case NextDisplay:
		return "nextDisplay"
	case PreviousDisplay:
		return "previousDisplay"
	case Minimize:
		return "minimize"
	case Fullscreen:
		return "fullscreen"
	}
	return "unknown"
}

// ActionName returns the §6.2 display name for action a.
func ActionName(a Action) string {
	switch a {
	case LeftHalf:
		return "Left Half"
	case RightHalf:
		return "Right Half"
	case TopHalf:
		return "Top Half"
	case BottomHalf:
		return "Bottom Half"
	case TopLeft:
		return "Top Left"
	case TopRight:
		return "Top Right"
	case BottomLeft:
		return "Bottom Left"
	case BottomRight:
		return "Bottom Right"
	case LeftThird:
		return "Left Third"
	case CenterThird:
		return "Center Third"
	case RightThird:
		return "Right Third"
	case LeftTwoThirds:
		return "Left Two Thirds"
	case CenterTwoThirds:
		return "Center Two Thirds"
	case RightTwoThirds:
		return "Right Two Thirds"
	case Maximize:
		return "Maximize"
	case Center:
		return "Center"
	case Restore:
		return "Restore"
	case NextDisplay:
		return "Next Display"
	case PreviousDisplay:
		return "Previous Display"
	case Minimize:
		return "Minimize"
	case Fullscreen:
		return "Fullscreen"
	}
	return "Unknown"
}

// AllActions lists all 19 §6.2 actions in canonical order, plus the
// Fullscreen zone row (same §6.2 list head — Fullscreen resolves via
// window.move zone "fullscreen", Adapter-owned, no geometry).
var AllActions = []Action{
	Maximize, Minimize, Fullscreen,
	LeftHalf, RightHalf, TopHalf, BottomHalf,
	TopLeft, TopRight, BottomLeft, BottomRight,
	LeftThird, CenterThird, RightThird,
	LeftTwoThirds, CenterTwoThirds, RightTwoThirds,
	Center, Restore,
	NextDisplay, PreviousDisplay,
}

// ActionByName resolves a display name back to its Action (for the
// config.setShortcuts IPC path, which speaks names, not enum values).
// Unknown names fail closed (never a silent default action).
func ActionByName(name string) (Action, bool) {
	for _, a := range AllActions {
		if ActionName(a) == name {
			return a, true
		}
	}
	return 0, false
}

// Fullscreen is §6.2's 19th-row "Fullscreen": it resolves through
// window.move zone "fullscreen" (Adapter toggles kAXFullscreenAttribute;
// geometry Frame() has no fullscreen rect — the Adapter owns it).
const FullscreenZone = "fullscreen"

// Within1pt reports whether the post-action frame equals the expected frame
// within 1pt tolerance on every edge (bead criterion: live AX assertion).
// Pure so Tier-1 tests assert the tolerance math; Tier-2 feeds it live frames.
func Within1pt(got, want Rect) bool {
	const tol = 1.0
	return math.Abs(got.X-want.X) <= tol &&
		math.Abs(got.Y-want.Y) <= tol &&
		math.Abs(got.W-want.W) <= tol &&
		math.Abs(got.H-want.H) <= tol
}

// Stage is one Recorder-bound trace line for a window action (feeds
// cross-os-nir.6): event → result via the standard stage vocabulary.
type Stage struct {
	Stage  string // "event","context","rule","intent","action","result"
	Detail string
}

// Executor resolves + records window actions. It owns the frame History
// (restore semantics, capacity 128) and appends one Stage trace per action
// for the Recorder. The Adapter performs the live AX call with the resolved
// capability/zone/frame and reports back via Confirm.
type Executor struct {
	History *History
	Stages  []Stage
}

// NewExecutor returns an Executor with a 128-capacity history (bead
// criterion: eviction at 128 — the 129th push drops the oldest).
func NewExecutor() *Executor { return &Executor{History: NewHistory(128)} }

// Resolve maps action → capability invocation for window winID. For geometry
// actions with a Frame-computable zone it also returns the target frame on
// the given screen; nil frame means adapter-side resolution (center needs
// window size, restore reads history, display moves need screens).
// Every call appends event→action trace stages; Confirm appends result.
func (e *Executor) Resolve(action Action, winID string, current Rect, screen Screen) (capability, zone string, target *Rect) {
	e.log("event", fmt.Sprintf("action=%s window=%s", ActionName(action), winID))
	capability, zone = CapabilityFor(action)
	if r := Frame(action, screen.Visible); r != nil {
		// Remember the pre-snap frame for Restore (first-snap-wins).
		if _, ok := e.History.frames[winID]; !ok {
			e.History.Push(winID, current)
		}
		target = r
		e.log("action", fmt.Sprintf("%s zone=%s frame=%+v", capability, zone, *r))
		return capability, zone, target
	}
	if action == Restore {
		if r, ok := e.History.Pop(winID); ok {
			target = &r
			e.log("action", fmt.Sprintf("%s zone=restore frame=%+v", capability, r))
			return capability, zone, target
		}
		e.log("action", fmt.Sprintf("%s zone=restore: no history for %s", capability, winID))
		return capability, zone, nil
	}
	e.log("action", fmt.Sprintf("%s zone=%s (adapter-side)", capability, zone))
	return capability, zone, nil
}

// Confirm records the post-action result stage. Live Tier-2 callers pass the
// AX-observed frame; ok=false names the failure (never a silent no-op).
func (e *Executor) Confirm(action Action, winID string, ok bool, observed Rect, want *Rect) {
	if !ok {
		e.log("result", fmt.Sprintf("%s window=%s FAILED (adapter error, see stage log)", ActionName(action), winID))
		return
	}
	if want != nil && !Within1pt(observed, *want) {
		e.log("result", fmt.Sprintf("%s window=%s MISMATCH observed=%+v want=%+v", ActionName(action), winID, observed, *want))
		return
	}
	e.log("result", fmt.Sprintf("%s window=%s ok frame=%+v", ActionName(action), winID, observed))
}

func (e *Executor) log(stage, detail string) {
	e.Stages = append(e.Stages, Stage{Stage: stage, Detail: detail})
}
