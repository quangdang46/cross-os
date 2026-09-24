// Snap zones: the user-placed rectangles the Windows page edits (page source
// core:snapZones, actions config.writeZones).
//
// A Zone is NOT an action zone. "leftHalf" is a RELATIVE slot Frame() resolves
// against whatever display the window is on; a Zone is an ABSOLUTE rectangle
// in screen points that somebody put there. The vocabularies stay separate on
// purpose: merging them would let an action zone inherit user geometry it
// never had, and would make a user zone look adapter-resolved when nothing
// resolves it yet.
//
// No defaults. The daemon holds no display geometry outside the adapter
// (winlayout.Screen arrives with the live query), so there is no screen to
// seed a rectangle against — publishing absolute numbers from an assumed
// screen would put points on the user's desktop that they never chose.
// DefaultZones is the honest empty list the editor starts from.
package winlayout

import (
	"fmt"
	"math"
	"strings"
)

// Zone is one user-placed snap rectangle in screen points. The flat X/Y/W/H
// shape (not a nested Rect) is what the config.getZones wire row carries, so
// the persisted config.json and the IPC row use the same key names.
type Zone struct {
	ID   string  `json:"id"`
	Name string  `json:"name"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	W    float64 `json:"w"`
	H    float64 `json:"h"`
}

// DefaultZones is the seed list: no user rectangles have been placed yet.
func DefaultZones() []Zone { return []Zone{} }

// ValidateZones rejects a zone list the editor cannot act on: a blank or
// duplicated ID, a nameless row, a non-finite coordinate, or a rectangle with
// no area. Every zone is checked before the caller swaps the running set, so a
// rejected edit leaves the previous list serving (fail closed).
func ValidateZones(zones []Zone) error {
	seen := make(map[string]struct{}, len(zones))
	for _, z := range zones {
		id := strings.TrimSpace(z.ID)
		if id == "" {
			return fmt.Errorf("winlayout: zone with an empty id")
		}
		if _, dup := seen[id]; dup {
			return fmt.Errorf("winlayout: duplicate zone id %q", id)
		}
		seen[id] = struct{}{}
		if strings.TrimSpace(z.Name) == "" {
			return fmt.Errorf("winlayout: zone %q has no name", id)
		}
		for _, f := range []struct {
			axis string
			v    float64
		}{{"x", z.X}, {"y", z.Y}, {"w", z.W}, {"h", z.H}} {
			// JSON accepts no NaN/Inf, but a Go caller can still hand one
			// over; refuse rather than persist a rect nothing can compare.
			if math.IsNaN(f.v) || math.IsInf(f.v, 0) {
				return fmt.Errorf("winlayout: zone %q %s is not a finite number", id, f.axis)
			}
		}
		if z.W <= 0 || z.H <= 0 {
			return fmt.Errorf("winlayout: zone %q has no area (w=%g h=%g)", id, z.W, z.H)
		}
	}
	return nil
}
