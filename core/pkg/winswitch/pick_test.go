package winswitch

import (
	"strings"
	"testing"
)

// real, placeholder, minimized and newcomer build the row shapes the pick has
// to tell apart. A placeholder stands in for an app owning no window, a
// minimized window is on the dock, and a newcomer appeared after the press.
func real(id string) Row        { return Row{ID: id} }
func placeholder(id string) Row { return Row{ID: id, Windowless: true} }
func minimized(id string) Row   { return Row{ID: id, Minimized: true} }
func newcomer(id string) Row    { return Row{ID: id, AppearedAfterSummon: true} }

func ids(rows []Row) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.ID)
	}
	return out
}

// Behavior 1: the default is the SECOND VISIBLE window, counting visible tiles
// and never raw indices. The failure the count guards against is concrete — a
// placeholder or a minimized window at index 0 with the current window at
// index 1 means counting indices hands back the window already on top of the
// screen.
func TestDefaultPickCountsVisibleTilesNotRawIndexes(t *testing.T) {
	tests := []struct {
		name string
		rows []Row
		want int
	}{
		{
			name: "three windows pick the second",
			rows: []Row{real("a"), real("b"), real("c")},
			want: 1,
		},
		{
			name: "a placeholder at 0 does not shift the pick onto the current window",
			rows: []Row{placeholder("app"), real("current"), real("other")},
			want: 2,
		},
		{
			name: "a minimized window and a placeholder are both stepped over",
			rows: []Row{minimized("dock"), placeholder("app"), real("a"), real("b")},
			want: 3,
		},
		{
			name: "one visible window wraps to itself",
			rows: []Row{real("a"), placeholder("app")},
			want: 0,
		},
		{
			name: "nothing visible picks nothing",
			rows: []Row{placeholder("app"), minimized("dock")},
			want: NoIndex,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SecondVisibleIndex(tt.rows); got != tt.want {
				t.Fatalf("SecondVisibleIndex(%v) = %d, want %d", ids(tt.rows), got, tt.want)
			}
			// The default pick is that same tile whenever the current window is
			// drawn and nothing arrived behind the switcher.
			in := PickInputs{CurrentWindowDrawn: true, VisibleAtSummon: 99}
			if got := DefaultPickIndex(tt.rows, in); got != tt.want {
				t.Fatalf("DefaultPickIndex(%v) = %d, want %d", ids(tt.rows), got, tt.want)
			}
		})
	}
}

// Behavior 2: the front tile is stepped over because it is the window the user
// is on, so it is not stepped over when it is not drawn. #5941: a filter that
// removes the frontmost app leaves the front tile already standing in for the
// window on top, and stepping over it handed the user a window they never
// chose and lost the two-window toggle.
func TestPickLandsOnTheFrontTileWhenTheCurrentWindowIsNotDrawn(t *testing.T) {
	tests := []struct {
		name string
		rows []Row
		in   PickInputs
		want int
	}{
		{
			name: "the control: with the current window drawn the pick is tile 1",
			rows: []Row{real("a"), real("b")},
			in:   PickInputs{CurrentWindowDrawn: true, VisibleAtSummon: 2},
			want: 1,
		},
		{
			name: "with the current window filtered out the pick is the front tile",
			rows: []Row{real("a"), real("b")},
			in:   PickInputs{CurrentWindowDrawn: false, VisibleAtSummon: 2},
			want: 0,
		},
		{
			name: "a single tile left lands on it",
			rows: []Row{real("a")},
			in:   PickInputs{CurrentWindowDrawn: false, VisibleAtSummon: 1},
			want: 0,
		},
		{
			name: "the tile count is over drawn tiles, not raw indexes",
			rows: []Row{placeholder("app"), real("a"), real("b")},
			in:   PickInputs{CurrentWindowDrawn: false, VisibleAtSummon: 2},
			want: 1,
		},
		{
			name: "it composes with the arrival step-over: the arrival is skipped, then the front tile",
			rows: []Row{newcomer("late"), real("a"), real("b")},
			in:   PickInputs{CurrentWindowDrawn: false, VisibleAtSummon: 2},
			want: 1,
		},
		{
			name: "a replacement newcomer keeps the pick on the front tile",
			rows: []Row{newcomer("late"), real("b")},
			in:   PickInputs{CurrentWindowDrawn: false, VisibleAtSummon: 2},
			want: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DefaultPickIndex(tt.rows, tt.in); got != tt.want {
				t.Fatalf("DefaultPickIndex(%v) = %d, want %d", ids(tt.rows), got, tt.want)
			}
		})
	}
}

