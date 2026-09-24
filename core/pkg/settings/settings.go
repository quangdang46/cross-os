// UI settings store: keyboard matrix toggles, window shortcuts, snap
// zones, app overrides (plan §7.2 Keyboard/Windows pages; beads 10s/nir.5).
//
// Design: the daemon owns ONE store behind the config.write* IPC methods.
// Rule enablement is a data toggle (ruleID → enabled), NOT a rule rewrite:
// decideLocked consults the toggle set alongside the plugin enable map, and a
// per-app override shadows both for the app it names, so "disable a shortcut
// takes effect without restart" with zero router-shape change. Shortcuts/
// zones/overrides persist via the Config Manager user layer (validated, then
// written to config.json atomically); invalid edits fail closed and never
// touch the running set.
//
// First-run state lives here too: the onboarding-complete flag, the plugin
// enable set and the active profile. That is why ShortcutSchema declares every
// key the store writes — the Config Manager rejects an undeclared key, so a
// key missing from the schema is not "ignored", it is a user's settings
// dropped on the next load. ApplyBatch is the multi-edit form (one profile
// card is a dozen edits at once): it validates the whole plan, commits it as
// one write, and a failure at any step leaves the file and the running set
// exactly as they were.
package settings

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"crossos/core/pkg/config"
	"crossos/core/pkg/winlayout"
)

// Store owns the editable UI settings behind the config.write* methods.
type Store struct {
	mu sync.RWMutex
	// disabledRules names rules the user turned off (matrix toggles).
	// Absent = enabled. Keyed by RuleID (stable across restarts).
	disabledRules map[string]bool
	// overrides are per-app verdicts on a matrix rule (Keyboard page
	// "core:appOverrides"). Absent = the global toggle decides. Both
	// verdicts are stored, keyed app\x00ruleID: keeping only the disabled
	// ones would lose an explicit "this app still wants it" the moment the
	// same rule is switched off globally, and the user's per-app intent
	// would silently follow the global switch.
	overrides map[string]bool
	shortcuts []winlayout.Shortcut
	zones     []winlayout.Zone
	panicStop bool
	// onboardingComplete latches the first-run wizard. It has to survive
	// restarts: a flag that clears on the next launch walks the user back
	// through setup with no way to say they are done.
	onboardingComplete bool
	// pluginsEnabled is the user's enable set, by plugin ID. Absent = off,
	// the same default the daemon's own plugin map has, so a daemon started
	// from this document enables exactly what the user left on.
	pluginsEnabled map[string]bool
	// activeProfile is the selected product profile — the "Windows 11
	// Experience" card the spec sells. Free text, like the per-app bundle IDs
	// below: the catalog of profiles is the daemon's, not the store's.
	activeProfile string
	cfg           *config.Manager
	cfgPath       string
}

// Override is one stored per-app verdict on a matrix rule. The json tags are
// the config.setOverride wire names, so the persisted document and the IPC
// row are the same shape.
type Override struct {
	App     string `json:"app"`
	RuleID  string `json:"rule_id"`
	Enabled bool   `json:"enabled"`
}

// Toggle is one id + verdict pair inside a Plan.
type Toggle struct {
	ID      string `json:"id"`
	Enabled bool   `json:"enabled"`
}

// Plan is one atomic settings change: the rule verdicts, plugin verdicts,
// profile selection and onboarding flag a single UI gesture produces. The
// profile card is the motivating case — "Windows 11 Experience" turns on a
// dozen rules, a couple of plugins and the profile itself, and the user must
// not be left with half of it because the last step named a rule that does
// not exist. A nil field means "leave this alone", never "clear it".
type Plan struct {
	Rules              []Toggle `json:"rules,omitempty"`
	Plugins            []Toggle `json:"plugins,omitempty"`
	OnboardingComplete *bool    `json:"onboardingComplete,omitempty"`
	ActiveProfile      *string  `json:"activeProfile,omitempty"`
}

// Catalog carries the id lookups ApplyBatch validates against — the same
// fail-closed predicates the per-item setters take (the daemon's rule table,
// the plugin registry). A nil predicate rejects every id in its space rather
// than waving it through: "no catalog" must not read as "anything goes".
type Catalog struct {
	Rules   func(string) bool
	Plugins func(string) bool
}

