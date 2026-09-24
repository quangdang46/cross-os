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

	"crossos/core/internal/adapter"
	"crossos/core/pkg/event"
	"crossos/core/pkg/intent"
	"crossos/core/pkg/ipc"
	"crossos/core/pkg/keyboard"
	"crossos/core/pkg/pluginapi"
	"crossos/core/pkg/profiles"
	"crossos/core/pkg/safety"
	"crossos/core/pkg/settings"
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

// ruleContexts lists the conditions a rule scopes itself to: the app modes
// first, then the app IDs it names. Both kinds land in one column because both
// answer the same question the Context column is asked — "where does this fire".
//
// App-ID scoping used to be dropped here, on the ground that six identifiers
// per developer row would bury the mode list. It does bury it, and the column
// still lied: a rule that fires only in VSCode, Terminal, Ghostty, WezTerm and
// Finder rendered as a plain "native", which is what a global rule looks like.
// The rule ID is the toggle handle, not a scope report, so it could not stand in
// for the omission either.
func ruleContexts(r event.CompiledRule) []string {
	out := make([]string, 0, len(r.AppModes)+len(r.AppIDs))
	for _, m := range r.AppModes {
		out = append(out, string(m))
	}
	return append(out, r.AppIDs...)
}

// chord renders a rule's physical binding.
func chord(r event.CompiledRule) string { return chordOf(r.KeyCode, r.Modifiers) }

// chordOf renders a keycode + modifier mask as the chord the pages show. The
// rule table is authored in Windows virtual-key codes (core/rules header) and
// the tap translates macOS codes at the boundary, so one rendering describes
// Windows muscle memory on either host — and the trace rows render the same way
// as the matrix, so a recorded chord and the rule that claims it look alike.
func chordOf(keyCode, modifiers uint32) string {
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
		if modifiers&m.bit != 0 {
			parts = append(parts, m.name)
		}
	}
	parts = append(parts, keyName(keyCode))
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
	return c.readinessRows(), nil
}

// finderBundleID is the one app the Finder readiness row reports on. The rules
// that scope themselves to it are read out of the rule table rather than listed
// here: this names the app, the rule set is the table's.
const finderBundleID = "com.apple.Finder"

// readinessRows derives the readiness list. It is a named function because the
// first-run wizard (core.onboardingState) reports the same rows: a wizard that
// counted its own checks could disagree with the checklist the user is looking
// at, and a step that passes while the row beside it is red is a first-run flow
// that lies about its own progress.
func (c *Core) readinessRows() []readinessRow {
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

	out := make([]readinessRow, 0, len(order)+4)
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

	// The three ids the first-run checklist declares (app/backend/onboarding.go
	// lists keyboard, windows, finder) are answered here by name. A checklist
	// item the daemon never reports renders as "Not checked" with no reason
	// attached, so an id the page declared and the daemon ignored reads as
	// "never reported" rather than "not ready" — and on the first-run page that
	// is the difference between fixing a permission and believing you fixed one.
	// keyboard is above; these are the two product surfaces behind it.
	//
	// Neither repeats the keyboard row: that one owns the tap and the kill
	// switch, these two own the plugin toggles, so one broken cause never turns
	// three rows red with the same sentence.
	out = append(out, surfaceRow("windows", "Windows shortcuts", profilePlugins(), enabled, killed))
	out = append(out, surfaceRow("finder", "Finder shortcuts", finderPlugins(), enabled, killed))

	// The profile row sits with the product rows and above the plugin rows it is
	// made of: it answers whether the pick landed, the rows below report the
	// switches. It goes red for the same plugin the windows row names, and that
	// overlap is the point — one row says the switch is off, the other says the
	// profile behind it is not in force, and the windows row keeps its own
	// meaning either way, because a switch is the user's to flip.
	out = append(out, c.profileReadinessRow(enabled, killed))

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
	return out
}

