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
	"crossos/core/pkg/filetype"
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
	// fileTypes is the user's file-type catalog, seeded from
	// filetype.Seeds(). The seeds are the fallback, not the answer: a catalog
	// a user edited has to be here after a restart, or the Finder menu quietly
	// offers back the eight presets they deleted.
	fileTypes []filetype.FileType
	// menuOff is the Explorer's per-item toggle: the ids a user has switched
	// OFF. Absence means on, exactly as the daemon's own in-memory map had it
	// before this field existed — an item the user never touched is in the
	// menu, and the only thing a document that predates this key changes is
	// that a toggle the user makes from now on is remembered.
	//
	// It was in memory only, which made the bound action a promise the
	// daemon did not keep: a user switched an item off, the control redrew
	// from the reply, and the item was back after a restart with nothing
	// said. The map is here so the write has somewhere to land.
	menuOff map[string]bool
	// snapshot is the prior verdict of every id the last profile plan
	// touched, so the same plan can be undone. Nil is the honest "nothing was
	// recorded", not "nothing was on": a profile applied by a build that
	// predates this field has no snapshot, and RevertProfile refuses in words
	// rather than pretending it returned somewhere.
	snapshot *ProfileSnapshot
	cfg      *config.Manager
	cfgPath  string
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

// ProfileSnapshot is the state a profile plan is about to overwrite, kept so
// the same plan can be undone exactly — the missing half of ApplyBatch, not a
// second model.
//
// Every id the plan touches is here, and so is the profile string, because
// planForProfile writes that too: restoring the enablement while leaving the
// store claiming a profile is active would be a store that says the opposite
// of what the machine is doing. A snapshot is OVERWRITTEN by each apply, so
// apply → revert → apply → revert returns to the same place twice rather than
// to the first apply's state the second time.
type ProfileSnapshot struct {
	// Profile is the active-profile id as it was BEFORE the plan — usually
	// "", which is what a first apply restores to.
	Profile string `json:"profile"`
	// Rules and Plugins are the prior verdicts, in the same Toggle shape the
	// plan that overwrote them was written in. Only ids the plan named appear,
	// because only those were touched: a revert that rewrote every rule in the
	// table would undo hand edits made after the apply.
	Rules   []Toggle `json:"rules,omitempty"`
	Plugins []Toggle `json:"plugins,omitempty"`
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
	FileTypes          []filetype.FileType  `json:"fileTypes"`
	// MenuOff is the Explorer's per-item toggle, as the sorted ids the user
	// switched OFF. A list rather than an object for the reason
	// disabledRules is one: the same verdict two writes, and a diff of a
	// user's config shows what changed rather than what map order felt like.
	MenuOff []string `json:"menuOff"`
	// ProfileSnapshot is ABSENT until a profile plan is applied, and a nil
	// pointer marshals to nothing — which is the whole point. A document
	// written by a build that predates the key carries no snapshot, and the
	// revert's answer for that is the refusal RevertProfile returns in
	// words, rather than a button that reports it undid something.
	ProfileSnapshot *ProfileSnapshot `json:"profileSnapshot,omitempty"`
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
	fileTypes     []filetype.FileType
	menuOff       map[string]bool
	snapshot      *ProfileSnapshot
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
		// The file-type catalog rides the same user layer for the same
		// reason: an undeclared key is a rejected whole layer, so a catalog
		// the user edited would be gone on the next launch.
		"fileTypes": {Type: "array", Required: false},
		// The Explorer's per-item toggle, for the same reason once more. It
		// was bound while the write lived in memory only, so this key is the
		// difference between a switch that takes and a switch that lies.
		"menuOff": {Type: "array", Required: false},
		// The undo point for the last profile plan. An object rather than an
		// array because it is one record — the prior verdict of a set of ids
		// — and a list of them would need a rule for which one is current.
		"profileSnapshot": {Type: "object", Required: false},
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
		FileTypes:      []filetype.FileType{},
		MenuOff:        []string{},
	})
	cfg, err := config.New(defaults, ShortcutSchema())
	if err != nil {
		return nil, err
	}
	s := &Store{
		disabledRules:  map[string]bool{},
		overrides:      map[string]bool{},
		pluginsEnabled: map[string]bool{},
		menuOff:        map[string]bool{},
		shortcuts:      winlayout.DefaultShortcuts(),
		zones:          winlayout.DefaultZones(),
		fileTypes:      filetype.Seeds(),
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
					// Absent key = the seeds, same deal as the shortcut
					// table: a document written before the catalog existed
					// must not read as "no presets". A list that no longer
					// validates (hand-edited file) is dropped back to them
					// rather than served — a menu row nothing can create is
					// worse than the default.
					if doc.FileTypes != nil {
						if verr := filetype.Validate(doc.FileTypes); verr == nil {
							s.fileTypes = doc.FileTypes
						}
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
					// The Explorer's per-item toggle, same best-effort deal
					// as the trio above: absent decodes to nil, and that is
					// the "nothing is switched off" answer the fresh store
					// already holds, so a document written before the key
					// existed comes up with every declared item ON — the
					// exact state the daemon was already serving from its
					// in-memory map. Nothing the user could see changes.
					for _, id := range doc.MenuOff {
						if id != "" {
							s.menuOff[id] = true
						}
					}
					// The profile undo point. A document with none is a
					// profile applied before this field existed, and the
					// honest reading of that is "there is nothing recorded
					// to return to" — the refusal RevertProfile says in
					// words, not a revert that silently succeeds.
					if doc.ProfileSnapshot != nil {
						s.snapshot = cloneSnapshot(doc.ProfileSnapshot)
					}
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
func (s *Store) PanicStopped() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.panicStop
}

// ConfigPath is the user-layer file this store persists to, or "" for the
// memory-only store. The ownership audit reads it to decide whether the file
// is a resource CrossOS has actually created yet.
func (s *Store) ConfigPath() string { return s.cfgPath }

// SetPanicStopped latches (or clears) the kill switch and persists it.
func (s *Store) SetPanicStopped(stopped bool) error {
	return s.update(func(next *state) { next.panicStop = stopped })
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
	return s.update(func(next *state) { next.onboarding = done })
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
	return s.update(func(next *state) {
		if enabled {
			next.plugins[pluginID] = true
		} else {
			delete(next.plugins, pluginID)
		}
	})
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
	return s.update(func(next *state) { next.profile = name })
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
	return s.update(func(next *state) {
		if enabled {
			delete(next.disabledRules, ruleID)
		} else {
			next.disabledRules[ruleID] = true
		}
	})
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
	return s.update(func(next *state) {
		next.shortcuts = append([]winlayout.Shortcut(nil), set...)
	})
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
	return s.update(func(next *state) {
		next.overrides[overrideKey(app, ruleID)] = enabled
	})
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
	return s.update(func(next *state) {
		next.zones = append([]winlayout.Zone(nil), zones...)
	})
}

// FileTypes returns the file-type catalog (copy), in the order the editor
// listed it. A store that has never had the catalog written hands back the
// eight seeds rather than nothing: the Finder menu renders from this list,
// and "no file types" would leave New > empty on a fresh install. Never nil,
// for the reason Zones() is never nil.
func (s *Store) FileTypes() []filetype.FileType {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]filetype.FileType, len(s.fileTypes))
	copy(out, s.fileTypes)
	return out
}

