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

	"crossos/core/pkg/pluginapi"
)

// Page is one discovered settings-page contribution.
type Page struct {
	ID       string // "<pluginID>.<contribID>"
	Title    string
	Location pluginapi.UILocation
	Actions  []string
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
	pages []Page
	seen  map[string]struct{}
}

// NewHost returns an empty host.
func NewHost() *Host { return &Host{seen: map[string]struct{}{}} }

// Register discovers one registry's UI contributions. Duplicate IDs are a
// typed error (two plugins claiming one page is a conflict, never a silent
// overwrite). Custom-view contributions never reach here — the Registry
// rejects them at Register time (MVP rule, bead cross-os-4lm).
func (h *Host) Register(reg *pluginapi.Registry) error {
	for _, u := range reg.UIContributions {
		if _, dup := h.seen[u.ID]; dup {
			return fmt.Errorf("shell: duplicate UI contribution %q", u.ID)
		}
		if u.Location != pluginapi.UILocationSettingsPage && u.Location != pluginapi.UILocationSidebar {
			continue // sections/commands/status render in their own slots
		}
		h.seen[u.ID] = struct{}{}
		h.pages = append(h.pages, Page{ID: u.ID, Title: u.Title, Location: u.Location, Actions: u.Actions, Schema: u.Schema})
	}
	sort.Slice(h.pages, func(i, j int) bool { return h.pages[i].ID < h.pages[j].ID })
	return nil
}

// Pages returns the discovered pages in stable order.
func (h *Host) Pages() []Page { return append([]Page(nil), h.pages...) }
