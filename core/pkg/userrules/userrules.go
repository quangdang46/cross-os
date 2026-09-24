// Package userrules is the user-authored rule table: the rules a person
// builds in the rule editor, stored as data and compiled onto the same
// event.CompiledRule the builtin plugin matrices compile onto.
//
// The mapping is 1:1 and total. Every field a rule table row has is a field
// CompiledRule has, and every CompiledRule field is either picked by the user
// or DERIVED from what they picked (derive). Priority, specificity and scope
// are never stored: they are a function of the rule's own dimensions, so a
// table cannot hold two rules that agree on what they match and disagree on
// which one wins — a disagreement that would otherwise only surface as one
// rule silently losing.
//
// A rule is validated before it is stored, twice over and in this order:
// intent.ValidateAgainst (the capability exists, is not a banned parallel
// name, and its parameters fit its input schema) and then
// rule.ValidateCandidates over the whole prospective table (no empty or
// duplicate rule ID, no negative specificity). Either failure persists
// nothing — not the running table, not the file.
package userrules

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"crossos/core/pkg/config"
	"crossos/core/pkg/ctx"
	"crossos/core/pkg/event"
	"crossos/core/pkg/intent"
	"crossos/core/pkg/rule"
	"crossos/core/pkg/winlayout"
)

// PluginID is the plugin ID a compiled user rule carries. The router
// authorizes per plugin (intent.Authorize checks the descriptor's permission
// against that plugin's grants) and a rule a person built has no plugin
// behind it — so the table is its own owner, and Grants keys the entry.
const PluginID = "user"

// Rule is one row of the user rule table. The json tags are the
// config.setUserRule wire names, so the persisted document and the IPC row
// are one shape rather than two that can drift.
//
// The fields are the rule's IF side (which chord, in which apps, on which
// device) plus the capability its THEN side names. There is no priority,
// specificity or scope field: those are derived (derive), because a person
// picking dimensions in a UI cannot be asked to also pick a rank.
type Rule struct {
	// ID is derived from the other fields (DeriveID) and is read-only to
	// callers: it is the name the Recorder prints on a winning line, and an
	// ID that does not follow from the rule is a label nobody can check.
	ID         string          `json:"id"`
	Key        string          `json:"key"`
	Modifiers  []string        `json:"modifiers"`
	AppModes   []string        `json:"app_modes"`
	AppIDs     []string        `json:"app_ids"`
	DeviceID   string          `json:"device_id"`
	Capability string          `json:"capability"`
	Parameters json.RawMessage `json:"parameters,omitempty"`
	// Emit selects Consume (false) vs Replace (true) on the router. Declared
	// at authoring, never guessed — the same contract CompiledRule.Emit has.
	Emit bool `json:"emit"`
}

// DeriveID is the rule's identity: the chord, then the scope it claims —
// "user.ctrl+c@terminal", "user.win+left@com.apple.finder". Derived so the
// name a rule answers to cannot disagree with what it matches, and so a
// person who builds the same rule twice gets the same row back rather than a
// second copy of it.
//
// The capability is deliberately NOT part of the identity. Two rules claiming
// one chord in one scope is the conflict case (a spec §5 "two plugins use the
// same chord" situation), and it is refused here as a duplicate rather than
// stored as a pair Resolve would have to break — with a derived ID, the
// refusal happens at the second build, where the person can still see it.
func DeriveID(r Rule) string {
	chord := make([]string, 0, len(r.Modifiers)+1)
	for _, m := range modifierRows {
		for _, want := range r.Modifiers {
			if m.name == want {
				chord = append(chord, strings.ToLower(m.name))
				break
			}
		}
	}
	chord = append(chord, strings.ToLower(r.Key))
	scope := make([]string, 0, len(r.AppModes)+len(r.AppIDs)+1)
	if r.DeviceID != "" {
		scope = append(scope, r.DeviceID)
	}
	scope = append(scope, r.AppModes...)
	scope = append(scope, r.AppIDs...)
	if len(scope) == 0 {
		scope = []string{"any"}
	}
	return "user." + strings.Join(chord, "+") + "@" + strings.Join(scope, ",")
}

