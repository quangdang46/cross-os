// Command crossos is the CrossOS core daemon entrypoint.
//
// Bead: cross-os-325 (serve half; scaffold was cross-os-ymh.2). Lifecycle
// mechanism lives in package daemon; TRIAL/rollback semantics live in
// package safety. This file wires: signal block → IPC server on the
// platform socket path with the five shell methods (matching
// app/backend/ipc_client.go) → builtin plugin matrices compiled into the
// event router + recorder tap.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"crossos/core/internal/adapter"
	"crossos/core/pkg/config"
	"crossos/core/pkg/daemon"
	"crossos/core/pkg/event"
	"crossos/core/pkg/intent"
	"crossos/core/pkg/ipc"
	"crossos/core/pkg/observe"
	"crossos/core/pkg/pluginapi"
	"crossos/core/pkg/record"
	"crossos/core/pkg/safety"
	"crossos/core/pkg/settings"
	"crossos/core/pkg/update"
	"crossos/core/pkg/userrules"
	"crossos/core/pkg/winlayout"
	builtin "crossos/core/rules"
)

// CurrentVersion is the running daemon's version, compared against update
// manifests by core.checkForUpdate. Bump on release (release.yml tags v*);
// the manifest Version uses the same "vM.m.p" vocabulary.
const CurrentVersion = "v0.1.0"

// DefaultSocketPath is the Unix-socket path the daemon serves and the shell
// dials (mirrors the FinderSync daemonSocketPath convention:
// ~/Library/Application Support/CrossOS/crossos.sock on macOS).
func DefaultSocketPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.TempDir()
	}
	return filepath.Join(home, "Library", "Application Support", "CrossOS", "crossos.sock")
}

// Core owns the daemon state behind the IPC methods. One mutex guards the
// plugin enable map + trial map (handlers run on independent connection
// goroutines). The settings Store owns its own lock internally.
type Core struct {
	mu     sync.Mutex
	daemon *daemon.Daemon
	router *event.Router
	rec    *record.Recorder
	set    *settings.Store
	// userRules is the rule builder's handler set (userules.go). Held as
	// the service, not the raw store, so the four user-rule methods cost one
	// field and four table entries instead of four of each.
	userRules *userRuleService
	// menuOff is the Explorer's per-item toggle (finderdata.go): a menu id
	// the user has switched off. Absence means on, so an item added to
	// findermenu.Menu is in the menu until a person turns it off — the daemon
	// ships what the product declares and the user narrows it, not the reverse.
	menuOff map[string]bool
	plugins map[string]bool // id → enabled
	trials  map[string]*safety.Trial
	order   []string
	// interception reports whether the live keyboard tap is installed and
	// running (bead cross-os-2io). Injected as a func so the daemon serves a
	// truthful on/off without the core package knowing about cgo.
	interception func() bool
	// tapError is the last tap install failure (usually TCC denial), served
	// over IPC so the UI can tell the user exactly what to fix.
	tapError string
	// killed latches on PANIC STOP: Decide passes everything through, so no
	// rule can consume or replace a key while the kill switch is active.
	killed bool
	// tapStopped records that the tap was torn down by the kill switch.
	tapStopped bool
	// stopTapFn tears down the tap installed by startTap.
	stopTapFn func()
	// windowCache is the watcher-maintained focused-window snapshot that
	// window actions read, so no action ever runs a blocking AX query.
	windowCache *appCache
	// owned is the ledger behind safety.ownershipAudit: the system resources
	// this daemon created, recorded at the moment of creation (see
	// pagedata.go). Together with the files the daemon has written it is the
	// §3.10 rollback scope — the audit names nothing else, because CrossOS
	// never rolls back state it did not make.
	// switcher is the Alt+Tab switcher's daemon state: the open session, the
	// unread triggers, and the platform window seam. Built here rather than
	// on first use so core.windows answers before any chord has been
	// pressed — a source that only appears once the gesture has fired cannot
	// render the list the gesture is about to choose from.
	switcher *switcherService
	owned    []safety.IntegrationRecord
}

// NewCore builds a Core with the daemon running and builtin matrices loaded.
// rules/grants are injected so tests bind fixtures without plugin modules
// (production passes loadBuiltin below). settingsPath "" = memory-only
// store (tests); production passes the config.json path for persistence.
func NewCore(rules []event.CompiledRule, grants map[string][]intent.Permission) (*Core, error) {
	return NewCoreWithSettings(rules, grants, "")
}

// NewCoreWithSettings is NewCore with an explicit settings persistence path.
func NewCoreWithSettings(rules []event.CompiledRule, grants map[string][]intent.Permission, settingsPath string) (*Core, error) {
	d := daemon.New()
	if err := d.Run(); err != nil {
		return nil, err
	}
	reg := intent.DefaultRegistry()
	set, err := settings.New(settingsPath)
	if err != nil {
		return nil, err
	}
	ruleStore, err := userrules.New(userRulesPath(settingsPath))
	if err != nil {
		return nil, err
	}
	c := &Core{
		daemon:    d,
		switcher:  newSwitcherService(adapter.NewWindowLister(&adapter.Driver{Log: daemonTapLog{}})),
		router:    event.Compile(rules, reg, grants, nil),
		rec:       record.NewRecorder(),
		set:       set,
		userRules: newUserRuleService(ruleStore),
		menuOff:   map[string]bool{},
		plugins:   map[string]bool{},
		trials:    map[string]*safety.Trial{},
	}
	return c, nil
}

// enableLocked flips one plugin; caller holds mu. Unknown IDs are an error
// (never a silent no-op toggle).
func (c *Core) enableLocked(id string, enabled bool) error {
	if _, ok := c.plugins[id]; !ok {
		return fmt.Errorf("core: unknown plugin %q", id)
	}
	c.plugins[id] = enabled
	return nil
}

// registerBuiltin records one builtin plugin's enable state (called once per
// plugin at startup; matrices arrive pre-compiled via NewCore).
func (c *Core) registerBuiltin(id string, enabled bool) {
	if _, ok := c.plugins[id]; !ok {
		c.order = append(c.order, id)
	}
	c.plugins[id] = enabled
}

// loadPluginEnablement seeds the running enable map from the settings
// document, so a restart comes up with the verdicts the user left rather
// than re-enabling everything. An id the document does not name is OFF:
// the document is the whole of the user's intent, and defaulting a missing
// id to true is how a plugin someone switched off came back after a
// crash-loop respawn.
func (c *Core) loadPluginEnablement() {
	enabled := map[string]bool{}
	for _, id := range c.set.PluginsEnabled() {
		enabled[id] = true
	}
	for _, id := range builtin.BuiltinIDs {
		c.registerBuiltin(id, enabled[id])
	}
}

// handleStatus serves core.status: running/safe_mode/killed plus live
// interception state so the UI can say "remapping on/off" truthfully
// (bead cross-os-2io).
func (c *Core) handleStatus(_ json.RawMessage) (any, *ipc.RPCError) {
	st := c.daemon.State()
	c.mu.Lock()
	interception := c.interception != nil && c.interception()
	tapErr := c.tapError
	c.mu.Unlock()
	// A tap that keeps timing out is disabled by macOS; say so instead of
	// reporting a healthy-looking "running" that no longer remaps anything.
	// A dropped action is reported for the same reason: the key was already
	// suppressed, so this is the ONLY way the user learns it did nothing.
	// Shared with core.readiness (tapDegraded) so the two never disagree.
	if degraded := tapDegraded(); degraded != "" && tapErr == "" {
		tapErr = degraded
	}
	return map[string]any{
		"running":      st == pluginapi.LifecycleRunning || st == pluginapi.LifecycleSafeMode,
		"safe_mode":    st == pluginapi.LifecycleSafeMode,
		"killed":       st == pluginapi.LifecycleStopped || c.killedState(),
		"interception": interception,
		"tap_error":    tapErr,
		// Served so the shell's About block shows the real version instead of
		// a literal that drifts at release time.
		"version": CurrentVersion,
	}, nil
}

