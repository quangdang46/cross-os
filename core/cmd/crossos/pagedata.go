// Page data sources over IPC (bead cross-os-jzj).
//
// The shell's ten settings pages declare a `source` string per control
// ("core:behaviorMatrix", "core:snapZones", "core:trialCountdown", …) and
// app/backend resolves nothing. This file is the resolving end: one IPC
// method per source, each reading the package that already owns the state
// (core/rules for the matrix, settings for overrides + zones, winlayout for
// zone geometry, safety for trials + ownership). No page id appears here and
// no list is hand-maintained — where a source has no real state behind it
// yet (commands, plugin schemas), the method serves an empty array and says
// so, because a fabricated row is worse than an honest gap.
//
// Two rules shape every handler below:
//   - Lists are ordered before they leave. The shell polls these; a list
//     that reshuffles per poll is a bug the user files, and Go map order is
//     not stable, so every map read here is sorted or copied in a fixed order.
//   - A write validates the whole payload before it touches the running set,
//     and an unknown id is a typed RPCError, never a silent no-op.

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"crossos/core/pkg/event"
	"crossos/core/pkg/intent"
	"crossos/core/pkg/ipc"
	"crossos/core/pkg/keyboard"
	"crossos/core/pkg/pluginapi"
	"crossos/core/pkg/safety"
	"crossos/core/pkg/winlayout"
	builtin "crossos/core/rules"
)

// --- config.getMatrix ---

// matrixRow is one behavior-matrix row (page control source
// "core:behaviorMatrix"). contexts is never null: a rule with no app-mode
// restriction renders an empty list, which is the honest reading of
// "anywhere" rather than a missing value.
type matrixRow struct {
	RuleID   string   `json:"rule_id"`
	Plugin   string   `json:"plugin"`
	Action   string   `json:"action"`
	Keys     string   `json:"keys"`
	Contexts []string `json:"contexts"`
	Enabled  bool     `json:"enabled"`
}

// handleGetMatrix serves config.getMatrix.
//
// The rows come from builtin.All() — the same table decideLocked compiles —
// so a rule the matrix shows is by construction a rule that fires, and a new
// matrix row needs no second registration. Enabled is the user toggle only
// (settings.IsRuleEnabled, the complement of the disabled set config.
// setRuleEnabled writes); plugin enablement is its own column on the Plugins
// page, and ANDing it in here would make one toggle look like it had turned
// the other off.
func (c *Core) handleGetMatrix(_ json.RawMessage) (any, *ipc.RPCError) {
	rules := builtin.All()
	out := make([]matrixRow, 0, len(rules))
	for _, r := range rules {
		out = append(out, matrixRow{
			RuleID:   r.RuleID,
			Plugin:   r.PluginID,
			Action:   matrixAction(r),
			Keys:     chord(r),
			Contexts: ruleContexts(r),
			Enabled:  c.set.IsRuleEnabled(r.RuleID),
		})
	}
	// Plugin, then action: the matrix is read as one block per plugin, and
	// rule ID breaks a tie so two rules claiming one action keep a fixed
	// order across polls.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Plugin != out[j].Plugin {
			return out[i].Plugin < out[j].Plugin
		}
		if out[i].Action != out[j].Action {
			return out[i].Action < out[j].Action
		}
		return out[i].RuleID < out[j].RuleID
	})
	return out, nil
}

// builtinRule resolves a RuleID against the builtin table (the same source
// knownRuleIDs validates against, so a stored override can never name a rule
// that does not exist).
func builtinRule(ruleID string) (event.CompiledRule, bool) {
	for _, r := range builtin.All() {
		if r.RuleID == ruleID {
			return r, true
		}
	}
	return event.CompiledRule{}, false
}