// surfaceRow reports on one product surface: ready when every plugin that
// carries it is switched on. The surfaces are derived from the tables that own
// them (profile bundle, rule table) so a row cannot name a plugin that ships
// no rules, and an empty surface says so rather than reporting a green tick for
// nothing.
func surfaceRow(id, label string, plugins []string, enabled map[string]bool, killed bool) readinessRow {
	row := readinessRow{ID: id, Label: label}
	switch {
	case killed:
		row.Detail = "plugin actions stopped by PANIC STOP"
	case len(plugins) == 0:
		row.Detail = "no rule or profile capability covers this yet"
	default:
		for _, p := range plugins {
			if !enabled[p] {
				row.Detail = p + " is not switched on"
				break
			}
		}
	}
	row.Ready = row.Detail == ""
	return row
}

// profileReadinessRow reports on the profile the user picked: ready only when
// the stored id names a bundle this build ships AND every capability that
// bundle delivers is live. The verdict is capabilityRollup's, the one the
// profile card already draws, so the checklist beside the card cannot say the
// pick landed while the card says it did not.
//
// The detail names which half is unmet — "you never chose" and "you chose and a
// switch is off" are different work, and "not ready" alone leaves the user
// guessing between them. A stored id that names no bundle is a third case: the
// field is free text, so a profile from a build that shipped one this one does
// not can be sitting in the document, and a row that went ready for a bundle it
// cannot enumerate would be green for nothing.
func (c *Core) profileReadinessRow(enabled map[string]bool, killed bool) readinessRow {
	row := readinessRow{ID: "profile", Label: "Profile"}
	if killed {
		row.Detail = "plugin actions stopped by PANIC STOP"
		return row
	}
	active := c.set.ActiveProfile()
	if active == "" {
		row.Detail = "no profile is chosen yet"
		return row
	}
	bundle, ok := profileByID(active)
	if !ok {
		row.Detail = "no profile bundle is named " + strconv.Quote(active)
		return row
	}
	row.Label = bundle.Label
	for _, cap := range bundle.Capabilities {
		if !cap.Available {
			continue // a gap the bundle declares, not a switch the user can flip
		}
		st := c.capabilityRollup(cap, enabled)
		switch {
		case st.Live:
		case !enabled[cap.Plugin]:
			row.Detail = cap.Plugin + " is not switched on"
		default:
			row.Detail = fmt.Sprintf("%s has %d of %d shortcuts switched off",
				cap.Label, st.Total-st.Enabled, st.Total)
		}
		if row.Detail != "" {
			break // the first gap is the one to fix
		}
	}
	row.Ready = row.Detail == ""
	return row
}

// profilePlugins lists the plugins the profile bundle's available capabilities
// name — the Windows experience's implementation, read from profiles.All()
// rather than typed, because the bundle already resolves its plugin IDs against
// rules.BuiltinIDs and a second literal here would be a second thing to rename.
// Sorted: the readiness list is polled, and Go map order would reorder the row.
func profilePlugins() []string {
	seen := map[string]bool{}
	for _, p := range profiles.All() {
		for _, c := range p.Capabilities {
			if c.Available && c.Plugin != "" {
				seen[c.Plugin] = true
			}
		}
	}
	return sortedKeys(seen)
}

// finderPlugins lists the plugins owning a rule scoped to Finder, read from the
// rule table. The developer matrix is the one that scopes itself by bundle ID
// today; a rule that stops naming Finder leaves this surface with its own
// remaining rules rather than with a stale plugin list.
func finderPlugins() []string {
	seen := map[string]bool{}
	for _, r := range builtin.All() {
		for _, app := range r.AppIDs {
			if app == finderBundleID {
				seen[r.PluginID] = true
				break
			}
		}
	}
	return sortedKeys(seen)
}

// sortedKeys is the fixed order for a plugin set read out of a map.
func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
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

// --- core.profiles / core.profileApply ---

// profileRow is one profile card (page control source "core:profiles"). The
// bundle is the profile's own data; what the handler adds is the rollup — which
// of its capabilities are live right now — so the card answers the product
// question ("turn Windows on") without the page joining a second source to find
// out whether the click landed.
type profileRow struct {
	ID           string            `json:"id"`
	Label        string            `json:"label"`
	Description  string            `json:"description"`
	Active       bool              `json:"active"`
	Capabilities []capabilityState `json:"capabilities"`
}

