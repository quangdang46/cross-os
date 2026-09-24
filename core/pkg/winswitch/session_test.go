package winswitch

import "testing"

// Behavior 3: a target the user picked is followed BY ID across a reorder,
// while an untouched default is re-derived. The same reordering and the same
// rows, decided one way and the other, is the whole distinction — the failure
// was a default that locked onto a slot mid-churn and then trailed that window
// down the list to an unrelated tile.
func TestUserPickedTargetIsFollowedWhileAnUntouchedDefaultIsRederived(t *testing.T) {
	settled := []Row{real("b"), real("c"), real("a")}
	tests := []struct {
		name       string
		pick       string
		wantTarget string
		wantIndex  int
		wantReason Reason
	}{
		{
			name:       "an untouched default re-derives over the new order",
			wantTarget: "c",
			wantIndex:  1,
			wantReason: ReasonDefault,
		},
		{
			name:       "a user-picked target is followed by id",
			pick:       "b",
			wantTarget: "b",
			wantIndex:  0,
			wantReason: ReasonFollow,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var s Session
			first := []Row{real("a"), real("b"), real("c")}
			if got := s.Summon(first, true); got.Index != 1 || got.TargetID != "b" {
				t.Fatalf("Summon = %+v, want the default on b at 1", got)
			}
			if tt.pick != "" {
				if got := s.Pick(first, tt.pick); got.TargetID != tt.pick {
					t.Fatalf("Pick(%s) = %+v", tt.pick, got)
				}
				if !s.UserPicked() {
					t.Fatal("a pick did not register as the user's own")
				}
			} else if s.UserPicked() {
				t.Fatal("the summon registered as a user pick; the default must keep re-deriving")
			}
			got := s.Decide(settled, Update{CurrentWindowDrawn: true})
			if got.TargetID != tt.wantTarget || got.Index != tt.wantIndex || got.Reason != tt.wantReason {
				t.Fatalf("Decide(%v) = %+v, want target %q at %d by %s",
					ids(settled), got, tt.wantTarget, tt.wantIndex, tt.wantReason)
			}
			if s.Target() != tt.wantTarget {
				t.Fatalf("Target() = %q, want %q", s.Target(), tt.wantTarget)
			}
		})
	}
}

// The captured regression: nothing was picked, and the default must follow the
// list rather than the window that happened to sit under it when the switcher
// opened.
func TestUntouchedDefaultDoesNotTrailAWindowThatSlidDownTheList(t *testing.T) {
	var s Session
	s.Summon([]Row{real("a"), real("b"), real("c")}, true) // default: b
	got := s.Decide([]Row{real("c"), real("a"), real("b")}, Update{CurrentWindowDrawn: true})
	if got.TargetID != "a" || got.Index != 1 {
		t.Fatalf("Decide = %+v, want the default re-derived onto a at 1, not b slid to 2", got)
	}
}

// The #5665 cluster: once the user has chosen, the highlight follows the window
// up and down the list as other windows arrive and leave.
func TestUserPickedTargetFollowsReordersInBothDirections(t *testing.T) {
	tests := []struct {
		name      string
		pick      string
		rows      []Row
		wantIndex int
	}{
		{
			name:      "an app launching above it pushes it down",
			pick:      "c",
			rows:      []Row{real("a"), real("b"), newcomer("photoshop"), real("c")},
			wantIndex: 3,
		},
		{
			name:      "a window above it closing pulls it up",
			pick:      "c",
			rows:      []Row{real("c"), real("a")},
			wantIndex: 0,
		},
		{
			name:      "churn that leaves it where it was",
			pick:      "b",
			rows:      []Row{real("a"), real("b"), real("c")},
			wantIndex: 1,
		},
		{
			name:      "a placeholder ahead of it is skipped by the count, not the index",
			pick:      "c",
			rows:      []Row{placeholder("app"), real("a"), real("c")},
			wantIndex: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var s Session
			open := []Row{real("a"), real("b"), real("c")}
			s.Summon(open, true)
			if got := s.Pick(open, tt.pick); got.Reason != ReasonUserPick {
				t.Fatalf("Pick = %+v, want a user-pick", got)
			}
			got := s.Decide(tt.rows, Update{CurrentWindowDrawn: true})
			if got.Index != tt.wantIndex || got.TargetID != tt.pick {
				t.Fatalf("Decide(%v) = %+v, want %q at %d", ids(tt.rows), got, tt.pick, tt.wantIndex)
			}
			if got.Reason != ReasonFollow {
				t.Fatalf("Decide reason = %s, want follow-target", got.Reason)
			}
		})
	}
}