// matrixAction names what a rule does. A window.* intent is resolved through
// winlayout — the package that owns the action vocabulary — so the matrix
// reads "Left Half" instead of "window.move" three times over. An intent
// winlayout does not own (clipboard.copy, terminal.openAt, app.open) keeps
// its intent ID, which is its honest name.
func matrixAction(r event.CompiledRule) string {
	zone := intentZone(r.Intent)
	if r.Intent.ID == "window.move" {
		if a, ok := winlayout.ActionForZone(zone); ok {
			return winlayout.ActionName(a)
		}
	}
	// The dedicated window capabilities carry no zone (window.minimize,
	// window.maximize); a zone-carrying window.move never matches here, so an
	// unknown zone falls through to the intent ID instead of picking a
	// neighbour's action.
	for _, a := range winlayout.AllActions {
		cap, z := winlayout.CapabilityFor(a)
		if cap == r.Intent.ID && z == zone {
			return winlayout.ActionName(a)
		}
	}
	return r.Intent.ID
}

// intentZone reads the window.move zone parameter. A malformed parameter is
// read as "no zone": this is a display path, and a bad parameter must not
// fail the whole matrix read.
func intentZone(in intent.Intent) string {
	if len(in.Parameters) == 0 {
		return ""
	}
	var p struct {
		Zone string `json:"zone"`
	}
	if err := json.Unmarshal(in.Parameters, &p); err != nil {
		return ""
	}
	return p.Zone
}

// ruleContexts lists the app modes a rule claims. App-ID scoping (the
// developer matrix names six bundle IDs) is deliberately not in this column:
// six identifiers per row would bury the mode list, and the rule ID already
// names what the user toggles.
func ruleContexts(r event.CompiledRule) []string {
	out := make([]string, 0, len(r.AppModes))
	for _, m := range r.AppModes {
		out = append(out, string(m))
	}
	return out
}

// chord renders a rule's physical binding. The rule table is authored in
// Windows virtual-key codes (core/rules header) and the tap translates macOS
// codes at the boundary, so one rendering describes Windows muscle memory on
// either host.
func chord(r event.CompiledRule) string {
	parts := make([]string, 0, 5)
	for _, m := range []struct {
		bit  uint32
		name string
	}{
		{keyboard.ModCtrl, "Ctrl"},
		{keyboard.ModShift, "Shift"},
		{keyboard.ModAlt, "Alt"},
		{keyboard.ModMeta, "Win"},
	} {
		if r.Modifiers&m.bit != 0 {
			parts = append(parts, m.name)
		}
	}
	parts = append(parts, keyName(r.KeyCode))
	return strings.Join(parts, "+")
}

// keyName names a Windows virtual-key code. The generated ranges cover
// letters, digits, the numpad and F1-F24; the cases cover the named keys.
// Anything else renders as "VK 0x..": a keycode added to the rule table shows
// up as a raw code the user can report, instead of an empty chord that looks
// like a rendering bug.
func keyName(vk uint32) string {
	switch {
	case vk >= 'A' && vk <= 'Z', vk >= '0' && vk <= '9':
		return string(rune(vk))
	case vk >= 0x70 && vk <= 0x87: // F1–F24
		return "F" + strconv.Itoa(int(vk-0x6F))
	case vk >= 0x60 && vk <= 0x69: // numpad 0–9
		return "Num" + string(rune('0'+vk-0x60))
	}
	switch vk {
	case 0x08:
		return "Backspace"
	case 0x09:
		return "Tab"
	case 0x0D:
		return "Return"
	case 0x14:
		return "CapsLock"
	case 0x1B:
		return "Escape"
	case 0x20:
		return "Space"
	case 0x21:
		return "PageUp"
	case 0x22:
		return "PageDown"
	case 0x23:
		return "End"
	case 0x24:
		return "Home"
	case 0x25:
		return "Left"
	case 0x26:
		return "Up"
	case 0x27:
		return "Right"
	case 0x28:
		return "Down"
	case 0x2C:
		return "PrintScreen"
	case 0x2D:
		return "Insert"
	case 0x2E:
		return "Delete"
	case 0x5B:
		return "LWin"
	case 0x5C:
		return "RWin"
	case 0x6A:
		return "NumMultiply"
	case 0x6B:
		return "NumAdd"
	case 0x6D:
		return "NumSubtract"
	case 0x6E:
		return "NumDecimal"
	case 0x6F:
		return "NumDivide"
	}
	return fmt.Sprintf("VK 0x%02X", vk)
}

