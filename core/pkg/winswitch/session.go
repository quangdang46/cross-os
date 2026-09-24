package winswitch

// Reason names the rule that produced a Decision, so a test can pin the rule
// instead of re-deriving it from the index.
type Reason string

const (
	ReasonNone        Reason = "none"
	ReasonEmpty       Reason = "empty"
	ReasonSearchClear Reason = "search-clear"
	ReasonSearchBest  Reason = "search-best"
	ReasonDefault     Reason = "default"
	ReasonFollow      Reason = "follow-target"
	ReasonAdapt       Reason = "adapt"
	ReasonCycle       Reason = "cycle"
	ReasonUserPick    Reason = "user-pick"
)

// Decision is one resolved highlight: the row index to draw, the window the
// target now names, and the rule that chose it. Index is NoIndex when nothing
// is drawn and the highlight is cleared.
type Decision struct {
	Index    int
	TargetID string
	Reason   Reason
}

// Update is what a refresh adds on top of the session: the current-window
// answer only the shell can give, and the query state. SearchBestID is the
// best match of the query that just changed.
type Update struct {
	CurrentWindowDrawn bool
	SearchActive       bool
	SearchBestID       string
}

// Session is one open switcher. The zero value is a closed session, ready to
// be summoned over; the shell owns one value and resets it on close.
//
// Its whole reason for existing is the split it keeps. Before the user moves
// the highlight, the target is only where the DEFAULT landed, so every refresh
// re-derives it — the switcher opens while the window set is still settling
// and a default that locked onto a slot mid-churn trailed that window down the
// list to an unrelated tile. Once the user cycles or hovers, the same field is
// a commitment and is followed by id however the list reorders.
type Session struct {
	targetID        string
	userPicked      bool
	lastIndex       int
	visibleAtSummon int
	searchActive    bool
}

// Summon opens a session over the drawn rows and returns the first pick. The
// candidate count is measured here, on the first selection pass — the same
// main-thread turn as the press, so no window event can land between the press
// and the count it is compared against.
func (s *Session) Summon(rows []Row, currentWindowDrawn bool) Decision {
	s.targetID = ""
	s.userPicked = false
	s.lastIndex = NoIndex
	s.searchActive = false
	s.visibleAtSummon = len(candidateIndexes(rows))
	return s.defaultDecision(rows, Update{CurrentWindowDrawn: currentWindowDrawn}, ReasonDefault)
}

// Decide resolves one refresh, in the spec's priority order: a cleared query
// re-derives the default, an empty list clears the target, a new query jumps to
// its best match, an untouched default is re-derived, and a target the user
// picked is followed by id — or backfilled when it has left the list.
func (s *Session) Decide(rows []Row, u Update) Decision {
	if s.searchActive && !u.SearchActive {
		// Clearing the query restores the default even against an empty list.
		// A target the user picked keeps its commitment from here on.
		s.searchActive = false
		return s.defaultDecision(rows, u, ReasonSearchClear)
	}
	if len(candidateIndexes(rows)) == 0 {
		return s.clear()
	}
	if u.SearchActive {
		s.searchActive = true
		if i, ok := indexOf(rows, u.SearchBestID); ok {
			return s.land(rows, i, ReasonSearchBest)
		}
	}
	if !s.userPicked {
		return s.defaultDecision(rows, u, ReasonDefault)
	}
	if i, ok := indexOf(rows, s.targetID); ok {
		return s.land(rows, i, ReasonFollow)
	}
	// The target left: backfill onto whatever occupies its slot.
	i := closestBelow(rows, s.lastIndex)
	if i == NoIndex {
		return s.clear()
	}
	return s.land(rows, i, ReasonAdapt)
}

// Cycle moves the highlight the way the user does, one candidate forward or
// back with wraparound. It is one of the two ways a default becomes a
// commitment; a session with no highlight yet steps onto the first candidate.
func (s *Session) Cycle(rows []Row, delta int) Decision {
	cands := candidateIndexes(rows)
	if len(cands) == 0 {
		return s.clear()
	}
	at := -1
	for n, i := range cands {
		if i == s.lastIndex {
			at = n
			break
		}
	}
	s.userPicked = true
	return s.land(rows, cands[wrap(at+delta, len(cands))], ReasonCycle)
}

// Pick commits to the window the user pointed at. The window must be drawn; an
// id that is not in the list is a stale hover and commits to nothing, leaving
// the session as it was.
func (s *Session) Pick(rows []Row, id string) Decision {
	i, ok := indexOf(rows, id)
	if !ok {
		return Decision{Index: NoIndex, Reason: ReasonNone}
	}
	s.userPicked = true
	return s.land(rows, i, ReasonUserPick)
}

// Close drops the session back to its zero value.
func (s *Session) Close() { *s = Session{} }

// Target is the window the highlight is on, or "" while none is drawn.
func (s *Session) Target() string { return s.targetID }

// UserPicked reports whether the target is a commitment rather than a default
// that re-derives on every refresh.
func (s *Session) UserPicked() bool { return s.userPicked }

// VisibleAtSummon is the candidate count the press was measured against.
func (s *Session) VisibleAtSummon() int { return s.visibleAtSummon }

// defaultDecision re-derives the pick from scratch, which is what an untouched
// target must do on every refresh. It overwrites the stored target, so the
// default never trails a window that slid down the list.
func (s *Session) defaultDecision(rows []Row, u Update, why Reason) Decision {
	i := DefaultPickIndex(rows, PickInputs{
		CurrentWindowDrawn: u.CurrentWindowDrawn,
		VisibleAtSummon:    s.visibleAtSummon,
	})
	if i == NoIndex {
		return s.clear()
	}
	return s.land(rows, i, why)
}

func (s *Session) land(rows []Row, i int, why Reason) Decision {
	s.targetID = rows[i].ID
	s.lastIndex = i
	return Decision{Index: i, TargetID: s.targetID, Reason: why}
}

func (s *Session) clear() Decision {
	s.targetID = ""
	s.lastIndex = NoIndex
	return Decision{Index: NoIndex, Reason: ReasonEmpty}
}

// indexOf finds a drawn row by id.
func indexOf(rows []Row, id string) (int, bool) {
	if id == "" {
		return 0, false
	}
	for i, r := range rows {
		if r.ID == id {
			return i, true
		}
	}
	return 0, false
}

// closestBelow backs a vanished target onto the window now occupying its slot:
// the same index when a window is there, else the next candidate down, and the
// last one when the list shrank below the slot. Wrapping to the front instead
// would put the highlight on the window furthest from where the user left it.
func closestBelow(rows []Row, at int) int {
	cands := candidateIndexes(rows)
	for _, i := range cands {
		if i >= at {
			return i
		}
	}
	if len(cands) == 0 {
		return NoIndex
	}
	return cands[len(cands)-1]
}

// wrap normalizes an index that stepped off either end into [0, n).
func wrap(i, n int) int {
	i %= n
	if i < 0 {
		i += n
	}
	return i
}