// handlePluginList serves plugin.list: [{id,enabled,healthy,origin}].
func (c *Core) handlePluginList(_ json.RawMessage) (any, *ipc.RPCError) {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]map[string]any, 0, len(c.order))
	for _, id := range c.order {
		// "disabled" is one of the three words PluginState documents and the
		// one this handler never sent, so a row for a switched-off plugin
		// carried a healthy chip beside the word Disabled and contradicted
		// itself. The trial gate reads this same field (TrialControl sends it
		// back as the healthy= claim), and a trial on a plugin that is off
		// should fail closed — so the two uses agree rather than one of them
		// being decoration.
		//
		// "degraded" is still unreachable while no Level-B supervisor runs and
		// no plugin is a child process. That is honest: there is nothing to
		// degrade. The builtins are compiled into this binary from the rule
		// table, so they cannot crash and cannot be health-checked either, and
		// origin says so where a person can act on it.
		health := "healthy"
		if !c.plugins[id] {
			health = "disabled"
		}
		out = append(out, map[string]any{
			"id": id, "enabled": c.plugins[id], "healthy": health, "origin": "builtin",
		})
	}
	return out, nil
}

// handlePluginSetEnabled serves plugin.setEnabled: {id, enabled}. The
// verdict is written to the settings document as well as the running map:
// a toggle that evaporates on restart is the one thing the Extensions page
// promises it will not do.
func (c *Core) handlePluginSetEnabled(raw json.RawMessage) (any, *ipc.RPCError) {
	var p struct {
		ID      string `json:"id"`
		Enabled bool   `json:"enabled"`
	}
	if err := json.Unmarshal(raw, &p); err != nil || p.ID == "" {
		return nil, &ipc.RPCError{Code: ipc.ErrBadParams, Message: "need {id, enabled}"}
	}
	c.mu.Lock()
	err := c.enableLocked(p.ID, p.Enabled)
	c.mu.Unlock()
	if err != nil {
		return nil, &ipc.RPCError{Code: ipc.ErrInvalid, Message: err.Error()}
	}
	// Outside mu: pluginKnown takes the same lock, and a write that
	// cannot reach disk must not leave the daemon and the document
	// disagreeing — so a failed write puts the running map back and
	// reports why the toggle did not take.
	if err := c.set.SetPluginEnabled(p.ID, p.Enabled, c.pluginKnown); err != nil {
		c.mu.Lock()
		_ = c.enableLocked(p.ID, !p.Enabled)
		c.mu.Unlock()
		return nil, &ipc.RPCError{Code: ipc.ErrInternal, Message: err.Error()}
	}
	return map[string]any{"id": p.ID, "enabled": p.Enabled}, nil
}

// decideLocked runs one key event through the router + recorder tap and
// returns the decision. Disabled plugins' rules never fire, and neither do
// user-disabled matrix rows or rules the focused app has overridden off: the
// router is rebuilt from enabled-only rules on every toggle (10 rules —
// recompile is microseconds, and correctness beats caching here). Caller holds
// no lock; this takes mu to snapshot the enable set.
func (c *Core) decideLocked(ev event.Event, ctx event.FastContext) event.Outcome {
	// Which phase of a key may be acted on is the RULE's declaration now, not
	// a blanket drop here: a rule that says nothing claims key-down only, and
	// a rule that declares EventKeyUp claims the release. Dropping every
	// non-key-down event before matching is what made the release half of a
	// gesture unreachable — the reason Alt+Tab could not commit on the chord
	// the user actually let go of.
	c.mu.Lock()
	enabled := map[string]bool{}
	for id, on := range c.plugins {
		enabled[id] = on
	}
	c.mu.Unlock()
	known := map[string]bool{}
	var all []event.CompiledRule
	// User rules are real rules: the rule builder saves them, the editor
	// lists them, and the decision path has to compile them alongside the
	// builtin table. Reading only builtin.All() left every stored rule
	// inert — saving one looked right in the UI and did nothing on the
	// keyboard, which is the worst possible shape for a feature.
	grants := builtin.Grants()
	userTable := c.userRules.table()
	if userTable != nil {
		all = append(all, userTable...)
		for pluginID, perms := range c.userRules.grants() {
			grants[pluginID] = append(grants[pluginID], perms...)
		}
	}
	for _, r := range builtin.All() {
		known[r.RuleID] = true
		if !enabled[r.PluginID] {
			continue
		}
		// A stored per-app verdict SHADOWS the global toggle: absent, the
		// global decides; present, the focused app's own opinion wins. This is
		// the read that makes config.setOverride a real edit rather than a
		// row in config.json — the Keyboard page promises the toggle takes
		// effect immediately. Plugin enablement above is NOT shadowable: an
		// override is a verdict about a rule, not a permission to run it.
		fires := c.set.IsRuleEnabled(r.RuleID)
		if on, ok := c.set.Override(ctx.AppID, r.RuleID); ok {
			fires = on
		}
		if fires {
			all = append(all, r)
		}
	}
	c.mu.Lock()
	killed := c.killed
	c.mu.Unlock()
	rt := event.Compile(all, intent.DefaultRegistry(), grants, func() bool { return killed })
	out := rt.Decide(ev, ctx)
	c.rec.OnOutcome(out)
	return out
}

// knownRuleIDs reports the builtin RuleIDs (for settings toggle validation).
func (c *Core) knownRuleIDs(ruleID string) bool {
	for _, r := range builtin.All() {
		if r.RuleID == ruleID {
			return true
		}
	}
	return false
}

// handleSetRuleEnabled serves config.setRuleEnabled: {ruleId, enabled}.
// Unknown RuleIDs fail closed (never a silent no-op toggle).
func (c *Core) handleSetRuleEnabled(raw json.RawMessage) (any, *ipc.RPCError) {
	var p struct {
		RuleID  string `json:"ruleId"`
		Enabled bool   `json:"enabled"`
	}
	if err := json.Unmarshal(raw, &p); err != nil || p.RuleID == "" {
		return nil, &ipc.RPCError{Code: ipc.ErrBadParams, Message: "need {ruleId, enabled}"}
	}
	if err := c.set.SetRuleEnabled(p.RuleID, p.Enabled, c.knownRuleIDs); err != nil {
		return nil, &ipc.RPCError{Code: ipc.ErrInvalid, Message: err.Error()}
	}
	return map[string]any{"ruleId": p.RuleID, "enabled": p.Enabled}, nil
}

// handleGetShortcuts serves config.getShortcuts: the current shortcut table
// (for the Windows page editor).
func (c *Core) handleGetShortcuts(_ json.RawMessage) (any, *ipc.RPCError) {
	return c.shortcutRows(), nil
}

// shortcutRows renders the table with action names (not runes).
func (c *Core) shortcutRows() []map[string]any {
	rows := make([]map[string]any, 0)
	for _, s := range c.set.Shortcuts() {
		rows = append(rows, map[string]any{
			"action": winlayout.ActionName(s.Action), "modifiers": s.Modifiers, "key": s.Key,
		})
	}
	return rows
}

