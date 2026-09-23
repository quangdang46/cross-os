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
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
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
	mu      sync.Mutex
	daemon  *daemon.Daemon
	router  *event.Router
	rec     *record.Recorder
	set     *settings.Store
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
	c := &Core{
		daemon:  d,
		router:  event.Compile(rules, reg, grants, nil),
		rec:     record.NewRecorder(),
		set:     set,
		plugins: map[string]bool{},
		trials:  map[string]*safety.Trial{},
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
	if unhealthyTap() && tapErr == "" {
		tapErr = "keyboard tap keeps timing out — remapping degraded"
	}
	return map[string]any{
		"running":      st == pluginapi.LifecycleRunning || st == pluginapi.LifecycleSafeMode,
		"safe_mode":    st == pluginapi.LifecycleSafeMode,
		"killed":       st == pluginapi.LifecycleStopped,
		"interception": interception,
		"tap_error":    tapErr,
		// Served so the shell's About block shows the real version instead of
		// a literal that drifts at release time.
		"version": CurrentVersion,
	}, nil
}

// handlePluginList serves plugin.list: [{id,enabled,healthy}].
func (c *Core) handlePluginList(_ json.RawMessage) (any, *ipc.RPCError) {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]map[string]any, 0, len(c.order))
	for _, id := range c.order {
		out = append(out, map[string]any{
			"id": id, "enabled": c.plugins[id], "healthy": "healthy",
		})
	}
	return out, nil
}