// capabilityState is one capability row plus its rollup.
//
// RuleIDs keeps the declared list, which mixes two vocabularies on purpose: a
// behavior-matrix rule id (something the user can toggle) and a window-action
// zone name (something winlayout resolves). Enabled/Total count only the former,
// because "1 of 21 shortcuts on" for a capability that ships no rules is a
// number that reads as a bug.
type capabilityState struct {
	ID        string   `json:"id"`
	Label     string   `json:"label"`
	Plugin    string   `json:"plugin"`
	Available bool     `json:"available"`
	Reason    string   `json:"reason,omitempty"`
	RuleIDs   []string `json:"rule_ids"`
	Enabled   int      `json:"enabled"`
	Total     int      `json:"total"`
	Live      bool     `json:"live"`
}

// handleCoreProfiles serves core.profiles. The rows are profiles.All() in
// declaration order, and an unavailable capability keeps its reason: a profile
// the spec promises and CrossOS does not deliver is shown as a gap, not dropped
// and not ticked.
func (c *Core) handleCoreProfiles(_ json.RawMessage) (any, *ipc.RPCError) {
	enabled := c.pluginEnableMap()
	active := c.set.ActiveProfile()
	all := profiles.All()
	out := make([]profileRow, 0, len(all))
	for _, p := range all {
		row := profileRow{
			ID: p.ID, Label: p.Label, Description: p.Description,
			Active:       p.ID == active,
			Capabilities: make([]capabilityState, 0, len(p.Capabilities)),
		}
		for _, cap := range p.Capabilities {
			row.Capabilities = append(row.Capabilities, c.capabilityRollup(cap, enabled))
		}
		out = append(out, row)
	}
	return out, nil
}

// capabilityRollup adds the live verdict to one declared capability. Live means
// the two edits this page makes — the plugin toggle and the rule toggles — and
// nothing else: the tap and the kill switch belong to core.readiness, and a
// profile card that reported those would go red for a reason none of its own
// controls can fix.
func (c *Core) capabilityRollup(cap profiles.Capability, enabled map[string]bool) capabilityState {
	st := capabilityState{
		ID: cap.ID, Label: cap.Label, Plugin: cap.Plugin,
		Available: cap.Available, Reason: cap.Reason,
		// Copied, never aliased: the bundle owns the slice, and a page that
		// sorted the wire rows in place would reorder the package's table.
		RuleIDs: append(make([]string, 0, len(cap.RuleIDs)), cap.RuleIDs...),
	}
	st.Live = cap.Available && enabled[cap.Plugin]
	for _, id := range cap.RuleIDs {
		if _, isRule := builtinRule(id); !isRule {
			continue // a window zone, not a matrix rule
		}
		st.Total++
		if c.set.IsRuleEnabled(id) {
			st.Enabled++
		}
	}
	st.Live = st.Live && st.Enabled == st.Total
	return st
}

// handleProfileApply serves core.profileApply: {"profile":"<id>"} — the
// one-click profile card. The bundle's available capabilities are projected
// into ONE settings.Plan and handed to Store.ApplyBatch, so a profile activates
// atomically: the same all-or-nothing contract the store keeps for a hand-built
// plan, and no path on which the card leaves half a dozen edits behind.
//
// The store's active-profile field is free text, so the catalog of what may be
// applied is this handler's job: an id the bundle does not declare is refused
// before the store sees it. Malformed input is a bad-params error, the same
// shape config.setOverride returns.
func (c *Core) handleProfileApply(raw json.RawMessage) (any, *ipc.RPCError) {
	var p struct {
		Profile string `json:"profile"`
	}
	if err := json.Unmarshal(raw, &p); err != nil || strings.TrimSpace(p.Profile) == "" {
		return nil, &ipc.RPCError{Code: ipc.ErrBadParams, Message: `need {"profile":"<profile id>"}`}
	}
	bundle, ok := profileByID(p.Profile)
	if !ok {
		return nil, &ipc.RPCError{Code: ipc.ErrInvalid,
			Message: "unknown profile " + strconv.Quote(p.Profile)}
	}
	plan := planForProfile(bundle)
	if err := c.set.ApplyBatch(plan, settings.Catalog{
		Rules:   c.knownRuleIDs,
		Plugins: c.pluginKnown,
	}); err != nil {
		return nil, &ipc.RPCError{Code: ipc.ErrInvalid, Message: err.Error()}
	}
	// The store persists; the RUNNING router reads c.plugins (decideLocked
	// recompiles from it), so the plan's plugin verdicts are applied there too.
	// A profile that saved to disk and left the daemon in its old state is the
	// half-activated outcome ApplyBatch exists to prevent, one layer up.
	c.mu.Lock()
	for _, t := range plan.Plugins {
		c.plugins[t.ID] = t.Enabled
	}
	c.mu.Unlock()
	return map[string]any{
		"profile": bundle.ID,
		"rules":   len(plan.Rules),
		"plugins": len(plan.Plugins),
	}, nil
}

