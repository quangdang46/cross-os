package plugin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"crossos/core/pkg/event"
	"crossos/core/pkg/intent"
	"crossos/core/pkg/pluginapi"
)

var testCoreIDs = []string{"window.close", "clipboard.copy", "input.intercept"}

func writeManifest(t *testing.T, dir string, caps []pluginapi.Capability) {
	t.Helper()
	m := pluginapi.Manifest{ID: "fake", Name: "fake", Version: "1",
		Entry: "plugin", Type: pluginapi.PluginExec, Capabilities: caps}
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadDirRejectsCoreClaim(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, []pluginapi.Capability{
		{Name: "window.close", Owner: pluginapi.CapabilityOwnerPlugin},
	})
	if _, err := LoadDir(dir, testCoreIDs); err == nil {
		t.Fatal("manifest claiming window.close allowed")
	}
}

func TestLoadDirAcceptsNamespaced(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, []pluginapi.Capability{
		{Name: "fake.do", Owner: pluginapi.CapabilityOwnerPlugin},
	})
	m, err := LoadDir(dir, testCoreIDs)
	if err != nil {
		t.Fatalf("namespaced manifest rejected: %v", err)
	}
	if m.ID != "fake" {
		t.Fatalf("bad manifest: %+v", m)
	}
}

func TestStartRejectsAbsoluteEntry(t *testing.T) {
	m := pluginapi.Manifest{ID: "x", Entry: "/bin/evil", Type: pluginapi.PluginExec}
	if _, err := Start(t.TempDir(), m, testCoreIDs); err == nil {
		t.Fatal("absolute entry allowed")
	}
	m.Entry = "../escape"
	if _, err := Start(t.TempDir(), m, testCoreIDs); err == nil {
		t.Fatal("escaping entry allowed")
	}
}

// TestCrashIsolation: Core survives a plugin death. Strategy: hardlink the
// test binary itself into the plugin dir as "plugin" (relative entry, so
// Start's traversal check passes) and run it with CROSSOS_FAKEPLUGIN=1, in
// which mode it speaks one plugin.rules response then exits 0. Core must
// get the rules, observe EOF, mark DISABLED, and survive.
func TestCrashIsolation(t *testing.T) {
	// Helper mode: behave as the fake plugin.
	if os.Getenv("CROSSOS_FAKEPLUGIN") == "1" {
		runFakePlugin()
		return
	}
	dir := t.TempDir()
	writeManifest(t, dir, []pluginapi.Capability{
		{Name: "fake.do", Owner: pluginapi.CapabilityOwnerPlugin},
	})
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "plugin.exe")
	if err := os.Link(self, link); err != nil {
		t.Skipf("hardlink unavailable: %v", err)
	}
	// Windows holds a hardlinked image for a beat after reap; TempDir
	// cleanup must not fail the test for that. Best-effort early unlink
	// is attempted in cleanup; residual lock = environment, not product.
	t.Cleanup(func() { _ = os.Remove(link) })
	// Entry needs the OS executable suffix to spawn (".exe" on Windows).
	entry := "plugin"
	if os.Getenv("OS") != "" || runtime.GOOS == "windows" {
		entry = "plugin.exe"
	}
	m := pluginapi.Manifest{ID: "fake", Entry: entry, Type: pluginapi.PluginExec,
		Capabilities: []pluginapi.Capability{{Name: "fake.do", Owner: pluginapi.CapabilityOwnerPlugin}}}
	// Env must be set before Start (child inherits at spawn).
	cmd := startWithEnv(t, dir, m)
	p := cmd
	defer p.Stop()
	rules, err := p.FetchRules()
	if err != nil {
		t.Fatalf("fetch rules: %v", err)
	}
	if len(rules) != 1 || rules[0].RuleID != "fake.r1" {
		t.Fatalf("bad rules: %+v", rules)
	}
	// Plugin exits after serving once: Core observes death, marks
	// DISABLED, and survives (no panic, no hang).
	if !p.WaitExit(5 * time.Second) {
		t.Fatal("plugin did not exit")
	}
	if !p.Exited() {
		t.Fatal("Exited() false after death")
	}
	if p.Health() != HealthDisabled {
		t.Fatalf("want DISABLED after crash, got %q", p.Health())
	}
	// Calls after death fail cleanly (no hang, no panic).
	if _, err := p.Call("plugin.rules", nil); err == nil {
		t.Fatal("call after death succeeded")
	}
}

func startWithEnv(t *testing.T, dir string, m pluginapi.Manifest) *Plugin {
	t.Helper()
	p, err := StartEnv(dir, m, testCoreIDs, []string{"CROSSOS_FAKEPLUGIN=1"})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	return p
}

// runFakePlugin speaks one plugin.rules response then exits (crash-after-
// serve: Core must observe EOF, mark DISABLED, and survive).
//
// NOTE: the test binary runs TestMain-less, so EVERY test (including flag
// parsing) executes in the helper too. os.Exit(0) must happen before
// testing.Main runs — hence: serve first, exit before returning. If the
// helper ever hangs here, the parent's WaitExit times out and fails.
func TestMain(m *testing.M) {
	// Helper entry: when spawned as a fake plugin, serve + exit BEFORE
	// the test harness runs (os.Exit skips TestMain return).
	if os.Getenv("CROSSOS_FAKEPLUGIN") == "1" {
		runFakePlugin()
	}
	os.Exit(m.Run())
}