// document is the persisted user layer: the keys ShortcutSchema declares, in
// the shape the Config Manager validator and the IPC rows both expect. One
// struct serves the defaults, the rehydrate decode and every write, so a key
// cannot land in one of the three and be missing from the others.
type document struct {
	DisabledRules      []string             `json:"disabledRules"`
	Shortcuts          []winlayout.Shortcut `json:"shortcuts"`
	Zones              []winlayout.Zone     `json:"zones"`
	Overrides          []Override           `json:"overrides"`
	PanicStop          bool                 `json:"panicStop"`
	OnboardingComplete bool                 `json:"onboardingComplete"`
	EnabledPlugins     []string             `json:"enabledPlugins"`
	ActiveProfile      string               `json:"activeProfile"`
}

// state is the whole persisted value at one instant. Writes render the
// document from it, so the file and the running set can never come from two
// different reads. A batch owns its state outright — the maps in it are
// copies — which is what lets it throw the whole thing away on failure.
type state struct {
	disabledRules map[string]bool
	overrides     map[string]bool
	shortcuts     []winlayout.Shortcut
	zones         []winlayout.Zone
	panicStop     bool
	onboarding    bool
	plugins       map[string]bool
	profile       string
}

// ShortcutSchema constrains the persisted shortcuts document: the shortcut
// table plus the disabled-rule list. Stored in the Config Manager user
// layer (validated keys only — unknown keys rejected, never silently
// kept).
func ShortcutSchema() config.Schema {
	return config.Schema{
		// shortcuts is a LIST of {Action, Modifiers, Key} rows, not an
		// object — the schema said "object" only because nothing persisted
		// the field before, so the mismatch never surfaced.
		"shortcuts":     {Type: "array", Required: false},
		"disabledRules": {Type: "array", Required: false},
		// zones and overrides are the two lists the Windows + Keyboard page
		// editors round-trip. Declared so the user layer can carry them:
		// an undeclared key is rejected at load, which would drop a user's
		// placements on every restart.
		"zones":     {Type: "array", Required: false},
		"overrides": {Type: "array", Required: false},
		// panicStop latches the kill switch across restarts. A control that
		// un-latches on the next crash-loop iteration is worse than none:
		// the user believes their keyboard is safe while the tap is live.
		"panicStop": {Type: "bool", Required: false},
		// The first-run trio. Declared because the validator is fail-closed
		// on undeclared keys, and a rejected key is a rejected WHOLE layer:
		// onboarding would greet a finished user again, a disabled plugin
		// would silently come back, and the chosen profile would be lost —
		// all of them on the next launch, with no error anywhere.
		"onboardingComplete": {Type: "bool", Required: false},
		"enabledPlugins":     {Type: "array", Required: false},
		"activeProfile":      {Type: "string", Required: false},
	}
}

// New returns a Store seeded from the winlayout defaults. cfgPath may be
// "" (no persistence — tests); otherwise the user layer loads from disk
// when present (absent file = defaults, never an error).
func New(cfgPath string) (*Store, error) {
	defaults, _ := json.Marshal(document{
		DisabledRules:  []string{},
		Shortcuts:      []winlayout.Shortcut{},
		Zones:          []winlayout.Zone{},
		Overrides:      []Override{},
		EnabledPlugins: []string{},
	})
	cfg, err := config.New(defaults, ShortcutSchema())
	if err != nil {
		return nil, err
	}
	s := &Store{
		disabledRules:  map[string]bool{},
		overrides:      map[string]bool{},
		pluginsEnabled: map[string]bool{},
		shortcuts:      winlayout.DefaultShortcuts(),
		zones:          winlayout.DefaultZones(),
		cfg:            cfg,
		cfgPath:        cfgPath,
	}
	if cfgPath != "" {
		if raw, err := os.ReadFile(cfgPath); err == nil {
			// Best-effort restore: validated user layer, then rehydrate
			// the disabled set from the persisted document.
			if jerr := cfg.SetUser(raw); jerr == nil {
				var doc document
				if uerr := json.Unmarshal(raw, &doc); uerr == nil {
					for _, id := range doc.DisabledRules {
						s.disabledRules[id] = true
					}
					// A persisted list that no longer validates (hand-edited
					// file, older schema) is dropped rather than served: the
					// editor would render rows nothing can act on. The
					// shortcuts branch is guarded on the key being PRESENT as
					// well: absent decodes to nil, and assigning that would
					// turn a document written before the editor existed into
					// "no shortcuts" instead of the seed table.
					if doc.Shortcuts != nil {
						if verr := winlayout.ValidateShortcuts(doc.Shortcuts); verr == nil {
							s.shortcuts = doc.Shortcuts
						}
					}
					if verr := winlayout.ValidateZones(doc.Zones); verr == nil {
						s.zones = doc.Zones
					}
					for _, o := range doc.Overrides {
						if o.App != "" && o.RuleID != "" {
							s.overrides[overrideKey(o.App, o.RuleID)] = o.Enabled
						}
					}
					s.panicStop = doc.PanicStop
					// The first-run trio, same best-effort deal: absent is
					// the false/empty/none answer the fresh store already
					// holds, so a document written before onboarding existed
					// still comes up as "not finished yet".
					s.onboardingComplete = doc.OnboardingComplete
					for _, id := range doc.EnabledPlugins {
						if id != "" {
							s.pluginsEnabled[id] = true
						}
					}
					s.activeProfile = doc.ActiveProfile
				}
			}
		}
	}
	if err := winlayout.ValidateShortcuts(s.shortcuts); err != nil {
		return nil, err
	}
	if err := winlayout.ValidateZones(s.zones); err != nil {
		return nil, err
	}
	return s, nil
}