// SetFileTypes replaces the catalog after filetype validation, and persists
// it. The validator runs over the whole list first, so one unusable extension
// cannot leave half the menu replaced — the page would show a row the daemon
// refuses to create, and the user would be hunting which one.
func (s *Store) SetFileTypes(set []filetype.FileType) error {
	if err := filetype.Validate(set); err != nil {
		return err
	}
	return s.update(func(next *state) {
		next.fileTypes = append([]filetype.FileType(nil), set...)
	})
}

// MenuOff returns the Explorer's menu ids the user has switched OFF, sorted.
//
// Absent is the answer, not a gap: a store whose document predates the key
// returns an empty list, which reads as "nothing is switched off" and hands
// the daemon exactly the map it was already holding. Sorted because the
// Explorer page polls this and Go map order would reshuffle rows between
// refreshes — the same reason PluginsEnabled sorts.
func (s *Store) MenuOff() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return enabledIDs(s.menuOff)
}

// SetMenuItemDisabled records one Explorer's menu row as off (or back on) and
// persists it. Absent = on, so turning a row back on DELETES it from the set
// rather than storing a false: a document that grew a list of explicit "on"
// verdicts would pin the menu to whatever it looked like the day the key
// landed, and every item added afterwards would be off.
//
// The id is not validated against the menu catalog here, and that is the same
// division of labour SetOverride keeps: the daemon's handler resolves the id
// against findermenu first and refuses one it does not know, so a second rule
// book in this layer would only be a second thing to drift. What IS rejected
// is the blank id, because a verdict nothing can act on is not a preference.
func (s *Store) SetMenuItemDisabled(id string, disabled bool) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("settings: empty menu item id")
	}
	return s.update(func(next *state) {
		if disabled {
			next.menuOff[id] = true
		} else {
			delete(next.menuOff, id)
		}
	})
}

