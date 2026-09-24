// Spike D tests — bead cross-os-3c6.
//
// Tier 1 (always runs): menu snapshot, IPC round-trip against a stub daemon,
// daemon-down hide — all on loopback Unix sockets, no Finder needed.
// Tier 2 (real Finder menu): gated behind -short; needs an interactive
// desktop with the appex installed.
//
// Pass criteria mapping:
//  1. Menu item appears → TestBuildMenu + Tier-2 TestLiveMenu.
//  2. Action reaches stub daemon → TestSendRoundTrip.
//  3. Daemon-down hides gracefully → TestDaemonDownHide (+ TestSendDaemonDown).
//  4. Requests only, never code → TestClosedVerbSet.
package findersync

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestBuildMenu(t *testing.T) {
	m, err := BuildMenu([]MenuEntry{
		{Title: "New Markdown", Action: Verb("newFile"), Ext: "md"},
		{Title: "Copy Path", Action: Verb("copyPath")},
	})
	if err != nil {
		t.Fatalf("BuildMenu: %v", err)
	}
	if m.Len() != 2 {
		t.Fatalf("Len=%d, want 2", m.Len())
	}
	e, err := m.Lookup(0)
	if err != nil {
		t.Fatalf("Lookup(0): %v", err)
	}
	if e.Title != "New Markdown" || e.Action != Verb("newFile") {
		t.Fatalf("Lookup(0)=%+v, want New Markdown/%s", e, Verb("newFile"))
	}
	if _, err := m.Lookup(99); err == nil {
		t.Fatal("Lookup(99): want out-of-range error, got nil")
	}
	if _, err := m.Lookup(-1); err == nil {
		t.Fatal("Lookup(-1): want out-of-range error, got nil")
	}
	// Empty input → single guide row, never a dead empty menu.
	empty, err := BuildMenu(nil)
	if err != nil {
		t.Fatalf("BuildMenu(nil): %v", err)
	}
	if empty.Len() != 1 {
		t.Fatalf("empty menu Len=%d, want 1 guide row", empty.Len())
	}
	if _, err := BuildMenu([]MenuEntry{{Title: "", Action: Verb("newFile")}}); err == nil {
		t.Fatal("empty title: want validation error, got nil")
	}
}

func TestClosedVerbSet(t *testing.T) {
	// Requests carry verbs + data only. Anything outside the closed set —
	// script paths, shell strings, code blobs — is rejected BEFORE dial.
	if _, err := BuildMenu([]MenuEntry{{Title: "Evil", Action: "/bin/sh -c pwn"}}); err == nil {
		t.Fatal("menu with code action: want rejection, got nil")
	}
	if _, err := Send(sockPath(t), Request{Method: "eval", Params: Params{Dir: "/tmp"}}); err == nil || IsDaemonDown(err) {
		t.Fatalf("Send(eval): want closed-set rejection, got %v", err)
	}
	// The row id is not the method. "newFile" is a row in the menu table;
	// the call the daemon answers is finder.createFile, and sending the id
	// would be a "no such method" the user sees as a dead menu item.
	if _, err := Send(sockPath(t), Request{Method: "newFile", Params: Params{Dir: "/tmp"}}); err == nil || IsDaemonDown(err) {
		t.Fatalf("Send(newFile): want closed-set rejection, got %v", err)
	}
}

// stubDaemon serves one request→response on socketPath, then closes. The
// reply is a real JSON-RPC result, so the client has to decode one.
func stubDaemon(t *testing.T, socketPath string, got chan<- Request, errCh chan<- string) {
	t.Helper()
	_ = os.Remove(socketPath)
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		errCh <- "listen: " + err.Error()
		return
	}
	defer ln.Close()
	conn, err := ln.Accept()
	if err != nil {
		return
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	var req Request
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		errCh <- "stub decode: " + err.Error()
		return
	}
	got <- req
	_ = json.NewEncoder(conn).Encode(Response{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  json.RawMessage(`{"items":[]}`),
	})
}

func sockPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "crossos-spike-d.sock")
}

func TestSendRoundTrip(t *testing.T) {
	path := sockPath(t)
	got := make(chan Request, 1)
	stubErrRT := make(chan string, 1)
	go stubDaemon(t, path, got, stubErrRT)
	var resp Response
	var err error
	for i := 0; i < 50; i++ {
		resp, err = Send(path, Request{
			Method: Verb("newFile"),
			Params: Params{Dir: "/tmp", Ext: "md", BaseName: "Untitled"},
			ID:     1,
		})
		if err == nil {
			break
		}
		if !IsDaemonDown(err) {
			t.Fatalf("Send: unexpected non-daemon error: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("Send after retries: %v", err)
	}
	if err := resp.Err(); err != nil {
		t.Fatalf("resp=%+v, want no refusal", resp)
	}
	var payload struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := resp.Decode(&payload); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	select {
	case req := <-got:
		want := Verb("newFile")
		if req.Method != want || req.JSONRPC != "2.0" {
			t.Fatalf("stub got %+v, want jsonrpc 2.0 method %s", req, want)
		}
		// finder.createFile reads dir/ext/baseName — a create names the
		// folder it lands in, not a path list.
		if req.Params.Dir != "/tmp" || req.Params.Ext != "md" ||
			req.Params.BaseName != "Untitled" {
			t.Fatalf("stub got params %+v, want dir=/tmp ext=md baseName=Untitled", req.Params)
		}
		if req.ID != 1 {
			t.Fatalf("stub got id %d, want 1 (JSON-RPC needs an id)", req.ID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("stub daemon never received the request")
	}
}

func TestSendDaemonDown(t *testing.T) {
	// Nothing listening: Send must return the typed DaemonDown, fast.
	start := time.Now()
	_, err := Send(filepath.Join(t.TempDir(), "no-daemon.sock"),
		Request{Method: Verb("newFile"), Params: Params{Dir: "/tmp"}, ID: 1})
	if !IsDaemonDown(err) {
		t.Fatalf("want *DaemonDown, got %v", err)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("daemon-down took %v — must return fast, never block", d)
	}
}

func TestDaemonDownHide(t *testing.T) {
	// Daemon up → Show; daemon gone → Hide with a reason for stage logs.
	path := sockPath(t)
	got := make(chan Request, 1)
	stubErrHide := make(chan string, 1)
	go stubDaemon(t, path, got, stubErrHide)
	var vis Visibility
	var reason string
	for i := 0; i < 50; i++ {
		vis, reason = ShouldShowMenu(path)
		if vis == Show {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if vis != Show {
		t.Fatalf("daemon up: want Show, got Hide(%s)", reason)
	}
	vis, reason = ShouldShowMenu(filepath.Join(t.TempDir(), "gone.sock"))
	if vis != Hide {
		t.Fatal("daemon down: want Hide, got Show")
	}
	if reason == "" {
		t.Fatal("Hide must carry a reason for stage logs")
	}
	t.Logf("daemon-down hides menu: %s (Finder never blocks)", reason)
}

// --- Tier 2: real Finder menu (interactive desktop + installed appex) ---

func TestLiveMenu(t *testing.T) {
	if testing.Short() {
		t.Skip("needs interactive desktop + installed appex")
	}
	if runtime.GOOS != "darwin" {
		t.Skip("darwin-only")
	}
	t.Skip("live appex harness lands with cross-os-vbl.1; Tier-1 menu+IPC+hide math is the sandbox-runnable proof")
}
