// IPC client — menu action → Unix socket → stub daemon.
//
// The extension is a minimal IPC client: on action it dials the daemon's Unix
// socket, sends one JSON request, reads one JSON response, with a short
// deadline. Daemon-down (dial fails / deadline exceeds): the client returns
// DaemonDown and the CALLER hides the menu — the extension never blocks
// Finder, never retries in a loop, never queues (criterion 3).
//
// Requests carry only data (verb + target dir + file params); the daemon
// decides and executes. The extension never sends executable code (§3.7).
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

// Request is the only shape the extension ever sends.
type Request struct {
	// Action is a closed-set verb (see allowedActions).
	Action string `json:"action"`
	// TargetDir is the Finder targetedURL / selected item dir.
	TargetDir string `json:"targetDir"`
	// Ext + BaseName parametrize createFile.
	Ext      string `json:"ext,omitempty"`
	BaseName string `json:"baseName,omitempty"`
}

// Response is the daemon's single reply.
type Response struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// DaemonDown marks the graceful-degradation path: dial failed or the daemon
// didn't answer in time. Callers hide the menu on this (never block Finder).
type DaemonDown struct{ Reason string }

func (e *DaemonDown) Error() string { return "daemon down: " + e.Reason }

// Send delivers one request to the daemon at socketPath and returns its
// response. Any transport failure maps to *DaemonDown — never a raw net
// error, so callers branch on one type.
func Send(socketPath string, req Request) (Response, error) {
	if !allowedActions[req.Action] {
		return Response{}, fmt.Errorf("request action %q not in closed verb set (never send code)", req.Action)
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
