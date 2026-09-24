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
	cfg       *config.Manager
	cfgPath   string
}

// Override is one stored per-app verdict on a matrix rule. The json tags are
// the config.setOverride wire names, so the persisted document and the IPC
// row are the same shape.
type Override struct {
	App     string `json:"app"`
	RuleID  string `json:"rule_id"`
	Enabled bool   `json:"enabled"`
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
	}
}

// New returns a Store seeded from the winlayout defaults. cfgPath may be
// "" (no persistence — tests); otherwise the user layer loads from disk
// when present (absent file = defaults, never an error).
func New(cfgPath string) (*Store, error) {
	defaults, _ := json.Marshal(map[string]any{
		"shortcuts": []any{}, "disabledRules": []string{}, "zones": []any{},
		"overrides": []any{}, "panicStop": false,
	})
	cfg, err := config.New(defaults, ShortcutSchema())
	if err != nil {
		return nil, err
	}
	s := &Store{
		disabledRules: map[string]bool{},
		overrides:     map[string]bool{},
		shortcuts:     winlayout.DefaultShortcuts(),
		zones:         winlayout.DefaultZones(),
		cfg:           cfg,
		cfgPath:       cfgPath,
	}
	if cfgPath != "" {
		if raw, err := os.ReadFile(cfgPath); err == nil {
			// Best-effort restore: validated user layer, then rehydrate
			// the disabled set from the persisted document.
			if jerr := cfg.SetUser(raw); jerr == nil {
				var doc struct {
					DisabledRules []string              `json:"disabledRules"`
					Shortcuts     []winlayout.Shortcut  `json:"shortcuts"`
					Zones         []winlayout.Zone      `json:"zones"`
					Overrides     []struct {
						App     string `json:"app"`
						RuleID  string `json:"rule_id"`
						Enabled bool   `json:"enabled"`
					} `json:"overrides"`
					PanicStop bool `json:"panicStop"`
				}
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
	out := make([]Override, 0, len(s.overrides))
	for k, on := range s.overrides {
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

// persistLocked writes the user layer (shortcuts + disabled list + zones +
// overrides) through the Config Manager (validated) to cfgPath atomically. No
// path = memory only (tests). Validation failure → error, running set
// untouched.
func (s *Store) persistLocked() error {
	if s.cfgPath == "" {
		return nil
	}
	s.mu.RLock()
	disabled := make([]string, 0, len(s.disabledRules))
	for id := range s.disabledRules {
		disabled = append(disabled, id)
	}
	sort.Strings(disabled)
	// overrides: read under the same lock as the write above, then sorted so
	// the document is byte-stable across writes of the same state.
	overrides := make([]Override, 0, len(s.overrides))
	for k, on := range s.overrides {
		app, ruleID, _ := strings.Cut(k, "\x00")
		overrides = append(overrides, Override{App: app, RuleID: ruleID, Enabled: on})
	}
	sort.Slice(overrides, func(i, j int) bool {
		if overrides[i].App != overrides[j].App {
			return overrides[i].App < overrides[j].App
		}
		return overrides[i].RuleID < overrides[j].RuleID
	})
	// shortcuts + zones + panicStop travel with the same atomic write.
	snapshot := s.shortcuts
	zoneSnapshot := s.zones
	panicStop := s.panicStop
	s.mu.RUnlock()
	doc, err := json.Marshal(map[string]any{
		"disabledRules": orEmpty(disabled),
		"shortcuts":     orEmpty(snapshot),
		"zones":         orEmpty(zoneSnapshot),
		"overrides":     orEmpty(overrides),
		"panicStop":     panicStop,
	})
	if err != nil {
		return err
	}
	if err := s.cfg.SetUser(doc); err != nil {
		return fmt.Errorf("settings: persist rejected: %w", err)
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