// profileByID resolves a profile the bundle declares.
func profileByID(id string) (profiles.Profile, bool) {
	for _, p := range profiles.All() {
		if p.ID == id {
			return p, true
		}
	}
	return profiles.Profile{}, false
}

// planForProfile projects a bundle into the plan the store applies: the
// behavior-matrix rules it names switched on, the plugins behind them switched
// on, and the profile recorded as the active one. A window zone in RuleIDs is
// not a rule and is left out — ApplyBatch validates every id against the
// daemon's table, so sending one would reject the whole profile over a zone the
// user cannot toggle. Ids are deduplicated because three capabilities sharing
// one plugin is one edit, not three.
func planForProfile(p profiles.Profile) settings.Plan {
	id := p.ID
	plan := settings.Plan{ActiveProfile: &id}
	seenPlugin, seenRule := map[string]bool{}, map[string]bool{}
	for _, cap := range p.Capabilities {
		if !cap.Available {
			continue
		}
		if cap.Plugin != "" && !seenPlugin[cap.Plugin] {
			seenPlugin[cap.Plugin] = true
			plan.Plugins = append(plan.Plugins, settings.Toggle{ID: cap.Plugin, Enabled: true})
		}
		for _, ruleID := range cap.RuleIDs {
			if _, isRule := builtinRule(ruleID); !isRule || seenRule[ruleID] {
				continue
			}
			seenRule[ruleID] = true
			plan.Rules = append(plan.Rules, settings.Toggle{ID: ruleID, Enabled: true})
		}
	}
	return plan
}

// pluginEnableMap snapshots the enable set. Caller holds no lock.
func (c *Core) pluginEnableMap() map[string]bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make(map[string]bool, len(c.plugins))
	for id, on := range c.plugins {
		out[id] = on
	}
	return out
}

// pluginKnown reports whether a plugin ID is registered with this daemon: the
// catalog ApplyBatch validates a plan's plugin verdicts against. A verdict for a
// plugin this process does not have is a row no page can act on.
func (c *Core) pluginKnown(id string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.plugins[id]
	return ok
}

// --- core.conflicts ---

// conflictRow is one contested chord (page control source "core:conflicts" —
// the `conflicts` field both shortcut-list controls declare). Winner and Losers
// are rule.Resolve's own verdict, so the editor's dialog cannot disagree with
// the decision path: the rule it offers to keep is the rule that actually loses.
type conflictRow struct {
	Keys   string          `json:"keys"`
	Winner string          `json:"winner"`
	Losers []string        `json:"losers"`
	Rules  []conflictClaim `json:"rules"`
}

// conflictClaim is one rule claiming the chord, in rank order (winner first).
type conflictClaim struct {
	RuleID string `json:"rule_id"`
	Plugin string `json:"plugin"`
	Action string `json:"action"`
}

