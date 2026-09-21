// Timeout-disable recovery: a loop, not one-shot (bead criterion 5).
//
// Karabiner reference (event_tap_monitor.hpp): the tap posts
// kCGEventTapDisabledByTimeout when the callback exceeds the system time
// budget; the owner must CGEventTapEnable(tap, true). A single re-enable is
// not enough — repeated disables mean the callback is systematically too
// slow, and the tap must escalate to a health signal instead of dying
// silently (or spinning re-enable forever).
//
// Recovery is pure state (no syscalls) so Tier-1 tests cover it exactly; the
// darwin tap loop calls NoteDisable + ShouldEscalate and performs the actual
// CGEventTapEnable via the enableFn seam.
package spikea

import "time"

// Recovery bounds. MaxReenables caps the loop: after this many disables
// inside Window the tap is declared unhealthy and the owner must surface a
// health signal (reload tap / notify user), never silently keep the tap dead.
const (
	RecoveryWindow  = 30 * time.Second
	MaxReenables    = 5
	ReenableBackoff = 100 * time.Millisecond
)

// Recovery tracks kCGEventTapDisabledByTimeout events.
type Recovery struct {
	disables []time.Time
	// Escalated latches once ShouldEscalate fires — the tap is unhealthy.
	Escalated bool
}

// NoteDisable records one timeout-disable at now. It returns the backoff the
// caller should wait before re-enabling (0 when already escalated — do not
// re-enable a dead tap, surface health instead).
func (r *Recovery) NoteDisable(now time.Time) time.Duration {
	cutoff := now.Add(-RecoveryWindow)
	kept := r.disables[:0]
	for _, t := range r.disables {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	r.disables = append(kept, now)
	if len(r.disables) > MaxReenables {
		r.Escalated = true
		return 0
	}
	return ReenableBackoff
}

// DisableCount reports disables inside the current window (for stage logs).
func (r *Recovery) DisableCount(now time.Time) int {
	cutoff := now.Add(-RecoveryWindow)
	n := 0
	for _, t := range r.disables {
		if t.After(cutoff) {
			n++
		}
	}
	return n
}

// Reset clears the window after a clean re-enable stretch (owner calls when
// the tap survives RecoveryWindow without a disable).
func (r *Recovery) Reset() {
	r.disables = r.disables[:0]
	r.Escalated = false
}