// Behavior 4: "the window you were on before" is a question about the moment
// the shortcut was pressed, so an arrival behind the switcher is stepped over
// and a replacement is not. The list length is what tells them apart: a
// newcomer that took a departing window's tile moved nothing down, so stepping
// over it aims a tile too far — two Finder windows with a tab switched is the
// live case, where tile 1 was still the other Finder window.
func TestNewcomersAreSteppedOverOnlyWhileTheListGrows(t *testing.T) {
	tests := []struct {
		name            string
		rows            []Row
		visibleAtSummon int
		want            int
	}{
		{
			name:            "an arrival is stepped over",
			rows:            []Row{newcomer("late"), real("a"), real("b")},
			visibleAtSummon: 2,
			want:            2,
		},
		{
			name:            "a newcomer that took a departing window's tile is not stepped over",
			rows:            []Row{newcomer("late"), real("b")},
			visibleAtSummon: 2,
			want:            1,
		},
		{
			name:            "two newcomers, one of them a replacement: only the arrival is dropped",
			rows:            []Row{real("a"), newcomer("replacement"), newcomer("arrival"), real("c")},
			visibleAtSummon: 3,
			want:            2,
		},
		{
			name:            "an arrival appended behind the front leaves the front undisturbed",
			rows:            []Row{real("a"), real("b"), newcomer("late")},
			visibleAtSummon: 2,
			want:            1,
		},
		{
			name:            "stepping over every drawn window falls back rather than returning nothing",
			rows:            []Row{newcomer("late"), newcomer("later")},
			visibleAtSummon: 0,
			want:            1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := PickInputs{CurrentWindowDrawn: true, VisibleAtSummon: tt.visibleAtSummon}
			if got := DefaultPickIndex(tt.rows, in); got != tt.want {
				t.Fatalf("DefaultPickIndex(%v) = %d, want %d", ids(tt.rows), got, tt.want)
			}
		})
	}
}

// A row the default may not land on is one the user cannot be looking at, so
// the shell's own flag and the kernel's skip rule have to agree on which rows
// those are. A hidden app is not among them: its windows are still windows.
func TestSkippableCoversExactlyTheRowsThatCannotBeTheCurrentWindow(t *testing.T) {
	tests := []struct {
		row  Row
		want bool
	}{
		{real("window"), false},
		{placeholder("app"), true},
		{minimized("dock"), true},
		{newcomer("late"), false},
		{Row{ID: "hidden", Hidden: true}, false},
	}
	for _, tt := range tests {
		if got := tt.row.Skippable(); got != tt.want {
			t.Errorf("Skippable(%+v) = %v, want %v", tt.row, got, tt.want)
		}
	}
}

// The default scans a list whose placeholders and minimized windows are
// interleaved with the real ones in a shape no index arithmetic survives.
func TestDefaultPickSurvivesAnInterleavedList(t *testing.T) {
	rows := []Row{
		placeholder("app-finder"),
		minimized("dock-notes"),
		real("current"),
		newcomer("late"),
		real("previous"),
		placeholder("app-editor"),
		real("other"),
	}
	// The newcomer at index 3 is a real window, so it is the second visible
	// tile; it arrived after a press that saw two candidates, so the default
	// steps over it and lands on "previous" at index 4.
	in := PickInputs{CurrentWindowDrawn: true, VisibleAtSummon: 2}
	if got := DefaultPickIndex(rows, in); got != 4 {
		t.Fatalf("DefaultPickIndex = %d, want 4 (previous)", got)
	}
	if got := FirstVisibleIndex(rows); got != 2 {
		t.Fatalf("FirstVisibleIndex = %d, want 2 (current)", got)
	}
	if got := SecondVisibleIndex(rows); got != 3 {
		t.Fatalf("SecondVisibleIndex = %d, want 3 (the newcomer is a window)", got)
	}
}

// The ids a failure message prints have to be the ids, not the structs.
func TestIdsAreReadableInAFailure(t *testing.T) {
	if got := strings.Join(ids([]Row{real("a"), placeholder("app")}), ","); got != "a,app" {
		t.Fatalf("ids() joined = %q, want %q", got, "a,app")
	}
}
