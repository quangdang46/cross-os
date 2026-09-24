// User rules over IPC — the rule builder's data path (bead w2-userrules).
//
// Same shape as pagedata.go: one method per page control `source`, lists
// ordered before they leave, a write validated whole before it touches the
// running set, an unknown id a typed RPCError. The rows are the userrules
// package's own types, so the persisted document and the wire row cannot drift
// into two shapes for one rule.
//
// The handlers hang off userRuleService rather than off *Core so this file is
// a complete unit on its own: it compiles, and every handler in it is
// reachable from a test, without the daemon struct having to grow a field
// that is not this file's to add. The four entries the method table needs are
// named in the wiring note at the bottom.

package main

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"crossos/core/pkg/event"
	"crossos/core/pkg/intent"
	"crossos/core/pkg/ipc"
	"crossos/core/pkg/userrules"
)

// userRuleService is the rule builder's handler set over one user-rule table.
type userRuleService struct {
	store *userrules.Store
}

// newUserRuleService binds the handlers to a table. A nil store is not a
// valid service: every handler reads or writes the table, and a handler that
// answered a missing store would be a page showing an empty list for a daemon
// that has rules.
func newUserRuleService(store *userrules.Store) *userRuleService {
	return &userRuleService{store: store}
}

// userRuleRow is one rule as the editor renders it. The stored rule is
// embedded, so the wire carries the picked dimensions under the same names
// the write path takes them; chord, action and the three derived fields are
// added for display, resolved through the same helpers the behavior matrix
// uses so one chord never reads two ways in two pages.
type userRuleRow struct {
	userrules.Rule
	Chord       string `json:"chord"`
	Action      string `json:"action"`
	Priority    int    `json:"priority"`
	Specificity int    `json:"specificity"`
	Scope       string `json:"scope"`
}

// getUserRules serves config.getUserRules.
//
// The stored rules and their compiled rows come from one store snapshot, so a
// row's picked dimensions and its derived fields are always the same rule and
// never either side of a write that landed between two reads.
func (s *userRuleService) getUserRules(_ json.RawMessage) (any, *ipc.RPCError) {
	rules, compiled := s.store.Snapshot()
	out := make([]userRuleRow, 0, len(rules))
	for _, r := range rules {
		row := userRuleRow{Rule: r}
		// Every stored rule compiled on the way in, so a miss here would be a
		// bug in the store. The row still goes out: the dimensions are what
		// the person can act on, and an empty derived set is the honest
		// reading of a rule whose rank is unavailable.
		if cr, ok := compiled[r.ID]; ok {
			row.Chord = chord(cr)
			row.Action = matrixAction(cr)
			row.Priority = cr.Priority
			row.Specificity = cr.Specificity
			row.Scope = userrules.ScopeName(cr.Scope)
		}
		out = append(out, row)
	}
	return out, nil
}

// setUserRule serves config.setUserRule: one rule as the editor holds it. A
// payload carrying an id is an edit of that rule and a payload without one is
// a create — the same call, because it is the same form. The id decides
// which, and an id the table does not have is an error rather than a create:
// a stale editor that silently got a second copy of its rule instead of an
// answer would be a duplicate the user cannot see.
func (s *userRuleService) setUserRule(raw json.RawMessage) (any, *ipc.RPCError) {
	var p userrules.Rule
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, &ipc.RPCError{Code: ipc.ErrBadParams, Message: "need a rule: {key, modifiers, app_modes, capability}"}
	}
	var (
		stored userrules.Rule
		err    error
	)
	if id := strings.TrimSpace(p.ID); id != "" {
		stored, err = s.store.Update(id, p)
	} else {
		stored, err = s.store.Create(p)
	}
	if err != nil {
		// The reason is the store's own — "unknown app mode %q (known: …)",
		// the registry's "unknown capability %q" — because the editor shows
		// it beside the field that caused it.
		return nil, &ipc.RPCError{Code: ipc.ErrInvalid, Message: err.Error()}
	}
	// The reply carries the stored id, which the editor could not have
	// computed: an edit that re-derives it (a re-picked chord) renames the
	// rule, and the next delete has to name the row that exists.
	return map[string]any{"id": stored.ID}, nil
}