// handleSetShortcuts serves config.setShortcuts: [{action, modifiers, key}]
// validated by winlayout (duplicate chords + non-window capabilities
// rejected before anything is stored or applied).
func (c *Core) handleSetShortcuts(raw json.RawMessage) (any, *ipc.RPCError) {
	var p struct {
		Shortcuts []struct {
			Action    string   `json:"action"`
			Modifiers []string `json:"modifiers"`
			Key       string   `json:"key"`
		} `json:"shortcuts"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, &ipc.RPCError{Code: ipc.ErrBadParams, Message: "need {shortcuts:[{action,modifiers,key}]}"}
	}
	set := make([]winlayout.Shortcut, 0, len(p.Shortcuts))
	for _, s := range p.Shortcuts {
		a, ok := winlayout.ActionByName(s.Action)
		if !ok {
			return nil, &ipc.RPCError{Code: ipc.ErrBadParams, Message: fmt.Sprintf("unknown action %q", s.Action)}
		}
		set = append(set, winlayout.Shortcut{Action: a, Modifiers: s.Modifiers, Key: s.Key})
	}
	if err := c.set.SetShortcuts(set); err != nil {
		return nil, &ipc.RPCError{Code: ipc.ErrInvalid, Message: err.Error()}
	}
	return map[string]any{"shortcuts": len(set)}, nil
}

// handleReset serves core.reset: audited plan steps (safety.PlanReset).
func (c *Core) handleReset(_ json.RawMessage) (any, *ipc.RPCError) {
	plan := safety.PlanReset(safety.IntegrationState{})
	steps := []string{}
	if plan.RemoveLoginItem {
		steps = append(steps, "remove login item")
	}
	if plan.DisableExtension {
		steps = append(steps, "disable extension")
	}
	if plan.CleanOwnedState {
		steps = append(steps, "clean owned state")
	}
	if plan.VerifyNoProcess {
		steps = append(steps, "verify no process remains")
	}
	return steps, nil
}

// handleEventLogs serves core.eventLogs: sanitized display lines.
func (c *Core) handleEventLogs(_ json.RawMessage) (any, *ipc.RPCError) {
	return observe.SanitizeForDisplay(c.rec.Traces()), nil
}

// handleSetObserve serves core.setObserve: {"enabled":true|false} — the
// Observe toggle on the Activity page (UX-05). The flag is the recorder's
// dry-run: record.go then writes a "dry-run: Would-execute <capability>"
// stage onto every trace an action would have produced, which is how a user
// reads back what their rules do without disturbing the desktop.
//
// Required, not defaulted: a payload missing the flag must not be read as
// "stop observing" (or "start") — the toggle the user pressed and the
// toggle that took effect have to be the same one.
func (c *Core) handleSetObserve(raw json.RawMessage) (any, *ipc.RPCError) {
	var p struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.Unmarshal(raw, &p); err != nil || p.Enabled == nil {
		return nil, &ipc.RPCError{Code: ipc.ErrBadParams, Message: `need {"enabled":true|false}`}
	}
	c.rec.SetDryRun(*p.Enabled)
	return map[string]any{"observe": *p.Enabled}, nil
}

// handleObserveState serves core.observeState: what the recorder is ACTUALLY
// doing, read back from the recorder rather than echoed from the last thing
// the shell asked for. handleSetObserve can only be written, and a write with
// no read is a fire-and-forget — a toggle that cannot say which position it is
// in can only ever be labelled from the click that produced it, which is the
// belief, not the state.
//
// The mode is reported and NOT set here. It changes what the recorder keeps,
// and who may change that is a policy question with its own answer; a read
// that also took a mode would be a second, quieter way to change it.
func (c *Core) handleObserveState(json.RawMessage) (any, *ipc.RPCError) {
	return map[string]any{
		"observe": c.rec.DryRun(),
		"mode":    c.rec.Mode().String(),
	}, nil
}

// accessibilityPane is the one system-settings deep link this daemon can
// act on: the focused-window watcher cannot prime without Accessibility
// consent, so every not-ready keyboard row points the user here.
const accessibilityPane = "x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility"

// openSettingsFn is the seam the deep link goes through. Tests swap it to
// capture the URL, so asserting the handler never opens System Settings on
// the machine running the suite.
var openSettingsFn = openSystemSettings

// handleOpenSettings serves permissions.openSettings. There is exactly one
// pane to open, so the method takes no pane: a caller that names a
// different one is told so rather than quietly handed Accessibility,
// which would send someone to a screen that cannot fix their problem.
func (c *Core) handleOpenSettings(raw json.RawMessage) (any, *ipc.RPCError) {
	if len(raw) > 0 {
		var p struct {
			Pane string `json:"pane"`
		}
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, &ipc.RPCError{Code: ipc.ErrBadParams, Message: "need {} or {\"pane\":\"accessibility\"}"}
		}
		if pane := strings.TrimSpace(p.Pane); pane != "" && pane != "accessibility" {
			return nil, &ipc.RPCError{
				Code:    ipc.ErrInvalid,
				Message: fmt.Sprintf("core: unknown settings pane %q (this daemon opens only accessibility)", pane),
			}
		}
	}
	if runtime.GOOS != "darwin" {
		return nil, &ipc.RPCError{
			Code:    ipc.ErrInvalid,
			Message: "core: the settings deep link is macOS-only; grant accessibility consent in your OS settings",
		}
	}
	if err := openSettingsFn(accessibilityPane); err != nil {
		return nil, &ipc.RPCError{Code: ipc.ErrInternal, Message: err.Error()}
	}
	return map[string]any{"pane": "accessibility", "opened": true}, nil
}

// openSystemSettings hands a URL to Launch Services. `open` is the only
// door to a settings pane that needs no extra entitlement of ours.
func openSystemSettings(url string) error { return launch("open", url) }

// handleKeyEvent serves core.keyEvent: one synthetic key event through the
// decision path (router + recorder tap), for shell diagnostics, Activity
// trace seeding, and headless e2e. Params: {keyCode, modifiers, appId,
// appMode ("native"|"terminal"|"remote"|"vm"|"excluded"), windowId}.
// type defaults to keydown; keyup never suppresses (matches spike A/B
// key-down-only suppression). This is a DIAGNOSTIC path — the live tap
// callback calls DecideOne directly, never over IPC (fast path stays
// synchronous, <1ms). Result: {decision, intent, winner}.
func (c *Core) handleKeyEvent(raw json.RawMessage) (any, *ipc.RPCError) {
	var p struct {
		KeyCode   uint32 `json:"keyCode"`
		Modifiers uint32 `json:"modifiers"`
		Type      string `json:"type"`
		AppID     string `json:"appId"`
		AppMode   string `json:"appMode"`
		WindowID  string `json:"windowId"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, &ipc.RPCError{Code: ipc.ErrBadParams, Message: "need {keyCode, ...}"}
	}
	typ := event.EventKeyDown
	switch p.Type {
	case "", "keydown":
	case "keyup":
		typ = event.EventKeyUp
	case "flags":
		typ = event.EventFlagsChanged
	default:
		return nil, &ipc.RPCError{Code: ipc.ErrBadParams, Message: "unknown type (keydown|keyup|flags)"}
	}
	mode := event.AppMode(p.AppMode)
	switch mode {
	case "", event.AppModeNative:
		mode = event.AppModeNative
	case event.AppModeTerminal, event.AppModeRemote, event.AppModeVM, event.AppModeExcluded:
	default:
		return nil, &ipc.RPCError{Code: ipc.ErrBadParams, Message: "unknown appMode"}
	}
	out := c.decideLocked(
		event.Event{Type: typ, Source: event.SourceKeyboard, KeyCode: p.KeyCode, Modifiers: p.Modifiers},
		event.FastContext{AppID: p.AppID, AppMode: mode, WindowID: p.WindowID},
	)
	return map[string]any{
		"decision": decisionName(out.Decision),
		"intent":   out.Intent.ID,
		"winner":   out.WinnerRule,
	}, nil
}

// decisionName names a Decision for the diagnostic result (the canonical
// vocabulary is PASS/CONSUME/REPLACE — never a bare number over IPC).
func decisionName(d pluginapi.Decision) string {
	switch d {
	case pluginapi.DecisionConsume:
		return "CONSUME"
	case pluginapi.DecisionReplace:
		return "REPLACE"
	default:
		return "PASS"
	}
}

// httpFetcher wires update.Fetcher to net/http with a short timeout.
// Finder menu handlers must return fast; the update check is off-path, but
// a hanging dial would still wedge the caller — deadline it like ipc.go.
type httpFetcher struct{ client *http.Client }

func (f httpFetcher) Fetch(url string) ([]byte, error) {
	resp, err := f.client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("update: fetch %s: status %s", url, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 256<<20))
}

func defaultFetcher() httpFetcher {
	return httpFetcher{client: &http.Client{Timeout: 30 * time.Second}}
}

