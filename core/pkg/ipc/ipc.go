package ipc

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"sync"
)

// Request is a JSON-RPC 2.0 request (notifications — no id — are rejected;
// every call gets a response so callers never hang wondering).
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	ID      any             `json:"id"`
}

// Response is a JSON-RPC 2.0 response.
type Response struct {
	JSONRPC string    `json:"jsonrpc"`
	Result  any       `json:"result,omitempty"`
	Error   *RPCError `json:"error,omitempty"`
	ID      any       `json:"id"`
}

// RPCError is a JSON-RPC 2.0 error object.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Standard error codes.
const (
	ErrParse     = -32700
	ErrInvalid   = -32600
	ErrNoMethod  = -32601
	ErrBadParams = -32602
	ErrInternal  = -32603
)

// Handler serves one method. Params arrive raw; the handler validates them.
type Handler func(params json.RawMessage) (any, *RPCError)

// Server is the JSON-RPC 2.0 server over a bidirectional stream transport.
// One Server per listener; connections are independent goroutines sharing
// only the method table (registered before Serve, read-only after).
type Server struct {
	mu       sync.RWMutex
	methods  map[string]Handler
	listener net.Listener
	wg       sync.WaitGroup
	closed   chan struct{}
}

// NewServer returns a Server with no methods. Register methods, then Serve.
func NewServer() *Server {
	return &Server{methods: map[string]Handler{}, closed: make(chan struct{})}
}

// Register adds a method. Register after Close is rejected. Registration
// during Serve is mutex-safe but discouraged — register everything before
// Serve. (review correction: cross-os-c0 — the old comment claimed a
// post-Serve rejection that the code never enforced.)
func (s *Server) Register(method string, h Handler) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	select {
	case <-s.closed:
		return fmt.Errorf("ipc: register after close")
	default:
	}
	if _, dup := s.methods[method]; dup {
		return fmt.Errorf("ipc: duplicate method %q", method)
	}
	s.methods[method] = h
	return nil
}

// Serve accepts connections on ln until Close. Each connection runs its own
// line-delimited JSON-RPC loop.
func (s *Server) Serve(ln net.Listener) {
	s.mu.Lock()
	s.listener = ln
	s.mu.Unlock()
	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-s.closed:
				return
			default:
				continue
			}
		}
		s.wg.Add(1)
		go s.serveConn(conn)
	}
}

// Close stops accepting; in-flight calls run to completion (waited).
func (s *Server) Close() {
	select {
	case <-s.closed:
		return
	default:
		close(s.closed)
	}
	s.mu.RLock()
	ln := s.listener
	s.mu.RUnlock()
	if ln != nil {
		ln.Close()
	}
	s.wg.Wait()
}

func (s *Server) serveConn(conn net.Conn) {
	defer s.wg.Done()
	defer conn.Close()
	sc := bufio.NewScanner(conn)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	w := bufio.NewWriter(conn)
	for sc.Scan() {
		line := sc.Bytes()
		resp := s.handle(line)
		raw, err := json.Marshal(resp)
		if err != nil {
			continue
		}
		raw = append(raw, '\n')
		if _, err := w.Write(raw); err != nil {
			return
		}
		w.Flush()
	}
}

// handle processes one request line. Never panics across the handler
// boundary — a panicking handler becomes ErrInternal, not a dead conn.
func (s *Server) handle(line []byte) (resp Response) {
	resp.JSONRPC = "2.0"
	var req Request
	if err := json.Unmarshal(line, &req); err != nil {
		return Response{JSONRPC: "2.0",
			Error: &RPCError{ErrParse, "parse error"}, ID: nil}
	}
	resp.ID = req.ID
	defer func() {
		if r := recover(); r != nil {
			resp.Result = nil
			resp.Error = &RPCError{ErrInternal, fmt.Sprintf("internal: %v", r)}
		}
	}()
	if req.JSONRPC != "2.0" || req.Method == "" || req.ID == nil {
		resp.Error = &RPCError{ErrInvalid, "invalid request (need jsonrpc/method/id)"}
		return resp
	}
	// Per-call RLock: cheap at this scale, and registration stays legal
	// during Serve (discouraged, not rejected — see Register).
	s.mu.RLock()
	h, ok := s.methods[req.Method]
	s.mu.RUnlock()
	if !ok {
		resp.Error = &RPCError{ErrNoMethod, "no such method: " + req.Method}
		return resp
	}
	result, rerr := h(req.Params)
	if rerr != nil {
		resp.Error = rerr
		return resp
	}
	resp.Result = result
	return resp
}

// Call performs one JSON-RPC call over conn (client side, used by tests and
// the future UI/plugin clients). Reconnect semantics: Call opens no state —
// the caller owns the conn and re-dials on error (see TestClientReconnect).
//
// WARNING: Call builds a fresh bufio.Scanner per call. Scanner read-ahead
// can swallow bytes belonging to the NEXT response on a persistent conn —
// sequential use is safe (server never pipelines), but UI/Level-B clients
// must use a persistent caller-owned reader, not this shape.
// (review: cross-os-c0)
func Call(conn net.Conn, method string, params any, id any) (Response, error) {
	praw, err := json.Marshal(params)
	if err != nil {
		return Response{}, err
	}
	req := Request{JSONRPC: "2.0", Method: method, Params: praw, ID: id}
	raw, err := json.Marshal(req)
	if err != nil {
		return Response{}, err
	}
	raw = append(raw, '\n')
	if _, err := conn.Write(raw); err != nil {
		return Response{}, err
	}
	sc := bufio.NewScanner(conn)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	if !sc.Scan() {
		if err := sc.Err(); err != nil {
			return Response{}, err
		}
		return Response{}, fmt.Errorf("ipc: connection closed")
	}
	var resp Response
	if err := json.Unmarshal(sc.Bytes(), &resp); err != nil {
		return Response{}, err
	}
	return resp, nil
}
