// Zone validation tests — the user-placed rectangles the Windows page edits.
package winlayout

import (
	"math"
	"testing"
)

func TestValidateZonesAcceptsAWholeListOrNothing(t *testing.T) {
	good := []Zone{
		{ID: "left", Name: "Left third", X: 0, Y: 0, W: 480, H: 1080},
		{ID: "right", Name: "Right third", X: 1440, Y: 0, W: 480, H: 1080},
	}
	if err := ValidateZones(good); err != nil {
		t.Fatalf("valid list rejected: %v", err)
	}
	if err := ValidateZones(nil); err != nil {
		t.Fatalf("an empty list is the seed state, not an error: %v", err)
	}
	// One bad row rejects the edit: the caller swaps the whole list or none.
	bad := [][]Zone{
		{{ID: "a", Name: "A", W: 1, H: 1}, {ID: "a", Name: "B", W: 1, H: 1}}, // duplicate
		{{ID: "  ", Name: "A", W: 1, H: 1}},                                  // blank id
		{{ID: "a", Name: " ", W: 1, H: 1}},                                   // nameless
		{{ID: "a", Name: "A", W: 0, H: 1}},                                   // no width
		{{ID: "a", Name: "A", W: 1, H: -1}},                                  // negative
		{{ID: "a", Name: "A", X: math.Inf(1), W: 1, H: 1}},                   // non-finite
		{{ID: "a", Name: "A", Y: math.NaN(), W: 1, H: 1}},                    // NaN
	}
	for _, list := range bad {
		if err := ValidateZones(list); err == nil {
			t.Errorf("zone list %+v must be rejected", list)
		}
	}
}

// A Zone is a user rectangle, never an action zone: the two vocabularies
// stay separate so an action zone cannot inherit geometry it never had.
func TestZonesAreNotActionZones(t *testing.T) {
	if z := DefaultZones(); len(z) != 0 {
		t.Fatalf("DefaultZones()=%+v, want the empty seed (no display geometry outside the adapter)", z)
	}
	// The action vocabulary still resolves on its own terms.
	if a, ok := ActionForZone("left-half"); !ok || ZoneName(a) != "leftHalf" {
		t.Fatalf("action zone vocabulary broke: %v %q", ok, ZoneName(a))
	}
}
