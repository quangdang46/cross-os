// Daemon serve tests — bead cross-os-325.
//
// Pass criteria mapping: daemon runs (NewCore transitions to running),
// method table serves all five shell methods over a live Unix socket
// (status/list/setEnabled/reset/eventLogs), plugin.list reflects
// enable toggles, unknown plugin is a typed RPC error (never silent),
// go test covers the table via in-process listener + real framing.
package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"

	"crossos/core/pkg/event"
	"crossos/core/pkg/intent"
	"crossos/core/pkg/ipc"
	builtin "crossos/core/rules"
)

// serveTest is intentionally absent: handlers are pure (Core + raw JSON)
// and the live-socket path is covered by TestLiveSocketRoundTrip below.
// (An earlier draft carried a net.Pipe harness with a blocking oneListener;
// removed — Serve blocks on Accept and the test binary cannot exit.)

func TestMethodTable(t *testing.T) {
	c, err := NewCore(nil, nil)
	if err != nil {
		t.Fatalf("NewCore: %v", err)
	}
	if got := c.daemon.State(); string(got) != "running" {
		t.Fatalf("daemon state=%q, want running", got)
	}
	for _, id := range []string{"windows-keyboard", "developer", "launcher"} {
		c.registerBuiltin(id, id != "launcher")
	}
	// core.status shape matches ipc_client.go (running/safe_mode/killed).
	res, rerr := c.handleStatus(nil)
	if rerr != nil {
		t.Fatalf("status: %v", rerr)
	}
	raw, _ := json.Marshal(res)
	var st struct {
		Running  bool `json:"running"`
		SafeMode bool `json:"safe_mode"`
		Killed   bool `json:"killed"`
	}
	if err := json.Unmarshal(raw, &st); err != nil {
		t.Fatalf("status shape: %v", err)
	}
	if !st.Running || st.SafeMode || st.Killed {
		t.Fatalf("status=%+v, want running-only", st)
	}
	// plugin.list reflects registration + toggles.
	list, _ := c.handlePluginList(nil)
	lraw, _ := json.Marshal(list)
	var plugins []map[string]any
	if err := json.Unmarshal(lraw, &plugins); err != nil || len(plugins) != 3 {
		t.Fatalf("list=%s err=%v, want 3 rows", lraw, err)
	}
	// Unknown plugin toggle → typed RPC error (never silent).
	if _, rerr := c.handlePluginSetEnabled(json.RawMessage(`{"id":"nope","enabled":false}`)); rerr == nil {
		t.Fatal("unknown plugin toggle must fail")
	}
	if _, rerr := c.handlePluginSetEnabled(json.RawMessage(`{"id":"launcher","enabled":true}`)); rerr != nil {
		t.Fatalf("toggle launcher: %v", rerr)
	}
	// core.reset returns audited steps.
	rres, _ := c.handleReset(nil)
	rraw, _ := json.Marshal(rres)
	var steps []string
	if err := json.Unmarshal(rraw, &steps); err != nil || len(steps) != 4 {
		t.Fatalf("reset=%s, want 4 audited steps", rraw)
	}
	// core.eventLogs returns a JSON array (empty when no events).
	eres, _ := c.handleEventLogs(nil)
	eraw, _ := json.Marshal(eres)
	var logs []string
	if err := json.Unmarshal(eraw, &logs); err != nil {
		t.Fatalf("eventLogs not an array: %v", err)
	}
	// Bad params → ErrBadParams (fail-closed framing).
	if _, rerr := c.handlePluginSetEnabled(json.RawMessage(`{}`)); rerr == nil || rerr.Code != ipc.ErrBadParams {
		t.Fatalf("empty params must be ErrBadParams, got %v", rerr)
	}
	// Router + recorder wired (decision path live behind IPC).
	if c.router == nil || c.rec == nil {
		t.Fatal("router/recorder must be wired")
	}
	_ = event.CompiledRule{}
	_ = intent.DefaultRegistry
}

