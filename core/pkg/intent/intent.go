package intent

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Intent is a versioned structured type — never a loose Action string +
// any-map. Parameters are validated against the target capability's input
// schema (Validate checks presence; ValidateAgainst checks shape).
type Intent struct {
	ID         string          // e.g. "window.snap", "clipboard.copy" (canonical registry, §3.12)
	Version    int             // intent schema version
	Source     Source          // "keyboard", "hotkey", "contextMenu"
	Parameters json.RawMessage // e.g. {"zone":"left-half"} — schema-validated
}

// Source names where an intent originated.
type Source string

const (
	SourceKeyboard    Source = "keyboard"
	SourceHotkey      Source = "hotkey"
	SourceContextMenu Source = "contextMenu"
)

// Platform names an OS a capability runs on.
type Platform string

const (
	PlatformMacOS   Platform = "macos"
	PlatformWindows Platform = "windows"
)

// SideEffect names an observable effect beyond the return value.
type SideEffect string

const (
	SideEffectClosesWindow  SideEffect = "closes-focused-window"
	SideEffectMovesWindow   SideEffect = "moves-focused-window"
	SideEffectModifiesFile  SideEffect = "modifies-file"
	SideEffectCreatesFile   SideEffect = "creates-file"
	SideEffectLaunchesApp   SideEffect = "launches-app"
	SideEffectInjectsInput  SideEffect = "injects-input"
	SideEffectReadsScreen   SideEffect = "reads-screen"
	SideEffectModifiesMenus SideEffect = "modifies-menus"
)

// Permission is a user-granted consent, checked by Core before any action.
type Permission string

const (
	PermInputMonitor   Permission = "input.monitor"
	PermInputIntercept Permission = "input.intercept"
	// PermAccessControl is the ONLY "control other apps" permission. A prior
	// draft's PermAccessibility ("accessibility") was removed as a duplicate;
	// do not reintroduce it.
	PermAccessControl  Permission = "accessibility.control"
	PermFilesystem     Permission = "filesystem"
	PermNetwork        Permission = "network"
	PermShellExecution Permission = "shell.execute"
	PermFinderModify   Permission = "finder.modify"
)

// Schema is a JSON-Schema-shaped constraint for capability parameters.
// MVP validates required-keys + primitive types only (see ValidateParams);
// full JSON-Schema evaluation is deferred to the config-manager bead.
type Schema struct {
	Required   []string          `json:"required"`
	Properties map[string]string `json:"properties"` // name → "string"|"int"|"bool"|"object"|"array"
}

// CapabilityDescriptor is the versioned contract for one capability (§3.12).
type CapabilityDescriptor struct {
	ID           string // e.g. "window.close"
	Version      string
	Description  string
	Permission   Permission // CrossOS permission required
	InputSchema  Schema
	OutputSchema Schema
	Platforms    []Platform // macOS / Windows
	SideEffects  []SideEffect
	Reversible   bool
}

// invalidNames are parallel inventions the plan explicitly bans. They read
// like capabilities but collide semantically with canonical IDs or
// permissions; requests using them fail closed.
var invalidNames = map[string]string{
	"keyboard.intercept": "use input.intercept",
	"window.manage":      "use window.* (read/move/close/minimize/maximize)",
	"finder.contextMenu": "use finder.menu",
}

// ValidateCapabilityID rejects unknown IDs and banned parallel names.
func ValidateCapabilityID(reg *Registry, id string) error {
	if hint, banned := invalidNames[id]; banned {
		return fmt.Errorf("intent: banned capability name %q: %s", id, hint)
	}
	if _, ok := reg.Get(id); !ok {
		return fmt.Errorf("intent: unknown capability %q", id)
	}
	return nil
}

// Registry is the canonical capability registry v1 (§3.12).
type Registry struct {
	byID map[string]CapabilityDescriptor
}

// NewRegistry builds the canonical v1 registry. Duplicate IDs fail.
func NewRegistry(descs []CapabilityDescriptor) (*Registry, error) {
	r := &Registry{byID: map[string]CapabilityDescriptor{}}
	for _, d := range descs {
		if d.ID == "" {
			return nil, fmt.Errorf("intent: capability with empty ID")
		}
		if _, dup := r.byID[d.ID]; dup {
			return nil, fmt.Errorf("intent: duplicate capability ID %q", d.ID)
		}
		if hint, banned := invalidNames[d.ID]; banned {
			return nil, fmt.Errorf("intent: banned capability ID %q: %s", d.ID, hint)
		}
		r.byID[d.ID] = d
	}
	return r, nil
}