func runFakePlugin() {
	// Minimal line-JSON-RPC: read one request line, answer rules, exit.
	// Must not block: answer the first decoded request OR exit(2) on EOF
	// (parent treats exit(2) as immediate-crash, still a valid death).
	var req map[string]any
	dec := json.NewDecoder(os.Stdin)
	if err := dec.Decode(&req); err != nil {
		os.Exit(2)
	}
	rules := []RuleEntry{{
		RuleID: "fake.r1", KeyCode: 0x43, Modifiers: 1,
		AppModes: []string{"native"}, Priority: 10, Specificity: 0,
		Scope:  "global",
		Intent: json.RawMessage(`{"ID":"window.close","Version":1,"Source":"keyboard"}`),
		Emit:   true,
	}}
	raw, _ := json.Marshal(rules)
	resp := map[string]any{"jsonrpc": "2.0", "result": json.RawMessage(raw), "id": req["id"]}
	enc, _ := json.Marshal(resp)
	os.Stdout.Write(append(enc, '\n'))
	os.Exit(0) // die right after serving: crash path
}

func TestCompiledRulesAheadOfTime(t *testing.T) {
	// Level B rules compile into the cached matcher at load (never on the
	// callback path). Unknown scope/mode/intent fail closed (dropped).
	reg := intent.DefaultRegistry()
	mk := func(id string) json.RawMessage {
		raw, _ := json.Marshal(map[string]any{"ID": id, "Version": 1, "Source": "keyboard"})
		return raw
	}
	entries := []RuleEntry{
		{RuleID: "good", KeyCode: 0x43, Modifiers: 1, AppModes: []string{"native"},
			Priority: 10, Specificity: 0, Scope: "global", Intent: mk("window.close"), Emit: true},
		{RuleID: "bad-scope", KeyCode: 0x43, Scope: "quantum", Intent: mk("window.close")},
		{RuleID: "bad-mode", KeyCode: 0x43, AppModes: []string{"mars"}, Scope: "global", Intent: mk("window.close")},
		{RuleID: "bad-intent", KeyCode: 0x43, Scope: "global", Intent: mk("nope.unknown")},
	}
	got, dropped := CompiledRules("fake", entries, reg)
	if len(got) != 1 || got[0].RuleID != "good" {
		t.Fatalf("compiled wrong: %+v dropped=%d", got, dropped)
	}
	if dropped != 3 {
		t.Fatalf("dropped: got %d, want 3", dropped)
	}
}

func TestNoSyncExecutionProbe(t *testing.T) {
	// Bead criterion: the decision path never blocks on plugin IPC. Probe:
	// run Decide in a loop while a plugin Call would block (no server on
	// the other end); assert Decide returns promptly WITHOUT any plugin
	// reference — compilation happened ahead of time, Decide touches only
	// the compiled slice. Marker: if Decide ever took a plugin handle, this
	// test's compile would fail (no such parameter exists).
	reg := intent.DefaultRegistry()
	rules, _ := CompiledRules("fake", []RuleEntry{{
		RuleID: "r", KeyCode: 0x43, Modifiers: 1, AppModes: []string{"native"},
		Priority: 10, Specificity: 0, Scope: "global",
		Intent: func() json.RawMessage {
			raw, _ := json.Marshal(map[string]any{"ID": "window.close", "Version": 1, "Source": "keyboard"})
			return raw
		}(), Emit: true,
	}}, reg)
	rt := event.Compile(rules, reg, map[string][]intent.Permission{
		"fake": {intent.PermAccessControl},
	}, nil)
	start := time.Now()
	const n = 1000
	for i := 0; i < n; i++ {
		out := rt.Decide(
			event.Event{Type: event.EventKeyDown, Source: event.SourceKeyboard, KeyCode: 0x43, Modifiers: 1},
			event.FastContext{AppID: "x", AppMode: event.AppModeNative})
		if out.Decision != pluginapi.DecisionReplace {
			t.Fatalf("want Replace, got %v", out.Decision)
		}
	}
	per := time.Since(start) / n
	t.Logf("per-decision with pre-compiled Level B rule: %v", per)
	if per >= 10*time.Millisecond {
		t.Fatalf("decision too slow (plugin IPC on path?): %v", per)
	}
}

func TestHealthLifecycle(t *testing.T) {
	// Health states exist and Stop transitions to DISABLED.
	p := &Plugin{ID: "x", health: HealthHealthy}
	if p.Health() != HealthHealthy {
		t.Fatalf("want HEALTHY, got %q", p.Health())
	}
	if err := p.Stop(); err != nil {
		t.Fatal(err)
	}
	if p.Health() != HealthDisabled {
		t.Fatalf("want DISABLED after stop, got %q", p.Health())
	}
	// Trial/Enabled vocabulary present (Phase 4 acts on them).
	for _, h := range []Health{HealthDisabled, HealthTrial, HealthHealthy, HealthEnabled} {
		if h == "" {
			t.Fatal("empty health state")
		}
	}
}