// The summon-time length is measured once, on the first selection pass, so a
// later refresh compares against the list as of the press and not against
// whatever it has become.
func TestSummonTimeLengthIsMeasuredOnceAndOnlyGainsAreSteppedOver(t *testing.T) {
	t.Run("an arrival grows the list and is stepped over", func(t *testing.T) {
		var s Session
		if got := s.Summon([]Row{real("a"), real("b")}, true); got.TargetID != "b" {
			t.Fatalf("Summon = %+v, want the default on b", got)
		}
		if s.VisibleAtSummon() != 2 {
			t.Fatalf("VisibleAtSummon = %d, want 2", s.VisibleAtSummon())
		}
		got := s.Decide([]Row{newcomer("late"), real("a"), real("b")}, Update{CurrentWindowDrawn: true})
		if got.Index != 2 || got.TargetID != "b" {
			t.Fatalf("Decide = %+v, want b at 2 with the arrival stepped over", got)
		}
		if s.VisibleAtSummon() != 2 {
			t.Fatalf("a refresh moved the summon-time length to %d", s.VisibleAtSummon())
		}
	})

	t.Run("a replacement does not grow the list and is not stepped over", func(t *testing.T) {
		var s Session
		s.Summon([]Row{real("a"), real("b")}, true)
		// The tab that was drawn at tile 0 stopped being drawn and an untracked
		// one took its place: the list never grew, so the pick stays on tile 1.
		got := s.Decide([]Row{newcomer("tab"), real("b")}, Update{CurrentWindowDrawn: true})
		if got.Index != 1 || got.TargetID != "b" {
			t.Fatalf("Decide = %+v, want b at 1", got)
		}
	})

	t.Run("a second summon re-measures", func(t *testing.T) {
		var s Session
		s.Summon([]Row{real("a"), real("b")}, true)
		s.Cycle([]Row{real("a"), real("b")}, 0)
		s.Summon([]Row{newcomer("late"), real("a"), real("b")}, true)
		if s.VisibleAtSummon() != 3 {
			t.Fatalf("VisibleAtSummon = %d, want 3", s.VisibleAtSummon())
		}
		if s.UserPicked() {
			t.Fatal("a second summon must clear the previous commitment")
		}
	})
}

// The #5941 flag says where the DEFAULT starts. A target the user picked is
// still followed by id afterwards.
func TestTheCurrentWindowFlagOnlyMovesTheDefault(t *testing.T) {
	rows := []Row{real("a"), real("b")}
	var s Session
	if got := s.Summon(rows, false); got.Index != 0 || got.TargetID != "a" {
		t.Fatalf("Summon = %+v, want the front tile a", got)
	}
	s.Pick(rows, "b")
	got := s.Decide(rows, Update{CurrentWindowDrawn: false})
	if got.Index != 1 || got.TargetID != "b" || got.Reason != ReasonFollow {
		t.Fatalf("Decide = %+v, want the picked b followed at 1", got)
	}
	// And the flag is not sticky: a refresh that reports the current window
	// drawn again re-derives the default, which is the front tile's neighbour
	// again, over the order rather than over the last flag.
	if got := s.Decide([]Row{real("a"), real("b"), real("c")}, Update{CurrentWindowDrawn: true}); got.TargetID != "b" {
		t.Fatalf("Decide = %+v, want the default re-derived onto b", got)
	}
}

// A target that leaves the list is backfilled onto whatever now occupies its
// slot, so the highlight lands on a window instead of on a hole.
func TestAVanishedTargetBackfillsOntoItsSlot(t *testing.T) {
	tests := []struct {
		name      string
		open      []Row
		pick      string
		rows      []Row
		wantIndex int
		wantID    string
	}{
		{
			name:      "the window now at that index takes it",
			open:      []Row{real("a"), real("b"), real("c")},
			pick:      "b",
			rows:      []Row{real("a"), real("new"), real("c")},
			wantIndex: 1,
			wantID:    "new",
		},
		{
			name:      "a list that shrank below it lands on the last window",
			open:      []Row{real("a"), real("b"), real("c")},
			pick:      "c",
			rows:      []Row{real("a"), real("b")},
			wantIndex: 1,
			wantID:    "b",
		},
		{
			name:      "a target filtered out backs onto the next candidate",
			open:      []Row{real("a"), real("b"), real("c")},
			pick:      "b",
			rows:      []Row{real("a"), placeholder("app"), real("c")},
			wantIndex: 2,
			wantID:    "c",
		},
		{
			name:      "an emptied list clears the highlight",
			open:      []Row{real("a"), real("b"), real("c")},
			pick:      "b",
			rows:      []Row{placeholder("app"), minimized("dock")},
			wantIndex: NoIndex,
			wantID:    "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var s Session
			s.Summon(tt.open, true)
			s.Pick(tt.open, tt.pick)
			got := s.Decide(tt.rows, Update{CurrentWindowDrawn: true})
			if got.Index != tt.wantIndex || got.TargetID != tt.wantID {
				t.Fatalf("Decide(%v) = %+v, want %q at %d", ids(tt.rows), got, tt.wantID, tt.wantIndex)
			}
			if tt.wantIndex == NoIndex && got.Reason != ReasonEmpty {
				t.Fatalf("reason = %s, want empty", got.Reason)
			}
		})
	}
}