// handleConflicts serves core.conflicts: chords two firing rules both claim.
//
// Only rules that still fire can contest a chord. decideLocked compiles the
// router from the enabled plugins' rules the user has left on, so a switched-off
// rule — or one behind a disabled plugin — shadows nothing, and reporting it
// would hand the user a conflict they do not have. The persisted window-shortcut
// table cannot contribute one either: winlayout.ValidateShortcuts refuses a
// duplicate chord, so the table the Windows page edits is conflict-free by
// construction and the rule table is the only place a collision can form.
func (c *Core) handleConflicts(_ json.RawMessage) (any, *ipc.RPCError) {
	enabled := c.pluginEnableMap()
	var firing []event.CompiledRule
	for _, r := range builtin.All() {
		if enabled[r.PluginID] && c.set.IsRuleEnabled(r.RuleID) {
			firing = append(firing, r)
		}
	}
	// User rules take part: they are compiled into the decision path, so a
	// collision that only exists once a person authored a rule is the common
	// case, not the exotic one. Leaving them out reported "no conflicts" for
	// exactly the collisions the editor exists to prevent.
	if userTable := c.userRules.table(); userTable != nil {
		firing = append(firing, userTable...)
	}
	return c.chordConflicts(firing), nil
}

// chordConflicts groups rules by the chord they claim and, for every contested
// group, asks the real router who wins. The previous version called
// rule.Resolve with Enabled/AppModeOK/RequiresOK hardcoded true on every
// candidate, which ranked rules whose app scopes can never both match and
// then reported that verdict as the outcome — the comment claimed the dialog
// could not disagree with the decision path, and it could: two rules on one
// chord with disjoint scopes were resolved as if a context existed where both
// fire, while the router would pick a different winner (or none) per context.
//
// Resolving through event.Router per context is slower and a little more
// code, but it is the only version whose "winner" is the same word the
// keyboard uses.
func (c *Core) chordConflicts(rules []event.CompiledRule) []conflictRow {
	groups := map[string][]event.CompiledRule{}
	for _, r := range rules {
		keys := chord(r)
		groups[keys] = append(groups[keys], r)
	}
	out := make([]conflictRow, 0, len(groups))
	for keys, group := range groups {
		if len(group) < 2 {
			continue
		}
		claim := map[string]conflictClaim{}
		for _, r := range group {
			claim[r.RuleID] = conflictClaim{
				RuleID: r.RuleID, Plugin: r.PluginID, Action: matrixAction(r),
			}
		}
		if row, contested := c.resolveConflict(keys, group, claim); contested {
			out = append(out, row)
		}
	}
	// Sorted by the chord the row shows: this list is polled, and Go map order
	// would reshuffle it between refreshes.
	sort.SliceStable(out, func(i, j int) bool { return out[i].Keys < out[j].Keys })
	return out
}

// --- core.traces ---

// traceRowLimit bounds the served list. The recorder is append-only and holds
// every decision the session made, so an unbounded list would serialize a whole
// session's keystrokes on every poll; the tail is the part a trace page reads.
const traceRowLimit = 200

// traceRow is one recorded decision as columns (page control source
// "core:traces"). core.eventLogs flattens these same traces into
// "winner action params=…" strings; the stages, the physical event, the focused
// app and the rules that lost are all still in the recorded trace, so a page
// asking for them gets fields instead of a sentence to parse.
type traceRow struct {
	At       string       `json:"at"`
	Decision string       `json:"decision"`
	Event    traceEvent   `json:"event"`
	Context  traceContext `json:"context"`
	Winner   string       `json:"winner"`
	Losers   []string     `json:"losers"`
	Intent   string       `json:"intent"`
	Action   string       `json:"action"`
	Stages   []traceStage `json:"stages"`
	Params   string       `json:"params"`
}

// traceEvent is the physical input: the chord rendered the way the matrix
// renders a rule's binding, so a recorded key and the rule that claims it read
// alike on one screen.
type traceEvent struct {
	Keys    string `json:"keys"`
	Source  string `json:"source"`
	Device  string `json:"device,omitempty"`
	KeyCode uint32 `json:"key_code"`
}

// traceContext is the cached decision context (event.FastContext) — never an AX
// query, which is the contract that keeps this path off the keyboard.
type traceContext struct {
	AppID    string `json:"app_id"`
	AppMode  string `json:"app_mode"`
	WindowID string `json:"window_id,omitempty"`
	WinClass string `json:"win_class,omitempty"`
}

