package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
)

// Level names the config precedence tier.
type Level int

const (
	LevelDefaults Level = iota
	LevelUser
	LevelPlugin
	LevelSession
)

// FieldSchema constrains one config key (MVP subset: type + required).
type FieldSchema struct {
	Type     string // "string"|"int"|"bool"|"object"|"array"
	Required bool
}

// Schema is a key → constraint map for one config layer.
type Schema map[string]FieldSchema

// Manager holds the four layers and merges them on demand. Layers are
// plain JSON objects; merge is key-wise with higher precedence winning.
// Nested objects merge recursively (one level per call); arrays and
// scalars replace wholesale.
//
// Concurrency: session overrides are runtime state, so Set-while-read is
// real (not hypothetical) — Manager is mutex-guarded (RWMutex: RLock on
// Merged/ValidateAtStartup, Lock on Set*/ClearSession). Concurrent map
// read/write would be a fatal panic, not a benign race.
// (review fix: cross-os-c0)
type Manager struct {
	mu       sync.RWMutex
	defaults json.RawMessage
	user     json.RawMessage
	plugins  map[string]json.RawMessage
	session  json.RawMessage
	schema   Schema
}

// New returns a Manager over system defaults + core schema. Defaults must
// already satisfy schema (programmer bug otherwise — fail fast).
func New(defaults json.RawMessage, schema Schema) (*Manager, error) {
	m := &Manager{defaults: defaults, plugins: map[string]json.RawMessage{}, schema: schema}
	if err := validateAgainst(defaults, schema, "defaults"); err != nil {
		return nil, err
	}
	return m, nil
}

// SetUser loads the user config layer (validated immediately — bad user
// config fails at load, not at merge).
func (m *Manager) SetUser(raw json.RawMessage) error {
	if err := validatePartial(raw, m.schema, "user"); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.user = raw
	return nil
}

// SetPlugin loads one plugin's config layer (validated against the same
// core schema; per-plugin config_schema contracts are enforced by the
// loader bead against the manifest — this package owns the merge).
func (m *Manager) SetPlugin(id string, raw json.RawMessage) error {
	if err := validatePartial(raw, m.schema, "plugin "+id); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.plugins[id] = raw
	return nil
}

// SetSession sets runtime temporary overrides (validated like the rest;
// session cannot smuggle undeclared keys).
func (m *Manager) SetSession(raw json.RawMessage) error {
	if err := validatePartial(raw, m.schema, "session"); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.session = raw
	return nil
}

// ClearSession drops session overrides.
func (m *Manager) ClearSession() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.session = nil
}

// Merged returns the 4-level merge: session > plugin(s, sorted by ID for
// determinism) > user > defaults.
func (m *Manager) Merged() map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := decodeObject(m.defaults)
	mergeInto(out, decodeObject(m.user))
	for _, id := range sortedKeys(m.plugins) {
		mergeInto(out, decodeObject(m.plugins[id]))
	}
	mergeInto(out, decodeObject(m.session))
	return out
}

// ValidateAtStartup validates every loaded layer against the schema. The
// daemon calls this at startup (ymh.2 owns the lifecycle hook; this bead
// owns the function): any error refuses startup with a schema error.
func (m *Manager) ValidateAtStartup() error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if err := validateAgainst(m.defaults, m.schema, "defaults"); err != nil {
		return err
	}
	if err := validatePartial(m.user, m.schema, "user"); err != nil {
		return err
	}
	for _, id := range sortedKeys(m.plugins) {
		if err := validatePartial(m.plugins[id], m.schema, "plugin "+id); err != nil {
			return err
		}
	}
	if err := validatePartial(m.session, m.schema, "session"); err != nil {
		return err
	}
	return nil
}

// ConfigPath returns the user config file path per OS (bead criterion:
// asserted by test, not eyeballed).
func ConfigPath(home, appSupport string) string {
	if runtime.GOOS == "darwin" {
		return filepath.Join(appSupport, "CrossOS", "config.json")
	}
	return filepath.Join(home, ".crossos", "config.json")
}