// IsRuleEnabled reports whether ruleID fires (plugin-gating happens above
// this layer in decideLocked — this is the USER toggle only).
// PanicStopped reports whether the user latched PANIC STOP. It survives
// restarts: a daemon that re-arms the tap after a crash would otherwise
// undo the emergency control without telling anyone.
func (s *Store) PanicStopped() bool { return s.panicStop }

// ConfigPath is the user-layer file this store persists to, or "" for the
// memory-only store. The ownership audit reads it to decide whether the file
// is a resource CrossOS has actually created yet.
func (s *Store) ConfigPath() string { return s.cfgPath }

// SetPanicStopped latches (or clears) the kill switch and persists it.
func (s *Store) SetPanicStopped(stopped bool) error {
	s.panicStop = stopped
	return s.persistLocked()
}

// OnboardingComplete reports whether the first-run wizard has been finished.
// False on a fresh install: the shell shows the welcome flow until the user
// says they are done, and only this persisted flag can remember that they did.
func (s *Store) OnboardingComplete() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.onboardingComplete
}

// SetOnboardingComplete latches (or clears) the wizard's done state.
func (s *Store) SetOnboardingComplete(done bool) error {
	s.mu.Lock()
	s.onboardingComplete = done
	s.mu.Unlock()
	return s.persistLocked()
}

// PluginsEnabled returns the plugin IDs the user has turned on, sorted. Absent
// means off — the default the daemon's own plugin map already has — so a
// daemon restarted from this document comes up enabling exactly what the user
// left on, and a plugin the user never touched stays off.
func (s *Store) PluginsEnabled() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return enabledIDs(s.pluginsEnabled)
}

// SetPluginEnabled records one plugin's verdict. Unknown IDs fail closed on
// the same `known` predicate the rule toggle takes, so a typo cannot persist a
// row no page can act on.
func (s *Store) SetPluginEnabled(pluginID string, enabled bool, known func(string) bool) error {
	if !known(pluginID) {
		return fmt.Errorf("settings: unknown plugin %q", pluginID)
	}
	s.mu.Lock()
	if enabled {
		s.pluginsEnabled[pluginID] = true
	} else {
		delete(s.pluginsEnabled, pluginID)
	}
	s.mu.Unlock()
	return s.persistLocked()
}

// ActiveProfile returns the selected profile ID, or "" when none is chosen.
func (s *Store) ActiveProfile() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.activeProfile
}

// SetActiveProfile records the selected profile. The ID is free text — the
// catalog of profiles is the daemon's, exactly like the per-app bundle IDs
// SetOverride takes — so only emptiness is rejected.
func (s *Store) SetActiveProfile(name string) error {
	if err := checkProfile(name); err != nil {
		return err
	}
	s.mu.Lock()
	s.activeProfile = name
	s.mu.Unlock()
	return s.persistLocked()
}

func checkProfile(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("settings: empty profile id")
	}
	return nil
}