// deleteUserRule serves config.deleteUserRule: {id}, and answers with the
// table as it now stands — the same reply shape a fresh read would give, so
// the editor updates from one response instead of re-reading to find out
// whether its write landed.
func (s *userRuleService) deleteUserRule(raw json.RawMessage) (any, *ipc.RPCError) {
	var p struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &p); err != nil || strings.TrimSpace(p.ID) == "" {
		return nil, &ipc.RPCError{Code: ipc.ErrBadParams, Message: "need {id}"}
	}
	if err := s.store.Delete(strings.TrimSpace(p.ID)); err != nil {
		return nil, &ipc.RPCError{Code: ipc.ErrInvalid, Message: err.Error()}
	}
	return s.getUserRules(nil)
}

// userRuleVocabulary is everything the rule builder's pickers need, in one
// reply: the four lists the builder asks about, so the page opens against a
// single read rather than four that can each be a different build.
type userRuleVocabulary struct {
	AppModes      []userrules.Option     `json:"app_modes"`
	AppCategories []userrules.Option     `json:"app_categories"`
	Modifiers     []userrules.Option     `json:"modifiers"`
	Keys          []userrules.Key        `json:"keys"`
	Capabilities  []userrules.Capability `json:"capabilities"`
	Actions       []userrules.Action     `json:"actions"`
}

// getUserRuleVocabulary serves config.getUserRuleVocabulary.
//
// Each list is read from the package that owns it (see userrules/vocabulary.go)
// and comes back in the order that package emits, which is already the order a
// person reads it in. There is no app list here: the daemon enumerates
// installed apps on macOS (adapter.ListApps) but not on the other platforms it
// builds for, so a list served from this method would be empty on some
// machines and misleading on others. The app field takes a typed bundle id
// instead, which is why a rule stores app IDs rather than a category.
func (s *userRuleService) getUserRuleVocabulary(_ json.RawMessage) (any, *ipc.RPCError) {
	return userRuleVocabulary{
		AppModes:      userrules.AppModes(),
		AppCategories: userrules.AppCategories(),
		Modifiers:     userrules.Modifiers(),
		Keys:          userrules.Keys(),
		Capabilities:  userrules.Capabilities(),
		Actions:       userrules.Actions(),
	}, nil
}

// userRulesPath is the user-rule table's file: a sibling of the settings
// document, or "" for the memory-only store a test runs with.
//
// Its own file, not a settings.Store key, because this table has a stronger
// contract than the settings schema can state: a row is only valid if it
// COMPILES — the capability exists, its parameters fit its input schema, and
// the resulting candidate is legal — and that check needs the registry, which
// the settings store does not hold. The persistence mechanism is the same one
// (a config.Manager user layer written to a temp file and renamed), not a
// second mechanism.
//
// Consequence worth knowing: the ownership audit (safety.ownershipAudit)
// reports the settings file, not this one, because that list is assembled in
// pagedata.go. Until the audit learns about both, Reset Everything reclaims
// the user rules by leaving this file behind rather than deleting it.
func userRulesPath(settingsPath string) string {
	if settingsPath == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(settingsPath), "user-rules.json")
}

// Wiring. The method table is one map literal in main.go and the daemon
// struct is there too, so publishing these four handlers takes five lines
// that belong to whoever owns those two:
//
//	type Core struct { …; userRules *userrules.Store }
//	// in NewCoreWithSettings, next to settings.New:
//	userRules, err := userrules.New(userRulesPath(settingsPath))
//	c.userRules = newUserRuleService(userRules)
//
//	"config.getUserRules":          c.userRules.getUserRules,
//	"config.setUserRule":           c.userRules.setUserRule,
//	"config.deleteUserRule":        c.userRules.deleteUserRule,
//	"config.getUserRuleVocabulary": c.userRules.getUserRuleVocabulary,
//
// Until those are in the table these handlers are unreachable over the
// socket, which is the dead-code case the note above methods() warns about.

// table and grants expose the store to the decision path. The rule builder
// is not a read-only surface: a saved rule that never reaches event.Compile
// passes every unit test and does nothing on the keyboard.
func (s *userRuleService) table() []event.CompiledRule {
	if s == nil || s.store == nil {
		return nil
	}
	return s.store.Table()
}

func (s *userRuleService) grants() map[string][]intent.Permission {
	if s == nil || s.store == nil {
		return nil
	}
	return s.store.Grants()
}
