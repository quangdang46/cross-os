// IPC client — menu action → Unix socket → daemon.
//
// The extension is a minimal IPC client: it dials the daemon's Unix socket,
// writes one JSON-RPC 2.0 request, reads one response, all under a short
// deadline. Daemon-down (dial fails / deadline exceeds) returns DaemonDown
// and the CALLER hides the menu — the extension never blocks Finder, never
// retries in a loop, never queues (criterion 3).
//
// This is the same wire the Swift appex speaks (FinderSync.swift): newline-
// delimited JSON-RPC 2.0, one object per line. The previous bare
// {action,targetDir,ext,baseName} request and {ok,error} reply are gone —
// the daemon's method table is what answers now, and a shape invented on
// this side is a second protocol nothing else implements.
//
// Requests carry only data (method + params); the daemon decides and
// executes. The extension never sends executable code (§3.7). The method is
// checked against DaemonMethods — the daemon's own list — before the socket
// is dialled, so a caller cannot smuggle a path to run past a check that
// lives here.
package findersync

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"
)

// DialTimeout bounds the whole action round-trip: Finder menu handlers must
// return fast; a slow daemon must degrade to hide, not hang Finder.
const DialTimeout = 500 * time.Millisecond

// DefaultSocketPath is the conventional daemon socket location. It MUST match
// the Swift literal daemonSocketPath in FinderSync.swift (cross-language
// parity is convention-only — Go tests cannot assert on a Swift literal —
// so this named constant is the single Go-side reference; the README repeats
// the same string for the Xcode side). (review: cross-os-ed)
const DefaultSocketPath = "~/Library/Application Support/CrossOS/crossos.sock"

// Request is a JSON-RPC 2.0 call. ID is always present: the daemon rejects a
// notification, so omitting it would turn every send into a silent no-op.
type Request struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  Params `json:"params,omitempty"`
	ID      int    `json:"id"`
}

// Params is the union of what the eight appex methods read. One flat struct
// rather than one per method, because JSON-RPC params are an untyped object
// and the daemon reads the keys its own handler needs: finder.createFile
// looks for dir/ext/baseName and ignores the rest. The key names are the
// daemon's (core/cmd/crossos/findermenu.go) — a param spelled differently
// here is a missing-param refusal the user sees as a menu that does nothing.
type Params struct {
	// Context is the selection context finder.menuEntries was asked for.
	Context string `json:"context,omitempty"`
	// Dir is the folder a create acts in.
	Dir string `json:"dir,omitempty"`
	// Path is the single target openTerminal / openEditor act on.
	Path string `json:"path,omitempty"`
	// BaseDir is the folder copyRelativePath expresses Paths against.
	BaseDir string `json:"baseDir,omitempty"`
	// Paths is the resolved selection for copy / duplicate.
	Paths []string `json:"paths,omitempty"`
	// Ext and BaseName parametrize a new file; the daemon picks the
	// collision-free name and refuses an ext the catalog does not carry.
	Ext      string `json:"ext,omitempty"`
	BaseName string `json:"baseName,omitempty"`
	// EditorID is the catalog entry picked out of the Open in Editor
	// submenu — the bundle id, which is also what the daemon launches, so
	// the menu and the launch cannot spell an editor two different ways.
	EditorID string `json:"editorID,omitempty"`
}

// Response is a JSON-RPC 2.0 reply. A refusal is Error, never a result with
// ok:false — the daemon answering "here is a failure as data" is how a
// caller ends up treating a refusal as success.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
	ID      int             `json:"id"`
}

// RPCError is the daemon's refusal.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *RPCError) Error() string { return fmt.Sprintf("daemon refused (%d): %s", e.Code, e.Message) }

// Err returns the refusal, or nil on a successful call. Callers branch on
// this rather than reading Error directly so a nil-deref cannot look like a
// refusal and vice versa.
func (r Response) Err() error {
	if r.Error == nil {
		return nil
	}
	return r.Error
}

// Decode unmarshals a successful result into v. It is an error to call it on
// a refused response: the daemon sent an error, not a result, and decoding
// absent bytes would leave v zeroed and the caller would act on it.
func (r Response) Decode(v any) error {
	if err := r.Err(); err != nil {
		return err
	}
	if len(r.Result) == 0 {
		return fmt.Errorf("finder_sync: empty result for call %d", r.ID)
	}
	return json.Unmarshal(r.Result, v)
}

// DaemonDown marks the graceful-degradation path: dial failed or the daemon
// didn't answer in time. Callers hide the menu on this (never block Finder).
type DaemonDown struct{ Reason string }

func (e *DaemonDown) Error() string { return "daemon down: " + e.Reason }

// Send delivers one JSON-RPC call to the daemon at socketPath and returns its
// response. Any transport failure maps to *DaemonDown — never a raw net
// error, so callers branch on one type. A refusal is NOT a transport failure
// and comes back in the response for the caller to read.
func Send(socketPath string, req Request) (Response, error) {
	if !DaemonMethods[req.Method] {
		return Response{}, fmt.Errorf("method %q not in the daemon method list (never send code)", req.Method)
	}
	if req.JSONRPC == "" {
		req.JSONRPC = "2.0"
	}
	conn, err := net.DialTimeout("unix", socketPath, DialTimeout)
	if err != nil {
		return Response{}, &DaemonDown{Reason: err.Error()}
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(DialTimeout))
	enc := json.NewEncoder(conn)
	if err := enc.Encode(req); err != nil {
		return Response{}, &DaemonDown{Reason: "encode: " + err.Error()}
	}
	var resp Response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return Response{}, &DaemonDown{Reason: "decode: " + err.Error()}
	}
	return resp, nil
}

// IsDaemonDown reports whether err is the graceful-degradation signal.
func IsDaemonDown(err error) bool {
	var dd *DaemonDown
	return errors.As(err, &dd)
}
