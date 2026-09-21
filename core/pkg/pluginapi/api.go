package pluginapi

import "encoding/json"

// Plugin is the v1 plugin interface. A plugin registers behavior at load;
// Core owns the pipeline and calls nothing per-event on the fast path.
type Plugin interface {
	Manifest() Manifest
	Register(registry *Registry) error
}

// Registry collects everything a plugin declares. Registry methods are the
// normative registration mechanism; Hook entries in the manifest are async
// slow-path observers only (see Hook).
type Registry struct {
	Rules             []Rule
	Intents           []IntentDef
	Capabilities      []Capability
	ContextProviders  []CtxProvider
	Menus             []MenuDef
	UIContributions   []UIContribution
	coreCapabilityIDs map[string]struct{}
}

// NewRegistry returns a Registry seeded with the canonical core capability
// IDs (§3.12). Any plugin-owned Capability that collides with a core ID is
// rejected by RegisterCapability — plugins can never shadow Core.
func NewRegistry(coreCapabilityIDs []string) *Registry {
	r := &Registry{coreCapabilityIDs: map[string]struct{}{}}
	for _, id := range coreCapabilityIDs {
		r.coreCapabilityIDs[id] = struct{}{}
	}
	return r
}

// RegisterRule declares a physical → intent mapping with
// requires/scope/priority.
func (r *Registry) RegisterRule(rule Rule) error {
	r.Rules = append(r.Rules, rule)
	return nil
}

// RegisterIntent declares an intent the plugin produces or consumes.
func (r *Registry) RegisterIntent(intent IntentDef) error {
	r.Intents = append(r.Intents, intent)
	return nil
}

// RegisterCapability declares a plugin-owned capability. Capabilities whose
// Owner is CapabilityOwnerCore, or whose namespaced Name collides with a
// canonical core capability ID, are rejected.
func (r *Registry) RegisterCapability(cap Capability) error {
	if cap.Owner == CapabilityOwnerCore {
		return &OwnershipError{Name: cap.Name, Reason: "core-owned IDs are reserved; request the capability, don't re-declare it"}
	}
	if _, reserved := r.coreCapabilityIDs[cap.Name]; reserved {
		return &OwnershipError{Name: cap.Name, Reason: "collides with canonical core capability"}
	}
	r.Capabilities = append(r.Capabilities, cap)
	return nil
}

// RegisterContextProvider declares a LazyContext provider.
func (r *Registry) RegisterContextProvider(p CtxProvider) error {
	r.ContextProviders = append(r.ContextProviders, p)
	return nil
}

// RegisterMenu declares Finder/context menus (native actions only).
func (r *Registry) RegisterMenu(menu MenuDef) error {
	r.Menus = append(r.Menus, menu)
	return nil
}

// RegisterUI declares a UI contribution (§3.6c). The ONLY way plugin UI
// reaches the shell. MVP rejects UILocationCustomView.
func (r *Registry) RegisterUI(contrib UIContribution) error {
	if contrib.Location == UILocationCustomView {
		return &OwnershipError{Name: contrib.ID, Reason: "custom-view is Phase 4+ only; MVP Registry rejects it"}
	}
	r.UIContributions = append(r.UIContributions, contrib)
	return nil
}

// OwnershipError reports a rejected registration.
type OwnershipError struct {
	Name   string
	Reason string
}

func (e *OwnershipError) Error() string {
	return "pluginapi: registration of " + e.Name + " rejected: " + e.Reason
}

// Decision is produced by CORE's conflict resolver, never returned by
// plugins. This is the ONE canonical decision enum for the whole system —
// the fast path and the slow-path resolver both produce exactly this type.
//
// Normative: DecisionReplace = suppress the original physical event and
// execute the resolved Intent. It does NOT imply synthetic keyboard
// injection. Intent → Capability uses CGEvent/SendInput ONLY when the
// capability itself is keyboard injection.
type Decision int

const (
	DecisionPass    Decision = iota // no rule matched — pass through
	DecisionConsume                 // winner handled — suppress original
	DecisionReplace                 // winner's Intent substitutes the native action
)