// handleCheckForUpdate serves core.checkForUpdate: {manifest:{version,
// platform, url, sha256}} → {updateAvailable, version}. Malformed versions
// are typed errors (update package fail-closed), never a silent false.
func (c *Core) handleCheckForUpdate(raw json.RawMessage) (any, *ipc.RPCError) {
	var p struct {
		Manifest update.Manifest `json:"manifest"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, &ipc.RPCError{Code: ipc.ErrBadParams, Message: "need {manifest:{version,platform,url,sha256}}"}
	}
	avail, err := update.CheckForUpdate(CurrentVersion, p.Manifest)
	if err != nil {
		return nil, &ipc.RPCError{Code: ipc.ErrBadParams, Message: err.Error()}
	}
	return map[string]any{"updateAvailable": avail, "version": p.Manifest.Version}, nil
}

// handleApplyUpdate serves core.applyUpdate: download + checksum-verify +
// approval-gated atomic install over the RUNNING binary (os.Executable).
// Params: {manifest:{...}, approved, approvedBy}. Unapproved → typed error
// (§8.4: never silent). Checksum mismatch → typed error, bytes never
// installed. Success restarts via launchd KeepAlive (daemon exits 0 after
// install; launchd relaunches the fresh binary).
func (c *Core) handleApplyUpdate(raw json.RawMessage) (any, *ipc.RPCError) {
	var p struct {
		Manifest   update.Manifest `json:"manifest"`
		Approved   bool            `json:"approved"`
		ApprovedBy string          `json:"approvedBy"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, &ipc.RPCError{Code: ipc.ErrBadParams, Message: "need {manifest, approved}"}
	}
	if strings.TrimSpace(p.Manifest.URL) == "" {
		return nil, &ipc.RPCError{Code: ipc.ErrBadParams, Message: "manifest needs a download URL"}
	}
	data, err := update.Download(defaultFetcher(), p.Manifest)
	if err != nil {
		return nil, &ipc.RPCError{Code: ipc.ErrInternal, Message: err.Error()}
	}
	self, err := os.Executable()
	if err != nil {
		return nil, &ipc.RPCError{Code: ipc.ErrInternal, Message: "self path: " + err.Error()}
	}
	if err := update.Install(data, self, update.Approval{Granted: p.Approved, By: p.ApprovedBy}); err != nil {
		return nil, &ipc.RPCError{Code: ipc.ErrInvalid, Message: err.Error()}
	}
	return map[string]any{"installed": p.Manifest.Version}, nil
}

// handlePanicStop serves safety.panicStop: kill switch (safety.PanicStop).
// Stops interception + plugin actions, keeps the login item (reversible
// via Re-enable). Result names what stopped — never a silent kill.
func (c *Core) handlePanicStop(_ json.RawMessage) (any, *ipc.RPCError) {
	res := safety.PanicStop()
	// Actually stop. safety.PanicStop describes the outcome; it cannot
	// perform it, and reporting success while the tap keeps swallowing keys
	// is the worst possible lie here. The kill flag makes Decide pass every
	// key through, and the tap is torn down so nothing is left intercepting.
	c.mu.Lock()
	c.killed = true
	stop := c.stopTapFn
	c.mu.Unlock()
	// Latch it. A crash-loop respawn would otherwise re-arm the tap and
	// undo the user's emergency stop without a word.
	if err := c.set.SetPanicStopped(true); err != nil {
		fmt.Fprintln(os.Stderr, "crossos: could not persist the kill switch:", err)
	}
	// Report the real reason interception is off. Leaving the previous
	// "consent missing" text in place would send the user to a permission
	// screen for a problem they already solved.
	c.mu.Lock()
	c.tapError = "interception stopped by PANIC STOP — re-enable from the Safety page"
	c.mu.Unlock()
	if stop != nil {
		stop()
		c.mu.Lock()
		c.tapStopped = true
		c.mu.Unlock()
	}
	return map[string]any{
		"interceptionDisabled": true,
		"pluginActionsStopped": res.PluginActionsStopped,
		"buffersFlushed":       res.BuffersFlushed,
		"loginItemKept":        res.LoginItemKept,
	}, nil
}

// handleResume clears a latched PANIC STOP and re-installs the tap. Without
// it the latch is a one-way door: status tells the user to re-enable from
// the Safety page and no such control exists.
func (c *Core) handleResume(_ json.RawMessage) (any, *ipc.RPCError) {
	if err := c.set.SetPanicStopped(false); err != nil {
		return nil, &ipc.RPCError{Code: ipc.ErrInternal, Message: err.Error()}
	}
	c.mu.Lock()
	c.killed = false
	c.tapError = ""
	stop := c.stopTapFn
	c.stopTapFn = nil
	c.mu.Unlock()
	if stop != nil {
		stop() // release the old stop func so it is not called twice
	}
	// startTap installs a fresh watcher + tap; replace the stop func.
	stop = c.startTap()
	c.mu.Lock()
	c.mu.Unlock()
	if !c.killedState() {
		return map[string]any{"resumed": true, "interception": tapLiveStatus()}, nil
	}
	return map[string]any{"resumed": true, "interception": false,
		"note": "tap still unavailable: " + c.tapErrorText()}, nil
}

// tapErrorText reads the last tap failure for reporting.
func (c *Core) tapErrorText() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.tapError
}