// --- config.getOverrides / config.setOverride ---

// overrideRow is one per-app verdict on a matrix rule (page control source
// "core:appOverrides"). action/keys are resolved from the rule table so the
// editor renders a row from one response instead of joining two.
type overrideRow struct {
	App     string `json:"app"`
	RuleID  string `json:"rule_id"`
	Action  string `json:"action"`
	Keys    string `json:"keys"`
	Enabled bool   `json:"enabled"`
}

// handleGetOverrides serves config.getOverrides. An empty list means the user
// has expressed no per-app opinion yet — every rule follows its global toggle.
// The list is deliberately not expanded to one row per app × rule: the app
// vocabulary is the machine's installed applications, which the daemon cannot
// enumerate, and inventing rows for apps nobody overrides would bury the ones
// they did.
func (c *Core) handleGetOverrides(_ json.RawMessage) (any, *ipc.RPCError) {
	stored := c.set.Overrides() // sorted by app, then rule ID
	out := make([]overrideRow, 0, len(stored))
	for _, o := range stored {
		row := overrideRow{App: o.App, RuleID: o.RuleID, Enabled: o.Enabled}
		// A stored override whose rule is not in the table any more (a rule
		// removed in a later release) keeps its row with no action/chord
		// rather than being dropped: the user's stored intent is still in
		// their config file, and hiding it would make the list lie.
		if r, ok := builtinRule(o.RuleID); ok {
			row.Action, row.Keys = matrixAction(r), chord(r)
		}
		out = append(out, row)
	}
	return out, nil
}

// handleSetOverride serves config.setOverride: {app, rule_id, enabled}. An
// unknown rule ID is refused before anything is stored, and the reply repeats
// the stored verdict so the editor never has to assume its own write landed.
// An absent `enabled` decodes to false, so a malformed call disables the
// override for that app rather than enabling it — the fail-closed direction.
func (c *Core) handleSetOverride(raw json.RawMessage) (any, *ipc.RPCError) {
	var p struct {
		App     string `json:"app"`
		RuleID  string `json:"rule_id"`
		Enabled bool   `json:"enabled"`
	}
	if err := json.Unmarshal(raw, &p); err != nil ||
		strings.TrimSpace(p.App) == "" || strings.TrimSpace(p.RuleID) == "" {
		return nil, &ipc.RPCError{Code: ipc.ErrBadParams, Message: "need {app, rule_id, enabled}"}
	}
	if err := c.set.SetOverride(p.App, p.RuleID, p.Enabled, c.knownRuleIDs); err != nil {
		return nil, &ipc.RPCError{Code: ipc.ErrInvalid, Message: err.Error()}
	}
	return map[string]any{"app": p.App, "rule_id": p.RuleID, "enabled": p.Enabled}, nil
}

// --- config.getZones / config.setZones ---

// handleGetZones serves config.getZones. winlayout.Zone's json tags ARE the
// wire row, so the editor and the persisted config document cannot drift.
// The list starts empty by design (see winlayout/zones.go): the daemon has no
// display geometry outside the adapter, and seeding rectangles from an
// assumed screen would put points on the user's desktop that they never chose.
func (c *Core) handleGetZones(_ json.RawMessage) (any, *ipc.RPCError) {
	return c.set.Zones(), nil
}

