package main

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode"

	"crossos/core/pkg/pluginapi"
)

// The page list, and what a client in another language is owed by it.
//
// These tests exist because the failure they guard against is invisible from
// the Go side: the daemon answering with an empty list, or with a page whose
// id is not what the Swift client decodes, both leave a Go test suite green
// and a window with no pages in it.

// servedPages runs the handler the way the IPC layer does — marshal a request,
// unmarshal the answer — so a type that does not survive the round trip fails
// here rather than in another language.
func servedPages(t *testing.T) []servedPage {
	t.Helper()
	c := testCore(t)
	raw, rpcErr := c.handleCorePages(nil)
	if rpcErr != nil {
		t.Fatalf("core.pages: %v", rpcErr)
	}
	// The same trip ipc.Handler results take: the daemon writes JSON, and
	// whatever reads it sees only what JSON can carry.
	encoded, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshalling the page list: %v", err)
	}
	var out []servedPage
	if err := json.Unmarshal(encoded, &out); err != nil {
		t.Fatalf("unmarshalling the page list: %v", err)
	}
	return out
}

func TestCorePagesServesTheFifteenCorePages(t *testing.T) {
	pages := servedPages(t)

	// Fifteen is the number `pluginapi.Pages()` returns. A page that vanishes
	// is a page the product no longer has, and the test that notices is this
	// one rather than a person opening a window.
	if len(pages) != 15 {
		var ids []string
		for _, p := range pages {
			ids = append(ids, p.ID)
		}
		t.Errorf("core.pages served %d pages, want 15: %s", len(pages), strings.Join(ids, ", "))
	}
}

func TestCorePagesHaveDistinctIDs(t *testing.T) {
	seen := map[string]bool{}
	for _, p := range servedPages(t) {
		if seen[p.ID] {
			t.Errorf("page id %q is served twice — the nav would show one page as two entries", p.ID)
		}
		seen[p.ID] = true
	}
}

// A page id is namespaced and dotted, and the Swift registry's coverage test
// asserts that no registered KIND is dotted. If a page stopped being dotted
// the two guards would meet in the middle and a kind could be named after a
// screen.
func TestCorePagesIDsAreNamespaced(t *testing.T) {
	for _, p := range servedPages(t) {
		if !strings.Contains(p.ID, ".") {
			t.Errorf("page id %q is not namespaced — ids are <pluginID>.<contribID>", p.ID)
		}
	}
}

// The wizard leads until it is done, then Home does. That tiebreak is the
// shell's (`app/backend/home.go:9-13`), but it depends on exactly one page
// declaring firstRun — two pages claiming it makes the landing page a coin
// toss.
func TestExactlyOnePageDeclaresFirstRun(t *testing.T) {
	var firstRun []string
	for _, p := range servedPages(t) {
		if p.FirstRun {
			firstRun = append(firstRun, p.ID)
		}
	}

	if len(firstRun) != 1 {
		t.Errorf("want exactly one firstRun page, got %d: %v", len(firstRun), firstRun)
	}
	if len(firstRun) == 1 && firstRun[0] != "core.onboard" {
		t.Errorf("firstRun page is %q, want core.onboard", firstRun[0])
	}
}

// Every page needs enough to be drawn: a title for the nav, a group to sort
// it under, and a schema with at least one control. A page missing any of the
// three is a nav entry that opens onto nothing.
func TestEveryPageIsDrawable(t *testing.T) {
	for _, p := range servedPages(t) {
		if p.Title == "" {
			t.Errorf("page %q has no title — the nav draws the title", p.ID)
		}
		if p.Group == "" {
			t.Errorf("page %q has no group — a page with no group sits on its own, which is not what any of these are", p.ID)
		}
		if len(p.Schema) == 0 {
			t.Errorf("page %q has no schema — the nav would open onto an empty window", p.ID)
			continue
		}
		var body struct {
			Controls []map[string]any `json:"controls"`
		}
		if err := json.Unmarshal(p.Schema, &body); err != nil {
			t.Errorf("page %q has a schema that is not an object: %v", p.ID, err)
			continue
		}
		if len(body.Controls) == 0 {
			t.Errorf("page %q has no controls", p.ID)
		}
	}
}

// Every control names a kind, because the shell dispatches on it and a control
// without one falls to the unsupported path. The kind is also the only thing
// that survives to the Swift registry, so a misspelled kind is a blank row on
// somebody's page.
func TestEveryControlNamesAKind(t *testing.T) {
	for _, p := range servedPages(t) {
		var body struct {
			Controls []map[string]any `json:"controls"`
		}
		if err := json.Unmarshal(p.Schema, &body); err != nil {
			continue // already reported by TestEveryPageIsDrawable
		}
		for i, control := range body.Controls {
			kind, _ := control["kind"].(string)
			if kind == "" {
				t.Errorf("page %q control %d has no kind — the shell cannot dispatch it", p.ID, i)
			}
		}
	}
}