// enabledIDs lists the plugins a verdict map has on, sorted. Sorted because
// the pages poll this list, and Go map order would reshuffle the rows between
// refreshes — a bug the user files as "the Extensions page flickers".
func enabledIDs(verdicts map[string]bool) []string {
	out := make([]string, 0, len(verdicts))
	for id, on := range verdicts {
		if on {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

func (s *Store) IsRuleEnabled(ruleID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return !s.disabledRules[ruleID]
}

// SetRuleEnabled toggles one matrix row by RuleID. Unknown RuleIDs fail
// closed (typo'd IDs never silently no-op).
func (s *Store) SetRuleEnabled(ruleID string, enabled bool, known func(string) bool) error {
	if !known(ruleID) {
		return fmt.Errorf("settings: unknown rule %q", ruleID)
	}
	s.mu.Lock()
	if enabled {
		delete(s.disabledRules, ruleID)
	} else {
		s.disabledRules[ruleID] = true
	}
	s.mu.Unlock()
	return s.persistLocked()
}

// Shortcuts returns the current shortcut table (copy).
func (s *Store) Shortcuts() []winlayout.Shortcut {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]winlayout.Shortcut, len(s.shortcuts))
	copy(out, s.shortcuts)
	return out
}

// SetShortcuts replaces the shortcut table after winlayout validation
// (duplicate chords + non-window capabilities rejected before anything
// is stored or applied).
func (s *Store) SetShortcuts(set []winlayout.Shortcut) error {
	if err := winlayout.ValidateShortcuts(set); err != nil {
		return err
	}
	s.mu.Lock()
	s.shortcuts = append([]winlayout.Shortcut(nil), set...)
	s.mu.Unlock()
	return s.persistLocked()
}

// overrideKey packs the (app, rule) pair into one map key. NUL cannot appear
// in a bundle ID or a rule ID, so the packing is unambiguous.
func overrideKey(app, ruleID string) string { return app + "\x00" + ruleID }

// orEmpty normalizes a nil slice to an empty one. A nil slice marshals to
// null, and the Config Manager's schema check rejects null for an array — so
// clearing a list would fail validation, the write would abort, and the
// previous document would stay on disk while the running set had already
// moved. The two would then disagree about what the user set.
func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// Overrides returns every explicit per-app verdict, sorted by app then rule ID.
// Sorted because the editor polls this list: Go map order would reshuffle the
// rows between refreshes and the user would file that as a bug.
func (s *Store) Overrides() []Override {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return overrideRows(s.overrides)
}

// overrideRows renders the packed verdict map as wire rows in a fixed order.
// Shared with the write path so a document and the editor list can never
// disagree about the row order.
func overrideRows(verdicts map[string]bool) []Override {
	out := make([]Override, 0, len(verdicts))
	for k, on := range verdicts {
		app, ruleID, _ := strings.Cut(k, "\x00")
		out = append(out, Override{App: app, RuleID: ruleID, Enabled: on})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].App != out[j].App {
			return out[i].App < out[j].App
		}
		return out[i].RuleID < out[j].RuleID
	})
	return out
}

// SetOverride records one per-app verdict. An unknown rule ID fails closed on
// the same `known` predicate the matrix toggle uses, so a typo can never leave
// a stored override nothing will ever read. The app bundle ID is free text by
// nature — the machine's installed apps are the vocabulary and the daemon
// cannot enumerate them — so only emptiness is rejected.
func (s *Store) SetOverride(app, ruleID string, enabled bool, known func(string) bool) error {
	if !known(ruleID) {
		return fmt.Errorf("settings: unknown rule %q", ruleID)
	}
	s.mu.Lock()
	s.overrides[overrideKey(app, ruleID)] = enabled
	s.mu.Unlock()
	return s.persistLocked()
}

// Override reports the stored verdict for one (app, rule) pair, and whether
// there is one at all. ok=false means "the app has expressed no opinion" and
// the caller falls back to the global toggle — the pair a rule engine needs,
// and the only reason the Keyboard page's per-app writes are more than
// config.json furniture.
func (s *Store) Override(app, ruleID string) (enabled, ok bool) {
	if app == "" {
		return false, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	enabled, ok = s.overrides[overrideKey(app, ruleID)]
	return enabled, ok
}

// Zones returns the user-placed snap rectangles (copy). The slice order is the
// editor's row order and is preserved: an index the user can see must not move
// because the list was rebuilt. Never nil — an empty list has to reach the
// wire as [], because null is a shape the editor cannot tell from a fault.
func (s *Store) Zones() []winlayout.Zone {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]winlayout.Zone, len(s.zones))
	copy(out, s.zones)
	return out
}

// SetZones replaces the zone list after winlayout validation. Validation runs
// over the WHOLE list first, so one bad row cannot leave the editor showing
// half the user's rectangles.
func (s *Store) SetZones(zones []winlayout.Zone) error {
	if err := winlayout.ValidateZones(zones); err != nil {
		return err
	}
	s.mu.Lock()
	s.zones = append([]winlayout.Zone(nil), zones...)
	s.mu.Unlock()
	return s.persistLocked()
}