// handleSetZones serves config.setZones: {zones:[{id,name,x,y,w,h}]} → {count}.
// winlayout validation covers the whole list first, so one bad row leaves the
// running set exactly as it was.
func (c *Core) handleSetZones(raw json.RawMessage) (any, *ipc.RPCError) {
	var p struct {
		Zones []winlayout.Zone `json:"zones"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, &ipc.RPCError{Code: ipc.ErrBadParams, Message: "need {zones:[{id,name,x,y,w,h}]}"}
	}
	if err := c.set.SetZones(p.Zones); err != nil {
		return nil, &ipc.RPCError{Code: ipc.ErrInvalid, Message: err.Error()}
	}
	return map[string]any{"count": len(p.Zones)}, nil
}

// --- core.commands / core.pluginSchemas ---

// commandRow is one command-palette entry (page control source
// "core:commands").
type commandRow struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Plugin string `json:"plugin"`
}

// handleCoreCommands serves core.commands.
//
// Empty, and that is the truthful answer today. The builtin plugins are
// compiled straight from core/rules, so no pluginapi.Registry exists at
// runtime and nothing has registered a command; the only registration channel
// (a UIContribution with Location "command") has no registrant. A hand-written
// entry here would be a command the user could open and find nothing behind —
// exactly the fabricated control this bead exists to prevent. The handler
// exists so the page has a data path the moment a plugin registers one.
func (c *Core) handleCoreCommands(_ json.RawMessage) (any, *ipc.RPCError) {
	return []commandRow{}, nil
}

// schemaRow is one plugin's declarative settings schema (page control source
// "core:pluginSchemas"), decoded from the manifest's config_schema.
type schemaRow struct {
	Plugin string         `json:"plugin"`
	Title  string         `json:"title"`
	Schema map[string]any `json:"schema"`
}

// handlePluginSchemas serves core.pluginSchemas.
//
// Empty for the same reason core.commands is: manifest loading is not wired
// into the daemon yet (core/pkg/plugin is not on the serving path), so there
// is no config_schema to report. A schema invented here would render a form
// with no write path behind it — a control that looks configurable and
// silently is not.
func (c *Core) handlePluginSchemas(_ json.RawMessage) (any, *ipc.RPCError) {
	return []schemaRow{}, nil
}

// --- safety.ownershipAudit ---

// auditRow is one CrossOS-owned system resource (page control source
// "core:ownershipAudit"). created_at is RFC3339 UTC: the Safety page shows it
// as the moment CrossOS took the resource, and an unparseable local format
// would make that un-sortable.
type auditRow struct {
	Resource  string `json:"resource"`
	ID        string `json:"id"`
	Owner     string `json:"owner"`
	CreatedAt string `json:"created_at"`
}

// handleOwnershipAudit serves safety.ownershipAudit: what CrossOS created on
// this machine, ordered by kind then id so the list does not reshuffle as
// resources come and go. It reports resources the daemon actually acquired or
// wrote — never a list of what it could create, and never OS state outside
// that set (the §3.10 boundary: CrossOS rolls back only what CrossOS made).
func (c *Core) handleOwnershipAudit(_ json.RawMessage) (any, *ipc.RPCError) {
	recs := c.ownedIntegrations()
	out := make([]auditRow, 0, len(recs))
	for _, r := range recs {
		out = append(out, auditRow{
			Resource:  string(r.Type),
			ID:        r.Identifier,
			Owner:     r.PluginID,
			CreatedAt: r.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Resource != out[j].Resource {
			return out[i].Resource < out[j].Resource
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// ownedIntegrations is the audit's source: the resources this process
// acquired, plus the files CrossOS has actually written, discovered on disk.
//
// The files are discovered rather than recorded at adoption time. The daemon
// manages a settings path from startup but writes it only on the user's first
// edit, and `crossos install-autostart` runs as its own process — the daemon
// serving the socket inherited that plist rather than writing it. Listing
// either one before it exists would answer "what did CrossOS create" with a
// path nobody created, and stamp it with a creation time that never happened.
func (c *Core) ownedIntegrations() []safety.IntegrationRecord {
	c.mu.Lock()
	recs := append([]safety.IntegrationRecord(nil), c.owned...)
	c.mu.Unlock()
	if path := c.set.ConfigPath(); path != "" {
		if rec, ok := fileRecord(safety.IntegrationConfig, path,
			"delete the settings file (Reset Everything)"); ok {
			recs = append(recs, rec)
		}
	}
	if path := autostartPath(); path != "" {
		if rec, ok := fileRecord(safety.IntegrationLoginItem, path,
			"crossos uninstall-autostart"); ok {
			recs = append(recs, rec)
		}
	}
	return recs
}

// fileRecord describes a file CrossOS wrote, dated by the file's own mtime.
// A path that does not exist yet is not an owned resource: nothing was
// created, so the audit says so rather than listing a future file.
func fileRecord(kind safety.IntegrationType, path, rollback string) (safety.IntegrationRecord, bool) {
	st, err := os.Stat(path)
	if err != nil {
		return safety.IntegrationRecord{}, false
	}
	return safety.IntegrationRecord{
		PluginID:    "core",
		Type:        kind,
		Identifier:  path,
		StateBefore: "absent",
		Rollback:    rollback,
		CreatedAt:   st.ModTime(),
	}, true
}

// own records one CrossOS-owned resource. Re-acquiring the same resource
// refreshes its record instead of appending a second row: the audit answers
// "what does CrossOS own right now", and a stale duplicate from a re-install
// would name a tap that no longer exists. A resource released by PANIC STOP is
// not un-recorded — the login item survives the kill switch by design, and
// the audit is the list of what Reset Everything would clean.
func (c *Core) own(rec safety.IntegrationRecord) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := range c.owned {
		if c.owned[i].Type == rec.Type && c.owned[i].Identifier == rec.Identifier {
			c.owned[i] = rec
			return
		}
	}
	c.owned = append(c.owned, rec)
}

// --- safety.trialState ---

// trialStateRow is the Safety page countdown (page control source
// "core:trialCountdown").
type trialStateRow struct {
	Plugin      string `json:"plugin"`
	State       string `json:"state"`
	RemainingMS int64  `json:"remaining_ms"`
	TimeoutMS   int64  `json:"timeout_ms"`
}

// handleTrialState serves safety.trialState.
//
// timeout_ms is always safety.TRIALTimeout — the single definition of the
// grace period — so the countdown on screen cannot drift from the rule that
// ends it. With no trial in flight the state is "none" and the timeout is
// still reported, because the page shows the window it WILL offer rather than
// a blank control.
//
// Several plugins can be in trial at once (begin is per plugin); the page has
// one countdown, so it shows the oldest — the one closest to expiring — with
// the plugin ID breaking a tie deterministically.
func (c *Core) handleTrialState(_ json.RawMessage) (any, *ipc.RPCError) {
	timeout := safety.TRIALTimeout.Milliseconds()
	// The trial's fields are COPIED under the lock, never the pointer. Confirm
	// and rollback mutate State and Log in place while holding this same mu
	// (main.go: handleConfirmTrial/handleRollbackTrial), and each IPC call is
	// served on its own goroutine, so the Safety page's poll genuinely runs
	// beside the click. A pointer carried past the unlock would read a field
	// mid-mutation and could name a state for a trial the critical section has
	// already deleted from c.trials.
	var begin time.Time
	var state pluginapi.LifecycleState
	plugin := ""
	found := false
	c.mu.Lock()
	for id, tr := range c.trials {
		if tr == nil {
			continue
		}
		if !found || tr.Begin.Before(begin) || (tr.Begin.Equal(begin) && id < plugin) {
			begin, state, plugin, found = tr.Begin, tr.State, id, true
		}
	}
	c.mu.Unlock()
	if !found {
		return trialStateRow{State: "none", TimeoutMS: timeout}, nil
	}
	// An expired trial nobody rolled back reports zero remaining, not a
	// negative countdown: it is still in TRIAL until confirm or rollback
	// moves it, and confirm on an expired trial fails closed.
	remaining := safety.TRIALTimeout - time.Since(begin)
	if remaining < 0 {
		remaining = 0
	}
	return trialStateRow{
		Plugin:      plugin,
		State:       string(state),
		RemainingMS: remaining.Milliseconds(),
		TimeoutMS:   timeout,
	}, nil
}

// --- core.readiness ---

// readinessRow is one readiness line (page control source "core:readiness").
// It is a report over live state, not a new capability: nothing here can
// change what the daemon does.
type readinessRow struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Ready  bool   `json:"ready"`
	Detail string `json:"detail"`
}

// handleReadiness serves core.readiness: the tap for the keyboard, the
// lifecycle state for the daemon, plugin enablement for the plugins — each row
// with the reason it is not ready, because a bare "not ready" leaves the user
// guessing which permission to grant.
func (c *Core) handleReadiness(_ json.RawMessage) (any, *ipc.RPCError) {
	c.mu.Lock()
	interception := c.interception != nil && c.interception()
	tapErr := c.tapError
	killed := c.killed
	order := append([]string(nil), c.order...)
	enabled := make(map[string]bool, len(c.plugins))
	for id, on := range c.plugins {
		enabled[id] = on
	}
	c.mu.Unlock()
	// Same degraded-reason derivation core.status serves, so the readiness row
	// and the status line can never disagree about a tap that is installed but
	// not working.
	if degraded := tapDegraded(); degraded != "" && tapErr == "" {
		tapErr = degraded
	}

	out := make([]readinessRow, 0, len(order)+2)
	state := c.daemon.State()
	daemonReady := !killed &&
		(state == pluginapi.LifecycleRunning || state == pluginapi.LifecycleSafeMode)
	detail := ""
	switch {
	case killed:
		detail = "PANIC STOP is latched — re-enable from the Safety page"
	case !daemonReady:
		detail = "daemon state: " + string(state)
	}
	out = append(out, readinessRow{ID: "daemon", Label: "Core daemon", Ready: daemonReady, Detail: detail})

	// A not-ready row must say why: with the tap uninstalled there is no
	// recorded failure to quote, and "not ready" alone leaves the user
	// guessing which permission to grant.
	keyboardDetail := tapErr
	if keyboardDetail == "" && !interception {
		keyboardDetail = "keyboard interception is not installed"
	}
	out = append(out, readinessRow{
		ID:     "keyboard",
		Label:  "Keyboard interception",
		Ready:  interception && tapErr == "" && !killed,
		Detail: keyboardDetail,
	})

	// The plugin ID doubles as the label: no manifest is loaded at runtime
	// (see core.pluginSchemas), so there is no display name to read and a
	// prettified guess would be a second source of truth to keep in sync.
	for _, id := range order {
		detail := ""
		switch {
		case killed:
			detail = "plugin actions stopped by PANIC STOP"
		case !enabled[id]:
			detail = "plugin disabled"
		}
		out = append(out, readinessRow{ID: id, Label: id, Ready: enabled[id] && !killed, Detail: detail})
	}
	return out, nil
}

// tapDegraded reports why an installed tap is not doing its job, or "" when
// the keyboard path is healthy. Shared by core.status and core.readiness so
// the two can never tell the user different stories about the same tap: a tap
// macOS keeps timing out reads as "running" from the lifecycle alone while it
// remaps nothing, and a dispatch failure means the key was already swallowed
// before the action failed.
func tapDegraded() string {
	if unhealthyTap() {
		return "keyboard tap keeps timing out — remapping degraded"
	}
	if n := droppedDispatches(); n > 0 {
		return fmt.Sprintf("%d shortcut action(s) could not run — remapping degraded", n)
	}
	return ""
}