// traceStage is one pipeline step the router logged.
type traceStage struct {
	Stage  string `json:"stage"`
	Detail string `json:"detail"`
}

// handleTraces serves core.traces in the order the recorder stored it — oldest
// first, so a polling page does not reshuffle under the reader — truncated from
// the old end once the list passes traceRowLimit.
//
// Params are the recorder's OWN redacted bytes (record.redactParams runs at
// record time), passed through as rendered: this handler re-reads nothing
// pre-redaction, which is why a trace page can show parameters at all.
func (c *Core) handleTraces(_ json.RawMessage) (any, *ipc.RPCError) {
	trs := c.rec.Traces()
	if len(trs) > traceRowLimit {
		trs = trs[len(trs)-traceRowLimit:]
	}
	out := make([]traceRow, 0, len(trs))
	for _, tr := range trs {
		row := traceRow{
			At:       tr.At.UTC().Format(time.RFC3339),
			Decision: decisionName(tr.Decision),
			Event: traceEvent{
				Keys:    chordOf(tr.Event.KeyCode, tr.Event.Modifiers),
				Source:  string(tr.Event.Source),
				Device:  tr.Event.DeviceID,
				KeyCode: tr.Event.KeyCode,
			},
			Context: traceContext{
				AppID:    tr.Context.AppID,
				AppMode:  string(tr.Context.AppMode),
				WindowID: tr.Context.WindowID,
				WinClass: tr.Context.WinClass,
			},
			Winner: tr.Winner,
			Losers: append(make([]string, 0, len(tr.Losers)), tr.Losers...),
			Intent: tr.IntentID,
			Action: tr.Action,
			Params: string(tr.Params),
			Stages: make([]traceStage, 0, len(tr.Stages)),
		}
		for _, s := range tr.Stages {
			row.Stages = append(row.Stages, traceStage{Stage: s.Stage, Detail: s.Detail})
		}
		out = append(out, row)
	}
	return out, nil
}

// --- core.pluginMeta ---

// pluginMetaRow is one registered plugin's manifest facts (page control source
// "core:pluginMeta").
type pluginMetaRow struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Permissions []string `json:"permissions"`
	Loaded      bool     `json:"loaded"`
	Reason      string   `json:"reason,omitempty"`
}

// handlePluginMeta serves core.pluginMeta in registration order — the same
// order plugin.list uses, so the two pages cannot disagree about which plugin
// is first.
//
// Name and version come from a manifest, and no manifest is loaded: the builtin
// plugins are compiled straight from core/rules and nothing constructs a
// pluginapi.Registry at runtime. So those two fields are empty and every row
// says why, rather than the daemon prettifying a plugin id into a display name
// and stamping a version it guessed. Permissions are not a guess: they are the
// grants the router authorizes this plugin's rules against, read from the same
// table the decision path uses.
func (c *Core) handlePluginMeta(_ json.RawMessage) (any, *ipc.RPCError) {
	c.mu.Lock()
	order := append([]string(nil), c.order...)
	c.mu.Unlock()
	grants := builtin.Grants()
	out := make([]pluginMetaRow, 0, len(order))
	for _, id := range order {
		perms := make([]string, 0, len(grants[id]))
		for _, p := range grants[id] {
			perms = append(perms, string(p))
		}
		out = append(out, pluginMetaRow{
			ID:          id,
			Permissions: perms,
			Loaded:      false,
			Reason:      "no manifest is loaded — the builtin plugins are compiled from the rule table",
		})
	}
	return out, nil
}

// --- core.onboardingState ---

// onboardingRow is the first-run wizard's state (page control source
// "core:onboardingState"): the persisted done flag, the step the user is on,
// and the readiness rollup the last step verifies against.
type onboardingRow struct {
	Completed   bool             `json:"completed"`
	CurrentStep string           `json:"current_step"`
	Steps       []onboardingStep `json:"steps"`
	Readiness   []readinessRow   `json:"readiness"`
	Ready       int              `json:"ready"`
	Total       int              `json:"total"`
}