// The order the pages are authored in is the order the nav sees until it sorts
// on Group/Order/firstRun. Pinning it means adding a page cannot silently
// reorder the nav by living in a different place in the slice.
func TestServedPageOrderIsStable(t *testing.T) {
	first, second := servedPages(t), servedPages(t)
	if len(first) != len(second) {
		t.Fatalf("two calls served different lengths: %d and %d", len(first), len(second))
	}
	for i := range first {
		if first[i].ID != second[i].ID {
			t.Fatalf("two calls disagreed at position %d: %q then %q", i, first[i].ID, second[i].ID)
		}
	}

	// And the authored order is what those two calls agree on.
	want := pluginapi.Pages()
	if len(want) != len(first) {
		t.Fatalf("pluginapi.Pages() returned %d pages, core.pages served %d", len(want), len(first))
	}
	for i := range want {
		if want[i].ID != first[i].ID {
			t.Errorf("position %d: authored %q, served %q", i, want[i].ID, first[i].ID)
		}
	}
}

// A malformed schema is a plugin's problem and must not empty the nav — the
// one rule `declaresFirstRun` has to keep, since it is the only place a bad
// schema is touched on the way out.
func TestDeclaresFirstRunSurvivesAMalformedSchema(t *testing.T) {
	cases := []struct {
		name   string
		schema json.RawMessage
		want   bool
	}{
		{"empty", nil, false},
		{"not json", json.RawMessage(`not json at all`), false},
		{"an array", json.RawMessage(`[1,2,3]`), false},
		{"no marker", json.RawMessage(`{"type":"page"}`), false},
		{"marker true", json.RawMessage(`{"firstRun":true}`), true},
		{"marker false", json.RawMessage(`{"firstRun":false}`), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := declaresFirstRun(tc.schema); got != tc.want {
				t.Errorf("declaresFirstRun(%s) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

// Every page's symbol must be an SF Symbol name, and nothing else.
//
// The pages used to carry Unicode geometric shapes — ⌂ ⚙ ⓘ — as the mark
// beside a section title. That was wrong twice: a geometric shape is not a
// symbol and macOS does not draw it as one, and an emoji pasted beside a nav
// title is the loudest possible signal that nobody chose the mark. The
// replacement is `NSImage(systemSymbolName:)`, which means a wrong name is a
// blank row — invisible in a Go test and obvious on screen, which is the wrong
// way round.
//
// So the check is here: a symbol is a lowercase, dot-separated name, which is
// what SF Symbols uses. It is a shape check, not an existence check — this
// codebase cannot ask macOS which symbols exist without being a macOS program,
// and a hardcoded list of every symbol would be stale the moment Apple ships
// one.
func TestEveryPageSymbolIsAnSFSymbolName(t *testing.T) {
	for _, p := range servedPages(t) {
		if p.Symbol == "" {
			continue // allowed: a contribution that chose no mark still renders
		}
		if strings.ContainsFunc(p.Symbol, unicode.IsUpper) {
			t.Errorf("page %q symbol %q looks like a display name, not a symbol name — "+
				"SF Symbols are lowercase (e.g. \"gearshape\")", p.ID, p.Symbol)
		}
		if strings.ContainsAny(p.Symbol, "⌂⚙ⓘ◈⌨⌥▣⇄⌕▤≣◉◫▭✦") {
			t.Errorf("page %q symbol %q is a Unicode glyph — SF Symbols are named, "+
				"and a glyph is decoration rather than a mark", p.ID, p.Symbol)
		}
		if strings.ContainsAny(p.Symbol, " ✨⭐️") {
			t.Errorf("page %q symbol %q contains an emoji", p.ID, p.Symbol)
		}
	}
}

// A section with no symbol is allowed; a section whose symbol is a shape the
// system has never heard of is not, and the difference is invisible until
// somebody looks at the nav. Pinning the names in use turns a rename that
// breaks the shell's lookup into a failing test rather than a blank row.
func TestTheSectionSymbolsAreTheOnesTheShellExpects(t *testing.T) {
	want := map[string]string{
		"core.home":       "house",
		"core.onboard":    "sparkles",
		"core.profiles":   "square.grid.2x2",
		"core.matrix":     "keyboard",
		"core.rules":      "command",
		"core.windows":    "rectangle.3.group",
		"core.switcher":   "arrow.triangle.2.circlepath",
		"core.commands":   "magnifyingglass",
		"core.finder":     "list.bullet.rectangle",
		"core.activity":   "list.bullet.indent",
		"core.observe":    "eye",
		"core.extensions": "puzzlepiece.extension",
		"core.schemaForm": "rectangle",
		"core.safety":     "gearshape",
		"core.about":      "info.circle",
	}

	for _, p := range servedPages(t) {
		if expected, known := want[p.ID]; known && p.Symbol != expected {
			t.Errorf("page %q symbol is %q, want %q", p.ID, p.Symbol, expected)
		}
	}
}
