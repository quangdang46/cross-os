package winswitch

import (
	"fmt"
	"sort"
	"testing"
)

func hit(id string, rank, mru int) Row {
	return Row{ID: id, LastFocusOrder: mru, SearchHit: true, SearchRank: rank}
}

// Behavior 5: two rows equal on every fact the comparator reads are not
// ordered before each other. sort.Slice requires a strict weak ordering and
// panics outright on a comparator that is not one, so the sort below is half
// the test — the equality assertions are the other half.
func TestEqualWindowsAreNotOrderedBeforeEachOther(t *testing.T) {
	full := Row{
		ID: "w", AppID: "app", AppName: "Editor", Title: "Notes",
		LastFocusOrder: 3, CreationOrder: 7, SpaceIndex: 1,
		SearchHit: true, SearchRank: 4,
	}
	same := full
	for _, o := range allOrderOptions() {
		t.Run(optionName(o), func(t *testing.T) {
			if Less(full, same, o) || Less(same, full, o) {
				t.Fatalf("equal rows are ordered before each other under %+v", o)
			}
			// The same row against itself, which is the case a caller hits by
			// re-sorting a list it already sorted.
			if Less(full, full, o) {
				t.Fatal("a row is ordered before itself")
			}
		})
	}
}

func TestSortKeepsEqualRowsFromBreakingTheOrdering(t *testing.T) {
	rows := []Row{
		{ID: "a", AppName: "Alpha", Title: "one", LastFocusOrder: 0},
		{ID: "b", AppName: "Beta", Title: "two", LastFocusOrder: 1},
		{ID: "c", AppName: "Alpha", Title: "one", LastFocusOrder: 0},
		{ID: "d", AppName: "Alpha", Title: "one", LastFocusOrder: 0},
		{ID: "e", AppName: "Gamma", Title: "three", LastFocusOrder: 2, Windowless: true},
		{ID: "f", AppName: "Delta", Title: "four", LastFocusOrder: 3, Minimized: true},
		{ID: "g", AppName: "Gamma", Title: "three", LastFocusOrder: 2, Windowless: true},
		{ID: "h", AppName: "Alpha", Title: "one", LastFocusOrder: 0},
	}
	for _, o := range allOrderOptions() {
		t.Run(optionName(o), func(t *testing.T) {
			work := append([]Row(nil), rows...)
			// sort.Slice panics on a comparator that is not a strict weak
			// ordering; reaching the assertions below is the no-panic half.
			sort.Slice(work, func(i, j int) bool { return Less(work[i], work[j], o) })
			for i := 1; i < len(work); i++ {
				if Less(work[i], work[i-1], o) {
					t.Fatalf("not sorted at %d under %+v: %v", i, o, ids(work))
				}
			}
			// Sorting twice must reach the same order: the equal rows carry no
			// defined order among themselves, but the list never flips a pair
			// that has one.
			again := append([]Row(nil), work...)
			Sort(again, o)
			for i := range work {
				if work[i].ID != again[i].ID {
					t.Fatalf("re-sorting moved %s past %s under %+v: %v", work[i].ID, again[i].ID, o, ids(again))
				}
			}
		})
	}
}

// optionName labels one knob combination, so a failure names the combination
// that broke rather than an opaque index into the sweep.
func optionName(o OrderOptions) string {
	return fmt.Sprintf("mode=%d/search=%t/windowless=%t/hidden=%t/minimized=%t",
		o.Mode, o.SearchActive, o.WindowlessAtEnd, o.HiddenAtEnd, o.MinimizedAtEnd)
}

