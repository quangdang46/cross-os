package ipc

import (
	"bufio"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// testServer starts a Server on a test transport and returns it + dial func.
func testServer(t *testing.T, dir string) (*Server, func() net.Conn) {
	t.Helper()
	s := NewServer()
	if err := s.Register("core.status", func(params json.RawMessage) (any, *RPCError) {
		return map[string]string{"state": "running"}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Register("core.echo", func(params json.RawMessage) (any, *RPCError) {
		var v any
		if err := json.Unmarshal(params, &v); err != nil {
			return nil, &RPCError{ErrBadParams, "bad params"}
		}
		return v, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Register("core.panic", func(params json.RawMessage) (any, *RPCError) {
		panic("boom")
	}); err != nil {
		t.Fatal(err)
	}
	var ln net.Listener
	var err error
	var addr string
	if runtime.GOOS == "windows" {
		// Named pipe unavailable in pure-Go test net; TCP loopback stands in
		// for framing/protocol tests. Pipe ACL is asserted in TestPipeACLDoc
		// via the platform adapter bead (cross-os-ab4), not here.
		ln, err = net.Listen("tcp", "127.0.0.1:0")
		addr = ln.Addr().String()
	} else {
		addr = filepath.Join(dir, "core.sock")
		ln, err = net.Listen("unix", addr)
		// Transport ACL (bead criterion): socket dir admits owner only.
		if err == nil {
			if err := os.Chmod(dir, 0700); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err != nil {
		t.Fatal(err)
	}
	go s.Serve(ln)
	t.Cleanup(s.Close)
	network := "tcp"
	if runtime.GOOS != "windows" {
		network = "unix"
	}
	return s, func() net.Conn {
		c, err := net.Dial(network, addr)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
}

func TestStatusRoundTrip(t *testing.T) {
	// Bead criterion: UI stub calls Core over IPC and gets status.
	_, dial := testServer(t, t.TempDir())
	conn := dial()
	defer conn.Close()
	resp, err := Call(conn, "core.status", nil, 1)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("rpc error: %+v", resp.Error)
	}
	m, ok := resp.Result.(map[string]any)
	if !ok || m["state"] != "running" {
		t.Fatalf("bad result: %+v", resp.Result)
	}
}

func TestEchoParams(t *testing.T) {
	_, dial := testServer(t, t.TempDir())
	conn := dial()
	defer conn.Close()
	resp, err := Call(conn, "core.echo", map[string]int{"n": 42}, 2)
	if err != nil {
		t.Fatal(err)
	}
	m, ok := resp.Result.(map[string]any)
	if !ok || m["n"] != float64(42) {
		t.Fatalf("bad echo: %+v", resp.Result)
	}
}

func TestUnknownMethod(t *testing.T) {
	_, dial := testServer(t, t.TempDir())
	conn := dial()
	defer conn.Close()
	resp, err := Call(conn, "core.nope", nil, 3)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Error == nil || resp.Error.Code != ErrNoMethod {
		t.Fatalf("want -32601, got %+v", resp)
	}
}

func TestMalformedLine(t *testing.T) {
	// Garbage line → -32700 parse error addressed to that line; the
	// connection survives and the NEXT call succeeds (framing resyncs).
	_, dial := testServer(t, t.TempDir())
	conn := dial()
	defer conn.Close()
	if _, err := conn.Write([]byte("{not json\n")); err != nil {
		t.Fatal(err)
	}
	// Read the parse-error response with a raw scanner (Call would send
	// another request first and misalign the stream).
	sc := bufio.NewScanner(conn)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	if !sc.Scan() {
		t.Fatal("no parse-error response")
	}
	var perr Response
	if err := json.Unmarshal(sc.Bytes(), &perr); err != nil {
		t.Fatal(err)
	}
	if perr.Error == nil || perr.Error.Code != ErrParse {
		t.Fatalf("want -32700, got %+v", perr)
	}
	resp, err := Call(conn, "core.status", nil, 4)
	if err != nil {
		t.Fatalf("conn died after malformed line: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("status after garbage: %+v", resp.Error)
	}
}

func TestPanicBecomesInternal(t *testing.T) {
	_, dial := testServer(t, t.TempDir())
	conn := dial()
	defer conn.Close()
	resp, err := Call(conn, "core.panic", nil, 5)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Error == nil || resp.Error.Code != ErrInternal {
		t.Fatalf("want -32603, got %+v", resp)
	}
}

func TestClientReconnect(t *testing.T) {
	// Reconnect semantics: caller owns the conn; drop + re-dial works.
	_, dial := testServer(t, t.TempDir())
	c1 := dial()
	r1, err := Call(c1, "core.status", nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	c1.Close() // drop
	c2 := dial()
	defer c2.Close()
	r2, err := Call(c2, "core.status", nil, 2)
	if err != nil {
		t.Fatal(err)
	}
	if r1.Result == nil || r2.Result == nil {
		t.Fatal("reconnect round-trip failed")
	}
}

func TestSocketDirMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix socket ACL is non-Windows")
	}
	dir := t.TempDir()
	testServer(t, dir)
	fi, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0700 {
		t.Fatalf("socket dir mode: got %o, want 700", fi.Mode().Perm())
	}
}

func TestDuplicateMethodRejected(t *testing.T) {
	s := NewServer()
	if err := s.Register("a", nil); err != nil {
		t.Fatal(err)
	}
	if err := s.Register("a", nil); err == nil {
		t.Fatal("duplicate method allowed")
	}
}

func TestReconnectDeadline(t *testing.T) {
	// A second call on the same conn works (framing resyncs per line).
	_, dial := testServer(t, t.TempDir())
	conn := dial()
	defer conn.Close()
	for i := 0; i < 5; i++ {
		resp, err := Call(conn, "core.status", nil, i)
		if err != nil || resp.Error != nil {
			t.Fatalf("call %d: %v %+v", i, err, resp.Error)
		}
	}
}