// validate checks every element of the plan against the caller's catalogs
// before anything is mutated — the batch form of the same fail-closed contract
// the per-item setters keep. A nil predicate rejects: "no catalog" is a
// missing answer, not permission.
func (p Plan) validate(cat Catalog) error {
	for _, t := range p.Rules {
		if cat.Rules == nil || !cat.Rules(t.ID) {
			return fmt.Errorf("settings: unknown rule %q", t.ID)
		}
	}
	for _, t := range p.Plugins {
		if cat.Plugins == nil || !cat.Plugins(t.ID) {
			return fmt.Errorf("settings: unknown plugin %q", t.ID)
		}
	}
	if p.ActiveProfile != nil {
		return checkProfile(*p.ActiveProfile)
	}
	return nil
}

// ApplyBatch applies a whole plan or none of it: every id is checked first,
// then the projected document is written, and only a document that lands
// becomes the running set. A rejected id, a schema violation or a failed write
// therefore leaves both the file and the running set exactly as they were —
// the difference between a profile that activated and a profile that half
// activated, which is worse than either because the UI reported success.
func (s *Store) ApplyBatch(p Plan, cat Catalog) error {
	if err := p.validate(cat); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// The plan lands on copies: the live maps are replaced only after the new
	// document is on disk, so nothing below can leave them half-applied and
	// there is no rollback path to get wrong.
	next := state{
		disabledRules: cloneVerdicts(s.disabledRules),
		overrides:     s.overrides,
		shortcuts:     s.shortcuts,
		zones:         s.zones,
		panicStop:     s.panicStop,
		onboarding:    s.onboardingComplete,
		plugins:       cloneVerdicts(s.pluginsEnabled),
		profile:       s.activeProfile,
	}
	for _, t := range p.Rules {
		if t.Enabled {
			delete(next.disabledRules, t.ID)
		} else {
			next.disabledRules[t.ID] = true
		}
	}
	for _, t := range p.Plugins {
		if t.Enabled {
			next.plugins[t.ID] = true
		} else {
			delete(next.plugins, t.ID)
		}
	}
	if p.OnboardingComplete != nil {
		next.onboarding = *p.OnboardingComplete
	}
	if p.ActiveProfile != nil {
		next.profile = *p.ActiveProfile
	}
	if err := s.commitLocked(next); err != nil {
		return err
	}
	s.disabledRules = next.disabledRules
	s.pluginsEnabled = next.plugins
	s.onboardingComplete = next.onboarding
	s.activeProfile = next.profile
	return nil
}

// cloneVerdicts copies a verdict map. The batch path mutates its own copy so a
// rejected write never touches the live one.
func cloneVerdicts(in map[string]bool) map[string]bool {
	out := make(map[string]bool, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// persistLocked writes the user layer (everything in state) through the Config
// Manager (validated) to cfgPath atomically. No path = memory only (tests).
// Validation failure → error, running set untouched.
func (s *Store) persistLocked() error {
	if s.cfgPath == "" {
		return nil
	}
	s.mu.RLock()
	snap := state{
		disabledRules: s.disabledRules,
		overrides:     s.overrides,
		shortcuts:     s.shortcuts,
		zones:         s.zones,
		panicStop:     s.panicStop,
		onboarding:    s.onboardingComplete,
		plugins:       s.pluginsEnabled,
		profile:       s.activeProfile,
	}
	s.mu.RUnlock()
	return s.commitLocked(snap)
}

// commitLocked renders one state into the persisted document and lands it:
// schema check first, then a temp file plus rename so a reader never sees a
// half-written config. Caller holds the lock (write lock for a batch).
func (s *Store) commitLocked(snap state) error {
	doc, err := marshalDocument(snap)
	if err != nil {
		return err
	}
	if err := s.cfg.SetUser(doc); err != nil {
		return fmt.Errorf("settings: persist rejected: %w", err)
	}
	if s.cfgPath == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.cfgPath), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.cfgPath), "config-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(doc); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, s.cfgPath)
}

// marshalDocument renders the persisted user layer. Every list is sorted, so
// two writes of the same state produce the same bytes and a diff of a user's
// config shows what changed rather than what Go's map order felt like that day.
func marshalDocument(snap state) ([]byte, error) {
	disabled := make([]string, 0, len(snap.disabledRules))
	for id, off := range snap.disabledRules {
		if off {
			disabled = append(disabled, id)
		}
	}
	sort.Strings(disabled)
	return json.Marshal(document{
		DisabledRules:      orEmpty(disabled),
		Shortcuts:          orEmpty(snap.shortcuts),
		Zones:              orEmpty(snap.zones),
		Overrides:          orEmpty(overrideRows(snap.overrides)),
		PanicStop:          snap.panicStop,
		OnboardingComplete: snap.onboarding,
		EnabledPlugins:     orEmpty(enabledIDs(snap.plugins)),
		ActiveProfile:      snap.profile,
	})
}
