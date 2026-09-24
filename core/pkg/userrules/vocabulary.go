package userrules

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"crossos/core/pkg/ctx"
	"crossos/core/pkg/intent"
	"crossos/core/pkg/keyboard"
	"crossos/core/pkg/rule"
	"crossos/core/pkg/winlayout"
)

// Picker vocabularies (bead w2-userrules).
//
// The rule builder asks four questions — which app, which key, which
// capability, which window action — and until now every answer had to be
// typed. These are those four lists, and each is read from the package that
// already owns the vocabulary rather than copied into a second table: ctx for
// app modes and categories, keyboard for the modifier bits,
// intent.DefaultRegistry for capabilities, winlayout for the §6.2 actions.
// A picker built from copies is a picker offering answers the router cannot
// honour — a mode that never matches, a capability the registry does not
// have, an action that resolves nowhere.
//
// Every list is built per call and returned as a fresh value, including the
// bytes inside a row. The Settings page polls these; a list that hands out a
// shared backing array is one whose second caller can rewrite the first
// caller's rows.

// Option is one picker row: the value a stored rule carries, and the label
// the editor shows for it. A separate label is what lets a vocabulary grow a
// user-facing name without the shell keeping its own mapping table.
type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// AppModes is the "IF App is a …" list, from ctx — the package that owns the
// taxonomy. event.AppMode mirrors these strings for the matcher, so a mode
// that appears here is a mode the router can match on.
func AppModes() []Option {
	return []Option{
		{string(ctx.AppModeNative), "Ordinary app"},
		{string(ctx.AppModeTerminal), "Terminal"},
		{string(ctx.AppModeRemote), "Remote desktop"},
		{string(ctx.AppModeVM), "Virtual machine"},
		{string(ctx.AppModeExcluded), "Excluded from CrossOS"},
	}
}

// AppCategories is the "in a …" grouping list, from ctx.
//
// A category is a grouping ctx applies when it classifies the focused app,
// not a dimension a rule stores: event.CompiledRule matches on app mode, app
// ID and device, and the mode is the one the category resolves to (the seed
// classifier maps terminal, remote, browser, system and user onto a mode and
// an app). So the builder offers categories to narrow WHICH apps the app
// picker lists, and the rule it stores carries the app it settled on.
func AppCategories() []Option {
	return []Option{
		{string(ctx.AppTerminal), "Terminals"},
		{string(ctx.AppBrowser), "Browsers"},
		{string(ctx.AppRemote), "Remote and virtual machines"},
		{string(ctx.AppSystem), "System apps"},
		{string(ctx.AppUser), "Everything else"},
	}
}

// modifierRow is one modifier-bit name, the spelling both the rule table and
// the matrix chord renderer use.
type modifierRow struct {
	name string
	bit  uint32
}

// modifierRows is the modifier vocabulary in chord order. One slice, read by
// the picker, by ModifierMask and by the derived rule ID, so a chord cannot
// render as "Ctrl+Win" in one place and "Win+Ctrl" in another.
var modifierRows = []modifierRow{
	{"Ctrl", keyboard.ModCtrl},
	{"Shift", keyboard.ModShift},
	{"Alt", keyboard.ModAlt},
	{"Win", keyboard.ModMeta}, // Cmd on macOS, Win on Windows
}

// Modifiers is the "press …" part of the chord picker.
func Modifiers() []Option {
	out := make([]Option, 0, len(modifierRows))
	for _, m := range modifierRows {
		out = append(out, Option{Value: m.name, Label: m.name})
	}
	return out
}

// ModifierMask folds picked modifier names into the bitmask
// event.CompiledRule matches on, which is keyboard.State's own mask — the
// overlay the native callback applies before matching. An unknown name is
// refused with the name: a mask missing a bit the user picked is a chord
// that fires on the wrong keystrokes.
func ModifierMask(names []string) (uint32, error) {
	var mask uint32
	for _, want := range names {
		found := false
		for _, m := range modifierRows {
			if m.name == want {
				mask |= m.bit
				found = true
				break
			}
		}
		if !found {
			return 0, fmt.Errorf("userrules: unknown modifier %q (known: %s)",
				want, strings.Join(modifierNames(), ", "))
		}
	}
	return mask, nil
}

func modifierNames() []string {
	out := make([]string, 0, len(modifierRows))
	for _, m := range modifierRows {
		out = append(out, m.name)
	}
	return out
}

// Key is one row of the key picker: the name a rule stores, and the Windows
// virtual-key code it compiles to.
//
// The codes are the Windows VK space core/rules is authored in — the tap
// translates macOS keycodes at the boundary — so a chord a user builds here
// and a row in the builtin table mean the same key, and the matrix chord
// renderer (keyName, the reverse map) can name any of them.
type Key struct {
	Value string `json:"value"`
	Label string `json:"label"`
	Code  uint32 `json:"code"`
}

// namedKeys are the keys the generated ranges do not cover, in the order the
// editor shows them: window actions bind the arrows and the navigation
// cluster, so those lead.
var namedKeys = []Key{
	{"Left", "Left", 0x25},
	{"Right", "Right", 0x27},
	{"Up", "Up", 0x26},
	{"Down", "Down", 0x28},
	{"Home", "Home", 0x24},
	{"End", "End", 0x23},
	{"PageUp", "Page Up", 0x21},
	{"PageDown", "Page Down", 0x22},
	{"Return", "Return", 0x0D},
	{"Tab", "Tab", 0x09},
	{"Space", "Space", 0x20},
	{"Escape", "Esc", 0x1B},
	{"Backspace", "Backspace", 0x08},
	{"CapsLock", "Caps Lock", 0x14},
	{"PrintScreen", "Print Screen", 0x2C},
	{"Insert", "Insert", 0x2D},
	{"Delete", "Delete", 0x2E},
	{"LWin", "Left Win", 0x5B},
	{"RWin", "Right Win", 0x5C},
	{"NumMultiply", "Numpad *", 0x6A},
	{"NumAdd", "Numpad +", 0x6B},
	{"NumSubtract", "Numpad -", 0x6D},
	{"NumDecimal", "Numpad .", 0x6E},
	{"NumDivide", "Numpad /", 0x6F},
}

