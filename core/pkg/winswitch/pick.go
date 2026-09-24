package winswitch

// NoIndex is the tile of nothing: an empty drawn list, or a target that left
// it and had nothing to backfill onto.
const NoIndex = -1

// Skippable reports whether the default pick steps over this row. A windowless
// placeholder and a minimized window are neither of them the window the user
// was on before the summon, so neither may hold the default. The grouped-away
// background tabs the MRU also carries are not rows at all: the shell hands
// over the drawn list, so counting its entries is already counting visible
// tiles.
func (r Row) Skippable() bool { return r.Windowless || r.Minimized }

// candidateIndexes returns the raw row indexes the default pick may land on,
// in tile order.
func candidateIndexes(rows []Row) []int {
	out := make([]int, 0, len(rows))
	for i, r := range rows {
		if !r.Skippable() {
			out = append(out, i)
		}
	}
	return out
}

// FirstVisibleIndex is the front candidate tile, or NoIndex.
func FirstVisibleIndex(rows []Row) int {
	for i, r := range rows {
		if !r.Skippable() {
			return i
		}
	}
	return NoIndex
}

// SecondVisibleIndex is the tile the default lands on when the current window
// is drawn: the SECOND visible window — the one the user was on before the
// current one. It counts visible tiles, never raw indices, because index 0 can
// be a placeholder or a minimized window and index 1 can be the current window
// itself: counting indices there picked the window already on top of the
// screen. A single visible window wraps to itself.
func SecondVisibleIndex(rows []Row) int {
	seen := 0
	for i, r := range rows {
		if r.Skippable() {
			continue
		}
		seen++
		if seen == 2 {
			return i
		}
	}
	return FirstVisibleIndex(rows)
}

// PickInputs are the two answers only the shell can give: whether the window on
// top of the screen is being kept out of the drawn list, and how many
// candidates the list held at the press.
type PickInputs struct {
	// CurrentWindowDrawn is false when a filter removed the whole frontmost app
	// or the current window itself, so the front tile already is "the window
	// you were on before" and stepping over it would hand the user a window
	// they never chose.
	CurrentWindowDrawn bool

	// VisibleAtSummon is the candidate count measured on the first selection
	// pass after the press.
	VisibleAtSummon int
}

// DefaultPickIndex is the default tile for a refresh: the front candidate when
// the current window is not drawn, the second one otherwise, taken over the
// candidates left after the arrivals are stepped over.
func DefaultPickIndex(rows []Row, in PickInputs) int {
	cands := candidateIndexes(rows)
	if len(cands) == 0 {
		return NoIndex
	}
	kept := stepOverNewcomers(rows, cands, in.VisibleAtSummon)
	if !in.CurrentWindowDrawn || len(kept) == 1 {
		return kept[0]
	}
	return kept[1]
}

// stepOverNewcomers drops the tiles that arrived behind the switcher, but only
// as many as the list actually grew. The MRU the pick reads is the one as of
// the summon: "the window you were on before" is a question about the moment
// the shortcut was pressed, and a window that took a departing window's tile
// moved nothing down, so stepping over it aims a tile too far.
func stepOverNewcomers(rows []Row, cands []int, visibleAtSummon int) []int {
	gained := len(cands) - visibleAtSummon
	if gained <= 0 {
		return cands
	}
	kept := make([]int, 0, len(cands))
	stepped := 0
	for _, i := range cands {
		if stepped < gained && rows[i].AppearedAfterSummon {
			stepped++
			continue
		}
		kept = append(kept, i)
	}
	if len(kept) == 0 {
		// Everything drawn arrived after the press. The plain rule takes over
		// rather than returning nothing.
		return cands
	}
	return kept
}
