// core.pages — the settings-page list, served.
//
// Until this handler existed, the page list lived in the shell process
// (`app/backend/host.go`), which meant the daemon could not answer "what pages
// do you serve?" and nothing outside the shell could see the answer. That was
// invisible while the only consumer was that shell. It stopped being invisible
// the moment the UI became a second program in a different language: a Swift
// client asking the daemon for its pages got a method-not-found, and the pages
// it could not see were the fifteen core ones — every page the product is.
//
// The pages themselves moved to `core/pkg/pluginapi/pages.go`, next to the
// `UIContribution` type plugins already use. The move is the point: a page is
// data a plugin can contribute, and data the daemon owns. A plugin shipping a
// page and the core shipping one now go through the same type and the same
// gates.
//
// What this does NOT do is decide the nav. `servedPage` reports what each page
// declared; the order and the first-run tiebreak are the shell's, because the
// shell is what draws the nav and a nav whose order the daemon also computes is
// two answers to one question. `TestServedPageOrder` pins the authored order so
// the two cannot drift apart silently.

package main

import (
	"encoding/json"

	"crossos/core/pkg/ipc"
	"crossos/core/pkg/pluginapi"
)

// servedPage is one page as core.pages reports it.
//
// The JSON keys are PascalCase because they are Go's exported field names and
// the Swift client decodes them by reflection the same way the Wails binding
// generator did. Changing them would be a wire break, so the comment is here
// rather than left to be discovered: ID and Title are not `id` and `title`.
type servedPage struct {
	ID         string
	Title      string
	Group      string
	Symbol     string
	Order      int
	FirstRun   bool
	Visibility string
	Actions    []string
	// Schema is the page's declarative body — `type`, `description` and the
	// `controls` array. It travels as raw JSON rather than a decoded struct
	// because the body is open: a control's fields depend on its `kind`, and
	// there are twenty-eight kinds, none of which this file knows about.
	Schema json.RawMessage
}

// handleCorePages serves core.pages: every page the daemon offers, in the
// order the nav will draw them.
//
// **Core pages only, and that is honest rather than incomplete.** The daemon
// does not hold a plugin registry — `Core` carries `plugins map[string]bool`
// and an `order []string` (main.go:79-81), which is enough to answer "is this
// plugin on" and not enough to say what pages it contributes. `plugin.schemas`
// is the same shape: it answers with an empty list
// (pagedata.go:409-411), because no plugin declares one yet.
//
// So this reports the fifteen core pages and says nothing about plugins. When a
// plugin does declare a page, this gains a second source and the Swift
// registry gains an entry — the client side already treats a plugin page as
// ordinary data, because the schema is open and a kind it has not seen is
// named rather than dropped. Adding a source here is a few lines; guessing at
// one now would be inventing a registry the daemon does not have.
func (c *Core) handleCorePages(_ json.RawMessage) (any, *ipc.RPCError) {
	pages := pluginapi.Pages()

	// A duplicate id is a bug in a page definition, not something to paper
	// over. `CorePages` names fifteen distinct ids and a test pins it, so a
	// collision here means someone copied a page and the nav would show one of
	// them twice.
	seen := make(map[string]bool, len(pages))
	out := make([]servedPage, 0, len(pages))
	for _, p := range pages {
		if seen[p.ID] {
			continue
		}
		seen[p.ID] = true
		if p.Location != pluginapi.UILocationSettingsPage && p.Location != pluginapi.UILocationSidebar {
			continue // sections, commands and status items render in their own slots
		}
		out = append(out, servedPage{
			ID:         p.ID,
			Title:      p.Title,
			Group:      p.Group,
			Symbol:     p.Symbol,
			Order:      p.Order,
			FirstRun:   declaresFirstRun(p.Schema),
			Visibility: p.Visibility,
			Actions:    p.Actions,
			Schema:     p.Schema,
		})
	}
	return out, nil
}

// declaresFirstRun reads a page's own firstRun marker out of its schema.
//
// The schema is the page's own declaration of what it is, and a page that
// declares nothing is an ordinary page. A schema that is not JSON at all is a
// malformed contribution, not a first-run one — so this returns false rather
// than failing, because one broken plugin must not empty the nav.
func declaresFirstRun(schema json.RawMessage) bool {
	var body struct {
		FirstRun bool `json:"firstRun"`
	}
	if len(schema) == 0 {
		return false
	}
	if err := json.Unmarshal(schema, &body); err != nil {
		return false
	}
	return body.FirstRun
}
