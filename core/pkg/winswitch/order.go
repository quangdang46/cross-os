package winswitch

import (
	"sort"
	"strings"
)

// Row is one entry of the switcher list: a real window, or a windowless
// placeholder standing in for an app that owns nothing drawn. Ordering and
// picking read the same snapshot, so the facts the adapter already computed —
// the filters that hid an app, the rank a query assigned — travel with the row
// instead of being re-derived once per comparison.
type Row struct {
	ID    string
	AppID string

	AppName string
	Title   string

	// LastFocusOrder is the MRU rank: 0 is the most recently focused and
	// higher is older. CreationOrder is the app's launch order, higher newer.
	LastFocusOrder int
	CreationOrder  int

	// SpaceIndex is the bound space, 0-based; OnAllSpaces windows are not
	// bound to any.
	SpaceIndex  int
	OnAllSpaces bool

	// Windowless marks an app placeholder, Hidden an app the user hid,
	// Minimized a window sent to the dock. The last three are the traits the
	// show-at-the-end buckets separate on, and the first two of them also
	// decide whether the default pick may land on this row.
	Windowless bool
	Hidden     bool
	Minimized  bool

	// SearchHit and SearchRank carry the rank the query assigned. Rank is only
	// read between two hits; the shell decides what a miss ranks as.
	SearchHit  bool
	SearchRank int

	// AppearedAfterSummon marks a window the model learned about after the
	// shortcut was pressed — absent from the list at the press, not merely
	// focused since.
	AppearedAfterSummon bool
}

// SortMode names the order the user's preference asks for.
type SortMode int

const (
	// SortRecentlyFocused is MRU order and the zero value.
	SortRecentlyFocused SortMode = iota
	SortRecentlyCreated
	SortAlphabetical
	SortSpace
)

// OrderOptions are the sort knobs, labeled parameters hoisted once per sort
// rather than read per comparison. The zero value is MRU order with every
// bucket off and no query.
type OrderOptions struct {
	Mode            SortMode
	SearchActive    bool
	WindowlessAtEnd bool
	HiddenAtEnd     bool
	MinimizedAtEnd  bool
}

// Less reports whether a is ordered before b. The decision order is the
// spec's, and each step only decides when the two rows differ on it:
//
//  1. search — matched first, then higher relevance, then lastFocusOrder
//  2. show-at-the-end buckets, each of which separates only while its flag is
//     set
//  3. the sort type
//  4. the lastFocusOrder tiebreak, on the alphabetical and space paths
//
// A fixed OrderOptions makes this a lexicographic order over per-row fields,
// so it is a strict weak ordering — which is what sort.Slice requires and what
// two rows equal on every field exercise: neither is ordered before the other.
func Less(a, b Row, o OrderOptions) bool {
	if o.SearchActive {
		if a.SearchHit != b.SearchHit {
			return a.SearchHit
		}
		if a.SearchRank != b.SearchRank {
			return a.SearchRank > b.SearchRank
		}
		if a.LastFocusOrder != b.LastFocusOrder {
			return a.LastFocusOrder < b.LastFocusOrder
		}
	}
	if o.WindowlessAtEnd && a.Windowless != b.Windowless {
		return !a.Windowless
	}
	if o.HiddenAtEnd && a.Hidden != b.Hidden {
		return !a.Hidden
	}
	if o.MinimizedAtEnd && a.Minimized != b.Minimized {
		return !a.Minimized
	}
	switch o.Mode {
	case SortRecentlyCreated:
		// No tiebreak: a window that started at the same instant as another
		// has no older one, and inventing an order would be a ranking this
		// kernel has no evidence for.
		return a.CreationOrder > b.CreationOrder
	case SortAlphabetical:
		// Byte order, not locale collation: a switcher sorts a handful of tiles
		// at a keystroke, and an order the tests can pin is worth more here
		// than the one a user's locale would choose.
		if c := strings.Compare(a.AppName, b.AppName); c != 0 {
			return c < 0
		}
		if c := strings.Compare(a.Title, b.Title); c != 0 {
			return c < 0
		}
		return a.LastFocusOrder < b.LastFocusOrder
	case SortSpace:
		if a.OnAllSpaces != b.OnAllSpaces {
			return a.OnAllSpaces
		}
		if !a.OnAllSpaces && a.SpaceIndex != b.SpaceIndex {
			return a.SpaceIndex < b.SpaceIndex
		}
		if c := strings.Compare(a.AppName, b.AppName); c != 0 {
			return c < 0
		}
		if c := strings.Compare(a.Title, b.Title); c != 0 {
			return c < 0
		}
		return a.LastFocusOrder < b.LastFocusOrder
	default:
		return a.LastFocusOrder < b.LastFocusOrder
	}
}

// Sort orders rows in place under o. It is not stable: rows equal on every
// compared field keep no defined order among themselves, and the kernel
// refuses to invent one.
func Sort(rows []Row, o OrderOptions) {
	sort.Slice(rows, func(i, j int) bool { return Less(rows[i], rows[j], o) })
}