// onboardingStep is one step with its verdict. Detail is the reason it is not
// done, so the wizard can say what to do instead of only that it is waiting.
type onboardingStep struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Done   bool   `json:"done"`
	Detail string `json:"detail,omitempty"`
}

// onboardingSteps is the flow the first-run page declares: Welcome → Enable per
// plugin → Open System Settings → Verify ready. The labels are spelled here
// because the daemon cannot import the shell module; the ids are the contract,
// and a page that shows its own copy renders these verdicts beside its own
// words.
var onboardingSteps = []struct{ id, label string }{
	{"welcome", "Welcome"},
	{"enable", "Enable per plugin"},
	{"settings", "Open System Settings"},
	{"verify", "Verify ready"},
}

// handleOnboardingState serves core.onboardingState.
//
// The steps are DERIVED from live state, never stored as a cursor: a step
// counter that disagreed with the readiness rows would send the user to fix
// something that is already fine. Welcome has no state of its own to observe —
// it is inferred as passed once any later step is done, and the persisted
// onboarding flag overrides all four, because the user saying "I am finished"
// is the one thing here the daemon cannot re-derive.
//
// The System Settings step reads the keyboard readiness row: the Accessibility
// grant IS that step, and having the tap's own derivation in two places is how
// the two would drift.
func (c *Core) handleOnboardingState(_ json.RawMessage) (any, *ipc.RPCError) {
	rows := c.readinessRows()
	completed := c.set.OnboardingComplete()

	ready, keyboardReady, allReady := 0, false, true
	for _, r := range rows {
		if r.Ready {
			ready++
		} else {
			allReady = false
		}
		if r.ID == "keyboard" && r.Ready {
			keyboardReady = true
		}
	}
	keyboardDetail := ""
	for _, r := range rows {
		if r.ID == "keyboard" {
			keyboardDetail = r.Detail
		}
	}
	anyPlugin := false
	for _, on := range c.pluginEnableMap() {
		if on {
			anyPlugin = true
			break
		}
	}

	done := map[string]bool{
		"welcome":  anyPlugin || keyboardReady || allReady,
		"enable":   anyPlugin,
		"settings": keyboardReady,
		"verify":   allReady,
	}
	detail := map[string]string{
		"enable":   "no plugin is switched on yet",
		"settings": keyboardDetail,
		"verify":   fmt.Sprintf("%d of %d checks are not ready", len(rows)-ready, len(rows)),
	}

	out := onboardingRow{
		Completed: completed,
		Steps:     make([]onboardingStep, 0, len(onboardingSteps)),
		Readiness: rows,
		Ready:     ready,
		Total:     len(rows),
	}
	out.CurrentStep = "done"
	for _, s := range onboardingSteps {
		step := onboardingStep{ID: s.id, Label: s.label, Done: completed || done[s.id]}
		if !step.Done {
			step.Detail = detail[s.id]
			if out.CurrentStep == "done" {
				out.CurrentStep = s.id
			}
		}
		out.Steps = append(out.Steps, step)
	}
	if completed {
		out.CurrentStep = "done"
	}
	return out, nil
}

// --- core.apps ---

// handleApps serves the installed applications the rule editor's "IF App = …"
// picker offers. adapter.ListApps is the only enumeration in the tree, so the
// rows come from the platform seam rather than a table kept here — a list
// assembled in the daemon would drift from what the machine actually runs.
//
// The shell reached this call before it had a handler and degraded to an
// empty picker, so the rows are also asserted against the same field names
// app/backend reads.
func (c *Core) handleApps(_ json.RawMessage) (any, *ipc.RPCError) {
	apps, err := adapter.ListApps(nil)
	if err != nil {
		return nil, &ipc.RPCError{Code: ipc.ErrInternal, Message: "list apps: " + err.Error()}
	}
	type appRow struct {
		BundleID    string
		Executable  string
		PID         int
		DisplayName string
		AppMode     string
		Category    string
	}
	// Sorted by display name with the bundle id as tiebreak, so the picker
	// does not reshuffle between polls.
	sort.Slice(apps, func(i, j int) bool {
		if apps[i].DisplayName != apps[j].DisplayName {
			return apps[i].DisplayName < apps[j].DisplayName
		}
		return apps[i].BundleID < apps[j].BundleID
	})
	out := make([]appRow, 0, len(apps))
	for _, a := range apps {
		out = append(out, appRow{
			BundleID:    a.BundleID,
			Executable:  a.Executable,
			PID:         a.PID,
			DisplayName: a.DisplayName,
			AppMode:     string(a.AppMode),
			Category:    string(a.Category),
		})
	}
	return out, nil
}

