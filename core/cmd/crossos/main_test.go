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