// ProfileSnapshot returns the recorded undo point for the last profile plan,
// and whether there is one.
//
// ok=false is the answer for a profile applied by a build that predates the
// field, and for a document that has never applied a profile at all. It is the
// same answer in both cases and it is meant to be shown: a Revert that has
// nowhere to go has to say so in words rather than grey out silently or report
// a success it did not earn.
func (s *Store) ProfileSnapshot() (ProfileSnapshot, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.snapshot == nil {
		return ProfileSnapshot{}, false
	}
	return *cloneSnapshot(s.snapshot), true
}

// RevertProfile restores the recorded prior state: the rule and plugin
// verdicts the last profile plan overwrote, the profile string it replaced,
// and the snapshot itself, all in ONE write.
//
// Atomic for the same reason ApplyBatch is. A revert that restored the
// plugins in one write and the rules in another could fail between them and
// leave the store disagreeing with itself — which is precisely the
// half-activated outcome the batch form exists to prevent, here in the
// undoing direction.
//
// The snapshot is CLEARED, and that is what makes the second click honest:
// there is nothing left to return to, so the next call refuses in words
// rather than re-applying an undo point that has already been spent. The
// window apply → revert → apply → revert is therefore the same neutral state
// twice, and the second apply recorded the reverted state as its own undo
// point on the way in.
//
// The snapshot is read INSIDE the write, not before it. Reading it under a
// separate lock and then writing would leave a window in which a concurrent
// apply replaces the undo point and this call restores the one it read — two
// writers undoing each other, which is the store disagreeing with itself in
// the one direction the batch form exists to prevent. Reading inside update's
// closure is free: the closure already runs under the write lock, and a nil
// snapshot there means the closure changes nothing, so the document it lands
// is the same bytes the store already held.
func (s *Store) RevertProfile() (ProfileSnapshot, error) {
	var restored ProfileSnapshot
	missing := false
	err := s.update(func(next *state) {
		if next.snapshot == nil {
			missing = true
			return
		}
		snap := *next.snapshot
		for _, t := range snap.Rules {
			if t.Enabled {
				delete(next.disabledRules, t.ID)
			} else {
				next.disabledRules[t.ID] = true
			}
		}
		for _, t := range snap.Plugins {
			if t.Enabled {
				next.plugins[t.ID] = true
			} else {
				delete(next.plugins, t.ID)
			}
		}
		next.profile = snap.Profile
		next.snapshot = nil
		restored = snap
	})
	if err != nil {
		return ProfileSnapshot{}, err
	}
	if missing {
		return ProfileSnapshot{}, fmt.Errorf("settings: no profile snapshot to return to; " +
			"nothing was recorded when the profile was applied, so there is nothing to undo")
	}
	return restored, nil
}

// cloneSnapshot copies a snapshot and its two slices. The write path hands the
// copy out to a caller that will read it after the lock is released, so an
// aliased slice would be a race against the next apply — the same reason
// cloneVerdicts copies rather than shares.
func cloneSnapshot(in *ProfileSnapshot) *ProfileSnapshot {
	if in == nil {
		return nil
	}
	return &ProfileSnapshot{
		Profile: in.Profile,
		Rules:   append([]Toggle(nil), in.Rules...),
		Plugins: append([]Toggle(nil), in.Plugins...),
	}
}