// Get returns the descriptor for id, or false.
func (r *Registry) Get(id string) (CapabilityDescriptor, bool) {
	d, ok := r.byID[id]
	return d, ok
}

// IDs lists all registered capability IDs.
func (r *Registry) IDs() []string {
	out := make([]string, 0, len(r.byID))
	for id := range r.byID {
		out = append(out, id)
	}
	return out
}

// Validate checks an Intent's structural validity (ID known + banned-name
// free, version positive, source known).
func (in Intent) Validate(reg *Registry) error {
	if err := ValidateCapabilityID(reg, in.ID); err != nil {
		return err
	}
	if in.Version <= 0 {
		return fmt.Errorf("intent: version must be positive, got %d", in.Version)
	}
	switch in.Source {
	case SourceKeyboard, SourceHotkey, SourceContextMenu:
	default:
		return fmt.Errorf("intent: unknown source %q", in.Source)
	}
	return nil
}

// ValidateAgainst additionally checks Parameters against the target
// capability's InputSchema (required keys + primitive types).
func (in Intent) ValidateAgainst(reg *Registry) error {
	if err := in.Validate(reg); err != nil {
		return err
	}
	desc, _ := reg.Get(in.ID)
	if len(desc.InputSchema.Required) == 0 && len(desc.InputSchema.Properties) == 0 {
		return nil // capability takes no parameters
	}
	var params map[string]json.RawMessage
	if len(in.Parameters) == 0 {
		params = map[string]json.RawMessage{}
	} else if err := json.Unmarshal(in.Parameters, &params); err != nil {
		return fmt.Errorf("intent: parameters must be a JSON object: %w", err)
	}
	for _, req := range desc.InputSchema.Required {
		if _, ok := params[req]; !ok {
			return fmt.Errorf("intent: missing required parameter %q for %s", req, in.ID)
		}
	}
	for name, raw := range params {
		want, declared := desc.InputSchema.Properties[name]
		if !declared {
			return fmt.Errorf("intent: undeclared parameter %q for %s", name, in.ID)
		}
		if err := checkPrimitive(want, raw); err != nil {
			return fmt.Errorf("intent: parameter %q: %w", name, err)
		}
	}
	return nil
}

func checkPrimitive(want string, raw json.RawMessage) error {
	s := strings.TrimSpace(string(raw))
	switch want {
	case "string":
		var v string
		if err := json.Unmarshal(raw, &v); err != nil {
			return fmt.Errorf("want string, got %s", s)
		}
	case "int":
		var v float64
		if err := json.Unmarshal(raw, &v); err != nil || v != float64(int(v)) {
			return fmt.Errorf("want int, got %s", s)
		}
	case "bool":
		var v bool
		if err := json.Unmarshal(raw, &v); err != nil {
			return fmt.Errorf("want bool, got %s", s)
		}
	case "object":
		var v map[string]any
		if err := json.Unmarshal(raw, &v); err != nil {
			return fmt.Errorf("want object, got %s", s)
		}
	case "array":
		var v []any
		if err := json.Unmarshal(raw, &v); err != nil {
			return fmt.Errorf("want array, got %s", s)
		}
	default:
		return fmt.Errorf("unknown schema type %q", want)
	}
	return nil
}

// Request is the capability request path: plugin → Core → Adapter. It binds
// an Intent to its capability descriptor and the granted permissions; the
// Adapter executes only what Request authorizes.
type Request struct {
	PluginID   string
	Intent     Intent
	Capability CapabilityDescriptor
	Granted    []Permission
}

// Authorize validates the intent against the registry and checks the
// required permission is within the plugin's grants. Fail-closed: any
// mismatch is an error, never a default-allow.
func Authorize(reg *Registry, pluginID string, in Intent, granted []Permission) (*Request, error) {
	if err := in.ValidateAgainst(reg); err != nil {
		return nil, err
	}
	desc, _ := reg.Get(in.ID)
	has := false
	for _, g := range granted {
		if g == desc.Permission {
			has = true
			break
		}
	}
	if !has {
		return nil, fmt.Errorf("intent: plugin %q lacks permission %q for %s",
			pluginID, desc.Permission, in.ID)
	}
	return &Request{PluginID: pluginID, Intent: in, Capability: desc, Granted: granted}, nil
}
