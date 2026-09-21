package plugin

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"crossos/core/pkg/event"
	"crossos/core/pkg/intent"
	"crossos/core/pkg/pluginapi"
	"crossos/core/pkg/rule"
)

// Health is the plugin health state (§10 Phase 4 vocabulary,
// forward-compatible: MVP 1 reports these, Phase 4 acts on them).
type Health string

const (
	HealthDisabled Health = "DISABLED"
	HealthTrial    Health = "TRIAL"
	HealthHealthy  Health = "HEALTHY"
	HealthEnabled  Health = "ENABLED"
)

// Plugin is a supervised Level B child process.
//
// Wait discipline (single-waiter, review: cross-os-c0): exactly ONE
// goroutine ever calls cmd.Wait — the reaper started by Start. Stop,
// Exited, and WaitExit all consume the reaped result via waitCh; none
// calls Wait itself (exec.Cmd.Wait is not safe for concurrent use, and a
// speculative Wait per liveness probe would leak a blocked goroutine per
// check). Liveness is a non-blocking ProcessState/seeBelow check, never a
// speculative Wait.
type Plugin struct {
	ID       string
	Dir      string // ~/.crossos/plugins/<id>
	Manifest pluginapi.Manifest

	mu     sync.Mutex
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Scanner
	health Health
	nextID int
	closed bool

	waitCh chan struct{} // closed by the single reaper when the child exits
}

// LoadDir reads manifest.json from a plugin dir and validates the core
// ownership rule: a manifest claiming core-owned capability IDs fails
// closed here, before any process spawns.
func LoadDir(dir string, coreIDs []string) (pluginapi.Manifest, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return pluginapi.Manifest{}, fmt.Errorf("plugin: read manifest: %w", err)
	}
	var m pluginapi.Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return pluginapi.Manifest{}, fmt.Errorf("plugin: bad manifest: %w", err)
	}
	reserved := map[string]bool{}
	for _, id := range coreIDs {
		reserved[id] = true
	}
	for _, c := range m.Capabilities {
		if c.Owner == pluginapi.CapabilityOwnerCore || reserved[c.Name] {
			return pluginapi.Manifest{}, fmt.Errorf(
				"plugin: manifest claims core-owned capability %q", c.Name)
		}
	}
	return m, nil
}

// Start spawns the plugin executable with stdio pipes. Path comes from the
// manifest Entry, resolved under Dir (never absolute — path traversal
// outside the plugin dir is rejected). Extra child env goes through
// StartEnv (tests); Start passes the parent environment unchanged.
func Start(dir string, m pluginapi.Manifest, coreIDs []string) (*Plugin, error) {
	return start(dir, m, coreIDs, nil)
}

// StartEnv is Start with extra environment entries for the child.
func StartEnv(dir string, m pluginapi.Manifest, coreIDs []string, extraEnv []string) (*Plugin, error) {
	return start(dir, m, coreIDs, extraEnv)
}

func start(dir string, m pluginapi.Manifest, coreIDs []string, extraEnv []string) (*Plugin, error) {
	if filepath.IsAbs(m.Entry) || m.Entry == "" {
		return nil, fmt.Errorf("plugin: entry must be a relative path, got %q", m.Entry)
	}
	bin := filepath.Join(dir, filepath.Clean(m.Entry))
	if rel, err := filepath.Rel(dir, bin); err != nil || rel == ".." ||
		len(rel) > 2 && rel[:3] == ".."+string(filepath.Separator) {
		return nil, fmt.Errorf("plugin: entry escapes plugin dir: %q", m.Entry)
	}
	// Re-validate ownership at spawn (defense in depth — LoadDir checked too).
	for _, c := range m.Capabilities {
		if c.Owner == pluginapi.CapabilityOwnerCore {
			return nil, fmt.Errorf("plugin: core-owned claim %q", c.Name)
		}
		for _, id := range coreIDs {
			if c.Name == id {
				return nil, fmt.Errorf("plugin: core-owned claim %q", c.Name)
			}
		}
	}
	cmd := exec.Command(bin)
	if len(extraEnv) > 0 {
		cmd.Env = append(os.Environ(), extraEnv...)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("plugin: spawn %q: %w", bin, err)
	}
	p := &Plugin{ID: m.ID, Dir: dir, Manifest: m, cmd: cmd,
		stdin: stdin, health: HealthHealthy, waitCh: make(chan struct{})}
	p.stdout = bufio.NewScanner(stdout)
	p.stdout.Buffer(make([]byte, 64*1024), 1024*1024)
	// Single reaper: the ONLY cmd.Wait in this package. Reaping sets
	// ProcessState (Exited() reads it), releases the exe file lock
	// (Windows), and wakes all WaitExit consumers via waitCh.
	go func() {
		_ = cmd.Wait()
		p.mu.Lock()
		p.health = HealthDisabled
		p.mu.Unlock()
		close(p.waitCh)
	}()
	return p, nil
}