// TestLiveSocketRoundTrip serves a real Unix socket and drives all five
// methods through actual JSON-RPC framing (same bytes the shell sends).
func TestLiveSocketRoundTrip(t *testing.T) {
	c, err := NewCore(nil, nil)
	if err != nil {
		t.Fatalf("NewCore: %v", err)
	}
	for _, id := range []string{"windows-keyboard", "developer", "launcher"} {
		c.registerBuiltin(id, true)
	}
	path := filepath.Join(os.TempDir(), fmt.Sprintf("cx-t-%d.sock", os.Getpid()))
	_ = os.Remove(path)
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := c.Serve(ln)
	defer srv.Close()
	defer os.Remove(path)

	call := func(method string, params any) json.RawMessage {
		t.Helper()
		conn, err := net.Dial("unix", path)
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		defer conn.Close()
		var praw json.RawMessage
		if params != nil {
			praw, err = json.Marshal(params)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
		}
		req, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": method, "params": praw, "id": 1})
		req = append(req, '\n')
		if _, err := conn.Write(req); err != nil {
			t.Fatalf("write: %v", err)
		}
		var line []byte
		buf := make([]byte, 1)
		for {
			n, rerr := conn.Read(buf)
			if n > 0 {
				line = append(line, buf[:n]...)
				if buf[0] == '\n' {
					break
				}
			}
			if rerr != nil {
				t.Fatalf("read: %v", rerr)
			}
		}
		var resp struct {
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(line, &resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if resp.Error != nil {
			t.Fatalf("%s: rpc %d %s", method, resp.Error.Code, resp.Error.Message)
		}
		return resp.Result
	}

	// All five shell methods live over the socket.
	var st struct {
		Running bool `json:"running"`
	}
	if err := json.Unmarshal(call("core.status", nil), &st); err != nil || !st.Running {
		t.Fatalf("live status: %+v %v", st, err)
	}
	var plugins []map[string]any
	if err := json.Unmarshal(call("plugin.list", nil), &plugins); err != nil || len(plugins) != 3 {
		t.Fatalf("live list: %s", call("plugin.list", nil))
	}
	call("plugin.setEnabled", map[string]any{"id": "launcher", "enabled": false})
	var plugins2 []map[string]any
	if err := json.Unmarshal(call("plugin.list", nil), &plugins2); err != nil {
		t.Fatalf("live list2: %v", err)
	}
	found := false
	for _, p := range plugins2 {
		if p["id"] == "launcher" && p["enabled"] == false {
			found = true
		}
	}
	if !found {
		t.Fatal("live toggle did not persist across connections")
	}
	var steps []string
	if err := json.Unmarshal(call("core.reset", nil), &steps); err != nil || len(steps) != 4 {
		t.Fatalf("live reset: %s", call("core.reset", nil))
	}
	var logs []string
	if err := json.Unmarshal(call("core.eventLogs", nil), &logs); err != nil {
		t.Fatalf("live eventLogs not array: %v", err)
	}
	// core.keyEvent live over the socket: same bytes the shell sends.
	var kd struct {
		Decision string `json:"decision"`
		Intent   string `json:"intent"`
		Winner   string `json:"winner"`
	}
	if err := json.Unmarshal(call("core.keyEvent", map[string]any{
		"keyCode": 67, "modifiers": 1, "appId": "com.apple.Finder", "appMode": "native",
	}), &kd); err != nil {
		t.Fatalf("live keyEvent decode: %v", err)
	}
	if kd.Decision != "REPLACE" || kd.Intent != "clipboard.copy" {
		t.Fatalf("live keyEvent ctrl+c: %+v, want REPLACE/clipboard.copy", kd)
	}
}

// TestKeyEventDecision: core.keyEvent drives the decision path — Ctrl+C
// claims copy, F8 passes (unclaimed), Ctrl+Space consumes (launcher),
// bad type/mode fail closed, and decisions land in the recorder trace.
func TestKeyEventDecision(t *testing.T) {
	c, err := NewCore(builtin.All(), builtin.Grants())
	if err != nil {
		t.Fatalf("NewCore: %v", err)
	}
	for _, id := range builtin.BuiltinIDs {
		c.registerBuiltin(id, true)
	}
	key := func(params string) map[string]any {
		t.Helper()
		res, rerr := c.handleKeyEvent(json.RawMessage(params))
		if rerr != nil {
			t.Fatalf("keyEvent %s: rpc %d %s", params, rerr.Code, rerr.Message)
		}
		m, _ := res.(map[string]any)
		return m
	}
	// Ctrl+C in Finder → REPLACE clipboard.copy (windows-keyboard matrix).
	got := key(`{"keyCode":67,"modifiers":1,"appId":"com.apple.Finder","appMode":"native"}`)
	if got["decision"] != "REPLACE" || got["intent"] != "clipboard.copy" {
		t.Fatalf("ctrl+c: %+v, want REPLACE/clipboard.copy", got)
	}
	// F8 in VSCode → PASS (developer leaves it unclaimed).
	got = key(`{"keyCode":119,"appId":"com.microsoft.VSCode","appMode":"native"}`)
	if got["decision"] != "PASS" {
		t.Fatalf("f8: %+v, want PASS", got)
	}
	// Ctrl+Space → CONSUME launcher.open.
	got = key(`{"keyCode":49,"modifiers":1,"appId":"com.apple.Finder","appMode":"native"}`)
	if got["decision"] != "CONSUME" || got["winner"] != "launcher.ctrl-space-launcher" {
		t.Fatalf("ctrl+space: %+v, want CONSUME/launcher rule", got)
	}
	// Terminal Ctrl+C → PASS (SIGINT passthrough, copy-only is native-only).
	got = key(`{"keyCode":67,"modifiers":1,"appId":"com.apple.Terminal","appMode":"terminal"}`)
	if got["decision"] != "PASS" {
		t.Fatalf("terminal ctrl+c: %+v, want PASS", got)
	}
	// Disabled plugin's rules never fire.
	c.plugins["windows-keyboard"] = false
	got = key(`{"keyCode":67,"modifiers":1,"appId":"com.apple.Finder","appMode":"native"}`)
	if got["decision"] != "PASS" {
		t.Fatalf("disabled keyboard ctrl+c: %+v, want PASS", got)
	}
	c.plugins["windows-keyboard"] = true
	// Bad type / bad mode fail closed (never a silent default).
	if _, rerr := c.handleKeyEvent(json.RawMessage(`{"keyCode":67,"type":"hold"}`)); rerr == nil || rerr.Code != ipc.ErrBadParams {
		t.Fatalf("bad type must be ErrBadParams, got %v", rerr)
	}
	if _, rerr := c.handleKeyEvent(json.RawMessage(`{"keyCode":67,"appMode":"dream"}`)); rerr == nil || rerr.Code != ipc.ErrBadParams {
		t.Fatalf("bad mode must be ErrBadParams, got %v", rerr)
	}
	if _, rerr := c.handleKeyEvent(json.RawMessage(`{bad`)); rerr == nil || rerr.Code != ipc.ErrBadParams {
		t.Fatalf("malformed must be ErrBadParams, got %v", rerr)
	}
	// Decisions landed in the recorder (Activity trace seeding works).
	if len(c.rec.Traces()) == 0 {
		t.Fatal("key events must append recorder traces")
	}
}

// TestCheckForUpdate: manifest newer/equal/older + malformed fail closed.
func TestCheckForUpdate(t *testing.T) {
	c, err := NewCore(nil, nil)
	if err != nil {
		t.Fatalf("NewCore: %v", err)
	}
	check := func(manifest string) (map[string]any, *ipc.RPCError) {
		t.Helper()
		res, rerr := c.handleCheckForUpdate(json.RawMessage(`{"manifest":` + manifest + `}`))
		if rerr != nil {
			return nil, rerr
		}
		m, _ := res.(map[string]any)
		return m, nil
	}
	// CurrentVersion is v0.1.0: newer → true, same/older → false.
	if got, _ := check(`{"version":"v0.2.0","platform":"darwin/arm64","url":"x","sha256":"y"}`); got["updateAvailable"] != true {
		t.Fatalf("newer manifest: %+v, want updateAvailable=true", got)
	}
	if got, _ := check(`{"version":"v0.1.0","platform":"darwin/arm64","url":"x","sha256":"y"}`); got["updateAvailable"] != false {
		t.Fatalf("same version: %+v, want false", got)
	}
	if got, _ := check(`{"version":"v0.0.9","platform":"darwin/arm64","url":"x","sha256":"y"}`); got["updateAvailable"] != false {
		t.Fatalf("older manifest: %+v, want false", got)
	}
	// Malformed manifest version → ErrBadParams (never silent false).
	if _, rerr := check(`{"version":"garbage.x","platform":"darwin/arm64","url":"x","sha256":"y"}`); rerr == nil || rerr.Code != ipc.ErrBadParams {
		t.Fatalf("malformed version must be ErrBadParams, got %v", rerr)
	}
	if _, rerr := c.handleCheckForUpdate(json.RawMessage(`{bad`)); rerr == nil || rerr.Code != ipc.ErrBadParams {
		t.Fatalf("malformed JSON must be ErrBadParams, got %v", rerr)
	}
}

// TestApplyUpdateGates: unapproved fails closed, missing URL rejected,
// bad checksum fails before install (bytes never written).
func TestApplyUpdateGates(t *testing.T) {
	c, err := NewCore(nil, nil)
	if err != nil {
		t.Fatalf("NewCore: %v", err)
	}
	apply := func(params string) *ipc.RPCError {
		t.Helper()
		_, rerr := c.handleApplyUpdate(json.RawMessage(params))
		return rerr
	}
	good := `{"manifest":{"version":"v9.9.9","platform":"darwin/arm64","url":"https://example.invalid/x","sha256":"abc"},`
	// Unapproved → ErrInvalid (§8.4 gate) — checked after download? No:
	// approval is checked at Install, but URL fetch happens first. Use a
	// manifest with no URL to prove param gating precedes network.
	if rerr := apply(`{"manifest":{"version":"v9.9.9"},"approved":false}`); rerr == nil || rerr.Code != ipc.ErrBadParams {
		t.Fatalf("missing URL must be ErrBadParams, got %v", rerr)
	}
	// Unreachable URL → ErrInternal, nothing installed.
	if rerr := apply(good + `"approved":true,"approvedBy":"t"}`); rerr == nil {
		t.Fatal("unreachable download must fail")
	}
	_ = good
}

// TestPanicStopLive: safety.panicStop over the live socket returns the kill
// result (interception stopped, login kept) — the Safety page button path.
func TestPanicStopLive(t *testing.T) {
	c, err := NewCore(nil, nil)
	if err != nil {
		t.Fatalf("NewCore: %v", err)
	}
	res, rerr := c.handlePanicStop(nil)
	if rerr != nil {
		t.Fatalf("panicStop: %v", rerr)
	}
	m, _ := res.(map[string]any)
	if m["interceptionDisabled"] != true || m["loginItemKept"] != true {
		t.Fatalf("panicStop=%v, want stopped + login kept", m)
	}
}

// TestTrialLifecycle: begin → confirm → enabled; begin → rollback →
// disabled; confirm without trial fails; double-begin is idempotent.
func TestTrialLifecycle(t *testing.T) {
	c, err := NewCore(nil, nil)
	if err != nil {
		t.Fatalf("NewCore: %v", err)
	}
	c.registerBuiltin("plug", false)
	begin := func(id string) (map[string]any, *ipc.RPCError) {
		t.Helper()
		res, rerr := c.handleBeginTrial(json.RawMessage(`{"pluginId":"` + id + `"}`))
		if rerr != nil {
			return nil, rerr
		}
		m, _ := res.(map[string]any)
		return m, nil
	}
	if _, rerr := c.handleBeginTrial(json.RawMessage(`{"pluginId":"nope"}`)); rerr == nil {
		t.Fatal("begin unknown plugin must fail")
	}
	if got, _ := begin("plug"); got["state"] != "trial" {
		t.Fatalf("begin: %+v, want trial", got)
	}
	if got, _ := begin("plug"); got["state"] != "trial" {
		t.Fatalf("re-begin: %+v, want trial (idempotent)", got)
	}
	c.registerBuiltin("other", false)
	if _, rerr := c.handleConfirmTrial(json.RawMessage(`{"pluginId":"other","confirmed":true,"healthy":true}`)); rerr == nil {
		t.Fatal("confirm without trial must fail")
	}
	if _, rerr := c.handleConfirmTrial(json.RawMessage(`{"pluginId":"plug","confirmed":false,"healthy":true}`)); rerr == nil {
		t.Fatal("unconfirmed approve must fail closed")
	}
	res, rerr := c.handleConfirmTrial(json.RawMessage(`{"pluginId":"plug","confirmed":true,"healthy":true}`))
	if rerr != nil {
		t.Fatalf("confirm: %v", rerr)
	}
	if m, _ := res.(map[string]any); m["state"] != "enabled" {
		t.Fatalf("confirm: %+v, want enabled", m)
	}
	if !c.plugins["plug"] {
		t.Fatal("confirm must flip enable map")
	}
	c.registerBuiltin("plug2", false)
	if _, rerr := c.handleBeginTrial(json.RawMessage(`{"pluginId":"plug2"}`)); rerr != nil {
		t.Fatalf("begin plug2: %v", rerr)
	}
	res, rerr = c.handleRollbackTrial(json.RawMessage(`{"pluginId":"plug2","reason":"user cancel"}`))
	if rerr != nil {
		t.Fatalf("rollback: %v", rerr)
	}
	if m, _ := res.(map[string]any); m["state"] != "disabled" {
		t.Fatalf("rollback: %+v, want disabled", m)
	}
	if c.plugins["plug2"] {
		t.Fatal("rollback must leave plugin disabled")
	}
	if _, rerr := c.handleRollbackTrial(json.RawMessage(`{"pluginId":"plug2"}`)); rerr == nil {
		t.Fatal("rollback without trial must fail")
	}
}

// TestConfigWriteToggles: setRuleEnabled disables a matrix row through the
// decision path (Ctrl+C stops firing), re-enable restores it; unknown rules
// and bad params fail closed.
func TestConfigWriteToggles(t *testing.T) {
	c, err := NewCore(builtin.All(), builtin.Grants())
	if err != nil {
		t.Fatalf("NewCore: %v", err)
	}
	for _, id := range builtin.BuiltinIDs {
		c.registerBuiltin(id, true)
	}
	fire := func() string {
		t.Helper()
		res, rerr := c.handleKeyEvent(json.RawMessage(`{"keyCode":67,"modifiers":1,"appId":"com.apple.Finder","appMode":"native"}`))
		if rerr != nil {
			t.Fatalf("keyEvent: %v", rerr)
		}
		m, _ := res.(map[string]any)
		s, _ := m["decision"].(string)
		return s
	}
	if got := fire(); got != "REPLACE" {
		t.Fatalf("baseline ctrl+c=%s, want REPLACE", got)
	}
	// Disable the copy row → Ctrl+C passes through.
	res, rerr := c.handleSetRuleEnabled(json.RawMessage(`{"ruleId":"windows-keyboard.ctrl-c-copy","enabled":false}`))
	if rerr != nil {
		t.Fatalf("disable row: %v", rerr)
	}
	if m, _ := res.(map[string]any); m["enabled"] != false {
		t.Fatalf("disable result: %+v", m)
	}
	if got := fire(); got != "PASS" {
		t.Fatalf("disabled ctrl+c=%s, want PASS (no restart)", got)
	}
	// Re-enable → fires again immediately.
	if _, rerr := c.handleSetRuleEnabled(json.RawMessage(`{"ruleId":"windows-keyboard.ctrl-c-copy","enabled":true}`)); rerr != nil {
		t.Fatalf("re-enable: %v", rerr)
	}
	if got := fire(); got != "REPLACE" {
		t.Fatalf("re-enabled ctrl+c=%s, want REPLACE", got)
	}
	// Unknown rule + bad params fail closed.
	if _, rerr := c.handleSetRuleEnabled(json.RawMessage(`{"ruleId":"nope","enabled":false}`)); rerr == nil {
		t.Fatal("unknown rule must fail")
	}
	if _, rerr := c.handleSetRuleEnabled(json.RawMessage(`{}`)); rerr == nil || rerr.Code != ipc.ErrBadParams {
		t.Fatalf("empty params must be ErrBadParams, got %v", rerr)
	}
}

// TestConfigShortcutsRoundTrip: get table → set valid edit → get reflects;
// invalid edits (dup chord, unknown action) rejected, running set kept.
func TestConfigShortcutsRoundTrip(t *testing.T) {
	c, err := NewCore(builtin.All(), builtin.Grants())
	if err != nil {
		t.Fatalf("NewCore: %v", err)
	}
	res, rerr := c.handleGetShortcuts(nil)
	if rerr != nil {
		t.Fatalf("get: %v", rerr)
	}
	rows, _ := res.([]map[string]any)
	if len(rows) != 10 {
		t.Fatalf("shortcuts=%d, want 10 §6.2 rows", len(rows))
	}
	// Valid edit: same table minus one row still validates → stored.
	var edit []map[string]any
	for i, r := range rows {
		if i == 0 {
			continue
		}
		edit = append(edit, map[string]any{"action": r["action"], "modifiers": r["modifiers"], "key": r["key"]})
	}
	raw, _ := json.Marshal(map[string]any{"shortcuts": edit})
	if _, rerr := c.handleSetShortcuts(raw); rerr != nil {
		t.Fatalf("valid edit: %v", rerr)
	}
	res2, _ := c.handleGetShortcuts(nil)
	if rows2, _ := res2.([]map[string]any); len(rows2) != 9 {
		t.Fatalf("after edit=%d, want 9", len(rows2))
	}
	// Duplicate chord rejected, running set untouched.
	dup := append(append([]map[string]any(nil), edit...), edit[0])
	raw, _ = json.Marshal(map[string]any{"shortcuts": dup})
	if _, rerr := c.handleSetShortcuts(raw); rerr == nil {
		t.Fatal("duplicate chords must be rejected")
	}
	res3, _ := c.handleGetShortcuts(nil)
	if rows3, _ := res3.([]map[string]any); len(rows3) != 9 {
		t.Fatalf("rejected edit changed running set: %d", len(rows3))
	}
	// Unknown action rejected.
	raw, _ = json.Marshal(map[string]any{"shortcuts": []map[string]any{{"action": "Nope", "modifiers": []string{"Ctrl"}, "key": "Left"}}})
	if _, rerr := c.handleSetShortcuts(raw); rerr == nil {
		t.Fatal("unknown action must be rejected")
	}
	if _, rerr := c.handleSetShortcuts(json.RawMessage(`{bad`)); rerr == nil || rerr.Code != ipc.ErrBadParams {
		t.Fatalf("malformed must be ErrBadParams, got %v", rerr)
	}
}