// DefaultConfigPath resolves via the OS environment.
func DefaultConfigPath() string {
	home, _ := os.UserHomeDir()
	appSupport := filepath.Join(home, "Library", "Application Support")
	return ConfigPath(home, appSupport)
}

func decodeObject(raw json.RawMessage) map[string]any {
	// Invariant: layers are pre-validated by Set*/ValidateAtStartup, so
	// garbage here means a programmer bug (direct field write), not user
	// input. Silent-empty is the merge-time behavior; startup validation is
	// the backstop that catches smuggled layers. (review: cross-os-c0)
	if len(raw) == 0 {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return map[string]any{}
	}
	return out
}

func mergeInto(dst, src map[string]any) {
	for k, v := range src {
		if dv, ok := dst[k]; ok {
			dm, dok := dv.(map[string]any)
			sm, sok := v.(map[string]any)
			if dok && sok {
				mergeInto(dm, sm)
				continue
			}
		}
		dst[k] = v
	}
}

func sortedKeys(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// validateAgainst enforces required keys + types (full-schema mode, for
// defaults which must be complete).
func validateAgainst(raw json.RawMessage, schema Schema, layer string) error {
	obj := decodeObject(raw)
	if len(raw) != 0 && len(obj) == 0 && string(raw) != "{}" && string(raw) != "null" {
		return fmt.Errorf("config: layer %s is not a JSON object", layer)
	}
	for k, fs := range schema {
		v, present := obj[k]
		if !present {
			if fs.Required {
				return fmt.Errorf("config: layer %s missing required key %q", layer, k)
			}
			continue
		}
		if err := checkType(fs.Type, v); err != nil {
			return fmt.Errorf("config: layer %s key %q: %w", layer, k, err)
		}
	}
	for k := range obj {
		if _, known := schema[k]; !known {
			return fmt.Errorf("config: layer %s has undeclared key %q", layer, k)
		}
	}
	return nil
}

// validatePartial enforces types + declared-keys on partial layers (user,
// plugin, session may omit keys; they may not add unknown ones).
func validatePartial(raw json.RawMessage, schema Schema, layer string) error {
	if len(raw) == 0 {
		return nil
	}
	obj := decodeObject(raw)
	if len(obj) == 0 && string(raw) != "{}" && string(raw) != "null" {
		return fmt.Errorf("config: layer %s is not a JSON object", layer)
	}
	for k, v := range obj {
		fs, known := schema[k]
		if !known {
			return fmt.Errorf("config: layer %s has undeclared key %q", layer, k)
		}
		if err := checkType(fs.Type, v); err != nil {
			return fmt.Errorf("config: layer %s key %q: %w", layer, k, err)
		}
	}
	return nil
}

func checkType(want string, v any) error {
	switch want {
	case "string":
		if _, ok := v.(string); !ok {
			return fmt.Errorf("want string, got %T", v)
		}
	case "bool":
		if _, ok := v.(bool); !ok {
			return fmt.Errorf("want bool, got %T", v)
		}
	case "int":
		f, ok := v.(float64)
		if !ok || f != float64(int(f)) {
			return fmt.Errorf("want int, got %v", v)
		}
	case "object":
		// Shallow: any map passes. TODO(loader): validate recursively when
		// schemas gain nested fields — same class as the recorder
		// nested-schema gap. Safe while schemas are flat primitives.
		// (review: cross-os-c0)
		if _, ok := v.(map[string]any); !ok {
			return fmt.Errorf("want object, got %T", v)
		}
	case "array":
		if _, ok := v.([]any); !ok {
			return fmt.Errorf("want array, got %T", v)
		}
	default:
		return fmt.Errorf("unknown schema type %q", want)
	}
	return nil
}
