// IPC client tests — bead cross-os-80g bridge criterion.
//
// Pass criteria mapping:
//  1. Bridge calls reach Core over IPC → TestIPCClientRoundTrip (in-memory
//     net.Pipe pair speaking the §3.9 framing against a stub server) +
//     TestIPCClientFraming (byte-level request shape matches core/pkg/ipc).
//  2. Bridge/IPC failures in UI logs, never silent → TestIPCClientDialFailure
//     (unreachable transport surfaces a typed error the bridge logs).
package shell

import (
	"encoding/json"
	"net"
	"strings"
	"testing"
)

// pipeTransport dials one end of an in-memory net.Pipe whose far end is
// served by the test's stub handler.
type pipeTransport struct {
	serve func(conn net.Conn)
}

func (p pipeTransport) Dial() (net.Conn, error) {
	a, b := net.Pipe()
	go p.serve(b)
	return a, nil
}

// stubServer answers core.status / plugin.list / plugin.setEnabled /
// core.reset / core.eventLogs with canned payloads.
func stubServer(conn net.Conn) {
	defer conn.Close()
	buf := make([]byte, 1)
	var line []byte
	for {
		n, err := conn.Read(buf)
		if n > 0 {
			line = append(line, buf[:n]...)
			if buf[0] != '\n' {
				continue
			}
			var req struct {
				Method string          `json:"method"`
				ID     any             `json:"id"`
				Params json.RawMessage `json:"params,omitempty"`
			}
			var resp string
			if jerr := json.Unmarshal(line, &req); jerr != nil {
				resp = `{"jsonrpc":"2.0","error":{"code":-32700,"message":"parse error"},"id":null}` + "\n"
			} else {
				resp = routeStub(req.Method, req.ID)
			}
			line = line[:0]
			if _, werr := conn.Write([]byte(resp)); werr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

func routeStub(method string, id any) string {
	enc := func(v any) string {
		raw, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "result": v, "id": id})
		return string(raw) + "\n"
	}
	errResp := func(code int, msg string) string {
		raw, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "error": map[string]any{"code": code, "message": msg}, "id": id})
		return string(raw) + "\n"
	}
	switch method {
	case "core.status":
		return enc(map[string]any{"running": true, "safe_mode": false, "killed": false})
	case "plugin.list":
		return enc([]PluginState{{ID: "win-kb", Enabled: true, Healthy: "healthy"}})
	case "plugin.setEnabled":
		return enc(true)
	case "core.reset":
		return enc([]string{"remove login item", "disable extension", "clean owned state"})
	case "core.eventLogs":
		return enc([]string{"e1"})
	default:
		return errResp(-32601, "no such method: "+method)
	}
}

func TestIPCClientRoundTrip(t *testing.T) {
	// Bridge calls reach Core over IPC: App bound to IPCCore over the pipe
	// serves Dashboard payloads end to end.
	app := NewApp(NewIPCCore(pipeTransport{stubServer}))
	st := app.GetStatus()
	if !st.Running || len(st.Plugins) != 1 || st.Plugins[0].ID != "win-kb" {
		t.Fatalf("GetStatus over IPC=%+v, want running + win-kb", st)
	}
	if err := app.TogglePlugin("win-kb", false); err != nil {
		t.Fatalf("TogglePlugin over IPC: %v", err)
	}
	if got := app.GetEventLogs(); len(got) != 1 || got[0] != "e1" {
		t.Fatalf("GetEventLogs over IPC=%v, want [e1]", got)
	}
	if got := app.ResetEverything(); len(got) != 3 {
		t.Fatalf("ResetEverything over IPC=%v, want 3 steps", got)
	}
}

func TestIPCClientFraming(t *testing.T) {
	// Byte-level contract: the request line carries jsonrpc/method/id and a
	// trailing newline (same framing core/pkg/ipc serves).
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	go func() {
		buf := make([]byte, 1)
		var line []byte
		for {
			n, err := b.Read(buf)
			if n > 0 {
				line = append(line, buf[:n]...)
				if buf[0] == '\n' {
					break
				}
			}
			if err != nil {
				return
			}
		}
		var req struct {
			JSONRPC string `json:"jsonrpc"`
			Method  string `json:"method"`
			ID      any    `json:"id"`
		}
		if jerr := json.Unmarshal(line, &req); jerr != nil {
			return
		}
		if req.JSONRPC != "2.0" || req.Method != "core.status" || req.ID == nil {
			return
		}
		b.Write([]byte(`{"jsonrpc":"2.0","result":{"running":true},"id":1}` + "\n"))
	}()
	c := NewIPCCore(dialFunc(func() (net.Conn, error) { return a, nil }))
	if !c.IsRunning() {
		t.Fatal("framing round trip failed")
	}
}

type dialFunc func() (net.Conn, error)

func (f dialFunc) Dial() (net.Conn, error) { return f() }

func TestIPCClientDialFailure(t *testing.T) {
	// Unreachable daemon: typed error for the bridge to log (never a hang,
	// never a silent blank page).
	bad := NewIPCCore(dialFunc(func() (net.Conn, error) {
		return nil, errDialRefused
	}))
	app := NewApp(bad)
	if err := app.TogglePlugin("win-kb", false); err == nil {
		t.Fatal("want bridge error on dead transport, got nil")
	}
	// Unknown plugin IDs still fail locally even when transport is live.
	live := NewApp(NewIPCCore(pipeTransport{stubServer}))
	if err := live.TogglePlugin("ghost-plugin", true); err == nil {
		t.Fatal("want unknown-plugin error, got nil")
	}
	if len(live.UILogs()) == 0 {
		t.Fatal("unknown-plugin failure must land in UI logs")
	}
	// GetStatus degrades (not running) rather than panicking on dead transport.
	dead := NewApp(bad)
	if st := dead.GetStatus(); st.Running {
		t.Fatal("dead transport must not report running")
	}
	if got := dead.GetEventLogs(); len(got) != 0 {
		t.Fatalf("dead transport logs=%v, want empty (with UI note)", got)
	}
	if len(dead.UILogs()) == 0 {
		t.Fatal("dead-transport degradation must note UI logs")
	}
}

// errDialRefused stands in for a refused socket/pipe dial.
var errDialRefused = errDialRefusedType{}

type errDialRefusedType struct{}

func (errDialRefusedType) Error() string { return "dial refused (daemon unreachable)" }

func TestIPCClientUnknownMethod(t *testing.T) {
	// RPC-level errors propagate as typed errors (not silent nils).
	c := NewIPCCore(pipeTransport{stubServer})
	if _, err := c.call("core.nope", nil); err == nil {
		t.Fatal("want RPC error, got nil")
	} else if !strings.Contains(err.Error(), "-32601") {
		t.Fatalf("want -32601 code, got %v", err)
	}
}