// Behavior 6: a show-at-the-end bucket separates two rows only while its flag
// is set and the two actually differ on that trait. With the flag off the
// trait is a fact the ordering does not look at, and the sort type decides.
func TestShowAtEndBucketsOnlySeparateWhenTheFlagIsSet(t *testing.T) {
	tests := []struct {
		name string
		opts OrderOptions
		a, b Row
		want bool
	}{
		{
			name: "a windowless row sinks below a real window when configured",
			opts: OrderOptions{WindowlessAtEnd: true},
			a:    Row{ID: "real", LastFocusOrder: 1},
			b:    Row{ID: "app", LastFocusOrder: 0, Windowless: true},
			want: true,
		},
		{
			name: "and the real window is not ordered after it",
			opts: OrderOptions{WindowlessAtEnd: true},
			a:    Row{ID: "app", LastFocusOrder: 0, Windowless: true},
			b:    Row{ID: "real", LastFocusOrder: 1},
			want: false,
		},
		{
			name: "with the bucket off the sort type decides",
			opts: OrderOptions{},
			a:    Row{ID: "app", LastFocusOrder: 0, Windowless: true},
			b:    Row{ID: "real", LastFocusOrder: 1},
			want: true,
		},
		{
			name: "the bucket does not separate two rows that agree on the trait",
			opts: OrderOptions{WindowlessAtEnd: true},
			a:    Row{ID: "app-a", LastFocusOrder: 0, Windowless: true},
			b:    Row{ID: "app-b", LastFocusOrder: 1, Windowless: true},
			want: true,
		},
		{
			name: "a hidden row sinks when configured",
			opts: OrderOptions{HiddenAtEnd: true},
			a:    Row{ID: "real", LastFocusOrder: 1},
			b:    Row{ID: "hidden", LastFocusOrder: 0, Hidden: true},
			want: true,
		},
		{
			name: "with the hidden bucket off the sort type decides",
			opts: OrderOptions{},
			a:    Row{ID: "hidden", LastFocusOrder: 0, Hidden: true},
			b:    Row{ID: "real", LastFocusOrder: 1},
			want: true,
		},
		{
			name: "a minimized row sinks when configured",
			opts: OrderOptions{MinimizedAtEnd: true},
			a:    Row{ID: "real", LastFocusOrder: 1},
			b:    Row{ID: "dock", LastFocusOrder: 0, Minimized: true},
			want: true,
		},
		{
			name: "with the minimized bucket off the sort type decides",
			opts: OrderOptions{},
			a:    Row{ID: "dock", LastFocusOrder: 0, Minimized: true},
			b:    Row{ID: "real", LastFocusOrder: 1},
			want: true,
		},
		{
			name: "a flag on one trait does not separate rows that differ on another",
			opts: OrderOptions{WindowlessAtEnd: true},
			a:    Row{ID: "hidden", LastFocusOrder: 0, Hidden: true},
			b:    Row{ID: "real", LastFocusOrder: 1},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Less(tt.a, tt.b, tt.opts); got != tt.want {
				t.Fatalf("Less(%s, %s) = %v, want %v", tt.a.ID, tt.b.ID, got, tt.want)
			}
			// Rows equal on every compared field are ordered neither way, so
			// only a verdict of true has to be mirrored by a false one.
			if tt.want && Less(tt.b, tt.a, tt.opts) {
				t.Fatalf("both %s and %s are ordered first under %+v", tt.a.ID, tt.b.ID, tt.opts)
			}
		})
	}
}

// The order the spec fixes: search, then the buckets, then the sort type, then
// the tiebreak. Each case states which field decided it.
func TestOrderDecisionOrder(t *testing.T) {
	search := OrderOptions{SearchActive: true}
	tests := []struct {
		name string
		opts OrderOptions
		a, b Row
		want bool
	}{
		{
			name: "a matched row sorts before an unmatched one",
			opts: search,
			a:    hit("match", 1, 5),
			b:    Row{ID: "miss", LastFocusOrder: 0},
			want: true,
		},
		{
			name: "higher relevance sorts first",
			opts: search,
			a:    hit("better", 9, 5),
			b:    hit("worse", 2, 0),
			want: true,
		},
		{
			name: "equal relevance tiebreaks by last focus order",
			opts: search,
			a:    hit("older", 4, 7),
			b:    hit("newer", 4, 1),
			want: false,
		},
		{
			name: "search is inert while no query is active",
			opts: OrderOptions{},
			a:    Row{ID: "miss", LastFocusOrder: 0},
			b:    hit("match", 9, 7),
			want: true,
		},
		{
			name: "recently focused is the lowest last focus order first",
			opts: OrderOptions{},
			a:    Row{ID: "now", LastFocusOrder: 0},
			b:    Row{ID: "old", LastFocusOrder: 4},
			want: true,
		},
		{
			name: "recently created is the highest creation order first",
			opts: OrderOptions{Mode: SortRecentlyCreated},
			a:    Row{ID: "new", CreationOrder: 9, LastFocusOrder: 9},
			b:    Row{ID: "old", CreationOrder: 2, LastFocusOrder: 0},
			want: true,
		},
		{
			name: "recently created has no tiebreak beyond creation order",
			opts: OrderOptions{Mode: SortRecentlyCreated},
			a:    Row{ID: "a", CreationOrder: 4, LastFocusOrder: 0},
			b:    Row{ID: "b", CreationOrder: 4, LastFocusOrder: 7},
			want: false,
		},
		{
			name: "alphabetical is by app name",
			opts: OrderOptions{Mode: SortAlphabetical},
			a:    Row{ID: "a", AppName: "Alpha", Title: "z", LastFocusOrder: 5},
			b:    Row{ID: "b", AppName: "Beta", Title: "a", LastFocusOrder: 0},
			want: true,
		},
		{
			name: "alphabetical breaks a same-app tie by title",
			opts: OrderOptions{Mode: SortAlphabetical},
			a:    Row{ID: "a", AppName: "App", Title: "alpha", LastFocusOrder: 5},
			b:    Row{ID: "b", AppName: "App", Title: "beta", LastFocusOrder: 0},
			want: true,
		},
		{
			name: "identical app and title tiebreak by last focus order",
			opts: OrderOptions{Mode: SortAlphabetical},
			a:    Row{ID: "a", AppName: "App", Title: "same", LastFocusOrder: 6},
			b:    Row{ID: "b", AppName: "App", Title: "same", LastFocusOrder: 2},
			want: false,
		},
		{
			name: "space puts all-spaces windows first",
			opts: OrderOptions{Mode: SortSpace},
			a:    Row{ID: "a", AppName: "Zeta", OnAllSpaces: true, LastFocusOrder: 5},
			b:    Row{ID: "b", AppName: "Alpha", SpaceIndex: 0, LastFocusOrder: 0},
			want: true,
		},
		{
			name: "and the mirror holds: only b on all spaces sorts b first",
			opts: OrderOptions{Mode: SortSpace},
			a:    Row{ID: "b", AppName: "Zeta", OnAllSpaces: true, LastFocusOrder: 0},
			b:    Row{ID: "a", AppName: "Alpha", SpaceIndex: 0, LastFocusOrder: 5},
			want: true,
		},
		{
			name: "space orders by the lower index within a space",
			opts: OrderOptions{Mode: SortSpace},
			a:    Row{ID: "a", AppName: "Zeta", SpaceIndex: 1, LastFocusOrder: 5},
			b:    Row{ID: "b", AppName: "Alpha", SpaceIndex: 2, LastFocusOrder: 0},
			want: true,
		},
		{
			name: "two windows on all spaces fall through to app name",
			opts: OrderOptions{Mode: SortSpace},
			a:    Row{ID: "a", AppName: "Alpha", OnAllSpaces: true, LastFocusOrder: 5},
			b:    Row{ID: "b", AppName: "Beta", OnAllSpaces: true, LastFocusOrder: 0},
			want: true,
		},
		{
			name: "a tie inside one space falls through to app name",
			opts: OrderOptions{Mode: SortSpace},
			a:    Row{ID: "a", AppName: "Alpha", SpaceIndex: 1, LastFocusOrder: 5},
			b:    Row{ID: "b", AppName: "Beta", SpaceIndex: 1, LastFocusOrder: 0},
			want: true,
		},
		{
			name: "the bucket outranks the sort type",
			opts: OrderOptions{WindowlessAtEnd: true},
			a:    Row{ID: "real", LastFocusOrder: 7},
			b:    Row{ID: "app", LastFocusOrder: 0, Windowless: true},
			want: true,
		},
		{
			name: "and search outranks the bucket",
			opts: OrderOptions{SearchActive: true, WindowlessAtEnd: true},
			a:    hit("real", 1, 7),
			b:    Row{ID: "app", LastFocusOrder: 0, Windowless: true},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Less(tt.a, tt.b, tt.opts); got != tt.want {
				t.Fatalf("Less(%s, %s) = %v, want %v", tt.a.ID, tt.b.ID, got, tt.want)
			}
			if tt.want && Less(tt.b, tt.a, tt.opts) {
				t.Fatalf("both %s and %s are ordered first", tt.a.ID, tt.b.ID)
			}
		})
	}
}