// handleBeginTrial serves safety.beginTrial: {pluginId} → TRIAL (idempotent:
// repeat calls return the in-flight trial, never a second one). The Safety
// page countdown + confirm/rollback buttons drive this.
func (c *Core) handleBeginTrial(raw json.RawMessage) (any, *ipc.RPCError) {
	var p struct {
		PluginID string `json:"pluginId"`
	}
	if err := json.Unmarshal(raw, &p); err != nil || p.PluginID == "" {
		return nil, &ipc.RPCError{Code: ipc.ErrBadParams, Message: "need {pluginId}"}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.plugins[p.PluginID]; !ok {
		return nil, &ipc.RPCError{Code: ipc.ErrInvalid, Message: fmt.Sprintf("core: unknown plugin %q", p.PluginID)}
	}
	if _, ok := c.trials[p.PluginID]; ok {
		return map[string]any{"pluginId": p.PluginID, "state": "trial"}, nil
	}
	tr, err := safety.BeginTrial(p.PluginID, nil)
	if err != nil {
		return nil, &ipc.RPCError{Code: ipc.ErrInternal, Message: err.Error()}
	}
	c.trials[p.PluginID] = tr
	return map[string]any{"pluginId": p.PluginID, "state": "trial"}, nil
}

// handleConfirmTrial serves safety.confirmTrial: {pluginId, confirmed,
// healthy} → ENABLED. confirmed=false or healthy=false fails closed
// (mirrors safety.Trial.Confirm — no auto-approve path, ever).
func (c *Core) handleConfirmTrial(raw json.RawMessage) (any, *ipc.RPCError) {
	var p struct {
		PluginID  string `json:"pluginId"`
		Confirmed bool   `json:"confirmed"`
		Healthy   bool   `json:"healthy"`
	}
	if err := json.Unmarshal(raw, &p); err != nil || p.PluginID == "" {
		return nil, &ipc.RPCError{Code: ipc.ErrBadParams, Message: "need {pluginId, confirmed, healthy}"}
	}
	c.mu.Lock()
	tr, ok := c.trials[p.PluginID]
	c.mu.Unlock()
	if !ok || tr == nil {
		return nil, &ipc.RPCError{Code: ipc.ErrInvalid, Message: fmt.Sprintf("core: no trial for plugin %q (begin trial first)", p.PluginID)}
	}
	if err := tr.Confirm(p.Confirmed, p.Healthy); err != nil {
		return nil, &ipc.RPCError{Code: ipc.ErrInvalid, Message: err.Error()}
	}
	// Persist BEFORE the running map flips, and outside mu for the
	// same reason plugin.setEnabled does: a confirmed trial that enabled
	// the plugin in memory but not on disk comes back disabled on the
	// next start, with the user told the opposite.
	if err := c.set.SetPluginEnabled(p.PluginID, true, c.pluginKnown); err != nil {
		return nil, &ipc.RPCError{Code: ipc.ErrInternal, Message: err.Error()}
	}
	c.mu.Lock()
	delete(c.trials, p.PluginID)
	c.plugins[p.PluginID] = true
	c.mu.Unlock()
	return map[string]any{"pluginId": p.PluginID, "state": "enabled"}, nil
}

// handleRollbackTrial serves safety.rollbackTrial: {pluginId, reason} →
// DISABLED (user cancel, timeout, kill-switch). Explicit, never surprising.
func (c *Core) handleRollbackTrial(raw json.RawMessage) (any, *ipc.RPCError) {
	var p struct {
		PluginID string `json:"pluginId"`
		Reason   string `json:"reason"`
	}
	if err := json.Unmarshal(raw, &p); err != nil || p.PluginID == "" {
		return nil, &ipc.RPCError{Code: ipc.ErrBadParams, Message: "need {pluginId}"}
	}
	c.mu.Lock()
	tr, ok := c.trials[p.PluginID]
	c.mu.Unlock()
	if !ok || tr == nil {
		return nil, &ipc.RPCError{Code: ipc.ErrInvalid, Message: fmt.Sprintf("core: no trial for plugin %q", p.PluginID)}
	}
	reason := p.Reason
	if reason == "" {
		reason = "user rollback"
	}
	if err := tr.Abort(reason); err != nil {
		return nil, &ipc.RPCError{Code: ipc.ErrInvalid, Message: err.Error()}
	}
	// Same contract as the confirm path: the verdict reaches the
	// document before the running map moves, or a rolled-back plugin
	// reappears on the next start.
	if err := c.set.SetPluginEnabled(p.PluginID, false, c.pluginKnown); err != nil {
		return nil, &ipc.RPCError{Code: ipc.ErrInternal, Message: err.Error()}
	}
	c.mu.Lock()
	delete(c.trials, p.PluginID)
	c.plugins[p.PluginID] = false
	c.mu.Unlock()
	return map[string]any{"pluginId": p.PluginID, "state": "disabled"}, nil
}

// Serve registers the shell methods and serves ln in the background,
// returning the server (Close stops the accept loop: Close closes the
// listener, Accept errors, and Serve returns via the closed channel).
// main blocks on signals instead — Serve must never block its caller or
// tests cannot drive it.
func (c *Core) Serve(ln net.Listener) *ipc.Server {
	srv := ipc.NewServer()
	for name, h := range c.methods() {
		if err := srv.Register(name, h); err != nil {
			panic("core: duplicate IPC method " + name)
		}
	}
	go srv.Serve(ln)
	return srv
}

// methods is the single IPC method table. It is a named function, not an
// inline literal, so the test can assert that every method the shell calls
// is actually registered: a handler written but left out of this map is
// dead code that only direct-call tests can see.
func (c *Core) methods() map[string]ipc.Handler {
	m := map[string]ipc.Handler{
		"core.status":           c.handleStatus,
		"plugin.list":           c.handlePluginList,
		"plugin.setEnabled":     c.handlePluginSetEnabled,
		"core.keyEvent":         c.handleKeyEvent,
		"core.reset":            c.handleReset,
		"core.eventLogs":        c.handleEventLogs,
		"safety.panicStop":      c.handlePanicStop,
		"safety.resume":         c.handleResume,
		"safety.beginTrial":     c.handleBeginTrial,
		"safety.confirmTrial":   c.handleConfirmTrial,
		"safety.rollbackTrial":  c.handleRollbackTrial,
		"core.checkForUpdate":   c.handleCheckForUpdate,
		"core.applyUpdate":      c.handleApplyUpdate,
		"config.setRuleEnabled": c.handleSetRuleEnabled,
		"config.getShortcuts":   c.handleGetShortcuts,
		"config.setShortcuts":   c.handleSetShortcuts,
		// The settings-page data sources (cross-os-jzj). One method per page
		// control `source` string; see pagedata.go.
		"config.getMatrix":      c.handleGetMatrix,
		"config.getOverrides":   c.handleGetOverrides,
		"config.setOverride":    c.handleSetOverride,
		"config.getZones":       c.handleGetZones,
		"config.setZones":       c.handleSetZones,
		"core.commands":         c.handleCoreCommands,
		"core.pluginSchemas":    c.handlePluginSchemas,
		"safety.ownershipAudit": c.handleOwnershipAudit,
		"safety.trialState":     c.handleTrialState,
		"core.readiness":        c.handleReadiness,
		// The second wave: the profile layer (w2-pagedata), the conflict
		// verdict, structured traces, plugin manifest facts, the first-run
		// wizard's derived state.
		"core.profiles":     c.handleCoreProfiles,
		"core.profileApply": c.handleProfileApply,
		"core.conflicts":    c.handleConflicts,
		"core.traces":       c.handleTraces,
		// The erase behind the Observe page's clear button. Registered beside
		// the read rather than on its own because a recorder that keeps every
		// keystroke decision this session and cannot be emptied is a list
		// nobody can get rid of — see pagedata.go's handleTracesClear.
		"core.tracesClear":     c.handleTracesClear,
		"core.pluginMeta":      c.handlePluginMeta,
		"core.onboardingState": c.handleOnboardingState,
		// The wizard's one write, beside its read: the derived steps cannot
		// record that the user is finished, so the flag gets its own method.
		"core.onboardingComplete": c.handleOnboardingComplete,
		"core.apps":               c.handleApps,
		// The Alt+Tab switcher (ws-3): the list the switcher page draws, the
		// bounded long-poll it waits on, and the focus a commit performs.
		"core.windows":       c.handleWindows,
		"core.switcherWait":  c.handleSwitcherWait,
		"core.switcherFocus": c.handleSwitcherFocus,
		// The user-rule table (w2-userrules). The handlers live on
		// userRuleService in userules.go, so these four entries are
		// the whole of that file's publishing.
		"config.getUserRules":          c.userRules.getUserRules,
		"config.setUserRule":           c.userRules.setUserRule,
		"config.deleteUserRule":        c.userRules.deleteUserRule,
		"config.getUserRuleVocabulary": c.userRules.getUserRuleVocabulary,
		// Observe mode (UX-05) and the deep link that fixes the one
		// permission the daemon can actually ask for: Accessibility,
		// which the focused-window watcher needs before it can prime.
		"core.setObserve":          c.handleSetObserve,
		"core.observeState":        c.handleObserveState,
		"permissions.openSettings": c.handleOpenSettings,
		// The Explorer page (be-finderdata): the Finder menu the user can
		// switch items off in, and the file-type catalog behind New >.
		// Two reads, three writes, and the writes answer with the whole
		// table they changed so the page never has to assume its own edit
		// landed.
		"core.finderMenu":         c.handleFinderMenu,
		"core.fileTypes":          c.handleFileTypes,
		"core.setFileType":        c.handleSetFileType,
		"core.reorderFileTypes":   c.handleReorderFileTypes,
		"core.setMenuItemEnabled": c.handleSetMenuItemEnabled,
	}
	// The appex's own seven menu verbs, published as the set be-finderverbs
	// builds rather than eight entries spelled out here: the names are a
	// protocol, and copying them into this map is how a typo becomes a menu
	// verb the appex can call and the daemon does not answer.
	for name, h := range finderMenuHandlers(c) {
		m[name] = h
	}
	return m
}

// socketLockFile holds the process-lifetime lock; deliberately never closed.
var socketLockFile *os.File

// errLockHeld means another live process owns the lock file. It is distinct
// from a lock that could not be taken at all, so the caller can tell "stop
// the leftover daemon" from "this platform cannot lock" instead of blaming a
// running daemon for a failure that never involved one. The lock itself is
// per-GOOS: lock_unix.go (flock), lock_windows.go (LockFileEx),
// lock_other.go (refuses).
var errLockHeld = errors.New("lock file is held by another process")

// errLockUnsupported is what lock_other.go returns on the platforms with no
// advisory file locking at all. The daemon refuses to start there: serving
// without the lock is what lets a second daemon steal a live socket.
var errLockUnsupported = fmt.Errorf("advisory file locking is not implemented on %s", runtime.GOOS)

// lockHolder reads the pid the owning daemon recorded in the lock file.
// Returns "" when nobody recorded one.
func lockHolder(lockPath string) string {
	raw, err := os.ReadFile(lockPath)
	if err != nil {
		return ""
	}
	pid := strings.TrimSpace(string(raw))
	if pid == "" {
		return ""
	}
	return " (pid " + pid + ")"
}

// lockHolderCmd renders the stop command for the recorded pid.
func lockHolderCmd(holder, sockPath string) string {
	if i := strings.Index(holder, "pid "); i >= 0 {
		rest := holder[i+4:]
		if j := strings.Index(rest, ")"); j >= 0 {
			return rest[:j]
		}
	}
	return "the leftover process holding " + sockPath + ".lock"
}

// listenSocket binds the Unix socket (0600 dir per ipc doc.go security note),
// removing a stale socket file first.
func listenSocket(path string) (net.Listener, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	// An exclusive lock, not just a probe. Probing and then binding is a
	// TOCTOU window: two daemons starting together both see "dead" and both
	// bind, and a live-but-slow daemon reads as dead. Hold the lock for the
	// process lifetime so the check and the bind cannot be interleaved.
	lockPath := path + ".lock"
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := lockExclusive(lockFile); err != nil {
		lockFile.Close()
		if !errors.Is(err, errLockHeld) {
			return nil, fmt.Errorf("cannot take the daemon lock on %s: %w", lockPath, err)
		}
		holder := lockHolder(lockPath)
		return nil, fmt.Errorf("another crossos daemon holds %s%s — it is not serving, so it is a leftover. Stop it with: kill %s",
			lockPath, holder, lockHolderCmd(holder, lockPath))
	}
	// Record the owner so a later starter can name the process to stop
	// instead of reporting an anonymous "already running".
	fmt.Fprintf(lockFile, "%d\n", os.Getpid())
	// The lock is intentionally leaked for the process lifetime: closing the
	// fd would release it and let a second daemon in.
	socketLockFile = lockFile

	if socketAlive(path) {
		return nil, fmt.Errorf("another crossos daemon is already serving on %s — stop it first (scripts/run.sh reuses a running one)", path)
	}
	// Stale path only (a crashed daemon left the file behind): safe to rebind.
	_ = os.Remove(path)
	return net.Listen("unix", path)
}

func main() {
	// launchd invokes `crossos serve`; bare `crossos` also serves (dev
	// convenience). The autostart pair manages the login item.
	switch {
	case len(os.Args) == 2 && os.Args[1] == "install-autostart":
		if err := installAutostart(); err != nil {
			fmt.Fprintln(os.Stderr, "crossos: install-autostart:", err)
			os.Exit(1)
		}
		return
	case len(os.Args) == 2 && os.Args[1] == "uninstall-autostart":
		if err := uninstallAutostart(); err != nil {
			fmt.Fprintln(os.Stderr, "crossos: uninstall-autostart:", err)
			os.Exit(1)
		}
		return
	case len(os.Args) > 2 || (len(os.Args) == 2 && os.Args[1] != "serve"):
		fmt.Fprintln(os.Stderr, "usage: crossos [serve|install-autostart|uninstall-autostart]")
		os.Exit(2)
	}
	c, err := NewCoreWithSettings(builtin.All(), builtin.Grants(), config.DefaultConfigPath())
	if err != nil {
		fmt.Fprintln(os.Stderr, "crossos: init:", err)
		os.Exit(1)
	}
	// Builtin plugins: matrices compiled into the router via core/rules
	// (data port of the plugin matrices — core cannot import the plugin
	// modules back; parity pinned by core/rules TestBuiltinParity).
	// loadPluginEnablement seeds the enable map from the document the
	// user last wrote, so a restart resumes the Extensions page as it was
	// left rather than switching every plugin back on.
	c.loadPluginEnablement()
	// Live keyboard tap (bead cross-os-2io). Best-effort: a TCC denial is
	// NOT fatal — the daemon still serves IPC and core.status reports
	// interception=off with the reason, so the user can grant consent and
	// see exactly what failed. The decide entry binds the LIVE decision path
	// (decideLocked), so config.setRuleEnabled and plugin toggles take effect
	// on the running tap instead of a start-up snapshot.
	// Own the socket BEFORE touching the keyboard. A process that cannot
	// serve must never install a tap: on a crash-loop respawn that printed
	// "interception live" and then exited, leaving launchd to respawn
	// forever and the log full of misleading lines.
	path := DefaultSocketPath()
	ln, err := listenSocket(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "crossos: listen:", err)
		return
	}
	srv := c.Serve(ln)
	defer srv.Close()
	fmt.Println("crossos: serving on", path)

	stopTap := c.startTap()
	defer stopTap()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	fmt.Println("crossos: shutting down")
}