// Call performs one JSON-RPC call over the plugin's stdio (same framing as
// package ipc: one object per line). No callback-path use: callers are
// slow-path workers that compiled the plugin's rules ahead of time.
//
// Single-flight: the mutex serializes calls (IDs increment under mu, no
// response matching needed — pipelining is impossible). Do NOT parallelize
// calls later without adding ID matching.
//
// Hang limitation (review: cross-os-c0): a live-but-silent plugin blocks
// Scan() indefinitely with mu held, wedging every caller including Stop.
// os.Pipe has no read deadline, so MVP 1 documents the hang; the supervisor
// watchdog bead owns the timeout (scan-in-goroutine + select, mark-
// unhealthy on expiry). TODO(watchdog): bound Call with a deadline.
func (p *Plugin) Call(method string, params any) (json.RawMessage, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil, fmt.Errorf("plugin %q: closed", p.ID)
	}
	p.nextID++
	req := map[string]any{"jsonrpc": "2.0", "method": method,
		"params": params, "id": p.nextID}
	raw, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	raw = append(raw, '\n')
	if _, err := p.stdin.Write(raw); err != nil {
		return nil, fmt.Errorf("plugin %q: write: %w", p.ID, err)
	}
	if !p.stdout.Scan() {
		// EOF/error = plugin died mid-call. Mark, don't propagate panic.
		p.health = HealthDisabled
		if err := p.stdout.Err(); err != nil {
			return nil, fmt.Errorf("plugin %q: died: %w", p.ID, err)
		}
		return nil, fmt.Errorf("plugin %q: died (EOF)", p.ID)
	}
	var resp struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		ID any `json:"id"`
	}
	if err := json.Unmarshal(p.stdout.Bytes(), &resp); err != nil {
		return nil, fmt.Errorf("plugin %q: bad response: %w", p.ID, err)
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("plugin %q: rpc %d: %s",
			p.ID, resp.Error.Code, resp.Error.Message)
	}
	return resp.Result, nil
}

// Health reports the current health state.
func (p *Plugin) Health() Health {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.health
}

// Stop terminates the child (disable path). Double-stop is safe. Never
// calls Wait — the single reaper owns it; Stop kills and returns, the
// reaper's close(waitCh) records the death.
func (p *Plugin) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	p.closed = true
	p.health = HealthDisabled
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
	return nil
}

// Exited reports whether the child process has died (crash detection).
// Non-blocking: consults the reaper's waitCh + ProcessState only — never a
// speculative Wait (no leaked goroutines, no concurrent-Wait race).
func (p *Plugin) Exited() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd == nil || p.cmd.Process == nil {
		return true
	}
	select {
	case <-p.waitCh:
		p.health = HealthDisabled
		return true
	default:
	}
	if p.cmd.ProcessState != nil && p.cmd.ProcessState.Exited() {
		p.health = HealthDisabled
		return true
	}
	return false
}

// RuleEntry is one Level B rule as declared by the plugin (fetched once via
// the `plugin.rules` RPC at load, then compiled into the cached matcher).
type RuleEntry struct {
	RuleID      string          `json:"rule_id"`
	KeyCode     uint32          `json:"key_code"`
	Modifiers   uint32          `json:"modifiers"`
	AppModes    []string        `json:"app_modes"`
	Priority    int             `json:"priority"`
	Specificity int             `json:"specificity"`
	Scope       string          `json:"scope"`
	Intent      json.RawMessage `json:"intent"`
	Emit        bool            `json:"emit"`
}

// FetchRules asks the plugin for its rule declarations (load-time only,
// never on the callback path).
func (p *Plugin) FetchRules() ([]RuleEntry, error) {
	raw, err := p.Call("plugin.rules", nil)
	if err != nil {
		return nil, err
	}
	var out []RuleEntry
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("plugin %q: bad rules: %w", p.ID, err)
	}
	return out, nil
}

// CompiledRules converts fetched entries to event.CompiledRule for the
// router's Compile step. Unknown scopes fail closed (dropped, counted).
// Unknown app modes fail closed the same way. This is the "compiled ahead
// of time" half of the fast-path invariant.
func CompiledRules(pluginID string, entries []RuleEntry, reg *intent.Registry) ([]event.CompiledRule, int) {
	var out []event.CompiledRule
	dropped := 0
	for _, e := range entries {
		scope, ok := parseScope(e.Scope)
		if !ok {
			dropped++
			continue
		}
		var modes []event.AppMode
		badMode := false
		for _, m := range e.AppModes {
			switch event.AppMode(m) {
			case event.AppModeNative, event.AppModeTerminal,
				event.AppModeRemote, event.AppModeVM, event.AppModeExcluded:
				modes = append(modes, event.AppMode(m))
			default:
				badMode = true
			}
		}
		if badMode {
			dropped++
			continue
		}
		var in intent.Intent
		if err := json.Unmarshal(e.Intent, &in); err != nil {
			dropped++
			continue
		}
		if err := in.Validate(reg); err != nil {
			dropped++
			continue
		}
		out = append(out, event.CompiledRule{
			KeyCode: e.KeyCode, Modifiers: e.Modifiers, AppModes: modes,
			RuleID: e.RuleID, PluginID: pluginID,
			Priority: e.Priority, Specificity: e.Specificity, Scope: scope,
			Intent: in, Emit: e.Emit,
		})
	}
	return out, dropped
}

// parseScope maps the wire string to rule.Scope. Unknown scopes fail
// closed (dropped at compile, counted, never matched). The zero Scope on
// failure is meaningless — callers must check ok first. (review: cross-os-c0)
func parseScope(s string) (rule.Scope, bool) {
	switch s {
	case "global":
		return rule.ScopeGlobal, true
	case "app":
		return rule.ScopeApp, true
	case "window":
		return rule.ScopeWindow, true
	case "device":
		return rule.ScopeDevice, true
	}
	return rule.Scope(0), false
}

// WaitExit blocks until the child exits or timeout elapses (supervisor use).
// Consumes the single reaper's waitCh — never calls Wait itself. A reaped
// child has ProcessState set (Exited() true) and the exe lock released.
func (p *Plugin) WaitExit(timeout time.Duration) bool {
	p.mu.Lock()
	ch := p.waitCh
	p.mu.Unlock()
	if ch == nil {
		return true
	}
	select {
	case <-ch:
		return true
	case <-time.After(timeout):
		return false
	}
}