// allOrderOptions sweeps every knob the comparator reads, so a rule that only
// breaks one combination cannot hide behind the tests that picked a different
// one.
func allOrderOptions() []OrderOptions {
	var out []OrderOptions
	for mode := SortRecentlyFocused; mode <= SortSpace; mode++ {
		for _, search := range []bool{false, true} {
			for _, windowless := range []bool{false, true} {
				for _, hidden := range []bool{false, true} {
					for _, minimized := range []bool{false, true} {
						out = append(out, OrderOptions{
							Mode:            mode,
							SearchActive:    search,
							WindowlessAtEnd: windowless,
							HiddenAtEnd:     hidden,
							MinimizedAtEnd:  minimized,
						})
					}
				}
			}
		}
	}
	return out
}

// The comparator is a lexicographic order over per-row fields, so a full sort
// has to reproduce the pairwise verdicts rather than only avoid panicking.
func TestSortReproducesThePairwiseVerdicts(t *testing.T) {
	rows := []Row{
		{ID: "a", AppName: "Alpha", Title: "one", LastFocusOrder: 0, SpaceIndex: 2},
		{ID: "b", AppName: "Beta", Title: "two", LastFocusOrder: 3, SpaceIndex: 1},
		{ID: "c", AppName: "Alpha", Title: "two", LastFocusOrder: 1, OnAllSpaces: true},
		{ID: "d", AppName: "Gamma", Title: "three", LastFocusOrder: 2, Windowless: true},
		{ID: "e", AppName: "Beta", Title: "one", LastFocusOrder: 5, Minimized: true},
		{ID: "f", AppName: "Gamma", Title: "one", LastFocusOrder: 4, SpaceIndex: 0},
	}
	for _, o := range allOrderOptions() {
		work := append([]Row(nil), rows...)
		Sort(work, o)
		for i := 0; i < len(work); i++ {
			for j := i + 1; j < len(work); j++ {
				if Less(work[j], work[i], o) {
					t.Fatalf("%s precedes %s against the comparator under %+v: %v",
						work[i].ID, work[j].ID, o, ids(work))
				}
			}
		}
	}
}