// Cycling is the other way a default becomes a commitment, and it walks
// candidates rather than raw indexes.
func TestCycleCommitsAndWrapsOverCandidates(t *testing.T) {
	rows := []Row{real("a"), placeholder("app"), real("b"), real("c")}
	var s Session
	if got := s.Summon(rows, true); got.TargetID != "b" || s.UserPicked() {
		t.Fatalf("Summon = %+v (userPicked %v), want the untouched default on b", got, s.UserPicked())
	}
	for _, step := range []struct {
		delta int
		want  string
	}{
		{delta: 1, want: "c"},
		{delta: -1, want: "b"},
		{delta: -1, want: "a"},
		{delta: 1, want: "b"},
		{delta: 1, want: "c"},
		{delta: 1, want: "a"},  // wraps past the end, skipping the placeholder
		{delta: -1, want: "c"}, // wraps past the start
	} {
		got := s.Cycle(rows, step.delta)
		if got.TargetID != step.want || got.Reason != ReasonCycle {
			t.Fatalf("Cycle(%d) = %+v, want %q", step.delta, got, step.want)
		}
		if !s.UserPicked() {
			t.Fatal("cycling did not register as the user's own")
		}
	}
	// A cycle past the end of a session with no highlight yet lands on the
	// front candidate rather than on the second one.
	var fresh Session
	fresh.Summon(nil, true)
	if got := fresh.Cycle(rows, 1); got.TargetID != "a" {
		t.Fatalf("Cycle on an empty session = %+v, want the front candidate a", got)
	}
	if got := fresh.Cycle(nil, 1); got.Index != NoIndex || got.Reason != ReasonEmpty {
		t.Fatalf("Cycle over nothing drawn = %+v, want an empty decision", got)
	}
}

// A hover that names a window no longer drawn is stale input, not a reason to
// throw away the session.
func TestPickingAWindowThatIsNotDrawnCommitsToNothing(t *testing.T) {
	rows := []Row{real("a"), real("b")}
	var s Session
	s.Summon(rows, true)
	got := s.Pick(rows, "gone")
	if got.Index != NoIndex || got.Reason != ReasonNone {
		t.Fatalf("Pick of a stale id = %+v, want no decision", got)
	}
	if s.Target() != "b" || s.UserPicked() {
		t.Fatalf("a stale pick disturbed the session: target %q userPicked %v", s.Target(), s.UserPicked())
	}
}

// The decision priority the spec fixes, in the order it fixes it.
func TestDecisionPriority(t *testing.T) {
	rows := []Row{real("a"), real("b"), real("c")}

	t.Run("a new query jumps to its best match", func(t *testing.T) {
		var s Session
		s.Summon(rows, true)
		got := s.Decide(rows, Update{CurrentWindowDrawn: true, SearchActive: true, SearchBestID: "c"})
		if got.Index != 2 || got.Reason != ReasonSearchBest {
			t.Fatalf("Decide = %+v, want c at 2 by search-best", got)
		}
	})

	t.Run("clearing the query restores the default", func(t *testing.T) {
		var s Session
		s.Summon(rows, true)
		s.Decide(rows, Update{CurrentWindowDrawn: false, SearchActive: true, SearchBestID: "c"})
		got := s.Decide(rows, Update{CurrentWindowDrawn: true})
		if got.Reason != ReasonSearchClear || got.TargetID != "b" {
			t.Fatalf("Decide = %+v, want the restored default on b", got)
		}
	})

	t.Run("a cleared query restores the default even against an empty list", func(t *testing.T) {
		var s Session
		s.Summon(rows, true)
		s.Decide(rows, Update{CurrentWindowDrawn: true, SearchActive: true, SearchBestID: "c"})
		got := s.Decide(nil, Update{})
		if got.Index != NoIndex || got.TargetID != "" {
			t.Fatalf("Decide = %+v, want the highlight cleared", got)
		}
	})

	t.Run("an empty list clears the target", func(t *testing.T) {
		var s Session
		s.Summon(rows, true)
		s.Pick(rows, "c")
		got := s.Decide([]Row{placeholder("app")}, Update{CurrentWindowDrawn: true})
		if got.Index != NoIndex || got.Reason != ReasonEmpty || s.Target() != "" {
			t.Fatalf("Decide = %+v with target %q, want a cleared highlight", got, s.Target())
		}
	})

	t.Run("a best match that is not drawn falls through to the default", func(t *testing.T) {
		var s Session
		s.Summon(rows, true)
		got := s.Decide(rows, Update{CurrentWindowDrawn: true, SearchActive: true, SearchBestID: "gone"})
		if got.Reason != ReasonDefault || got.TargetID != "b" {
			t.Fatalf("Decide = %+v, want the default on b", got)
		}
	})
}

// Closing a switcher must leave nothing behind for the next one.
func TestCloseReturnsTheSessionToItsZeroValue(t *testing.T) {
	rows := []Row{real("a"), real("b")}
	var s Session
	s.Summon(rows, true)
	s.Pick(rows, "a")
	s.Cycle(rows, 1)
	s.Close()
	if s.Target() != "" || s.UserPicked() || s.VisibleAtSummon() != 0 {
		t.Fatalf("Close left target %q userPicked %v visibleAtSummon %d", s.Target(), s.UserPicked(), s.VisibleAtSummon())
	}
	got := s.Summon(rows, true)
	if got.Index != 1 || got.TargetID != "b" {
		t.Fatalf("Summon after Close = %+v, want the default on b", got)
	}
}
