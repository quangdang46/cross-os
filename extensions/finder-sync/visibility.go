// Visibility policy — daemon-down hide (bead criterion 3).
//
// The appex asks ShouldShowMenu before building: socket reachable → show;
// anything else → hide (return nil menu). No retry, no queue, no error UI
// inside Finder — the extension degrades to invisible, Finder never blocks.
// menu(for:) maps this to: Show → BuildMenu, Hide → nil.
package findersync

import (
	"net"
	"time"
)

// Visibility is the menu decision.
type Visibility int

const (
	// Show builds the menu via BuildMenu.
	Show Visibility = iota
	// Hide returns a nil menu: daemon unreachable, degrade to invisible.
	Hide
)

// ProbeTimeout bounds the visibility check — faster than the action path
// because menu-open is latency-sensitive (Finder waits on menu(for:)).
const ProbeTimeout = 150 * time.Millisecond

// ShouldShowMenu dials the daemon socket with a short timeout. Success →
// Show (caller closes nothing; this is a probe, Send redials on action).
// Any failure → Hide with the reason for stage logs.
func ShouldShowMenu(socketPath string) (Visibility, string) {
	conn, err := net.DialTimeout("unix", socketPath, ProbeTimeout)
	if err != nil {
		return Hide, err.Error()
	}
	conn.Close()
	return Show, ""
}