// handlePluginSetEnabled serves plugin.setEnabled: {id, enabled}.
func (c *Core) handlePluginSetEnabled(raw json.RawMessage) (any, *ipc.RPCError) {
	var p struct {
		ID      string `json:"id"`
		Enabled bool   `json:"enabled"`
	}
	if err := json.Unmarshal(raw, &p); err != nil || p.ID == "" {
		return nil, &ipc.RPCError{Code: ipc.ErrBadParams, Message: "need {id, enabled}"}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.enableLocked(p.ID, p.Enabled); err != nil {
		return nil, &ipc.RPCError{Code: ipc.ErrInvalid, Message: err.Error()}
	}
	return map[string]any{"id": p.ID, "enabled": p.Enabled}, nil
}

// decideLocked runs one key event through the router + recorder tap and
// returns the decision. Disabled plugins' rules never fire, and neither do
// user-disabled matrix rows: the router is rebuilt from enabled-only rules
// on every toggle (10 rules — recompile is microseconds, and correctness
// beats caching here). Caller holds no lock; this takes mu to snapshot the
// enable set.
func (c *Core) decideLocked(ev event.Event, ctx event.FastContext) event.Outcome {
	c.mu.Lock()
	enabled := map[string]bool{}
	for id, on := range c.plugins {
		enabled[id] = on
	}
	c.mu.Unlock()
	known := map[string]bool{}
	var all []event.CompiledRule
	for _, r := range builtin.All() {
		known[r.RuleID] = true
		if enabled[r.PluginID] && c.set.IsRuleEnabled(r.RuleID) {
			all = append(all, r)
		}
	}
	rt := event.Compile(all, intent.DefaultRegistry(), builtin.Grants(), nil)
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
	return map[string]any{
		"interceptionDisabled": res.InterceptionDisabled,
		"pluginActionsStopped": res.PluginActionsStopped,
		"buffersFlushed":       res.BuffersFlushed,
		"loginItemKept":        res.LoginItemKept,
	}, nil
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
	defer c.mu.Unlock()
	tr, ok := c.trials[p.PluginID]
	if !ok || tr == nil {
		return nil, &ipc.RPCError{Code: ipc.ErrInvalid, Message: fmt.Sprintf("core: no trial for plugin %q (begin trial first)", p.PluginID)}
	}
	if err := tr.Confirm(p.Confirmed, p.Healthy); err != nil {
		return nil, &ipc.RPCError{Code: ipc.ErrInvalid, Message: err.Error()}
	}
	delete(c.trials, p.PluginID)
	c.plugins[p.PluginID] = true
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
	defer c.mu.Unlock()
	tr, ok := c.trials[p.PluginID]
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
	delete(c.trials, p.PluginID)
	c.plugins[p.PluginID] = false
	return map[string]any{"pluginId": p.PluginID, "state": "disabled"}, nil
}

// Serve registers the shell methods and serves ln in the background,
// returning the server (Close stops the accept loop: Close closes the
// listener, Accept errors, and Serve returns via the closed channel).
// main blocks on signals instead — Serve must never block its caller or
// tests cannot drive it.
func (c *Core) Serve(ln net.Listener) *ipc.Server {
	srv := ipc.NewServer()
	methods := map[string]ipc.Handler{
		"core.status":           c.handleStatus,
		"plugin.list":           c.handlePluginList,
		"plugin.setEnabled":     c.handlePluginSetEnabled,
		"core.keyEvent":         c.handleKeyEvent,
		"core.reset":            c.handleReset,
		"core.eventLogs":        c.handleEventLogs,
		"safety.panicStop":      c.handlePanicStop,
		"safety.beginTrial":     c.handleBeginTrial,
		"safety.confirmTrial":   c.handleConfirmTrial,
		"safety.rollbackTrial":  c.handleRollbackTrial,
		"core.checkForUpdate":   c.handleCheckForUpdate,
		"core.applyUpdate":      c.handleApplyUpdate,
		"config.setRuleEnabled": c.handleSetRuleEnabled,
		"config.getShortcuts":   c.handleGetShortcuts,
		"config.setShortcuts":   c.handleSetShortcuts,
	}
	for name, h := range methods {
		if err := srv.Register(name, h); err != nil {
			panic("core: duplicate IPC method " + name)
		}
	}
	go srv.Serve(ln)
	return srv
}

// listenSocket binds the Unix socket (0600 dir per ipc doc.go security note),
// removing a stale socket file first.
func listenSocket(path string) (net.Listener, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if socketAlive(path) {
		return nil, fmt.Errorf("another crossos daemon is already serving on %s — stop it first (scripts/run.sh reuses a running one)", path)
	}
	// Stale path only (a crashed daemon left the file behind): safe to rebind.
	_ = os.Remove(path)
	return net.Listen("unix", path)
}

func main() {
	// launchd invokes `crossos serve` (see scripts/launchd/); bare `crossos`
	// with no args also serves (dev convenience). Any other subcommand is a
	// usage error, never a silent serve.
	if len(os.Args) > 2 || (len(os.Args) == 2 && os.Args[1] != "serve") {
		fmt.Fprintln(os.Stderr, "usage: crossos [serve]")
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
	// registerBuiltin seeds the enable map the shell toggles.
	for _, id := range builtin.BuiltinIDs {
		c.registerBuiltin(id, true)
	}
	// Live keyboard tap (bead cross-os-2io). Best-effort: a TCC denial is
	// NOT fatal — the daemon still serves IPC and core.status reports
	// interception=off with the reason, so the user can grant consent and
	// see exactly what failed. The decide entry binds the LIVE decision path
	// (decideLocked), so config.setRuleEnabled and plugin toggles take effect
	// on the running tap instead of a start-up snapshot.
	stopTap := c.startTap()
	defer stopTap()

	path := DefaultSocketPath()
	ln, err := listenSocket(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "crossos: listen:", err)
		os.Exit(1)
	}
	srv := c.Serve(ln)
	defer srv.Close()
	fmt.Println("crossos: serving on", path)
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
	watchStop := make(chan struct{})
	defer close(watchStop)
	go c.watchFocusedApp(watchStop, cache)

	d := &adapter.Driver{
		// Live path: honours plugin enable + rule toggles on EVERY key.
		Decide:   c.decideLocked,
		Dispatch: c.dispatch,
		Context:  cache.get,
		Log:      daemonTapLog{},
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
	return func() { _ = tapStopFn(d) }
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
		return c.windowDispatch(req)
	default:
		return fmt.Errorf("core: no adapter for capability %q (not implemented yet)", req.Capability.ID)
	}
}

// windowDispatch routes a window.* capability to the platform seam.
func (c *Core) windowDispatch(req intent.Request) error {
	var zone string
	if len(req.Intent.Parameters) > 0 {
		var p struct {
			Zone string `json:"zone"`
		}
		if err := json.Unmarshal(req.Intent.Parameters, &p); err == nil {
			zone = p.Zone
		}
	}
	wq := adapter.NewWindowQuery(&adapter.Driver{Log: daemonTapLog{}})
	fw, err := wq.Focused()
	if err != nil {
		return err
	}
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

// --- focused-app cache (bead cross-os-heu) ---

// appCache holds the most recent focused-app snapshot the tap's decision
// path reads. The CGEventTap callback is on the hot path (<1ms budget), so
// it must never call AX; instead a background watcher refreshes this and the
// callback reads the cached value.
type appCache struct {
	mu   sync.RWMutex
	last event.FastContext
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

// watchFocusedApp keeps the cache fresh off the hot path. A bounded-interval
// poll is deliberate: an AX observer per focused-window change would be
// tidier, but the poll is off the callback path, costs one AX call per
// interval, and keeps the tap free of notification handling.
func (c *Core) watchFocusedApp(stop <-chan struct{}, cache *appCache) {
	wq := adapter.NewWindowQuery(&adapter.Driver{Log: daemonTapLog{}})
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-stop:
			return
		case <-tick.C:
			fw, err := wq.Focused()
			if err != nil {
				continue // keep the last known app rather than blanking it
			}
			cache.set(event.FastContext{
				AppID:    fw.BundleID,
				AppMode:  event.AppModeNative,
				WindowID: fw.Title,
			})
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