// resolveConflict asks the real router who wins this chord in a given
// context, and reports the row only when the answer is not the same
// everywhere. A group whose winner is identical under every context is not a
// conflict the user has to act on — the ranking settles it — while a group
// that flips with the focused app is exactly what the editor exists to show.
//
// The router is handed the group alone, so a decision here cannot be changed
// by a rule on some other chord.
func (c *Core) resolveConflict(keys string, group []event.CompiledRule, claim map[string]conflictClaim) (conflictRow, bool) {
	// Two rules on one chord is a conflict whoever wins: the loser is dropped
	// silently, which is the thing the row exists to show. The winner is
	// whatever the real router picks, and a chord whose answer changes with
	// the focused app is flagged so the row is not read as a fixed ranking.
	contexts := conflictContexts(group)
	byWinner := map[string]bool{}
	var primary string
	for i, ctx := range contexts {
		rt := event.Compile(group, intent.DefaultRegistry(), c.grantSet(), nil)
		out := rt.Decide(event.Event{
			Type:      event.EventKeyDown,
			Source:    event.SourceKeyboard,
			KeyCode:   firstKeyCode(group),
			Modifiers: firstModifiers(group),
		}, ctx)
		byWinner[out.WinnerRule] = true
		if i == 0 {
			primary = out.WinnerRule
		}
	}
	// The first context produced no winner (the group cannot co-match
	// anywhere the rules declare). Nothing is being dropped there, so there
	// is no conflict to report.
	if primary == "" {
		return conflictRow{}, false
	}
	row := conflictRow{Keys: keys, Winner: primary, Losers: make([]string, 0, len(group)-1)}
	row.Rules = append(row.Rules, claim[primary])
	for _, r := range group {
		if r.RuleID != primary {
			row.Losers = append(row.Losers, r.RuleID)
			row.Rules = append(row.Rules, claim[r.RuleID])
		}
	}
	sort.Strings(row.Losers)
	return row, true
}

// conflictContexts is the set of focused-app contexts a chord group can differ
// in: one per declared app id, plus the app-less context for a rule scoped to
// no app. Deriving them from the rules keeps the check finite without
// inventing contexts nobody declared.
func conflictContexts(group []event.CompiledRule) []event.FastContext {
	seen := map[string]bool{}
	var out []event.FastContext
	add := func(appID string) {
		if seen[appID] {
			return
		}
		seen[appID] = true
		out = append(out, event.FastContext{AppID: appID, AppMode: event.AppModeNative})
	}
	for _, r := range group {
		if len(r.AppIDs) == 0 {
			add("")
			continue
		}
		for _, id := range r.AppIDs {
			add(id)
		}
	}
	return out
}

// firstKeyCode and firstModifiers are the chord the group was grouped by, so
// every member agrees on them. The probe event has to carry the modifiers too:
// without them nothing in the group matches and the conflict reads as
// "no winner" — a false all-clear.
func firstKeyCode(group []event.CompiledRule) uint32 {
	if len(group) == 0 {
		return 0
	}
	return group[0].KeyCode
}

func firstModifiers(group []event.CompiledRule) uint32 {
	if len(group) == 0 {
		return 0
	}
	return group[0].Modifiers
}

// grantSet is the permission set the router authorizes against, matching what
// decideLocked compiles with — a conflict resolved under different grants
// would be a different answer than the keyboard gives.
func (c *Core) grantSet() map[string][]intent.Permission {
	grants := builtin.Grants()
	if user := c.userRules.grants(); user != nil {
		for pluginID, perms := range user {
			grants[pluginID] = append(grants[pluginID], perms...)
		}
	}
	return grants
}