// derive computes the three ranking fields from the dimensions the rule
// picked — never from anything the caller supplied, because these three are
// what rule.Resolve ranks on (rule.go:38, rule.go:63-75) and a stored copy
// would be a number a user could set without changing the rule.
//
//   - scope is the narrowest tier the rule constrains. A device is
//     ScopeDevice; app modes or app IDs are ScopeApp; neither is
//     ScopeGlobal. ScopeWindow is deliberately unreachable: CompiledRule has
//     no window field, so a rule claiming window scope would compile to one
//     that matches every window — a tier that says "one window" while
//     narrowing nothing.
//   - priority is the tier rule.go:23-29 names: a rule scoped to the
//     contexts where the OS binding is the wrong one (terminal, remote, VM,
//     excluded) takes PriorityTerminal, a rule that otherwise constrains the
//     app takes PriorityApp, and an unconstrained rule takes
//     PriorityGlobal. A mode list that includes native covers ordinary apps
//     too, so it is not a terminal rule however many modes it names.
//   - specificity is match depth: the chord every rule has, plus one for each
//     dimension the rule adds (app mode, app ID, device). It is the
//     monotonic form of the "app+window+device match depth" rule.go:38
//     describes — a rule cannot claim depth it does not have.
func derive(r Rule) (priority, specificity int, scope rule.Scope) {
	specificity = 1
	if len(r.AppModes) > 0 {
		specificity++
	}
	if len(r.AppIDs) > 0 {
		specificity++
	}
	if r.DeviceID != "" {
		specificity++
	}
	switch {
	case r.DeviceID != "":
		scope = rule.ScopeDevice
	case len(r.AppModes) > 0 || len(r.AppIDs) > 0:
		scope = rule.ScopeApp
	default:
		scope = rule.ScopeGlobal
	}
	switch {
	case specialOnly(r.AppModes):
		priority = rule.PriorityTerminal
	case scope != rule.ScopeGlobal:
		priority = rule.PriorityApp
	default:
		priority = rule.PriorityGlobal
	}
	return priority, specificity, scope
}

// specialOnly reports whether the mode list is a non-empty set of the
// contexts that are not ordinary apps. rule.go:23 calls out terminal/VM as
// the 100 tier; remote desktops sit with them because ctx's seed classifier
// groups all three as the contexts CrossOS leaves alone.
func specialOnly(modes []string) bool {
	if len(modes) == 0 {
		return false
	}
	native := string(ctx.AppModeNative)
	for _, m := range modes {
		if m == native {
			return false
		}
	}
	return true
}

// Compile maps one row onto the router's rule shape. Every CompiledRule field
// is filled: the picked key, modifiers, modes, app IDs and device, the
// derived rank, the named capability as a keyboard-source intent, and Emit.
// The two validations the table contract names run here, so nothing reaches
// the store unvalidated: the intent against the registry (ValidateAgainst —
// capability exists, no banned parallel name, parameters fit the input
// schema) and, for the whole prospective table, rule.ValidateCandidates.
func (r Rule) Compile(reg *intent.Registry) (event.CompiledRule, error) {
	if r.ID == "" {
		return event.CompiledRule{}, fmt.Errorf("userrules: rule has no id")
	}
	key, err := KeyCode(r.Key)
	if err != nil {
		return event.CompiledRule{}, fmt.Errorf("userrules: rule %q: %w", r.ID, err)
	}
	mods, err := ModifierMask(r.Modifiers)
	if err != nil {
		return event.CompiledRule{}, fmt.Errorf("userrules: rule %q: %w", r.ID, err)
	}
	modes, err := checkAppModes(r.AppModes)
	if err != nil {
		return event.CompiledRule{}, fmt.Errorf("userrules: rule %q: %w", r.ID, err)
	}
	in, err := r.intent(reg)
	if err != nil {
		return event.CompiledRule{}, fmt.Errorf("userrules: rule %q: %w", r.ID, err)
	}
	priority, specificity, scope := derive(r)
	return event.CompiledRule{
		KeyCode:   key,
		Modifiers: mods,
		AppModes:  modes,
		AppIDs:    append([]string(nil), r.AppIDs...),
		DeviceIDs: deviceIDs(r.DeviceID),
		RuleID:    r.ID,
		PluginID:  PluginID,
		Priority:  priority,
		// Specificity and Scope are the ranking, not the shape: they live
		// here because CompiledRule carries them to the router, and they are
		// computed above precisely so no caller can set them.
		Specificity: specificity,
		Scope:       scope,
		Intent:      in,
		Emit:        r.Emit,
	}, nil
}

