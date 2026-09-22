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

	"crossos/core/pkg/daemon"
	"crossos/core/pkg/event"
	"crossos/core/pkg/intent"
	"crossos/core/pkg/ipc"
	"crossos/core/pkg/observe"
	"crossos/core/pkg/pluginapi"
	"crossos/core/pkg/record"
	"crossos/core/pkg/safety"
	"crossos/core/pkg/update"
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
// plugin enable map (handlers run on independent connection goroutines).
type Core struct {
	mu      sync.Mutex
	daemon  *daemon.Daemon
	router  *event.Router
	rec     *record.Recorder
	plugins map[string]bool // id → enabled
	order   []string
}

// NewCore builds a Core with the daemon running and builtin matrices loaded.
// rules/grants are injected so tests bind fixtures without plugin modules
// (production passes loadBuiltin below).
func NewCore(rules []event.CompiledRule, grants map[string][]intent.Permission) (*Core, error) {
	d := daemon.New()
	if err := d.Run(); err != nil {
		return nil, err
	}
	reg := intent.DefaultRegistry()
	c := &Core{
		daemon:  d,
		router:  event.Compile(rules, reg, grants, nil),
		rec:     record.NewRecorder(),
		plugins: map[string]bool{},
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

// handleStatus serves core.status: running/safe_mode/killed.
func (c *Core) handleStatus(_ json.RawMessage) (any, *ipc.RPCError) {
	st := c.daemon.State()
	return map[string]any{
		"running":   st == pluginapi.LifecycleRunning || st == pluginapi.LifecycleSafeMode,
		"safe_mode": st == pluginapi.LifecycleSafeMode,
		"killed":    st == pluginapi.LifecycleStopped,
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
// returns the decision. Disabled plugins' rules never fire: the router is
// rebuilt from enabled-only rules on every toggle (10 rules — recompile is
// microseconds, and correctness beats caching here). Caller holds no lock;
// this takes mu to snapshot the enable set.
func (c *Core) decideLocked(ev event.Event, ctx event.FastContext) event.Outcome {
	c.mu.Lock()
	enabled := map[string]bool{}
	for id, on := range c.plugins {
		enabled[id] = on
	}
	c.mu.Unlock()
	var all []event.CompiledRule
	for _, r := range builtin.All() {
		if enabled[r.PluginID] {
			all = append(all, r)
		}
	}
	rt := event.Compile(all, intent.DefaultRegistry(), builtin.Grants(), nil)
	out := rt.Decide(ev, ctx)
	c.rec.OnOutcome(out)
	return out
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

// Serve registers the shell methods and serves ln in the background,
// returning the server (Close stops the accept loop: Close closes the
// listener, Accept errors, and Serve returns via the closed channel).
// main blocks on signals instead — Serve must never block its caller or
// tests cannot drive it.
func (c *Core) Serve(ln net.Listener) *ipc.Server {
	srv := ipc.NewServer()
	methods := map[string]ipc.Handler{
		"core.status":         c.handleStatus,
		"plugin.list":         c.handlePluginList,
		"plugin.setEnabled":   c.handlePluginSetEnabled,
		"core.keyEvent":       c.handleKeyEvent,
		"core.reset":          c.handleReset,
		"core.eventLogs":      c.handleEventLogs,
		"safety.panicStop":    c.handlePanicStop,
		"core.checkForUpdate": c.handleCheckForUpdate,
		"core.applyUpdate":    c.handleApplyUpdate,
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
	c, err := NewCore(builtin.All(), builtin.Grants())
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