// snapshotTouched captures the prior verdict of every id the plan is about to
// write, from a state that has NOT been mutated yet. Caller holds the lock.
//
// Only the ids the plan names are captured, and that is the difference between
// an undo and a reset: a revert that rewrote every rule in the table would also
// undo hand edits a person made after the apply, which is not what "undo the
// profile" means to them.
func snapshotTouched(snap state, p Plan) *ProfileSnapshot {
	out := &ProfileSnapshot{Profile: snap.profile}
	for _, t := range p.Rules {
		out.Rules = append(out.Rules, Toggle{ID: t.ID, Enabled: !snap.disabledRules[t.ID]})
	}
	for _, t := range p.Plugins {
		out.Plugins = append(out.Plugins, Toggle{ID: t.ID, Enabled: snap.plugins[t.ID]})
	}
	return out
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
//
// A plan that NAMES a profile also records its undo point, in the same write.
// The alternative — a separate SetProfileSnapshot call beside this one — is a
// crash between the two: a profile applied with no record of what it replaced
// is exactly the state Revert cannot undo, and the user is no worse off than
// with no Revert at all. Taking it here rather than asking the caller to
// supply it is also what keeps the promise true by construction: a handler
// cannot forget to build one, because there is nothing to build.
//
// The snapshot is overwritten by each such plan, so the undo point is always
// the state the LAST apply found.
func (s *Store) ApplyBatch(p Plan, cat Catalog) error {
	if err := p.validate(cat); err != nil {
		return err
	}
	return s.update(func(next *state) {
		if p.ActiveProfile != nil {
			next.snapshot = snapshotTouched(*next, p)
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
	})
}

// update is the one write path. The running state is copied, the copy is
// changed, the document is landed, and only a document that lands is adopted —
// so a failed write leaves the running set as it was, instead of moving it and
// leaving the file behind (the disagreement this store keeps being bitten by).
//
// The write lock is held from the copy to the rename, which is what serializes
// the path: two concurrent setters take it in turn, so they cannot snapshot the
// same before-state and race each other's document to disk, and a reader sees
// either the state before the write or the state after it, never a document
// mid-flight. The ipc server runs a handler per connection, so "two quick UI
// toggles" is the normal case, not a stress test.
func (s *Store) update(mutate func(*state)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.snapshotLocked()
	mutate(&next)
	if err := s.commitLocked(next); err != nil {
		return err
	}
	s.adoptLocked(next)
	return nil
}

// snapshotLocked deep-copies the running state. Aliasing the maps and walking
// them after the lock is released is a data race, not a style choice: Go aborts
// the whole process on concurrent map iteration and map write. A shallow
// snapshot is what made this crash.
func (s *Store) snapshotLocked() state {
	return state{
		disabledRules: cloneVerdicts(s.disabledRules),
		overrides:     cloneVerdicts(s.overrides),
		shortcuts:     append([]winlayout.Shortcut(nil), s.shortcuts...),
		zones:         append([]winlayout.Zone(nil), s.zones...),
		panicStop:     s.panicStop,
		onboarding:    s.onboardingComplete,
		plugins:       cloneVerdicts(s.pluginsEnabled),
		profile:       s.activeProfile,
		fileTypes:     append([]filetype.FileType(nil), s.fileTypes...),
		menuOff:       cloneVerdicts(s.menuOff),
		snapshot:      cloneSnapshot(s.snapshot),
	}
}

// adoptLocked installs a committed state as the running one.
func (s *Store) adoptLocked(next state) {
	s.disabledRules = next.disabledRules
	s.overrides = next.overrides
	s.shortcuts = next.shortcuts
	s.zones = next.zones
	s.panicStop = next.panicStop
	s.onboardingComplete = next.onboarding
	s.pluginsEnabled = next.plugins
	s.activeProfile = next.profile
	s.fileTypes = next.fileTypes
	s.menuOff = next.menuOff
	s.snapshot = next.snapshot
}

// cloneVerdicts copies a verdict map. The write path mutates its own copy so a
// rejected write never touches the live one.
func cloneVerdicts(in map[string]bool) map[string]bool {
	out := make(map[string]bool, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// commitLocked renders one state into the persisted document and lands it:
// schema check first, then a temp file plus rename so a reader never sees a
// half-written config. No path = memory only (tests). Caller is update(),
// holding the write lock, so the rename is the last thing between the caller's
// change and the disk.
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
// The file-type catalog is the one list that keeps the user's order instead:
// its index is the menu row they can see.
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
		FileTypes:          orEmpty(snap.fileTypes),
		MenuOff:            orEmpty(enabledIDs(snap.menuOff)),
		ProfileSnapshot:    cloneSnapshot(snap.snapshot),
	})
}