// checkAppModes resolves picked modes against the vocabulary the picker
// offers and reports the ones that are not in it. A mode the taxonomy does
// not have is refused here with its name, because a rule that stored it
// would compile to a matcher that never matches — a key the user believes
// they have remapped and have not.
func checkAppModes(modes []string) ([]event.AppMode, error) {
	if len(modes) == 0 {
		return nil, nil
	}
	known := AppModes()
	out := make([]event.AppMode, 0, len(modes))
	for _, want := range modes {
		found := false
		for _, o := range known {
			if o.Value == want {
				out = append(out, event.AppMode(o.Value))
				found = true
				break
			}
		}
		if !found {
			values := make([]string, 0, len(known))
			for _, o := range known {
				values = append(values, o.Value)
			}
			return nil, fmt.Errorf("unknown app mode %q (known: %s)",
				want, strings.Join(values, ", "))
		}
	}
	return out, nil
}

func deviceIDs(id string) []string {
	if id == "" {
		return nil
	}
	return []string{id}
}

// intent builds the THEN side: the named capability as a keyboard-source
// intent, with the picked parameters. ValidateAgainst is the capability
// contract — the ID must be in the registry and not one of the banned
// parallel names, and the parameters must satisfy the capability's input
// schema (required keys present, declared keys only, primitive types right).
// A capability that fails it is reported with the registry's own reason,
// which is the message the editor can show next to the failing field.
func (r Rule) intent(reg *intent.Registry) (intent.Intent, error) {
	in := intent.Intent{
		ID:         r.Capability,
		Version:    1,
		Source:     intent.SourceKeyboard,
		Parameters: r.Parameters,
	}
	if err := in.ValidateAgainst(reg); err != nil {
		return intent.Intent{}, err
	}
	// A capability whose input schema is empty takes no parameters, and
	// ValidateAgainst returns before it looks at Parameters in that case — so
	// arguments to an action that accepts none would pass validation and be
	// persisted alongside it. Refused here so that every parameter a rule
	// carries is one the capability's own schema vouched for.
	desc, _ := reg.Get(in.ID)
	if len(desc.InputSchema.Required) == 0 && len(desc.InputSchema.Properties) == 0 &&
		len(r.Parameters) > 0 {
		return intent.Intent{}, fmt.Errorf("capability %s takes no parameters", in.ID)
	}
	// window.move's zone is the one parameter value with a vocabulary of its
	// own, and winlayout owns it. ValidateAgainst only checks that "zone" is
	// a string, so a typo would otherwise compile, store, and fire into the
	// adapter with a zone nothing resolves. Checking it here is the same
	// check the matrix action path makes.
	if in.ID == "window.move" {
		var p struct {
			Zone string `json:"zone"`
		}
		if err := json.Unmarshal(in.Parameters, &p); err != nil {
			return intent.Intent{}, fmt.Errorf("window.move parameters: %w", err)
		}
		if _, ok := winlayout.ActionForZone(p.Zone); !ok {
			return intent.Intent{}, fmt.Errorf("unknown window zone %q (see the window action list)", p.Zone)
		}
	}
	return in, nil
}

// Candidate projects a compiled rule onto the resolution input, field for
// field the way Router.Decide builds one for a rule that has matched: the
// three filter facts are all true because the caller has already narrowed
// the table to the rules that match. Resolve ranks what it is given — it
// does not match — so the narrowing is the caller's job.
func Candidate(cr event.CompiledRule) rule.Candidate {
	return rule.Candidate{
		RuleID:      cr.RuleID,
		PluginID:    cr.PluginID,
		Priority:    cr.Priority,
		Specificity: cr.Specificity,
		Scope:       cr.Scope,
		Intent:      cr.Intent,
		Enabled:     true,
		AppModeOK:   true,
		RequiresOK:  true,
	}
}