// Keys is the key picker: the named keys, then F1–F24, the letters, the top
// row digits, and the numpad — the order a person scans them in.
func Keys() []Key {
	out := make([]Key, 0, len(namedKeys)+24+26+10+10)
	out = append(out, namedKeys...)
	for i := 1; i <= 24; i++ {
		name := fmt.Sprintf("F%d", i)
		out = append(out, Key{name, name, uint32(0x6F + i)})
	}
	for c := 'A'; c <= 'Z'; c++ {
		name := string(c)
		out = append(out, Key{name, name, uint32(c)})
	}
	for d := '0'; d <= '9'; d++ {
		name := string(d)
		out = append(out, Key{name, name, uint32(d)})
	}
	for d := 0; d <= 9; d++ {
		name := "Num" + string(rune('0'+d))
		out = append(out, Key{name, name, uint32(0x60 + d)})
	}
	return out
}

// KeyCode resolves a picked key name to its virtual-key code. An unknown
// name is refused with the name, so a chord that can never fire is reported
// at the picker rather than swallowing the key at the tap.
func KeyCode(name string) (uint32, error) {
	for _, k := range Keys() {
		if k.Value == name {
			return k.Code, nil
		}
	}
	return 0, fmt.Errorf("userrules: unknown key %q", name)
}

// Capability is one row of the "THEN …" picker: the registry ID a rule names,
// plus the descriptor fields the editor needs to render that capability's
// parameters without asking the daemon twice.
type Capability struct {
	Value      string            `json:"value"`
	Label      string            `json:"label"`
	Permission string            `json:"permission"`
	Required   []string          `json:"required"`
	Properties map[string]string `json:"properties"`
}

// Capabilities is every capability intent.DefaultRegistry holds, sorted by
// ID. Registry.IDs walks a map, so the sort is what makes the picker stable
// across polls.
func Capabilities() []Capability {
	reg := intent.DefaultRegistry()
	ids := reg.IDs()
	sort.Strings(ids)
	out := make([]Capability, 0, len(ids))
	for _, id := range ids {
		d, _ := reg.Get(id)
		required := d.InputSchema.Required
		if required == nil {
			required = []string{}
		}
		out = append(out, Capability{
			Value:      id,
			Label:      d.Description,
			Permission: string(d.Permission),
			Required:   required,
			Properties: d.InputSchema.Properties,
		})
	}
	return out
}

// Action is one row of the window-action picker: the §6.2 display name, and
// the capability plus parameters a rule naming it stores. Resolving the
// action here is what keeps the editor from composing a window.move
// parameter of its own — winlayout already says which zone each action means.
type Action struct {
	Value      string          `json:"value"`
	Label      string          `json:"label"`
	Capability string          `json:"capability"`
	Parameters json.RawMessage `json:"parameters,omitempty"`
}

// Actions is the window-action picker, from winlayout.AllActions in its own
// order. Round-trip: every row's Value resolves back through
// ActionCapability, which is winlayout.ActionByName.
func Actions() []Action {
	out := make([]Action, 0, len(winlayout.AllActions))
	for _, a := range winlayout.AllActions {
		cap, params, err := actionParts(a)
		if err != nil {
			// Unreachable: the table is winlayout's own and CapabilityFor
			// covers every action in it. Skipping is still the right answer
			// if that ever stops holding — a picker row whose action
			// resolves to nothing is a rule that cannot be built.
			continue
		}
		name := winlayout.ActionName(a)
		out = append(out, Action{Value: name, Label: name, Capability: cap, Parameters: params})
	}
	return out
}

// ActionCapability resolves a picked window action to the capability and
// parameters a stored rule carries. The reverse of Actions, and the
// round-trip that keeps the picker and the compiler reading one table: an
// unknown name fails closed rather than defaulting to a neighbour's action.
func ActionCapability(name string) (capability string, params json.RawMessage, err error) {
	a, ok := winlayout.ActionByName(name)
	if !ok {
		return "", nil, fmt.Errorf("userrules: unknown window action %q", name)
	}
	return actionParts(a)
}

func actionParts(a winlayout.Action) (string, json.RawMessage, error) {
	cap, zone := winlayout.CapabilityFor(a)
	if zone == "" {
		// window.minimize and window.maximize carry no parameter; the
		// geometry actions all do, and ZoneName is the zone the adapter
		// resolves the frame from.
		return cap, nil, nil
	}
	raw, err := json.Marshal(map[string]string{"zone": zone})
	if err != nil {
		return "", nil, err
	}
	return cap, raw, nil
}

// ScopeName is the label for a resolution scope, for the rule row that
// shows the user why their rule outranks another.
func ScopeName(s rule.Scope) string {
	switch s {
	case rule.ScopeGlobal:
		return "global"
	case rule.ScopeApp:
		return "app"
	case rule.ScopeWindow:
		return "window"
	case rule.ScopeDevice:
		return "device"
	}
	return "unknown"
}
