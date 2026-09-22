// Finder lifecycle state machine: AVAILABLE / DISABLED / NOT_APPROVED /
// HOST_NOT_RUNNING / DAEMON_UNAVAILABLE (bead cross-os-vbl.4).
//
// Plan: §4.1 Finder Context Menu, §10 Spike D, §8.1/8.4, §3.6c
// status-indicator. Spike D proved menu → socket → daemon-down fallback;
// this hardens it into the shipped machine. A Finder hang is the worst
// possible failure — daemon-down hides menus, never blocks.
//
// States:
//
//	HOST_NOT_RUNNING: pre-launch (extension loaded, daemon never seen).
//	AVAILABLE: daemon reachable, approved → menus shown.
//	DAEMON_UNAVAILABLE: daemon unreachable → menus hidden silently.
//	NOT_APPROVED: System Settings approval missing → guide the user
//	  (non-tech copy), menus hidden.
//	DISABLED: user-disabled → CrossOS stops affecting behavior (OS
//	  permission entries may remain until the user removes them).
package findersync

import "fmt"

// ExtState is one lifecycle state.
type ExtState int

const (
	// HostNotRunning: pre-launch, daemon never seen.
	HostNotRunning ExtState = iota
	// Available: daemon reachable + approved → show menus.
	Available
	// DaemonUnavailable: daemon unreachable → hide silently.
	DaemonUnavailable
	// NotApproved: approval missing → guide to System Settings.
	NotApproved
	// Disabled: user-disabled → no behavior effect.
	Disabled
)

// String names the state for UI status + logs.
func (s ExtState) String() string {
	switch s {
	case Available:
		return "AVAILABLE"
	case DaemonUnavailable:
		return "DAEMON_UNAVAILABLE"
	case NotApproved:
		return "NOT_APPROVED"
	case Disabled:
		return "DISABLED"
	default:
		return "HOST_NOT_RUNNING"
	}
}

// Lifecycle tracks the extension state + transitions. Every transition is
// explicit (no silent jumps); ShowMenu reports the single menu decision
// (only AVAILABLE shows) so callers cannot invent a sixth path.
type Lifecycle struct {
	State ExtState
	Log   []string
}

// event records a transition reason for the UI log.
func (l *Lifecycle) event(format string, args ...any) {
	l.Log = append(l.Log, fmt.Sprintf(format, args...))
}

// OnDaemonUp: reachable probe succeeded.
func (l *Lifecycle) OnDaemonUp(approved bool) {
	if l.State == Disabled {
		l.event("daemon up ignored (disabled)")
		return
	}
	if !approved {
		l.State = NotApproved
		l.event("daemon up → NOT_APPROVED (guide to System Settings)")
		return
	}
	l.State = Available
	l.event("daemon up → AVAILABLE")
}

// OnDaemonDown: probe failed or mid-session disconnect → hide silently.
func (l *Lifecycle) OnDaemonDown(reason string) {
	if l.State == Disabled {
		return
	}
	l.State = DaemonUnavailable
	l.event("daemon down → DAEMON_UNAVAILABLE (%s): menus hidden", reason)
}

// OnApprovalChanged tracks System Settings approval flips.
func (l *Lifecycle) OnApprovalChanged(approved bool) {
	if l.State == Disabled {
		return
	}
	if approved && l.State == NotApproved {
		l.State = Available
		l.event("approved → AVAILABLE")
	} else if !approved {
		l.State = NotApproved
		l.event("approval revoked → NOT_APPROVED (guide to System Settings)")
	}
}

// SetDisabled: explicit user disable → no behavior effect.
func (l *Lifecycle) SetDisabled(disabled bool) {
	if disabled {
		l.State = Disabled
		l.event("user disabled → DISABLED (no behavior effect)")
	} else if l.State == Disabled {
		l.State = HostNotRunning
		l.event("re-enabled → HOST_NOT_RUNNING (await daemon)")
	}
}

// ShowMenu is the single menu decision: only AVAILABLE shows.
func (l *Lifecycle) ShowMenu() bool { return l.State == Available }

// ApprovalCopy is the non-tech guidance for NOT_APPROVED (no jargon: no
// CGEventTap/AXUIElement-class words anywhere near the user).
func ApprovalCopy() string {
	return "CrossOS needs your approval to add menu items in Finder. " +
		"Open System Settings → Privacy & Security → Extensions, " +
		"turn on CrossOS, then come back here."
}