// Store owns the user rule table. One mutex guards it: the IPC handlers run
// on independent connection goroutines, and the Settings page polls the
// table while the user edits it.
type Store struct {
	mu    sync.RWMutex
	path  string
	cfg   *config.Manager
	reg   *intent.Registry
	rules []Rule
}

// document is the persisted user-rule table. One key, declared by Schema and
// written only from a validated state, so the file cannot carry a rule the
// table would refuse to load.
type document struct {
	Rules []Rule `json:"rules"`
}

// Schema constrains the persisted user-rule document. It is the same Config
// Manager user layer the rest of the settings live in — validated keys only,
// an undeclared key rejected rather than silently kept — and it is declared
// here rather than added to settings.ShortcutSchema because this table has a
// stronger contract than a type check: every row must also compile, which
// Schema cannot say. The document is its own file for the same reason.
func Schema() config.Schema {
	return config.Schema{"rules": {Type: "array", Required: false}}
}

// New returns a table loaded from path, or empty when there is no file yet
// (a first run is not an error). A document that does not parse is treated
// the same way: the table comes up empty and the next write replaces it,
// rather than refusing to start over a file the user can only fix by hand.
// Rows that no longer compile — a capability dropped from the registry
// between releases — are dropped individually and the rest load, because the
// editor would otherwise render a rule that nothing can act on.
func New(path string) (*Store, error) {
	defaults, err := json.Marshal(document{Rules: []Rule{}})
	if err != nil {
		return nil, err
	}
	cfg, err := config.New(defaults, Schema())
	if err != nil {
		return nil, err
	}
	s := &Store{path: path, cfg: cfg, reg: intent.DefaultRegistry()}
	if path == "" {
		return s, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return s, nil
	}
	var doc document
	if err := json.Unmarshal(raw, &doc); err != nil {
		return s, nil
	}
	loaded := make([]Rule, 0, len(doc.Rules))
	for _, r := range doc.Rules {
		r = normalize(r)
		// The id is re-derived rather than trusted, so "id follows from the
		// dimensions" holds for every row in the table and not only for the
		// ones this version wrote. A hand-edited file that renamed a rule —
		// or, worse, renamed one to a BUILTIN rule id — cannot inject a
		// second claim on an identity the router resolves by name.
		r.ID = DeriveID(r)
		if _, err := r.Compile(s.reg); err != nil {
			continue
		}
		loaded = append(loaded, r)
	}
	if _, err := s.validated(loaded); err != nil {
		// A hand-edited file with two rows claiming one ID cannot be loaded
		// as a table: rule.Resolve would see a duplicate. Keep the first
		// claim of each ID and drop the rest, so the user's rules still come
		// up and the ambiguity is resolved in a documented direction.
		loaded = firstByID(loaded)
	}
	s.rules = sortRules(loaded)
	return s, nil
}

// firstByID keeps the first row for each ID. Only a hand-edited document
// reaches this — the writer refuses a duplicate before it can be persisted —
// so which of two conflicting claims survives is the order they were written
// in, and the file's order is the person's. A duplicate whose rows are
// identical apart from the capability loses its second action, which is the
// honest reading: the table keeps one rule per identity, and the next edit
// through the handler is how the person changes what it does.
func firstByID(rules []Rule) []Rule {
	seen := make(map[string]bool, len(rules))
	out := make([]Rule, 0, len(rules))
	for _, r := range rules {
		if seen[r.ID] {
			continue
		}
		seen[r.ID] = true
		out = append(out, r)
	}
	return out
}

// Rules returns every stored rule, sorted by rule ID, as a copy. Sorted
// because the editor polls this list and Go map order would reshuffle the
// rows between refreshes.
func (s *Store) Rules() []Rule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]Rule(nil), s.rules...)
}