// --- live tap wiring (bead cross-os-2io) ---

// tapStartFn is the seam the daemon installs through. Tests swap it to
// assert the daemon ACTUALLY attempts an install (so deleting the startTap
// call from main fails the suite) without needing TCC consent.
// Per-platform defaults live in tap_darwin.go / tap_other.go.
var tapStartFn = tapStartLive
var tapStopFn = tapStopLive
var tapLiveStatus = tapLiveLive

// startTap installs the live tap, bound to the live decision path, and
// returns a stop func for shutdown. It never returns an error to main: a
// denial is recorded on the Core and surfaced over IPC.
func (c *Core) startTap() func() {
	// Focused-app cache for the tap's decision path (cross-os-heu). Seeded
	// before install so the very first keystroke has a context, then kept
	// fresh by a watcher off the hot path.
	cache := &appCache{}
	cache.set(event.FastContext{AppMode: event.AppModeNative})
	// The watcher lives until the daemon stops. A defer here would fire when
	// startTap RETURNS — microseconds in — and freeze the cache at its seed
	// value, which makes every app-scoped rule permanently unreachable.
	watchStop := make(chan struct{})
	go c.watchFocusedApp(watchStop, cache)

	c.mu.Lock()
	c.windowCache = cache
	c.mu.Unlock()

	d := &adapter.Driver{
		// Live path: honours plugin enable + rule toggles on EVERY key.
		Decide:   c.decideLocked,
		Dispatch: c.dispatch,
		Context:  cache.get,
		Log:      daemonTapLog{},
	}
	// A persisted PANIC STOP outranks everything: do not touch the keyboard.
	if c.set.PanicStopped() {
		c.mu.Lock()
		c.killed = true
		c.interception = func() bool { return false }
		c.tapError = "interception stopped by PANIC STOP (re-enable from the Safety page)"
		c.mu.Unlock()
		fmt.Println("crossos: PANIC STOP is latched — not installing the keyboard tap")
		return func() {}
	}
	err := tapStartFn(d)
	c.mu.Lock()
	c.interception = tapLiveStatus
	if err != nil {
		c.tapError = err.Error()
		c.interception = func() bool { return false }
		fmt.Fprintln(os.Stderr, "crossos: keyboard interception unavailable:", err)
	} else {
		c.tapError = ""
		fmt.Println("crossos: keyboard interception live")
	}
	c.mu.Unlock()
	if err == nil {
		// The tap is a real OS resource (an event tap, or a low-level hook on
		// Windows) that exists only because CrossOS installed it, so it
		// belongs in the ownership ledger. The process is its identity: there
		// is no other handle to name.
		c.own(safety.IntegrationRecord{
			PluginID:    "core",
			Type:        safety.IntegrationAccessTap,
			Identifier:  fmt.Sprintf("pid:%d", os.Getpid()),
			StateBefore: "no keyboard interception",
			Rollback:    "safety.panicStop stops interception and removes the tap",
			CreatedAt:   time.Now(),
		})
	}

	var stopOnce sync.Once
	stop := func() {
		// Idempotent: PANIC STOP stops the tap, and the deferred stop runs
		// again at shutdown. Closing an already-closed channel panics, and a
		// panic on the way out of a kill switch is the last thing anyone
		// needs.
		stopOnce.Do(func() {
			// Stop the watcher BEFORE the tap: no more key events can
			// arrive once the tap is gone, so nothing left to keep current.
			close(watchStop)
			_ = tapStopFn(d)
		})
	}
	c.mu.Lock()
	c.stopTapFn = stop
	c.mu.Unlock()
	return stop
}

