// UI settings store: keyboard matrix toggles, window shortcuts, snap
// zones, app overrides (plan §7.2 Keyboard/Windows pages; beads 10s/nir.5).
//
// Design: the daemon owns ONE store behind the config.write* IPC methods.
// Rule enablement is a data toggle (ruleID → enabled), NOT a rule rewrite:
// decideLocked consults the toggle set alongside the plugin enable map, so
// "disable a shortcut takes effect without restart" with zero router-shape
// change. Shortcuts/zones/overrides persist via the Config Manager user
// layer (validated, then written to config.json atomically); invalid edits
// fail closed and never touch the running set.
package settings

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	shortcuts     []winlayout.Shortcut
	panicStop     bool
	cfg           *config.Manager
	cfgPath       string
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
		"shortcuts": []any{}, "disabledRules": []string{}, "panicStop": false,
	})
	cfg, err := config.New(defaults, ShortcutSchema())
	if err != nil {
		return nil, err
	}
	s := &Store{disabledRules: map[string]bool{}, shortcuts: winlayout.DefaultShortcuts(), cfg: cfg, cfgPath: cfgPath}
	if cfgPath != "" {
		if raw, err := os.ReadFile(cfgPath); err == nil {
			// Best-effort restore: validated user layer, then rehydrate
			// the disabled set from the persisted document.
			if jerr := cfg.SetUser(raw); jerr == nil {
				var doc struct {
					DisabledRules []string `json:"disabledRules"`
					PanicStop     bool     `json:"panicStop"`
				}
				if uerr := json.Unmarshal(raw, &doc); uerr == nil {
					for _, id := range doc.DisabledRules {
						s.disabledRules[id] = true
					}
					s.panicStop = doc.PanicStop
				}
			}
		}
	}
	if err := winlayout.ValidateShortcuts(s.shortcuts); err != nil {
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

// persistLocked writes the user layer (shortcuts + disabled list) through
// the Config Manager (validated) to cfgPath atomically. No path = memory
// only (tests). Validation failure → error, running set untouched.
func (s *Store) persistLocked() error {
	if s.cfgPath == "" {
		return nil
	}
	s.mu.RLock()
	disabled := make([]string, 0)
	for id := range s.disabledRules {
		disabled = append(disabled, id)
	}
	// shortcuts + panicStop travel with the same atomic write.
	snapshot := s.shortcuts
	panicStop := s.panicStop
	s.mu.RUnlock()
	doc, err := json.Marshal(map[string]any{
		"disabledRules": disabled,
		"shortcuts":     snapshot,
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