// Table returns the compiled rules, sorted by rule ID, as a fresh slice
// every call. Fresh is the point: event.Compile keeps the slice it is handed
// read-only for the life of the router, so a table that reused its backing
// array would change under a running router with no recompile.
func (s *Store) Table() []event.CompiledRule {
	rules, compiled := s.Snapshot()
	out := make([]event.CompiledRule, 0, len(compiled))
	for _, r := range rules {
		cr, ok := compiled[r.ID]
		if !ok {
			continue
		}
		out = append(out, cr)
	}
	return out
}

// Snapshot returns the stored rules and their compiled rows from ONE lock
// acquisition, paired by rule ID. The editor polls both halves of a rule row
// — the picked dimensions from the first, the chord, action and derived rank
// from the second — and two separate reads could come from either side of a
// write, producing a row whose chord says one thing and whose scope says
// another. A rule with no compiled row cannot happen (they are validated
// together on the way in); the map simply has no entry for an ID the caller
// invented.
func (s *Store) Snapshot() ([]Rule, map[string]event.CompiledRule) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	compiled := make(map[string]event.CompiledRule, len(s.rules))
	for _, r := range s.rules {
		// Every stored rule compiled on the way in, so a failure here would
		// be a bug in the store rather than bad user data. Leaving the rule
		// out of the map keeps a panic off the read path; the caller renders
		// the row it can account for.
		cr, err := r.Compile(s.reg)
		if err != nil {
			continue
		}
		compiled[r.ID] = cr
	}
	return append([]Rule(nil), s.rules...), compiled
}

// Candidates returns the table as resolution candidates — the whole table,
// not the matching subset. rule.Resolve ranks what it is given; narrowing to
// the rules that match an event is the caller's step, exactly as it is in
// Router.Decide.
func (s *Store) Candidates() []rule.Candidate {
	table := s.Table()
	out := make([]rule.Candidate, 0, len(table))
	for _, cr := range table {
		out = append(out, Candidate(cr))
	}
	return out
}

// Grants is the permission set a compiled user rule authorizes against. It
// is the union of the permissions the stored capabilities require, read off
// their registry descriptors — derived, not declared, so dropping a rule
// drops the access it needed instead of leaving a standing grant behind.
func (s *Store) Grants() map[string][]intent.Permission {
	out := map[string][]intent.Permission{}
	for _, cr := range s.Table() {
		desc, ok := s.reg.Get(cr.Intent.ID)
		if !ok {
			continue
		}
		if !containsPermission(out[PluginID], desc.Permission) {
			out[PluginID] = append(out[PluginID], desc.Permission)
		}
	}
	return out
}

func containsPermission(list []intent.Permission, p intent.Permission) bool {
	for _, have := range list {
		if have == p {
			return true
		}
	}
	return false
}

// Create stores a new rule and returns the stored row, which carries the
// derived ID. A rejected rule persists nothing: the whole prospective table
// is compiled and validated first, so a bad row can neither half-apply nor
// leave a file the editor has already moved past.
func (s *Store) Create(r Rule) (Rule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r = normalize(r)
	r.ID = DeriveID(r)
	next := make([]Rule, 0, len(s.rules)+1)
	next = append(next, s.rules...)
	next = append(next, r)
	if _, err := s.validated(next); err != nil {
		return Rule{}, err
	}
	if err := s.commitLocked(next); err != nil {
		return Rule{}, err
	}
	return r, nil
}

// Update replaces the rule carrying id with the picked dimensions. The ID
// is re-derived, so changing the chord or the scope renames the rule the way
// a newly built one would be named. Updating an ID that is not in the table
// is an error, never a silent create.
func (s *Store) Update(id string, r Rule) (Rule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	at := -1
	for i, have := range s.rules {
		if have.ID == id {
			at = i
			break
		}
	}
	if at < 0 {
		return Rule{}, fmt.Errorf("userrules: no rule %q to update", id)
	}
	next := make([]Rule, 0, len(s.rules))
	next = append(next, s.rules[:at]...)
	r = normalize(r)
	r.ID = DeriveID(r)
	next = append(next, r)
	next = append(next, s.rules[at+1:]...)
	if _, err := s.validated(next); err != nil {
		return Rule{}, err
	}
	if err := s.commitLocked(next); err != nil {
		return Rule{}, err
	}
	return r, nil
}