// daemonTapLog forwards adapter stage logs to the daemon's stdout so a
// tap failure is visible without the UI.
type daemonTapLog struct{}

func (daemonTapLog) Log(stage, msg string) {
	fmt.Fprintf(os.Stderr, "crossos: [%s] %s\n", stage, msg)
}

// dispatch executes an authorized capability through the adapter seam
// (bead cross-os-vx9). The tap calls it between Decide and suppress, so a
// key is only swallowed when its action actually ran.
//
// The darwin window seam still answers ErrPermissionDenied until the AX
// bridge lands (bead qhp AX) — that is CORRECT behaviour for now: the
// original key passes through and the user keeps a working keyboard.
func (c *Core) dispatch(req intent.Request) error {
	switch req.Capability.ID {
	case "window.move", "window.minimize", "window.maximize", "window.close":
		return c.windowDispatch(req, c.windowCache)
	case "window.switcher":
		return c.applySwitcher(req)
	case "clipboard.copyPath":
		return copyPaths(req.Intent.Parameters)
	case "filesystem.createFile":
		return createFile(req.Intent.Parameters)
	case "filesystem.createFolder":
		return createFolder(req.Intent.Parameters)
	case "file.moveToTrash":
		return moveToTrash(req.Intent.Parameters)
	case "app.open":
		return openTarget(req.Intent.Parameters)
	case "terminal.openAt":
		return openTerminalAt(req.Intent.Parameters)
	default:
		return fmt.Errorf("core: no adapter for capability %q (not implemented yet)", req.Capability.ID)
	}
}

// windowDispatch routes a window.* capability to the platform seam.
func (c *Core) windowDispatch(req intent.Request, cache *appCache) error {
	var zone string
	if len(req.Intent.Parameters) > 0 {
		var p struct {
			Zone string `json:"zone"`
		}
		if err := json.Unmarshal(req.Intent.Parameters, &p); err == nil {
			zone = p.Zone
		}
	}
	// Read the CACHED focused window. A live AX query here would block for
	// the life of the process when accessibility consent is missing, and it
	// runs on the single dispatch worker — the first window action would
	// wedge every later action.
	if cache == nil {
		return fmt.Errorf("core: no focused-window cache (the tap is not running)")
	}
	fw, ok := cache.window()
	if !ok {
		return fmt.Errorf("core: no focused window known yet (accessibility consent missing, or the watcher has not primed)")
	}
	wq := adapter.NewWindowQuery(&adapter.Driver{Log: daemonTapLog{}})
	visible := winlayout.Rect{X: fw.X, Y: fw.Y, W: fw.W, H: fw.H}
	m := adapter.MoveResize{ID: fw.ID}
	switch req.Capability.ID {
	case "window.move":
		r, ok := winlayout.FrameForZone(zone, visible)
		if !ok {
			return fmt.Errorf("core: window zone %q has no adapter geometry yet", zone)
		}
		m = adapter.MoveResize{ID: fw.ID, X: r.X, Y: r.Y, W: r.W, H: r.H, Move: true, Resize: true}
	case "window.minimize", "window.maximize", "window.close":
		// Geometry actions need a real AX action (not yet bridged); report
		// honestly instead of pretending.
		return fmt.Errorf("core: %s not implemented in the adapter yet", req.Capability.ID)
	}
	return wq.MoveResize(m)
}

// --- Explorer-pack capabilities ---
//
// The Finder context menu is the only surface that knows which files the
// user acted on, so these capabilities read their paths out of the intent
// parameters the caller supplies. A rule fired from the keyboard carries
// none — or carries the rule table's unresolved {finderDir} template — and
// is refused by name rather than acted on a guess. The error travels back
// through the dispatch worker, which is the same channel that already lets
// a failed window action pass the original key through.

// dispatchParams decodes a capability's parameters. An absent or empty
// object is the valid "takes no parameters" case, so a nil map only ever
// means the request itself was unreadable.
func dispatchParams(raw json.RawMessage) (map[string]json.RawMessage, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var p map[string]json.RawMessage
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("core: capability parameters must be a JSON object: %w", err)
	}
	return p, nil
}

// stringParam reads one required, non-blank string parameter.
func stringParam(params map[string]json.RawMessage, name string) (string, error) {
	raw, ok := params[name]
	if !ok {
		return "", fmt.Errorf("core: missing %q", name)
	}
	var v string
	if err := json.Unmarshal(raw, &v); err != nil {
		return "", fmt.Errorf("core: %q must be a string", name)
	}
	v = strings.TrimSpace(v)
	if v == "" {
		return "", fmt.Errorf("core: %q is empty", name)
	}
	return v, nil
}

// unresolvedTemplate reports a value the rule table left for the context
// to fill in. Acting on one would create a folder literally named
// "{finderDir}", which is worse than refusing: the user would find it
// later and have no idea which press made it.
func unresolvedTemplate(v string) bool {
	return strings.HasPrefix(v, "{") && strings.HasSuffix(v, "}")
}

// pathParam is stringParam for the parameters that name a location.
func pathParam(params map[string]json.RawMessage, name string) (string, error) {
	v, err := stringParam(params, name)
	if err != nil {
		return "", err
	}
	if unresolvedTemplate(v) {
		return "", fmt.Errorf("core: %q is the rule table's unresolved template %s, not a path", name, v)
	}
	return v, nil
}

// pathList reads a path list from a capability's parameters, taking the
// registry's "paths" array or the single "path" the file capabilities
// name. One reader for both shapes, so a request is never refused for
// spelling a field a sibling accepted.
func pathList(raw json.RawMessage) ([]string, error) {
	params, err := dispatchParams(raw)
	if err != nil {
		return nil, err
	}
	if list, ok := params["paths"]; ok {
		var names []string
		if err := json.Unmarshal(list, &names); err != nil {
			return nil, fmt.Errorf("core: %q must be an array of strings", "paths")
		}
		out := make([]string, 0, len(names))
		for _, name := range names {
			name = strings.TrimSpace(name)
			if name == "" {
				return nil, fmt.Errorf("core: %q holds an empty path", "paths")
			}
			if unresolvedTemplate(name) {
				return nil, fmt.Errorf("core: %s is the rule table's unresolved template, not a path", name)
			}
			out = append(out, name)
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("core: %q names no path", "paths")
		}
		return out, nil
	}
	p, err := pathParam(params, "path")
	if err != nil {
		return nil, err
	}
	return []string{p}, nil
}

// createFile runs filesystem.createFile: {path, template?}. O_EXCL, so
// "create a new file" cannot quietly truncate one that is already there —
// the caller gets the error and the original key passes through.
func createFile(raw json.RawMessage) error {
	params, err := dispatchParams(raw)
	if err != nil {
		return err
	}
	path, err := pathParam(params, "path")
	if err != nil {
		return err
	}
	body := ""
	if tmpl, ok := params["template"]; ok {
		if err := json.Unmarshal(tmpl, &body); err != nil {
			return fmt.Errorf("core: %q must be a string", "template")
		}
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("core: create %s: %w", path, err)
	}
	if _, err := f.WriteString(body); err != nil {
		f.Close()
		return fmt.Errorf("core: create %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("core: create %s: %w", path, err)
	}
	return nil
}

// createFolder runs filesystem.createFolder: {path}. MkdirAll rather than
// Mkdir: a folder that is already there is the state the user asked for,
// and an error over it would be a false alarm from their own menu click.
func createFolder(raw json.RawMessage) error {
	params, err := dispatchParams(raw)
	if err != nil {
		return err
	}
	path, err := pathParam(params, "path")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("core: create folder %s: %w", path, err)
	}
	return nil
}

