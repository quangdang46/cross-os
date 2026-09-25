// Contribution host: renders UIContributions by discovery.
//
// The host reads pluginapi.Registry.UIContributions and serves page/section/
// sidebar models to the frontend. It never switches on plugin names —
// unknown plugin IDs flow through the same path as Core pages.
package shell

import (
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	"crossos/core/pkg/pluginapi"
)

// Page is one discovered settings-page contribution.
type Page struct {
	ID       string // "<pluginID>.<contribID>"
	Title    string
	Group    string // nav section, from the contribution
	Order    int    // position in the nav
	Location pluginapi.UILocation
	// FirstRun marks the page a fresh profile lands on. The nav needs it
	// before it can choose an opening page, so the Host reads it here
	// rather than leaving the frontend to guess from the id.
	FirstRun   bool
	Visibility string
	Actions    []string
	// Schema is the declarative page body (controls array): button, trial,
	// auditList, pluginList, enableFlow, traceList, matrix, overrides,
	// shortcutList, zoneEditor, palette, checklist, actionSettings,
	// gateBadge, note, version, license, credits. The frontend renders
	// controls generically — new kinds arrive via Registry, never via a
	// shell code change (§7.2).
	Schema json.RawMessage
}

// Host discovers UI contributions from registries.
type Host struct {
	// mu guards pages, seen and onboarded together. Wails serves every
	// binding on its own goroutine, so Pages() and OnboardingComplete() do
	// genuinely run at the same time: the goroutine carrying the user's click
	// on "Finish setup" re-sorts the slice while any other binding may be
	// copying it. The slice is shared, not copied-on-write, so an unguarded
	// sort hands a reader a header it will index into. CI runs
	// `go test -short` with no -race (ci.yml:42), which means a race shipped
	// here would stay green indefinitely.
	mu    sync.RWMutex
	pages []Page
	seen  map[string]struct{}
	// onboarded flips when the first-run flow is done. The first-run page
	// only LEADS while this is false; afterwards it takes its place in the
	// group it declares like every other page, because a Welcome page that
	// outranks Home forever is a nav bug the first time it is finished.
	onboarded bool
}

// NewHost returns an empty host.
func NewHost() *Host { return &Host{seen: map[string]struct{}{}} }

// OnboardingComplete records that the first-run flow is finished, and
// re-sorts so the landing page stops leading.
func (h *Host) OnboardingComplete(done bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onboarded = done
	h.sortLocked()
}

// Register discovers one registry's UI contributions. Duplicate IDs are a
// typed error (two plugins claiming one page is a conflict, never a silent
// overwrite). Custom-view contributions never reach here — the Registry
// rejects them at Register time (MVP rule, bead cross-os-4lm).
func (h *Host) Register(reg *pluginapi.Registry) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, u := range reg.UIContributions {
		if _, dup := h.seen[u.ID]; dup {
			return fmt.Errorf("shell: duplicate UI contribution %q", u.ID)
		}
		if u.Location != pluginapi.UILocationSettingsPage && u.Location != pluginapi.UILocationSidebar {
			continue // sections/commands/status render in their own slots
		}
		h.seen[u.ID] = struct{}{}
		h.pages = append(h.pages, Page{
			ID:         u.ID,
			Title:      u.Title,
			Group:      u.Group,
			Order:      u.Order,
			FirstRun:   isFirstRun(u.Schema),
			Visibility: u.Visibility,
			Location:   u.Location,
			Actions:    u.Actions,
			Schema:     u.Schema,
		})
	}
	h.sortLocked()
	return nil
}

// Pages returns the discovered pages in stable order.
func (h *Host) Pages() []Page {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return append([]Page(nil), h.pages...)
}

// isFirstRun reads a page's own firstRun marker out of its schema. The
// schema is the page's own declaration of what it is, and a page that
// declares nothing is an ordinary page — a schema that is not JSON at all
// is a malformed contribution, not a first-run one.
func isFirstRun(schema json.RawMessage) bool {
	var body struct {
		FirstRun bool `json:"firstRun"`
	}
	if json.Unmarshal(schema, &body) != nil {
		return false
	}
	return body.FirstRun
}

// sortLocked orders the nav: the declared Order first, then — while the
// profile is fresh — the first-run page, then the id.
//
// The name says what the caller owes: h.mu is held for writing. It is not
// taken here, because Register and OnboardingComplete both hold it already
// and a plain Mutex is not reentrant.
//
// The first-run page only leads until onboarding completes; after that it
// is just another page in its group, and the id tiebreak settles whatever
// Order left ambiguous. The id comparison last is what makes the result
// total: two pages that agree on all three keys would otherwise be sorted
// arbitrarily, and a nav that reshuffles between launches is worse than one
// that is merely alphabetical.
func (h *Host) sortLocked() {
	sort.SliceStable(h.pages, func(i, j int) bool {
		a, b := h.pages[i], h.pages[j]
		if a.Order != b.Order {
			return a.Order < b.Order
		}
		if !h.onboarded && a.FirstRun != b.FirstRun {
			return a.FirstRun
		}
		return a.ID < b.ID
	})
}