// Delete removes the rule carrying id. Deleting an ID that is not in the
// table is an error: the editor's delete button must not report success for
// a row that was already gone.
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	at := -1
	for i, have := range s.rules {
		if have.ID == id {
			at = i
			break
		}
	}
	if at < 0 {
		return fmt.Errorf("userrules: no rule %q to delete", id)
	}
	next := make([]Rule, 0, len(s.rules)-1)
	next = append(next, s.rules[:at]...)
	next = append(next, s.rules[at+1:]...)
	return s.commitLocked(next)
}

// validated compiles every rule in the prospective table and runs
// rule.ValidateCandidates over the result. The whole table, not just the
// edited row: a duplicate rule ID and a rule that no longer compiles are both
// properties of the table, and checking only the new row would let a create
// collide with a rule nobody re-validated. Returns the sorted table.
func (s *Store) validated(rules []Rule) ([]Rule, error) {
	cands := make([]rule.Candidate, 0, len(rules))
	for _, r := range rules {
		cr, err := r.Compile(s.reg)
		if err != nil {
			return nil, err
		}
		cands = append(cands, Candidate(cr))
	}
	if err := rule.ValidateCandidates(cands); err != nil {
		return nil, err
	}
	return sortRules(rules), nil
}

// normalize puts a picked rule in the one shape the table stores: trimmed
// values, lists sorted and de-duplicated, empty lists empty rather than nil so
// the persisted document reads the same however the rule was picked. Sorting
// is what makes the derived ID stable — the same five apps picked in a
// different order are the same rule and must not compile to two.
func normalize(r Rule) Rule {
	r.ID = strings.TrimSpace(r.ID)
	r.Key = strings.TrimSpace(r.Key)
	r.DeviceID = strings.TrimSpace(r.DeviceID)
	r.Capability = strings.TrimSpace(r.Capability)
	r.Modifiers = normalizeList(r.Modifiers)
	r.AppModes = normalizeList(r.AppModes)
	r.AppIDs = normalizeList(r.AppIDs)
	if len(r.Parameters) == 0 {
		r.Parameters = nil
	}
	return r
}

func normalizeList(in []string) []string {
	if len(in) == 0 {
		return []string{}
	}
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func sortRules(rules []Rule) []Rule {
	out := append([]Rule(nil), rules...)
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// commitLocked writes the whole table: schema check, then a temp file and a
// rename so a reader never sees a half-written document. The running table is
// replaced only after the document is on disk, so the two can never disagree
// about what the user set. Caller holds the write lock.
func (s *Store) commitLocked(rules []Rule) error {
	doc, err := json.Marshal(document{Rules: orEmpty(rules)})
	if err != nil {
		return fmt.Errorf("userrules: persist rejected: %w", err)
	}
	if err := s.cfg.SetUser(doc); err != nil {
		return fmt.Errorf("userrules: persist rejected: %w", err)
	}
	if s.path != "" {
		if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
			return err
		}
		tmp, err := os.CreateTemp(filepath.Dir(s.path), "user-rules-*.json")
		if err != nil {
			return err
		}
		name := tmp.Name()
		if _, err := tmp.Write(doc); err != nil {
			tmp.Close()
			os.Remove(name)
			return err
		}
		if err := tmp.Close(); err != nil {
			os.Remove(name)
			return err
		}
		if err := os.Rename(name, s.path); err != nil {
			os.Remove(name)
			return err
		}
	}
	s.rules = sortRules(rules)
	return nil
}

// orEmpty normalizes a nil slice to an empty one. A nil slice marshals to
// null, which the schema's "array" check rejects — so clearing the table
// would fail validation, the write would abort, and the previous document
// would stay on disk while the running table had already been emptied.
func orEmpty(rules []Rule) []Rule {
	if rules == nil {
		return []Rule{}
	}
	return rules
}