// moveToTrash runs file.moveToTrash: {paths:[…]}. ~/.Trash is the user's
// own trash, so the file comes back through the mechanism they already
// know — which is the whole difference between "trash" and "delete".
//
// Rename only. A file on another volume cannot be renamed into ~/.Trash
// and is reported as a failure rather than copied-then-deleted: a
// cross-volume move that runs out of space halfway is a lost file, and
// the reversible path is worth more than the convenience of covering it.
func moveToTrash(raw json.RawMessage) error {
	paths, err := pathList(raw)
	if err != nil {
		return err
	}
	if err := darwinOnly("file.moveToTrash"); err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("core: locate the trash: %w", err)
	}
	trash := filepath.Join(home, ".Trash")
	for _, path := range paths {
		dst, err := freeTrashName(trash, filepath.Base(path))
		if err != nil {
			return err
		}
		if err := os.Rename(path, dst); err != nil {
			return fmt.Errorf("core: move %s to trash: %w", path, err)
		}
	}
	return nil
}

// freeTrashName picks a destination inside the trash that nothing
// occupies. os.Rename overwrites on POSIX, so trashing "notes.txt" twice
// would destroy the first copy — the one file whose whole purpose is that
// it comes back.
func freeTrashName(dir, name string) (string, error) {
	free := func(candidate string) bool {
		_, err := os.Lstat(candidate)
		return os.IsNotExist(err)
	}
	if candidate := filepath.Join(dir, name); free(candidate) {
		return candidate, nil
	}
	ext, stem := filepath.Ext(name), strings.TrimSuffix(name, filepath.Ext(name))
	for i := 2; i < 1000; i++ {
		candidate := filepath.Join(dir, fmt.Sprintf("%s %d%s", stem, i, ext))
		if free(candidate) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("core: the trash already holds %d copies of %s", 999, name)
}

// copyPaths runs clipboard.copyPath: {paths:[…]} or {path}. One
// newline-joined payload, the shape Finder's own "Copy file paths" writes,
// so a paste into a terminal or an editor lands the same way.
func copyPaths(raw json.RawMessage) error {
	paths, err := pathList(raw)
	if err != nil {
		return err
	}
	if err := darwinOnly("clipboard.copyPath"); err != nil {
		return err
	}
	cmd := exec.Command("pbcopy")
	cmd.Stdin = strings.NewReader(strings.Join(paths, "\n"))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("core: copy paths: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// openTarget runs app.open: {target} — hand a file or URL to whichever app
// claims it.
func openTarget(raw json.RawMessage) error {
	params, err := dispatchParams(raw)
	if err != nil {
		return err
	}
	target, err := stringParam(params, "target")
	if err != nil {
		return err
	}
	if err := darwinOnly("app.open"); err != nil {
		return err
	}
	return launch("open", target)
}

// openTerminalAt runs terminal.openAt: {path} — a terminal sitting in that
// directory, the workflow the developer matrix's Ctrl+Shift+Enter claims.
func openTerminalAt(raw json.RawMessage) error {
	params, err := dispatchParams(raw)
	if err != nil {
		return err
	}
	path, err := pathParam(params, "path")
	if err != nil {
		return err
	}
	if err := darwinOnly("terminal.openAt"); err != nil {
		return err
	}
	return launch("open", "-a", "Terminal", path)
}

// darwinOnly refuses a capability this daemon cannot perform on the
// running platform, naming both. Naming the capability is the point: the
// dispatch path suppresses the key only when the action ran, and a
// capability that quietly did nothing is the one failure the user cannot
// see from the keyboard they are still holding down.
func darwinOnly(capability string) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("core: %s has no adapter on %s yet", capability, runtime.GOOS)
	}
	return nil
}

// launch runs a command and reports its stderr. `open` is the only door to
// a handler or a settings pane that costs us no entitlement of our own, and
// its failures come back on stderr rather than as an exit status alone.
func launch(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("core: %s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// --- focused-app cache (bead cross-os-heu) ---

// appCache holds the most recent focused-app snapshot the tap's decision
// path reads. The CGEventTap callback is on the hot path (<1ms budget), so
// it must never call AX; instead a background watcher refreshes this and the
// callback reads the cached value.
type appCache struct {
	mu     sync.RWMutex
	last   event.FastContext
	win    adapter.FocusedWindow
	hasWin bool
}

// get returns the cached context. Safe for the tap's hot path (one RWMutex
// read; no syscall, no cgo, no allocation beyond the struct copy).
func (a *appCache) get() event.FastContext {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.last
}

func (a *appCache) set(ctx event.FastContext) {
	a.mu.Lock()
	a.last = ctx
	a.mu.Unlock()
}

// setWindow records the focused window alongside its context.
func (a *appCache) setWindow(ctx event.FastContext, fw adapter.FocusedWindow) {
	a.mu.Lock()
	a.last = ctx
	a.win = fw
	a.hasWin = true
	a.mu.Unlock()
}

// window returns the cached focused window. Window actions read this instead
// of querying AX: the live AX call blocks indefinitely without accessibility
// consent, so calling it on the action path wedges the dispatch worker for
// the life of the daemon.
func (a *appCache) window() (adapter.FocusedWindow, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.win, a.hasWin
}

// watchFocusedApp keeps the cache fresh off the hot path. A bounded-interval
// poll is deliberate: an AX observer per focused-window change would be
// tidier, but the poll is off the callback path, costs one AX call per
// interval, and keeps the tap free of notification handling.
func (c *Core) watchFocusedApp(stop <-chan struct{}, cache *appCache) {
	wq := adapter.NewWindowQuery(&adapter.Driver{Log: daemonTapLog{}})
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	// The AX focused-window call is a synchronous IPC to the frontmost app
	// with no timeout, so it blocks indefinitely when that app is
	// unresponsive or consent is missing. Exactly ONE query may be in
	// flight: starting a new one per tick would pin an OS thread per
	// blocked call, forever.
	inFlight := make(chan struct{}, 1)
	type result struct {
		fw  adapter.FocusedWindow
		err error
	}
	for {
		select {
		case <-stop:
			return
		case <-tick.C:
		}
		select {
		case inFlight <- struct{}{}:
		default:
			continue // a query is still outstanding; do not stack another
		}
		done := make(chan result, 1)
		go func() {
			fw, err := wq.Focused()
			done <- result{fw, err}
		}()
		select {
		case <-stop:
			return // abandon the query; it writes to a buffered channel
		case r := <-done:
			<-inFlight
			if r.err != nil {
				continue // keep the last known app rather than blanking it
			}
			cache.setWindow(event.FastContext{
				AppID:    r.fw.BundleID,
				AppMode:  event.AppModeNative,
				WindowID: r.fw.Title,
			}, r.fw)
		case <-time.After(2 * time.Second):
			// Still blocked. Release the slot anyway: the abandoned call
			// will finish eventually and write to its buffered channel, and
			// one stuck query at a time is the bound we can hold.
			<-inFlight
		}
	}
}

// socketAlive reports whether a live daemon already answers on path. Used to
// refuse stealing the socket from a running instance (bead cross-os-jn1):
// unlinking a live daemon's socket and binding our own orphans the first one,
// and when IT exits, Go's default unlink-on-close deletes a path that now
// belongs to the second — leaving a running but deaf daemon forever.
func socketAlive(path string) bool {
	conn, err := net.DialTimeout("unix", path, 300*time.Millisecond)
	if err != nil {
		return false
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(500 * time.Millisecond))
	if _, err := conn.Write([]byte(`{"jsonrpc":"2.0","method":"core.status","id":1}` + "\n")); err != nil {
		return false
	}
	buf := make([]byte, 512)
	n, err := conn.Read(buf)
	return err == nil && n > 0
}

// killedState reports whether PANIC STOP is latched. Read under the same
// lock the handlers use so status and the decision path agree.
func (c *Core) killedState() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.killed
}