// CapabilityOwner distinguishes Core-owned from plugin-owned capabilities.
type CapabilityOwner string

const (
	CapabilityOwnerCore   CapabilityOwner = "core"
	CapabilityOwnerPlugin CapabilityOwner = "plugin"
)

// Capability describes one operation. Core-owned IDs (§3.12) are reserved;
// plugin-owned capabilities must be namespaced "<pluginID>.<name>".
type Capability struct {
	Name        string          // canonical name or "<pluginID>.<name>"
	Owner       CapabilityOwner // "core" IDs are reserved; Register fails on claim
	Description string
}

// Manifest is the plugin manifest (plugin.json).
type Manifest struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Version      string          `json:"version"`
	Entry        string          `json:"entry"`
	Type         PluginType      `json:"type"`
	Permissions  []string        `json:"permissions"`
	Capabilities []Capability    `json:"capabilities"`
	ConfigSchema json.RawMessage `json:"config_schema"`
	Hooks        []Hook          `json:"hooks"`
	Safety       PluginSafety    `json:"safety"`
	APIVersion   APIVersion      `json:"api_version"`
}

// APIVersion keeps manifest/plugin/capability versions independent.
type APIVersion struct {
	ManifestVersion      string `json:"manifest_version"`
	PluginAPIVersion     string `json:"plugin_api_version"`
	CapabilityAPIVersion string `json:"capability_api_version"`
	MinCoreVersion       string `json:"min_core_version"`
}

// PluginType classifies plugins. Go `.so` is REJECTED — not listed.
type PluginType string

const (
	PluginBuiltin PluginType = "builtin"
	PluginDecl    PluginType = "declarative"
	PluginExec    PluginType = "executable"
	PluginExtPack PluginType = "extpack"
)

// Hook names. Registry methods above are normative; hooks are ASYNC
// OBSERVERS on the slow path ONLY — never the keyboard critical path,
// never returning a Decision.
type Hook string

const (
	HookOnIntent    Hook = "onIntent"    // async observer of resolved intents
	HookOnEvent     Hook = "onEvent"     // async observer, slow path only
	HookOnContext   Hook = "onContext"   // async observer
	HookOnLifecycle Hook = "onLifecycle" // async observer
)

// PluginSafety declares safe-mode/rollback behavior for a plugin.
type PluginSafety struct {
	SafeModeDurationSec int  `json:"safe_mode_duration_sec"`
	AutoRollback        bool `json:"auto_rollback"`
}

// Rule maps physical input to an intent (full schema lives with the rule
// engine bead cross-os-xtk; the shape is referenced here for Registry use).
type Rule struct {
	ID       string `json:"id"`
	Priority int    `json:"priority"`
	Scope    string `json:"scope"`
	Intent   string `json:"intent"`
}

// IntentDef declares an intent's parameter schema.
type IntentDef struct {
	Name   string          `json:"name"`
	Schema json.RawMessage `json:"schema"`
}

// CtxProvider declares a LazyContext provider.
type CtxProvider struct {
	Name string `json:"name"`
}

// MenuDef declares a Finder/context menu contribution.
type MenuDef struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// UILocation is a UI extension point (§3.6c).
type UILocation string

const (
	UILocationSettingsPage    UILocation = "settings-page"
	UILocationSettingsSection UILocation = "settings-section"
	UILocationSidebar         UILocation = "sidebar-item"
	UILocationContextMenu     UILocation = "context-menu-item"
	UILocationCommandPalette  UILocation = "command"
	UILocationStatus          UILocation = "status-indicator"
	UILocationCustomView      UILocation = "custom-view" // Phase 4+ only; MVP Registry rejects it
)

// UIContribution is the ONLY way plugin UI reaches the shell — declared at
// Register() time, rendered by the shell from schema, never by plugin code.
type UIContribution struct {
	ID         string // "<pluginID>.<contribID>", namespaced
	Location   UILocation
	Title      string
	Schema     json.RawMessage // declarative form: checkbox/select/slider/button/actions
	Visibility string          // condition, e.g. "plugin.enabled && platform == macOS"
	Actions    []string        // capabilities the UI may invoke (permission-checked)
}
